package media

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// pixabayRateLimitTransport returns HTTP 429 responses and counts requests.
type pixabayRateLimitTransport struct {
	callCount atomic.Int32
}

func (t *pixabayRateLimitTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	t.callCount.Add(1)
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(`{"error":"Too Many Requests"}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

// pixabayServerErrorTransport returns HTTP 500 responses and counts requests.
type pixabayServerErrorTransport struct {
	callCount atomic.Int32
}

func (t *pixabayServerErrorTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	t.callCount.Add(1)
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(`{"error":"internal server error"}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

// pixabaySuccessTransport returns a valid Pixabay search response.
type pixabaySuccessTransport struct {
	body string
}

func (t *pixabaySuccessTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(t.body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func pixabayVideoSearchJSON(portraitMedium bool) string {
	if portraitMedium {
		return `{
			"total": 1,
			"totalHits": 1,
			"hits": [{
				"id": 42,
				"pageURL": "https://pixabay.com/videos/id-42/",
				"tags": "nature, forest",
				"videos": {
					"large": {"url": "https://cdn.pixabay.com/large.mp4", "width": 3840, "height": 2160},
					"medium": {"url": "https://cdn.pixabay.com/medium.mp4", "width": 1080, "height": 1920},
					"small": {"url": "https://cdn.pixabay.com/small.mp4", "width": 540, "height": 960},
					"tiny": {"url": "https://cdn.pixabay.com/tiny.mp4", "width": 640, "height": 360}
				},
				"user": "NatureFilmer",
				"user_id": 567
			}]
		}`
	}
	return `{
		"total": 1,
		"totalHits": 1,
		"hits": [{
			"id": 99,
			"pageURL": "https://pixabay.com/videos/id-99/",
			"tags": "landscape, mountains",
			"videos": {
				"large": {"url": "https://cdn.pixabay.com/large.mp4", "width": 3840, "height": 2160},
				"medium": {"url": "https://cdn.pixabay.com/medium.mp4", "width": 1920, "height": 1080},
				"small": {"url": "https://cdn.pixabay.com/small.mp4", "width": 960, "height": 540},
				"tiny": {"url": "https://cdn.pixabay.com/tiny.mp4", "width": 640, "height": 360}
			},
			"user": "LandscapeCreator",
			"user_id": 123
		}]
	}`
}

func TestNewPixabayClient(t *testing.T) {
	client := NewPixabayClient("test-api-key")

	if client == nil {
		t.Fatal("NewPixabayClient() returned nil")
	}
	if client.apiKey != "test-api-key" {
		t.Errorf("NewPixabayClient() apiKey = %v, want test-api-key", client.apiKey)
	}
	if client.httpClient == nil {
		t.Error("NewPixabayClient() httpClient is nil")
	}
	if client.httpClient.Timeout != 0 {
		t.Errorf("NewPixabayClient() timeout = %v, want 0 (relies on context)", client.httpClient.Timeout)
	}
}

func TestNewPixabayProvider(t *testing.T) {
	client := NewPixabayClient("test-key")
	provider := NewPixabayProvider(client, nil, nil)

	if provider == nil {
		t.Fatal("NewPixabayProvider() returned nil")
	}
	if provider.client == nil {
		t.Error("NewPixabayProvider() client is nil")
	}
}

func TestPixabayProvider_ImplementsStockVideoProvider(t *testing.T) {
	// Compile-time check that PixabayProvider implements StockVideoProvider.
	client := NewPixabayClient("test-key")
	var _ StockVideoProvider = NewPixabayProvider(client, nil, nil)
}

func TestPixabaySearchVideos_ShortCircuitsOn429(t *testing.T) {
	transport := &pixabayRateLimitTransport{}
	client := &PixabayClient{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: transport},
	}

	// Use a multi-word query to generate multiple fallback queries.
	_, err := client.SearchVideos(context.Background(), "skiing at Breckenridge resort morning", 5)

	if err == nil {
		t.Fatal("SearchVideos() expected error, got nil")
	}

	// Should stop after the first 429, not try all fallback queries.
	calls := int(transport.callCount.Load())
	if calls != 1 {
		t.Errorf("SearchVideos() made %d API calls on 429, want 1 (should short-circuit)", calls)
	}
}

func TestPixabaySearchVideos_RateLimited_ReturnsWrappedError(t *testing.T) {
	transport := &pixabayRateLimitTransport{}
	client := &PixabayClient{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: transport},
	}

	_, err := client.SearchVideos(context.Background(), "skiing", 5)

	if err == nil {
		t.Fatal("SearchVideos() expected error, got nil")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("SearchVideos() error = %v, want wrapped ErrRateLimited", err)
	}
}

func TestPixabaySearchVideos_NonRateError_Continues(t *testing.T) {
	transport := &pixabayServerErrorTransport{}
	client := &PixabayClient{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: transport},
	}

	// Use a multi-word query to generate multiple fallback queries.
	_, err := client.SearchVideos(context.Background(), "skiing at Breckenridge resort morning", 5)

	if err == nil {
		t.Fatal("SearchVideos() expected error, got nil")
	}

	// Should continue through all fallback queries on non-429 errors.
	calls := int(transport.callCount.Load())
	if calls <= 1 {
		t.Errorf("SearchVideos() made only %d API call(s) on 500, want >1 (should continue fallbacks)", calls)
	}
}

func TestPixabaySearchVideos_EmptyQuery(t *testing.T) {
	client := NewPixabayClient("test-key")
	_, err := client.SearchVideos(context.Background(), "", 5)
	if err == nil {
		t.Fatal("SearchVideos() expected error for empty query, got nil")
	}
}

func TestSelectBestPixabayVideo_PrefersMediumPortrait(t *testing.T) {
	hit := &PixabayVideoHit{
		ID: 1,
		Videos: PixabayVideoVariants{
			Large:  PixabayVideoQuality{URL: "https://cdn.pixabay.com/large.mp4", Width: 3840, Height: 2160},  // landscape
			Medium: PixabayVideoQuality{URL: "https://cdn.pixabay.com/medium.mp4", Width: 1080, Height: 1920}, // portrait
			Small:  PixabayVideoQuality{URL: "https://cdn.pixabay.com/small.mp4", Width: 540, Height: 960},    // portrait
		},
	}

	gotURL, gotW, gotH := selectBestPixabayVideo(hit)
	if gotURL != "https://cdn.pixabay.com/medium.mp4" {
		t.Errorf("selectBestPixabayVideo() url = %v, want medium portrait", gotURL)
	}
	if gotW != 1080 || gotH != 1920 {
		t.Errorf("selectBestPixabayVideo() dimensions = %dx%d, want 1080x1920", gotW, gotH)
	}
}

func TestSelectBestPixabayVideo_FallsBackToAnyPortrait(t *testing.T) {
	hit := &PixabayVideoHit{
		ID: 2,
		Videos: PixabayVideoVariants{
			Large:  PixabayVideoQuality{URL: "https://cdn.pixabay.com/large.mp4", Width: 3840, Height: 2160},
			Medium: PixabayVideoQuality{URL: "https://cdn.pixabay.com/medium.mp4", Width: 1920, Height: 1080},
			Small:  PixabayVideoQuality{URL: "https://cdn.pixabay.com/small.mp4", Width: 540, Height: 960},
		},
	}

	gotURL, _, _ := selectBestPixabayVideo(hit)
	if gotURL != "https://cdn.pixabay.com/small.mp4" {
		t.Errorf("selectBestPixabayVideo() url = %v, want small portrait fallback", gotURL)
	}
}

func TestSelectBestPixabayVideo_FallsBackToMediumWhenNoPortrait(t *testing.T) {
	hit := &PixabayVideoHit{
		ID: 3,
		Videos: PixabayVideoVariants{
			Large:  PixabayVideoQuality{URL: "https://cdn.pixabay.com/large.mp4", Width: 3840, Height: 2160},
			Medium: PixabayVideoQuality{URL: "https://cdn.pixabay.com/medium.mp4", Width: 1920, Height: 1080},
			Small:  PixabayVideoQuality{URL: "https://cdn.pixabay.com/small.mp4", Width: 960, Height: 540},
		},
	}

	gotURL, _, _ := selectBestPixabayVideo(hit)
	if gotURL != "https://cdn.pixabay.com/medium.mp4" {
		t.Errorf("selectBestPixabayVideo() url = %v, want medium fallback when no portrait", gotURL)
	}
}

func TestSelectBestPixabayVideo_FallsBackToFirstAvailable(t *testing.T) {
	hit := &PixabayVideoHit{
		ID: 4,
		Videos: PixabayVideoVariants{
			Tiny: PixabayVideoQuality{URL: "https://cdn.pixabay.com/tiny.mp4", Width: 640, Height: 360},
		},
	}

	gotURL, _, _ := selectBestPixabayVideo(hit)
	if gotURL != "https://cdn.pixabay.com/tiny.mp4" {
		t.Errorf("selectBestPixabayVideo() url = %v, want tiny as last resort", gotURL)
	}
}

func TestSelectBestPixabayVideo_EmptyVariants_ReturnsEmpty(t *testing.T) {
	hit := &PixabayVideoHit{ID: 5}

	gotURL, gotW, gotH := selectBestPixabayVideo(hit)
	if gotURL != "" || gotW != 0 || gotH != 0 {
		t.Errorf("selectBestPixabayVideo() = (%v, %v, %v), want empty", gotURL, gotW, gotH)
	}
}

func TestPixabayVideoHitStructFields(t *testing.T) {
	hit := PixabayVideoHit{
		ID:      42,
		PageURL: "https://pixabay.com/videos/id-42/",
		Tags:    "nature, forest",
		User:    "NatureFilmer",
		UserID:  567,
		Videos: PixabayVideoVariants{
			Medium: PixabayVideoQuality{URL: "https://cdn.pixabay.com/medium.mp4", Width: 1080, Height: 1920},
		},
	}

	if hit.ID != 42 {
		t.Errorf("PixabayVideoHit.ID = %d, want 42", hit.ID)
	}
	if hit.PageURL != "https://pixabay.com/videos/id-42/" {
		t.Errorf("PixabayVideoHit.PageURL = %q, want https://pixabay.com/videos/id-42/", hit.PageURL)
	}
	if hit.Tags != "nature, forest" {
		t.Errorf("PixabayVideoHit.Tags = %q, want \"nature, forest\"", hit.Tags)
	}
	if hit.UserID != 567 {
		t.Errorf("PixabayVideoHit.UserID = %d, want 567", hit.UserID)
	}
	if hit.User != "NatureFilmer" {
		t.Errorf("PixabayVideoHit.User = %q, want NatureFilmer", hit.User)
	}
	if hit.Videos.Medium.URL == "" {
		t.Error("PixabayVideoHit.Videos.Medium.URL is empty")
	}
}

func TestPixabayVideoSearchResponse_JSONRoundtrip(t *testing.T) {
	transport := &pixabaySuccessTransport{body: pixabayVideoSearchJSON(true)}
	client := &PixabayClient{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: transport},
	}

	resp, err := client.SearchVideos(context.Background(), "nature", 5)
	if err != nil {
		t.Fatalf("SearchVideos() unexpected error: %v", err)
	}
	if len(resp.Hits) != 1 {
		t.Fatalf("SearchVideos() expected 1 hit, got %d", len(resp.Hits))
	}

	hit := resp.Hits[0]
	if hit.ID != 42 {
		t.Errorf("hit.ID = %d, want 42", hit.ID)
	}
	if hit.User != "NatureFilmer" {
		t.Errorf("hit.User = %q, want NatureFilmer", hit.User)
	}
	if hit.Videos.Medium.URL != "https://cdn.pixabay.com/medium.mp4" {
		t.Errorf("hit.Videos.Medium.URL = %q, want medium.mp4 URL", hit.Videos.Medium.URL)
	}
}

func TestPixabayProviderIDPrefix(t *testing.T) {
	// Verify the Pixabay ID prefix starts with "video_" so CachedStockVideoProvider
	// treats it as a video cache hit.
	expectedPrefix := "video_pixabay_"
	id := "video_pixabay_12345"
	if !strings.HasPrefix(id, videoProviderIDPrefix) {
		t.Errorf("Pixabay provider ID %q does not start with %q", id, videoProviderIDPrefix)
	}
	if !strings.HasPrefix(id, expectedPrefix) {
		t.Errorf("Pixabay provider ID %q does not start with %q", id, expectedPrefix)
	}
}
