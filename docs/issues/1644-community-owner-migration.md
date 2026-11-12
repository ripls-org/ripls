# Issue #1644: Phase 1 of #1619 — data migration: backfill owner_user_id and verify community_user uniqueness

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1644
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** briangbrown
**Context docs:** docs/community_delete_and_leave.md (read in full); docs/server/architecture.md §"Storage Layer"; docs/server/observability.md

## Audit Notes (2026-05-01)

`/ripls-audit` checked this plan against `docs/server/architecture.md`,
`docs/server/observability.md`, and the `chat_conversation_schema.go`
precedent. Findings, all incorporated below:

1. **Test invocation pattern.** `InitializePostgreSQLDatabase` runs the
   helper at storage setup time — *before* any test seeds rows — so unit
   tests in Phase 2/3/4 must call the exported helper a second time
   after seeding. Phase 2/3/4 task lists updated to spell this out.
2. **Log field name parity.** The chat backfill uses
   `chat_conversation_count` (`server/storage/chat_conversation_schema.go:147`).
   Plan updated to use `community_count` instead of
   `community_count_backfilled` for parity.
3. **Raw SQL in init helpers is fine.**
   `docs/server/architecture.md` §"Storage Best Practices" says "don't
   write raw SQL in *services*" — that does not apply to schema-init
   helpers under `server/storage/`, which legitimately need DDL. Plan
   is consistent with the existing `chat_conversation_schema.go` and
   `story_schema.go` precedents.
4. **Standard logging fields.** All helpers must extract a logger via
   `logging.LoggerWithContext(ctx).With("operation",
   "InitializeCommunityOwnerUserID")` per
   `docs/server/observability.md` §"Standard Field Names". Phase 2/4
   tasks include this explicitly.
5. **Failure mode for duplicate `community_user` rows.** The plan
   chooses startup-fatal (option chosen in Open Questions). This
   matches CLAUDE.md §"Fail Fast" — preferred over silent continuation
   when invariants are violated.
6. **Rollback story.** Schema changes are forward-only here (no
   migrations framework); if the new CHECK constraint causes a
   production incident, the manual rollback is a single
   `ALTER TABLE community DROP CONSTRAINT community_owner_required`.
   Captured below as Open Question 6.
7. **Client docs (`docs/client/*.md`) do not apply.** This is pure
   server-side storage work with no API surface change.
8. **`connecterr.Internal` is not relevant.** That helper is for
   Connect-Go handlers returning `CodeInternal`; storage init helpers
   return regular `error` values. Plan correctly does not invoke it.

## Problem Statement

#1643 added the `Community.owner_user_id` and `Community.deleted_snapshot`
fields to the proto, plus `CommunityUser.deleted` for the rejoin window. The
proto-SQL layer auto-added the new columns at server startup, but every
existing row has `owner_user_id` empty (NULL on rows that existed before
#1645 deployed; `""` on any rows created since, because `CreateCommunity`
in `server/services/community/lifecycle.go` does not yet write the field).

Phase 2 of #1619 will rewrite `LeaveCommunity` / `DeleteCommunity` and add
`RestoreCommunity` / `RejoinCommunity`. Those rewrites assume:

1. Every active community has a non-empty `owner_user_id` pointing at an
   active member.
2. The DB itself enforces the single-owner invariant so a transaction
   bug can't produce a community with no owner.
3. `community_user` cannot have two rows with the same `(community_id,
   user_id)` — the rejoin path replaces a soft-deleted row with an
   active one and must not create duplicates.

This issue lands all three guarantees as a single, idempotent startup
migration. No user-visible behavior changes.

## Context from Comments

