package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/invopop/jsonschema"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// warnIfTruncated logs a warning when the Anthropic response hit the
// MaxTokens cap. Tool_use outputs truncated at the cap produce invalid JSON,
// which fails downstream unmarshal — this gives us an explicit signal in logs
// before the parse error obscures the root cause. Call immediately after a
// successful Messages.New / NewStreaming completion. The caller-provided logger
// already carries the "operation" attr from logger.With(...) earlier in the
// function, so we don't need to pass it again.
func warnIfTruncated(ctx context.Context, logger *logging.Logger, resp *anthropic.Message) {
	if resp == nil || resp.StopReason != anthropic.StopReasonMaxTokens {
		return
	}
	logger.WarnContext(ctx,
		"anthropic response truncated by MaxTokens cap — output JSON may be invalid",
		"stop_reason", string(resp.StopReason),
	)
}

// logAPICallCompleted emits the standard "anthropic API call completed" info
// log with duration, token counts, and any extra key/value pairs. Token fields
// (input_tokens, output_tokens, cache_creation_input_tokens,
// cache_read_input_tokens) come from resp.Usage — the cache_* fields are zero
// when prompt caching is not in use. Keeps all call sites consistent so prod
// log queries can rely on a stable field set.
func logAPICallCompleted(logger *logging.Logger, startTime time.Time, resp *anthropic.Message, extra ...any) {
	fields := []any{"duration_ms", time.Since(startTime).Milliseconds()}
	if resp != nil {
		fields = append(fields,
			"input_tokens", resp.Usage.InputTokens,
			"output_tokens", resp.Usage.OutputTokens,
			"cache_creation_input_tokens", resp.Usage.CacheCreationInputTokens,
			"cache_read_input_tokens", resp.Usage.CacheReadInputTokens,
		)
	}
	fields = append(fields, extra...)
	logger.Info("anthropic API call completed", fields...)
}

// AnthropicProvider implements the Provider interface using Anthropic's Claude API.
type AnthropicProvider struct {
	client      *anthropic.Client
	model       string
	temperature float64
}

// NewAnthropicProvider creates a new Anthropic provider with the specified configuration.
func NewAnthropicProvider(apiKey, model string, temperature float64) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required for the Anthropic provider")
	}

	if model == "" {
		model = "claude-haiku-4-5"
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey), option.WithMaxRetries(5))

	return &AnthropicProvider{
		client:      &client,
		model:       model,
		temperature: temperature,
	}, nil
}

// Name returns the provider's name for logging.
func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

// generateToolSchema creates a JSON schema for the given type for use with Anthropic's tool use.
func generateToolSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)

	// Convert to the format Anthropic expects
	schemaBytes, _ := json.Marshal(schema)
	var schemaMap map[string]interface{}
	_ = json.Unmarshal(schemaBytes, &schemaMap)

	return anthropic.ToolInputSchemaParam{
		Properties: schemaMap["properties"],
		ExtraFields: map[string]interface{}{
			"type":     schemaMap["type"],
			"required": schemaMap["required"],
		},
	}
}

