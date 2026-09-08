package ratelimit

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/time/rate"
)

// TestIPBudget_EnforcesBurstThenRejects verifies that burst requests are allowed
// and subsequent ones are rejected until the limiter refills.
func TestIPBudget_EnforcesBurstThenRejects(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	const burst = 3
	fn := buildInterceptorWithState(s, Rule{
		Procedure: testProc,
		IPLimit:   &Budget{Rate: rate.Every(time.Minute), Burst: burst},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	ctx := ctxWithIP("1.2.3.4")

	// First `burst` requests should succeed.
	for i := 0; i < burst; i++ {
		if _, err := fn(ctx, req); err != nil {
			t.Fatalf("request %d should be allowed, got: %v", i+1, err)
		}
	}

	// Next request should be rejected.
	_, err := fn(ctx, req)
	if err == nil {
		t.Fatal("expected rate-limit rejection, got nil")
	}
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("expected CodeResourceExhausted, got %v", connect.CodeOf(err))
	}
}

// TestEmailBudget_IndependentOfIPKey verifies that email and IP buckets are tracked separately.
func TestEmailBudget_IndependentOfIPKey(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	const burst = 2
	fn := buildInterceptorWithState(s, Rule{
		Procedure:  testProc,
		IPLimit:    &Budget{Rate: rate.Every(time.Minute), Burst: 100}, // very loose IP limit
		EmailLimit: &Budget{Rate: rate.Every(time.Hour), Burst: burst},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgWithEmail{email: "test@example.com"}}
	ctx := ctxWithIP("1.2.3.4")

	for i := 0; i < burst; i++ {
		if _, err := fn(ctx, req); err != nil {
			t.Fatalf("email request %d should be allowed, got: %v", i+1, err)
		}
	}

	_, err := fn(ctx, req)
	if err == nil {
		t.Fatal("expected email rate-limit rejection")
	}
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("expected CodeResourceExhausted, got %v", connect.CodeOf(err))
	}
}

// TestCombinedIPAndEmail_EitherKeyTriggersRejection verifies that hitting either limit rejects.
func TestCombinedIPAndEmail_EitherKeyTriggersRejection(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure:  testProc,
		IPLimit:    &Budget{Rate: rate.Every(time.Minute), Burst: 1},
		EmailLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 100},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgWithEmail{email: "user@example.com"}}
	ctx := ctxWithIP("5.6.7.8")

	// First request uses up the IP burst of 1.
	if _, err := fn(ctx, req); err != nil {
		t.Fatalf("first request should succeed: %v", err)
	}

	// Second request should be rejected due to IP limit (email still has room).
	_, err := fn(ctx, req)
	if err == nil {
		t.Fatal("expected rejection from IP limit")
	}
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("expected CodeResourceExhausted, got %v", connect.CodeOf(err))
	}
}

// TestPassThrough_NoMatchingRule verifies procedures with no rule are not throttled.
func TestPassThrough_NoMatchingRule(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure: "/other.Service/OtherMethod",
		IPLimit:   &Budget{Rate: rate.Every(time.Minute), Burst: 0}, // burst 0 would always reject
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	ctx := ctxWithIP("1.2.3.4")

	for i := 0; i < 10; i++ {
		if _, err := fn(ctx, req); err != nil {
			t.Fatalf("request %d to unmatched procedure should pass through: %v", i+1, err)
		}
	}
}

// TestEmptyIP_PassThrough verifies that an empty IP does not trigger rejection.
func TestEmptyIP_PassThrough(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure: testProc,
		IPLimit:   &Budget{Rate: rate.Every(time.Minute), Burst: 0},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	ctx := context.Background() // no IP in context

	if _, err := fn(ctx, req); err != nil {
		t.Fatalf("empty IP should pass through, got: %v", err)
	}
}

// TestEmptyEmail_PassThrough verifies that an empty email does not trigger rejection.
func TestEmptyEmail_PassThrough(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure:  testProc,
		EmailLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 0},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgWithEmail{email: ""}}
	ctx := context.Background()

	if _, err := fn(ctx, req); err != nil {
		t.Fatalf("empty email should pass through, got: %v", err)
	}
}

// TestJanitor_EvictsIdleLimiters verifies that the janitor removes entries after
// the fake clock advances past the eviction threshold.
func TestJanitor_EvictsIdleLimiters(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure: testProc,
		IPLimit:   &Budget{Rate: rate.Every(time.Minute), Burst: 1},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	ctx := ctxWithIP("9.9.9.9")

	// Use up the burst so the entry exists.
	if _, err := fn(ctx, req); err != nil {
		t.Fatalf("first request should succeed: %v", err)
	}

	// Verify the entry is present.
	key := testProc + "\x00" + "9.9.9.9"
	if _, ok := s.ipLimiters.Load(key); !ok {
		t.Fatal("expected limiter entry to exist before eviction")
	}

	// Advance past the eviction threshold and fire the janitor timer.
	clk.advance(6 * time.Minute)

	// Verify the entry was evicted.
	if _, ok := s.ipLimiters.Load(key); ok {
		t.Fatal("expected limiter entry to be evicted after janitor ran")
	}
}

// TestWarnLog_Shape verifies the warn log line structure on rate-limit rejection.
func TestWarnLog_Shape(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure: testProc,
		IPLimit:   &Budget{Rate: rate.Every(time.Minute), Burst: 0},
	})

	var buf bytes.Buffer
	ctx := ctxWithIPAndLogger("10.0.0.1", &buf)
	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}

	fn(ctx, req) // we only care about the log

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry")
	}
	e := entries[0]
	if e["procedure"] == "" {
		t.Errorf("expected non-empty 'procedure' field in warn log")
	}
	if e["key_type"] != "ip" {
		t.Errorf("expected key_type=ip, got %v", e["key_type"])
	}
	if e["remote_addr"] == "" {
		t.Errorf("expected non-empty 'remote_addr' field in warn log")
	}
}

// TestEmailWarnLog_MasksEmail verifies that the email in the warn log is masked.
func TestEmailWarnLog_MasksEmail(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	fn := buildInterceptorWithState(s, Rule{
		Procedure:  testProc,
		EmailLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 0},
	})

	const plainEmail = "real@example.com"
	var buf bytes.Buffer
	ctx := ctxWithIPAndLogger("", &buf)
	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgWithEmail{email: plainEmail}}

	fn(ctx, req)

	entries := parseWarnLines(&buf)
	if len(entries) == 0 {
		t.Fatal("expected at least one warn log entry for email rejection")
	}
	for _, e := range entries {
		for _, v := range e {
			if s, ok := v.(string); ok && strings.Contains(s, plainEmail) {
				t.Errorf("plain email found in warn log — must be masked: %v", e)
			}
		}
	}
}

// TestConcurrent_NoRace verifies the interceptor is safe under concurrent access.
func TestConcurrent_NoRace(t *testing.T) {
	interceptFn := Interceptor(Rule{
		Procedure: testProc,
		IPLimit:   &Budget{Rate: rate.Every(time.Second), Burst: 10},
	})
	fn := interceptFn.WrapUnary(noopNext)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := &fakeRequest{
				spec:   connect.Spec{Procedure: testProc},
				anyMsg: &msgNoEmail{},
			}
			ctx := ctxWithIP("1.2.3.4")
			fn(ctx, req)
		}(i)
	}
	wg.Wait()
}
