package impact_metrics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// Calculator handles impact metric calculation logic.
type Calculator struct {
	storage      *storage.ProtoSQLStorage
	estimatorCfg *estimator.Config
}

// NewCalculator creates a new Calculator instance.
func NewCalculator(storage *storage.ProtoSQLStorage, cfg *estimator.Config) *Calculator {
	return &Calculator{storage: storage, estimatorCfg: cfg}
}

// Storage returns the underlying storage instance.
func (c *Calculator) Storage() *storage.ProtoSQLStorage {
	return c.storage
}

// EstimatorConfig returns the estimator configuration.
func (c *Calculator) EstimatorConfig() *estimator.Config {
	return c.estimatorCfg
}

// ActivityCounts holds basic activity metrics for a community.
type ActivityCounts struct {
	GearCount          int32
	ActiveLoans        int32
	CompletedLoans     int32
	OpenGiveaways      int32
	CompletedGiveaways int32
	OpenRequests       int32
	FulfilledRequests  int32
	UpcomingEvents     int32
	PastEvents         int32
	MemberCount        int32
}

// CalculateActivityCounts loads a one-shot dataset and delegates to
// CalculateActivityCountsFromDataset. Use the dataset variant directly
// in handlers that compute multiple metrics over the same community
// (one preload, many calculate calls). See #2055.
func (c *Calculator) CalculateActivityCounts(ctx context.Context, communityID string) (*ActivityCounts, error) {
	ds, err := LoadCommunityDataset(ctx, c.storage, communityID)
	if err != nil {
		return nil, err
	}
	return c.CalculateActivityCountsFromDataset(ctx, ds), nil
}

// CalculateActivityCountsFromDataset computes activity counts from a
// preloaded CommunityDataset. Pure compute — no DB access, no error
// path; safe to call repeatedly over the same dataset.
func (c *Calculator) CalculateActivityCountsFromDataset(ctx context.Context, ds *CommunityDataset) *ActivityCounts {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CalculateActivityCounts",
		"community_id", ds.CommunityID,
	)

	counts := &ActivityCounts{}

	// Gear count: every non-deleted gear referenced by CommunityGear.
	gearIDs := make(map[string]bool)
	for _, cg := range ds.CommunityGear {
		gear, ok := ds.Gear[cg.GearId]
		if !ok || gear.Deleted != nil {
			continue
		}
		gearIDs[gear.Id] = true
	}
	// #nosec G115 - len(map) will not overflow int32 in practice
	counts.GearCount = int32(len(gearIDs))

	// Loans and giveaways from the Transfer table.
	for _, transfer := range ds.Transfers {
		if transfer.Deleted != nil {
			continue
		}
		switch transfer.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			switch transfer.State {
			case models.TransferState_TRANSFER_STATE_ACTIVE:
				counts.ActiveLoans++
			case models.TransferState_TRANSFER_STATE_COMPLETED:
				counts.CompletedLoans++
			}
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			switch transfer.State {
			case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
				models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
				counts.OpenGiveaways++
			case models.TransferState_TRANSFER_STATE_COMPLETED:
				counts.CompletedGiveaways++
			}
		}
	}

	// Requests: non-archived CommunityRequest rows whose Request is
	// non-deleted contribute to open/fulfilled counts.
	for _, cr := range ds.CommunityRequests {
		if cr.Archived {
			continue
		}
		req, ok := ds.Requests[cr.RequestId]
		if !ok || req.Deleted != nil {
			continue
		}
		switch req.State {
		case models.RequestState_REQUEST_STATE_ACTIVE,
			models.RequestState_REQUEST_STATE_OFFERS_RECEIVED:
			counts.OpenRequests++
		case models.RequestState_REQUEST_STATE_FULFILLED:
			counts.FulfilledRequests++
		}
	}

	// Experiences: classify by state + time into upcoming / past.
	now := time.Now().Unix()
	for _, exp := range ds.Experiences {
		if exp.Deleted != nil {
			continue
		}
		isUpcoming := false
		if exp.State == models.ExperienceState_EXPERIENCE_STATE_ACTIVE ||
			exp.State == models.ExperienceState_EXPERIENCE_STATE_JOINED {
			if exp.Time != nil {
				switch t := exp.Time.TimeType.(type) {
				case *models.ExperienceTime_Specific:
					if t.Specific.UnixTimestampSec > now {
						isUpcoming = true
					}
				case *models.ExperienceTime_Range:
					if t.Range.StartUnixSec > now {
						isUpcoming = true
					}
				case *models.ExperienceTime_Tbd:
					isUpcoming = true
				}
			}
		}
		if isUpcoming {
			counts.UpcomingEvents++
		} else if exp.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			counts.PastEvents++
		}
	}

	// #nosec G115 - len(slice) will not overflow int32 in practice
	counts.MemberCount = int32(len(ds.CommunityUsers))

	logger.InfoContext(ctx, "calculated activity counts",
		"gear_count", counts.GearCount,
		"active_loans", counts.ActiveLoans,
		"completed_loans", counts.CompletedLoans,
		"open_giveaways", counts.OpenGiveaways,
		"completed_giveaways", counts.CompletedGiveaways,
		"open_requests", counts.OpenRequests,
		"fulfilled_requests", counts.FulfilledRequests,
		"upcoming_events", counts.UpcomingEvents,
		"past_events", counts.PastEvents,
		"member_count", counts.MemberCount,
	)

	return counts
}

