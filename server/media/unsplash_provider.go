package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

const (
	// defaultSearchResults is the number of results to request from Unsplash.
	// We request multiple to support RequireUnique filtering.
	defaultSearchResults = 10

	// Unsplash API constants.
	unsplashAPIURL = "https://api.unsplash.com"
)

// Photo represents a photo from Unsplash.
type Photo struct {
	ID             string    `json:"id"`
	Description    string    `json:"description"`
	AltDescription string    `json:"alt_description"`
	URLs           PhotoURLs `json:"urls"`
	User           PhotoUser `json:"user"`
}

// PhotoURLs contains the different sizes of a photo.
type PhotoURLs struct {
	Raw     string `json:"raw"`
	Full    string `json:"full"`
	Regular string `json:"regular"`
	Small   string `json:"small"`
	Thumb   string `json:"thumb"`
}

// PhotoUser contains information about the photo's creator.
type PhotoUser struct {
	Name     string `json:"name"`
	Username string `json:"username"`
}

// SearchResponse represents the response from Unsplash's search API.
type SearchResponse struct {
	Total      int     `json:"total"`
	TotalPages int     `json:"total_pages"`
	Results    []Photo `json:"results"`
}

// UnsplashClient provides access to the Unsplash API.
type UnsplashClient struct {
	accessKey  string
	httpClient *http.Client
}

// NewUnsplashClient creates a new Unsplash API client.
// The client does not set an HTTP-level timeout; instead, it relies on context
// deadlines passed to each request for cancellation. This ensures that when a
// context deadline is exceeded, requests are cancelled immediately rather than
// waiting for a separate HTTP timeout.
func NewUnsplashClient(accessKey string) *UnsplashClient {
	return &UnsplashClient{
		accessKey:  accessKey,
		httpClient: &http.Client{},
	}
}

// SearchPhotos searches for photos on Unsplash.
// Returns a list of photos matching the query.
// This method tries multiple search strategies with fallbacks to maximize the chance of finding results.
func (c *UnsplashClient) SearchPhotos(ctx context.Context, query string, perPage int) (*SearchResponse, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	if perPage <= 0 || perPage > 30 {
		perPage = 10 // Default to 10 results
	}

	// Try multiple search strategies with fallbacks
	searchQueries := []string{query}

	// Extract keywords for alternative searches
	keywords := ExtractSearchKeywords(query)
	if len(keywords) > 0 {
		// Try keywords joined together
		searchQueries = append(searchQueries, strings.Join(keywords, " "))

		// If we have multiple keywords, try just the first 2-3 most important ones
		if len(keywords) > 3 {
			searchQueries = append(searchQueries, strings.Join(keywords[:3], " "))
		}

		// Try just the first keyword as a last resort
		searchQueries = append(searchQueries, keywords[0])
	}

	// Try each query until we get results
	var lastErr error
	for _, searchQuery := range searchQueries {
		resp, err := c.executeSearch(ctx, searchQuery, perPage)
		if err != nil {
			lastErr = err
			continue
		}

		// If we got results, return them
		if len(resp.Results) > 0 {
			return resp, nil
		}
	}

	// If we exhausted all queries without results, return the last error or empty results
	if lastErr != nil {
		return nil, lastErr
	}

	return &SearchResponse{Results: []Photo{}}, nil
}

