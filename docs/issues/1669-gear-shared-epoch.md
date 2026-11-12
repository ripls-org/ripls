# Issue #1669: Loan shows shared 20K+ days ago

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1669
**Filed:** 2026-05-02 | **Last activity:** 2026-05-02
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The gear detail screen on the gear type pane renders a relative
"Marc shared 20575d ago" label. 20,575 days from 2026-05-02 lands at
1970-01-01 — the Unix epoch — confirming the reporter's hypothesis that
an uninitialized timestamp is being rendered as a relative age.

Code inspection narrows this to `app/lib/presentation/screens/gear/widgets/gear_type_pane.dart:75`, which passes `gear.sharedAtUnixSec` straight
into `_buildOwnerLine`, where it becomes:

```dart
final createdAt = DateTime.fromMillisecondsSinceEpoch(createdAtUnixSec * 1000);
final age = DateTime.now().difference(createdAt);
final agoStr = age.inDays > 0 ? '${age.inDays}d ago' : ...;
```

When `createdAtUnixSec` is `0`, the result is "20575d ago" instead of a
meaningful "shared X ago" string.

The root cause is a **contract mismatch** between the proto field's
documented semantics and the client's usage:

- `api.GetGearResponse.shared_at_unix_sec` is documented as *"Unix
  timestamp (seconds) when the gear was shared to the community.
  Only populated when community_id is provided in the request.
  Returns 0 if not shared to the specified community."*
  (`proto/ripls/api/gear_service.proto:138-141`)
- The server populates this field only when `req.Msg.CommunityId != ""`
  AND a matching `CommunityGear` row exists
  (`server/services/gear/service.go:410-418, 482-483`).
- The client treats it as a gear-level "first shared" timestamp on the
  type pane — a label that should be valid even when the gear screen is
  opened without a community context (e.g. from a deep link, push
  notification, or feed item that lacks a `communityId`).

`gear_content_view.dart:143-144` chooses `effectiveCommunityId =
widget.initialCommunityId ?? selectedCommunity?.id` — when both are
null, the server returns 0, and the client renders 1970-01-01.

## Context from Comments

The issue has **no comments** beyond the auto-generated feedback report.
The reporter is the project maintainer (`user2@example.org`), so
clarifications are easy to source but the issue itself is brief.

The screenshot ("Yellow Pedal-Assist E-Bike", a Loan-tagged gear card)
confirms the gear detail screen as the affected surface. The "HO 10"
badge and "Borrow" CTA indicate the gear is shared to at least one
community, so the underlying data is fine — only the **display path** is
broken.

## Potential Solutions

### Option A: Add a gear-level `created_at_unix_sec` field on `GetGearResponse` and switch the type-pane label to use it
- **Approach:**
  1. Add `int64 created_at_unix_sec = N` to `GetGearResponse` in
     `proto/ripls/api/gear_service.proto`, populated unconditionally
     from `Gear.created_at_unix_sec` (`proto/ripls/models/gear.proto:74`).
  2. Server: set `response.CreatedAtUnixSec = gearStored.CreatedAtUnixSec`
     in `service.go:395-406`, before any community-scoped logic.
  3. Client: in `gear_type_pane.dart:75`, pass
     `gear!.createdAtUnixSec.toInt()` instead of
     `gear!.sharedAtUnixSec.toInt()` to `_buildOwnerLine`.
  4. Defensive: skip the "X ago" suffix entirely when the value is 0
     (legacy gears predating field 26 may still have 0 here).
  5. Verify whether legacy gears have `Gear.created_at_unix_sec = 0` and
     backfill from earliest `CommunityGear.created_at_unix_sec` if so.
- **Pros:**
  - Fixes the semantic mismatch — a gear-level "shared X ago" should not
    be community-scoped.
  - Wire-compatible: adds a new field; old clients keep working.
  - The community-scoped `shared_at_unix_sec` retains its current
    semantics for the access sheet creator line (which *is*
    community-scoped) and for any future community-context UI.
- **Cons:**
  - Requires a proto change, regen, and a coordinated client+server
    rollout (clients with the new code must wait until the field is
    populated by deployed server).
  - May need a backfill if older `Gear` rows have `created_at_unix_sec
    = 0` (field was added in a prior phase).
- **Effort:** small (proto + 4-5 line server change + 1 line client +
  defensive guard).

