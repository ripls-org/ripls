// OpenAIProvider streaming implementations of the Provider interface's
// *Streaming methods. Uses Chat.Completions.NewStreaming with structured
// output; concatenates Delta.Content text fragments into StreamingJSONFields
// so the watched top-level keys fire FieldEvents mid-response.
package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"

	"go.ripls.org/ripls/server/logging"
)

// buildOpenAIImageContent constructs the image content part for an OpenAI
// Chat Completions call from a DetectionImage. Shared across the *FromImage
// streaming methods. Localhost URLs are fetched and inlined as a base64
// data URL since OpenAI can't reach localhost; non-localhost URLs are
// passed through; raw bytes are encoded as a data URL.
func buildOpenAIImageContent(ctx context.Context, image *DetectionImage) (openai.ChatCompletionContentPartUnionParam, error) {
	hasData := len(image.ImageData) > 0
	hasURL := image.ImageURL != ""

	if !hasData && !hasURL {
		return openai.ChatCompletionContentPartUnionParam{}, fmt.Errorf("either ImageData or ImageURL must be set")
	}
	if hasData && hasURL {
		return openai.ChatCompletionContentPartUnionParam{}, fmt.Errorf("only one of ImageData or ImageURL can be set")
	}

	if hasData {
		dataURL := fmt.Sprintf("data:%s;base64,%s", image.MimeType, base64.StdEncoding.EncodeToString(image.ImageData))
		return openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: dataURL}), nil
	}

	if strings.Contains(image.ImageURL, "localhost") || strings.Contains(image.ImageURL, "127.0.0.1") {
		hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, image.ImageURL, nil)
		if err != nil {
			return openai.ChatCompletionContentPartUnionParam{}, fmt.Errorf("build localhost image request: %w", err)
		}
		resp, err := http.DefaultClient.Do(hreq)
		if err != nil {
			return openai.ChatCompletionContentPartUnionParam{}, fmt.Errorf("failed to fetch localhost image: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return openai.ChatCompletionContentPartUnionParam{}, fmt.Errorf("failed to fetch localhost image: status %d", resp.StatusCode)
		}
		imageData, err := io.ReadAll(resp.Body)
		if err != nil {
			return openai.ChatCompletionContentPartUnionParam{}, fmt.Errorf("failed to read localhost image: %w", err)
		}
		mimeType := resp.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = image.MimeType
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(imageData))
		return openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: dataURL}), nil
	}

	return openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: image.ImageURL}), nil
}

// openaiStreamFinal is the terminal value of an OpenAI structured-output
// streaming call. ToolInput here is misnamed for symmetry with the Anthropic
// helper — for OpenAI it is the accumulated bytes of the structured-output
// JSON document (delivered as Delta.Content fragments rather than tool_use
// input deltas). Exactly one of JSON or Err is populated.
type openaiStreamFinal struct {
	JSON []byte
	Err  error
}

// streamStructuredOutput opens an OpenAI Chat Completions streaming call
// configured for structured output (ResponseFormat = JSONSchema), feeds
// incoming Delta.Content fragments to a StreamingJSONFields parser, and
// forwards parser events on the fields channel as each watched key closes.
// When the stream completes, the accumulated JSON document is sent on the
// terminal channel for the caller to unmarshal.
//
// OpenAI's structured-output streaming delivers the JSON document as a
// sequence of Delta.Content text fragments — the same shape Anthropic
// delivers via input_json_delta, just on a different field. The
// StreamingJSONFields tokenizer is provider-agnostic and consumes either
// shape unchanged.
func (p *OpenAIProvider) streamStructuredOutput(
	ctx context.Context,
	operation string,
	params openai.ChatCompletionNewParams,
	watchKeys []StreamFieldKey,
) (<-chan FieldEvent, <-chan openaiStreamFinal) {
	fields := make(chan FieldEvent, len(watchKeys))
	final := make(chan openaiStreamFinal, 1)

	// Bound the streaming call so a wedged upstream fails fast instead of
	// blocking the caller's Gen* RPC for minutes. Image calls get a larger
	// budget than text calls because vision models take longer to respond.
	isImageCall := strings.Contains(operation, "Image")
	ctx, cancel := context.WithTimeout(ctx, streamTimeout(ProviderTypeOpenAI, p.model, isImageCall))

	go func() {
		defer cancel()
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-streamStructuredOutput",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- openaiStreamFinal{Err: fmt.Errorf("panic in openai streamStructuredOutput: %v", r)}
			}
		}()

		startTime := time.Now()
		logger := logging.LoggerWithContext(ctx).With(
			"provider", p.Name(),
			"operation", operation,
			"model", p.model,
		)

		parser := NewStreamingJSONFields(streamFieldKeysToStrings(watchKeys))
		stream := p.client.Chat.Completions.NewStreaming(ctx, params)
		var firstDelta time.Time

		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta.Content
			if delta == "" {
				continue
			}
			if firstDelta.IsZero() {
				firstDelta = time.Now()
			}

			evs, perr := parser.Write([]byte(delta))
			if perr != nil {
				logger.Warn("streaming JSON parse failed",
					"error", perr,
					"partial_so_far_bytes", len(parser.Bytes()))
				final <- openaiStreamFinal{Err: fmt.Errorf("parse streaming JSON: %w", perr)}
				return
			}
			for _, ev := range evs {
				fields <- ev
			}
		}

		if err := stream.Err(); err != nil {
			logger.Warn("openai streaming call failed",
				"error", err,
				"duration_ms", time.Since(startTime).Milliseconds())
			final <- openaiStreamFinal{Err: fmt.Errorf("openai streaming call failed: %w", err)}
			return
		}

		jsonBytes := parser.Bytes()
		if len(jsonBytes) == 0 {
			final <- openaiStreamFinal{Err: fmt.Errorf("openai returned no content")}
			return
		}

		fields := []any{
			"duration_ms", time.Since(startTime).Milliseconds(),
		}
		if !firstDelta.IsZero() {
			fields = append(fields, "streaming_first_delta_ms", firstDelta.Sub(startTime).Milliseconds())
		}
		logger.Info("openai streaming call completed", fields...)

		final <- openaiStreamFinal{JSON: jsonBytes}
	}()

	return fields, final
}

