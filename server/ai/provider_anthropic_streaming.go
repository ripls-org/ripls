// AnthropicProvider streaming implementations of the Provider interface's
// *Streaming methods. Uses Anthropic's Messages.NewStreaming API with
// input_json_delta events to surface tool-input bytes per token, feeding
// them into StreamingJSONFields so the watched top-level keys fire
// FieldEvents mid-response rather than after the full reply arrives.
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

	"github.com/anthropics/anthropic-sdk-go"

	"go.ripls.org/ripls/server/logging"
)

// buildAnthropicImageBlock constructs the image content block for an
// Anthropic Messages call from a DetectionImage. Shared across the
// *FromImage streaming methods. Localhost URLs are fetched and inlined
// as base64 since Anthropic can't reach localhost; non-localhost URLs
// are passed through as URL image blocks; raw bytes are base64-encoded.
func buildAnthropicImageBlock(ctx context.Context, image *DetectionImage) (anthropic.ContentBlockParamUnion, error) {
	hasData := len(image.ImageData) > 0
	hasURL := image.ImageURL != ""

	if !hasData && !hasURL {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("either ImageData or ImageURL must be set")
	}
	if hasData && hasURL {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("only one of ImageData or ImageURL can be set")
	}

	if hasData {
		base64Data := base64.StdEncoding.EncodeToString(image.ImageData)
		return anthropic.NewImageBlockBase64(image.MimeType, base64Data), nil
	}

	if strings.Contains(image.ImageURL, "localhost") || strings.Contains(image.ImageURL, "127.0.0.1") {
		hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, image.ImageURL, nil)
		if err != nil {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("build localhost image request: %w", err)
		}
		resp, err := http.DefaultClient.Do(hreq)
		if err != nil {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("failed to fetch localhost image: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("failed to fetch localhost image: status %d", resp.StatusCode)
		}
		imageData, err := io.ReadAll(resp.Body)
		if err != nil {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("failed to read localhost image: %w", err)
		}
		mimeType := resp.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = image.MimeType
		}
		base64Data := base64.StdEncoding.EncodeToString(imageData)
		return anthropic.NewImageBlockBase64(mimeType, base64Data), nil
	}

	return anthropic.NewImageBlock(anthropic.URLImageSourceParam{URL: image.ImageURL}), nil
}

// toolCallStreamFinal is the terminal value of a streaming tool-use call.
// Exactly one of ToolInput (raw JSON bytes of the tool input) or Err is
// populated. Used internally by the Anthropic provider's streaming methods —
// the public *Streaming methods unmarshal ToolInput into their typed result
// before sending an *StreamFinal to the caller.
type toolCallStreamFinal struct {
	ToolInput json.RawMessage
	Err       error
}

