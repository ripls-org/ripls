// Streaming variant of GenGear. Opens the streaming AI call, fires Mapbox +
// Pexels fan-out as the driving JSON fields close (Gear keys Pexels on
// `title`, not `search_keywords`), emits incremental events, and ends with
// a terminal `final` or `error`. Image mode (see #1229) emits the detected
// title mid-stream and runs post-AI gear-detection processing (product lookup +
// value estimate) after the stream closes. URL mode still falls through to
// the unary handler.

package gear

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/genai/mediacand"
	locationlib "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/streaming"
)

// maxImageCandidates caps how many alternate URLs ride on MediaReady.
// Matches the client's 4-up Replace Media row capacity. Defined per file
// rather than as a global constant so each per-type service can tune its
// own ceiling later if telemetry warrants it.
const maxImageCandidates = 4

// StreamGenGear streams AI-generated gear content.
func (s *Service) StreamGenGear(
	ctx context.Context,
	req *connect.Request[api.StreamGenGearRequest],
	stream *connect.ServerStream[api.StreamGenGearResponse],
) error {
	streamStart := time.Now()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"operation", "StreamGenGear",
	)
	logger.InfoContext(ctx, "stream started")

	sender := newGearEventSender(stream)
	defer func() {
		if cerr := sender.close(); cerr != nil {
			logger.WarnContext(ctx, "stream send failed",
				"error", cerr,
				"duration_ms", time.Since(streamStart).Milliseconds())
		}
	}()

	// Validate oneof-ish input: exactly one of prompt, media_id, or
	// website_url. URL-in-prompt is also treated as webpage mode for
	// backward compat with clients that route URLs through the prompt
	// field today.
	hasPrompt := req.Msg.Prompt != ""
	hasMedia := req.Msg.MediaId != ""
	hasWebsiteURL := req.Msg.WebsiteUrl != ""
	provided := 0
	if hasPrompt {
		provided++
	}
	if hasMedia {
		provided++
	}
	if hasWebsiteURL {
		provided++
	}
	if provided == 0 {
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"one of prompt, media_id, or website_url must be provided")
		return nil
	}
	if provided > 1 {
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"provide exactly one of prompt, media_id, or website_url")
		return nil
	}
	if s.aiProvider == nil {
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INTERNAL,
			"AI provider not configured")
		return nil
	}

	args := GenGearContext{UserID: authInfo.UserID}

	if hasWebsiteURL {
		return s.StreamGenGearFromWebpage(ctx, logger, streamStart, args, req.Msg.WebsiteUrl, sender)
	}

	// URL-in-prompt: route the extracted URL through the webpage handler
	// rather than the unary fallback. Maintains backward compat for clients
	// that still pass URLs in the prompt field.
	if hasPrompt {
		if extracted := extractURL(req.Msg.Prompt); extracted != "" {
			return s.StreamGenGearFromWebpage(ctx, logger, streamStart, args, extracted, sender)
		}
	}

	if hasMedia {
		return s.StreamGenGearFromImage(ctx, logger, streamStart, args, req.Msg.MediaId, sender)
	}

	// Text mode: streaming win.
	return s.StreamGenGearFromText(ctx, logger, streamStart, args, req.Msg.Prompt, sender)
}

