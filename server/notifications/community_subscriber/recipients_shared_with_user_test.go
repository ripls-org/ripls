package community_subscriber

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// A direct share is addressed to one person. Resolving it against the member
// set instead would re-notify everyone already holding the item every time the
// audience widened, and would miss the recipient entirely on the path that
// prompted #3106, where the member row is written after the item's own
// GEAR_SHARED has already been published and snapshotted.
func TestResolveRecipients_ItemSharedWithUser(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		event *models.CommunityEvent
		want  []string
	}{
		{
			name: "the person it was shared with",
			event: &models.CommunityEvent{
				EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER,
				ActorId:      "sharer",
				ObjectUserId: "recipient",
			},
			want: []string{"recipient"},
		},
		{
			name: "sharing with yourself notifies nobody",
			event: &models.CommunityEvent{
				EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER,
				ActorId:      "sharer",
				ObjectUserId: "sharer",
			},
			want: nil,
		},
		{
			name: "no recipient named",
			event: &models.CommunityEvent{
				EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER,
				ActorId:   "sharer",
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveRecipients(ctx, nil, &cebus.PublishedEvent{Event: tt.event})
			if len(got) != len(tt.want) {
				t.Fatalf("recipients = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("recipients[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestShouldNotify_ItemSharedWithUser(t *testing.T) {
	if !ShouldNotify(models.CommunityEventType_COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER) {
		t.Error("a direct share must reach the person it was shared with")
	}
}