// streamToolCall opens an Anthropic Messages.NewStreaming call configured
// for tool use, feeds incoming input_json_delta fragments to a
// StreamingJSONFields parser, and forwards parser events on the fields
// channel as each watched key closes. When the stream completes, the final
// tool_use block is extracted from the accumulated message and sent on the
// returned terminal channel as raw JSON bytes for the caller to unmarshal.
//
// The fields channel is buffered to len(watchKeys) so a fast Anthropic stream
// never blocks waiting for a slow consumer; in practice the service-layer
// consumer drains it immediately.
func (p *AnthropicProvider) streamToolCall(
	ctx context.Context,
	operation string,
	params anthropic.MessageNewParams,
	watchKeys []StreamFieldKey,
) (<-chan FieldEvent, <-chan toolCallStreamFinal) {
	fields := make(chan FieldEvent, len(watchKeys))
	final := make(chan toolCallStreamFinal, 1)

	// Bound the streaming call so a wedged upstream fails fast instead of
	// blocking the caller's Gen* RPC for minutes. Image calls get a larger
	// budget because vision models take longer to respond.
	isImageCall := strings.Contains(operation, "Image")
	ctx, cancel := context.WithTimeout(ctx, streamTimeout(ProviderTypeAnthropic, p.model, isImageCall))

	go func() {
		defer cancel()
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-streamToolCall",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- toolCallStreamFinal{Err: fmt.Errorf("panic in anthropic streamToolCall: %v", r)}
			}
		}()

		startTime := time.Now()
		logger := logging.LoggerWithContext(ctx).With(
			"provider", p.Name(),
			"operation", operation,
			"model", p.model,
		)

		parser := NewStreamingJSONFields(streamFieldKeysToStrings(watchKeys))
		stream := p.client.Messages.NewStreaming(ctx, params)
		msg := anthropic.Message{}
		var firstDelta time.Time

		for stream.Next() {
			event := stream.Current()
			if err := msg.Accumulate(event); err != nil {
				final <- toolCallStreamFinal{Err: fmt.Errorf("accumulate stream event: %w", err)}
				return
			}

			cbd, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent)
			if !ok {
				continue
			}
			jd, ok := cbd.Delta.AsAny().(anthropic.InputJSONDelta)
			if !ok {
				continue
			}
			if firstDelta.IsZero() {
				firstDelta = time.Now()
			}

			evs, perr := parser.Write([]byte(jd.PartialJSON))
			if perr != nil {
				logger.Warn("streaming JSON parse failed",
					"error", perr,
					"partial_so_far_bytes", len(parser.Bytes()))
				final <- toolCallStreamFinal{Err: fmt.Errorf("parse streaming JSON: %w", perr)}
				return
			}
			for _, ev := range evs {
				fields <- ev
			}
		}

		if err := stream.Err(); err != nil {
			logger.Warn("anthropic streaming call failed",
				"error", err,
				"duration_ms", time.Since(startTime).Milliseconds())
			final <- toolCallStreamFinal{Err: fmt.Errorf("anthropic streaming call failed: %w", err)}
			return
		}
		warnIfTruncated(ctx, logger, &msg)

		var toolInput json.RawMessage
		for _, block := range msg.Content {
			if block.Type == blockTypeToolUse {
				inputBytes, err := json.Marshal(block.Input)
				if err != nil {
					final <- toolCallStreamFinal{Err: fmt.Errorf("marshal tool input: %w", err)}
					return
				}
				toolInput = inputBytes
				break
			}
		}
		if toolInput == nil {
			final <- toolCallStreamFinal{Err: fmt.Errorf("anthropic returned no tool_use block")}
			return
		}

		extra := []any{}
		if !firstDelta.IsZero() {
			extra = append(extra, "streaming_first_delta_ms", firstDelta.Sub(startTime).Milliseconds())
		}
		logAPICallCompleted(logger, startTime, &msg, extra...)

		final <- toolCallStreamFinal{ToolInput: toolInput}
	}()

	return fields, final
}

// GenerateExperienceFromTextStreaming implements the Provider interface.
// Real per-token streaming via the Anthropic SDK; the watched keys (title,
// location_query, search_keywords) close mid-stream so the service layer
// can fire Mapbox + Pexels ~1 s earlier than the unary path. Schema lives
// in schemas.go (experienceFromTextOutput) — single source of truth
// across all three providers.
func (p *AnthropicProvider) GenerateExperienceFromTextStreaming(ctx context.Context, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildExperienceFromTextPrompt(prompt, region, currentTime)
	toolSchema := generateToolSchema[experienceFromTextOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   800,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(promptText)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "experience_generation")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("experience_generation"),
	}

	fields, raw := p.streamToolCall(ctx, "GenerateExperienceFromTextStreaming", params, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateExperienceFromTextStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateExperienceFromTextStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: toolFinal.Err}
			return
		}
		var result experienceFromTextOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
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

	return fields, final, nil
}

// GenerateRequestContentStreaming implements the Provider interface. Real
// per-token streaming on the request-from-text path. Watched keys: title,
// search_keywords, location_query.
func (p *AnthropicProvider) GenerateRequestContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildRequestGenerationPrompt(prompt, region)
	toolSchema := generateToolSchema[requestFromTextOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   4096,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(promptText)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "request_generation")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("request_generation"),
	}

	fields, raw := p.streamToolCall(ctx, "GenerateRequestContentStreaming", params, requestStreamingKeys)

	final := make(chan RequestStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateRequestContentStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- RequestStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateRequestContentStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- RequestStreamFinal{Err: toolFinal.Err}
			return
		}
		var result requestFromTextOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- RequestStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
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

	return fields, final, nil
}

