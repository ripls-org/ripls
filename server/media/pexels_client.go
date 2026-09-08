package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.ripls.org/ripls/server/logging"
)

const (
	pexelsAPIURL      = "https://api.pexels.com/v1"
	pexelsVideoAPIURL = "https://api.pexels.com/videos"
)

// ErrRateLimited indicates the Pexels API returned HTTP 429 (too many requests).
var ErrRateLimited = errors.New("pexels API rate limited")

// isRateLimited reports whether the error represents a Pexels 429 rate limit response.
func isRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

// PexelsPhotoSrc contains URLs for different sizes of a photo.
type PexelsPhotoSrc struct {
	Original  string `json:"original"`
	Large2x   string `json:"large2x"`
	Large     string `json:"large"`
	Medium    string `json:"medium"`
	Small     string `json:"small"`
	Portrait  string `json:"portrait"`
	Landscape string `json:"landscape"`
	Tiny      string `json:"tiny"`
}

// PexelsPhoto represents a photo from Pexels.
type PexelsPhoto struct {
	ID              int            `json:"id"`
	Width           int            `json:"width"`
	Height          int            `json:"height"`
	URL             string         `json:"url"`
	Photographer    string         `json:"photographer"`
	PhotographerURL string         `json:"photographer_url"`
	PhotographerID  int            `json:"photographer_id"`
	AvgColor        string         `json:"avg_color"`
	Src             PexelsPhotoSrc `json:"src"`
	Alt             string         `json:"alt"`
}

// PexelsSearchResponse represents the response from Pexels search API.
type PexelsSearchResponse struct {
	Page         int           `json:"page"`
	PerPage      int           `json:"per_page"`
	Photos       []PexelsPhoto `json:"photos"`
	TotalResults int           `json:"total_results"`
	NextPage     string        `json:"next_page"`
}

// PexelsVideoFile represents a single video file format from the Pexels Videos API.
type PexelsVideoFile struct {
	ID      int     `json:"id"`
	Quality string  `json:"quality"` // "uhd", "hd", "sd"
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	Fps     float64 `json:"fps"`
	// Size is the file size in bytes. Pexels exposes this on each
	// video file so callers can pick a variant that fits a body cap
	// without HEAD-checking the URL.
	Size int64  `json:"size"`
	Link string `json:"link"`
}

// PexelsVideoUser represents the creator of a Pexels video.
type PexelsVideoUser struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// PexelsVideo represents a video from the Pexels Videos API.
type PexelsVideo struct {
	ID         int               `json:"id"`
	URL        string            `json:"url"`   // Video page URL (for attribution)
	Image      string            `json:"image"` // Poster/thumbnail image URL
	User       PexelsVideoUser   `json:"user"`
	VideoFiles []PexelsVideoFile `json:"video_files"`
}

// PexelsVideoSearchResponse represents the response from the Pexels video search API.
type PexelsVideoSearchResponse struct {
	Page         int           `json:"page"`
	PerPage      int           `json:"per_page"`
	Videos       []PexelsVideo `json:"videos"`
	TotalResults int           `json:"total_results"`
}

// PexelsClient handles communication with the Pexels API.
type PexelsClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewPexelsClient creates a new Pexels API client.
// The client does not set an HTTP-level timeout; instead, it relies on context
// deadlines passed to each request for cancellation. This ensures that when a
// context deadline is exceeded, requests are cancelled immediately rather than
// waiting for a separate HTTP timeout.
func NewPexelsClient(apiKey string) *PexelsClient {
	return &PexelsClient{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// SearchPhotos searches for photos on Pexels with fallback strategies.
func (c *PexelsClient) SearchPhotos(ctx context.Context, query string, perPage int) (*PexelsSearchResponse, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	if perPage <= 0 {
		perPage = defaultSearchResults
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsClient.SearchPhotos",
		"query", query,
		"per_page", perPage,
	)

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
			if isRateLimited(err) {
				logger.WarnContext(ctx, "rate limited, skipping remaining photo search fallbacks",
					"search_query", searchQuery)
				break
			}
			continue
		}

		// If we got results, return them
		if len(resp.Photos) > 0 {
			logger.InfoContext(ctx, "search succeeded",
				"search_query", searchQuery,
				"result_count", len(resp.Photos))
			return resp, nil
		}
	}

	// If we exhausted all queries without results, return the last error or empty results
	if lastErr != nil {
		return nil, fmt.Errorf("pexels search failed: %w", lastErr)
	}

	return &PexelsSearchResponse{Photos: []PexelsPhoto{}}, nil
}

