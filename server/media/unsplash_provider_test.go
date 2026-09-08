package media

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

// unsplashTransport rewrites requests destined for unsplashAPIURL onto
// a test server's host. Used by GetPhoto tests to exercise the real
// HTTP layer without depending on the Unsplash API.
type unsplashTransport struct {
	target string // base URL of the test server, e.g. "http://127.0.0.1:1234"
}

func (t *unsplashTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(t.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newTestUnsplashClient returns an UnsplashClient whose HTTP requests
// are redirected onto srv. Mirrors the approach used in other server-
// package tests that need to exercise the real http.Client layer.
func newTestUnsplashClient(srv *httptest.Server) *UnsplashClient {
	return &UnsplashClient{
		accessKey: "test-access-key",
		httpClient: &http.Client{
			Transport: &unsplashTransport{target: srv.URL},
		},
	}
}

func TestUnsplashClient_GetPhoto(t *testing.T) {
	t.Run("happy path decodes the response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/photos/abc123" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Client-ID test-access-key" {
				t.Errorf("missing/wrong Authorization header: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"id": "abc123",
				"description": "Mountain at dawn",
				"alt_description": "snowy peak",
				"urls": {
					"raw": "https://images.unsplash.com/raw",
					"full": "https://images.unsplash.com/full",
					"regular": "https://images.unsplash.com/regular",
					"small": "https://images.unsplash.com/small",
					"thumb": "https://images.unsplash.com/thumb"
				},
				"user": {"name": "Alice Photographer", "username": "alice"}
			}`)
		}))
		defer srv.Close()

		c := newTestUnsplashClient(srv)
		photo, err := c.GetPhoto(context.Background(), "abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if photo.ID != "abc123" {
			t.Errorf("photo.ID = %q, want abc123", photo.ID)
		}
		if photo.URLs.Full != "https://images.unsplash.com/full" {
			t.Errorf("photo.URLs.Full = %q", photo.URLs.Full)
		}
		if photo.User.Name != "Alice Photographer" {
			t.Errorf("photo.User.Name = %q", photo.User.Name)
		}
	})

	t.Run("404 returns non-nil error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"errors":["Couldn't find Photo"]}`, http.StatusNotFound)
		}))
		defer srv.Close()

		c := newTestUnsplashClient(srv)
		_, err := c.GetPhoto(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected error for 404, got nil")
		}
		if errors.Is(err, ErrRateLimited) {
			t.Errorf("404 should not be classified as rate-limited: %v", err)
		}
	})

	t.Run("429 wraps ErrRateLimited", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
		}))
		defer srv.Close()

		c := newTestUnsplashClient(srv)
		_, err := c.GetPhoto(context.Background(), "abc123")
		if err == nil {
			t.Fatal("expected error for 429, got nil")
		}
		if !errors.Is(err, ErrRateLimited) {
			t.Errorf("429 should wrap ErrRateLimited, got: %v", err)
		}
	})

	t.Run("escapes special characters in photo id", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The path is the un-escaped form Go's mux delivers.
			if r.URL.Path != "/photos/has spaces" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"x"}`)
		}))
		defer srv.Close()

		c := newTestUnsplashClient(srv)
		if _, err := c.GetPhoto(context.Background(), "has spaces"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestExtractSearchKeywords(t *testing.T) {
	tests := []struct {
		name        string
		description string
		expected    []string
	}{
		{
			name:        "simple request",
			description: "Looking for a power drill",
			expected:    []string{"power", "drill"},
		},
		{
			name:        "complex request with filler words",
			description: "I am looking for some camping gear for a weekend trip",
			expected:    []string{"camping", "gear", "weekend", "trip"},
		},
		{
			name:        "request with punctuation",
			description: "Need a bike, helmet, and locks!",
			expected:    []string{"bike", "helmet", "locks"},
		},
		{
			name:        "very short description",
			description: "tent",
			expected:    []string{"tent"},
		},
		{
			name:        "all filler words",
			description: "I am looking for the",
			expected:    []string{},
		},
		{
			name:        "mixed case",
			description: "Looking For CAMPING Equipment",
			expected:    []string{"camping", "equipment"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSearchKeywords(tt.description)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ExtractSearchKeywords(%q) = %v, expected %v", tt.description, result, tt.expected)
			}
		})
	}
}
