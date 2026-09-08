// Package ai provides AI provider implementations and decorators.
//
// provider_loadbalanced.go implements a load-balanced provider that distributes
// requests across multiple underlying providers using weighted random selection.
// Use this to spread load across providers or to A/B test different models.
//
// Example:
//
//	lb, _ := NewLoadBalancedProvider([]WeightedProvider{
//	    {Provider: openai, Name: "openai", Weight: 0.7},
//	    {Provider: anthropic, Name: "anthropic", Weight: 0.3},
//	})
//	// 70% of requests go to OpenAI, 30% to Anthropic
package ai

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// WeightedProvider pairs a provider with its selection weight.
type WeightedProvider struct {
	Provider Provider
	Name     string
	Weight   float64
}

// MethodWeights holds per-method weight configurations.
type MethodWeights struct {
	DetectGearInImage             []WeightedProvider
	GenerateCommunityContent      []WeightedProvider
	GenerateRequestContent        []WeightedProvider
	GenerateRequestFromImage      []WeightedProvider
	GenerateExperienceFromText    []WeightedProvider
	GenerateExperienceFromImage   []WeightedProvider
	GenerateConversationSummary   []WeightedProvider
	ParseInformalTime             []WeightedProvider
	GenerateExperienceSuggestions []WeightedProvider
	InferSocialAttributes         []WeightedProvider
	ClassifyUnifiedCreate         []WeightedProvider
}

// byMethod flattens the overrides into a lookup keyed by Provider method name.
// See MethodFallbacks.byMethod: a switch that read these fields back one at a
// time is how the fallback table came to silently ignore one of its own
// fields (#2816).
func (m *MethodWeights) byMethod() map[string][]WeightedProvider {
	if m == nil {
		return nil
	}
	return map[string][]WeightedProvider{
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

// LoadBalancedProvider implements Provider by randomly selecting from weighted providers.
type LoadBalancedProvider struct {
	providers []WeightedProvider
	// weightsByMethod is the flattened per-method override table; nil when no
	// overrides are configured.
	weightsByMethod map[string][]WeightedProvider
}

// NewLoadBalancedProvider creates a new load-balanced provider.
// Weights should sum to 1.0 but will be normalized if they don't.
func NewLoadBalancedProvider(providers []WeightedProvider) (*LoadBalancedProvider, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}

	// Normalize weights
	normalized := normalizeWeights(providers)

	return &LoadBalancedProvider{
		providers: normalized,
	}, nil
}

// Name returns the provider's name for logging.
func (p *LoadBalancedProvider) Name() string {
	return "load-balanced"
}

// SetMethodWeights sets per-method weight overrides.
// If set, these weights will be used instead of the default weights for specific methods.
func (p *LoadBalancedProvider) SetMethodWeights(weights *MethodWeights) {
	p.weightsByMethod = weights.byMethod()
}

// normalizeWeights ensures weights sum to 1.0.
func normalizeWeights(providers []WeightedProvider) []WeightedProvider {
	total := 0.0
	for _, wp := range providers {
		total += wp.Weight
	}

	if total == 0 {
		// Equal weights if all are zero
		equal := 1.0 / float64(len(providers))
		normalized := make([]WeightedProvider, len(providers))
		for i, wp := range providers {
			normalized[i] = WeightedProvider{
				Provider: wp.Provider,
				Name:     wp.Name,
				Weight:   equal,
			}
		}
		return normalized
	}

	normalized := make([]WeightedProvider, len(providers))
	for i, wp := range providers {
		normalized[i] = WeightedProvider{
			Provider: wp.Provider,
			Name:     wp.Name,
			Weight:   wp.Weight / total,
		}
	}
	return normalized
}

// selectProvider randomly selects a provider based on weights.
func selectProvider(providers []WeightedProvider) (Provider, string) {
	//nolint:gosec // G404: picks which AI provider handles a request by weight.
	// Nothing secret depends on it being unpredictable.
	r := rand.Float64()
	cumulative := 0.0
	for _, wp := range providers {
		cumulative += wp.Weight
		if r < cumulative {
			return wp.Provider, wp.Name
		}
	}
	// Fallback to last provider (shouldn't happen with normalized weights)
	last := providers[len(providers)-1]
	return last.Provider, last.Name
}

// getProvidersForMethod returns the weighted providers for a method, or the
// provider's default set when none is configured.
func (p *LoadBalancedProvider) getProvidersForMethod(method string) []WeightedProvider {
	if weighted := p.weightsByMethod[method]; len(weighted) > 0 {
		return weighted
	}
	return p.providers
}

// DetectGearInImage selects a provider and delegates the call.
func (p *LoadBalancedProvider) DetectGearInImage(ctx context.Context, req *DetectionImage) (*GearDetection, error) {
	providers := p.getProvidersForMethod("DetectGearInImage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "DetectGearInImage",
		"provider", name)
	return selected.DetectGearInImage(ctx, req)
}

// GenerateGearFromText selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateGearFromText(ctx context.Context, prompt, region string) (*GearGeneration, error) {
	providers := p.getProvidersForMethod("GenerateGearFromText")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateGearFromText",
		"provider", name)
	return selected.GenerateGearFromText(ctx, prompt, region)
}

