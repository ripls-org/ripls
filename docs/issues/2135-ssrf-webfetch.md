# Issue #2135: Security: SSRF via webfetch — user-controlled URLs in webpage-mode RPCs reach internal HTTP endpoints, response body exfiltrated through og:image media store

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2135
**Filed:** 2026-05-23T17:02:32Z | **Last activity:** 2026-05-23T17:18:53Z
**Labels:** bug, security, server | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Several RPCs accept a caller-supplied URL and hand it directly to
`webfetch.HTTPFetcher.FetchPageContent`
(`server/webfetch/fetcher.go:168`). The current validation checks the
URL scheme (`http`/`https`) and that the host is non-empty, then issues
the request through a stock `*http.Client` that follows redirects with
default behavior. There is no check for private, loopback, link-local,
CGNAT, multicast, or unspecified addresses, and there is no
post-resolution / redirect-time guard.

The same fetch surface exists in two more places:

1. `server/services/experience/webpage_image.go::downloadAndStoreWebpageImage`
   issues a follow-up `GET` to whatever `og:image` URL the previous
   fetch extracted, with a freshly constructed `&http.Client{}` that
   has no SSRF guard at all.
2. `server/services/media/service.go::AddMediaFromURL` already routes
   through an SSRF-guarded client built by
   `server/services/media/safe_outbound.go::newSafeOutboundClient`. The
   guard is the right design — it uses a `net.Dialer.Control` callback
   that runs after DNS resolution (so DNS-rebind is closed) plus a
   `CheckRedirect` hook that re-asserts the scheme on every hop.

The webfetch and webpage-image paths are exfiltration channels: the
fetched page text is round-tripped through the AI provider and the
generated `Name`/`Description` quote it back; the `og:image` response
body is stored bytes-for-bytes in GCS as a media row owned by the
caller, retrievable through standard `MediaService` reads.

The two RPC chains share a single fix: lift the proven SSRF guard out
of the `media` package into a shared location, use it in `webfetch`
and `webpage_image`, and harden the small differences that remain
(initial-URL validation, scheme allowance for the og:image fetch).

## Context from Comments

- `briangbrown` (collaborator) added a comment confirming
  `media.AddMediaFromURL` is also in scope and pointed out that its
  initial https-only check does not cover redirect targets — Go's
  default `http.Client.Do` follows cross-scheme redirects. (This is
  already mitigated on `main` by `newSafeOutboundClient`'s
  `CheckRedirect` rejecting non-https redirect targets; the
  pre-existing safeguard is the model we should reuse.)
