# Issue #1920: Regression in chat message ordering

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1920
**Filed:** 2026-05-13 | **Last activity:** 2026-05-13
**Labels:** _none_ | **Assignees:** unassigned
**Context docs:** none (PR #1829 / issue #1826 reviewed for prior art)

**Audit (`/ripls-audit`)** — checked against `docs/server/architecture.md`,
`docs/server/observability.md`, `docs/client/testing.md`,
`docs/client/architecture.md`, `docs/client/accessibility.md`,
`docs/client/caching.md`, `docs/client/i18n.md`. Five refinements
applied inline (all marked `[audit]`): file-size split for
`activity.go`, explicit `npm run build` gate, `flutter test --reporter
expanded` flag, alignment with the existing fake-injection pattern in
the target test file, and explicit confirmation that no new server
logging is needed. Core reverse-construction design unchanged.

## Problem Statement

The reporter believes #1829 (which fixed #1826 by making chat backlog
ordering deterministic) introduced a regression: "old" messages now
appear *below* "new" ones in conversations. Two screenshots are
attached:

- **Screenshot A** (camping-stove loan): four simulator user-messages
  labeled "7m ago" at top, then a user-typed message labeled **"now"**,
  then two system pills "The loan has started" and "This transfer has
  been completed" *underneath* the "now" message.
- **Screenshot B** (pressure-washer loan): "Marcus 14m ago" at top,
  then "The loan has started", then three simulator-style messages
  labeled **"now"**, then "This transfer has been completed" at the
  bottom.

**Root cause is not in #1829's sort code.** `_sortByChronology`
correctly orders messages ascending by `(sentAtUnixSec, messageId)`.
The messages at the visual bottom legitimately *have* a higher
`sentAtUnixSec` than the "now" message — because the simulator
(`server/simulation/activity.go`) schedules lifecycle steps at times
that can exceed `scenario.EndTime` (= wall-clock `time.Now()`),
producing rows whose stored `sent_at_unix_sec` is **in the future**
relative to real time.

**Verified against local dev Postgres** (2026-05-14):

```
                  total_messages |    6895
                 future_messages |      64
            max_future_offset_sec | 2408617   (~28 days)
```

```
  kind   | action | count | max_days_future
---------+--------+-------+----------------
 system  | 7      |   35  |      27         -- CHAT_SYSTEM_ACTION_COMPLETED
 system  | 6      |   17  |      27         -- CHAT_SYSTEM_ACTION_STARTED
 system  | 10     |    6  |       2         -- CHAT_SYSTEM_ACTION_RSVP_YES
 user    | (none) |    4  |       0         -- ~hours into future
 system  | 13     |    1  |       0         -- ADDED_VIA_MENTION
 system  | 5      |    1  |       0         -- CANCELLED
```

`STARTED` + `COMPLETED` pills trail the conversation, spread up to
27 days in the future — exactly what the screenshots show. The
distribution maps precisely to `generateLoanFlow` at
`activity.go:366,376`:

```go
startTime    := baseTime.Add(jitterDuration(rng,  24*time.Hour,  72*time.Hour))   // +1–3d
completeTime := startTime.Add(jitterDuration(rng, 7*24*time.Hour, 30*24*time.Hour)) // +7–30d
```

`baseTime` is already clamped to `≤ scenario.EndTime` (`activity.go:267`),
but derived events grow forward without bound. The same forward-growth
pattern appears in `generateExperienceFlow` (RSVP deadlines + completion
durations), `generateGiveawayFlow`, and `generateRequestFlow`. The
simulator then executes those steps with `pool.SetTimestamp(step.Time)`
(`executor.go:73`), and the server stores
`sent_at_unix_sec = clock.UnixSec(ctx) = step.Time.Unix()` — a future
epoch second.

**Why messages render as "now":** `timeago.format(timestamp)` is called
without `allowFromNow: true` (`chat_bubble.dart:137`, `:215`), so
future timestamps fall into the `lessThanOneMinute` bucket and render
as the literal string "now" — masking the underlying future timestamp.

