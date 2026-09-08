package community_subscriber

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestShouldNotifyMatchesAudienceLabel guards against drift between the two
// delivery-metadata functions: a type fires a notification (ShouldNotify) iff it
// has a recipient-audience label. The notification_examples review page derives
// its "delivered vs omitted" partition from these, so they must agree.
func TestShouldNotifyMatchesAudienceLabel(t *testing.T) {
	for num, name := range models.CommunityEventType_name {
		et := models.CommunityEventType(num)
		notified := ShouldNotify(et)
		labeled := RecipientAudienceLabel(et) != ""
		if notified != labeled {
			t.Errorf("%s: ShouldNotify=%v but hasAudienceLabel=%v — keep ShouldNotify and RecipientAudienceLabel in sync", name, notified, labeled)
		}
	}
}
