package request

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/fanout"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// genRequestSuggestionsBestEffort runs GenerateRequestSuggestions with a
// short timeout and returns empty slices on any failure. Used to seed the
// owner-facing chip strip on GenRequestResponse / StreamGenRequest's
// final event so the compose sheet can render chips without a separate
// round-trip. Mirrors the best-effort post-publish path in
// lazyFillRequestSuggestions — a missing chip set never blocks publish.
func (s *Service) genRequestSuggestionsBestEffort(ctx context.Context, title, description string, logger *logging.Logger) (additionalAsks, breakdownPieces, offerIdeas, seedNeeds []string) {
	if s.aiProvider == nil || strings.TrimSpace(title) == "" {
		return nil, nil, nil, nil
	}
	start := time.Now()
	result, err := ai.CallWithTimeout(ctx, 15*time.Second, func(ctx context.Context) (*ai.RequestSuggestionResult, error) {
		return s.aiProvider.GenerateRequestSuggestions(ctx, title, description, "")
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.WarnContext(ctx, "gen suggestion chips exceeded timeout, returning empty",
				"external_service", "ai_provider",
				"operation", "GenerateRequestSuggestions",
				"duration_ms", durationMs,
				"error", err)
		} else {
			logger.WarnContext(ctx, "gen suggestion chips failed, returning empty",
				"operation", "GenerateRequestSuggestions",
				"duration_ms", durationMs,
				"error", err)
		}
		return nil, nil, nil, nil
	}
	if result == nil {
		return nil, nil, nil, nil
	}
	logger.DebugContext(ctx, "gen suggestion chips ready",
		"duration_ms", durationMs,
		"additional_asks", len(result.AdditionalAsks),
		"breakdown_pieces", len(result.BreakdownPieces),
		"offer_ideas", len(result.OfferIdeas),
		"seed_needs", len(result.SeedNeeds))
	return result.AdditionalAsks, result.BreakdownPieces, result.OfferIdeas, result.SeedNeeds
}

// GenRequest generates request suggestions from a user prompt using AI.
// This method does NOT save the request to the database - it only returns
// AI-generated suggestions for the client to preview and optionally edit
// before calling SubmitRequest to persist.
func (s *Service) GenRequest(
	ctx context.Context,
	req *connect.Request[api.GenRequestRequest],
) (*connect.Response[api.GenRequestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Prompt == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("prompt cannot be empty"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.Info("generating request from AI prompt")

	// 1. Generate request content using AI
	if s.aiProvider == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("AI provider not configured"))
	}

	// Resolve region and coordinates from location_id for AI context and proximity bias.
	region := ""
	var locationProximity *models.Geolocation
	if req.Msg.LocationId != "" {
		loc := &models.Location{}
		if err := s.storage.GetByID(ctx, req.Msg.LocationId, loc); err != nil {
			logger.Warn("failed to get location for AI context", "location_id", req.Msg.LocationId, "error", err)
		} else {
			if loc.Address.Locality != "" {
				region = loc.Address.Locality
			} else if loc.Address.RegionCode != "" {
				region = loc.Address.RegionCode
			}
			if loc.Geolocation != nil &&
				(loc.Geolocation.LatitudeDeg != 0 || loc.Geolocation.LongitudeDeg != 0) {
				locationProximity = loc.Geolocation
			}
			logger.Debug("resolved region from location", "region", region)
		}
	}

	// Streaming AI + concurrent post-AI fan-out. Mapbox fires the instant
	// location_query closes; Pexels fires the instant search_keywords
	// closes. Closures inspect their trigger value to skip the call when
	// the AI returned empty / USER_PRIMARY_LOCATION.
	proximity, proximitySource := fanout.ProximityForGeocoding(
		nil, nil,
		req.Msg.LatitudeDeg, req.Msg.LongitudeDeg,
		locationProximity,
	)
	proximityReq := location.ProximityRequestFromSource(proximity, proximitySource)

	generation, streamingResult, err := s.runStreamingFanout(ctx, logger, req.Msg.Prompt, region, proximityReq)
	if err != nil {
		logger.Error("failed to generate request content (streaming)", "error", err)
		return nil, connecterr.Internal(ctx, "GenRequest", err)
	}

	// Use the user's original text as the description verbatim.
	generation.Description = req.Msg.Prompt

	logger.Info("generated request content",
		"operation", "GenRequest",
		"title", generation.Title,
		"confidence", generation.Confidence,
		"has_value_estimate", generation.ValueEstimate != nil)

	// Log the value estimate details if present
	if generation.ValueEstimate != nil {
		logger.Info("value estimate included in response",
			"operation", "GenRequest",
			"value_usd", generation.ValueEstimate.EstimatedValueUSD,
			"value_confidence", generation.ValueEstimate.Confidence,
			"sources_count", len(generation.ValueEstimate.Sources))
	} else {
		logger.Warn("AI provider did not generate a value estimate",
			"operation", "GenRequest")
	}

	var mediaIDs []string
	if streamingResult.StockMediaID != "" {
		mediaIDs = []string{streamingResult.StockMediaID}
	}

	// Fall back to the user-primary-location helper for the USER_PRIMARY_LOCATION
	// and empty-query cases (both are fast storage lookups, not worth a goroutine).
	geocodedLocation := streamingResult.GeocodedLocation
	if geocodedLocation == nil {
		switch generation.LocationQuery {
		case ai.LocationQueryUserPrimary:
			if s.locationProvider != nil {
				_, geocodedLocation = location.GetUserLocationFallbackLogged(ctx, s.storage, authInfo.UserID)
			}
		case "":
			logger.Debug("no location extracted by AI, falling back to user location")
			_, geocodedLocation = location.GetUserLocationFallbackLogged(ctx, s.storage, authInfo.UserID)
		}
	}

	logger.Info("generated request suggestions",
		"title", generation.Title,
		"media_count", len(mediaIDs),
		"has_geocoded_location", geocodedLocation != nil,
		"location_source", func() string {
			if geocodedLocation == nil {
				return "none"
			}
			if generation.LocationQuery != "" {
				return "ai_extracted"
			}
			return "primary_residence_fallback"
		}())

	// Build metadata from AI generation
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
			Category: "", // TODO(#2013): Add category detection to AI provider
		}
	}

	// Generate owner/helper chip suggestions for the compose sheet. Best-
	// effort: any failure here returns empty slices so chip generation never
	// blocks GenRequest. Same call powers lazyFillRequestSuggestions
	// post-publish; surfacing it here means the compose stage can render
	// chips without a second round-trip.
	additionalAsks, breakdownPieces, offerIdeas, seedNeeds := s.genRequestSuggestionsBestEffort(
		ctx, generation.Title, generation.Description, logger,
	)

	// Return suggestions WITHOUT saving to database
	// Client will preview and call SubmitRequest to persist
	return connect.NewResponse(&api.GenRequestResponse{
		Title:            generation.Title,
		Description:      generation.Description,
		MediaIds:         mediaIDs,
		Tags:             generation.SearchKeywords,
		LocationQuery:    generation.LocationQuery,
		GeocodedLocation: geocodedLocation, // Geocoded data for client to store on confirm
		Metadata:         metadata,
		AdditionalAsks:   additionalAsks,
		BreakdownPieces:  breakdownPieces,
		OfferIdeas:       offerIdeas,
		SeedNeeds:        seedNeeds,
	}), nil
}