// GenerateGearFromWebpage selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateGearFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (*GearGeneration, error) {
	providers := p.getProvidersForMethod("GenerateGearFromWebpage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateGearFromWebpage",
		"provider", name)
	return selected.GenerateGearFromWebpage(ctx, pageTitle, pageDescription, pageBody, region)
}

// GenerateCommunityContent selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateCommunityContent(ctx context.Context, prompt, region string) (*CommunityGeneration, error) {
	providers := p.getProvidersForMethod("GenerateCommunityContent")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateCommunityContent",
		"provider", name)
	return selected.GenerateCommunityContent(ctx, prompt, region)
}

// GenerateRequestContent selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateRequestContent(ctx context.Context, prompt, region string) (*RequestGeneration, error) {
	providers := p.getProvidersForMethod("GenerateRequestContent")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateRequestContent",
		"provider", name)
	return selected.GenerateRequestContent(ctx, prompt, region)
}

// GenerateExperienceFromText selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateExperienceFromText(ctx context.Context, prompt, region, currentTime string) (*ExperienceGeneration, error) {
	providers := p.getProvidersForMethod("GenerateExperienceFromText")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateExperienceFromText",
		"provider", name)
	return selected.GenerateExperienceFromText(ctx, prompt, region, currentTime)
}

// GenerateExperienceFromImage selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateExperienceFromImage(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (*ExperienceGeneration, error) {
	providers := p.getProvidersForMethod("GenerateExperienceFromImage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateExperienceFromImage",
		"provider", name)
	return selected.GenerateExperienceFromImage(ctx, image, region, notes, currentTime)
}

// GenerateExperienceFromWebpage selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateExperienceFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (*ExperienceGeneration, error) {
	providers := p.getProvidersForMethod("GenerateExperienceFromWebpage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateExperienceFromWebpage",
		"provider", name)
	return selected.GenerateExperienceFromWebpage(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
}

// GenerateRequestFromImage selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateRequestFromImage(ctx context.Context, image *DetectionImage, region string) (*RequestGeneration, error) {
	providers := p.getProvidersForMethod("GenerateRequestFromImage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateRequestFromImage",
		"provider", name)
	return selected.GenerateRequestFromImage(ctx, image, region)
}

// GenerateConversationSummary selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateConversationSummary(ctx context.Context, messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) (*ConversationSummary, error) {
	providers := p.getProvidersForMethod("GenerateConversationSummary")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateConversationSummary",
		"provider", name)
	return selected.GenerateConversationSummary(ctx, messages, requestTitle, requestDescription, style)
}

// GenerateExperienceCompletionSummary selects a provider and delegates the call.
func (p *LoadBalancedProvider) GenerateExperienceCompletionSummary(ctx context.Context, messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) (*ConversationSummary, error) {
	providers := p.getProvidersForMethod("GenerateExperienceCompletionSummary")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateExperienceCompletionSummary",
		"provider", name)
	return selected.GenerateExperienceCompletionSummary(ctx, messages, experienceTitle, experienceDescription, attendeeNames)
}

