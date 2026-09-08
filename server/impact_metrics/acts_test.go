package impact_metrics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func insertCommunityEvent(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID string,
	eventType models.CommunityEventType,
	occurredAt int64,
) {
	t.Helper()
	ev := &models.CommunityEvent{
		Id:                uuid.New().String(),
		CommunityId:       communityID,
		EventType:         eventType,
		ActorId:           uuid.New().String(),
		OccurredAtUnixSec: occurredAt,
	}
	if _, err := db.Insert(context.Background(), ev); err != nil {
		t.Fatalf("insert community_event: %v", err)
	}
}

func TestCountActs_EmptyCommunity(t *testing.T) {
	db := setupTestDatabase(t)
	calc := NewCalculator(db, nil)

	got, err := calc.CountActs(context.Background(), uuid.New().String())
	if err != nil {
		t.Fatalf("CountActs error: %v", err)
	}
	if got.Total != 0 {
		t.Errorf("Total = %d, want 0", got.Total)
	}
	for _, cat := range ActsCategoryOrder {
		if got.ByCategory[cat] != 0 {
			t.Errorf("ByCategory[%s] = %d, want 0", cat, got.ByCategory[cat])
		}
	}
}

func TestCountActs_CountsEachActTypeOnce(t *testing.T) {
	db := setupTestDatabase(t)
	calc := NewCalculator(db, nil)
	communityID := uuid.New().String()
	now := time.Now().Unix()

	cases := []struct {
		eventType models.CommunityEventType
		category  ActsCategory
	}{
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, ActsCategoryItemsShared},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, ActsCategoryEventsCreated},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES, ActsCategoryRSVPs},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, ActsCategoryHelpRequested},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE, ActsCategoryHelpOffered},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED, ActsCategoryPitchedIn},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED, ActsCategoryPitchedIn},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED, ActsCategoryLoansGiveaways},
	}
	for _, c := range cases {
		insertCommunityEvent(t, db, communityID, c.eventType, now)
	}

	got, err := calc.CountActs(context.Background(), communityID)
	if err != nil {
		t.Fatalf("CountActs error: %v", err)
	}
	if got.Total != int32(len(cases)) {
		t.Errorf("Total = %d, want %d", got.Total, len(cases))
	}
	if got.ByCategory[ActsCategoryPitchedIn] != 2 {
		t.Errorf("Pitched-in count = %d, want 2", got.ByCategory[ActsCategoryPitchedIn])
	}
	for _, cat := range []ActsCategory{
		ActsCategoryItemsShared, ActsCategoryEventsCreated, ActsCategoryRSVPs,
		ActsCategoryHelpRequested, ActsCategoryHelpOffered, ActsCategoryLoansGiveaways,
	} {
		if got.ByCategory[cat] != 1 {
			t.Errorf("ByCategory[%s] = %d, want 1", cat, got.ByCategory[cat])
		}
	}
}

func TestCountActs_ExcludesNonActEventTypes(t *testing.T) {
	db := setupTestDatabase(t)
	calc := NewCalculator(db, nil)
	communityID := uuid.New().String()
	now := time.Now().Unix()

	// One act + several non-act events.
	insertCommunityEvent(t, db, communityID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, now)
	for _, et := range []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_UNSHARED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
	} {
		insertCommunityEvent(t, db, communityID, et, now)
	}

	got, err := calc.CountActs(context.Background(), communityID)
	if err != nil {
		t.Fatalf("CountActs error: %v", err)
	}
	if got.Total != 1 {
		t.Errorf("Total = %d, want 1", got.Total)
	}
}

func TestCountActs_IsolatesPerCommunity(t *testing.T) {
	db := setupTestDatabase(t)
	calc := NewCalculator(db, nil)
	communityA := uuid.New().String()
	communityB := uuid.New().String()
	now := time.Now().Unix()

	insertCommunityEvent(t, db, communityA,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, now)
	insertCommunityEvent(t, db, communityA,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, now)
	insertCommunityEvent(t, db, communityB,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED, now)

	gotA, err := calc.CountActs(context.Background(), communityA)
	if err != nil {
		t.Fatalf("CountActs(A) error: %v", err)
	}
	if gotA.Total != 2 {
		t.Errorf("A.Total = %d, want 2", gotA.Total)
	}

	gotB, err := calc.CountActs(context.Background(), communityB)
	if err != nil {
		t.Fatalf("CountActs(B) error: %v", err)
	}
	if gotB.Total != 1 {
		t.Errorf("B.Total = %d, want 1", gotB.Total)
	}
}

func TestCountActsByMonth_GapsFilledWithZeros(t *testing.T) {
	db := setupTestDatabase(t)
	calc := NewCalculator(db, nil)
	communityID := uuid.New().String()

	// Two acts in Jan 2025 and one in Apr 2025; Feb and Mar must
	// be emitted as zero-count rows so the chart axis is continuous.
	jan := time.Date(2025, time.January, 15, 0, 0, 0, 0, time.UTC).Unix()
	apr := time.Date(2025, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	insertCommunityEvent(t, db, communityID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, jan)
	insertCommunityEvent(t, db, communityID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, jan)
	insertCommunityEvent(t, db, communityID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED, apr)

	got, err := calc.CountActsByMonth(context.Background(), communityID)
	if err != nil {
		t.Fatalf("CountActsByMonth error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4 (Jan, Feb, Mar, Apr)", len(got))
	}
	if got[0].Month != time.January || got[0].Count != 2 {
		t.Errorf("Jan: month=%v count=%d, want January count=2", got[0].Month, got[0].Count)
	}
	if got[1].Count != 0 || got[2].Count != 0 {
		t.Errorf("Feb/Mar: counts %d, %d, want 0, 0", got[1].Count, got[2].Count)
	}
	if got[3].Month != time.April || got[3].Count != 1 {
		t.Errorf("Apr: month=%v count=%d, want April count=1", got[3].Month, got[3].Count)
	}
}

func TestCountActsByMonth_EmptyCommunity(t *testing.T) {
	db := setupTestDatabase(t)
	calc := NewCalculator(db, nil)

	got, err := calc.CountActsByMonth(context.Background(), uuid.New().String())
	if err != nil {
		t.Fatalf("CountActsByMonth error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