// ImpactSavingsResult holds the aggregated impact savings for a community.
type ImpactSavingsResult struct {
	CostSavings      *api.Estimate // USD
	CarbonSavings    *api.Estimate // grams CO2e
	TimeBanked       *api.Estimate // minutes
	TimeFromLoans    *api.Estimate // minutes
	TimeFromRequests *api.Estimate // minutes
	TimeFromSkills   *api.Estimate // minutes
	QualityTime      *api.Estimate // Quality Time minutes
	CostCount        int32
	CarbonCount      int32
}

// CalculateImpactSavings loads a one-shot dataset and delegates to
// CalculateImpactSavingsFromDataset. Use the dataset variant directly
// when the caller already has a preloaded dataset.
func (c *Calculator) CalculateImpactSavings(ctx context.Context, communityID string) (*ImpactSavingsResult, error) {
	ds, err := LoadCommunityDataset(ctx, c.storage, communityID)
	if err != nil {
		return nil, err
	}
	return c.CalculateImpactSavingsFromDataset(ctx, ds), nil
}

// CalculateImpactSavingsFromDataset aggregates impact savings from a
// preloaded CommunityDataset. Pure compute — no DB access. Reads
// ImpactEstimate from each transaction; if absent, computes a fallback
// via the builder functions against the dataset's gear map.
func (c *Calculator) CalculateImpactSavingsFromDataset(ctx context.Context, ds *CommunityDataset) *ImpactSavingsResult {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CalculateImpactSavings",
		"community_id", ds.CommunityID,
	)

	var costEstimates, carbonEstimates []*api.Estimate
	var loanTimeEstimates, requestTimeEstimates, skillTimeEstimates []*api.Estimate
	var sfEstimates []*api.Estimate
	var costCount, carbonCount int32

	// 1. Transfers: completed, non-deleted only. Fallback gear lookups
	// resolve from ds.Gear (which LoadCommunityDataset preloaded).
	for _, transfer := range ds.Transfers {
		if transfer.Deleted != nil || transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		ie := c.transferImpactWithGear(transfer, ds.Gear)
		if ce := CostEstimate(ie); ce != nil {
			costEstimates = append(costEstimates, ce)
			costCount++
		}
		if ce := CarbonEstimate(ie); ce != nil {
			carbonEstimates = append(carbonEstimates, ce)
			carbonCount++
		}
		if te := TimeEstimate(ie); te != nil {
			loanTimeEstimates = append(loanTimeEstimates, te)
		}
		if se := QualityTimeEstimate(ie); se != nil {
			sfEstimates = append(sfEstimates, se)
		}
	}

	// 2. Requests: fulfilled (including archived — fulfilled requests are always archived).
	for _, request := range ds.Requests {
		if request.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		ie := c.requestImpact(request)
		// Cost from requests/experiences feeds the brief's
		// TotalReplacedCostUSD so the Workshop home tile aligns with
		// the detail screen's source breakdown — which already groups
		// transfers + requests + experiences (#1898).
		if ce := CostEstimate(ie); ce != nil {
			costEstimates = append(costEstimates, ce)
			costCount++
		}
		if ce := CarbonEstimate(ie); ce != nil {
			carbonEstimates = append(carbonEstimates, ce)
			carbonCount++
		}
		if te := TimeEstimate(ie); te != nil {
			requestTimeEstimates = append(requestTimeEstimates, te)
		}
		if se := QualityTimeEstimate(ie); se != nil {
			sfEstimates = append(sfEstimates, se)
		}
	}

	// 3. Experiences: completed, non-deleted.
	for _, experience := range ds.Experiences {
		if experience.Deleted != nil {
			continue
		}
		if experience.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			continue
		}
		ie := c.experienceImpact(experience)
		// See the matching block in the requests loop above — experiences
		// also contribute to the brief's TotalReplacedCostUSD so the home
		// tile lines up with the detail-screen breakdown.
		if ce := CostEstimate(ie); ce != nil {
			costEstimates = append(costEstimates, ce)
			costCount++
		}
		if ce := CarbonEstimate(ie); ce != nil {
			carbonEstimates = append(carbonEstimates, ce)
			carbonCount++
		}
		if te := TimeEstimate(ie); te != nil {
			skillTimeEstimates = append(skillTimeEstimates, te)
		}
		if se := QualityTimeEstimate(ie); se != nil {
			sfEstimates = append(sfEstimates, se)
		}
	}

	timeFromLoans := estimator.SumEstimates(loanTimeEstimates)
	timeFromRequests := estimator.SumEstimates(requestTimeEstimates)
	timeFromSkills := estimator.SumEstimates(skillTimeEstimates)

	allTimeEstimates := append(append(loanTimeEstimates, requestTimeEstimates...), skillTimeEstimates...)
	timeBanked := estimator.SumEstimates(allTimeEstimates)

	result := &ImpactSavingsResult{
		CostSavings:      estimator.SumEstimates(costEstimates),
		CarbonSavings:    estimator.SumEstimates(carbonEstimates),
		TimeBanked:       timeBanked,
		TimeFromLoans:    timeFromLoans,
		TimeFromRequests: timeFromRequests,
		TimeFromSkills:   timeFromSkills,
		QualityTime:      estimator.SumEstimates(sfEstimates),
		CostCount:        costCount,
		CarbonCount:      carbonCount,
	}

	logger.InfoContext(ctx, "calculated impact savings",
		"cost_count", costCount,
		"carbon_count", carbonCount,
		"cost_mean_usd", result.CostSavings.Mean,
		"carbon_mean_grams", result.CarbonSavings.Mean,
		"time_banked_mean_minutes", result.TimeBanked.Mean,
		"quality_time_mean_minutes", result.QualityTime.Mean,
	)

	return result
}