// StreamGenGearFromImage runs the streaming AI gear-detection call for
// image-mode gear creation. Exported so other services (e.g.
// unified_create) can call it with their own [StreamSender] that
// bridges to a different response envelope. Image-mode gear has no
// Mapbox or Pexels branches — see StreamGenGearFromText for those.
func (s *Service) StreamGenGearFromImage(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	args GenGearContext,
	mediaID string,
	sender StreamSender,
) error {
	logger = logger.With("media_id", mediaID)

	// args.Image is the pre-resolved fast path used by unified-create:
	// the classifier already resolved the same media to a *DetectionImage,
	// so we reuse it instead of doing another GetByID + signed-URL
	// generation. Reuses the same signed URL across the classifier and
	// per-type calls, which lets Anthropic prompt-cache the image input
	// on the per-type call (#1952). Direct StreamGenGear callers pass
	// nil and we do the full resolve + ownership check.
	detectionImage := args.Image
	if detectionImage == nil {
		media := &models.Media{}
		if err := s.storage.GetByID(ctx, mediaID, media); err != nil {
			logger.WarnContext(ctx, "media not found", "error", err)
			sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
				"media not found")
			return nil
		}
		if media.UserId != args.UserID {
			sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
				"user does not have access to this media")
			return nil
		}
		bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
		presignedURL, err := s.bucket.GetSignedURL(ctx, bucketKey, 15*time.Minute)
		if err != nil {
			logger.ErrorContext(ctx, "failed to generate presigned URL", "error", err)
			sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INTERNAL,
				"failed to generate media access URL")
			return nil
		}
		detectionImage = &ai.DetectionImage{
			ImageURL: presignedURL,
			MimeType: media.ContentType,
			Filename: media.GetFilename(),
		}
	}

	streamFields, streamFinal, err := s.aiProvider.DetectGearInImageStreaming(ctx, detectionImage)
	if err != nil {
		logger.ErrorContext(ctx, "failed to open AI stream", "error", err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, err.Error())
		return nil
	}

	for ev := range streamFields {
		switch ev.Key {
		case ai.StreamFieldTitle:
			var title string
			if err := json.Unmarshal(ev.Value, &title); err == nil && title != "" {
				sender.EmitTitle(title)
			}
		case ai.StreamFieldDescription:
			var description string
			if err := json.Unmarshal(ev.Value, &description); err == nil && description != "" {
				sender.EmitDescription(description)
			}
		}
	}

	final := <-streamFinal
	if final.Err != nil {
		logger.ErrorContext(ctx, "AI streaming detection failed", "error", final.Err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, final.Err.Error())
		return nil
	}

	detection := final.Result
	_, geocodedLocation := locationlib.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)

	if detection == nil {
		logger.InfoContext(ctx, "streaming AI + fan-out completed",
			"input_mode", "image",
			"item_detected", false,
			"stream_duration_ms", time.Since(streamStart).Milliseconds())
		sender.EmitFinal(&api.GenGearResponse{})
		return nil
	}

	// The client's preview only takes location from the mid-stream
	// geocoded event (the terminal payload's location is ignored), so
	// surface the residence fallback there too — same fix as the
	// experience paths (#2684); without it image-mode items land "TBD".
	if geocodedLocation != nil {
		sender.EmitGeocoded(geocodedLocation)
	}

	detectedGear := s.buildDetectedGearFromDetection(ctx, logger, detection, mediaID, geocodedLocation)

	logger.InfoContext(ctx, "streaming AI + fan-out completed",
		"input_mode", "image",
		"item_detected", true,
		"title", detectedGear.Title,
		"brand", detectedGear.Brand,
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	sender.EmitFinal(&api.GenGearResponse{DetectedGear: detectedGear})
	return nil
}