- He also flagged that the read-side enforcement gap in `media.GetMedia`
  (#1529) is what makes the og:image stored body retrievable
  end-to-end. That issue is tracked separately and is **out of scope**
  for this fix; closing the SSRF write-side already eliminates the
  exfiltration vector regardless of read-side policy.
- The final comment is `@claude plan`, requesting the plan path.

No disagreement in the thread. The fix shape is well-specified by the
issue body and the existing `media/safe_outbound.go` implementation.

## Potential Solutions

### Option A: Extract a shared safe-outbound package, retrofit both call sites

- **Approach:** Move the existing `newSafeOutboundClient`,
  `safeDialControl`, `isPubliclyRoutable`, and `classifyDialErr`
  helpers out of `server/services/media/safe_outbound.go` into a new
  `server/safehttp/` package. Update `media.AddMediaFromURL` to import
  the shared symbols. Add an `Option` to `webfetch.NewHTTPFetcher`
  that swaps in the safe transport (preserving the existing cookie
  jar / timeout / max-body behavior). Rewrite
  `experience.downloadAndStoreWebpageImage` to use a `safehttp` client
  too, and validate the og:image URL scheme up front before the dial
  ever runs. Add a `CheckRedirect` variant for webfetch that allows
  both http and https (since the webpage-mode fetcher is permitted to
  fetch http://) but still re-validates against the dialer guard on
  every hop (the `Control` callback does this automatically).
- **Pros:** One canonical implementation, one test surface, one place
  to fix future bypasses (e.g. new RFC for additional reserved
  ranges). Mirrors the architecture in `server/connecterr/` and
  similar small server-shared packages.
- **Cons:** Touches three packages (`server/services/media`,
  `server/webfetch`, `server/services/experience`) plus the new
  `server/safehttp` package. Requires moving the existing tests
  (`safe_outbound_test.go`) and renaming exported symbols.
- **Effort:** Medium.

### Option B: Duplicate the guard inside `webfetch` and `webpage_image.go`

- **Approach:** Copy the dialer-Control logic and IP-range allow-list
  into private helpers inside `server/webfetch/` and (privately)
  inside `server/services/experience/`. Leave the `media` package
  copy alone.
- **Pros:** Smaller diff; no cross-package imports to wire up.
- **Cons:** Three copies of the same security-critical IP-range
  allow-list. Future changes (a new range to reject, a fix to a
  parser bug) have to land in three places, and skew is the most
  common cause of SSRF-guard regressions. Violates the "single
  responsibility" guidance in `CLAUDE.md`.
- **Effort:** Small.

### Option C: Switch to `github.com/zenazn/goji/httpsafe` (or similar third-party SSRF middleware)

- **Approach:** Pull in a maintained third-party SSRF-guard library
  and use it everywhere.
- **Pros:** Less code in-tree.
- **Cons:** Adds a new dependency on a security-critical path, and
  the existing `safe_outbound.go` already covers the cases we care
  about including ULA, CGNAT, and IPv4-mapped IPv6 (which several
  library implementations miss). Library choices for Go SSRF guards
  are not well-established; auditing one is nontrivial.
- **Effort:** Medium (auditing + integration).

## Recommendation

**Option A.** The required transport already exists and is
test-covered in `server/services/media/safe_outbound.go`. Lifting it
into `server/safehttp/` is a low-risk move that closes every call
site the issue lists, eliminates duplication, and gives us a single
place to extend the allow-list as new internal address ranges show
up. Option B's code-duplication cost is unacceptable on a security
boundary. Option C introduces dependency risk for no incremental
coverage.

The plan should also tighten two details that Option A reveals:

1. **Initial-URL validation in webfetch.** Today the webfetch input
   URL is only checked for scheme/host non-emptiness. Add a parallel
   check that rejects URLs whose literal host is an IP literal in a
   non-public range — this catches the case where an attacker passes
   `http://127.0.0.1/` directly (the dialer hook would also reject
   it, but a fail-fast error before any DNS is cleaner and gives a
   precise error type for `connectErrorFromFetchError` to map to
   `CodeInvalidArgument`).
2. **og:image scheme guard.** `webpage_image.go` currently passes
   the og:image URL through unchanged. Add an explicit
   `url.Parse` + scheme check (http or https) before the dial — so
   `file://` or `gopher://` og:image entries are rejected before they
   become a request error.

## Open Questions

- **Should `webfetch` allow http:// URLs at all, or upgrade to
  https-only?** — Recommendation: keep `http` allowed. The fetcher is
  used by `unified_create`, `gear.DetectGearFromURL`, and impact
  backfill against arbitrary product / event pages, some of which
  legitimately serve http (older garage-sale-style listings, local
  community sites, dev fixtures). The SSRF risk is gone once the
  dialer guard rejects internal IPs, and the existing https-upgrade
  inside `resolveImageURL` already covers the og:image promotion case.
  `webpage_image.go` should similarly allow both schemes for parity
  with the page fetcher — an og:image hosted on an http-only CDN is
  not unusual.
- **Should `AddMediaFromURL` switch from "https only" to "any
  http(s)" once it shares the same guard?** — Recommendation: no, keep
  the existing https-only policy in the handler. That policy is a
  defense-in-depth check unrelated to SSRF (it prevents mixed-content
  warnings and discourages MITM-able imports). The shared client's
  CheckRedirect already enforces https on the AddMediaFromURL path
  via a per-call configuration; we'll expose that as an option.
- **Where should the new package live?** — Recommendation:
  `server/safehttp/`. Mirrors the naming convention used by
  `server/connecterr/` and is short enough to import inline. Reject
  `server/httpsafe/` (less idiomatic Go), `server/security/` (too
  broad — likely to attract unrelated code), or nesting under
  `server/webfetch/` (the package is a peer of webfetch, not a
  subordinate).
- **Should the impact-backfill job
  (`server/jobs/impact_backfill.go`) also use the safe transport?**
  — Recommendation: yes, automatically. The job calls
  `webfetch.FetchPageContent(ctx, g.SourceUrl)` for every gear with a
  source URL; even though the URL was originally caller-supplied at
  gear-creation time and went through the (post-fix) safe webfetch,
  DNS records can change between creation and backfill, so the dialer
  guard at fetch time is the right defense. This is free because the
  job constructs its own `webfetch.NewHTTPFetcher()` in
  `server/main.go:1087` and inherits the hardened default.

## Implementation Plan

### Phase 1: Extract shared `safehttp` package

**Goal:** Single canonical SSRF-guarded HTTP client used by all
user-URL-driven server fetches.

**Tasks:**

- [ ] Create `server/safehttp/doc.go` with a `// Package safehttp ...`
  comment (1–3 sentences) describing the package as "SSRF-guarded
  HTTP client builder for user-supplied URLs." This satisfies the
  `revive.package-comments` lint rule (CLAUDE.md, Style Guidelines).
- [ ] Create `server/safehttp/client.go` containing the moved
  implementation:
  - Move `newSafeOutboundClient`, `safeDialControl`,
    `isPubliclyRoutable`, `errUnsafeOutboundAddr`, `classifyDialErr`
    from `server/services/media/safe_outbound.go`.
  - Export them: `NewClient(opts ...Option) *http.Client`,
    `IsSSRFBlockError(err error) bool`. Internalize the dial-control
    and IP-range helpers as unexported.
  - Add `Option` plumbing for:
    - `WithTimeout(time.Duration)`
    - `WithRedirectSchemes(...string)` — e.g. `"https"` only (used
      by AddMediaFromURL) vs. `"http", "https"` (used by webfetch /
      webpage_image). Default to https-only when no option is set.
    - `WithCookieJar(*cookiejar.Jar)` — webfetch currently builds a
      jar, so we need a way to retain that behavior.
    - `WithMaxRedirects(int)` — default 5 matches the existing impl.
- [ ] Create `server/safehttp/client_test.go` by moving the bodies of
  `safe_outbound_test.go` over. Tests:
  - `TestIsPubliclyRoutable` (rename internal helper or expose a
    test-only `IsPubliclyRoutable` if the test stays in the same
    package — preferable to keep the helper unexported).
  - `TestSafeDialControl_RejectsNonTCP`, `_RejectsPrivateAddr`,
    `_AllowsPublicAddr`.
  - `TestNewClient_CheckRedirect`.
  - **New:** `TestNewClient_AllowsHTTPRedirectWhenConfigured` — opt
    into `WithRedirectSchemes("http", "https")` and assert that an
    `http://` redirect target passes.
- [ ] Add a `server/safehttp/README.md` covering purpose, key files,
  when to import, and a one-paragraph "when to add code here vs.
  elsewhere" guide. This satisfies the per-package README rule
  (CLAUDE.md, Style Guidelines) — model on
  `server/connecterr/README.md` if present, or a short README in the
  shape of `server/webfetch/README.md`. Use the **capability**
  (SSRF-guarded HTTP client), not vendor names; this is internal
  docs, but it's still a package-purpose statement and the rule
  prefers capability framing.
- [ ] Add `server/safehttp/` to the dependency arrows in
  `docs/server/architecture.md` if it documents package layering
  (verify; if the file does not enumerate packages, no change).

**Validates:** Existing media-service tests continue to pass after
the move (signal that the API is compatible); new redirect-scheme
test passes (signal that the option plumbing works).

### Phase 2: Switch `media.AddMediaFromURL` to the shared package

**Goal:** Confirm zero behavior change for the already-protected path,
delete the in-package copy.

**Tasks:**

- [ ] Update `server/services/media/service.go`:
  - Replace `s.outboundClient = newSafeOutboundClient` with
    `s.outboundClient = func(t time.Duration) *http.Client {
       return safehttp.NewClient(safehttp.WithTimeout(t),
         safehttp.WithRedirectSchemes("https"))
     }`.
  - Replace `classifyDialErr(err)` with
    `safehttp.IsSSRFBlockError(err)`.
- [ ] Delete `server/services/media/safe_outbound.go` and
  `server/services/media/safe_outbound_test.go` (now lives in
  `server/safehttp/`).
- [ ] Run `npm run build` and `go test ./server/services/media/...` —
  full media-service test suite must stay green, including the
  integration test that injects a custom `outboundClient` for
  loopback test servers (`media/service_test.go` — verify by reading
  it during this phase).
- [ ] If the media test injects its own custom client (it does, per
  the comment in `service.go:46-49`), confirm the test still works
  with the new shape; if needed, add an unexported `setOutboundClient`
  helper to the media `Service` so tests can override.

**Validates:** AddMediaFromURL path is unchanged in observable
behavior. Demonstrates the shared package is API-compatible.

### Phase 3: Harden `webfetch.HTTPFetcher`

**Goal:** Every `FetchPageContent` caller (six RPCs plus impact
backfill plus product lookup) is SSRF-safe by default.

**Tasks:**

- [ ] Update `server/webfetch/fetcher.go`:
  - In `NewHTTPFetcher`, build `f.client` by default via
    `safehttp.NewClient(safehttp.WithTimeout(DefaultTimeout),
       safehttp.WithRedirectSchemes("http", "https"),
       safehttp.WithCookieJar(jar))`. Keep `WithTimeout` working:
    defer client construction until after option processing so the
    final timeout is the one passed in.
  - Do **not** add a public "disable safe outbound" Option — that is
    a foot-gun on a security boundary. For tests that need to hit a
    loopback `httptest` server, add an **unexported, package-level**
    test seam in `server/webfetch/`: e.g. an internal
    `withClientForTesting(*http.Client) Option` defined in a file
    suffixed `_testing.go` or guarded by a small `func
    setClientForTesting(f *HTTPFetcher, c *http.Client)` (mirrors the
    `outboundClient` injection in `server/services/media/service.go`).
    Production code keeps the safe default unconditionally.
  - In `FetchPageContent`, immediately after the existing scheme
    check, add: if `parsedURL.Hostname()` parses as a literal IP,
    reject it via `safehttp.IsHostPubliclyRoutable(host string) bool`
    (implementable as
    `if ip := net.ParseIP(host); ip != nil { return
    isPubliclyRoutable(ip) }; return true` since real hostnames are
    only resolvable at dial time, when the dialer hook re-checks).
  - When `f.client.Do` returns an error, map
    `safehttp.IsSSRFBlockError(err)` to a new `FetchError` kind
    `FetchErrorInvalidURL` (so handlers map it to
    `CodeInvalidArgument`). Update `connectErrorFromFetchError` in
    `server/services/gear/detect_url.go` (and the equivalent in
    `experience`) — verify they already route `FetchErrorInvalidURL`
    to `CodeInvalidArgument`; they do today.
- [ ] Update `server/webfetch/fetcher_test.go` (verify file exists; if
  not, add):
  - Spin up an `httptest` server bound to `127.0.0.1`, point
    `FetchPageContent` at it, and assert the returned error is a
    `FetchError` with kind `FetchErrorInvalidURL`.
  - Add a second `httptest` server that returns a redirect to a
    `127.0.0.1` URL; assert the redirect is rejected with the same
    error kind.
  - Add a third test for direct IP-literal URL (e.g.
    `http://10.0.0.1/`) confirming the up-front check fires before
    any dial.
  - The tests should build a `HTTPFetcher` via a test-only helper
    that opts out of the dialer guard, then exercise the
    `httptest.Server` URL the normal way. Use the same
    `outboundClient`-injection pattern the media tests use.

**Validates:** Six RPCs and the backfill job stop fetching internal
IPs through page-mode generation. Tests prove the new error kind
flows through.

### Phase 4: Harden `experience.downloadAndStoreWebpageImage`

**Goal:** og:image follows are SSRF-safe; bytes-for-bytes
exfiltration channel closed.

**Tasks:**

- [ ] Update `server/services/experience/webpage_image.go`:
  - Add an early `url.Parse` of `imageURL` and reject schemes other
    than `http` / `https`.
  - Reject literal IP hosts in non-public ranges up front via
    `safehttp.IsHostPubliclyRoutable`.
  - Replace the ad-hoc `&http.Client{Timeout: imageDownloadTimeout}`
    with `safehttp.NewClient(safehttp.WithTimeout
    (imageDownloadTimeout), safehttp.WithRedirectSchemes("http",
    "https"))`.
  - All logging continues via `logging.LoggerWithContext(ctx)` (the
    existing logger in the function) — when the underlying client
    error matches `safehttp.IsSSRFBlockError`, log at `Warn` with the
    standard field naming (`operation=downloadAndStoreWebpageImage`,
    `image_url=<masked>`, `error=<err>`, `reason="ssrf_blocked"`).
    Avoid logging the rejected target IP at INFO+ — keep it at DEBUG
    or omit, so a malicious caller cannot use the log to scan our
    egress topology.
  - Keep the existing non-fatal contract: callers (`gen_ai.go:209`
    and `gen_ai_streaming.go:782`) already treat a non-nil return
    error as "no webpage image, continue without it." That behavior
    is documented in the function comment and the SSRF reject simply
    becomes another reason the image is skipped. This is **not** a
    violation of CLAUDE.md's "Fail Fast" rule — the function's
    documented contract is "log and skip on any download failure,"
    which is the right tradeoff for an optional image (failing the
    whole GenExperience flow because an attacker pointed an og:image
    at 127.0.0.1 would be worse UX).
- [ ] Add tests in
  `server/services/experience/webpage_image_test.go` (verify whether
  the file exists; if not, create it):
  - Stand up an `httptest` server on 127.0.0.1, invoke
    `downloadAndStoreWebpageImage` with that URL, assert it returns
    an error and writes nothing to storage. Use the existing test
    storage harness (`storage.SetupTestStorage` if available, or the
    in-package fake).
  - Add a test for the redirect-to-internal case (public host that
    redirects to 10/8).
  - Add a test for a non-http(s) scheme (e.g. `file:///etc/passwd`).
  - In each case, assert no media row is created.
- [ ] Audit the two call sites of `downloadAndStoreWebpageImage`
  (`gen_ai.go:209` and `gen_ai_streaming.go:782`) — confirm the
  callers already treat a non-nil error as non-fatal (`logger.Warn`,
  continue without an image). They do; no change needed there.

**Validates:** Scenario B in the issue (full-body exfiltration through
og:image) is closed. The negative tests will fail on `main` and pass
on the branch.

### Phase 5: Cleanup + docs

**Goal:** Catch any straggler `http.Client{}` construction and
document the policy.

**Tasks:**

- [ ] `grep -rn "http.Client{" server/` and triage each hit. Most are
  trusted-egress (Mapbox, Pexels / Pixabay / Unsplash provider
  fetches, Gemini API, GitHub App auth) — those can keep their
  default transports because the destination URL is hard-coded /
  signed-by-us, not user-controlled. The point of `safehttp` is to
  guard **user-supplied** URLs; trusted egress to known providers
  doesn't need it. Add a one-line comment near the safehttp call
  sites in `webfetch`, `webpage_image.go`, and `media/service.go`
  pointing future readers at `server/safehttp/`.
- [ ] Update `server/webfetch/README.md` and add a `server/safehttp/
  README.md` covering the package's role.
- [ ] Add a one-paragraph entry to `CLAUDE.md` under a new
  "Security: outbound HTTP" subsection, pointing future contributors
  to `server/safehttp/` for any code that fetches a URL whose host is
  even partly attacker-controlled.
- [ ] Confirm `npm run build` passes and `go test -tags=integration
  -timeout 20m ./...` is clean.

**Validates:** Future server contributions land on the safe default.

## Related Issues

- #1529 — Phase-1 log-only access policy in `media.GetMedia`. The
  read-side gap is what makes the stored og:image bytes retrievable
  end-to-end today. This issue closes the write-side; the Phase-2
  enforcement on the read side remains scoped to #1529.

## Audit notes

- The plan was audited against `docs/server/architecture.md` and
  `docs/server/observability.md`. The proposed `server/safehttp/`
  package matches the existing shared-library convention
  (`server/connecterr/`, `server/logging/`, `server/auth/`,
  `server/email/`, etc.): a small, focused package with a clear
  single responsibility, no service dependencies, used by multiple
  services through dependency injection.
- Logging follows the structured-log conventions
  (`logging.LoggerWithContext(ctx)`, standard field names
  `operation` / `error`, no PII, no `log.Printf`).
- The "non-fatal og:image" pattern is preserved intentionally; it is
  not a "warn-and-continue" anti-pattern (CLAUDE.md), because the
  function's documented contract is "image is optional; skip on any
  failure." The SSRF reject becomes one more reason to skip.
- Tests run against `httptest` servers bound to 127.0.0.1 with an
  unexported test seam that swaps in a non-safe client, mirroring
  the existing `outboundClient` injection in `media/service.go`. No
  network is required to run the test suite.