// transferImpact returns the ImpactEstimate for a transfer, converting from storage model
// if present, or computing a fallback via the builder using an empty gear struct.
func (c *Calculator) transferImpact(transfer *models.Transfer) *api.ImpactEstimate {
	return c.transferImpactWithGear(transfer, map[string]*models.Gear{})
}

// transferImpactWithGear returns the ImpactEstimate for a transfer, converting from storage
// model if present, or computing a fallback using the pre-fetched gear map.
func (c *Calculator) transferImpactWithGear(transfer *models.Transfer, gearMap map[string]*models.Gear) *api.ImpactEstimate {
	if transfer.ImpactEstimate != nil {
		return ModelsImpactToAPI(transfer.ImpactEstimate)
	}

	// Fallback: look up gear from pre-fetched map and compute via builder.
	gear := &models.Gear{}
	if g, ok := gearMap[transfer.GearId]; ok {
		gear = g
	}
	return BuildTransferImpactMetrics(gear, transfer.TransferType, c.estimatorCfg, nil, nil)
}

// requestImpact returns the ImpactEstimate for a request, converting from storage model
// if present, or computing a fallback via the builder. Transfer-adopted
// dimensions are masked (#2702) — the transfer counts them, not the request.
func (c *Calculator) requestImpact(request *models.Request) *api.ImpactEstimate {
	if request.ImpactEstimate != nil {
		return MaskTransferAdoptedRequestDimensions(request, ModelsImpactToAPI(request.ImpactEstimate))
	}
	// Extract value from request if available, otherwise use 0
	var valueUSD float32
	if request.ValueEstimate != nil {
		valueUSD = request.ValueEstimate.EstimatedValueUsd
	}
	return MaskTransferAdoptedRequestDimensions(request, BuildRequestImpactMetrics(valueUSD, c.estimatorCfg, nil, nil))
}

