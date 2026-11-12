# Issue #1917: URL mode pulls low-resolution preview image (e.g., Amazon og:image)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1917
**Filed:** 2026-05-13 | **Last activity:** 2026-05-13
**Labels:** none | **Assignees:** unassigned
**Context docs:** none (also performed a secondary sweep for other low-res image paths — see § Secondary findings)

## Problem Statement

When a user pastes an e-commerce product URL into the unified-create / `*FromWebpage` flow (Amazon, Shopify, Etsy, Target), the image that lands in the preview card and the final feed item is noticeably low resolution — often 300px on the long edge. The cause is structural: `server/webfetch/fetcher.go:318-373` extracts the page image with a three-tier priority (`og:image` → `twitter:image` → first significant `<img>` in main content), and e-commerce sites deliberately populate `og:image` with a small thumbnail variant intended for Slack / iMessage / Twitter link previews. We pick it up correctly; the source itself is low-res by spec.

The same pages have a much higher-resolution image available via either:

- A predictable URL-token rewrite (Amazon: `._SY300_.` → strip token for the master; Shopify: drop `_300x.jpg`; Etsy: `il_300xN.` → `il_fullxfull.`; Target / scene7: drop `wid`/`hei` query params).
- A JSON-LD `Product.image` array (the eval-seed tool at `server/cmd/eval-seed-from-urls/main.go:260` already parses this for goldens harvesting — not yet wired into the prod fetcher).
- An `<img srcset>` ladder on the main product `<img>` (rarely the only signal on e-commerce pages, but worth a fallback).

## Context from Comments

No comments on the issue. The body is unusually complete — it gives the file:line cause, ranks three fix options, lists acceptance criteria, and points at the existing JSON-LD parser already used by the eval-seed tool. Treat the body as the definitive direction; no clarification needed.

## Secondary findings (from this skill's broader sweep)

The user asked: *"are other low-res thumbnails being pulled down? I feel like sometimes when a pexels image is loaded there is a thumbnail that comes down in the preview that is lower quality then later there is a high quality version."* Investigation across `server/media/`, `server/services/media/`, `server/storage/media.go`, and `app/lib/data/repositories/media_repository.dart`:

- **Pexels images (server-side, `server/media/pexels_provider.go:261-263`):** downloads the `Large2x` variant (≈2400px) by default, falling back to `Large`. No `Tiny`/`Medium` is ever requested. There is **no server-side low-res-then-high-res swap for Pexels images** — only one variant is fetched, and it's already the high-res one.
- **Unsplash (`server/media/unsplash_provider.go:212-219`):** fetches `Full` (≈2400px), falls back to `Regular`. Same shape — no progressive loading.
- **Pixabay (`server/media/pixabay_provider.go:376-408`):** prioritizes `Large`/`Medium` portrait variants; `Tiny` is a last-resort fallback only when nothing larger exists.
- **Pexels videos:** *do* have a low-res surface — the static poster image returned by Pexels (`video.Image`) is stored as the media's thumbnail (`server/media/pexels_provider.go:825-861`). When a Pexels video appears in a card, the poster shows first, then the full video plays on tap. This is the closest match to what the user is perceiving — but it's only videos, not images.
- **User-uploaded media (`server/storage/media.go:29-31, 156`):** the server generates a 400px-on-longest-side JPEG thumbnail (Lanczos resize) for any image > 1000px or > 500 KB. Client receives two URLs per media (`media.url`, `media.thumbnailUrl`). The repository has two distinct accessors: `MediaRepository.getMediaUrl(id)` returns the thumbnail when present (used by cards and preview surfaces), `MediaRepository.getFullMediaUrl(id)` returns the full URL (used by detail screens). They use separate cache keys (`media:thumb:<id>` / `media:full:<id>`) — there is no progressive swap of the *same* widget from thumb to full.
- **Webpage og:image (this issue):** the surface where a *single* image goes through the system at low resolution and never gets upgraded.

