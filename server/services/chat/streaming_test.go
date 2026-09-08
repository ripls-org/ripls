package chat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestRegisterStream(t *testing.T) {
	svc, _ := setupTestChatService(t)

	conversationID := "test-conversation"
	ch := make(chan *api.StreamMessagesResponse, 10)

	// Register a stream
	svc.registerStream(context.Background(), conversationID, "user123", ch)

	// Verify stream was registered
	svc.streamsMu.RLock()
	streams := svc.streams[conversationID]
	svc.streamsMu.RUnlock()

	if len(streams) != 1 {
		t.Errorf("Expected 1 stream registered, got %d", len(streams))
	}
}

func TestUnregisterStream(t *testing.T) {
	svc, _ := setupTestChatService(t)

	conversationID := "test-conversation"
	ch := make(chan *api.StreamMessagesResponse, 10)

	// Register and then unregister
	svc.registerStream(context.Background(), conversationID, "user123", ch)
	svc.unregisterStream(context.Background(), conversationID, ch)

	// Verify stream was unregistered and cleaned up
	svc.streamsMu.RLock()
	_, exists := svc.streams[conversationID]
	svc.streamsMu.RUnlock()

	if exists {
		t.Error("Expected conversation to be removed from streams map after last stream unregistered")
	}
}

func TestBroadcastMessage_SingleStream(t *testing.T) {
	svc, _ := setupTestChatService(t)

	conversationID := "test-conversation"
	ch := make(chan *api.StreamMessagesResponse, 10)

	// Register stream
	svc.registerStream(context.Background(), conversationID, "user123", ch)

	// Broadcast a message (create user message variant)
	event := &api.StreamMessagesResponse{
		MessageId:      "msg123",
		ConversationId: conversationID,
		SentAtUnixSec:  1000,
	}
	event.Message = &api.StreamMessagesResponse_UserMessage{
		UserMessage: &api.UserMessage{
			Sender: &api.User{Id: "user456", Name: "Test User"},
			Text:   "Test message",
		},
	}

	svc.broadcastMessage(context.Background(), conversationID, event)

	// Verify stream received the message
	if len(ch) != 1 {
		t.Fatalf("Expected 1 message in stream, got %d", len(ch))
	}

	receivedMsg := <-ch
	if receivedMsg.GetUserMessage() == nil {
		t.Error("Expected user message variant")
	} else if receivedMsg.GetUserMessage().Text != "Test message" {
		t.Errorf("Expected 'Test message', got '%s'", receivedMsg.GetUserMessage().Text)
	}

	if receivedMsg.MessageId != "msg123" {
		t.Errorf("Expected message ID 'msg123', got '%s'", receivedMsg.MessageId)
	}
}

func TestBroadcastMessage_MultipleStreams(t *testing.T) {
	svc, _ := setupTestChatService(t)

	conversationID := "test-conversation"
	ch1 := make(chan *api.StreamMessagesResponse, 10)
	ch2 := make(chan *api.StreamMessagesResponse, 10)
	ch3 := make(chan *api.StreamMessagesResponse, 10)

	// Register three streams (simulating three devices)
	svc.registerStream(context.Background(), conversationID, "user1", ch1)
	svc.registerStream(context.Background(), conversationID, "user2", ch2)
	svc.registerStream(context.Background(), conversationID, "user3", ch3)

	// Verify all streams registered
	svc.streamsMu.RLock()
	streamCount := len(svc.streams[conversationID])
	svc.streamsMu.RUnlock()

	if streamCount != 3 {
		t.Fatalf("Expected 3 streams registered, got %d", streamCount)
	}

	// Broadcast a message (create user message variant)
	event := &api.StreamMessagesResponse{
		MessageId:      "broadcast123",
		ConversationId: conversationID,
		SentAtUnixSec:  1000,
	}
	event.Message = &api.StreamMessagesResponse_UserMessage{
		UserMessage: &api.UserMessage{
			Sender: &api.User{Id: "user1", Name: "Test User"},
			Text:   "Message to all devices",
		},
	}

	svc.broadcastMessage(context.Background(), conversationID, event)

	// Verify all three streams received the message
	for i, ch := range []chan *api.StreamMessagesResponse{ch1, ch2, ch3} {
		if len(ch) != 1 {
			t.Errorf("Stream %d: Expected 1 message, got %d", i+1, len(ch))
			continue
		}

		msg := <-ch
		if msg.GetUserMessage() == nil {
			t.Errorf("Stream %d: Expected user message variant", i+1)
		} else if msg.GetUserMessage().Text != "Message to all devices" {
			t.Errorf("Stream %d: Expected 'Message to all devices', got '%s'", i+1, msg.GetUserMessage().Text)
		}
	}
}