The issue has no comments. The body, the parent issue (#1619), and
`docs/community_delete_and_leave.md` §6.1 are the authoritative inputs.
Notable judgments inherited from the body:

- The strong invariant ("owner_user_id must reference a live `community_user`
  row") is hard to express as a CHECK constraint and the body explicitly
  permits punting it to Phase 2's transactional discipline.
- The minimum CHECK is `deleted IS NOT NULL OR owner_user_id IS NOT NULL`.
  The body uses proto-level field names; the schema-layer translation
  has to use the flattened column names (see Open Questions).
- `UNIQUE (community_id, user_id)` "already exists, or should — verify;
  if missing, add as part of this work."

## Codebase research

Confirmed against the actual repo state on `main` after #1645:

- **Migration pattern.** This codebase has two migration shapes:
  - **One-shot data migrations** like `MigrateConversationIDs` in
    `server/storage/migrations.go`, called once from `server/main.go:557`
    at startup. Read row → unmarshal proto → mutate → `storage.Update`
    (which writes both flat columns and `binary_proto`).
  - **Schema-init helpers** like
    `InitializeChatConversationParticipantColumn` in
    `server/storage/chat_conversation_schema.go`, wired from
    `server/storage/postgresql.go:106`. Idempotent: existence checks via
    `information_schema`, ALTER, backfill, index. Runs on every startup
    and on `SetupTestStorage` in tests.
  This issue's work fits the **schema-init helper** shape because it
  combines column backfill, constraint add, and constraint verification.
- **Schema generation.** `server/storage/protosql_schema.go` auto-creates
  tables from proto descriptors and adds new columns via `ALTER TABLE
  ADD COLUMN` (no DEFAULT, so existing rows are NULL-filled). New rows
  go through `Insert(ctx, msg)` which writes both the flat column and
  `binary_proto`. Empty-string proto fields land as `""` in the flat
  column.
- **The `deleted` field flattens.** `optional DeletedMetadata deleted = 20`
  becomes columns `deleted_deleted_by_user_id` and
  `deleted_deleted_at_unix_sec`. The migration in
  `server/storage/migrations.go:56` already uses this naming. The CHECK
  constraint must reference `deleted_deleted_by_user_id`.
- **`CreateCommunity` does not yet write `owner_user_id`.**
  `server/services/community/lifecycle.go:38-46`. New rows post-#1645 have
  `owner_user_id = ""`. Adding the CHECK constraint without first fixing
  CreateCommunity would break community creation.
- **Test storage path** runs `InitializePostgreSQLDatabase`, which is
  the same code path as production. Adding an init helper to
  `postgresql.go` automatically covers tests.
- **No existing `UNIQUE (community_id, user_id)` constraint** found by
  grep. Need to add it.

## Potential Solutions

### Option A: Single startup-init helper + CreateCommunity fix (recommended)

- **Approach:** Add `server/storage/community_schema.go` containing
  `InitializeCommunityOwnerUserID(ctx)`. Wire it from
  `postgresql.go` after the existing chat_conversation init. The helper:
  1. Checks the `community` table exists.
  2. Backfills `owner_user_id` from `creator_id` for every row where
     `owner_user_id IS NULL OR owner_user_id = ''` — by reading
     `binary_proto`, unmarshaling, setting `OwnerUserId`, calling
     `storage.Update` so both the flat column and proto are consistent.
  3. Adds the CHECK constraint via a `pg_constraint` lookup-then-add
     pattern (idempotent).
  4. Checks `community_user` for duplicate `(community_id, user_id)`
     rows; logs and aborts the helper if any exist (caller turns this
     into a startup failure so we don't silently continue).
  5. Adds `UNIQUE (community_id, user_id)` if not present.

  Also fix `CreateCommunity` in
  `server/services/community/lifecycle.go` to set `OwnerUserId =
  authInfo.UserID` (same value as `CreatorId`). This is a one-line
  change, required so the new CHECK constraint doesn't reject new
  communities.

- **Pros:** Matches the canonical pattern in this repo
  (`InitializeChatConversationParticipantColumn`). Single PR, single
  review, single deploy. Test coverage is automatic via
  `SetupTestStorage`. Idempotent — safe to re-run.
- **Cons:** Bundles a `CreateCommunity` behavior change with the
  migration. Mitigated by the change being one line and necessary for
  the constraint to ship.
- **Effort:** Small to medium.

### Option B: Two PRs — backfill first, then constraint

- **Approach:** PR1 ships only the backfill helper and the
  `CreateCommunity` fix. Verify on dev for a deploy or two. PR2 adds
  the CHECK constraint and the UNIQUE constraint.
- **Pros:** Lets us watch the backfill behavior in dev before tightening
  the screws.
- **Cons:** Doubles the review/deploy overhead. The backfill and the
  constraint are tightly coupled — there is no real value to checking
  in between, since the helper is idempotent and the test path
  exercises both.
- **Effort:** Medium (two PRs).

### Option C: Backfill only; punt constraints to Phase 2

- **Approach:** Ship just the backfill + `CreateCommunity` fix. Leave
  the CHECK and UNIQUE constraints for whichever Phase 2 PR first
  touches them.
- **Pros:** Smallest possible diff for this issue.
- **Cons:** The acceptance criteria in the issue explicitly call for
  the constraint and the uniqueness verification. Splitting them off
  would re-file a follow-up issue. Phase 2 is more complex and would
  benefit from these guardrails being already in place.
- **Effort:** Small.

## Recommendation

**Option A.** It matches the established pattern, ships the issue's
acceptance criteria in one piece, and the bundled `CreateCommunity`
fix is small and necessary (the constraint needs new rows to populate
the field).

Run order at startup:
1. ProtoSQL auto-creates / alters tables to match proto definitions
   (column already added by #1645's deploy).
2. Existing inits (`InitializeStoryParticipantColumn`,
   `InitializeChatConversationParticipantColumn`).
3. New `InitializeCommunityOwnerUserID`.
4. The unrelated `MigrateConversationIDs` in `main.go` continues as-is.

## Open Questions

- **CHECK constraint exact SQL.** The issue body says
  `CHECK (deleted IS NOT NULL OR owner_user_id IS NOT NULL)` but the
  flat schema has `deleted_deleted_by_user_id`, not `deleted`, and
  proto3 strings round-trip as `''` not NULL. **Recommendation:**
  ```sql
  CHECK (
    (deleted_deleted_by_user_id IS NOT NULL AND deleted_deleted_by_user_id <> '')
    OR
    (owner_user_id IS NOT NULL AND owner_user_id <> '')
  )
  ```
  This expresses "either soft-deleted or has an owner" in terms the
  schema layer actually uses. Empty string is treated as absence to
  match how the rest of the codebase reads these columns
  (`server/storage/migrations.go:56` uses the same idiom).
- **`CreateCommunity` ownership.** Strict reading of #1619 places
  CreateCommunity changes in Phase 2. **Recommendation:** include the
  one-line `OwnerUserId: authInfo.UserID` fix in this PR. It's not a
  behavior change visible to clients (the field is server-only on the
  API today and not exposed) and it's required for the new CHECK
  constraint to be satisfiable on new rows. The alternative — landing
  the constraint without it — breaks production.
- **Duplicate `(community_id, user_id)` rows on prod.** The body says
  "should be none." If they do exist, what's the policy?
  **Recommendation:** Fail startup with a clear error pointing at the
  duplicate IDs. Don't silently delete or merge — duplicates indicate
  a data-integrity issue we want a human to resolve. The startup-init
  helper logs the duplicate keys and returns an error;
  `postgresql.go` propagates it and `main.go` exits non-zero. If this
  fires, file a fast-follow cleanup issue.
- **The strong owner-membership invariant.** "owner_user_id must
  reference an active `community_user` row." A CHECK constraint can't
  do this in vanilla PostgreSQL (subqueries forbidden in CHECKs). A
  trigger could, but that introduces a maintenance burden.
  **Recommendation:** punt to Phase 2 per the issue's permission. The
  transactional `SELECT … FOR UPDATE` discipline in
  `LeaveCommunity` / owner-handoff is the load-bearing enforcement;
  the CHECK constraint is belt-and-suspenders for the simpler
  not-null-on-active-rows shape.
- **Backfill cap.** `MigrateConversationIDs` uses `LIMIT 10000` per
  invocation. **Recommendation:** mirror that for the community
  backfill; current dev/prod community counts are well below 10k. If
  we ever exceed that, we'll need a paged loop — file a follow-up
  if the cap ever hits.
- **Dev → prod sequencing.** The issue says "dev first, then prod."
  **Recommendation:** the helper is idempotent and runs at every
  startup; "dev first" just means we deploy the server to dev first,
  watch it succeed, then promote. Standard release flow handles this
  — no special migration deployment needed.
- **Rollback story for the new CHECK constraint.** Forward-only
  proto-SQL has no migration-framework rollback. If the new
  `community_owner_required` CHECK ever causes a production incident,
  the manual remediation is `ALTER TABLE community DROP CONSTRAINT
  community_owner_required` from a connected `psql` session, followed
  by either fixing the offending insertion path or shipping a
  follow-up PR that revises the constraint. **Recommendation:**
  document this single-line rollback in the PR description so an
  on-call who needs it can find it without reading the codebase. No
  in-repo rollback automation needed.

## Implementation Plan

### Phase 1 — `CreateCommunity` writes `owner_user_id`
**Goal:** Stop the bleeding so new rows post-PR satisfy the upcoming
CHECK constraint.
**Tasks:**
- [ ] In `server/services/community/lifecycle.go:38-46`, add
      `OwnerUserId: authInfo.UserID,` to the `&models.Community{...}`
      literal. Comment-free; the field's proto comment carries the
      semantics.
- [ ] Spot-check: run a single test that inserts and re-reads a
      Community to verify the column round-trips.
**Validates:** New communities have `owner_user_id` populated; the
constraint we add in Phase 3 will be satisfiable.

### Phase 2 — Backfill helper (no constraint yet)
**Goal:** Get all existing rows into a constraint-compliant state.
**Tasks:**
- [ ] Create `server/storage/community_schema.go` with
      `InitializeCommunityOwnerUserID(ctx)`. Mirror the structure of
      `chat_conversation_schema.go`:
  - [ ] First line of the function: `logger :=
        logging.LoggerWithContext(ctx).With("operation",
        "InitializeCommunityOwnerUserID")` per
        `docs/server/observability.md` §"Standard Field Names".
  - [ ] `information_schema.tables` check for `community`. Skip if
        absent (lets test runs that don't register the type pass).
  - [ ] `information_schema.columns` check for `owner_user_id`. Skip if
        absent (means proto-SQL hasn't auto-added it yet for some
        reason; non-fatal).
  - [ ] Query rows where `owner_user_id IS NULL OR owner_user_id = ''`
        with `LIMIT 10000`. Read `id`, `binary_proto`.
  - [ ] For each row: unmarshal `models.Community`, set `OwnerUserId =
        community.CreatorId` if currently empty, call
        `s.Update(ctx, community)`. Log per-row failures at WARN and
        continue (mirrors `backfillChatConversationParticipantIds`).
  - [ ] Log a summary at INFO at the end: `community_count`,
        `duration_ms` (matches the chat helper's `chat_conversation_count`
        + `duration_ms` shape).
- [ ] Wire into `server/storage/postgresql.go:111` after the
      chat_conversation init. Bubble error like the others — fatal so
      ops sees backfill failures.
- [ ] Unit test in `community_schema_test.go`: get a fresh storage via
      `SetupTestStorage(t)` (which already invoked the helper against
      an empty DB), seed a community with `OwnerUserId: ""` via
      `s.Insert`, then **call `s.InitializeCommunityOwnerUserID(ctx)`
      again explicitly** — the second call is what exercises the
      backfill path. Assert `owner_user_id == creator_id` afterward,
      both via `s.GetByID` (binary_proto round-trip) and via direct
      SQL on the flat column.
**Validates:** Existing rows are constraint-compliant; the next phase
can safely add the constraint.

### Phase 3 — Add the CHECK constraint
**Goal:** Lock the active-row owner invariant at the DB level.
**Tasks:**
- [ ] In `InitializeCommunityOwnerUserID`, after backfill: query
      `pg_constraint` for `conname = 'community_owner_required' AND
      conrelid = '"community"'::regclass`. If absent, run:
      ```sql
      ALTER TABLE "community" ADD CONSTRAINT community_owner_required
        CHECK (
          (deleted_deleted_by_user_id IS NOT NULL AND deleted_deleted_by_user_id <> '')
          OR
          (owner_user_id IS NOT NULL AND owner_user_id <> '')
        );
      ```
- [ ] Unit test: with the constraint in place, attempt to
      `s.Insert(ctx, &models.Community{...})` with explicit
      `OwnerUserId: ""` (and no `Deleted`). Storage writes the flat
      column verbatim from the proto, so this should hit the CHECK
      and the returned error wraps a `pgconn.PgError` with
      `Code: "23514"` (check_violation). Then fix `OwnerUserId` and
      assert the retry succeeds. Same test pattern as Phase 2:
      `SetupTestStorage` runs the helper once at boot; re-running is
      not needed since the constraint has already been added.
**Validates:** Phase 2 server work in #1619 can rely on the
not-null-on-active-rows invariant without re-checking it everywhere.

### Phase 4 — `community_user` uniqueness
**Goal:** Block any path that would create duplicate memberships.
**Tasks:**
- [ ] New init helper `InitializeCommunityUserUniqueness(ctx)` in the
      same file (or its own file if line-count concerns hit).
- [ ] First line: `logger := logging.LoggerWithContext(ctx).With(
      "operation", "InitializeCommunityUserUniqueness")`.
- [ ] `information_schema.tables` check for `community_user`. Skip if
      absent.
- [ ] Query for duplicates:
      ```sql
      SELECT community_id, user_id, COUNT(*)
      FROM "community_user"
      GROUP BY 1, 2
      HAVING COUNT(*) > 1
      LIMIT 10
      ```
      If any rows: log them at ERROR with the offending IDs, return a
      startup error (not a warning). The caller in `postgresql.go`
      propagates the error and `main.go` exits non-zero. Do not
      auto-fix. Per CLAUDE.md §"Fail Fast": invariant violations are
      not safe to ignore.
- [ ] Query `pg_constraint` for the unique constraint by name
      (`community_user_unique`). If absent:
      ```sql
      ALTER TABLE "community_user"
        ADD CONSTRAINT community_user_unique
        UNIQUE (community_id, user_id);
      ```
- [ ] Wire after `InitializeCommunityOwnerUserID` in
      `postgresql.go`.
- [ ] Unit test: `SetupTestStorage`, insert one membership via
      `s.Insert`, attempt to insert a second `CommunityUser` with
      the same `(community_id, user_id)` and a different `Id`,
      assert the error wraps a `pgconn.PgError` with
      `Code: "23505"` (unique_violation).
**Validates:** Phase 2 server work in #1619 can use UPSERT-style
membership writes without worrying about duplicate-row leakage.

### Phase 5 — Verify and ship
**Goal:** Prove the migration is safe to re-run and that all gates
pass.
**Tasks:**
- [ ] `npm run build` — clean.
- [ ] `npm run lint:ephemeral-comments` — clean.
- [ ] `go test ./server/storage/...` and `go test
      ./server/services/community/...` — green. Both should now
      include the new tests.
- [ ] Restart twice in a local dev DB (or by running the test setup
      twice) to confirm idempotence: no new updates, no constraint
      add attempts, no errors.
- [ ] Open PR titled "Phase 1 of #1619: data migration —
      backfill owner_user_id and verify community_user uniqueness
      (issue #1644)".
**Validates:** Phase 1 of #1619 is complete; Phase 2 server work can
start.

## Related Issues

- **#1619** — Parent: full community soft-delete project. Phase 1
  step 2.
- **#1643** — Phase 1 step 1 (proto + schema additions). Merged via
  #1645. This issue's preconditions.
- **#1621, #1622, #1623, #1624** — Sibling Phase 1 issues. All closed.
- **#1620** — Phase 4: daily community purge job. Out of scope.
- **#1571** — Will close as superseded once Phase 2 step 8 ships.
