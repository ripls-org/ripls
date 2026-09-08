package ratelimit

import (
	"context"
	"errors"
	"math"
	"strconv"
	"sync"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/time/rate"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/logging"
)

// errRateLimited is the sentinel error returned to callers when a request is
// rejected by a token-bucket limit. Connect maps CodeResourceExhausted to HTTP 429.
var errRateLimited = errors.New("too many requests")

// Budget defines a token-bucket limit for a single key dimension.
type Budget struct {
	// Rate is the sustained number of events allowed per second.
	Rate rate.Limit
	// Burst is the maximum burst size (events allowed in an instant).
	Burst int
	// MetricsOnly enables soak-before-enforce mode: the limiter still
	// tracks state, increments the rejection counter (labeled
	// mode="metrics_only"), and emits the warn log on what would have
	// been a rejection, but **does not actually reject the request**.
	// Use this to characterize burst patterns in production before
	// flipping to enforce mode (Budget.MetricsOnly = false, the default).
	MetricsOnly bool

	// shadowedFromEnforce records that this budget was an enforce-mode
	// budget in the source rule set but has been forced to MetricsOnly
	// by AllRulesAsMetricsOnly. The rejection metric tags such records
	// mode="dev_shadow" instead of "metrics_only" so dashboards can
	// distinguish "would-have-rejected-in-prod" from "actually-soaking-
	// in-prod" without confusing the two populations. Package-private
	// because only the shadow transformer should set it.
	shadowedFromEnforce bool
}

// Rule describes the rate-limiting policy for a single Connect procedure.
// Any combination of IPLimit, EmailLimit, and UserIDLimit may be set; a nil
// field means that dimension is not rate-limited for this procedure.
//
// Dimension selection guidance:
//
//   - IPLimit: unauthenticated traffic (e.g. login, register, password
//     reset). At this layer the client IP is the only stable signal.
//   - EmailLimit: unauthenticated procedures whose request body carries
//     an email — lets the limiter discriminate between a single email
//     under a brute-force attempt and many emails from one shared IP.
//   - UserIDLimit: authenticated procedures. User id is more stable
//     than IP (mobile carriers, shared egress) and more privacy-respectful
//     than email (no PII in the limiter map).
type Rule struct {
	// Procedure is the fully-qualified Connect procedure name (e.g.
	// "/ripls.api.LoginService/EmailLogin").
	Procedure string
	// IPLimit enforces a per-(procedure, client-IP) token-bucket limit.
	IPLimit *Budget
	// EmailLimit enforces a per-(procedure, email) token-bucket limit. The
	// interceptor extracts the email via the GetEmail() method on the request
	// message; procedures whose request type lacks GetEmail() are silently
	// skipped for this dimension.
	EmailLimit *Budget
	// UserIDLimit enforces a per-(procedure, user-id) token-bucket limit.
	// User id is extracted from auth.GetAuthInfo(ctx); unauthenticated
	// procedures (e.g. LoginService) are silently skipped for this
	// dimension because no auth info is present in the context.
	UserIDLimit *Budget
}

// emailGetter is satisfied by any proto message that carries an email field.
// The Connect-generated getter GetEmail() is present on every request type that
// has an email field, so no hand-written adapter is needed.
type emailGetter interface {
	GetEmail() string
}

// clock is a minimal interface over time functions, swapped in tests for a fake.
type clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) *time.Timer
}

type realClock struct{}

func (realClock) Now() time.Time                                  { return time.Now() }
func (realClock) AfterFunc(d time.Duration, f func()) *time.Timer { return time.AfterFunc(d, f) }

// entry pairs a rate.Limiter with the last-used timestamp for janitor eviction.
type entry struct {
	limiter  *rate.Limiter
	lastUsed time.Time
	mu       sync.Mutex
}

// interceptorState holds the sync.Maps and janitor bookkeeping shared across
// all requests handled by a single Interceptor instance.
type interceptorState struct {
	ipLimiters     sync.Map
	emailLimiters  sync.Map
	userIDLimiters sync.Map
	janitorOnce    sync.Once
	clk            clock
}