// GenRequestFromMedia generates request suggestions from an uploaded image using AI vision.
// This method does NOT save the request to the database - it only returns
// AI-generated suggestions for the client to preview and optionally edit
// before calling SubmitRequest to persist.
func (s *Service) GenRequestFromMedia(
	ctx context.Context,
	req *connect.Request[api.GenRequestFromMediaRequest],
) (*connect.Response[api.GenRequestFromMediaResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.MediaId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("media_id cannot be empty"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"media_id", req.Msg.MediaId,
	)

	logger.Info("generating request from uploaded image")

	// 1. Check if AI provider is configured
	if s.aiProvider == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("AI provider not configured"))
	}

	// 2. Resolve region from location_id if provided for AI context
	region := ""
	if req.Msg.LocationId != "" {
		loc := &models.Location{}
		if err := s.storage.GetByID(ctx, req.Msg.LocationId, loc); err != nil {
			logger.Warn("failed to get location for AI context", "location_id", req.Msg.LocationId, "error", err)
			// Continue without region - non-critical error
		} else {
			// Use locality (city) if available, otherwise region code
			if loc.Address.Locality != "" {
				region = loc.Address.Locality
			} else if loc.Address.RegionCode != "" {
				region = loc.Address.RegionCode
			}
			logger.Debug("resolved region from location", "region", region)
		}
	}

	// 3. Generate from media
	generation, err := s.generateFromMedia(ctx, req.Msg.MediaId, authInfo.UserID, region)
	if err != nil {
		return nil, err
	}

	logger.Info("generated request content from image",
		"operation", "GenRequestFromMedia",
		"title", generation.Title,
		"confidence", generation.Confidence,
		"has_value_estimate", generation.ValueEstimate != nil)

	// Log the value estimate details if present
	if generation.ValueEstimate != nil {
		logger.Info("value estimate included in response",
			"operation", "GenRequestFromMedia",
			"value_usd", generation.ValueEstimate.EstimatedValueUSD,
			"value_confidence", generation.ValueEstimate.Confidence,
			"sources_count", len(generation.ValueEstimate.Sources))
	} else {
		logger.Warn("AI provider did not generate a value estimate",
			"operation", "GenRequestFromMedia")
	}

	// 4. Use user's primary residence or most recent location as fallback only if AI didn't extract a location
	// (Image mode: AI rarely extracts locations from images, so this will usually trigger)
	var geocodedLocation *api.GeocodedLocation
	if generation.LocationQuery == "" {
		logger.Debug("no location extracted by AI from image, falling back to user location")
		_, geocodedLocation = location.GetUserLocationFallbackLogged(ctx, s.storage, authInfo.UserID)
	} else {
		logger.Debug("AI extracted location from image, not using fallback", "location_query", generation.LocationQuery)
	}

	// Build metadata from AI generation
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
			Category: "", // TODO(#2013): Add category detection to AI provider
		}
	}

	// Generate chip suggestions for the compose sheet (same call as the
	// text-mode path; see genRequestSuggestionsBestEffort).
	additionalAsks, breakdownPieces, offerIdeas, seedNeeds := s.genRequestSuggestionsBestEffort(
		ctx, generation.Title, generation.Description, logger,
	)

	// Return suggestions WITHOUT saving to database
	// Client will preview and call SubmitRequest to persist
	return connect.NewResponse(&api.GenRequestFromMediaResponse{
		Title:            generation.Title,
		Description:      generation.Description,
		MediaIds:         []string{req.Msg.MediaId}, // Return the original media ID as array
		Tags:             generation.SearchKeywords,
		GeocodedLocation: geocodedLocation, // Geocoded data for client to store on confirm
		Metadata:         metadata,
		AdditionalAsks:   additionalAsks,
		BreakdownPieces:  breakdownPieces,
		OfferIdeas:       offerIdeas,
		SeedNeeds:        seedNeeds,
	}), nil
}

