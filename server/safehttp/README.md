# safehttp

This package provides an SSRF-guarded HTTP client builder for use with
user-supplied URLs.

## Why this exists

Any code path that accepts a URL from an untrusted caller (user input, an
`og:image` meta tag, a gear source URL) must use a client that rejects
requests to internal infrastructure. Without this guard, an attacker can
direct the server to fetch `http://169.254.169.254/` (cloud metadata),
internal services, or RFC1918 addresses, then exfiltrate the response body
through a public API.

`safehttp.NewClient` closes the attack surface in three layers:

1. **Dialer Control hook** — runs after DNS resolution and rejects loopback,
   link-local (incl. cloud metadata at 169.254.169.254), broadcast, multicast,
   unspecified, RFC1918, CGNAT (100.64/10), and IPv6 ULAs. DNS rebinding is
   caught here because the resolved IP at connect time is what matters.
2. **CheckRedirect** — re-asserts the allowed scheme on every redirect hop, so
   a server cannot issue an `http://` or `file://` redirect to escape a
   scheme-enforcing input check.
3. **Timeout** — caps the per-request lifetime so a slow server cannot hold a
   goroutine open.

## Key files

- `client.go` — `NewClient`, `IsSSRFBlockError`, `IsHostPubliclyRoutable`,
  and the `WithTimeout`, `WithRedirectSchemes`, `WithCookieJar`,
  `WithMaxRedirects` options.
- `client_test.go` — unit tests for IP-range classification, dial control,
  and redirect-scheme enforcement.

## When to import this package

Import `safehttp` whenever a code path fetches a URL whose host is even
partly attacker-controlled:

- User-submitted gear source URLs (webfetch paths)
- `og:image` URLs extracted from fetched pages
- Generic-URL media imports (AddMediaFromURL)

For trusted egress to known third-party endpoints (APIs with hard-coded
base URLs), use the standard `http.Client` directly — the dialer guard
adds overhead and can cause false positives if the provider ever moves to
a CDN with addresses in reserved ranges.

## When to add code here vs. elsewhere

- New SSRF guard option (e.g., custom IP allow-list)? **Here.**
- URL validation before any HTTP request (scheme check, IP-literal
  pre-check)? **In the caller** — before the dial, using
  `IsHostPubliclyRoutable` for IP literals.
- Logging of SSRF rejections? **In the caller** — use
  `IsSSRFBlockError(err)` to detect rejections and log at `Warn`.
- Generic outbound HTTP helpers unrelated to SSRF? **`server/webfetch`**
  (page fetching) or the calling package.
