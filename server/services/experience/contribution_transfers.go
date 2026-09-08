package experience

import (
	"context"

	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/planning"
	"go.ripls.org/ripls/server/storage"
)

// enrichContributionTransfers populates the transfer fields (#2708) on
// contribution responses from the child transfers their contributions
// escalated into ("I'll bring my wheelbarrow"). Batched: one query for all
// referenced transfers, matched to the responses by their shared order (both
// are built from the same slice). A response whose contribution carries no
// transfer, or whose transfer can't be found, is left unchanged.
func (s *Service) enrichContributionTransfers(
	ctx context.Context,
	resps []*api.ExperienceContributionResponse,
	contribs []*planning.EnrichedContribution,
) error {
	ids := make([]string, 0, len(contribs))
	for _, e := range contribs {
		if tid := e.Contribution.GetTransferId(); tid != "" {
			ids = append(ids, tid)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	transfers, err := storage.GetByIDs[*models.Transfer](s.storage, ctx, ids, storage.QueryOptions{})
	if err != nil {
		return connecterr.Internal(ctx, "enrichContributionTransfers", err)
	}
	for i, e := range contribs {
		tid := e.Contribution.GetTransferId()
		if tid == "" {
			continue
		}
		if t, ok := transfers[tid]; ok {
			applyTransferToContribResp(resps[i], t)
		}
	}
	return nil
}

// applyTransferToContribResp stamps a child transfer's id/state/type onto a
// contribution response (#2708).
func applyTransferToContribResp(resp *api.ExperienceContributionResponse, t *models.Transfer) {
	id := t.Id
	resp.TransferId = &id
	st := transferStateToAPI(t.State)
	resp.TransferState = &st
	tt := transferTypeToAPI(t.TransferType)
	resp.TransferType = &tt
}

// transferTypeToAPI converts a storage TransferType to its API form.
func transferTypeToAPI(t models.TransferType) api.TransferType {
	switch t {
	case models.TransferType_TRANSFER_TYPE_LOAN:
		return api.TransferType_TRANSFER_TYPE_LOAN
	case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
		return api.TransferType_TRANSFER_TYPE_GIVEAWAY
	default:
		return api.TransferType_TRANSFER_TYPE_UNSPECIFIED
	}
}

// transferStateToAPI converts a storage TransferState to its API form.
func transferStateToAPI(s models.TransferState) api.TransferState {
	switch s {
	case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
		return api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED
	case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		return api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		return api.TransferState_TRANSFER_STATE_ACTIVE
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		return api.TransferState_TRANSFER_STATE_COMPLETED
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		return api.TransferState_TRANSFER_STATE_CANCELLED
	default:
		return api.TransferState_TRANSFER_STATE_UNSPECIFIED
	}
}