### Option B: Server-side fallback — populate `shared_at_unix_sec` with `Gear.created_at_unix_sec` when no community match
- **Approach:** In `populateCommunityFields` (or in the unconditional
  response setup), set
  `response.SharedAtUnixSec = gearStored.CreatedAtUnixSec` first, then
  let the existing community-match path overwrite it with
  `communityGear.CreatedAtUnixSec` when applicable. Update the proto
  comment to reflect the new contract.
- **Pros:**
  - Smaller diff: no new proto field, just a behavior change and a
    comment update.
  - All existing client code that already reads `shared_at_unix_sec`
    starts working without modification.
- **Cons:**
  - Conflates two semantics under one field: "when was this gear
    created" vs "when was this gear shared to community X". Callers
    that depended on `0 == not shared to this community` lose that
    signal — though grep shows the only consumers are the two Dart
    sites already discussed.
  - The breaking semantic change is invisible to the wire (still an
    int64) but loud in any code that branches on `shared_at_unix_sec
    == 0`.
- **Effort:** small.

### Option C: Client-only defensive guard
- **Approach:** In `gear_type_pane.dart` and `gear_content_view.dart`'s
  access-sheet helper, treat `sharedAtUnixSec == 0` as "unknown" and
  hide the relative-time suffix (or render "shared recently"). No
  server change.
- **Pros:**
  - Smallest possible change. Ships in the next client release.
  - Useful as a belt-and-suspenders complement to either A or B.
- **Cons:**
  - Doesn't fix the root cause: the gear screen still has *no* "shared
    X ago" label when the user enters without a community context, even
    though we know the answer.
  - Loses information that *exists* on the server.
- **Effort:** trivial (~5 lines).

## Recommendation

**Option A** (new `created_at_unix_sec` field on `GetGearResponse`),
**derived server-side from `MIN(CommunityGear.created_at_unix_sec)`
across non-archived shares**, with a fallback to
`Gear.created_at_unix_sec` for never-shared gear, **plus a slim
version of Option C as a defensive client guard.**

The right model is that the gear's "shared X ago" line in the type pane
is a property of the **gear** (when the owner first put it into the
system), not of any specific community pairing. Adding
`GetGearResponse.created_at_unix_sec` makes the proto contract honest
and mirrors how the same data already flows out of `SaveGearResponse`
(`gear_service.proto:219`).

Option B works in the short term but layers ambiguity onto a field whose
proto comment explicitly promises "Returns 0 if not shared to the
specified community". Honoring that contract is cheaper than rewriting
it.

Option C alone leaves the type-pane label silently absent for any user
who enters the gear screen without a community context — a regression
in information density, not a fix.

**Why MIN(CommunityGear) instead of `Gear.created_at_unix_sec`
directly:** `Gear.created_at_unix_sec` (proto field 26) only landed on
2026-03-29 and has no backfill — so every gear created before that
date carries `0`. `CommunityGear.created_at_unix_sec`, by contrast,
has been reliably written on every `ShareGear` insert since well
before the project was renamed to ripls (October 2025) and is never
clobbered on availability updates or archive flows. Every gear that
has ever been shared therefore carries a correct timestamp on at
least one of its `CommunityGear` rows. Reading the minimum gives us
the canonical "first shared with anyone" date for free — no
backfill, no migration, no startup job. Falling back to
`Gear.created_at_unix_sec` only matters for the niche case of a
never-shared (or all-archived) gear, and that case is already
shielded by the client's `formatTimeAgo(unixSec) → null when <= 0`
guard.

The defensive client guard is still worth keeping because it (a)
short-circuits the niche fallback case where both `Gear` and
`CommunityGear` carry zero, and (b) prevents a future regression of
the same shape in any other code path that reads a Unix timestamp
into `DateTime.fromMillisecondsSinceEpoch` without a presence check.

## Open Questions

- ~~**How many existing `Gear` rows have `created_at_unix_sec = 0`?**~~ —
  **Resolved during implementation.** History showed
  `Gear.created_at_unix_sec` (field 26) was added on 2026-03-29 and
  has no backfill, so every gear created before that date carries 0.
  Rather than build a startup backfill job to repair these rows, the
  fix derives the API field from
  `MIN(CommunityGear.created_at_unix_sec)` instead — that side has
  been reliable since long before the gear-level field existed, so
  no backfill is needed for any gear that's ever been shared. The
  remaining "never-shared, pre-2026-03-29 gear" niche is covered by
  the client's `formatTimeAgo(unixSec) → null` guard.
