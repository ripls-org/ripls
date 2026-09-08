package ratelimit

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"golang.org/x/time/rate"

	"go.ripls.org/ripls/server/auth"
)

// TestUserIDBudget_EnforcesBurstThenRejects verifies user-id keyed limits
// allow burst-many requests from the same authenticated user and reject the
// next one.
func TestUserIDBudget_EnforcesBurstThenRejects(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	const burst = 3
	fn := buildInterceptorWithState(s, Rule{
		Procedure:   testProc,
		UserIDLimit: &Budget{Rate: rate.Every(time.Minute), Burst: burst},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	ctx := ctxWithUserID("user-abc")

	for i := 0; i < burst; i++ {
		if _, err := fn(ctx, req); err != nil {
			t.Fatalf("request %d should be allowed, got: %v", i+1, err)
		}
	}

	_, err := fn(ctx, req)
	if err == nil {
		t.Fatal("expected user-id rate-limit rejection, got nil")
	}
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("expected CodeResourceExhausted, got %v", connect.CodeOf(err))
	}
}

// TestUserIDBudget_IndependentOfIPAndEmail verifies that user-id buckets
// are tracked separately from IP and email buckets — a user hitting their
// per-user limit does not affect users sharing the same IP, and vice
// versa.
func TestUserIDBudget_IndependentOfIPAndEmail(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	const burst = 2
	fn := buildInterceptorWithState(s, Rule{
		Procedure:   testProc,
		IPLimit:     &Budget{Rate: rate.Every(time.Hour), Burst: 100}, // very loose
		UserIDLimit: &Budget{Rate: rate.Every(time.Hour), Burst: burst},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}

	ctxA := authn.SetInfo(ctxWithIP("1.2.3.4"), &auth.Info{UserID: "user-A"})
	ctxB := authn.SetInfo(ctxWithIP("1.2.3.4"), &auth.Info{UserID: "user-B"})

	// user-A burns through their burst.
	for i := 0; i < burst; i++ {
		if _, err := fn(ctxA, req); err != nil {
			t.Fatalf("user-A request %d should be allowed, got: %v", i+1, err)
		}
	}
	if _, err := fn(ctxA, req); err == nil {
		t.Fatal("expected user-A to be rate-limited")
	}

	// user-B should still be allowed from the same IP.
	if _, err := fn(ctxB, req); err != nil {
		t.Fatalf("user-B should not be affected by user-A's limit: %v", err)
	}
}

// TestUserIDBudget_NoAuthInfoPassesThrough verifies that requests with no
// auth context are not rate-limited by the user-id dimension. This is the
// deliberate fail-open behavior: missing auth wiring should not produce
// spurious 429s.
func TestUserIDBudget_NoAuthInfoPassesThrough(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure:   testProc,
		UserIDLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 0}, // would reject everything
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	ctx := context.Background() // no auth info

	for i := 0; i < 5; i++ {
		if _, err := fn(ctx, req); err != nil {
			t.Fatalf("request %d with no auth info should pass through, got: %v", i+1, err)
		}
	}
}

// TestUserIDWarnLog_LogsRawUserID verifies the user-id is logged raw
// (not masked) — the observability contract treats user_id as a standard
// non-PII identifier.
func TestUserIDWarnLog_LogsRawUserID(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure:   testProc,
		UserIDLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 0},
	})

	const userID = "user-xyz-789"
	var buf bytes.Buffer
	ctx := ctxWithUserIDAndLogger(userID, &buf)
	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}

	fn(ctx, req)

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for user-id rejection")
	}
	e := entries[0]
	if e["key_type"] != "user_id" {
		t.Errorf("expected key_type=user_id, got %v", e["key_type"])
	}
	if e["user_id"] != userID {
		t.Errorf("expected raw user_id=%q in warn log, got %v", userID, e["user_id"])
	}
}

// TestRejection_RetryAfterHeader verifies every rate-limit rejection sets
// the standard HTTP Retry-After header to a positive integer string via
// the Connect error's Meta() (which propagates as a response header).
func TestRejection_RetryAfterHeader(t *testing.T) {
	tests := []struct {
		name string
		rule Rule
		ctx  context.Context
	}{
		{
			name: "ip dimension",
			rule: Rule{Procedure: testProc, IPLimit: &Budget{Rate: rate.Every(time.Minute), Burst: 1}},
			ctx:  ctxWithIP("1.2.3.4"),
		},
		{
			name: "email dimension",
			rule: Rule{Procedure: testProc, EmailLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 1}},
			ctx:  context.Background(),
		},
		{
			name: "user-id dimension",
			rule: Rule{Procedure: testProc, UserIDLimit: &Budget{Rate: rate.Every(time.Minute), Burst: 1}},
			ctx:  ctxWithUserID("user-rt"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := newFakeClock(time.Now())
			s := makeState(clk)
			fn := buildInterceptorWithState(s, tc.rule)

			req := &fakeRequest{
				spec:   connect.Spec{Procedure: testProc},
				anyMsg: &msgWithEmail{email: "rt@example.com"},
			}

			// Burn the burst.
			if _, err := fn(tc.ctx, req); err != nil {
				t.Fatalf("first request should be allowed: %v", err)
			}

			// Second request rejects.
			_, err := fn(tc.ctx, req)
			if err == nil {
				t.Fatal("expected rejection")
			}
			var connectErr *connect.Error
			if !connectAs(err, &connectErr) {
				t.Fatalf("expected *connect.Error, got %T", err)
			}
			raw := connectErr.Meta().Get("Retry-After")
			if raw == "" {
				t.Fatal("expected Retry-After header on rejection")
			}
			secs, err := strconv.Atoi(raw)
			if err != nil {
				t.Fatalf("Retry-After must be an integer string, got %q: %v", raw, err)
			}
			if secs < 1 {
				t.Errorf("Retry-After must be >= 1, got %d", secs)
			}
		})
	}
}

