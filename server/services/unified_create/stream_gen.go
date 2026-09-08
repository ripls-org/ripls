package unified_create

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/genai/experiencegen"
	"go.ripls.org/ripls/server/genai/geargen"
	"go.ripls.org/ripls/server/genai/requestgen"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/streaming"
	"go.ripls.org/ripls/server/webfetch"
)

// StreamGenUnifiedCreate streams a unified-create generation. Wraps the
// connect.ServerStream into a streaming.SendTarget so the bulk of the
// handler is testable without spinning up an HTTP server.
func (s *Service) StreamGenUnifiedCreate(
	ctx context.Context,
	req *connect.Request[api.StreamGenUnifiedCreateRequest],
	stream *connect.ServerStream[api.StreamGenUnifiedCreateResponse],
) error {
	return s.streamGenUnifiedCreate(ctx, req.Msg, streamTargetAdapter{stream: stream})
}

// streamTargetAdapter adapts connect.ServerStream to streaming.SendTarget.
type streamTargetAdapter struct {
	stream *connect.ServerStream[api.StreamGenUnifiedCreateResponse]
}

func (a streamTargetAdapter) Send(ev *api.StreamGenUnifiedCreateResponse) error {
	return a.stream.Send(ev)
}

// streamGenUnifiedCreate is the testable core. Validates the input, runs
// the classifier (or honours force_type), emits a type event, then
// delegates to the matched single-create streaming generator via a
// per-type adapter. All fan-out, fallback, time-extraction, and
// final-assembly happens in the per-type service — unified_create owns
// only classification and envelope translation.
func (s *Service) streamGenUnifiedCreate(
	ctx context.Context,
	msg *api.StreamGenUnifiedCreateRequest,
	target streaming.SendTarget[api.StreamGenUnifiedCreateResponse],
) error {
	streamStart := time.Now()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"operation", "StreamGenUnifiedCreate",
		"prompt_version", ai.UnifiedCreateClassifierPromptVersion,
	)
	logger.InfoContext(ctx, "stream started")

	emitError := func(code api.GenStreamErrorCode, m string) error {
		return target.Send(&api.StreamGenUnifiedCreateResponse{
			Event: &api.StreamGenUnifiedCreateResponse_Error{
				Error: &api.GenStreamError{Code: code, Message: m},
			},
		})
	}

	// Validate prompt oneof: exactly one of text / media_id / website_url.
	hasText := msg.GetText() != ""
	hasMedia := msg.GetMediaId() != ""
	hasURL := msg.GetWebsiteUrl() != ""
	count := 0
	if hasText {
		count++
	}
	if hasMedia {
		count++
	}
	if hasURL {
		count++
	}
	if count == 0 {
		logger.WarnContext(ctx, "no input provided")
		return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"one of text, media_id, or website_url must be provided")
	}
	if count > 1 {
		logger.WarnContext(ctx, "multiple inputs provided")
		return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"provide exactly one of text, media_id, or website_url")
	}

	inputMode := "text"
	if hasMedia {
		inputMode = "image"
	}
	if hasURL {
		inputMode = "url"
	}

	// URL mode: fetch the page once up front so the classifier and the
	// per-type generator both work off the same fetched content. (The
	// per-type webpage handler re-fetches today — a follow-up could
	// expose a *FromWebpageContent variant that accepts pre-fetched
	// content to avoid the double-fetch.)
	var pageContent *webfetch.PageContent
	if hasURL {
		if s.fetcher == nil {
			return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INTERNAL,
				"URL mode requires web fetcher configured")
		}
		fetchStart := time.Now()
		pc, err := s.fetcher.FetchPageContent(ctx, msg.GetWebsiteUrl())
		if err != nil {
			logger.WarnContext(ctx, "webpage fetch failed",
				"url", msg.GetWebsiteUrl(),
				"error", err,
				"duration_ms", time.Since(fetchStart).Milliseconds())
			return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
				"could not fetch the URL")
		}
		pageContent = pc
		logger.InfoContext(ctx, "webpage fetched",
			"url", msg.GetWebsiteUrl(),
			"title_len", len(pc.Title),
			"description_len", len(pc.Description),
			"body_len", len(pc.BodyText),
			"duration_ms", time.Since(fetchStart).Milliseconds())
	}

	// Image mode: resolve media_id to a *DetectionImage up front so the
	// classifier sees the actual image content (#1939) instead of a
	// text-only "imagine an image is attached" prompt. The same value
	// is reused by the per-type image-stream call to avoid double-fetching
	// the media row + signed URL.
	var classifierImage *ai.DetectionImage
	if hasMedia {
		resolveStart := time.Now()
		img, err := s.resolveMediaForClassifier(ctx, msg.GetMediaId(), authInfo.UserID)
		if err != nil {
			logger.ErrorContext(ctx, "media resolution failed",
				"operation", "ResolveClassifierMedia",
				"media_id", msg.GetMediaId(),
				"error", err,
				"duration_ms", time.Since(resolveStart).Milliseconds())
			return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INTERNAL,
				"could not resolve media for classification")
		}
		classifierImage = img
		logger.InfoContext(ctx, "media resolved",
			"operation", "ResolveClassifierMedia",
			"media_id", msg.GetMediaId(),
			"mime_type", img.MimeType,
			"duration_ms", time.Since(resolveStart).Milliseconds())
	}

	// Determine type: force_type short-circuits the classifier.
	var detected api.DetectedContentType
	forceTypeUsed := false
	classifierStart := time.Now()
	var classifierDuration time.Duration

	if ft := msg.ForceType; ft != nil {
		detected = *ft
		forceTypeUsed = true
		if detected == api.DetectedContentType_DETECTED_CONTENT_TYPE_UNSPECIFIED {
			return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
				"force_type cannot be DETECTED_CONTENT_TYPE_UNSPECIFIED")
		}
	} else {
		classifierInput := ClassifierInput{
			Text:       msg.GetText(),
			Image:      classifierImage,
			WebsiteURL: msg.GetWebsiteUrl(),
		}
		if pageContent != nil {
			classifierInput.WebsiteTitle = pageContent.Title
			classifierInput.WebsiteDescription = pageContent.Description
		}
		result, err := s.classifier.Classify(ctx, classifierInput)
		classifierDuration = time.Since(classifierStart)
		if err != nil {
			ai.LogAIError(ctx, logger, "classifier failed", err,
				"classifier_duration_ms", classifierDuration.Milliseconds())
			return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED,
				"classification failed")
		}
		detected = detectedTypeFromString(string(result.Type))
		if detected == api.DetectedContentType_DETECTED_CONTENT_TYPE_UNSPECIFIED {
			logger.WarnContext(ctx, "classifier returned unknown type",
				"raw_type", string(result.Type))
			return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED,
				"classifier returned unknown type")
		}
		logger.InfoContext(ctx, "classified",
			"detected_type", string(result.Type),
			"classifier_duration_ms", classifierDuration.Milliseconds())
	}

	// Emit type event.
	if err := target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Type{Type: detected},
	}); err != nil {
		return fmt.Errorf("send type event: %w", err)
	}

	if s.aiProvider == nil {
		logger.ErrorContext(ctx, "AI provider not configured")
		return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INTERNAL,
			"AI provider not configured")
	}

	perTypeStart := time.Now()
	var (
		final  *api.StreamGenUnifiedCreateFinal
		genErr error
	)
	switch {
	case hasMedia:
		final, genErr = s.streamPerTypeImage(ctx, logger, authInfo.UserID, msg, detected, classifierImage, target)
	case hasURL:
		final, genErr = s.streamPerTypeWebpage(ctx, logger, authInfo.UserID, msg, pageContent, detected, target)
	default:
		final, genErr = s.streamPerTypeText(ctx, logger, authInfo.UserID, msg, detected, target)
	}
	perTypeDuration := time.Since(perTypeStart)
	if final != nil {
		logger.InfoContext(ctx, "per-type generation returned",
			"detected_type", detected.String(),
			"title", titleFromFinal(final),
			"description_len", len(descriptionFromFinal(final)),
			"per_type_duration_ms", perTypeDuration.Milliseconds(),
		)
	}
	if genErr != nil {
		ai.LogAIError(ctx, logger, "per-type generation failed", genErr,
			"detected_type", detected.String(),
			"per_type_duration_ms", perTypeDuration.Milliseconds())
		return emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED,
			"per-type generation failed")
	}
	if err := target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Final{Final: final},
	}); err != nil {
		return fmt.Errorf("send final: %w", err)
	}

	logger.InfoContext(ctx, "stream completed",
		"detected_type", detected.String(),
		"force_type_used", forceTypeUsed,
		"input_mode", inputMode,
		"classifier_duration_ms", classifierDuration.Milliseconds(),
		"total_duration_ms", time.Since(streamStart).Milliseconds(),
	)
	return nil
}

