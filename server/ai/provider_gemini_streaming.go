// GeminiProvider streaming implementations of the Provider interface's
// *Streaming methods. Uses Genkit's Generate with a WithStreaming callback
// that delivers response chunks as they arrive; the accumulated text is
// fed into StreamingJSONFields so watched top-level keys fire FieldEvents
// mid-response.
package ai

import (
	"context"
	"encoding/base64"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	genkitai "github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"google.golang.org/genai"

	"go.ripls.org/ripls/server/logging"
)

// geminiStreamFinal is the terminal value of a Gemini streaming call.
// Either Response (containing the typed structured output via Output()) or
// Err is populated.
type geminiStreamFinal struct {
	Response *genkitai.ModelResponse
	Err      error
}

// streamStructuredOutput drives a Genkit Generate call with a streaming
// callback, feeding each ModelResponseChunk's Text() to a
// StreamingJSONFields parser and forwarding events on the fields channel.
// The schemaInstance argument should be a zero value of the structured-
// output type (e.g., outputSchema{}); it is passed to ai.WithOutputType so
// Genkit configures the generation for JSON output matching that shape.
//
// Note on Gemini granularity: Gemini's structured-output streaming has been
// historically coarser than Anthropic/OpenAI for some models — Vertex
// Gemini 2.5 Flash typically delivers JSON in larger chunks per Server-
// Sent-Event than per-token. The tokenizer absorbs whatever granularity
// Gemini provides; the early-fire wall-clock win scales with how soon
// each watched key closes in the chunk timeline.
func (p *GeminiProvider) streamStructuredOutput(
	ctx context.Context,
	operation string,
	schemaInstance any,
	parts []*genkitai.Part,
	watchKeys []StreamFieldKey,
) (<-chan FieldEvent, <-chan geminiStreamFinal) {
	fields := make(chan FieldEvent, len(watchKeys))
	final := make(chan geminiStreamFinal, 1)

	// Bound the streaming call so a wedged upstream fails fast instead of
	// blocking the caller's Gen* RPC for minutes. Gemini's HTTP/2
	// streaming occasionally stalls indefinitely on specific inputs; the
	// timeout is the only thing that unblocks the caller in that case.
	// Image calls get a larger budget because vision models take longer.
	isImageCall := strings.Contains(operation, "Image")
	ctx, cancel := context.WithTimeout(ctx, streamTimeout(ProviderTypeGemini, p.model, isImageCall))

	go func() {
		defer cancel()
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-streamStructuredOutput",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- geminiStreamFinal{Err: fmt.Errorf("panic in gemini streamStructuredOutput: %v", r)}
			}
		}()

		startTime := time.Now()
		logger := logging.LoggerWithContext(ctx).With(
			"provider", p.Name(),
			"operation", operation,
			"model", p.model,
		)

		parser := NewStreamingJSONFields(streamFieldKeysToStrings(watchKeys))
		var firstDelta time.Time
		var cbErr error

		cb := func(_ context.Context, chunk *genkitai.ModelResponseChunk) error {
			text := chunk.Text()
			if text == "" {
				return nil
			}
			if firstDelta.IsZero() {
				firstDelta = time.Now()
			}
			evs, perr := parser.Write([]byte(text))
			if perr != nil {
				cbErr = fmt.Errorf("parse streaming JSON: %w", perr)
				return cbErr
			}
			for _, ev := range evs {
				fields <- ev
			}
			return nil
		}

		resp, err := genkit.Generate(ctx, p.g,
			genkitai.WithModelName(p.model),
			genkitai.WithConfig(&genai.GenerateContentConfig{Temperature: geminiTempPtr(p.temperature)}),
			genkitai.WithMessages(genkitai.NewUserMessage(parts...)),
			genkitai.WithOutputType(schemaInstance),
			genkitai.WithStreaming(cb),
		)
		if err != nil {
			logger.Warn("gemini streaming call failed",
				"error", err,
				"duration_ms", time.Since(startTime).Milliseconds())
			final <- geminiStreamFinal{Err: fmt.Errorf("gemini streaming call failed: %w", err)}
			return
		}
		if cbErr != nil {
			final <- geminiStreamFinal{Err: cbErr}
			return
		}

		fieldsLog := []any{
			"duration_ms", time.Since(startTime).Milliseconds(),
		}
		if !firstDelta.IsZero() {
			fieldsLog = append(fieldsLog, "streaming_first_delta_ms", firstDelta.Sub(startTime).Milliseconds())
		}
		logger.Info("gemini streaming call completed", fieldsLog...)

		final <- geminiStreamFinal{Response: resp}
	}()

	return fields, final
}

