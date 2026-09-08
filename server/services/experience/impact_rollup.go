package experience

import (
	"context"
	"fmt"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// deliverableChildTransfers returns this experience's gear-backed child
// transfers whose item-based impact the completion delivers (#2724): every
// origin-linked transfer that is not cancelled. completeEventChildTransfers
// (transfer service) completes the non-terminal ones when the
// EXPERIENCE_COMPLETED event lands; already-completed children keep the
// impact stamped at their own completion.
func (s *Service) deliverableChildTransfers(ctx context.Context, experienceID string) ([]*models.Transfer, error) {
	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "origin_experience_id", experienceID)
	if err != nil {
		return nil, fmt.Errorf("query origin-experience transfers: %w", err)
	}
	deliverable := transfers[:0]
	for _, t := range transfers {
		if t.State == models.TransferState_TRANSFER_STATE_CANCELLED {
			continue
		}
		deliverable = append(deliverable, t)
	}
	return deliverable, nil
}

// adoptChildTransferImpact splices the summed item-based dimensions of the
// experience's deliverable child transfers into ie: money, emissions, and
// time roll up from every child's estimate (falling back to a fresh
// item-based build when a transfer has none persisted yet), summed with
// quadrature uncertainty; Quality Time stays the experience's own
// computation. Mirrors the request-side roll-up (#2702). The adopted
// dimensions are display copies — community and user aggregation counts each
// child once, on the transfer (see impact_metrics.ExperienceDimensionValue).
// Returns the adopted transfer id ("" when no child produced an estimate, in
// which case ie is unchanged).
func (s *Service) adoptChildTransferImpact(ctx context.Context, ie *api.ImpactEstimate, transfers []*models.Transfer, connCtx *api.ConnectionContext, logger *logging.Logger) string {
	if ie == nil || s.estimatorCfg == nil || len(transfers) == 0 {
		return ""
	}
	itemEstimates := make([]*api.ImpactEstimate, 0, len(transfers))
	for _, transfer := range transfers {
		itemBased := impact_metrics.ModelsImpactToAPI(transfer.ImpactEstimate)
		if itemBased == nil {
			gear := &models.Gear{}
			if err := s.storage.GetByID(ctx, transfer.GearId, gear); err != nil {
				logger.Warn("gear missing for child-transfer impact; skipping child",
					"gear_id", transfer.GearId, "transfer_id", transfer.Id, "error", err)
				continue
			}
			itemBased = impact_metrics.BuildTransferImpactMetrics(gear, transfer.TransferType, s.estimatorCfg, connCtx, nil)
		}
		itemEstimates = append(itemEstimates, itemBased)
	}
	if len(itemEstimates) == 0 {
		return ""
	}
	summed := impact_metrics.SumTransferImpactDimensions(itemEstimates)
	ie.MoneySaved = summed.MoneySaved
	ie.EmissionsPrevented = summed.EmissionsPrevented
	ie.TimeSaved = summed.TimeSaved
	return transfers[0].Id
}
