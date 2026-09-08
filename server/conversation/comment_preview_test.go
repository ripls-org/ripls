package conversation

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

func insertUser(t *testing.T, store *storage.ProtoSQLStorage, name, email string) string {
	t.Helper()
	user := &models.User{
		Name:      name,
		Email:     email,
		Role:      models.Role_ROLE_USER,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	id, err := store.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("failed to insert user %s: %v", name, err)
	}
	return id
}

func insertConversation(t *testing.T, store *storage.ProtoSQLStorage, communityID, gearID string, participantIDs []string) string {
	t.Helper()
	conv := &models.ChatConversation{
		CommunityId:      communityID,
		ParticipantIds:   participantIDs,
		Topic:            &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		CreatedAtUnixSec: time.Now().Unix(),
	}
	id, err := store.Insert(context.Background(), conv)
	if err != nil {
		t.Fatalf("failed to insert conversation: %v", err)
	}
	return id
}

func insertUserMessage(t *testing.T, store *storage.ProtoSQLStorage, conversationID, senderID string, sentAt int64, readBy map[string]bool) {
	t.Helper()
	msg := &models.ChatMessage{
		ConversationId:        conversationID,
		SentAtUnixSec:         sentAt,
		ParticipantIdToIsRead: readBy,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: senderID,
				Text:     "Hello from " + senderID,
			},
		},
	}
	_, err := store.Insert(context.Background(), msg)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}
}

func TestEnrichWithCommentPreview_NoConversation(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	preview, err := EnrichWithCommentPreview(ctx, store, "", "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.UnreadCount != 0 {
		t.Errorf("expected unread count 0, got %d", preview.UnreadCount)
	}
	if preview.LastMessageText != nil {
		t.Errorf("expected nil last message text, got %v", preview.LastMessageText)
	}
	if preview.LastMessageSender != nil {
		t.Errorf("expected nil last message sender")
	}
	if len(preview.RecentCommenters) != 0 {
		t.Errorf("expected 0 recent commenters, got %d", len(preview.RecentCommenters))
	}
}