// ParseInformalTime selects a provider and delegates the call.
func (p *LoadBalancedProvider) ParseInformalTime(ctx context.Context, informalDescription, currentTime, timezone string) (string, error) {
	providers := p.getProvidersForMethod("ParseInformalTime")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "ParseInformalTime",
		"provider", name)
	return selected.ParseInformalTime(ctx, informalDescription, currentTime, timezone)
}

// GenerateExperienceSuggestions generates suggestion chips using a randomly selected provider.
func (p *LoadBalancedProvider) GenerateExperienceSuggestions(ctx context.Context, name, description, category string) (*ExperienceSuggestionResult, error) {
	providers := p.getProvidersForMethod("GenerateExperienceSuggestions")
	selected, providerName := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateExperienceSuggestions",
		"provider", providerName)
	return selected.GenerateExperienceSuggestions(ctx, name, description, category)
}

// GenerateRequestSuggestions generates request suggestion chips using a randomly selected provider.
func (p *LoadBalancedProvider) GenerateRequestSuggestions(ctx context.Context, title, description, location string) (*RequestSuggestionResult, error) {
	providers := p.getProvidersForMethod("GenerateRequestSuggestions")
	selected, providerName := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateRequestSuggestions",
		"provider", providerName)
	return selected.GenerateRequestSuggestions(ctx, title, description, location)
}

// InferSocialAttributes infers social attributes using a randomly selected provider.
func (p *LoadBalancedProvider) InferSocialAttributes(ctx context.Context, title, description, txType string, itemValueUSD float32) (*SocialAttributeInference, error) {
	providers := p.getProvidersForMethod("InferSocialAttributes")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "InferSocialAttributes",
		"provider", name)
	return selected.InferSocialAttributes(ctx, title, description, txType, itemValueUSD)
}

// ClassifyUnifiedCreate selects a provider and delegates the call.
func (p *LoadBalancedProvider) ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
	providers := p.getProvidersForMethod("ClassifyUnifiedCreate")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "ClassifyUnifiedCreate",
		"provider", name)
	return selected.ClassifyUnifiedCreate(ctx, in)
}

// CheckHealth returns a single composite status reflecting whether the AI load-balanced
// chain can serve requests. The composite is healthy when at least one underlying provider
// is healthy — a single working provider is sufficient to serve requests via weighted
// selection. Unhealthy leaves emit WARN-level "dependency_leaf_unhealthy" logs without
// triggering the health-check alert metric.
func (p *LoadBalancedProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	logger := logging.LoggerWithContext(ctx)

	var totalLatencyMs int64
	var healthyCount int
	var backends []string
	var firstError string

	for _, wp := range p.providers {
		statuses, err := wp.Provider.CheckHealth(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range statuses {
			totalLatencyMs += s.LatencyMs
			backends = append(backends, s.Backend)
			if s.IsHealthy() {
				healthyCount++
			} else {
				if firstError == "" {
					firstError = s.Error
				}
				logger.WarnContext(ctx, "dependency_leaf_unhealthy",
					"dependency", "ai",
					"backend", s.Backend,
					"error", s.Error)
			}
		}
	}

	compositeError := ""
	if healthyCount == 0 {
		if firstError != "" {
			compositeError = firstError
		} else {
			compositeError = "all providers unhealthy"
		}
	}

	metadata := map[string]string{
		"weighted_backends":      strings.Join(backends, ","),
		"healthy_provider_count": strconv.Itoa(healthyCount),
		"total_provider_count":   strconv.Itoa(len(backends)),
	}
	if firstError != "" {
		metadata["first_unhealthy_error"] = firstError
	}

	return []*health.Status{{
		Name:      "ai",
		Backend:   "load-balanced",
		LatencyMs: totalLatencyMs,
		Metadata:  metadata,
		Error:     compositeError,
	}}, nil
}
