# Issue #1705: Multi-community items: client passes single communityId without verifying user membership; multiple surfaces fail with permission_denied or leak data

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1705
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** none
**Audit pass:** 2026-05-04 — see [Audit Notes](#audit-notes) for what changed and why.

## Problem Statement

Items (gear, requests, experiences) can be shared into multiple communities. Many client-side RPCs scope by a single `community_id` taken from local UI state — typically `state.communityId`, "the community the user is currently viewing through." When the item is in communities {A, B, C} and the user is a member of only one of them (say B), but local state's `communityId` happens to be A, scoped reads fail because the server enforces "user must be a member of *that specific* community" via `RequireMemberOfActiveCommunity`.

Three observed failure modes:

1. **Hard fail with `permission_denied`** that the client doesn't handle — silent UI drop or stuck spinner (e.g. #1667, #1676 member-list spinner).
2. **Mutation succeeds, post-mutation refresh fails** — UI never reflects the server-side change, leading to repeated user actions and duplicate writes (#1684, #1685 borrow-N-times). Crashlytics records the rethrow as a fatal exception (#1703).
3. **Data leakage** — *summary* queries (e.g. unread counts) don't enforce per-community membership and return data from communities the user isn't in, then *detail* queries do enforce and reject (#1665 chat tab shows "2 unread" but loading the chat fails permission_denied).

The server is not at fault: the strict membership check is a deliberate authorization invariant. The bug is on the client side, but the same pattern repeats across many surfaces, so a structured audit + fix is required, not just one-off patches.

## Context from Comments

One comment (mine, after a sweep of recently filed issues):

- Three more instances added to the umbrella: #1666 (invite-from-non-member-community), #1667 (member-list spinner), #1676 (sees other groups + spinner on expand). All fit the same pattern.
- Near-miss called out: #1680 ("duplicate inbox items for multi-community gear") was thematically similar but root-caused differently — server-side fan-out without dedup. Already fixed by maintainer in PR #1687 (`server/services/portfolio/inbox_feed.go` two-pass dedup). **Confirms the multi-community theme spans multiple distinct bug classes; this umbrella is specifically the client-scoping subset.**
- Defensive-UI option revised: when a member-required scoped read fails, surface an error and stop the loading indicator rather than spin forever (the #1667/#1676 sub-symptom).
- Confirmed not pre-empted by today's merges (#1683, #1687, #1688, #1691, #1696, #1700, #1651) — none touch the scoped-read endpoints in scope here.

## Codebase Inventory

**Server endpoints calling `RequireMemberOfActiveCommunity` (the bug class — strict per-community membership check):**

Confirmed by grep across `server/services/`:

| File:line | Surface | Notes |
|---|---|---|
| `server/services/transfer/context.go:45` | `GetGearTransferContext` | **Confirmed bug** (#1703 stack trace) |
| `server/services/transfer/queries.go:399` | Transfer list/queries | Probably same client mismatch |
| `server/services/transfer/create.go:75` | `CreateTransfer` | Mutation — usually has the right communityId |
| `server/services/chat/conversations.go:185, 641` | Chat conversation lookup/create | **Likely #1665 root cause** |
| `server/services/community/invitations.go:38, 167` | `InviteToCommunity` | **Likely #1666 root cause** |
| `server/services/community/membership.go:118` | List members | **Likely #1667/#1676 root cause** |
| `server/services/community/gear_sharing.go:40, 207, 354` | Share gear | Mutation surfaces |
| `server/services/community/events.go:38` | Community events | |
| `server/services/community/streaming.go:48` | Community streaming | |
| `server/services/community/past_giveaways.go:37` | Past giveaways | |
| `server/services/community/notification_prefs.go:34, 69` | Per-community notification prefs | |
| `server/services/community/shadow_users.go:37, 82, 121, 168, 256` | Shadow users (5 sites) | |
| `server/services/experience/get.go:46` | Get experience (community-scoped) | |
| `server/services/experience/share.go:51` | Share experience | Mutation |
| `server/services/experience/rsvp.go:55` | Create RSVP | Mutation; sibling of borrow flow |
| `server/services/experience/people.go:60` | Experience people list | |
| `server/services/experience/stats.go:58` | Experience stats | |
| `server/services/feed/service.go:1113` | Feed | |
| `server/services/feed/stories.go:38` | Stories | |
| `server/services/request/queries.go:139` | Request queries | |
| `server/services/request/lifecycle.go:54` | Request lifecycle | Mutation |
| `server/services/request/sharing.go:75` | Share request | Mutation |
| `server/services/request/offers.go:71` | Request offers | |

**Client repositories that pass `communityId`:**

`app/lib/data/repositories/`: `transfer_repository.dart`, `gear_repository.dart`, `experience_repository.dart`, `chat_repository.dart`, `feed_repository.dart`, `shadow_user_repository.dart`, `unread_count_repository.dart`, `search_repository.dart`.

The bug surfaces wherever a repository call's `communityId` argument is sourced from `state.communityId` (or analogous local state) for an item that may be in multiple communities.

## Confirmed asymmetry (Gear / Transfer)

- `ExpressInterest` (`server/services/transfer/interest.go:22-88`): does not take `community_id`; auto-picks via `findMatchingCommunityAndType` (line 48) based on user membership. Returns the chosen `community_id` in `Transfer.CommunityId` (set at line 145).
- `GetGearTransferContext` (`server/services/transfer/context.go:19-74`): requires `community_id`; calls `RequireMemberOfActiveCommunity` at line 45.
- Client (`app/lib/presentation/viewmodels/gear_view_model.dart:838-841`): passes `state.communityId` — wrong for multi-community gear when local state's communityId differs from the user's actual member community.

The mutation response already carries the server-authoritative communityId; the client just isn't using it.

## Potential Solutions

Pulled from issue body and refined with codebase research:

### Option A: Per-surface client fix — thread server-authoritative communityId through

For mutation+refresh pairs, use the `community_id` returned by the mutation response in the followup read instead of `state.communityId`. Maintains the server's strict-membership invariant.

- **Approach:** Audit each mutation+refresh client flow; replace `state.communityId` with the mutation response's `community_id`.
- **Pros:** Surgical, no protocol change, no security model change.
- **Cons:** Per-surface; doesn't help pure read paths (no preceding mutation to crib from). Misses the data-leakage class (#1665 unread count) entirely.
- **Effort:** Small per surface; medium across all mutation flows.

### Option B: Defensive-UI guard layer (orthogonal, complementary to A)

Two protections, applied wherever a viewmodel does mutation-then-refresh or scoped-read-with-spinner:

1. **Optimistic update on mutation success:** even if the post-mutation refresh fails, reflect the mutation in local state from the response. Prevents repeated taps creating duplicate writes (#1684, #1685).
2. **Failed-read error surfacing:** when a scoped read returns `permission_denied` or any error, stop the spinner and show an error UI. Prevents indefinite spinners (#1667, #1676).

- **Approach:** Generalize as viewmodel utilities or a `ScopedRead<T>` wrapper. Apply across `gear_view_model.dart`, `experience_creation_modal`, etc.
- **Pros:** Catches the worst user-visible symptoms even when Option A doesn't apply or hasn't shipped yet. Handles the spinner-forever class.
- **Cons:** Doesn't fix the underlying scoping bug — the call still fails, the user just sees a graceful error or correct state instead of silent breakage.
- **Effort:** Medium. Lifting common patterns into shared helpers is the bulk of it.

### Option C: Server-side relaxation — accept "any community the resource is in where user is a member"

For the read endpoints in scope, replace `RequireMemberOfActiveCommunity(req.CommunityId, userID)` with logic that:
1. Loads the resource's set of communities.
2. Confirms the user is a member of *at least one* community in that set.
3. Either ignores `community_id` in the request, or uses it only as a hint (validating it's in the resource's set and the user is in it; falling back to any valid one).

- **Approach:** Per-endpoint server change with security review.
- **Pros:** Pattern-level fix; future-proof; pure read paths covered without a dance.
- **Cons:** Larger change. Per-community visibility scoping (e.g. an experience showing per-community RSVPs) loses its scoping argument. Auth model change requires per-endpoint analysis: which endpoints need *any-community* read vs. *specific-community* read?
- **Effort:** Medium per endpoint; large in aggregate. Security-review gate.

### Option D: Audit-first, fix-as-classified

Inventory every server endpoint that calls `RequireMemberOfActiveCommunity` (already started above — 30+ sites) and every client call passing `communityId`. Classify each:
- **(a)** Membership-required, client always has a member-correct communityId → no action.
- **(b1)** Membership-required, client may pass wrong communityId, mutation pair available → Option A.
- **(b2)** Membership-required, client may pass wrong communityId, no mutation pair → Option C.
- **(c)** Should not be membership-required (e.g. summary/aggregation queries) → server relaxation, possibly with new "any membership" auth helper.

- **Approach:** Document the audit table, then schedule the per-class fixes.
- **Pros:** Captures the full scope, prevents whack-a-mole.
- **Cons:** Audit is upfront cost; not a single-PR fix.
- **Effort:** Medium for audit, then sums of the per-surface classifications.

## Recommendation

**Land in this order:**

1. **Phase 1 (immediate user impact):** Option A for the gear borrow flow + Option B's optimistic-update guard locally on that flow. Closes #1684, #1685, half of #1703 (the upstream cause of the Crashlytics fatal).
2. **Phase 2 (broaden defensive UI):** Option B as a generalized pattern across viewmodels — optimistic update on mutation, and failed-read error surfacing. Closes #1667, #1676 spinner-forever class, and broadly improves resilience to any scoped-read failure.
3. **Phase 3 (audit + classify):** Option D — full inventory and classification of all `RequireMemberOfActiveCommunity` sites and their client callers. Output is a per-surface fix plan.
4. **Phase 4+ (per-class fixes):** Per Phase 3 findings, land Option A for category (b1) and Option C for category (b2)/(c). Includes the chat surfaces (#1665 — both the load-fails half and the unread-count data-leakage half) and the member-list / "who can see" surfaces (#1666, #1667, #1676).

Rationale: Phase 1 is a small, surgical PR with high user-visible impact and low risk — closes the loudest reported instances. Phase 2 is a refactor that pays back across the codebase regardless of which specific scoping bugs exist. Phase 3 is the audit needed to scope the rest of the work. Phases 4+ fall out of the audit and ship per-surface, allowing parallel work.

## Open Questions

- **Q1: Should `Phase 1` change the `expressInterest` client wrapper signature, or thread the response value within the viewmodel only?** Recommendation: keep the wrapper signature stable, return the response, and let the viewmodel pull `result.transfer.communityId` from the response. Avoids spreading the change across all callers; minimal blast radius.
- **Q2: Should the optimistic update in Phase 1 fully reconstruct `transferContext` from the mutation response, or just toggle a flag like `hasUserTransfer = true`?** Recommendation: full reconstruct — set `userTransfer` from `result.transfer` and recompute `availableActions` accordingly. A flag-only update would cause UI inconsistency (action menu still shows "Borrow"). The mutation response carries everything needed.
- **Q3: For Phase 2's failed-read pattern, do we want a shared helper (e.g. a Notifier mixin) or per-viewmodel try/catch?** Recommendation: a Notifier mixin (e.g. `ScopedReadMixin`) built on top of the existing `SafeNotifierMixin` (`app/lib/core/utils/safe_notifier.dart`). Keeps `safeUpdateState` semantics end-to-end and respects the no-`BuildContext`-in-business-logic rule from `docs/client/architecture.md` §5. Reduces per-call boilerplate; makes the pattern enforceable.
- **Q4: Does the Phase 3 audit need to enumerate all 30+ server endpoints, or just the user-visible scoped reads?** Recommendation: enumerate all, but prioritize the user-visible scoped reads first (chat, members, gear-context, experience-detail). Background/internal endpoints can be deferred unless they show up in Crashlytics.
- **Q5: For the chat unread-count data leak (#1665), is the leak server-side (query returns rows it shouldn't) or client-side (client subscribes to all communities including non-member)?** Recommendation: trace from `unread_count_repository.dart` and the corresponding server query. Likely a missing JOIN to `community_user` membership in the unread aggregator. Owner-of-find-out is part of Phase 4-chat. Worth filing as its own sub-issue once located.
- **Q6: For Option C cases, do we add a new `auth.RequireMemberOfAnyCommunityFor(resourceID)` helper, or modify call sites individually?** Recommendation: helper. The pattern recurs (we'd need it for chat, members, gear context, etc.) and centralizing the "load resource → check user is in any of resource's communities" logic improves consistency and testability.
- **Q7: How do we close the instance issues without leaving orphaned ones?** Recommendation: tag each instance with `umbrella:1705` (or similar), close as their corresponding phase ships. #1684/#1685/#1703 close after Phase 1. #1667/#1676 close after Phase 2. #1665/#1666 close after Phase 4-chat / Phase 4-members.

## Implementation Plan

### Phase 1: Fix gear borrow flow (Option A + local Option B)
**Goal:** Close #1684 and #1685 user-visible behavior; eliminate the Crashlytics fatal source for `getGearTransferContext` (Bug A in #1703).

**Layering note (per `docs/client/architecture.md`):** changes are in the ViewModel layer (`gear_view_model.dart`); the repository wrapper for `expressInterest` (`transfer_repository.dart`) needs to return the response, not just `void`, but its own logic and cache invalidation stay in the repository. ViewModels never call services directly.

**Tasks:**
- [ ] In `app/lib/data/repositories/transfer_repository.dart`, change `expressInterest` to return the `ExpressInterestResponse` (or at minimum the resulting `Transfer` so the viewmodel can read `communityId`). Preserve any existing repository-side cache invalidation in the same method — repositories own caching.
- [ ] In `app/lib/presentation/viewmodels/gear_view_model.dart:825-854`, change `expressInterest()` to:
  - Capture the response from the new return value.
  - Use `response.transfer.communityId` for the followup `getGearTransferContext` call instead of `state.communityId`.
  - On mutation success but refresh failure: optimistically construct a `transferContext` reflecting the new transfer (so the button shows "in line" and the user can't double-tap).
  - Continue using `safeUpdateState` (already in this file at line 842, 851) for all state writes after async gaps. Preserves the existing async-safety contract; do not regress to bare `state = ...`.
  - In the catch block, classify the error via `RpcErrorHandler.classify(e)` and store as `UserError? error` on state per the typed-error convention (`docs/client/i18n.md`). Do not store a raw string. The widget layer resolves the message via `RpcErrorHandler.localize(state.error!, context.l10n)`.
- [ ] Mirror the change in `withdrawInterest()` (currently at line 858+) and any other gear viewmodel methods that follow the mutation+refresh pattern with `state.communityId`.
- [ ] Add a ViewModel unit test using `ProviderContainer` + Mockito mocks (per `docs/client/testing.md` Pattern 1): borrow → followup `getGearTransferContext` throws `permission_denied` → state still reflects the transfer; `userTransfer` is set; second tap is a no-op or surfaces "already in line."
- [ ] Add a disposal test (per `docs/client/architecture.md` Disposal Testing section): borrow → dispose container before followup completes → no crash.
- [ ] Verify on a real device with a multi-community gear (the Liberty Puzzle scenario from #1665).

**Validates:** The threading-server-communityId fix works for the canonical case; optimistic UI prevents the duplicate-borrow symptom even when the read fails. Async-safety contract preserved.

### Phase 2: Generalize defensive UI patterns — **DEFERRED**
**Status:** Not pursued at this time. Decision 2026-05-04.

**Why:** A scoping audit of the candidate surfaces (gear borrow flows, ManageMembers, CommunityContent, AccessSheet, GearSharingAudience, InviteSheet) showed only **one** real "spinner forever" instance: AccessSheet (`access_sheet.dart:183`) swallows fetch errors. The other notifier-based viewmodels (`ManageMembersNotifier`, `CommunityContentViewModel`) already classify errors via `RpcErrorHandler` and clear loading flags correctly — any user-visible spinner-forever from those is a UI binding bug, not a viewmodel bug. With only one shared call site beyond the gear flow, generalizing the Phase 1 try/catch into a typed two-mixin abstraction is more abstraction than the codebase needs (per project guidance: "Don't add abstractions beyond what the task requires").

**When to revisit:** If/when Phase 4-chat or Phase 4-other lands a third call site with the same shape (mutation+refresh with optimistic fallback, or scoped-read with typed-error storage), extract the pattern. Until then, hand-rolled Phase 1 pattern is fine.

### Phase 2 (original, retained for reference): Generalize defensive UI patterns
**Goal:** Close #1667 / #1676 (spinner-forever) class. Broadly improve any scoped-read or post-mutation-refresh resilience.

**Layering note:** these helpers are utilities/mixins on Notifiers — explicitly not "Controllers" (per `docs/client/architecture.md` §5: "There are no controllers"). Prefer composing on top of the existing `SafeNotifierMixin` (`app/lib/core/utils/safe_notifier.dart`) so `safeUpdateState` semantics are preserved end-to-end.

**Tasks:**
- [ ] Add a Notifier mixin (or extension) — e.g. `ScopedReadMixin` — that wraps a scoped read with: (a) start: set `isLoading` via `safeUpdateState`; (b) success: apply payload via `safeUpdateState`; (c) failure: classify via `RpcErrorHandler.classify`, store as `UserError? error`, clear `isLoading`. Built on top of `SafeNotifierMixin` rather than parallel to it. Free functions over `BuildContext` are not appropriate here — business logic stays in ViewModels.
- [ ] Generalize the optimistic-update pattern from Phase 1 as a sibling helper: `mutateOptimistic({mutate, applyToState, refresh, classify})`. Use in any viewmodel doing mutation-then-refresh (RSVP create, request offer, gear share, etc.). Aligns with the "Optimistic Updates" pattern documented in `docs/client/architecture.md` lines 614-645.
- [ ] Audit viewmodels for the "spinner that depends on a single scoped read" pattern (`grep -nE "isLoading|isSaving" app/lib/presentation/viewmodels/`) and migrate the riskiest to the new mixin.
- [ ] Widget layer: ensure error UIs surfacing the new `UserError` use the existing `ContentErrorBanner` (`app/lib/presentation/widgets/content/content_error_banner.dart`), which already wraps in `LiveRegion` for screen-reader announcement (per `docs/client/accessibility.md`). Resolve message via `RpcErrorHandler.localize(error, context.l10n)` in `build()`. No raw string error messages on viewmodel state — typed `UserError` only.
- [ ] All new accessibility-visible strings (e.g. error labels, retry button labels) get keys in `app/lib/l10n/app_en.arb` and `app_es.arb` with the `a11y` prefix where they label semantic actions, per `docs/client/i18n.md` and `docs/client/accessibility.md`. The `require_l10n_for_semantic_labels` lint rule will catch raw literals.
- [ ] Tests: ViewModel unit test with `ProviderContainer` — scoped read returns `permission_denied` → `isLoading` cleared, `error` is a `UserError.serverMessage(code: permissionDenied)`, state otherwise untouched. Disposal test for the mixin.
- [ ] Run `npm run lint:dart:i18n` and `npm run lint:dart:a11y` — both must remain clean.

**Validates:** The generalized pattern catches future scoping bugs as graceful errors instead of silent breakage. Closes the spinner-forever class. Async-safety, i18n, and a11y contracts preserved.

### Phase 3: Audit and classify scoped-read sites
**Goal:** Output a per-surface fix plan; nothing ships in this phase, but Phase 4+ depends on it.

**Layering note (per `docs/server/architecture.md`):** the new auth helper proposed below lives in `server/auth/` (alongside `community_active.go`), not inside any specific service. Services depend on shared libraries; services do not call other services.

**Tasks:**
- [ ] Build a table of every server endpoint calling `RequireMemberOfActiveCommunity` (start from the inventory in this doc — ~30 sites).
- [ ] For each, find the client caller(s) and identify the source of `communityId` (mutation response, `state.communityId`, route param, etc.).
- [ ] Classify each site as (a) safe, (b1) needs Option A, (b2) needs Option C, or (c) should not be membership-required at all.
- [ ] Decide whether a new `auth.RequireMemberOfAnyCommunityFor(ctx, storage, resourceType, resourceID, userID)` helper should be introduced (likely yes — see Q6). Add it to `server/auth/community_active.go` next to `RequireMemberOfActiveCommunity` (`server/auth/community_active.go:86`). Mirror its observability shape: structured logging via `logging.LoggerWithContext(ctx)`, `operation` field set, no plaintext emails (use `logging.MaskEmail`), per `docs/server/observability.md`.
- [ ] Output: an audit doc at `docs/issues/1705-audit.md` (sibling of this plan) or a follow-up issue, listing each surface with its category and pointer to its fix PR.
- [ ] Cross-reference with category (c) findings to file the chat unread-count data-leak as its own sub-issue if not already.

**Validates:** Full scope of the bug is known; per-surface fixes can proceed in parallel without missing or duplicating.

### Phase 4: Per-surface fixes (parallel sub-issues per Phase 3 output)
**Goal:** Close #1665, #1666, #1667, #1676, and any other surfaces flagged by Phase 3.
**Tasks (per sub-issue):**
- [ ] **Phase 4-chat (#1665):** Fix the chat-load endpoint scoping (likely category b2 → Option C using the new auth helper). Separately fix the unread-count data leak (likely missing membership JOIN in aggregator query) — verify with a multi-community item where user is in only one community.
- [ ] **Phase 4-members (#1666, #1667, #1676):** Surgical fixes (no mixin) — see Phase 2 deferral note above.
  - **#1676 AccessSheet:** for rows whose `communityId` is NOT in the user's communities, replace the displayed name with "A community you're not in" (new i18n string), do not call `getMembers()` on those rows, hide the expand chevron / disable tap, and ensure the summary banner does not spin when no fetches are pending. The existing subtitle ("Shared by X · Yd ago · Z members") stays — it explains why the row is in the list. Closes both halves: data leakage of community names + members AND the spinner-forever expand.
  - **#1667 manage-members spinner:** the notifier already classifies errors and clears `isLoadingMembers`; the screen wasn't binding `state.error` to anything. Fix: surface a `ContentErrorBanner` for `state.error`.
  - **#1666 InviteSheet:** existing call sites already filter to member-only via `enabledCommunities`; treated as already-fixed in this PR.
- [x] **Phase 4-gear-page-load:** `GearNotifier.loadGearDetails` was passing `state.communityId` to `transferRepository.getGearTransferContext`, which failed with `permission_denied` for multi-community gear when the user wasn't a member of `state.communityId`. Reproduced 2026-05-04 on dev build (PID 20859) against gear `00000000-0000-4000-8000-000000000007`: every visit to the gear page logged `permission_denied` and the transfer-context UI never populated, even before any borrow tap. **Fix:** extended `resolveCommunityId` (`app/lib/core/utils/community_resolver.dart`) with an optional `userMemberIds` parameter; `loadGearDetails` now fetches the user's communities (cached via `CommunityRepository.listUserCommunities`) and feeds those IDs into the resolver so the resulting `effectiveCommunityId` is always one the user is a member of. Degrades to the pre-membership behavior on fetch failure or empty membership list.
- [ ] **Phase 4-other:** Whatever else Phase 3 surfaces (experience get/people, request queries, etc.).

**Validates:** All seven instance issues close. Multi-community gear, chat, and member-list flows work cleanly when the user is a member of any of the resource's communities.

### Phase 5: Regression guardrails
**Goal:** Prevent future regressions.
**Tasks:**
- [ ] Add a CI check (script in `scripts/`, mirroring the style of `scripts/check_a11y.js` and `scripts/check_i18n.js`) that flags new server endpoints calling `RequireMemberOfActiveCommunity(req.Msg.CommunityId, ...)` whose corresponding client caller passes a state-sourced `communityId` for a resource that may live in multiple communities. Allowlist file similar to `scripts/a11y_allowlist.txt` for legitimate exceptions.
- [ ] Document the pattern by extending `docs/client/architecture.md` (the right home, since this is a client-side rule about what to pass to the server). New section under "Data Access" covering: "If a resource can be in multiple communities, do not pass `state.communityId` to a member-required RPC; use the resource's authoritative `community_id` via mutation response or the any-community auth helper introduced in Phase 3." Cross-link from `docs/server/architecture.md` and `docs/server/observability.md` if helpful.
- [ ] Verify the Phase 2 mixin is the default for new viewmodels by adding a contributor-facing line to `docs/client/architecture.md`'s "Common Patterns" section.

**Validates:** The class of bug is unlikely to recur; if it does, defensive UI catches it gracefully.

## Audit Notes

This plan was audited against the project's architectural references on 2026-05-04. Changes applied:

- **Phase 1 — Layering**: clarified that the repository wrapper (`transfer_repository.dart`) is what changes signature to return the response; viewmodels still don't call services. Per `docs/client/architecture.md` §1-2.
- **Phase 1 — Async safety**: explicitly preserve existing `safeUpdateState` calls (already used in `gear_view_model.dart`); do not regress to bare `state = ...` writes after async gaps. Per architecture.md "Async Safety in Notifiers" + `safe_notifier.dart`.
- **Phase 1 — Typed errors**: error state is `UserError? error` via `RpcErrorHandler.classify`, not raw strings. Widget layer localizes via `RpcErrorHandler.localize(..., context.l10n)`. Per `docs/client/i18n.md`.
- **Phase 1 — Disposal test**: added per architecture.md "Disposal Testing" requirement for autoDispose Notifiers.
- **Phase 2 — Naming/shape**: replaced the `safeScopedRead<T>(BuildContext, ...)` free-function-over-context shape with a Notifier mixin built on top of `SafeNotifierMixin`. Free-functions over `BuildContext` would invert MVVM (business logic running with widget context); a mixin keeps the `safeUpdateState` contract end-to-end. Per architecture.md §5 ("No Controllers") and the existing `SafeNotifierMixin` precedent.
- **Phase 2 — a11y/i18n**: error UI surfaces use the existing `ContentErrorBanner` (already wraps in `LiveRegion`); new strings go in ARB with `a11y` prefix where relevant; `lint:dart:i18n` and `lint:dart:a11y` must remain clean. Per `docs/client/accessibility.md` and `docs/client/i18n.md`.
- **Phase 3 — Server boundaries**: the new `auth.RequireMemberOfAnyCommunityFor` helper lives in `server/auth/community_active.go`, not in any specific service. Services depend on shared libraries; services do not call services. Per `docs/server/architecture.md` §1.
- **Phase 3 — Observability**: the new auth helper follows the project's structured-logging conventions (`LoggerWithContext`, `operation` field, `MaskEmail`). Per `docs/server/observability.md`.
- **Phase 5 — Lint script style**: new CI check follows the established pattern of `scripts/check_a11y.js` / `scripts/check_i18n.js` with an allowlist file (CI-ratchet style — may shrink, never grow).
- **Phase 5 — Doc home**: pattern documentation lives in `docs/client/architecture.md` under "Data Access," not a new `docs/architecture/` directory. Aligns with existing structure.
- **Q3 / Phase 2 helper shape**: corrected to a mixin (not a `(BuildContext, ...)` free function) per the layering audit above.

## Related Issues

- #1665 — Chat won't load for multi-community item (chat-load endpoint scoping + unread-count data leak)
- #1666 — Invite shown for non-member community (invite endpoint + UI exposure)
- #1667 — Member-list spinner for non-member community
- #1676 — Combines data-leak (sees other groups) + spinner (expand fails)
- #1684 — Borrow doesn't do anything (mutation+refresh-fail, multi-community noted)
- #1685 — Borrow N times (mutation+refresh-fail, may or may not be multi-community)
- #1703 — Crashlytics fatal trace; this umbrella is "Bug A" of #1703. Bug B (Crashlytics fatal classification of expected RPC errors) is handled in #1703 directly and is independent of this plan.
- #1680 — *Near-miss, not in scope.* Multi-community theme but server-side fan-out, not client-side scoping. Already fixed in PR #1687.
- #1701 — Adjacent (delete-cascade gap for transfers); separate umbrella.
- #1698 — Adjacent (delete-cascade for experiences); already merged via PR #1700.