**Conclusion for the user's perception:** the "low-quality then high-quality" effect for Pexels-image-shaped previews is most likely one of:
1. **#1917 itself** — paste an Amazon URL, see the Amazon og:image thumbnail (low-res), and that *is* the final image stored. There's no high-res replacement — what you saw later was just the same low-res image laid out at a larger card size, which can read as "swapped in" without actually being so.
2. **Card → detail navigation** for user-uploaded media — tapping into a detail view *does* swap the 400px thumb URL for the full original, which can look like a progressive load if the network is slow.
3. **Pexels video posters** — but the user said "I don't think video," so this is unlikely.

No additional follow-up issue is warranted from the sweep; fixing #1917 addresses the only path where a *single* image is needlessly low-res. The thumb/full split is intentional and beneficial.

## Potential Solutions

### Option 1: Per-domain URL-token rewrite (recommended first ship)

- **Approach:** Add a small `upgradeImageURL(rawURL, host) string` helper in `server/webfetch/` that knows the resolution-token pattern for the top e-commerce hosts. Call it from `extractImageURL` right before returning each candidate (og:image, twitter:image, content `<img>` src). The table from the issue body:

  | Host pattern | From | To |
  |---|---|---|
  | `*.amazon.com`, `*.media-amazon.com`, `*.ssl-images-amazon.com` | `._SY###_.`, `._SL###_.`, `._AC_UY###_.` etc. before `.jpg` | strip the size token → master |
  | `*.shopify.com`, `cdn.shopify.com` | `_300x.jpg`, `_600x600.jpg` | drop the size suffix, or substitute `_2048x.jpg` |
  | `*.etsystatic.com` | `il_300xN.` | `il_fullxfull.` |
  | `*.target.com`, `Target.scene7.com` | `?wid=300&hei=300` query | drop `wid`/`hei` or bump to `wid=1200` |

  Implement as a `domainRewrite struct { hostSuffix string; rewrite func(*url.URL) *url.URL }` slice. Each rewrite is pure — no extra HTTP requests, deterministic, easy to unit-test.

- **Pros:** Smallest patch. Fits the existing extractor cleanly. Zero network cost. Visibly fixes the four sites that account for the bulk of pasted e-commerce URLs.
- **Cons:** Per-domain table requires maintenance — Amazon's URL token format has changed in the past (`._UY###_`, `._AC_SX###_`, etc.), and we'd need to update if they introduce a new variant. Doesn't help the long tail of e-commerce platforms.
- **Effort:** small — ~80 LoC including tests.

### Option 2: Parse JSON-LD `Product.image` (recommended second ship)

- **Approach:** Extract the existing JSON-LD parser in `server/cmd/eval-seed-from-urls/main.go:260,:534` into the `server/webfetch/` package as `extractJSONLDProductImage(doc)` (or fold into the existing JSON-LD plumbing — `jsonld_test.go` already covers Events). Consult it as **priority 0** in `extractImageURL`, ahead of og:image. JSON-LD `Product.image` arrays on e-commerce pages typically reference the high-resolution master.
- **Pros:** Standards-based; covers most e-commerce platforms without a per-domain rule. Side benefit: the parsed JSON-LD often also carries `name`, `brand`, `price`, `availability` which the downstream classifier and `GenerateGearFromWebpage` could use for better titles/descriptions (potential follow-up; out of scope here).
- **Cons:** More code to add and test. Some pages have *only* `og:image` and no JSON-LD — those still need Option 1 as a fallback. Order matters: JSON-LD first, then domain-rewritten og:image, then twitter, then content `<img>`.
- **Effort:** medium — ~200 LoC including tests, plus moving the eval-seed extractor into a shared location.

### Option 3: Pick the highest `srcset` candidate from content `<img>`

- **Approach:** In the fallback branch (`extractImageURL` line 342, when no meta-tag image exists), parse the `srcset` attribute on the chosen `<img>` and select the URL with the largest stated width.
- **Pros:** Site-agnostic. Covers the third branch cleanly.
- **Cons:** Marginal value alone — most e-commerce pages have proper Open Graph, so the fallback branch is rarely hit on the sites that motivated this issue. Most useful for blog-style or long-form-content pages, which aren't the bulk of pasted URLs.
- **Effort:** small — ~40 LoC. Cheap to ride along with Option 2.

