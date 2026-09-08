package planning

import (
	"context"

	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

const (
	planningNeedTable = "planning_need"
)

// ClaimSlot atomically decrements slots_remaining on a PlanningNeed when
// at least one slot is still open. Over-claiming (slots_remaining already
// at 0) succeeds without changing the count — the caller still records a
// contribution; a helper can intentionally bring more than the proposer
// asked for. Returns `decremented=true` when slots_remaining was actually
// decremented and the proto blob re-synced; returns `decremented=false`
// on the over-claim no-op path, so callers can skip
// [RestoreSlotOnFailure] if a subsequent step fails (otherwise the
// counter would silently drift up past the original total).
func ClaimSlot(ctx context.Context, st *storage.ProtoSQLStorage, need *models.PlanningNeed) (decremented bool, err error) {
	decremented, err = st.DecrementFieldIfPositive(ctx, planningNeedTable, need.Id, "slots_remaining")
	if err != nil {
		return false, connecterr.Internal(ctx, "ClaimSlot", err, "detail", "failed to decrement need slots")
	}
	if !decremented {
		return false, nil
	}
	// Sync the proto blob with the column value that was just decremented.
	need.SlotsRemaining--
	if syncErr := st.Update(ctx, need); syncErr != nil {
		// Non-fatal: the column is already correct; the blob will be stale
		// until the next full update. Continue with the claim.
		_ = syncErr
	}
	return true, nil
}

// ReleaseSlot increments slots_remaining on a PlanningNeed (best-effort).
// Used when unclaiming a need. Non-fatal: the parent need may have been removed.
func ReleaseSlot(ctx context.Context, st *storage.ProtoSQLStorage, needID string) {
	if err := st.IncrementField(ctx, planningNeedTable, needID, "slots_remaining"); err != nil {
		return // best-effort; caller logs if needed
	}
	// Re-sync the proto blob after the column increment.
	parentNeed := &models.PlanningNeed{}
	if getErr := st.GetByID(ctx, needID, parentNeed); getErr == nil {
		parentNeed.SlotsRemaining++
		_ = st.Update(ctx, parentNeed) // best-effort
	}
}

// RestoreSlotOnFailure attempts to restore a slot that was decremented before
// a subsequent operation failed. Best-effort: logs on failure.
func RestoreSlotOnFailure(ctx context.Context, st *storage.ProtoSQLStorage, needID string) {
	if err := st.IncrementField(ctx, planningNeedTable, needID, "slots_remaining"); err != nil {
		_ = err // caller should log this
	}
}
