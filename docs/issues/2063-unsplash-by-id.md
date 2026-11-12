# Issue #2063: Unsplash candidate imports skip the trusted provider path

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2063
**Filed:** 2026-05-20 | **Last activity:** 2026-05-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

PR #2062 wired a "trusted-provider" import path through `MediaService.AddMediaFromURL`: when a Replace Media candidate carries `provider + provider_photo_id`, the service routes to `importViaTrustedProvider` (`server/services/media/service.go:291`), which calls `stockImageryProvider.GetStockImageByID(...)` to look the photo up via the provider's API, then uses `CopyStockImageForUser` to clone the canonical bucket object into a per-user media row with `Attribution` populated.

This works for **Pexels** (`server/media/pexels_provider.go:1075` is a full implementation) but is a **stub for Unsplash** (`server/media/unsplash_provider.go:549-551`):

```go
func (p *UnsplashProvider) GetStockImageByID(_ context.Context, _ string) (*models.StockImage, error) {
    return nil, fmt.Errorf("unsplash by-id lookup not supported")
}
```

When a user taps an Unsplash candidate in the Replace Media row, the flow:

1. Hits `importViaTrustedProvider`.
2. Tries the `FallbackStockImageryProvider` chain (`stock_imagery_fallback.go:118`). Pexels doesn't recognise the Unsplash id and errors; Unsplash returns the stub error.
3. Falls back to the **generic-URL path** at `service.go:271-277`: server-side HTTPS fetch through the SSRF-guarded outbound client, 60 MB body cap, content-type sniff, `StoreMedia` with `sourceStockImageID = ""`.

Three concrete consequences:

- **No attribution recorded.** The resulting media row's `SourceStockImageId` is empty, so `GetMedia` won't populate `Attribution` for the photographer (whereas Pexels imports do, via `buildAttribution(stockImage)` in `service.go:503`). The image still renders, but the photographer credit silently disappears — which is an Unsplash API guideline violation.
- **60 MB body cap applies** to Unsplash CDN URLs even though we'd otherwise trust them.
- **Download path lacks Unsplash's mandatory download-tracking ping.** Unsplash's API guidelines require a `GET /photos/:id/download` call before downloading the image (already done by `UnsplashClient.triggerDownload` on the search path); the generic-URL fallback skips this.

The fix is "implement `GetStockImageByID` for Unsplash, mirroring the Pexels pattern."

## Context from Comments

No comments. This is a self-filed follow-up from the PR #2062 review checklist. The implementation pattern is unambiguous because Pexels already proves it; the planning work is mainly factoring shared bits cleanly so we don't duplicate ~50 lines of "download + store + insert StockImage."

## Potential Solutions

### Option A: Real `/photos/{id}` integration, parallel to Pexels (recommended)
- **Approach:**
  1. Add `UnsplashClient.GetPhoto(ctx, id) (*Photo, error)` calling `GET /photos/{id}` on `unsplashAPIURL`. Mirror `PexelsClient.GetPhotoByID` (`pexels_provider.go:486`) — same shape: build the request URL, set the Client-ID Authorization header, decode the JSON into the existing `Photo` struct.
  2. Extend the `unsplashClientInterface` (`unsplash_provider.go:327`) to include `GetPhoto` and `DownloadPhoto` so the existing fake client (used in tests) can stub them.
  3. Extract a shared `storeUnsplashPhotoAsStockImage(ctx, photo, startTime)` helper that handles "download + StoreMedia + insert StockImage" — mirroring `PexelsProvider.storePexelsPhotoAsStockImage` (`pexels_provider.go:1144`). The existing `UnsplashProvider.GetStockImage` (`unsplash_provider.go:352`) has this logic inline; the refactor pulls it out so both `GetStockImage` (search-then-pick) and `GetStockImageByID` (direct lookup) reuse it.
  4. Replace the stub `GetStockImageByID` with a real implementation that (a) dedupes via the existing `getExistingStockImage` (`unsplash_provider.go:489`), (b) fetches the photo via `GetPhoto`, (c) delegates to `storeUnsplashPhotoAsStockImage`.
