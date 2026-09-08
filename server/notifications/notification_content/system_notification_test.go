package notification_content

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
)

// TestSystemNotificationsReachOffApp is the #2896 regression test at the level
// the bug actually lived: the payload a system producer emits, rendered on the
// surface the user complained about.
//
// Every one of these event types used to fall through to the actorless default
// line and text the recipient "Ripls:  posted an update in " — two blank slots
// and a dangling preposition — because the producer's copy existed only as a
// fmt.Sprintf in its own package.
func TestSystemNotificationsReachOffApp(t *testing.T) {
	loc := enLoc(t)
	ctx := context.Background()
	expID, reqID := "e-1", "r-1"

	tests := []struct {
		name        string
		payload     *models.CommunityEventPayload
		wantMessage string
		wantCTA     string
		wantTitle   string
	}{
		{
			name: "event reminder 2h before",
			payload: &models.CommunityEventPayload{
				EventType:      "EXPERIENCE_REMINDER_TWO_HOUR",
				ExperienceName: "Backyard BBQ",
				ExperienceId:   &expID,
			},
			wantMessage: "Backyard BBQ starts in 2 hours",
			wantCTA:     "See details at",
			wantTitle:   "Event starting soon",
		},
		{
			name: "event reminder day before",
			payload: &models.CommunityEventPayload{
				EventType:      "EXPERIENCE_REMINDER_DAY_BEFORE",
				ExperienceName: "Backyard BBQ",
			},
			wantMessage: "Backyard BBQ is tomorrow",
			wantCTA:     "See details at",
			wantTitle:   "Event tomorrow",
		},
		{
			name: "close prompt",
			payload: &models.CommunityEventPayload{
				EventType:      "EXPERIENCE_CLOSE_PROMPT",
				ExperienceName: "Backyard BBQ",
			},
			wantMessage: "Mark Backyard BBQ as completed when you're done",
			wantCTA:     "Wrap it up at",
			wantTitle:   "Wrap up your event",
		},
		{
			name: "loan return overdue",
			payload: &models.CommunityEventPayload{
				EventType: "LOAN_RETURN_REMINDER_OVERDUE",
				GearName:  "Pressure Cooker",
				ActorName: "Alice",
			},
			wantMessage: "Pressure Cooker is overdue — please return it to Alice",
			wantCTA:     "See the loan at",
			wantTitle:   "Overdue",
		},
		{
			name: "request followup",
			payload: &models.CommunityEventPayload{
				EventType:    "REQUEST_FOLLOWUP_PROMPT",
				RequestTitle: "Folding Table",
				RequestId:    &reqID,
			},
			wantMessage: "You asked for Folding Table — did your community come through?",
			wantCTA:     "Close it out at",
			wantTitle:   "Still need this?",
		},
		{
			name: "experience needs nudge",
			payload: &models.CommunityEventPayload{
				EventType:      "EXPERIENCE_NEEDS_NUDGE",
				ExperienceName: "Backyard BBQ",
			},
			wantMessage: "Backyard BBQ still needs a few things",
			wantCTA:     "Claim one at",
			wantTitle:   "Who's bringing what?",
		},
		{
			name: "request needs nudge",
			payload: &models.CommunityEventPayload{
				EventType:    "REQUEST_NEEDS_NUDGE",
				RequestTitle: "Folding Table",
			},
			wantMessage: "Folding Table still needs a hand",
			wantCTA:     "Claim something at",
			wantTitle:   "Can you help?",
		},
		{
			// The poll nudges' old push body named no entity at all — the event
			// name lived only in the title, which off-app surfaces do not show.
			name: "time poll nudge",
			payload: &models.CommunityEventPayload{
				EventType:      "EXPERIENCE_TIME_NUDGE",
				ExperienceName: "Backyard BBQ",
			},
			wantMessage: "Pick a time that works for Backyard BBQ",
			wantCTA:     "Vote at",
			wantTitle:   "When should we meet?",
		},
		{
			name: "location poll nudge",
			payload: &models.CommunityEventPayload{
				EventType:      "EXPERIENCE_LOCATION_NUDGE",
				ExperienceName: "Backyard BBQ",
			},
			wantMessage: "Pick a place for Backyard BBQ",
			wantCTA:     "Vote at",
			wantTitle:   "Where should we meet?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := FromCommunityEvent(ctx, tt.payload)
			if c.Kind == "default" {
				t.Fatalf("%q still falls through to the default kind — the copy will not name the entity",
					tt.payload.EventType)
			}

			// SMS: branded, the real sentence, the CTA on the link.
			sms := c.RenderSMS(ctx, loc, "https://example.com/go/abc", false)
			wantSMS := "Ripls: " + tt.wantMessage + "\n\n" + tt.wantCTA + " https://example.com/go/abc"
			if sms != wantSMS {
				t.Errorf("SMS =\n%q\nwant\n%q", sms, wantSMS)
			}

			// Email: the same sentence as the subject, never a blank one.
			parts := c.RenderEmail(ctx, loc)
			if parts.Subject != tt.wantMessage {
				t.Errorf("email subject = %q, want %q", parts.Subject, tt.wantMessage)
			}
			if strings.TrimSpace(parts.CTALabel) == "" {
				t.Error("email CTA label is empty")
			}

			// Push: the same sentence, with the kind's title as its only
			// surface-specific adornment.
			notif := SystemNotification(ctx, tt.payload)
			if notif.Title != tt.wantTitle {
				t.Errorf("push title = %q, want %q", notif.Title, tt.wantTitle)
			}
			if notif.Body != tt.wantMessage {
				t.Errorf("push body = %q, want the shared sentence %q", notif.Body, tt.wantMessage)
			}
			// The payload rides along untouched — the off-app senders re-render
			// from it rather than reusing Title/Body.
			if notif.GetCommunityEvent() != tt.payload {
				t.Error("SystemNotification must carry the payload through verbatim")
			}
		})
	}
}

