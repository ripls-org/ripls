package momentum

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

// TestDetectHighestPriority_EmptyChainReturnsNil — the orchestrator handles
// the empty-detector-list edge case without panicking.
func TestDetectHighestPriority_EmptyChainReturnsNil(t *testing.T) {
	store := setupTestStorage(t)
	got, err := DetectHighestPriority(context.Background(), store, "u1", "c1", nil)
	if err != nil {
		t.Fatalf("DetectHighestPriority: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil detection, got %v", got)
	}
}

// TestDetectHighestPriority_ReturnsFirstMatch — when multiple detectors
// would match, the orchestrator returns the first non-nil hit.
func TestDetectHighestPriority_ReturnsFirstMatch(t *testing.T) {
	store := setupTestStorage(t)
	first := fakeDetector{slot: SlotActiveQuest, det: &Detection{Slot: SlotActiveQuest, CtaLabel: "Schedule round 6 — Saturday at 9?"}}
	second := fakeDetector{slot: SlotSeasonalTrigger, det: &Detection{Slot: SlotSeasonalTrigger, CtaLabel: "Bring it back"}}

	got, err := DetectHighestPriority(context.Background(), store, "u1", "c1",
		[]Detector{first, second})
	if err != nil {
		t.Fatalf("DetectHighestPriority: %v", err)
	}
	if got == nil || got.Slot != SlotActiveQuest {
		t.Errorf("expected ActiveQuest detection first, got %v", got)
	}
}

// TestDetectHighestPriority_SkipsErroringDetectors — a per-detector error
// does not block lower-priority slots.
func TestDetectHighestPriority_SkipsErroringDetectors(t *testing.T) {
	store := setupTestStorage(t)
	failing := fakeDetector{slot: SlotActiveQuest, err: errBoom}
	working := fakeDetector{slot: SlotSeasonalTrigger, det: &Detection{Slot: SlotSeasonalTrigger}}

	got, err := DetectHighestPriority(context.Background(), store, "u1", "c1",
		[]Detector{failing, working})
	if err != nil {
		t.Fatalf("DetectHighestPriority: %v", err)
	}
	if got == nil || got.Slot != SlotSeasonalTrigger {
		t.Errorf("expected SeasonalTrigger detection after failing detector, got %v", got)
	}
}

// TestCascade_SharedGearAloneProducesNothing — owning gear that is shared
// into a community is not, on its own, a signal. The detector that used to
// treat it as one asserted things it never measured and dispatched a gear id
// into an experience lookup, so every tap dead-ended (#2892). Nothing in the
// cascade may fire on a bare listing again.
func TestCascade_SharedGearAloneProducesNothing(t *testing.T) {
	store := setupTestStorage(t)
	ctx := context.Background()

	gearID, err := store.Insert(ctx, &models.Gear{
		Name: "12-Piece Metric Ratcheting Wrench Set", OwnerId: "u1",
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{
		CommunityId: "c1", GearId: gearID,
	}); err != nil {
		t.Fatalf("insert community_gear: %v", err)
	}

	got, err := DetectHighestPriority(ctx, store, "u1", "c1", DefaultDetectors())
	if err != nil {
		t.Fatalf("DetectHighestPriority: %v", err)
	}
	if got != nil {
		t.Errorf("expected no detection from a bare gear listing, got %+v", got)
	}
}

// TestDefaultDetectors_HasExpectedOrder — the chain is in cascade priority order.
func TestDefaultDetectors_HasExpectedOrder(t *testing.T) {
	got := DefaultDetectors()
	// SlotIdleOffer (priority 5) is deliberately unfilled — see its
	// declaration in detector.go.
	want := []Slot{
		SlotActiveQuest,
		SlotESMRepeatSignal,
		SlotCalendarGap,
		SlotSeasonalTrigger,
	}
	if len(got) != len(want) {
		t.Fatalf("DefaultDetectors length: want %d, got %d", len(want), len(got))
	}
	for i, d := range got {
		if d.Slot() != want[i] {
			t.Errorf("[%d]: want %v, got %v", i, want[i], d.Slot())
		}
	}
}

// fakeDetector lets tests synthesize specific (slot, detection, error) triples.
type fakeDetector struct {
	slot Slot
	det  *Detection
	err  error
}

func (f fakeDetector) Slot() Slot { return f.slot }
func (f fakeDetector) Detect(_ context.Context, _ *storage.ProtoSQLStorage, _, _ string) (*Detection, error) {
	return f.det, f.err
}

var errBoom = boomError{}

type boomError struct{}

func (boomError) Error() string { return "boom" }
