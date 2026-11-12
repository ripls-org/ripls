# Issue #2237: Community-scoped access (#2136) breaks share/invite links for non-members (event RSVP + loan/giveaway/request view)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2237
**Filed:** 2026-05-28 | **Last activity:** 2026-05-28
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/issues/2050-web-rsvp-actions.md
**Decision:** Option 1 (join the community). This plan covers **only** the Option-1
fix; Options 2 (code-scoped item access) and 3 (events public) are not pursued.

## Problem Statement

After #2136 tightened entity reads to community members
(`auth.RequireAccessToCommunityScopedEntity`), an **authenticated non-member**
following an **event** share link cannot load or RSVP to the event: `GetExperience`
→ 403 ("Failed to load experience details") and `RSVPToExperience` → 403.

Root cause (confirmed in dev logs + code): the **event** share-link flow joins a
visitor **only via registration** — it has no join step for an already-
authenticated visitor. The 2050 §D1 design is "register → become a `CommunityUser`
via the `short_code` → `RSVPToExperience`"; that membership step happens **inside
registration** (the `short_code` rides into `EmailRegister`/`PhoneRegister`, which
creates the `CommunityUser` — *not* a separate `acceptInvitationLink` call). So a
**new** account joins and RSVPs fine, but an **already-authenticated existing**
account skips registration, so **no join ever fires** and it hits the gated RPCs as
a non-member. (The gear/request `/invite` branch always joins via
`acceptInvitationLink`, so those item types aren't affected.)

**Scope confirmed in dev logs — the bug is specifically authenticated existing
non-members:** new registrations `16e4d275` and `53dfd9b0` went `EmailRegister 200
→ recorded RSVP community=b884c0eb entity=e3a326da` (join-at-register works);
existing accounts `d4cd9cda`, `75bac154`, `18628bf8` all hit
`RequireAccessToCommunityScopedEntity reason=nonmember entity=e3a326da` → 403. The
fix below targets exactly that gap.

This is client-only: the server's `AcceptInvitationLink`
(`server/services/community/invitations.go:158-179`) already resolves an **event**
share-link code to its community and joins the caller idempotently (returns
success if already a member). The client simply isn't calling it on the event
path.

## Context from Comments

One maintainer comment (briangbrown) records the investigation and the decision:

- **Decision: Option 1.** Access = community membership; make the event flow join
  the shared community before view/RSVP, mirroring gear/request. Options 2/3 are
  explicitly out.
- **Log proof.** Test account `d4cd9cda` is a member of `728ad8f6`/`be836df0`, not
  `b884c0eb` (the event's community); no `AcceptInvitationLink` from it ever fires;
  member `53dfd9b0` RSVP'd fine.
- **Two client sites identified:** `app_router.dart:378` (`/invite` event branch →
  `ExperienceScreen`, no join) and `web_experience_screen.dart`
  (`/event/{id}?rsvp=&code=` via `WebEventRsvpHandoffNotifier`, RSVP without join).
- **Server consideration raised** ("does `AcceptInvitationLink` handle event
  codes?") — **resolved during this planning:** yes, it does (see Problem
  Statement). So the fix is client-only.

## Codebase findings (Phase 2 research)

- **Two event entry points, both unauthenticated of a join:**
  1. `app/lib/core/router/app_router.dart:378` — the `/invite` handler resolves the
     code via `CheckInvitation` (`describe`); for `describe.isEvent` it returns
     `ExperienceScreen(experienceId, initialCommunityId)` directly. The non-event
     branch (`:387`) wraps `acceptInvitationLink(shortCode: token)` in a
     FutureBuilder ("Joining community…") with full error UX (`:404+`:
     revoked / capacity / invalid).
  2. `app/lib/presentation/screens/experience/web_experience_screen.dart` —
     renders the event content (which loads via `GetExperience`) and watches
     `webEventRsvpHandoffProvider` to auto-fire RSVP.
- **The handoff** (`app/lib/presentation/viewmodels/web_event_rsvp_handoff_view_model.dart`)
  does `shareLinkRepo.describe(shortCode)` → `experienceService.rsvp(...)`. **No
  join.** It also runs *after* the screen builds, so it is too late to unblock the
  `GetExperience` load.
- **Ordering constraint:** the join must complete **before** `GetExperience`
  (the screen-load read), not merely before RSVP. A join placed only in the
  handoff fixes RSVP but leaves "Failed to load experience details" intact.
- **Join RPC is already wired client-side:** `acceptInvitationLink(shortCode:)`
  on `communityServiceProvider` (`services/community_service.dart:381`) /
  `communityRepositoryProvider` (`data/repositories/community_repository.dart:517`),
  used today by the `/invite` gear/request branch.
- **Server:** `AcceptInvitationLink` accepts all share-link variants incl. `event`
  (`invitations.go:171-179`, `requireAcceptableViaCommunityJoin` at
  `share_links.go:315`), joins `link.CommunityId`, idempotent, emits
  `INVITATION_LINK_USED`.

## Potential Solutions (all implement Option 1; the question is *where* the join goes)

### Option A: Idempotent join-gate at both event entry points (recommended)
- **Approach:** Add an `acceptInvitationLink(shortCode)` step that completes
  **before** the event content loads, at both sites: (1) the `/invite` event
  branch (mirror the gear/request FutureBuilder, then route to `ExperienceScreen`),
  and (2) `WebExperienceScreen` (a join gate that runs before rendering the event
  content / before the handoff). Call it unconditionally when a `code` is present —
  it's idempotent, so members fall straight through. Leave the handoff RSVP-only
  (it now succeeds because membership exists).
- **Pros:** Fixes both "Failed to load" (`GetExperience`) and RSVP, on both paths,
  for authenticated non-members. Reuses the existing RPC and the gear/request error
  UX. No server change. Matches 2050 §D1 (join precedes RSVP).
- **Cons:** Touches two screens; needs a small join view-state for the gate.
- **Effort:** Small–medium.

### Option B: Join inside the handoff view-model only
- **Approach:** Change the handoff to `describe` → `acceptInvitationLink` → `rsvp`.
- **Pros:** Smallest diff.
- **Cons:** **Insufficient** — the handoff runs after the screen builds, so
  `GetExperience` still 403s and the user still sees "Failed to load." Rejected as
  a complete fix (may be a complementary tidy-up, but not the gate).
- **Effort:** Small.

### Option C: Funnel `/event?code=` through the existing `/invite` join flow
- **Approach:** Make the SSR landing / event route redirect through `/invite` so a
  single join path serves all entry points.
- **Pros:** One join path.
- **Cons:** The SSR landing links straight to `/event` (server-rendered), so this
  needs an SSR/template change — drifts out of "client-only" and is more invasive
  than the gate. Rejected for now.
- **Effort:** Medium (crosses into server SSR).

## Recommendation

**Option A.** It's the smallest change that actually fixes the reported flow
(authenticated non-member: SSR landing → `/event/{id}?rsvp=&code=` → "Failed to
load"), covers the `/invite` event branch for parity, reuses the existing RPC +
error UX, and needs no server work. Call `acceptInvitationLink` unconditionally
when a code is present and lean on its server-side idempotency rather than a
pre-membership check.

## Open Questions

- **Gate on non-members only, or always when a code is present?** — Recommendation:
  **always when a code is present.** `AcceptInvitationLink` short-circuits to
  success for existing members (`invitations.go:199-205`), so a membership
  pre-check just adds a round-trip and a race. One idempotent call is simplest.
- **Where does the join live in `WebExperienceScreen` — a tiny join Notifier or an
  inline FutureBuilder?** — Recommendation: a small **autoDispose Notifier**
  (`webEventJoinProvider`, family-keyed by shortCode) mirroring
  `WebEventRsvpHandoffNotifier`, so it's unit-testable without a widget tree
  (per `client/testing.md`) and the screen just renders join/loading/error from
  its state before mounting the event content.
- **Error UX when join fails (revoked / capacity / invalid)?** — Recommendation:
  reuse the messaging already in the `/invite` gear branch
  (`app_router.dart:404+`); factor it into a shared widget/helper so both event and
  gear paths show the same "couldn't join" surface. Keep strings in `context.l10n`
  (events are user-facing — say "event", not "experience").
- **Does the handoff still need its own `describe` call?** — Recommendation: keep
  it for now (it resolves `communityId` for the RSVP write); revisit folding the
  join+RSVP into one notifier later. Not in scope for the fix.
- **Guest (unauthenticated) path — already covered?** — Per 2050 §D1, registration
  via the `short_code` creates the `CommunityUser`, so guests join during
  register. Recommendation: confirm with one manual guest run; if the membership
  isn't created at register time, that's a separate follow-up (this plan targets
  the authenticated-non-member regression).

## Implementation Plan

### Phase 1: Join gate in `WebExperienceScreen` (fixes the reported flow)
**Goal:** An authenticated non-member who lands on `/event/{id}?rsvp=&code=` joins
the community before the event loads, so `GetExperience` and the auto-RSVP succeed.
**Tasks:**
- [ ] Add `webEventJoinProvider` (autoDispose Notifier, family-keyed by shortCode):
  `join(shortCode)` calls `communityRepository.acceptInvitationLink(shortCode:)`,
  exposing idle/running/succeeded/failed. **Mirror `WebEventRsvpHandoffNotifier`
  exactly:** plain `Notifier` + `SafeNotifierMixin` (`core/utils/safe_notifier.dart`)
  with `if (!ref.mounted) return;` guards after the await — required async-safety
  for autoDispose notifiers (architecture §Async Safety).
- [ ] In `WebExperienceScreen`: when `shortCode` is present, run the join and gate
  the event content on success ("Joining…" while running; reuse the shared error
  surface on failure). Only mount `ExperienceContent` (which fires `GetExperience`)
  and trigger the RSVP handoff **after** the join resolves. Announce the running
  status and any error to screen readers via `SemanticAnnouncer`
  (`presentation/widgets/accessibility/semantic_announcer.dart`); all strings via
  `context.l10n` (no raw literals — `require_l10n_for_semantic_labels`).
- [ ] Tests (`client/testing.md` — ViewModel-first, `ProviderContainer`, Mockito
  mock of `CommunityRepository`): `webEventJoinProvider` calls `acceptInvitationLink`
  with the code; success → succeeded; error → failed; member (idempotent success)
  → succeeded; **disposal test** (dispose the container mid-`join()` → completes
  without throwing, required for all async Notifiers). Reset mocks in `tearDown`,
  dispose the container. Plus a widget test that `ExperienceContent` isn't mounted
  until join succeeds.
**Validates:** The exact repro (`d4cd9cda` on `/event/…?code=Ts7uj2TZ`) loads and
RSVPs; member accounts are unaffected (idempotent join).

### Phase 2: Join in the `/invite` event branch — **the Android/iOS fix** (+ web SPA `/go/:code`)
**Goal:** Join before showing the event on the shared `/invite` event branch.
**This is the mobile fix, not just web parity:** `/event/:experienceId` →
`WebExperienceScreen` is **web-only** (`app_router.dart:642`; mobile "never visits
this URL"), so Phase 1 did **not** touch Android/iOS. Mobile event share links
route `/go/:code` → `/invite` → (authenticated) `describe` → the event branch at
`app_router.dart:378`, which today returns `ExperienceScreen` **with no join** — so
an already-authenticated existing non-member 403s on `GetExperience` on mobile too.
New users are fine on both platforms (they join inside `RegisterScreen` via the
`short_code`). This phase fixes mobile **and** the web SPA `/go/:code → /invite` path.
**Tasks:**
- [ ] `app_router.dart:378`: for `describe.isEvent`, call
  `acceptInvitationLink(shortCode: token)` before routing to `ExperienceScreen`
  (reuse the gear/request FutureBuilder + "Joining community…" + error UX, or the
  Phase-1 join provider/widget).
- [ ] Factor the shared join loading/error UX (revoked / capacity / invalid) out of
  the gear branch into one widget/helper used by both branches. **i18n:** the gear
  branch currently hardcodes `Text('Joining community...')` (`app_router.dart:397`)
  — move it (and the error strings) to `context.l10n` with **both en + es** added
  (`lint:dart:l10n-parity`), e.g. `inviteJoiningCommunity`. Use **"event"** wording
  on the event path per the UI-terminology rule (never "experience" in user copy).
- [ ] Tests: routing/widget test that the event branch joins before `ExperienceScreen`;
  widget tests wrap `MaterialApp` with `localizationsDelegates`/`supportedLocales`
  (`test/helpers/l10n_helpers.dart`).
**Validates:** Non-members join on **Android, iOS, and the web SPA** `/invite` path
(the SSR-landing → `/event` web path was Phase 1); no regression for gear/request.

### Phase 3: Verification + guest-path confirmation
**Goal:** Prove the end-to-end flow and rule out a guest-path gap.
**Tasks:**
- [ ] Manual: authenticated non-member RSVPs via a real dev event link end-to-end
  (load + RSVP succeed; dev logs show one `AcceptInvitationLink` then a 200
  `GetExperience`/`RSVPToExperience`).
- [ ] Manual: guest (unauthenticated) via the same link still joins at register and
  RSVPs (confirms 2050 §D1 membership-at-register holds); file a follow-up if not.
- [ ] `flutter analyze` + `flutter test` green; no new a11y/l10n allowlist growth.
**Validates:** The #2136 regression is closed for events on web for both
authenticated and guest visitors.

## Related Issues
- #2136 / PR #2143 — introduced `RequireAccessToCommunityScopedEntity`; this fixes
  its non-member event share-link regression.
- #2050 — web RSVP actions; §D1 defines the join-then-RSVP client flow this restores.
- Loan/giveaway/request share links share the same gate; the gear/request `/invite`
  branch already joins, so they are not broken — but Phase 2's shared error UX
  should keep them consistent.

## Audit Log

Reviewed against `docs/client/architecture.md`, `client/testing.md`,
`client/accessibility.md`, `client/i18n.md`, `client/caching.md`,
`server/architecture.md`, `server/observability.md`. Changes applied:

- **Async safety (architecture §Async Safety):** specified the new
  `webEventJoinProvider` mirror `WebEventRsvpHandoffNotifier` — plain `Notifier` +
  `SafeNotifierMixin` + `ref.mounted` guards after awaits (it's autoDispose) — and
  required a **disposal test** (testing.md: every async Notifier has one).
- **Testing conventions:** made the test bullets explicit — ViewModel-first,
  `ProviderContainer`, Mockito `CommunityRepository` mock, mock-reset/container-dispose
  in `tearDown`, l10n-wrapped `MaterialApp` for widget tests.
- **i18n:** flagged the existing hardcoded `Text('Joining community...')`
  (`app_router.dart:397`); Phase 2 now moves it + error strings to `context.l10n`
  with **en + es** parity, using **"event"** (not "experience") in user copy.
- **Accessibility:** the join loading/error surface must announce via
  `SemanticAnnouncer` and pull labels from `context.l10n` (empty a11y allowlist is a
  CI ratchet — new violations must be fixed in-PR).
- **Navigation:** confirmed the change stays inside the existing `/invite` route
  builder and `WebExperienceScreen`; it introduces no `context.push` /
  `Navigator.push(MaterialPageRoute)` (reserved/forbidden), so nav conventions hold.
- **N/A:** caching (the fix calls a mutation, `acceptInvitationLink`; the existing
  `shareLinkRepo.describe` is already Stash-cached — no cache changes), server
  architecture, and server observability (slog conventions don't apply to client
  Dart) — this is a client-only change, confirmed against the server finding that
  `AcceptInvitationLink` already supports event codes.