// executeSearch performs a single Unsplash search request.
func (c *UnsplashClient) executeSearch(ctx context.Context, query string, perPage int) (*SearchResponse, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "unsplash",
		"operation", "executeSearch",
		"query", query,
	)
	startTime := time.Now()

	// Build URL with query parameters
	u, err := url.Parse(unsplashAPIURL + "/search/photos")
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	q.Set("query", query)
	q.Set("per_page", fmt.Sprintf("%d", perPage))
	u.RawQuery = q.Encode()

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authorization header
	req.Header.Set("Authorization", "Client-ID "+c.accessKey)
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error("unsplash API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("unsplash API call failed", "status", resp.StatusCode, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("unsplash API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		logger.Error("unsplash API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Info("unsplash API call completed", "results_count", len(searchResp.Results), "duration_ms", time.Since(startTime).Milliseconds())
	return &searchResp, nil
}

// DownloadPhoto downloads a photo from Unsplash and returns the image bytes.
// This function also triggers a download event on Unsplash (required by their API guidelines).
func (c *UnsplashClient) DownloadPhoto(ctx context.Context, photo *Photo) ([]byte, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "unsplash",
		"operation", "DownloadPhoto",
	)
	startTime := time.Now()

	if photo == nil {
		return nil, fmt.Errorf("photo cannot be nil")
	}

	logger = logger.With("photo_id", photo.ID)

	// Trigger download tracking (required by Unsplash API)
	if err := c.triggerDownload(ctx, photo.ID); err != nil {
		// Log but don't fail - this is for Unsplash's analytics
		logger.Warn("failed to trigger Unsplash download event", "error", err)
	}

	// Download the image (use full size for high quality)
	// Full is typically ~2400px wide, providing good quality for cards and detail views
	imageURL := photo.URLs.Full
	if imageURL == "" {
		// Fallback to regular if full isn't available
		imageURL = photo.URLs.Regular
		if imageURL == "" {
			return nil, fmt.Errorf("photo has no download URL")
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error("unsplash API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("unsplash API call failed", "status", resp.StatusCode, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	// Read image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("unsplash API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	logger.Info("unsplash API call completed", "image_size_bytes", len(imageData), "duration_ms", time.Since(startTime).Milliseconds())
	return imageData, nil
}

// triggerDownload triggers a download event on Unsplash (required by API guidelines).
func (c *UnsplashClient) triggerDownload(ctx context.Context, photoID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "unsplash",
		"operation", "triggerDownload",
		"photo_id", photoID,
	)
	startTime := time.Now()

	u := fmt.Sprintf("%s/photos/%s/download", unsplashAPIURL, photoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Client-ID "+c.accessKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Error("unsplash API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error("unsplash API call failed", "status", resp.StatusCode, "duration_ms", time.Since(startTime).Milliseconds())
		return fmt.Errorf("download trigger failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	logger.Info("unsplash API call completed", "duration_ms", time.Since(startTime).Milliseconds())
	return nil
}

// GetPhoto fetches a single Unsplash photo by its Unsplash-assigned id.
// Used by the candidate-import path so the Replace Media swap can
// download the exact photo the client picked while preserving
// photographer attribution. Mirrors PexelsClient.GetPhotoByID; see
// #2063.
func (c *UnsplashClient) GetPhoto(ctx context.Context, photoID string) (*Photo, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "unsplash",
		"operation", "GetPhoto",
		"photo_id", photoID,
	)
	startTime := time.Now()

	requestURL := fmt.Sprintf("%s/photos/%s", unsplashAPIURL, url.PathEscape(photoID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Client-ID "+c.accessKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.WarnContext(ctx, "unsplash photo lookup failed",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("unsplash photo lookup failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.WarnContext(ctx, "unsplash photo lookup non-OK",
			"status_code", resp.StatusCode,
			"response_body", string(body),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("unsplash photo lookup: %w", ErrRateLimited)
		}
		return nil, fmt.Errorf("unsplash photo lookup: status %d", resp.StatusCode)
	}

	var photo Photo
	if err := json.NewDecoder(resp.Body).Decode(&photo); err != nil {
		return nil, fmt.Errorf("decode unsplash photo: %w", err)
	}
	logger.InfoContext(ctx, "unsplash photo lookup completed",
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return &photo, nil
}

// ExtractSearchKeywords extracts useful keywords from a request description for better Unsplash search results.
// It removes common filler words and focuses on nouns that are likely to match photos.
func ExtractSearchKeywords(description string) []string {
	// Common filler words to remove
	fillerWords := map[string]bool{
		"looking": true, "for": true, "need": true, "want": true, "requesting": true,
		"seeking": true, "in": true, "search": true, "of": true, "to": true, "a": true,
		"an": true, "the": true, "some": true, "any": true, "my": true, "your": true,
		"our": true, "their": true, "i": true, "we": true, "you": true, "they": true,
		"am": true, "is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "should": true, "could": true,
		"may": true, "might": true, "must": true, "can": true, "and": true, "or": true,
		"but": true, "if": true, "then": true, "else": true, "when": true, "where": true,
		"who": true, "what": true, "how": true, "why": true, "with": true, "without": true,
	}

	// Convert to lowercase and split into words
	words := strings.Fields(strings.ToLower(description))

	// Filter out filler words and keep meaningful keywords
	keywords := make([]string, 0, len(words))
	for _, word := range words {
		// Remove punctuation
		word = strings.Trim(word, ".,!?;:")
		// Keep if not a filler word and has reasonable length
		if !fillerWords[word] && len(word) > 2 {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// GetPhotoAttribution returns the attribution text for a photo.
// This is required when displaying Unsplash photos per their API guidelines.
func GetPhotoAttribution(photo *Photo) string {
	if photo == nil {
		return ""
	}
	return fmt.Sprintf("Photo by %s on Unsplash", photo.User.Name)
}

// unsplashClientInterface defines the methods needed from UnsplashClient.
// This allows for testing with mock implementations.
type unsplashClientInterface interface {
	SearchPhotos(ctx context.Context, query string, perPage int) (*SearchResponse, error)
	DownloadPhoto(ctx context.Context, photo *Photo) ([]byte, error)
	GetPhoto(ctx context.Context, photoID string) (*Photo, error)
}

// UnsplashProvider implements StockImageryProvider using the Unsplash API.
type UnsplashProvider struct {
	client  unsplashClientInterface
	storage *storage.ProtoSQLStorage
	bucket  storage.BucketStorage
}

// NewUnsplashProvider creates a new Unsplash-based stock imagery provider.
func NewUnsplashProvider(
	client *UnsplashClient,
	sqlStorage *storage.ProtoSQLStorage,
	bucket storage.BucketStorage,
) *UnsplashProvider {
	return &UnsplashProvider{
		client:  client,
		storage: sqlStorage,
		bucket:  bucket,
	}
}

// GetStockImage fetches a stock image from Unsplash matching the query.
func (p *UnsplashProvider) GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "unsplash",
		"operation", "GetStockImage",
		"query", query,
	)
	startTime := time.Now()

	if opts == nil {
		opts = &StockImageOptions{}
	}

	// Search for photos
	searchResp, err := p.client.SearchPhotos(ctx, query, defaultSearchResults)
	if err != nil {
		logger.ErrorContext(ctx, "stock imagery fetch failed",
			"failure_type", "search_error",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to search Unsplash: %w", err)
	}

	if len(searchResp.Results) == 0 {
		logger.WarnContext(ctx, "stock imagery fetch failed",
			"failure_type", "no_results",
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("no images found for query: %s", query)
	}

	// Filter results if uniqueness is required
	var photo *Photo
	if opts.RequireUnique {
		photo, err = p.selectUniquePhoto(ctx, searchResp.Results)
		if err != nil {
			logger.ErrorContext(ctx, "stock imagery fetch failed",
				"failure_type", "all_results_used",
				"error", err,
				"duration_ms", time.Since(startTime).Milliseconds())
			return nil, err
		}
	} else {
		photo = &searchResp.Results[0]

		// Check if we already have this image stored
		existing, err := p.getExistingStockImage(ctx, photo.ID)
		if err == nil && existing != nil {
			logger.InfoContext(ctx, "stock imagery fetch succeeded",
				"result", "cached",
				"stock_image_id", existing.Id,
				"media_id", existing.MediaId,
				"photo_id", photo.ID,
				"duration_ms", time.Since(startTime).Milliseconds(),
			)
			return existing, nil
		}
	}

	return p.storeUnsplashPhotoAsStockImage(ctx, photo, startTime)
}

// storeUnsplashPhotoAsStockImage handles the shared "download + store +
// insert StockImage" path used by both GetStockImage (search-then-pick)
// and GetStockImageByID (direct lookup). Mirrors
// PexelsProvider.storePexelsPhotoAsStockImage. See #2063.
func (p *UnsplashProvider) storeUnsplashPhotoAsStockImage(ctx context.Context, photo *Photo, startTime time.Time) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "unsplash",
		"operation", "storeUnsplashPhotoAsStockImage",
		"photo_id", photo.ID,
	)

	imageData, err := p.client.DownloadPhoto(ctx, photo)
	if err != nil {
		logger.ErrorContext(ctx, "stock imagery fetch failed",
			"failure_type", "download_error",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	description := photo.Description
	if description == "" {
		description = photo.AltDescription
	}
	mediaID, err := storage.StoreMedia(
		ctx,
		p.storage,
		p.bucket,
		SystemUserID,
		imageData,
		"image/jpeg",
		fmt.Sprintf("unsplash_%s.jpg", photo.ID),
		description,
		"",    // Canonical stock images don't reference themselves
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		logger.ErrorContext(ctx, "stock imagery fetch failed",
			"failure_type", "storage_error",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to store media: %w", err)
	}

	stockImage := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
		ProviderImage: &models.ProviderImage{
			Id:             photo.ID,
			Url:            photo.URLs.Full,
			Description:    photo.Description,
			AltDescription: photo.AltDescription,
			Creator: &models.CreatorInfo{
				Name:     photo.User.Name,
				Username: photo.User.Username,
			},
		},
		MediaId:          mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}

	stockImageID, err := p.storage.Insert(ctx, stockImage)
	if err != nil {
		logger.ErrorContext(ctx, "stock imagery fetch failed",
			"failure_type", "database_error",
			"error", err,
			"media_id", mediaID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to insert stock image record: %w", err)
	}
	stockImage.Id = stockImageID

	logger.InfoContext(ctx, "stock imagery fetch succeeded",
		"result", "new",
		"stock_image_id", stockImageID,
		"media_id", mediaID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return stockImage, nil
}

// getExistingStockImage returns an existing StockImage for the given provider image ID, if one exists.
func (p *UnsplashProvider) getExistingStockImage(ctx context.Context, providerImageID string) (*models.StockImage, error) {
	results, err := p.storage.QueryByField(ctx, "provider_image_id", providerImageID, &models.StockImage{})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0].(*models.StockImage), nil
}

// selectUniquePhoto finds a photo that hasn't been used before in Ripls.
func (p *UnsplashProvider) selectUniquePhoto(ctx context.Context, photos []Photo) (*Photo, error) {
	// Get all provider image IDs from the search results
	providerIDs := make([]string, len(photos))
	for i, photo := range photos {
		providerIDs[i] = photo.ID
	}

	// Query which ones already exist
	existingIDs, err := p.getExistingProviderImageIDs(ctx, providerIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing images: %w", err)
	}

	// Find first photo that doesn't exist
	existingSet := make(map[string]bool, len(existingIDs))
	for _, id := range existingIDs {
		existingSet[id] = true
	}

	for i := range photos {
		if !existingSet[photos[i].ID] {
			return &photos[i], nil
		}
	}

	return nil, fmt.Errorf("all %d search results already exist in Ripls", len(photos))
}

// getExistingProviderImageIDs returns which of the given provider image IDs already exist.
func (p *UnsplashProvider) getExistingProviderImageIDs(ctx context.Context, providerIDs []string) ([]string, error) {
	var existing []string
	for _, id := range providerIDs {
		// Query by the flattened field name
		results, err := p.storage.QueryByField(ctx, "provider_image_id", id, &models.StockImage{})
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			existing = append(existing, id)
		}
	}
	return existing, nil
}

// GetStockImageByID downloads the Unsplash photo identified by photoID
// and stores it as a canonical StockImage (deduped against existing).
// Used by the Replace Media import path so the swap preserves
// photographer attribution. Skips the search step that GetStockImage
// runs; otherwise behaves identically. See #2063.
func (p *UnsplashProvider) GetStockImageByID(ctx context.Context, photoID string) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "unsplash",
		"operation", "GetStockImageByID",
		"photo_id", photoID,
	)
	startTime := time.Now()

	// Dedupe: if we already have this photo stored, return the existing
	// canonical StockImage. The candidate-import path will then create a
	// per-user copy via CopyStockImageForUser.
	if existing, err := p.getExistingStockImage(ctx, photoID); err == nil && existing != nil {
		logger.InfoContext(ctx, "stock imagery by-id returning cached",
			"stock_image_id", existing.Id,
			"media_id", existing.MediaId,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return existing, nil
	}

	photo, err := p.client.GetPhoto(ctx, photoID)
	if err != nil {
		logger.WarnContext(ctx, "unsplash photo lookup failed",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("unsplash photo by id: %w", err)
	}
	return p.storeUnsplashPhotoAsStockImage(ctx, photo, startTime)
}

// SearchStockImageCandidates returns up to `limit` candidate photos for
// `query`. Mirrors PexelsProvider.SearchStockImageCandidates so the
// fallback chain (CachedStockImageryProvider → FallbackStockImageryProvider
// → primary) can surface alternates regardless of which provider serves
// the chosen image.
func (p *UnsplashProvider) SearchStockImageCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "unsplash",
		"operation", "SearchStockImageCandidates",
		"query", query,
		"limit", limit,
	)
	startTime := time.Now()

	perPage := limit + 2
	if perPage > defaultSearchResults {
		perPage = defaultSearchResults
	}
	resp, err := p.client.SearchPhotos(ctx, query, perPage)
	if err != nil {
		logger.WarnContext(ctx, "candidate search failed",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("failed to search Unsplash candidates: %w", err)
	}
	if len(resp.Results) == 0 {
		return nil, nil
	}

	out := make([]StockImageCandidate, 0, limit)
	for i := range resp.Results {
		if len(out) >= limit {
			break
		}
		photo := &resp.Results[i]
		// URL is the import target (what AddMediaFromURL fetches when
		// Unsplash's by-id path is unimplemented). Pick Regular
		// (~1080px) so the hero renders sharply; small thumbnails are
		// served from ThumbnailURL instead.
		importURL := photo.URLs.Regular
		if importURL == "" {
			importURL = photo.URLs.Full
		}
		if importURL == "" {
			importURL = photo.URLs.Small
		}
		if importURL == "" {
			continue
		}
		thumbURL := photo.URLs.Small
		if thumbURL == "" {
			thumbURL = importURL
		}
		out = append(out, StockImageCandidate{
			URL:             importURL,
			ContentType:     "image/jpeg",
			Provider:        models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
			ProviderPhotoID: photo.ID,
			ThumbnailURL:    thumbURL,
		})
	}
	logger.DebugContext(ctx, "candidate search succeeded",
		"result_count", len(out),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return out, nil
}

// CheckHealth validates Unsplash API connectivity with a minimal search request.
func (p *UnsplashProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()

	status := &health.Status{
		Name:    "stock_imagery",
		Backend: "unsplash",
	}

	// Use a minimal search to validate API key and connectivity
	u, err := url.Parse(unsplashAPIURL + "/search/photos")
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}

	q := u.Query()
	q.Set("query", "test")
	q.Set("per_page", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}

	// Need to get the access key from the client - access it via the underlying client
	client, ok := p.client.(*UnsplashClient)
	if !ok {
		// For mock clients in tests, just return healthy
		return []*health.Status{status}, nil
	}
	req.Header.Set("Authorization", "Client-ID "+client.accessKey)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}
	defer resp.Body.Close()

	status.LatencyMs = time.Since(start).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		status.Error = fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))
		return []*health.Status{status}, nil
	}

	return []*health.Status{status}, nil
}
