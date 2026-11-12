# Issue #1643: Phase 1 of #1619 — proto + schema additions for community soft-delete

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1643
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** briangbrown
**Context docs:** docs/community_delete_and_leave.md (read in full)

## Audit Notes (2026-05-01)

`/ripls-audit` checked this plan against `docs/server/architecture.md`,
`docs/server/observability.md`, and `docs/proto_conventions.md`. Findings,
all incorporated into the sections below:

1. **Server-only soft-delete fields.** `proto_conventions.md` §"What
   belongs in the API" makes this explicit: `models.X.deleted` and the
   restorer snapshot are server-internal and never exposed on the wire.
   Phase 1 task list updated to include the recommended
   `// Server-only; not exposed on the API.` field comments on
   `Community.deleted_snapshot` and `CommunityUser.deleted`.
2. **`owner_user_id` API exposure is deliberately out of scope for this
   PR.** Phase 3 client work needs it (Danger Zone gating per design doc
   §7.2: `community.owner_user_id == currentUser.id`). The proto
   convention says never to add API fields in anticipation of future use,
   so this PR adds the field only on the model. Added an explicit
   forward-looking note in Open Questions so it isn't lost; the API
   exposure should ride on whichever Phase 2 step first needs it (likely
   the `RestoreCommunity` response or an enrichment of
   `GetCommunityResponse`).
3. **`Unimplemented` is not gated by `check_codeinternal.js`.** That
   linter only catches bare `connect.NewError(connect.CodeInternal, err)`.
   `CodeUnimplemented` is allowed; `connecterr.Internal` is not the right
   helper for stubs. Confirmed in Phase 3 task notes.
4. **Stub handlers don't need explicit logging.** The HTTP middleware
   already logs every RPC with method/path/status/duration/request_id
   per `docs/server/observability.md` §"Logging Middleware". Adding a
   handler-level log line would be redundant. Phase 3 updated.
5. **File-size check.** `docs/server/architecture.md` §"File Size
   Guidelines" targets <500 lines, max 1000. Phase 3 task list now
   includes a quick line-count check on `lifecycle.go` /
   `membership.go` before deciding whether to host the stubs there or
   create `restore.go` / `rejoin.go`.
6. **No round-trip tests required.** This is pure scaffolding with no
   save/read pair. `proto_conventions.md` §"Round-trip tests" applies to
   API fields the client sets and reads back; the new RPCs do neither in
   Phase 1. Documented under Phase 5 to avoid the question coming up in
   review.
7. **No service-to-service coupling.** The plan adds handlers to the
   existing community service only; no new cross-service deps —
   compliant with `docs/server/architecture.md` §"Service Independence".
8. **Client docs (`docs/client/*.md`) do not apply.** Phase 3 ships all
   user-visible behavior — caching, i18n, accessibility, ViewModel
   wiring — and is governed by those docs at that time. This PR has no
   client surface.

## Problem Statement