// executeSearch performs the actual HTTP request to Pexels API.
func (c *PexelsClient) executeSearch(ctx context.Context, query string, perPage int) (*PexelsSearchResponse, error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsClient.executeSearch",
		"query", query,
	)

	if perPage <= 0 {
		perPage = defaultSearchResults
	}

	requestURL := fmt.Sprintf("%s/search?query=%s&per_page=%d", pexelsAPIURL, url.QueryEscape(query), perPage)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", c.apiKey)

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

	// Parse rate limit headers
	rateLimitRemaining := resp.Header.Get("X-Ratelimit-Remaining")
	rateLimitLimit := resp.Header.Get("X-Ratelimit-Limit")
	rateLimitReset := resp.Header.Get("X-Ratelimit-Reset")

	logFields := []any{
		"duration_ms", duration,
		"status_code", resp.StatusCode,
		"rate_limit_remaining", rateLimitRemaining,
		"rate_limit_limit", rateLimitLimit,
		"rate_limit_reset", rateLimitReset,
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.ErrorContext(ctx, "pexels API returned error",
			append(logFields, "response_body", string(body))...)
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("pexels photo API: %w", ErrRateLimited)
		}
		return nil, fmt.Errorf("pexels API error: status %d", resp.StatusCode)
	}

	var searchResp PexelsSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		logger.ErrorContext(ctx, "failed to decode response",
			append(logFields, "error", err)...)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.InfoContext(ctx, "search completed",
		append(logFields, "result_count", len(searchResp.Photos))...)

	return &searchResp, nil
}

// DownloadPhoto downloads a photo from Pexels.
func (c *PexelsClient) DownloadPhoto(ctx context.Context, photo *PexelsPhoto) ([]byte, error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsClient.DownloadPhoto",
		"photo_id", photo.ID,
	)

	// Use the large2x URL for high quality
	downloadURL := photo.Src.Large2x
	if downloadURL == "" {
		downloadURL = photo.Src.Large
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		duration := time.Since(startTime).Milliseconds()
		logger.ErrorContext(ctx, "download request failed",
			"error", err,
			"duration_ms", duration)
		return nil, fmt.Errorf("failed to download photo: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		logger.ErrorContext(ctx, "download failed with non-200 status",
			"status_code", resp.StatusCode,
			"duration_ms", duration)
		return nil, fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorContext(ctx, "failed to read response body",
			"error", err,
			"duration_ms", duration)
		return nil, fmt.Errorf("failed to read photo data: %w", err)
	}

	logger.InfoContext(ctx, "photo downloaded successfully",
		"duration_ms", duration,
		"size_bytes", len(data))

	return data, nil
}

// SearchVideos searches for videos on Pexels using the given query and orientation.
// orientation should be "portrait", "landscape", or "" (any).
func (c *PexelsClient) SearchVideos(ctx context.Context, query string, perPage int, orientation string) (*PexelsVideoSearchResponse, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	if perPage <= 0 {
		perPage = defaultSearchResults
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsClient.SearchVideos",
		"query", query,
		"per_page", perPage,
		"orientation", orientation,
	)

	// Try multiple search strategies with fallbacks
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
		resp, err := c.executeVideoSearch(ctx, searchQuery, perPage, orientation)
		if err != nil {
			lastErr = err
			if isRateLimited(err) {
				logger.WarnContext(ctx, "rate limited, skipping remaining video search fallbacks",
					"search_query", searchQuery)
				break
			}
			continue
		}
		if len(resp.Videos) > 0 {
			logger.InfoContext(ctx, "video search succeeded",
				"search_query", searchQuery,
				"result_count", len(resp.Videos))
			return resp, nil
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("pexels video search failed: %w", lastErr)
	}

	return &PexelsVideoSearchResponse{Videos: []PexelsVideo{}}, nil
}

