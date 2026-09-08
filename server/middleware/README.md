# Middleware

The `middleware` package provides HTTP middleware for the Connect/gRPC server: structured request logging, request-ID injection, and per-request query-stat collection.

## Key files

- `logging.go` — `Logging(logger)`: logs method, path, status, duration, response bytes, and extracted RPC method name for every request. Includes user ID when the request is authenticated.
- `request_id.go` — `RequestID`: generates a unique request ID per request and stores it in context for downstream structured log fields.
- `remote_addr.go` — `RemoteAddr`: resolves the client IP from `X-Forwarded-For` or `r.RemoteAddr` and stores it in context via `logging.WithRemoteAddr`. Must run early in the chain so auth-failure log lines and rate limiters see a real IP.
- `query_stats.go` — middleware that initializes the per-request `QueryStats` context used by `storage.AssertMaxQueries` in tests.
- `cors.go` — `CORSOriginValidator(allowed)`: origin-allowlist matcher for the CORS layer, supporting exact origins, `host:*` port wildcards, and single-label subdomain wildcards (`https://*.staging.ripls.org`) for per-PR staging previews.
- `request_host.go` — `RequestHost(defaultPort)`: injects the client's request host into context so local bucket storage can mint presigned URLs that work from emulators and localhost alike. Skipped entirely when media lives in GCS.
- `ratelimit/` — `ratelimit.Interceptor`: Connect unary interceptor that enforces per-(procedure, IP) and per-(procedure, email) token-bucket limits on the `LoginService`.

## When to add code here vs. elsewhere

HTTP-level concerns (headers, timing, IDs) belong here. Authentication and authorization enforcement belong in `server/auth`. Business-logic middleware (e.g. simulation clock injection) belongs in `server/clock`.
