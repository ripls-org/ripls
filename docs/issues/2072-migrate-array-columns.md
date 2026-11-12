# Issue #2072: Migrate existing TEXT[] denormalizations to the generic RegisterArrayColumn pattern

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2072
**Filed:** 2026-05-20 | **Last activity:** 2026-05-20
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** [`docs/issues/1529-getmedia-auth.md`](./1529-getmedia-auth.md) — design of the generic pattern this issue retrofits onto.

## Problem Statement

PR #2071 (#1529 Phase 1) introduced a generic `RegisterArrayColumn` / `InitializeArrayColumn` helper on `ProtoSQLStorage` that denormalizes repeated proto fields into queryable `TEXT[]` columns with auto-sync via `WithTx` on every `Insert` / `Update`. The new pattern is **transactional** (atomic with the row write) and **caller-transparent** (no existing call sites change).

Three TEXT[] denormalizations in `server/storage/` predate that helper, each implemented bespoke with ~80 lines of duplicated schema-init code plus per-column write logic. All three are explicitly **non-transactional** — the existing comments call this out — and rely on the startup backfill to recover from drift on partial failure.

Retrofitting them onto the generic pattern is a strict upgrade: less code, transactional writes, one consistent codepath across the codebase.

## Context from Comments

Issue has no comments yet. The body itself authored the migration plan in broad strokes; this doc fleshes it out with file:line refs and per-column subtleties surfaced during codebase research.

## Codebase Findings

### Pre-existing TEXT[] columns

**1. `chat_conversation.participant_ids`** — `ChatConversation.participant_ids` (top-level repeated string).
- Schema init: `server/storage/chat_conversation_schema.go:14-92`.
- Typed storage wrapper: `server/storage/chat_conversation.go:21-136` with bespoke `Insert` (line 35) and `Update` (line 61) doing non-transactional two-phase writes.
- Read methods on the wrapper: `GetByID` (line 84), `ListByParticipant` (line 96 — uses `WHERE $1 = ANY(participant_ids)`).
- Many callers via `chatConvStorage.Insert` / `.Update`:
  - `server/chat/conversations.go` (4 sites — lines 44, 100, 200, 237)
  - `server/jobs/community_conversation_backfill.go` (line 19 — service-level dep)
  - `server/services/experience/share.go:72`
  - `server/services/experience/rsvp.go:174`
  - `server/services/chat/service.go:23` (the chat service holds a `ChatConversationStorage`)
- Test callers: `server/storage/chat_conversation_test.go` (6 tests), `server/chat/conversations_test.go` (2 tests), `server/services/chat/unread_counts_test.go` (1 test).

**2. `Story.participant_ids`** — `Story.participant_ids` (top-level repeated string).
- Schema init: `server/storage/story_schema.go:13-150`.
- Typed storage wrapper: `server/storage/story.go:13-229` with bespoke `Insert` (line 24) doing non-transactional two-phase write.
- Read methods on the wrapper: `GetByID`, `ListByCommunity`, `ListByUser` (line 98 — uses `WHERE $1 = ANY(participant_ids)`), `ListByItem`, `Exists`.
- Callers: `server/main.go:902` constructs via `NewStoryStorage(sqlStorage)`; story-service consumers go through that.
- Test callers: `server/storage/story_test.go` (many tests).

**3. `community.snapshot_member_user_ids`** — `Community.deleted_snapshot.member_user_ids` (**nested** field on an optional sub-message).
- Schema init: `server/storage/community_schema.go:290-417`.
- Backfill is gated on `WHERE deleted_deleted_at_unix_sec > 0` — only soft-deleted rows have a snapshot. Live rows leave the column NULL today.
- No typed storage wrapper. Writes happen inline in two places:
  - **Soft-delete path:** `server/storage/community.go:640` — raw `UPDATE … SET snapshot_member_user_ids = %s` after the proto Update. Lifecycle code sets `community.DeletedSnapshot.MemberUserIds` in `server/services/community/lifecycle.go:241`.
  - **Restore path:** `server/storage/community.go:198` — raw `UPDATE … SET … snapshot_member_user_ids = NULL` plus other deleted-state column clears. **This is intentionally atomic** (the WHERE clause has `AND deleted_deleted_by_user_id IS NOT NULL` for restore-only-if-deleted semantics) — switching it to `storage.Update(community)` would lose that atomicity.

