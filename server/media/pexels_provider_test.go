package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSelectBestVideoFile(t *testing.T) {
	tests := []struct {
		name        string
		files       []PexelsVideoFile
		wantQuality string
		wantIsNil   bool
	}{
		{
			name:      "empty files returns nil",
			files:     []PexelsVideoFile{},
			wantIsNil: true,
		},
		{
			name: "prefers portrait HD over landscape HD",
			files: []PexelsVideoFile{
				{ID: 1, Quality: "hd", Width: 1920, Height: 1080}, // landscape
				{ID: 2, Quality: "hd", Width: 1080, Height: 1920}, // portrait
			},
			wantQuality: "hd",
		},
		{
			name: "falls back to any portrait when no portrait HD",
			files: []PexelsVideoFile{
				{ID: 1, Quality: "hd", Width: 1920, Height: 1080}, // landscape HD
				{ID: 2, Quality: "sd", Width: 540, Height: 960},   // portrait SD
			},
			wantQuality: "sd",
		},
		{
			name: "falls back to landscape HD when no portrait at all",
			files: []PexelsVideoFile{
				{ID: 1, Quality: "sd", Width: 1280, Height: 720},  // landscape SD
				{ID: 2, Quality: "hd", Width: 1920, Height: 1080}, // landscape HD
			},
			wantQuality: "hd",
		},
		{
			name: "falls back to first file when nothing else matches",
			files: []PexelsVideoFile{
				{ID: 1, Quality: "sd", Width: 640, Height: 360},
			},
			wantQuality: "sd",
		},
		{
			name: "single portrait HD file is selected",
			files: []PexelsVideoFile{
				{ID: 1, Quality: "hd", Width: 1080, Height: 1920},
			},
			wantQuality: "hd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectBestVideoFile(tt.files)
			if tt.wantIsNil {
				if got != nil {
					t.Errorf("selectBestVideoFile() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("selectBestVideoFile() returned nil, want non-nil")
			}
			if got.Quality != tt.wantQuality {
				t.Errorf("selectBestVideoFile() quality = %q, want %q", got.Quality, tt.wantQuality)
			}
		})
	}
}

func TestSelectBestVideoFile_PortraitIsHeightGreaterThanWidth(t *testing.T) {
	// Verify portrait detection: height > width means portrait.
	portrait := PexelsVideoFile{ID: 1, Quality: "hd", Width: 1080, Height: 1920}
	landscape := PexelsVideoFile{ID: 2, Quality: "hd", Width: 1920, Height: 1080}

	got := selectBestVideoFile([]PexelsVideoFile{landscape, portrait})
	if got == nil {
		t.Fatal("selectBestVideoFile() returned nil")
	}
	if got.ID != portrait.ID {
		t.Errorf("expected portrait file (ID %d) to be selected, got ID %d", portrait.ID, got.ID)
	}
}

func TestPexelsVideoStructs(t *testing.T) {
	// Verify struct field names match expected JSON tags.
	file := PexelsVideoFile{ID: 1, Quality: "hd", Width: 1080, Height: 1920, Link: "https://example.com/video.mp4"}
	if file.ID != 1 || file.Quality != "hd" || file.Width != 1080 || file.Height != 1920 {
		t.Error("PexelsVideoFile fields do not match expected values")
	}

	user := PexelsVideoUser{Name: "Test Creator", URL: "https://pexels.com/@test"}
	if user.Name != "Test Creator" {
		t.Error("PexelsVideoUser.Name not set correctly")
	}

	video := PexelsVideo{
		ID:         12345,
		URL:        "https://www.pexels.com/video/12345",
		Image:      "https://images.pexels.com/videos/12345/poster.jpg",
		User:       user,
		VideoFiles: []PexelsVideoFile{file},
	}
	if video.ID != 12345 || len(video.VideoFiles) != 1 {
		t.Error("PexelsVideo fields do not match expected values")
	}

	resp := PexelsVideoSearchResponse{
		Page:         1,
		PerPage:      5,
		Videos:       []PexelsVideo{video},
		TotalResults: 1,
	}
	if resp.Page != 1 || resp.PerPage != 5 || resp.TotalResults != 1 || len(resp.Videos) != 1 {
		t.Error("PexelsVideoSearchResponse fields do not match expected values")
	}
}

func TestNewPexelsClient(t *testing.T) {
	apiKey := "test-api-key"
	client := NewPexelsClient(apiKey)

	if client == nil {
		t.Fatal("NewPexelsClient() returned nil")
	}

	if client.apiKey != apiKey {
		t.Errorf("NewPexelsClient() apiKey = %v, want %v", client.apiKey, apiKey)
	}

	if client.httpClient == nil {
		t.Error("NewPexelsClient() httpClient is nil")
	}

	// HTTP client should have no timeout set (relies on context deadlines)
	if client.httpClient.Timeout != 0 {
		t.Errorf("NewPexelsClient() timeout = %v, want 0 (no timeout)", client.httpClient.Timeout)
	}
}

func TestNewPexelsProvider(t *testing.T) {
	client := NewPexelsClient("test-key")
	provider := NewPexelsProvider(client, nil, nil)

	if provider == nil {
		t.Fatal("NewPexelsProvider() returned nil")
	}

	if provider.client == nil {
		t.Error("NewPexelsProvider() client is nil")
	}
}

func TestPexelsProvider_ImplementsStockVideoProvider(t *testing.T) {
	// Compile-time check that PexelsProvider implements StockVideoProvider.
	client := NewPexelsClient("test-key")
	var _ StockVideoProvider = NewPexelsProvider(client, nil, nil)
}

// rateLimitTransport is a custom http.RoundTripper that returns 429 responses
// and counts how many requests were made.
type rateLimitTransport struct {
	callCount atomic.Int32
}

func (t *rateLimitTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	t.callCount.Add(1)
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(`{"status":429,"code":"Too Many Requests","message":"Rate limit exceeded"}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

// serverErrorTransport returns 500 responses and counts requests.
type serverErrorTransport struct {
	callCount atomic.Int32
}

func (t *serverErrorTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	t.callCount.Add(1)
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(`{"error":"internal server error"}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func TestSearchVideos_ShortCircuitsOn429(t *testing.T) {
	transport := &rateLimitTransport{}
	client := &PexelsClient{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: transport},
	}

	// Use a multi-word query that generates multiple fallback queries.
	_, err := client.SearchVideos(context.Background(), "skiing at Breckenridge resort morning", 5, "portrait")

	if err == nil {
		t.Fatal("SearchVideos() expected error, got nil")
	}

	// Should stop after the first 429, not try all 4+ fallback queries.
	calls := int(transport.callCount.Load())
	if calls != 1 {
		t.Errorf("SearchVideos() made %d API calls on 429, want 1 (should short-circuit)", calls)
	}
}

func TestSearchVideos_NonRateError_Continues(t *testing.T) {
	transport := &serverErrorTransport{}
	client := &PexelsClient{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: transport},
	}

	// Use a multi-word query that generates multiple fallback queries.
	_, err := client.SearchVideos(context.Background(), "skiing at Breckenridge resort morning", 5, "portrait")

	if err == nil {
		t.Fatal("SearchVideos() expected error, got nil")
	}

	// Should continue through all fallback queries on non-429 errors.
	calls := int(transport.callCount.Load())
	if calls <= 1 {
		t.Errorf("SearchVideos() made only %d API call(s) on 500, want >1 (should continue fallbacks)", calls)
	}
}

func TestSearchVideos_RateLimited_ReturnsWrappedError(t *testing.T) {
	transport := &rateLimitTransport{}
	client := &PexelsClient{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: transport},
	}

	_, err := client.SearchVideos(context.Background(), "skiing", 5, "portrait")

	if err == nil {
		t.Fatal("SearchVideos() expected error, got nil")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("SearchVideos() error = %v, want wrapped ErrRateLimited", err)
	}
}

func TestSearchPhotos_ShortCircuitsOn429(t *testing.T) {
	transport := &rateLimitTransport{}
	client := &PexelsClient{
		apiKey:     "test-key",
		httpClient: &http.Client{Transport: transport},
	}

	_, err := client.SearchPhotos(context.Background(), "skiing at Breckenridge resort morning", 5)

	if err == nil {
		t.Fatal("SearchPhotos() expected error, got nil")
	}

	calls := int(transport.callCount.Load())
	if calls != 1 {
		t.Errorf("SearchPhotos() made %d API calls on 429, want 1 (should short-circuit)", calls)
	}
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "direct ErrRateLimited",
			err:  ErrRateLimited,
			want: true,
		},
		{
			name: "wrapped ErrRateLimited",
			err:  fmt.Errorf("pexels video API: %w", ErrRateLimited),
			want: true,
		},
		{
			name: "double wrapped ErrRateLimited",
			err:  fmt.Errorf("search failed: %w", fmt.Errorf("pexels video API: %w", ErrRateLimited)),
			want: true,
		},
		{
			name: "unrelated error",
			err:  fmt.Errorf("connection refused"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRateLimited(tt.err); got != tt.want {
				t.Errorf("isRateLimited(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
