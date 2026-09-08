package chat_event_bus

import (
	"context"
	"errors"
	"testing"
	"time"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

// setup returns a storage instance, a fresh InProcessBus, and a teardown helper.
func setup(t *testing.T) (*storage.ProtoSQLStorage, *InProcessBus, func()) {
	t.Helper()
	st, cleanup := storage.SetupTestStorage(t)
	topic := pubsub.NewMemTopic[*PublishedEvent](TopicName)
	bus := NewInProcessBus(st, topic)
	return st, bus, cleanup
}

func testConversation(id string) *models.ChatConversation {
	return &models.ChatConversation{Id: id, CommunityId: "community-1"}
}

func testUserMessage(id, senderID, text string) *models.ChatMessage {
	return &models.ChatMessage{
		Id:             id,
		ConversationId: "conv-1",
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: senderID,
				Text:     text,
			},
		},
	}
}

func TestInProcessBus_PublishDispatchesToSubscribers(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("test-sub")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	ctx := context.Background()
	conv := testConversation("conv-1")
	msg := testUserMessage("msg-1", "user-1", "hello")

	if err := bus.Publish(ctx, KindUserMessage, msg, conv); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events, want 1", len(got))
	}
	if got[0].Kind != KindUserMessage {
		t.Errorf("event kind = %d, want %d", got[0].Kind, KindUserMessage)
	}
	if got[0].Message == nil || got[0].Message.Id != "msg-1" {
		t.Errorf("event message = %v, want msg-1", got[0].Message)
	}
}

func TestInProcessBus_PublishPrefetchesSenderAndTransfer(t *testing.T) {
	st, bus, cleanup := setup(t)
	defer cleanup()

	ctx := context.Background()

	// Seed a user and a transfer.
	user := &models.User{Name: "Alice", Email: "alice@example.com", Role: models.Role_ROLE_USER}
	userID, err := st.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Insert user: %v", err)
	}

	transfer := &models.Transfer{OwnerId: userID, RecipientId: "other", GearId: "gear-1"}
	transferID, err := st.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("Insert transfer: %v", err)
	}

	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("prefetch-checker")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	conv := &models.ChatConversation{
		Id:          "conv-1",
		CommunityId: "community-1",
		Topic: &models.ConversationTopic{
			TopicId: &models.ConversationTopic_TransferId{TransferId: transferID},
		},
	}
	msg := testUserMessage("msg-1", userID, "test")

	if err := bus.Publish(ctx, KindUserMessage, msg, conv); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events, want 1", len(got))
	}
	pe := got[0]
	if pe.Sender == nil {
		t.Error("Sender should be prefetched, got nil")
	} else if pe.Sender.Name != "Alice" {
		t.Errorf("Sender.Name = %q, want Alice", pe.Sender.Name)
	}
	if pe.Transfer == nil {
		t.Error("Transfer should be prefetched, got nil")
	} else if pe.Transfer.GearId != "gear-1" {
		t.Errorf("Transfer.GearId = %q, want gear-1", pe.Transfer.GearId)
	}
}

func TestInProcessBus_PrefetchFailureDoesNotAbortDispatch(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("missing-sender")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	conv := testConversation("conv-1")
	msg := testUserMessage("msg-1", "missing-user", "test")

	if err := bus.Publish(context.Background(), KindUserMessage, msg, conv); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events despite prefetch errors, want 1, got %d", len(got), 1)
	}
	if got[0].Sender != nil {
		t.Errorf("expected Sender nil after prefetch failure, got %+v", got[0].Sender)
	}
}

func TestInProcessBus_NilConversationReturnsError(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	msg := testUserMessage("msg-1", "user-1", "test")
	if err := bus.Publish(context.Background(), KindUserMessage, msg, nil); err == nil {
		t.Error("Publish(nil conversation) should return an error")
	}
}

func TestInProcessBus_NilMessageReturnsErrorForNonReactionKind(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	conv := testConversation("conv-1")
	if err := bus.Publish(context.Background(), KindUserMessage, nil, conv); err == nil {
		t.Error("Publish(nil message, KIND_USER_MESSAGE) should return an error")
	}
}

func TestInProcessBus_NilMessageAcceptedForReactionUpdate(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("reaction-sub")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	conv := testConversation("conv-1")
	ru := &api.ReactionUpdate{MessageId: "msg-1"}
	if err := bus.Publish(context.Background(), KindReactionUpdate, nil, conv,
		WithReactionUpdate(ru)); err != nil {
		t.Fatalf("Publish(KindReactionUpdate, nil msg) should be accepted: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events, want 1", len(got))
	}
	if got[0].Kind != KindReactionUpdate {
		t.Errorf("kind = %d, want KindReactionUpdate", got[0].Kind)
	}
	if got[0].ReactionUpdate == nil {
		t.Error("ReactionUpdate should be set, got nil")
	}
}

func TestInProcessBus_MultipleSubscribersIsolated(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	good := pubsub.NewFakeSubscriber[*PublishedEvent]("good")
	bad := pubsub.NewFakeSubscriber[*PublishedEvent]("bad")
	bad.HandleErr = errors.New("subscriber boom")

	unsubGood, _ := bus.Subscribe(good)
	defer unsubGood()
	unsubBad, _ := bus.Subscribe(bad)
	defer unsubBad()

	conv := testConversation("conv-1")
	msg := testUserMessage("msg-1", "user-1", "test")

	if err := bus.Publish(context.Background(), KindUserMessage, msg, conv); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	if len(good.Received()) != 1 {
		t.Errorf("good subscriber received %d events, want 1", len(good.Received()))
	}
	if len(bad.Received()) != 1 {
		t.Errorf("bad subscriber should still have seen the event, got %d", len(bad.Received()))
	}
}

func TestInProcessBus_PublishOptionsMentionedUserIDs(t *testing.T) {
	_, bus, cleanup := setup(t)
	defer cleanup()

	sub := pubsub.NewFakeSubscriber[*PublishedEvent]("mention-checker")
	unsub, err := bus.Subscribe(sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsub()

	conv := testConversation("conv-1")
	msg := testUserMessage("msg-1", "user-1", "hi @user-2")
	if err := bus.Publish(context.Background(), KindUserMessage, msg, conv,
		WithMentionedUserIDs([]string{"user-2"})); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	got := sub.Received()
	if len(got) != 1 {
		t.Fatalf("subscriber received %d events, want 1", len(got))
	}
	if len(got[0].MentionedUserIDs) != 1 || got[0].MentionedUserIDs[0] != "user-2" {
		t.Errorf("MentionedUserIDs = %v, want [user-2]", got[0].MentionedUserIDs)
	}
}

func TestPublishedEvent_LogContextAlwaysIncludesCanonicalFields(t *testing.T) {
	pe := &PublishedEvent{
		Kind:         KindUserMessage,
		Message:      &models.ChatMessage{Id: "msg-123"},
		Conversation: &models.ChatConversation{Id: "conv-456"},
	}
	got := pe.LogContext()
	want := map[string]any{
		"message_id":      "msg-123",
		"conversation_id": "conv-456",
	}
	for i := 0; i+1 < len(got); i += 2 {
		key, _ := got[i].(string)
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("LogContext missing expected keys: %v", want)
	}
}

func TestPublishedEvent_LogContextSafeOnNil(t *testing.T) {
	var pe *PublishedEvent
	if got := pe.LogContext(); got != nil {
		t.Errorf("(*PublishedEvent)(nil).LogContext() = %v, want nil", got)
	}
}