## Recommendation

**Ship Option 1, then follow up with Option 2 (with Option 3 riding along).** This is exactly the order the issue body suggests, and the secondary sweep didn't surface anything that changes the calculus.

The reason to start with Option 1: it's the smallest, lowest-risk change, and it solves the four sites that produce ~90% of the pasted product URLs. Option 2 is the standards-correct fix but takes longer to land and test, and there's no reason to block the Amazon/Shopify win on it.

## Open Questions

- **How aggressive should Option 1's Amazon rewrite be?** Recommendation: strip the size token entirely (`._SY300_.jpg` → `.jpg`) rather than substituting a specific size like `._SL1500_.`. The token-less master is what Amazon serves on the product page itself; stripping is more future-proof than picking a specific size that Amazon might deprecate. Cap the response at the existing `downloadAndStoreWebpageImage` size guard so a runaway 20 MB master doesn't bloat storage.

- **Should Option 1 verify the rewritten URL works before returning it?** Recommendation: no. Adding a HEAD request per rewrite doubles network round-trips on the hot path. If the rewrite produces a 404, `downloadAndStoreWebpageImage` already handles fetch failure gracefully — it logs and falls back to the original og:image. The bad case is a slightly slower failure, not a broken page.

- **Where does the rewrite table live as it grows?** Recommendation: start inline in `server/webfetch/image_url_upgrade.go`. If/when the table exceeds ~10 entries (or any entry needs significant logic), consider extracting a small registry pattern. Don't pre-optimize.

