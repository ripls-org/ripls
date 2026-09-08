package activity_digest

import (
	"testing"

	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestEveryDigestEventTypeClassified is the load-bearing enforcement for
// the digest classification registry (#2665). It fails when any
// CommunityEventType enum value lacks an entry in Registry — which is
// what we want when a contributor adds a new state-changing event
// without deciding how the ops digest should report it.
//
// The fix when this test fails is to add an entry to
// server/activity_digest/registry.go: either a counted entry with a
// Category and an ActivityTotals counter, or an explicit exclusion.
// The undo-side sibling gate is server/undo/registry_test.go.
func TestEveryDigestEventTypeClassified(t *testing.T) {
	for value, name := range models.CommunityEventType_name {
		eventType := models.CommunityEventType(value)

		// UNSPECIFIED is the proto enum zero value and not a real event.
		if eventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED {
			continue
		}

		entry, ok := Registry[eventType]
		if !ok {
			t.Errorf("CommunityEventType %s has no digest classification — add an entry in server/activity_digest/registry.go", name)
			continue
		}
		switch entry.Disposition {
		case DispositionCounted:
			if entry.CategoryOf == nil {
				t.Errorf("CommunityEventType %s is counted but has no CategoryOf", name)
			}
			if entry.Apply == nil {
				t.Errorf("CommunityEventType %s is counted but has no Apply", name)
			}
		case DispositionExcludedRetraction, DispositionExcludedInternal:
			// Explicit exclusions are complete on their own.
		default:
			t.Errorf("CommunityEventType %s has DispositionUnspecified — pick counted or an explicit exclusion", name)
		}
	}
}

// TestEveryCountedTypeIncrementsATotal asserts that every counted
// entry's Apply actually moves an ActivityTotals counter and that its
// CategoryOf resolves to a real category — for transfer-scoped types,
// under both loan and giveaway stamping. A counted type whose Apply is
// a no-op would pass the classification gate while still vanishing
// from the ACTIVITY section.
func TestEveryCountedTypeIncrementsATotal(t *testing.T) {
	transferTypes := []models.TransferType{
		models.TransferType_TRANSFER_TYPE_LOAN,
		models.TransferType_TRANSFER_TYPE_GIVEAWAY,
	}
	for eventType, entry := range Registry {
		if entry.Disposition != DispositionCounted {
			continue
		}
		for _, tt := range transferTypes {
			ev := &models.CommunityEvent{EventType: eventType, TransferType: tt}
			var totals email.ActivityTotals
			entry.Apply(ev, &totals)
			if totals == (email.ActivityTotals{}) {
				t.Errorf("%s (transfer_type=%s): Apply did not increment any ActivityTotals counter",
					eventType, tt)
			}
			if got := entry.CategoryOf(ev); got == CategoryUnspecified {
				t.Errorf("%s (transfer_type=%s): CategoryOf returned CategoryUnspecified", eventType, tt)
			}
		}
	}
}

// TestTransferCategorySplit pins the loan/giveaway split for the
// transfer-scoped types, including the default-to-loan behavior for
// rows without a transfer_type.
func TestTransferCategorySplit(t *testing.T) {
	entry := Registry[models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE]
	loan := &models.CommunityEvent{TransferType: models.TransferType_TRANSFER_TYPE_LOAN}
	giveaway := &models.CommunityEvent{TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY}
	unset := &models.CommunityEvent{}

	if got := entry.CategoryOf(loan); got != CategoryLoans {
		t.Errorf("loan CategoryOf = %v, want CategoryLoans", got)
	}
	if got := entry.CategoryOf(giveaway); got != CategoryGiveaways {
		t.Errorf("giveaway CategoryOf = %v, want CategoryGiveaways", got)
	}
	if got := entry.CategoryOf(unset); got != CategoryLoans {
		t.Errorf("unset transfer_type CategoryOf = %v, want CategoryLoans (default)", got)
	}
}

// TestCategoryLabels pins the pluralization used in per-community
// breakdown lines.
func TestCategoryLabels(t *testing.T) {
	cases := []struct {
		c    Category
		n    int
		want string
	}{
		{CategoryLoans, 1, "loan"},
		{CategoryLoans, 3, "loans"},
		{CategoryGiveaways, 1, "giveaway"},
		{CategoryRequests, 2, "requests"},
		{CategoryEvents, 1, "event"},
		{CategoryEvents, 4, "events"},
		{CategoryGear, 5, "gear actions"},
		{CategoryGear, 1, "gear action"},
		{CategoryPlanning, 2, "planning actions"},
		{CategoryMembership, 1, "membership change"},
		{CategoryCommunity, 3, "community changes"},
		{CategoryUnspecified, 1, "other"},
	}
	for _, tc := range cases {
		if got := tc.c.Label(tc.n); got != tc.want {
			t.Errorf("Category(%d).Label(%d) = %q, want %q", tc.c, tc.n, got, tc.want)
		}
	}
}
