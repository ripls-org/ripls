# Issue #2083: Avatar rule misses legacy users whose media_ids[] is empty but media_id is set

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2083
**Filed:** 2026-05-21 | **Last activity:** 2026-05-21
**Labels:** none | **Assignees:** unassigned
**Context docs:** [`docs/issues/2081-getmedia-telemetry-gaps.md`](./2081-getmedia-telemetry-gaps.md) — the parent telemetry work that surfaced this de-sync

## Audit log (post-`/ripls-audit`)

Audited against `docs/server/architecture.md` (Storage Layer + Proto-SQL Pattern), `docs/proto_conventions.md` (API/models separation, field deprecation), and `docs/client/caching.md` (repository pattern). Four changes made:

1. **Phase 2 backfill becomes a startup-init helper, not a manual SQL run.** Mirrors the existing pattern at `server/storage/postgresql.go:100-137` (e.g. `InitializeMediaIdsColumns`, `InitializeCommunityOwnerUserID`). One-shot manual SQL violates the architecture doc's "don't write raw SQL in services" rule and produces a per-environment human step instead of an idempotent startup gate.
2. **Backfill logging matches the existing pattern** in `server/storage/array_column_schema.go:94-141`: `operation`, `table_name`, `row_count` at INFO. Specified in Phase 2 tasks.
3. **Phase 4 defensive-read removal is now explicitly optional.** Belt-and-suspenders is fine — one indexed query on the deny path is cheap, and leaving it protects against any future de-sync regression that escapes Phase 2's writer fix.
4. **Phase ordering invariant called out.** The Phase 4 proto drop must follow Phase 2 backfill **in every environment** (dev, staging, prod). Otherwise stored `binary_proto` blobs with field 13 set would silently lose the only canonical avatar id on next deserialize/reserialize cycle. Added to Phase 4 preconditions.

No client-side concerns: the Dart code reads `user.mediaId` from `api.User` (the API-layer proto), which the plan correctly preserves through Phase 5. Repository pattern (`docs/client/caching.md`) isn't affected — the field's source doesn't change, just its internal population path on the server.

## Problem Statement

`User` carries the profile image in two storage fields:

- `User.media_id` (`proto/ripls/models/user.proto:62`) — singular string. The original avatar field.
- `User.media_ids[]` (`proto/ripls/models/user.proto:69`) — repeated string. Added later for an avatar history feature.

The proto comments at lines 60-69 declare an invariant: `media_id` is always synced with `media_ids[0]`. That invariant is **not** upheld:

- Prod query (2026-05-21): 5 of 12 users with an avatar have `media_id` set but `media_ids` NULL. 0 have non-NULL-but-out-of-sync arrays.
- Phase 1 telemetry from #2071 / #2081 caught one of these de-synced users as a real `would_deny` event — owner `e972c9e8-…` has `media_id = '33248a60-…'` and `media_ids = NULL`. The avatar rule in `server/services/media/authorization.go:65` only probes the array, so it missed.

This blocks **#1529 Phase 2 enforce flip** (the deferred phase in #2081). Flipping enforce today would 403 every avatar for those 5 users.

The user has chosen the long-term direction: **fix the invariant by consolidating onto `media_ids[]` and dropping `media_id`**, with a data backfill plus a code audit. This plan covers that path. The short-term defensive read (originally Option A in the issue) becomes Phase 1 of this plan — it lets us unblock Phase 2 enforce immediately while the deeper consolidation lands as Phases 2-4.

## Context from Comments

No comments on the issue yet; the direction was chosen in the planning conversation, not via comment thread. Key user statement: *"I would prefer just fixing the invariant. We can do a data migration, simple SQL call, then get rid of the single field. We will have to audit the code to see how involved this will be."*

## Codebase Audit Findings

A read-only audit (Explore agent, 2026-05-21) measured the consolidation surface:

| Category | Go | Dart |
|---|---|---|
| READ singular `User.media_id` | ~33 sites | 17 sites |
| WRITE singular `User.media_id` | 2 sites (1 storage, 1 API-response) | 0 sites |
| READ `User.media_ids[]` | ~18 sites | 1 site |
| WRITE `User.media_ids[]` | 4 sites (all explicitly synced) | 0 sites |

Key load-bearing observations:

