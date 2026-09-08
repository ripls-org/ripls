package planning

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// ValidateTransferLink confirms a contribution can carry a transfer link
// (#2702 request scope, #2708 experience scope): it exists, is not deleted, is
// scoped to the given planning scope, is owned by userID, and — when it links
// gear — links the same gear the transfer moves. Returns the contribution so
// the caller can set its transfer link, or CodeInvalidArgument on any mismatch.
func ValidateTransferLink(ctx context.Context, st *storage.ProtoSQLStorage, userID, contributionID string, scope Scope, gearID string) (*models.PlanningContribution, error) {
	contribution := &models.PlanningContribution{}
	if err := st.GetByID(ctx, contributionID, contribution); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("contribution not found"))
	}
	if contribution.Deleted != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("contribution has been removed"))
	}
	if !scope.matchesContribution(contribution) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("contribution is not scoped to this %s", scope.scopeKind()))
	}
	if contribution.ContributorId != userID {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("contribution is not owned by the caller"))
	}
	if contribution.GetGearId() != "" && contribution.GetGearId() != gearID {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("contribution links different gear"))
	}
	return contribution, nil
}

// UnlinkTransfer clears the transfer link on the contribution that carries
// transferID within the given scope, if any. Used when an origin-linked
// transfer is cancelled (#2702/#2708): the claim survives, the escalation
// doesn't. Missing or already-unlinked contributions are a no-op, so the call
// is safe on replayed events.
func UnlinkTransfer(ctx context.Context, st *storage.ProtoSQLStorage, scope Scope, transferID string) error {
	rows, err := storage.QueryByField[*models.PlanningContribution](st, ctx, scope.scopeFieldName(), scope.ScopeID())
	if err != nil {
		return fmt.Errorf("query contributions for transfer unlink: %w", err)
	}
	for _, c := range rows {
		if c.GetTransferId() != transferID {
			continue
		}
		c.TransferId = nil
		if err := st.Update(ctx, c); err != nil {
			return fmt.Errorf("unlink transfer from contribution %s: %w", c.Id, err)
		}
	}
	return nil
}
