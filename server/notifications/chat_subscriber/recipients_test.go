package chat_subscriber

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

func testLogger() *logging.Logger {
	return logging.Default()
}

func TestResolveRecipients_ParticipantList(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	senderID := uuid.New().String()
	r1 := uuid.New().String()
	r2 := uuid.New().String()

	conv := &models.ChatConversation{
		Id:             uuid.New().String(),
		ParticipantIds: []string{senderID, r1, r2},
	}

	got := resolveRecipients(context.Background(), st, conv, senderID, testLogger())
	if len(got) != 2 {
		t.Fatalf("expected 2 recipients (sender excluded), got %d: %v", len(got), got)
	}
	for _, id := range got {
		if id == senderID {
			t.Errorf("sender should not be a recipient")
		}
	}
}

func TestResolveRecipients_CommunityWide(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	communityID := uuid.New().String()
	senderID := uuid.New().String()
	m1 := uuid.New().String()
	m2 := uuid.New().String()

	ctx := context.Background()
	if _, err := st.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: senderID}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := st.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: m1}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := st.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: m2}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	conv := &models.ChatConversation{
		Id:             uuid.New().String(),
		CommunityId:    communityID,
		ParticipantIds: []string{}, // community-wide has empty participants
		Topic: &models.ConversationTopic{
			TopicId: &models.ConversationTopic_CommunityId{CommunityId: communityID},
		},
	}

	got := resolveRecipients(ctx, st, conv, senderID, testLogger())
	if len(got) != 2 {
		t.Fatalf("expected 2 recipients (3 members minus sender), got %d: %v", len(got), got)
	}
	for _, id := range got {
		if id == senderID {
			t.Errorf("sender should not be a recipient")
		}
	}
}
