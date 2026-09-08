package ratelimit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/logging"
)

// fakeClock is a deterministic clock used in tests.
type fakeClock struct {
	mu      sync.Mutex
	current time.Time
	timers  []fakeTimer
}

type fakeTimer struct {
	at time.Time
	fn func()
}

func newFakeClock(t time.Time) *fakeClock {
	return &fakeClock{current: t}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current
}

func (f *fakeClock) AfterFunc(d time.Duration, fn func()) *time.Timer {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timers = append(f.timers, fakeTimer{at: f.current.Add(d), fn: fn})
	return nil
}

// advance moves the clock forward and fires any timers whose deadline has passed.
func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	f.current = f.current.Add(d)
	now := f.current
	var fire []func()
	var remaining []fakeTimer
	for _, t := range f.timers {
		if !t.at.After(now) {
			fire = append(fire, t.fn)
		} else {
			remaining = append(remaining, t)
		}
	}
	f.timers = remaining
	f.mu.Unlock()
	for _, fn := range fire {
		fn()
	}
}

// ---- minimal fake request types ----.

type fakeRequest struct {
	connect.AnyRequest
	spec   connect.Spec
	anyMsg any
}

func (r *fakeRequest) Spec() connect.Spec { return r.spec }
func (r *fakeRequest) Any() any           { return r.anyMsg }

type msgWithEmail struct{ email string }

func (m *msgWithEmail) GetEmail() string { return m.email }

type msgNoEmail struct{}

// fakeStreamingConn is a minimal connect.StreamingHandlerConn good enough
// to drive the streaming-handler interceptor at stream-open time. The
// interceptor only reads Spec(), so the Receive/Send/header bits are
// stubs.
type fakeStreamingConn struct {
	spec connect.Spec
}

func (c *fakeStreamingConn) Spec() connect.Spec           { return c.spec }
func (c *fakeStreamingConn) Peer() connect.Peer           { return connect.Peer{} }
func (c *fakeStreamingConn) Receive(_ any) error          { return nil }
func (c *fakeStreamingConn) RequestHeader() http.Header   { return http.Header{} }
func (c *fakeStreamingConn) Send(_ any) error             { return nil }
func (c *fakeStreamingConn) ResponseHeader() http.Header  { return http.Header{} }
func (c *fakeStreamingConn) ResponseTrailer() http.Header { return http.Header{} }

// ---- helpers ----.

func ctxWithIP(ip string) context.Context {
	return logging.WithRemoteAddr(context.Background(), ip)
}

func ctxWithIPAndLogger(ip string, buf *bytes.Buffer) context.Context {
	logger := logging.NewLogger(logging.Options{Format: "json", Output: buf})
	ctx := logging.WithLogger(context.Background(), logger)
	return logging.WithRemoteAddr(ctx, ip)
}

// ctxWithUserID injects auth info carrying the given user id so the
// user-id rate-limit dimension can read it via auth.GetAuthInfo.
func ctxWithUserID(userID string) context.Context {
	return authn.SetInfo(context.Background(), &auth.Info{UserID: userID})
}

// ctxWithUserIDAndLogger is the user-id equivalent of ctxWithIPAndLogger,
// used by tests that assert on the warn-log shape.
func ctxWithUserIDAndLogger(userID string, buf *bytes.Buffer) context.Context {
	logger := logging.NewLogger(logging.Options{Format: "json", Output: buf})
	ctx := logging.WithLogger(context.Background(), logger)
	return authn.SetInfo(ctx, &auth.Info{UserID: userID})
}

// rateLimitedLogs filters parseWarnLines output down to "rate_limited"
// entries. The warn log (procedure, key_type, mode) is the observable
// record of rejections — including MetricsOnly/dev-shadow modes where no
// error is returned.
func rateLimitedLogs(buf *bytes.Buffer) []map[string]any {
	var out []map[string]any
	for _, e := range parseWarnLines(buf) {
		if e["message"] == "rate_limited" {
			out = append(out, e)
		}
	}
	return out
}

func parseWarnLines(buf *bytes.Buffer) []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["severity"] == "WARNING" {
			out = append(out, entry)
		}
	}
	return out
}

func noopNext(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
	return nil, nil
}

const testProc = "/test.Service/TestMethod"

// makeState creates an interceptorState with the given fake clock for testing.
func makeState(clk clock) *interceptorState {
	return &interceptorState{clk: clk}
}

// buildInterceptorWithState wraps the production interceptor with an
// injected fake clock so tests can advance time deterministically.
// Returns the unary path bound to noopNext for the same call shape as
// before the streaming-handler split.
func buildInterceptorWithState(s *interceptorState, rules ...Rule) connect.UnaryFunc {
	byProc := make(map[string]*Rule, len(rules))
	for i := range rules {
		r := rules[i]
		byProc[r.Procedure] = &r
	}
	s.startJanitor()
	i := &interceptor{state: s, byProc: byProc}
	return i.WrapUnary(noopNext)
}

// buildStreamingInterceptorWithState is the streaming-handler equivalent
// of buildInterceptorWithState. Returns the streaming-handler path bound
// to a no-op stream sink so tests can drive stream-open behavior without
// a real connect.StreamingHandlerConn.
func buildStreamingInterceptorWithState(s *interceptorState, rules ...Rule) connect.StreamingHandlerFunc {
	byProc := make(map[string]*Rule, len(rules))
	for i := range rules {
		r := rules[i]
		byProc[r.Procedure] = &r
	}
	s.startJanitor()
	i := &interceptor{state: s, byProc: byProc}
	return i.WrapStreamingHandler(func(_ context.Context, _ connect.StreamingHandlerConn) error {
		return nil
	})
}

// connectAs is a small adapter around errors.As for use in tests; keeps
// the call site readable when chaining inside table-driven cases.
func connectAs(err error, target **connect.Error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*connect.Error); ok {
		*target = e
		return true
	}
	return false
}
