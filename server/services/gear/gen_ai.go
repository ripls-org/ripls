package gear

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	locationlib "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/product"
	"go.ripls.org/ripls/server/storage"
)

// urlRegex matches http:// or https:// URLs in text.
var urlRegex = regexp.MustCompile(`https?://[^\s]+`)

const (
	// minPromptLength is the minimum allowed prompt length in characters.
	minPromptLength = 3
	// maxPromptLength is the maximum allowed prompt length in characters.
	maxPromptLength = 500
)

// extractURL returns the URL only if the entire text is a single URL.
// Returns empty string if the text contains any non-URL content.
func extractURL(text string) string {
	trimmed := strings.TrimSpace(text)
	match := urlRegex.FindString(trimmed)
	if match == trimmed {
		return match
	}
	return ""
}

// GenGear generates gear suggestions from a text prompt or detects gear in an image using AI.
// Either prompt (text mode) or media_id (image mode) must be provided, but not both.
// If the prompt is entirely a URL, it will be used for URL-based generation.
// This method does NOT save the gear to the database - it only returns AI-generated
// suggestions for the client to preview and optionally edit before calling SaveGear to persist.
func (s *Service) GenGear(
	ctx context.Context,
	req *connect.Request[api.GenGearRequest],
) (*connect.Response[api.GenGearResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Validate that exactly one of prompt or media_id is provided
	hasPrompt := req.Msg.Prompt != ""
	hasMedia := req.Msg.MediaId != ""

	if !hasPrompt && !hasMedia {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("either prompt or media_id must be provided"))
	}

	if hasPrompt && hasMedia {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("provide either prompt or media_id, not both"))
	}

	// Check if AI provider is configured
	if s.aiProvider == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("AI provider not configured"))
	}

	// Route to appropriate generation mode
	if hasPrompt {
		// Check if prompt contains a URL - if so, use URL-based generation
		if extractedURL := extractURL(req.Msg.Prompt); extractedURL != "" {
			return s.genGearFromURL(ctx, authInfo, extractedURL)
		}
		return s.genGearFromText(ctx, req, authInfo)
	}
	return s.genGearFromMedia(ctx, req, authInfo)
}

// genGearFromText generates gear from a text prompt.
func (s *Service) genGearFromText(
	ctx context.Context,
	req *connect.Request[api.GenGearRequest],
	authInfo *auth.Info,
) (*connect.Response[api.GenGearResponse], error) {
	prompt := req.Msg.Prompt

	// Validate prompt length
	promptLength := utf8.RuneCountInString(prompt)
	if promptLength < minPromptLength {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("prompt must be at least %d characters", minPromptLength))
	}
	if promptLength > maxPromptLength {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("prompt must not exceed %d characters", maxPromptLength))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"prompt_length", promptLength,
	)

	logger.Info("generating gear from AI prompt")

	// Use empty region for now (could be enhanced later with community/user location)
	region := ""

	// Streaming AI + concurrent post-AI fan-out. Pexels fires the instant
	// title closes (Gear's stock-image trigger); Mapbox fires when
	// location_query closes. Closures inspect their trigger value to skip
	// the call when the AI returned empty / USER_PRIMARY_LOCATION.
	generation, streamingResult, err := s.runStreamingFanout(ctx, logger, prompt, region, authInfo.UserID)
	if err != nil {
		logger.Error("failed to generate gear content (streaming)", "error", err)
		return nil, connecterr.Internal(ctx, "genGearFromText", err)
	}

	// Clear placeholder values (e.g., "<UNKNOWN>", "N/A") from AI response.
	ai.SanitizeGearGeneration(generation)

	// Use the user's original text as the description verbatim.
	generation.Description = prompt

	logger.Info("generated gear content",
		"title", generation.Title,
		"confidence", generation.Confidence)

	mediaIds := []string{}
	if streamingResult.StockMediaID != "" {
		mediaIds = append(mediaIds, streamingResult.StockMediaID)
	}

	// Gear has no location-provider wiring today; the only fallback path
	// is "" (no AI-extracted query), where we resolve the user's primary
	// location from storage.
	geocodedLocation := streamingResult.GeocodedLocation
	if geocodedLocation == nil && generation.LocationQuery == "" {
		logger.Debug("no location extracted by AI, falling back to user location")
		_, geocodedLocation = locationlib.GetUserLocationFallbackLogged(ctx, s.storage, authInfo.UserID)
	}

	logger.Info("generated gear suggestions",
		"title", generation.Title,
		"has_geocoded_location", geocodedLocation != nil,
		"has_value_estimate", generation.ValueEstimate != nil)

	// Build detected gear response
	detectedGear := &api.DetectedGearItem{
		Title:            generation.Title,
		Description:      generation.Description,
		Confidence:       generation.Confidence,
		GeocodedLocation: geocodedLocation, // Geocoded data for client to store on confirm
		LocationQuery:    generation.LocationQuery,
		MediaIds:         mediaIds,
		Category:         generation.Category,
		Brand:            generation.Brand,
		MaterialCategory: ai.MaterialCategoryFromJSON(generation.MaterialCategory),
	}
	if generation.WeightGrams > 0 {
		detectedGear.WeightGrams = &api.Estimate{Mean: generation.WeightGrams, Stddev: generation.WeightGrams * 0.3}
	}

	// Include value estimate if returned by AI
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
		logger.Info("included value estimate from text generation",
			"value_usd", generation.ValueEstimate.EstimatedValueUSD,
			"value_confidence", generation.ValueEstimate.Confidence)
	}

	// Return suggestions WITHOUT saving to database
	// Client will preview and call SaveGear to persist
	return connect.NewResponse(&api.GenGearResponse{
		DetectedGear: detectedGear,
	}), nil
}

