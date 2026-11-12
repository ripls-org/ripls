# Issue #1680: Duplicate inbox items for gear shared to multiple communities

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1680
**Filed:** 2026-05-03 | **Last activity:** 2026-05-03
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** [docs/client/inbox.md](../client/inbox.md) (added during audit — canonical inbox spec; documents existing dedup conventions)

> **Audit applied 2026-05-03.** Aligned the plan against
> `docs/client/inbox.md`, `docs/server/observability.md`, and
> `docs/client/testing.md`. Findings inline below — search for
> `**Audit:**` markers.

## Problem Statement

When a piece of gear is shared into more than one community, the user's
Portfolio Inbox renders one card per community sharing instead of a single
card representing the gear. The screenshot in the issue shows two visually
identical inbox rows for the same gear. The duplicates are noisy, take up
space at the top of the inbox, and obscure other activity.

The root cause is in `server/services/portfolio/inbox_feed.go:325-364`
(`assembleFeedItems`). The handler iterates over every `CommunityGear`
record for the user — and `CommunityGear` is a join row keyed by
`(gear_id, community_id)` — so a gear shared to N communities yields N
inbox rows. The transfer-bound rows above already carry a
`gearIDsWithTransfer` skip-set, but there is no equivalent dedup for the
"shared gear listing" branch.

The bug only fires for the *shared listing* card type — the case where
the owner shared gear and at least one community member has messaged on
the gear's primary conversation but no transfer has started yet. Once a
transfer exists, the `gearIDsWithTransfer` guard suppresses the
duplicates.

**Audit:** `docs/client/inbox.md` already documents the established
inbox dedup conventions:

- **Same-gear / multi-transfer owner case** is collapsed to one row by
  `ownerGearGroup` aggregation in `assembleFeedItems`
  (`server/services/portfolio/inbox_feed.go:97-193`) — "the most advanced
  transfer (by state, then by recency) is selected as the primary."
- **Transfer ↔ shared-listing dedup** is handled by `gearIDsWithTransfer`
  — "if an active transfer already exists for the same gear ... the
  shared gear listing is suppressed."

What the doc and the code both *miss* is **same-gear / multi-community
shared-listing dedup**. The fix below extends the existing dedup
conventions to that gap and should be reflected back into
`docs/client/inbox.md` once it lands.

**Audit (watch-table side effect):** Watch state is keyed at the **GEAR**
level (`watched_item` row per `(user_id, item_type=GEAR, item_id=gear_id)`),
not per `(gear, community)`. So today's duplicate inbox rows all point at
the same single watch entry — `applyWatchState` is being asked to attach
that one entry to N `DailyItem`s, and the auto-watch `UpsertWatch` is
firing N times for one row's worth of state. After dedup, both
`applyWatchState` and the auto-watch upsert run 1× per gear — which is
the correct behavior. **No watch-table cleanup is needed**; the dedup is
sufficient.

## Context from Comments

The issue has no comments — it's a single user-feedback report from
user2@example.org with a screenshot. There's no maintainer discussion or
clarification thread to draw from. Treat the screenshot + description as
the entire spec.

The work is being done on branch `duplicate_inbox_items` (currently empty
relative to `main`).

## Potential Solutions

### Option A: Server-side dedup by gear_id in `assembleFeedItems`
- **Approach:** In `server/services/portfolio/inbox_feed.go`, mirror the
  existing `ownerGearGroup` aggregation pattern at lines 97-193
  (transfer multiplicity). Build a `sharedGearGroup` map keyed by
  `gear_id`, with each entry holding the list of `CommunityGear`
  records and a `primary` selected deterministically. The chosen
  primary drives the single emitted `DailyItem`. The `assembleSharedGear`
  callee already receives `d.gearCommunityIDs[gear.Id]` (the full
  shared-into list) and uses it for the social-proof ring fields, so it
  can correctly render "Shared into N circles" without each community
  generating its own row.
- **Pros:** Smallest possible blast radius. One file, one loop. Adopts
  the codebase's existing same-gear dedup idiom verbatim. The downstream
  item already knows about cross-community sharing — the call site is
  the only thing that doesn't.
- **Cons:** The `commName` shown on a card becomes ambiguous when gear
  is shared to multiple communities — we have to pick one to display
  (or stop displaying it for multi-community shares). Need to decide on
  the rule.
- **Effort:** Small.

