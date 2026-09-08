package request

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
)

// GetRequestStats retrieves statistics for a request (offers, fulfillments, help value, days open).
func (s *Service) GetRequestStats(
	ctx context.Context,
	req *connect.Request[api.GetRequestStatsRequest],
) (*connect.Response[api.GetRequestStatsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"target_request_id", req.Msg.RequestId,
	)

	logger.DebugContext(ctx, "fetching request stats")

	// Fetch request to verify it exists
	request := &models.Request{}
	err = s.storage.GetByID(ctx, req.Msg.RequestId, request)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get request",
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
	}

	// Build query for offers related to this request
	queryFields := map[string]any{
		"request_id": req.Msg.RequestId,
		"withdrawn":  false, // Only include non-withdrawn offers
	}

	// If community context is provided, validate it's active and filter by it.
	if req.Msg.CommunityId != "" {
		if _, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.CommunityId); err != nil {
			return nil, err
		}
		queryFields["community_id"] = req.Msg.CommunityId
	}

	// Fetch all offers for this request
	offersProto, err := s.storage.QueryByFields(ctx, queryFields, &models.RequestOffer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query offers",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetRequestStats", fmt.Errorf("failed to query offers"))
	}

	// Count offers received
	offersReceived := int32(len(offersProto))

	// Calculate times fulfilled
	// A fulfilled request counts once; multiple fulfillments may be tracked in the future.
	var timesFulfilled int32
	if request.State == models.RequestState_REQUEST_STATE_FULFILLED {
		timesFulfilled = 1
	}

	// Calculate days open by finding the earliest shared_at time
	var earliestSharedAtUnixSec int64
	if req.Msg.CommunityId != "" {
		// Query for specific community
		communityRequestFields := map[string]any{
			"request_id":   req.Msg.RequestId,
			"community_id": req.Msg.CommunityId,
		}
		communityRequestsProto, err := s.storage.QueryByFields(ctx, communityRequestFields, &models.CommunityRequest{})
		if err == nil && len(communityRequestsProto) > 0 {
			communityRequest := communityRequestsProto[0].(*models.CommunityRequest)
			earliestSharedAtUnixSec = communityRequest.SharedAtUnixSec
		}
	} else {
		// Query all communities this request is shared with
		communityRequestsProto, err := s.storage.QueryByField(ctx, "request_id", req.Msg.RequestId, &models.CommunityRequest{})
		if err == nil && len(communityRequestsProto) > 0 {
			// Find the earliest shared_at time
			earliestSharedAtUnixSec = communityRequestsProto[0].(*models.CommunityRequest).SharedAtUnixSec
			for _, msg := range communityRequestsProto {
				communityRequest := msg.(*models.CommunityRequest)
				if communityRequest.SharedAtUnixSec > 0 && (earliestSharedAtUnixSec == 0 || communityRequest.SharedAtUnixSec < earliestSharedAtUnixSec) {
					earliestSharedAtUnixSec = communityRequest.SharedAtUnixSec
				}
			}
		}
	}

	// Calculate days open
	var daysOpen int32
	if earliestSharedAtUnixSec > 0 {
		now := clock.UnixSec(ctx)
		daysSinceShared := (now - earliestSharedAtUnixSec) / 86400
		if daysSinceShared < 0 {
			daysSinceShared = 0
		}
		daysOpen = int32(daysSinceShared)
	}

	// Extract valueUSD from stored request metadata.
	var valueUSD float32
	if request.ValueEstimate != nil {
		valueUSD = request.ValueEstimate.EstimatedValueUsd
	}

	// Build a hint from the stored impact estimate and social context so that
	// user-set attributes (tie strength, reciprocity, etc.) are reflected in
	// the stats response. UpdateRequest already stores the recalculated estimate,
	// so we prefer it directly; the hint is a fallback for vulnerability level.
	var storedHint *impact_metrics.QualityTimeHint
	if request.SocialContext != nil || request.ImpactEstimate != nil {
		storedHint = buildHintFromStoredRequest(request)
	}

	// Impact for fulfilled requests must come from the value persisted at
	// completion — that is the only place user overrides and per-input
	// provenance live. Recomputing here would silently drop overrides.
	// Fall back to a fresh build only if no impact was persisted.
	var totalImpact *api.ImpactEstimate
	if timesFulfilled > 0 {
		if request.ImpactEstimate != nil {
			totalImpact = impact_metrics.ModelsImpactToAPI(request.ImpactEstimate)
		} else if s.estimatorCfg != nil {
			totalImpact = impact_metrics.BuildRequestImpactMetrics(valueUSD, s.estimatorCfg, nil, storedHint)
		}
	}

	// Derive help_value_usd from the persisted ImpactEstimate when available; fall back to the
	// simple value × fulfillments formula for older records that predate impact persistence.
	var helpValueUSD float32
	if totalImpact != nil && totalImpact.MoneySaved != nil && totalImpact.MoneySaved.ValueUsd != nil {
		helpValueUSD = totalImpact.MoneySaved.ValueUsd.Mean
	} else if timesFulfilled > 0 {
		helpValueUSD = valueUSD * float32(timesFulfilled)
	}

	// Calculate per-fulfillment impact (always populated). Used by SharingImpactCard to show
	// what each individual fulfillment saves — independent of fulfillment state.
	// Prefer the already-computed stored estimate when available (it incorporates
	// the social context set by the owner).
	var potentialImpact *api.ImpactEstimate
	if request.ImpactEstimate != nil {
		potentialImpact = impact_metrics.ModelsImpactToAPI(request.ImpactEstimate)
	} else if s.estimatorCfg != nil {
		potentialImpact = impact_metrics.BuildRequestImpactMetrics(valueUSD, s.estimatorCfg, nil, storedHint)
	}

	logger.DebugContext(ctx, "calculated request stats",
		"offers_received", offersReceived,
		"times_fulfilled", timesFulfilled,
		"help_value_usd", helpValueUSD,
		"days_open", daysOpen,
	)

	return connect.NewResponse(&api.GetRequestStatsResponse{
		OffersReceived:  offersReceived,
		TimesFulfilled:  timesFulfilled,
		HelpValueUsd:    helpValueUSD,
		DaysOpen:        daysOpen,
		Impact:          totalImpact,
		PotentialImpact: potentialImpact,
	}), nil
}

