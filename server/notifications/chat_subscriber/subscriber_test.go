package chat_subscriber

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/chat_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

func setupSubscriberTest(t *testing.T) (*Subscriber, *mockNotificationService, *storage.ProtoSQLStorage, func()) {
	t.Helper()
	st, cleanup := storage.SetupTestStorage(t)
	mock := &mockNotificationService{}
	sub := New(st, mock)
	return sub, mock, st, cleanup
}

func seedUser(t *testing.T, st *storage.ProtoSQLStorage, id, name string) {
	t.Helper()
	if _, err := st.Insert(context.Background(), &models.User{Id: id, Name: name}); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func seedCommunity(t *testing.T, st *storage.ProtoSQLStorage, id, creatorID string) {
	t.Helper()
	if _, err := st.Insert(context.Background(), &models.Community{
		Id: id, Name: "Test Community", CreatorId: creatorID, OwnerUserId: creatorID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
}

func seedMembership(t *testing.T, st *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	if _, err := st.Insert(context.Background(), &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
}

func userMessageEvent(senderID, senderName string, recipientIDs []string, communityID string) *chat_event_bus.PublishedEvent {
	msgID := uuid.New().String()
	convID := uuid.New().String()
	return &chat_event_bus.PublishedEvent{
		Kind: chat_event_bus.KindUserMessage,
		Message: &models.ChatMessage{
			Id:             msgID,
			ConversationId: convID,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: senderID,
					Text:     "Hello",
				},
			},
		},
		Conversation: &models.ChatConversation{
			Id:             convID,
			CommunityId:    communityID,
			ParticipantIds: append([]string{senderID}, recipientIDs...),
		},
		Sender: &api.User{Id: senderID, Name: senderName},
	}
}

func TestHandle_SystemMessageKindSkipped(t *testing.T) {
	sub, mock, _, cleanup := setupSubscriberTest(t)
	defer cleanup()

	evt := &chat_event_bus.PublishedEvent{
		Kind: chat_event_bus.KindSystemMessage,
		Message: &models.ChatMessage{
			Id: "msg-1",
			Message: &models.ChatMessage_SystemMessage{
				SystemMessage: &models.SystemChatMessage{},
			},
		},
		Conversation: &models.ChatConversation{Id: "conv-1"},
	}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(mock.dispatched()) != 0 {
		t.Errorf("expected 0 notifications for system message, got %d", len(mock.dispatched()))
	}
}

func TestHandle_ReactionUpdateKindSkipped(t *testing.T) {
	sub, mock, _, cleanup := setupSubscriberTest(t)
	defer cleanup()

	evt := &chat_event_bus.PublishedEvent{
		Kind:         chat_event_bus.KindReactionUpdate,
		Conversation: &models.ChatConversation{Id: "conv-1"},
	}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(mock.dispatched()) != 0 {
		t.Errorf("expected 0 notifications for reaction update, got %d", len(mock.dispatched()))
	}
}

func TestHandle_OnBehalfOfSkipped(t *testing.T) {
	sub, mock, _, cleanup := setupSubscriberTest(t)
	defer cleanup()

	evt := &chat_event_bus.PublishedEvent{
		Kind:       chat_event_bus.KindUserMessage,
		OnBehalfOf: true,
		Message: &models.ChatMessage{
			Id: "msg-1",
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{SenderId: "user-1"},
			},
		},
		Conversation: &models.ChatConversation{
			Id:             "conv-1",
			ParticipantIds: []string{"user-1", "user-2"},
		},
	}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(mock.dispatched()) != 0 {
		t.Errorf("expected 0 notifications for on-behalf-of, got %d", len(mock.dispatched()))
	}
}

func TestHandle_SoftDeletedCommunitySkipped(t *testing.T) {
	sub, mock, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	senderID := uuid.New().String()
	recipientID := uuid.New().String()
	communityID := uuid.New().String()

	if _, err := st.Insert(context.Background(), &models.Community{
		Id: communityID, Name: "Deleted", CreatorId: senderID,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  senderID,
			DeletedAtUnixSec: 1700000000,
		},
	}); err != nil {
		t.Fatalf("seed deleted community: %v", err)
	}

	evt := userMessageEvent(senderID, "Sender", []string{recipientID}, communityID)

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(mock.dispatched()) != 0 {
		t.Errorf("expected 0 notifications for soft-deleted community, got %d", len(mock.dispatched()))
	}
}

func TestHandle_ActiveCommunityDispatchesNotification(t *testing.T) {
	sub, mock, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	senderID := uuid.New().String()
	recipientID := uuid.New().String()
	communityID := uuid.New().String()

	seedCommunity(t, st, communityID, senderID)
	seedUser(t, st, senderID, "Sender")
	seedUser(t, st, recipientID, "Recipient")

	evt := userMessageEvent(senderID, "Sender", []string{recipientID}, communityID)

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(mock.dispatched()) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(mock.dispatched()))
	}
	if mock.dispatched()[0].userID != recipientID {
		t.Errorf("expected notification to %s, got %s", recipientID, mock.dispatched()[0].userID)
	}
}

