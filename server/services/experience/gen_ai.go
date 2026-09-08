package experience

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/fanout"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	locationlib "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// GenExperience generates an experience using AI based on a user prompt, media, or webpage URL.
// Supports three modes:
// - Text mode: User provides a text prompt
// - Media mode: User provides a media_id (e.g., photo of a flyer)
// - Webpage mode: User provides a website_url (e.g., Eventbrite link)
// Returns suggested content WITHOUT saving to database (client previews first).
func (s *Service) GenExperience(
	ctx context.Context,
	req *connect.Request[api.GenExperienceRequest],
) (*connect.Response[api.GenExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
	)

	// Check if AI provider is configured
	if s.aiProvider == nil {
		logger.Error("AI provider not configured - experience generation unavailable")
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("AI provider not configured"))
	}

	// Determine user's region and coordinates from their default location.
	region := "Unknown region"
	var locationProximity *models.Geolocation
	if req.Msg.LocationId != "" {
		location := &models.Location{}
		if err := s.storage.GetByID(ctx, req.Msg.LocationId, location); err == nil {
			if location.Address != nil && location.Address.Locality != "" {
				region = location.Address.Locality
			}
			if location.Geolocation != nil &&
				(location.Geolocation.LatitudeDeg != 0 || location.Geolocation.LongitudeDeg != 0) {
				locationProximity = location.Geolocation
			}
		}
	}

	// Resolve user timezone. Date math falls back to UTC when the client
	// didn't provide a usable timezone, but userTimezone (which is persisted
	// on the suggested time) stays empty in that case — a stored "UTC"
	// literal is indistinguishable from a real UTC-timezone event (#2621).
	userTimezone := req.Msg.Timezone
	loc := time.UTC
	if userTimezone != "" {
		var err error
		loc, err = time.LoadLocation(userTimezone)
		if err != nil {
			logger.Warn("invalid timezone, falling back to UTC", "timezone", userTimezone, "error", err)
			userTimezone = ""
			loc = time.UTC
		}
	}

	// Get current time for relative date parsing, formatted in the user's timezone
	// so the LLM interprets relative references ("tomorrow", "this Friday") correctly.
	var currentTime string
	if req.Msg.CurrentTimeUnixSec > 0 {
		currentTime = time.Unix(req.Msg.CurrentTimeUnixSec, 0).In(loc).Format(time.RFC3339)
	} else {
		currentTime = time.Now().In(loc).Format(time.RFC3339)
	}

	var aiResponse *ai.ExperienceGeneration
	var mediaID string           // Track media ID for response if provided
	var webpageImageURL string   // Track webpage image URL for download
	var sourceURL string         // Track source URL for experiences created from websites
	var eventLatitude *float64   // Geo coordinates from structured event data (JSON-LD).
	var eventLongitude *float64  //
	var eventLocationName string // Place name from JSON-LD (e.g., "Boulder Airport").
	// streamingDone indicates the text-mode streaming path has already run
	// post-AI fan-out (Mapbox + Pexels) concurrently with the AI call. The
	// unary fan-out block below skips when this is set; streamingResult
	// carries its outputs forward to be merged with mediaIds / geocodedLocation.
	var streamingDone bool
	var streamingResult fanout.StreamingResults

	// Build media_ids upfront from any user-provided media. Done before the
	// text-mode streaming call so the streaming-fan-out closure knows whether
	// to skip the Pexels stock-media branch (skip when the user already
	// supplied media — e.g., webpage mode adds it later, after the stream).
	mediaIds := []string{}
	// Handle the oneof prompt field
	switch p := req.Msg.GetPrompt().(type) {
	case *api.GenExperienceRequest_Text:
		if p.Text == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("text cannot be empty"))
		}
		logger = logger.With("prompt_length", len(p.Text))

		// Compute proximity now — the streaming branch fires Mapbox the
		// instant location_query closes, before the unary post-AI block
		// runs, so we need the bias ladder resolved upfront.
		streamProximity, streamSource := fanout.ProximityForGeocoding(
			eventLatitude, eventLongitude,
			req.Msg.LatitudeDeg, req.Msg.LongitudeDeg,
			locationProximity,
		)
		streamProximityReq := locationlib.ProximityRequestFromSource(streamProximity, streamSource)

		aiResponse, streamingResult, err = s.runStreamingFanout(
			ctx, logger, p.Text, region, currentTime,
			streamProximityReq, len(mediaIds) > 0, authInfo.UserID,
		)
		if err != nil {
			logger.Error("AI streaming generation from text failed", "error", err)
			return nil, connecterr.Internal(ctx, "GenExperience", err, "detail",

				// Use the user's original text as the description verbatim.
				"failed to generate experience")
		}

		aiResponse.Description = p.Text
		streamingDone = true
		logger.Info("generated experience from text prompt (streaming)")

	case *api.GenExperienceRequest_MediaId:
		if p.MediaId == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("media_id cannot be empty"))
		}
		mediaID = p.MediaId
		logger = logger.With("media_id", mediaID)

		aiResponse, err = s.generateFromMedia(ctx, mediaID, authInfo.UserID, region, currentTime)
		if err != nil {
			return nil, err
		}
	case *api.GenExperienceRequest_WebsiteUrl:
		if p.WebsiteUrl == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("website_url cannot be empty"))
		}
		sourceURL = p.WebsiteUrl
		logger = logger.With("website_url", p.WebsiteUrl)

		webpageRes, err := s.generateFromWebpage(ctx, p.WebsiteUrl, region, currentTime)
		if err != nil {
			return nil, err
		}
		aiResponse = webpageRes.aiResponse
		webpageImageURL = webpageRes.imageURL
		eventLatitude = webpageRes.eventLatitude
		eventLongitude = webpageRes.eventLongitude
		eventLocationName = webpageRes.eventLocationName
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("exactly one of text, media_id, or website_url must be provided"))
	}

	logger.Info("AI generation completed",
		"operation", "GenExperience",
		"title", aiResponse.Title,
		"has_value_estimate", aiResponse.ValueEstimate != nil)

	// Log the value estimate details if present
	if aiResponse.ValueEstimate != nil {
		logger.Info("value estimate included in response",
			"operation", "GenExperience",
			"value_usd", aiResponse.ValueEstimate.EstimatedValueUSD,
			"value_confidence", aiResponse.ValueEstimate.Confidence,
			"sources_count", len(aiResponse.ValueEstimate.Sources))
	} else {
		logger.Warn("AI provider did not generate a value estimate",
			"operation", "GenExperience")
	}

	// Build the suggested time. Parse date/time after timeConfidence is known
	// below, then set here. Initially TBD; overwritten when confidence is
	// EXPLICIT or INFERRED and the date string parses successfully.
	suggestedTime := &api.ExperienceTime{
		TimeType: &api.ExperienceTime_Tbd{
			Tbd: &api.TimeTBD{},
		},
	}

	// Build media_ids array (include uploaded media if provided). Note:
	// mediaIds was declared above so the streaming text-mode path could
	// inspect it before deciding whether to fire the Pexels stock-media
	// branch. The reset below preserves prior behavior — text mode arrives
	// here with empty mediaIds, media/webpage modes populate it.
	if mediaID != "" {
		// Media mode: include the user's uploaded media
		mediaIds = append(mediaIds, mediaID)
	} else if webpageImageURL != "" {
		// Webpage mode: try to download the primary image from the webpage (like chat link previews)
		webpageMediaID, err := s.downloadAndStoreWebpageImage(ctx, webpageImageURL, authInfo.UserID)
		if err != nil {
			// Log but don't fail - image download is optional, will fall back to stock imagery
			logger.Warn("failed to download webpage image", "image_url", webpageImageURL, "error", err)
		} else if webpageMediaID != "" {
			mediaIds = append(mediaIds, webpageMediaID)
			logger.Debug("using webpage image for experience", "media_id", webpageMediaID, "image_url", webpageImageURL)
		}
	}

	// Parse time confidence enum (cheap, no I/O, keep serial).
	timeConfidence := api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN
	switch aiResponse.TimeConfidence {
	case ai.TimeConfidenceExplicit:
		timeConfidence = api.TimeConfidence_TIME_CONFIDENCE_EXPLICIT
	case ai.TimeConfidenceInferred:
		timeConfidence = api.TimeConfidence_TIME_CONFIDENCE_INFERRED
	case ai.TimeConfidenceUnknown:
		timeConfidence = api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN
	}

	// Post-AI fan-out. Text mode already ran the streaming variant above
	// (Mapbox + Pexels fired concurrently with the AI tokens) so we skip
	// the unary block here and carry forward streamingResult. Media and
	// webpage modes still use the unary fan-out — those paths have
	// different upstream shapes (image preprocessing, JSON-LD event
	// metadata) that haven't been wired into the streaming surface yet.
	var (
		fanoutStockMediaID string
		geocodedLocation   *api.GeocodedLocation
	)
	if streamingDone {
		fanoutStockMediaID = streamingResult.StockMediaID
		geocodedLocation = streamingResult.GeocodedLocation
	} else {
		needsStockMedia := len(mediaIds) == 0 && len(aiResponse.SearchKeywords) > 0 &&
			(s.stockVideoProvider != nil || s.stockImageryProvider != nil)
		needsMapbox := aiResponse.LocationQuery != "" &&
			aiResponse.LocationQuery != ai.LocationQueryUserPrimary &&
			s.locationProvider != nil

		proximity, proximitySource := fanout.ProximityForGeocoding(
			eventLatitude, eventLongitude,
			req.Msg.LatitudeDeg, req.Msg.LongitudeDeg,
			locationProximity,
		)
		proximityReq := locationlib.ProximityRequestFromSource(proximity, proximitySource)
		if needsMapbox {
			logger.Info("geocoding location query",
				"location_query", aiResponse.LocationQuery,
				"proximity_source", proximitySource)
		}

		branches := fanout.Branches{}
		if needsStockMedia {
			branches.StockMediaFn = func(ctx context.Context) string {
				return s.fetchStockMediaFallback(ctx, aiResponse.SearchKeywords, authInfo.UserID)
			}
		}
		if needsMapbox {
			branches.MapboxFn = func(ctx context.Context) *api.GeocodedLocation {
				return s.geocodePlaceQuery(ctx, aiResponse.LocationQuery, proximityReq, eventLocationName)
			}
		}

		fanoutStart := time.Now()
		fanoutResult := fanout.Run(ctx, branches)
		fanoutDuration := time.Since(fanoutStart)
		fanout.LogCompleted(logger, "GenExperience", fanoutDuration, needsStockMedia, needsMapbox, fanoutResult)

		fanoutStockMediaID = fanoutResult.StockMediaID
		geocodedLocation = fanoutResult.GeocodedLocation
	}

	if fanoutStockMediaID != "" {
		mediaIds = append(mediaIds, fanoutStockMediaID)
	}

	// Build final location fields. Mapbox result wins; otherwise fall back
	// to the user-primary-location helper for the USER_PRIMARY_LOCATION and
	// empty-query cases (both are fast storage lookups, not worth a goroutine).
	var locationID string
	if geocodedLocation == nil {
		switch aiResponse.LocationQuery {
		case ai.LocationQueryUserPrimary:
			if s.locationProvider != nil {
				locationID, geocodedLocation = locationlib.GetUserLocationFallbackLogged(ctx, s.storage, authInfo.UserID)
			}
		case "":
			logger.Debug("no location extracted by AI, falling back to user location")
			locationID, geocodedLocation = locationlib.GetUserLocationFallbackLogged(ctx, s.storage, authInfo.UserID)
		}
	}

	// Parse extracted date/time into Unix timestamp.
	// The AI returns date (YYYY-MM-DD) and time (HH:MM) as bare strings.
	// Parse them in the user's timezone since the LLM was told the user is in that timezone.
	var extractedTimeUnixSec int64
	if aiResponse.Date != "" {
		dateTimeStr := aiResponse.Date
		if aiResponse.Time != "" {
			dateTimeStr += "T" + aiResponse.Time + ":00"
		} else {
			dateTimeStr += "T00:00:00"
		}
		if parsedTime, err := time.ParseInLocation("2006-01-02T15:04:05", dateTimeStr, loc); err == nil {
			extractedTimeUnixSec = parsedTime.Unix()
		}
	}

	// Wire extracted time into suggested_time when confidence is actionable.
	// EXPLICIT or INFERRED both have enough signal for the client to pre-populate
	// the time picker; UNKNOWN stays as TBD so the user sets it manually.
	if extractedTimeUnixSec != 0 &&
		(timeConfidence == api.TimeConfidence_TIME_CONFIDENCE_EXPLICIT ||
			timeConfidence == api.TimeConfidence_TIME_CONFIDENCE_INFERRED) {
		suggestedTime = &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: extractedTimeUnixSec,
					Timezone:         userTimezone,
				},
			},
		}
	}

	// Build metadata from AI generation
	var metadata *api.ExperienceMetadata
	if aiResponse.ValueEstimate != nil {
		metadata = &api.ExperienceMetadata{
			ValueEstimate: &api.ValueEstimate{
				EstimatedValueUsd: aiResponse.ValueEstimate.EstimatedValueUSD,
				Provenance: &api.Provenance{
					Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
					Name:       "genai_value_estimate",
					Confidence: proto.Float32(float32(aiResponse.ValueEstimate.Confidence)),
					Sources:    aiResponse.ValueEstimate.Sources,
				},
			},
			Category: "", // TODO(#2013): Add category detection to AI provider
		}
	}

	// Return suggestions WITHOUT saving to database
	// This follows the pattern of GenRequest - client previews and can edit before saving
	return connect.NewResponse(&api.GenExperienceResponse{
		Name:                 aiResponse.Title,
		Description:          aiResponse.Description,
		MediaIds:             mediaIds,
		LocationId:           locationID, // Empty for now - client will create Location on confirm
		SuggestedTime:        suggestedTime,
		Tags:                 []string{}, // Could extract from AI reasoning in future
		TimeConfidence:       timeConfidence,
		ExtractedTimeUnixSec: extractedTimeUnixSec,
		LocationQuery:        aiResponse.LocationQuery,
		GeocodedLocation:     geocodedLocation, // Geocoded data for client to store on confirm
		SourceUrl:            sourceURL,        // Source URL for experiences created from websites
		Metadata:             metadata,
		MentionedNames:       aiResponse.MentionedNames, // Names extracted from prompt for participant suggestions
	}), nil
}