**Why #1829 made this visible:** before #1829 the client never
re-sorted on state merges; messages displayed in arrival order (heap
order from `QueryByField`, or insertion order from the stream).
Future-timestamped messages were hidden inside that noise. After
#1829, `_sortByChronology` deterministically places them where their
`sent_at_unix_sec` says they belong — at the bottom — exposing a
latent data-generation bug.

## Context from Comments

Issue #1920 itself has no comments. The relevant context is in
**PR #1829**:

- The PR explicitly traded heap-order chaos for strict ascending
  order: server uses `ListByConversation … ORDER BY sent_at_unix_sec
  ASC, id ASC`, client adds `_sortByChronology` on every state-merge
  path.
- The PR added a regression test
  (`server/services/chat/streaming_test.go:TestStreamMessages_Backlog…`)
  that proves the backlog is ordered, plus four Flutter tests in
  `conversation_messages_ordering_test.dart`.
- Neither side tested `sentAtUnixSec > now`. The fix is robust to
  #1826's symptom (heap permutation of messages 20 minutes apart) but
  not to bad upstream data.

The reporter's diagnosis ("#1829 broke ordering") is partly right —
#1829 *revealed* the issue — but the durable fix is upstream, in the
simulator's timeline generator. Reverting #1829 would restore the
appearance of order without fixing the data.

## Approach: Reverse-Construct the Timeline

The simulator currently builds flows **forward** from a `baseTime`
chosen inside `[StartTime, EndTime]`, then layers derived offsets
that can grow past `EndTime`. The principled fix is to build each
flow **backward** from a completion anchor chosen inside the
scenario window:

```go
// generateLoanFlow — backward planning
// 1. Pick a completion time inside the scenario window. Some flows
//    are still in-progress at "now": for those, we pick a StartLoan
//    time near EndTime and emit no CompleteLoan step.
completeTime := pickWithin(rng, scenario.StartTime, scenario.EndTime)
startTime    := completeTime.Sub(jitterDuration(rng, 7*24*time.Hour, 30*24*time.Hour))
baseTime     := startTime.Sub(jitterDuration(rng,   24*time.Hour,   72*time.Hour))
```

This makes the invariant `step.Time ≤ scenario.EndTime` a
**construction property** of every flow generator, rather than
something patched on after the fact. A small assertion test
(`max(step.Time) ≤ scenario.EndTime` for every scenario, every flow
type) guards against regression.

In-progress flows are a first-class case: instead of always picking
a `completeTime` and deriving backward to baseTime, a fraction of
flows pick a `startTime` (or earlier) near `EndTime` and stop —
producing realistic "loan started yesterday, not yet returned"
states that the data needs anyway.

Two alternatives considered and rejected:

- **Clamp / skip steps post-generation.** Generate forward as today,
  then drop any step with `Time > EndTime`. Reactive rather than
  preventive; leaves the latent forward-growth design intact for the
  next person to step on. Rejected.
- **Server-side guard in `clock.UnixSec`.** Cap the returned value at
  wall-clock now. Silently rewrites the simulator's contract
  ("messages must appear at step.Time on the wire" —
  `executor.go:72`); hides bugs instead of surfacing them. Rejected.

A client-side display clamp was also considered and rejected per the
saved feedback `feedback_fix_root_cause` (prefer tests that find bugs
over client workarounds).

## Open Questions

- **What's the mix of "completed" vs. "in-progress" flows?** —
  Recommendation: keep the existing cancellation rates (e.g., 8% of
  loans cancel) and add a parallel in-progress rate (e.g., ~15–20%
  of non-cancelled loans started within the last 30 days are still
  in-flight at `EndTime`). Tunable per scenario; the unit test
  asserts a non-zero in-progress count so the simulator keeps that
  state shape rather than always producing finished histories.
- **Should chat sub-flows in `sendChatMessages` also reverse-plan?** —
  Recommendation: **Yes.** The 4 future *user* messages in local dev
  came from forward `msgTime` advance crossing `EndTime`. Either
  anchor the last chat message at the parent step's `Time` and step
  backward, or accept that chat sub-flows precede the parent step
  and bound their forward advance by the next sibling step's `Time`.
  Either is fine; the invariant test will catch a missed case.
