package planning

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// SoftDeleteContributionsForNeed soft-deletes every active
// PlanningContribution that points at needID via from_need_id. Used by
// RemoveRequestNeed / RemoveExperienceNeed to cascade removal so the
// breakdown disappears as a unit instead of leaving orphaned "this
// helper said they were bringing X" rows after the need is gone.
//
// Returns the number of contributions soft-deleted. A contribution
// that is already soft-deleted is skipped. Errors short-circuit so
// callers can decide whether to surface as Internal or warn-and-
// continue.
func SoftDeleteContributionsForNeed(
	ctx context.Context,
	st *storage.ProtoSQLStorage,
	needID string,
) (int, error) {
	if needID == "" {
		return 0, fmt.Errorf("needID is required")
	}

	contribs, err := storage.QueryByFields[*models.PlanningContribution](
		st, ctx, map[string]any{"from_need_id": needID},
	)
	if err != nil {
		return 0, connecterr.Internal(ctx, "SoftDeleteContributionsForNeed", err,
			"need_id", needID)
	}

	now := clock.UnixSec(ctx)
	count := 0
	for _, c := range contribs {
		if c.Deleted != nil {
			continue
		}
		c.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: now}
		if err := st.Update(ctx, c); err != nil {
			return count, connecterr.Internal(ctx, "SoftDeleteContributionsForNeed", err,
				"need_id", needID, "contribution_id", c.Id)
		}
		count++
	}
	return count, nil
}
