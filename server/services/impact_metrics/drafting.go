package impact_metrics

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
)

// draftImpactEstimate handles the DraftImpactEstimate RPC.
// It retrieves the target experience or request, runs LLM inference over its
// chat transcript and planning contributions, and returns a fully-stamped
// ImpactEstimate without persisting anything.
func draftImpactEstimate(
	ctx context.Context,
	s *Service,
	req *connect.Request[api.DraftImpactEstimateRequest],
) (*connect.Response[api.DraftImpactEstimateResponse], error) {
	logger := logging.LoggerWithContext(ctx).With("operation", "DraftImpactEstimate")

	switch t := req.Msg.Target.(type) {
	case *api.DraftImpactEstimateRequest_ExperienceId:
		return draftExperienceEstimate(ctx, s, t.ExperienceId, logger)
	case *api.DraftImpactEstimateRequest_RequestId:
		return draftRequestEstimate(ctx, s, t.RequestId, logger)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("one of experience_id or request_id must be set"))
	}
}

// draftImpactEstimateWithOverrides handles the DraftImpactEstimateWithOverrides RPC.
// It does not re-call the LLM. Instead it takes the provided input overrides,
// recomputes the composite values deterministically, and returns the updated estimate.
func draftImpactEstimateWithOverrides(
	ctx context.Context,
	s *Service,
	req *connect.Request[api.DraftImpactEstimateWithOverridesRequest],
) (*connect.Response[api.DraftImpactEstimateWithOverridesResponse], error) {
	logger := logging.LoggerWithContext(ctx).With("operation", "DraftImpactEstimateWithOverrides")

	var experienceID, requestID string
	switch t := req.Msg.Target.(type) {
	case *api.DraftImpactEstimateWithOverridesRequest_ExperienceId:
		experienceID = t.ExperienceId
	case *api.DraftImpactEstimateWithOverridesRequest_RequestId:
		requestID = t.RequestId
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("one of experience_id or request_id must be set"))
	}

	// Authorize based on which target is set.
	if experienceID != "" {
		if err := authorizeExperienceDraft(ctx, experienceID, s.storage); err != nil {
			return nil, err
		}
	} else {
		if err := authorizeRequestDraft(ctx, requestID, s.storage); err != nil {
			return nil, err
		}
	}

	// Build a baseline estimate from the stored record, then apply overrides.
	var baseEstimate *api.ImpactEstimate
	var attendeeCount int32 = 2

	if experienceID != "" {
		exp := &models.Experience{}
		if err := s.storage.GetByID(ctx, experienceID, exp); err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("experience not found"))
		}
		baseEstimate = buildExperienceBaseEstimate(ctx, s, exp, logger)
		attendeeCount = countExperienceAttendees(ctx, s, experienceID, logger)
	} else {
		req2 := &models.Request{}
		if err := s.storage.GetByID(ctx, requestID, req2); err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("request not found"))
		}
		baseEstimate = buildRequestBaseEstimate(s, req2)
	}

	overrides := &impact_metrics.ImpactOverrides{}
	if req.Msg.QualityTimeInput != nil {
		overrides.QualityTime = req.Msg.QualityTimeInput
		// Propagate attendee count into the attributes when group_size is not overridden.
		if overrides.QualityTime.GroupSize == 0 {
			overrides.QualityTime.GroupSize = attendeeCount
		}
	}
	if req.Msg.MoneySavingsInput != nil {
		overrides.MoneySavings = req.Msg.MoneySavingsInput
	}
	if req.Msg.EmissionsInput != nil {
		overrides.Emissions = req.Msg.EmissionsInput
	}

	result, err := impact_metrics.ApplyImpactOverrides(baseEstimate, overrides, s.estimatorCfg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	return connect.NewResponse(&api.DraftImpactEstimateWithOverridesResponse{
		Impact: result,
	}), nil
}