// GenerateCommunityContentStreaming implements the Provider interface.
// Real per-token streaming on the community-from-text path. Watched key:
// search_keywords (the only field community generation emits).
func (p *AnthropicProvider) GenerateCommunityContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildCommunityGenerationPrompt(prompt, region)
	toolSchema := generateToolSchema[communityGenerationOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   anthropicDefaultMaxTokens,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(promptText)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "community_generation")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("community_generation"),
	}

	fields, raw := p.streamToolCall(ctx, "GenerateCommunityContentStreaming", params, communityStreamingKeys)

	final := make(chan CommunityStreamFinal, 1)
	logging.GoSafe(ctx, "anthropic-community-stream-final", func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateCommunityContentStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- CommunityStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateCommunityContentStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- CommunityStreamFinal{Err: toolFinal.Err}
			return
		}
		var result communityGenerationOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- CommunityStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
			return
		}
		final <- CommunityStreamFinal{Result: &CommunityGeneration{
			SearchKeywords: result.SearchKeywords,
		}}
	})

	return fields, final, nil
}

// DetectGearInImageStreaming implements the Provider interface. Flat
// schema — top-level title / description close mid-stream so the
// service-layer handler can emit them as FieldEvents while the AI
// call is still running. Mirrors the GearDetection field set
// directly; the unary GenerateGearFromImage drains this stream
// unchanged.
func (p *AnthropicProvider) DetectGearInImageStreaming(ctx context.Context, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal, error) {
	imageBlock, err := buildAnthropicImageBlock(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	prompt := buildGearDetectionPrompt()
	toolSchema := generateToolSchema[gearDetectionOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   anthropicDefaultMaxTokens,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(imageBlock, anthropic.NewTextBlock(prompt)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "gear_detection")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("gear_detection"),
	}

	fields, raw := p.streamToolCall(ctx, "DetectGearInImageStreaming", params, gearStreamingKeys)

	final := make(chan GearDetectionStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-DetectGearInImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearDetectionStreamFinal{Err: fmt.Errorf("panic in anthropic DetectGearInImageStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- GearDetectionStreamFinal{Err: toolFinal.Err}
			return
		}
		var result gearDetectionOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- GearDetectionStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
			return
		}
		if result.Title == "" {
			final <- GearDetectionStreamFinal{}
			return
		}
		final <- GearDetectionStreamFinal{Result: &GearDetection{
			Title:            result.Title,
			Description:      result.Description,
			Category:         result.Category,
			Brand:            result.Brand,
			Model:            result.Model,
			MaterialCategory: result.MaterialCategory,
			WeightGrams:      result.WeightGrams,
			Confidence:       NormalizeConfidence(float32(result.Confidence)),
			ValueEstimate:    result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fields, final, nil
}

// GenerateExperienceFromImageStreaming implements the Provider interface.
// Real per-token streaming on the experience-from-image path. Watched keys
// match the text-mode experience stream (title, location_query,
// search_keywords, date, time, time_confidence). Image-mode adds a
// `description` field that arrives mid-stream (text mode synthesizes
// description from the user's prompt server-side, image mode generates
// it via the AI), but `description` does not currently drive post-AI
// fan-out — it is in the final result for the client to surface.
func (p *AnthropicProvider) GenerateExperienceFromImageStreaming(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	imageBlock, err := buildAnthropicImageBlock(ctx, image)
	if err != nil {
		return nil, nil, err
	}

	promptText := buildExperienceFromImagePrompt(region, notes, currentTime)
	toolSchema := generateToolSchema[experienceFromImageOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   anthropicExperienceMaxTokens,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(imageBlock, anthropic.NewTextBlock(promptText)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "experience_generation")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("experience_generation"),
	}

	fields, raw := p.streamToolCall(ctx, "GenerateExperienceFromImageStreaming", params, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateExperienceFromImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateExperienceFromImageStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: toolFinal.Err}
			return
		}
		var result experienceFromImageOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
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

	return fields, final, nil
}

// GenerateRequestFromImageStreaming implements the Provider interface. Real
// per-token streaming on the request-from-image path. Watched keys: title,
// search_keywords, location_query — same as the text-mode request stream
// (image-mode adds a `description` field, which is included in the final
// result but does not drive post-AI fan-out).
func (p *AnthropicProvider) GenerateRequestFromImageStreaming(ctx context.Context, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	imageBlock, err := buildAnthropicImageBlock(ctx, image)
	if err != nil {
		return nil, nil, err
	}

	promptText := buildRequestImageAnalysisPrompt(region)
	toolSchema := generateToolSchema[requestFromImageOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   anthropicDefaultMaxTokens,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(imageBlock, anthropic.NewTextBlock(promptText)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "request_generation")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("request_generation"),
	}

	fields, raw := p.streamToolCall(ctx, "GenerateRequestFromImageStreaming", params, requestStreamingKeys)

	final := make(chan RequestStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateRequestFromImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- RequestStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateRequestFromImageStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- RequestStreamFinal{Err: toolFinal.Err}
			return
		}
		var result requestFromImageOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- RequestStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
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

	return fields, final, nil
}

// GenerateGearFromTextStreaming implements the Provider interface. Real
// per-token streaming on the gear-from-text path. Watched keys: title,
// location_query (gear has no separate search_keywords field — title doubles
// as the stock-image query).
func (p *AnthropicProvider) GenerateGearFromTextStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildGearGenerationPrompt(prompt, region)
	toolSchema := generateToolSchema[gearFromTextOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   4096,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(promptText)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "gear_generation")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("gear_generation"),
	}

	fields, raw := p.streamToolCall(ctx, "GenerateGearFromTextStreaming", params, gearStreamingKeys)

	final := make(chan GearStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateGearFromTextStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateGearFromTextStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- GearStreamFinal{Err: toolFinal.Err}
			return
		}
		var result gearFromTextOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- GearStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
			return
		}
		final <- GearStreamFinal{Result: &GearGeneration{
			Title:            result.Title,
			Category:         result.Category,
			Brand:            result.Brand,
			MaterialCategory: result.MaterialCategory,
			WeightGrams:      result.WeightGrams,
			LocationQuery:    result.LocationQuery,
			Confidence:       NormalizeConfidence(float32(result.Confidence)),
			ValueEstimate:    result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fields, final, nil
}

// GenerateExperienceFromWebpageStreaming implements the Provider interface.
// Real per-token streaming on the experience-from-webpage path. Watched keys
// match the text-mode and image-mode experience streams.
func (p *AnthropicProvider) GenerateExperienceFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if pageTitle == "" && pageDescription == "" && pageBody == "" {
		return nil, nil, fmt.Errorf("at least one of pageTitle, pageDescription, or pageBody must be provided")
	}

	promptText := buildExperienceFromWebpagePrompt(pageTitle, pageDescription, pageBody, region, currentTime)
	toolSchema := generateToolSchema[experienceFromWebpageOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   anthropicExperienceMaxTokens,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(promptText)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "experience_generation")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("experience_generation"),
	}

	fields, raw := p.streamToolCall(ctx, "GenerateExperienceFromWebpageStreaming", params, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateExperienceFromWebpageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateExperienceFromWebpageStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: toolFinal.Err}
			return
		}
		var result experienceFromWebpageOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
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

	return fields, final, nil
}

