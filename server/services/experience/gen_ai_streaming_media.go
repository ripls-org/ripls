package experience

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/fanout"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/genai/mediacand"
	locationlib "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// streamGenExperienceFromImage runs the streaming AI call for image-mode
// experience generation, fires Mapbox + Pexels concurrently as their
// trigger fields close, emits incremental events, and ends with a `final`
// (full response) or `error`. Mirrors streamGenExperienceFromText's
// fan-out and field-event handling — the only differences are media
// resolution at the top, the AI provider call (image-streaming variant),
// and that the AI-generated description is preserved (text mode replaces
// it with the user's prompt).
// StreamGenExperienceFromImage runs the streaming AI call for image-mode
// experience generation. Exported so other services can call it with their
// own [StreamSender]. See [StreamGenExperienceFromText] for the
// shape; the only differences are media resolution at the top, the AI
// provider call (image-streaming variant), and that the AI-generated
// description is preserved (text mode replaces it with the user's prompt).
func (s *Service) StreamGenExperienceFromImage(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	args GenExperienceContext,
	mediaID string,
	sender StreamSender,
) error {
	logger = logger.With("media_id", mediaID)

	region, locationProximity := s.resolveRegion(ctx, args.LocationID)
	userTimezone, loc := s.resolveTimezone(logger, args.Timezone)
	currentTime := formatCurrentTime(args.CurrentTimeUnixSec, loc)

	proximity, proximitySource := fanout.ProximityForGeocoding(
		nil, nil,
		args.LatitudeDeg, args.LongitudeDeg,
		locationProximity,
	)
	proximityReq := locationlib.ProximityRequestFromSource(proximity, proximitySource)

	// args.Image is the pre-resolved fast path used by unified-create:
	// the classifier already resolved the same media to a *DetectionImage,
	// so we reuse it instead of doing another GetByID + signed-URL
	// generation. See StreamGenGearFromImage for the full rationale (#1952).
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
		}
	}

	streamFields, streamFinal, err := s.aiProvider.GenerateExperienceFromImageStreaming(ctx, detectionImage, region, "", currentTime)
	if err != nil {
		logger.ErrorContext(ctx, "failed to open AI stream", "error", err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, err.Error())
		return nil
	}

	var (
		geocodedLocation *api.GeocodedLocation
		stockMediaID     string
		branchWG         sync.WaitGroup
	)
	mapboxDone := make(chan struct{})
	mediaDone := make(chan struct{})
	mapboxFired := false
	mediaFired := false

	var (
		streamedDate                 string
		streamedTime                 string
		streamedTimeConfidence       string
		dateSeen, timeSeen, confSeen bool
	)
	maybeEmitTime := func() {
		if !(dateSeen && timeSeen && confSeen) {
			return
		}
		extraction := buildExperienceTimeExtraction(
			streamedDate, streamedTime, streamedTimeConfidence,
			userTimezone, loc,
		)
		sender.EmitTime(extraction)
	}

	skipStockMedia := s.stockVideoProvider == nil && s.stockImageryProvider == nil

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

		case ai.StreamFieldDate:
			if err := json.Unmarshal(ev.Value, &streamedDate); err == nil {
				dateSeen = true
				maybeEmitTime()
			}

		case ai.StreamFieldTime:
			if err := json.Unmarshal(ev.Value, &streamedTime); err == nil {
				timeSeen = true
				maybeEmitTime()
			}

		case ai.StreamFieldTimeConfidence:
			if err := json.Unmarshal(ev.Value, &streamedTimeConfidence); err == nil {
				confSeen = true
				maybeEmitTime()
			}

		case ai.StreamFieldLocationQuery:
			if s.locationProvider == nil {
				continue
			}
			var locQuery string
			if err := json.Unmarshal(ev.Value, &locQuery); err != nil {
				continue
			}
			if locQuery == "" || locQuery == ai.LocationQueryUserPrimary {
				continue
			}
			mapboxFired = true
			branchWG.Add(1)
			logging.GoSafe(ctx, "stream-gen-experience-image-mapbox", func() {
				defer branchWG.Done()
				defer close(mapboxDone)
				geo := s.geocodePlaceQuery(ctx, locQuery, proximityReq, "")
				if geo != nil {
					geocodedLocation = geo
					sender.EmitGeocoded(geo)
				}
			})

		case ai.StreamFieldSearchKeywords:
			if skipStockMedia {
				continue
			}
			var keywords []string
			if err := json.Unmarshal(ev.Value, &keywords); err != nil || len(keywords) == 0 {
				continue
			}
			mediaFired = true
			keywordsCopy := keywords
			branchWG.Add(1)
			logging.GoSafe(ctx, "stream-gen-experience-image-media", func() {
				defer branchWG.Done()
				defer close(mediaDone)
				id := s.fetchStockMediaFallback(ctx, keywordsCopy, args.UserID)
				if id == "" {
					return
				}
				stockMediaID = id
				cands := s.fetchStockCandidates(ctx, keywordsCopy, maxImageCandidates, logger)
				if len(cands) > 0 {
					sender.EmitMediaReadyWithCandidates([]string{id}, cands)
				} else {
					sender.EmitMediaReady([]string{id})
				}
			})
		}
	}

	if !mapboxFired {
		close(mapboxDone)
	}
	if !mediaFired {
		close(mediaDone)
	}

	branchWG.Wait()
	final := <-streamFinal
	if final.Err != nil {
		ai.LogAIError(ctx, logger, "AI streaming generation failed", final.Err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, final.Err.Error())
		return nil
	}

	aiResponse := final.Result

	logger.InfoContext(ctx, "streaming AI + fan-out completed",
		"input_mode", "image",
		"title", aiResponse.Title,
		"mapbox_fired", mapboxFired,
		"mapbox_hit", geocodedLocation != nil,
		"media_fired", mediaFired,
		"media_hit", stockMediaID != "",
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	// Image mode: the user-uploaded media is part of the experience even
	// if no stock-media branch fired. Include the input media_id alongside
	// any stock media the AI surfaced.
	mediaIds := []string{mediaID}
	if stockMediaID != "" && stockMediaID != mediaID {
		mediaIds = append(mediaIds, stockMediaID)
	}

	var locationID string
	if geocodedLocation == nil {
		switch aiResponse.LocationQuery {
		case ai.LocationQueryUserPrimary:
			if s.locationProvider != nil {
				locationID, geocodedLocation = locationlib.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
			}
		case "":
			locationID, geocodedLocation = locationlib.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
		}
		// The client's preview only takes location from the mid-stream
		// geocoded event (the terminal payload's location is ignored), so
		// surface the fallback there too.
		if geocodedLocation != nil {
			sender.EmitGeocoded(geocodedLocation)
		}
	}

	finalResp := buildGenExperienceResponse(aiResponse, mediaIds, locationID, geocodedLocation, "", userTimezone, loc)
	sender.EmitFinal(finalResp)
	return nil
}

