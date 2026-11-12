# Issue #1062: Simulator schedules RSVPs beyond experience max participants

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1062
**Filed:** 2026-04-07 | **Last activity:** 2026-04-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The simulator's activity-timeline generator emits an `ActionRSVP` (yes)
step for every community member who rolls above the persona attend
threshold, with no reference to the experience's `max_participants`
capacity. When the server hits capacity, every subsequent `yes` RSVP is
rejected with `resource_exhausted: experience has reached maximum
participants in this community`.

This is a low-frequency but persistent noise source (2/4053 steps in the
canonical seed-42 run of `suburban-neighborhood`), and the failure rate
scales directly with the ratio of community size to template
`max_participants`. Scenarios with larger communities or stricter
template caps will surface it more.

## Reproduction and evidence

Reproduced locally with `go run ./server/cmd/simulate -scenario
suburban-neighborhood -seed 42 -url http://localhost:8080 -assets-dir
server/simulation/assets`:

```
WARN activity step failed step=3373 action=RSVP actor=chris@sim.ripls.app
     ref=exp-karen@sim.ripls.app-0309-2246
     error="RSVP: resource_exhausted: experience has reached maximum
     participants in this community"
WARN activity step failed step=3390 action=RSVP actor=brian@sim.ripls.app
     ref=exp-karen@sim.ripls.app-0309-2246
     ...
```

Both failures target the same experience, which had hit its
`max_participants` by the time those steps ran. The root cause is in
`server/simulation/activity.go:513-546` inside `generateExperienceFlow`:

```go
for _, member := range comm.Members {
    if member.Email == host.Email { continue }
    // ...
    if roll < attendWeight*2 {
        steps = append(steps, ActivityStep{Action: ActionRSVP, ...})
    } else if roll < attendWeight*2+0.15 {
        steps = append(steps, ActivityStep{Action: ActionRSVPNo, ...})
    }
}
```

There's no counter, no cap on yes-RSVPs, and no reference to
`tmpl.MaxParticipants`. The executor at `executor.go:598-604` passes the
template's `MaxParticipants` through to `SaveExperience` unchanged
(`MaxParticipants: int32(tmpl.MaxParticipants)`), so the server
enforces a ceiling that the timeline never respects.

`tmpl.MaxParticipants == 0` in the template JSON means unlimited (per
the comment at `types.go:132`), so the fix must preserve that
semantics.

## Context from Comments

The issue has no comments. The reporter correctly identified both the
location (`activity.go`, RSVP generation) and the right shape of the
fix (track per-experience attendee count and stop scheduling at cap).
Implementation is straightforward.

## Potential Solutions

### Option A: Cap yes-RSVPs in the generator
- **Approach:** In `generateExperienceFlow`, track a running count of
  scheduled `ActionRSVP` (yes) steps for this experience. When the
  count reaches `tmpl.MaxParticipants - 1` (host excluded), stop
  emitting further yes-RSVPs. Keep emitting `ActionRSVPNo` freely
  (declines never consume capacity). `MaxParticipants == 0` disables
  the cap.
- **Pros:** Local change, one function, deterministic under a given
  seed, matches reporter's suggested fix. No state plumbing required —
  the cap is enforced within a single flow.
- **Cons:** None of note; the host/member loop already iterates once,
  so the counter is free.
- **Effort:** Small.

### Option B: Probabilistic down-scaling of attend rolls
- **Approach:** Scale each member's `attendWeight` by
  `max_participants / (members − 1)` so the *expected* number of
  yes-RSVPs stays under the cap. Keeps statistical intent (each member
  rolls independently).
- **Pros:** Smooth distribution of attendees across experiences; no
  hard cutoff where the last few members always say no.
- **Cons:** Doesn't eliminate tail-risk of overshoot; needs a fallback
  hard cap anyway; harder to reason about in tests.
- **Effort:** Small–medium.

### Option C: Executor-side skip
- **Approach:** In `executeRSVP`, catch `resource_exhausted` and treat
  it as a non-error (skip). Convert capacity rejections into
  `StepsSkipped` rather than `StepsFailed`.
- **Pros:** Trivial, requires no state tracking.
- **Cons:** Papers over a generator bug; every failed RSVP still costs
  an RPC; the simulator no longer reflects intent (it schedules work
  it *knows* will fail). Poor fit for a deterministic correctness tool.
- **Effort:** Trivial.

## Recommendation

**Option A.** Matches the reporter's suggested shape, fixes the bug at
the source, keeps the simulator deterministic, and has no downsides.
Option B is a nice-to-have that can layer on top later if we want
smoother attendance curves — but it doesn't fix the bug on its own.
Option C is a non-starter; fixing the symptom, not the cause.