func TestSendMessage_BroadcastsToRegisteredStreams(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	// Create community and add lender and borrower as members
	communityID := createTestCommunity(t, sqlStorage, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "lender1")
	createTestCommunityMembership(t, sqlStorage, communityID, "borrower1")

	loanID := createTestLoan(t, sqlStorage, "lender1", "borrower1")
	createTestUsers(t, sqlStorage, "lender1", "Lender")

	// Start conversation
	lenderCtx := contextWithAuth("lender1", "lender@example.com")
	startReq := connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	})
	startResp, _ := svc.StartConversation(lenderCtx, startReq)

	// Register two streams for the conversation (simulating two devices listening)
	ch1 := make(chan *api.StreamMessagesResponse, 10)
	ch2 := make(chan *api.StreamMessagesResponse, 10)
	svc.registerStream(context.Background(), startResp.Msg.ConversationId, "borrower1", ch1)
	svc.registerStream(context.Background(), startResp.Msg.ConversationId, "borrower1", ch2)

	// Send a message
	sendReq := connect.NewRequest(&api.SendMessageRequest{
		ConversationId: startResp.Msg.ConversationId,
		Text:           "Real message test",
	})
	_, err := svc.SendMessage(lenderCtx, sendReq)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Wait for async bus fan-out to complete before asserting channel contents.
	drainChatBus(t, svc)

	// Both streams should have received the message
	if len(ch1) != 1 {
		t.Errorf("Stream 1: Expected 1 message, got %d", len(ch1))
	}

	if len(ch2) != 1 {
		t.Errorf("Stream 2: Expected 1 message, got %d", len(ch2))
	}

	// Verify message content in first stream
	if len(ch1) > 0 {
		msg := <-ch1
		if msg.GetUserMessage() == nil {
			t.Error("Expected user message variant")
		} else {
			if msg.GetUserMessage().Text != "Real message test" {
				t.Errorf("Expected 'Real message test', got '%s'", msg.GetUserMessage().Text)
			}
			if msg.GetUserMessage().Sender == nil || msg.GetUserMessage().Sender.Id != "lender1" {
				if msg.GetUserMessage().Sender == nil {
					t.Errorf("Expected sender 'lender1', got nil")
				} else {
					t.Errorf("Expected sender 'lender1', got '%s'", msg.GetUserMessage().Sender.Id)
				}
			}
		}
	}
}

func TestBroadcastMessage_NoStreamsRegistered(t *testing.T) {
	svc, _ := setupTestChatService(t)

	// Broadcast to conversation with no streams (should not panic)
	event := &api.StreamMessagesResponse{
		MessageId:      "msg123",
		ConversationId: "no-streams-conversation",
		SentAtUnixSec:  1000,
	}
	event.Message = &api.StreamMessagesResponse_UserMessage{
		UserMessage: &api.UserMessage{
			Sender: &api.User{Id: "user1", Name: "Test"},
			Text:   "Test",
		},
	}

	svc.broadcastMessage(context.Background(), "no-streams-conversation", event)

	// Test passes if no panic occurs
}

