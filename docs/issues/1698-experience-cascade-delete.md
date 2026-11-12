# Issue #1698: DeleteExperience leaves dangling RSVPs / time proposals / community events — non-owner clients hit 404 after delete

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1698
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** bug | **Assignees:** unassigned
**Context docs:** none

> **Revision (2026-05-04, post-author-review):** The original plan
> proposed soft-deleting `CommunityEvent` and `Story` rows via cascade.
> Author push-back: those tables are the central activity log of the
> system; we deliberately have not added soft-delete to them even
> though it exists on most other entities. Retroactively tombstoning
> historical events to hide a dangling reference is the wrong shape —
> it loses audit trail and breaks the established pattern that
> `ListCommunityEvents` already follows for deleted *users* (the
> `FormerMember` placeholder via `ResolveUserOrFormer` at
> `server/services/community/events.go:145` and the
> `feed/service.go:964` WARN-and-skip for missing experiences).
>
> Plan now splits the six tables by **shape**:
>
> - **Current-state participant items** (cascade soft-delete): only
>   `ExperienceRSVP`, `ExperienceTimeProposal`, `PlanningNeed`,
>   `PlanningContribution`. These represent live commitments by
>   participants; when the parent experience is deleted, those
>   commitments should disappear from "my upcoming events" / planning
>   screens just like a deleted gear's `community_gear` row disappears.
> - **Historical activity log** (lenient-read, no cascade):
>   `CommunityEvent`, `Story`. These are append-only records of what
>   happened. When the parent experience is gone, readers should
>   degrade gracefully (omit the dead reference, mirror the
>   FormerMember pattern) rather than retroactively erasing history.
>
> This eliminates two of the three proposed proto schema changes
> (CommunityEvent stays untouched; Story doesn't get a new cascade) and
> keeps the audit log intact. RSVP and TimeProposal still need
> `optional DeletedMetadata` added — those are current-state by
> definition.
>
> Plan also retains: (1) the resolved oneof open question
> (protosql handles `QueryByField(ctx, "experience_id", id, ...)` against
> oneof scopes per `server/planning/list.go:39-43`); (2) the capturing
> `slog.Handler` test sub-task; (3) the note that no
> `CommunityEventType` enum value is added (the gate at
> `TestEveryEventTypeClassified` remains untriggered).

## Problem Statement

`DeleteExperience` soft-deletes the parent experience and cascades to chat
conversations, the `community_experience` join table, and media — but does not
cascade to the **current-state participant tables** that hold an
`experience_id`. As a result, when an experience is deleted, every RSVP and
time proposal for that experience (and every planning need / contribution
scoped to it) stays alive. Non-owner clients with state pointing at the
deleted experience (typically a cached "my upcoming events" or RSVP screen)
hit `GetExperience` / `GetExperienceStats` on the next session start, the
server correctly returns `connect.CodeNotFound`, and the handler logs at
ERROR — which fires the `Server Error Logged` alert.

A separate but related problem is that **historical activity-log readers**
(`ListCommunityEvents`, story renderers) need to gracefully omit references
to deleted experiences rather than skipping the whole event/story or 404'ing.
The `feed/service.go:964` and `community/events.go:145` paths already do this
for missing users via the `FormerMember` placeholder pattern; we extend the
same shape to missing experiences. We do **not** soft-delete the historical
log itself — it's append-only by design.

This is the experience-side equivalent of #1185 (DeleteGear not cascading to
`community_gear`). The doctrine settled in #1184 — keep ERROR logs loud and
fix the data bug they expose — applies directly: the alert is correctly
surfacing a real cascade gap on the current-state side.

## Context from Comments

No comments yet. The issue body is detailed and self-consistent. The
description has prod-log evidence (timeline of the incident on revision
`server-00088-n2c` at 2026-05-04 01:44 UTC) and proposes a 3-phase fix.

## Codebase findings

### Tables holding `experience_id` (current cascade status)

| Storage model | Shape | Has `DeletedMetadata`? | Today | Plan |
|---|---|---|---|---|
| `Experience` (parent) | parent | YES (`experience.proto:64`) | — | — |
| `CommunityExperience` | join | YES (`community.proto:192`) | cascade | unchanged |
| `ChatConversation` (via `topic_experience_id`) | join | YES | cascade | unchanged |
| `Media` (via `experience.media_ids`) | parent owns ids | YES | cascade | unchanged |
| `PlanningNeed` | current-state | YES (`planning.proto:23`) | none | **cascade** (Phase 1) |
| `PlanningContribution` | current-state | YES (`planning.proto:46`) | none | **cascade** (Phase 1) |
| `ExperienceRSVP` | current-state | **NO** | none | **add `Deleted` + cascade** (Phase 2) |
| `ExperienceTimeProposal` | current-state | **NO** | none | **add `Deleted` + cascade** (Phase 2) |
| `CommunityEvent` | activity log | NO | none | **lenient-read only** (Phase 3) — no soft-delete, no schema change |
| `Story` | activity log | YES (`story.proto:44`) | none | **lenient-read only** (Phase 3) — keep history intact |
| `Notification.CommunityEventPayload` | transient | — | — | out of scope |

