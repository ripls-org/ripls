# Issue #1623: Background-job write guard: skip writes to deleted communities

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1623
**Filed:** 2026-04-30 | **Last activity:** 2026-04-30
**Labels:** none | **Assignees:** briangbrown
**Context docs:** [`docs/community_delete_and_leave.md`](../community_delete_and_leave.md), [`docs/issues/1621-deleted-community-read-filter.md`](./1621-deleted-community-read-filter.md), [`docs/issues/1622-notification-dispatcher-deleted-guard.md`](./1622-notification-dispatcher-deleted-guard.md)

> **Helper renamed** (during implementation, 2026-04-30) — the plan
> below originally proposed adding `MayWriteToCommunity` as a sibling
> to `MaySendNotification` from #1622. While writing Phase 1 we
> noticed the two helpers would be byte-for-byte identical except
> for log strings, so we consolidated. The single canonical helper
> is now `community.IsActive(ctx, s, communityID) bool`, used by
> both the notification dispatchers (#1622) and the async-write
> guard (this issue). Throughout the rest of this plan, mentally
> substitute `MayWriteToCommunity` → `IsActive`. Phase 1's
> tasks/tests/CI-rule references update accordingly. The behaviour
> is unchanged; only the helper name is generalized.
>
> **Audit applied** (`/ripls-audit`, 2026-04-30) — three refinements vs.
> `docs/server/architecture.md` and `docs/server/observability.md`:
> 1. **Context propagation differs from #1622.** The async-write
>    paths in this issue use
>    `detachedContextWithTimeout(parent, ...)` which is built on
>    `context.WithoutCancel(parent)` — that **preserves** the
>    parent context's values (`request_id`, `user_id`,
>    `target_request_id`). Verified at
>    `server/services/request/lifecycle.go:27-32`,
>    `server/services/feed/feed_nudges.go:49,72,152,186`. So unlike
>    #1622's dispatcher (which uses fresh `context.Background()`
>    and loses correlation), `MayWriteToCommunity`'s logger inside
>    these paths inherits the originating RPC's correlation fields.
>    The exception is `server/jobs/experience_stories.go::Run` —
>    scheduler-scoped, not request-scoped, so its logger carries
>    `job=experience_stories` instead.
> 2. **Goroutine-safety enforcement is already in place.** This
>    issue adds logic *inside* existing goroutines (none new), all
>    of which already use `logging.GoSafe` (the architecture-doc-
>    mandated wrapper) or have inline panic recovery (request
>    lifecycle's `go func` blocks). No new audit concern; flagged
>    only because future contributors may copy this pattern.
> 3. **Helper file location is consistent with #1622's audit.**
>    `MayWriteToCommunity` lands alongside `MaySendNotification`,
>    `shouldNotify`, `CategoryFor`, `CategoryEnabled` in
>    `server/community/notifications.go`. The file is becoming a
>    community-deletion gates registry — a future rename to
>    `community_gates.go` is plausible but not in scope here.

## Problem Statement

Async background jobs and goroutines keyed on a community ID may
complete *after* the community is soft-deleted. If they blindly
write their result, the user restores the community 20 days later
and finds AI-generated content (gear images, suggestion chips,
stories, nudges) from the deletion window — content that wasn't
visible to anyone during the soft-delete period and shouldn't
suddenly appear on restore.

This issue adds a single chokepoint helper and wires it into every
async write path that targets community-scoped data. Ships as a
no-op until #1619 Phase 2 produces deleted state. Tests force-flip
`Community.Deleted` mid-flight and prove the write is dropped.

This issue is **stacked on #1632 (issue #1622)**, which is itself
stacked on #1630 (issue #1621). That gives a clean linear stack
\#1630 → #1632 → this PR so the full Phase 1 trifecta lands in
coordinated review. The CI gate from #1621 (Rules A/B) and #1622
(Rule C) gets a Rule D extension here.

## Context from Comments

No comments — issue was filed as part of the #1619 decomposition
along with #1621 and #1622. The design doc and the merged work in
the sibling PRs provide the surrounding context.

What's worth knowing from the codebase audit:

- The issue's "Affected jobs" table is **partially wrong**. The named
  files (`gen_ai.go`, `gen_streaming.go`, `gen_fanout.go`, etc.) are
  AI generation paths that return suggestions to the client and do
  NOT write to the database — the client previews the result and
  calls `SaveExperience`/`SubmitRequest`/etc. to persist. So the gen
  paths themselves are not the risk surface.
- The actual async-write paths that touch community-scoped data are
  in **post-save side-effect goroutines** and **scheduled jobs**:
  - `server/services/request/lifecycle.go:155` — async stock image
    fetch after `SubmitRequest` (writes `Request.MediaIds`).
  - `server/services/request/lifecycle.go:186` — async AI suggestion
    chips (writes `Request.AdditionalAsks` etc.). Already has a
    `Request.Deleted` check at line 217-220 — that pattern is the
    template, but it doesn't check `Community.Deleted`.
  - `server/services/request/stock_imagery.go::fetchAndAttachStockImage`
    — the worker function called by the line-155 goroutine.
  - The gear-side equivalents (stock image fetch on `SaveGear`).
  - `server/services/feed/feed_nudges.go::generateAndPersistNudges`
    — writes per `(user, community)` nudge rows.
  - `server/jobs/experience_stories.go::createStoryForExperience`
    — daily job that creates a `Story` per community an experience
    was shared with.
- `server/services/experience/save.go:129/154` async LLM goroutines
  use a `sync.WaitGroup` and complete **before** `SaveExperience`
  returns — not truly async-after-RPC. Out of scope.

## Potential Solutions

### Option A: Inline `Community.Deleted` check at every write site

- **Approach:** Each affected goroutine and job adds `GetByID(community,
  IncludeDeleted: true)` before the write, checks `Deleted`, returns
  early.
- **Pros:** Minimal new abstraction. Each check is local.
- **Cons:** Six+ copies of the same logic. Drift risk: a future
  goroutine author writes the check slightly differently, or skips
  it. Same failure mode #1621 and #1622 already learned from.
- **Effort:** Small.

### Option B: Shared `community.MayWriteToCommunity` helper, mirrors
`MaySendNotification`

- **Approach:** Add `community.MayWriteToCommunity(ctx, s,
  communityID) bool` — a sibling to `MaySendNotification` from
  #1622, semantically identical (returns `false` for soft-deleted,
  fail-open on lookup error). Different name so call-site reads as
  self-documenting at write paths. Wire it into every async write
  path. Extend the existing CI gate from #1621/#1622 with Rule D.