func TestHasActiveStream(t *testing.T) {
	svc, _ := setupTestChatService(t)

	conversationID := "test-conversation"
	ch1 := make(chan *api.StreamMessagesResponse, 10)
	ch2 := make(chan *api.StreamMessagesResponse, 10)

	// Register streams for two different users
	svc.registerStream(context.Background(), conversationID, "user1", ch1)
	svc.registerStream(context.Background(), conversationID, "user2", ch2)

	// Test hasActiveStream
	if !svc.hasActiveStream(conversationID, "user1") {
		t.Error("Expected user1 to have active stream")
	}

	if !svc.hasActiveStream(conversationID, "user2") {
		t.Error("Expected user2 to have active stream")
	}

	if svc.hasActiveStream(conversationID, "user3") {
		t.Error("Expected user3 to NOT have active stream")
	}

	if svc.hasActiveStream("other-conversation", "user1") {
		t.Error("Expected user1 to NOT have stream for different conversation")
	}
}

// Presence Tracking Tests.

func TestUpdatePresence_Foreground(t *testing.T) {
	svc, _ := setupTestChatService(t)

	ctx := contextWithAuth("user1", "user1@example.com")
	req := connect.NewRequest(&api.UpdatePresenceRequest{
		Status: api.PresenceStatus_PRESENCE_STATUS_FOREGROUND,
	})

	resp, err := svc.UpdatePresence(ctx, req)
	if err != nil {
		t.Fatalf("UpdatePresence failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected non-nil response")
	}

	// Verify presence was updated
	if !svc.isUserInForeground("user1") {
		t.Error("Expected user to be in foreground")
	}
}

func TestUpdatePresence_Background(t *testing.T) {
	svc, _ := setupTestChatService(t)

	ctx := contextWithAuth("user1", "user1@example.com")

	// First set to foreground
	reqFg := connect.NewRequest(&api.UpdatePresenceRequest{
		Status: api.PresenceStatus_PRESENCE_STATUS_FOREGROUND,
	})
	_, err := svc.UpdatePresence(ctx, reqFg)
	if err != nil {
		t.Fatalf("UpdatePresence foreground failed: %v", err)
	}

	// Then set to background
	reqBg := connect.NewRequest(&api.UpdatePresenceRequest{
		Status: api.PresenceStatus_PRESENCE_STATUS_BACKGROUND,
	})
	_, err = svc.UpdatePresence(ctx, reqBg)
	if err != nil {
		t.Fatalf("UpdatePresence background failed: %v", err)
	}

	// Verify presence was updated
	if svc.isUserInForeground("user1") {
		t.Error("Expected user to be in background")
	}
}

func TestUpdatePresence_RequiresAuth(t *testing.T) {
	svc, _ := setupTestChatService(t)

	ctx := context.Background() // No auth
	req := connect.NewRequest(&api.UpdatePresenceRequest{
		Status: api.PresenceStatus_PRESENCE_STATUS_FOREGROUND,
	})

	_, err := svc.UpdatePresence(ctx, req)
	if err == nil {
		t.Fatal("Expected error when not authenticated")
	}

	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("Expected Unauthenticated error, got: %v", err)
	}
}

func TestUpdatePresence_RejectsUnspecified(t *testing.T) {
	svc, _ := setupTestChatService(t)

	ctx := contextWithAuth("user1", "user1@example.com")
	req := connect.NewRequest(&api.UpdatePresenceRequest{
		Status: api.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED,
	})

	_, err := svc.UpdatePresence(ctx, req)
	if err == nil {
		t.Fatal("Expected error when status is unspecified")
	}

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got: %v", err)
	}
}

func TestPresence_StreamRegistrationSetsForeground(t *testing.T) {
	svc, _ := setupTestChatService(t)

	conversationID := "test-conversation"

	// Verify user2 has no presence initially
	if svc.isUserInForeground("user2") {
		t.Error("User2 should not be in foreground initially")
	}

	// Register stream (should auto-set to foreground)
	ch := make(chan *api.StreamMessagesResponse, 10)
	svc.registerStream(context.Background(), conversationID, "user2", ch)
	defer svc.unregisterStream(context.Background(), conversationID, ch)

	// Verify user2 was automatically set to foreground
	if !svc.isUserInForeground("user2") {
		t.Error("User2 should be automatically set to foreground when stream is registered")
	}
}

