// provider_fallback.go implements a fallback provider that automatically retries
// requests with backup providers when the primary provider returns any error.
// Use this for high availability when provider outages would impact your service.
//
// Example:
//
//	fb, _ := NewFallbackProvider(openai, []Provider{anthropic, gemini})
//	// Tries OpenAI first, falls back to Anthropic then Gemini on any error
package ai

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// MethodFallbacks holds per-method fallback configurations.
type MethodFallbacks struct {
	DetectGearInImage             []Provider
	GenerateCommunityContent      []Provider
	GenerateRequestContent        []Provider
	GenerateRequestFromImage      []Provider
	GenerateExperienceFromText    []Provider
	GenerateExperienceFromImage   []Provider
	GenerateConversationSummary   []Provider
	ParseInformalTime             []Provider
	GenerateExperienceSuggestions []Provider
	InferSocialAttributes         []Provider
	ClassifyUnifiedCreate         []Provider
}

// byMethod flattens the overrides into a lookup keyed by Provider method name.
//
// This used to be a twelve-case switch that read the fields back one at a
// time, and the two drifted: one method had a field but no case, so a fallback
// chain configured for it was silently ignored. Listing each field exactly
// once, here, makes a missing entry visible next to the field it belongs to
// (#2816).
func (m *MethodFallbacks) byMethod() map[string][]Provider {
	if m == nil {
		return nil
	}
	return map[string][]Provider{
		"DetectGearInImage":             m.DetectGearInImage,
		"GenerateCommunityContent":      m.GenerateCommunityContent,
		"GenerateRequestContent":        m.GenerateRequestContent,
		"GenerateRequestFromImage":      m.GenerateRequestFromImage,
		"GenerateExperienceFromText":    m.GenerateExperienceFromText,
		"GenerateExperienceFromImage":   m.GenerateExperienceFromImage,
		"GenerateConversationSummary":   m.GenerateConversationSummary,
		"ParseInformalTime":             m.ParseInformalTime,
		"GenerateExperienceSuggestions": m.GenerateExperienceSuggestions,
		"InferSocialAttributes":         m.InferSocialAttributes,
		"ClassifyUnifiedCreate":         m.ClassifyUnifiedCreate,
	}
}

// FallbackProvider implements Provider by trying fallback providers on any error.
type FallbackProvider struct {
	primary   Provider
	fallbacks []Provider
	// fallbacksByMethod is the flattened per-method override table; nil when
	// no overrides are configured.
	fallbacksByMethod map[string][]Provider
}

// NewFallbackProvider creates a new fallback provider.
func NewFallbackProvider(primary Provider, fallbacks []Provider) (*FallbackProvider, error) {
	if primary == nil {
		return nil, fmt.Errorf("primary provider is required")
	}

	return &FallbackProvider{
		primary:   primary,
		fallbacks: fallbacks,
	}, nil
}

// Name returns the provider's name for logging.
func (p *FallbackProvider) Name() string {
	return "fallback"
}

// SetMethodFallbacks sets per-method fallback overrides.
func (p *FallbackProvider) SetMethodFallbacks(fallbacks *MethodFallbacks) {
	p.fallbacksByMethod = fallbacks.byMethod()
}

// getFallbacksForMethod returns the fallback chain for a method, or the
// provider's default chain when none is configured.
func (p *FallbackProvider) getFallbacksForMethod(method string) []Provider {
	if chain := p.fallbacksByMethod[method]; len(chain) > 0 {
		return chain
	}
	return p.fallbacks
}

// withFallback executes a provider call with fallback behavior.
// On any error from the primary provider, it tries each fallback in order.
func withFallback[T any](
	ctx context.Context,
	methodName string,
	primaryCall func() (T, error),
	fallbacks []Provider,
	fallbackCall func(Provider) (T, error),
) (T, error) {
	logger := logging.LoggerWithContext(ctx).With("method_name", methodName)

	logger.Debug("trying primary provider")
	result, err := primaryCall()
	if err == nil {
		logger.Info("primary provider succeeded")
		return result, nil
	}

	logger.Warn("primary provider failed, trying fallbacks", "error", err)
	for _, fallback := range fallbacks {
		logger.Debug("trying fallback provider", "provider", fallback.Name())
		result, err = fallbackCall(fallback)
		if err == nil {
			logger.Info("fallback provider succeeded", "provider", fallback.Name())
			return result, nil
		}
		logger.Warn("fallback provider failed", "provider", fallback.Name(), "error", err)
	}

	logger.Error("all providers failed", "error", err)
	return result, err
}