// StreamGenGearFromText runs the streaming AI call with concurrent Pexels
// (keyed on title) + Mapbox (keyed on location_query) fan-out. Exported
// so other services (e.g. unified_create) can call it with their own
// [StreamSender] that bridges to a different response envelope.
func (s *Service) StreamGenGearFromText(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	args GenGearContext,
	prompt string,
	sender StreamSender,
) error {
	promptLength := utf8.RuneCountInString(prompt)
	if promptLength < minPromptLength {
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			fmt.Sprintf("prompt must be at least %d characters", minPromptLength))
		return nil
	}
	if promptLength > maxPromptLength {
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			fmt.Sprintf("prompt must not exceed %d characters", maxPromptLength))
		return nil
	}
	logger = logger.With("prompt_length", promptLength)

	// Description in text mode IS the user's prompt — gearFromTextOutput
	// has no description field (see server/ai/schemas.go) and the server
	// later stamps generation.Description = prompt verbatim. Emit it now
	// so the client renders it during streaming instead of having to dig
	// description out of the terminal final event. (Image and webpage
	// modes emit description from the AI's structured output as it
	// streams in — no equivalent pre-emit needed there.)
	sender.EmitDescription(prompt)

	// Gear uses empty region for AI context today.
	region := ""

	streamFields, streamFinal, err := s.aiProvider.GenerateGearFromTextStreaming(ctx, prompt, region)
	if err != nil {
		logger.ErrorContext(ctx, "failed to open AI stream", "error", err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, err.Error())
		return nil
	}

	var (
		stockMediaID string
		branchWG     sync.WaitGroup
	)
	mediaFired := false
	skipStockMedia := s.stockImageryProvider == nil

	for ev := range streamFields {
		switch ev.Key {
		case ai.StreamFieldTitle:
			var title string
			if err := json.Unmarshal(ev.Value, &title); err != nil || title == "" {
				continue
			}
			sender.EmitTitle(title)
			// The prompt went out as the description before a title
			// existed; now that one does, drop the clause it duplicates so
			// the listing doesn't read with its own heading repeated
			// underneath it (#2724). A no-op unless the title is the
			// prompt's opening clause.
			if trimmed := ai.DescriptionWithoutTitleClause(prompt, title); trimmed != prompt {
				sender.EmitDescription(trimmed)
			}
			if skipStockMedia {
				continue
			}
			// Gear fires Pexels off title (its stock-image query).
			mediaFired = true
			branchWG.Add(1)
			titleCopy := title
			logging.GoSafe(ctx, "stream-gen-gear-media", func() {
				defer branchWG.Done()
				id := s.fetchStockImage(ctx, titleCopy, args.UserID)
				if id == "" {
					return
				}
				stockMediaID = id
				// Also fetch alternates from the same Pexels query so
				// the client's Replace Media modal can offer one-tap
				// swaps. Best-effort: empty/erroring candidates fall
				// back to a candidate-less MediaReady.
				var cands []*api.MediaCandidate
				if s.stockImageryProvider != nil {
					stockCands, cErr := s.stockImageryProvider.SearchStockImageCandidates(ctx, titleCopy, maxImageCandidates)
					if cErr != nil {
						logger.WarnContext(ctx, "stock candidate search failed", "error", cErr)
					} else {
						cands = mediacand.StockImageCandidatesToAPI(stockCands)
					}
				}
				if len(cands) > 0 {
					sender.EmitMediaReadyWithCandidates([]string{id}, cands)
				} else {
					sender.EmitMediaReady([]string{id})
				}
			})

		case ai.StreamFieldDescription:
			var description string
			if err := json.Unmarshal(ev.Value, &description); err == nil && description != "" {
				sender.EmitDescription(description)
			}
		}
	}

	branchWG.Wait()
	final := <-streamFinal
	if final.Err != nil {
		ai.LogAIError(ctx, logger, "AI streaming generation failed", final.Err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, final.Err.Error())
		return nil
	}

	generation := final.Result
	ai.SanitizeGearGeneration(generation)
	generation.Description = prompt

	logger.InfoContext(ctx, "streaming AI + fan-out completed",
		"title", generation.Title,
		"media_fired", mediaFired,
		"media_hit", stockMediaID != "",
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	mediaIds := []string{}
	if stockMediaID != "" {
		mediaIds = append(mediaIds, stockMediaID)
	}

	// Gear has no location-provider wiring today; the only fallback path
	// is "" (no AI-extracted query), where we resolve the user's primary
	// location from storage.
	var geocodedLocation *api.GeocodedLocation
	if generation.LocationQuery == "" {
		_, geocodedLocation = locationlib.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
	}
	// Surface the fallback mid-stream — the client's preview ignores the
	// terminal payload's location (see the experience paths, #2684).
	if geocodedLocation != nil {
		sender.EmitGeocoded(geocodedLocation)
	}

	finalResp := s.buildGenGearResponse(generation, mediaIds, geocodedLocation)
	sender.EmitFinal(finalResp)
	return nil
}