// generateFromMedia analyzes media using AI to generate experience content.
func (s *Service) generateFromMedia(ctx context.Context, mediaID, userID, region, currentTime string) (*ai.ExperienceGeneration, error) {
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

	aiResponse, err := s.aiProvider.GenerateExperienceFromImage(ctx, detectionImage, region, "", currentTime)
	if err != nil {
		logger.Error("AI generation from image failed", "error", err)
		return nil, connecterr.Internal(ctx, "generateFromMedia", err, "detail", "failed to analyze image")
	}

	logger.Info("generated experience from media")

	return aiResponse, nil
}

// webpageResult contains the results of webpage processing.
type webpageResult struct {
	aiResponse        *ai.ExperienceGeneration
	imageURL          string   // Primary image URL from the webpage (og:image, twitter:image, etc.)
	eventLatitude     *float64 // Geo latitude from JSON-LD Event (nil if not present).
	eventLongitude    *float64 // Geo longitude from JSON-LD Event (nil if not present).
	eventLocationName string   // Place name from JSON-LD Event (e.g., "Boulder Airport").
}

// generateFromWebpage fetches a webpage and uses AI to extract experience content.
// Also extracts the primary image URL for potential use as experience media.
func (s *Service) generateFromWebpage(ctx context.Context, websiteURL, region, currentTime string) (*webpageResult, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"website_url", websiteURL,
	)

	// Check if web fetcher is configured
	if s.webFetcher == nil {
		logger.Error("web fetcher not configured - URL mode unavailable")
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("web fetcher not configured"))
	}

	// Fetch webpage content
	pageContent, err := s.webFetcher.FetchPageContent(ctx, websiteURL)
	if err != nil {
		logger.Error("failed to fetch webpage", "error", err)
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("failed to fetch webpage: %w", err))
	}

	hasStructuredEvent := pageContent.Event != nil && pageContent.Event.StartDate != nil

	logger.Debug("fetched webpage content",
		"title", pageContent.Title,
		"description_length", len(pageContent.Description),
		"body_length", len(pageContent.BodyText),
		"image_url", pageContent.ImageURL,
		"has_structured_event", hasStructuredEvent,
	)

	// Call AI provider to extract experience details from webpage content.
	aiResponse, err := s.aiProvider.GenerateExperienceFromWebpage(
		ctx,
		pageContent.Title,
		pageContent.Description,
		pageContent.BodyText,
		region,
		currentTime,
	)
	if err != nil {
		ai.LogAIError(ctx, logger, "AI extraction from webpage failed", err)
		if ai.IsTransientError(err) {
			return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("AI provider temporarily unavailable"))
		}
		return nil, connecterr.Internal(ctx, "generateFromWebpage", err, "detail",
			"failed to extract event details")
	}

	if hasStructuredEvent {
		ev := pageContent.Event
		start := *ev.StartDate
		aiResponse.Date = start.Format("2006-01-02")
		aiResponse.Time = start.Format("15:04")
		aiResponse.TimeConfidence = ai.TimeConfidenceExplicit

		// Override AI-extracted location with structured event data when available.
		// JSON-LD locations are machine-readable and more reliable than LLM extraction.
		if ev.Location != "" {
			aiResponse.LocationQuery = ev.Location
		}

		logger.Info("overrode AI time with structured event data",
			"structured_date", aiResponse.Date,
			"structured_time", aiResponse.Time,
			"structured_location", ev.Location,
		)
	}

	logger.Info("extracted experience from webpage", "title", aiResponse.Title)

	result := &webpageResult{
		aiResponse: aiResponse,
		imageURL:   pageContent.ImageURL,
	}

	// Pass through structured event data for geocoding.
	if pageContent.Event != nil {
		result.eventLatitude = pageContent.Event.Latitude
		result.eventLongitude = pageContent.Event.Longitude
		result.eventLocationName = pageContent.Event.LocationName
	}

	return result, nil
}