// experienceImpact returns the ImpactEstimate for an experience, converting from storage model
// if present, or computing a fallback via the builder. Transfer-adopted
// dimensions are masked (#2724) — the child transfers count them, not the
// experience.
func (c *Calculator) experienceImpact(experience *models.Experience) *api.ImpactEstimate {
	if experience.ImpactEstimate != nil {
		return MaskTransferAdoptedExperienceDimensions(experience, ModelsImpactToAPI(experience.ImpactEstimate))
	}
	// Extract value from experience if available, otherwise use 0
	var valueUSD float32
	if experience.ValueEstimate != nil {
		valueUSD = experience.ValueEstimate.EstimatedValueUsd
	}
	return BuildExperienceImpactMetrics(valueUSD, 1, c.estimatorCfg, nil, nil)
}

// TotalValueResult holds the result of total value calculation.
type TotalValueResult struct {
	TotalValueUsd float32
	GearCount     int32
}

// CalculateTotalValue loads a one-shot dataset and delegates to
// CalculateTotalValueFromDataset.
func (c *Calculator) CalculateTotalValue(ctx context.Context, communityID string) (*TotalValueResult, error) {
	ds, err := LoadCommunityDataset(ctx, c.storage, communityID)
	if err != nil {
		return nil, err
	}
	return c.CalculateTotalValueFromDataset(ctx, ds), nil
}

// CalculateTotalValueFromDataset sums value estimates (USD) for active
// gear in a preloaded dataset. Items without value estimates are
// excluded (counted as 0). Pure compute — no DB access.
func (c *Calculator) CalculateTotalValueFromDataset(ctx context.Context, ds *CommunityDataset) *TotalValueResult {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CalculateTotalValue",
		"community_id", ds.CommunityID,
	)

	var totalUsd float32
	var gearCount int32
	for _, cg := range ds.CommunityGear {
		gear, ok := ds.Gear[cg.GearId]
		if !ok || gear.Deleted != nil {
			continue
		}
		gearCount++
		if gear.ValueEstimate != nil && gear.ValueEstimate.EstimatedValueUsd > 0 {
			totalUsd += gear.ValueEstimate.EstimatedValueUsd
		}
	}

	logger.InfoContext(ctx, "calculated total community value",
		"total_value_usd", totalUsd,
		"gear_count", gearCount,
	)

	return &TotalValueResult{
		TotalValueUsd: totalUsd,
		GearCount:     gearCount,
	}
}

// CalculateCo2Potential loads a one-shot dataset and delegates to
// CalculateCo2PotentialFromDataset.
func (c *Calculator) CalculateCo2Potential(ctx context.Context, communityID string) (float32, error) {
	ds, err := LoadCommunityDataset(ctx, c.storage, communityID)
	if err != nil {
		return 0, err
	}
	return c.CalculateCo2PotentialFromDataset(ctx, ds), nil
}

// CalculateCo2PotentialFromDataset returns the total potential CO₂
// savings (grams) if every active gear item were shared once: sum of
// embodied_carbon.co2e_grams.mean across the dataset's library. Pure
// compute — no DB access.
func (c *Calculator) CalculateCo2PotentialFromDataset(ctx context.Context, ds *CommunityDataset) float32 {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CalculateCo2Potential",
		"community_id", ds.CommunityID,
	)

	var totalGrams float32
	for _, cg := range ds.CommunityGear {
		gear, ok := ds.Gear[cg.GearId]
		if !ok || gear.Deleted != nil {
			continue
		}
		if gear.EmbodiedCarbon != nil && gear.EmbodiedCarbon.Co2EGrams != nil {
			totalGrams += gear.EmbodiedCarbon.Co2EGrams.Mean
		}
	}

	logger.InfoContext(ctx, "calculated CO2 potential",
		"total_grams", totalGrams,
	)
	return totalGrams
}