func TestPresence_StreamClosureSetsBackground(t *testing.T) {
	svc, _ := setupTestChatService(t)

	conversationID := "test-conversation"

	// Register stream
	ch := make(chan *api.StreamMessagesResponse, 10)
	svc.registerStream(context.Background(), conversationID, "user2", ch)

	// Verify user2 is in foreground
	if !svc.isUserInForeground("user2") {
		t.Error("User2 should be in foreground after stream is registered")
	}

	// Unregister stream (simulates app crash or disconnect)
	svc.unregisterStream(context.Background(), conversationID, ch)

	// Verify user2 was automatically set to background
	if svc.isUserInForeground("user2") {
		t.Error("User2 should be automatically set to background when stream is unregistered")
	}
}

// testAuthMiddleware returns an authn.Middleware that bypasses token validation
// and injects the given auth.Info into every request's context. Used to stand
// up a connect handler for in-process tests without real JWT infrastructure.
func testAuthMiddleware(info *auth.Info) func(http.Handler) http.Handler {
	mw := authn.NewMiddleware(func(ctx context.Context, req *http.Request) (any, error) {
		return info, nil
	})
	return mw.Wrap
}

// TestStreamMessages_BacklogReturnedInChronologicalOrder verifies that the
// initial backlog delivered by StreamMessages is sorted by sent_at_unix_sec
// (oldest first) regardless of the order in which rows were inserted into the
// database — i.e. that PostgreSQL heap order does not leak through to the
// client.
func TestStreamMessages_BacklogReturnedInChronologicalOrder(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	userID := "stream-order-user"
	createTestUsers(t, sqlStorage, userID, "Stream Order User")
	communityID := createTestCommunity(t, sqlStorage, userID)
	createTestCommunityMembership(t, sqlStorage, communityID, userID)
	loanID := createTestLoan(t, sqlStorage, userID, "borrower")

	userCtx := contextWithAuth(userID, "stream-order@example.com")
	startResp, err := svc.StartConversation(userCtx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	conversationID := startResp.Msg.ConversationId

	// Insert three messages in deliberately non-chronological order:
	// t+30 first, then t, then t+15. With heap-order storage this would
	// arrive as 1030, 1000, 1015 without the ORDER BY fix.
	baseTime := int64(1000)
	for _, ts := range []int64{baseTime + 30, baseTime, baseTime + 15} {
		simCtx := clock.WithSimulationTime(userCtx, time.Unix(ts, 0))
		_, sendErr := svc.SendMessage(simCtx, connect.NewRequest(&api.SendMessageRequest{
			ConversationId: conversationID,
			Text:           fmt.Sprintf("message at %d", ts),
		}))
		if sendErr != nil {
			t.Fatalf("SendMessage at ts=%d failed: %v", ts, sendErr)
		}
	}

	// Stand up a minimal HTTP test server so we can exercise the real
	// StreamMessages handler (which requires a connect.ServerStream).
	authInfo := &auth.Info{
		UserID: userID,
		Email:  "stream-order@example.com",
		Role:   models.Role_ROLE_USER,
	}
	_, handler := apiconnect.NewChatServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle("/", testAuthMiddleware(authInfo)(handler))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := apiconnect.NewChatServiceClient(http.DefaultClient, srv.URL)

	streamCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamMessages(streamCtx, connect.NewRequest(&api.StreamMessagesRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	var received []int64
	for stream.Receive() {
		msg := stream.Msg()
		received = append(received, msg.SentAtUnixSec)
		if len(received) == 3 {
			// All backlog messages received; cancel to end the stream.
			cancel()
			break
		}
	}
	// Ignore the error from cancel — that's expected.

	if len(received) != 3 {
		t.Fatalf("want 3 backlog messages, got %d", len(received))
	}

	want := []int64{baseTime, baseTime + 15, baseTime + 30}
	for i, wantTS := range want {
		if received[i] != wantTS {
			t.Errorf("message[%d]: want sent_at=%d, got %d (full order: %v)", i, wantTS, received[i], received)
		}
	}
}
