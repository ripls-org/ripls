package community_subscriber

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestShouldNotify(t *testing.T) {
	tests := []struct {
		eventType       models.CommunityEventType
		shouldNotifyVal bool
		description     string
	}{
		// Targeted notifications - user is directly involved
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED, true, "owner should know someone wants their gear"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED, true, "recipient should know they were approved"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED, true, "other party should know transfer was cancelled"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED, true, "other party should know pickup time was proposed"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE, true, "requester should know someone offered help"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED, false, "request-fulfilled does not notify (product decision)"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES, true, "owner should know someone is attending"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE, true, "owner should know someone might attend"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, true, "community members should know about new experience"},

		// Broadcast events — pass ShouldNotify; per-recipient preference
		// gating happens later in notifyTargetedUsers.
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, true, "gear shared broadcast (gated by preference)"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, true, "request created broadcast (gated by preference)"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED, true, "experience completed notifies Yes/Maybe RSVPs (gated by preference)"},

		// Targeted events newly added to the allow-list.
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN, true, "owner should know interest was withdrawn"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE, true, "other party should know transfer is active"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED, true, "chosen helper should know their gear offer was picked (#2702)"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN, true, "requester should know an offer was withdrawn"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED, true, "offerers should know request was cancelled"},

		// New member joining is a broadcast event gated by the
		// NEW_MEMBERS preference category.
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED, true, "existing members should know someone joined"},

		// Still off the allow-list.
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO, false, "declined RSVP is low-value"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_UNSHARED, false, "internal housekeeping"},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT, false, "internal housekeeping"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType.String(), func(t *testing.T) {
			result := ShouldNotify(tt.eventType)
			if result != tt.shouldNotifyVal {
				t.Errorf("ShouldNotify(%v) = %v, expected %v (%s)", tt.eventType, result, tt.shouldNotifyVal, tt.description)
			}
		})
	}
}

func TestShouldNotify_LifecycleTypes(t *testing.T) {
	if !ShouldNotify(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED) {
		t.Error("EXPERIENCE_STARTED should pass ShouldNotify")
	}
	if !ShouldNotify(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED) {
		t.Error("EXPERIENCE_CANCELLED should pass ShouldNotify")
	}
	if !ShouldNotify(models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED) {
		t.Error("EXPERIENCE_UPDATED should pass ShouldNotify")
	}
}

func TestCategoryFor_LifecycleTypes(t *testing.T) {
	want := models.NotificationCategory_NOTIFICATION_CATEGORY_EXPERIENCE_RSVPS
	for _, et := range []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
	} {
		if got := community.CategoryFor(et); got != want {
			t.Errorf("CategoryFor(%v) = %v, want %v", et, got, want)
		}
	}
}

// TestIsActive covers the consolidated soft-delete gate that #1622 (push
// notification dispatch) and #1623 (async write guard) both call. Per the
// design in docs/community_delete_and_leave.md §6.5 / §12, no community-
// scoped push or async write should land for a soft-deleted community.
// Audit trail rows (CommunityEvent, ChatMessage) are preserved upstream
// of this helper; only the user-facing action is suppressed.
func TestIsActive(t *testing.T) {
	ctx := context.Background()
	s := setupTestStorage(t)

	t.Run("active community: returns true", func(t *testing.T) {
		communityID := uuid.New().String()
		creatorID := uuid.New().String()
		if _, err := s.Insert(ctx, &models.Community{
			Id:          communityID,
			Name:        "Active",
			CreatorId:   creatorID,
			OwnerUserId: creatorID,
		}); err != nil {
			t.Fatalf("seed community: %v", err)
		}
		if !community.IsActive(ctx, s, communityID) {
			t.Error("expected community.IsActive=true for active community")
		}
	})

	t.Run("soft-deleted community: returns false", func(t *testing.T) {
		communityID := uuid.New().String()
		creatorID := uuid.New().String()
		if _, err := s.Insert(ctx, &models.Community{
			Id:        communityID,
			Name:      "Deleted",
			CreatorId: creatorID,
			Deleted: &models.DeletedMetadata{
				DeletedByUserId:  creatorID,
				DeletedAtUnixSec: 1700000000,
			},
		}); err != nil {
			t.Fatalf("seed deleted community: %v", err)
		}
		if community.IsActive(ctx, s, communityID) {
			t.Error("expected community.IsActive=false for soft-deleted community")
		}
	})

	t.Run("missing community: fail open (returns true)", func(t *testing.T) {
		// A transient lookup failure must not silently drop user-facing
		// work for the active majority. Soft-deletes are rare; the
		// day-30 purge job (#1620) sweeps any residual ghost rows from
		// the rare race where a write/dispatch slips through.
		if !community.IsActive(ctx, s, uuid.New().String()) {
			t.Error("expected community.IsActive=true on lookup failure (fail open)")
		}
	})

	t.Run("DeletedAtUnixSec=0 is treated as not deleted", func(t *testing.T) {
		// Defensive: a zero-valued DeletedMetadata sub-message must not
		// trip the gate. The flat soft-delete column convention treats
		// 0 / NULL as "not deleted" (see protosql_search.go:334).
		communityID := uuid.New().String()
		creatorID := uuid.New().String()
		if _, err := s.Insert(ctx, &models.Community{
			Id:          communityID,
			Name:        "Zero-deleted-at",
			CreatorId:   creatorID,
			OwnerUserId: creatorID,
			Deleted:     &models.DeletedMetadata{}, // present but zeroed
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if !community.IsActive(ctx, s, communityID) {
			t.Error("expected community.IsActive=true for community with zero DeletedAtUnixSec")
		}
	})
}