- **Should the access sheet creator line (`gear_content_view.dart:266-273`) also use the new gear-level field?** —
  Recommendation: **no**. The access sheet is opened in a
  community-scoped flow and the "shared X ago" attribution there
  reads as "when was this gear shared *with us*" — exactly what the
  community-scoped `shared_at_unix_sec` already provides. Add a
  defensive 0-guard there too, but keep the field choice. **(Final
  implementation: kept the community-scoped read and added the
  guard.)**
- ~~**Field number for the new `created_at_unix_sec` on `GetGearResponse`?**~~ —
  **Resolved during implementation.** Used field number 25 (next
  unclaimed; no `reserved` collision), declared adjacent to
  `shared_at_unix_sec` (field 13) per the semantic-grouping
  convention.
- **Is the 20K-days display the only symptom, or are downstream
  metrics (impact/age aggregations) also reading 0 timestamps?** —
  Recommendation: grep for other readers of `gear.sharedAtUnixSec`
  in the client and any `Gear.CreatedAtUnixSec`-only readers on the
  server to confirm. Spot-checked during Phase 1; no other
  user-visible consumers found. The new `formatTimeAgo` helper is a
  natural single chokepoint to route any future call sites through.

## Implementation Plan

### Phase 1: Proto + server + client fix — **SHIPPED** in PR #1688

**Goal:** Replace the epoch-rendering "20575d ago" label with a real
gear-level "shared X ago" string regardless of whether the screen was
entered with a community context.

**Tasks:**
- [x] Added `int64 created_at_unix_sec = 25` to `GetGearResponse` in
      `proto/ripls/api/gear_service.proto`, declared adjacent to
      `shared_at_unix_sec` (field 13). Final proto comment:
      *"Unix timestamp (seconds) when this gear was first shared.
      For gear shared with one or more communities, this is the
      earliest share timestamp across all communities; for gear
      that has never been shared, this is the gear's creation
      timestamp. Distinct from `shared_at_unix_sec`, which carries
      the timestamp of when the gear was shared with the community
      identified in the request."*
- [x] `npm run generate`.
- [x] In `server/services/gear/service.go`, set
      `response.CreatedAtUnixSec = gearStored.CreatedAtUnixSec` as
      a fallback during initial response construction, then in
      `populateGearSharedCommunities` track the minimum
      `cg.CreatedAtUnixSec` across non-archived `CommunityGear`
      rows and override `response.CreatedAtUnixSec` with that
      value when present. This makes the canonical "first shared"
      date the earliest community share timestamp — which is
      reliably populated for every gear that's ever been shared,
      regardless of when the gear-level field was introduced.
- [x] Extracted a pure helper: `DateTimeFormatter.formatTimeAgo(int unixSec) → String?`
      in `app/lib/core/utils/date_time_formatter.dart`, returning
      `null` for `unixSec <= 0` and `${Xd|Xh|Xm} ago` otherwise.
      Strings remain hardcoded English (matching the pre-existing
      strings the helper consolidates); ARB migration deferred to
      a separate workstream.
- [x] Switched `gear_type_pane.dart` to read
      `gear.createdAtUnixSec.toInt()` and route through the
      helper, omitting the relative-time `Text` when the helper
      returns `null`.
- [x] Routed `gear_content_view.dart:266` and
      `gear_access_sheet_helper.dart:32, :130` through the same
      helper with empty-string fallback (the `AccessGroup.sharedTimeAgo`
      contract requires a `String`, not a nullable). These sites
      consume community-scoped values that should always be
      populated; the guard is defensive against unexpected 0s.
- [x] Helper unit tests in
      `app/test/core/utils/date_time_formatter_test.dart`: zero,
      negative, minutes, hours, days, "now". 6 cases / 6 passing.
- [x] Server test in `server/services/gear/service_test.go` covering
      both the orphan-gear fallback path (uses
      `Gear.CreatedAtUnixSec`) and the earliest-CommunityGear path
      (uses `MIN(...)` even when `Gear.CreatedAtUnixSec == 0`).
- [x] `npm run build`, `flutter test --reporter expanded`,
      `npm run lint:dart:i18n`, `npm run lint:dart:a11y` — all clean.

**Validates:** Opening any gear screen without a community context
no longer shows the epoch-derived label; server test pins both
paths; helper unit tests guard against regression at the
formatting layer.

### Phase 2 (deferred — likely unnecessary): Audit other 0-timestamp render paths

