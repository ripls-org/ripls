# Issue #1061: Simulator picks gear owner as interested party in giveaway flow

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1061
**Filed:** 2026-04-07 | **Last activity:** 2026-04-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Every run of the `suburban-neighborhood` simulation produces ~43
`ExpressInterestGiveaway` failures with the server error
`invalid_argument: cannot express interest in your own gear`. The reporter
assumed the timeline generator was picking the gear owner as the interested
actor. **It isn't.** A deterministic reproduction (seed 42) plus a timeline
diagnostic and a code read shows the step's `Actor` field is correctly a
non-owner. The actual bug is in the *executor*, not the generator: the
simulator's shared `Client` object gets its auth token silently flipped to
the gear owner and never restored before the `ExpressInterest` RPC fires.

## Reproduction and evidence

Reproduced locally (seed 42, `suburban-neighborhood`): **43/43 failures**
match, each reporting a step like `actor=alex ref=gear-...-30-giveaway-123`.

A small diagnostic (`server/simulation/diag_giveaway/main.go`,
written during investigation) generates the same deterministic timeline
and for every failing ref prints the step's intended `Actor`, the gear's
original creator, and any prior giveaway recipient. Summary across all
43 failures:

```
summary: actor==origOwner=0  actor==prevRecipient=0  other=43  total=43
```

In other words: in the timeline itself, none of the failing steps has the
gear's owner listed as actor, and no earlier giveaway of the same gear
exists. So the timeline generator is fine.

The bug is in the executor. In `server/simulation/executor.go` the shared
client is mutated in place to switch users:

```go
// executor.go:34
getClient := func(email string) *Client {
    token := state.UserTokens[email]
    if token == "" {
        return nil
    }
    client.AsUser(token)   // <-- mutates the shared client
    return client
}
```

And in `executeExpressInterest` (executor.go:366–404) the giveaway path
calls `getClient(ownerEmail)` to re-share the gear as `FOR_GIVEAWAY` on
behalf of the owner, then falls through to `client.Transfer().ExpressInterest(...)`
without restoring the actor's token:

```go
// executor.go:377
if isGiveaway {
    ownerEmail := state.GearOwners[gearRef]
    ownerClient := getClient(ownerEmail)        // token -> owner
    ownerClient.Community().ShareGear(...)      // runs as owner ✓
    // no restore — shared client still auth'd as owner
}
// executor.go:399
resp, err := client.Transfer().ExpressInterest(...)  // runs as OWNER, not step.Actor
```

The server then — correctly — rejects because `gear.OwnerId == userID`
(`server/services/transfer/interest.go:111`). The `actor=alex` in the
warning is just the in-memory step field; the authenticated user on the
wire is the owner. A server-side grep confirms `gear.OwnerId` is never
mutated anywhere after creation, which rules out the "gear changed owners
in the server" hypothesis from the original plan draft.

## Context from Comments

The issue has no comments. The reporter's suspected location
(`activity.go` — the generator) turns out to be wrong; the suspected fix
(filter `actor != gear.OwnerID` in the generator) is already in place and
working. So a reader of just the issue would likely produce the wrong PR.

## Potential Solutions

### Option A: Restore the actor's token after the owner-side re-share
- **Approach:** In `executeExpressInterest`, after the `FOR_GIVEAWAY`
  re-share completes, call `client.AsUser(state.UserTokens[step.Actor])`
  (or equivalently `getClient(step.Actor)`) before the `ExpressInterest`
  call. Smallest possible change; preserves the existing "shared client,
  mutate-in-place" pattern.
- **Pros:** One-line fix at the bug site; minimal blast radius; easy to
  test with the existing simulation.
- **Cons:** Leaves the land-mine in place — every other executor function
  that calls `getClient(...)` for a side operation must remember to
  restore the actor's token. One already-correct caller may paper over
  another future mistake.
- **Effort:** Small.

### Option B: Eliminate token mutation; hand each caller its own `Client`
- **Approach:** Replace `getClient` with a real per-user client pool
  that returns a distinct `*Client` per email (each with its own
  `simulationTransport` and token). Callers no longer share state; the
  "which user am I?" question is answered by which `Client` you call.
  Update `executeExpressInterest` (and any other caller that currently
  relies on the main-loop auth) accordingly.
- **Pros:** Removes the class of bug — no more hidden token drift. Also
  makes the concurrent executor (`concurrent.go`) safer by construction;
  today it already holds a wlock around write actions partially because
  of this shared mutable state.
- **Cons:** Touches more code; the main loop's `client.AsUser(...)` at
  `executor.go:73` pattern has to be rethought; some allocation overhead
  (17 users × 14 cached service clients ≈ fine).
- **Effort:** Medium.

### Option C: Restore at a single choke-point in the main loop
- **Approach:** Add `defer client.AsUser(state.UserTokens[step.Actor])`
  after line 73, or re-assert the actor's token after every executor
  call returns. Effectively turns "per-step auth" into a loop-level
  invariant even if individual executors mutate mid-step.
- **Pros:** Small change at a central location; protects against future
  `getClient` misuse inside any executor.
- **Cons:** Still implicit; a future executor that calls ExpressInterest
  *after* a chat injection that uses `getClient` could still be broken
  inside a single step. Feels fragile.
