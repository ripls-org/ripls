# Issue #2064: Community hero surfaces don't use getHeroMediaUrl yet

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2064
**Filed:** 2026-05-20 | **Last activity:** 2026-05-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

PR #2062 introduced `MediaRepository.getHeroMediaUrl` so that surfaces displaying a community's media at hero size (full-bleed background, large card) would:

- **Image media:** load the full-resolution URL — sharp at large render sizes (instead of a fuzzy thumbnail).
- **Video media:** load the poster (thumbnail JPEG) — so `CachedNetworkImage` / `NetworkImage` has a decodable still while the MP4 downloads in the background, instead of a broken-image render of raw video bytes.

The unified-create preview and the community-preview modal were wired through `getHeroMediaUrl` in PR #2062, but the same fix wasn't propagated to the **other** hero-sized surfaces that show the same community media — most notably the community public/metrics screen.

After reading the codebase (current `main`, post-merge of PR #2062), the actual gap is narrower than the original issue body suggested. Several sites the issue named are small avatars, not heroes:

- `workshop_community_identity.dart:138` — 64×64 ClipOval avatar (not a hero).
- `community_avatar.dart:57` — default radius 16 → 32×32 (avatar).
- `community_context_header.dart:178` — 32×32 dropdown row (avatar).
- `community_members_pane.dart:68,121` — small member avatars.
- `community_edit_screen.dart:179` — uses `state.mediaPath` (a URL already in hand), not `mediaObjectProvider`, so the thumbnail-default bug doesn't apply.

The **real** broken hero is:

- `community_impact_view_model.dart:314` — `loadCommunityImage` calls `mediaRepository.getFullMediaUrl(communityMediaId)` and passes the result into `ImpactMetricsHeroSection.backgroundImageUrl`, which feeds `NetworkImage(backgroundImageUrl)`. For an **image** community media that's fine; for a **video** community media this returns the raw MP4 URL and `NetworkImage` can't decode it — same bug `getHeroMediaUrl` was created to fix.

Secondary candidates worth deciding on:

- Discover carousel cards (e.g. `discover_event_carousel.dart`, ~230×160) and grid tiles (`discover_gear_grid.dart`) currently use `mediaObjectProvider` (thumbnail). They're bigger than avatars but smaller than a true hero. Sharpness on these surfaces is a "B-tier" concern.

So the bug-shaped part of this issue is small (one call site swap); the rest is a hygiene/convention pass to decide what each surface should use going forward.

## Context from Comments

No comments on the issue. Note that the original issue body was filed by the same person who's now planning it (briangbrown), so there are no third-party clarifications or pushback yet — but reading the actual code surfaced that the listed "where the gap is" pointers were partially wrong. The plan corrects that.

## Potential Solutions

### Option A: Add a `heroMediaUrlProvider` and fix the one real hero (recommended)
- **Approach:** Add a `heroMediaUrlProvider` family in `app/lib/services/providers/media_providers.dart` mirroring `fullMediaUrlProvider` but delegating to `mediaRepository.getHeroMediaUrl`. Update the one identified hero gap (`community_impact_view_model.loadCommunityImage`) to use `getHeroMediaUrl` instead of `getFullMediaUrl` — fixes the broken render on video community media.
- **Pros:**
  - Minimal blast radius. Fixes the actual functional bug.
  - Adds a clean opt-in for any future hero-sized surface.
  - No avatar/thumbnail churn — those keep their lightweight default.
- **Cons:**
  - Doesn't address the soft-image question on carousel/grid cards (defer to a follow-up if/when measured).
- **Effort:** small.

### Option B: Make `getHeroMediaUrl` the default and demote thumbnails to an explicit flag
- **Approach:** Rename `getMediaUrl` semantics to return hero-shaped URLs; add an explicit `thumbnail: true` flag for small surfaces. Audit every `mediaUrlProvider` / `mediaObjectProvider` call site to confirm hero shape is safe and update where it isn't.
- **Pros:**
  - Defaults to "always look good." Less chance of a future surface forgetting to opt in.
- **Cons:**
  - Most existing call sites are avatars and small list rows where the thumbnail is the right choice — they'd need explicit opt-out, which is the opposite of the actual usage frequency. Lots of mechanical churn for little net win.
  - Heavier bytes-on-the-wire for every surface that *currently* benefits from thumbnails (avatars, member lists, dropdown rows, carousel cards).
- **Effort:** medium (one-line provider change + 20+ call-site audits).

### Option C: Per-widget `getHeroMediaUrl` direct calls
- **Approach:** Skip a new provider. Each hero widget inlines its own `await mediaRepository.getHeroMediaUrl(mediaId)` (`FutureBuilder`/`AsyncNotifier` pattern, same as the existing `loadCommunityImage`).
- **Pros:**
  - No new provider to maintain.
- **Cons:**
  - Reinvents the async-loading dance at every site that would benefit; misses Riverpod's caching/scoping. Inconsistent with how `mediaUrlProvider`/`mediaObjectProvider`/`fullMediaUrlProvider` are exposed today.
- **Effort:** small but messier.

### Option D: Do nothing (defer entirely)
- **Approach:** Ship the existing fix and revisit if users actually complain about video community media not rendering.
- **Cons:** It's a real bug — any community whose hero media is a video will render broken on the public screen. Closed-beta users may hit this without knowing why.
- **Effort:** none.

## Recommendation

**Option A.** The bug-shaped piece is one call-site swap; the provider addition is one helper. The convention questions (carousel cards, grid tiles) are better answered after we have at least one well-documented hero provider in the codebase that other surfaces can adopt incrementally.

If anyone audits Option B and finds a way to thread it without 20+ explicit `thumbnail: true` opt-outs, that becomes preferable. But the current call-site distribution heavily favors thumbnails-as-default, so the opt-in `heroMediaUrlProvider` shape matches usage.

## Open Questions

- **Should the `loadCommunityImage` flow keep manually storing the resolved URL in `CommunityImpactData.communityImageUrl`, or should it switch to consuming `heroMediaUrlProvider` directly?** — Recommendation: switch the consumption point (the widget that renders `backgroundImageUrl`) to watch `heroMediaUrlProvider(communityMediaId)`. That way `CommunityImpactData` no longer has to mirror the resolved URL, the value comes straight from the provider, and a media refresh upstream invalidates correctly. If that proves to be a bigger refactor than the issue warrants, fall back to having `loadCommunityImage` call `getHeroMediaUrl` instead of `getFullMediaUrl` — same one-line bug fix, smaller blast radius.
- **What about carousel/grid cards that are ~200–230 px?** — Recommendation: leave on thumbnail in Phase 1. File a follow-up if anyone complains about sharpness. The bytes-saved-vs-pixels-shown tradeoff at that size is genuinely a wash; without user feedback we can't tell whether it matters.
- **What about `ImpactMetricsHeroSection` using `NetworkImage` directly instead of `CachedMediaImage`?** — Recommendation: out of scope for this issue. It works today for the image case; the video-fallback fix lives in `getHeroMediaUrl` returning the poster URL, which `NetworkImage` can decode. A separate hygiene pass could migrate to `CachedMediaImage` for stable cache keys, but that's a different concern from the hero-URL choice.
- **Should `getFullMediaUrl` be deprecated in favour of `getHeroMediaUrl`?** — Recommendation: not yet. `getFullMediaUrl` is the correct shape for surfaces that *definitely* show an image (fullscreen viewer, share preview). They explicitly want the full URL, not a video poster. Keep both helpers; document when to reach for which.

## Audit Findings (`/ripls-audit` 2026-05-20)

Cross-checked against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/testing.md`, `docs/client/i18n.md`, `docs/client/accessibility.md`. Notes folded into the Implementation Plan below:

1. **Image cache-key pairing is automatic via `MediaUrl.cacheKey`.** Per `docs/client/caching.md` § Image Cache Keys, thumbnail and full images MUST use different cache keys (`media:thumb:{mediaId}` vs `media:full:{mediaId}`) — otherwise `CachedNetworkImage` serves the smaller cached entry when the larger is requested, causing blurry backgrounds. `MediaUrl.cacheKey` (defined in `app/lib/data/repositories/media_url.dart:39`) already picks the right key from `isThumbnail`, and `getHeroMediaUrl` sets `isThumbnail` correctly for both image and video paths. So consumers of `heroMediaUrlProvider` who use `CachedMediaImage` just pass `mediaUrl.cacheKey` — no separate decision needed. Document this pairing in the Phase 2 doc update so the next hero-using surface doesn't fall into the trap.
2. **Phase 1's call site uses `NetworkImage`, not `CachedMediaImage`.** `ImpactMetricsHeroSection` wraps `backgroundImageUrl` in `NetworkImage(backgroundImageUrl!)` (`impact_hero_section.dart:74-76`). That means the new `MediaUrl.cacheKey` is unused at this specific call site — but the functional bug (raw MP4 URL into NetworkImage) is fixed regardless because `getHeroMediaUrl` returns the poster JPEG URL for video media, which `NetworkImage` decodes fine. The migration from `NetworkImage` → `CachedMediaImage` for proper cache-key reuse is captured in Phase 3 as a deferred follow-up; it isn't required for the functional fix.
3. **Caching architecture is unchanged.** All caching stays at the repository layer (`MediaRepository.getHeroMediaUrl` already exists and uses `_cache.get` via `MediaRepository.get`). The new `heroMediaUrlProvider` is just a thin wrapper following the same pattern as `mediaUrlProvider` / `mediaObjectProvider` / `fullMediaUrlProvider` (`app/lib/services/providers/media_providers.dart`). No new cache namespace, no manual viewmodel cache.
4. **Testing follows the project convention.** ViewModel-first tests using `ProviderContainer` + Mockito (`@GenerateMocks([MediaRepository])`). The existing `app/test/presentation/viewmodels/community_impact_view_model_test.dart` already has `MockMediaRepository` wired (line 52 / 61 / 81), so Phase 1's video-content-type test plugs straight in.
5. **a11y is unaffected by Phase 1.** No new interactive widgets, no new user-visible strings. If Phase 3 migrates to `CachedMediaImage`, the `require_semantic_label_on_cached_media_image` rule (per `docs/client/accessibility.md`) requires a `semanticsLabel`. The hero is decorative (the community name is rendered separately by the impact-hero section as a header), so the appropriate value would be a deliberately-decorative `null` with an inline comment matching the existing convention, e.g. `// Decorative; the surrounding card carries the semantic label.`
6. **i18n unchanged.** No ARB additions.
7. **`docs/client/media.md` already exists** (1286 lines) — the Phase 2 task should extend it, not create.

## Implementation Plan

### Phase 1: Add provider + fix the one broken hero

**Goal:** Add a clean opt-in for hero-sized surfaces and fix the broken render on video community media at the community-public/metrics screen.

**Tasks:**
- [ ] In `app/lib/services/providers/media_providers.dart`, add `heroMediaUrlProvider` as a `FutureProvider.family<MediaUrl, String>` that delegates to `mediaRepository.getHeroMediaUrl(mediaId)`. Doc-comment explains the hero-vs-full-vs-thumbnail decision matrix so the next developer doesn't have to read three providers to find the right one.
- [ ] Convert `community_impact_view_model.dart:303` `loadCommunityImage` to call `mediaRepository.getHeroMediaUrl(communityMediaId)` instead of `getFullMediaUrl`. One-line change. Preserve the rest of the flow (manual URL caching on `CommunityImpactData`).
- [ ] Add a focused viewmodel test in `app/test/presentation/viewmodels/community_impact_view_model_test.dart` (existing file already wires `MockMediaRepository` via `@GenerateMocks` at line 52 and `ProviderContainer` overrides). Tests should:
  - Stub `mockMediaRepository.getHeroMediaUrl` to return a `MediaUrl` with `isThumbnail: true` for the video case (i.e. the thumbnail URL is what `getHeroMediaUrl` resolves video media to).
  - Call `loadCommunityImage` and assert the resolved `communityImageUrl` matches the stubbed thumbnail URL.
- [ ] Add an analogous test for image-content-type media — stub `getHeroMediaUrl` to return `isThumbnail: false` with the full URL; assert the full URL flows through. Mirrors the existing image happy-path tests in the same file.
- [ ] Regenerate mocks if `MediaRepository`'s mocked surface changes: `flutter pub run build_runner build --delete-conflicting-outputs` (per `docs/client/testing.md` § Mock Generation).
- [ ] Smoke-test on the dev server with a video community media: visit the community public screen; confirm the hero renders the poster while the video downloads.

**Validates:** Video community media no longer renders a broken NetworkImage on the community-public screen. The new provider exists and is consumable.

### Phase 2: Convention doc + audit secondary surfaces

**Goal:** Document the hero/full/thumbnail decision so future hero-shaped surfaces opt in correctly, and decide carousel/grid sites.

**Tasks:**
- [ ] Extend the existing `docs/client/media.md` with a short subsection (place it near the existing "Media Data Flow" content) documenting the four `MediaRepository` methods + their providers:
  - When to use `mediaUrlProvider` / `getMediaUrl` (default: thumbnail-preferring with video safety).
  - When to use `heroMediaUrlProvider` / `getHeroMediaUrl` (large background / preview hero).
  - When to use `fullMediaUrlProvider` / `getFullMediaUrl` (fullscreen viewer, share preview — always image, never video).
  - Cross-reference the **Image Cache Keys** section in `docs/client/caching.md` so readers know to pair the URL with `mediaUrl.cacheKey` when rendering via `CachedMediaImage` / `CachedNetworkImage` (avoids the thumbnail-vs-full cache-key collision).
- [ ] Audit `discover_event_carousel.dart`, `discover_help_card.dart`, `discover_gear_grid.dart` for visible softness on real device. The size-vs-method table in `docs/client/caching.md` § Choosing Thumbnail vs Full says `< 200px → thumbnail`, `>= 200px → full`. The discover carousel cards are 230×160 — strictly at the threshold. If users notice (or screenshots clearly show fuzziness), file a follow-up to convert specific cards to `heroMediaUrlProvider`. Otherwise document the decision to keep them on thumbnail.

**Validates:** The next contributor adding a hero surface knows which provider to reach for without reading three repositories.

### Phase 3: (Deferred) — `CommunityImpactData` URL ownership + `CachedMediaImage` migration

**Goal:** Stop having the viewmodel manually mirror the resolved hero URL into its own state; let the widget watch the provider directly. While we're in the file, migrate `ImpactMetricsHeroSection` off raw `NetworkImage` onto `CachedMediaImage` so the image bytes get a stable disk cache (`media:full:{mediaId}` for image media, `media:thumb:{mediaId}` for video posters — both keyed correctly by `MediaUrl.cacheKey`).

**Tasks:**
- [ ] Remove `communityImageUrl` / `communityImageMediaId` from `CommunityImpactData`.
- [ ] Update `CommunityMetricsScreen` to watch `heroMediaUrlProvider(communityMediaId)` directly and pass `.value?.url` + `.value?.cacheKey` into `ImpactMetricsHeroSection`.
- [ ] Migrate `ImpactMetricsHeroSection`'s `DecorationImage(image: NetworkImage(...))` to `CachedMediaImage` (or `CachedNetworkImage` with explicit `cacheKey`) so the image bytes cache via `flutter_cache_manager`. **a11y note (per `docs/client/accessibility.md`):** The hero is decorative — the community name is rendered as a separate header within `ImpactMetricsHeroSection`. Pass `semanticsLabel: null` with the existing in-tree comment convention `// Decorative; the surrounding card carries the semantic label.` so the `require_semantic_label_on_cached_media_image` lint rule still passes (the rule trips on raw string literals, not on a deliberately-null label).
- [ ] Remove the `loadCommunityImage` method entirely.
- [ ] Update tests — drop the now-obsolete `loadCommunityImage` test cases, add a widget-level smoke if any of the visual changes warrant it (otherwise rely on the existing ViewModel coverage).

**Validates:** Hero URL lifecycle is owned by the provider — invalidating media upstream automatically refreshes the hero without explicit `loadCommunityImage` calls. Image bytes hit the disk cache with the correct thumbnail-vs-full namespace. Defer unless someone is already in this code for another reason.

## Related Issues
- #2038 — feature where `getHeroMediaUrl` was introduced.
- PR #2062 — added the helper and the unified-create / community-preview wiring.
- #2067 — sibling follow-up about surfacing candidate thumbnails when editing existing items. Tangential to this issue but lives in the same provider/repo area.