// CalculateTimeToSolveMedian loads a one-shot dataset and delegates to
// CalculateTimeToSolveMedianFromDataset.
func (c *Calculator) CalculateTimeToSolveMedian(ctx context.Context, communityID string) (float32, error) {
	ds, err := LoadCommunityDataset(ctx, c.storage, communityID)
	if err != nil {
		return 0, err
	}
	return c.CalculateTimeToSolveMedianFromDataset(ctx, ds), nil
}

// CalculateTimeToSolveMedianFromDataset returns the median minutes
// from request creation to fulfillment for fulfilled, non-deleted
// requests in the preloaded dataset. Pure compute — no DB access.
//
// Note: the dataset preloads requests with IncludeDeleted:true; this
// function applies the soft-delete filter inline to preserve the
// behaviour of the original pre-#2055 implementation (which queried
// without IncludeDeleted and so never saw deleted rows).
func (c *Calculator) CalculateTimeToSolveMedianFromDataset(ctx context.Context, ds *CommunityDataset) float32 {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CalculateTimeToSolveMedian",
		"community_id", ds.CommunityID,
	)

	if len(ds.CommunityRequests) == 0 {
		return 0
	}

	sharedAt := make(map[string]int64, len(ds.CommunityRequests))
	for _, cr := range ds.CommunityRequests {
		sharedAt[cr.RequestId] = cr.SharedAtUnixSec
	}

	var solveTimes []float64
	for _, r := range ds.Requests {
		if r.Deleted != nil {
			continue
		}
		if r.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		if r.FulfilledAtUnixSec == nil || *r.FulfilledAtUnixSec <= 0 {
			continue
		}
		shared, ok := sharedAt[r.Id]
		if !ok || shared <= 0 {
			continue
		}
		diffSec := *r.FulfilledAtUnixSec - shared
		if diffSec <= 0 {
			continue
		}
		solveTimes = append(solveTimes, float64(diffSec)/60)
	}

	if len(solveTimes) == 0 {
		return 0
	}

	sort.Float64s(solveTimes)
	median := solveTimes[len(solveTimes)/2]
	if len(solveTimes)%2 == 0 {
		median = (solveTimes[len(solveTimes)/2-1] + solveTimes[len(solveTimes)/2]) / 2
	}

	logger.InfoContext(ctx, "calculated time-to-solve median",
		"median_minutes", median,
		"fulfilled_count", len(solveTimes),
	)

	return float32(median)
}

// CalculateUserImpactFromPreloadedData computes a user's total impact for a
// dimension using pre-loaded community data, performing no database I/O.
// transfers is the full set of community transfers; requestMap maps request ID
// to request for all community requests. Call this in a loop to avoid N+1 queries.
func CalculateUserImpactFromPreloadedData(
	userID string,
	transfers []*models.Transfer,
	requestMap map[string]*models.Request,
	dimension api.ImpactMetricDimension,
) float64 {
	var total float64

	for _, t := range transfers {
		if t.Deleted != nil || t.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		if t.OwnerId != userID && t.RecipientId != userID {
			continue
		}
		total += ExtractDimensionValue(t.ImpactEstimate, dimension)
	}

	for _, r := range requestMap {
		if r.State != models.RequestState_REQUEST_STATE_FULFILLED {
			continue
		}
		if r.RequesterId != userID && !ContainsString(r.ConfirmedHelperIds, userID) {
			continue
		}
		total += RequestDimensionValue(r, dimension)
	}

	return total
}

