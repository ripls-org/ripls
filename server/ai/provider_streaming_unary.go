package ai

import (
	"context"

	"go.ripls.org/ripls/server/logging"
)

// This file holds the streaming-method implementations for the *composite*
// providers (Fallback, LoadBalanced). Each delegates to its underlying
// selected provider's streaming method. Mid-stream errors are NOT
// auto-retried — once a FieldEvent has been emitted to the consumer, a
// retry can't take it back, so the safe behavior is to surface the error
// on the final channel and let the caller decide. Synchronous setup-time
// errors (validation, etc.) propagate directly.
//
// The leaf providers (Anthropic, OpenAI, Gemini) implement real per-token
// streaming in their respective *_streaming.go files; the Mock provider's
// streaming methods default to a unary wrapper but expose override hooks for
// scripted-event tests.

// GenerateExperienceFromTextStreaming implements the Provider interface by
// delegating to the primary provider. Synchronous errors propagate; on
// success, the stream from the primary is returned as-is.
func (p *FallbackProvider) GenerateExperienceFromTextStreaming(ctx context.Context, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	return p.primary.GenerateExperienceFromTextStreaming(ctx, prompt, region, currentTime)
}

// GenerateRequestContentStreaming implements the Provider interface.
func (p *FallbackProvider) GenerateRequestContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	return p.primary.GenerateRequestContentStreaming(ctx, prompt, region)
}

// GenerateCommunityContentStreaming implements the Provider interface.
func (p *FallbackProvider) GenerateCommunityContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal, error) {
	return p.primary.GenerateCommunityContentStreaming(ctx, prompt, region)
}

// GenerateGearFromTextStreaming implements the Provider interface.
func (p *FallbackProvider) GenerateGearFromTextStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	return p.primary.GenerateGearFromTextStreaming(ctx, prompt, region)
}

// GenerateRequestFromImageStreaming implements the Provider interface.
func (p *FallbackProvider) GenerateRequestFromImageStreaming(ctx context.Context, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	return p.primary.GenerateRequestFromImageStreaming(ctx, image, region)
}

// GenerateExperienceFromImageStreaming implements the Provider interface.
func (p *FallbackProvider) GenerateExperienceFromImageStreaming(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	return p.primary.GenerateExperienceFromImageStreaming(ctx, image, region, notes, currentTime)
}

// DetectGearInImageStreaming implements the Provider interface.
func (p *FallbackProvider) DetectGearInImageStreaming(ctx context.Context, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal, error) {
	return p.primary.DetectGearInImageStreaming(ctx, req)
}

// GenerateExperienceFromWebpageStreaming implements the Provider interface.
func (p *FallbackProvider) GenerateExperienceFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	return p.primary.GenerateExperienceFromWebpageStreaming(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
}

// GenerateGearFromWebpageStreaming implements the Provider interface.
func (p *FallbackProvider) GenerateGearFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	return p.primary.GenerateGearFromWebpageStreaming(ctx, pageTitle, pageDescription, pageBody, region)
}

// GenerateExperienceFromTextStreaming implements the Provider interface by
// selecting a provider via the LoadBalancedProvider's existing weighted
// distribution and delegating. No per-attempt retry on stream-side errors.
// Uses the same method-name key as the unary variant for weight lookup so
// operators don't need to configure streaming separately.
func (p *LoadBalancedProvider) GenerateExperienceFromTextStreaming(ctx context.Context, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	providers := p.getProvidersForMethod("GenerateExperienceFromText")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateExperienceFromTextStreaming",
		"provider", name)
	return selected.GenerateExperienceFromTextStreaming(ctx, prompt, region, currentTime)
}

// GenerateRequestContentStreaming implements the Provider interface.
func (p *LoadBalancedProvider) GenerateRequestContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	providers := p.getProvidersForMethod("GenerateRequestContent")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateRequestContentStreaming",
		"provider", name)
	return selected.GenerateRequestContentStreaming(ctx, prompt, region)
}

// GenerateCommunityContentStreaming implements the Provider interface.
func (p *LoadBalancedProvider) GenerateCommunityContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal, error) {
	providers := p.getProvidersForMethod("GenerateCommunityContent")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateCommunityContentStreaming",
		"provider", name)
	return selected.GenerateCommunityContentStreaming(ctx, prompt, region)
}

// GenerateGearFromTextStreaming implements the Provider interface.
func (p *LoadBalancedProvider) GenerateGearFromTextStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	providers := p.getProvidersForMethod("GenerateGearFromText")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateGearFromTextStreaming",
		"provider", name)
	return selected.GenerateGearFromTextStreaming(ctx, prompt, region)
}

// GenerateRequestFromImageStreaming implements the Provider interface. Uses
// the same method-name key as the unary variant for weight lookup so
// operators don't need to configure streaming separately.
func (p *LoadBalancedProvider) GenerateRequestFromImageStreaming(ctx context.Context, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	providers := p.getProvidersForMethod("GenerateRequestFromImage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateRequestFromImageStreaming",
		"provider", name)
	return selected.GenerateRequestFromImageStreaming(ctx, image, region)
}

// GenerateExperienceFromImageStreaming implements the Provider interface.
func (p *LoadBalancedProvider) GenerateExperienceFromImageStreaming(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	providers := p.getProvidersForMethod("GenerateExperienceFromImage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateExperienceFromImageStreaming",
		"provider", name)
	return selected.GenerateExperienceFromImageStreaming(ctx, image, region, notes, currentTime)
}

// DetectGearInImageStreaming implements the Provider interface.
func (p *LoadBalancedProvider) DetectGearInImageStreaming(ctx context.Context, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal, error) {
	providers := p.getProvidersForMethod("DetectGearInImage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "DetectGearInImageStreaming",
		"provider", name)
	return selected.DetectGearInImageStreaming(ctx, req)
}

// GenerateExperienceFromWebpageStreaming implements the Provider interface.
func (p *LoadBalancedProvider) GenerateExperienceFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	providers := p.getProvidersForMethod("GenerateExperienceFromWebpage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateExperienceFromWebpageStreaming",
		"provider", name)
	return selected.GenerateExperienceFromWebpageStreaming(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
}

// GenerateGearFromWebpageStreaming implements the Provider interface.
func (p *LoadBalancedProvider) GenerateGearFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	providers := p.getProvidersForMethod("GenerateGearFromWebpage")
	selected, name := selectProvider(providers)
	logging.LoggerWithContext(ctx).Info("load balancer selected provider",
		"method_name", "GenerateGearFromWebpageStreaming",
		"provider", name)
	return selected.GenerateGearFromWebpageStreaming(ctx, pageTitle, pageDescription, pageBody, region)
}
