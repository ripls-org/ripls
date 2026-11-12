# Issue #1547: Can't offer help on request

**Source:** <https://github.com/Type2Fun-Foundation/ripls/issues/1547>
**Filed:** 2026-04-28T18:54:56Z | **Last activity:** 2026-04-28T18:54:56Z
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

user2@example.org tapped a notification that opened a "new request" detail
screen, then tapped "Offer help". The app showed:

- Full-screen error: "Failed to load request" / "Failed to create offer" / Retry
- Snackbar: "Failed to offer help: The requested item could not be found."

He retried four times within ~50 seconds (18:53:22 → 18:54:11). The bad
state survived a full app quit/reload — the notification took him right back
to the same request, with the same failure. Roughly 1h45m later (20:36:01)
he successfully offered help on the **same request** by reaching it through
some other path.

### Server perspective

Prod logs show the OfferToFulfill RPC returning **404 NotFound** for that
request. The server is correct: the client sent a `community_id` the
request was never shared with.

- Request `00000000-0000-4000-8000-000000000002` was created by
  le***@gmail.com in community `00000000-0000-4000-8000-000000000003` at
  18:52:36 (about 45s before bryan's first attempt). No subsequent
  ShareRequest call.
- Bryan's four failing OfferToFulfill calls all sent
  `community_id=00000000-0000-4000-8000-000000000004` — a different
  community he is also a member of.
- `server/services/request/offers.go:75-85` queries `CommunityRequest` by
  `(request_id, community_id)` and returns `CodeNotFound("request not found
  in this community")` when there's no match.
- The 20:36 successful call sent the correct `community_id=a1611f6f...`.

### Client perspective: the actual root cause

Single-select state was migrated to a multi-select model, but the
notification-deep-link path was not updated. The FCM flow tries to "switch"
to the notification's community by calling
`SelectedCommunityNotifier.selectCommunity(community)`
(`community_providers.dart:171`):

```dart
Future<void> selectCommunity(CommunityItem community) async {
  final enabledIds = Set<String>.from(state.enabledCommunityIds)
    ..add(community.id);                       // adds, never removes
  state = state.copyWith(enabledCommunityIds: enabledIds);
  ...
}
```

The derived `selectedCommunity` getter (line 114) returns the **first**
enabled community in the `communities` list:

```dart
CommunityItem? get selectedCommunity {
  if (enabledCommunityIds.isEmpty || communities.isEmpty) return null;
  for (final c in communities) {
    if (enabledCommunityIds.contains(c.id)) return c;
  }
  return null;
}
```

So when `main.dart::_switchToCommunity('a1611f6f')` runs:

1. It adds `a1611f6f` to `enabledCommunityIds` — but `b42c4991` is still in
   the set.
2. `selectedCommunity` walks `communities` in list order. If `b42c4991`
   appears before `a1611f6f`, the getter still returns `b42c4991`.
3. `RequestScreen.initState` reads
   `selectedCommunityProvider.selectedCommunity?.id` (line 57-58) and pins
   it as the community context for the request screen and all its child
   view models.
4. `OfferToFulfill` is called with `b42c4991`. Server 404s.

`enabledCommunityIds` is persisted to `SharedPreferences`
(`_persistEnabledIds`), which is why an app quit/reload didn't fix it —
both communities stayed enabled, and `b42c4991` kept winning the
"first in list" race.

The 20:36 success is consistent with this story: by then bryan had either
deselected `b42c4991`, navigated through a UI flow that wrote a single-id
selection, or the request was reached via a path that overrode the screen's
community context — any of which makes `selectedCommunity` resolve to
`a1611f6f`.

## Context from Comments

The issue has no GitHub comments — it was filed via the in-app feedback
flow with a screenshot. The user provided additional context in chat:
notification deep-link entry; bug persisted across app quit; later success
was on the same request.

## Potential Solutions

### Option A: Make `_switchToCommunity` a real switch (single-select replace)

**Approach:** When a notification deep link asks to switch to a community,
*replace* the enabled set rather than adding to it — at least when the user
is using the legacy single-select UX. Add a
`SelectedCommunityNotifier.switchTo(community)` method that sets
`enabledCommunityIds = {community.id}` and persists it. Have
`main.dart::_switchToCommunity` call that.

- **Pros:** Fixes the bug at its actual root. After this, taps on a
  notification reliably set the deep-link community as the active context.
  Easy to test.
- **Cons:** Subtly changes notification UX for users who currently rely on
  multi-select feeds — a tap on a notification would *narrow* their
  selection. Might surprise power users. Mitigation: add a secondary
  `switchToWithoutDeselecting` if we decide the wide-feed behavior is
  actually desired, but don't ship that until someone asks.
- **Effort:** small.

### Option B: Defense in depth in the request view model

**Approach:** In `request_view_model.dart::loadRequestDetails`, after
`getRequest` returns, validate that `state.communityId` is present in
`response.sharedCommunities`. If not, replace it with the first shared
community. Apply the same guard in any other RPC that takes
`(requestId, communityId)`.

- **Pros:** Catches *any* upstream miswiring of community context, not just
  the FCM bug. Cheap. A separate test file can pin the invariant.
- **Cons:** Doesn't fix the underlying bug — a user who navigates to a
  request via deep link will still see "their" feed flicker to a community
  the request isn't actually in. Treats the symptom.
- **Effort:** small.

### Option C: Make `community_id` optional on OfferToFulfill (server)

**Approach:** Allow the client to omit `community_id`. If absent, server
picks a single eligible community (request is shared there + user is a
member). If multiple are eligible, return `FailedPrecondition` with the
list so the client can prompt.

- **Pros:** The cleanest API: the user is offering help on a *request*, not
  on a *(request, community)* tuple. Removes the entire class of "wrong
  community" failures.
- **Cons:** Bigger change — proto + server + every client call site +
  tests. New ambiguity when a request is shared across several of the
  user's communities. Doesn't fix the fact that the request screen still
  *displays* under the wrong community.
- **Effort:** medium.

## Recommendation

Ship **A and B together**. A is the actual fix; B is one-line insurance
that closes off any other path that might still leak a stale community id
into per-request RPCs.

Skip C unless we hit it again in a different shape. The proto change isn't
worth it for a one-off bug now that A removes the immediate cause.

## Open Questions

- **Should `_switchToCommunity` replace or merge?** — Recommendation:
  replace. Notification taps are explicit "go here now" gestures; the user
  expects the UI to follow. If multi-select feed users complain, add an
  opt-in setting later. The downside of merging (stale context, this bug)
  is worse than the downside of replacing (one extra toggle for power
  users).
- **Should we surface the auto-correction in the UI when option B fires?**
  — Recommendation: silent in the offer flow, but emit a `_log.warning`
  including both the original and corrected community ids. After a few
  days of prod data we'll know whether B is catching anything that A
  doesn't.
- **Is the FCM payload actually carrying the right `community_id`?** —
  Recommendation: verify in the next prod test. The chat-message and
  community-event paths in `server/notifications/fcm/provider.go` both
  populate `community_id` from `conversation.CommunityId` /
  `CommunityEvent.CommunityId`, which for this request would be
  `a1611f6f`. If a payload is going out without it, that's a separate bug
  worth fixing — but the in-app multi-select issue still bites whenever
  the user has another community in the enabled set.

## Implementation Plan

### Phase 1: Fix the root cause in selected-community state

**Goal:** Notification deep links unambiguously make their target
community the active one.

**Tasks:**

- [ ] Add `SelectedCommunityNotifier.switchTo(CommunityItem community)` in
      `app/lib/services/providers/community_providers.dart` that sets
      `enabledCommunityIds = {community.id}` and persists. Keep
      `selectCommunity` (additive) for the multi-select picker UI which
      legitimately wants to add without removing.
- [ ] Update `main.dart::_switchToCommunity` to call `switchTo` instead of
      `selectCommunity`. Keep the no-op short-circuit
      (`if (state.selectedCommunity?.id == communityId) return;`).
- [ ] Notifier unit test (primary check, per `architecture.md`'s testing
      guidance — ViewModels are pure Dart and tested with
      `ProviderContainer`): pre-seed `enabledCommunityIds = {idB}`, call
      `switchTo(communityA)`, assert `enabledCommunityIds == {idA}` and
      that the persisted value matches.

**Validates:** Tapping a notification routes to the correct community
context, even when the user has other communities enabled in their feed.

**Out of scope here, worth a follow-up:** `app_router.dart:392` (invite
acceptance) and `governance_screen.dart:257` (post-create) both call
the additive `selectCommunity` where the semantic intent is also "make
this the active one." Migrating them to `switchTo` is a small,
independent cleanup; bundle it only if it lands the same day.

### Phase 2: Defensive validation in request view model

**Goal:** Per-request RPCs never send a community the request isn't shared
with — a single guard in the load path makes every downstream RPC safe.

**Tasks:**

- [ ] In `request_view_model.dart::loadRequestDetails` (around line 174),
      replace the raw `??` fallback with logic that validates
      `state.communityId` against
      `response.sharedCommunities.map((c) => c.communityId)` and falls
      back to the first shared community when not present. Log a
      `_log.warning('communityId not in sharedCommunities; correcting',
      original: ..., corrected: ...)` when the guard fires.
- [ ] **Do not** duplicate the guard inside `request_edit_actions.dart`,
      `request_helper_view_model.dart`, or `request_offer_view_model.dart`
      — they all read `state.communityId` after the load, so a single
      correction at the load site covers them. (Earlier draft of this plan
      had the guard in three places; that was redundant.)
- [ ] Notifier unit test in `app/test/presentation/viewmodels/`: load a
      request whose `sharedCommunities` doesn't contain the initial
      `state.communityId`, assert post-load `state.communityId` matches a
      shared community. Mock the repository per `caching.md` §Testing.
- [ ] Notifier unit test for the offer flow: with a corrected
      `state.communityId`, verify `requestRepository.offerToFulfill` is
      called with the corrected id (using mockito's `verify`).

**Architecture compliance notes:**

- No new user-facing strings — the existing `Code.notFound` localization
  in `rpc_error_handler.dart` keeps covering any residual NotFound. No
  ARB updates needed (per `i18n.md`).
- No repository or cache changes. The data needed (`sharedCommunities`)
  is already on the `Request` response that `RequestRepository.getRequest`
  returns.
- Validation lives in the ViewModel layer, which `architecture.md` lists
  as the correct home for business logic that depends on response data.

**Validates:** OfferToFulfill no longer 404s on community mismatch under
any code path that goes through `RequestNotifier`.

### Phase 3: Server-side error clarity (small, optional)

**Goal:** When a 404 of this shape *does* happen, the log line tells us
exactly what the mismatch is.

**Tasks:**

- [ ] In `offers.go:84`, log a warning before returning the 404 with the
      list of communities the request *is* shared with, the user's
      memberships, and the supplied `community_id`. Tag with
      `mismatch=community`.

**Validates:** Future occurrences are diagnosable in one log query rather
than multiple cross-referenced ones.

## Related Issues

- #1548 — Logging hygiene: duplicate `request_id` slog attribute causes
  Cloud Logging to concatenate two UUIDs into one ~72-char string.
  Surfaced during this investigation but unrelated to the user-visible
  bug; tracked separately so this PR stays focused.