## Open Questions

- **Should capacity consider maybe/tentative RSVPs?** — Recommendation:
  no. The generator only emits yes/no today; the server enforces the
  cap on accepted yes-RSVPs. Keep parity. If a future
  `ActionRSVPMaybe` is added, revisit.
- **Should the cap reserve headroom for late-arriving "no→yes"
  changes?** — Recommendation: no. The generator doesn't model RSVP
  changes; treat `max_participants` as the hard ceiling the server
  sees, because that's what it is.
- **Host counts toward capacity?** — **Resolved (Phase 1):** Yes. In
  `server/services/experience/share.go:153-164`, `ShareExperience`
  auto-creates a YES `ExperienceRSVP` for the host. The capacity
  check in `rsvp.go:56-79` counts all YES RSVPs excluding only the
  user currently RSVPing. So the effective cap on *non-host*
  yes-RSVPs is `MaxParticipants - 1`. Use that in the generator.

## Implementation Plan

### Phase 1: Confirm host accounting
**Goal:** Know the exact off-by-one to use.
**Tasks:**
- [ ] Read the `SaveExperience` / RSVP path in
      `server/services/experience/` to confirm whether the host
      occupies a participant slot.
- [ ] Record the answer in this plan under "Open Questions" before
      implementing.
**Validates:** The cap formula is correct the first time.

### Phase 2: Fix generator
**Goal:** No yes-RSVP step is ever scheduled beyond the experience's
`max_participants`.
**Tasks:**
- [ ] In `generateExperienceFlow`
      (`server/simulation/activity.go`), maintain a `yesCount` local
      counter.
- [ ] Before appending an `ActionRSVP` step, check
      `tmpl.MaxParticipants == 0 || yesCount < effectiveCap`. If full,
      skip (or flip to `ActionRSVPNo` — decide in Phase 1; skipping is
      simpler).
- [ ] Iterate `comm.Members` in a deterministic shuffled order seeded
      by `rng`, so capacity filling doesn't always bias toward the
      first members listed.
- [ ] Add a unit test in `server/simulation/activity_test.go` that
      generates a timeline with a small-capacity experience and
      asserts the per-experience yes-RSVP count never exceeds the
      cap.
**Validates:** Deterministic correctness at generation time.

### Phase 3: Regression gate
**Goal:** Prove the end-to-end failure is gone and stays gone.
**Tasks:**
- [ ] Rerun `suburban-neighborhood` at seed 42 and confirm zero
      `experience has reached maximum participants` failures.
- [ ] Add an assertion in the runner (or a scenario smoke test) that
      flags any occurrence of this specific error — like
      `cannot express interest in your own gear`, it has no
      legitimate cause in a correctly-generated timeline.
**Validates:** The bug doesn't silently come back under future changes.

## Related Issues
- #1061 — Simulator gives wrong actor credentials on
  `ExpressInterestGiveaway`. Same simulation run, same file family,
  same class of "timeline vs. runtime mismatch" problem. Bundle the
  fixes into the same PR / branch.

## Audit notes (ripls-audit, 2026-04-21)

Checked against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`. Findings:

- **Client docs: N/A.** Change is confined to `server/simulation/`
  (Go test harness); no Flutter code is affected.
- **Server architecture — testing strategy:** Plan's Phase 2
  adds a unit test of the generator in `activity_test.go` that
  seeds a small-capacity experience and asserts the per-experience
  yes-RSVP count never exceeds the cap. That's the preferred
  "local, isolated" unit test level from
  `docs/server/architecture.md §4`. No integration test needed —
  the fix is pure generator state, no server interaction.
- **CLAUDE.md — minimal change:** plan keeps the fix to a single
  function (`generateExperienceFlow`) with a local counter. No new
  state in `state.go`; capacity enforcement is per-flow, which is
  the simplest correct place for it. Good.
- **CLAUDE.md — fail-fast:** Phase 3's runner assertion that any
  `experience has reached maximum participants` error fails the
  run aligns with the "Fail Fast" principle — the condition is a
  generator bug, not a legitimate server response in a correctly
  built timeline.
- **Determinism:** Plan's Phase 2 specifies iterating
  `comm.Members` in a deterministic shuffled order seeded by `rng`.
  This matches the simulator's "deterministic for a given seed"
  invariant established in the rest of `activity.go` (every
  `rng.Intn` / `rng.Float64` call is driven by the scenario seed).
  Keeping the seed as the sole source of randomness is essential
  for the diagnostic reproducibility that `sim_profile.sh` and
  scenario regression tests rely on.
- **No architectural violations found.** Plan is ready to implement.