// TestRejection_LogsEnforceMode verifies that an enforce-mode rejection
// emits the "rate_limited" warn log with the procedure FQN, key_type, and
// mode=enforce — the observable record log-based queries key on.
func TestRejection_LogsEnforceMode(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)
	fn := buildInterceptorWithState(s, Rule{
		Procedure:   testProc,
		UserIDLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 0},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	var buf bytes.Buffer
	ctx := ctxWithUserIDAndLogger("user-metrics", &buf)

	_, err := fn(ctx, req)
	if err == nil {
		t.Fatal("expected rejection")
	}

	entries := rateLimitedLogs(&buf)
	if len(entries) != 1 {
		t.Fatalf("expected exactly one rate_limited log, got %d", len(entries))
	}
	e := entries[0]
	if e["procedure"] != testProc {
		t.Errorf("procedure = %v, want %s", e["procedure"], testProc)
	}
	if e["key_type"] != "user_id" {
		t.Errorf("key_type = %v, want user_id", e["key_type"])
	}
	if e["mode"] != "enforce" {
		t.Errorf("mode = %v, want enforce", e["mode"])
	}
}

// TestStreamingHandler_UserIDBudget_RejectsAfterBurst verifies that the
// streaming-handler wrapper enforces the user-id budget at stream-open
// time. Stream open is where AI calls are kicked off, so guarding the
// open is sufficient.
func TestStreamingHandler_UserIDBudget_RejectsAfterBurst(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	const burst = 2
	const streamProc = "/test.Service/StreamMethod"
	fn := buildStreamingInterceptorWithState(s, Rule{
		Procedure:   streamProc,
		UserIDLimit: &Budget{Rate: rate.Every(time.Minute), Burst: burst},
	})

	conn := &fakeStreamingConn{spec: connect.Spec{Procedure: streamProc, StreamType: connect.StreamTypeServer}}
	ctx := ctxWithUserID("stream-user")

	for i := 0; i < burst; i++ {
		if err := fn(ctx, conn); err != nil {
			t.Fatalf("stream open %d should be allowed, got: %v", i+1, err)
		}
	}

	err := fn(ctx, conn)
	if err == nil {
		t.Fatal("expected stream open to be rate-limited after burst")
	}
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("expected CodeResourceExhausted, got %v", connect.CodeOf(err))
	}
}

// TestStreamingHandler_EmailDimensionSkipped verifies that the email
// dimension is a no-op on streams: there's no request body at stream-open
// time, so EmailLimit must not affect stream gating even if set.
func TestStreamingHandler_EmailDimensionSkipped(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	const streamProc = "/test.Service/StreamMethod"
	fn := buildStreamingInterceptorWithState(s, Rule{
		Procedure:  streamProc,
		EmailLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 0}, // would reject every unary call
	})

	conn := &fakeStreamingConn{spec: connect.Spec{Procedure: streamProc, StreamType: connect.StreamTypeServer}}

	// Make many opens; EmailLimit must not fire on streams.
	for i := 0; i < 5; i++ {
		if err := fn(context.Background(), conn); err != nil {
			t.Fatalf("stream open %d must not be rate-limited by EmailLimit, got: %v", i+1, err)
		}
	}
}

// TestStreamingHandler_PassThrough_NoMatchingRule verifies that procedures
// with no matching rule are not throttled on the streaming path.
func TestStreamingHandler_PassThrough_NoMatchingRule(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildStreamingInterceptorWithState(s, Rule{
		Procedure:   "/other.Service/Other",
		UserIDLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 0},
	})

	conn := &fakeStreamingConn{spec: connect.Spec{Procedure: "/test.Service/Unmatched", StreamType: connect.StreamTypeServer}}
	ctx := ctxWithUserID("any")
	for i := 0; i < 5; i++ {
		if err := fn(ctx, conn); err != nil {
			t.Fatalf("unmatched stream-open %d should pass through, got: %v", i+1, err)
		}
	}
}
