package media

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/safehttp"
	"go.ripls.org/ripls/server/services"
)

func TestService_AddMediaFromURL(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("rejects http (non-https) URL", func(t *testing.T) {
		ctx := createAuthenticatedContext("user1", "u1@example.com", models.Role_ROLE_USER)
		_, err := service.AddMediaFromURL(ctx, connect.NewRequest(&api.AddMediaFromURLRequest{
			Url: "http://example.com/x.jpg",
		}))
		if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("rejects empty URL", func(t *testing.T) {
		ctx := createAuthenticatedContext("user1", "u1@example.com", models.Role_ROLE_USER)
		_, err := service.AddMediaFromURL(ctx, connect.NewRequest(&api.AddMediaFromURLRequest{Url: ""}))
		if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("rejects non-image / non-video content type", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		}))
		defer srv.Close()

		// Use the test server's own client so the loopback TLS cert is
		// trusted; override Service.outboundClient so the fetch bypasses
		// the production SSRF dialer (which would block loopback).
		service.outboundClient = func(_ time.Duration) *http.Client {
			return srv.Client()
		}
		t.Cleanup(func() {
			service.outboundClient = func(t time.Duration) *http.Client {
				return safehttp.NewClient(safehttp.WithTimeout(t), safehttp.WithRedirectSchemes("https"))
			}
		})

		ctx := createAuthenticatedContext("user1", "u1@example.com", models.Role_ROLE_USER)
		_, err := service.AddMediaFromURL(ctx, connect.NewRequest(&api.AddMediaFromURLRequest{
			Url: srv.URL + "/x",
		}))
		if err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("expected InvalidArgument for text/html, got %v", err)
		}
		if !strings.Contains(err.Error(), "content type") {
			t.Errorf("expected content-type message, got %q", err.Error())
		}
	})

	t.Run("accepts image content type and stores media", func(t *testing.T) {
		// A real, decodable JPEG: StoreMedia now sanitizes (decodes + re-encodes)
		// the fetched image, so a bare SOI/EOI stub would fail to decode.
		body := makeTestJPEG()
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(body)
		}))
		defer srv.Close()

		service.outboundClient = func(_ time.Duration) *http.Client {
			return srv.Client()
		}
		t.Cleanup(func() {
			service.outboundClient = func(t time.Duration) *http.Client {
				return safehttp.NewClient(safehttp.WithTimeout(t), safehttp.WithRedirectSchemes("https"))
			}
		})

		ctx := createAuthenticatedContext("user1", "u1@example.com", models.Role_ROLE_USER)
		resp, err := service.AddMediaFromURL(ctx, connect.NewRequest(&api.AddMediaFromURLRequest{
			Url: srv.URL + "/photo.jpg",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Msg.Id == "" {
			t.Error("expected non-empty media id")
		}
	})

	t.Run("ssrf guard blocks loopback URL through default client", func(t *testing.T) {
		// Default client is the safe one; loopback should be rejected at
		// dial time with InvalidArgument (mapped from classifyDialErr).
		// We bind to a free loopback port — the dial fails before TLS
		// negotiation, so the URL can be a phantom.
		ctx := createAuthenticatedContext("user1", "u1@example.com", models.Role_ROLE_USER)
		_, err := service.AddMediaFromURL(ctx, connect.NewRequest(&api.AddMediaFromURLRequest{
			Url: "https://127.0.0.1:1/x.jpg",
		}))
		if err == nil {
			t.Fatal("expected SSRF guard to reject loopback URL")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got code %v err=%v", connect.CodeOf(err), err)
		}
	})

	t.Run("ssrf guard blocks GCP metadata link-local IP", func(t *testing.T) {
		ctx := createAuthenticatedContext("user1", "u1@example.com", models.Role_ROLE_USER)
		_, err := service.AddMediaFromURL(ctx, connect.NewRequest(&api.AddMediaFromURLRequest{
			Url: "https://169.254.169.254/computeMetadata/v1/instance/",
		}))
		if err == nil {
			t.Fatal("expected SSRF guard to reject GCP metadata IP")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got code %v err=%v", connect.CodeOf(err), err)
		}
	})
}