- **Pros:** Single chokepoint, consistent with #1622's pattern.
  Future async writers grep for "MayWriteToCommunity" and find the
  helper. The CI rule prevents forgetting.
- **Cons:** Two near-identical helpers (`MaySend*` and `MayWrite*`).
  Could be unified later as
  `community.IsActiveCommunity(...) bool`, but that touches the
  #1622 helper which is in review — defer.
- **Effort:** Medium. Helper itself is ~20 lines; the wiring covers
  6-ish goroutines/jobs; the CI rule extension is small.

### Option C: Push the check into the storage write helpers
(`Update` / `Insert`)

- **Approach:** Make `ProtoSQLStorage.Update`/`Insert` look up the
  community of the entity being written and refuse if soft-deleted.
- **Pros:** Strongest enforcement. Compile-time impossible to write
  to a deleted community.
- **Cons:** Storage layer is intentionally policy-free per the
  architecture doc. Storage methods would need to know which fields
  on which entity types map to a community ID — a per-table schema
  the generic protosql layer deliberately doesn't carry. Coupling
  storage to community semantics is the wrong place. Same conclusion
  as #1622 reached for Option C in its plan.
- **Effort:** Large + breaks layering.

## Recommendation

**Option B**, mirroring the #1622 design exactly. Rationale:

1. The codebase already pattern-matches: `MaySendNotification` (pushes)
   and `MayWriteToCommunity` (writes) are obvious siblings. New
   contributors find one when they need the other.