// draftExperienceEstimate builds an LLM-enriched draft for an experience.
func draftExperienceEstimate(
	ctx context.Context,
	s *Service,
	experienceID string,
	logger *logging.Logger,
) (*connect.Response[api.DraftImpactEstimateResponse], error) {
	if err := authorizeExperienceDraft(ctx, experienceID, s.storage); err != nil {
		return nil, err
	}

	exp := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, exp); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("experience not found"))
	}

	attendeeCount := countExperienceAttendees(ctx, s, experienceID, logger)

	var valueUSD float32
	if exp.ValueEstimate != nil {
		valueUSD = exp.ValueEstimate.EstimatedValueUsd
	}

	// Run LLM social-attribute inference when available.
	var hint *impact_metrics.QualityTimeHint

	if s.aiProvider != nil {
		maxMessages := int(s.estimatorCfg.GetQualityTime().GetExperienceChatTranscriptMaxMessages())
		if maxMessages <= 0 {
			maxMessages = 50
		}
		transcript := loadExperienceTranscript(ctx, s, exp, maxMessages, logger)
		inference, inferErr := ai.CallWithTimeout(ctx, 30*time.Second, func(ctx context.Context) (*ai.SocialAttributeInference, error) {
			return s.aiProvider.InferSocialAttributes(ctx, exp.Name, buildTranscriptContext(exp.Description, transcript), "experience", valueUSD)
		})
		if inferErr != nil {
			if errors.Is(inferErr, context.DeadlineExceeded) {
				logger.WarnContext(ctx, "LLM call exceeded timeout, using config defaults",
					"external_service", "ai_provider",
					"operation", "InferSocialAttributes",
					"error", inferErr)
			} else {
				logger.WarnContext(ctx, "LLM inference failed, using config defaults", "error", inferErr)
			}
		} else if inference != nil {
			hint = &impact_metrics.QualityTimeHint{
				DurationMinutes:    inference.DurationMinutes,
				VulnerabilityLevel: inference.VulnerabilityLevel,
			}
		}
	}

	// User-set duration takes priority over LLM inference.
	if userDuration := extractExperienceDurationMinutes(exp); userDuration > 0 {
		if hint == nil {
			hint = &impact_metrics.QualityTimeHint{}
		}
		hint.UserDurationMinutes = userDuration
	}

	contributions := loadContributions(ctx, s, "experience_id", experienceID, logger)

	// Derive secondary signals from contributions and attendee data.
	if hint == nil {
		hint = &impact_metrics.QualityTimeHint{}
	}
	applyQualityTimeHints(hint, contributions)

	// Cross-community attendees: check if RSVPs span multiple communities.
	hint.HasCrossCommunityAttendees = hasCrossCommunityAttendees(ctx, s, experienceID, logger)

	ie := impact_metrics.BuildExperienceImpactMetrics(valueUSD, attendeeCount, s.estimatorCfg, nil, hint)
	ie = applyContributionEstimates(ie, contributions, valueUSD, estimator.TransactionExperienceConcluded, s.estimatorCfg)

	return connect.NewResponse(&api.DraftImpactEstimateResponse{
		Impact: ie,
	}), nil
}

// draftRequestEstimate builds an LLM-enriched draft for a request.
func draftRequestEstimate(
	ctx context.Context,
	s *Service,
	requestID string,
	logger *logging.Logger,
) (*connect.Response[api.DraftImpactEstimateResponse], error) {
	if err := authorizeRequestDraft(ctx, requestID, s.storage); err != nil {
		return nil, err
	}

	req := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, req); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("request not found"))
	}

	var valueUSD float32
	if req.ValueEstimate != nil {
		valueUSD = req.ValueEstimate.EstimatedValueUsd
	}

	var hint *impact_metrics.QualityTimeHint

	if s.aiProvider != nil {
		inference, inferErr := ai.CallWithTimeout(ctx, 30*time.Second, func(ctx context.Context) (*ai.SocialAttributeInference, error) {
			return s.aiProvider.InferSocialAttributes(ctx, req.Title, req.Description, "request", valueUSD)
		})
		if inferErr != nil {
			if errors.Is(inferErr, context.DeadlineExceeded) {
				logger.WarnContext(ctx, "LLM call exceeded timeout, using config defaults",
					"external_service", "ai_provider",
					"operation", "InferSocialAttributes",
					"error", inferErr)
			} else {
				logger.WarnContext(ctx, "LLM inference failed, using config defaults", "error", inferErr)
			}
		} else if inference != nil {
			hint = &impact_metrics.QualityTimeHint{
				DurationMinutes:    inference.DurationMinutes,
				VulnerabilityLevel: inference.VulnerabilityLevel,
			}
		}
	}

	contributions := loadContributions(ctx, s, "request_id", requestID, logger)
	ie := impact_metrics.BuildRequestImpactMetrics(valueUSD, s.estimatorCfg, nil, hint)
	ie = applyContributionEstimates(ie, contributions, valueUSD, estimator.TransactionRequestFulfilled, s.estimatorCfg)

	return connect.NewResponse(&api.DraftImpactEstimateResponse{
		Impact: ie,
	}), nil
}

