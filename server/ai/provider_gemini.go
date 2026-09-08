package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"golang.org/x/oauth2/google"
	"google.golang.org/genai"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// GeminiProvider implements the Provider interface using Google's Gemini models via Genkit.
// Supports both Google AI (API key) and Vertex AI (Application Default Credentials).
type GeminiProvider struct {
	g           *genkit.Genkit
	model       string
	projectID   string
	location    string
	temperature float64
}

// NewGeminiProvider creates a new Gemini provider using Vertex AI.
// Model should start with "vertexai/" prefix. If empty, defaults to "vertexai/gemini-2.5-flash-lite".
// Uses Application Default Credentials for authentication.
func NewGeminiProvider(ctx context.Context, model, projectID, location string, temperature float64) (*GeminiProvider, error) {
	if model == "" {
		model = "vertexai/gemini-2.5-flash-lite"
	}

	g := genkit.Init(ctx, genkit.WithPlugins(&googlegenai.VertexAI{
		ProjectID: projectID,
		Location:  location,
	}))

	return &GeminiProvider{
		g:           g,
		model:       model,
		projectID:   projectID,
		location:    location,
		temperature: temperature,
	}, nil
}

// Name returns the provider's name for logging.
func (p *GeminiProvider) Name() string {
	return "gemini"
}