// DetectGearInImage analyzes an image using Anthropic Claude and returns
// the primary detected gear item. Drains the streaming counterpart for
// shape parity with the streaming code path.
func (p *AnthropicProvider) DetectGearInImage(ctx context.Context, req *DetectionImage) (*GearDetection, error) {
	fields, final, err := p.DetectGearInImageStreaming(ctx, req)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateGearFromText creates gear content from a text prompt using Anthropic Claude.
func (p *AnthropicProvider) GenerateGearFromText(ctx context.Context, prompt, region string) (*GearGeneration, error) {
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
// webpage content using Anthropic. Drains the streaming counterpart for
// shape parity with the streaming code path.
func (p *AnthropicProvider) GenerateGearFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (*GearGeneration, error) {
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
// using Anthropic Claude. Drains the streaming counterpart for shape parity
// with the streaming code path — the canonical schema and the AI prompt
// are owned by GenerateCommunityContentStreaming.
func (p *AnthropicProvider) GenerateCommunityContent(ctx context.Context, prompt, region string) (*CommunityGeneration, error) {
	fields, final, err := p.GenerateCommunityContentStreaming(ctx, prompt, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateRequestContent creates request content from a user prompt using Anthropic Claude.
func (p *AnthropicProvider) GenerateRequestContent(ctx context.Context, prompt, region string) (*RequestGeneration, error) {
	fields, final, err := p.GenerateRequestContentStreaming(ctx, prompt, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateRequestFromImage analyzes an image and creates request content
// using Anthropic Claude vision. Drains the streaming counterpart for
// shape parity with the streaming code path.
func (p *AnthropicProvider) GenerateRequestFromImage(ctx context.Context, image *DetectionImage, region string) (*RequestGeneration, error) {
	fields, final, err := p.GenerateRequestFromImageStreaming(ctx, image, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateConversationSummary creates a summary of a conversation using Anthropic Claude.
func (p *AnthropicProvider) GenerateConversationSummary(ctx context.Context, messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) (*ConversationSummary, error) {
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
	result, resp, err := anthropicCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperature, anthropicDefaultMaxTokens,
		[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(promptText))}, "conversation_summary")
	if err != nil {
		return nil, err
	}
	logAPICallCompleted(logger, startTime, resp)
	return &ConversationSummary{Summary: result.Summary}, nil
}

// GenerateExperienceCompletionSummary creates a summary of how an experience went using Anthropic Claude.
func (p *AnthropicProvider) GenerateExperienceCompletionSummary(ctx context.Context, messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) (*ConversationSummary, error) {
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
	result, resp, err := anthropicCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperature, anthropicDefaultMaxTokens,
		[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(promptText))}, "experience_completion_summary")
	if err != nil {
		return nil, err
	}
	logAPICallCompleted(logger, startTime, resp)
	return &ConversationSummary{Summary: result.Summary}, nil
}

// GenerateExperienceFromText creates experience content from a user prompt using Anthropic Claude.
func (p *AnthropicProvider) GenerateExperienceFromText(ctx context.Context, prompt, region, currentTime string) (*ExperienceGeneration, error) {
	fields, final, err := p.GenerateExperienceFromTextStreaming(ctx, prompt, region, currentTime)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateExperienceFromImage analyzes an image and creates experience
// content using Anthropic Claude. Drains the streaming counterpart for
// shape parity with the streaming code path.
func (p *AnthropicProvider) GenerateExperienceFromImage(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (*ExperienceGeneration, error) {
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
// text using Anthropic. Drains the streaming counterpart for shape parity
// with the streaming code path.
func (p *AnthropicProvider) GenerateExperienceFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (*ExperienceGeneration, error) {
	fields, final, err := p.GenerateExperienceFromWebpageStreaming(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// ParseInformalTime converts an informal time description to structured JSON.
func (p *AnthropicProvider) ParseInformalTime(ctx context.Context, informalDescription, currentTime, timezone string) (string, error) {
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

	// Call Anthropic API for text completion
	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   1024,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(promptText)),
		},
	})
	if err != nil {
		logger.Warn("anthropic API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("anthropic API call failed: %w", err)
	}

	// Extract text content from response
	for _, block := range resp.Content {
		if block.Type == "text" {
			logAPICallCompleted(logger, startTime, resp)
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("anthropic returned no text content")
}

// ClassifyUnifiedCreate runs the unified-create classifier prompt against the
// configured Anthropic model and returns the parsed classification.
//
// Uses a hand-built tool schema with an enum constraint so the model emits
// exactly one of the three valid types and decode collapses to a tiny
// single-value JSON object. Bypasses anthropicCallStructured (which uses
// reflection-based schema generation that can't express enum constraints).
func (p *AnthropicProvider) ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
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

	const toolName = "unified_create_classification"
	toolSchema := anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"type": map[string]any{
				"type": "string",
				"enum": []string{
					string(UnifiedCreateContentTypeGear),
					string(UnifiedCreateContentTypeEvent),
					string(UnifiedCreateContentTypeRequest),
				},
			},
		},
		ExtraFields: map[string]any{
			"type":     "object",
			"required": []string{"type"},
		},
	}

	// Build the user-message content blocks. Image mode includes the
	// image block alongside the text prompt so the model classifies on
	// actual visual content (#1939). Image goes first to match the
	// [image, text] order used by the per-type *FromImage methods in
	// provider_anthropic_streaming.go — minor codebase consistency.
	contentBlocks := []anthropic.ContentBlockParamUnion{}
	if in.Image != nil {
		imgBlock, err := buildAnthropicImageBlock(ctx, in.Image)
		if err != nil {
			return nil, fmt.Errorf("build anthropic image block: %w", err)
		}
		contentBlocks = append(contentBlocks, imgBlock)
	}
	contentBlocks = append(contentBlocks, anthropic.NewTextBlock(promptText))

	resp, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   anthropicTinyMaxTokens,
		Temperature: anthropicTempParam(p.temperature),
		Messages:    []anthropic.MessageParam{anthropic.NewUserMessage(contentBlocks...)},
		Tools:       []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, toolName)},
		ToolChoice:  anthropic.ToolChoiceParamOfTool(toolName),
	})
	if err != nil {
		logger.WarnContext(ctx, "anthropic API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("anthropic API call failed: %w", err)
	}
	warnIfTruncated(ctx, logger, resp)
	logAPICallCompleted(logger, startTime, resp)

	var result UnifiedCreateClassification
	for _, block := range resp.Content {
		if block.Type == blockTypeToolUse {
			inputBytes, err := json.Marshal(block.Input)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool input: %w", err)
			}
			if err := json.Unmarshal(inputBytes, &result); err != nil {
				return nil, fmt.Errorf("failed to parse anthropic response: %w", err)
			}
			break
		}
	}
	if result.Type == "" {
		return nil, fmt.Errorf("anthropic returned no classification")
	}
	return &result, nil
}

// CheckHealth validates Anthropic API connectivity by listing available models (free API call).
func (p *AnthropicProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
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
	_, err := p.client.Models.List(ctx, anthropic.ModelListParams{})
	status.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		status.Error = err.Error()
	}

	return []*health.Status{status}, nil
}

// GenerateExperienceSuggestions produces category-specific suggestion chip labels using Anthropic.
func (p *AnthropicProvider) GenerateExperienceSuggestions(ctx context.Context, name, description, category string) (*ExperienceSuggestionResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"provider", p.Name(),
		"operation", "GenerateExperienceSuggestions",
		"model", p.model,
	)
	startTime := time.Now()
	promptText := buildExperienceSuggestionsPrompt(name, description, category)
	type outputSchema struct {
		Suggestions  []string `json:"suggestions"`
		CategoryHint string   `json:"category_hint"`
	}
	result, resp, err := anthropicCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperature, anthropicSmallMaxTokens,
		[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(promptText))}, "experience_suggestions")
	if err != nil {
		return nil, err
	}
	logAPICallCompleted(logger, startTime, resp)
	return &ExperienceSuggestionResult{
		Suggestions:  truncateSuggestions(result.Suggestions, 8),
		CategoryHint: result.CategoryHint,
	}, nil
}