// buildExperienceBaseEstimate computes a config-default impact estimate for an experience.
// Used as the starting point before applying caller-supplied overrides.
func buildExperienceBaseEstimate(ctx context.Context, s *Service, exp *models.Experience, logger *logging.Logger) *api.ImpactEstimate {
	var valueUSD float32
	if exp.ValueEstimate != nil {
		valueUSD = exp.ValueEstimate.EstimatedValueUsd
	}
	attendeeCount := countExperienceAttendees(ctx, s, exp.Id, logger)
	return impact_metrics.BuildExperienceImpactMetrics(valueUSD, attendeeCount, s.estimatorCfg, nil, nil)
}

// buildRequestBaseEstimate computes a config-default impact estimate for a request.
func buildRequestBaseEstimate(s *Service, req *models.Request) *api.ImpactEstimate {
	var valueUSD float32
	if req.ValueEstimate != nil {
		valueUSD = req.ValueEstimate.EstimatedValueUsd
	}
	return impact_metrics.BuildRequestImpactMetrics(valueUSD, s.estimatorCfg, nil, nil)
}

// countExperienceAttendees returns the number of confirmed attendees for an experience.
// Returns 2 (the minimum dyadic count) if no attendees are found.
func countExperienceAttendees(ctx context.Context, s *Service, experienceID string, logger *logging.Logger) int32 {
	rsvps, err := s.storage.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceRSVP{})
	if err != nil {
		logger.WarnContext(ctx, "failed to count attendees", "experience_id", experienceID, "error", err)
		return 2
	}
	count := int32(0)
	for _, r := range rsvps {
		if r.(*models.ExperienceRSVP).GetIntention() == models.RSVPIntention_RSVP_INTENTION_YES {
			count++
		}
	}
	if count < 2 {
		return 2
	}
	return count
}

// loadExperienceTranscript loads up to maxMessages user chat messages from the experience conversation.
// Messages are sorted by sent_at_unix_sec ascending and the most recent maxMessages are returned.
func loadExperienceTranscript(ctx context.Context, s *Service, exp *models.Experience, maxMessages int, logger *logging.Logger) []ai.ConversationMessage {
	conversationID := exp.ConversationId
	if conversationID == "" {
		return nil
	}

	msgs, err := s.storage.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		logger.WarnContext(ctx, "failed to load transcript", "conversation_id", conversationID, "error", err)
		return nil
	}

	// Collect user messages only.
	type timestampedMsg struct {
		msg       *models.ChatMessage
		sentAtSec int64
	}
	var userMsgs []timestampedMsg
	for _, m := range msgs {
		cm := m.(*models.ChatMessage)
		if cm.GetUserMessage() != nil {
			userMsgs = append(userMsgs, timestampedMsg{cm, cm.SentAtUnixSec})
		}
	}

	// Sort ascending and keep the most recent maxMessages.
	sort.Slice(userMsgs, func(i, j int) bool {
		return userMsgs[i].sentAtSec < userMsgs[j].sentAtSec
	})
	if len(userMsgs) > maxMessages {
		userMsgs = userMsgs[len(userMsgs)-maxMessages:]
	}

	result := make([]ai.ConversationMessage, 0, len(userMsgs))
	for _, tm := range userMsgs {
		um := tm.msg.GetUserMessage()
		result = append(result, ai.ConversationMessage{
			SenderName:    um.SenderId, // Use ID as fallback; name lookup would be N+1.
			Text:          um.Text,
			SentAtUnixSec: tm.sentAtSec,
		})
	}
	return result
}

