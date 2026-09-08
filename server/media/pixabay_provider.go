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
	pixabayVideoAPIURL = "https://pixabay.com/api/videos/"
)

// PixabayVideoQuality holds the URL and dimensions for one quality tier of a Pixabay video.
type PixabayVideoQuality struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Size   int    `json:"size"`
}

// PixabayVideoVariants holds the different quality tiers available for a Pixabay video.
type PixabayVideoVariants struct {
	Large  PixabayVideoQuality `json:"large"`
	Medium PixabayVideoQuality `json:"medium"`
	Small  PixabayVideoQuality `json:"small"`
	Tiny   PixabayVideoQuality `json:"tiny"`
}

// PixabayVideoHit represents a single video result returned by the Pixabay Video API.
type PixabayVideoHit struct {
	ID      int                  `json:"id"`
	PageURL string               `json:"pageURL"`
	Tags    string               `json:"tags"`
	Videos  PixabayVideoVariants `json:"videos"`
	User    string               `json:"user"`
	UserID  int                  `json:"user_id"`
}

// PixabayVideoSearchResponse represents the response from the Pixabay Video API.
type PixabayVideoSearchResponse struct {
	Total     int               `json:"total"`
	TotalHits int               `json:"totalHits"`
	Hits      []PixabayVideoHit `json:"hits"`
}

// PixabayClient handles communication with the Pixabay Video API.
type PixabayClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewPixabayClient creates a new Pixabay API client.
// The client does not set an HTTP-level timeout; instead, it relies on context
// deadlines passed to each request for cancellation.
func NewPixabayClient(apiKey string) *PixabayClient {
	return &PixabayClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// SearchVideos searches for videos on Pixabay with fallback strategies.
// It tries progressively simplified queries until results are found or exhausted.
func (c *PixabayClient) SearchVideos(ctx context.Context, query string, perPage int) (*PixabayVideoSearchResponse, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	if perPage <= 0 {
		perPage = defaultSearchResults
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PixabayClient.SearchVideos",
		"query", query,
		"per_page", perPage,
	)

	// Try multiple search strategies with fallbacks.
	searchQueries := []string{query}
	keywords := ExtractSearchKeywords(query)
	if len(keywords) > 0 {
		searchQueries = append(searchQueries, strings.Join(keywords, " "))
		if len(keywords) > 3 {
			searchQueries = append(searchQueries, strings.Join(keywords[:3], " "))
		}
		searchQueries = append(searchQueries, keywords[0])
	}

	var lastErr error
	for _, searchQuery := range searchQueries {
		resp, err := c.executeSearch(ctx, searchQuery, perPage)
		if err != nil {
			lastErr = err
			if isRateLimited(err) {
				logger.WarnContext(ctx, "rate limited, skipping remaining video search fallbacks",
					"search_query", searchQuery)
				break
			}
			continue
		}
		if len(resp.Hits) > 0 {
			logger.InfoContext(ctx, "video search succeeded",
				"search_query", searchQuery,
				"result_count", len(resp.Hits))
			return resp, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("pixabay video search failed: %w", lastErr)
	}

	return &PixabayVideoSearchResponse{Hits: []PixabayVideoHit{}}, nil
}

// executeSearch performs a single HTTP request to the Pixabay Video API.
func (c *PixabayClient) executeSearch(ctx context.Context, query string, perPage int) (*PixabayVideoSearchResponse, error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PixabayClient.executeSearch",
		"query", query,
	)

	// Pixabay auth uses a `key` query parameter, not an Authorization header.
	requestURL := fmt.Sprintf("%s?key=%s&q=%s&per_page=%d&video_type=film&orientation=vertical",
		pixabayVideoAPIURL,
		url.QueryEscape(c.apiKey),
		url.QueryEscape(query),
		perPage,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		duration := time.Since(startTime).Milliseconds()
		logger.ErrorContext(ctx, "HTTP request failed",
			"error", err,
			"duration_ms", duration)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime).Milliseconds()

	// Parse rate limit headers for observability.
	rateLimitRemaining := resp.Header.Get("X-Ratelimit-Remaining")
	rateLimitLimit := resp.Header.Get("X-Ratelimit-Limit")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.ErrorContext(ctx, "pixabay video API returned error",
			"status_code", resp.StatusCode,
			"response_body", string(body),
			"duration_ms", duration,
			"rate_limit_remaining", rateLimitRemaining,
			"rate_limit_limit", rateLimitLimit)
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("pixabay video API: %w", ErrRateLimited)
		}
		return nil, fmt.Errorf("pixabay video API error: status %d", resp.StatusCode)
	}

	var searchResp PixabayVideoSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		logger.ErrorContext(ctx, "failed to decode pixabay response",
			"error", err,
			"duration_ms", duration)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.InfoContext(ctx, "pixabay video search completed",
		"result_count", len(searchResp.Hits),
		"duration_ms", duration,
		"rate_limit_remaining", rateLimitRemaining,
		"rate_limit_limit", rateLimitLimit)

	return &searchResp, nil
}

