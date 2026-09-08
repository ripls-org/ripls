package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// OpenAIProvider implements the Provider interface using OpenAI's API.
type OpenAIProvider struct {
	client      *openai.Client
	model       string
	temperature float64
}

// validateGearMetadata ensures material_category and weight_grams have valid values.
// Returns validated values and logs warnings if defaults were applied.
func validateGearMetadata(materialCategory string, weightGrams float32, title, category string, logger *logging.Logger) (string, float32) {
	// Validate material_category
	validMaterial := materialCategory
	if MaterialCategoryFromJSON(materialCategory) == api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED {
		// Provide sensible default based on category or title
		switch {
		case strings.Contains(strings.ToLower(category), "power tool") ||
			strings.Contains(strings.ToLower(category), "tool"):
			validMaterial = "mixed_plastic_metal"
		case strings.Contains(strings.ToLower(category), "electronic"):
			validMaterial = "electronics_small"
		case strings.Contains(strings.ToLower(category), "camping") ||
			strings.Contains(strings.ToLower(category), "outdoor"):
			validMaterial = "fabric"
		case strings.Contains(strings.ToLower(category), "kitchen") ||
			strings.Contains(strings.ToLower(title), "mixer"):
			validMaterial = "solid_metal"
		default:
			validMaterial = "mixed_plastic_metal" // Generic fallback
		}
		logger.Warn("material_category invalid or empty, using default",
			"original", materialCategory,
			"default", validMaterial,
			"category", category,
		)
	}

	// Validate weight_grams
	validWeight := weightGrams
	if validWeight <= 0 {
		// Provide reasonable default based on category or title
		switch {
		case strings.Contains(strings.ToLower(category), "bicycle"):
			validWeight = 12000 // ~12 kg for bike
		case strings.Contains(strings.ToLower(category), "power tool"):
			validWeight = 2000 // ~2 kg for power tool
		case strings.Contains(strings.ToLower(category), "camping"):
			validWeight = 3000 // ~3 kg for tent
		case strings.Contains(strings.ToLower(category), "kitchen") ||
			strings.Contains(strings.ToLower(title), "mixer"):
			validWeight = 7000 // ~7 kg for stand mixer
		default:
			validWeight = 1000 // 1 kg generic fallback
		}
		logger.Warn("weight_grams invalid, using default",
			"original", weightGrams,
			"default", validWeight,
			"category", category,
		)
	}

	return validMaterial, validWeight
}

// NewOpenAIProvider creates a new OpenAI provider with the specified configuration.
func NewOpenAIProvider(apiKey, model string, temperature float64) (*OpenAIProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	if model == "" {
		model = "gpt-5-mini"
	}

	client := openai.NewClient(option.WithAPIKey(apiKey), option.WithMaxRetries(5))

	return &OpenAIProvider{
		client:      &client,
		model:       model,
		temperature: temperature,
	}, nil
}

// Name returns the provider's name for logging.
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// temperatureParam returns the temperature value to send on a Chat Completions
// call, or an unset Opt for callers (and reasoning-class models) that should
// not impose an explicit temperature. A NaN p.temperature signals "let the
// model use its own default" and is used by the eval harness to compare
// across models that accept different temperature ranges (e.g. gpt-5.5 only
// accepts the fixed default of 1.0). The OpenAI gpt-5 reasoning family
// (gpt-5, gpt-5-mini, gpt-5-nano) and the o-series (o1, o3, o4) return a 400
// error if temperature is provided, even at 1.0. The conventional gpt-5.x
// chat family (e.g., gpt-5.4-nano) accepts custom temperatures normally.
func (p *OpenAIProvider) temperatureParam() param.Opt[float64] {
	if math.IsNaN(p.temperature) {
		return param.Opt[float64]{}
	}
	m := strings.ToLower(p.model)
	if strings.HasPrefix(m, "gpt-5-") || m == "gpt-5" {
		return param.Opt[float64]{}
	}
	if strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") {
		return param.Opt[float64]{}
	}
	return param.NewOpt(p.temperature)
}

// generateSchema creates a JSON schema for the given type using the invopop/jsonschema package.
// The schema is configured for OpenAI's structured output requirements.
func generateSchema[T any]() any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return schema
}

