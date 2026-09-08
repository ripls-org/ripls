package chat

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// newDescriptionTestConversation creates a conversation with the given
// participants and returns the writer, mock bus, and conversation ID.
func newDescriptionTestConversation(t *testing.T, store *storage.ProtoSQLStorage) (*SystemMessageWriter, *chat_event_bus.MockBus, string) {
	t.Helper()
	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"creator1", "user2"},
	}
	convID, err := store.Insert(context.Background(), conversation)
	if err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	mockBus := chat_event_bus.NewMockBus()
	return NewSystemMessageWriter(store, mockBus), mockBus, convID
}

func TestPostCreationDescription_EmptyIsNoOp(t *testing.T) {
	store, _ := setupTestStorage(t)
	ctx := context.Background()

	for _, desc := range []string{"", "   ", "\n\t "} {
		writer, mockBus, convID := newDescriptionTestConversation(t, store)
		if err := PostCreationDescription(ctx, writer, convID, "creator1", desc); err != nil {
			t.Fatalf("PostCreationDescription(%q) returned error: %v", desc, err)
		}
		if mockBus.CallCount() != 0 {
			t.Errorf("PostCreationDescription(%q) published %d events, want 0", desc, mockBus.CallCount())
		}
		msgs, err := storage.ListByConversation(ctx, store, convID, true, 0)
		if err != nil {
			t.Fatalf("ListByConversation failed: %v", err)
		}
		if len(msgs) != 0 {
			t.Errorf("PostCreationDescription(%q) inserted %d messages, want 0", desc, len(msgs))
		}
	}
}

func TestPostCreationDescription_NilWriterOrConversation(t *testing.T) {
	ctx := context.Background()
	if err := PostCreationDescription(ctx, nil, "conv1", "creator1", "hello"); err != nil {
		t.Errorf("nil writer should be a no-op, got error: %v", err)
	}

	store, _ := setupTestStorage(t)
	writer, _, _ := newDescriptionTestConversation(t, store)
	if err := PostCreationDescription(ctx, writer, "", "creator1", "hello"); err != nil {
		t.Errorf("empty conversation ID should be a no-op, got error: %v", err)
	}
}