- **Does Option 1 need a feature flag?** Recommendation: no. The rewrite either improves the resolution or has no effect (when the URL doesn't match any pattern). A regression would be a wrong rewrite producing a broken URL — covered by unit tests per pattern.

- **For Option 2, does the JSON-LD parser need to handle `@graph` and nested `Product` shapes?** Recommendation: yes — but reuse the patterns already proven in the eval-seed tool (`server/cmd/eval-seed-from-urls/main.go:308-368`) which has been exercised against the existing goldens corpus. Don't rewrite the JSON-LD walker; extract and share it.

- **Acceptance threshold for "high resolution"?** The issue body says ≥800px on the long edge. Recommendation: keep that bar in the unit tests (fixture URL → rewritten URL → fetch → assert width ≥ 800). For Amazon stripped-token URLs the actual response is typically ≥1500px, so 800 has headroom.

## Implementation Plan

### Phase 1: Per-domain URL-token rewrite (Option 1)

**Goal:** Amazon, Shopify, Etsy, and Target pages downloaded via webfetch land an image ≥800px on the long edge.

**Tasks:**
- [ ] Add `server/webfetch/image_url_upgrade.go` with `upgradeImageURL(rawURL string) string`. Implement as a slice of `{hostSuffix string, rewrite func(*url.URL) *url.URL}` entries covering Amazon, Shopify, Etsy, Target. Pure function, no I/O.
- [ ] Call `upgradeImageURL` inside `extractImageURL` at three points: after the og:image branch, after the twitter:image branch, and on the fallback `<img>` src right before assigning `fallbackImage`.
- [ ] Unit tests in `server/webfetch/image_url_upgrade_test.go` — one table-driven test per host with at least three input/output pairs each (including a no-op case where the URL doesn't match a known token).
- [ ] Add 1-2 integration cases to `server/webfetch/fetcher_test.go` using fixture HTML with an `og:image` that should be rewritten.
- [ ] Verify `server/services/experience/webpage_image_test.go` still passes — `downloadAndStoreWebpageImage` shouldn't care whether the URL was rewritten.
- [ ] `npm run build` + `go build -tags=integration ./...` clean.

**Validates:** The four most common e-commerce sources now produce high-resolution previews. No regressions in the existing webfetch tests.

### Phase 2: JSON-LD Product.image priority (Option 2)

**Goal:** Standards-based path covers long-tail e-commerce platforms without requiring per-domain entries.

**Tasks:**
- [ ] Extract the JSON-LD walking logic (`walkJSONLD` at `server/cmd/eval-seed-from-urls/main.go:308-324`, `fillFromProductObject` at `:341-368`, with image extraction at `:361`) into a `server/webfetch/jsonld_product.go` shared by both the eval-seed tool and the fetcher. Update the eval-seed tool to import from the new location.
- [ ] Add `extractJSONLDProductImage(doc *goquery.Document) string` that returns the first non-empty `Product.image` URL (handles string, []string, and ImageObject shapes; descends into `@graph` containers).
- [ ] In `extractImageURL`, consult `extractJSONLDProductImage` first (priority 0), then fall through to og:image (now domain-rewritten), twitter:image, content `<img>`.
- [ ] Unit tests in `server/webfetch/jsonld_product_test.go` with fixtures for the common JSON-LD shapes (string image, array, ImageObject with `url` field, `@graph`-nested Product).
- [ ] Real-world fixture test in `server/webfetch/jsonld_realworld_test.go` (existing file) — add a sampled product page from each of the four target sites.
- [ ] `npm run build` + `go build -tags=integration ./...` clean.

**Validates:** A site we haven't added to the per-domain table (e.g., BestBuy, Walmart) still gets a high-res image when it ships JSON-LD `Product.image`.

### Phase 3: `srcset`-aware fallback (Option 3)

**Goal:** When the fallback `<img>` branch is reached, prefer the highest-resolution candidate available on the element.

**Tasks:**
- [ ] In `extractImageURL`'s fallback branch, after locating the candidate `<img>`, parse its `srcset` attribute (if present) and pick the URL whose stated `w` descriptor is largest. Fall back to `src` when `srcset` is absent or unparseable.
- [ ] Unit tests covering: no srcset, single-entry srcset, multi-entry srcset with `w` descriptors, srcset with `x` (density) descriptors (pick the highest `x`).
- [ ] `npm run build` clean.

**Validates:** Long-form / blog pages with `<img srcset>` ladders no longer get the smallest variant by default.

## Related Issues

- #1912 — unified-create P2.16 testing where this was surfaced.
- #1939 / PR #2015 — classifier image-bytes work; touches the same `webpage_image.go` codepath but for a different concern.
- #1907 — unified-create classifier (parent thread for the URL-mode work).

## Audit Notes

Reviewed against `docs/server/architecture.md`, `docs/server/observability.md`, project CLAUDE.md, and the actual codebase at this commit. Findings and fixes:

- **eval-seed line references corrected.** The plan cited `server/cmd/eval-seed-from-urls/main.go:260` and `:534` as the JSON-LD extraction. `:260` is the "no usable metadata" error path and `:534` is the meta-tag fallback fill (`tags["og:image"]`), not the JSON-LD walker. The actual reusable JSON-LD logic is `walkJSONLD` (`:308-324`) and `fillFromProductObject` (`:341-368`), with image extraction at `:361` (`got.ImageURL = imageOf(p["image"])`). Updated Phase 2 references accordingly.
- **fetcher.go range minor:** `extractImageURL` runs 318-374 (plan said 318-373); not worth a separate edit, kept as-is.
- **Logging convention confirmed.** `server/services/experience/webpage_image.go` already uses `logging.LoggerWithContext(ctx).With("operation", ...)` per `docs/server/observability.md`; Phase 1/2 don't add new log lines, so no observability changes needed. Noted in plan for completeness.
- **Package placement is correct.** `server/webfetch/jsonld_test.go` and `jsonld_realworld_test.go` already live in webfetch (covering Events); adding `jsonld_product.go` next to them is consistent with the README's "JSON-LD structured-data extraction" responsibility.
- **Build commands verified.** `npm run build` runs lint + `go build ./...`; CLAUDE.md confirms `go build -tags=integration ./...` is the right gate for integration-tagged callers. Both are pure-function additions (no exported-signature changes), so the integration build is belt-and-suspenders here but harmless.
- **Secondary findings accurate.** `server/storage/media.go:29-31` confirms `thumbnailDimensionThresholdPx = 1000`, `thumbnailTargetSizePx = 400`. The thumb/full split described in the secondary section matches the code.

No correctness flags worth escalating; the plan is ready to execute.