**Audit (canonical-row rule):** Mirror the `ownerGearGroup.primary`
selection style — pick by deterministic priority. Suggested order:
(1) `CommunityGear.CommunityId` matches `gear.HomeCommunityId` if such
a field exists, else fall through; (2) earliest `CreatedAtUnixSec`
(matches "first place this was shared"); (3) lexicographic
`CommunityId` as a final stable tiebreaker. The conversation surfaced
on the row is `gear.ConversationId` (gear-level, set in code at
`inbox_feed.go:345-348`) regardless of which `CommunityGear` we pick,
so the choice does not affect `convID`, `messages`, or `unread`. It
only affects the `community_name` / `community_id` displayed on the
card.

**Audit (doc/code discrepancy):** `docs/client/inbox.md` says shared gear
uses "Per-community `CommunityGear.conversation_id`", but the code at
`inbox_feed.go:345-348` prefers `gear.ConversationId` (gear-level)
first, falling back to `cg.ConversationId` only if the gear-level field
is empty. This is a pre-existing inconsistency, not introduced by this
fix — flag it in the PR description and update the inbox doc to match
reality (or open a follow-up if the code is the surface that should
change).

### Option B: Aggregate in the data fetch (`fetch.go`)
- **Approach:** Change `fetchAll` so `commGearRaw` is pre-deduplicated by
  `gear_id`. Either query `community_gear` with a `DISTINCT ON (gear_id)`
  ordering, or do the dedup in Go after the batch query.
- **Pros:** Keeps the assembly loop unchanged. Centralizes the rule
  closer to the data load.
- **Cons:** `commGearRaw` may legitimately need every `(gear_id,
  community_id)` row for other purposes in the same fetch path
  (e.g. populating `gearCommunityIDs`). Trimming it at the source risks
  silently breaking those derived maps. The loop-level fix in Option A
  is more local and safer.
- **Effort:** Small, but with higher cross-impact risk.

### Option C: Client-side dedup in the Flutter inbox viewmodel
- **Approach:** In `app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart`,
  collapse `DailyItem`s by `(item_type, item_id)` before display.
- **Pros:** Defensive — also protects against future server regressions.
- **Cons:** Wrong layer. The server already computes social-proof
  aggregates (`SharedCommunities`, `TotalCommunities`, `TotalPeople`) per
  item. If the client picks one of N near-duplicate items arbitrarily,
  the surfaced community context, conversation ID, and unread count may
  be inconsistent across users. Also pushes work onto every client that
  consumes the `GetPortfolioInboxFeed` RPC.
- **Effort:** Small but ill-fitting.

## Recommendation

**Option A.** The bug is a one-call-site oversight in
`assembleFeedItems` — the transfer branch already has gear-level dedup,
the shared-listing branch was missed. Build a `sharedGearGroup` map
keyed by `gear_id` (mirroring the existing `ownerGearGroup` shape at
lines 97-193), pick a canonical `CommunityGear` deterministically, and
emit one `DailyItem` per gear. Keep the fix tight — single file, single
loop, mirrors the existing pattern.

`assembleSharedGear` already receives the full shared-into community list
via `d.gearCommunityIDs[gear.Id]` and uses it for the ring/social-proof
counts, so the surviving row will correctly indicate "shared into N
communities" without code changes downstream.

## Open Questions

- **Which `commName` should the surviving row display when gear is
  shared to multiple communities?**
  Recommendation: show the `communityName` of the canonical
  `CommunityGear` chosen by the dedup rule (home community → earliest
  share). This is consistent with how the `CommunityId` field will be
  set on the resulting `DailyItem` and matches the conversation the
  card links into. The `SharedCommunities` ring already conveys the
  "shared into N" social proof, so the single-name display is not
  misleading.

- **Should the dedup cover the transfer branch's analog — gear with an
  active transfer that's also shared into other communities?**
  Recommendation: no change needed. The transfer branch already
  produces one row per gear (it's keyed by transfer, not by
  `CommunityGear`), so it doesn't have the same multiplicity. Confirm
  this with a quick read of lines 97-126 before declaring it out of
  scope.

- **Are there integration tests that today inadvertently lock in the
  duplicate behavior?**
  Recommendation: grep
  `server/services/portfolio/*_test.go` for fixtures that share gear
  into multiple communities and assert on `len(items)`. Update those
  expectations as part of the fix rather than as a separate cleanup.

- **Does any client UI rely on receiving N copies of the same gear
  (e.g. for per-community filtering)?**
  Recommendation: very unlikely — the inbox is a flat cross-community
  feed. Verify by grepping the Flutter side for any logic that buckets
  inbox items by `community_id`. If something does, surface it before
  changing server behavior.