The split is shape-driven, not capability-driven:

- **Current-state participant items** are commitments — RSVPs, planning
  needs, time proposals, contributions. When the parent experience dies,
  the commitment is meaningless and should disappear from participant
  views. Cascade soft-delete is correct, mirroring
  `CascadeDeleteCommunityGearByGearID`.
- **Activity log** rows (`CommunityEvent`, `Story`) are an append-only
  historical record. Soft-deleting them retroactively rewrites history
  to hide that an experience ever existed. The codebase has deliberately
  kept `CommunityEvent` without a `Deleted` field for that reason. The
  right pattern for these is graceful read-side degradation — the same
  `FormerMember` placeholder pattern already used for deleted *users*.

### Read-side filtering

All current queries against `ExperienceRSVP` / `ExperienceTimeProposal` /
`CommunityEvent` use `QueryByField` / `QueryByFieldIn` without
`IncludeDeleted: true` (e.g.
`server/services/experience/build_api.go:68,79`,
`stats.go:69`, `summary.go:102`, `rsvp.go:93`). The `protosql` query layer
filters soft-deleted rows by default — see existing CommunityExperience
behavior. So once the cascade soft-deletes these rows, **read sites need no
changes**: existing queries will simply stop returning the dead rows.
This only holds after the proto schema change for the three tables that
don't yet have `Deleted`; until that change lands, the field literally
doesn't exist for them, so the soft-delete filter is a no-op.

### Existing cascade helper pattern

`server/storage/cascade_delete.go` already establishes the pattern:
`CascadeDeleteCommunityExperienceByExperienceID` (line 153) uses
`QueryByField` with `IncludeDeleted: true` to find all rows including
already-soft-deleted ones, skips already-deleted (`cg.Deleted != nil`), and
sets the same `DeletedMetadata` on the rest. New helpers should follow this
shape exactly. Note the planning models use a `oneof` for scope — protosql
flattens oneof string fields to plain columns, so `QueryByField(ctx,
"experience_id", id, &PlanningNeed{})` should work — but this needs
verification (see Open Questions).

### What surfaced in the incident

