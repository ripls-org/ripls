package chat

import (
	"context"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestSystemMessageWriter_TemplateKeyAndParamsRoundTrip confirms Phase 4b's
// structured-template payload (template_key + template_params) persists
// through storage when emitted via InsertLocalized. Without this the
// client would never receive the locale-renderable payload.
func TestSystemMessageWriter_TemplateKeyAndParamsRoundTrip(t *testing.T) {
	storage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2"},
	}
	convID, err := storage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Insert conversation: %v", err)
	}

	writer := NewSystemMessageWriter(storage, nil)

	// Use a real lifecycle helper so the test covers the whole
	// LocalizedMessage → InsertLocalized → storage path that
	// production emit sites traverse.
	if err := writer.InsertLocalized(
		ctx,
		convID,
		"actor123",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED,
		ApprovedMessage("Alice"),
	); err != nil {
		t.Fatalf("InsertLocalized: %v", err)
	}

	messages, err := storage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByField: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	sys := messages[0].(*models.ChatMessage).GetSystemMessage()
	if sys == nil {
		t.Fatal("expected SystemMessage variant")
	}

	if sys.GetTemplateKey() != "chat.transfer.approved" {
		t.Errorf("TemplateKey round-trip: got %q; want %q",
			sys.GetTemplateKey(), "chat.transfer.approved")
	}
	if sys.TemplateParams["recipientName"] != "Alice" {
		t.Errorf("TemplateParams round-trip: got %v; want recipientName=Alice",
			sys.TemplateParams)
	}
	// Wire-compatibility: the literal description must still be
	// present for older clients without LocalizedMessage handling.
	if sys.Description != "Alice was selected as the recipient" {
		t.Errorf("Description should preserve the literal text for fallback; got %q",
			sys.Description)
	}
}

// TestSystemMessageWriter_LegacyInsertOmitsTemplateKey confirms a
// pre-migration emit site (using InsertSystemMessage with a literal
// text string) persists no template_key, so the client correctly
// falls back to the literal description for historical rows.
func TestSystemMessageWriter_LegacyInsertOmitsTemplateKey(t *testing.T) {
	storage, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1"},
	}
	convID, err := storage.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("Insert conversation: %v", err)
	}

	writer := NewSystemMessageWriter(storage, nil)
	if err := writer.InsertSystemMessage(
		ctx,
		convID,
		"actor123",
		models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
		"Actor joined the conversation",
	); err != nil {
		t.Fatalf("InsertSystemMessage: %v", err)
	}

	messages, err := storage.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByField: %v", err)
	}
	sys := messages[0].(*models.ChatMessage).GetSystemMessage()
	if sys.TemplateKey != nil {
		t.Errorf("TemplateKey should be nil for legacy insert; got %q", sys.GetTemplateKey())
	}
	if len(sys.TemplateParams) != 0 {
		t.Errorf("TemplateParams should be empty for legacy insert; got %v", sys.TemplateParams)
	}
}

func TestTextHelpers(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() string
		expected string
	}{
		{"CancelledText", CancelledText, "This request was cancelled"},
		{"StartedText", StartedText, "The loan has started"},
		{"FulfilledText", FulfilledText, "This request has been fulfilled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(); got != tt.expected {
				t.Errorf("%s() = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

// TestJourneyPolishTemplates covers the #2724 chat-copy templates: the
// possessive-free gear offers (I3c), the combined offer-for-need line (I3a),
// the accept beat (I3b), the raise-a-hand register (I3d), and the
// jargon-free completion lines (I3e).
func TestJourneyPolishTemplates(t *testing.T) {
	tests := []struct {
		name     string
		msg      LocalizedMessage
		wantText string
		wantKey  string
	}{
		{
			name:     "gear offer lend drops the possessive",
			msg:      GearOfferedMessage("Lisa", "Honda mower", false),
			wantText: "Lisa is lending Honda mower",
			wantKey:  "chat.request.gear_offer_lend",
		},
		{
			name:     "gear offer give drops the possessive",
			msg:      GearOfferedMessage("Maya", "Bread machine", true),
			wantText: "Maya is giving Bread machine",
			wantKey:  "chat.request.gear_offer_give",
		},
		{
			name:     "combined need + lend line",
			msg:      GearOfferedForNeedMessage("Lisa", "Lawn mower", "Honda mower", false),
			wantText: "Lisa is bringing Lawn mower — lending Honda mower",
			wantKey:  "chat.request.gear_offer_lend_for_need",
		},
		{
			name:     "combined need + give line",
			msg:      GearOfferedForNeedMessage("Maya", "Bread", "Sourdough loaf", true),
			wantText: "Maya is bringing Bread — giving Sourdough loaf",
			wantKey:  "chat.request.gear_offer_give_for_need",
		},
		{
			name:     "accept beat",
			msg:      OfferAcceptedMessage("June", "Lisa"),
			wantText: "June accepted Lisa's offer",
			wantKey:  "chat.request.offer_accepted",
		},
		{
			name:     "raised a hand register",
			msg:      RaisedHandMessage("Theo"),
			wantText: "Theo raised a hand",
			wantKey:  "chat.transfer.raised_hand",
		},
		{
			name:     "loan completion without transfer jargon",
			msg:      HandedOffMessage(),
			wantText: "Handed off ✓",
			wantKey:  "chat.transfer.handed_off",
		},
		{
			name:     "giveaway completion names the recipient",
			msg:      GivenToMessage("June"),
			wantText: "Given to June",
			wantKey:  "chat.transfer.given_to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.Text != tt.wantText {
				t.Errorf("Text = %q, want %q", tt.msg.Text, tt.wantText)
			}
			if tt.msg.TemplateKey != tt.wantKey {
				t.Errorf("TemplateKey = %q, want %q", tt.msg.TemplateKey, tt.wantKey)
			}
		})
	}
}

func TestTextHelpersWithParams(t *testing.T) {
	tests := []struct {
		name     string
		fn       func(string) string
		input    string
		expected string
	}{
		{"JoinedText", JoinedText, "Alex", "Alex joined the conversation"},
		{"LeftText", LeftText, "Bob", "Bob left the conversation"},
		{"ApprovedText", ApprovedText, "Carol", "Carol was selected as the recipient"},
		{"DeniedText", DeniedText, "Dave", "Dave was removed from consideration"},
		{"OfferedText", OfferedText, "Eve", "Eve offered to help"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(tt.input); got != tt.expected {
				t.Errorf("%s(%q) = %q, want %q", tt.name, tt.input, got, tt.expected)
			}
		})
	}
}