// streamPerTypeText delegates text-mode generation to the matching
// single-create streaming generator via a per-type adapter that bridges
// emits into the unified envelope.
func (s *Service) streamPerTypeText(
	ctx context.Context,
	logger *logging.Logger,
	userID string,
	msg *api.StreamGenUnifiedCreateRequest,
	t api.DetectedContentType,
	target streaming.SendTarget[api.StreamGenUnifiedCreateResponse],
) (*api.StreamGenUnifiedCreateFinal, error) {
	out := &api.StreamGenUnifiedCreateFinal{Type: t}
	start := time.Now()
	switch t {
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT:
		if s.experienceService == nil {
			return nil, fmt.Errorf("experience service not configured")
		}
		args := experiencegen.Context{
			UserID:             userID,
			LocationID:         msg.GetLocationId(),
			Timezone:           msg.GetTimezone(),
			CurrentTimeUnixSec: msg.GetCurrentTimeUnixSec(),
			LatitudeDeg:        msg.GetLatitudeDeg(),
			LongitudeDeg:       msg.GetLongitudeDeg(),
		}
		adapter := &experienceAdapter{target: target, out: out}
		if err := s.experienceService.StreamGenExperienceFromText(ctx, logger, start, args, msg.GetText(), adapter); err != nil {
			return nil, err
		}
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_GEAR:
		if s.gearService == nil {
			return nil, fmt.Errorf("gear service not configured")
		}
		args := geargen.Context{UserID: userID}
		adapter := &gearAdapter{target: target, out: out}
		if err := s.gearService.StreamGenGearFromText(ctx, logger, start, args, msg.GetText(), adapter); err != nil {
			return nil, err
		}
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_REQUEST:
		if s.requestService == nil {
			return nil, fmt.Errorf("request service not configured")
		}
		args := requestgen.Context{
			UserID:       userID,
			LocationID:   msg.GetLocationId(),
			LatitudeDeg:  msg.GetLatitudeDeg(),
			LongitudeDeg: msg.GetLongitudeDeg(),
		}
		adapter := &requestAdapter{target: target, out: out}
		if err := s.requestService.StreamGenRequestFromText(ctx, logger, start, args, msg.GetText(), adapter); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("streamPerTypeText: unsupported type %v", t)
	}
	return out, nil
}