// GenerateExperienceFromTextStreaming implements the Provider interface.
// Real per-token streaming via OpenAI structured output. Watched keys
// (title, location_query, search_keywords) close mid-stream so the service
// layer's Mapbox + Pexels fan-out fires the moment each key is complete.
func (p *OpenAIProvider) GenerateExperienceFromTextStreaming(ctx context.Context, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildExperienceFromTextPrompt(prompt, region, currentTime)
	schema := generateSchema[experienceFromTextOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(promptText),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "experience_generation",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateExperienceFromTextStreaming", params, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateExperienceFromTextStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in openai GenerateExperienceFromTextStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: streamFinal.Err}
			return
		}
		var result experienceFromTextOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		final <- ExperienceStreamFinal{Result: &ExperienceGeneration{
			Title:          result.Title,
			Confidence:     NormalizeConfidence(float32(result.Confidence)),
			SearchKeywords: result.SearchKeywords,
			Date:           result.Date,
			Time:           result.Time,
			TimeConfidence: result.TimeConfidence,
			LocationQuery:  result.LocationQuery,
			MentionedNames: result.MentionedNames,
			ValueEstimate:  result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fieldsCh, final, nil
}

// GenerateRequestContentStreaming implements the Provider interface.
func (p *OpenAIProvider) GenerateRequestContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildRequestGenerationPrompt(prompt, region)
	schema := generateSchema[requestFromTextOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(promptText),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "request_generation",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateRequestContentStreaming", params, requestStreamingKeys)

	final := make(chan RequestStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateRequestContentStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- RequestStreamFinal{Err: fmt.Errorf("panic in openai GenerateRequestContentStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- RequestStreamFinal{Err: streamFinal.Err}
			return
		}
		var result requestFromTextOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- RequestStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		final <- RequestStreamFinal{Result: &RequestGeneration{
			Title:          result.Title,
			SearchKeywords: result.SearchKeywords,
			LocationQuery:  result.LocationQuery,
			Confidence:     NormalizeConfidence(float32(result.Confidence)),
			ValueEstimate:  result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fieldsCh, final, nil
}

// GenerateCommunityContentStreaming implements the Provider interface.
func (p *OpenAIProvider) GenerateCommunityContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildCommunityGenerationPrompt(prompt, region)
	schema := generateSchema[communityGenerationOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(promptText),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "community_generation",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateCommunityContentStreaming", params, communityStreamingKeys)

	final := make(chan CommunityStreamFinal, 1)
	logging.GoSafe(ctx, "openai-community-stream-final", func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateCommunityContentStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- CommunityStreamFinal{Err: fmt.Errorf("panic in openai GenerateCommunityContentStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- CommunityStreamFinal{Err: streamFinal.Err}
			return
		}
		var result communityGenerationOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- CommunityStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		final <- CommunityStreamFinal{Result: &CommunityGeneration{
			SearchKeywords: result.SearchKeywords,
		}}
	})

	return fieldsCh, final, nil
}

// DetectGearInImageStreaming implements the Provider interface. Flat
// schema — top-level title / description close mid-stream so the
// service-layer handler can emit them as FieldEvents while the AI
// call is still running. Mirrors the GearDetection field set
// directly.
func (p *OpenAIProvider) DetectGearInImageStreaming(ctx context.Context, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal, error) {
	imagePart, err := buildOpenAIImageContent(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	prompt := buildGearDetectionPrompt()
	schema := generateSchema[gearDetectionOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				imagePart,
				openai.TextContentPart(prompt),
			}),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "gear_detection",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "DetectGearInImageStreaming", params, gearStreamingKeys)

	final := make(chan GearDetectionStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-DetectGearInImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearDetectionStreamFinal{Err: fmt.Errorf("panic in openai DetectGearInImageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- GearDetectionStreamFinal{Err: streamFinal.Err}
			return
		}
		var result gearDetectionOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- GearDetectionStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		if result.Title == "" {
			final <- GearDetectionStreamFinal{}
			return
		}
		logger := logging.LoggerWithContext(ctx).With(
			"provider", p.Name(),
			"operation", "DetectGearInImageStreaming",
			"model", p.model,
		)
		materialCategory, weightGrams := validateGearMetadata(
			result.MaterialCategory,
			result.WeightGrams,
			result.Title,
			result.Category,
			logger,
		)
		final <- GearDetectionStreamFinal{Result: &GearDetection{
			Title:            result.Title,
			Description:      result.Description,
			Category:         result.Category,
			Brand:            result.Brand,
			Model:            result.Model,
			MaterialCategory: materialCategory,
			WeightGrams:      weightGrams,
			Confidence:       NormalizeConfidence(float32(result.Confidence)),
			ValueEstimate:    result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fieldsCh, final, nil
}

// GenerateExperienceFromImageStreaming implements the Provider interface.
// Note that gpt-5-mini's structured-output streaming buffers JSON output
// until near end-of-call (see eval doc 2026-04-22 entry); the early-fire
// window is small in practice. Faithful to the streaming contract for
// cross-provider uniformity.
func (p *OpenAIProvider) GenerateExperienceFromImageStreaming(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	imagePart, err := buildOpenAIImageContent(ctx, image)
	if err != nil {
		return nil, nil, err
	}

	promptText := buildExperienceFromImagePrompt(region, notes, currentTime)
	schema := generateSchema[experienceFromImageOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				imagePart,
				openai.TextContentPart(promptText),
			}),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "experience_generation",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateExperienceFromImageStreaming", params, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateExperienceFromImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in openai GenerateExperienceFromImageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: streamFinal.Err}
			return
		}
		var result experienceFromImageOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		final <- ExperienceStreamFinal{Result: &ExperienceGeneration{
			Title:          result.Title,
			Description:    result.Description,
			Confidence:     NormalizeConfidence(float32(result.Confidence)),
			SearchKeywords: result.SearchKeywords,
			Date:           result.Date,
			Time:           result.Time,
			TimeConfidence: result.TimeConfidence,
			LocationQuery:  result.LocationQuery,
			ValueEstimate:  result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fieldsCh, final, nil
}

// GenerateRequestFromImageStreaming implements the Provider interface. Note
// that gpt-5-mini's structured-output streaming buffers JSON output until
// near end-of-call (see eval doc 2026-04-22 entry); this method emits the
// same shape as text mode but the early-fire fan-out window is small in
// practice. Faithful to the streaming contract for cross-provider
// uniformity.
func (p *OpenAIProvider) GenerateRequestFromImageStreaming(ctx context.Context, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	imagePart, err := buildOpenAIImageContent(ctx, image)
	if err != nil {
		return nil, nil, err
	}

	promptText := buildRequestImageAnalysisPrompt(region)
	schema := generateSchema[requestFromImageOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
				imagePart,
				openai.TextContentPart(promptText),
			}),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "request_generation",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateRequestFromImageStreaming", params, requestStreamingKeys)

	final := make(chan RequestStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateRequestFromImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- RequestStreamFinal{Err: fmt.Errorf("panic in openai GenerateRequestFromImageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- RequestStreamFinal{Err: streamFinal.Err}
			return
		}
		var result requestFromImageOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- RequestStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		final <- RequestStreamFinal{Result: &RequestGeneration{
			Title:          result.Title,
			Description:    result.Description,
			SearchKeywords: result.SearchKeywords,
			LocationQuery:  result.LocationQuery,
			Confidence:     NormalizeConfidence(float32(result.Confidence)),
			ValueEstimate:  result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fieldsCh, final, nil
}

// GenerateGearFromTextStreaming implements the Provider interface.
func (p *OpenAIProvider) GenerateGearFromTextStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildGearGenerationPrompt(prompt, region)
	schema := generateSchema[gearFromTextOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(promptText),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "gear_generation",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateGearFromTextStreaming", params, gearStreamingKeys)

	final := make(chan GearStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateGearFromTextStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearStreamFinal{Err: fmt.Errorf("panic in openai GenerateGearFromTextStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- GearStreamFinal{Err: streamFinal.Err}
			return
		}
		var result gearFromTextOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- GearStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		logger := logging.LoggerWithContext(ctx).With(
			"provider", p.Name(),
			"operation", "GenerateGearFromTextStreaming",
			"model", p.model,
		)
		materialCategory, weightGrams := validateGearMetadata(
			result.MaterialCategory,
			result.WeightGrams,
			result.Title,
			result.Category,
			logger,
		)
		final <- GearStreamFinal{Result: &GearGeneration{
			Title:            result.Title,
			Category:         result.Category,
			Brand:            result.Brand,
			MaterialCategory: materialCategory,
			WeightGrams:      weightGrams,
			LocationQuery:    result.LocationQuery,
			Confidence:       NormalizeConfidence(float32(result.Confidence)),
			ValueEstimate:    result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fieldsCh, final, nil
}

// GenerateExperienceFromWebpageStreaming implements the Provider interface.
func (p *OpenAIProvider) GenerateExperienceFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if pageTitle == "" && pageDescription == "" && pageBody == "" {
		return nil, nil, fmt.Errorf("at least one of pageTitle, pageDescription, or pageBody must be provided")
	}

	promptText := buildExperienceFromWebpagePrompt(pageTitle, pageDescription, pageBody, region, currentTime)
	schema := generateSchema[experienceFromWebpageOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(promptText),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "experience_generation",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateExperienceFromWebpageStreaming", params, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateExperienceFromWebpageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in openai GenerateExperienceFromWebpageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: streamFinal.Err}
			return
		}
		var result experienceFromWebpageOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		final <- ExperienceStreamFinal{Result: &ExperienceGeneration{
			Title:          result.Title,
			Description:    result.Description,
			Confidence:     NormalizeConfidence(float32(result.Confidence)),
			SearchKeywords: result.SearchKeywords,
			Date:           result.Date,
			Time:           result.Time,
			TimeConfidence: result.TimeConfidence,
			LocationQuery:  result.LocationQuery,
			ValueEstimate:  result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fieldsCh, final, nil
}

// GenerateGearFromWebpageStreaming implements the Provider interface.
func (p *OpenAIProvider) GenerateGearFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	if pageTitle == "" && pageDescription == "" && pageBody == "" {
		return nil, nil, fmt.Errorf("at least one of pageTitle, pageDescription, or pageBody must be provided")
	}

	promptText := buildGearFromWebpagePrompt(pageTitle, pageDescription, pageBody, region)
	schema := generateSchema[gearFromWebpageOutput]()

	params := openai.ChatCompletionNewParams{
		Model:       p.model,
		Temperature: p.temperatureParam(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(promptText),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "gear_from_webpage",
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	}

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateGearFromWebpageStreaming", params, gearStreamingKeys)

	final := make(chan GearStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateGearFromWebpageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearStreamFinal{Err: fmt.Errorf("panic in openai GenerateGearFromWebpageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- GearStreamFinal{Err: streamFinal.Err}
			return
		}
		var result gearFromWebpageOutput
		if err := json.Unmarshal(streamFinal.JSON, &result); err != nil {
			final <- GearStreamFinal{Err: fmt.Errorf("parse openai streaming response: %w", err)}
			return
		}
		logger := logging.LoggerWithContext(ctx).With(
			"provider", p.Name(),
			"operation", "GenerateGearFromWebpageStreaming",
			"model", p.model,
		)
		materialCategory, weightGrams := validateGearMetadata(
			result.MaterialCategory, result.WeightGrams, result.Title, result.Category, logger,
		)
		final <- GearStreamFinal{Result: &GearGeneration{
			Title:            result.Title,
			Description:      result.Description,
			Category:         result.Category,
			Brand:            result.Brand,
			MaterialCategory: materialCategory,
			WeightGrams:      weightGrams,
			SearchKeywords:   result.SearchKeywords,
			Confidence:       NormalizeConfidence(float32(result.Confidence)),
			ValueEstimate:    result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fieldsCh, final, nil
}