// TestTimeAndLocationTemplateVariants covers the #2827 chat-param migration:
// concrete times carry only the structured instant, informal descriptions
// pass through untranslated, and TBD/unnamed states select dedicated template
// keys instead of server-invented English placeholders.
// TestUnknownActorStaysOutOfParams pins the #2844 split: an unresolved actor
// gets an English stand-in in the legacy `description` text, but reaches the
// client as an empty param so the client can render its own localized one.
// Putting "Someone" in the params instead would splice an English word into
// an otherwise-translated sentence — the render-boundary violation this fixed.
func TestUnknownActorStaysOutOfParams(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  LocalizedMessage
	}{
		{"no args", ExperienceStartedMessage("")},
		{"extra args", GearOfferedMessage("", "Drill", false)},
		{"time-bearing", TimeConfirmedMessage("", TimeParam{TBD: true})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.msg.Params["actorName"]; got != "" {
				t.Errorf("actorName param = %q, want empty for an unknown actor", got)
			}
			if !strings.HasPrefix(tc.msg.Text, "Someone ") {
				t.Errorf("English fallback = %q, want it to lead with the stand-in", tc.msg.Text)
			}
		})
	}

	// A real name is untouched on both sides.
	named := ExperienceStartedMessage("Ana")
	if named.Params["actorName"] != "Ana" || !strings.HasPrefix(named.Text, "Ana ") {
		t.Errorf("named actor mangled: text=%q params=%v", named.Text, named.Params)
	}
}

func TestTimeAndLocationTemplateVariants(t *testing.T) {
	concrete := TimeConfirmedMessage("Ana", TimeParam{
		UnixSec:      1735747200,
		UTCOffsetMin: -420,
	})
	if concrete.TemplateKey != "chat.experience.time_confirmed" {
		t.Errorf("concrete key = %q", concrete.TemplateKey)
	}
	if concrete.Params["timeUnixSec"] != "1735747200" ||
		concrete.Params["timeUtcOffsetMin"] != "-420" {
		t.Errorf("concrete structured params = %v", concrete.Params)
	}
	// A concrete instant carries no rendered timestamp: the client formats it
	// in the viewer's locale, and an English one here would land inside an
	// otherwise-translated sentence (#2844).
	if _, hasRendered := concrete.Params["formattedTime"]; hasRendered {
		t.Errorf("concrete time must not carry rendered English: %v", concrete.Params)
	}
	// The English `description` column still reads as a sentence, though.
	if !strings.Contains(concrete.Text, "Ana confirmed the time: ") {
		t.Errorf("concrete English fallback = %q", concrete.Text)
	}

	informal := DetailChangedTimeMessage("Ana", TimeParam{Description: "Late March"})
	if informal.TemplateKey != "chat.experience.detail_changed_time" {
		t.Errorf("informal key = %q", informal.TemplateKey)
	}
	if informal.Params["formattedTime"] != "Late March" {
		t.Errorf("informal params = %v", informal.Params)
	}
	if _, hasInstant := informal.Params["timeUnixSec"]; hasInstant {
		t.Errorf("informal description must not carry an instant: %v", informal.Params)
	}

	tbdChanged := DetailChangedTimeMessage("Ana", TimeParam{TBD: true})
	if tbdChanged.TemplateKey != "chat.experience.detail_changed_time_tbd" {
		t.Errorf("tbd-changed key = %q", tbdChanged.TemplateKey)
	}
	if tbdChanged.Text != "Ana updated time → TBD" {
		t.Errorf("tbd-changed fallback text = %q", tbdChanged.Text)
	}

	tbdConfirmed := TimeConfirmedMessage("Ana", TimeParam{TBD: true})
	if tbdConfirmed.TemplateKey != "chat.experience.time_confirmed_tbd" {
		t.Errorf("tbd-confirmed key = %q", tbdConfirmed.TemplateKey)
	}

	unnamed := DetailChangedLocationUnnamedMessage("Ana")
	if unnamed.TemplateKey != "chat.experience.detail_changed_location_unnamed" {
		t.Errorf("unnamed-location key = %q", unnamed.TemplateKey)
	}
	if unnamed.Text != "Ana updated the location" {
		t.Errorf("unnamed-location fallback text = %q", unnamed.Text)
	}
}
