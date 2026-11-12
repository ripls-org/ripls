# Issue #2152: Single TBD option creates poll instead of setting value directly

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2152
**Filed:** 2026-05-24 | **Last activity:** 2026-05-24
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Audit (2026-05-24)

Audited against
[docs/client/architecture.md](../client/architecture.md),
[docs/client/caching.md](../client/caching.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/accessibility.md](../client/accessibility.md),
[docs/client/testing.md](../client/testing.md),
[docs/server/architecture.md](../server/architecture.md), and
[docs/server/observability.md](../server/observability.md).

Changes from the first draft:

- **Layer compliance.** Confirmed the recommended fix keeps the
  MVVM contract intact (modal widget → viewmodel notifier →
  repository → service). No repository or service code is reached
  from the widget directly. Spelled this out in the helper specs.
- **Async safety.** Added `ref.mounted` guard requirements after
  every `await` in the new helpers; the location and time modal
  providers are `family`/`autoDispose`-style notifiers, and direct
  state assignments after async gaps would otherwise crash on
  modal dismissal mid-confirm (see
  [docs/client/architecture.md#async-safety-in-notifiers](../client/architecture.md#async-safety-in-notifiers)).
- **Cache invalidation.** Called out the existing
  `_syncExperience(...)` pattern in `LocationModalViewModel` —
  the new `setSingleLocation` helper must do the same so other
  consumers of the experience proto (event detail screen, daily
  feed) don't render stale poll state. Time side already has the
  `ref.invalidateSelf()` call inside `finalizeEventTime`.
- **Testing.** Specified `ProviderContainer` + Mockito-generated
  mocks, called out the disposal test required for all async
  notifier methods, and added the explicit test cases the plan
  was previously hand-waving.
- **i18n.** Confirmed no new ARB keys are needed —
  `locationPollProposeSetSpot` / `timePollProposeSetTime` are
  already in both locales. Removed the speculative "check for
  related string drift" task and replaced it with a concrete
  `npm run lint:dart:l10n-parity` check.
- **Accessibility.** Confirmed no new tappable primitives are
  being introduced (the existing `_PrimaryButton` is unchanged);
  no a11y rule additions needed.
- **Server side (Phase 3).** If a Phase 3 server change is needed
  to suppress the "poll opened" event, the new error path must
  use `connecterr.Internal(ctx, op, err, kv...)`, not bare
  `connect.NewError(connect.CodeInternal, err)` — gated by
  `scripts/check_codeinternal.js`. Logging must go through
  `logging.LoggerWithContext(ctx)` with `operation` and
  `experience_id` fields.
- **Build verification.** Added the `npm run build`,
  `npm run lint:dart:size`, and `npm run test:app` steps that
  the QA phase needs to invoke explicitly.

## Problem Statement

When an event has a TBD location or TBD time, tapping the field opens a
modal where the user can stage one or more candidate places / date-times.
The CTA label correctly switches on candidate count — it reads
"Set the spot" / "Set the time" when exactly one candidate is staged,
and "Ask the group" when two or more are staged
([location_poll_propose_modal.dart:230-234](app/lib/presentation/screens/experience/location_poll_propose_modal.dart#L230-L234),
[time_poll_propose_modal.dart:290-294](app/lib/presentation/screens/experience/time_poll_propose_modal.dart#L290-L294)).

The label, however, is the only thing that branches on count. Tapping the
button always invokes the propose-poll path —
`proposeLocationsSequentially` / `proposeGeocodedLocationsSequentially` for
location, `createTimePoll` for time. The server therefore opens a poll
(setting `LocationPollActive` / `TimePollActive` true, posting a "poll
opened" system chat message, etc.) with a single option.

A poll-of-one is semantically wrong: nobody votes on it, the deadline UI
is pointless, and we still need a second tap from the organizer to lock
it. The user-facing contract conveyed by the singular CTA label is "this
just sets the value." The implementation should match.

## Context from Comments

There are no comments on the issue. The repro and expected behavior are
unambiguous, so the lack of discussion is not blocking — but anything
nuanced (e.g. how the server emits chat messages, whether RSVPs already on
the event need notifying differently) will need to be worked out during
implementation.

## Current code path

### Location modal — [`_submit()`](app/lib/presentation/screens/experience/location_poll_propose_modal.dart#L187-L214)

```
if _stagedLocationIds.isNotEmpty:
  notifier.proposeLocationsSequentially(_stagedLocationIds)
if _stagedGeocoded.isNotEmpty:
  notifier.proposeGeocodedLocationsSequentially(_stagedGeocoded)
```

No branch on `_totalStagedCount == 1`. Both notifier methods call
`_repository.proposeLocation(...)` per item and append to `proposals` /
flip `locationPollActive = true`.

### Time modal — [`_submit()`](app/lib/presentation/screens/experience/time_poll_propose_modal.dart#L242-L274)

```
options = [(date, time), ...]
notifier.createTimePoll(options)
```

`createTimePoll` proposes each option sequentially; first proposal flips
`TimePollActive` and emits the "poll opened" system message.

### Existing direct-set primitives

The viewmodels already expose direct-set helpers we can reuse:

- [`LocationModalViewModel.confirmLocation(proposalId)`](app/lib/presentation/viewmodels/location_modal_view_model.dart#L357)
  — sets `eventLocationId`, flips `locationPollActive = false`,
  `locationPollCompleted = true`. Requires an existing proposal id.
- [`TimeModalViewModel.finalizeEventTime()`](app/lib/presentation/viewmodels/time_modal_view_model.dart#L548)
  — propose-then-confirm helper that already implements
  "set this single time as the event's actual time," driven off form
  state (`proposeDate` / `proposeTime` / `proposeDurationMinutes`),
  not staged candidates.
- [`TimeModalViewModel.updateEventTime()`](app/lib/presentation/viewmodels/time_modal_view_model.dart#L488)
  — direct `SaveExperience` write, also driven off form state.

For location, the analogue to `finalizeEventTime` does not exist as a
single helper — we'd need to compose `proposeLocation` →
`confirmLocation`, or extend the repository / use `SaveExperience` to
set `locationId` directly. Geocoded drop-pins need materialization
before they have a canonical `location_id`; `proposeLocation` does that
materialization today and returns the resolved id via the
`ConfirmLocation` response (see the comment at
[location_modal_view_model.dart:349-356](app/lib/presentation/viewmodels/location_modal_view_model.dart#L349-L356)).

## Potential Solutions

### Option A: Branch in `_submit()`, reuse propose-then-confirm

**Approach:** In both modals, branch on staged count.

- count == 1: propose the single candidate, then immediately confirm.
  - Location: call a new `LocationModalViewModel.setSingleLocation(...)`
    that wraps `proposeLocation` + `confirmLocation` (handling both
    locationId and geocoded staged shapes).
  - Time: add a `setSingleTime(date, time)` analogue, or refactor
    `finalizeEventTime` to accept an explicit `DateTime` so it can be
    driven off `_stagedTimes` instead of the form-state fields.
- count > 1: keep the existing poll-creation path.

**Pros:**
- Smallest surface area; no new RPCs.
- Reuses well-tested primitives (`confirmLocation`, `confirmTime`).
- Server-side behavior for "confirmed location/time" is already what
  we want — `locationPollCompleted` / `timePollCompleted` end up true,
  poll flags end up false.

**Cons:**
- The server still briefly sees `LocationPollActive`/`TimePollActive`
  flip to `true` on `ProposeLocation`/`ProposeTime` before
  `ConfirmLocation`/`ConfirmTime` flips it back to `false`. That
  transient flip is the trigger for the "poll opened" system chat
  message — if it fires before the confirm settles, users see a
  spurious "poll opened" event in the chat log. **This is the main
  risk and must be verified.**
- Two round-trips instead of one.

**Effort:** Small (frontend-only if the chat-message timing is okay) or
Medium (if we need server-side suppression).

### Option B: Use `SaveExperience` for the direct-set case

**Approach:** When count == 1, skip propose entirely and call
`SaveExperience` with `locationId` (or `time`) set on the existing
experience.

- For location:
  - Staged locationId: pass directly.
  - Staged geocoded: first materialize via a single
    `ProposeLocation` call (the cheapest way to resolve a drop-pin to
    a canonical location id today) — *or* extend `SaveExperience` /
    introduce a `MaterializeLocation` RPC. Then call `SaveExperience`.
- For time: build the `ExperienceTime` proto inline and call
  `SaveExperience` — mirrors `updateEventTime()` exactly.

**Pros:**
- Truly no-poll path: no transient `*PollActive=true`, no chance of a
  stray "poll opened" chat message.
- For time, the path already exists (`updateEventTime`).

**Cons:**
- Location side still needs to resolve geocoded inputs to a
  `location_id`, which today is a side effect of `ProposeLocation`.
  Either accept one `ProposeLocation` round-trip (then defeats the
  "no poll flip" advantage) or build a new RPC.
- `SaveExperience` requires re-passing all existing fields
  (`updateEventTime` already shows this pattern at lines 517-530) —
  if any field is out of date in the client cache, this can stomp it.
  Has to fetch fresh details first.
- Diverges location and time paths: location ends up doing propose +
  confirm, time ends up doing SaveExperience.

**Effort:** Medium.

### Option C: New RPC — `SetExperienceLocation` / `SetExperienceTime`

**Approach:** Add server RPCs that atomically set the location/time and
mark the experience confirmed, with no poll state involved.

**Pros:**
- Cleanest semantics on the wire; bypasses every transient-state
  concern in Options A/B.
- Future-proof for other "set directly" entry points (e.g. the
  experience-creation form, deep links).

**Cons:**
- Largest change; touches proto, server services, and client.
- Duplicates work that `SaveExperience` already does. Hard to justify
  unless we plan to deprecate the `SaveExperience` full-payload
  pattern.

**Effort:** Large.

## Recommendation

**Option A**, contingent on a quick verification that confirming a
freshly-created proposal does **not** leave a "poll opened" system
chat message behind. The check is cheap — run the flow on a dev
instance and inspect the chat log + `community_events` table for the
experience.

- If clean: ship Option A.
- If a spurious message leaks: take Option B for the time side
  (mirrors existing `updateEventTime`) and an extended Option A for
  the location side, plus a small server-side guard that suppresses
  the "poll opened" emit when the same RPC chain confirms in the
  same transaction. (Server-side suppression is the durable fix; it
  also future-proofs the propose-then-confirm pattern used by
  `finalizeEventTime`.)

Why not C: this is a UI-flow polish, not a new capability. The
existing primitives compose to the desired behavior.

## Open Questions

- **Does propose-then-confirm in rapid succession leak a "poll opened"
  system chat message?** Recommendation: verify on dev first (Phase 1).
  This is the single load-bearing question for picking A vs B.
- **For location with a single staged geocoded drop-pin, is one
  `ProposeLocation` round-trip acceptable?** Recommendation: yes — it
  doubles as the geocoded-materialization path, and a future server
  optimization could merge propose + confirm into one round trip
  without breaking the client.
- **Should the modal close immediately on tap, or wait for the
  confirm to settle?** Recommendation: wait — matches the existing
  poll-create path which `await`s before `Navigator.pop()`, and keeps
  error toasts attached to the modal context.
- **Live-poll edit mode (`isLivePollEdit == true`).** The submit button
  is already hidden in this branch, so the bug does not apply. No
  change needed — but the new branching should keep that assumption
  intact. Recommendation: assert it with a precondition in the new
  helpers (`assert(!current.locationPollActive)` etc.).
- **What if the user stages one location, the modal sets it directly,
  and a previous poll for that experience has `proposals` carried
  over for history?** Recommendation: existing logic already filters
  `existingProposals` by `currentLocationPollId` on render
  (see lines 287-296 of the location modal). Direct-set should not
  open a new poll, so this filtering remains correct.

## Implementation Plan

### Phase 1: Verify server behavior

**Goal:** Determine whether propose-then-confirm leaks a "poll opened"
chat message, which decides A vs B.

**Tasks:**
- [ ] Trace `ProposeLocation` / `ProposeTime` server handlers; locate
      the code that emits the "poll opened" community/chat event when
      `*PollActive` transitions false → true.
- [ ] On a dev instance, run propose → confirm back-to-back and
      inspect the chat log + relevant tables. Confirm whether a
      "poll opened" or analogous message is persisted.
- [ ] Document findings in the issue thread.

**Validates:** Whether Option A is sufficient, or we need server-side
suppression / Option B for time.

### Phase 2: Client branching + viewmodel helpers

**Goal:** Land the single-candidate direct-set path behind the
existing CTA labels.

**Layer contract:** All new logic lives in viewmodels and is
invoked from the modal widgets via `ref.read(...).notifier`. No
new service or repository calls are introduced from the widget
layer — the modals only call the helpers added here.

**Tasks:**

- [ ] Add `LocationModalViewModel.setSingleLocation({ String? locationId,
      GeocodedLocation? geocoded })` in
      [location_modal_view_model.dart](app/lib/presentation/viewmodels/location_modal_view_model.dart):
  - Precondition: `assert(!state.requireValue.locationPollActive)` so the
      caller never invokes this in live-poll-edit mode.
  - Body: call `_repository.proposeLocation(...)` (passing whichever of
      `locationId` / `geocoded` is non-null), then `_repository.confirmLocation(...)`
      with the returned `proposalId`.
  - After each `await`, check `if (!ref.mounted) return;` before any
      state assignment — required because `locationModalProvider` is
      autoDispose and the modal can be dismissed mid-confirm.
  - Optimistically and finally `_syncExperience((exp) { exp.locationId =
      resolvedLocationId; exp.locationPollActive = false;
      exp.locationPollCompleted = true; })` to mirror the existing
      `confirmLocation` flow at
      [location_modal_view_model.dart:385-388](app/lib/presentation/viewmodels/location_modal_view_model.dart#L385-L388),
      so the experience-detail and daily-feed consumers of the cached
      proto don't render stale poll state.
- [ ] Add `TimeModalViewModel.setSingleTime(DateTime dt)` in
      [time_modal_view_model.dart](app/lib/presentation/viewmodels/time_modal_view_model.dart):
  - Built on top of (or factoring out of) the propose-then-confirm shape
      in [finalizeEventTime](app/lib/presentation/viewmodels/time_modal_view_model.dart#L548).
      Drive it off the explicit `DateTime` argument rather than form
      state so the staged-candidate caller doesn't have to write through
      `proposeDate` / `proposeTime`.
  - Same precondition (no active time poll) and same `ref.mounted`
      guards after every `await`.
  - End with `ref.invalidateSelf()` (matches `finalizeEventTime`'s
      final line at
      [time_modal_view_model.dart:613](app/lib/presentation/viewmodels/time_modal_view_model.dart#L613)).
- [ ] In [location_poll_propose_modal.dart `_submit()`](app/lib/presentation/screens/experience/location_poll_propose_modal.dart#L187-L214):
      branch on `_totalStagedCount == 1`. If the single staged item is a
      locationId, call `setSingleLocation(locationId: ...)`; if it's a
      `GeocodedLocation`, call `setSingleLocation(geocoded: ...)`. Keep
      the existing propose-sequentially path for `_totalStagedCount > 1`.
- [ ] In [time_poll_propose_modal.dart `_submit()`](app/lib/presentation/screens/experience/time_poll_propose_modal.dart#L242-L274):
      branch on `_stagedTimes.length == 1` and call `setSingleTime(dt)`;
      keep `createTimePoll(options)` for `> 1`.
- [ ] Unit tests in `app/test/presentation/viewmodels/`
      (`location_modal_view_model_test.dart`, `time_modal_view_model_test.dart`):
  - Set up `ProviderContainer` with `overrides: [...]` injecting a
      Mockito-generated mock repository — pattern documented at
      [docs/client/testing.md#viewmodel-testing-primary-focus](../client/testing.md#1-viewmodel-testing-primary-focus).
  - `setSingleLocation` happy path (locationId variant): verify
      `proposeLocation` then `confirmLocation` are called once each,
      in order, and final state has `eventLocationId` set,
      `locationPollActive = false`, `locationPollCompleted = true`.
  - `setSingleLocation` happy path (geocoded variant): same assertions,
      verifying the geocoded shape is forwarded.
  - `setSingleLocation` precondition: calling while
      `locationPollActive == true` should fail the assert (or throw)
      so future drift to live-edit mode is caught.
  - `setSingleTime` happy path mirroring the above for time.
  - **Disposal-safety test** (one per helper, required by
      [docs/client/architecture.md#disposal-testing](../client/architecture.md#disposal-testing)):
      start the helper, dispose the container before the second
      `await` returns, assert the future completes without throwing.
- [ ] Run `npm run build` (Go server side — no change here, just a
      smoke run since the integration tag is irrelevant for client-only
      changes) and `npm run test:app` to confirm.

**Validates:** End-to-end UX matches the singular CTA label without
breaking the multi-option poll flow, autoDispose disposal-during-await,
or the cached-experience consumers downstream.

### Phase 3: Server-side suppression (only if Phase 1 found a leak)

**Goal:** Eliminate the spurious "poll opened" chat message for the
propose-then-confirm pattern.

**Tasks:**
- [ ] Suppress the "poll opened" emit when the same actor confirms
      within the same RPC chain — likely by deferring the
      community-event emit until poll close-out, or by detecting a
      single-proposal poll that is immediately confirmed.
- [ ] If a new internal helper or RPC path is introduced, return
      errors via `connecterr.Internal(ctx, op, err, kv...)` — never
      bare `connect.NewError(connect.CodeInternal, err)` (gated by
      `scripts/check_codeinternal.js`, see
      [docs/server/architecture.md#error-wrapping](../server/architecture.md#error-wrapping)).
- [ ] Use `logging.LoggerWithContext(ctx).With("operation", "...",
      "experience_id", ...)` for any new log lines on this path.
      Don't emit raw `log.Printf`; level the lines per
      [docs/server/observability.md#appropriate-log-levels](../server/observability.md#appropriate-log-levels)
      (INFO for the normal direct-set, WARN for unexpected races
      between the propose and confirm, ERROR only if a system
      dependency fails).
- [ ] Add a server-side integration test covering the
      propose-then-confirm chat-message behavior. New `Undo*` or
      direct-set RPCs (if any) must also be classified in
      `server/undo/registry.go` per `CLAUDE.md` — though a direct
      `SaveExperience`-style path is `ClassificationClientOnly`
      (the inverse is just re-tapping the field and choosing a
      different value).
- [ ] Run `npm run build` and `go build -tags=integration ./...`
      before pushing if any exported signature in `server/services/experience/`
      changed.

**Validates:** No regression on the existing multi-option poll path,
and no spurious chat message on the new direct-set path.

### Phase 4: QA pass

**Goal:** Cover the matrix.

**Tasks:**
- [ ] Manual repro per the issue's repro steps, for both location
      and time, for both staged-locationId and staged-geocoded
      variants of location.
- [ ] Verify the multi-option flow ("Ask the group") still creates a
      poll as before.
- [ ] Verify live-poll edit mode is unaffected (submit button still
      hidden).
- [ ] Run `npm run lint:dart:l10n-parity` to confirm ARB parity is
      preserved (no new keys are added by this work, but the
      in-flight diff to `app_en.arb` / `app_es.arb` on the branch
      should still pass parity).
- [ ] Run `npm run lint:dart:size` to confirm neither modal file
      crossed the 1000-line gate (current sizes are well under, but
      the precondition + branching add a handful of lines).
- [ ] Run `npm run test:app` for the full Flutter test pass.

**Validates:** Behavior matches the issue's expected outcome across
all surfaces, with no regression to the poll flow.

## Related Issues

- None linked. The propose-then-confirm pattern at
  [time_modal_view_model.dart:548 `finalizeEventTime`](app/lib/presentation/viewmodels/time_modal_view_model.dart#L548)
  is the closest existing analogue and a likely model for the
  location-side helper.
