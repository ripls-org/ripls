# ratelimit

The `ratelimit` package provides a Connect interceptor that enforces per-(procedure, key) token-bucket rate limits across both unary and streaming RPCs. Three key dimensions are supported: client IP, request-body email, and authenticated user id.

## Why a Connect interceptor rather than HTTP middleware

Rate limiting needs to see request-body fields (email for login) and the authenticated user id, both of which are only available after Connect deserialization. HTTP middleware operates before Connect deserializes the request, so extracting these would require buffering and re-parsing the wire format. A Connect interceptor receives the typed request struct via `req.Any()` and runs after the auth middleware populates the context, making all three dimensions available cleanly.

## API

```go
// Budget defines a token-bucket limit for a single key dimension.
type Budget struct {
    Rate        rate.Limit // tokens per second (sustained)
    Burst       int        // maximum instant burst
    MetricsOnly bool       // soak-before-enforce: log and count but don't reject
}

// Rule describes the policy for a single Connect procedure. Any
// combination of dimensions may be set; nil means "not limited on
// that dimension".
type Rule struct {
    Procedure   string
    IPLimit     *Budget // per-(procedure, client-IP) limit
    EmailLimit  *Budget // per-(procedure, email-from-request-body) limit — unary only
    UserIDLimit *Budget // per-(procedure, authenticated-user-id) limit
}

// Interceptor returns a connect.Interceptor enforcing the supplied rules
// for both unary and streaming server RPCs.
func Interceptor(rules ...Rule) connect.Interceptor

// AllRules returns every default rule set the server enforces — login,
// media, gen-AI. The shared interceptor in main.go consumes this.
func AllRules() []Rule
```

## Choosing a dimension

| Procedure type             | Recommended dimension                              |
|----------------------------|----------------------------------------------------|
| Unauthenticated, no email  | `IPLimit`                                          |
| Unauthenticated, has email | `IPLimit` + `EmailLimit`                           |
| Authenticated              | `UserIDLimit` (more stable than IP, less PII than email) |

The interceptor reads:

- IP from `logging.RemoteAddrFromContext(ctx)` (wired by `server/middleware/remote_addr.go`).
- Email from `req.Any().(emailGetter).GetEmail()` (the Connect-generated getter on any request type with an `email` field).
- User id from `auth.GetAuthInfo(ctx).UserID` (populated by `authn.Middleware` running before the interceptor).

Any dimension whose key resolves to the empty string is skipped — a deliberate fail-open so transport-wiring bugs don't spuriously 429 every request.

## Rejection response

In **enforce** mode, every rejection returns `connect.CodeResourceExhausted` (HTTP 429) with a standard HTTP `Retry-After` response header (integer seconds, computed from the next-token delay via `Reserve()/Cancel()`).

Every rejection (and every would-be rejection in `MetricsOnly` mode) emits a `rate_limited` WARN log with `procedure`, `key_type`, and `mode` fields, so Cloud Logging queries (or a log-based metric, if one is ever needed) can watch for abuse spikes before they hit users.

## Metrics-only soak mode

Set `Budget.MetricsOnly = true` to ship a new rule live without actually rejecting requests. The limiter still:

- Tracks per-key state (so behavior on flip-to-enforce matches what was observed during soak).
- Emits the `rate_limited` warn log with `metrics_only=true`.
- Increments the rejection counter with `mode="metrics_only"`.

But it does **not** return `CodeResourceExhausted` — the request proceeds. Use this for rules whose budget is tighter than measured production traffic; flip to enforce by clearing `MetricsOnly` once burst patterns are characterized. All `DefaultGenAIRules()` ship with `MetricsOnly = true` for exactly this reason.

## Dev-mode rule selection

`main.go` calls `AllRules()` in prod and `AllRulesAsMetricsOnly()` in dev. The dev variant is the same rule set with every budget forced to `MetricsOnly = true`, so dev never sees a 429 from this interceptor — but every rule still produces a warn log and a rejection-counter increment for local observation.

The mode label preserves source provenance so dev observations match prod populations on the same dashboards:

| source budget    | dev `mode` label  | prod `mode` label |
|------------------|-------------------|-------------------|
| enforce-mode     | `dev_shadow`      | `enforce`         |
| `MetricsOnly`    | `metrics_only`    | `metrics_only`    |

Practical consequences:

- **Adding an enforce rule and want to validate it locally?** Run the server in dev mode and trip the rule; you'll see a `rate_limited` warn log with `mode="dev_shadow"` and the procedure FQN. Prod behavior is unaffected — the budget is still enforce-mode in `AllRules()`.
- **Validating a MetricsOnly rule?** Same flow; you'll see `mode="metrics_only"` — the exact field prod populates, so you can confirm the log query before the rule reaches production traffic.
- **Dev never throttles.** `MetricsOnly = true` is enforced for every dev request, so integration tests and simulation runs don't hit 429s regardless of how aggressive a rule's budget is.

## Streaming RPCs

Streaming handlers go through `WrapStreamingHandler`, which guards stream *initiation* (open) rather than per-message rate. For gen-AI streams this is the right cut-point: cost is dominated by the call-setup decision (one or more token spends per opened stream), not by individual chunks. The email dimension is skipped on streams because no request body is available at stream-open time.

## Budget table (DefaultLoginRules)

| Procedure              | IP limit       | Email limit    |
|------------------------|----------------|----------------|
| EmailLogin             | 5 / minute     | 5 / hour       |
| OIDCLogin              | 5 / minute     | —              |
| PhoneLogin             | 5 / minute     | —              |
| EmailRegister          | 3 / hour       | —              |
| OIDCRegister           | 3 / hour       | —              |
| PhoneRegister          | 3 / hour       | —              |
| RequestPasswordReset   | 5 / minute     | 3 / hour       |
| ResetPassword          | 5 / minute     | —              |
| CheckResetPasswordToken | 30 / minute   | —              |
| CheckInvitation        | 30 / minute    | —              |
| RefreshToken           | 60 / minute    | —              |

Budgets are conservative starting points. Tune via `ratelimit.DefaultLoginRules()` once production traffic shapes are understood.

## Fail-open on missing IP

When the client IP is not present in the context (e.g. the `RemoteAddr` middleware is unwired), the IP dimension is skipped rather than rejected. This is a deliberate fail-open: the audit log still records the failure, and a transport-wiring bug should not produce spurious 429s for all users. Fix the wiring, not the rate limiter.

## Dev mode

Wire the interceptor only when `cfg.devMode == false` (see `server/main.go`). Simulation runs and integration tests make rapid requests and would be flaky if throttled.

## Memory management

Limiters accumulate one entry per distinct (procedure, IP) or (procedure, email) key. A background janitor goroutine (started once per `Interceptor` call) runs every 5 minutes and evicts entries that have been idle long enough to fully refill. This prevents unbounded memory growth in long-running servers.

## See also

- `server/middleware/remote_addr.go` — wires the client IP into the request context.
- `server/services/login/README.md` — describes the LoginService and its rate-limiting configuration.