### Per-column migration shape

| Column | Schema-init replacement | Write-path change | Notes |
|---|---|---|---|
| `chat_conversation.participant_ids` | `InitializeArrayColumn(..., TopLevelRepeatedString(&ChatConversation{}, "participant_ids"))` + `RegisterArrayColumn(...)` | Delete bespoke `UPDATE participant_ids` inside `ChatConversationStorage.Insert` / `.Update` | Typed wrapper stays — read methods are still useful. Wrapper's `Insert` becomes a 2-line shim around `s.storage.Insert`. |
| `Story.participant_ids` | Same shape | Delete bespoke `UPDATE participant_ids` inside `StoryStorage.Insert` | Typed wrapper stays — `ListByCommunity`, `ListByUser`, `ListByItem`, `Exists` are still useful. |
| `community.snapshot_member_user_ids` | `InitializeArrayColumn(..., <nested extractor>)` + `RegisterArrayColumn(...)` | Delete bespoke `UPDATE snapshot_member_user_ids = ...` at `community.go:640`. **Keep** the restore-path `... = NULL` at `community.go:198` for atomicity. | Nested-field extractor needed; mirror `chatMessageMediaIds` in `media_ids_schema.go`. |

### One subtle semantics change to flag

Today, `community.snapshot_member_user_ids` is **only written during soft-delete and restore** — live rows have it permanently NULL. After migration, **every** Community Insert / Update will run an auto-sync UPDATE on the column (extractor returns nil for live rows, so the column ends up NULL — same end state, but with one extra UPDATE per write).

Same for `chat_conversation.participant_ids` and `Story.participant_ids`: today the column is updated only by the bespoke two-phase write paths; after migration every `storage.Insert` / `Update` of those types runs an auto-sync UPDATE. End-state is identical; the cost is one constant-time additional SQL statement per write. Acceptable given the strict-upgrade benefits (transactional, less code).

This is the same trade-off PR #2071 already accepted for `media_ids` on seven other tables.

## Potential Solutions

### Option A: Three commits in one stacked PR — recommended

