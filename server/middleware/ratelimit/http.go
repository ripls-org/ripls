package ratelimit

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"go.ripls.org/ripls/server/logging"
)

// HTTPBudget mirrors Budget for the raw-HTTP middleware path. The
// Connect interceptor in this package targets RPC procedures; HTTP
// endpoints (e.g. /go/{code}/decline) need a sibling that wraps an
// http.Handler instead.
type HTTPBudget struct {
	// Rate is the sustained allowed events per second.
	Rate rate.Limit
	// Burst is the maximum burst size.
	Burst int
}

// httpLimiterState holds the per-IP token buckets and janitor
// bookkeeping for a single PerIPHTTP middleware instance.
type httpLimiterState struct {
	limiters    sync.Map // key=IP, val=*entry
	janitorOnce sync.Once
	clk         clock
	budget      HTTPBudget
}

// PerIPHTTP returns an http middleware that enforces a per-IP token-
// bucket limit on the wrapped handler. Designed for anonymous HTTP
// endpoints where the client IP (resolved by middleware.RemoteAddr
// earlier in the chain) is the only identity dimension.
//
// On a 429 the response carries `Retry-After: <seconds>` (estimated
// from the bucket's recovery time toward burst capacity) and a plain-
// text body. Logging is structured at Warn level — the refusal is a
// handled user-error case, not a system failure, so it stays out of
// the auto-filed-issue alert pipeline.
//
// `operation` is the structured-log operation tag included with each
// refusal log line, e.g. `"RecordWebDecline"`. The middleware doesn't
// reach into the wrapped handler's own logging; it logs only on
// rate-limit refusals.
//
// Empty IP — unusual outside tests, as middleware.RemoteAddr always
// resolves to some value — is treated as a single shared bucket so
// addrless floods are still capped.
func PerIPHTTP(budget HTTPBudget, operation string) func(http.Handler) http.Handler {
	s := &httpLimiterState{clk: realClock{}, budget: budget}
	s.startJanitor()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := logging.RemoteAddrFromContext(r.Context())
			if ip == "" {
				ip = "unknown"
			}
			limiter := s.getLimiter(ip)
			if !limiter.Allow() {
				retrySec := estimateRetryAfter(limiter, budget)
				w.Header().Set("Retry-After", strconv.Itoa(retrySec))
				logging.LoggerWithContext(r.Context()).WarnContext(r.Context(),
					"rate limit exceeded",
					"operation", operation,
					"dimension", "ip",
					"retry_after_sec", retrySec,
				)
				http.Error(w,
					fmt.Sprintf("rate limit exceeded; retry after %ds", retrySec),
					http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// getLimiter returns the bucket for `ip`, creating one (sized from
// `s.budget`) on first use.
func (s *httpLimiterState) getLimiter(ip string) *rate.Limiter {
	now := s.clk.Now()
	if actual, ok := s.limiters.Load(ip); ok {
		e := actual.(*entry)
		e.mu.Lock()
		e.lastUsed = now
		e.mu.Unlock()
		return e.limiter
	}
	e := &entry{
		limiter:  rate.NewLimiter(s.budget.Rate, s.budget.Burst),
		lastUsed: now,
	}
	actual, _ := s.limiters.LoadOrStore(ip, e)
	loaded := actual.(*entry)
	loaded.mu.Lock()
	loaded.lastUsed = now
	loaded.mu.Unlock()
	return loaded.limiter
}

// startJanitor evicts idle buckets every 5 minutes to prevent
// unbounded memory growth from one entry per distinct IP ever seen.
func (s *httpLimiterState) startJanitor() {
	s.janitorOnce.Do(func() {
		var clean func()
		clean = func() {
			now := s.clk.Now()
			s.limiters.Range(func(k, v any) bool {
				e := v.(*entry)
				e.mu.Lock()
				idle := now.Sub(e.lastUsed)
				e.mu.Unlock()
				if idle > 5*time.Minute {
					s.limiters.Delete(k)
				}
				return true
			})
			s.clk.AfterFunc(5*time.Minute, clean)
		}
		s.clk.AfterFunc(5*time.Minute, clean)
	})
}

// estimateRetryAfter returns a small upper-bound on the seconds the
// caller should wait before retrying. We don't pretend to be precise
// — `rate.Limiter` doesn't expose its internal token count — so we
// fall back to a conservative ceil(1 / Rate) when burst is 0/1, or 1
// when Rate is essentially infinite. Sub-issue 5 can tighten this.
func estimateRetryAfter(_ *rate.Limiter, budget HTTPBudget) int {
	if budget.Rate <= 0 || math.IsInf(float64(budget.Rate), 1) {
		return 1
	}
	secs := int(math.Ceil(1 / float64(budget.Rate)))
	if secs < 1 {
		return 1
	}
	return secs
}