// DetectGearInImage tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) DetectGearInImage(ctx context.Context, req *DetectionImage) (*GearDetection, error) {
	return withFallback(
		ctx,
		"DetectGearInImage",
		func() (*GearDetection, error) { return p.primary.DetectGearInImage(ctx, req) },
		p.getFallbacksForMethod("DetectGearInImage"),
		func(fb Provider) (*GearDetection, error) { return fb.DetectGearInImage(ctx, req) },
	)
}

// GenerateGearFromText tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateGearFromText(ctx context.Context, prompt, region string) (*GearGeneration, error) {
	return withFallback(
		ctx,
		"GenerateGearFromText",
		func() (*GearGeneration, error) { return p.primary.GenerateGearFromText(ctx, prompt, region) },
		p.getFallbacksForMethod("GenerateGearFromText"),
		func(fb Provider) (*GearGeneration, error) { return fb.GenerateGearFromText(ctx, prompt, region) },
	)
}

// GenerateGearFromWebpage tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateGearFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (*GearGeneration, error) {
	return withFallback(
		ctx,
		"GenerateGearFromWebpage",
		func() (*GearGeneration, error) {
			return p.primary.GenerateGearFromWebpage(ctx, pageTitle, pageDescription, pageBody, region)
		},
		p.getFallbacksForMethod("GenerateGearFromWebpage"),
		func(fb Provider) (*GearGeneration, error) {
			return fb.GenerateGearFromWebpage(ctx, pageTitle, pageDescription, pageBody, region)
		},
	)
}

// GenerateCommunityContent tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateCommunityContent(ctx context.Context, prompt, region string) (*CommunityGeneration, error) {
	return withFallback(
		ctx,
		"GenerateCommunityContent",
		func() (*CommunityGeneration, error) { return p.primary.GenerateCommunityContent(ctx, prompt, region) },
		p.getFallbacksForMethod("GenerateCommunityContent"),
		func(fb Provider) (*CommunityGeneration, error) {
			return fb.GenerateCommunityContent(ctx, prompt, region)
		},
	)
}

// GenerateRequestContent tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateRequestContent(ctx context.Context, prompt, region string) (*RequestGeneration, error) {
	return withFallback(
		ctx,
		"GenerateRequestContent",
		func() (*RequestGeneration, error) { return p.primary.GenerateRequestContent(ctx, prompt, region) },
		p.getFallbacksForMethod("GenerateRequestContent"),
		func(fb Provider) (*RequestGeneration, error) { return fb.GenerateRequestContent(ctx, prompt, region) },
	)
}

// GenerateExperienceFromText tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateExperienceFromText(ctx context.Context, prompt, region, currentTime string) (*ExperienceGeneration, error) {
	return withFallback(
		ctx,
		"GenerateExperienceFromText",
		func() (*ExperienceGeneration, error) {
			return p.primary.GenerateExperienceFromText(ctx, prompt, region, currentTime)
		},
		p.getFallbacksForMethod("GenerateExperienceFromText"),
		func(fb Provider) (*ExperienceGeneration, error) {
			return fb.GenerateExperienceFromText(ctx, prompt, region, currentTime)
		},
	)
}

// GenerateExperienceFromImage tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateExperienceFromImage(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (*ExperienceGeneration, error) {
	return withFallback(
		ctx,
		"GenerateExperienceFromImage",
		func() (*ExperienceGeneration, error) {
			return p.primary.GenerateExperienceFromImage(ctx, image, region, notes, currentTime)
		},
		p.getFallbacksForMethod("GenerateExperienceFromImage"),
		func(fb Provider) (*ExperienceGeneration, error) {
			return fb.GenerateExperienceFromImage(ctx, image, region, notes, currentTime)
		},
	)
}