// getLimiter returns the existing limiter for key, creating one if absent.
func (s *interceptorState) getLimiter(m *sync.Map, key string, b *Budget) *rate.Limiter {
	now := s.clk.Now()
	if actual, ok := m.Load(key); ok {
		e := actual.(*entry)
		e.mu.Lock()
		e.lastUsed = now
		e.mu.Unlock()
		return e.limiter
	}
	e := &entry{
		limiter:  rate.NewLimiter(b.Rate, b.Burst),
		lastUsed: now,
	}
	actual, _ := m.LoadOrStore(key, e)
	loaded := actual.(*entry)
	loaded.mu.Lock()
	loaded.lastUsed = now
	loaded.mu.Unlock()
	return loaded.limiter
}

// startJanitor ensures a single background goroutine runs every 5 minutes to
// evict limiters that have been idle long enough to fully refill. This prevents
// unbounded memory growth from one entry per distinct IP or email forever.
func (s *interceptorState) startJanitor() {
	s.janitorOnce.Do(func() {
		var clean func()
		clean = func() {
			now := s.clk.Now()
			evict := func(m *sync.Map) {
				m.Range(func(k, v any) bool {
					e := v.(*entry)
					e.mu.Lock()
					idle := now.Sub(e.lastUsed)
					e.mu.Unlock()
					if idle > 5*time.Minute {
						m.Delete(k)
					}
					return true
				})
			}
			evict(&s.ipLimiters)
			evict(&s.emailLimiters)
			evict(&s.userIDLimiters)
			s.clk.AfterFunc(5*time.Minute, clean)
		}
		s.clk.AfterFunc(5*time.Minute, clean)
	})
}

// userIDFromContext extracts the authenticated user id from ctx, returning
// "" when no auth info is present. Used by the user-id rate-limit
// dimension; an empty result causes that dimension to be skipped (a
// deliberate fail-open: missing auth context is upstream wiring's
// problem to fix, not a reason to 429 the request).
func userIDFromContext(ctx context.Context) string {
	info, ok := auth.GetAuthInfo(ctx)
	if !ok || info == nil {
		return ""
	}
	return info.UserID
}

// tryAcquire attempts to consume one token from lim. Returns (true, 0)
// when the token was consumed; (false, delay) when the limiter is empty
// and the request would have to wait at least `delay` for the next
// token. Uses Reserve() rather than Allow() so the caller can read the
// precise delay; Cancel() restores the reserved token when the call
// returns false so the limiter state is unchanged on rejection.
//
// A Burst-of-0 budget produces a non-OK reservation; tryAcquire returns
// (false, 0) in that case and the caller uses a non-zero fallback for
// Retry-After. Budget{Burst: 0} is a deny-all sentinel used only in
// tests.
func tryAcquire(lim *rate.Limiter) (bool, time.Duration) {
	r := lim.Reserve()
	if !r.OK() {
		return false, 0
	}
	if d := r.Delay(); d > 0 {
		r.Cancel()
		return false, d
	}
	return true, 0
}

// retryAfterSeconds rounds delay up to whole seconds with a minimum of 1,
// matching the smallest sensible HTTP Retry-After value. The HTTP header
// is an integer field so we always emit something the client can parse.
func retryAfterSeconds(delay time.Duration) int {
	if delay <= 0 {
		return 1
	}
	secs := int(math.Ceil(delay.Seconds()))
	if secs < 1 {
		return 1
	}
	return secs
}

// budgetMode reports the mode field used in the "rate_limited" warn
// logs for budget b. Three values:
//
//   - "enforce" — b.MetricsOnly is false; rejections actually 429.
//   - "metrics_only" — b.MetricsOnly is true in the source rule set
//     (production soak). Rejections are logged but allowed.
//   - "dev_shadow" — b.MetricsOnly is true because the dev shadow
//     transformer forced it on; in the source rule set this budget
//     was enforce. Dev-only signal so log queries can show "would
//     have rejected in prod" separately from "soaking in prod".
func budgetMode(b *Budget) string {
	if !b.MetricsOnly {
		return "enforce"
	}
	if b.shadowedFromEnforce {
		return "dev_shadow"
	}
	return "metrics_only"
}