**Goal:** Surface any other places where an uninitialized Unix
timestamp could leak to the UI as "epoch days ago".

**Tasks:**
- [ ] Grep client code for `fromMillisecondsSinceEpoch(.*UnixSec`
      and confirm each call site routes through `formatTimeAgo`
      or has an equivalent 0-guard with a server-side guarantee
      that the value is populated.
- [ ] Where existing call sites duplicate the inline
      `inDays/inHours/inMinutes` ternary, replace them with
      `formatTimeAgo`. This both consolidates the bug fix and
      reduces the number of hardcoded English strings the i18n
      lint allowlist would catch on a future ARB migration.

**Validates:** No other "20575d ago" surfaces are lurking; the
formatter is the single chokepoint for this rendering pattern.

### Phase 3 (dropped): Backfill `Gear.created_at_unix_sec`

The original plan called for a startup backfill job to repair
`Gear.created_at_unix_sec = 0` rows. This was rendered unnecessary
by the Phase 1 pivot to derive the API field from
`MIN(CommunityGear.created_at_unix_sec)` instead — every gear that's
ever been shared already has the right answer in its `CommunityGear`
rows, so no migration is needed. The niche case of a never-shared
pre-2026-03-29 gear is covered by the client's `formatTimeAgo → null`
guard.

## Implementation Notes (2026-05-03)

The original recommendation was to populate `created_at_unix_sec`
from `Gear.created_at_unix_sec` and ship a startup backfill for
legacy zero rows. During implementation, a cleaner derivation
surfaced: `CommunityGear.created_at_unix_sec` has been reliably
written on every share since well before the gear-level field
existed, and is never clobbered on subsequent updates or archival.
Reading the minimum across all non-archived `CommunityGear` rows for
the gear gives the canonical "first shared with anyone" timestamp
without any backfill. The plan above reflects the shipped approach;
the backfill phase has been crossed out as unnecessary.

## Audit Notes (added 2026-05-03 by `/ripls-audit`)

The following changes were applied to align the plan with the
project's reference docs:

1. **Test layer corrected.** Original Phase 1 said "unit test for
   `_buildOwnerLine`" — that's a private widget builder, not a
   ViewModel, and `docs/client/testing.md` directs us toward
   pure-Dart tests over widget tests. Plan now extracts a pure
   `formatTimeAgo` helper and tests *that*, removing the need to
   bring up `MaterialApp` + `localizationsDelegates` for a one-line
   formatting test.
2. **Proto comment rewritten.** Original comment claimed "Always
   populated" — this is wrong before Phase 2 runs. Reworded to admit
   `0` as "unknown", and stripped any internal-mechanics phrasing
   per `CLAUDE.md` proto-comment rules.
3. **Field-number guidance tightened.** Added explicit instruction
   to verify against `reserved` declarations and place the new
   field adjacent to `shared_at_unix_sec` (field 13) per the
   memory-noted "place fields near semantically related fields"
   feedback.
4. **Server logging requirements added to Phase 2.** Backfill job
   must use `logging.LoggerWithContext(ctx)` with an `operation`
   field, follow the level taxonomy in
   `docs/server/observability.md`, and surface query-count totals
   via `storage.WithQueryStats`.
5. **SQL-efficiency guardrail added to Phase 2.** Per `CLAUDE.md`
   SQL rules, the backfill must batch reads via `QueryByFieldIn`
   rather than looping `QueryByField` per gear.
6. **i18n status acknowledged.** The pre-existing `'Xd ago'` /
   `'Xh ago'` / `'Xm ago'` strings already violate
   `docs/client/i18n.md` (they're not in `app_*.arb`). Plan now
   notes this is pre-existing, scopes the fix as *no net new
   violations*, and defers full ARB migration of these strings to
   a separate workstream so the bug fix doesn't grow.
7. **Round-trip test scoped out.** `proto_conventions.md` §round-trip
   only requires round-trip tests for fields the client *sets and
   reads back*. `created_at_unix_sec` is server-only on the read
   path; a one-direction server test in
   `server/services/gear/service_test.go` is the right shape.
8. **Lint passes added to Phase 1 verification.**
   `npm run lint:dart:i18n` and `npm run lint:dart:a11y` are now
   explicit so the contributor confirms the allowlist hasn't
   grown.

## Related Issues
- None linked in the issue body or comments. (If a related "gear
  detail timestamps" issue surfaces during implementation, link it
  here.)