// streamPerTypeImage delegates image-mode generation to the matching
// single-create streaming generator. The single-create handlers already
// stamp the user-uploaded media on the final and emit media_ready
// themselves (gear has its own + experience/request prepend the uploaded
// media to the response media list), so the adapter passes through
// unchanged.
//
// classifierImage is the *DetectionImage the classifier already resolved
// for this request. Passed forward via Context.Image so the per-type
// service skips its own storage.GetByID + bucket.GetSignedURL and reuses
// the same signed URL — enabling Anthropic prompt-cache hits on the
// per-type call when paired with cache_control markers (#1952).
func (s *Service) streamPerTypeImage(
	ctx context.Context,
	logger *logging.Logger,
	userID string,
	msg *api.StreamGenUnifiedCreateRequest,
	t api.DetectedContentType,
	classifierImage *ai.DetectionImage,
	target streaming.SendTarget[api.StreamGenUnifiedCreateResponse],
) (*api.StreamGenUnifiedCreateFinal, error) {
	out := &api.StreamGenUnifiedCreateFinal{Type: t}
	start := time.Now()
	mediaID := msg.GetMediaId()
	switch t {
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT:
		if s.experienceService == nil {
			return nil, fmt.Errorf("experience service not configured")
		}
		args := experiencegen.Context{
			UserID:             userID,
			LocationID:         msg.GetLocationId(),
			Timezone:           msg.GetTimezone(),
			CurrentTimeUnixSec: msg.GetCurrentTimeUnixSec(),
			LatitudeDeg:        msg.GetLatitudeDeg(),
			LongitudeDeg:       msg.GetLongitudeDeg(),
			Image:              classifierImage,
		}
		adapter := &experienceAdapter{target: target, out: out}
		if err := s.experienceService.StreamGenExperienceFromImage(ctx, logger, start, args, mediaID, adapter); err != nil {
			return nil, err
		}
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_GEAR:
		if s.gearService == nil {
			return nil, fmt.Errorf("gear service not configured")
		}
		args := geargen.Context{
			UserID: userID,
			Image:  classifierImage,
		}
		adapter := &gearAdapter{target: target, out: out}
		if err := s.gearService.StreamGenGearFromImage(ctx, logger, start, args, mediaID, adapter); err != nil {
			return nil, err
		}
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_REQUEST:
		if s.requestService == nil {
			return nil, fmt.Errorf("request service not configured")
		}
		args := requestgen.Context{
			UserID:       userID,
			LocationID:   msg.GetLocationId(),
			LatitudeDeg:  msg.GetLatitudeDeg(),
			LongitudeDeg: msg.GetLongitudeDeg(),
			Image:        classifierImage,
		}
		adapter := &requestAdapter{target: target, out: out}
		if err := s.requestService.StreamGenRequestFromImage(ctx, logger, start, args, mediaID, adapter); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("streamPerTypeImage: unsupported type %v", t)
	}
	return out, nil
}