// rejection returns the connect.Error the caller should propagate for a
// rate-limit hit. In any MetricsOnly mode (real or shadowed) this returns
// nil — the request proceeds to the handler; the caller's "rate_limited"
// warn log (which carries procedure, key_type, and mode) is the record.
//
// Sets the standard HTTP Retry-After response header via Meta() on the
// returned error so CDNs and well-behaved clients back off without
// parsing custom metadata.
func rejection(retryAfter int, b *Budget) error {
	if b.MetricsOnly {
		return nil
	}
	e := connect.NewError(connect.CodeResourceExhausted, errRateLimited)
	e.Meta().Set("Retry-After", strconv.Itoa(retryAfter))
	return e
}

// Interceptor returns a Connect interceptor that enforces the supplied rules
// against both unary and streaming server RPCs. Rules are matched by exact
// procedure name; unmatched procedures pass through unconditionally.
//
// Empty key values (no IP in context, no email in request body, no
// authenticated user) are treated as pass-through for that dimension — a
// deliberate fail-open for transport plumbing bugs. The audit log still
// records what's missing via the surrounding handlers, so the wiring
// gap is visible without spurious 429s for every user.
//
// On streams, the email dimension is a no-op because there's no request
// body to extract from. Use IP for unauthenticated streams and user-id
// for authenticated streams.
func Interceptor(rules ...Rule) connect.Interceptor {
	byProc := make(map[string]*Rule, len(rules))
	for i := range rules {
		r := rules[i]
		byProc[r.Procedure] = &r
	}

	s := &interceptorState{clk: realClock{}}
	s.startJanitor()

	return &interceptor{state: s, byProc: byProc}
}

// interceptor implements connect.Interceptor for both unary and streaming
// server RPCs. The same Rule registry feeds both wrappers; the streaming
// path omits the email dimension because no request body is available
// at stream-open time.
type interceptor struct {
	state  *interceptorState
	byProc map[string]*Rule
}