// buildTranscriptContext combines a description and transcript messages into a single context string
// for the AI inference call.
func buildTranscriptContext(description string, messages []ai.ConversationMessage) string {
	if len(messages) == 0 {
		return description
	}
	combined := description
	for _, m := range messages {
		if m.Text != "" {
			combined += "\n" + m.Text
		}
	}
	return combined
}

// extractExperienceDurationMinutes returns the host-set duration from an experience's time field.
// Returns 0 if no duration is set.
func extractExperienceDurationMinutes(exp *models.Experience) float32 {
	if exp.Time == nil {
		return 0
	}
	switch t := exp.Time.TimeType.(type) {
	case *models.ExperienceTime_Specific:
		return float32(t.Specific.DurationMinutes)
	case *models.ExperienceTime_Range:
		return float32(t.Range.DurationMinutes)
	default:
		return 0
	}
}

// loadContributions returns active (non-deleted) PlanningContributions for a target.
// field is "experience_id" or "request_id"; id is the target's UUID.
func loadContributions(ctx context.Context, s *Service, field, id string, logger *logging.Logger) []*models.PlanningContribution {
	records, err := s.storage.QueryByField(ctx, field, id, &models.PlanningContribution{})
	if err != nil {
		logger.WarnContext(ctx, "failed to load contributions", field, id, "error", err)
		return nil
	}
	active := make([]*models.PlanningContribution, 0, len(records))
	for _, r := range records {
		c := r.(*models.PlanningContribution)
		if c.Deleted == nil {
			active = append(active, c)
		}
	}
	return active
}