// DownloadVideo downloads video bytes from the given direct URL.
func (c *PixabayClient) DownloadVideo(ctx context.Context, videoURL string) ([]byte, error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PixabayClient.DownloadVideo",
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, videoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		duration := time.Since(startTime).Milliseconds()
		logger.ErrorContext(ctx, "video download request failed",
			"error", err,
			"duration_ms", duration)
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		logger.ErrorContext(ctx, "video download failed with non-200 status",
			"status_code", resp.StatusCode,
			"duration_ms", duration)
		return nil, fmt.Errorf("video download failed: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorContext(ctx, "failed to read video response body",
			"error", err,
			"duration_ms", duration)
		return nil, fmt.Errorf("failed to read video data: %w", err)
	}

	logger.InfoContext(ctx, "video downloaded successfully",
		"duration_ms", duration,
		"size_bytes", len(data))

	return data, nil
}

// pixabayClientInterface defines the methods needed from PixabayClient for testing.
type pixabayClientInterface interface {
	SearchVideos(ctx context.Context, query string, perPage int) (*PixabayVideoSearchResponse, error)
	DownloadVideo(ctx context.Context, videoURL string) ([]byte, error)
}

// PixabayProvider implements StockVideoProvider using the Pixabay Video API.
type PixabayProvider struct {
	client  pixabayClientInterface
	storage *storage.ProtoSQLStorage
	bucket  storage.BucketStorage
}

// NewPixabayProvider creates a new Pixabay-based stock video provider.
func NewPixabayProvider(
	client *PixabayClient,
	sqlStorage *storage.ProtoSQLStorage,
	bucket storage.BucketStorage,
) *PixabayProvider {
	return &PixabayProvider{
		client:  client,
		storage: sqlStorage,
		bucket:  bucket,
	}
}