Three feed-service WARN lines at `feed/service.go:964` ("experience not found
in batch for feed") in the same session window confirm a dangling
`CommunityEvent` reference for the experience. The two `GetExperience` ERRORs
plus the `GetExperienceStats` ERROR almost certainly came from a client
screen rendering a cached `ExperienceRSVP` for the deleted experience —
that's the most natural client-side surface for "open app → fetch experience
I have an RSVP for."

## Potential Solutions

### Option A: Shape-driven — cascade current-state, lenient-read activity log (recommended)

- **Approach:** Cascade soft-delete only the four current-state tables
  (RSVP, time proposal, planning need, planning contribution). Add
  `optional DeletedMetadata` to RSVP and TimeProposal (the two that don't
  yet have it). For activity-log readers (`ListCommunityEvents`, story
  rendering), extend the existing `FormerMember`-style graceful degradation
  to dangling experience refs — log at WARN, render with a placeholder or
  omit cleanly, never 404 the whole row.
- **Pros:** Matches the codebase's settled split between commitments and
  history. Two new proto fields instead of three. Doesn't rewrite history.
  Each side (write-time cascade, read-time tolerance) addresses the actual
  bug shape.
- **Cons:** Two distinct mechanisms to maintain. Read-time tolerance
  requires touching every `CommunityEvent`/`Story` reader (limited
  inventory: `ListCommunityEvents`, the feed-service feed lookup, story
  rendering — all already have placeholder patterns to extend).
- **Effort:** Medium.

### Option B: Cascade everything (original plan, rejected)

- **Approach:** Soft-delete CommunityEvent and Story rows when the parent
  experience is deleted. Adds `optional DeletedMetadata` to CommunityEvent.
- **Pros:** Single mechanism (cascade only).
- **Cons:** Tombstones an append-only activity log to hide a dangling
  reference. Loses audit trail of what events existed. Sets a precedent
  that "any dangling ref → delete the log row," which compounds over
  future deletions (delete user → delete every event they actor'd?). The
  team has deliberately not added `Deleted` to `CommunityEvent` — this
  plan should respect that.
- **Effort:** Same as A, but architecturally wrong.

### Option C: Read-time tolerance only (no cascade)

- **Approach:** Don't cascade anything. Make every reader lenient about
  dangling experience refs.
- **Pros:** No write-path changes.
- **Cons:** A non-owner with an RSVP to a deleted experience would still
  see "RSVP'd to <deleted>" everywhere their RSVP is rendered. Current-state
  views should not surface dead commitments — that's a UX bug, not just a
  log noise issue. Leaves the underlying data shape inconsistent with
  `community_gear` and `community_experience`, which already cascade.
- **Effort:** Smallest, but underserves the problem.

## Recommendation

**Option A.** The shape-driven split is what the codebase already implies:
`community_gear` cascades (current-state join), `CommunityEvent` doesn't
have `Deleted` (history). Aligning the new work with that split keeps the
mental model coherent. Two proto fields (RSVP, TimeProposal) is a low cost
for closing the current-state half; the activity-log half is read-side
hardening that mostly extends an already-present pattern.

## Open Questions

- **Does protosql support `QueryByField(ctx, "experience_id", id,
  &PlanningNeed{})` when `experience_id` lives inside a `oneof`?** —
  **Resolved (audit):** Yes. `server/planning/list.go:39-43` already calls
  `QueryByFields[*models.PlanningNeed]` with `{"experience_id": id}`
  against the same oneof scope and it works in production. Protosql
  flattens oneof string fields to their bare proto field name as columns,
  so the cascade helper is a one-liner just like
  `CascadeDeleteCommunityExperienceByExperienceID`. The same applies to
  `CommunityEvent.experience_id` (oneof topic) — query by
  `"experience_id"` directly.

- **What happens to undo when the parent experience is deleted?** — With
  the revised plan, `CommunityEvent` rows are not soft-deleted, so
  `UndoCompleteExperience` (`server/services/experience/undo.go:57`)
  still loads the event normally. But the experience itself is
  soft-deleted, so any undo path that dereferences the parent will
  surface that. Recommendation: keep the existing undo behavior; if a
  user attempts undo on an event whose parent experience is deleted,
  return a clean `CodeFailedPrecondition` rather than panicking. Add a
  Phase-4 sub-assertion to lock this in.
- **Should `CommunityEvent` get a `DeletedMetadata` field, or should the
  cascade just filter at read time using a `JOIN` against
  `experience.deleted_at`?** — Recommendation: add `DeletedMetadata`. The
  feed service already does the right thing at WARN, but other consumers
  of `CommunityEvent` (story generation, undo, notification fanout
  history) deserve a uniform "soft-deleted means hidden" rule. The
  read-time-JOIN approach scatters the policy across every reader and is
  fragile. Same shape as `community_experience`.
- **Should the cascade also touch `Notification` rows referencing the
  experience?** — Recommendation: out of scope for this issue.
  Notifications are transient (already-delivered push records); no UI
  reads them post-delivery to make a `GetExperience` call. If the alert
  recurs on a notification-driven surface, file separately.
- **Does undo of `DeleteExperience` need to un-cascade?** — Recommendation:
  out of scope for this issue, but write the cascade helpers so that an
  un-cascade is straightforward (single `Update` per row, clearing
  `Deleted`). Track in #1240 / undo work if a design exists.
- **What tests should assert "no ERROR logs"?** — Recommendation: a single
  integration test using a capturing `slog.Handler` (the structured-logging
  package supports this — see `server/logging/`). Run a representative
  non-owner read sequence post-delete and assert zero ERROR records. This
  is the regression guard the system has been missing for both #1185 and
  this issue.

## Implementation Plan

### Phase 0: Scope check (DB read-only, no code)

**Goal:** Quantify the dangling-reference population for the four
current-state tables so the Phase 3 backfill has measured scope. Mirrors
the 220-row count from #1185.

**Tasks:**

- [ ] Run on dev DB (adjust column names if `Deleted` is flattened with a
      `deleted_*` prefix in your schema):

  ```sql
  SELECT 'rsvp' AS kind, COUNT(*) FROM experience_rsvp r
  JOIN experience e ON e.id = r.experience_id
  WHERE e.deleted_at IS NOT NULL
  UNION ALL SELECT 'time_proposal', COUNT(*) FROM experience_time_proposal p
  JOIN experience e ON e.id = p.experience_id WHERE e.deleted_at IS NOT NULL
  UNION ALL SELECT 'planning_need', COUNT(*) FROM planning_need n
  JOIN experience e ON e.id = n.experience_id WHERE e.deleted_at IS NOT NULL
  UNION ALL SELECT 'planning_contribution', COUNT(*) FROM planning_contribution c
  JOIN experience e ON e.id = c.experience_id WHERE e.deleted_at IS NOT NULL;
  ```

- [ ] Also count `community_event` and `story` rows that reference
      soft-deleted experiences — not for cascade, but to size the
      lenient-read code path's exposure (how often will the WARN-and-skip
      branches fire in steady state?).
- [ ] Repeat on prod DB.
- [ ] Record counts in this doc and in the issue thread.

**Validates:** Sizing the backfill, and confirming the lenient-read
mechanism isn't being asked to handle pathological volume.

### Phase 1: Cascade helpers for planning tables (no proto changes)

**Goal:** Wire up cascade for the two current-state tables that already
have `DeletedMetadata` (PlanningNeed, PlanningContribution).

**Tasks:**

- [ ] In `server/storage/cascade_delete.go`, add (mirroring
      `CascadeDeleteCommunityExperienceByExperienceID`):
      - `CascadeDeletePlanningNeedsByExperienceID(ctx, storage, experienceID, deletedMetadata)`
      - `CascadeDeletePlanningContributionsByExperienceID(ctx, storage, experienceID, deletedMetadata)`

      Both can use `QueryByField(ctx, "experience_id", id, ...)` directly —
      `server/planning/list.go:39-43` already does this against the same
      oneof scope, so protosql handles the flattening transparently.
- [ ] In `server/services/experience/delete.go`, wire the two new helpers
      in after the existing cascades.
- [ ] Add per-helper unit tests under `server/storage/cascade_delete_test.go`
      (or wherever existing cascade tests live) — assert idempotency,
      correct filtering of already-deleted rows, propagation of
      `DeletedMetadata`, and that contributions/needs scoped to a
      *request* (the other branch of the scope oneof) are unaffected.

**Validates:** Two of four current-state surfaces are closed without a
proto change; the harder half (proto + cascade) is unblocked but not yet
required.

### Phase 2: Add `DeletedMetadata` to RSVP and TimeProposal, cascade them

**Goal:** Enable soft-delete on the two current-state tables that don't yet
support it, then cascade. **CommunityEvent and Story are intentionally
not touched here** — they are activity logs, handled in Phase 3.

**Tasks:**

- [ ] In `proto/ripls/models/experience.proto`, add
      `optional DeletedMetadata deleted = N;` to `ExperienceRSVP` and
      `ExperienceTimeProposal`. Pick the next free field number on each
      message.
- [ ] Run `npm run generate`.
- [ ] In `server/storage/cascade_delete.go`, add:
      - `CascadeDeleteExperienceRSVPsByExperienceID(...)`
      - `CascadeDeleteExperienceTimeProposalsByExperienceID(...)`
- [ ] Wire both into `server/services/experience/delete.go`.
- [ ] Per-helper unit tests.
- [ ] Verify existing read sites (`build_api.go:68,79`, `stats.go:69`,
      `summary.go:102`, `rsvp.go:93`, etc.) keep working without
      modification — protosql's default soft-delete filter starts excluding
      these rows automatically once `Deleted` is a real field.

**Validates:** All four current-state surfaces (RSVP, time proposal,
planning need, planning contribution) are now cascaded on
`DeleteExperience`. New deletes leak no current-state dangling rows.

### Phase 3: Lenient-read for CommunityEvent and Story (no schema change)

**Goal:** Make every reader of `CommunityEvent` and `Story` tolerate a
dangling experience reference the same way they already tolerate a deleted
user (the `FormerMember` placeholder pattern at
`server/services/community/events.go:145` via
`services.ResolveUserOrFormer`).

**Tasks:**

- [ ] Inventory readers of `CommunityEvent` that dereference
      `event.GetExperienceId()` (start with `ListCommunityEvents`,
      `feed/service.go`, story rendering, undo handlers). For each:
      - Batch-fetch experiences with `IncludeDeleted: true` so soft-deleted
        ones are visible in the lookup map, then check `experience.Deleted
        != nil` at render time and substitute a `FormerExperience`-style
        placeholder (or omit the row cleanly, matching `feed/service.go:964`'s
        existing WARN-and-skip).
      - Log at WARN with `experience_id` and operation context, never ERROR.
- [ ] Add a small helper analogous to `ResolveUserOrFormer` —
      e.g. `services.ResolveExperienceOrDeleted(map, id) (*api.Experience, bool)` —
      so every reader uses the same shape. Place it in
      `server/services/services` (where `ResolveUserOrFormer` lives) for
      reuse.
- [ ] For `Story` rendering, decide whether the deleted-experience case
      should hide the whole story or render with a "this event was
      removed" caption. Default: hide the story (story has no value
      without its experience context). Document the choice in code.