Phase 1 of the community soft-delete project (#1619) needs all proto and
schema additions in place before Phase 2 server work can begin. Today the
`Community` message has only `creator_id` and an unused `deleted` field;
`CommunityUser` has no soft-delete metadata; the new restore/rejoin RPCs and
event types do not exist; and the undo registry has no entries for the new
events.

This issue lands all of that scaffolding in a single PR with **no callers**,
so it is functionally invisible: code generates and compiles, the new RPCs
return `Unimplemented`, and the new fields stay zero-valued until Phase 2
populates them. The matching DB migration (backfilling `owner_user_id` and
adding constraints) is tracked separately in #1644 and depends on this PR.

## Context from Comments

The issue has no comments yet — it was filed in this same session along
with the rest of Phase 1. Authoritative context lives in:

- **Parent #1619** — the sequencing plan that puts this PR before Phase 2.
- **`docs/community_delete_and_leave.md` §6.1, §6.3, §11** — proto shapes,
  field tags, and the undo-registry classifications.
- **Sibling Phase 1 PRs already merged**: #1630 (read-path filter), #1632
  (notification dispatcher guard), #1633 (background-job write guard).
  These rely only on `Community.deleted` (which already exists), so they
  do not constrain the shapes added here. They do, however, set the
  precedent for "merge as a pure no-op, prove the helper fires when
  forced."

Because there are no comments, all design decisions below are derived
directly from the design doc and existing codebase patterns.

## Codebase research

Confirmed against the actual repo state:

- **`proto/ripls/models/community.proto`**: `Community` uses tags 1–10 and
  20; tags 11–19 and 21+ are free. `CommunityUser` uses tags 1–5; tag 6+
  free. `CommunityEventType` last value is 39 (`EXPERIENCE_CANCELLED`); next
  free is 40. `DeletedMetadata` already exists in `common.proto` and is
  imported.
- **`proto/ripls/api/community_service.proto`**: defines a parallel
  `CommunityEventType` enum (line 479) that is a **subset** of the models
  enum — it is missing planning events, undone events, and
  `EXPERIENCE_COMPLETED` (24–37). The conversion site
  (`server/services/community/events.go:70`) just casts
  `models.CommunityEventType` → `api.CommunityEventType` by integer value,
  so missing API-side labels degrade gracefully but events of those types
  are not filterable from the client. **Action:** add the four new values
  to **both** enums to preserve client filterability.
- **`server/undo/registry.go`**: requires every value in the models enum
  to have an entry; `TestEveryEventTypeClassified` enforces this.
  `COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED` is already classified
  `ClassificationRetentionRestore` ("Soft-delete on Community + existing
  restore path") — anticipating this work.
- **Service registration**: Connect-Go requires every RPC declared on the
  service to have a handler. We will register `RestoreCommunity` and
  `RejoinCommunity` and ship handlers that return
  `connect.NewError(connect.CodeUnimplemented, ...)` until Phase 2 step 7.

## Potential Solutions

### Option A: Single PR, exact shapes from doc (recommended)

- **Approach:** Add every proto field, RPC, and registry entry the design
  doc specifies, in one PR. New RPCs ship as `Unimplemented` stubs.
- **Pros:** Phase 2 PRs become pure behavior changes — no proto churn.
  Single review covers the full surface and the resulting types are
  immediately usable in Phase 2 tests. The contract is locked in before
  any caller exists, so we cannot accidentally evolve it under callers.
- **Cons:** A larger initial diff than each individual sibling Phase 1 PR.
- **Effort:** Small to medium — almost entirely additive proto + a
  registry block.

### Option B: Split proto-models from API and registry

- **Approach:** PR1: `models/community.proto` fields + registry entries.
  PR2: `api/community_service.proto` request/response/RPC additions and
  `Unimplemented` handlers.
- **Pros:** Smaller individual PRs.
- **Cons:** PR1 leaves the API surface inconsistent (event types in models
  but no parallel API enum values). The two halves are tightly coupled —
  no consumer ships between them — so the split adds review overhead with
  no shipping benefit.
- **Effort:** Medium (PR coordination cost).

### Option C: Skip RPC registration; only add request/response shapes

- **Approach:** Add the proto messages but do not register the RPCs on
  the service yet. Phase 2 step 7 registers and implements together.
- **Pros:** Slightly smaller PR.
- **Cons:** Generated client code lacks the methods until Phase 2, which
  makes early integration smoke-testing harder. No real benefit since the
  `Unimplemented` stub is one line per handler.
- **Effort:** Small.

## Recommendation

**Option A.** The design doc is precise enough that the entire surface can
land in one self-contained PR, matching how #1630/#1632/#1633 each landed
their full scaffold in one piece. The `Unimplemented` stubs are cheap and
make the new RPCs visible to client code from day one (which is useful for
the Phase 3 client work and for write-time integration tests in Phase 2).

The single open spec ambiguity below is small enough that a quick decision
unblocks the PR without a doc revision.

## Open Questions

- **Exact name and count of the new event types.** The design doc is
  internally inconsistent: §6.4 lists four
  (`COMMUNITY_DELETED`, `COMMUNITY_RESTORED`, `OWNERSHIP_TRANSFERRED`,
  `MEMBER_REJOINED_WITHIN_WINDOW`); §11 says "three new CommunityEventType
  values"; §6.3 (undo registry) only enumerates three (omits the rejoin
  event). The issue body inherits this ambiguity by writing "REJOINED" as a
  placeholder. **Recommendation:** add **all four** values, naming the
  rejoin one `COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW` to match
  §6.4 verbatim. Classify it `ClassificationClientOnly` with rationale
  "Inverse is `LeaveCommunity` within the same window." The §6.3 omission
  reads like a doc oversight; §6.4 is the more recent prose. Fixes the doc
  in passing if needed (one-line edit to §11 to say "four").
- **`Unimplemented` vs. omit handlers.** Connect-Go forces a handler for
  every registered RPC. **Recommendation:** register both new RPCs and
  return `connect.NewError(connect.CodeUnimplemented, errors.New("Phase
  2"))` from the handlers. Cheaper than a temporary "skip these RPCs in
  Phase 1, add them in Phase 2" branch, and makes the proto contract
  observable to clients immediately.
- **API-side `CommunityEventType` parity.** The API enum is currently a
  subset of the models enum, missing 24–37. This is an existing gap; we
  should not fix the back-debt in this PR. **Recommendation:** add only
  the four new values (40–43) to the API enum to keep them filterable.
  File the historical drift as a separate cleanup if anyone cares; it is
  out of scope here.
- **`new_owner_user_id` shape on `LeaveCommunityRequest`.**
  **Recommendation:** add it as `optional string new_owner_user_id = 2;`.
  Optional gives explicit presence per the proto-conventions rule in
  CLAUDE.md and matches the doc's "required only when caller is owner"
  semantics — Phase 2 server logic enforces presence, the proto stays
  permissive.
- **`creator_id` comment update.** The doc says
  `creator_id` becomes "historical creator only, never updated."
  **Recommendation:** update the existing field comment in this PR;
  the comment is a doc change, not a contract change, and it is the only
  mechanism that prevents a future contributor from setting `creator_id`
  on update paths. Keep the proto comment vendor-neutral and clean per the
  proto-comments rule in CLAUDE.md.
- **`owner_user_id` on the API (deferred).** Phase 3 (client) needs to
  know the current owner to gate the Danger Zone (§7.2 of the design
  doc). The proto convention forbids adding API fields in anticipation
  of future use, so `owner_user_id` stays model-only in this PR.
  **Recommendation:** add `string owner_user_id` to the relevant API
  read response (likely `GetCommunityResponse` and/or `CommunityItem`)
  in the same PR that first ships an active caller — i.e., as part of
  Phase 2's `RestoreCommunity` end-to-end work or the
  `Get/ListCommunities` enrichment that Phase 3 depends on. File a
  one-line follow-up note on #1619 if it isn't already captured there.

## Implementation Plan

### Phase 1 — Proto additions (models)
**Goal:** Land every model-side type and field needed for Phase 2.
**Tasks:**
- [ ] In `proto/ripls/models/community.proto`:
  - [ ] Add `string owner_user_id = 11;` to `Community` immediately
        after `creator_id` in source order. Tag 11 sits in the
        active-fields band (1–19 today), not the soft-delete band
        starting at 20; this matches its semantics as an
        active-community identity field. Doc comment: "Current owner;
        transfer changes this."
  - [ ] Add `optional CommunityDeletedSnapshot deleted_snapshot = 21;`
        clustered next to `deleted = 20` (genuine soft-delete metadata
        cluster). Comment describes its role as the eligible-restorer
        set, plus the convention line "Server-only; not exposed on the
        API." (per `docs/proto_conventions.md` §"What belongs in the
        API").
  - [ ] Update `creator_id` comment: "Immutable historical creator. Never
        updated. Use `owner_user_id` for current ownership."
  - [ ] Add new message
        `CommunityDeletedSnapshot { repeated string member_user_ids = 1; }`
        with comment from doc §6.1.
  - [ ] Add `optional DeletedMetadata deleted = 6;` to `CommunityUser`,
        with the same "Server-only; not exposed on the API." line. Tag
        6 confirmed free.
  - [ ] Append four new `CommunityEventType` values (40–43):
        `COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED`,
        `COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED`,
        `COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED`,
        `COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW`.
**Validates:** `npm run generate` succeeds; resulting Go types compile.

### Phase 2 — Proto additions (API)
**Goal:** Mirror what Phase 3 client code will consume.
**Tasks:**
- [ ] In `proto/ripls/api/community_service.proto`:
  - [ ] Add `RestoreCommunityRequest { string community_id = 1; }` and
        empty `RestoreCommunityResponse {}`.
  - [ ] Add `RejoinCommunityRequest { string community_id = 1; }` and
        empty `RejoinCommunityResponse {}`.
  - [ ] Add `optional string new_owner_user_id = 2;` to
        `LeaveCommunityRequest` with comment describing owner-leave
        usage; defer validation to Phase 2 server work.
  - [ ] Append the four new values to the API-side `CommunityEventType`
        enum (matching numeric values 40–43).
  - [ ] Register two new RPCs on `CommunityService`:
        `rpc RestoreCommunity(...) returns (...)` and
        `rpc RejoinCommunity(...) returns (...)`.
**Validates:** `npm run generate` regenerates Go and Dart cleanly; no
cross-import between `models` and `api` packages introduced.

### Phase 3 — Server stub handlers
**Goal:** Keep the build green now that the service has new methods.
**Tasks:**
- [ ] Check current line counts of `server/services/community/lifecycle.go`
      and `server/services/community/membership.go` (target <500 per
      `docs/server/architecture.md` §"File Size Guidelines"). If either
      is already >700 lines, host the stubs in a new
      `server/services/community/restore.go` /
      `server/services/community/rejoin.go` instead. Otherwise add to
      the closest-fit existing file: `RestoreCommunity` next to
      `DeleteCommunity` in `lifecycle.go`; `RejoinCommunity` next to
      `LeaveCommunity` in `membership.go`.
- [ ] Add stub handlers returning
      `connect.NewError(connect.CodeUnimplemented, errors.New("RestoreCommunity not implemented; tracked in #1619 Phase 2"))`
      (and analogous text for `RejoinCommunity`). `connecterr.Internal`
      is **not** the right helper here — it's reserved for `CodeInternal`
      and is what `scripts/check_codeinternal.js` gates. `CodeUnimplemented`
      via `connect.NewError` is allowed and idiomatic for stubs.
- [ ] **Do not** add handler-level logging to the stubs. The HTTP
      logging middleware (`server/middleware/logging.go`) already
      records every RPC with method/path/status/duration/request_id —
      see `docs/server/observability.md` §"Logging Middleware". A
      `WARN`-level entry is logged automatically when the handler
      returns `Unimplemented` (4xx-equivalent).
- [ ] Run `npm run build` and `npm run lint:ephemeral-comments` —
      confirm the issue references in the error messages count as
      stable per CLAUDE.md ("Issue and PR numbers are allowed").
**Validates:** `go build ./...` passes; service can be invoked and
returns the expected `Unimplemented` Connect code; request middleware
logs the call automatically.

### Phase 4 — Undo-registry entries
**Goal:** Keep `TestEveryEventTypeClassified` passing for the new enum
values.
**Tasks:**
- [ ] In `server/undo/registry.go` add four entries:
  - [ ] `COMMUNITY_DELETED` → `ClassificationRetentionRestore`,
        rationale: "30-day soft-delete reversed by `RestoreCommunity`
        RPC, not by an `Undo*` RPC."
  - [ ] `COMMUNITY_RESTORED` → `ClassificationInternal`,
        rationale: "System-emitted by `RestoreCommunity`; not a
        user-actionable event."
  - [ ] `OWNERSHIP_TRANSFERRED` → `ClassificationClientOnly`,
        rationale: "Inverse is the new owner initiating their own
        ownership transfer."
  - [ ] `MEMBER_REJOINED_WITHIN_WINDOW` → `ClassificationClientOnly`,
        rationale: "Inverse is `LeaveCommunity` within the same
        30-day window."
- [ ] Update the `MEMBER_LEFT` entry's rationale to mention the 30-day
      rejoin window as the natural inverse during that window (per
      design doc §6.3).
**Validates:** `go test ./server/undo/...` runs `TestEveryEventTypeClassified`
green; `TestEveryServerUndoableActionHasUndoDataValidator` continues to
warn (no new server-authoritative entries).

### Phase 5 — Verify and ship
**Goal:** Prove this is a true no-op for users.
**Tasks:**
- [ ] `npm run generate && npm run build` — both clean.
- [ ] `go test ./...` — all green, including the registry test.
- [ ] `go test -tags=integration ./...` if any integration tests exist
      that touch the community service signature (likely none for pure
      additions, but spot-check).
- [ ] Smoke-call the new RPCs against a local server; confirm
      `Unimplemented` response.
- [ ] No round-trip tests are added — confirmed not applicable
      (`docs/proto_conventions.md` §"Round-trip tests" applies to API
      fields the client sets and reads back; the new RPCs are not
      save-style and the model-only fields are server-internal).
- [ ] Open PR titled exactly per #1630 / #1632 style:
      "Phase 1 of #1619: proto + schema additions for community
      soft-delete (issue #1643)".
**Validates:** The PR ships; #1644 (data migration) becomes ready to
work.

## Related Issues

- **#1619** — Parent: full community soft-delete project. This PR is
  Phase 1 step 1.
- **#1644** — Phase 1 step 2: data migration (backfill `owner_user_id`,
  add single-owner constraint, verify `community_user` uniqueness).
  Depends on this PR landing first.
- **#1621, #1622, #1623, #1624** — Sibling Phase 1 issues, all already
  closed. They establish the merge pattern and confirm the
  read-path/notification/job-write/cleanup helpers are in place when
  Phase 2 starts populating deleted state.
- **#1620** — Phase 4: daily community purge job. Out of scope.
- **#1571** — Stale community-gear after leave. Will close as
  superseded once Phase 2 step 8 ships.