// DetectGearInImage analyzes an image using OpenAI and returns the primary
// detected gear item. Drains the streaming counterpart for shape parity
// with the streaming code path.
func (p *OpenAIProvider) DetectGearInImage(ctx context.Context, req *DetectionImage) (*GearDetection, error) {
	fields, final, err := p.DetectGearInImageStreaming(ctx, req)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateGearFromText creates gear content from a text prompt using OpenAI.
func (p *OpenAIProvider) GenerateGearFromText(ctx context.Context, prompt, region string) (*GearGeneration, error) {
	fields, final, err := p.GenerateGearFromTextStreaming(ctx, prompt, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateGearFromWebpage extracts gear content and value estimate from
// webpage content using OpenAI. Drains the streaming counterpart.
func (p *OpenAIProvider) GenerateGearFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (*GearGeneration, error) {
	fields, final, err := p.GenerateGearFromWebpageStreaming(ctx, pageTitle, pageDescription, pageBody, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateCommunityContent creates community content from a user prompt
// using OpenAI. Drains the streaming counterpart for shape parity with the
// streaming code path.
func (p *OpenAIProvider) GenerateCommunityContent(ctx context.Context, prompt, region string) (*CommunityGeneration, error) {
	fields, final, err := p.GenerateCommunityContentStreaming(ctx, prompt, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateRequestContent creates request content from a user prompt using OpenAI.
func (p *OpenAIProvider) GenerateRequestContent(ctx context.Context, prompt, region string) (*RequestGeneration, error) {
	fields, final, err := p.GenerateRequestContentStreaming(ctx, prompt, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateConversationSummary creates a summary of a conversation using OpenAI.
func (p *OpenAIProvider) GenerateConversationSummary(ctx context.Context, messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) (*ConversationSummary, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages cannot be empty")
	}
	logger := logging.LoggerWithContext(ctx).With(
		"provider", p.Name(),
		"operation", "GenerateConversationSummary",
		"model", p.model,
		"style", style,
	)
	startTime := time.Now()
	promptText := buildConversationSummaryPrompt(messages, requestTitle, requestDescription, style)
	type outputSchema struct {
		Summary string `json:"summary"`
	}
	result, err := openaiCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperatureParam(),
		[]openai.ChatCompletionMessageParamUnion{openai.UserMessage(promptText)}, "conversation_summary")
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "openai API call completed", "duration_ms", time.Since(startTime).Milliseconds())
	return &ConversationSummary{Summary: result.Summary}, nil
}

// GenerateExperienceCompletionSummary creates a summary of how an experience went using OpenAI.
func (p *OpenAIProvider) GenerateExperienceCompletionSummary(ctx context.Context, messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) (*ConversationSummary, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"provider", p.Name(),
		"operation", "GenerateExperienceCompletionSummary",
		"model", p.model,
	)
	startTime := time.Now()
	promptText := buildExperienceCompletionSummaryPrompt(messages, experienceTitle, experienceDescription, attendeeNames)
	type outputSchema struct {
		Summary string `json:"summary"`
	}
	result, err := openaiCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperatureParam(),
		[]openai.ChatCompletionMessageParamUnion{openai.UserMessage(promptText)}, "experience_completion_summary")
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "openai API call completed", "duration_ms", time.Since(startTime).Milliseconds())
	return &ConversationSummary{Summary: result.Summary}, nil
}

// GenerateExperienceFromText creates experience content from a user prompt using OpenAI.
func (p *OpenAIProvider) GenerateExperienceFromText(ctx context.Context, prompt, region, currentTime string) (*ExperienceGeneration, error) {
	fields, final, err := p.GenerateExperienceFromTextStreaming(ctx, prompt, region, currentTime)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateExperienceFromImage analyzes an image and creates experience content using OpenAI.
// GenerateExperienceFromImage analyzes an image and creates experience
// content using OpenAI. Drains the streaming counterpart for shape parity
// with the streaming code path.
func (p *OpenAIProvider) GenerateExperienceFromImage(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (*ExperienceGeneration, error) {
	fields, final, err := p.GenerateExperienceFromImageStreaming(ctx, image, region, notes, currentTime)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateExperienceFromWebpage extracts experience content from webpage
// text using OpenAI. Drains the streaming counterpart.
func (p *OpenAIProvider) GenerateExperienceFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (*ExperienceGeneration, error) {
	fields, final, err := p.GenerateExperienceFromWebpageStreaming(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateRequestFromImage analyzes an image and creates request content
// using OpenAI. Drains the streaming counterpart for shape parity with
// the streaming code path.
func (p *OpenAIProvider) GenerateRequestFromImage(ctx context.Context, image *DetectionImage, region string) (*RequestGeneration, error) {
	fields, final, err := p.GenerateRequestFromImageStreaming(ctx, image, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// ParseInformalTime converts an informal time description to structured JSON.
func (p *OpenAIProvider) ParseInformalTime(ctx context.Context, informalDescription, currentTime, timezone string) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"provider", p.Name(),
		"operation", "ParseInformalTime",
		"model", p.model,
	)
	startTime := time.Now()

	if informalDescription == "" {
		return "", fmt.Errorf("informal description cannot be empty")
	}

	// Build the prompt text
	promptText := buildInformalToCalendarPrompt(informalDescription, currentTime, timezone)

	// Call OpenAI API for text completion (no structured output needed, we want raw JSON)
	resp, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(promptText),
		},
	})
	if err != nil {
		logger.Warn("openai API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("openai API call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}

	logger.Info("openai API call completed", "duration_ms", time.Since(startTime).Milliseconds())
	return resp.Choices[0].Message.Content, nil
}

// ClassifyUnifiedCreate runs the unified-create classifier prompt against the
// configured OpenAI model and returns the parsed classification.
//
// Uses OpenAI's strict-mode JSON schema with an enum constraint on the
// type field, so the model is forced to emit exactly one of the three
// valid values. Bypasses openaiCallStructured (reflection-based schema
// generation can't express enum constraints).
func (p *OpenAIProvider) ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"provider", p.Name(),
		"operation", "UnifiedCreateClassify",
		"model", p.model,
		"prompt_version", UnifiedCreateClassifierPromptVersion,
	)
	startTime := time.Now()

	var promptText string
	switch {
	case in.Text != "":
		promptText = buildUnifiedCreateClassifierPrompt(in.Text)
	case in.WebsiteURL != "":
		promptText = buildUnifiedCreateClassifierWebpagePrompt(in.WebsiteTitle, in.WebsiteDescription)
	case in.Image != nil:
		promptText = buildUnifiedCreateClassifierImagePrompt()
	default:
		return nil, fmt.Errorf("unified classifier: empty input")
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type": "string",
				"enum": []string{
					string(UnifiedCreateContentTypeGear),
					string(UnifiedCreateContentTypeEvent),
					string(UnifiedCreateContentTypeRequest),
				},
			},
		},
		"required":             []string{"type"},
		"additionalProperties": false,
	}

	// Build the user message. Text-only mode passes the prompt directly;
	// image mode wraps text + image as multi-part content so the model
	// classifies on actual visual content (#1939).
	var userMsg openai.ChatCompletionMessageParamUnion
	if in.Image != nil {
		imgPart, err := buildOpenAIImageContent(ctx, in.Image)
		if err != nil {
			return nil, fmt.Errorf("build openai image content: %w", err)
		}
		userMsg = openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
			imgPart,
			openai.TextContentPart(promptText),
		})
	} else {
		userMsg = openai.UserMessage(promptText)
	}

	resp, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages:    []openai.ChatCompletionMessageParamUnion{userMsg},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "unified_create_classification",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	})
	if err != nil {
		logger.WarnContext(ctx, "openai API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("openai API call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}
	logger.InfoContext(ctx, "openai API call completed", "duration_ms", time.Since(startTime).Milliseconds())

	var result UnifiedCreateClassification
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("parse openai response: %w (body: %q)", err, resp.Choices[0].Message.Content)
	}
	if result.Type == "" {
		return nil, fmt.Errorf("openai returned empty classification")
	}
	return &result, nil
}