func TestHandle_PrivateConversationNoGate(t *testing.T) {
	sub, mock, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	senderID := uuid.New().String()
	recipientID := uuid.New().String()

	seedUser(t, st, senderID, "Sender")
	seedUser(t, st, recipientID, "Recipient")

	evt := userMessageEvent(senderID, "Sender", []string{recipientID}, "")

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(mock.dispatched()) != 1 {
		t.Fatalf("expected 1 notification for private conversation, got %d", len(mock.dispatched()))
	}
}

func TestHandle_PreferenceDisabledSkips(t *testing.T) {
	sub, mock, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	senderID := uuid.New().String()
	mutedID := uuid.New().String()
	communityID := uuid.New().String()

	seedCommunity(t, st, communityID, senderID)
	seedUser(t, st, senderID, "Sender")
	seedUser(t, st, mutedID, "Muted")

	off := false
	if _, err := st.Insert(context.Background(), &models.CommunityNotificationPreferences{
		CommunityId: communityID,
		UserId:      mutedID,
		NotifyChats: &off,
	}); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	evt := userMessageEvent(senderID, "Sender", []string{mutedID}, communityID)

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(mock.dispatched()) != 0 {
		t.Errorf("expected 0 notifications (preference disabled), got %d", len(mock.dispatched()))
	}
}

func TestHandle_ForegroundStreamUserSuppressed(t *testing.T) {
	sub, mock, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	senderID := uuid.New().String()
	streamingID := uuid.New().String()
	offlineID := uuid.New().String()
	communityID := uuid.New().String()

	seedCommunity(t, st, communityID, senderID)
	seedUser(t, st, senderID, "Sender")
	seedUser(t, st, streamingID, "Streaming")
	seedUser(t, st, offlineID, "Offline")

	// Wire stream checker: streamingID is active.
	sub.SetStreamChecker(func(_, userID string) bool {
		return userID == streamingID
	})
	// Foreground checker: streamingID is in foreground.
	sub.SetForegroundChecker(func(userID string) bool {
		return userID == streamingID
	})

	evt := userMessageEvent(senderID, "Sender", []string{streamingID, offlineID}, communityID)

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	dispatched := mock.dispatched()
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 notification (foreground streamer suppressed), got %d", len(dispatched))
	}
	if dispatched[0].userID != offlineID {
		t.Errorf("expected notification to %s, got %s", offlineID, dispatched[0].userID)
	}
}

func TestHandle_MentionTitleCustomized(t *testing.T) {
	sub, mock, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	senderID := uuid.New().String()
	mentionedID := uuid.New().String()
	communityID := uuid.New().String()

	seedCommunity(t, st, communityID, senderID)
	seedUser(t, st, senderID, "Alice")
	seedUser(t, st, mentionedID, "Bob")

	evt := userMessageEvent(senderID, "Alice", []string{mentionedID}, communityID)
	evt.MentionedUserIDs = []string{mentionedID}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	dispatched := mock.dispatched()
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(dispatched))
	}
	if dispatched[0].notification.Title != "Alice mentioned you" {
		t.Errorf("title = %q, want %q", dispatched[0].notification.Title, "Alice mentioned you")
	}
}

func TestHandle_CommunityWideFanOut(t *testing.T) {
	sub, mock, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	communityID := uuid.New().String()
	senderID := uuid.New().String()
	member1 := uuid.New().String()
	member2 := uuid.New().String()

	seedCommunity(t, st, communityID, senderID)
	seedUser(t, st, senderID, "Sender")
	seedUser(t, st, member1, "Member1")
	seedUser(t, st, member2, "Member2")
	seedMembership(t, st, communityID, senderID)
	seedMembership(t, st, communityID, member1)
	seedMembership(t, st, communityID, member2)

	// Community-wide: empty ParticipantIds, topic = CommunityId.
	evt := &chat_event_bus.PublishedEvent{
		Kind: chat_event_bus.KindUserMessage,
		Message: &models.ChatMessage{
			Id:             uuid.New().String(),
			ConversationId: uuid.New().String(),
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: senderID,
					Text:     "community broadcast",
				},
			},
		},
		Conversation: &models.ChatConversation{
			Id:             uuid.New().String(),
			CommunityId:    communityID,
			ParticipantIds: []string{},
			Topic: &models.ConversationTopic{
				TopicId: &models.ConversationTopic_CommunityId{CommunityId: communityID},
			},
		},
		Sender: &api.User{Name: "Sender"},
	}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	dispatched := mock.dispatched()
	if len(dispatched) != 2 {
		t.Fatalf("expected 2 notifications (3 members minus sender), got %d", len(dispatched))
	}
	for _, d := range dispatched {
		if d.userID == senderID {
			t.Errorf("sender %s should not receive a notification", senderID)
		}
	}
}