func TestPostCreationDescription_PostsVerbatimAsUserMessage(t *testing.T) {
	store, _ := setupTestStorage(t)
	writer, mockBus, convID := newDescriptionTestConversation(t, store)

	// Use a fixed simulation clock so we can assert the +1s offset.
	now := time.Unix(1_700_000_000, 0)
	ctx := clock.WithSimulationTime(context.Background(), now)

	// Description with surrounding whitespace must be posted verbatim so it
	// matches the description stored on the entity exactly.
	const desc = "  Bring your own mug, please.\n"
	if err := PostCreationDescription(ctx, writer, convID, "creator1", desc); err != nil {
		t.Fatalf("PostCreationDescription returned error: %v", err)
	}

	msgs, err := storage.ListByConversation(ctx, store, convID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	user := msgs[0].GetUserMessage()
	if user == nil {
		t.Fatal("inserted message is not a user message")
	}
	if user.SenderId != "creator1" {
		t.Errorf("sender = %q, want creator1", user.SenderId)
	}
	if user.Text != desc {
		t.Errorf("text = %q, want verbatim %q", user.Text, desc)
	}
	if got, want := msgs[0].SentAtUnixSec, now.Unix()+1; got != want {
		t.Errorf("sent_at = %d, want now+1 (%d) so it sorts after the anchor", got, want)
	}

	// The comment is delivered live over the chat bus as a user message.
	if mockBus.CallCount() != 1 {
		t.Fatalf("expected 1 bus publish, got %d", mockBus.CallCount())
	}
	if kind := mockBus.Captured()[0].Kind; kind != chat_event_bus.KindUserMessage {
		t.Errorf("published kind = %v, want KindUserMessage", kind)
	}
}

// TestPostCreationDescription_SortsAfterAnchor verifies that a creation anchor
// system message inserted in the same second still sorts before the seeded
// description comment, since history breaks sent_at ties on a random UUID id.
func TestPostCreationDescription_SortsAfterAnchor(t *testing.T) {
	store, _ := setupTestStorage(t)
	writer, _, convID := newDescriptionTestConversation(t, store)

	now := time.Unix(1_700_000_500, 0)
	ctx := clock.WithSimulationTime(context.Background(), now)

	if err := writer.InsertLocalized(
		ctx,
		convID,
		"creator1",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED,
		ExperienceCreatedMessage("Alex", "Trail Cleanup"),
	); err != nil {
		t.Fatalf("InsertLocalized(anchor) failed: %v", err)
	}
	if err := PostCreationDescription(ctx, writer, convID, "creator1", "Meet at the north gate."); err != nil {
		t.Fatalf("PostCreationDescription failed: %v", err)
	}

	msgs, err := storage.ListByConversation(ctx, store, convID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if !IsCreationAnchorMessage(msgs[0]) {
		t.Errorf("first message should be the creation anchor, got %+v", msgs[0])
	}
	if msgs[1].GetUserMessage() == nil {
		t.Errorf("second message should be the description user comment, got %+v", msgs[1])
	}
}

func TestPostGearCreationMessages_SeedsAnchorThenDescription(t *testing.T) {
	cases := []struct {
		name         string
		availability models.Availability
		wantAction   models.ChatSystemAction
	}{
		{
			name:         "loan",
			availability: models.Availability_AVAILABILITY_FOR_LOAN,
			wantAction:   models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED,
		},
		{
			name:         "giveaway",
			availability: models.Availability_AVAILABILITY_FOR_GIVEAWAY,
			wantAction:   models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, _ := setupTestStorage(t)
			writer, _, convID := newDescriptionTestConversation(t, store)
			ctx := context.Background()

			const desc = "Sturdy 24ft fiberglass ladder."
			if err := PostGearCreationMessages(ctx, writer, convID, "creator1", "Owner", "Ladder", desc, tc.availability); err != nil {
				t.Fatalf("PostGearCreationMessages failed: %v", err)
			}

			msgs, err := storage.ListByConversation(ctx, store, convID, true, 0)
			if err != nil {
				t.Fatalf("ListByConversation failed: %v", err)
			}
			if len(msgs) != 2 {
				t.Fatalf("expected 2 messages (anchor + description), got %d", len(msgs))
			}
			sys := msgs[0].GetSystemMessage()
			if sys == nil {
				t.Fatalf("first message should be the sharing anchor, got %+v", msgs[0])
			}
			if sys.Action != tc.wantAction {
				t.Errorf("anchor action = %v, want %v", sys.Action, tc.wantAction)
			}
			user := msgs[1].GetUserMessage()
			if user == nil {
				t.Fatalf("second message should be the description comment, got %+v", msgs[1])
			}
			if user.SenderId != "creator1" {
				t.Errorf("comment sender = %q, want creator1", user.SenderId)
			}
			if user.Text != desc {
				t.Errorf("comment text = %q, want %q", user.Text, desc)
			}
		})
	}
}

func TestPostGearCreationMessages_NoOpCases(t *testing.T) {
	store, _ := setupTestStorage(t)
	ctx := context.Background()

	// Unspecified availability: no sharing context, so nothing is seeded.
	writer, _, convID := newDescriptionTestConversation(t, store)
	if err := PostGearCreationMessages(ctx, writer, convID, "creator1", "Owner", "Ladder", "desc", models.Availability_AVAILABILITY_UNSPECIFIED); err != nil {
		t.Fatalf("unspecified availability returned error: %v", err)
	}
	msgs, err := storage.ListByConversation(ctx, store, convID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("unspecified availability should seed no messages, got %d", len(msgs))
	}

	// Empty description on a loan: the anchor is still posted, but no comment.
	writer2, _, convID2 := newDescriptionTestConversation(t, store)
	if err := PostGearCreationMessages(ctx, writer2, convID2, "creator1", "Owner", "Ladder", "  ", models.Availability_AVAILABILITY_FOR_LOAN); err != nil {
		t.Fatalf("empty description returned error: %v", err)
	}
	msgs2, err := storage.ListByConversation(ctx, store, convID2, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs2) != 1 {
		t.Fatalf("empty description should seed only the anchor, got %d messages", len(msgs2))
	}
	if msgs2[0].GetSystemMessage() == nil {
		t.Errorf("the single message should be the anchor, got %+v", msgs2[0])
	}
}
