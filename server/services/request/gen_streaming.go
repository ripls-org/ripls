// Streaming variant of GenRequest. Opens the streaming AI call, fires
// Mapbox + Pexels fan-out as the driving JSON fields close, emits
// incremental events (title, geocoded, media_ready), and ends with a
// terminal `final` (full response) or `error` event.

package request

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/fanout"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/streaming"
)

// StreamGenRequest streams AI-generated request content.
func (s *Service) StreamGenRequest(
	ctx context.Context,
	req *connect.Request[api.StreamGenRequestRequest],
	stream *connect.ServerStream[api.StreamGenRequestResponse],
) error {
	streamStart := time.Now()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"operation", "StreamGenRequest",
	)
	logger.InfoContext(ctx, "stream started")

	sender := newRequestEventSender(stream)
	defer func() {
		if cerr := sender.close(); cerr != nil {
			logger.WarnContext(ctx, "stream send failed",
				"error", cerr,
				"duration_ms", time.Since(streamStart).Milliseconds())
		}
	}()

	args := GenRequestContext{
		UserID:       authInfo.UserID,
		LocationID:   req.Msg.LocationId,
		LatitudeDeg:  req.Msg.LatitudeDeg,
		LongitudeDeg: req.Msg.LongitudeDeg,
	}

	hasPrompt := req.Msg.Prompt != ""
	hasMedia := req.Msg.MediaId != ""
	if !hasPrompt && !hasMedia {
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"either prompt or media_id must be provided")
		return nil
	}
	if hasPrompt && hasMedia {
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"provide either prompt or media_id, not both")
		return nil
	}
	if s.aiProvider == nil {
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INTERNAL,
			"AI provider not configured")
		return nil
	}

	if hasMedia {
		return s.StreamGenRequestFromImage(ctx, logger, streamStart, args, req.Msg.MediaId, sender)
	}
	return s.StreamGenRequestFromText(ctx, logger, streamStart, args, req.Msg.Prompt, sender)
}