// genGearFromMedia detects gear in an uploaded image.
func (s *Service) genGearFromMedia(
	ctx context.Context,
	req *connect.Request[api.GenGearRequest],
	authInfo *auth.Info,
) (*connect.Response[api.GenGearResponse], error) {
	mediaID := req.Msg.GetMediaId()

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"media_id", mediaID,
	)
	logger.Info("requesting AI gear detection from media")

	// Fetch media from storage
	media := &models.Media{}
	err := s.storage.GetByID(ctx, mediaID, media)
	if err != nil {
		logger.Error("failed to get media", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Verify user has access to this media
	if media.UserId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user does not have access to this media"))
	}

	// Generate presigned URL for AI provider access (15 minute expiration)
	bucketKey := storage.MediaBucketKey(media.UserId, media.Id)
	presignedURL, err := s.bucket.GetSignedURL(ctx, bucketKey, 15*time.Minute)
	if err != nil {
		logger.Error("failed to generate presigned URL", "error", err)
		return nil, connecterr.Internal(ctx, "genGearFromMedia", err, "detail", "failed to generate media access URL")
	}

	logger.Debug("generated presigned URL for AI analysis", "content_type", media.ContentType)

	// Call AI provider with presigned URL
	startTime := time.Now()
	detection, err := s.aiProvider.DetectGearInImage(ctx, &ai.DetectionImage{
		ImageURL: presignedURL,
		MimeType: media.ContentType,
	})
	if err != nil {
		logger.Error("AI analysis failed", "error", err)
		return nil, connecterr.Internal(ctx, "genGearFromMedia", err)
	}
	processingTime := time.Since(startTime)

	// Use user's primary residence or most recent location as fallback
	// (Gear detection from images doesn't include location extraction)
	logger.Debug("using user location fallback for gear image")
	_, geocodedLocation := locationlib.GetUserLocationFallbackLogged(ctx, s.storage, authInfo.UserID)

	if detection == nil {
		logger.Info("AI gear detection complete",
			"item_detected", false,
			"duration_ms", processingTime.Milliseconds(),
		)
		return connect.NewResponse(&api.GenGearResponse{}), nil
	}

	logger.Info("AI gear detection complete",
		"item_detected", true,
		"brand", detection.Brand,
		"duration_ms", processingTime.Milliseconds(),
	)

	detectedGear := s.buildDetectedGearFromDetection(ctx, logger, detection, mediaID, geocodedLocation)
	return connect.NewResponse(&api.GenGearResponse{DetectedGear: detectedGear}), nil
}