// streamPerTypeWebpage delegates URL-mode generation. Experience and gear
// have native webpage handlers; request synthesises a text prompt from
// the fetched page title + description and routes through the text-mode
// generator (no request webpage variant exists). The per-type handlers
// re-fetch the URL today even though we have pageContent in hand — a
// follow-up should expose *FromWebpageContent variants on experience and
// gear so the up-front fetch can be reused.
func (s *Service) streamPerTypeWebpage(
	ctx context.Context,
	logger *logging.Logger,
	userID string,
	msg *api.StreamGenUnifiedCreateRequest,
	pageContent *webfetch.PageContent,
	t api.DetectedContentType,
	target streaming.SendTarget[api.StreamGenUnifiedCreateResponse],
) (*api.StreamGenUnifiedCreateFinal, error) {
	out := &api.StreamGenUnifiedCreateFinal{Type: t}
	start := time.Now()
	switch t {
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_EVENT:
		if s.experienceService == nil {
			return nil, fmt.Errorf("experience service not configured")
		}
		args := experiencegen.Context{
			UserID:             userID,
			LocationID:         msg.GetLocationId(),
			Timezone:           msg.GetTimezone(),
			CurrentTimeUnixSec: msg.GetCurrentTimeUnixSec(),
			LatitudeDeg:        msg.GetLatitudeDeg(),
			LongitudeDeg:       msg.GetLongitudeDeg(),
		}
		adapter := &experienceAdapter{target: target, out: out}
		if err := s.experienceService.StreamGenExperienceFromWebpage(ctx, logger, start, args, msg.GetWebsiteUrl(), adapter); err != nil {
			return nil, err
		}
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_GEAR:
		if s.gearService == nil {
			return nil, fmt.Errorf("gear service not configured")
		}
		args := geargen.Context{UserID: userID}
		adapter := &gearAdapter{target: target, out: out}
		if err := s.gearService.StreamGenGearFromWebpage(ctx, logger, start, args, msg.GetWebsiteUrl(), adapter); err != nil {
			return nil, err
		}
	case api.DetectedContentType_DETECTED_CONTENT_TYPE_REQUEST:
		if s.requestService == nil {
			return nil, fmt.Errorf("request service not configured")
		}
		// Synthesise a text prompt from the page title + description.
		// Request has no native webpage streaming variant; this mirrors
		// the original unified-create webpage fallback.
		synthPrompt := pageContent.Title
		if pageContent.Description != "" {
			if synthPrompt != "" {
				synthPrompt += ". "
			}
			synthPrompt += pageContent.Description
		}
		args := requestgen.Context{
			UserID:       userID,
			LocationID:   msg.GetLocationId(),
			LatitudeDeg:  msg.GetLatitudeDeg(),
			LongitudeDeg: msg.GetLongitudeDeg(),
		}
		adapter := &requestAdapter{target: target, out: out}
		if err := s.requestService.StreamGenRequestFromText(ctx, logger, start, args, synthPrompt, adapter); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("streamPerTypeWebpage: unsupported type %v", t)
	}
	return out, nil
}