// streamGenGearFromWebpage runs the streaming AI gear-from-webpage call.
// Fetches the webpage synchronously up front, opens
// GenerateGearFromWebpageStreaming, emits the title mid-stream as it
// closes, and runs the post-AI gear assembly (user-primary-location
// fallback, webpage-image download with stock-image fallback,
// DetectedGearItem with SourceUrl) once the stream completes. Mirrors
// the unary genGearFromURL behavior — image download today happens
// after AI completion; parallelizing it with the AI call is a follow-up.
// StreamGenGearFromWebpage runs the streaming AI gear-from-webpage call.
// Exported so other services can call it with their own [StreamSender].
// Fetches the webpage synchronously up front, opens the AI stream, emits
// the title mid-stream as it closes, and runs the post-AI gear assembly
// (user-primary-location fallback, webpage-image download with stock-image
// fallback, DetectedGearItem with SourceUrl) once the stream completes.
func (s *Service) StreamGenGearFromWebpage(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	args GenGearContext,
	websiteURL string,
	sender StreamSender,
) error {
	logger = logger.With("website_url", websiteURL)

	if s.webFetcher == nil {
		logger.ErrorContext(ctx, "web fetcher not configured")
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INTERNAL,
			"web fetcher not configured")
		return nil
	}

	pageContent, err := s.webFetcher.FetchPageContent(ctx, websiteURL)
	if err != nil {
		logger.WarnContext(ctx, "failed to fetch webpage", "error", err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"failed to fetch webpage")
		return nil
	}
	if pageContent.Title == "" && pageContent.Description == "" && pageContent.BodyText == "" {
		logger.WarnContext(ctx, "webpage returned no extractable content")
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"no content extractable from URL")
		return nil
	}

	region := ""

	streamFields, streamFinal, err := s.aiProvider.GenerateGearFromWebpageStreaming(
		ctx, pageContent.Title, pageContent.Description, pageContent.BodyText, region,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to open AI stream", "error", err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, err.Error())
		return nil
	}

	for ev := range streamFields {
		switch ev.Key {
		case ai.StreamFieldTitle:
			var title string
			if err := json.Unmarshal(ev.Value, &title); err == nil && title != "" {
				sender.EmitTitle(title)
			}
		case ai.StreamFieldDescription:
			var description string
			if err := json.Unmarshal(ev.Value, &description); err == nil && description != "" {
				sender.EmitDescription(description)
			}
		}
	}

	final := <-streamFinal
	if final.Err != nil {
		ai.LogAIError(ctx, logger, "AI streaming generation failed", final.Err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, final.Err.Error())
		return nil
	}

	generation := final.Result
	ai.SanitizeGearGeneration(generation)

	// User-primary-location fallback (gear webpage flow has no Mapbox
	// branch; same as the unary path).
	_, geocodedLocation := locationlib.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
	// Surface the fallback mid-stream — the client's preview ignores the
	// terminal payload's location (see the experience paths, #2684).
	if geocodedLocation != nil {
		sender.EmitGeocoded(geocodedLocation)
	}

	mediaIds := []string{}
	if pageContent.ImageURL != "" {
		webpageMediaID, dlErr := s.downloadAndStoreWebpageImage(ctx, pageContent.ImageURL, args.UserID)
		if dlErr != nil {
			logger.WarnContext(ctx, "failed to download webpage image",
				"image_url", pageContent.ImageURL, "error", dlErr)
		} else if webpageMediaID != "" {
			mediaIds = append(mediaIds, webpageMediaID)
		}
	}
	if len(mediaIds) == 0 && s.stockImageryProvider != nil && generation.Title != "" {
		stockMediaID, stockErr := s.findAndStoreStockImage(ctx, generation.Title, args.UserID)
		if stockErr != nil {
			logger.WarnContext(ctx, "failed to get stock image", "error", stockErr)
		} else if stockMediaID != "" {
			mediaIds = append(mediaIds, stockMediaID)
		}
	}
	// Surface up to 4 alternate candidate URLs the page also offered.
	// The first entry is the picked image (already downloaded above),
	// so skip it; entries [1:] are alternates the client may render in
	// the Replace Media modal.
	var candidates []*api.MediaCandidate
	if len(pageContent.ImageURLs) > 1 {
		alternates := pageContent.ImageURLs[1:]
		if len(alternates) > maxImageCandidates {
			alternates = alternates[:maxImageCandidates]
		}
		candidates = mediacand.ImageURLsToCandidates(alternates)
	}
	// Stock-image fallback path: when the webpage produced no image but
	// the stock provider supplied one, surface alternates from the same
	// stock query so the user can swap in one tap.
	if len(candidates) == 0 && s.stockImageryProvider != nil && generation.Title != "" {
		stockCands, stockCErr := s.stockImageryProvider.SearchStockImageCandidates(ctx, generation.Title, maxImageCandidates)
		if stockCErr != nil {
			logger.WarnContext(ctx, "failed to fetch stock image candidates", "error", stockCErr)
		} else if len(stockCands) > 0 {
			candidates = mediacand.StockImageCandidatesToAPI(stockCands)
		}
	}
	if len(mediaIds) > 0 || len(candidates) > 0 {
		sender.EmitMediaReadyWithCandidates(mediaIds, candidates)
	}

	logger.InfoContext(ctx, "streaming AI + fan-out completed",
		"input_mode", "webpage",
		"title", generation.Title,
		"media_hit", len(mediaIds) > 0,
		"used_webpage_image", pageContent.ImageURL != "",
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	detectedGear := &api.DetectedGearItem{
		Title:            generation.Title,
		Description:      generation.Description,
		Confidence:       generation.Confidence,
		GeocodedLocation: geocodedLocation,
		LocationQuery:    generation.LocationQuery,
		MediaIds:         mediaIds,
		SourceUrl:        websiteURL,
		Category:         generation.Category,
		Brand:            generation.Brand,
		MaterialCategory: ai.MaterialCategoryFromJSON(generation.MaterialCategory),
	}
	if generation.WeightGrams > 0 {
		detectedGear.WeightGrams = &api.Estimate{Mean: generation.WeightGrams, Stddev: generation.WeightGrams * 0.3}
	}
	if generation.ValueEstimate != nil {
		veProvenance := &api.Provenance{
			Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Name:       "genai_value_estimate",
			Confidence: proto.Float32(float32(generation.ValueEstimate.Confidence)),
			Sources:    generation.ValueEstimate.Sources,
		}
		if s.estimatorCfg != nil {
			veProvenance.Version = s.estimatorCfg.ProvenanceVersion("genai_value_estimate")
		}
		detectedGear.ValueEstimate = &api.ValueEstimate{
			EstimatedValueUsd: generation.ValueEstimate.EstimatedValueUSD,
			Provenance:        veProvenance,
		}
	}

	sender.EmitFinal(&api.GenGearResponse{DetectedGear: detectedGear})
	return nil
}

