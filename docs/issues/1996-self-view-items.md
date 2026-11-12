# Issue #1996: Users items are not showing up

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1996
**Filed:** 2026-05-17 | **Last activity:** 2026-05-17
**Labels:** user-feedback | **Assignees:** tescobar
**Context docs:** [docs/issues/1929-profile-v4-redesign.md](1929-profile-v4-redesign.md)

## Problem Statement

When the current user navigates to their own profile via the Profile menu → "My Profile", the screen renders the identity block, communities subtitle, and the three impact rows (52 HRS GIVEN, $0 SAVED, 0 KG CO₂ AVOIDED) but **nothing below them** — no Available Now rail, no postcards, no items of any kind. The reporter is the target of the screen ("this is the current user") and clearly has items in the system (the impact-row stats are non-zero for time given), so the expectation is that some part of the screen surfaces their offerings.

This is **self-view of `UserScreen`**, which was redesigned in #1929 to render through "the viewing user's lens" — every section below Impact Rows is intentionally gated on shared-community overlap between viewer and target. For self-view, viewer == target, the shared-community set is empty by construction, and every gated section is hidden. The user is seeing the design's intended self-view output and reading it as a bug.

The ambiguity to resolve before fixing: is the right fix to **show the user their own items inside self-view of `UserScreen`**, or to **route "My Profile" somewhere else entirely** (a dedicated self-screen, or back to the existing Workshop tab that was the pre-#1929 self-surface)? Both are coherent product answers; the issue body does not disambiguate.

## Context from Comments

The issue has **no comments** — only the feedback-system body and a screenshot. That means:
- We don't have the reporter's expectation in their own words ("I expected to see X").
- We don't have maintainer direction on whether self-view should mirror other-view or diverge.
- The reporter is the assignee (`tescobar`), so they have full latitude on the product call.

Flagging this as the single largest source of risk: the fix shape depends on a product decision that the issue doesn't make. See Open Questions.

## Codebase Findings

What the investigation surfaced (see referenced files for the full context):

- **Self-view route.** [app/lib/presentation/screens/profile/profile_screen.dart:41-48](../../app/lib/presentation/screens/profile/profile_screen.dart#L41-L48) — Profile menu → "My Profile" calls `UserScreen(userId: currentUserId)`. Same widget for self and other.
- **Gate at the client.** [app/lib/presentation/screens/users/user_screen.dart:96-103](../../app/lib/presentation/screens/users/user_screen.dart#L96-L103) — `UserProfileAvailableNowRail` and `UserProfileLeanCards` both render only when `state.hasSharedCommunities` is true. `hasSharedCommunities` is `sharedCommunities.isNotEmpty` ([viewer_profile_view_model.dart:51-52](../../app/lib/presentation/viewmodels/viewer_profile_view_model.dart#L51-L52)).
- **Gate at the server.** Four separate self-view suppressions exist in [server/services/profile/profile.go](../../server/services/profile/profile.go), all keyed off `req.Msg.TargetUserId != authInfo.UserID`:
  - `availableNowForTarget` ([profile.go:100-119](../../server/services/profile/profile.go#L100-L119)) → empty `availableNowItems`.
  - `knownForForTarget` ([profile.go:100-119](../../server/services/profile/profile.go#L100-L119)) → empty `knownFor`.
  - `readImpactNarrativesInto` + background `EnsureNarratives` regeneration ([profile.go:135-161](../../server/services/profile/profile.go#L135-L161)) → null `timeNarrative` / `moneyNarrative` / `co2Narrative` and no LLM refresh.
  - `generatePostcards` ([profile.go:80-91](../../server/services/profile/profile.go#L80-L91)) — gated implicitly on `len(sharedRefs) > 0`, which is empty for self-view by definition (you don't share communities with yourself).
- **`resolveSharedCommunities` for self-view.** The viewer-target intersection is empty by definition because the join requires two distinct memberships. The shared-community subtitle ("With Oakwood Heights Neighbors, Horizons K8 Parents and Ripls Developers" in the screenshot) is populated from `target.communities` directly, not from the intersection — that's why the identity block still names communities while the Available Now rail collapses.
- **Design intent (#1929).** The Available Now rail was specified as "currently-available gear and upcoming experiences **inside shared communities**" and gated on `hasSharedCommunities` because "their entities are only actionable inside shared communities" (1929 plan, "Shared-community gating" decision). Self-view was not called out as an exception. The pre-#1929 Workshop widgets that previously rendered self-view inventory (`WorkshopHero`, `WorkshopTicker`, `WorkshopPostcard`) were removed from `UserScreen` but kept in the codebase — the Workshop tab still uses them.

## Potential Solutions

### Option A: Show self-view items by treating "all my communities" as the shared set

**Approach:** Special-case `viewer == target` in `Service.GetUserProfileForViewer`. Instead of skipping `availableNowForTarget`, call it with the full list of the target's community memberships. On the client, change the gate from `state.hasSharedCommunities` to `state.hasSharedCommunities || state.isSelfView` (or equivalent), where `isSelfView` is derived by comparing `targetUserId` to the authenticated user ID surfaced through `authProvider`.

- **Pros:** Smallest behavioral change — the rail does exactly what it does for other-view, just sourced from the user's full membership set. Reuses every code path the rail already exercises. Items the user has shared into any community show up; items they own but haven't published into a community don't (consistent with the "actionable inside a community" framing).
- **Cons:** Doesn't surface "I own this gear but haven't published it anywhere" items — those are arguably the most important class of self-view items because they represent the user's draft inventory. The user's `52 HRS GIVEN` includes loans that may have been against unpublished gear, so the rail can still look empty while stats are non-zero.
- **Effort:** Small. ~30 lines server-side, ~10 lines client-side, no proto change.

### Option B: Add a dedicated self-view section that shows the user's full inventory

**Approach:** Keep the Available Now rail as-is (community-scoped, hidden on self-view). Add a new section that renders only when `isSelfView` — e.g. "Your items" or "Your inventory" — backed by a new RPC field on `GetUserProfileForViewerResponse` (`my_items`, populated only when viewer == target) or by a separate `GearRepository` fetch on the client. The section surfaces all of the user's owned gear regardless of community publication state.

- **Pros:** Distinguishes self-view from a community-relationship view, which is semantically what the user actually wants ("show me my stuff"). Surfaces unpublished gear and gives the user a path to publish from the profile. Doesn't conflate "available in a shared community" with "what I own".
- **Cons:** Larger surface — new section, new strings, new widget, new fetch. Risks duplicating what the Workshop tab already does (which is exactly the "show me my stuff" view). Pushes `UserScreen` further from "viewer's lens" into a mode-switching screen.
- **Effort:** Medium. New widget (~80-120 LoC), new fetch path or proto field, new ARB strings, a11y labels, tests.

### Option C: Route "My Profile" to the Workshop tab instead of UserScreen

**Approach:** Treat self-view of `UserScreen` as a wrong-screen problem. Change the Profile menu's "My Profile" entry ([profile_screen.dart:41-48](../../app/lib/presentation/screens/profile/profile_screen.dart#L41-L48)) to deep-link into the Workshop tab — that's where pre-#1929 self-view items lived and where the `WorkshopHero` / `WorkshopTicker` / `WorkshopPostcard` widgets still render. `UserScreen` becomes an other-view-only surface (and we can gate it explicitly).

- **Pros:** Zero new code on the profile screen — leverages the surface that already shows the user their own items. Reinforces #1929's framing that `UserScreen` is for viewing *others*. Avoids accumulating viewer/self conditionals across the screen.
- **Cons:** Loses the impact-rows / known-for / community-subtitle / narrative experience for self-view (the Workshop tab doesn't render any of those). The user reporting this issue is on `UserScreen` precisely because the menu sent them there — if the answer is "you shouldn't be here", we need a clearly better destination, and Workshop in its current form isn't obviously that destination. Workshop tab's own future is uncertain (#1889 and the deprecation hinted at by #1929 removing Workshop widgets from `UserScreen`).
- **Effort:** Small in lines, large in product implication. Re-routing one tap is trivial; deciding that Workshop is the canonical self-view is not.

## Recommendation

**Option A, scaled up: self-view as a strict mirror of other-view.** Lift the self-view suppression on all four target-only sections — `availableNowForTarget`, `knownForForTarget`, `readImpactNarrativesInto`, and `EnsureNarratives` — and source the community-scoped sections (`availableNowForTarget`, `knownForForTarget`) from the target's full active community membership rather than the empty viewer-target intersection. Narratives are per-user and need no scope change. Postcards stay implicitly empty because their gate (`len(sharedRefs) > 0`) is empty for self-view by definition, and "you two — 2 events together…" framing is incoherent for a single user; this is the one principled exception to strict mirror.

Why this over plain Option A: the user's complaint ("items not showing up") is the most visible symptom, but knownFor chips and per-user impact narratives are suppressed by the same self-view check and would feel like a half-fix if items came back but knownFor + narratives stayed empty. The screen's own doc comment ([user_screen.dart:29-30](../../app/lib/presentation/screens/users/user_screen.dart#L29-L30)) already commits to "Identity, Known For, Impact Rows render unconditionally — they describe the target, not the relationship" — so the client is already correct; only the server is gating these. Lifting all three suppressions together is the change that matches the doc-stated contract.

Why this over Option B (dedicated self-view section): the issue says "users items are not showing up", not "I want to see my drafts". The user's mental model when looking at their own profile is the same as when looking at a friend's. If items are missing because they haven't been published into a community, that's a separate, real, and surfaceable problem — file as follow-up.

Why this over Option C (re-route to Workshop): re-routing "My Profile" discards the impact rows, narratives, and known-for chips that strict mirror gives the user. The screenshot evidence is that the user *expected* items underneath those sections, not instead of them.

## Open Questions (Resolved)

All four resolved 2026-05-17 by the assignee. Decisions are now load-bearing for the implementation plan below.

- **Is self-view of `UserScreen` meant to be a strict mirror of other-view, or a different surface?** → **Strict mirror.** Identity, knownFor, impact rows + narratives, and the Available Now rail all render on self-view exactly as they would for any other viewer with shared communities. Community-scoped sections (rail, knownFor) source from the target's full active community membership instead of the empty viewer-target intersection. The one principled exception is postcards (see below).
- **Should the rail on self-view include items the user owns but hasn't published into any community?** → **No, not in this change.** The rail's identity is "available now in a community"; an unpublished item isn't available to anyone. A separate "drafts / unpublished" surface can be filed as follow-up if it's a real need.
- **Does the server still need to suppress `knownFor` and per-user AI narratives on self-view?** → **No — lift both.** The strict-mirror posture means self-view sees the same knownFor chips and per-user AI narratives any other viewer would see. `knownFor` derives from the target's completed gear loans inside their own communities (source set: `targetActiveCommunityIDs`). Narratives are per-user and have no community scope — the existing `readImpactNarrativesInto` and background `EnsureNarratives` regeneration run unchanged on self-view. The original suppression's "don't tell a user what they're known for" rationale doesn't outweigh the user-visible "my profile looks broken" cost evidenced by this issue. The narrative regeneration cost is bounded by `EnsureNarratives`'s own dedup/throttle logic — self-view doesn't bypass that.
- **Should postcards (lean cards) also render on self-view as part of strict mirror?** → **No — keep implicitly empty.** Postcards are inherently viewer-relationship content ("you two — 2 events together"), framed as a second-person CTA between viewer and target. The "you two" framing collapses to nonsense when viewer == target. The existing implicit gate (`len(sharedRefs) > 0`, empty for self-view by construction) keeps them empty on self-view without any explicit `viewer == target` check — leave both the server gate and the client `UserProfileLeanCards` gate alone.
- **Does the rail need a different header / mini-label on self-view?** → **No.** Ship with the same rail. If it reads oddly in the user's own profile we can add an ARB-driven self-view variant later — but the rail today has no explicit header text, so no string change is needed.

## Implementation Plan

### Phase 1: Server lifts self-view suppressions

**Goal:** When the authenticated user requests their own profile, the response is a strict mirror of what any other viewer-with-shared-communities would see — populated `availableNowItems`, populated `knownFor`, and populated narrative fields — with the single principled exception that postcards remain empty (their gate is shared-communities, which is empty for self-view by construction).

**Tasks:**
- [ ] **Lift Available Now suppression.** In [server/services/profile/profile.go:100-119](../../server/services/profile/profile.go#L100-L119), call `availableNowForTarget` unconditionally. For self-view, pass `targetActiveCommunityIDs` (see next task) instead of the empty intersection.
- [ ] **Lift Known For suppression.** Same block — call `knownForForTarget` unconditionally. For self-view, pass `targetActiveCommunityIDs`. `knownFor` derives from completed gear loans inside the supplied community set, so this is the same scope swap as Available Now.
- [ ] **Lift narrative suppression.** In [server/services/profile/profile.go:135-161](../../server/services/profile/profile.go#L135-L161), remove the `if req.Msg.TargetUserId != authInfo.UserID` guard around `readImpactNarrativesInto` and the background `EnsureNarratives` goroutine. Narratives are per-user and have no community scope — the existing dedup/throttle inside `EnsureNarratives` bounds the regeneration cost. Both the read and the regeneration run for self-view.
- [ ] **Postcards stay implicitly empty.** Do **not** change the `len(sharedRefs) > 0` gate around `generatePostcards` at [profile.go:80-91](../../server/services/profile/profile.go#L80-L91). Self-view's intersection is empty by definition, so postcards collapse out naturally — no `viewer == target` carve-out needed and no client gate change.
- [ ] **Source target's own community memberships.** Extend `resolveSharedCommunities` ([server/services/profile/profile.go:176+](../../server/services/profile/profile.go#L176)) to return an additional `targetActiveCommunityIDs []string` alongside `sharedRefs` — the active, non-deleted target memberships it already loads via `storage.QueryByField[*models.CommunityUser]`. Reuse the in-scope `targetMemberships` slice; do **not** re-query. Update the helper's doc comment and call sites to reflect the new return value. (Per CLAUDE.md §SQL Efficiency: never re-query when the rows are already loaded.)
- [ ] **Observability.** Add a `self_view` boolean log field to the handler's `logger` chain (`logger = logger.With("self_view", req.Msg.TargetUserId == authInfo.UserID)`), placed alongside the existing `shared_community_count` / `other_community_count` `.With` block near [profile.go:62-65](../../server/services/profile/profile.go#L62-L65). This makes the existing `available_now_count` / `known_for_count` log line sliceable by viewer-relationship. Per [docs/server/observability.md](../server/observability.md): standard snake_case field, no new log line needed — the existing "profile assembled" line picks it up.
- [ ] **Available Now unit test.** Add `TestAvailableNowForTarget_SelfViewSourcesFromOwnCommunities` to [server/services/profile/available_now_test.go](../../server/services/profile/available_now_test.go) modeled on the existing same-community tests. Single target user with gear published into one of their own communities; assert the item appears when called with `targetActiveCommunityIDs`.
- [ ] **Known For unit test.** Add a corresponding test (`TestKnownForForTarget_SelfViewSourcesFromOwnCommunities`) to whichever `*_test.go` file currently covers `knownForForTarget`. Seed a completed loan inside the target's own community and assert the capability tag appears.
- [ ] **RPC-level test.** Add `TestGetUserProfileForViewer_SelfView_StrictMirror` to [server/services/profile/profile_test.go](../../server/services/profile/profile_test.go) — a unit-style test using `storage.SetupTestStorage(t)` that seeds a user with active community membership, AVAILABLE gear listed in that community, at least one completed loan, and a pre-existing impact-narratives row. Call the RPC handler directly with viewer == target. Assert: non-empty `AvailableNowItems`, non-empty `KnownFor`, populated `time_narrative` / `money_narrative` / `co2_narrative`, and empty `payload.cta_rows` (postcards stay empty). **No `//go:build integration` tag** — that tag is reserved for live-AI tests per CLAUDE.md, and this exercise touches no AI provider (the test seeds a narrative row directly to avoid triggering live generation).
- [ ] **Narrative regeneration test.** Add a unit test that asserts `EnsureNarratives` is invoked (via a mock `aiProvider`) on self-view, confirming the strict-mirror posture extends to the background regeneration path. Use `storage.AssertMaxQueries` or a counter on the mock — don't make a real AI call.
- [ ] **Build verification.** Run `npm run build` and `go test ./server/services/profile/...`. Skip `go test -tags=integration ./...` for this change — neither `resolveSharedCommunities` nor `availableNowForTarget` nor `knownForForTarget` is an exported API consumed by integration tests, so per CLAUDE.md §Go Server Build Verification the integration build/test cycle isn't required.

**Validates:** A call to `GetUserProfileForViewer` with viewer == target returns the same shape of response as a call with viewer ≠ target (where the viewer shares all the target's communities) — populated rail, populated knownFor, populated narratives — with postcards as the one designed-empty exception.

### Phase 2: Client gate accepts self-view

**Goal:** `UserScreen` renders the Available Now rail on self-view when items are present. KnownFor chips and impact-row narratives need no client change — they already render unconditionally per the screen's own doc comment ([user_screen.dart:29-30](../../app/lib/presentation/screens/users/user_screen.dart#L29-L30)) and pick up the server's lifted suppression automatically.

**Tasks:**
- [ ] In [app/lib/presentation/viewmodels/viewer_profile_view_model.dart](../../app/lib/presentation/viewmodels/viewer_profile_view_model.dart), add `@Default(false) bool isSelfView` to the `ViewerProfileState` freezed class. Populate it inside `_load` by reading `authStateProvider` once (`final selfId = ref.read(authStateProvider).user?.id;`) and computing `isSelfView: response.targetUserId == selfId`. This keeps the screen pure and follows the existing repository-driven `_load` pattern in [docs/client/architecture.md](../client/architecture.md).
- [ ] In [app/lib/presentation/screens/users/user_screen.dart:96](../../app/lib/presentation/screens/users/user_screen.dart#L96), change the rail's gate from `if (state.hasSharedCommunities)` to `if (state.hasSharedCommunities || state.isSelfView)`. Do **not** change the lean-cards gate on line 102 — postcards stay other-view-only (server keeps `len(sharedRefs) > 0` gate; "you two" framing is incoherent for self-view).
- [ ] Update the `UserScreen` doc comment block ([user_screen.dart:16-33](../../app/lib/presentation/screens/users/user_screen.dart#L16-L33)) to call out self-view: the rail renders, knownFor + impact-row narratives continue to render unconditionally as they always did, and postcards stay empty (their gate is shared-communities, which is empty for self-view).
- [ ] **Verify no other client-side gate exists.** Audit `UserProfileKnownForChips`, `UserProfileImpactRows`, and the narrative rendering inside the impact rows. Confirm none of these widgets independently check `hasSharedCommunities` or `isSelfView` — the doc comment claims they're unconditional, and the fix relies on that claim. If a hidden gate is found, lift it the same way the rail gate is being lifted.
- [ ] **ViewModel test (primary, per [docs/client/testing.md](../client/testing.md) §Testing Philosophy):** add a `ViewerProfileViewModel` test in [app/test/presentation/viewmodels/viewer_profile_view_model_test.dart](../../app/test/presentation/viewmodels/viewer_profile_view_model_test.dart) using `ProviderContainer` with `authStateProvider` overridden. Two cases: viewer == target → `state.isSelfView == true`; viewer ≠ target → `state.isSelfView == false`. Mock `ProfileRepository` per the existing repo-test pattern in `app/test/data/repositories/`.
- [ ] **Widget smoke test (secondary):** add a widget test under `app/test/presentation/screens/users/` that wraps `UserScreen` in `ProviderScope` overrides and uses `app/test/helpers/l10n_helpers.dart` to resolve localizations. Assert on self-view with full server response: (a) rail renders, (b) knownFor chips render, (c) impact rows render with narratives, (d) lean cards do **not** render (postcards empty by design). Also assert on other-view with empty `sharedCommunities`: rail and lean cards both hidden.
- [ ] **No caching change:** `ProfileRepository` cache continues to key by `targetUserId`. The response shape gains additional fields populated for self-view at the existing cache key; existing TTL governs freshness. Existing `refresh()` path is unaffected.
- [ ] **No ARB / no a11y change:** neither `app_en.arb` nor `app_es.arb` is touched, and the affected widgets retain their existing accessibility semantics — the change is server-side suppression-lift plus one client gate-flip. (If a follow-up adds a self-view rail-header variant per the rail-header open question, both locales and any new `a11y*` keys must be added.)
- [ ] **Terminology note:** uses of "experience" in this plan refer to the code-level domain concept (`Experience`, `ExperienceTime`, `UPCOMING` mini-label) and are appropriate in internal docs. Per CLAUDE.md §UI Terminology, any new *user-visible* string introduced during this work must use **Event**, not Experience — but this plan introduces no new user-visible strings.

**Validates:** The bug reporter's exact path (Profile menu → My Profile) now shows their items, knownFor chips, and narratives, with no regression for other-view (which still gates the rail/postcards on shared communities).

### Phase 3: Manual verification + ship

**Goal:** Eyes-on confirmation that the fix matches the screenshot's expectation.

**Tasks:**
- [ ] Run the iOS simulator (or Android emulator per [ripls-app-logs](../../.claude/skills/ripls-app-logs)), sign in as a user with published, available gear and a non-trivial loan history, navigate Profile menu → My Profile.
- [ ] Confirm the Available Now rail renders with the user's published items.
- [ ] Confirm knownFor chips render with the user's capability tags (if any have been earned through completed loans).
- [ ] Confirm the impact-row narratives render (will populate once `EnsureNarratives` regenerates — may need a second open if the user has no pre-existing narrative row).
- [ ] Confirm tapping a rail tile routes correctly via `_openAvailableNowItem` ([user_screen.dart:114-124](../../app/lib/presentation/screens/users/user_screen.dart#L114-L124)) — should land on the gear / experience / request detail screen.
- [ ] Confirm no regression for other-view: navigate to another user's profile and verify the rail renders only when shared communities exist with relevant items, and postcards still render when applicable.
- [ ] Confirm postcards (`UserProfileLeanCards`) are still hidden on self-view — they should be, since the server's `len(sharedRefs) > 0` gate stays in place.

**Validates:** The fix is observable end-to-end and the design's other-view contract is intact.

### Phase 4: Update design-doc-of-record

**Goal:** The #1929 plan that established the shared-community gate is updated so the next contributor reading it sees the self-view strict-mirror posture.

**Tasks:**
- [ ] In [docs/issues/1929-profile-v4-redesign.md](1929-profile-v4-redesign.md), find the "Shared-community gating" decision under "Open Questions (Resolved)" and append an addendum: self-view (viewer == target) is a **strict mirror** of other-view with one principled exception (postcards). The Available Now rail and Known For chips source from the target's full active community membership rather than the empty viewer-target intersection; per-user AI narratives run unchanged (they have no community scope); postcards stay implicitly empty because their gate (`len(sharedRefs) > 0`) collapses out on self-view by construction, and the "you two…" CTA framing is incoherent for a single user. Link back to #1996 (this plan) so the rationale is retrievable.

**Validates:** Documentation hygiene — future readers of #1929 won't be surprised by the strict-mirror posture, and the rationale chain (#1929 decision → #1996 carve-out) is recoverable from the doc.

## Related Issues

- #1929 — Profile V4 redesign that introduced the shared-community gate this issue is bumping into. The plan doc explicitly resolved the gating decision without calling out self-view; this issue is the natural follow-up.
- #1889 — Earlier user-profile / Workshop work. Relevant because Option C above would resurface Workshop as the self-view canonical surface; #1889 has prior context on what Workshop is for.
- #1930 — Known For chips data source. Tangentially related — Open Question #3 leaves `knownFor` suppression on self-view alone, so #1930's eventual implementation can decide that question independently.
- #1931 — Per-user AI narratives. Same posture as #1930 — narratives stay suppressed on self-view per Open Question #3; #1931 owns any future change to that.