// GenerateExperienceFromWebpage tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateExperienceFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (*ExperienceGeneration, error) {
	return withFallback(
		ctx,
		"GenerateExperienceFromWebpage",
		func() (*ExperienceGeneration, error) {
			return p.primary.GenerateExperienceFromWebpage(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
		},
		p.getFallbacksForMethod("GenerateExperienceFromWebpage"),
		func(fb Provider) (*ExperienceGeneration, error) {
			return fb.GenerateExperienceFromWebpage(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
		},
	)
}

// GenerateRequestFromImage tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateRequestFromImage(ctx context.Context, image *DetectionImage, region string) (*RequestGeneration, error) {
	return withFallback(
		ctx,
		"GenerateRequestFromImage",
		func() (*RequestGeneration, error) {
			return p.primary.GenerateRequestFromImage(ctx, image, region)
		},
		p.getFallbacksForMethod("GenerateRequestFromImage"),
		func(fb Provider) (*RequestGeneration, error) {
			return fb.GenerateRequestFromImage(ctx, image, region)
		},
	)
}

// GenerateConversationSummary tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateConversationSummary(ctx context.Context, messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) (*ConversationSummary, error) {
	return withFallback(
		ctx,
		"GenerateConversationSummary",
		func() (*ConversationSummary, error) {
			return p.primary.GenerateConversationSummary(ctx, messages, requestTitle, requestDescription, style)
		},
		p.getFallbacksForMethod("GenerateConversationSummary"),
		func(fb Provider) (*ConversationSummary, error) {
			return fb.GenerateConversationSummary(ctx, messages, requestTitle, requestDescription, style)
		},
	)
}

// GenerateExperienceCompletionSummary tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateExperienceCompletionSummary(ctx context.Context, messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) (*ConversationSummary, error) {
	return withFallback(
		ctx,
		"GenerateExperienceCompletionSummary",
		func() (*ConversationSummary, error) {
			return p.primary.GenerateExperienceCompletionSummary(ctx, messages, experienceTitle, experienceDescription, attendeeNames)
		},
		p.getFallbacksForMethod("GenerateExperienceCompletionSummary"),
		func(fb Provider) (*ConversationSummary, error) {
			return fb.GenerateExperienceCompletionSummary(ctx, messages, experienceTitle, experienceDescription, attendeeNames)
		},
	)
}

// ParseInformalTime tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) ParseInformalTime(ctx context.Context, informalDescription, currentTime, timezone string) (string, error) {
	return withFallback(
		ctx,
		"ParseInformalTime",
		func() (string, error) {
			return p.primary.ParseInformalTime(ctx, informalDescription, currentTime, timezone)
		},
		p.getFallbacksForMethod("ParseInformalTime"),
		func(fb Provider) (string, error) {
			return fb.ParseInformalTime(ctx, informalDescription, currentTime, timezone)
		},
	)
}

// GenerateExperienceSuggestions tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateExperienceSuggestions(ctx context.Context, name, description, category string) (*ExperienceSuggestionResult, error) {
	return withFallback(
		ctx,
		"GenerateExperienceSuggestions",
		func() (*ExperienceSuggestionResult, error) {
			return p.primary.GenerateExperienceSuggestions(ctx, name, description, category)
		},
		p.getFallbacksForMethod("GenerateExperienceSuggestions"),
		func(fb Provider) (*ExperienceSuggestionResult, error) {
			return fb.GenerateExperienceSuggestions(ctx, name, description, category)
		},
	)
}

// GenerateRequestSuggestions tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) GenerateRequestSuggestions(ctx context.Context, title, description, location string) (*RequestSuggestionResult, error) {
	return withFallback(
		ctx,
		"GenerateRequestSuggestions",
		func() (*RequestSuggestionResult, error) {
			return p.primary.GenerateRequestSuggestions(ctx, title, description, location)
		},
		p.getFallbacksForMethod("GenerateRequestSuggestions"),
		func(fb Provider) (*RequestSuggestionResult, error) {
			return fb.GenerateRequestSuggestions(ctx, title, description, location)
		},
	)
}

// InferSocialAttributes tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) InferSocialAttributes(ctx context.Context, title, description, txType string, itemValueUSD float32) (*SocialAttributeInference, error) {
	return withFallback(
		ctx,
		"InferSocialAttributes",
		func() (*SocialAttributeInference, error) {
			return p.primary.InferSocialAttributes(ctx, title, description, txType, itemValueUSD)
		},
		p.getFallbacksForMethod("InferSocialAttributes"),
		func(fb Provider) (*SocialAttributeInference, error) {
			return fb.InferSocialAttributes(ctx, title, description, txType, itemValueUSD)
		},
	)
}