func TestHandle_TextTruncatedTo100Chars(t *testing.T) {
	sub, mock, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	senderID := uuid.New().String()
	recipientID := uuid.New().String()
	communityID := uuid.New().String()

	seedCommunity(t, st, communityID, senderID)
	seedUser(t, st, senderID, "Sender")
	seedUser(t, st, recipientID, "Recipient")

	longText := ""
	for i := 0; i < 150; i++ {
		longText += "a"
	}

	evt := &chat_event_bus.PublishedEvent{
		Kind: chat_event_bus.KindUserMessage,
		Message: &models.ChatMessage{
			Id:             uuid.New().String(),
			ConversationId: uuid.New().String(),
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: senderID,
					Text:     longText,
				},
			},
		},
		Conversation: &models.ChatConversation{
			Id:             uuid.New().String(),
			CommunityId:    communityID,
			ParticipantIds: []string{senderID, recipientID},
		},
		Sender: &api.User{Name: "Sender"},
	}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	dispatched := mock.dispatched()
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(dispatched))
	}
	if len(dispatched[0].notification.Body) > 100 {
		t.Errorf("body length = %d, want ≤ 100", len(dispatched[0].notification.Body))
	}
}

// TestHandle_TopicNameLookupIsHoistedOutOfFanOut asserts that for a
// community-wide chat with N recipients, the gear-name lookup runs
// once per event — not once per recipient. Regression test for #2173:
// before the fix, a 200-member community fan-out issued 200 redundant
// Gear GetByIDs.
//
// The query-count budget here is a ceiling, not a target — it covers
// the soft-delete check, recipient resolution, preference fetch,
// locale batch, topic resolution, and any per-recipient device or
// preference reads the dispatch path issues. The invariant under
// test is that the per-recipient query count is **flat as N grows**:
// adding a recipient should not multiply the topic-lookup queries.
func TestHandle_TopicNameLookupIsHoistedOutOfFanOut(t *testing.T) {
	sub, _, st, cleanup := setupSubscriberTest(t)
	defer cleanup()

	ctx := context.Background()
	communityID := uuid.New().String()
	senderID := uuid.New().String()
	gearID := "gear-fanout"

	seedCommunity(t, st, communityID, senderID)
	seedUser(t, st, senderID, "Sender")
	if _, err := st.Insert(ctx, &models.Gear{
		Id: gearID, Name: "Shared Drill", OwnerId: senderID,
	}); err != nil {
		t.Fatalf("seed gear: %v", err)
	}

	const recipientCount = 25
	recipientIDs := make([]string, recipientCount)
	for i := 0; i < recipientCount; i++ {
		id := uuid.New().String()
		recipientIDs[i] = id
		seedUser(t, st, id, "Recipient")
	}

	evt := userMessageEvent(senderID, "Sender", recipientIDs, communityID)
	evt.Conversation.Topic = &models.ConversationTopic{
		TopicId: &models.ConversationTopic_GearId{GearId: gearID},
	}

	// Budget: a generous ceiling. With the fix, the topic-name
	// lookup is one query regardless of recipient count; without
	// the fix it would be one *per recipient* (≥ 25 here),
	// breaching this ceiling.
	const maxQueries = 20

	statsCtx := storage.WithQueryStats(ctx)
	storage.AssertMaxQueries(t, statsCtx, maxQueries, func() {
		if err := sub.Handle(statsCtx, evt); err != nil {
			t.Fatalf("Handle: %v", err)
		}
	})
}

// TestHandle_BusIntegration uses a real InProcessBus to verify the full
// publish → dispatch → Handle path produces push notifications.
func TestHandle_BusIntegration(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	topic := pubsub.NewMemTopic[*chat_event_bus.PublishedEvent](chat_event_bus.TopicName)
	bus := chat_event_bus.NewInProcessBus(st, topic)

	mock := &mockNotificationService{}
	sub := New(st, mock)
	if _, err := bus.Subscribe(sub); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	senderID := uuid.New().String()
	recipientID := uuid.New().String()
	communityID := uuid.New().String()

	seedCommunity(t, st, communityID, senderID)
	seedUser(t, st, senderID, "Sender")
	seedUser(t, st, recipientID, "Recipient")

	conv := &models.ChatConversation{
		Id:             uuid.New().String(),
		CommunityId:    communityID,
		ParticipantIds: []string{senderID, recipientID},
	}
	msg := &models.ChatMessage{
		Id:             uuid.New().String(),
		ConversationId: conv.Id,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: senderID,
				Text:     "hello from bus integration test",
			},
		},
	}

	if err := bus.Publish(context.Background(), chat_event_bus.KindUserMessage, msg, conv); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	dispatched := mock.dispatched()
	if len(dispatched) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(dispatched))
	}
	if dispatched[0].userID != recipientID {
		t.Errorf("expected notification to %s, got %s", recipientID, dispatched[0].userID)
	}
}