- [ ] Unit-test each reader path: experience exists → renders normally;
      experience soft-deleted → graceful fallback, no ERROR log; experience
      genuinely missing (race / data corruption) → also graceful, but log
      WARN with `experience_id`.

**Validates:** Activity-log readers stop emitting ERROR on dangling
experience refs. History stays intact in DB; UX degrades gracefully.

### Phase 4: Backfill already-deleted experiences

**Goal:** Apply the Phase 1 + Phase 2 cascades retroactively to experiences
deleted before the fix shipped, so existing dangling current-state rows
go away. (Activity-log rows are not backfilled — Phase 3's read-time
tolerance handles them in place.)

**Tasks:**

- [ ] Add `server/jobs/experience_cascade_backfill.go` (mirroring
      `server/jobs/impact_backfill.go` shape — startup goroutine,
      idempotent, logs counts).
- [ ] Backfill walks all soft-deleted experiences, runs every cascade
      helper from Phases 1 and 2 (RSVP, time proposal, planning need,
      planning contribution), logs `dangling_rows_cleaned` per table.
- [ ] Wire into `main.go` as a startup goroutine via `logging.GoSafe`.
- [ ] Unit test in `server/jobs/experience_cascade_backfill_test.go`:
      empty DB, deleted experience with dangling rows of each kind,
      idempotent re-run, missing-experience tolerance.

