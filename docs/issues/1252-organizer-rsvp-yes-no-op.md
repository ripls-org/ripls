# Issue #1252: RSVP 'Yes' button on experience management does nothing

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1252
**Filed:** 2026-04-23 | **Last activity:** 2026-04-23
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

An experience organizer creates an experience, opens **Manage**, taps **RSVP** →
**Yes**, and sees no visible response. Expected behavior: the button confirms
the Yes selection and the organizer's entry appears in the responses list on
the modal.

The "Manage" flow for organizers opens
[event_organizer_modal.dart](app/lib/presentation/screens/experience/event_organizer_modal.dart),
not the standalone
[rsvp_modal.dart](app/lib/presentation/screens/experience/rsvp_modal.dart)
(which is currently unused — no callers). So the bug lives in the organizer
modal's RSVP path.

Two distinct defects combine to make the tap look like a no-op:

1. **Responses section is hidden while the list is empty.**
   [event_organizer_modal.dart:287](app/lib/presentation/screens/experience/event_organizer_modal.dart#L287)
   wraps the entire `RESPONSES` block in `if (state.rsvps.isNotEmpty) ...[`.
   A freshly created experience has zero RSVPs, so the block doesn't render at
   all — the organizer's new entry can't appear there until rsvps is
   non-empty.

2. **Optimistic update can't insert a first-time RSVP.**
   [event_modal_view_model.dart:403-418](app/lib/presentation/viewmodels/event_modal_view_model.dart#L403-L418)
   does `state.rsvps.map(...)` and only rewrites an existing entry whose
   `user.id == currentUserId`. If the user isn't already in the list (the
   normal case for a first RSVP) the map is a no-op and `state.rsvps` stays
   empty until the server round-trip completes.

The button's `_selectedIntention` does flip via
[event_organizer_modal.dart:272-274](app/lib/presentation/screens/experience/event_organizer_modal.dart#L272-L274),
so the toggle itself may highlight briefly — but combined with the hidden
responses section and the still-zero "going" count in the summary row, the
user's perception is "nothing happened." Any latency, a dropped network call,
or a silently swallowed error makes the gap arbitrarily long and can leave the
UI permanently unchanged.

## Context from Comments

No comments on the issue. This was filed by the user (Thomas Escobar, also
assignee) immediately after reproduction. No discussion or clarification
beyond the description.

## Potential Solutions

### Option A: Fix the optimistic update to insert first-time RSVPs

- **Approach:** In `submitRsvp`
  ([event_modal_view_model.dart:389](app/lib/presentation/viewmodels/event_modal_view_model.dart#L389)),
  detect whether the current user already has an entry in `state.rsvps`. If
  yes, map-rewrite as today. If no, append a new `RSVP` with the current
  user's profile (from `authStateProvider.user`), the chosen `intention`, and
  `rsvpedAtUnixSec`/`lastUpdatedUnixSec` set to the client clock. Also relax
  the `if (state.rsvps.isNotEmpty)` guard in the organizer modal so the
  responses container renders as soon as there is at least one entry
  (including the optimistic one) — or move the organizer's own row above the
  guard so they always see themselves.
- **Pros:** Fixes the perceived "nothing happened" directly. Immediate visual
  confirmation independent of network latency. The standalone `rsvp_modal.dart`
  already does the right thing (renders the owner row unconditionally, lines
  227-234), so this aligns the two modals.
- **Cons:** Optimistic state can diverge from server state if the RPC fails —
  we already rollback `currentUserIntention` on the catch path, but the
  inserted rsvp entry isn't rolled back. Needs symmetric rollback.
- **Effort:** Small.

### Option B: Skip the optimistic update, show a submission spinner instead

- **Approach:** Keep the existing `isSubmittingRsvp` flag but surface it
  visibly on the tapped button (small spinner overlay) and on the responses
  section (skeleton row). Rely on the server refresh to populate the final
  state.
- **Pros:** No optimistic/authoritative divergence risk. Loading state makes
  it obvious the tap registered even on slow networks.
- **Cons:** Keeps a visible delay between tap and confirmation. Doesn't fix
  the hidden-responses-section defect on its own — still need to render the
  container when `rsvps` is empty (or show the skeleton inside it). More UI
  work than Option A.
- **Effort:** Medium.

### Option C: Always render the organizer's own row, optimistic or not

- **Approach:** Port the pattern from
  [rsvp_modal.dart:227-234](app/lib/presentation/screens/experience/rsvp_modal.dart#L227-L234)
  to the organizer modal: always emit a row for `experience.owner`, deriving
  its intention from `state.rsvps.firstWhere(user.id == owner.id)` or falling
  back to `state.currentUserIntention`. Drop the `isNotEmpty` guard. Keep the
  optimistic-update code path as-is.
- **Pros:** Smallest possible change. Matches existing pattern in sibling
  modal — easier to reason about.
- **Cons:** Doesn't fix the optimistic update for non-organizer joiners
  opening the same modal. (Though: only organizers see `event_organizer_modal`,
  so the scope may be fine. Joiners hit `event_joiner_modal.dart`, which needs
  the same audit.)
- **Effort:** Small.

## Recommendation

**Option A + Option C together**, as a single small fix:

- Fix the optimistic update in `submitRsvp` to append when the user isn't
  already in `state.rsvps` (Option A). Add rollback in the catch path.
- In the organizer modal, drop the `state.rsvps.isNotEmpty` guard and always
  render the owner's row, matching the standalone `rsvp_modal.dart` pattern
  (Option C).

The two together produce the shortest-path fix with no user-visible delay and
no loading skeleton work. Option B is a better architecture for long-running
mutations in general, but RSVP is a trivial write and the optimistic path is
already in place — finishing it is cheaper than replacing it.

Audit `event_joiner_modal.dart` for the same two defects while we're here —
very likely the same bug exists for non-organizer first-time RSVPs.

## Open Questions

- **Should the optimistic rsvp insert be rolled back on RPC failure?** —
  Recommendation: yes. The existing catch path rolls back
  `currentUserIntention` implicitly via the failed refresh but doesn't touch
  the `rsvps` list. Snapshot `state.rsvps` (and `state.currentUserIntention`)
  before the optimistic mutation and restore both on error, mirroring the
  "Optimistic Updates" pattern documented in
  [docs/client/architecture.md](../client/architecture.md#optimistic-updates).
- **Does `event_joiner_modal.dart` have the same bugs?** — Recommendation:
  assume yes and fix both in the same PR. The optimistic update lives in the
  shared view model, so one fix covers both modals for the update defect;
  only the `isNotEmpty` render guard needs to be checked per-modal.
- **Should we add a test that prevents regression?** — Recommendation: yes.
  Add a viewmodel test (ProviderContainer + Mockito mocks per
  [docs/client/caching.md](../client/caching.md#unit-tests-mock-repositories))
  asserting that `submitRsvp` populates `state.rsvps` with the current user's
  entry before the server call resolves, with `state.rsvps` starting empty.
  Plus a disposal test per the architecture rule that "all async Notifiers
  must have disposal tests"
  ([docs/client/architecture.md](../client/architecture.md#disposal-testing)).
- **Does the `rsvp_modal.dart` file have any production callers we missed?** —
  Recommendation: `RsvpModal` has no callers (only self-references). Either
  it's dead code or a planned flow that was never wired up. Out of scope for
  this bug, but flag for cleanup / tracking issue.
- **Does `ExperienceRepository.rsvp()` already invalidate the right caches?**
  — Verified during audit: yes.
  [experience_repository.dart:287](../../app/lib/data/repositories/experience_repository.dart#L287)
  invalidates `experience:$experienceId*` and refreshes conversations, so
  `_refreshExperience()` pulls authoritative data after the optimistic update.
  No repository-layer changes needed — this is purely a viewmodel + widget
  fix.

## Implementation Plan

### Phase 1: Fix the optimistic update

**Goal:** Tapping Yes immediately reflects the organizer in `state.rsvps`,
independent of network latency.

**Tasks:**
- [ ] In
  [event_modal_view_model.dart:402-418](app/lib/presentation/viewmodels/event_modal_view_model.dart#L402-L418),
  check whether `state.rsvps` already contains an entry for `currentUserId`.
  If yes, keep the existing `.map` rewrite. If no, append a new `RSVP` with
  the full `User` object from `ref.read(authStateProvider).user` (verified
  during audit: returns the `ripls/api/user.pb.dart` `User` type that matches
  the `RSVP.user` field), the new `intention`, empty `attended`, and
  `clock-now` for both timestamps.
- [ ] Snapshot both `state.rsvps` and `state.currentUserIntention` before the
  optimistic mutation. In the existing catch block
  ([event_modal_view_model.dart:431-441](app/lib/presentation/viewmodels/event_modal_view_model.dart#L431-L441)),
  restore both along with clearing `isSubmittingRsvp`.
- [ ] Use `safeUpdateState((s) => s.copyWith(...))` for every state mutation
  (matches the existing pattern in this viewmodel, which uses
  `SafeNotifierMixin`). Keep the `if (!ref.mounted) return;` guard before any
  state update that follows an `await`, per the async safety rule in
  [docs/client/architecture.md](../client/architecture.md#async-safety-in-notifiers).

**Validates:** Viewmodel test — starting from an empty `rsvps`, call
`submitRsvp(YES, ...)` and assert `state.rsvps.length == 1` and the entry's
user id matches `currentUserId`, before the server call resolves. Use
Mockito-generated mocks for `ExperienceRepository` via
`@GenerateMocks([ExperienceRepository])` per
[docs/client/caching.md](../client/caching.md#unit-tests-mock-repositories).

### Phase 2: Render the responses section and organizer's own row unconditionally

**Goal:** The organizer's entry is visible on the modal the moment the
optimistic update completes.

**Tasks:**
- [ ] In
  [event_organizer_modal.dart:287](app/lib/presentation/screens/experience/event_organizer_modal.dart#L287),
  remove the `if (state.rsvps.isNotEmpty)` guard. Always render the
  `RESPONSES` heading and container.
- [ ] Mirror the `rsvp_modal.dart` pattern: always render `experience.owner`'s
  row first (deriving intention from `state.rsvps`), then map the remaining
  rsvps for non-owner entries. This guarantees the owner sees themselves even
  before the optimistic update lands.
- [ ] If no other responses exist, the owner row alone is sufficient — match
  the standalone modal's behavior. **If a new user-visible string is
  introduced** (e.g. an explicit empty-state line for non-owner responses),
  it must be added to both `app/lib/l10n/app_en.arb` and `app_es.arb` with
  `@key` metadata and accessed via `context.l10n.*`, per
  [docs/client/i18n.md](../client/i18n.md). Hardcoded strings are caught by
  `npm run lint:dart:i18n` in CI.
- [ ] All colors in any new/modified UI must use `AppColors.*(context)` or
  `Theme.of(context)` — never hardcoded `Colors.white`, `Colors.grey[...]`,
  or hex values. The existing modal already follows this; new code must
  match.

**Validates:** Manual reproduction of the original bug — create experience,
Manage → RSVP → Yes, confirm the button highlights, the "going" count
increments, and the organizer's row appears in the responses list, all
without perceptible delay. Test in both light and dark mode.

### Phase 3: Audit the joiner modal and add regression coverage

**Goal:** Prevent the same defect from shipping for non-organizer users and
prevent silent regressions.

**Tasks:**
- [ ] Review
  [event_joiner_modal.dart](app/lib/presentation/screens/experience/event_joiner_modal.dart)
  for an equivalent `isNotEmpty` guard and/or missing first-time rendering.
  Apply the same fix if present. (Viewmodel fix from Phase 1 already covers
  the joiner's optimistic path.)
- [ ] Add viewmodel unit tests for `submitRsvp` covering: first-time RSVP
  (empty list), existing RSVP update (map rewrite), failure rollback (server
  returns error → `rsvps` and `currentUserIntention` restored), and
  **disposal during the in-flight RPC** (dispose the ProviderContainer mid-
  call and assert the future completes without throwing). Disposal tests are
  required for async notifiers per
  [docs/client/architecture.md](../client/architecture.md#disposal-testing).
- [ ] Run `npm run test:app` and `npm run build` to confirm Flutter tests
  and Go lint/build pass. Since this change touches only client code, no Go
  tests are affected — but the `build` target also runs `golangci-lint` and
  acts as a sanity check.

**Validates:** Tests fail on the un-fixed code and pass on the fixed code.
Joiner modal verified visually or via widget test.

## Related Issues

- #1012 — "Manage button social proof." The Manage entry point invokes the
  organizer modal; UX changes to that button may overlap with this fix but
  don't block it.
- #1062 — "Simulator schedules RSVPs beyond experience max participants."
  Same RSVP server path. Not a direct dependency, but test fixtures for
  first-time-RSVP behavior may overlap.

---

## Audit Changes (applied 2026-04-23)

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`. Adjustments:

- **Phase 1 — SafeNotifierMixin + ref.mounted.** Called out that the existing
  viewmodel uses `safeUpdateState` and mandated its use for all new mutations,
  plus explicit `if (!ref.mounted) return;` after await points. Matches the
  "Async Safety in Notifiers" rule in architecture.md; prevents disposal
  crashes on `autoDispose` providers.
- **Phase 1 — User type confirmed.** Verified
  `ref.read(authStateProvider).user` returns the proto `User` from
  `ripls/api/user.pb.dart`, which is the exact type the `RSVP.user` field
  expects. Added this note so the implementer doesn't need to re-verify.
- **Phase 1 — rollback snapshot extended to `currentUserIntention`.** The
  original plan only snapshotted `state.rsvps`, but the optimistic path also
  writes `currentUserIntention`; both must roll back on failure to keep
  state consistent.
- **Phase 2 — i18n requirement added.** If Phase 2 introduces any new
  user-visible string (e.g. an empty-state line), it must be added to both
  ARB files and accessed via `context.l10n.*`. `npm run lint:dart:i18n`
  enforces this in CI.
- **Phase 2 — theme-aware colors rule restated.** All new UI must use
  `AppColors.*(context)` / `Theme.of(context)` per the architecture doc's
  "Colors and Theming" section. The existing modal already complies; this
  is a reminder to not regress.
- **Phase 2 — dark-mode validation added to manual test.**
- **Phase 3 — disposal test added.** Architecture doc requires disposal
  tests for async notifiers. The original plan only listed first-time /
  existing / failure cases.
- **Phase 3 — Mockito specified.** Repository mocking uses
  `@GenerateMocks([ExperienceRepository])` per the caching doc's testing
  pattern. No ad-hoc mocks.
- **Open Questions — cache invalidation answered.** Verified
  `ExperienceRepository.rsvp()` already invalidates `experience:$id*` and
  refreshes conversations. No repository changes needed — the fix is purely
  viewmodel + widget.
- **Architecture alignment confirmed.** Fix stays within MVVM + Riverpod
  boundaries: widget → viewmodel → repository → service. No rule violations
  (no manual cache maps in viewmodel, no service calls from widgets, no
  string resolution in viewmodel).
- **Server-side unchanged.** No server/proto work needed; server
  architecture doc reviewed for completeness but does not apply to this
  plan.