// buildGenGearResponse assembles the terminal `final` response.
func (s *Service) buildGenGearResponse(
	generation *ai.GearGeneration,
	mediaIds []string,
	geocodedLocation *api.GeocodedLocation,
) *api.GenGearResponse {
	detected := &api.DetectedGearItem{
		Title:            generation.Title,
		Description:      generation.Description,
		Confidence:       generation.Confidence,
		GeocodedLocation: geocodedLocation,
		LocationQuery:    generation.LocationQuery,
		MediaIds:         mediaIds,
		Category:         generation.Category,
		Brand:            generation.Brand,
		MaterialCategory: ai.MaterialCategoryFromJSON(generation.MaterialCategory),
	}
	if generation.WeightGrams > 0 {
		detected.WeightGrams = &api.Estimate{Mean: generation.WeightGrams, Stddev: generation.WeightGrams * 0.3}
	}
	if generation.ValueEstimate != nil {
		veProvenance := &api.Provenance{
			Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Name:       "genai_value_estimate",
			Confidence: proto.Float32(float32(generation.ValueEstimate.Confidence)),
			Sources:    generation.ValueEstimate.Sources,
		}
		if s.estimatorCfg != nil {
			veProvenance.Version = s.estimatorCfg.ProvenanceVersion("genai_value_estimate")
		}
		detected.ValueEstimate = &api.ValueEstimate{
			EstimatedValueUsd: generation.ValueEstimate.EstimatedValueUSD,
			Provenance:        veProvenance,
		}
	}
	return &api.GenGearResponse{DetectedGear: detected}
}

// gearEventSender wraps streaming.Sender with typed emit helpers.
type gearEventSender struct {
	*streaming.Sender[api.StreamGenGearResponse]
}

func newGearEventSender(target streaming.SendTarget[api.StreamGenGearResponse]) *gearEventSender {
	return &gearEventSender{Sender: streaming.NewSender[api.StreamGenGearResponse](target)}
}

func (s *gearEventSender) emitTitle(title string) {
	s.Emit(&api.StreamGenGearResponse{Event: &api.StreamGenGearResponse_Title{Title: title}})
}

func (s *gearEventSender) emitDescription(description string) {
	s.Emit(&api.StreamGenGearResponse{Event: &api.StreamGenGearResponse_Description{Description: description}})
}

func (s *gearEventSender) emitGeocoded(g *api.GeocodedLocation) {
	s.Emit(&api.StreamGenGearResponse{Event: &api.StreamGenGearResponse_Geocoded{Geocoded: g}})
}

func (s *gearEventSender) emitMediaReady(mediaIDs []string) {
	s.emitMediaReadyWithCandidates(mediaIDs, nil)
}

func (s *gearEventSender) emitMediaReadyWithCandidates(mediaIDs []string, candidates []*api.MediaCandidate) {
	s.Emit(&api.StreamGenGearResponse{
		Event: &api.StreamGenGearResponse_MediaReady{
			MediaReady: &api.MediaReady{
				MediaIds:   mediaIDs,
				Candidates: candidates,
			},
		},
	})
}

func (s *gearEventSender) emitFinal(resp *api.GenGearResponse) {
	s.Emit(&api.StreamGenGearResponse{Event: &api.StreamGenGearResponse_Final{Final: resp}})
}

func (s *gearEventSender) emitError(code api.GenStreamErrorCode, message string) {
	s.Emit(&api.StreamGenGearResponse{
		Event: &api.StreamGenGearResponse_Error{
			Error: &api.GenStreamError{Code: code, Message: message},
		},
	})
}

func (s *gearEventSender) close() error { return s.Close() }