func TestEnrichWithCommentPreview_NoMessages(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	userID := insertUser(t, store, "Alice", "alice@test.com")
	convID := insertConversation(t, store, "comm1", "gear1", []string{userID})

	preview, err := EnrichWithCommentPreview(ctx, store, convID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.UnreadCount != 0 {
		t.Errorf("expected unread count 0, got %d", preview.UnreadCount)
	}
	if preview.LastMessageText != nil {
		t.Errorf("expected nil last message text")
	}
}

func TestEnrichWithCommentPreview_WithMessages(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice Smith", "alice@test.com")
	bobID := insertUser(t, store, "Bob Jones", "bob@test.com")
	convID := insertConversation(t, store, "comm1", "gear1", []string{aliceID, bobID})

	now := time.Now().Unix()

	// Bob sends a message that Alice hasn't read.
	insertUserMessage(t, store, convID, bobID, now-3600, map[string]bool{
		bobID:   true,
		aliceID: false,
	})

	// Bob sends another message.
	insertUserMessage(t, store, convID, bobID, now-1800, map[string]bool{
		bobID:   true,
		aliceID: false,
	})

	// Alice sends a message (should not count as unread for Alice).
	insertUserMessage(t, store, convID, aliceID, now-900, map[string]bool{
		aliceID: true,
		bobID:   false,
	})

	preview, err := EnrichWithCommentPreview(ctx, store, convID, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Alice has 2 unread messages from Bob.
	if preview.UnreadCount != 2 {
		t.Errorf("expected unread count 2, got %d", preview.UnreadCount)
	}

	// Most recent message is Alice's own (now-900), even though she is currentUser.
	if preview.LastMessageSender == nil {
		t.Fatal("expected last message sender to be set")
	}
	if preview.LastMessageSender.Name != "Alice Smith" {
		t.Errorf("expected sender name 'Alice Smith', got %q", preview.LastMessageSender.Name)
	}

	if preview.LastMessageText == nil {
		t.Fatal("expected last message text to be set")
	}
	if *preview.LastMessageText != "Hello from "+aliceID {
		t.Errorf("expected message text 'Hello from %s', got %q", aliceID, *preview.LastMessageText)
	}

	if preview.LastMessageTimeAgo == nil {
		t.Fatal("expected time-ago to be set")
	}

	// Recent commenters include both Bob and Alice (current user is no longer excluded).
	// Alice's message is most recent (now-900), so she appears first.
	if len(preview.RecentCommenters) != 2 {
		t.Fatalf("expected 2 recent commenters, got %d", len(preview.RecentCommenters))
	}
	if preview.RecentCommenters[0].Name != "Alice Smith" {
		t.Errorf("expected first commenter 'Alice Smith', got %q", preview.RecentCommenters[0].Name)
	}
	if preview.RecentCommenters[1].Name != "Bob Jones" {
		t.Errorf("expected second commenter 'Bob Jones', got %q", preview.RecentCommenters[1].Name)
	}
}

func TestEnrichWithCommentPreview_CurrentUserLastMessage(t *testing.T) {
	// Regression: when the current user sends the most recent message, the preview
	// must still show it (not hide it by filtering on currentUserID).
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice Smith", "alice3@test.com")
	bobID := insertUser(t, store, "Bob Jones", "bob3@test.com")
	convID := insertConversation(t, store, "comm1", "gear1", []string{aliceID, bobID})

	now := time.Now().Unix()

	// Bob sends first.
	insertUserMessage(t, store, convID, bobID, now-3600, map[string]bool{
		bobID:   true,
		aliceID: true,
	})

	// Alice replies last — this is the most recent message.
	insertUserMessage(t, store, convID, aliceID, now-60, map[string]bool{
		aliceID: true,
		bobID:   false,
	})

	preview, err := EnrichWithCommentPreview(ctx, store, convID, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Preview must show Alice's message, not be blank.
	if preview.LastMessageSender == nil {
		t.Fatal("expected last message sender to be set; got nil (regression: own message hidden)")
	}
	if preview.LastMessageSender.Name != "Alice Smith" {
		t.Errorf("expected sender 'Alice Smith', got %q", preview.LastMessageSender.Name)
	}
	if preview.LastMessageText == nil {
		t.Fatal("expected last message text to be set")
	}
}

func TestBatchEnrichWithCommentPreview(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice", "alice2@test.com")
	bobID := insertUser(t, store, "Bob", "bob2@test.com")

	conv1ID := insertConversation(t, store, "comm1", "gear1", []string{aliceID, bobID})
	conv2ID := insertConversation(t, store, "comm1", "gear2", []string{aliceID})

	now := time.Now().Unix()
	insertUserMessage(t, store, conv1ID, bobID, now-600, map[string]bool{
		bobID:   true,
		aliceID: false,
	})

	// conv2 has no messages.

	results, err := BatchEnrichWithCommentPreview(ctx, store, []string{conv1ID, conv2ID, ""}, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// conv1 should have 1 unread.
	p1 := results[conv1ID]
	if p1 == nil {
		t.Fatal("expected preview for conv1")
	}
	if p1.UnreadCount != 1 {
		t.Errorf("conv1: expected unread count 1, got %d", p1.UnreadCount)
	}

	// conv2 should have 0 unread.
	p2 := results[conv2ID]
	if p2 == nil {
		t.Fatal("expected preview for conv2")
	}
	if p2.UnreadCount != 0 {
		t.Errorf("conv2: expected unread count 0, got %d", p2.UnreadCount)
	}
}

// TestBuildPreview_DecodesMentionsInLastMessageText verifies that encoded
// @-mentions in user-message text are decoded to "@Display Name" form so the
// inbox preview renders human-readable text instead of the raw on-wire format.
// Regression test for #1668.
func TestBuildPreview_DecodesMentionsInLastMessageText(t *testing.T) {
	now := time.Now()
	messages := []proto.Message{
		&models.ChatMessage{
			ConversationId: "conv1",
			SentAtUnixSec:  now.Unix(),
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: "u1",
					Text:     "Thanks @[user:u-viewer:Viewer Name] for the @[loan:loan-1:Cordless Drill]!",
				},
			},
		},
	}
	userMap := map[string]*api.User{
		"u1": {Id: "u1", Name: "Sender"},
	}

	preview, err := buildPreviewWithUserMap(messages, "viewer", userMap, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.LastMessageText == nil {
		t.Fatal("expected non-nil LastMessageText")
	}
	want := "Thanks @Viewer Name for the @Cordless Drill!"
	if *preview.LastMessageText != want {
		t.Errorf("LastMessageText = %q, want %q", *preview.LastMessageText, want)
	}
}

func TestBuildRecentCommenters_Limit(t *testing.T) {
	// Test that recent commenters is limited to 3 via buildPreviewWithUserMap.
	now := time.Now()
	messages := make([]proto.Message, 0, 4)
	senderIDs := []string{"u1", "u2", "u3", "u4"}
	for i, sid := range senderIDs {
		messages = append(messages, &models.ChatMessage{
			ConversationId: "conv1",
			SentAtUnixSec:  now.Unix() - int64(len(senderIDs)-i)*100,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: sid,
					Text:     "msg " + sid,
				},
			},
		})
	}

	userMap := map[string]*api.User{
		"u1": {Id: "u1", Name: "User1"},
		"u2": {Id: "u2", Name: "User2"},
		"u3": {Id: "u3", Name: "User3"},
		"u4": {Id: "u4", Name: "User4"},
	}

	preview, err := buildPreviewWithUserMap(messages, "viewer", userMap, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(preview.RecentCommenters) != 3 {
		t.Errorf("expected 3 recent commenters, got %d", len(preview.RecentCommenters))
	}
}

func insertCreationAnchorMessage(t *testing.T, store *storage.ProtoSQLStorage, conversationID string, action models.ChatSystemAction, sentAt int64) {
	t.Helper()
	msg := &models.ChatMessage{
		ConversationId:        conversationID,
		SentAtUnixSec:         sentAt,
		ParticipantIdToIsRead: map[string]bool{},
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: &models.SystemChatMessage{
				Action:      action,
				Description: "item shared with community",
			},
		},
	}
	_, err := store.Insert(context.Background(), msg)
	if err != nil {
		t.Fatalf("failed to insert anchor message: %v", err)
	}
}

func TestEnrichWithCommentPreview_CreationAnchorExcluded(t *testing.T) {
	// Regression: creation anchor messages (e.g., EXPERIENCE_CREATED) must not
	// appear as the most recent comment or contribute to unread/message counts.
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice", "alice4@test.com")
	convID := insertConversation(t, store, "comm1", "gear1", []string{aliceID})

	now := time.Now().Unix()

	// Only message in the conversation is the creation anchor.
	insertCreationAnchorMessage(t, store, convID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED, now-3600)

	preview, err := EnrichWithCommentPreview(ctx, store, convID, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Anchor must not count as unread.
	if preview.UnreadCount != 0 {
		t.Errorf("expected unread count 0 (anchor excluded), got %d", preview.UnreadCount)
	}
	// Anchor must not surface as the most recent comment.
	if preview.LastMessageText != nil {
		t.Errorf("expected nil LastMessageText (anchor excluded), got %q", *preview.LastMessageText)
	}
	if preview.LastMessageSender != nil {
		t.Errorf("expected nil LastMessageSender (anchor excluded)")
	}
}

func TestEnrichWithCommentPreview_AnchorPlusRealMessage(t *testing.T) {
	// When there is both an anchor and a real message, only the real message
	// should appear as the preview.
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice", "alice5@test.com")
	bobID := insertUser(t, store, "Bob", "bob5@test.com")
	convID := insertConversation(t, store, "comm1", "gear1", []string{aliceID, bobID})

	now := time.Now().Unix()

	// Anchor is the oldest message.
	insertCreationAnchorMessage(t, store, convID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED, now-7200)

	// Bob sends a real message after the anchor.
	insertUserMessage(t, store, convID, bobID, now-60, map[string]bool{
		bobID:   true,
		aliceID: false,
	})

	preview, err := EnrichWithCommentPreview(ctx, store, convID, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if preview.UnreadCount != 1 {
		t.Errorf("expected unread count 1, got %d", preview.UnreadCount)
	}
	if preview.LastMessageSender == nil || preview.LastMessageSender.Name != "Bob" {
		t.Errorf("expected last sender 'Bob', got %v", preview.LastMessageSender)
	}
}

func TestEnrichWithCommentPreview_NonAnchorSystemMessagesExcluded(t *testing.T) {
	// All system messages (not just creation anchors) must be excluded from
	// unread counts and last-message preview.
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice", "alice-sys@test.com")
	bobID := insertUser(t, store, "Bob", "bob-sys@test.com")
	convID := insertConversation(t, store, "comm1", "gear1", []string{aliceID, bobID})

	now := time.Now().Unix()

	// Insert a real user message from Bob (unread by Alice).
	insertUserMessage(t, store, convID, bobID, now-120, map[string]bool{
		bobID:   true,
		aliceID: false,
	})

	// Insert several non-anchor system messages (unread by Alice).
	for _, action := range []models.ChatSystemAction{
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED,
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED,
	} {
		actorID := bobID
		msg := &models.ChatMessage{
			ConversationId: convID,
			SentAtUnixSec:  now - 30,
			ParticipantIdToIsRead: map[string]bool{
				aliceID: false,
				bobID:   true,
			},
			Message: &models.ChatMessage_SystemMessage{
				SystemMessage: &models.SystemChatMessage{
					ActorId:     &actorID,
					Action:      action,
					Description: "system event",
				},
			},
		}
		_, err := store.Insert(ctx, msg)
		if err != nil {
			t.Fatalf("failed to insert system message: %v", err)
		}
	}

	preview, err := EnrichWithCommentPreview(ctx, store, convID, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the user message should count as unread.
	if preview.UnreadCount != 1 {
		t.Errorf("expected unread count 1 (system messages excluded), got %d", preview.UnreadCount)
	}

	// Last message preview should be the user message, not a system message.
	if preview.LastMessageSender == nil || preview.LastMessageSender.Name != "Bob" {
		t.Errorf("expected last sender 'Bob', got %v", preview.LastMessageSender)
	}
}

func TestBuildPreviewWithUserMap_ImageOnlyMessage(t *testing.T) {
	now := time.Now()
	messages := []proto.Message{
		&models.ChatMessage{
			ConversationId: "conv1",
			SentAtUnixSec:  now.Unix() - 60,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: "u1",
					Text:     "", // no text
					MediaIds: []string{"media-abc"},
				},
			},
		},
	}

	userMap := map[string]*api.User{
		"u1": {Id: "u1", Name: "Alice Smith"},
	}

	preview, err := buildPreviewWithUserMap(messages, "u2", userMap, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if preview.LastMessageText == nil {
		t.Fatal("expected LastMessageText to be set for image-only message")
	}
	if *preview.LastMessageText != "Alice shared an image" {
		t.Errorf("expected 'Alice shared an image', got %q", *preview.LastMessageText)
	}
	if preview.LastMessageSender == nil || preview.LastMessageSender.Name != "Alice Smith" {
		t.Errorf("expected sender 'Alice Smith', got %v", preview.LastMessageSender)
	}
}

func TestBuildPreviewWithUserMap_SystemMessageOnlyNoPreview(t *testing.T) {
	// When only system messages exist, the last-message preview should be empty.
	// System messages are excluded from previews to avoid showing automated
	// state transitions as the most recent "comment."
	now := time.Now()
	messages := []proto.Message{
		&models.ChatMessage{
			ConversationId: "conv1",
			SentAtUnixSec:  now.Unix() - 60,
			Message: &models.ChatMessage_SystemMessage{
				SystemMessage: &models.SystemChatMessage{
					ActorId:     proto.String("u1"),
					Description: "Alice marked the item as received.",
				},
			},
		},
	}

	userMap := map[string]*api.User{
		"u1": {Id: "u1", Name: "Alice"},
	}

	preview, err := buildPreviewWithUserMap(messages, "u2", userMap, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if preview.LastMessageSender != nil {
		t.Errorf("expected nil LastMessageSender (system messages excluded), got %v", preview.LastMessageSender)
	}
	if preview.LastMessageText != nil {
		t.Errorf("expected nil LastMessageText (system messages excluded), got %q", *preview.LastMessageText)
	}
}

func TestBatchEnrichWithCommentPreviewAndCounts_Empty(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	previews, counts, posters, err := BatchEnrichWithCommentPreviewAndCounts(ctx, store, []string{}, "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(previews) != 0 {
		t.Errorf("expected empty previews map, got %d entries", len(previews))
	}
	if len(counts) != 0 {
		t.Errorf("expected empty counts map, got %d entries", len(counts))
	}
	if len(posters) != 0 {
		t.Errorf("expected empty posters map, got %d entries", len(posters))
	}
}

func TestBatchEnrichWithCommentPreviewAndCounts_NoMessages(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice", "alice-bnc@test.com")
	convID := insertConversation(t, store, "comm1", "gear1", []string{aliceID})

	previews, counts, posters, err := BatchEnrichWithCommentPreviewAndCounts(ctx, store, []string{convID}, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p := previews[convID]; p == nil {
		t.Error("expected preview entry for conversation with no messages")
	}
	if got := counts[convID]; got != 0 {
		t.Errorf("expected message count 0 for empty conversation, got %d", got)
	}
	if got := posters[convID]; got != nil {
		t.Errorf("expected nil posters for empty conversation, got %v", got)
	}
}

func TestBatchEnrichWithCommentPreviewAndCounts_MixedMessages(t *testing.T) {
	// Conversation with both system and user messages — count must exclude system messages.
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice", "alice-bnc2@test.com")
	bobID := insertUser(t, store, "Bob", "bob-bnc2@test.com")
	convID := insertConversation(t, store, "comm1", "gear2", []string{aliceID, bobID})

	now := time.Now().Unix()

	// 2 real user messages.
	insertUserMessage(t, store, convID, bobID, now-600, map[string]bool{bobID: true, aliceID: false})
	insertUserMessage(t, store, convID, aliceID, now-300, map[string]bool{aliceID: true, bobID: false})

	// 1 system message (should not count toward message_count).
	insertCreationAnchorMessage(t, store, convID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED, now-900)

	previews, counts, _, err := BatchEnrichWithCommentPreviewAndCounts(ctx, store, []string{convID}, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if previews[convID] == nil {
		t.Fatal("expected preview for conversation")
	}
	if got := counts[convID]; got != 2 {
		t.Errorf("expected message count 2 (system excluded), got %d", got)
	}
}

func TestBatchEnrichWithCommentPreviewAndCounts_AbsentConvNotInMaps(t *testing.T) {
	// Conversations not in the request set must be absent from both maps.
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice", "alice-bnc3@test.com")
	convID := insertConversation(t, store, "comm1", "gear3", []string{aliceID})

	insertUserMessage(t, store, convID, aliceID, time.Now().Unix(), map[string]bool{aliceID: true})

	// Request a different (absent) conversation ID.
	previews, counts, posters, err := BatchEnrichWithCommentPreviewAndCounts(ctx, store, []string{"absent-conv-id"}, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := previews[convID]; ok {
		t.Error("convID not in request set should be absent from previews map")
	}
	if _, ok := counts[convID]; ok {
		t.Error("convID not in request set should be absent from counts map")
	}
	if _, ok := posters[convID]; ok {
		t.Error("convID not in request set should be absent from posters map")
	}
}

func TestBatchEnrichWithCommentPreviewAndCounts_PosterOrder(t *testing.T) {
	// postersByConv must be deduplicated and in first-post (chronological) order.
	// System messages must not contribute poster IDs.
	store := setupTestStorage(t)
	ctx := context.Background()

	aliceID := insertUser(t, store, "Alice", "alice-poster@test.com")
	bobID := insertUser(t, store, "Bob", "bob-poster@test.com")
	carolID := insertUser(t, store, "Carol", "carol-poster@test.com")

	// conv1: three posters in chronological order alice -> bob -> carol,
	// with alice posting again later (dedup: alice appears only once, first).
	conv1ID := insertConversation(t, store, "comm1", "gear-po1", []string{aliceID, bobID, carolID})
	now := time.Now().Unix()
	insertUserMessage(t, store, conv1ID, aliceID, now-3000, map[string]bool{aliceID: true})
	insertUserMessage(t, store, conv1ID, bobID, now-2000, map[string]bool{bobID: true})
	insertUserMessage(t, store, conv1ID, carolID, now-1000, map[string]bool{carolID: true})
	insertUserMessage(t, store, conv1ID, aliceID, now-500, map[string]bool{aliceID: true}) // duplicate — must not re-appear

	// conv2: only a system message — no posters.
	conv2ID := insertConversation(t, store, "comm1", "gear-po2", []string{aliceID})
	insertCreationAnchorMessage(t, store, conv2ID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED, now-100)

	// conv3: single poster.
	conv3ID := insertConversation(t, store, "comm1", "gear-po3", []string{bobID})
	insertUserMessage(t, store, conv3ID, bobID, now-50, map[string]bool{bobID: true})

	_, _, postersByConv, err := BatchEnrichWithCommentPreviewAndCounts(ctx, store, []string{conv1ID, conv2ID, conv3ID}, aliceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// conv1: alice first, bob second, carol third; alice's second post must not create a duplicate.
	p1 := postersByConv[conv1ID]
	if len(p1) != 3 {
		t.Fatalf("conv1: expected 3 unique posters, got %d: %v", len(p1), p1)
	}
	if p1[0] != aliceID {
		t.Errorf("conv1: expected first poster aliceID, got %q", p1[0])
	}
	if p1[1] != bobID {
		t.Errorf("conv1: expected second poster bobID, got %q", p1[1])
	}
	if p1[2] != carolID {
		t.Errorf("conv1: expected third poster carolID, got %q", p1[2])
	}

	// conv2: system message only — no posters.
	if p2 := postersByConv[conv2ID]; len(p2) != 0 {
		t.Errorf("conv2: expected 0 posters (system message only), got %d: %v", len(p2), p2)
	}

	// conv3: single poster.
	p3 := postersByConv[conv3ID]
	if len(p3) != 1 || p3[0] != bobID {
		t.Errorf("conv3: expected [bobID], got %v", p3)
	}
}

func TestFormatTimeAgo(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		unixSec  int64
		expected string
	}{
		{"zero", 0, ""},
		{"30 seconds ago", now.Unix() - 30, "0m"},
		{"5 minutes ago", now.Unix() - 300, "5m"},
		{"2 hours ago", now.Unix() - 7200, "2h"},
		{"3 days ago", now.Unix() - 259200, "3d"},
		{"future", now.Unix() + 100, "0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTimeAgo(tt.unixSec, now)
			if result != tt.expected {
				t.Errorf("formatTimeAgo(%d) = %q, want %q", tt.unixSec, result, tt.expected)
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world this is a test", 5, "hello"},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}

	for _, tt := range tests {
		result := truncateText(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}