## Implementation Plan

### Phase 1: Server-side dedup with regression test
**Goal:** Eliminate duplicate shared-listing inbox rows for gear shared
to multiple communities, with a server-side test that locks in the
single-row behavior.

**Tasks:**
- [ ] Read the owner-transfer branch (`inbox_feed.go:97-193`) and confirm
      it already groups by `gear_id` via `ownerGearGroup` — the new code
      should be a parallel `sharedGearGroup`, not a separate idiom.
- [ ] Build `sharedGearGroup` keyed by `gear_id` with `records []*CommunityGear`
      and `primary *CommunityGear`. Selection rule for `primary`:
      (1) match home community if such a relationship is derivable from
      the gear, (2) earliest `CreatedAtUnixSec`, (3) lexicographic
      `CommunityId` as a stable tiebreaker. Skip records that fail the
      existing per-record gates (archived, missing gear, non-owner,
      deleted, no user message in conversation, `gearIDsWithTransfer`
      hit).
- [ ] Replace the per-`CommunityGear` loop at `inbox_feed.go:325-364`
      with a two-pass version: first pass populates `sharedGearGroup`,
      second pass emits one `DailyItem` per group from the primary
      record.
- [ ] **Audit (observability):** Emit one `Debug` log per dedup event
      via `logging.LoggerWithContext(ctx)` with fields `operation =
      "GetPortfolioInboxFeed"`, `gear_id`, `dropped_count` (the number
      of `CommunityGear` records collapsed into the primary). Use the
      `gear_id` field name from the standard observability conventions
      (`docs/server/observability.md`). No `INFO`-level log — dedup is
      not exceptional.
- [ ] Add a focused unit test in
      `server/services/portfolio/inbox_feed_test.go` (or the existing
      assembly test file, whichever is closest in style): set up a user
      who owns one gear shared into two communities with a member
      message on the conversation; assert exactly one `DailyItem` is
      returned for that gear, with `SharedCommunities`/`TotalCommunities`
      reflecting both communities.
- [ ] Search existing `portfolio` tests for fixtures that match the bug
      shape and update assertions as needed.
- [ ] Run `npm run build` and `go test ./server/services/portfolio/...`
      to confirm lint + unit tests pass.
- [ ] Update `docs/client/inbox.md` to add a "Multi-community shared
      listings" sub-bullet under "Deduplication" that mirrors the new
      `sharedGearGroup` rule, parallel to the existing
      transfer-grouping description at lines 259-261. Resolve (or open
      a follow-up for) the per-community vs. gear-level
      `conversation_id` doc/code discrepancy noted above.

**Validates:** A single-screen reproduction yields one inbox row instead
of N, and a regression test prevents future regressions.

### Phase 2: Client smoke test
**Goal:** Confirm the fix in a real device session and rule out any
client-side fallout.

**Audit (client scope):** This change is server-only. The
`portfolio:inbox_feed` cache key in the `'portfolio'` namespace
(see `docs/client/inbox.md` "Caching" section and
`docs/client/caching.md`) holds `GetPortfolioInboxFeedResponse`; the
response *shape* is unchanged, only the row count drops, so no cache
invalidation, no schema migration, and no `refresh*()` plumbing changes
are needed beyond the existing pull-to-refresh path. Per
`docs/client/testing.md` (ViewModel-first), no new ViewModel test is
warranted because no ViewModel logic changes — the smoke test below is
the right level of coverage for the client.

**Tasks:**
- [ ] Set up a dev account that owns gear shared into 2+ communities,
      with at least one member message on the gear's conversation.
- [ ] Pull the inbox in the Flutter app on dev; confirm a single
      inbox row, correct `community_name` display, and correct
      "shared into N" ring count.
- [ ] Confirm `is_watched_unread` and the nav-bar / app-icon unread
      badge are correct (1 unread instead of N) — this is the most
      visible user-facing improvement beyond the row collapse itself.
- [ ] Pull `git log -- app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart`
      to verify no recent client-side dedup that might also need
      adjustment.

**Validates:** End-to-end behavior matches the fix on a real device.

## Related Issues
- #1646 — Standardize the inbox item. Recent inbox standardization
  work; the assembly helpers updated there are the same ones the fix
  touches. Useful context for matching the existing code style.
- #1660 — Add community chats in multi-community view. Recent
  multi-community surface work. The inbox-side multi-community case was
  not extended at the same time, which is how the gap that produced
  this bug was introduced.