// ClassifyUnifiedCreate tries the primary provider, then fallbacks on any error.
func (p *FallbackProvider) ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
	return withFallback(
		ctx,
		"ClassifyUnifiedCreate",
		func() (*UnifiedCreateClassification, error) { return p.primary.ClassifyUnifiedCreate(ctx, in) },
		p.getFallbacksForMethod("ClassifyUnifiedCreate"),
		func(fb Provider) (*UnifiedCreateClassification, error) { return fb.ClassifyUnifiedCreate(ctx, in) },
	)
}

// CheckHealth returns a single composite status reflecting whether the AI fallback
// chain can serve requests. The composite is healthy when the primary or any fallback
// is healthy — matching the at-least-one-healthy guarantee provided by the fallback
// request path. Unhealthy individual leaves emit WARN-level "dependency_leaf_unhealthy"
// logs without triggering the health-check alert metric (which fires only on
// "dependency_health_check_failed" messages at ERROR severity).
func (p *FallbackProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	logger := logging.LoggerWithContext(ctx)

	// Check primary provider.
	primaryStatuses, err := p.primary.CheckHealth(ctx)
	if err != nil {
		return nil, err
	}

	var totalLatencyMs int64
	primaryHealthy := true
	var primaryBackend, primaryError string

	for _, s := range primaryStatuses {
		totalLatencyMs += s.LatencyMs
		if !s.IsHealthy() {
			primaryHealthy = false
			if primaryError == "" {
				primaryError = s.Error
			}
		}
		if primaryBackend == "" {
			primaryBackend = s.Backend
		}
	}

	// Emit WARN per unhealthy primary leaf (WARN level keeps this out of the
	// all_errors metric which filters severity >= ERROR).
	for _, s := range primaryStatuses {
		if !s.IsHealthy() {
			logger.WarnContext(ctx, "dependency_leaf_unhealthy",
				"dependency", "ai",
				"backend", s.Backend,
				"error", s.Error)
		}
	}

	// Check fallback providers sequentially (mirrors the request-path retry order).
	var fallbackBackends []string
	var healthyFallbackCount int
	var firstFallbackError string

	for _, fb := range p.fallbacks {
		fbStatuses, err := fb.CheckHealth(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range fbStatuses {
			totalLatencyMs += s.LatencyMs
			fallbackBackends = append(fallbackBackends, s.Backend)
			if s.IsHealthy() {
				healthyFallbackCount++
			} else {
				if firstFallbackError == "" {
					firstFallbackError = s.Error
				}
				logger.WarnContext(ctx, "dependency_leaf_unhealthy",
					"dependency", "ai",
					"backend", s.Backend,
					"error", s.Error)
			}
		}
	}

	// Composite error is non-empty only when ALL leaves are unhealthy, which causes
	// Service.CheckHealth to emit "dependency_health_check_failed" at ERROR and
	// trip the Cloud Monitoring alert. A single healthy leaf suppresses the alert.
	compositeError := ""
	if !primaryHealthy && healthyFallbackCount == 0 {
		if primaryError != "" {
			compositeError = primaryError
		} else if firstFallbackError != "" {
			compositeError = firstFallbackError
		} else {
			compositeError = "all providers unhealthy"
		}
	}

	metadata := map[string]string{
		"primary_backend":        primaryBackend,
		"primary_healthy":        strconv.FormatBool(primaryHealthy),
		"fallback_backends":      strings.Join(fallbackBackends, ","),
		"healthy_fallback_count": strconv.Itoa(healthyFallbackCount),
	}
	if primaryError != "" {
		metadata["primary_error"] = primaryError
	}
	if firstFallbackError != "" {
		metadata["first_fallback_error"] = firstFallbackError
	}

	return []*health.Status{{
		Name:      "ai",
		Backend:   primaryBackend + "+fallback",
		LatencyMs: totalLatencyMs,
		Metadata:  metadata,
		Error:     compositeError,
	}}, nil
}
