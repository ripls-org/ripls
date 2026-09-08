package chat_subscriber

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/chat_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// Helper: build a baseline KindUserMessage event for use across topic-specific
// title-resolution tests. Each test customizes the conversation's topic and
// any associated prefetched entities (Transfer) before calling buildNotification.
func baseChatEvent(senderID, senderName, communityID, conversationID string) *chat_event_bus.PublishedEvent {
	return &chat_event_bus.PublishedEvent{
		Kind: chat_event_bus.KindUserMessage,
		Message: &models.ChatMessage{
			Id:             uuid.New().String(),
			ConversationId: conversationID,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: senderID,
					Text:     "Hello",
				},
			},
		},
		Conversation: &models.ChatConversation{
			Id:          conversationID,
			CommunityId: communityID,
		},
		Sender: &api.User{Id: senderID, Name: senderName},
	}
}

func TestBuildNotification_ConversationTitle_GearTopic(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	communityID := uuid.New().String()
	ownerID := uuid.New().String()
	if _, err := st.Insert(ctx, &models.Gear{
		Id: "gear-1", Name: "Cordless Drill", OwnerId: ownerID,
	}); err != nil {
		t.Fatalf("seed gear: %v", err)
	}

	evt := baseChatEvent(ownerID, "Owner", communityID, uuid.New().String())
	evt.Conversation.Topic = &models.ConversationTopic{
		TopicId: &models.ConversationTopic_GearId{GearId: "gear-1"},
	}

	got := buildNotification(ctx, evt, "recipient-1", enLoc(t), resolveTopic(ctx, st, evt))
	cm := got.GetChatMessage()
	if cm == nil {
		t.Fatal("expected ChatMessage payload")
	}
	if cm.GetConversationTitle() != "Cordless Drill" {
		t.Errorf("ConversationTitle = %q, want %q", cm.GetConversationTitle(), "Cordless Drill")
	}
	if cm.GetGearId() != "gear-1" {
		t.Errorf("GearId = %q, want %q", cm.GetGearId(), "gear-1")
	}
}

func TestBuildNotification_ConversationTitle_ExperienceTopic(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	communityID := uuid.New().String()
	hostID := uuid.New().String()
	if _, err := st.Insert(ctx, &models.Experience{
		Id: "exp-1", Name: "Hiking at Zilker", OwnerId: hostID,
	}); err != nil {
		t.Fatalf("seed experience: %v", err)
	}

	evt := baseChatEvent(hostID, "Host", communityID, uuid.New().String())
	evt.Conversation.Topic = &models.ConversationTopic{
		TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: "exp-1"},
	}

	got := buildNotification(ctx, evt, "recipient-1", enLoc(t), resolveTopic(ctx, st, evt))
	cm := got.GetChatMessage()
	if cm.GetConversationTitle() != "Hiking at Zilker" {
		t.Errorf("ConversationTitle = %q, want %q", cm.GetConversationTitle(), "Hiking at Zilker")
	}
	if cm.GetExperienceId() != "exp-1" {
		t.Errorf("ExperienceId = %q, want %q", cm.GetExperienceId(), "exp-1")
	}
}

func TestBuildNotification_ConversationTitle_RequestTopic(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	communityID := uuid.New().String()
	requesterID := uuid.New().String()
	if _, err := st.Insert(ctx, &models.Request{
		Id: "req-1", Title: "Need a ladder", RequesterId: requesterID,
	}); err != nil {
		t.Fatalf("seed request: %v", err)
	}

	evt := baseChatEvent(requesterID, "Requester", communityID, uuid.New().String())
	evt.Conversation.Topic = &models.ConversationTopic{
		TopicId: &models.ConversationTopic_RequestId{RequestId: "req-1"},
	}

	got := buildNotification(ctx, evt, "recipient-1", enLoc(t), resolveTopic(ctx, st, evt))
	cm := got.GetChatMessage()
	if cm.GetConversationTitle() != "Need a ladder" {
		t.Errorf("ConversationTitle = %q, want %q", cm.GetConversationTitle(), "Need a ladder")
	}
	if cm.GetRequestId() != "req-1" {
		t.Errorf("RequestId = %q, want %q", cm.GetRequestId(), "req-1")
	}
}

func TestBuildNotification_ConversationTitle_TransferTopicUsesPrefetched(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	communityID := uuid.New().String()
	ownerID := uuid.New().String()
	if _, err := st.Insert(ctx, &models.Gear{
		Id: "gear-x", Name: "Folding Table", OwnerId: ownerID,
	}); err != nil {
		t.Fatalf("seed gear: %v", err)
	}

	evt := baseChatEvent(ownerID, "Owner", communityID, uuid.New().String())
	evt.Conversation.Topic = &models.ConversationTopic{
		TopicId: &models.ConversationTopic_TransferId{TransferId: "transfer-1"},
	}
	evt.Transfer = &models.Transfer{Id: "transfer-1", GearId: "gear-x"}

	got := buildNotification(ctx, evt, "recipient-1", enLoc(t), resolveTopic(ctx, st, evt))
	cm := got.GetChatMessage()
	if cm.GetConversationTitle() != "Folding Table" {
		t.Errorf("ConversationTitle = %q, want %q", cm.GetConversationTitle(), "Folding Table")
	}
	if cm.GetGearId() != "gear-x" {
		t.Errorf("GearId = %q, want %q (transfer should resolve to gear)", cm.GetGearId(), "gear-x")
	}
}

func TestBuildNotification_ConversationTitle_CommunityWide(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	communityID := uuid.New().String()
	ownerID := uuid.New().String()
	if _, err := st.Insert(ctx, &models.Community{
		Id: communityID, Name: "Oakwood Heights Neighbors",
		CreatorId: ownerID, OwnerUserId: ownerID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}

	// Community-wide conversation has no Topic.
	evt := baseChatEvent(ownerID, "Owner", communityID, uuid.New().String())

	got := buildNotification(ctx, evt, "recipient-1", enLoc(t), resolveTopic(ctx, st, evt))
	cm := got.GetChatMessage()
	if cm.GetConversationTitle() != "Oakwood Heights Neighbors" {
		t.Errorf("ConversationTitle = %q, want %q (community-wide falls back to community name)",
			cm.GetConversationTitle(), "Oakwood Heights Neighbors")
	}
}

func TestBuildNotification_ConversationTitle_MissingEntityIsBenign(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Topic references a gear that was never inserted — best-effort lookup
	// returns "" and ConversationTitle stays unset. The notification itself
	// must still be built (the user shouldn't lose a push because the topic
	// entity was deleted between message-send and notification-build).
	evt := baseChatEvent(uuid.New().String(), "Sender", "", uuid.New().String())
	evt.Conversation.Topic = &models.ConversationTopic{
		TopicId: &models.ConversationTopic_GearId{GearId: "nonexistent-gear"},
	}

	got := buildNotification(ctx, evt, "recipient-1", enLoc(t), resolveTopic(ctx, st, evt))
	if got == nil {
		t.Fatal("notification should still be built when topic entity is missing")
	}
	cm := got.GetChatMessage()
	if cm.GetConversationTitle() != "" {
		t.Errorf("ConversationTitle = %q, want empty for missing entity",
			cm.GetConversationTitle())
	}
	// Sender + preview must still be intact.
	if cm.GetSenderName() != "Sender" {
		t.Errorf("SenderName = %q, want %q", cm.GetSenderName(), "Sender")
	}
}