// streamGenExperienceFromWebpage runs the streaming AI call for webpage-mode
// experience generation. Fetches the webpage synchronously up front (this
// is sequential before the AI call — webpage mode's wall-clock has an
// unavoidable fetch tax that text and image modes don't), then opens
// GenerateExperienceFromWebpageStreaming and fans Mapbox / Pexels out as
// their trigger fields close. JSON-LD event metadata (date / time /
// location / lat-lng) overrides the AI's extracted values where present —
// machine-readable structured data is more reliable than LLM extraction
// from body text. Webpage-mode media has two priority levels: a downloaded
// webpage image (chat-link-preview style) takes priority over Pexels stock
// imagery. The image download runs after the AI stream closes today;
// parallelizing it with the AI call is a follow-up.
// StreamGenExperienceFromWebpage runs the streaming AI call for webpage-mode
// experience generation. Exported so other services can call it with their
// own [StreamSender]. Fetches the webpage synchronously up front
// (this is sequential before the AI call), then opens the streaming AI call
// and fans Mapbox / Pexels out as their trigger fields close.
func (s *Service) StreamGenExperienceFromWebpage(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	args GenExperienceContext,
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

	region, locationProximity := s.resolveRegion(ctx, args.LocationID)
	userTimezone, loc := s.resolveTimezone(logger, args.Timezone)
	currentTime := formatCurrentTime(args.CurrentTimeUnixSec, loc)

	// JSON-LD overrides resolved up front so the post-AI block can apply them
	// without re-inspecting pageContent.
	hasStructuredEvent := pageContent.Event != nil && pageContent.Event.StartDate != nil
	var (
		eventLatitude     *float64
		eventLongitude    *float64
		eventLocationName string
	)
	if pageContent.Event != nil {
		eventLatitude = pageContent.Event.Latitude
		eventLongitude = pageContent.Event.Longitude
		eventLocationName = pageContent.Event.LocationName
	}

	proximity, proximitySource := fanout.ProximityForGeocoding(
		eventLatitude, eventLongitude,
		args.LatitudeDeg, args.LongitudeDeg,
		locationProximity,
	)
	proximityReq := locationlib.ProximityRequestFromSource(proximity, proximitySource)

	streamFields, streamFinal, err := s.aiProvider.GenerateExperienceFromWebpageStreaming(
		ctx, pageContent.Title, pageContent.Description, pageContent.BodyText, region, currentTime,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to open AI stream", "error", err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, err.Error())
		return nil
	}

	var (
		geocodedLocation *api.GeocodedLocation
		stockMediaID     string
		branchWG         sync.WaitGroup
	)
	mapboxFired := false
	mediaFired := false

	var (
		streamedDate                 string
		streamedTime                 string
		streamedTimeConfidence       string
		dateSeen, timeSeen, confSeen bool
	)
	maybeEmitTime := func() {
		if !(dateSeen && timeSeen && confSeen) {
			return
		}
		extraction := buildExperienceTimeExtraction(
			streamedDate, streamedTime, streamedTimeConfidence,
			userTimezone, loc,
		)
		sender.EmitTime(extraction)
	}

	// Webpage image takes priority over stock media — when the page has an
	// og:image (or equivalent), skip the Pexels branch entirely.
	hasWebpageImage := pageContent.ImageURL != ""
	skipStockMedia := hasWebpageImage || (s.stockVideoProvider == nil && s.stockImageryProvider == nil)

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

		case ai.StreamFieldDate:
			if err := json.Unmarshal(ev.Value, &streamedDate); err == nil {
				dateSeen = true
				maybeEmitTime()
			}

		case ai.StreamFieldTime:
			if err := json.Unmarshal(ev.Value, &streamedTime); err == nil {
				timeSeen = true
				maybeEmitTime()
			}

		case ai.StreamFieldTimeConfidence:
			if err := json.Unmarshal(ev.Value, &streamedTimeConfidence); err == nil {
				confSeen = true
				maybeEmitTime()
			}

		case ai.StreamFieldLocationQuery:
			if s.locationProvider == nil {
				continue
			}
			var locQuery string
			if err := json.Unmarshal(ev.Value, &locQuery); err != nil {
				continue
			}
			if locQuery == "" || locQuery == ai.LocationQueryUserPrimary {
				continue
			}
			mapboxFired = true
			branchWG.Add(1)
			locQueryCopy := locQuery
			eventLocationNameCopy := eventLocationName
			logging.GoSafe(ctx, "stream-gen-experience-webpage-mapbox", func() {
				defer branchWG.Done()
				geo := s.geocodePlaceQuery(ctx, locQueryCopy, proximityReq, eventLocationNameCopy)
				if geo != nil {
					geocodedLocation = geo
					sender.EmitGeocoded(geo)
				}
			})

		case ai.StreamFieldSearchKeywords:
			if skipStockMedia {
				continue
			}
			var keywords []string
			if err := json.Unmarshal(ev.Value, &keywords); err != nil || len(keywords) == 0 {
				continue
			}
			mediaFired = true
			keywordsCopy := keywords
			branchWG.Add(1)
			logging.GoSafe(ctx, "stream-gen-experience-webpage-media", func() {
				defer branchWG.Done()
				id := s.fetchStockMediaFallback(ctx, keywordsCopy, args.UserID)
				if id == "" {
					return
				}
				stockMediaID = id
				cands := s.fetchStockCandidates(ctx, keywordsCopy, maxImageCandidates, logger)
				if len(cands) > 0 {
					sender.EmitMediaReadyWithCandidates([]string{id}, cands)
				} else {
					sender.EmitMediaReady([]string{id})
				}
			})
		}
	}

	branchWG.Wait()
	final := <-streamFinal
	if final.Err != nil {
		ai.LogAIError(ctx, logger, "AI streaming generation failed", final.Err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, final.Err.Error())
		return nil
	}

	aiResponse := final.Result

	// JSON-LD overrides for date/time/location. JSON-LD is machine-readable
	// and more reliable than LLM extraction, so it wins.
	if hasStructuredEvent {
		ev := pageContent.Event
		start := *ev.StartDate
		aiResponse.Date = start.Format("2006-01-02")
		aiResponse.Time = start.Format("15:04")
		aiResponse.TimeConfidence = ai.TimeConfidenceExplicit
		if ev.Location != "" {
			aiResponse.LocationQuery = ev.Location
		}
	}

	mediaIds := []string{}
	if hasWebpageImage {
		// Download the page's primary image as the experience's media —
		// matches chat-link-preview behavior. Best-effort: failure logs and
		// falls back to no media.
		mediaID, dlErr := s.downloadAndStoreWebpageImage(ctx, pageContent.ImageURL, args.UserID)
		if dlErr != nil {
			logger.WarnContext(ctx, "failed to download webpage image",
				"image_url", pageContent.ImageURL, "error", dlErr)
		} else if mediaID != "" {
			mediaIds = append(mediaIds, mediaID)
			// Surface up to 4 alternate candidate URLs the page also
			// offered. Entries [1:] are the alternates the client may
			// render in the Replace Media modal.
			var candidates []*api.MediaCandidate
			if len(pageContent.ImageURLs) > 1 {
				alternates := pageContent.ImageURLs[1:]
				if len(alternates) > maxImageCandidates {
					alternates = alternates[:maxImageCandidates]
				}
				candidates = mediacand.ImageURLsToCandidates(alternates)
			}
			sender.EmitMediaReadyWithCandidates(mediaIds, candidates)
		}
	} else if stockMediaID != "" {
		mediaIds = append(mediaIds, stockMediaID)
	}

	logger.InfoContext(ctx, "streaming AI + fan-out completed",
		"input_mode", "webpage",
		"title", aiResponse.Title,
		"mapbox_fired", mapboxFired,
		"mapbox_hit", geocodedLocation != nil,
		"media_fired", mediaFired,
		"media_hit", len(mediaIds) > 0,
		"used_webpage_image", hasWebpageImage,
		"json_ld_override", hasStructuredEvent,
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	var locationID string
	if geocodedLocation == nil {
		switch aiResponse.LocationQuery {
		case ai.LocationQueryUserPrimary:
			if s.locationProvider != nil {
				locationID, geocodedLocation = locationlib.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
			}
		case "":
			locationID, geocodedLocation = locationlib.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
		}
		// The client's preview only takes location from the mid-stream
		// geocoded event (the terminal payload's location is ignored), so
		// surface the fallback there too.
		if geocodedLocation != nil {
			sender.EmitGeocoded(geocodedLocation)
		}
	}

	finalResp := buildGenExperienceResponse(aiResponse, mediaIds, locationID, geocodedLocation, websiteURL, userTimezone, loc)
	sender.EmitFinal(finalResp)
	return nil
}