// buildDetectedGearFromDetection runs the post-AI work shared by the unary
// and streaming gear-from-image paths: sanitize placeholders, attempt a
// product-lookup enhancement when brand confidence is high, derive a
// value estimate (preferring the looked-up specs over the AI's estimate),
// and assemble the final DetectedGearItem with the input media_id and
// user-primary-location fallback already attached.
func (s *Service) buildDetectedGearFromDetection(
	ctx context.Context,
	logger *logging.Logger,
	detection *ai.GearDetection,
	mediaID string,
	geocodedLocation *api.GeocodedLocation,
) *api.DetectedGearItem {
	ai.SanitizeGearDetection(detection)

	var valueEstimate *api.ValueEstimate
	if s.productLookup != nil && product.ShouldAttemptLookup(detection.Brand, float64(detection.Confidence)) {
		logger.Debug("attempting product spec lookup",
			"brand", detection.Brand,
			"confidence", detection.Confidence)

		productInfo := &product.Info{
			Brand:      detection.Brand,
			Model:      "",
			Confidence: float64(detection.Confidence),
		}

		specs, err := s.productLookup.LookupSpecs(ctx, productInfo)
		if err != nil {
			logger.Debug("product lookup failed, using AI-only detection", "error", err)
		} else if specs != nil {
			logger.Info("enhanced detection with product specs",
				"has_value_estimate", specs.ValueEstimate != nil)

			if specs.Description != "" && len(specs.Description) > len(detection.Description) {
				detection.Description = specs.Description
			}

			if specs.ValueEstimate != nil {
				specsProv := &api.Provenance{
					Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
					Name:       "genai_value_estimate",
					Confidence: proto.Float32(float32(specs.ValueEstimate.Confidence)),
					Sources:    specs.ValueEstimate.Sources,
				}
				if s.estimatorCfg != nil {
					specsProv.Version = s.estimatorCfg.ProvenanceVersion("genai_value_estimate")
				}
				valueEstimate = &api.ValueEstimate{
					EstimatedValueUsd: specs.ValueEstimate.EstimatedValueUSD,
					Provenance:        specsProv,
				}
			}
		}
	}

	if valueEstimate == nil && detection.ValueEstimate != nil {
		detProv := &api.Provenance{
			Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Name:       "genai_value_estimate",
			Confidence: proto.Float32(float32(detection.ValueEstimate.Confidence)),
			Sources:    detection.ValueEstimate.Sources,
		}
		if s.estimatorCfg != nil {
			detProv.Version = s.estimatorCfg.ProvenanceVersion("genai_value_estimate")
		}
		valueEstimate = &api.ValueEstimate{
			EstimatedValueUsd: detection.ValueEstimate.EstimatedValueUSD,
			Provenance:        detProv,
		}
		logger.Info("using AI detection value estimate",
			"value_usd", detection.ValueEstimate.EstimatedValueUSD,
			"value_confidence", detection.ValueEstimate.Confidence)
	}

	detectedGear := &api.DetectedGearItem{
		Title:            detection.Title,
		Description:      detection.Description,
		Confidence:       detection.Confidence,
		GeocodedLocation: geocodedLocation,
		MediaIds:         []string{mediaID},
		ValueEstimate:    valueEstimate,
		Category:         detection.Category,
		Brand:            detection.Brand,
		Model:            detection.Model,
		MaterialCategory: ai.MaterialCategoryFromJSON(detection.MaterialCategory),
	}
	if detection.WeightGrams > 0 {
		detectedGear.WeightGrams = &api.Estimate{Mean: detection.WeightGrams, Stddev: detection.WeightGrams * 0.3}
	}
	return detectedGear
}