// CalculateUserImpactInCommunity computes a user's total impact value for a
// specific dimension within a community. Sums impact from completed transfers
// and fulfilled requests where the user participated.
func (c *Calculator) CalculateUserImpactInCommunity(
	ctx context.Context,
	userID, communityID string,
	dimension api.ImpactMetricDimension,
) (float64, error) {
	transfers, err := storage.QueryByField[*models.Transfer](c.storage, ctx, "community_id", communityID)
	if err != nil {
		return 0, fmt.Errorf("failed to query transfers: %w", err)
	}

	communityRequests, err := storage.QueryByField[*models.CommunityRequest](c.storage, ctx, "community_id", communityID)
	if err != nil {
		return 0, fmt.Errorf("failed to query community requests: %w", err)
	}

	requestIDs := storage.CollectField(communityRequests, func(cr *models.CommunityRequest) string { return cr.RequestId })
	var requestMap map[string]*models.Request
	if len(requestIDs) > 0 {
		requestMap, err = storage.GetByIDs[*models.Request](c.storage, ctx, requestIDs, storage.QueryOptions{})
		if err != nil {
			return 0, fmt.Errorf("failed to batch fetch requests: %w", err)
		}
	}

	return CalculateUserImpactFromPreloadedData(userID, transfers, requestMap, dimension), nil
}

// ProblemsCounts is the "problems handled" fraction for a community, split by
// bucket. The Handled* fields are the numerator (X) — completed loans,
// fulfilled requests, and claimed needs. The Potential* fields are the
// denominator (Y) — every non-cancelled loan transfer, every non-cancelled
// request, and every posted need. Each Handled* is a subset of its matching
// Potential*, so Handled() <= Potential().
type ProblemsCounts struct {
	HandledLoans      int
	PotentialLoans    int
	HandledRequests   int
	PotentialRequests int
	HandledNeeds      int
	PotentialNeeds    int
}

// Handled returns the numerator: completed loans + fulfilled requests +
// claimed needs.
func (p ProblemsCounts) Handled() int {
	return p.HandledLoans + p.HandledRequests + p.HandledNeeds
}

// Potential returns the denominator: all non-cancelled loan transfers + all
// non-cancelled requests + all posted needs.
func (p ProblemsCounts) Potential() int {
	return p.PotentialLoans + p.PotentialRequests + p.PotentialNeeds
}

// CalculateProblemsCounts computes the "problems handled" X-of-Y fraction for a
// community — the single source of truth for both the Workshop headline figure
// and the problems-solved deep-dive's numbers. See [ProblemsCounts].
func (c *Calculator) CalculateProblemsCounts(
	ctx context.Context,
	communityID string,
) (*ProblemsCounts, error) {
	out := &ProblemsCounts{}

	// Loans: LOAN transfers — completed (handled) vs all non-cancelled
	// (potential). Giveaways are excluded entirely.
	transfersRaw, err := c.storage.QueryByField(ctx, "community_id", communityID, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("query transfers: %w", err)
	}
	for _, raw := range transfersRaw {
		t := raw.(*models.Transfer)
		if t.Deleted != nil || t.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
			continue
		}
		if t.State == models.TransferState_TRANSFER_STATE_UNSPECIFIED ||
			t.State == models.TransferState_TRANSFER_STATE_CANCELLED {
			continue
		}
		out.PotentialLoans++
		if t.State == models.TransferState_TRANSFER_STATE_COMPLETED {
			out.HandledLoans++
		}
	}

	// Requests: fulfilled (handled) vs all non-cancelled (potential).
	communityRequestsRaw, err := c.storage.QueryByField(ctx, "community_id", communityID, &models.CommunityRequest{})
	if err != nil {
		return nil, fmt.Errorf("query community requests: %w", err)
	}
	for _, raw := range communityRequestsRaw {
		cr := raw.(*models.CommunityRequest)
		req := &models.Request{}
		if getErr := c.storage.GetByID(ctx, cr.RequestId, req); getErr != nil {
			continue
		}
		if req.Deleted != nil {
			continue
		}
		if req.State == models.RequestState_REQUEST_STATE_UNSPECIFIED ||
			req.State == models.RequestState_REQUEST_STATE_CANCELLED {
			continue
		}
		out.PotentialRequests++
		if req.State == models.RequestState_REQUEST_STATE_FULFILLED {
			out.HandledRequests++
		}
	}

	// Needs: claimed (handled) vs all posted (potential).
	needData, err := c.CommunityNeedData(ctx, communityID)
	if err != nil {
		return nil, fmt.Errorf("community need data: %w", err)
	}
	for _, n := range needData.Needs {
		out.PotentialNeeds++
		if needData.IsClaimed(n) {
			out.HandledNeeds++
		}
	}

	return out, nil
}