- **Approach:** One PR (stacked on `1529-getmedia-auth-phase1` until #2071 merges), three commits — one per column. Each commit is small, mechanical, and reviewable on its own.
- **Pros:** Single coordination point. Each commit isolates risk to one column. CI runs against the whole stack so we catch cross-column regressions immediately.
- **Cons:** Bigger PR overall (~400-600 LOC of diffs). Reviewer commits to all three at once.
- **Effort:** Medium total, small per commit.

### Option B: Three separate sequential PRs

- **Approach:** PR per column, each stacked on the prior. Phase 1 (community-snapshot) merges first, then Phase 2 (Story), then Phase 3 (chat_conversation).
- **Pros:** Each PR is tiny and trivially revertable. Each merge validates the pattern before touching the next.
- **Cons:** Three rounds of CI + review overhead. Stacked-PR coordination cost. The migrations are mechanical and similar enough that splitting feels like ceremony.
- **Effort:** Same total work; more elapsed time.

### Option C: Defer chat_conversation; ship community-snapshot + Story only

- **Approach:** Phase 1 + Phase 2 in one PR; file Phase 3 (chat_conversation) as a separate follow-up to land later. Rationale: chat is the highest-throughput write path, biggest call-site audit, and worth taking more carefully.
- **Pros:** De-risks the riskiest column. Validates the retrofit pattern on two simpler columns before touching chat.
- **Cons:** Leaves the goal half-done. The chat_conversation migration is still in scope for this issue; deferring just splits the work without much risk reduction since Phase 1 + Phase 2 are already low-risk.
- **Effort:** Less in this PR, more later.

## Recommendation

**Option A.** Three commits in one PR, stacked on `1529-getmedia-auth-phase1`. The migrations are mechanical and similar enough that bundling makes the reviewer's job easier (they read the pattern once and apply it three times mentally), not harder. Splitting into three PRs adds coordination overhead without changing the risk profile — each migration is internally bounded to one column anyway.

Retarget the PR base to `main` once #2071 merges (per `feedback_stacked_pr_merge.md` in user memory).

## Open Questions

- **Q1: Should the typed wrappers (`ChatConversationStorage`, `StoryStorage`) be deleted, or kept with the bespoke sync removed?**
  — **Recommendation:** keep them. They carry read methods (`ListByParticipant`, `ListByUser`, `ListByCommunity`, `ListByItem`, `Exists`) that are genuinely useful and aren't obsoleted by the generic pattern. After the migration, the `Insert` and `Update` methods become 2-line shims around `s.storage.Insert`/`Update`. Callers don't change.

- **Q2: Does the community-snapshot restore path's atomicity need to migrate too?**
  — **Recommendation:** no. The restore path at `community.go:198` uses a single raw `UPDATE … WHERE … AND deleted_deleted_by_user_id IS NOT NULL` for restore-only-if-deleted semantics. That atomicity is correct and unrelated to the array-column pattern. Migrating just removes the redundant `snapshot_member_user_ids = ...` clause from the bespoke soft-delete path (line 640); the restore SQL keeps its explicit `... = NULL` for atomicity with the rest of the restore. Documented in the per-column shape table above.

- **Q3: Will existing tests still pass without modification?**
  — **Recommendation:** yes for the storage and service tests. The end state of the column after Insert / Update is identical (the auto-sync produces the same UPDATE as the bespoke code did). Test fixtures that call `chatConvStorage.Insert(...)` or `storyStorage.Insert(...)` continue to work because we keep the typed wrappers and the methods still return `(id, error)`. The only test surface that might need touching is the per-column schema-init tests in `server/storage/*_schema_test.go` if they assert specifics of the bespoke `Initialize*` helper — verify in implementation.

- **Q4: What about the existing `backfill*` helpers (`backfillStoryParticipantIds`, etc.)?**
  — **Recommendation:** delete them along with the bespoke `Initialize*` helpers. `InitializeArrayColumn`'s generic `backfillArrayColumn` (`array_column_schema.go:91`) subsumes them. The generic backfill reads every row and calls the extractor; for community-snapshot the extractor returns nil for live rows (no DeletedSnapshot), so live rows end up with NULL — same end state as the current `WHERE deleted_deleted_at_unix_sec > 0` filter, just without the where clause. End state equivalent; one extra row-read at startup per live community. Modest.

- **Q5: Does deleting `Initialize*` helpers break anything besides `postgresql.go`?**
  — **Recommendation:** verified via grep — only `postgresql.go` calls these. No tests call them directly. Safe to delete.

## Implementation Plan

All three commits stacked on `1529-getmedia-auth-phase1`. Retarget to `main` once #2071 merges (`feedback_stacked_pr_merge.md`).

### Phase 1: Migrate `community.snapshot_member_user_ids`

**Goal:** Smallest blast radius — only soft-delete and restore paths touch this column. Validates the nested-extractor case on a real retrofit.

**Tasks:**
- [ ] Add a `communitySnapshotMembers` extractor to `server/storage/community_schema.go` (or a new `community_array_columns.go`) — mirrors `chatMessageMediaIds` in `media_ids_schema.go`. Body: `return msg.(*models.Community).GetDeletedSnapshot().GetMemberUserIds()`.
- [ ] Replace the `InitializeCommunitySnapshotMembersColumn` call in `server/storage/postgresql.go:129` with `s.InitializeArrayColumn(ctx, &models.Community{}, "snapshot_member_user_ids", communitySnapshotMembers)`.
- [ ] Add `s.RegisterArrayColumn(&models.Community{}, "snapshot_member_user_ids", communitySnapshotMembers)` alongside.
- [ ] Delete `InitializeCommunitySnapshotMembersColumn` and `backfillCommunitySnapshotMembers` from `community_schema.go:290-417`.
- [ ] Delete the redundant `UPDATE … SET snapshot_member_user_ids = ...` at `community.go:640` — auto-sync now handles it.
- [ ] Leave the restore-path SQL at `community.go:198` as-is (keeps atomic restore-only-if-deleted; the explicit `... = NULL` is the snapshot clear for that atomic transaction).
- [ ] `go test ./server/storage/... ./server/services/community/...` — full pass.

**Validates:** Nested-extractor path works on a real retrofit. Soft-delete + restore semantics unchanged.

### Phase 2: Migrate `Story.participant_ids`

**Goal:** Moderate write surface — every story create writes this. Validates the typed-wrapper retrofit shape.

**Tasks:**
- [ ] Replace `InitializeStoryParticipantColumn` call in `postgresql.go:99` with `s.InitializeArrayColumn(ctx, &models.Story{}, "participant_ids", TopLevelRepeatedString(&models.Story{}, "participant_ids"))`.
- [ ] Add matching `RegisterArrayColumn` call.
- [ ] Delete `InitializeStoryParticipantColumn` and `backfillStoryParticipantIds` from `story_schema.go:13-150`. (Whole file goes away unless something else lives in it.)
- [ ] In `server/storage/story.go:24-43`: delete the bespoke `UPDATE participant_ids` block from `StoryStorage.Insert`; the method becomes `return ss.storage.Insert(ctx, story)`.
- [ ] If `StoryStorage` has any `Update` method that does similar bespoke sync, prune that too (research showed only `Insert` does this today).
- [ ] `go test ./server/storage/...` — full pass. Tests that call `storyStorage.Insert(story)` still work.

**Validates:** Typed-wrapper retrofit pattern: keep the struct + read methods, strip the bespoke write-side sync.

### Phase 3: Migrate `chat_conversation.participant_ids`

**Goal:** Highest write surface — many call sites across the chat service. Save for last so the pattern is well-exercised before touching this hot path.

**Tasks:**
- [ ] Replace `InitializeChatConversationParticipantColumn` call in `postgresql.go:106` with `s.InitializeArrayColumn(ctx, &models.ChatConversation{}, "participant_ids", TopLevelRepeatedString(&models.ChatConversation{}, "participant_ids"))`.
- [ ] Add matching `RegisterArrayColumn` call.
- [ ] Delete `InitializeChatConversationParticipantColumn` and `backfillChatConversationParticipantIds` from `chat_conversation_schema.go:14-152`. Whole file goes away.
- [ ] In `server/storage/chat_conversation.go`:
  - Delete the bespoke `UPDATE participant_ids` block from `Insert` (line 35-55). Method becomes `return cs.storage.Insert(ctx, conversation)`.
  - Delete the bespoke `UPDATE participant_ids` block from `Update` (line 61-81). Method becomes `return cs.storage.Update(ctx, conversation)`.
  - Keep `GetByID`, `ListByParticipant`, and the struct itself — they're still useful.
- [ ] Audit callers (no API change needed — wrapper signature stays the same):
  - `server/chat/conversations.go` (4 callers) — should be unchanged.
  - `server/services/chat/service.go` — unchanged.
  - `server/services/experience/share.go:72` — unchanged.
  - `server/services/experience/rsvp.go:174` — unchanged.
  - `server/jobs/community_conversation_backfill.go` — unchanged.
- [ ] `go test ./server/...` — full pass. Tests in `chat_conversation_test.go`, `conversations_test.go`, `unread_counts_test.go` keep working because the wrapper's API didn't change.

**Validates:** Hottest write path retrofitted without touching call sites. Confirms the retrofit pattern is generally safe.

### Phase 4: Build + ship

**Tasks:**
- [ ] `npm run build` — lint clean (`golangci-lint`, `check_codeinternal`, `check_goroutines`, `gofumpt`).
- [ ] `go test ./server/...` — full server suite.
- [ ] PR description summarizes the three-commit retrofit; cite #2071 as the pattern source; cite this plan doc.
- [ ] Stacked on `1529-getmedia-auth-phase1`; retarget to `main` once #2071 merges.

**Validates:** Same shape as #2071 ship. CI green, reviewer can land independently.

## Out of Scope

- **Adding new TEXT[] denormalizations.** This issue is strictly a retrofit of existing columns. New columns (e.g., follower-id sets, watchlist members) would use `RegisterArrayColumn` directly without going through this migration.
- **Refactoring the read methods on `ChatConversationStorage` / `StoryStorage`.** `ListByParticipant`, `ListByUser`, etc. are tangentially related (they read the column we're maintaining) but their query shapes are correct and stay as-is.
- **Restore-path atomicity changes for community-snapshot.** The atomic restore at `community.go:198` keeps its explicit `snapshot_member_user_ids = NULL` clear; we don't move it to a separate auto-sync write.

## Related Issues / Context

- **PR #2071** — Introduces the `RegisterArrayColumn` pattern this issue retrofits onto. Reviewable as the reference implementation.
- **#1529** — The security issue that drove #2071 and surfaced the duplicated pattern in three pre-existing columns.
- **#1619** — Community soft-delete work; `community.snapshot_member_user_ids` was introduced as part of that umbrella (for #1717 / #1722 — the restorable-deleted-communities list).

---

## Audit notes (added by `/ripls-audit` on 2026-05-20)

Reviewed against `docs/server/architecture.md` and `docs/server/observability.md`. Client docs (`docs/client/*.md`) not consulted — server-only retrofit.

Changes applied to the plan:

- **Storage-boundary preservation called out explicitly.** Per `docs/server/architecture.md` § "Storage Best Practices": *"Don't write raw SQL in services. Add new storage methods for complex queries."* The plan keeps the typed `ChatConversationStorage` / `StoryStorage` wrappers — service-layer callers continue going through those, never touching raw SQL. The community-snapshot case has no typed wrapper but its raw SQL already lives in `server/storage/community.go`, on the right side of the boundary. The retrofit doesn't move any raw SQL toward the service layer; it just trades bespoke per-column wrappers for the generic one. Compliant.

- **Observability shape — slight regression to flag.** Today, the bespoke wrappers log `operation=ChatConversationStorage.Insert` on the column-sync error path (`chat_conversation.go:49-51`). After migration, the auto-sync's error bubbles up wrapped as `"sync array columns on insert: %w"` from inside `ProtoSQLStorage.Insert`. The wrapped error is preserved; the structured `operation` field on the failure log changes from `ChatConversationStorage.Insert` to whatever the caller had set. Net: slightly less specific operation tag on sync failures. This is acceptable — the failure is rare (atomic tx; column sync rarely fails on its own) and the wrapped error retains the original cause. If telemetry later needs a finer breakdown, `syncArrayColumns` can attach a `column` field to a log line; file as follow-up if it matters.

- **Phase markers banned from source comments.** Per CLAUDE.md § "TODO Comments" / `scripts/check_ephemeral_comments.js`: `*.go` and `*.dart` files cannot contain `\bphase\s*\d+\b`. Phase numbers in this plan and the PR description are fine; source comments must NOT say "Phase 1 of #2072" or similar. Stable references like `#2072` itself are allowed and useful. Each commit message should reference the issue (`#2072`) without phase markers.

- **No new tests required.** Existing tests in `chat_conversation_test.go`, `story_test.go`, `conversations_test.go`, `unread_counts_test.go` already exercise the typed wrappers' `Insert` / `Update`. After migration, those tests continue to pass because the wrapper API doesn't change and the end-state of the indexed column is identical (auto-sync produces the same UPDATE the bespoke code did). The generic `RegisterArrayColumn` / `InitializeArrayColumn` / `QueryByArrayContains` already have unit tests in `server/storage/array_columns_test.go` (added in #2071). Adding per-retrofit tests would be redundant — the test coverage tracks the wrapper API, which is unchanged.

- **No proto changes needed.** Pure code-and-schema retrofit; no `.proto` files touched. Skips the entire proto-comments / proto-conventions audit lane.

- **File-size guidelines improved.** Deleting `chat_conversation_schema.go` (152 lines), `story_schema.go` (150 lines), and the `community_schema.go:290-417` snippet (~125 lines) net-removes ~400 lines from `server/storage/`. Every retrofit makes the file inventory smaller.

- **No `connecterr.Internal` involved.** Storage-layer errors stay as wrapped `error` values; service-layer code already converts to `connecterr.Internal` when surfacing to RPCs. `check_codeinternal.js` gate doesn't apply to this PR.

**No changes needed in other areas.** Goroutine safety (`logging.GoSafe`) untouched. Goroutine-spawning code paths (none in this retrofit). Logging conventions (`LoggerWithContext`, masked fields, level mapping) — the auto-sync in `Insert`/`Update` doesn't add new log lines, and the deleted bespoke helpers had their logging absorbed by the generic helper's `operation=InitializeArrayColumn` / `operation=backfillArrayColumn` lines (already shipped in #2071).