1. **The de-sync source is a single Go write path.** `server/services/login/registration.go:235` sets `user.MediaId = mediaID` during OIDC registration but does NOT append to `media_ids[]`. Every other production writer of `media_id` is paired with a corresponding `media_ids[]` write (e.g. `server/services/user/service.go:128-133` SaveUser explicitly syncs both directions).
2. **Most singular-field READs are response builders**, not load-bearing logic. Code like `server/services/user_util.go:21` ToAPIUser converter, `server/services/login/helpers.go:130` toAPIUser, and dozens of impact / workshop / feed / search builders read `user.MediaId` only to assemble an API response. Switching them to `media_ids[0]` is mechanical.
3. **`media_ids[]` is already the read path that matters.** The recently-shipped avatar auth rule (`server/services/media/authorization.go:65`) probes only the array. Impact backfill and storage denormalization both prefer the array. The array is already the source of truth for new code.
4. **The API surface is independent.** `proto/ripls/api/` carries its own `media_id` + `media_ids` fields on three message types (`GetUserResponse`, `SaveUserRequest`, `api.User`). Dropping the *storage* field doesn't require dropping the *API* field — the API can keep both for one or two more revisions while clients catch up, transposing from `media_ids[0]` server-side.
5. **The Flutter client reads only `user.mediaId`** in 17 places (every avatar widget) and `user.mediaIds` in one place (the workshop category detail screen). After the API layer transposes, the client can switch to `mediaIds[0]` at its own pace.

## Potential Solutions

### Option A — Defensive read only (rejected: long-term direction)

Already documented as Option A in the issue body. Adds a fallback `QueryByField("media_id", ...)` probe to the avatar rule. Smallest change, ships in a day, but leaves the proto smell in place forever.

**Rejected:** the user chose the longer-term cleanup. Option A becomes Phase 1 of this plan — it ships first as a stop-gap to unblock Phase 2 enforce of #1529, but it's not the destination.

### Option B — Backfill + drop singular field (chosen direction)

Land in four phases (detailed below):
1. **Defensive read first** so the auth check is correct regardless of data state.
2. **Backfill** `media_ids[]` from `media_id` for every desynced row.
3. **Audit + fix the writer** that produced the de-sync, then sweep all readers from `media_id` to `media_ids[0]`.
4. **Drop the field** from the storage proto (reserve the tag) once no code reads it.

**Pros:** Removes the smell. Single source of truth (`media_ids[]`). The API layer remains backwards-compatible by transposing internally.
**Cons:** Larger surface than just shipping Option A. Real number of touch sites is ~50 Go reads + 17 Dart reads, mostly mechanical. Requires holding the line on Phase 4 — incomplete consolidations are worse than no consolidation.
**Effort:** medium.

### Option C — Drop `media_ids[]`, keep singular field

Inverse of B — if the multi-avatar history feature was never shipped to users and is dead weight, reverting to single-avatar semantics is the smaller change.

**Rejected:** the avatar auth rule (`server/services/media/authorization.go:65`) and the storage denormalization (`server/storage/media_ids_schema.go`) both already use the array. Removing it would require backing out work that just landed. The user's preference also explicitly mentions keeping the array.

## Recommendation

**Option B.** Already chosen by the user. The plan below stages it so each phase is shippable on its own.

## Open Questions

