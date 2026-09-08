// Streaming variant of GenExperience. Opens the streaming AI call, fires
// Mapbox + Pexels fan-out as the driving JSON fields close, emits incremental
// events (title, geocoded, media_ready) to the client as each resolves, and
// ends with a terminal `final` or `error` event carrying the full response.
// Text mode gets the full streaming win; media/webpage modes fall through
// to the unary code path and emit only the terminal event (see #1229).

package experience

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/fanout"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	locationlib "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
)

// maxImageCandidates caps how many alternate URLs ride on MediaReady.
// Matches the client's 4-up Replace Media row capacity.
const maxImageCandidates = 4

// StreamGenExperience streams AI-generated experience content, emitting
// title / geocoded / media_ready as each resolves and terminating with a
// `final` (full response) or `error` event.
func (s *Service) StreamGenExperience(
	ctx context.Context,
	req *connect.Request[api.StreamGenExperienceRequest],
	stream *connect.ServerStream[api.StreamGenExperienceResponse],
) error {
	streamStart := time.Now()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"operation", "StreamGenExperience",
	)
	logger.InfoContext(ctx, "stream started")

	if s.aiProvider == nil {
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("AI provider not configured"))
	}

	sender := newExperienceEventSender(stream)
	defer func() {
		if cerr := sender.close(); cerr != nil {
			logger.WarnContext(ctx, "stream send failed",
				"error", cerr,
				"duration_ms", time.Since(streamStart).Milliseconds())
		}
	}()

	args := GenExperienceContext{
		UserID:             authInfo.UserID,
		LocationID:         req.Msg.LocationId,
		Timezone:           req.Msg.Timezone,
		CurrentTimeUnixSec: req.Msg.CurrentTimeUnixSec,
		LatitudeDeg:        req.Msg.LatitudeDeg,
		LongitudeDeg:       req.Msg.LongitudeDeg,
	}

	// Dispatch by mode. Image mode uses the streaming fan-out; webpage mode
	// fetches the page synchronously up front then streams the AI response.
	switch p := req.Msg.GetPrompt().(type) {
	case *api.StreamGenExperienceRequest_Text:
		if p.Text == "" {
			sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
				"text cannot be empty")
			return nil
		}
		return s.StreamGenExperienceFromText(ctx, logger, streamStart, args, p.Text, sender)
	case *api.StreamGenExperienceRequest_MediaId:
		if p.MediaId == "" {
			sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
				"media_id cannot be empty")
			return nil
		}
		return s.StreamGenExperienceFromImage(ctx, logger, streamStart, args, p.MediaId, sender)
	case *api.StreamGenExperienceRequest_WebsiteUrl:
		if p.WebsiteUrl == "" {
			sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
				"website_url cannot be empty")
			return nil
		}
		return s.StreamGenExperienceFromWebpage(ctx, logger, streamStart, args, p.WebsiteUrl, sender)
	default:
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"exactly one of text, media_id, or website_url must be provided")
		return nil
	}
}

// StreamGenExperienceFromText runs the streaming AI call, fires Mapbox +
// Pexels concurrently as their trigger fields close, emits incremental
// events, and ends with a `final` (full response) or `error`. Exported
// so other services (e.g. unified_create) can call it with their own
// [StreamSender] implementation that bridges to a different
// response envelope.
func (s *Service) StreamGenExperienceFromText(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	args GenExperienceContext,
	prompt string,
	sender StreamSender,
) error {
	logger = logger.With("prompt_length", len(prompt))

	region, locationProximity := s.resolveRegion(ctx, args.LocationID)
	userTimezone, loc := s.resolveTimezone(logger, args.Timezone)
	currentTime := formatCurrentTime(args.CurrentTimeUnixSec, loc)

	// Resolve geocoding proximity now — the streaming branch fires Mapbox
	// the instant location_query closes, before any post-AI block runs.
	proximity, proximitySource := fanout.ProximityForGeocoding(
		nil, nil,
		args.LatitudeDeg, args.LongitudeDeg,
		locationProximity,
	)
	proximityReq := locationlib.ProximityRequestFromSource(proximity, proximitySource)

	// Description equals the user's prompt verbatim — emit it immediately so
	// the client can render it before the AI returns its first token.
	sender.EmitDescription(prompt)

	streamFields, streamFinal, err := s.aiProvider.GenerateExperienceFromTextStreaming(ctx, prompt, region, currentTime)
	if err != nil {
		logger.ErrorContext(ctx, "failed to open AI stream", "error", err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, err.Error())
		return nil
	}

	// Branch outputs captured here — used to build the terminal `final`
	// event after every branch has reported.
	var (
		geocodedLocation *api.GeocodedLocation
		stockMediaID     string
		branchWG         sync.WaitGroup
	)
	mapboxDone := make(chan struct{})
	mediaDone := make(chan struct{})
	mapboxFired := false
	mediaFired := false

	// Mid-stream `time` event accumulator: hold whichever of date / time /
	// time_confidence arrived first, emit once all three are present.
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
				// The prompt went out as the description before a title
				// existed; now that one does, drop the clause it duplicates
				// so the event doesn't read with its own heading repeated
				// underneath it (#2724). A no-op unless the title is the
				// prompt's opening clause.
				if trimmed := ai.DescriptionWithoutTitleClause(prompt, title); trimmed != prompt {
					sender.EmitDescription(trimmed)
				}
			}

		// Text mode: description = user's prompt (already emitted above).
		// Ignore the AI-streamed description so the client's stable
		// description is the user's text, not a mid-stream AI rewrite that
		// would be replaced by the prompt at terminal time anyway.

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
				// Handled post-stream via the user-primary-location fallback.
				continue
			}
			mapboxFired = true
			branchWG.Add(1)
			logging.GoSafe(ctx, "stream-gen-experience-mapbox", func() {
				defer branchWG.Done()
				defer close(mapboxDone)
				// Text mode has no JSON-LD event location, so eventLocationName="".
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
			branchWG.Add(1)
			keywordsCopy := keywords
			logging.GoSafe(ctx, "stream-gen-experience-media", func() {
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

	// Close unfired-branch channels so the wait below is uniform.
	if !mapboxFired {
		close(mapboxDone)
	}
	if !mediaFired {
		close(mediaDone)
	}

	// Wait for the AI terminal value and all branches. Order matters for
	// error handling: if AI errored, there may not be a valid result even
	// if branches succeeded.
	branchWG.Wait()
	final := <-streamFinal
	if final.Err != nil {
		ai.LogAIError(ctx, logger, "AI streaming generation failed", final.Err)
		sender.EmitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, final.Err.Error())
		return nil
	}

	aiResponse := final.Result
	// Use the user's original text as the description verbatim.
	aiResponse.Description = prompt

	logger.InfoContext(ctx, "streaming AI + fan-out completed",
		"title", aiResponse.Title,
		"mapbox_fired", mapboxFired,
		"mapbox_hit", geocodedLocation != nil,
		"media_fired", mediaFired,
		"media_hit", stockMediaID != "",
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	mediaIds := []string{}
	if stockMediaID != "" {
		mediaIds = append(mediaIds, stockMediaID)
	}

	// Fall back to user-primary-location for USER_PRIMARY_LOCATION and
	// empty-query cases (both fast storage lookups, not worth a goroutine).
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