- **Pros:**
  - Pexels flow is the template; structure is already proven and reviewed.
  - Preserves photographer attribution (Unsplash API requirement).
  - Routes Unsplash imports through `CopyStockImageForUser`, so cascade-delete + attribution work correctly.
  - Picks up Unsplash's mandatory download-tracking ping on the by-id path.
- **Cons:**
  - Touches `unsplashClientInterface`, so the fake/mock implementations in tests need the new methods.
  - Two helpers (`GetPhoto` + `storeUnsplashPhotoAsStockImage`) — small additive surface but real diff.
- **Effort:** small.

### Option B: Reuse the search-cache row (partial coverage, additive)
- **Approach:** Before calling Unsplash's API, look up the StockImage by `(provider, provider_photo_id)` via `getExistingStockImage`. If we already imported this photo from a recent search, skip the network round-trip entirely.
- **Pros:**
  - The lookup is free when the candidate emission ran inside the same session — search emits candidates that we may already have a StockImage row for.
  - Cheapest possible fast path.
- **Cons:**
  - Only handles the "candidate from same search" case. Bookmark imports, share-link imports, anything where the StockImage isn't in our DB still need the by-id fetch — i.e. still need Option A.
  - Pexels already does this exact dedupe in `GetStockImageByID` (`pexels_provider.go:1087`). Option A naturally includes this for Unsplash too.
- **Effort:** trivial — but only as a sub-step of Option A.

### Option C: Drop Unsplash from candidate emission entirely (defer the fix)
- **Approach:** Strip Unsplash candidates from `MediaReady` events until Option A lands. `mediacand.StockImageCandidatesToAPI` could filter on `provider != UNSPLASH`, or the search layer could skip Unsplash for candidate-emission purposes.
- **Pros:**
  - Avoids the silent attribution gap until we have time to do this properly.
- **Cons:**
  - Removes Unsplash alternates from the Replace Media row — Pexels is the only source today, and Unsplash has a different stylistic catalog (more lifestyle, less stock). Cutting it noticeably reduces variety.
  - Doesn't fix the underlying gap, just hides it.
- **Effort:** trivial.

## Recommendation

**Option A.** The diff is contained (`GetPhoto` client method + a shared store helper + replacing the stub), the Pexels flow is the template, and it closes the attribution gap that Unsplash's API guidelines technically require us to fix. Option B's dedupe happens naturally inside Option A as the first step of `GetStockImageByID`.

Option C is only the right call if there's an immediate compliance / shipping pressure to remove the silent attribution gap *today* — which there isn't, given the small effort to do Option A properly.

## Open Questions

- **Should the new `storeUnsplashPhotoAsStockImage` helper be backported to the existing `GetStockImage` flow?** — Recommendation: yes. `UnsplashProvider.GetStockImage` (`unsplash_provider.go:352-486`) currently inlines the download + store logic across ~80 lines. Refactoring it out (as the Pexels flow already does) means both entry points (search-then-pick and by-id) share one well-tested path. Doing this in Phase 1 keeps the diff localized and reviewable; backporting later would mean revisiting `GetStockImage` for a second pass.
- **Should the by-id path also call `triggerDownload`?** — Recommendation: yes. The download-tracking ping is required by Unsplash's API guidelines and is already invoked from `DownloadPhoto`. Whether we end up calling `DownloadPhoto` (which internally pings) or a different download path that needs an explicit `triggerDownload` call depends on whether `storeUnsplashPhotoAsStockImage` reuses `DownloadPhoto` or rolls its own. Reuse `DownloadPhoto` — the ping comes along for free.
- **What about the dedupe-key namespace if Unsplash ever ships video?** — Recommendation: punt. Unsplash doesn't currently expose video. If it ever does, mirror the Pexels `video_<id>` prefix on `provider_image_id` so the StockImage table can hold both kinds. Not a Phase-1 concern.
- **Should the fake/mock Unsplash client in tests gain `GetPhoto` too?** — Recommendation: yes. The interface extension is part of the smallest workable diff; mocks need to satisfy the new method or compilation breaks. Run `flutter pub run build_runner build` analogue on the Go side — actually no, Go mocks here aren't generated; they're hand-rolled per `server/media/*_test.go`. Just add the new method to existing test fakes.