- **Clean up the 60 future-timestamped rows already in local dev?** —
  **No.** The user will reset the DB and replay the simulation after
  the simulator fix lands, so a backfill is unnecessary.
- **Add `allowFromNow: true` to `timeago.format` so any remaining
  future timestamps render as "in 2d" instead of "now"?** —
  Recommendation: **No, not as part of this fix.** Conflating display
  changes with the simulator fix muddies intent. Optional follow-up
  below.

## Implementation Plan

### Phase 1: Invariant test (must fail on `main`)

**Goal:** Capture the bug as a deterministic, fast-running unit test
before any production code changes.

**Tasks:**

- [ ] Add `server/simulation/activity_test.go::TestTimelineNeverExceedsEndTime`:
  for each representative scenario in `scenarios.go`, call the
  timeline generator with a fixed-seed `rng` and assert
  `max(step.Time over timeline) ≤ scenario.EndTime`. Use
  table-driven sub-tests per scenario as per
  `docs/server/architecture.md` §"Unit Tests" — one row per
  scenario, deterministic seed.
- [ ] Add `TestChatSubFlowsRespectDeadline`: drive
  `sendChatMessages` with a `msgTime` near the deadline and assert
  no produced message exceeds it.
- [ ] Run both — confirm they fail on `main`.

**Validates:** The failure mode is reproducible deterministically
without a database or executor. The test is the durable specification
of the invariant.

### Phase 2: Reverse-construct each flow generator

**Goal:** Move every flow generator to backward planning anchored at
a completion (or in-progress) time inside `[StartTime, EndTime]`.

**Tasks:**

- [ ] **[audit]** Split `activity.go` (currently 756 lines; reverse
  construction will push it past CLAUDE.md's 1000-line soft cap) into
  per-flow files alongside a shared helper:
  - `server/simulation/activity.go` — `BuildTimeline`, shared types,
    `pickAnchor` helper, the per-week loop that dispatches to flow
    generators.
  - `server/simulation/loan_flow.go` — `generateLoanFlow`.
  - `server/simulation/giveaway_flow.go` — `generateGiveawayFlow`.
  - `server/simulation/experience_flow.go` — `generateExperienceFlow`.
  - `server/simulation/request_flow.go` — `generateRequestFlow`.
  Use `git mv`-equivalent moves so blame is preserved (or extract
  into new files and delete from `activity.go` in the same commit
  so reviewers can see the move).
- [ ] Introduce a small helper in `activity.go`:
  ```go
  // pickAnchor returns a time inside [scenario.StartTime, scenario.EndTime]
  // suitable as a flow's terminal step time.
  func pickAnchor(rng *rand.Rand, scenario Scenario) time.Time
  ```
- [ ] Rewrite `generateLoanFlow`:
  - With probability `pComplete`: pick `completeTime := pickAnchor(...)`,
    derive `startTime`, `baseTime` backward, emit
    `[ExpressInterest, StartLoan, CompleteLoan]`.
  - With probability `pInProgress`: pick `startTime` near `EndTime`,
    derive `baseTime` backward, emit `[ExpressInterest, StartLoan]`
    only.
  - With probability `pCancel` (existing 8%): pick `cancelTime`
    backward from a `baseTime` inside the window, emit
    `[ExpressInterest, CancelTransfer]`.
- [ ] Rewrite `generateGiveawayFlow` analogously (completed /
  in-progress / cancelled).
- [ ] Rewrite `generateExperienceFlow`: anchor at
  `completeExperienceTime` (or `startExperienceTime` for in-progress);
  RSVPs and DETAIL_CHANGED events derive backward from there. Skip
  RSVPs whose derived time would precede `scenario.StartTime`.
- [ ] Rewrite `generateRequestFlow`: anchor at `fulfillTime` (or
  `latestOfferTime` for in-progress).
- [ ] Rewrite `sendChatMessages` (`chat.go`) so chat sub-flow
  messages count backward from the parent step's `Time` (with a
  configurable spacing), OR thread the next sibling step's `Time`
  as an upper bound and break on overshoot. The invariant test
  decides which path is acceptable.