// buildHintFromStoredRequest extracts a QualityTimeHint from the stored request.
// It uses the stored SocialContext (owner-set attributes) and falls back to the
// vulnerability level from the stored ImpactEstimate (LLM-inferred). Returns nil
// when neither is available.
func buildHintFromStoredRequest(req *models.Request) *impact_metrics.QualityTimeHint {
	hint := &impact_metrics.QualityTimeHint{}
	hasData := false

	if req.SocialContext != nil {
		hint.SocialContext = &api.SocialContext{
			TieStrength:   api.SocialTieStrength(req.SocialContext.TieStrength),
			Reciprocity:   api.SocialReciprocity(req.SocialContext.Reciprocity),
			Novelty:       api.SocialNovelty(req.SocialContext.Novelty),
			Vulnerability: api.SocialVulnerabilityLevel(req.SocialContext.Vulnerability),
			Modality:      api.SocialModality(req.SocialContext.Modality),
		}
		hasData = true
	}

	if req.ImpactEstimate != nil && hint.SocialContext == nil {
		// Use stored vulnerability level as fallback when no explicit social context is set.
		stored := impact_metrics.ModelsImpactToAPI(req.ImpactEstimate)
		if stored.QualityTime != nil && stored.QualityTime.Attributes != nil {
			hint.VulnerabilityLevel = vulnerabilityLevelToString(stored.QualityTime.Attributes.Vulnerability)
			if hint.VulnerabilityLevel != "" {
				hasData = true
			}
		}
	}

	if !hasData {
		return nil
	}
	return hint
}
