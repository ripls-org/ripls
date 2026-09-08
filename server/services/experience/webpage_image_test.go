package experience

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// plainHTTPClientFactory returns a plain (non-SSRF-guarded) http.Client
// for use in tests that target loopback httptest servers.
func plainHTTPClientFactory(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func TestService_downloadAndStoreWebpageImage(t *testing.T) {
	// A small, decodable JPEG. The upload path now sanitizes (decodes +
	// re-encodes) fetched images, so the fixture must be genuinely decodable.
	validJPEG := realTestJPEG()

	tests := []struct {
		name           string
		handler        http.HandlerFunc
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "successful image download",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(validJPEG)
			},
			wantErr: false,
		},
		{
			name: "non-OK status code",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr:        true,
			wantErrContain: "unexpected status code",
		},
		{
			name: "non-image content type",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte("<html></html>"))
			},
			wantErr:        true,
			wantErrContain: "not an image",
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:        true,
			wantErrContain: "unexpected status code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			// Create service with test storage using helper from common_test.go.
			// Inject a plain client so the loopback httptest server is reachable.
			svc, _, _ := setupTestService(t)
			svc.imageDownloadClient = plainHTTPClientFactory

			// Call the function
			mediaID, err := svc.downloadAndStoreWebpageImage(context.Background(), server.URL, "test-user-123")

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrContain)
				}
				if mediaID != "" {
					t.Errorf("expected empty media ID on error, got %q", mediaID)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if mediaID == "" {
					t.Error("expected non-empty media ID")
				}
			}
		})
	}
}

func TestService_downloadAndStoreWebpageImage_Timeout(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the context timeout (100ms)
		time.Sleep(500 * time.Millisecond) //nolint:forbidigo // fake HTTP server deliberately blocks to trigger a timeout
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte{0xff, 0xd8})
	}))
	defer server.Close()

	// Create service with test storage using helper from common_test.go.
	// Inject a plain client so the loopback httptest server is reachable.
	svc, _, _ := setupTestService(t)
	svc.imageDownloadClient = plainHTTPClientFactory

	// Use a context with timeout shorter than server delay
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Call the function - should timeout
	mediaID, err := svc.downloadAndStoreWebpageImage(ctx, server.URL, "test-user-123")

	if err == nil {
		t.Error("expected timeout error, got nil")
	}
	if mediaID != "" {
		t.Errorf("expected empty media ID on timeout, got %q", mediaID)
	}
}

// TestDownloadAndStoreWebpageImage_SSRFBlocked verifies that the SSRF guard
// rejects user-supplied og:image URLs pointing at non-public IP addresses.
func TestDownloadAndStoreWebpageImage_SSRFBlocked(t *testing.T) {
	tests := []struct {
		name           string
		imageURL       string
		wantErrContain string
	}{
		{
			name:           "loopback IP literal blocked by pre-check",
			imageURL:       "http://127.0.0.1:12345/image.jpg",
			wantErrContain: "not publicly routable",
		},
		{
			name:           "RFC1918 10/8 IP literal blocked by pre-check",
			imageURL:       "http://10.0.0.1/image.jpg",
			wantErrContain: "not publicly routable",
		},
		{
			name:           "link-local (cloud metadata) IP literal blocked by pre-check",
			imageURL:       "http://169.254.169.254/latest/meta-data/",
			wantErrContain: "not publicly routable",
		},
		{
			name:           "non-http scheme rejected before dial",
			imageURL:       "file:///etc/passwd",
			wantErrContain: "unsupported image URL scheme",
		},
		{
			name:           "gopher scheme rejected before dial",
			imageURL:       "gopher://example.com/image",
			wantErrContain: "unsupported image URL scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the default (SSRF-guarded) client — no imageDownloadClient override.
			// The pre-check must fire before any network request.
			svc, _, _ := setupTestService(t)

			mediaID, err := svc.downloadAndStoreWebpageImage(
				context.Background(), tt.imageURL, "test-user-123",
			)

			if err == nil {
				t.Fatalf("expected SSRF/scheme rejection for %q, got nil error", tt.imageURL)
			}
			if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrContain)
			}
			if mediaID != "" {
				t.Errorf("expected empty media ID when rejected, got %q", mediaID)
			}
		})
	}
}

// TestDownloadAndStoreWebpageImage_SSRFBlocked_DialerHook verifies that the
// SSRF dialer hook blocks connections to private IPs reached via hostname
// (e.g. "localhost"), which the IP-literal pre-check cannot catch.
func TestDownloadAndStoreWebpageImage_SSRFBlocked_DialerHook(t *testing.T) {
	svc, _, _ := setupTestService(t)
	// "localhost" is not a literal IP, so the pre-check passes. The
	// dialer resolves it to 127.0.0.1 and the Control hook rejects it.
	_, err := svc.downloadAndStoreWebpageImage(
		context.Background(), "http://localhost:12345/image.jpg", "test-user-123",
	)
	if err == nil {
		t.Fatal("expected SSRF dialer rejection for localhost, got nil")
	}
	if !strings.Contains(err.Error(), "routable") && !strings.Contains(err.Error(), "blocked") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}