// DetectGearInImage analyzes an image using Gemini and returns the primary detected gear item.
// DetectGearInImage analyzes an image using Gemini and returns the
// primary detected gear item. Drains the streaming counterpart for
// shape parity with the streaming code path.
func (p *GeminiProvider) DetectGearInImage(ctx context.Context, req *DetectionImage) (*GearDetection, error) {
	fields, final, err := p.DetectGearInImageStreaming(ctx, req)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateGearFromText creates gear content from a text prompt using Gemini.
func (p *GeminiProvider) GenerateGearFromText(ctx context.Context, prompt, region string) (*GearGeneration, error) {
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
// webpage content using Gemini. Drains the streaming counterpart.
func (p *GeminiProvider) GenerateGearFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (*GearGeneration, error) {
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
// using Gemini. Drains the streaming counterpart for shape parity with the
// streaming code path.
func (p *GeminiProvider) GenerateCommunityContent(ctx context.Context, prompt, region string) (*CommunityGeneration, error) {
	fields, final, err := p.GenerateCommunityContentStreaming(ctx, prompt, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateRequestContent creates request content from a user prompt using Gemini.
func (p *GeminiProvider) GenerateRequestContent(ctx context.Context, prompt, region string) (*RequestGeneration, error) {
	fields, final, err := p.GenerateRequestContentStreaming(ctx, prompt, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateExperienceFromText creates experience content from a text prompt using Gemini.
func (p *GeminiProvider) GenerateExperienceFromText(ctx context.Context, prompt, region, currentTime string) (*ExperienceGeneration, error) {
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
// content using Gemini. Drains the streaming counterpart for shape parity
// with the streaming code path.
func (p *GeminiProvider) GenerateExperienceFromImage(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (*ExperienceGeneration, error) {
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
// text using Gemini. Drains the streaming counterpart.
func (p *GeminiProvider) GenerateExperienceFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (*ExperienceGeneration, error) {
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
// using Gemini. Drains the streaming counterpart for shape parity with
// the streaming code path.
func (p *GeminiProvider) GenerateRequestFromImage(ctx context.Context, image *DetectionImage, region string) (*RequestGeneration, error) {
	fields, final, err := p.GenerateRequestFromImageStreaming(ctx, image, region)
	if err != nil {
		return nil, err
	}
	for range fields {
	}
	f := <-final
	return f.Result, f.Err
}

// GenerateConversationSummary creates a summary of a conversation using Gemini.
func (p *GeminiProvider) GenerateConversationSummary(ctx context.Context, messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) (*ConversationSummary, error) {
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
	result, err := geminiCallStructured[outputSchema](ctx, logger, startTime, p.g, p.model, p.temperature,
		ai.WithMessages(ai.NewUserMessage(ai.NewTextPart(promptText))))
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "gemini API call completed", "message_count", len(messages), "duration_ms", time.Since(startTime).Milliseconds())
	return &ConversationSummary{Summary: result.Summary}, nil
}

// GenerateExperienceCompletionSummary creates a summary of how an experience went using Gemini.
func (p *GeminiProvider) GenerateExperienceCompletionSummary(ctx context.Context, messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) (*ConversationSummary, error) {
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
	result, err := geminiCallStructured[outputSchema](ctx, logger, startTime, p.g, p.model, p.temperature,
		ai.WithMessages(ai.NewUserMessage(ai.NewTextPart(promptText))))
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "gemini API call completed", "message_count", len(messages), "duration_ms", time.Since(startTime).Milliseconds())
	return &ConversationSummary{Summary: result.Summary}, nil
}

// ParseInformalTime converts an informal time description to structured JSON using Gemini.
// Returns the raw JSON string from the LLM for parsing by the caller.
func (p *GeminiProvider) ParseInformalTime(ctx context.Context, informalDescription, currentTime, timezone string) (string, error) {
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

	// Call Gemini with text generation (not structured output, since we want raw JSON)
	result, err := genkit.Generate(ctx, p.g,
		ai.WithModelName(p.model),
		ai.WithConfig(&genai.GenerateContentConfig{Temperature: geminiTempPtr(p.temperature)}),
		ai.WithMessages(
			ai.NewUserMessage(ai.NewTextPart(promptText)),
		),
	)
	if err != nil {
		logger.Warn("gemini API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("gemini API call failed: %w", err)
	}

	// Extract text from result
	if result == nil || result.Text() == "" {
		logger.Warn("empty response from Gemini", "duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("empty response from Gemini")
	}

	logger.Info("gemini API call completed", "duration_ms", time.Since(startTime).Milliseconds())

	return result.Text(), nil
}

// countTokensEndpoint constructs the Vertex AI CountTokens URL for a given
// location, project, and model. Regional locations route through
// "<location>-aiplatform.googleapis.com"; the "global" location uses the
// bare "aiplatform.googleapis.com" host (Google does not serve a
// "global-aiplatform.googleapis.com" — that subdomain returns a generic
// HTML 404). The location segment in the path remains the literal
// location string in both cases.
func countTokensEndpoint(location, projectID, modelName string) string {
	host := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	if location == vertexLocationGlobal {
		host = "aiplatform.googleapis.com"
	}
	return fmt.Sprintf(
		"https://%s/v1/projects/%s/locations/%s/publishers/google/models/%s:countTokens",
		host, projectID, location, modelName,
	)
}

// ClassifyUnifiedCreate runs the unified-create classifier prompt against the
// configured Gemini model and returns the parsed classification.
//
// Uses Vertex AI's native ResponseSchema with an enum constraint on the
// type field, so the model is forced to emit exactly one of the three
// valid values. Bypasses geminiCallStructured (reflection-based; can't
// express enum constraints).
func (p *GeminiProvider) ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
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

	// Genkit rejects setting ResponseSchema / ResponseMIMEType directly on
	// GenerateContentConfig (see plugins/googlegenai/gemini.go line ~298).
	// The blessed path is ai.WithOutputSchema with a JSON-Schema map; the
	// plugin converts it into a genai.Schema and sets ResponseSchema +
	// ResponseMIMEType="application/json" itself.
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
		"required": []string{"type"},
	}

	// Build the user-message parts. Image mode appends the image part so
	// the model classifies on actual visual content (#1939).
	// buildGeminiImagePart accepts both the URL and bytes shapes of
	// *DetectionImage; the production path passes a signed URL.
	parts := []*ai.Part{ai.NewTextPart(promptText)}
	if in.Image != nil {
		imgPart, err := buildGeminiImagePart(in.Image)
		if err != nil {
			return nil, fmt.Errorf("build gemini image part: %w", err)
		}
		parts = append(parts, imgPart)
	}

	resp, err := genkit.Generate(ctx, p.g,
		ai.WithModelName(p.model),
		ai.WithConfig(&genai.GenerateContentConfig{Temperature: geminiTempPtr(p.temperature)}),
		ai.WithOutputSchema(schema),
		ai.WithMessages(ai.NewUserMessage(parts...)),
	)
	if err != nil {
		logger.WarnContext(ctx, "gemini API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("gemini API call failed: %w", err)
	}
	logger.InfoContext(ctx, "gemini API call completed", "duration_ms", time.Since(startTime).Milliseconds())

	if resp == nil || resp.Text() == "" {
		return nil, fmt.Errorf("gemini returned empty response")
	}
	var result UnifiedCreateClassification
	if err := json.Unmarshal([]byte(resp.Text()), &result); err != nil {
		return nil, fmt.Errorf("parse gemini response: %w (body: %q)", err, resp.Text())
	}
	if result.Type == "" {
		return nil, fmt.Errorf("gemini returned empty classification")
	}
	return &result, nil
}

// CheckHealth validates Gemini API connectivity using the CountTokens API.
// CountTokens is a free operation that doesn't count against inference quotas,
// avoiding 429 rate limit errors during health monitoring.
func (p *GeminiProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
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

	// Extract model name without the "vertexai/" prefix
	modelName := strings.TrimPrefix(p.model, "vertexai/")
	endpoint := countTokensEndpoint(p.location, p.projectID, modelName)

	// Get Application Default Credentials
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = fmt.Sprintf("failed to get credentials: %v", err)
		return []*health.Status{status}, nil
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = fmt.Sprintf("failed to get token: %v", err)
		return []*health.Status{status}, nil
	}

	// Minimal request body for CountTokens
	reqBody := []byte(`{"contents":[{"role":"user","parts":[{"text":"health check"}]}]}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = fmt.Sprintf("failed to create request: %v", err)
		return []*health.Status{status}, nil
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}
	defer resp.Body.Close()

	status.LatencyMs = time.Since(start).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		status.Error = fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))
		return []*health.Status{status}, nil
	}

	// Parse response to verify we got a valid token count
	var countResp struct {
		TotalTokens int `json:"totalTokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&countResp); err != nil {
		status.Error = fmt.Sprintf("failed to parse response: %v", err)
		return []*health.Status{status}, nil
	}

	return []*health.Status{status}, nil
}

// GenerateExperienceSuggestions produces category-specific suggestion chip labels using Gemini.
func (p *GeminiProvider) GenerateExperienceSuggestions(ctx context.Context, name, description, category string) (*ExperienceSuggestionResult, error) {
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
	result, err := geminiCallStructured[outputSchema](ctx, logger, startTime, p.g, p.model, p.temperature,
		ai.WithMessages(ai.NewUserMessage(ai.NewTextPart(promptText))))
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "gemini API call completed", "duration_ms", time.Since(startTime).Milliseconds())
	return &ExperienceSuggestionResult{
		Suggestions:  truncateSuggestions(result.Suggestions, 8),
		CategoryHint: result.CategoryHint,
	}, nil
}

// GenerateRequestSuggestions produces Plan-tab chip labels for a help request using Gemini.
func (p *GeminiProvider) GenerateRequestSuggestions(ctx context.Context, title, description, location string) (*RequestSuggestionResult, error) {
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
	result, err := geminiCallStructured[outputSchema](ctx, logger, startTime, p.g, p.model, p.temperature,
		ai.WithMessages(ai.NewUserMessage(ai.NewTextPart(promptText))))
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "gemini API call completed", "duration_ms", time.Since(startTime).Milliseconds())
	return &RequestSuggestionResult{
		AdditionalAsks:  truncateSuggestions(result.AdditionalAsks, 4),
		BreakdownPieces: truncateSuggestions(result.BreakdownPieces, 4),
		OfferIdeas:      truncateSuggestions(result.OfferIdeas, 8),
		SeedNeeds:       truncateSuggestions(result.SeedNeeds, 8),
	}, nil
}

// InferSocialAttributes estimates Social Footprint input attributes from transaction context.
func (p *GeminiProvider) InferSocialAttributes(ctx context.Context, title, description, txType string, itemValueUSD float32) (*SocialAttributeInference, error) {
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
	result, err := geminiCallStructured[outputSchema](ctx, logger, startTime, p.g, p.model, p.temperature,
		ai.WithMessages(ai.NewUserMessage(ai.NewTextPart(promptText))))
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
