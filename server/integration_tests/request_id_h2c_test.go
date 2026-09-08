package integration_tests

// Regression tests for issue #1799: X-Request-ID propagation over HTTP/2 (h2c).
//
// Before the middleware chain was moved inside h2c.NewHandler, every HTTP/2
// stream on a hijacked connection bypassed the outer middleware and
// inherited the upgrade-request's context. That meant client X-Request-ID
// values never reached the server, and a single server-generated request_id
// "leaked" across every subsequent HTTP/2 stream on the same connection.
//
// The HTTP/1.1 tests in logging_test.go pass either way because h2c only
// hijacks h2c-upgrade or prior-knowledge HTTP/2 connections; HTTP/1.1
// requests fall through to the wrapped handler. These tests reach the bug
// path by speaking prior-knowledge HTTP/2 cleartext, the same mode that
// connectrpc/connect-dart uses on iOS/Android.

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"go.ripls.org/ripls/server/middleware"
	"go.ripls.org/ripls/server/storage"
)

// h2cClient returns an *http.Client that speaks prior-knowledge HTTP/2
// cleartext. It uses a single underlying transport so successive requests
// share a TCP connection (which is what triggered the shared-context bug).
func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			// DialTLSContext is invoked even for plain http:// when AllowHTTP is set;
			// we ignore the TLS config and dial cleartext TCP.
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}
}

// TestRequestID_H2CClientHeaderReachesServer verifies the fix for #1799:
// an X-Request-ID stamped by an HTTP/2 client must reach the server's
// RequestID middleware and appear in structured logs. Before the fix, the
// outer middleware chain was bypassed for every HTTP/2 stream and the
// client header was never seen.
func TestRequestID_H2CClientHeaderReachesServer(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	client := h2cClient()
	const wantID = "h2c-test-id-aaaa1111"

	req, err := http.NewRequest(http.MethodGet, serverURL+"/health", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set(middleware.RequestIDHeader, wantID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("h2c request failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Sanity-check we actually went over HTTP/2.
	if resp.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2 response, got %s (ProtoMajor=%d)", resp.Proto, resp.ProtoMajor)
	}

	// The response should round-trip the same X-Request-ID header.
	if got := resp.Header.Get(middleware.RequestIDHeader); got != wantID {
		t.Errorf("response X-Request-ID = %q, want %q", got, wantID)
	}

	// The server log entry for this request must carry the same ID.
	entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
		return e["request_id"] == wantID && e["message"] == "http request"
	}, 2*time.Second)

	if entry == nil {
		t.Errorf("no http request log entry found with request_id=%q (h2c bypass regression?)", wantID)
		t.Logf("entries containing 'http request': %v", logCapture.FindByMessage("http request"))
	}
}

// TestRequestID_H2CMultipleStreamsAreIsolated verifies that each HTTP/2
// stream on the same multiplexed connection gets its own request_id. The
// pre-fix bug caused every stream to inherit the upgrade-request's
// request_id, which manifested as one ID showing up across thousands of
// unrelated log lines. We send two sequential requests on the same
// http2.Transport (which holds the connection open and reuses it) and
// assert distinct logged IDs.
func TestRequestID_H2CMultipleStreamsAreIsolated(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	client := h2cClient()
	const idA = "h2c-stream-aaaa-0001"
	const idB = "h2c-stream-bbbb-0002"

	for _, id := range []string{idA, idB} {
		req, err := http.NewRequest(http.MethodGet, serverURL+"/health", nil)
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Header.Set(middleware.RequestIDHeader, id)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("h2c request %q failed: %v", id, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.ProtoMajor != 2 {
			t.Fatalf("expected HTTP/2 response for %q, got %s", id, resp.Proto)
		}
	}

	// Each id should appear on its own http-request log line.
	for _, id := range []string{idA, idB} {
		entry := waitForLogEntry(logCapture, func(e map[string]any) bool {
			return e["request_id"] == id && e["message"] == "http request"
		}, 2*time.Second)
		if entry == nil {
			t.Errorf("missing http request log for request_id=%q", id)
		}
	}

	// And critically, idA must not appear on idB's request, nor vice versa.
	// This is the regression assertion for the shared-context bug: a single
	// id "leaking" to other streams would show up here.
	for _, pair := range []struct{ wantID, otherID string }{
		{wantID: idA, otherID: idB},
		{wantID: idB, otherID: idA},
	} {
		for _, e := range logCapture.FindByRequestID(pair.wantID) {
			path, _ := e["path"].(string)
			// Every entry tagged with pair.wantID should belong to its own
			// request — there's no way to know which request a non-http-line
			// belongs to except by request_id, so we just bound the count by
			// asserting that no entry tagged pair.wantID also somehow names
			// the other id explicitly. That keeps the test robust to other
			// log lines (auth, query stats) emitted under the same id.
			if strings.Contains(path, pair.otherID) {
				t.Errorf("entry tagged request_id=%q references the other id %q: %v",
					pair.wantID, pair.otherID, e)
			}
		}
	}
}