2. Fail-open semantics are correct here too: if the community lookup
   itself errors (transient DB hiccup), we should write — losing
   async work to a transient failure is worse than the rare case of
   writing to a community that's about to be deleted.
3. CI gate Rule D is tractable: every async write path that touches
   a community-scoped entity goes through a small, identifiable set
   of files. Grep for `go func\|GoSafe\|server/jobs/` + a write
   call + `community_id` reference — flag if no `MayWriteToCommunity`.
4. Stacking this PR on #1632 (which already includes Rules A–C from
   #1621 and #1622) means the gate is already there to extend with
   Rule D, no coordination overhead.

## Open Questions

- **Should `MaySendNotification` and `MayWriteToCommunity` be unified
  as a single `IsActiveCommunity` helper?**
  **Recommendation:** Keep them separate for now. Same implementation
  under the hood but distinct call-site naming. A future cleanup PR
  (small) can unify after both #1622 and #1623 are landed; doing it
  here would force a #1622 churn that complicates review.
- **For multi-step writes (job creates Story + writes nudge in same
  transaction), check once or per write?**
  **Recommendation:** Check once at the top of the goroutine /
  transaction. The race window between check and write is small
  enough that we don't need per-write rechecks. Same convention as
  #1622's notifyTargetedUsers (single check, not per-recipient).
- **What about writes that are scoped to an entity (e.g. Request,
  Experience, Gear) which is itself scoped to a community?** The
  existing `Request.Deleted` re-check at
  `request/lifecycle.go:217-220` is one shape; should the new
  Community-deleted check be a peer check or replace it?
  **Recommendation:** Peer check. Both can be true: a Request can be
  soft-deleted while its community is active (user cancelled the
  request), or a Community can be soft-deleted while its Requests
  remain (deletion cascade hasn't run yet, or per the cascade rules
  in #1619 §4 the Request soft-delete fires *as part of* the
  community delete cascade — but there's a small window). Keep both
  checks; they protect different failure modes.
- **The `experience_stories.go` daily job iterates all completed
  experiences — does it need batched community checks?**
  **Recommendation:** Yes. The job already loops per `CommunityExperience`
  row at line 65. Adding a `MayWriteToCommunity` call inside that
  loop is fine — it's one extra `GetByID` per iteration, dwarfed by
  the existing per-iteration cost. Could batch with a
  `GetByIDs(activeCommunityIDs)` if performance shows up as an issue;
  defer until it does.
- **Should the helper return `(bool, *models.Community)` so callers
  that need the community object don't refetch?**
  **Recommendation:** No. Most write sites only need the bool;
  forcing them to handle the Community pointer adds noise. The
  rare site that wants the object can call
  `auth.RequireActiveCommunity` from #1621 instead.

## Implementation Plan

### Phase 1: Helper

**Goal:** Land `community.MayWriteToCommunity` alongside
`MaySendNotification` with full unit tests. No callers wired yet —
system unchanged in production.

**Tasks:**
- [ ] Add `community.MayWriteToCommunity(ctx context.Context, s
      *storage.ProtoSQLStorage, communityID string) bool` in
      `server/community/notifications.go` (the same file where
      `MaySendNotification` lives — peer helpers stay together).
      Implementation mirrors `MaySendNotification`:
      `GetByID(IncludeDeleted: true)`, check `Deleted != nil &&
      DeletedAtUnixSec > 0`, return `false` when deleted; on lookup
      error return `true` (fail open) and log a warning. Logger
      fields: `operation=MayWriteToCommunity`, `community_id`.
      **Note:** unlike `MaySendNotification`'s callers, the async
      paths that call this helper preserve `request_id` / `user_id`
      via `context.WithoutCancel`, so the helper's logger inherits
      that correlation. Scheduled jobs (`server/jobs/`) carry
      `job=...` instead — both are useful in different
      ways. No special-casing in the helper.
- [ ] Unit tests in `server/community/notifications_test.go`
      mirroring `TestMaySendNotification`'s four cases (active
      community, soft-deleted, missing/fail-open, zero
      DeletedAtUnixSec).