// GenerateExperienceFromTextStreaming implements the Provider interface via
// Genkit streaming. Watched keys: title, location_query, search_keywords.
func (p *GeminiProvider) GenerateExperienceFromTextStreaming(ctx context.Context, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildExperienceFromTextPrompt(prompt, region, currentTime)

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateExperienceFromTextStreaming", experienceFromTextOutput{}, []*genkitai.Part{genkitai.NewTextPart(promptText)}, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateExperienceFromTextStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in gemini GenerateExperienceFromTextStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: streamFinal.Err}
			return
		}
		var result experienceFromTextOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
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
func (p *GeminiProvider) GenerateRequestContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildRequestGenerationPrompt(prompt, region)

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateRequestContentStreaming", requestFromTextOutput{}, []*genkitai.Part{genkitai.NewTextPart(promptText)}, requestStreamingKeys)

	final := make(chan RequestStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateRequestContentStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- RequestStreamFinal{Err: fmt.Errorf("panic in gemini GenerateRequestContentStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- RequestStreamFinal{Err: streamFinal.Err}
			return
		}
		var result requestFromTextOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- RequestStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
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
func (p *GeminiProvider) GenerateCommunityContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildCommunityGenerationPrompt(prompt, region)

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateCommunityContentStreaming", communityGenerationOutput{}, []*genkitai.Part{genkitai.NewTextPart(promptText)}, communityStreamingKeys)

	final := make(chan CommunityStreamFinal, 1)
	logging.GoSafe(ctx, "gemini-community-stream-final", func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateCommunityContentStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- CommunityStreamFinal{Err: fmt.Errorf("panic in gemini GenerateCommunityContentStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- CommunityStreamFinal{Err: streamFinal.Err}
			return
		}
		var result communityGenerationOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- CommunityStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
			return
		}
		final <- CommunityStreamFinal{Result: &CommunityGeneration{
			SearchKeywords: result.SearchKeywords,
		}}
	})

	return fieldsCh, final, nil
}

// buildGeminiImagePart constructs the Genkit media part for an image input.
// Shared across the *FromImage streaming methods. URL inputs pass through to
// Genkit (which can fetch HTTP/HTTPS); raw bytes are encoded as a base64
// data URL.
func buildGeminiImagePart(image *DetectionImage) (*genkitai.Part, error) {
	hasData := len(image.ImageData) > 0
	hasURL := image.ImageURL != ""

	if !hasData && !hasURL {
		return nil, fmt.Errorf("either ImageData or ImageURL must be set")
	}
	if hasData && hasURL {
		return nil, fmt.Errorf("only one of ImageData or ImageURL can be set")
	}

	if hasURL {
		return genkitai.NewMediaPart(image.MimeType, image.ImageURL), nil
	}
	dataURL := fmt.Sprintf("data:%s;base64,%s", image.MimeType, base64.StdEncoding.EncodeToString(image.ImageData))
	return genkitai.NewMediaPart(image.MimeType, dataURL), nil
}