**Validates:** Existing prod backlog of dangling current-state rows is
cleaned up; the alert stops firing on historical state, not just on new
deletes.

### Phase 5: Integration regression test

**Goal:** Lock in the no-ERROR-log invariant so a future cascade gap fails CI
loudly.

**Tasks:**
- [ ] Add `server/services/experience/delete_cascade_integration_test.go`
      (or extend an existing experience integration test) that:
      1. Creates an experience with the owner
      2. Has a non-owner user RSVP (`YES` intention)
      3. Owner adds a need; non-owner contributes
      4. Owner creates a time proposal
      5. Server emits a `CommunityEvent` for the experience creation
      6. Story generation runs (or stub a story row)
      7. Owner calls `DeleteExperience`
      8. Non-owner calls `GetExperience`, `GetExperienceStats`,
         `ListMyExperiences`, `ListCommunityEvents`, and the feed list
      9. Assert: every non-owner RPC returns either an empty list / a
         clean `CodeNotFound` / a list with the dangling-reference row
         omitted (for activity-log paths), and the captured `slog.Handler`
         records **zero ERROR-level entries** for the test's request ID
      10. Assert: the `CommunityEvent` and `Story` rows are still **alive**
          in the DB after delete — history isn't tombstoned
- [ ] Use a capturing `slog.Handler` for the assertion. If a test helper
      doesn't already exist in `server/logging/` for this, add one
      (~30 lines: a `slog.Handler` that records all `slog.Record`s into
      a slice, with a `RecordsAtLevel(slog.Level) []slog.Record`
      accessor). The structured-logging design in
      `docs/server/observability.md` §Part 1 is built on stdlib `slog`
      so a custom handler is the natural extension point.
- [ ] Add a sub-assertion: call `UndoCompleteExperience` against an
      already-cascaded `CommunityEvent` and assert it returns clean
      `CodeNotFound` rather than panicking or logging ERROR (covers
      the audit-flagged undo-interaction concern).
- [ ] Run on CI

**Validates:** The exact alert signature this issue is about can never
recur silently; if a new dangling table is added without a cascade helper,
the test fails before merge.

## Related Issues

- #1184 — "Not-found lookups logged at ERROR" (closed working-as-intended).
  Established the doctrine: keep ERROR logs loud, fix the data bug they
  expose.
- #1185 — "DeleteGear doesn't cascade to community_gear — 220 dangling live
  refs in dev." Direct precedent for the cascade-gap fix shape, including
  the backfill pattern this plan mirrors.
- #1690 — Adjacent recent fix for false ERROR alerts in the notifications
  cleanup path (race-on-delete). Different root cause (concurrent cleanup
  race) but same alert-policy hit.