// CheckHealth validates OpenAI API connectivity by listing available models (free API call).
func (p *OpenAIProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()

	status := &health.Status{
		Name:    "ai",
		Backend: p.Name(),
		Metadata: map[string]string{
			"model": p.model,
		},
	}

	// List models - this is a free API call that validates the API key
	_, err := p.client.Models.List(ctx)
	status.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		status.Error = err.Error()
	}

	return []*health.Status{status}, nil
}

// GenerateExperienceSuggestions produces category-specific suggestion chip labels using OpenAI.
func (p *OpenAIProvider) GenerateExperienceSuggestions(ctx context.Context, name, description, category string) (*ExperienceSuggestionResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"provider", p.Name(),
		"operation", "GenerateExperienceSuggestions",
		"model", p.model,
	)
	startTime := time.Now()
	promptText := buildExperienceSuggestionsPrompt(name, description, category)
	// OpenAI Strict mode requires value types (no pointers, no omitempty).
	type outputSchema struct {
		Suggestions  []string `json:"suggestions"`
		CategoryHint string   `json:"category_hint"`
	}
	result, err := openaiCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperatureParam(),
		[]openai.ChatCompletionMessageParamUnion{openai.UserMessage(promptText)}, "experience_suggestions")
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "openai API call completed", "duration_ms", time.Since(startTime).Milliseconds())
	return &ExperienceSuggestionResult{
		Suggestions:  truncateSuggestions(result.Suggestions, 8),
		CategoryHint: result.CategoryHint,
	}, nil
}