// TestSystemNotificationLocalizesOffApp asserts the one sentence really is one
// sentence: the off-app render follows the recipient's locale even though the
// push strings are materialized in English (see the TODO(#2897) in
// SystemNotification).
func TestSystemNotificationLocalizesOffApp(t *testing.T) {
	ctx := context.Background()
	payload := &models.CommunityEventPayload{
		EventType:      "EXPERIENCE_REMINDER_TWO_HOUR",
		ExperienceName: "Backyard BBQ",
	}

	es, err := l10n.NewLocalizer(language.Spanish)
	if err != nil {
		t.Fatalf("l10n.NewLocalizer(es): %v", err)
	}
	got := FromCommunityEvent(ctx, payload).RenderSMS(ctx, es, "https://example.com/go/abc", false)
	want := "Ripls: Backyard BBQ empieza en 2 horas\n\nVe los detalles en https://example.com/go/abc"
	if got != want {
		t.Errorf("Spanish SMS =\n%q\nwant\n%q", got, want)
	}
}

// TestSystemNotificationMissingNameUsesStandIn asserts the producers' old
// empty-name fallbacks ("your event", "the item") survived the move into the
// catalog as stand-ins, so a failed entity lookup still renders a sentence.
func TestSystemNotificationMissingNameUsesStandIn(t *testing.T) {
	loc := enLoc(t)
	ctx := context.Background()

	notif := SystemNotification(ctx, &models.CommunityEventPayload{
		EventType: "EXPERIENCE_REMINDER_DAY_BEFORE",
	})
	if notif.Body != "an event is tomorrow" {
		t.Errorf("body with no name = %q, want the stand-in sentence", notif.Body)
	}

	sms := FromCommunityEvent(ctx, &models.CommunityEventPayload{
		EventType: "LOAN_RETURN_REMINDER_UPCOMING",
	}).RenderSMS(ctx, loc, "", false)
	if sms != "Ripls: Return an item to Someone tomorrow" {
		t.Errorf("SMS with no names = %q", sms)
	}
}