// genGearFromURL generates gear from a product page URL extracted from the prompt.
// This extracts product information from the webpage and uses AI to generate gear details.
func (s *Service) genGearFromURL(
	ctx context.Context,
	authInfo *auth.Info,
	extractedURL string,
) (*connect.Response[api.GenGearResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"extracted_url", extractedURL,
	)
	logger.Info("generating gear from URL")

	// Check if web fetcher is configured
	if s.webFetcher == nil {
		logger.Error("web fetcher not configured - URL mode unavailable")
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("web fetcher not configured"))
	}

	// Fetch webpage content.
	pageContent, err := s.webFetcher.FetchPageContent(ctx, extractedURL)
	if err != nil {
		logger.Warn("failed to fetch webpage", "error", err)
		return nil, connectErrorFromFetchError(ctx, err)
	}

	logger.Debug("fetched webpage content",
		"title", pageContent.Title,
		"description_length", len(pageContent.Description),
		"body_length", len(pageContent.BodyText),
		"image_url", pageContent.ImageURL,
	)

	// Use empty region for now (could be enhanced later with community/user location)
	region := ""

	// Generate gear content using AI from webpage content with value extraction
	generation, err := s.aiProvider.GenerateGearFromWebpage(
		ctx,
		pageContent.Title,
		pageContent.Description,
		pageContent.BodyText,
		region,
	)
	if err != nil {
		logger.Error("failed to generate gear from webpage content", "error", err)
		return nil, connecterr.Internal(ctx, "genGearFromURL", err)
	}

	// Clear placeholder values (e.g., "<UNKNOWN>", "N/A") from AI response.
	ai.SanitizeGearGeneration(generation)

	logger.Info("generated gear content from webpage",
		"title", generation.Title,
		"confidence", generation.Confidence,
		"has_value_estimate", generation.ValueEstimate != nil)

	// Use user's primary residence or most recent location as fallback
	logger.Debug("using user location fallback for gear from URL")
	_, geocodedLocation := locationlib.GetUserLocationFallbackLogged(ctx, s.storage, authInfo.UserID)

	// Try to download the product image from the webpage
	mediaIds := []string{}
	if pageContent.ImageURL != "" {
		webpageMediaID, err := s.downloadAndStoreWebpageImage(ctx, pageContent.ImageURL, authInfo.UserID)
		if err != nil {
			// Log but don't fail - image download is optional
			logger.Warn("failed to download webpage image", "image_url", pageContent.ImageURL, "error", err)
		} else if webpageMediaID != "" {
			mediaIds = append(mediaIds, webpageMediaID)
			logger.Debug("using webpage image for gear", "media_id", webpageMediaID)
		}
	}

	// Fall back to stock imagery if no webpage image
	if len(mediaIds) == 0 && s.stockImageryProvider != nil && generation.Title != "" {
		stockMediaID, err := s.findAndStoreStockImage(ctx, generation.Title, authInfo.UserID)
		if err != nil {
			logger.Warn("failed to get stock image", "error", err)
		} else if stockMediaID != "" {
			mediaIds = append(mediaIds, stockMediaID)
		}
	}

	logger.Info("generated gear from URL",
		"title", generation.Title,
		"has_geocoded_location", geocodedLocation != nil,
		"has_media", len(mediaIds) > 0,
		"source_url", extractedURL)

	// Build the detected gear response
	detectedGear := &api.DetectedGearItem{
		Title:            generation.Title,
		Description:      generation.Description,
		Confidence:       generation.Confidence,
		GeocodedLocation: geocodedLocation,
		LocationQuery:    generation.LocationQuery,
		MediaIds:         mediaIds,
		SourceUrl:        extractedURL, // Track where the gear info came from
		Category:         generation.Category,
		Brand:            generation.Brand,
		MaterialCategory: ai.MaterialCategoryFromJSON(generation.MaterialCategory),
	}
	if generation.WeightGrams > 0 {
		detectedGear.WeightGrams = &api.Estimate{Mean: generation.WeightGrams, Stddev: generation.WeightGrams * 0.3}
	}

	// Add value estimate if available from AI
	if generation.ValueEstimate != nil {
		urlProv := &api.Provenance{
			Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Name:       "genai_value_estimate",
			Confidence: proto.Float32(float32(generation.ValueEstimate.Confidence)),
			Sources:    generation.ValueEstimate.Sources,
		}
		if s.estimatorCfg != nil {
			urlProv.Version = s.estimatorCfg.ProvenanceVersion("genai_value_estimate")
		}
		detectedGear.ValueEstimate = &api.ValueEstimate{
			EstimatedValueUsd: generation.ValueEstimate.EstimatedValueUSD,
			Provenance:        urlProv,
		}
		logger.Info("included value estimate",
			"value_usd", generation.ValueEstimate.EstimatedValueUSD,
			"value_confidence", generation.ValueEstimate.Confidence)
	}

	// Return suggestions with source_url and value_estimate populated
	return connect.NewResponse(&api.GenGearResponse{
		DetectedGear: detectedGear,
	}), nil
}