// GenerateGearFromWebpageStreaming implements the Provider interface. Real
// per-token streaming on the gear-from-webpage path. Watched keys match the
// text-mode gear stream (title, description, location_query).
func (p *AnthropicProvider) GenerateGearFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	if pageTitle == "" && pageDescription == "" && pageBody == "" {
		return nil, nil, fmt.Errorf("at least one of pageTitle, pageDescription, or pageBody must be provided")
	}

	promptText := buildGearFromWebpagePrompt(pageTitle, pageDescription, pageBody, region)
	toolSchema := generateToolSchema[gearFromWebpageOutput]()

	params := anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   anthropicDefaultMaxTokens,
		Temperature: anthropicTempParam(p.temperature),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(promptText)),
		},
		Tools:      []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, "gear_from_webpage")},
		ToolChoice: anthropic.ToolChoiceParamOfTool("gear_from_webpage"),
	}

	fields, raw := p.streamToolCall(ctx, "GenerateGearFromWebpageStreaming", params, gearStreamingKeys)

	final := make(chan GearStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateGearFromWebpageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateGearFromWebpageStreaming consumer: %v", r)}
			}
		}()
		toolFinal := <-raw
		if toolFinal.Err != nil {
			final <- GearStreamFinal{Err: toolFinal.Err}
			return
		}
		var result gearFromWebpageOutput
		if err := json.Unmarshal(toolFinal.ToolInput, &result); err != nil {
			final <- GearStreamFinal{Err: fmt.Errorf("parse anthropic streaming response: %w", err)}
			return
		}
		final <- GearStreamFinal{Result: &GearGeneration{
			Title:            result.Title,
			Description:      result.Description,
			Category:         result.Category,
			Brand:            result.Brand,
			MaterialCategory: result.MaterialCategory,
			WeightGrams:      result.WeightGrams,
			SearchKeywords:   result.SearchKeywords,
			Confidence:       NormalizeConfidence(float32(result.Confidence)),
			ValueEstimate:    result.ValueEstimate.toValueEstimate(),
		}}
	}()

	return fields, final, nil
}
