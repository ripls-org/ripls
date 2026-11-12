# Issue #1002: [Bug Report] cannot add gear from URL

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1002
**Filed:** 2026-04-02 | **Last activity:** 2026-04-03
**Labels:** bug, user-feedback | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

Users attempting to add gear by pasting a product URL see a silent failure with
no error message. The root cause is HTTP 403 responses from retailer websites
blocking the bot's requests. The current User-Agent (`RiplsBot/1.0`) is easily
identified and blocked by anti-bot protections. Additionally, even when the
server does return an error, the client displays a raw, unhelpful error string
rather than a user-friendly message explaining what happened.

There are two distinct problems:
1. **Low success rate**: Many retailer/manufacturer websites block the fetcher
2. **Poor error UX**: When fetching fails, the user sees no message (or a
   cryptic one)

## Context from Comments

- **maintainer (maintainer)**: Confirmed 403 errors in production logs.
  Directed three work items: (1) informative error messages about
  robots/blocking, (2) research best practices for the fetcher UA and headers,
  (3) integration tests against real retailer URLs to measure success rates.
- **tescobar (maintainer)**: Suggested considering disabling the feature
  entirely due to high failure rate, while acknowledging its power. Reinforced
  that silent failure is unacceptable regardless.

## Potential Solutions

### Option A: Improve User-Agent and headers, add error UX

- **Approach:** Switch the User-Agent to a standard browser-like UA string
  (while keeping the bot info page URL). Add `Accept-Encoding`, `Referer`, and
  other standard browser headers. Improve error messages to be human-readable.
  Add integration tests against real retailer URLs.
- **Pros:** Minimal code changes. Likely to improve success rate significantly
  since many sites only do UA-based blocking. Honest identification. No
  infrastructure cost.
- **Cons:** Some sites with aggressive bot detection (Cloudflare, Akamai) will
  still block. May not be sufficient for Amazon, which has very aggressive
  anti-bot.
- **Effort:** Small.

### Option B: Headless browser rendering

- **Approach:** Use a headless Chrome/Chromium instance (via a library like
  chromedp or rod) to render pages with full JavaScript execution before
  extracting content.
- **Pros:** Handles JavaScript-rendered pages and many anti-bot protections.
  Highest success rate.
- **Cons:** Significant infrastructure complexity. Requires Chrome binary in
  Docker images. Higher resource usage, memory, and latency. Overkill for the
  current feature scope.
- **Effort:** Large.

### Option C: Retailer API integrations

- **Approach:** For the highest-value retailers (REI, Amazon, Backcountry),
  integrate with their product APIs or affiliate programs instead of scraping.
- **Pros:** Reliable, structured data. No anti-bot concerns. Richer product
  information.
- **Cons:** Each integration is custom work. API access often requires affiliate
  agreements. Limited to supported retailers. High ongoing maintenance.
- **Effort:** Large (per retailer).

## Recommendation

**Option A** is the right starting point. The fetcher currently uses a
bot-identifying UA that is trivially blocked. Switching to a browser-like UA
with proper headers will unlock most sites that use basic UA filtering. This
should be combined with clear, user-friendly error messages and an integration
test suite to measure and track success rates across major retailers.

If Option A proves insufficient for critical retailers after measuring, Option C
can be pursued selectively for the highest-value targets (REI, Backcountry)
in a follow-up issue.

## Open Questions

- **Should the bot info page URL be kept in the UA?** Recommendation: No.
  Use a standard browser UA. The current `RiplsBot/1.0` string is the primary
  reason sites block us. A bot info page is polite but counterproductive when
  the goal is to extract a single page of product info for a user's personal
  use. This is closer to a user action than a crawl.
- **Should we add retry logic for transient failures (5xx, timeouts)?**
  Recommendation: Not in this issue. A single retry with backoff could help but
  adds complexity. If integration tests show significant transient failures,
  address in a follow-up.
- **What user-facing language for blocked sites?** Recommendation: Something
  like "We couldn't load product details from this website. Try entering the
  item details manually instead." Avoid technical jargon about robots.txt or
  HTTP status codes.

## Implementation Plan

### Phase 1: Error UX improvements
**Goal:** Users always see a clear, actionable error when URL fetching fails.
**Tasks:**
- [x] Update `FetchPageContent` to return typed `*FetchError` with `Kind` field (Blocked, NotFound, Timeout, Connection, BadStatus, InvalidURL, Parse)
- [x] Update `genGearFromURL` and `DetectGearFromURL` to use `connectErrorFromFetchError()` mapping each `FetchErrorKind` to a distinct Connect error code
- [x] Add localized client-side error strings in ARB files (`app_en.arb`, `app_es.arb`) under the `gearGenError*` prefix (urlBlocked, urlNotFound, urlTimeout, urlUnavailable, urlInvalid, generationFailed, uploadFailed, saveFailed)
- [x] Add `GenGearError` enum to viewmodel; widget resolves to `context.l10n.gearGenError*` strings
- [x] Update fetcher tests to verify typed error kinds on each error path
**Validates:** Users always see helpful, localized error messages when URL fetching fails.

> **Audit note (i18n compliance):** The original plan embedded user-facing English strings in server error messages. Per `docs/client/i18n.md`, all user-visible strings must use `context.l10n.*` via ARB files, and viewmodels must not resolve strings — they expose typed state (error codes/enums) that widgets resolve in `build()`. Server errors should remain technical for logging; the client maps Connect error codes to localized messages.

### Phase 2: Fetcher improvements
**Goal:** Maximize the success rate of URL fetching against major retailer sites.
**Tasks:**
- [x] Change `DefaultUserAgent` to Chrome 131 browser UA string
- [x] Add browser headers: `Sec-Fetch-Dest`, `Sec-Fetch-Mode`, `Sec-Fetch-Site`, `Sec-Fetch-User`, `Upgrade-Insecure-Requests`, improved `Accept`
- [x] Let Go handle `Accept-Encoding` automatically (no explicit header = automatic gzip decompression by `http.Transport`)
- [x] Enable cookie jar (`net/http/cookiejar`) for sites that require cookies
- [x] Go's default redirect policy already limits to 10 redirects — no change needed
- [x] Add `TestHTTPFetcher_BrowserHeaders` verifying all new headers and browser-like UA
- [x] Update webpage image downloader to use `webfetch.DefaultUserAgent` instead of `RiplsBot/1.0`
**Validates:** Fetcher sends requests that look like a standard browser.

### Phase 3: Integration test suite
**Goal:** Measure and track success rates across major retailer sites, gated behind the `integration` build tag.
**Tasks:**
- [x] Create `server/webfetch/fetcher_integration_test.go` with `//go:build integration`
- [x] Add test cases for 19 major retailers and manufacturers
- [x] Each test fetches a real URL, reports success/failure with content summary
- [x] Summary test reports overall success rate, fails below 25% threshold
- [x] Baseline results documented (2026-04-03): **10/19 pass (53%)**
  - **Working:** Patagonia, Thule, Makita, Weber, Dyson, Osprey, Evo, Walmart, eBay, Craigslist
  - **Blocked (403):** REI, Moosejaw, Home Depot, Lowe's, Coleman (Cloudflare/Akamai)
  - **404 / stale URLs:** Amazon, Target, DeWalt
  - **Bot challenge (202):** Backcountry
**Validates:** We know exactly which sites work and which don't, and can track improvements over time.

## Related Issues
- No directly related issues found.