// GenerateRequestSuggestions produces Plan-tab chip labels for a help request using Anthropic.
func (p *AnthropicProvider) GenerateRequestSuggestions(ctx context.Context, title, description, location string) (*RequestSuggestionResult, error) {
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
	result, resp, err := anthropicCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperature, anthropicSmallMaxTokens,
		[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(promptText))}, "request_suggestions")
	if err != nil {
		return nil, err
	}
	logAPICallCompleted(logger, startTime, resp)
	return &RequestSuggestionResult{
		AdditionalAsks:  truncateSuggestions(result.AdditionalAsks, 4),
		BreakdownPieces: truncateSuggestions(result.BreakdownPieces, 4),
		OfferIdeas:      truncateSuggestions(result.OfferIdeas, 8),
		SeedNeeds:       truncateSuggestions(result.SeedNeeds, 8),
	}, nil
}

// InferSocialAttributes estimates Social Footprint input attributes from transaction context.
func (p *AnthropicProvider) InferSocialAttributes(ctx context.Context, title, description, txType string, itemValueUSD float32) (*SocialAttributeInference, error) {
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
	result, _, err := anthropicCallStructured[outputSchema](ctx, logger, startTime, p.client, p.model, p.temperature, anthropicTinyMaxTokens,
		[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(promptText))}, "social_attributes")
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