// DetectGearInImageStreaming implements the Provider interface. Flat
// schema — top-level title / description close mid-stream so the
// service-layer handler can emit them as FieldEvents while the AI
// call is still running. Combined with the
// gemini-2.5-flash-lite default (which streams per-token), this
// gives the same early-fire shape Anthropic gets.
func (p *GeminiProvider) DetectGearInImageStreaming(ctx context.Context, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal, error) {
	imagePart, err := buildGeminiImagePart(req)
	if err != nil {
		return nil, nil, err
	}

	prompt := buildGearDetectionPrompt()

	parts := []*genkitai.Part{imagePart, genkitai.NewTextPart(prompt)}
	fieldsCh, raw := p.streamStructuredOutput(ctx, "DetectGearInImageStreaming", gearDetectionOutput{}, parts, gearStreamingKeys)

	final := make(chan GearDetectionStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-DetectGearInImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearDetectionStreamFinal{Err: fmt.Errorf("panic in gemini DetectGearInImageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- GearDetectionStreamFinal{Err: streamFinal.Err}
			return
		}
		var result gearDetectionOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- GearDetectionStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
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

	return fieldsCh, final, nil
}

// GenerateExperienceFromImageStreaming implements the Provider interface.
// Flat schema (experienceFromImageOutput) — top-level title /
// location_query / search_keywords / date / time / time_confidence keys
// close mid-stream so the parser fires FieldEvents that drive the
// post-AI fan-out (Mapbox, Pexels) concurrently with the AI call.
// Mirrors the Anthropic + OpenAI experience-from-image schemas.
// Combined with the gemini-2.5-flash-lite default (which streams
// per-token rather than buffering until end-of-call), this delivers the
// same early-fire shape on Gemini that Anthropic gets.
func (p *GeminiProvider) GenerateExperienceFromImageStreaming(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	imagePart, err := buildGeminiImagePart(image)
	if err != nil {
		return nil, nil, err
	}

	promptText := buildExperienceFromImagePrompt(region, notes, currentTime)

	parts := []*genkitai.Part{imagePart, genkitai.NewTextPart(promptText)}
	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateExperienceFromImageStreaming", experienceFromImageOutput{}, parts, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateExperienceFromImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in gemini GenerateExperienceFromImageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: streamFinal.Err}
			return
		}
		var result experienceFromImageOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
			return
		}
		if result.Title == "" {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("no experience detected in image")}
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

// GenerateRequestFromImageStreaming implements the Provider interface. Gemini's
// structured-output streaming buffers JSON until late in the call, so the
// early-fire fan-out window on image mode is small.
// Implemented faithfully for cross-provider uniformity.
func (p *GeminiProvider) GenerateRequestFromImageStreaming(ctx context.Context, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	imagePart, err := buildGeminiImagePart(image)
	if err != nil {
		return nil, nil, err
	}

	promptText := buildRequestImageAnalysisPrompt(region)

	parts := []*genkitai.Part{imagePart, genkitai.NewTextPart(promptText)}
	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateRequestFromImageStreaming", requestFromImageOutput{}, parts, requestStreamingKeys)

	final := make(chan RequestStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateRequestFromImageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- RequestStreamFinal{Err: fmt.Errorf("panic in gemini GenerateRequestFromImageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- RequestStreamFinal{Err: streamFinal.Err}
			return
		}
		var result requestFromImageOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- RequestStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
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
func (p *GeminiProvider) GenerateGearFromTextStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt cannot be empty")
	}

	promptText := buildGearGenerationPrompt(prompt, region)

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateGearFromTextStreaming", gearFromTextOutput{}, []*genkitai.Part{genkitai.NewTextPart(promptText)}, gearStreamingKeys)

	final := make(chan GearStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateGearFromTextStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearStreamFinal{Err: fmt.Errorf("panic in gemini GenerateGearFromTextStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- GearStreamFinal{Err: streamFinal.Err}
			return
		}
		var result gearFromTextOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- GearStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
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

	return fieldsCh, final, nil
}

// GenerateExperienceFromWebpageStreaming implements the Provider interface.
func (p *GeminiProvider) GenerateExperienceFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if pageTitle == "" && pageDescription == "" && pageBody == "" {
		return nil, nil, fmt.Errorf("at least one of pageTitle, pageDescription, or pageBody must be provided")
	}

	promptText := buildExperienceFromWebpagePrompt(pageTitle, pageDescription, pageBody, region, currentTime)

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateExperienceFromWebpageStreaming", experienceFromWebpageOutput{}, []*genkitai.Part{genkitai.NewTextPart(promptText)}, experienceStreamingKeys)

	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateExperienceFromWebpageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in gemini GenerateExperienceFromWebpageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- ExperienceStreamFinal{Err: streamFinal.Err}
			return
		}
		var result experienceFromWebpageOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- ExperienceStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
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
func (p *GeminiProvider) GenerateGearFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	if pageTitle == "" && pageDescription == "" && pageBody == "" {
		return nil, nil, fmt.Errorf("at least one of pageTitle, pageDescription, or pageBody must be provided")
	}

	promptText := buildGearFromWebpagePrompt(pageTitle, pageDescription, pageBody, region)

	fieldsCh, raw := p.streamStructuredOutput(ctx, "GenerateGearFromWebpageStreaming", gearFromWebpageOutput{}, []*genkitai.Part{genkitai.NewTextPart(promptText)}, gearStreamingKeys)

	final := make(chan GearStreamFinal, 1)
	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateGearFromWebpageStreaming-consumer",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearStreamFinal{Err: fmt.Errorf("panic in gemini GenerateGearFromWebpageStreaming consumer: %v", r)}
			}
		}()
		streamFinal := <-raw
		if streamFinal.Err != nil {
			final <- GearStreamFinal{Err: streamFinal.Err}
			return
		}
		var result gearFromWebpageOutput
		if err := streamFinal.Response.Output(&result); err != nil {
			final <- GearStreamFinal{Err: fmt.Errorf("parse gemini streaming response: %w", err)}
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

	return fieldsCh, final, nil
}
