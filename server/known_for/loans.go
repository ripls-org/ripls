package known_for

import (
	"context"
	"fmt"
	"strings"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// addLoanCounts accumulates gear-lending contributions into [counts].
//
// Weighting per category:
//   - +1 for every distinct gear listed in scope (via community_gear)
//   - +1 for every completed loan of that gear in scope, even when
//     the gear is no longer listed via community_gear (the completed
//     loan is itself evidence of community-scoped lending activity)
//
// Query budget: 3 reads (gear by owner / community_gear by gear-id /
// transfer by owner for per-user; community_gear by community /
// transfer by community / gear batch over the union for per-
// community).
func addLoanCounts(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
	counts categoryCounts,
) error {
	inputs, err := loadLoanInputs(ctx, s, opts, sharedSet)
	if err != nil {
		return err
	}
	if len(inputs.gearByID) == 0 {
		return nil
	}
	for _, id := range inputs.sharedGearIDs {
		if g := inputs.gearByID[id]; g != nil {
			counts.add(gearCategoryString(g))
		}
	}
	for _, t := range inputs.loanTransfers {
		if g := inputs.gearByID[t.GearId]; g != nil {
			counts.add(gearCategoryString(g))
		}
	}
	return nil
}

// loanInputs is the fully-resolved set of rows that
// [deriveLoanCandidates] needs to tally up: a gear-id-to-entity
// lookup, the ids that are currently shared in scope, and the
// qualifying completed-loan transfers.
type loanInputs struct {
	gearByID      map[string]*models.Gear
	sharedGearIDs []string
	loanTransfers []*models.Transfer
}

// loadLoanInputs runs the mode-specific storage queries and returns
// a normalized [loanInputs]. Both modes spend 3 reads on the happy
// path.
func loadLoanInputs(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
) (loanInputs, error) {
	if opts.Mode == ModePerUser {
		return loadLoanInputsPerUser(ctx, s, opts, sharedSet)
	}
	return loadLoanInputsPerCommunity(ctx, s, opts, sharedSet)
}

// loadLoanInputsPerUser pulls the target's gear universe, intersects
// it with community_gear to find the shared subset, and pulls every
// transfer the target owns to find the qualifying loans.
func loadLoanInputsPerUser(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
) (loanInputs, error) {
	owned, err := storage.QueryByField[*models.Gear](
		s, ctx, "owner_id", opts.OwnerID,
	)
	if err != nil {
		return loanInputs{}, fmt.Errorf("query gear by owner: %w", err)
	}
	gearByID := make(map[string]*models.Gear, len(owned))
	ownedIDs := make([]string, 0, len(owned))
	for _, g := range owned {
		if g == nil || g.Deleted != nil {
			continue
		}
		gearByID[g.Id] = g
		ownedIDs = append(ownedIDs, g.Id)
	}

	var sharedGearIDs []string
	if len(ownedIDs) > 0 {
		pivots, err := storage.QueryByFieldIn[*models.CommunityGear](
			s, ctx, "gear_id", ownedIDs,
		)
		if err != nil {
			return loanInputs{}, fmt.Errorf("query community_gear: %w", err)
		}
		seen := make(map[string]struct{})
		for _, cg := range pivots {
			if cg.Deleted != nil {
				continue
			}
			if _, ok := sharedSet[cg.CommunityId]; !ok {
				continue
			}
			if _, ok := gearByID[cg.GearId]; !ok {
				continue
			}
			if _, dup := seen[cg.GearId]; dup {
				continue
			}
			seen[cg.GearId] = struct{}{}
			sharedGearIDs = append(sharedGearIDs, cg.GearId)
		}
	}

	transfers, err := storage.QueryByField[*models.Transfer](
		s, ctx, "owner_id", opts.OwnerID,
	)
	if err != nil {
		return loanInputs{}, fmt.Errorf("query transfers by owner: %w", err)
	}
	qualifying := make([]*models.Transfer, 0, len(transfers))
	for _, t := range transfers {
		if !loanQualifies(t, sharedSet) {
			continue
		}
		qualifying = append(qualifying, t)
	}
	return loanInputs{
		gearByID:      gearByID,
		sharedGearIDs: sharedGearIDs,
		loanTransfers: qualifying,
	}, nil
}

// loadLoanInputsPerCommunity pulls every community_gear pivot in
// scope, every completed-loan transfer in scope, and a single
// batched gear fetch over the union of referenced ids — so a gear
// that has been loaned in scope but is no longer listed via
// community_gear still contributes its loans.
func loadLoanInputsPerCommunity(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
) (loanInputs, error) {
	pivots, err := storage.QueryByFieldIn[*models.CommunityGear](
		s, ctx, "community_id", opts.CommunityIDs,
	)
	if err != nil {
		return loanInputs{}, fmt.Errorf("query community_gear by community: %w", err)
	}
	sharedIDSet := make(map[string]struct{}, len(pivots))
	for _, cg := range pivots {
		if cg.Deleted != nil {
			continue
		}
		sharedIDSet[cg.GearId] = struct{}{}
	}

	transfers, err := storage.QueryByFieldIn[*models.Transfer](
		s, ctx, "community_id", opts.CommunityIDs,
	)
	if err != nil {
		return loanInputs{}, fmt.Errorf("query transfers by community: %w", err)
	}
	qualifying := make([]*models.Transfer, 0, len(transfers))
	transferGearIDs := make(map[string]struct{})
	for _, t := range transfers {
		if !loanQualifies(t, sharedSet) {
			continue
		}
		qualifying = append(qualifying, t)
		transferGearIDs[t.GearId] = struct{}{}
	}

	allIDSet := make(map[string]struct{}, len(sharedIDSet)+len(transferGearIDs))
	for id := range sharedIDSet {
		allIDSet[id] = struct{}{}
	}
	for id := range transferGearIDs {
		allIDSet[id] = struct{}{}
	}
	if len(allIDSet) == 0 {
		return loanInputs{gearByID: map[string]*models.Gear{}}, nil
	}
	allIDs := make([]string, 0, len(allIDSet))
	for id := range allIDSet {
		allIDs = append(allIDs, id)
	}
	gearByID, err := storage.GetByIDs[*models.Gear](s, ctx, allIDs)
	if err != nil {
		return loanInputs{}, fmt.Errorf("get gear by ids: %w", err)
	}
	for id, g := range gearByID {
		if g == nil || g.Deleted != nil {
			delete(gearByID, id)
		}
	}
	sharedGearIDs := make([]string, 0, len(sharedIDSet))
	for id := range sharedIDSet {
		if _, ok := gearByID[id]; ok {
			sharedGearIDs = append(sharedGearIDs, id)
		}
	}
	return loanInputs{
		gearByID:      gearByID,
		sharedGearIDs: sharedGearIDs,
		loanTransfers: qualifying,
	}, nil
}

// loanQualifies returns true when a transfer is a non-deleted
// completed loan inside the requested community scope.
func loanQualifies(t *models.Transfer, sharedSet map[string]struct{}) bool {
	if t.Deleted != nil {
		return false
	}
	if t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
		return false
	}
	if t.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
		return false
	}
	if _, ok := sharedSet[t.CommunityId]; !ok {
		return false
	}
	if t.GearId == "" {
		return false
	}
	return true
}

// gearCategoryString pulls the inner string out of Gear.Category,
// which is a TrackedString wrapper. Returns the empty string when the
// wrapper is absent or carries an empty value.
func gearCategoryString(g *models.Gear) string {
	if g == nil || g.Category == nil {
		return ""
	}
	return strings.TrimSpace(g.Category.Value)
}
