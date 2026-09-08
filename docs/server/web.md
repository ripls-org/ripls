---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Generating gear and experiences from product/event page URLs — the webfetch HTTPFetcher (goquery HTML cleaning, og-tag extraction, safety limits), the AI extraction layer, product spec lookup, og-image download, and source-url attribution.
  globs: [server/webfetch/**, server/product/**, server/services/gear/**, server/services/experience/**]
  triggers: [web-extraction, url, webpage, goquery, og-image, scraping, gen-gear, gen-experience, source-url]
  lens: [server, domain]
  domain: server
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Web Content Extraction

## Overview

The server can generate gear items and experiences from product or event page URLs. When a user provides a URL (e.g., an Amazon product page or Eventbrite event), the server fetches the webpage, extracts structured content, and uses AI to identify product details, pricing, event dates, and other metadata. This powers both the creation flow (pasting a URL to create a new item) and the auto-fill flow (updating an existing item's metadata from a product page).

## Architecture

The web extraction pipeline has three layers:

1. **Web Fetcher** — HTTP client that retrieves webpages and extracts structured text content from HTML
2. **AI Provider** — LLM that analyzes the extracted content and returns structured product/event data
3. **Service Layer** — RPC handlers that orchestrate fetching, AI extraction, image download, and response assembly

```
Client sends URL
    ↓
[Service] Extract/validate URL
    ↓
[HTTPFetcher] GET webpage → parse HTML via goquery
    ↓
Extract: title, description, body text (cleaned), og:image URL
    ↓
[AI Provider] GenerateGearFromWebpage / GenerateExperienceFromWebpage
    ↓
Structured output: item details, pricing/value, confidence
    ↓
[Service] Download og:image as media (optional, fallback to stock)
    ↓
Return DetectedGearItem / GenExperienceResponse with source_url
    ↓
[Client] Preview → user confirms → Save RPC persists with source_url
```

## Web Fetcher

The `webfetch.Fetcher` interface ([server/webfetch/fetcher.go](../../server/webfetch/fetcher.go)) handles HTTP fetching and HTML cleaning. The concrete `HTTPFetcher` implementation uses `net/http` and `goquery` for HTML parsing.

### PageContent

`FetchPageContent(ctx, url)` returns a `PageContent` struct:

| Field         | Description                                              |
|---------------|----------------------------------------------------------|
| `URL`         | Canonical URL after redirects                            |
| `Title`       | `og:title` → `<title>` fallback                         |
| `Description` | `og:description` → `meta[name=description]` fallback    |
| `BodyText`    | Cleaned body text (HTML stripped, truncated for LLM)     |
| `ImageURL`    | Primary image: `og:image` → `twitter:image` → first content `<img>` |
| `ImageURLs`   | Priority-ordered image candidates (primary + alternates), deduped |
| `Event`       | `*StructuredEventData` from schema.org JSON-LD (nil if absent) — authoritative event date/time/location, preferred over LLM inference in the experience flow |

### Content Extraction

**Title priority**: `og:title` > `<title>` tag

**Description priority**: `og:description` > `meta[name=description]`

**Body text cleaning**:
- Removes: `<script>`, `<style>`, `<noscript>`, `<iframe>`, `<nav>`, `<footer>`, `<header>`, `<aside>`, `[role=navigation]`, `[role=banner]`, `[aria-hidden=true]`
- Prefers content from: `<main>`, `<article>`, `[role=main]`, `.content`, `.event-details`, `.event-description`, `.event-info`
- Falls back to `<body>` text if no main content area found
- Normalizes whitespace (collapses runs of spaces/tabs/newlines)
- Truncated to 8,000 characters for LLM context window

**Image priority**: `og:image` > `twitter:image` > first significant `<img>` in main content area. Skips data URIs and images with explicit dimensions < 100px (likely icons/spacers). Resolves relative URLs against the page base URL.

### Safety Limits

| Limit                | Default   | Purpose                           |
|----------------------|-----------|-----------------------------------|
| HTTP timeout         | 30s       | Prevent hanging on slow sites     |
| Max body size        | 5 MB      | Prevent memory abuse              |
| Max body text length | 8,000 ch  | LLM context window budget         |
| User-Agent           | A desktop-Chrome UA string (`DefaultUserAgent`) | Avoid bot-blocking on product/event pages |

All limits are configurable via `Option` functions: `WithTimeout()`, `WithMaxBodySize()`, `WithMaxBodyTextLength()`, `WithUserAgent()`.

## Gear from URL

### RPC: GenGear

The `GenGear` RPC ([server/services/gear/gen_ai.go](../../server/services/gear/gen_ai.go)) supports three generation modes: text prompt, media (image), and URL. URL mode is triggered automatically when the `prompt` field is, in its entirety, an HTTP/HTTPS URL.

**URL detection**: `extractURL()` returns the URL only if the entire trimmed `prompt` is a single URL (matched against regex `https?://[^\s]+` and required to equal the whole trimmed text). A prompt with any non-URL content returns empty and routes to `genGearFromText()`; a bare URL routes to `genGearFromURL()`.

**Generation flow** (`genGearFromURL`):

1. Validate web fetcher is configured
2. Fetch webpage via `HTTPFetcher.FetchPageContent(ctx, url)`
3. Call `aiProvider.GenerateGearFromWebpage(ctx, title, description, body, region)` — returns structured `GearGeneration` with title, description, category, brand, material, weight, and value estimate
4. Sanitize AI response (clear placeholder values like `<UNKNOWN>`)
5. Get user's location fallback for geocoding
6. Download og:image as media via `downloadAndStoreWebpageImage()` (optional — logs warning on failure)
7. Fall back to stock imagery via `findAndStoreStockImage()` if no webpage image
8. Build `DetectedGearItem` response with `source_url` set to the extracted URL

**AI extraction** (via `GenerateGearFromWebpage` on the `ai.Provider` interface):

The LLM receives the page title, description, and body text, then extracts:

| Field             | Description                                                |
|-------------------|------------------------------------------------------------|
| Title             | Product name (e.g., "Ryobi 2300 PSI Pressure Washer")     |
| Description       | Brief product description                                  |
| Category          | Product category (e.g., "Power Tools")                     |
| Brand             | Brand name (e.g., "Ryobi")                                 |
| MaterialCategory  | Material/type classification                               |
| WeightGrams       | Estimated weight in grams                                  |
| ValueEstimate     | Extracted or estimated price with confidence and reasoning  |

**Value extraction** is a key feature — the prompt instructs the AI to:
- Look for listed price, "was" price, or MSRP on the page
- Return high confidence (0.9–1.0) when an exact price is visible
- Fall back to category-based estimates if no price found
- Never return 0 — always provide some estimate
- Include price source (e.g., "Amazon product listing") in the `Sources` field

### Product Lookup

The `product.Lookup` ([server/product/lookup.go](../../server/product/lookup.go)) provides a second path for web-based spec enrichment: given a brand and model identified by AI (from an image), it searches manufacturer and retailer websites to find and extract additional product details.

**Flow**:
1. AI detects a product from an image with brand/model and confidence ≥ 0.8
2. `LookupSpecs()` tries the manufacturer website first (if brand is recognized)
3. Falls back to retailer search (Google Shopping by default)
4. For each URL: fetches the page, runs `GenerateGearFromWebpage`, returns specs
5. `MergeGearGeneration()` combines AI detection results with fetched specs — fetched description and value take precedence if more detailed or higher confidence

**Manufacturer URL construction** ([server/product/search.go](../../server/product/search.go)):
- Maps ~20 known brands to manufacturer domains (DeWalt → dewalt.com, Ryobi → ryobitools.com, etc.)
- Uses site-specific Google search: `google.com/search?q=<brand+model>+site:<domain>`
- Falls back to Google Shopping: `google.com/search?q=<query>&tbm=shop`

**Timeout**: Product lookups have a 3-second default timeout to avoid blocking the user. If lookup times out, the system falls back to AI-only results.

## Experience from URL

### RPC: GenExperience

The `GenExperience` RPC ([server/services/experience/gen_ai.go](../../server/services/experience/gen_ai.go)) accepts a `website_url` field (via oneof with text/media_id) for creating experiences from event pages.

**Generation flow** (`generateFromWebpage`):

1. Validate web fetcher is configured
2. Fetch webpage via `HTTPFetcher.FetchPageContent(ctx, url)`
3. Call `aiProvider.GenerateExperienceFromWebpage(ctx, title, description, body, region, currentTime)`
4. Download og:image as experience media (optional)
5. Fall back to stock video or image if no webpage image
6. Return `GenExperienceResponse` with `source_url` set

**AI extraction** (via `GenerateExperienceFromWebpage`):

| Field           | Description                                              |
|-----------------|----------------------------------------------------------|
| Title           | Event name                                               |
| Description     | Event description                                        |
| Date            | Event date (YYYY-MM-DD)                                  |
| Time            | Event time (HH:MM)                                       |
| TimeConfidence  | EXPLICIT / INFERRED / UNKNOWN                            |
| LocationQuery   | Location string for geocoding                            |
| SearchKeywords  | Keywords for related content                             |
| ValueEstimate   | Per-person hosting value estimate with confidence         |

## Webpage Image Download

Both gear and experience services attempt to download the primary image from the webpage (typically the `og:image` URL) and store it as a media asset owned by the user. This happens in `downloadAndStoreWebpageImage()` in each service.

**Behavior**:
- Downloads the image from the extracted URL
- Stores it via the media storage system, associated with the requesting user
- Returns the media ID for inclusion in the response
- On failure: logs a warning and continues — image download is optional
- Fallback: if no webpage image, uses stock imagery search based on the generated title

## Source URL Attribution

All generation responses include a `source_url` field that tracks where the item information originated. This URL is:
- Returned to the client in `DetectedGearItem.source_url` or `GenExperienceResponse.source_url`
- Persisted via `SaveGearRequest.source_url` or `SaveExperienceRequest.source_url`
- Displayed in the product details modal as a clickable link to the original page
- Used for provenance tracking on value estimates (the `Provenance` message includes source attribution)

## Key Files

**Web Fetcher**:
- [server/webfetch/fetcher.go](../../server/webfetch/fetcher.go) — `Fetcher` interface, `HTTPFetcher` implementation, HTML parsing

**Gear URL Generation**:
- [server/services/gear/gen_ai.go](../../server/services/gear/gen_ai.go) — `GenGear` RPC, `genGearFromURL()`, URL detection
- [server/services/gear/webpage_image.go](../../server/services/gear/webpage_image.go) — Webpage image download
- [server/services/gear/stock_imagery.go](../../server/services/gear/stock_imagery.go) — Stock image fallback

**Experience URL Generation**:
- [server/services/experience/gen_ai.go](../../server/services/experience/gen_ai.go) — `GenExperience` RPC, `generateFromWebpage()`
- [server/services/experience/webpage_image.go](../../server/services/experience/webpage_image.go) — Webpage image download
- [server/services/experience/stock_imagery.go](../../server/services/experience/stock_imagery.go) — Stock media fallback

**Product Lookup**:
- [server/product/lookup.go](../../server/product/lookup.go) — `Lookup` struct, `LookupSpecs()`, `MergeGearGeneration()`
- [server/product/search.go](../../server/product/search.go) — Search URL construction, manufacturer domains

**AI Provider**:
- [server/ai/provider.go](../../server/ai/provider.go) — `GenerateGearFromWebpage`, `GenerateExperienceFromWebpage` interface methods
- [server/ai/provider_gemini.go](../../server/ai/provider_gemini.go) — Gemini implementation
- [server/ai/prompts.go](../../server/ai/prompts.go) — `buildGearFromWebpagePrompt()`, `buildExperienceFromWebpagePrompt()`

**Proto Definitions**:
- [proto/ripls/api/gear_service.proto](../../proto/ripls/api/gear_service.proto) — `GenGearRequest.prompt`, `DetectedGearItem.source_url`, `SaveGearRequest.source_url`
- [proto/ripls/api/experience_service.proto](../../proto/ripls/api/experience_service.proto) — `GenExperienceRequest.website_url`, `SaveExperienceRequest.source_url`

## Related Documentation

- [LLM Architecture](llm.md) — Provider interface, prompt engineering, structured output
- [Client Creation Flows](../client/create.md) — Client-side UX for AI-powered creation