- **What's the right wire-compat story for proto field 13?** — Recommendation: mark it `reserved 13;` after the storage column is dropped. Old stored protos with field 13 set will silently lose the value on next deserialize-then-reserialize, which is fine since the data is also in `media_ids[]` after the backfill. Don't reuse the tag — protobuf's tag numbering is forever even on internal storage protos.
- **Should we drop the `media_id` column from the SQL schema at the same time as the proto field?** — Recommendation: yes, in the same migration. The column is a flat denormalization of the proto field (`server/storage/protosql_schema.go:109+` generates it from the top-level scalar). Once the proto field is gone, the column has no source and stays as a NOT-NULL ghost. Drop both together.
- **Does the API layer need to drop `media_id` too?** — Recommendation: not yet. Keep `GetUserResponse.media_id`, `SaveUserRequest.media_id`, and `api.User.media_id` for at least the next app release. Server-side, populate them by reading `media_ids[0]` (or empty string when `media_ids` is empty). Clients that read these fields keep working unchanged. Schedule the API-side drop as a separate later issue tied to a minimum-app-version bump.
- **Will dropping the singular field cause issues with the binary_proto blobs already stored?** — Recommendation: no. proto3 deserialization tolerates unknown fields. Stored blobs with field 13 set will deserialize cleanly (field 13's value will land in `XXX_unrecognized`), and on next save (Phase 2 backfill or any Update) the blob is rewritten without field 13. The desynced 5 users' avatars stay reachable via the array after backfill.
- **Is there a Dart-side proto change required?** — Recommendation: not immediately. The Dart client reads `User.mediaId` from `api.User` (the API proto), not the storage proto. As long as the API field stays around, the client doesn't have to change. When the API field is later removed, the client switches to `user.mediaIds.first`.

## Implementation Plan

### Phase 1: Defensive read in avatar rule (unblocks #1529 Phase 2)

**Goal:** Make `canUserAccessMedia` correct for the de-synced rows, so Phase 2 enforce of #1529 can ship without 403'ing five users.

**Tasks:**
- [ ] Add a fallback `QueryByField(ctx, "media_id", media.Id, &models.User{})` lookup in `server/services/media/authorization.go` after the existing `media_ids[]` array probe at line 65. Returns `accessReasonAvatar` on hit, falls through otherwise.
- [ ] New test in `authorization_test.go`: `TestCanUserAccessMedia_AvatarViaSingularMediaId` — User row with `media_id` set, `media_ids` NULL, asserts `(true, accessReasonAvatar, nil)`.
- [ ] Update the existing `TestCanUserAccessMedia_Avatar` to make clear it covers the array case specifically.

**Validates:** The avatar rule returns allow for any user row that has the media id in *either* field. Soak on dev + prod; verify the prod de-sync user's reads no longer fire `would_deny`.

### Phase 2: Backfill data + close the writer hole

**Goal:** Restore the invariant in stored data so future code can safely assume `media_ids[]` is the source of truth.

**Tasks:**
- [ ] Fix `server/services/login/registration.go:235` — when setting `user.MediaId = mediaID` during OIDC registration, also set `user.MediaIds = []string{mediaID}` (or append if non-empty). Add a unit test that asserts both fields are populated.
- [ ] Implement the backfill as a **startup-init helper** in `server/storage/`, following the existing pattern (e.g. `InitializeCommunityOwnerUserID` at `community_schema.go`, `InitializeMediaIdsColumns` at `media_ids_schema.go`). Suggested shape: a new file `server/storage/user_media_ids_schema.go` with:
  ```go
  // InitializeUserMediaIdsBackfill idempotently copies media_id into
  // media_ids[] for legacy User rows that predate the multi-avatar
  // invariant. Becomes a no-op once every row is consistent. Required
  // before #2083 Phase 4 drops the singular media_id field — without
  // the backfill, those rows would lose their canonical avatar.
  func (s *ProtoSQLStorage) InitializeUserMediaIdsBackfill(ctx context.Context) error { ... }
  ```
  The query inside is the same NULL-safe SQL from #2083:
  ```sql
  UPDATE "user"
  SET media_ids = ARRAY[media_id]
  WHERE media_id <> ''
    AND (media_ids IS NULL OR NOT (media_id = ANY(media_ids)));
  ```
- [ ] Wire the helper into `InitializePostgreSQLDatabase` in `server/storage/postgresql.go` after the existing `Initialize*` block (around line 137). Idempotent — every subsequent startup runs the same UPDATE and updates zero rows.
- [ ] Logging: match `server/storage/array_column_schema.go:94-141` shape — log at INFO with `operation: "InitializeUserMediaIdsBackfill"`, `table_name: "user"`, `row_count: <rows updated>`. First-deploy log carries the real number; subsequent deploys log `row_count: 0`.
- [ ] Post-deploy verification query (run manually once after each environment's first restart):
  ```sql
  SELECT COUNT(*) FILTER (
    WHERE media_id <> ''
      AND (media_ids IS NULL OR NOT (media_id = ANY(media_ids)))
  ) AS still_desynced
  FROM "user";
  ```
  Expect `still_desynced = 0` on every environment.
- [ ] **The Phase 1 defensive read stays in place** — it's no harm now that the data is consistent, and Phase 4 explicitly leaves it as a belt-and-suspenders option.

**Validates:** Every user has `media_ids[]` populated. Phase 1 telemetry no longer fires avatar denies on legacy rows. Backfill is idempotent and re-runs on every startup as a guard.

### Phase 3: Sweep readers from `media_id` → `media_ids[0]`

**Goal:** Move every storage-side reader off `User.MediaId` so the singular field has no callers.

**Tasks:**
- [ ] Sweep the ~33 Go reads identified by the audit. Each read pattern looks roughly like `mediaID := user.MediaId; if mediaID != "" { ... }`; replace with `var mediaID string; if len(user.MediaIds) > 0 { mediaID = user.MediaIds[0] }`. Consider a tiny helper `primaryAvatarMediaID(*models.User) string` so the empty-array fallback is uniform.
- [ ] At the API boundary, response builders that write to `api.User.media_id` keep doing so but populate from `user.MediaIds` (transposing). Don't drop the *API* field yet — that's a separate later issue.
- [ ] Run `npm run build` to catch any remaining `MediaId` references in server code outside generated.
- [ ] Sweep the 17 Dart reads after the API field still works (no Dart change required since we kept the API field; this sub-task is a no-op until the API drop ships).

**Validates:** Server code only reads `User.MediaIds` from the storage proto. Any direct `user.MediaId` reference outside `server/storage/` or generated code becomes a build failure (enforced by grep in CI optional).

### Phase 4: Drop the storage proto field

**Goal:** Remove `User.media_id` from the storage proto and the corresponding flat column from the SQL schema.

**Preconditions (hard):**
- Phase 2 backfill must have run on **every** environment (dev, staging, prod). Run the post-deploy verification query in each and confirm `still_desynced = 0` before merging Phase 4. Dropping field 13 before backfill completes would silently delete the only canonical avatar id for those 5+ rows on next deserialize/reserialize.
- Phase 3 sweep must have landed so no Go reader still touches `user.MediaId` (verified by `npm run build` — generated code will compile only if all readers are gone).

**Tasks:**
- [ ] Edit `proto/ripls/models/user.proto`: replace `string media_id = 13;` with `reserved 13;` and update the comment on `media_ids[]` to drop the "synced with media_id" language. Do not reuse tag 13 — protobuf tag numbering is forever, even on internal storage protos.
- [ ] Run `npm run generate` and `npm run build`. Any remaining compile errors are the last places that still touch `user.MediaId`. After Phase 3, this should be only the registration writer (already fixed in Phase 2 to populate `media_ids`) and the API response builders (already migrated in Phase 3 to read `media_ids[0]`).
- [ ] SQL migration: implement as a startup-init helper (matching Phase 2's pattern) — new function `DropUserMediaIdColumn` in `server/storage/user_media_ids_schema.go` (same file as the backfill, since they're paired migrations). Run-once idempotent shape:
  ```sql
  ALTER TABLE "user" DROP COLUMN IF EXISTS media_id;
  ```
  Pair with the proto change in the same PR. The `protosql_schema.go` reconciler adds missing columns but does not drop excess ones, so leaving the migration out would leave `media_id` as inert dead space in the schema. Drop it explicitly to keep the schema clean.
- [ ] **Optional:** remove the Phase 1 defensive read from `authorization.go`. The array probe alone suffices once the data is consistent and `media_id` no longer exists. Keeping the defensive read as belt-and-suspenders is also fine — its only cost is one indexed query on the deny path of the avatar rule, and it protects against any future de-sync escape. Decide at PR time based on how strict you want to keep the auth code surface.

**Validates:** `User` proto no longer carries `media_id`. The SQL schema matches. Server build green. Phase 1 telemetry still shows zero avatar denies.

### Phase 5 (deferred — separate issue): Drop API-layer `media_id`

**Goal:** Remove the singular field from the API protos once clients have caught up.

Not in scope of this issue. File as a follow-up once the next mobile release with `media_ids`-aware client code is in user hands for long enough. Likely 3-6 months after Phase 4 lands.

## Related Issues

- #1529 — Per-resource auth check (the parent that surfaced this).
- #2071 — Phase 1 telemetry implementation; the WARN log that caught the de-sync.
- #2081 — Telemetry fixes; this issue is exactly the kind of legitimate deny those fixes made debuggable.
- #2082 — Deny taxonomy + system rule (the work that's currently soaking on prod).
