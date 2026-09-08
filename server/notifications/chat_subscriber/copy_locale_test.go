package chat_subscriber

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/chat_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// The tests in this file exercise the localization PIPELINE, not the
// correctness of any specific translation. We assert that a non-
// default-locale Localizer is consulted (output differs from the
// English path), that ICU params round-trip in any locale, and that
// author-typed content bypasses translation. We pick English vs
// Spanish because they're the two locales we ship today; the contract
// is identical for any future language.
//
// Catalog completeness (every key present in every locale) is
// enforced by `scripts/check_l10n_toml_parity.js`. Translator wording
// is a human-review concern, not a code-test concern — we deliberately
// avoid asserting specific non-default-locale strings here so a
// future translation refinement doesn't break a green test.

func TestBuildNotification_MentionFrame_RoutesThroughLocalizer(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	communityID := uuid.New().String()
	senderID := uuid.New().String()
	recipientID := uuid.New().String()

	evt := &chat_event_bus.PublishedEvent{
		Kind: chat_event_bus.KindUserMessage,
		Message: &models.ChatMessage{
			Id:             uuid.New().String(),
			ConversationId: uuid.New().String(),
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: senderID,
					Text:     "hey everyone",
				},
			},
		},
		Conversation: &models.ChatConversation{
			Id:          uuid.New().String(),
			CommunityId: communityID,
		},
		Sender:           &api.User{Id: senderID, Name: "Alice"},
		MentionedUserIDs: []string{recipientID},
	}

	topic := resolveTopic(ctx, st, evt)
	en := buildNotification(ctx, evt, recipientID, enLoc(t), topic)
	other := buildNotification(ctx, evt, recipientID, esLoc(t), topic)
	if en == nil || other == nil {
		t.Fatal("buildNotification returned nil")
	}

	// The localizer is actually consulted (different output per locale)
	// — without this the whole notification path renders English
	// regardless of recipient locale.
	if en.Title == other.Title {
		t.Errorf("title identical across locales (%q); localizer not routing", en.Title)
	}

	// ICU params (sender name) round-trip in every locale.
	if !strings.Contains(en.Title, "Alice") || !strings.Contains(other.Title, "Alice") {
		t.Errorf("sender name dropped: en=%q other=%q", en.Title, other.Title)
	}

	// Author-typed message body is never translated — same verbatim
	// text in any locale.
	if en.Body != "hey everyone" || other.Body != "hey everyone" {
		t.Errorf("author body should round-trip verbatim; en=%q other=%q", en.Body, other.Body)
	}
}

func TestBuildNotification_SenderFallback_RoutesThroughLocalizer(t *testing.T) {
	st, cleanup := storage.SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	evt := &chat_event_bus.PublishedEvent{
		Kind: chat_event_bus.KindUserMessage,
		Message: &models.ChatMessage{
			Id:             uuid.New().String(),
			ConversationId: uuid.New().String(),
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: uuid.New().String(),
					Text:     "hello",
				},
			},
		},
		Conversation: &models.ChatConversation{
			Id:          uuid.New().String(),
			CommunityId: uuid.New().String(),
		},
		// Sender intentionally nil so the catalog fallback path runs.
	}

	recipientID := uuid.New().String()
	topic := resolveTopic(ctx, st, evt)
	en := buildNotification(ctx, evt, recipientID, enLoc(t), topic)
	other := buildNotification(ctx, evt, recipientID, esLoc(t), topic)
	if en == nil || other == nil {
		t.Fatal("buildNotification returned nil")
	}

	// Fallback sender name comes from the catalog, so it must differ
	// across locales — confirms the catalog lookup runs for both.
	if en.Title == other.Title {
		t.Errorf("fallback sender identical across locales (%q); catalog lookup not consulted", en.Title)
	}
	if en.Title == "" || other.Title == "" {
		t.Errorf("fallback title empty: en=%q other=%q", en.Title, other.Title)
	}
}