// generateFromMedia analyzes a media image and generates request content using AI vision.
func (s *Service) generateFromMedia(ctx context.Context, mediaID, userID, region string) (*ai.RequestGeneration, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"media_id", mediaID,
		"user_id", userID,
	)

	// Fetch media from storage
	media := &models.Media{}
	err := s.storage.GetByID(ctx, mediaID, media)
	if err != nil {
		logger.Error("failed to get media", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Verify user has access to this media
	if media.UserId != userID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user does not have access to this media"))
	}

	// Generate presigned URL for AI provider access (15 minute expiration)
	bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
	presignedURL, err := s.bucket.GetSignedURL(ctx, bucketKey, 15*time.Minute)
	if err != nil {
		logger.Error("failed to generate presigned URL", "error", err)
		return nil, connecterr.Internal(ctx, "generateFromMedia", err, "detail",

			// Call AI provider with presigned URL
			"failed to generate media access URL")
	}

	detectionImage := &ai.DetectionImage{
		ImageURL: presignedURL,
		MimeType: media.ContentType,
	}

	aiResponse, err := s.aiProvider.GenerateRequestFromImage(ctx, detectionImage, region)
	if err != nil {
		logger.Error("AI generation from image failed", "error", err)
		return nil, connecterr.Internal(ctx, "generateFromMedia", err, "detail", "failed to analyze image")
	}

	logger.Info("generated request from media")

	return aiResponse, nil
}

// findAndStoreStockImage searches for a stock image using keywords and creates a copy for the request.
// Returns the media ID of the stored image, or empty string if no image found.
// Uses the copy-on-use pattern to support cascade deletion and attribution tracking.
func (s *Service) findAndStoreStockImage(ctx context.Context, keywords []string) (string, error) {
	if s.stockImageryProvider == nil {
		return "", fmt.Errorf("stock imagery provider not configured")
	}

	if len(keywords) == 0 {
		return "", fmt.Errorf("no keywords provided")
	}

	logger := logging.LoggerWithContext(ctx)

	// Join keywords into a single query
	query := strings.Join(keywords, " ")
	logger.DebugContext(ctx, "searching for stock image", "query", query)

	// Get stock image from provider
	stockImage, err := s.stockImageryProvider.GetStockImage(ctx, query, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get stock image: %w", err)
	}

	// Create a copy for this request (enables cascade deletion and attribution tracking)
	mediaID, err := storage.CopyStockImageForUser(
		ctx,
		s.storage,
		s.bucket,
		stockImage,
		"system",
		"request-stock.jpg",
		fmt.Sprintf("Stock image for request: %s", query),
	)
	if err != nil {
		return "", fmt.Errorf("failed to copy stock image: %w", err)
	}

	logger.InfoContext(ctx, "stored stock image copy for request preview",
		"media_id", mediaID,
		"stock_image_id", stockImage.Id)

	return mediaID, nil
}

// GenerateResolutionSummary is retired (#2936). It used to write the
// requester's wrap-up in first person, thanking the people who helped.
//
// #992 removed the modal step that showed it; this removes the
// generation behind it. The handler stays as an Unimplemented stub only
// because the generated RequestServiceHandler interface still declares
// the method; TODO(#2938): delete it and the RPC once the release
// carrying #2936 has soaked, per docs/proto_conventions.md.
func (s *Service) GenerateResolutionSummary(
	_ context.Context,
	_ *connect.Request[api.GenerateResolutionSummaryRequest],
) (*connect.Response[api.GenerateResolutionSummaryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented,
		fmt.Errorf("resolution summaries are no longer generated"))
}