// titleFromFinal returns the title field of whichever per-type payload
// is set on the final, or empty string if none. Used for the dispatcher
// log line; the per-type generators are the source of truth for what
// ends up on the wire.
func titleFromFinal(f *api.StreamGenUnifiedCreateFinal) string {
	if g := f.GetGear(); g != nil && g.GetDetectedGear() != nil {
		return g.GetDetectedGear().GetTitle()
	}
	if e := f.GetExperience(); e != nil {
		return e.GetName()
	}
	if r := f.GetRequest(); r != nil {
		return r.GetTitle()
	}
	return ""
}

// resolveMediaForClassifier fetches the media row for mediaID, verifies
// ownership, and produces a *DetectionImage carrying a 15-minute
// presigned URL plus the media's content type. Returns an error on any
// of: missing media, permission denied, signed-URL generation failure.
// Caller is responsible for translating the error into a stream-event
// emit + log line.
//
// The presigned URL is intentionally not returned alongside the
// DetectionImage in a way that lands in logs — it carries time-limited
// credentials in its query string. The caller should log media_id, not
// the URL.
func (s *Service) resolveMediaForClassifier(ctx context.Context, mediaID, userID string) (*ai.DetectionImage, error) {
	if s.bucket == nil {
		return nil, fmt.Errorf("bucket storage not configured")
	}
	media := &models.Media{}
	if err := s.storage.GetByID(ctx, mediaID, media); err != nil {
		return nil, fmt.Errorf("get media: %w", err)
	}
	if media.UserId != userID {
		return nil, fmt.Errorf("user does not own media")
	}
	bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
	presignedURL, err := s.bucket.GetSignedURL(ctx, bucketKey, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("generate signed URL: %w", err)
	}
	return &ai.DetectionImage{
		ImageURL: presignedURL,
		MimeType: media.ContentType,
		Filename: media.GetFilename(),
	}, nil
}

// descriptionFromFinal mirrors titleFromFinal for description.
func descriptionFromFinal(f *api.StreamGenUnifiedCreateFinal) string {
	if g := f.GetGear(); g != nil && g.GetDetectedGear() != nil {
		return g.GetDetectedGear().GetDescription()
	}
	if e := f.GetExperience(); e != nil {
		return e.GetDescription()
	}
	if r := f.GetRequest(); r != nil {
		return r.GetDescription()
	}
	return ""
}