## Audit Findings (`/ripls-audit` 2026-05-21)

Cross-checked against `docs/server/architecture.md`, `docs/server/observability.md`, plus CLAUDE.md proto/file/error conventions. Adjustments folded into the Implementation Plan below:

1. **Use `*Context` logging variants on new code.** Per `docs/server/observability.md` § Structured Logging, all new server code uses `logger.InfoContext(ctx, ...)`, `logger.WarnContext(ctx, ...)`, `logger.ErrorContext(ctx, ...)`, `logger.DebugContext(ctx, ...)` so context-derived fields (request_id, user_id) thread through. The existing Unsplash client mixes the context-less form (`logger.Info(...)`, `logger.Error(...)` at lines 163/171/178/182/204/225/231/238/242/266/273/277) which predates the convention. The new `GetPhoto` + `storeUnsplashPhotoAsStockImage` must use the `*Context` form. Don't broaden the cleanup beyond touched lines in this PR — that's a separate hygiene pass.
2. **Standard log field names.** Per `observability.md` § Standard Field Names: use `operation` for the function name, `provider` for vendor ("unsplash"), `external_service` only at boundary points if needed, `duration_ms`, `error`. Entity ids: `photo_id` for the Unsplash photo id, `stock_image_id` for the StockImage row id, `media_id` for the Media row id. The Pexels flow already follows this — mirror it exactly.
3. **Error wrapping.** Plan returns plain Go errors from `GetStockImageByID` (Go convention at the library layer), not `connect.Error`. The handler-level `importViaTrustedProvider` already routes through `connecterr.Internal(ctx, "AddMediaFromURL", err, ...)` for any error that surfaces to the wire — no change there. Don't introduce `connect.NewError` at the library layer.
4. **File size guideline.** `server/media/unsplash_provider.go` is 683 lines today; Phase 1 adds ~50 lines (`GetPhoto` + `storeUnsplashPhotoAsStockImage` extraction nets out close to zero since the extraction *moves* code rather than adding it). Stays well under the 1000-line guideline; no split needed.
5. **Testing convention.** Per `docs/server/architecture.md` § Testing Strategy: unit tests use `httptest.NewServer` to stub HTTP boundaries (the Pexels client tests do this) and hand-rolled fakes that satisfy the package's interface for higher-level provider tests. Plan already follows this.
6. **No proto changes**, so the proto-comment vendor-name rule doesn't apply. The Unsplash vendor name is fine in Go code and READMEs per CLAUDE.md § READMEs and Internal Documentation.
7. **No goroutines spawned.** N/A for the goroutine-safety lint.
8. **No new TODOs introduced.** Plan removes the existing follow-up reference rather than adding new TODOs.

## Implementation Plan

### Phase 1: Add `GetPhoto` to `UnsplashClient` + refactor `GetStockImage`

**Goal:** Land the client-side `/photos/{id}` integration and pull the shared "download + store + insert" path out of `GetStockImage` into a reusable helper. The on-disk behaviour of `GetStockImage` is identical before/after — pure refactor.

**Tasks:**
- [ ] Add `func (c *UnsplashClient) GetPhoto(ctx context.Context, photoID string) (*Photo, error)` to `unsplash_provider.go`. Mirror `PexelsClient.GetPhotoByID` (`pexels_provider.go:486-516`): build `GET /photos/{photoID}` URL, set `Authorization: Client-ID ...`, decode into a `Photo`, propagate `ErrRateLimited` on 429. **All log lines on this new method use the `*Context` variants** (`logger.InfoContext(ctx, ...)`, `logger.WarnContext(ctx, ...)`, etc.) per `docs/server/observability.md`. Standard fields: `operation=GetPhoto`, `provider=unsplash`, `photo_id`, `duration_ms`.
- [ ] Extend the `unsplashClientInterface` (`unsplash_provider.go:327`) with `GetPhoto(ctx, id) (*Photo, error)`. Update any hand-rolled fakes in `server/media/*_test.go` to satisfy the new method.
- [ ] Refactor: extract `func (p *UnsplashProvider) storeUnsplashPhotoAsStockImage(ctx, photo, startTime) (*models.StockImage, error)` from the bottom half of `GetStockImage` (current lines ~411-485). Move the download (`DownloadPhoto`), `storage.StoreMedia`, `models.StockImage` build, and `Insert` calls into the helper. **Use `*Context` log variants on the new helper** even though the surrounding `GetStockImage` mixes the older form — don't touch the existing call sites in this PR, just write the new code on the convention. `GetStockImage` then becomes: search → pick → delegate to the new helper.
- [ ] Add a unit test for `UnsplashClient.GetPhoto` in `server/media/unsplash_provider_test.go` using `httptest.NewServer` to stub `/photos/{id}` (the Pexels client tests are the pattern model). Cover: happy-path JSON decode, 404 → `errors.Is(err, somethingSensible)` so callers can distinguish "not found" from network error, 429 → `errors.Is(err, ErrRateLimited)`.

