package impact_metrics

import (
	"context"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/storage"
)

// Service handles impact metrics RPC endpoints.
type Service struct {
	calculator             *impactlib.Calculator
	metricDetailCalculator *impactlib.MetricDetailCalculator
	estimatorCfg           *estimator.Config
	storage                *storage.ProtoSQLStorage
	aiProvider             ai.Provider // Optional: enables LLM-enriched impact drafts.
}

// NewService creates a new impact Service instance.
func NewService(db *storage.ProtoSQLStorage, cfg *estimator.Config) *Service {
	return &Service{
		calculator:             impactlib.NewCalculator(db, cfg),
		metricDetailCalculator: impactlib.NewMetricDetailCalculator(db, cfg),
		estimatorCfg:           cfg,
		storage:                db,
	}
}

// SetAIProvider sets the AI provider used for LLM-enriched impact drafts.
// When not set, DraftImpactEstimate falls back to config-default attributes.
func (s *Service) SetAIProvider(provider ai.Provider) {
	s.aiProvider = provider
}

// GetCommunityImpactMetrics retrieves impact metrics for a community.
func (s *Service) GetCommunityImpactMetrics(
	ctx context.Context,
	req *connect.Request[api.GetCommunityImpactMetricsRequest],
) (*connect.Response[api.GetCommunityImpactMetricsResponse], error) {
	return getCommunityImpactMetrics(ctx, s.calculator, req)
}

// GetUserImpactMetrics retrieves aggregated impact metrics for a user.
func (s *Service) GetUserImpactMetrics(
	ctx context.Context,
	req *connect.Request[api.GetUserImpactMetricsRequest],
) (*connect.Response[api.GetUserImpactMetricsResponse], error) {
	return getUserImpactMetrics(ctx, s.calculator, req)
}

// GetCommunityMetricDetail retrieves detailed breakdown data for a specific metric dimension.
func (s *Service) GetCommunityMetricDetail(
	ctx context.Context,
	req *connect.Request[api.GetCommunityMetricDetailRequest],
) (*connect.Response[api.GetCommunityMetricDetailResponse], error) {
	return getCommunityMetricDetail(ctx, s, req)
}

// GetCommunityActsDetail retrieves the per-category and per-month
// breakdown of community acts.
func (s *Service) GetCommunityActsDetail(
	ctx context.Context,
	req *connect.Request[api.GetCommunityActsDetailRequest],
) (*connect.Response[api.GetCommunityActsDetailResponse], error) {
	return getCommunityActsDetail(ctx, s, req)
}

// GetCommunityActions retrieves a paginated list of individual sharing actions.
func (s *Service) GetCommunityActions(
	ctx context.Context,
	req *connect.Request[api.GetCommunityActionsRequest],
) (*connect.Response[api.GetCommunityActionsResponse], error) {
	return getCommunityActions(ctx, s, req)
}

// GetCommunityProblemsSolvedDetail retrieves the community's "problems solved"
// breakdown (completed loans + fulfilled requests + need-fulfilling experiences).
func (s *Service) GetCommunityProblemsSolvedDetail(
	ctx context.Context,
	req *connect.Request[api.GetCommunityProblemsSolvedDetailRequest],
) (*connect.Response[api.GetCommunityProblemsSolvedDetailResponse], error) {
	return getCommunityProblemsSolvedDetail(ctx, s, req)
}

// GetCommunityUtilization retrieves utilization analytics for a community.
func (s *Service) GetCommunityUtilization(
	ctx context.Context,
	req *connect.Request[api.GetCommunityUtilizationRequest],
) (*connect.Response[api.GetCommunityUtilizationResponse], error) {
	return getCommunityUtilization(ctx, s, req)
}

// GetTimeToSolveDetail retrieves time-to-solve detail for a community.
func (s *Service) GetTimeToSolveDetail(
	ctx context.Context,
	req *connect.Request[api.GetTimeToSolveDetailRequest],
) (*connect.Response[api.GetTimeToSolveDetailResponse], error) {
	return getTimeToSolveDetail(ctx, s.calculator.Storage(), req)
}

// GetCommunityLeaderboard retrieves top members ranked by impact.
func (s *Service) GetCommunityLeaderboard(
	ctx context.Context,
	req *connect.Request[api.GetCommunityLeaderboardRequest],
) (*connect.Response[api.GetCommunityLeaderboardResponse], error) {
	return getCommunityLeaderboard(ctx, s.storage, req)
}

// GetUserCommunityImpactDetail retrieves a user's impact breakdown in a community.
func (s *Service) GetUserCommunityImpactDetail(
	ctx context.Context,
	req *connect.Request[api.GetUserCommunityImpactDetailRequest],
) (*connect.Response[api.GetUserCommunityImpactDetailResponse], error) {
	return getUserCommunityImpactDetail(ctx, s.storage, s.estimatorCfg, req)
}

// GetCommunityPercentileDetail retrieves percentile ranking detail for a community.
func (s *Service) GetCommunityPercentileDetail(
	ctx context.Context,
	req *connect.Request[api.GetCommunityPercentileDetailRequest],
) (*connect.Response[api.GetCommunityPercentileDetailResponse], error) {
	return getCommunityPercentileDetail(ctx, s.storage, s.estimatorCfg, req)
}

// DraftImpactEstimate produces an AI-enriched impact estimate for an experience or request.
func (s *Service) DraftImpactEstimate(
	ctx context.Context,
	req *connect.Request[api.DraftImpactEstimateRequest],
) (*connect.Response[api.DraftImpactEstimateResponse], error) {
	return draftImpactEstimate(ctx, s, req)
}

// DraftImpactEstimateWithOverrides recomputes an impact estimate from caller-supplied overrides.
func (s *Service) DraftImpactEstimateWithOverrides(
	ctx context.Context,
	req *connect.Request[api.DraftImpactEstimateWithOverridesRequest],
) (*connect.Response[api.DraftImpactEstimateWithOverridesResponse], error) {
	return draftImpactEstimateWithOverrides(ctx, s, req)
}

// Verify that Service implements the ImpactServiceHandler interface.
var _ apiconnect.ImpactServiceHandler = (*Service)(nil)
