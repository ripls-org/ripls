# Webfetch

The `webfetch` package fetches and cleans web page content for downstream AI processing. It handles HTTP retrieval, cookie management, HTML cleaning (script/style stripping, boilerplate removal), and JSON-LD structured-data extraction. Callers pass the cleaned text to an AI provider; this package has no AI dependency itself.

## Key files

- `fetcher.go` — `Fetcher` interface and the production implementation (`HTTPFetcher`). Provides `FetchPageContent` which returns cleaned text and any extracted JSON-LD. Typed `FetchError` classifies failures (blocked, timeout, not found, etc.).
- `mock.go` — configurable mock `Fetcher` for tests.

## Security: SSRF guard

`NewHTTPFetcher` builds its client via `server/safehttp.NewClient`, which
rejects requests to private, loopback, link-local, CGNAT, and cloud-metadata
addresses at dial time (DNS-rebinding safe) and on every redirect hop. An
additional IP-literal pre-check in `FetchPageContent` fires before any dial
for URLs whose host is already a non-public IP literal (e.g.
`http://10.0.0.1/`).

Tests that must reach loopback `httptest` servers must use the
`newTestFetcher` helper defined in `testing_helpers_test.go` rather than
`NewHTTPFetcher`, so the SSRF guard remains unconditionally active in
production.

## When to add code here vs. elsewhere

HTML retrieval and cleaning belongs here. Parsing structured content from the
cleaned text belongs in `server/ai` (the AI provider decides how to interpret
it). Product spec retrieval that combines web fetching with AI extraction
belongs in `server/product`.