// executeVideoSearch performs the actual HTTP request to Pexels Videos API.
func (c *PexelsClient) executeVideoSearch(ctx context.Context, query string, perPage int, orientation string) (*PexelsVideoSearchResponse, error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsClient.executeVideoSearch",
		"query", query,
	)

	requestURL := fmt.Sprintf("%s/search?query=%s&per_page=%d", pexelsVideoAPIURL, url.QueryEscape(query), perPage)
	if orientation != "" {
		requestURL += "&orientation=" + url.QueryEscape(orientation)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", c.apiKey)

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
	rateLimitReset := resp.Header.Get("X-Ratelimit-Reset")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.ErrorContext(ctx, "pexels video API returned error",
			"status_code", resp.StatusCode,
			"response_body", string(body),
			"duration_ms", duration,
			"rate_limit_remaining", rateLimitRemaining,
			"rate_limit_limit", rateLimitLimit,
			"rate_limit_reset", rateLimitReset)
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("pexels video API: %w", ErrRateLimited)
		}
		return nil, fmt.Errorf("pexels video API error: status %d", resp.StatusCode)
	}

	var searchResp PexelsVideoSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		logger.ErrorContext(ctx, "failed to decode video response",
			"error", err,
			"duration_ms", duration)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.InfoContext(ctx, "video search completed",
		"result_count", len(searchResp.Videos),
		"duration_ms", duration,
		"rate_limit_remaining", rateLimitRemaining,
		"rate_limit_limit", rateLimitLimit,
		"rate_limit_reset", rateLimitReset)

	return &searchResp, nil
}

// DownloadVideo downloads a video from the given direct URL.
func (c *PexelsClient) DownloadVideo(ctx context.Context, videoURL string) ([]byte, error) {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsClient.DownloadVideo",
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

// GetPhotoByID fetches a single Pexels photo by its Pexels-assigned id.
// Used by the candidate-import path so the Replace Media swap can
// download the exact image the client picked while preserving
// photographer attribution.
func (c *PexelsClient) GetPhotoByID(ctx context.Context, photoID string) (*PexelsPhoto, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsClient.GetPhotoByID",
		"photo_id", photoID,
	)
	requestURL := fmt.Sprintf("%s/photos/%s", pexelsAPIURL, url.PathEscape(photoID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels photo lookup failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.ErrorContext(ctx, "pexels photo lookup non-OK",
			"status_code", resp.StatusCode, "response_body", string(body))
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("pexels photo lookup: %w", ErrRateLimited)
		}
		return nil, fmt.Errorf("pexels photo lookup: status %d", resp.StatusCode)
	}
	var photo PexelsPhoto
	if err := json.NewDecoder(resp.Body).Decode(&photo); err != nil {
		return nil, fmt.Errorf("decode pexels photo: %w", err)
	}
	return &photo, nil
}

// GetVideoByID fetches a single Pexels video by its Pexels-assigned id.
// Mirrors GetPhotoByID for the video endpoint.
func (c *PexelsClient) GetVideoByID(ctx context.Context, videoID string) (*PexelsVideo, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsClient.GetVideoByID",
		"video_id", videoID,
	)
	requestURL := fmt.Sprintf("%s/videos/%s", pexelsVideoAPIURL, url.PathEscape(videoID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pexels video lookup failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.ErrorContext(ctx, "pexels video lookup non-OK",
			"status_code", resp.StatusCode, "response_body", string(body))
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("pexels video lookup: %w", ErrRateLimited)
		}
		return nil, fmt.Errorf("pexels video lookup: status %d", resp.StatusCode)
	}
	var video PexelsVideo
	if err := json.NewDecoder(resp.Body).Decode(&video); err != nil {
		return nil, fmt.Errorf("decode pexels video: %w", err)
	}
	return &video, nil
}

// pexelsClientInterface defines the methods needed from PexelsClient.
// This allows for testing with mock implementations.
type pexelsClientInterface interface {
	SearchPhotos(ctx context.Context, query string, perPage int) (*PexelsSearchResponse, error)
	DownloadPhoto(ctx context.Context, photo *PexelsPhoto) ([]byte, error)
	SearchVideos(ctx context.Context, query string, perPage int, orientation string) (*PexelsVideoSearchResponse, error)
	DownloadVideo(ctx context.Context, videoURL string) ([]byte, error)
	GetPhotoByID(ctx context.Context, photoID string) (*PexelsPhoto, error)
	GetVideoByID(ctx context.Context, videoID string) (*PexelsVideo, error)
}
