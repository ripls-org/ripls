# Issue #1185: DeleteGear doesn't cascade to community_gear — 220 dangling live refs in dev

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1185
**Filed:** 2026-04-18 | **Last activity:** 2026-04-18
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a user soft-deletes their gear, the `community_gear` join rows that link it into each community are left untouched — specifically, `archived` stays `false` and there is no soft-delete marker. The join row then keeps surfacing a dead `gear_id` to any client path that reads `community_gear` without also checking gear liveness, producing `"record not found"` 404s that page us via the `Server Error Logged` Cloud Monitoring alert (the triggering symptom was in #1184).

The same pattern exists for `DeleteRequest` → `community_request` and `DeleteExperience` → `community_experience`. Complete / Cancel / Unshare paths do archive these join rows; the Delete paths were simply missed.

Concrete numbers from `ripls-dev:gear-db-dev`:

```
dangling_live_community_gear_rows | distinct_deleted_gears_referenced
                              220 |                               220
```

220 join rows currently dangle against soft-deleted gear in dev alone. Prod almost certainly has an equivalent population (#1149 shows the same alert shape firing in prod).

## Context from Comments

The issue has one maintainer research comment that establishes three things:

1. **No semantic reason for the missing cascade.** Complete / Cancel / Unshare paths all archive Community\* join rows (`transfer/state_machine.go:325`, `request/lifecycle.go:285,443`, `request/sharing.go:202`, `experience/lifecycle.go:292,467`). Delete\* paths cascade to conversations and media only. The comment at `state_machine.go:316` — *"Archive all community gear relationships (preserve instead of deleting)"* — is the clearest statement of intent.

2. **"Just archive on delete" would leak deleted items into screens that assume `archived=true` means "lifecycle completed successfully."** `past_giveaways.go:50` queries `CommunityGear WHERE archived=true`, hardcodes `Availability = FOR_GIVEAWAY` in the response, then `GetByID`s the gear without `IncludeDeleted`. Reusing `archived` conflates two distinct states and reproduces the `record not found` ERROR on a different code path.

3. **Recommended approach (Option A in the comment):** add `optional DeletedMetadata deleted` to all three shareable-item join types. `protosql.buildDeletedFilter` already filters any proto with a `deleted` field automatically in `GetByID` / `QueryByField` / `QueryByFieldIn` / `QueryByFields`, so every existing caller gets the right behavior without being touched. Cascade-on-parent-delete sets `deleted`, not `archived`, keeping the two semantics cleanly separated.

The maintainer also scoped out `CommunityUser` (hard-deleted + events), `CommunityInvitationLink` (uses `is_revoked`), and `CommunityEvent` / `CommunityRegion` / `CommunitySkill` (reference rows, not parent-owned) — none need this treatment.

## Potential Solutions

### Option A: Add `optional DeletedMetadata deleted` to the three join types (recommended in comment)

- **Approach:** Add one proto field to each of `CommunityGear`, `CommunityRequest`, `CommunityExperience`. `Delete*` RPCs set `deleted` on every matching join row via a new `storage.CascadeDeleteCommunity*ByID` helper. `buildDeletedFilter` (which keys off the presence of a `deleted` field on the proto descriptor) automatically adds `AND (deleted_deleted_at_unix_sec = 0 OR IS NULL)` to all generic read paths. A one-shot backfill fixes existing dangling rows.
- **Pros:** Consistent with the rest of the schema (`Community`, `Gear`, `Request`, `Experience`, `ChatConversation`, `Media` all use `DeletedMetadata`). Zero-touch for the vast majority of callers — the filter attaches via reflection. Preserves the archived-vs-deleted semantic distinction, so `past_giveaways` and similar archived-aware screens keep meaning what they mean today. Proto3 `optional` field adds are wire-compatible.
- **Cons:** Three raw-SQL call sites need to be updated manually (they don't go through the generic readers): `IsGearInCommunity` in `protosql_search.go:321`, and the unified-search `extraWhereSQL` in `unifiedRequestConfig` / `unifiedExperienceConfig` / `requestSearchConfig`. Schema churn for what is otherwise a reflection-only change. Backfill touches 220+ rows (small).
- **Effort:** Medium.

### Option B: Hard delete the Community\* rows on parent delete

- **Approach:** `Delete*` RPC calls `storage.Delete` on every matching join row instead of soft-deleting.
- **Pros:** Simplest — no schema change. No dangling rows by construction. No backfill code needed (just a one-shot SQL `DELETE`).
- **Cons:** Diverges from the project's soft-delete-everywhere convention (every other parent-owned entity is soft-deleted, and chat conversations + media in the existing Delete cascade are soft-deleted). Anything analytical that reconstructs community history — feed stories, impact metrics, audit trails — loses signal irrecoverably the moment a user deletes their gear/request/experience. No way to undo an accidental delete.
- **Effort:** Small.

### Option C: Reuse `archived=true` on delete, tighten consuming queries

- **Approach:** Keep `archived` as the only lever. Delete cascade sets `archived=true`. Update every consumer that reads `archived` to also verify the gear/request/experience is not deleted — `past_giveaways`, `portfolio/fetch.go`, `feed/service.go`, `feed/feed_status.go`, several transfer queries, `chat/context.go`, etc.
- **Pros:** No schema change. Uses an existing field.
- **Cons:** Semantic collision — "completed" and "deleted" become indistinguishable at the join-row level, so *every* reader has to re-derive the distinction by joining back to the parent. Every current and future consumer of `community_gear` / `community_request` / `community_experience` becomes a potential bug site. Negative of Option A flipped on its head — instead of one filter that applies automatically, every reader has to remember.
- **Effort:** Medium-to-large because of the reader audit, but fragile and unbounded.

## Recommendation

**Option A.** The schema addition is tiny (one `optional DeletedMetadata` field per type), `buildDeletedFilter` does the hard work for free, and the archived-vs-deleted semantic distinction stays clean. The three raw-SQL sites that need manual updates are explicit, enumerable, and easy to test. Option B loses too much history for a soft-delete-everywhere codebase; Option C spreads the problem to every current and future reader.

## Open Questions

- **Should the Delete cascade set `deleted` on rows that are already `archived=true`?**
  Recommendation: **yes.** A gear that was fully given away (all `community_gear` rows archived) and then later deleted by its owner should stop surfacing in `past_giveaways` — the parent is gone. Setting `deleted` on already-archived rows is idempotent at the filter level (the row becomes "deleted" regardless of archived state) and fixes the adjacent `past_giveaways` → `GetByID` → 404 bug for free. Implication for the cascade helpers: they must use `QueryOptions{IncludeDeleted: true}` when fetching candidates (otherwise already-archived rows would be pulled via the normal generic path — they're not yet filtered by `buildDeletedFilter` pre-backfill — but `IncludeDeleted:true` is still the right contract, mirroring `CascadeDeleteMedia`).

- **Should the backfill archive-and-delete, delete only, or archive only?**
  Recommendation: **delete only** (set `deleted` to a backfill-marker `DeletedMetadata` with `deleted_by_user_id = parent.deleted_by_user_id` and `deleted_at_unix_sec = parent.deleted_at_unix_sec` when available, else `now()`). `archived` retains its "completed successfully" meaning; we should not retroactively flip it. `buildDeletedFilter` hides the row regardless.

- **Do we also need to update the raw-SQL unified-search `extraWhereSQL` to filter `j.deleted_deleted_at_unix_sec`?**
  Recommendation: **yes, for defense in depth.** `unifiedRequestConfig` and `unifiedExperienceConfig` already filter `e.deleted_deleted_at_unix_sec` on the parent entity, which catches the common case. `unifiedGearConfig`, `gearSearchConfig`, and `requestSearchConfig` don't filter parent-deleted at all today (pre-existing gap worth closing in the same pass). Adding `COALESCE(j.deleted_deleted_at_unix_sec, 0) = 0` everywhere makes the join table's soft-delete state authoritative and matches the behavior every other reader gets via `buildDeletedFilter`.

- **Should `IsGearInCommunity` (`protosql_search.go:321`) exclude deleted rows?**
  Recommendation: **yes.** Its callers use it to gate authorization / membership checks; a deleted row should not grant access. Add `AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)` to the existence check.

- **Should the backfill run as a startup goroutine (like `ImpactBackfillJob`) or as a one-shot admin RPC?**
  Recommendation: **startup goroutine**, mirroring `impact_backfill` (`server/main.go:699-704`). It's small (~hundreds of rows per environment), self-healing (idempotent — re-runs skip already-deleted rows), and matches existing project patterns. No operator action required to roll out.

- **Who does the backfill record as `deleted_by_user_id`?**
  Recommendation: copy the parent entity's `Deleted.DeletedByUserId` when available. That way the audit trail is preserved: `community_gear.deleted.deleted_by` == the user who deleted the gear. Falls back to empty string if the parent's field is missing.

- **Do we need a unit-test-level invariant ("no live community\_\* row references a deleted parent")?**
  Recommendation: **yes, one per type.** Cheap to write with `storage.AssertMaxQueries`-style helpers and catches regressions from any future Delete\* path that forgets the cascade.

- **Does the Flutter client need any changes?**
  Recommendation: **no.** `app/lib/data/repositories/gear_repository.dart:134` already invalidates the gear, feed, and search caches after `deleteGear`. The same pattern applies for request/experience delete paths. The 30-minute global TTL (`CACHE_TTL_MINUTES`) means any stale deleted-gear IDs already cached in client repositories will expire naturally once the server stops returning them in list responses. The client-side architecture (`docs/client/architecture.md`) requires all mutation paths to go through repositories, which they do.

- **Do we need new indexes on `deleted_deleted_at_unix_sec` for the three join tables?**
  Recommendation: **no.** The project's established pattern (`protosql_schema.go:385-388`) is that only explicitly hot query paths get explicit indexes — today, only `idx_story_community`. The three join tables have no explicit indexes today and the new filter predicate is a simple `IS NULL OR = 0` check that Postgres evaluates inline against sequential scans on small-to-medium tables. At current scale (~220 dangling rows, low thousands of live rows per table) this is inconsequential. Revisit if `docs/ai/sql_profile_*.md` profiling shows these queries getting expensive.

## Implementation Plan

### Phase 1: Proto + generated schema

**Goal:** Give the three join types a `deleted` field that `buildDeletedFilter` recognizes, without changing any runtime behavior yet.

**Tasks:**
- [ ] Add `optional DeletedMetadata deleted` to `CommunityGear` (field 8), `CommunityRequest` (field 7), `CommunityExperience` (field 7) in `proto/ripls/models/community.proto`. Proto3 `optional` adds are wire-compatible.
- [ ] Run `npm run generate` to regenerate Go code (not checked in per `CLAUDE.md`). The auto-migration path at `storage/protosql_schema.go:170` (`generateAlterTableAddColumnSQL`) adds `deleted_deleted_by_user_id TEXT` and `deleted_deleted_at_unix_sec BIGINT` columns on server startup — no manual migration is needed.
- [ ] Verify `npm run build` (which runs `golangci-lint` + `go build ./...`) and `npm run lint:proto` both pass.

**Validates:** Schema changes are wire-compatible and the SQL migration picks up the new columns on startup. No behavioral change observable yet at the RPC layer — `buildDeletedFilter` starts filtering these tables as soon as the proto gains the `deleted` field, but nothing writes to the new columns so it's effectively a no-op.

### Phase 2: Cascade helpers + wire into the three Delete\* RPCs

**Goal:** Make the cascade happen on new deletes. After this phase, no *new* dangling rows can be created.

**Tasks:**
- [ ] Add `CascadeDeleteCommunityGearByGearID(ctx, storage, gearID, deletedMetadata)` to `server/storage/cascade_delete.go`, mirroring `CascadeDeleteConversationsByTopic` (same file). Query by `gear_id` with `QueryOptions{IncludeDeleted: true}` so already-archived rows are still reached, skip rows whose `Deleted != nil`, set `Deleted` and `Update`. Use `logging.LoggerWithContext(ctx)` per `docs/server/architecture.md` observability guidance.
- [ ] Mirror for `CascadeDeleteCommunityRequestByRequestID` and `CascadeDeleteCommunityExperienceByExperienceID`. All three helpers ≤ ~40 lines; keeps `cascade_delete.go` well under the 1000-line target from `docs/server/architecture.md`.
- [ ] Wire the three helpers into `DeleteGear` (`gear/service.go:863-871`), `DeleteRequest` (`request/lifecycle.go:596-623`), and `DeleteExperience` (`experience/delete.go:62-70`), next to the existing conversation + media cascades.
- [ ] Cascade-helper unit tests in `server/storage/cascade_delete_test.go` using `storage.SetupTestStorage(t)` (testcontainers-backed, per `CLAUDE.md`). Table-driven test cases: live row cascades; already-deleted row skipped; missing parent id noop; mixed archived/unarchived set.
- [ ] Per-RPC unit tests in each service's existing `*_test.go`: after `DeleteGear` / `DeleteRequest` / `DeleteExperience`, assert that matching `Community*` rows end up with `Deleted != nil`. Query via `QueryByField(..., QueryOptions{IncludeDeleted: true})` to see soft-deleted rows.
- [ ] **Terminology note:** these are "integration tests" in the `docs/server/architecture.md` sense (real-DB RPC flow coverage via testcontainers). They are **not** behind the `integration` build tag — per `CLAUDE.md`, that tag is reserved for tests that hit live AI APIs.

**Validates:** New gear / request / experience deletions produce the expected cascade. Existing dangling rows are not yet cleaned up.

### Phase 3: Backfill existing dangling rows

**Goal:** Bring the existing 220+ dangling rows (across dev and prod) in line with the new invariant.

**Tasks:**
- [ ] Add `CommunityCascadeBackfillJob` in `server/jobs/community_cascade_backfill.go` — patterned after `ImpactBackfillJob` (`jobs/impact_backfill.go`). Iterate soft-deleted parents (gear / request / experience), query their matching `Community*` rows with `IncludeDeleted: true`, set `Deleted` using parent's `DeletedByUserId` and `DeletedAtUnixSec`, skip rows that already have `Deleted != nil` (idempotent re-run). Log progress with the same cadence as `gearBackfillLogInterval`.
- [ ] Wire into `main.go` as a startup goroutine alongside `ImpactBackfillJob` (`main.go:699-704`).
- [ ] Backfill unit test in `server/jobs/community_cascade_backfill_test.go` using `storage.SetupTestStorage(t)`: fixture with (deleted parent, live Community\* row) → Community\* row ends up soft-deleted with provenance from the parent. Second case: re-run is a noop.
- [ ] Invariant test in `storage` package: "no live Community\_\* row references a deleted parent" after backfill. This doubles as the regression guard for any future Delete\* path that forgets the cascade.

**Validates:** After one deploy, dev and prod stop serving dangling rows. The root cause of #1184 is gone.

### Phase 4: Raw-SQL call sites

**Goal:** Close the raw-SQL holes that bypass `buildDeletedFilter`. There are more than originally scoped — five search configs need attention, not three.

**Tasks:**
- [ ] Update `IsGearInCommunity` (`storage/protosql_search.go:321`) to add `AND (deleted_deleted_at_unix_sec = 0 OR deleted_deleted_at_unix_sec IS NULL)` to the `SELECT EXISTS` query.
- [ ] Update all five search configs in `storage/protosql_search.go` to add `AND COALESCE(j.deleted_deleted_at_unix_sec, 0) = 0` to `extraWhereSQL`:
  - `gearSearchConfig()` (line 40): currently has no `extraWhereSQL` at all — add both join-table deleted filter **and** `AND COALESCE(e.deleted_deleted_at_unix_sec, 0) = 0` on the parent entity (pre-existing gap worth closing in the same pass).
  - `requestSearchConfig()` (line 50): has `j.archived = false AND e.state = '1'` — add join-table deleted filter.
  - `unifiedGearConfig()` (line 61): no `extraWhereSQL` — add both filters as above.
  - `unifiedRequestConfig()` (line 74): has parent-deleted filter — add join-table deleted filter.
  - `unifiedExperienceConfig()` (line 90): has parent-deleted filter — add join-table deleted filter.
- [ ] Unit tests in `storage/protosql_search_test.go` (or equivalent existing search tests): fixture with a soft-deleted `Community*` row → search results exclude it. One case per config.
- [ ] Unit test: `IsGearInCommunity` returns `false` for a soft-deleted `community_gear` row.

**Validates:** Every reader — generic and raw-SQL — consistently filters soft-deleted join rows. Also closes a pre-existing parent-deleted-filter gap in `gearSearchConfig` and `unifiedGearConfig`.

### Phase 5: Observability + issue closeout

**Goal:** Prove the alert stops firing and close out the linked issues.

**Tasks:**
- [ ] After Phase 3 ships to dev, re-run the diagnostic query (`SELECT COUNT(*) FROM community_gear cg JOIN gear g ... WHERE NOT cg.archived AND g.deleted_deleted_at_unix_sec IS NOT NULL`) and the equivalents for request + experience. Expect zero. (Connection method from the earlier investigation: temporarily enable public IP, authorize current IP, disable both when done.)
- [ ] Monitor `Server Error Logged - server-dev` for a week. If the "failed to get gear / request / experience" volume drops to ~zero, close #1185. If not, investigate residual sources (different bug).
- [ ] Ship to prod; repeat the diagnostic query against the prod database instance.
- [ ] Close #1185; leave #1184 closed (WAI) and cross-reference #1149 for alert auto-filing.

**Validates:** The real-world signal matches the code-level invariant.

## Audit Notes (applied by `/ripls-audit` 2026-04-18)

Reviewed against `docs/server/architecture.md`, `docs/client/architecture.md`, `docs/client/caching.md`, and `docs/client/i18n.md`. Changes made in this pass:

- **Scope:** expanded Phase 4 from three to five search configs after confirming `gearSearchConfig` and `unifiedGearConfig` have no `extraWhereSQL` at all today (pre-existing parent-deleted gap, fixed while here).
- **Testing terminology:** clarified that all new tests live under regular `go test ./...` and use `storage.SetupTestStorage(t)` — they are *not* behind the `integration` build tag, which `CLAUDE.md` reserves for live-AI-API tests. Aligned with `docs/server/architecture.md`'s split between unit tests and real-DB-flow tests.
- **Storage layer boundary:** cascade helpers stay in `server/storage/cascade_delete.go` per `docs/server/architecture.md` Storage Best Practices ("Keep storage logic in storage package"). Services call them via the existing cascade pattern; no service-to-service dependency is introduced.
- **Observability:** called out `logging.LoggerWithContext(ctx)` as the logger pattern per `docs/server/architecture.md` Observability section; specified standard field names (`gear_id`, `request_id`, `experience_id`) are already available.
- **Client-side impact:** added explicit "no client changes needed" open question with evidence from `app/lib/data/repositories/gear_repository.dart:134`. Matches `docs/client/caching.md` repository-ownership-of-caching principle.
- **Indexing:** added explicit "no new index" open question, citing the project's existing sparse-indexing pattern in `protosql_schema.go:385-388`.
- **Migration mechanics:** replaced the vague reference to `protosql.ensureProtoSQLSchema` with the concrete path (`generateAlterTableAddColumnSQL` at `storage/protosql_schema.go:170`) and named the exact columns Postgres will auto-create.
- **File organization:** noted the ~40-line helpers leave `cascade_delete.go` well under the 1000-line target from `docs/server/architecture.md` File Organization section.
- **Tasks file-placement:** named exact test file locations (`server/storage/cascade_delete_test.go`, `server/jobs/community_cascade_backfill_test.go`) instead of vague "unit tests" lines, per the "file naming alongside implementation" guidance.

No contradictions with existing architecture decisions surfaced. No i18n impact (this plan touches server only, no user-visible strings).

## Related Issues

- #1184 — "Not-found lookups logged at ERROR, spamming Cloud Monitoring alerts" (closed WAI). Symptom of this bug; kept loud intentionally as signal for exactly this class of data-integrity issue.
- #1149 — "Auto-file GitHub issues for Cloud Monitoring alerts." Would have surfaced #1185 faster and is the right tool for catching future lifecycle cascade misses.