- **Effort:** Small.

## Recommendation

**Ship Option A now, plan Option B next.** Option A is a one-line patch
that clears the 43 warnings per simulation run and restores the
integrity of the failure counter — it's the right move today. Option B
is the right architectural fix; the `getClient` pattern is a latent
bug-farm (anyone reading `getClient(...)` naturally assumes it returns
an independent client, because the name suggests that). Do Option B as a
follow-up once the simulator is back to a clean baseline, so we're not
conflating a bug fix with a refactor.

## Open Questions

- **Do other executors have the same bug?** — Recommendation: grep for
  all `getClient(` call sites and audit each. Chat injection (used for
  RSVP, loan interest, request offers) is the highest-risk area — those
  callers intentionally impersonate multiple users per step. If any of
  them is followed by a write under the original actor's identity, the
  same bug exists there. Fix as part of Option A.
- **Should we assert in the server that the authenticated user matches
  the declared actor in the simulator?** — Recommendation: no; the
  server is working as designed and the rejection is exactly the
  behavior we want in production. The simulator is the thing that lied.
- **Should we add a regression test?** — Recommendation: yes — an
  executor-level test that stubs the transport, runs a single giveaway
  flow, and asserts the token at `ExpressInterest` time equals the
  interested actor's token. Cheap, catches this specific regression.

## Implementation Plan

### Phase 1: Immediate fix (Option A)
**Goal:** Eliminate the 43 `cannot express interest in your own gear`
failures per `suburban-neighborhood` run.
**Tasks:**
- [ ] In `executeExpressInterest` (`server/simulation/executor.go`), after
      the `FOR_GIVEAWAY` re-share block (line ~396), restore the actor's
      auth before the `ExpressInterest` call with
      `client.AsUser(state.UserTokens[step.Actor])`.
- [ ] Audit every other `getClient(` call site in `executor.go` and
      `chat.go` for the same leak pattern; patch or document each.
- [ ] Delete `server/simulation/diag_giveaway/` (investigation scratch).
- [ ] Rerun `suburban-neighborhood` at seed 42 and confirm zero
      `cannot express interest in your own gear` failures.
**Validates:** The bug is in the executor token handling, not the
timeline, and the fix resolves it end-to-end.

### Phase 2: Regression test
**Goal:** Prevent recurrence.
**Tasks:**
- [ ] Add an executor-level unit test in
      `server/simulation/executor_test.go` (or a new file) that stubs the
      HTTP transport, runs a single giveaway flow, and asserts the
      Authorization header on the `ExpressInterest` request matches the
      interested actor's token, not the owner's.
- [ ] Optional integration check: a top-level assertion in the
      simulation runner that fails the run if any
      `ExpressInterestGiveaway` step errors with "your own gear" for the
      canonical scenarios — this error has no non-bug explanation.
**Validates:** The fix survives future refactors.

### Phase 3: Remove the token-mutation foot-gun (Option B, follow-up)
**Goal:** Make the class of bug impossible.
**Tasks:**
- [ ] Replace `getClient` with a per-user client pool keyed by email;
      each entry holds its own `simulationTransport` with a fixed token.
- [ ] Remove the main loop's `client.AsUser(...)` at executor.go:73;
      instead call `getClient(step.Actor).<svc>()...`.
- [ ] Update `concurrent.go` to use the new pool; the wlock can be
      audited for scope once shared-token mutation is gone.
- [ ] Rerun all canonical scenarios to confirm no regression.
**Validates:** The simulator's auth layer is no longer implicit shared
state.

## Related Issues
- #1062 — Simulator schedules RSVPs beyond experience max_participants.
  Co-located bug in the same simulation run; also surfaced by the same
  seed-42 reproduction. Worth landing in the same branch.

## Audit notes (ripls-audit, 2026-04-21)

Checked against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`. Findings:

- **Client docs: N/A.** The change is purely in `server/simulation/`
  (a Go test harness). No Flutter, Riverpod, Stash cache, or ARB code
  is touched, so MVVM/caching/i18n conventions don't apply.
- **Server architecture — testing strategy:** plan aligns.
  Phase 2 adds a unit-level executor test that stubs the HTTP
  transport and asserts token identity — exactly the "unit tests are
  always preferable to integration tests" and "mock external
  dependencies" guidance from `docs/server/architecture.md §4`.
- **Server architecture — file size:** `executor.go` is already
  ~700 lines. The fix adds one line; Phase 3's refactor (per-user
  client pool) would likely land in a new file (`client_pool.go`)
  rather than bloating `client.go`/`executor.go`. No change needed
  to the plan — flagging here for the implementer.
- **CLAUDE.md — don't over-refactor during bugfix:** plan already
  separates the one-line fix (Phase 1) from the structural
  refactor (Phase 3). Good. Do not bundle them.
- **CLAUDE.md — fail-fast on errors:** plan's Phase 2 assertion that
  the runner fails any simulation reporting "cannot express interest
  in your own gear" matches the repo's "Fail Fast" principle: this
  error has no legitimate cause and silently tolerating it would
  mask future regressions.
- **No architectural violations found.** Plan is ready to implement.