// GenerateRequestSuggestions produces Plan-tab chip labels for a help request using OpenAI.
func (p *OpenAIProvider) GenerateRequestSuggestions(ctx context.Context, title, description, location string) (*RequestSuggestionResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"provider", p.Name(),
		"operation", "GenerateRequestSuggestions",
		"model", p.model,
	)
	startTime := time.Now()
	promptText := buildRequestSuggestionsPrompt(title, description, location)
	type outputSchema struct {
		AdditionalAsks  []string `json:"additional_asks"`
		BreakdownPieces []string `json:"breakdown_pieces"`
		OfferIdeas      []string `json:"offer_ideas"`
		SeedNeeds       []string `json:"seed_needs"`
	}
	result, err := openaiCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperatureParam(),
		[]openai.ChatCompletionMessageParamUnion{openai.UserMessage(promptText)}, "request_suggestions")
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "openai API call completed", "duration_ms", time.Since(startTime).Milliseconds())
	return &RequestSuggestionResult{
		AdditionalAsks:  truncateSuggestions(result.AdditionalAsks, 4),
		BreakdownPieces: truncateSuggestions(result.BreakdownPieces, 4),
		OfferIdeas:      truncateSuggestions(result.OfferIdeas, 8),
		SeedNeeds:       truncateSuggestions(result.SeedNeeds, 8),
	}, nil
}

// InferSocialAttributes estimates Social Footprint input attributes from transaction context.
func (p *OpenAIProvider) InferSocialAttributes(ctx context.Context, title, description, txType string, itemValueUSD float32) (*SocialAttributeInference, error) {
	if title == "" && description == "" {
		return nil, nil
	}
	logger := logging.LoggerWithContext(ctx).With(
		"provider", p.Name(),
		"operation", "InferSocialAttributes",
		"model", p.model,
		"tx_type", txType,
	)
	startTime := time.Now()
	promptText := buildSocialAttributePrompt(title, description, txType, itemValueUSD)
	type outputSchema struct {
		DurationMinutes        float32 `json:"duration_minutes"`
		DurationConfidence     float32 `json:"duration_confidence"`
		DurationReasoning      string  `json:"duration_reasoning"`
		VulnerabilityLevel     string  `json:"vulnerability_level"`
		VulnerabilityReasoning string  `json:"vulnerability_reasoning"`
	}
	result, err := openaiCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperatureParam(),
		[]openai.ChatCompletionMessageParamUnion{openai.UserMessage(promptText)}, "social_attributes")
	if err != nil {
		return nil, err
	}
	logger.DebugContext(ctx, "social attributes inferred",
		"duration_ms", time.Since(startTime).Milliseconds(),
		"duration_minutes", result.DurationMinutes,
		"vulnerability_level", result.VulnerabilityLevel,
	)
	if result.DurationMinutes <= 0 {
		return nil, nil
	}
	return &SocialAttributeInference{
		DurationMinutes:        result.DurationMinutes,
		DurationConfidence:     NormalizeConfidence(result.DurationConfidence),
		DurationReasoning:      result.DurationReasoning,
		VulnerabilityLevel:     result.VulnerabilityLevel,
		VulnerabilityReasoning: result.VulnerabilityReasoning,
	}, nil
}
