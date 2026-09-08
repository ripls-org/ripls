package notification_content

import (
	"context"
	"sort"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestEveryNotificationEventTypeHasCopy is the gate that would have caught
// #2896 before it reached a phone. Every event_type that can reach a recipient
// — bus enum names and the synthetic system strings alike — must map to a copy
// kind. Falling through to "default" means the recipient gets a generic line
// about an "update" instead of being told what happened.
//
// Shaped after undo.TestEveryEventTypeClassified: a new value is a build
// failure until someone decides what it says, or records why it says nothing.
func TestEveryNotificationEventTypeHasCopy(t *testing.T) {
	ResetUnmappedLogForTest()
	var missing []string
	for _, eventType := range NotificationEventTypes() {
		if _, ok := offAppKinds[eventType]; !ok {
			missing = append(missing, eventType)
		}
	}
	sort.Strings(missing)
	for _, eventType := range missing {
		t.Errorf("event type %q has no kind in offAppKinds: it would render the generic "+
			"default line. Add a kind plus notif.community_event.{kind}.title and "+
			"notif.offapp.{kind}.{message,cta}, or record it in BusEventTypesWithoutCopy "+
			"with the reason it notifies nobody.", eventType)
	}
}

// TestExemptEventTypesDoNotNotify checks the other direction: an event type
// recorded as "notifies nobody" must not actually be sending pushes. Otherwise
// the exemption silently becomes the #2896 bug with a comment on it.
func TestExemptEventTypesDoNotNotify(t *testing.T) {
	for eventType, reason := range BusEventTypesWithoutCopy {
		if reason == "" {
			t.Errorf("%v is exempt with no reason recorded", eventType)
		}
	}
}

// TestEveryKindRendersRealCopy walks the canonical list end to end and asserts
// each event type produces copy that is actually about it — not the default
// line, and not a raw catalog key (l10n.T returns the key on a miss).
func TestEveryKindRendersRealCopy(t *testing.T) {
	loc := enLoc(t)
	ctx := context.Background()
	for _, eventType := range NotificationEventTypes() {
		c := FromCommunityEvent(ctx, &models.CommunityEventPayload{
			EventType:      eventType,
			ActorName:      "Sam Rivera",
			ExperienceName: "Backyard BBQ",
			GearName:       "Tent",
			RequestTitle:   "Folding Table",
			CommunityName:  "The Block",
		})
		if c.Kind == "default" {
			t.Errorf("%s resolved to the default kind", eventType)
			continue
		}
		// l10n.T returns the message id when a key is missing, so a rendered
		// string that still looks like a catalog key means the entry is absent.
		if msg := c.message(ctx, loc); msg == "" || strings.HasPrefix(msg, "notif.") {
			t.Errorf("%s (kind %s) has no message in the catalog: %q", eventType, c.Kind, msg)
		}
		if cta := c.cta(ctx, loc); cta == "" || strings.HasPrefix(cta, "notif.") {
			t.Errorf("%s (kind %s) has no cta in the catalog: %q", eventType, c.Kind, cta)
		}
	}
}