// WrapUnary applies the rate-limit rule (if any) for the request's procedure,
// then forwards to next. Rejected requests return CodeResourceExhausted with
// a Retry-After header; metrics-only rejections log + count but proceed.
func (i *interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		proc := req.Spec().Procedure
		rule, ok := i.byProc[proc]
		if !ok {
			return next(ctx, req)
		}
		if err := i.applyRules(ctx, proc, rule, req.Any()); err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

// WrapStreamingClient is a no-op: the rate limiter is server-side only.
func (i *interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// WrapStreamingHandler applies the rate-limit rule for the stream's procedure
// at stream-open time. Once accepted, the stream proceeds unthrottled; the
// limiter intentionally guards stream *initiation*, not in-stream message
// rate, because gen-AI cost is dominated by the call-setup decision (a
// single token spend per opened stream).
func (i *interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		proc := conn.Spec().Procedure
		rule, ok := i.byProc[proc]
		if !ok {
			return next(ctx, conn)
		}
		// Streams have no request body at stream-open time, so the email
		// dimension is unconditionally skipped here (msg = nil).
		if err := i.applyRules(ctx, proc, rule, nil); err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

// applyRules walks the configured key dimensions for rule and returns the
// first dimension's rejection error (or nil to allow). msg may be nil for
// streaming entry points that have no request body; the email dimension is
// skipped in that case.
func (i *interceptor) applyRules(ctx context.Context, proc string, rule *Rule, msg any) error {
	logger := logging.LoggerWithContext(ctx)

	// IP dimension.
	if rule.IPLimit != nil {
		ip := logging.RemoteAddrFromContext(ctx)
		if ip != "" {
			key := proc + "\x00" + ip
			lim := i.state.getLimiter(&i.state.ipLimiters, key, rule.IPLimit)
			if allowed, delay := tryAcquire(lim); !allowed {
				logger.WarnContext(ctx, "rate_limited",
					"procedure", proc,
					"key_type", "ip",
					"remote_addr", ip,
					"mode", budgetMode(rule.IPLimit),
				)
				if err := rejection(retryAfterSeconds(delay), rule.IPLimit); err != nil {
					return err
				}
			}
		}
	}

	// Email dimension (unary-only — no request body on streams).
	if rule.EmailLimit != nil && msg != nil {
		if eg, ok := msg.(emailGetter); ok {
			email := eg.GetEmail()
			if email != "" {
				key := proc + "\x00" + email
				lim := i.state.getLimiter(&i.state.emailLimiters, key, rule.EmailLimit)
				if allowed, delay := tryAcquire(lim); !allowed {
					logger.WarnContext(ctx, "rate_limited",
						"procedure", proc,
						"key_type", "email",
						"user_email", logging.MaskEmail(email),
						"mode", budgetMode(rule.EmailLimit),
					)
					if err := rejection(retryAfterSeconds(delay), rule.EmailLimit); err != nil {
						return err
					}
				}
			}
		}
	}

	// User-id dimension. Skipped for unauthenticated procedures
	// where auth.GetAuthInfo returns no info.
	if rule.UserIDLimit != nil {
		userID := userIDFromContext(ctx)
		if userID != "" {
			key := proc + "\x00" + userID
			lim := i.state.getLimiter(&i.state.userIDLimiters, key, rule.UserIDLimit)
			if allowed, delay := tryAcquire(lim); !allowed {
				logger.WarnContext(ctx, "rate_limited",
					"procedure", proc,
					"key_type", "user_id",
					"user_id", userID,
					"mode", budgetMode(rule.UserIDLimit),
				)
				if err := rejection(retryAfterSeconds(delay), rule.UserIDLimit); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// AllRules returns every default rule set the server enforces. The shared
// rate-limit interceptor in main.go consumes this — adding a new rule set
// is one append here, no main.go edit. Empty rule sets are pass-through,
// so concatenation is safe even when only some procedures are limited.
func AllRules() []Rule {
	rules := make([]Rule, 0)
	rules = append(rules, DefaultLoginRules()...)
	rules = append(rules, DefaultUserRules()...)
	rules = append(rules, DefaultMediaRules()...)
	rules = append(rules, DefaultGenAIRules()...)
	return rules
}

// AllRulesAsMetricsOnly returns AllRules() with every non-nil budget
// transformed to MetricsOnly = true. main.go wires this set in dev so
// every rule produces warn logs and rejection-counter increments for
// local observation, without dev ever seeing a 429 from any rule
// (enforce-mode or already-soaking).
//
// The transformation preserves observability of the source rule set
// via the mode label / log field:
//
//   - A budget that was MetricsOnly in source still tags
//     mode="metrics_only" (true production soak — what the dashboards
//     in prod see).
//   - A budget that was enforce-mode in source tags mode="dev_shadow"
//     (would-have-rejected-in-prod). Same observability without the
//     accidental promotion: in prod the budget is still enforce, in
//     dev it is shadowed.
//
// This lets a developer add an enforce-mode rule and immediately see it
// fire locally — without changing prod behavior — and lets them validate
// a MetricsOnly rule against the same metric series prod will populate.
func AllRulesAsMetricsOnly() []Rule {
	all := AllRules()
	out := make([]Rule, 0, len(all))
	for _, r := range all {
		out = append(out, Rule{
			Procedure:   r.Procedure,
			IPLimit:     shadowBudget(r.IPLimit),
			EmailLimit:  shadowBudget(r.EmailLimit),
			UserIDLimit: shadowBudget(r.UserIDLimit),
		})
	}
	return out
}

// shadowBudget returns a copy of b with MetricsOnly forced to true. If
// b was enforce-mode in source, the copy is tagged shadowedFromEnforce
// so the rejection metric labels it mode="dev_shadow". nil-safe.
func shadowBudget(b *Budget) *Budget {
	if b == nil {
		return nil
	}
	return &Budget{
		Rate:                b.Rate,
		Burst:               b.Burst,
		MetricsOnly:         true,
		shadowedFromEnforce: !b.MetricsOnly,
	}
}

// DefaultLoginRules returns the default rate-limit rules for the LoginService.
// All budgets are conservative starting points; tune once production traffic
// patterns are understood.
func DefaultLoginRules() []Rule {
	return []Rule{
		// Login endpoints: tight per-IP, per-email for email-based login.
		{
			Procedure:  apiconnect.LoginServiceEmailLoginProcedure,
			IPLimit:    &Budget{Rate: rate.Every(12 * time.Second), Burst: 5}, // 5/min
			EmailLimit: &Budget{Rate: rate.Every(12 * time.Minute), Burst: 5}, // 5/hour
		},
		{
			Procedure: apiconnect.LoginServiceOIDCLoginProcedure,
			IPLimit:   &Budget{Rate: rate.Every(12 * time.Second), Burst: 5}, // 5/min
		},
		{
			// Reports account existence to an unauthenticated caller, so the
			// budget is what bounds bulk probing of the number space.
			Procedure: apiconnect.LoginServiceCheckPhoneRegisteredProcedure,
			IPLimit:   &Budget{Rate: rate.Every(6 * time.Second), Burst: 10}, // 10/min
		},
		{
			Procedure: apiconnect.LoginServicePhoneLoginProcedure,
			IPLimit:   &Budget{Rate: rate.Every(12 * time.Second), Burst: 5}, // 5/min
		},
		// Registration endpoints: tight per-IP (registration is rare).
		{
			Procedure: apiconnect.LoginServiceEmailRegisterProcedure,
			IPLimit:   &Budget{Rate: rate.Every(20 * time.Minute), Burst: 3}, // 3/hour
		},
		{
			Procedure: apiconnect.LoginServiceOIDCRegisterProcedure,
			IPLimit:   &Budget{Rate: rate.Every(20 * time.Minute), Burst: 3}, // 3/hour
		},
		{
			Procedure: apiconnect.LoginServicePhoneRegisterProcedure,
			IPLimit:   &Budget{Rate: rate.Every(20 * time.Minute), Burst: 3}, // 3/hour
		},
		// Email sign-in codes. RequestEmailCode sends mail on every accepted
		// call, so it gets the same budget as password reset. VerifyEmailCode
		// needs a per-email budget on top of the per-code attempt cap the
		// handler enforces: the cap bounds guesses against one issued code,
		// but nothing stops an attacker requesting a fresh code and spending a
		// new allowance, so the address itself is also budgeted.
		{
			Procedure:  apiconnect.LoginServiceRequestEmailCodeProcedure,
			IPLimit:    &Budget{Rate: rate.Every(12 * time.Second), Burst: 5}, // 5/min
			EmailLimit: &Budget{Rate: rate.Every(20 * time.Minute), Burst: 3}, // 3/hour
		},
		{
			Procedure:  apiconnect.LoginServiceVerifyEmailCodeProcedure,
			IPLimit:    &Budget{Rate: rate.Every(6 * time.Second), Burst: 10}, // 10/min
			EmailLimit: &Budget{Rate: rate.Every(6 * time.Minute), Burst: 10}, // 10/hour
		},
		// Password reset: matches issue recommendation.
		{
			Procedure:  apiconnect.LoginServiceRequestPasswordResetProcedure,
			IPLimit:    &Budget{Rate: rate.Every(12 * time.Second), Burst: 5}, // 5/min
			EmailLimit: &Budget{Rate: rate.Every(20 * time.Minute), Burst: 3}, // 3/hour
		},
		{
			Procedure: apiconnect.LoginServiceResetPasswordProcedure,
			IPLimit:   &Budget{Rate: rate.Every(12 * time.Second), Burst: 5}, // 5/min
		},
		// Lookup/check endpoints: looser budget (used during normal flows).
		{
			Procedure: apiconnect.LoginServiceCheckResetPasswordTokenProcedure,
			IPLimit:   &Budget{Rate: rate.Every(2 * time.Second), Burst: 30}, // 30/min
		},
		{
			Procedure: apiconnect.LoginServiceCheckInvitationProcedure,
			IPLimit:   &Budget{Rate: rate.Every(2 * time.Second), Burst: 30}, // 30/min
		},
		// Refresh: loose IP-only (authenticated implicitly by token possession).
		{
			Procedure: apiconnect.LoginServiceRefreshTokenProcedure,
			IPLimit:   &Budget{Rate: rate.Every(time.Second), Burst: 60}, // 60/min
		},
	}
}

// DefaultUserRules returns the default rate-limit rules for the UserService.
// AddPhoneNumber verifies a Firebase phone OTP token and mutates the account;
// it is a rare, authenticated upgrade action, so both per-user and per-IP
// budgets are tight. Firebase already gates OTP cost upstream — this is a
// cheap server-side backstop against the verify path being hammered.
func DefaultUserRules() []Rule {
	return []Rule{
		{
			Procedure:   apiconnect.UserServiceAddPhoneNumberProcedure,
			IPLimit:     &Budget{Rate: rate.Every(6 * time.Minute), Burst: 5}, // 5/30min
			UserIDLimit: &Budget{Rate: rate.Every(6 * time.Minute), Burst: 5}, // 5/30min
		},
	}
}

// DefaultMediaRules returns the default rate-limit rules for the MediaService.
// AddMediaFromURL triggers a server-side outbound HTTPS fetch and writes
// a 60 MB-capped media row per call — the per-user budget is sized to
// comfortably accommodate Replace-Media interaction (a few candidate
// picks per modal open) while preventing scripted abuse.
func DefaultMediaRules() []Rule {
	return []Rule{
		{
			Procedure:   apiconnect.MediaServiceAddMediaFromURLProcedure,
			UserIDLimit: &Budget{Rate: rate.Every(6 * time.Second), Burst: 3}, // 10/min, burst 3
		},
	}
}

// DefaultGenAIRules returns the default rate-limit rules for the AI
// streaming gen RPCs. Each call opens a stream that fans out to one or
// more paid AI provider invocations (token spend, model quota, latency),
// so the per-user budget is tighter than the media rules.
//
// All gen-AI budgets ship with MetricsOnly = true so the "rate_limited"
// warn logs record what *would* have been rejected. Flip MetricsOnly to
// false in a follow-up PR once burst patterns are characterized in
// production — the mode="metrics_only" log lines give the visibility to
// make that call without guessing.
func DefaultGenAIRules() []Rule {
	// 4/min per user per RPC: each stream opens at least one expensive AI
	// call and can run for several seconds; a power user creating four
	// entities back-to-back in a minute is at the upper edge of
	// realistic intentional use.
	budget := &Budget{Rate: rate.Every(15 * time.Second), Burst: 4, MetricsOnly: true}
	return []Rule{
		{
			Procedure:   apiconnect.UnifiedCreateServiceStreamGenUnifiedCreateProcedure,
			UserIDLimit: budget,
		},
		{
			Procedure:   apiconnect.ExperienceServiceStreamGenExperienceProcedure,
			UserIDLimit: budget,
		},
		{
			Procedure:   apiconnect.GearServiceStreamGenGearProcedure,
			UserIDLimit: budget,
		},
		{
			Procedure:   apiconnect.RequestServiceStreamGenRequestProcedure,
			UserIDLimit: budget,
		},
		{
			Procedure:   apiconnect.CommunityServiceStreamGenCommunityProcedure,
			UserIDLimit: budget,
		},
	}
}