// StreamGenRequestFromText runs the streaming AI call with concurrent
// Mapbox (keyed on location_query) + Pexels (keyed on search_keywords)
// fan-out. Exported so other services (e.g. unified_create) can call it
// with their own [StreamSender] that bridges to a different
// response envelope.
func (s *Service) StreamGenRequestFromText(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	args GenRequestContext,
	prompt string,
	sender StreamSender,
) error {
	region, locationProximity := s.resolveRegion(ctx, logger, args.LocationID)
	proximity, proximitySource := fanout.ProximityForGeocoding(
		nil, nil,
		args.LatitudeDeg, args.LongitudeDeg,
		locationProximity,
	)
	proximityReq := location.ProximityRequestFromSource(proximity, proximitySource)

	// Description equals the user's prompt verbatim — emit it immediately so
	// the client can render it before the AI returns its first token.
	sender.EmitDescription(prompt)

	streamFields, streamFinal, err := s.aiProvider.GenerateRequestContentStreaming(ctx, prompt, region)
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
	skipStockMedia := s.stockImageryProvider == nil

	for ev := range streamFields {
		switch ev.Key {
		case ai.StreamFieldTitle:
			var title string
			if err := json.Unmarshal(ev.Value, &title); err == nil && title != "" {
				sender.EmitTitle(title)
				// The prompt went out as the description before a title
				// existed; now that one does, drop the clause it duplicates
				// so the request doesn't read with its own heading repeated
				// underneath it (#2724). A no-op unless the title is the
				// prompt's opening clause.
				if trimmed := ai.DescriptionWithoutTitleClause(prompt, title); trimmed != prompt {
					sender.EmitDescription(trimmed)
				}
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
			logging.GoSafe(ctx, "stream-gen-request-mapbox", func() {
				defer branchWG.Done()
				geo := s.geocodePlaceQuery(ctx, locQueryCopy, proximityReq)
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
			logging.GoSafe(ctx, "stream-gen-request-media", func() {
				defer branchWG.Done()
				id := s.fetchStockImage(ctx, keywordsCopy)
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

	generation := final.Result
	generation.Description = prompt

	logger.InfoContext(ctx, "streaming AI + fan-out completed",
		"title", generation.Title,
		"mapbox_fired", mapboxFired,
		"mapbox_hit", geocodedLocation != nil,
		"media_fired", mediaFired,
		"media_hit", stockMediaID != "",
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	var mediaIDs []string
	if stockMediaID != "" {
		mediaIDs = []string{stockMediaID}
	}

	if geocodedLocation == nil {
		switch generation.LocationQuery {
		case ai.LocationQueryUserPrimary:
			if s.locationProvider != nil {
				_, geocodedLocation = location.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
			}
		case "":
			_, geocodedLocation = location.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
		}
	}

	// Compute owner/helper chip suggestions sequentially after the primary
	// AI stream completes — best-effort, empty on any failure. Same call
	// powers lazyFillRequestSuggestions post-publish; surfacing chips here
	// means the compose sheet can render them off the streaming final
	// event without a second round-trip.
	additionalAsks, breakdownPieces, offerIdeas, seedNeeds := s.genRequestSuggestionsBestEffort(
		ctx, generation.Title, generation.Description, logger,
	)

	finalResp := buildGenRequestResponse(generation, mediaIDs, geocodedLocation,
		additionalAsks, breakdownPieces, offerIdeas, seedNeeds)
	sender.EmitFinal(finalResp)
	return nil
}

// resolveRegion looks up the user's primary location and returns a region
// label for the AI prompt plus the geocoding-proximity geolocation.
// streamGenRequestFromImage runs the streaming AI call for image-mode
// request generation, fires Mapbox + Pexels concurrently as their
// trigger fields close, emits incremental events, and ends with a
// `final` (full response) or `error`. Mirrors the text-mode field-event
// loop except that the AI-generated description is preserved (text
// mode replaces it with the user's prompt) and the user-uploaded media
// is included in the response media list alongside any stock media.
// StreamGenRequestFromImage runs the streaming AI call for image-mode
// request generation. Exported so other services can call it with their
// own [StreamSender]. Fires Mapbox + Pexels concurrently as
// trigger fields close, emits incremental events, falls back to
// user-primary-location for geocoding, and ends with a `final` carrying
// the user-uploaded media alongside any stock media. The AI-generated
// description is preserved (text mode replaces it with the user's
// prompt).
func (s *Service) StreamGenRequestFromImage(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	args GenRequestContext,
	mediaID string,
	sender StreamSender,
) error {
	logger = logger.With("media_id", mediaID)

	region, locationProximity := s.resolveRegion(ctx, logger, args.LocationID)
	proximity, proximitySource := fanout.ProximityForGeocoding(
		nil, nil,
		args.LatitudeDeg, args.LongitudeDeg,
		locationProximity,
	)
	proximityReq := location.ProximityRequestFromSource(proximity, proximitySource)

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

	streamFields, streamFinal, err := s.aiProvider.GenerateRequestFromImageStreaming(ctx, detectionImage, region)
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
	skipStockMedia := s.stockImageryProvider == nil

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
			logging.GoSafe(ctx, "stream-gen-request-image-mapbox", func() {
				defer branchWG.Done()
				geo := s.geocodePlaceQuery(ctx, locQueryCopy, proximityReq)
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
			logging.GoSafe(ctx, "stream-gen-request-image-media", func() {
				defer branchWG.Done()
				id := s.fetchStockImage(ctx, keywordsCopy)
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

	generation := final.Result

	logger.InfoContext(ctx, "streaming AI + fan-out completed",
		"input_mode", "image",
		"title", generation.Title,
		"mapbox_fired", mapboxFired,
		"mapbox_hit", geocodedLocation != nil,
		"media_fired", mediaFired,
		"media_hit", stockMediaID != "",
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	// Image mode: the user-uploaded media is part of the request even if no
	// stock-media branch fired. Include the input media_id alongside any
	// stock media.
	mediaIDs := []string{mediaID}
	if stockMediaID != "" && stockMediaID != mediaID {
		mediaIDs = append(mediaIDs, stockMediaID)
	}

	// Image mode: fall back to user-primary-location whenever AI didn't
	// extract a geocodable location (mirrors the unary GenRequestFromMedia
	// behavior at gen.go:252-259 — "AI rarely extracts locations from
	// images, so this will usually trigger").
	if geocodedLocation == nil {
		_, geocodedLocation = location.GetUserLocationFallbackLogged(ctx, s.storage, args.UserID)
	}

	// Compute chip suggestions sequentially after the primary stream.
	// Best-effort: failure returns empty slices.
	additionalAsks, breakdownPieces, offerIdeas, seedNeeds := s.genRequestSuggestionsBestEffort(
		ctx, generation.Title, generation.Description, logger,
	)

	finalResp := buildGenRequestResponse(generation, mediaIDs, geocodedLocation,
		additionalAsks, breakdownPieces, offerIdeas, seedNeeds)
	sender.EmitFinal(finalResp)
	return nil
}

func (s *Service) resolveRegion(ctx context.Context, logger *logging.Logger, locationID string) (string, *models.Geolocation) {
	region := ""
	var proximity *models.Geolocation
	if locationID == "" {
		return region, nil
	}
	loc := &models.Location{}
	if err := s.storage.GetByID(ctx, locationID, loc); err != nil {
		logger.Warn("failed to get location for AI context", "location_id", locationID, "error", err)
		return region, nil
	}
	if loc.Address != nil {
		if loc.Address.Locality != "" {
			region = loc.Address.Locality
		} else if loc.Address.RegionCode != "" {
			region = loc.Address.RegionCode
		}
	}
	if loc.Geolocation != nil &&
		(loc.Geolocation.LatitudeDeg != 0 || loc.Geolocation.LongitudeDeg != 0) {
		proximity = loc.Geolocation
	}
	return region, proximity
}

// buildGenRequestResponse assembles the terminal `final` response.
// Chip lists (additionalAsks / breakdownPieces / offerIdeas) seed the
// compose sheet without a second round-trip; empty slices are fine and
// rendered as no chips on the client.
func buildGenRequestResponse(
	generation *ai.RequestGeneration,
	mediaIDs []string,
	geocodedLocation *api.GeocodedLocation,
	additionalAsks, breakdownPieces, offerIdeas, seedNeeds []string,
) *api.GenRequestResponse {
	var metadata *api.RequestMetadata
	if generation.ValueEstimate != nil {
		metadata = &api.RequestMetadata{
			ValueEstimate: &api.ValueEstimate{
				EstimatedValueUsd: generation.ValueEstimate.EstimatedValueUSD,
				Provenance: &api.Provenance{
					Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
					Name:       "genai_value_estimate",
					Confidence: proto.Float32(float32(generation.ValueEstimate.Confidence)),
					Sources:    generation.ValueEstimate.Sources,
				},
			},
		}
	}
	return &api.GenRequestResponse{
		Title:            generation.Title,
		Description:      generation.Description,
		MediaIds:         mediaIDs,
		Tags:             generation.SearchKeywords,
		LocationQuery:    generation.LocationQuery,
		GeocodedLocation: geocodedLocation,
		Metadata:         metadata,
		AdditionalAsks:   additionalAsks,
		BreakdownPieces:  breakdownPieces,
		OfferIdeas:       offerIdeas,
		SeedNeeds:        seedNeeds,
	}
}

// requestEventSender wraps streaming.Sender with typed emit helpers for the
// StreamGenRequestResponse oneof.
type requestEventSender struct {
	*streaming.Sender[api.StreamGenRequestResponse]
}

func newRequestEventSender(target streaming.SendTarget[api.StreamGenRequestResponse]) *requestEventSender {
	return &requestEventSender{Sender: streaming.NewSender[api.StreamGenRequestResponse](target)}
}

func (s *requestEventSender) emitTitle(title string) {
	s.Emit(&api.StreamGenRequestResponse{
		Event: &api.StreamGenRequestResponse_Title{Title: title},
	})
}

func (s *requestEventSender) emitGeocoded(g *api.GeocodedLocation) {
	s.Emit(&api.StreamGenRequestResponse{
		Event: &api.StreamGenRequestResponse_Geocoded{Geocoded: g},
	})
}

func (s *requestEventSender) emitMediaReady(mediaIDs []string) {
	s.emitMediaReadyWithCandidates(mediaIDs, nil)
}

func (s *requestEventSender) emitMediaReadyWithCandidates(mediaIDs []string, candidates []*api.MediaCandidate) {
	s.Emit(&api.StreamGenRequestResponse{
		Event: &api.StreamGenRequestResponse_MediaReady{
			MediaReady: &api.MediaReady{
				MediaIds:   mediaIDs,
				Candidates: candidates,
			},
		},
	})
}

func (s *requestEventSender) emitDescription(description string) {
	s.Emit(&api.StreamGenRequestResponse{
		Event: &api.StreamGenRequestResponse_Description{Description: description},
	})
}

func (s *requestEventSender) emitFinal(resp *api.GenRequestResponse) {
	s.Emit(&api.StreamGenRequestResponse{
		Event: &api.StreamGenRequestResponse_Final{Final: resp},
	})
}

func (s *requestEventSender) emitError(code api.GenStreamErrorCode, message string) {
	s.Emit(&api.StreamGenRequestResponse{
		Event: &api.StreamGenRequestResponse_Error{
			Error: &api.GenStreamError{Code: code, Message: message},
		},
	})
}

func (s *requestEventSender) close() error { return s.Close() }