**Validates:** Helper is correct in isolation and indistinguishable
in shape from `MaySendNotification`.

### Phase 2: Wire into async write paths

**Goal:** Every async path that writes community-scoped data calls
`MayWriteToCommunity` before the write. Tests prove the guard fires
under a force-flipped delete.

**Tasks:**
- [ ] `server/services/request/lifecycle.go:155` async stock image
      fetch — add a check at the top of the goroutine. The community
      ID is on `Request.CommunityId`... wait, requests aren't directly
      community-scoped; they're linked via `CommunityRequest`. The
      goroutine needs to fetch the request first to learn its
      community(ies). Defer to within `fetchAndAttachStockImage`.
- [ ] `server/services/request/stock_imagery.go::fetchAndAttachStockImage`
      — at the top, after the existing `s.storage.GetByID(ctx, requestID,
      request)` call (line 61), iterate the request's `CommunityRequest`
      rows. If ALL community parents are deleted, skip the write. If
      any parent is active, proceed. Document the per-request
      semantics inline.
- [ ] `server/services/request/lifecycle.go:186` async suggestion
      chips — extend the existing `Request.Deleted` check at line
      217 with a parallel community check. Same logic: any active
      community parent → proceed; all deleted → skip.
- [ ] Gear-side stock image fetch — locate the analogous path
      (`server/services/gear/stock_imagery.go` or similar) and apply
      the same fix.
- [ ] `server/services/feed/feed_nudges.go::generateAndPersistNudges`
      — at the top of the goroutine, call `MayWriteToCommunity` on
      `event.CommunityId`. Skip if false.
- [ ] `server/jobs/experience_stories.go::createStoryForExperience`
      — at the top, call `MayWriteToCommunity(ctx, j.storage,
      communityID)`. Skip the story creation if false. The job
      already handles "skipped" as a non-error case (see
      `skippedCount` counter at line 48).
- [ ] Per-path test in each of the modified files: seed entity +
      community + members, force-flip community to deleted, run the
      goroutine/job, assert no rows were written for that community.
      Use `notifications.NewMockService()` style — fixtures for the
      drain channel where applicable
      (`s.stockImageryDone`, `notificationDone`, etc.).

**Validates:** No async write lands against a soft-deleted community
across all 6 entry points, even when the delete fires mid-flight.

### Phase 3: CI grep gate Rule D

**Goal:** A new background goroutine or job that writes
community-scoped data can't ship without the gate.

**Tasks:**
- [ ] Extend `scripts/check_active_community_gate.js` with Rule D:
      any non-test, non-helper file under `server/services/` or
      `server/jobs/` that contains BOTH (a) a goroutine spawn
      (`logging.GoSafe(` or `go func`) AND (b) a community-id-keyed
      storage write (heuristic: `community_id` literal AND
      `s.storage.Update`/`s.storage.Insert`) must reference
      `community.MayWriteToCommunity`. The helper file
      (`server/community/notifications.go`) is exempt.
- [ ] The grep-based heuristic will produce false positives — that's
      acceptable for a "did you remember the check?" lint. False
      positives are silenced by adding the helper call (cheap) or
      adding the file to an explicit allowlist with a one-sentence
      rationale (similar to `scripts/ref_listen_allowlist.txt`).
- [ ] Confirm zero violations after Phases 1 and 2 land.

**Validates:** A future async-writer can't merge without the helper
call, even if they don't read this plan.

## Related Issues

- #1619 — parent issue (community soft-delete with 30-day restore
  window). This is the third Phase 1 prerequisite.
- #1621 — sibling (read-path filter). PR #1630, in review. This
  branch stacks on it for the CI gate script.
- #1622 — sibling (notification dispatcher gate). PR #1632, in
  review. Established the `MaySendNotification` pattern that this
  issue mirrors with `MayWriteToCommunity`.
- #1340 — closed; established `connecterr.Internal` pattern.