- [ ] **[audit]** Confirm no new server logging is needed: the
  simulator already logs step execution via `slog.Warn(...)` on
  errors (`chat.go:107`, `executor.go:76`, etc.), and timeline
  generation is offline / pre-execute — there is nothing for
  `logging.LoggerWithContext(ctx)` to attach to. Don't introduce
  any new log lines from the generators.
- [ ] **[audit]** After Go changes land, run `npm run build` per
  CLAUDE.md (executes `golangci-lint` + `go build ./...`). Fix any
  lint hits before opening the PR. No exported-API signatures
  change, so `go build -tags=integration ./...` is not required.

**Validates:** `TestTimelineNeverExceedsEndTime` and
`TestChatSubFlowsRespectDeadline` pass. Re-seeding produces zero
future-timestamped chat rows. `npm run build` clean.

### Phase 3: Realism test + docs

**Goal:** Guard the new "some flows are in progress" property and
document the invariant where future contributors will see it.
(Existing future-timestamped rows are not cleaned up — the user will
reset the dev DB and replay the simulation after Phase 2 lands.)

**Tasks:**

- [ ] Add `TestTimelineIncludesInProgressFlows`: re-run timeline
  generation across all scenarios and assert a non-zero count of
  flows that emit StartLoan-without-CompleteLoan (and similarly for
  other flow types). This locks in the "snapshot at now" shape so a
  future tweak doesn't silently re-flatten the data to all-completed.
- [ ] Update `server/services/chat/README.md` "ordering invariant"
  section: _"This invariant assumes `sent_at_unix_sec` is ≤
  wall-clock time at insert. Producers must enforce that; the
  client and storage layer do not clamp."_
- [ ] Add a Flutter test in
  `app/test/presentation/viewmodels/conversation_messages_ordering_test.dart`
  that feeds a history containing a `sentAtUnixSec` 7 days in the
  future and asserts the optimistic `sendMessage` message sorts
  _before_ it. Documents current (correct) client behavior — the
  display layer faithfully sorts whatever timestamps it receives.
  **[audit]** Follow the existing pattern in that file: local
  `_FakeChatService` / `_Fake*Repository` classes wired via
  `ProviderContainer` overrides (per `docs/client/testing.md`
  §"ViewModel Testing"). Do *not* mix in Mockito-generated mocks —
  the file is committed to the fake-injection style. Run with
  `flutter test --reporter expanded` per CLAUDE.md (the default
  `compact` reporter mangles output when piped).

**Validates:** Future contributors who introduce another future-
timestamp producer will see the test + docs pointing at the right
layer.

### Phase 4 _(optional, deferred)_: `timeago` future-time display

**Goal:** Defense-in-depth — if future-timestamped data sneaks in
(clock skew, a producer not yet covered), render it honestly instead
of as "now".

**Tasks:**

- [ ] Change `timeago.format(timestamp)` calls in `chat_bubble.dart`
  to `timeago.format(timestamp, allowFromNow: true)`.
- [ ] Override `prefixFromNow` / `suffixFromNow` in `ShortMessages`
  (`app/lib/core/utils/timeago_short_messages.dart`) with short
  labels ("in 2d"). **[audit]** This continues the existing
  pattern: `ShortMessages` already hard-codes English ("1m ago",
  "2h ago", etc.) because it's the `LookupMessages` implementation
  registered with `en_short`. It does *not* route through ARB
  files, and adding "in 2d" stays consistent. If/when we
  localize to Spanish, the entire `ShortMessages` file moves to
  ARB — that's a separate refactor, not a regression introduced
  here. Per `docs/client/i18n.md`, ViewModels still never resolve
  strings; this is the formatter layer, which sits inside the
  widget tree.
- [ ] One widget test confirming a 2-day-future timestamp renders
  as "in 2d" rather than "now".

**Validates:** Intentionally _not_ in Phases 1–3 so the core fix
stays focused.

## Related Issues

- **PR #1829** — added strict client-side `_sortByChronology` that
  surfaced this latent bug. No revert needed; #1829 is correct.
- **Issue #1826** — original chat-ordering bug that #1829 fixed.
- Potential follow-up: separate display issue for future-timestamp
  formatting (Phase 4 above), if not bundled.