// CommunityNeedData bundles a community's planning needs with the claim
// contributions that handled them. Claims maps a need id to the earliest
// non-deleted contribution claimed from that need
// (PlanningContribution.from_need_id).
type CommunityNeedData struct {
	Needs  []*models.PlanningNeed
	Claims map[string]*models.PlanningContribution
}

// IsClaimed reports whether a need has been claimed at least once.
func (d *CommunityNeedData) IsClaimed(need *models.PlanningNeed) bool {
	_, ok := d.Claims[need.Id]
	return ok
}

// CommunityNeedData loads every non-deleted planning need scoped to a
// community's experiences and requests, plus the claim contributions that
// handled them. Shared by the headline count and the deep-dive item list so
// both agree on which needs are "handled".
func (c *Calculator) CommunityNeedData(
	ctx context.Context,
	communityID string,
) (*CommunityNeedData, error) {
	expIDs, reqIDs, err := c.communityScopeIDs(ctx, communityID)
	if err != nil {
		return nil, err
	}

	data := &CommunityNeedData{Claims: map[string]*models.PlanningContribution{}}

	addNeeds := func(field string, ids []string) error {
		if len(ids) == 0 {
			return nil
		}
		rows, qerr := c.storage.QueryByFieldIn(ctx, field, ids, &models.PlanningNeed{})
		if qerr != nil {
			return fmt.Errorf("query needs by %s: %w", field, qerr)
		}
		for _, raw := range rows {
			n := raw.(*models.PlanningNeed)
			if n.Deleted == nil {
				data.Needs = append(data.Needs, n)
			}
		}
		return nil
	}
	if err := addNeeds("experience_id", expIDs); err != nil {
		return nil, err
	}
	if err := addNeeds("request_id", reqIDs); err != nil {
		return nil, err
	}

	addClaims := func(field string, ids []string) error {
		if len(ids) == 0 {
			return nil
		}
		rows, qerr := c.storage.QueryByFieldIn(ctx, field, ids, &models.PlanningContribution{})
		if qerr != nil {
			return fmt.Errorf("query contributions by %s: %w", field, qerr)
		}
		for _, raw := range rows {
			pc := raw.(*models.PlanningContribution)
			needID := pc.GetFromNeedId()
			if pc.Deleted != nil || needID == "" {
				continue
			}
			if prev, ok := data.Claims[needID]; !ok || pc.CreatedAtUnixSec < prev.CreatedAtUnixSec {
				data.Claims[needID] = pc
			}
		}
		return nil
	}
	if err := addClaims("experience_id", expIDs); err != nil {
		return nil, err
	}
	if err := addClaims("request_id", reqIDs); err != nil {
		return nil, err
	}

	return data, nil
}

// communityScopeIDs returns the community's experience and request ids via the
// CommunityExperience / CommunityRequest junctions (soft-deleted rows are
// excluded by the default query filter).
func (c *Calculator) communityScopeIDs(
	ctx context.Context,
	communityID string,
) (expIDs, reqIDs []string, err error) {
	ceRows, err := c.storage.QueryByField(ctx, "community_id", communityID, &models.CommunityExperience{})
	if err != nil {
		return nil, nil, fmt.Errorf("query community experiences: %w", err)
	}
	for _, raw := range ceRows {
		expIDs = append(expIDs, raw.(*models.CommunityExperience).ExperienceId)
	}
	crRows, err := c.storage.QueryByField(ctx, "community_id", communityID, &models.CommunityRequest{})
	if err != nil {
		return nil, nil, fmt.Errorf("query community requests: %w", err)
	}
	for _, raw := range crRows {
		reqIDs = append(reqIDs, raw.(*models.CommunityRequest).RequestId)
	}
	return expIDs, reqIDs, nil
}