// GetStockVideo fetches a stock video from Pixabay matching the query.
// Returns a StockImage record whose media has content_type video/mp4.
// ProviderImage.Id uses the "video_pixabay_<id>" prefix to distinguish from
// Pexels videos ("video_<id>") in the shared stock_image table.
func (p *PixabayProvider) GetStockVideo(ctx context.Context, query string) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_video",
		"provider", "pixabay",
		"operation", "GetStockVideo",
		"query", query,
	)
	startTime := time.Now()

	searchResp, err := p.client.SearchVideos(ctx, query, defaultSearchResults)
	if err != nil {
		logger.ErrorContext(ctx, "stock video fetch failed",
			"failure_type", "search_error",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to search Pixabay videos: %w", err)
	}

	if len(searchResp.Hits) == 0 {
		logger.WarnContext(ctx, "stock video fetch failed",
			"failure_type", "no_results",
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("no videos found for query: %s", query)
	}

	hit := &searchResp.Hits[0]

	videoURL, width, height := selectBestPixabayVideo(hit)
	if videoURL == "" {
		return nil, fmt.Errorf("no usable video files for Pixabay video %d", hit.ID)
	}

	videoData, err := p.client.DownloadVideo(ctx, videoURL)
	if err != nil {
		logger.ErrorContext(ctx, "stock video fetch failed",
			"failure_type", "download_error",
			"error", err,
			"video_id", hit.ID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to download video: %w", err)
	}

	description := fmt.Sprintf("Video by %s on Pixabay", hit.User)
	mediaID, err := storage.StoreMedia(
		ctx,
		p.storage,
		p.bucket,
		SystemUserID,
		videoData,
		"video/mp4",
		fmt.Sprintf("pixabay_video_%d.mp4", hit.ID),
		description,
		"",
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		logger.ErrorContext(ctx, "stock video fetch failed",
			"failure_type", "storage_error",
			"error", err,
			"video_id", hit.ID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to store video: %w", err)
	}

	// Use "video_pixabay_<id>" to distinguish from Pexels videos ("video_<id>").
	// Both prefixes start with "video_", so CachedStockVideoProvider treats them
	// as cache hits equally.
	providerImage := &models.ProviderImage{
		Id:          fmt.Sprintf("video_pixabay_%d", hit.ID),
		Url:         hit.PageURL,
		Description: description,
		Creator: &models.CreatorInfo{
			Name: hit.User,
		},
	}

	stockImage := &models.StockImage{
		Provider:         models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPECIFIED,
		ProviderImage:    providerImage,
		MediaId:          mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}

	stockImageID, err := p.storage.Insert(ctx, stockImage)
	if err != nil {
		logger.ErrorContext(ctx, "stock video fetch failed",
			"failure_type", "database_error",
			"error", err,
			"media_id", mediaID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to insert stock video record: %w", err)
	}
	stockImage.Id = stockImageID

	logger.InfoContext(ctx, "stock video fetch succeeded",
		"stock_image_id", stockImageID,
		"media_id", mediaID,
		"video_id", hit.ID,
		"width", width,
		"height", height,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return stockImage, nil
}

// selectBestPixabayVideo selects the best quality video URL from a Pixabay hit.
// Preference order: portrait medium/large → any portrait → medium → any available.
func selectBestPixabayVideo(hit *PixabayVideoHit) (videoURL string, width, height int) {
	candidates := []PixabayVideoQuality{
		hit.Videos.Large,
		hit.Videos.Medium,
		hit.Videos.Small,
		hit.Videos.Tiny,
	}

	// Priority 1: Portrait orientation (height > width) in large or medium quality.
	for _, q := range candidates[:2] {
		if q.URL != "" && q.Height > q.Width {
			return q.URL, q.Width, q.Height
		}
	}

	// Priority 2: Any portrait orientation.
	for _, q := range candidates {
		if q.URL != "" && q.Height > q.Width {
			return q.URL, q.Width, q.Height
		}
	}

	// Priority 3: Medium quality in any orientation.
	if hit.Videos.Medium.URL != "" {
		q := hit.Videos.Medium
		return q.URL, q.Width, q.Height
	}

	// Priority 4: First non-empty quality level.
	for _, q := range candidates {
		if q.URL != "" {
			return q.URL, q.Width, q.Height
		}
	}

	return "", 0, 0
}

// GetStockVideoByID is not yet implemented for Pixabay. Returns an
// "unsupported" error so callers fall back to the generic-URL import
// path.
func (p *PixabayProvider) GetStockVideoByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("pixabay by-id lookup not supported")
}

// SearchStockVideoCandidates is not yet supported on Pixabay — the
// Pixabay Video API does not surface a poster-image URL on each result,
// only the page URL and the video MP4 variants. Returning nil keeps the
// fallback chain compatible while we explore whether a derived poster
// URL (e.g. constructed from the Pixabay vimeocdn pattern) is reliable
// enough to use. See #2038 follow-up.
func (p *PixabayProvider) SearchStockVideoCandidates(_ context.Context, _ string, _ int) ([]StockImageCandidate, error) {
	return nil, nil
}

// CheckHealth validates Pixabay API connectivity with a minimal search request.
func (p *PixabayProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()

	status := &health.Status{
		Name:    "stock_video",
		Backend: "pixabay",
	}

	client, ok := p.client.(*PixabayClient)
	if !ok {
		// For mock clients in tests, return healthy without an actual HTTP call.
		return []*health.Status{status}, nil
	}

	// Use a unique query with timestamp to validate the API key and connectivity.
	query := fmt.Sprintf("healthcheck%d", time.Now().UnixNano())
	requestURL := fmt.Sprintf("%s?key=%s&q=%s&per_page=1",
		pixabayVideoAPIURL,
		url.QueryEscape(client.apiKey),
		url.QueryEscape(query),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}

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