**Validates:** `npm run build` clean. Existing `UnsplashProvider.GetStockImage` tests still green (no behaviour change). New `GetPhoto` test confirms the by-id client method round-trips correctly.

### Phase 2: Replace the `GetStockImageByID` stub

**Goal:** Wire `GetStockImageByID` through the new client method + shared helper. Unsplash candidate imports now preserve attribution and skip the generic-URL fallback.

**Tasks:**
- [ ] Replace the stub `UnsplashProvider.GetStockImageByID` (`unsplash_provider.go:549-551`) with the real implementation:
  - Standard logger header: `logger := logging.LoggerWithContext(ctx).With("service", "stock_imagery", "provider", "unsplash", "operation", "GetStockImageByID", "photo_id", photoID)` (matches the Pexels equivalent at `pexels_provider.go:1076-1081`).
  - Dedupe first via `getExistingStockImage(ctx, photoID)` — if a StockImage row already exists for this photo, return it with a `logger.InfoContext(ctx, "stock imagery by-id returning cached", ...)` line.
  - Otherwise call `p.client.GetPhoto(ctx, photoID)` to fetch the photo metadata. Log a `WarnContext` on error (mirroring `pexels_provider.go:1097-1102`).
  - Delegate to `p.storeUnsplashPhotoAsStockImage(ctx, photo, startTime)`.
- [ ] Add a unit test `TestUnsplashProvider_GetStockImageByID` covering (a) dedupe hit, (b) fresh fetch + store, (c) `GetPhoto` returns 404 → caller falls through to generic-URL path (return non-nil error so the fallback chain in `FallbackStockImageryProvider.GetStockImageByID` keeps moving). Use the hand-rolled `unsplashClientInterface` fake to stub `GetPhoto` + `DownloadPhoto`.
- [ ] Smoke against dev: import a Unsplash candidate via Replace Media; verify the resulting media row has `SourceStockImageId` populated and `GetMedia` returns `Attribution` with the photographer name and Unsplash provider. Watch server logs to confirm the new `WithContext`-attributed log lines fire with `request_id` propagated correctly.

**Validates:** End-to-end attribution preserved on Unsplash imports. `importViaTrustedProvider` no longer falls through to the generic-URL path for Unsplash candidates.

### Phase 3: Documentation + plan-doc closure

**Goal:** Update the candidate-import documentation to reflect that Unsplash now has parity with Pexels, and close the issue.

**Tasks:**
- [ ] Update any inline comment in `service.go:280-291` that mentioned "Unsplash by-id is a follow-up (#2063)" — confirm there are none left after the refactor.
- [ ] Cross-reference this plan + the PR in `docs/issues/2038-replace-media-candidates.md` § Out-of-scope follow-ups so the historical trail stays accurate.
- [ ] Close #2063 referencing the merged PR.

**Validates:** No stale TODO / follow-up references to the Unsplash by-id gap remain in the codebase.

## Related Issues
- #2038 — feature that introduced the candidate-import path.
- PR #2062 — implementation that landed the trusted-provider scaffold (with the Unsplash stub).
- #2066 — companion follow-up on rate-limiting `AddMediaFromURL`. Tangentially related: rate-limiting is a defense-in-depth measure for the generic-URL path, which Unsplash candidates would stop hitting once this issue lands.
