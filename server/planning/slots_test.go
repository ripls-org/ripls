package planning

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestClaimSlot_DecrementsAndSyncsBlob(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	need := &models.PlanningNeed{
		ProposerId:     "proposer-1",
		Name:           "Bring chairs",
		SlotsRemaining: 3,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: "exp-slots"},
	}
	needID, err := st.Insert(ctx, need)
	if err != nil {
		t.Fatalf("insert need: %v", err)
	}
	need.Id = needID

	decremented, err := ClaimSlot(ctx, st, need)
	if err != nil {
		t.Fatalf("ClaimSlot: %v", err)
	}
	if !decremented {
		t.Errorf("decremented = false, want true on the happy path")
	}

	if need.SlotsRemaining != 2 {
		t.Errorf("need.SlotsRemaining after claim = %d, want 2", need.SlotsRemaining)
	}

	// Verify the blob was updated in storage.
	stored := &models.PlanningNeed{}
	if err := st.GetByID(ctx, needID, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.SlotsRemaining != 2 {
		t.Errorf("stored SlotsRemaining = %d, want 2", stored.SlotsRemaining)
	}
}

func TestClaimSlot_NoSlots_OverClaimNoOp(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	need := &models.PlanningNeed{
		ProposerId:     "proposer-1",
		Name:           "Full need",
		SlotsRemaining: 0,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: "exp-slots-full"},
	}
	needID, err := st.Insert(ctx, need)
	if err != nil {
		t.Fatalf("insert need: %v", err)
	}
	need.Id = needID

	// Over-claim succeeds without changing the counter — a helper can
	// intentionally bring more than the proposer asked for; the
	// contribution is recorded by the caller regardless.
	decremented, err := ClaimSlot(ctx, st, need)
	if err != nil {
		t.Fatalf("ClaimSlot returned error on over-claim: %v", err)
	}
	if decremented {
		t.Error("decremented = true, want false when starting from 0")
	}
	if need.SlotsRemaining != 0 {
		t.Errorf("need.SlotsRemaining = %d, want 0 after over-claim no-op", need.SlotsRemaining)
	}
	stored := &models.PlanningNeed{}
	if err := st.GetByID(ctx, needID, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.SlotsRemaining != 0 {
		t.Errorf("stored SlotsRemaining = %d, want 0", stored.SlotsRemaining)
	}
}

func TestReleaseSlot_IncrementsAndSyncsBlob(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	need := &models.PlanningNeed{
		ProposerId:     "proposer-1",
		Name:           "Bring tables",
		SlotsRemaining: 1,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: "exp-release"},
	}
	needID, err := st.Insert(ctx, need)
	if err != nil {
		t.Fatalf("insert need: %v", err)
	}

	ReleaseSlot(ctx, st, needID)

	stored := &models.PlanningNeed{}
	if err := st.GetByID(ctx, needID, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.SlotsRemaining != 2 {
		t.Errorf("SlotsRemaining after release = %d, want 2", stored.SlotsRemaining)
	}
}

func TestReleaseSlot_MissingNeed_NoOp(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	// Should not panic or error even if the need doesn't exist.
	ReleaseSlot(ctx, st, "nonexistent-need-id")
}

func TestRestoreSlotOnFailure_Increments(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	// Start with zero slots so DecrementFieldIfPositive would fail without restore.
	need := &models.PlanningNeed{
		ProposerId:     "proposer-1",
		Name:           "Bring blankets",
		SlotsRemaining: 0,
		Scope:          &models.PlanningNeed_ExperienceId{ExperienceId: "exp-restore"},
	}
	needID, err := st.Insert(ctx, need)
	if err != nil {
		t.Fatalf("insert need: %v", err)
	}

	// RestoreSlotOnFailure increments the column from 0 to 1.
	RestoreSlotOnFailure(ctx, st, needID)

	// Verify the column was incremented by attempting an atomic decrement —
	// DecrementFieldIfPositive returns true only when the column is > 0.
	decremented, err := st.DecrementFieldIfPositive(ctx, planningNeedTable, needID, "slots_remaining")
	if err != nil {
		t.Fatalf("DecrementFieldIfPositive: %v", err)
	}
	if !decremented {
		t.Error("RestoreSlotOnFailure did not increment the slots_remaining column")
	}
}

func TestRestoreSlotOnFailure_MissingNeed_NoOp(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	// Should not panic or error even if the need doesn't exist.
	RestoreSlotOnFailure(ctx, st, "nonexistent-need-id")
}