// applyContributionEstimates enriches a draft ImpactEstimate with contribution-aware
// money savings and emissions derived from planning contributions.
// Each active contribution is treated as a shared-item interaction with a co-use discount.
// Returns base unchanged when contributions is empty or valueUSD is zero.
func applyContributionEstimates(
	base *api.ImpactEstimate,
	contributions []*models.PlanningContribution,
	valueUSD float32,
	txType estimator.TransactionType,
	cfg *estimator.Config,
) *api.ImpactEstimate {
	if len(contributions) == 0 || valueUSD <= 0 {
		return base
	}

	coUseRate := cfg.QualityTime.GetExperienceContributionCoUseRate()
	if txType == estimator.TransactionRequestFulfilled {
		coUseRate = cfg.QualityTime.GetRequestContributionCoUseRate()
	}
	if coUseRate <= 0 {
		coUseRate = 0.35
	}

	n := float32(len(contributions))
	perValue := valueUSD / n
	relStddev := cfg.Global.GetMoneySavedRelativeStddev()

	// Build per-contribution money and emissions inputs.
	moneyInputs := make([]*api.ContributionValueInput, 0, len(contributions))
	emissionsInputs := make([]*api.ContributionEmissionsInput, 0, len(contributions))

	factors := cfg.GetSpendBasedEmissionFactors()
	spendFactor := float32(0.10)
	if f, ok := factors["general_consumer_goods"]; ok {
		spendFactor = f
	}

	for _, c := range contributions {
		savedVal := estimator.RelativeUncertainty(perValue*coUseRate, relStddev)
		moneyInputs = append(moneyInputs, &api.ContributionValueInput{
			PlanningContributionId: c.Id,
			ValueUsd:               savedVal,
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
				Name:    "service_value",
				Version: cfg.ProvenanceVersion("service_value"),
			},
		})

		carbonGrams := perValue * spendFactor * 1000 * coUseRate
		carbonEst := estimator.RelativeUncertainty(carbonGrams, cfg.MethodStddev("spend_based_v1"))
		emissionsInputs = append(emissionsInputs, &api.ContributionEmissionsInput{
			PlanningContributionId: c.Id,
			EmbodiedCarbon:         carbonEst,
			EmbodiedCarbonProvenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
				Name:    estimator.ProvenanceSpendBasedCarbon,
				Version: cfg.ProvenanceVersion(estimator.ProvenanceSpendBasedCarbon),
			},
		})
	}

	// Sum composite values from per-contribution inputs.
	moneyParts := make([]*api.Estimate, 0, len(moneyInputs))
	for _, mi := range moneyInputs {
		if mi.ValueUsd != nil {
			moneyParts = append(moneyParts, mi.ValueUsd)
		}
	}
	carbonParts := make([]*api.Estimate, 0, len(emissionsInputs))
	for _, ei := range emissionsInputs {
		if ei.EmbodiedCarbon != nil {
			carbonParts = append(carbonParts, ei.EmbodiedCarbon)
		}
	}

	provName := "service_value"
	moneySaved := &api.MoneySavings{
		ValueUsd: estimator.SumEstimates(moneyParts),
		Inputs:   &api.MoneySavingsInput{ContributionInputs: moneyInputs},
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
			Name:      provName,
			Version:   cfg.ProvenanceVersion(provName),
			Reasoning: proto.String(fmt.Sprintf("%d contributions × $%.2f avg × %.0f%% co-use rate", len(contributions), perValue, coUseRate*100)),
			Sources:   []string{"Community labor valuation"},
		},
	}

	emProvName := estimator.ProvenanceSpendBasedCarbon
	emissions := &api.PreventedEmissions{
		ManufactureAvoidedCarbon: &api.CarbonEstimate{
			Co2EGrams: estimator.SumEstimates(carbonParts),
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
				Name:    emProvName,
				Version: cfg.ProvenanceVersion(emProvName),
			},
		},
		Inputs: &api.PreventedEmissionsInput{ContributionInputs: emissionsInputs},
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
			Name:      emProvName,
			Version:   cfg.ProvenanceVersion(emProvName),
			Reasoning: proto.String(fmt.Sprintf("%d contributions spend-based CO2, %.0f%% co-use rate", len(contributions), coUseRate*100)),
			Sources:   []string{"Spend-based EEIO emission factors"},
		},
	}

	result := proto.Clone(base).(*api.ImpactEstimate)
	result.MoneySaved = moneySaved
	result.EmissionsPrevented = emissions
	return result
}

// applyPhase3Hints sets effort-asymmetry and multi-host signals on hint based on
// the contribution list and total attendee count.
// Distinct contributor count drives both signals because PlanningContribution has
// no hosting-type field — any contributor is treated as a potential co-host.
func applyQualityTimeHints(hint *impact_metrics.QualityTimeHint, contributions []*models.PlanningContribution) {
	if len(contributions) == 0 {
		return
	}
	contributorIDs := make(map[string]struct{}, len(contributions))
	for _, c := range contributions {
		if c.ContributorId != "" {
			contributorIDs[c.ContributorId] = struct{}{}
		}
	}
	distinctContributors := len(contributorIDs)
	if distinctContributors == 0 {
		return
	}

	hint.EffortContributorCount = distinctContributors

	// Multi-host: more than one distinct contributor signals distributed hosting.
	if distinctContributors >= 2 {
		hint.IsMultiHost = true
	}
}

// hasCrossCommunityAttendees returns true when the experience's RSVP attendees
// are members of more than one distinct community.
func hasCrossCommunityAttendees(ctx context.Context, s *Service, experienceID string, logger *logging.Logger) bool {
	rsvps, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": experienceID,
		"attended":      int32(models.AttendedStatus_ATTENDED_STATUS_YES),
	}, &models.ExperienceRSVP{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query RSVPs for cross-community check",
			"experience_id", experienceID, "error", err)
		return false
	}

	communityIDs := make(map[string]struct{})
	for _, r := range rsvps {
		if rsvp := r.(*models.ExperienceRSVP); rsvp.CommunityId != "" {
			communityIDs[rsvp.CommunityId] = struct{}{}
		}
	}
	return len(communityIDs) > 1
}
