---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: The viewer-facing user profile screen (v11 synthesis chassis) — photo/monogram hero, shared-community subsetting, the presence-driven pulse pill and Message/Plans/Library action row, the aggregate hero-stat + metric ledger, and the privacy invariants that keep non-shared community IDs off the wire and out of logs.
  globs: [server/services/profile/**, app/lib/presentation/screens/users/**, app/lib/services/profile_service.dart, app/lib/data/repositories/profile_repository.dart, app/lib/presentation/viewmodels/viewer_profile_view_model.dart]
  triggers: [profile, viewer-profile, shared-community, postcards, privacy, hero, ticker]
  lens: [client, server, domain]
  domain: profile
  # Moved from docs/users/ to the top level in #2953: this is a feature doc for
  # the profile surface, not one of the product-strategy docs that directory
  # otherwise held.
freshness:
  verified_commit: "69c4ab218"
  verified_on: "2026-07-19"
---
# User Profile (Viewer-Facing)

The user profile is rendered when one user (the *viewer*) taps another user's name or avatar. [`ContentViewHelpers.openUserScreen`](../app/lib/presentation/widgets/content/content_view_helpers.dart) routes *every* user — including the viewer themselves — to the same [`UserScreen`](../app/lib/presentation/screens/users/user_screen.dart). When the viewer is the target (self-view), the shared-groups line is hidden and the target's own overflow menu (edit name) replaces the viewer-facing action row's external destinations; see [`docs/issues/1996-self-view-items.md`](issues/1996-self-view-items.md).

## Surface

The screen is built on the **v11 synthesis chassis** ([`docs/issues/2568-directory-and-profiles.md`](issues/2568-directory-and-profiles.md), which superseded the v4 redesign at [`docs/issues/1929-profile-v4-redesign.md`](issues/1929-profile-v4-redesign.md)): the target's photo fills the top ~46% under a scrim dissolving into a dark page, or a circular-monogram masthead when they have no photo (same body either way — no layout shift when a photo arrives). Below it, one scrolling column, top to bottom:

1. **Identity** — an eyebrow ("What {name} brings", or a self/cold-start variant), the serif display name, and a one-line description sentence.
2. **Shared-groups line** — "With you in {crew} +N", naming the first shared community with a small face-stack. Hidden on self-view or when the viewer shares no community with the target.
3. **Signature tags (Known For chips)** — short capability tags derived from the target's completed gear loans, clamped and expanding in place. Renders whenever the target has any — unconditional, since it describes the target, not the relationship.
4. **Live-pulse pill + action row** — "Next up: {event}" when the presence read (see *Presence sheet* below) resolves to an upcoming shared event, then an inline circular action row: **Message** (primary — opens the most recently shared community's conversation inline; hidden when nothing is shared), **Plans** (opens the shared calendar), **Library** (opens the target's available-now items). Each action expands its destination in place via `openContentMorphPanel` rather than navigating to a new route.
5. **Hero stat + quiet ledger** — one editorial "Time together" stat (hours, plus a tiered equivalence sentence) over four metric tiles: Problems solved, Money saved, Library, Plans. Numbers are **target-aggregate** (not scoped to the intersection); each tile's subtext states the shared-communities scoping in plain language instead. Library and Plans tiles drill down in place; Problems-solved and Money-saved are inert (see *Deferred* below). The whole block is hidden on a cold-start connection (a brand-new pair with a single shared upcoming event and no history), via the presence read's `suppressHistory`.

The v4 "Keep It Going" postcard section (lean cards routed through `UserProfileLeverDispatcher`) is no longer rendered on this screen — the Message/Plans/Library action row replaced it as the viewer's primary lever.

### Presence sheet

A companion RPC, `ProfileService.GetProfilePresenceForViewer` (server: [`presence.go`](../server/services/profile/presence.go); client: [`profile_sheet_view_model.dart`](../app/lib/presentation/viewmodels/profile_sheet_view_model.dart)), selects the pinned state that feeds the pulse pill and the cold-start gate: the target's most pressing open ask in a shared community → the next upcoming event both are RSVPed to → quiet. Self-view always resolves to quiet (there's no ask to commit to on your own profile). A read failure also resolves to quiet rather than surfacing an error — the profile stays fully usable without the sheet's dynamic content. Every card, face, and count in the response is scoped to communities shared with the viewer, consistent with the *Privacy invariants* below.

## Privacy invariants

These hold both on the wire and in server log fields:

- The aggregate impact-row numbers may reflect activity inside communities the viewer is not in; the *names* and *IDs* of those communities must not.
- The response carries an `other_community_count` integer but never an accompanying list of non-shared community IDs.
- No postcard ever references an entity inside a non-shared community.
- Log entries emitted by [`ProfileService`](../server/services/profile/) must not carry non-shared community IDs (logs are searchable).

Tests in [`profile_test.go`](../server/services/profile/profile_test.go) and [`postcards_test.go`](../server/services/profile/postcards_test.go) pin these invariants via wire-byte leakage assertions.

## Client architecture

- **Service**: [`profile_service.dart`](../app/lib/services/profile_service.dart) — thin Connect client over `ProfileService.GetUserProfileForViewer` and `GetProfilePresenceForViewer`.
- **Repository**: [`profile_repository.dart`](../app/lib/data/repositories/profile_repository.dart) — caches `GetUserProfileForViewer` responses under namespace `'profile'`, key format `profile:<target_user_id>`. Joining or leaving a community must invalidate the entire `profile:*` namespace because the viewer's membership set governs *every* potential profile's shared-community subset. `getPresence` is deliberately uncached — the pinned sheet must reflect commits (offers, RSVPs) immediately, and a stale ask card would prompt the viewer to commit to something already resolved.
- **View models**: [`viewer_profile_view_model.dart`](../app/lib/presentation/viewmodels/viewer_profile_view_model.dart) — `AsyncNotifierProvider.autoDispose.family` keyed by target user id; exposes typed `SharedCommunity` lists, never pre-formatted strings (i18n lives in the widget). `profile_sheet_view_model.dart` is a sibling `AsyncNotifier` (`profileSheetProvider`) mapping the presence RPC onto the `ProfileSheetState` union (`QuietSheet` / `ActiveAskSheet` / `NextEventSheet` / `ColdStartSheet`); it is shared with the community profile screen.
- **Screen**: [`user_screen.dart`](../app/lib/presentation/screens/users/user_screen.dart) — the v11 layout described above, composed from `profile_section/` widgets (`ProfileHero`, `ProfileHeroStat`, `ProfileMetricRows`, `ProfileChips`, `ProfileActionRow`, `ProfilePulsePill`, `ProfileMemberRow`, `ProfileConversationPanel`, plus the photo/monogram backdrop pair) rather than the retired `user_profile/` widgets (`UserProfileIdentity`, `UserProfileLeverDispatcher`).

## Deferred (v1)

- **Problems-solved / Money-saved drill-downs** — tapping either of these two ledger tiles is intentionally inert (Library and Plans already drill down in place). Wiring them to scoped versions of the community impact detail screens would require parameterising those screens with a `target_user_id` filter on `GetCommunityMetricDetailRequest` and adding a per-record anonymization layer for rows that belong to non-shared communities. Tracked in [`docs/issues/1889-user-profile-workshop.md` §Phase 3.5](issues/1889-user-profile-workshop.md).
