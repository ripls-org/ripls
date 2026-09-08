package integration_tests

// Regression test for the chat broadcast request_id propagation introduced
// alongside #1799. Before, broadcastMessage used logging.Default() and its
// fan-out log lines carried no request_id at all — meaning the SendMessage
// RPC's request_id never reached the broadcast log lines that delivered
// the message to subscribers. After threading ctx through the broadcaster
// interface, the originating RPC's request_id flows into the broadcast.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/storage"
)

// createAuthChatClientWithRequestID returns a chat client whose every
// outgoing request stamps the given X-Request-ID. The token-bearing
// authTransport is composed inside requestIDTransport so the header
// is added before auth (header order in the request doesn't matter).
func createAuthChatClientWithRequestID(token, serverURL, requestID string) apiconnect.ChatServiceClient {
	return apiconnect.NewChatServiceClient(
		&http.Client{
			Transport: &requestIDTransport{
				requestID: requestID,
				base:      &authTransport{token: token, base: http.DefaultTransport},
			},
		},
		serverURL,
	)
}

// TestChatBroadcast_PropagatesSendMessageRequestID asserts that the broadcast
// log line emitted by chat.broadcastMessage carries the *originating
// SendMessage RPC's* request_id. This is the chain that was severed when
// broadcastMessage used logging.Default() and ignored the request context.
func TestChatBroadcast_PropagatesSendMessageRequestID(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// Set up two users in a shared community + transfer + conversation,
	// matching the well-trodden setup in chat_test.go.
	lenderToken, _ := registerFirstUser(t, serverURL, "lender@example.com", "Lender")
	lenderGear := createAuthGearClient(lenderToken, serverURL)
	lenderCommunity := createAuthCommunityClient(lenderToken, serverURL)

	communityResp, err := lenderCommunity.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Broadcast Test Community",
		Description: "Verifies SendMessage request_id propagates into the chat broadcast.",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity: %v", err)
	}
	communityID := communityResp.Msg.Id

	borrowerToken, _ := registerUserByInvite(t, serverURL, lenderToken, communityID, "borrower@example.com", "Borrower")
	borrowerLoan := createAuthLoanClient(borrowerToken, serverURL)

	gearResp, err := lenderGear.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        proto.String("Test Item"),
		Description: proto.String("for broadcast correlation test"),
	}))
	if err != nil {
		t.Fatalf("SaveGear: %v", err)
	}
	gearID := gearResp.Msg.Id

	shareGearIntoCommunity(t, ctx, lenderCommunity, gearID, communityID, api.Availability_AVAILABILITY_FOR_LOAN)

	interestResp, err := borrowerLoan.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest: %v", err)
	}
	conversationID := interestResp.Msg.Transfer.GetConversationId()
	if conversationID == "" {
		t.Fatal("Expected ExpressInterest to populate ConversationId")
	}

	// Subscribe the borrower to StreamMessages so the lender's SendMessage
	// triggers an actual broadcast. Without a registered subscriber,
	// broadcastMessage bails out before logging — that's by design (no
	// recipients = nothing to log) but means we'd have no broadcast line
	// to assert on.
	borrowerChat := createAuthChatClient(borrowerToken, serverURL)
	streamCtx, streamCancel := context.WithTimeout(ctx, 30*time.Second)
	defer streamCancel()

	stream, err := borrowerChat.StreamMessages(streamCtx, connect.NewRequest(&api.StreamMessagesRequest{
		ConversationId: conversationID,
	}))
	if err != nil {
		t.Fatalf("StreamMessages: %v", err)
	}
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for stream.Receive() {
			// Drain — we just need the subscription to exist server-side.
		}
	}()
	defer streamCancel()

	// Wait until the server logs that the stream is registered, so the
	// SendMessage broadcast definitely sees a subscriber.
	if entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["message"] == "registered stream" && e["conversation_id"] == conversationID
	}, 5*time.Second); entry == nil {
		t.Fatal("borrower stream did not register in time")
	}

	// Drain the bursts of log lines from setup + stream registration so
	// they don't pollute our assertion. The test only cares about lines
	// emitted by the SendMessage call below.
	logCapture.Clear()

	// Send the message under a known request ID. The server's broadcast
	// path runs synchronously inside the SendMessage handler, so by the
	// time SendMessage returns the broadcast log line has been emitted.
	const sendRequestID = "broadcast-correlation-rpc-id-001"
	lenderChat := createAuthChatClientWithRequestID(lenderToken, serverURL, sendRequestID)

	sendResp, err := lenderChat.SendMessage(ctx, connect.NewRequest(&api.SendMessageRequest{
		ConversationId: conversationID,
		Text:           "ping for broadcast correlation",
	}))
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if sendResp.Msg.MessageId == "" {
		t.Fatal("expected SendMessage to return a message id")
	}

	// The broadcast log line must carry the SendMessage RPC's request_id.
	// Match on the structured message + the message_id we just got back to
	// avoid false positives from any other broadcast happening concurrently.
	entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["message"] == "broadcasting message" &&
			e["request_id"] == sendRequestID &&
			e["message_id"] == sendResp.Msg.MessageId
	}, 2*time.Second)

	if entry == nil {
		t.Errorf("no broadcasting message log line found with request_id=%q and message_id=%q",
			sendRequestID, sendResp.Msg.MessageId)
		t.Logf("entries with request_id=%q: %v", sendRequestID, logCapture.FindByRequestID(sendRequestID))
		t.Logf("'broadcasting message' entries: %v", logCapture.FindByMessage("broadcasting message"))
	}
}
