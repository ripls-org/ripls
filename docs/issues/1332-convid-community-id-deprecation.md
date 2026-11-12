# Issue #1332: [code_quality] Complete ConversationId/CommunityId deprecation migration

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1332
**Filed:** 2026-04-25 | **Last activity:** 2026-07-08
**Labels:** auto-filed, review:code_quality | **Assignees:** unassigned
**Context docs:** `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/proto_conventions.md`, `docs/client/chat.md`, `docs/client/sharing.md`

## Problem Statement

Two overlapping deprecations have been left unfinished, and the resulting
`//nolint:staticcheck` suppressions have grown to 46+ across ~20 server files.
The pattern is accreting — every new feature that needs to resolve a
conversation from a `Community{Gear,Request,Experience}` join row (or a
community ID from a feed/search request) adds another suppression, and each
fallback read hides a data-model inconsistency.

Two distinct deprecations are conflated in the review:

1. **Model-level** — `CommunityGear.conversation_id`,
   `CommunityRequest.conversation_id`,
   `CommunityExperience.conversation_id` are all marked
   `[deprecated = true]` in `proto/ripls/models/community.proto`. The
   canonical fields live on the parent entity: `Gear.conversation_id`,
   `Request.conversation_id`, `Experience.conversation_id`. Server code reads
   canonical first, then falls back to the deprecated field for
   "pre-migration records". One writer still dual-populates the deprecated
   `CommunityGear.conversation_id` on every share
   (`server/services/community/gear_sharing.go:321`).
2. **API-level** — `GetFeedRequest.community_id` (singular, field 1) and
   `SearchRequest.community_id` (singular, field 3) are documented as
   deprecated in favour of `community_ids` (plural). The Flutter client
   already only sends the plural form
   (`app/lib/services/feed_service.dart:63`,
   `app/lib/services/search_service.dart:72`), but the server still accepts
   both. Inside the search service the singular field is also used as a
   per-community carrier while iterating over `community_ids`
   (`server/services/search/service.go:98–104`).

The completion criteria are shared: (1) prove no live production data still
depends on the deprecated columns, (2) delete the deprecated fields via a
coordinated proto change, and (3) remove all 46 `//nolint:staticcheck`
suppressions together.

## Context from Comments

- **2026-04-28 triage (maintainer):** classified as Task/P2, description
  "code_quality Medium: complete ConversationId/CommunityId deprecation
  migration". No further design direction.
- **2026-07-08 (maintainer):** `@claude` mention to file this plan.

No maintainer decisions on approach, scope, or backfill strategy have been
recorded. Open Questions below capture the choices a reviewer needs to sign
off on before implementation.

Related in-tree context that shaped the analysis:

- `docs/client/chat.md:34-58` documents the current conversation-scoping
  model (per-gear / per-request / global-per-experience). It calls out that
  `CommunityExperience.conversation_id` "still exists in storage
  (deprecated) but is not surfaced" and that the parent-entity fields are
  canonical — the migration is real; only the code cleanup is pending.
- `docs/client/chat.md:574-600` describes a completed Experience conversation
  migration that (a) unified per-community conversations onto
  `Experience.conversation_id` and (b) explicitly left
  `CommunityExperience.conversation_id` rows in place "backward-compatible;
  the server simply ignores them". Gear and Request received the same
  treatment (the fallback reads accreted afterwards).
- `server/storage/community_schema.go:13-124` is the reference backfill
  shape used for `Community.owner_user_id`: idempotent, reads
  `binary_proto`, updates via `storage.Update` so scalar column and blob
  stay in lockstep, LIMIT 10000 per run, safe to invoke on every startup.
  A conversation_id backfill can follow this exact pattern.

## Potential Solutions

### Option A: Single-cut proto removal + one-shot startup backfill
- **Approach:**
  1. Add a startup backfill helper
     `InitializeCommunityConversationIDMigration` in
     `server/storage/community_schema.go` that iterates each of the three
     join tables. For every row where the deprecated `conversation_id` is
     non-empty but the parent entity's `conversation_id` is empty, copy it
     over via `storage.Update` on the parent. Wire from `server/main.go`
     alongside the existing `InitializeCommunityOwnerUserID` call. Deploy
     to prod, observe log counts drop to zero, then leave the helper in
     place as a permanent idempotent guard (matches the owner_user_id
     precedent).
  2. Remove the write to `communityGear.ConversationId` in
     `server/services/community/gear_sharing.go:321` so no new rows
     accumulate deprecated values.
  3. Convert every read site (46 suppressions) from
     "canonical-then-fallback" to "canonical only". Where the fallback
     read was structural (e.g. `feed/feed_status.go` map building), delete
     the fallback branch.
  4. Delete `conversation_id` from `CommunityGear`, `CommunityRequest`,
     `CommunityExperience` in `proto/ripls/models/community.proto` and
     add `reserved 6;` / `reserved 4;` / `reserved 5;` +
     `reserved "conversation_id";` so the tags can never be reused (matches
     the `notification.proto:82-84` precedent).
  5. Repeat for the API-level singular `community_id` fields on
     `GetFeedRequest` and `SearchRequest` — delete field + `reserved`.
     Simultaneously refactor
     `server/services/search/service.go:95-104` to pass the per-community
     ID as an explicit function parameter instead of tunnelling it through
     the singular request field (that pattern only exists because the
     helpers were written for the pre-multi-community API).
  6. Regenerate code (`npm run generate`), run
     `go test -tags=integration -timeout 20m ./...`, and land as one PR.
- **Pros:** removes the whole class of `//nolint:staticcheck` suppressions
  in one PR, eliminates the accretion pathway, matches the coordinated cut
  the issue text recommends. The API-level fields are safe to cut because
  the Flutter client already sends only `community_ids` and there's no
  pinned min-client-version story that keeps very old builds in scope.
- **Cons:** any pre-1.0-era Android/iOS build that still sends the
  singular `community_id` (feed / search) will silently see "no
  communities specified" and get an InvalidArgument error. Requires
  confirming with the release owner that no such builds are still in the
  field (there is no server-side min-client-version enforcement to lean
  on). The proto message changes are wire-compatible for reads (unknown
  fields are ignored) so old server binaries reading new protos and
  vice-versa are fine — the risk is entirely on the client-side clients.
- **Effort:** medium. ~20 server files touched, plus proto edits, one
  backfill helper (~150 LOC + tests mirroring
  `community_schema_test.go`), and a modest search-service refactor.

### Option B: Phased — backfill + writer cleanup now, proto delete after a soak
- **Approach:** ship phases 1–3 of Option A (backfill helper wired in
  main.go, writer removal, fallback-branch cleanup), leaving the
  deprecated proto fields and `//nolint:staticcheck` suppressions in
  place. Add a Prometheus/log-counter that fires whenever a fallback read
  actually returns non-empty. When the counter reads zero for two full
  release cycles (weeks, not months — the shape of "how long we watch"
  should still be picked deliberately), a follow-up PR does the proto
  delete + suppression removal.
- **Pros:** lowest-risk single PR. If the backfill somehow misses rows
  or new writes sneak in, we notice at the log/metric layer before the
  proto fields disappear. Safer against unknown-unknowns.
- **Cons:** perpetuates the exact accretion problem the issue is filed to
  stop. The reason 46 suppressions exist is that "delete later" already
  happened once and never actually landed. A soak that requires a human
  to remember to file the follow-up PR is not likely to close.
- **Effort:** medium up front, small follow-up. Total effort > Option A
  because of the two-PR overhead.

### Option C: Backfill only — do not delete the proto fields
- **Approach:** only ship the backfill helper. Leave the deprecated
  proto fields, the writer, all reads, and every `//nolint:staticcheck`
  suppression in place forever.
- **Pros:** zero risk to old clients or in-flight data.
- **Cons:** does not resolve the issue at all. Every new feature that
  needs a conversation ID keeps adding suppressions. The data-model
  inconsistency stays hidden behind the fallback.
- **Effort:** small, but negative ROI.

## Recommendation

**Option A (single-cut proto removal + one-shot startup backfill).** The
issue text explicitly calls for a single coordinated proto change that
removes all 46 suppressions together, and every ingredient needed to do
that safely is already in the codebase:

- The backfill shape is proven (`InitializeCommunityOwnerUserID`).
- The dual-write is already down to a single line
  (`gear_sharing.go:321`), so preventing new deprecated writes is a
  one-line change.
- The Experience conversation migration is documented as already
  complete for that entity and unsurfaced (`docs/client/chat.md:57`);
  extending the same treatment to Gear and Request is the same shape.
- The Flutter client no longer populates the singular API `community_id`
  fields.

Option B's staged soak is the right call in an environment with hostile
client pinning or an unclear write path. Neither applies here: the write
path is exactly one line, and clients already send the canonical shape.
The main gate for Option A is confirming with the release owner that no
old client builds (pre-`community_ids` rollout) are still in the wild.
If that check surfaces meaningful old-client traffic on the deprecated
API field, downgrade the API-level portion to Option B (server keeps
accepting singular `community_id` until traffic drains) while still
shipping the model-level cut. The model-level fields are internal — no
client reads them — so they can be cut unconditionally.

## Open Questions

- **Are there any client builds still in the field that populate the
  singular `SearchRequest.community_id` or `GetFeedRequest.community_id`?**
  — Recommendation: grep the last 14 days of prod access logs for these
  fields (or add a temporary log line and observe for a release cycle
  before the proto delete). If yes, keep the API-level fallback until
  traffic drains; if no, cut in the same PR. Ship the model-level cut
  either way.
- **Should the backfill helper stay wired at startup permanently, or
  self-remove after one clean run?** — Recommendation: leave it wired,
  matching `InitializeCommunityOwnerUserID`. Startup cost is a bounded
  scan (`LIMIT 10000`) that returns zero rows once complete, and a
  permanent idempotent guard defends against any future code path that
  writes to the deprecated column by accident before the proto delete
  reaches all readers.
- **Does the search service's per-community loop
  (`server/services/search/service.go:92-126`) need a small refactor to
  drop the singular-field tunnelling, or can we just delete the
  `//nolint:staticcheck` markers because the singular field will be
  gone?** — Recommendation: refactor. Once the proto field is deleted
  the code won't compile, so a refactor is not optional; the choice is
  only about shape. Pass `communityID string` as an explicit first arg
  to `semanticSearch` / `exactSearch` / `searchUsers` /
  `searchGearExact` / `searchRequestsExact` /
  `searchExperiencesExact` / `convertToSearchResultItemBatch`. Small
  and mechanical.
- **Does `admin/cleanup.go` (which sums conversation IDs across all
  three entity types for a full community delete) need the fallback
  read?** — Recommendation: after the backfill run, no — parent
  `conversation_id` is authoritative and every row that had a
  deprecated value now has a canonical one. Drop the three fallback
  `add(cg.ConversationId)` calls in the same cleanup pass.
- **Should the doc updates land in the same PR?** — Recommendation:
  yes, keep `docs/client/chat.md` claims true to the code (the
  freshness gate will otherwise flag it). Remove the "deprecated
  back-compat" language and the CommunityExperience note at
  `docs/client/chat.md:57` since the fields no longer exist.

## Implementation Plan

### Phase 1: Backfill helper + writer cleanup
**Goal:** guarantee every existing row has its canonical
`conversation_id` populated, and prevent any new deprecated writes.
**Tasks:**
- [ ] Add `InitializeCommunityConversationIDMigration` to
      `server/storage/community_schema.go`. For each of the three
      join tables, load rows in bounded batches
      (matching `backfillCommunityOwnerUserID` — `LIMIT 10000`
      per pass, filtered to `conversation_id IS NOT NULL AND
      conversation_id != ''`), decode `binary_proto`, and copy the
      deprecated `conversation_id` to the parent entity if the
      parent's field is empty. Idempotent, safe to run on every
      startup. Log structured slog info with row counts and duration
      (per `docs/server/observability.md`) so an operator can watch
      the backfill drain to zero across restarts.
- [ ] Add a `_test.go` companion mirroring
      `server/storage/community_schema_test.go`. Cover: empty parent
      gets backfilled; non-empty parent is left alone; missing parent
      is skipped with a warn log; no-op when tables are absent;
      idempotency (second run over the same data returns zero
      updated rows).
- [ ] Wire the helper from `server/main.go` alongside the existing
      `InitializeCommunityOwnerUserID` call. Use `logging.GoSafe` if
      the scan cost turns out to be non-trivial on prod; the
      `owner_user_id` backfill runs inline at startup so match that
      shape unless benchmarks say otherwise.
- [ ] Delete `communityGear.ConversationId = conversationID` at
      `server/services/community/gear_sharing.go:321` and its
      `//nolint:staticcheck` marker. The write is redundant — the
      canonical `gear.ConversationId` has already been set two lines
      above.
**Validates:** the backfill closes the data gap; no new deprecated
writes can be added. Server keeps compiling with fallback reads still
in place, so this phase is independently deployable and reversible.

### Phase 2: Convert all reads to canonical-only
**Goal:** collapse every "canonical-or-fallback" read pattern to
"canonical only", so the proto fields become unreferenced.
**Tasks:**
- [ ] For each read site in the file list below, delete the
      fallback branch (`else if cg.ConversationId != ""` etc.) and
      remove the `//nolint:staticcheck` suppression. Verify the code
      still handles the "no conversation yet" case correctly — the
      canonical field is empty until the sharing flow creates the
      conversation, and the surrounding logic already handles that
      (see e.g. `server/services/feed/service.go:498-509` where the
      ChatConversation-by-topic query catches unresolved gear).
      Files: `server/services/admin/cleanup.go`,
      `server/services/chat/conversations.go`,
      `server/services/community/past_giveaways.go`,
      `server/services/community/gear_sharing.go`,
      `server/services/experience/summary.go`,
      `server/services/feed/feed_status.go`,
      `server/services/feed/service.go`,
      `server/services/gear/service.go`,
      `server/services/portfolio/fetch.go`,
      `server/services/portfolio/inbox_feed.go`,
      `server/services/portfolio/inbox_view.go`,
      `server/services/request/gen.go`,
      `server/services/request/helpers.go`,
      `server/services/request/helpers_batch.go`,
      `server/services/request/lifecycle.go`,
      `server/services/request/offers.go`,
      `server/services/transfer/interest.go`,
      `server/services/transfer/lifecycle.go`,
      `server/services/transfer/service.go`.
- [ ] Refactor the search service's per-community carrier: pass
      `communityID` as an explicit `string` parameter into
      `semanticSearch`, `exactSearch`, `searchUsers`,
      `searchGearExact`, `searchRequestsExact`,
      `searchExperiencesExact`, and
      `convertToSearchResultItemBatch`. Remove the
      `perCommunityReq.CommunityId = cid` assignment and its
      `//nolint:staticcheck` at `server/services/search/service.go:98`.
      Delete the `resolveSearchCommunityIDs` fallback branch and the
      matching one in `server/services/feed/service.go:56-64`.
      Return an InvalidArgument error when `community_ids` is empty
      instead of falling through to the singular field.
- [ ] Update all test call sites that currently construct
      `api.GetFeedRequest{CommunityId: ...}` or
      `api.SearchRequest{CommunityId: ...}` to use the plural
      `CommunityIds` field. There are ~46 such sites, concentrated
      in `server/services/feed/service_test.go` and
      `server/services/search/service_test.go`; grep is authoritative
      (`grep -rn "api\.GetFeedRequest{" server/ | grep _test.go`).
      Without this update the removal in Phase 3 breaks the test
      build; do it now so Phase 2 stays independently deployable.
- [ ] `#2020` markers in `server/services/profile/postcards_test.go`
      are out of scope and must be preserved — they are for a
      separate deprecation (`CtaRow.CommunityId`).
**Validates:** `go vet ./...` and
`go test -tags=integration -timeout 20m ./...` still pass; a full
`grep` shows zero remaining `//nolint:staticcheck` for
ConversationId/CommunityId under `server/`.

### Phase 3: Proto delete + regen
**Goal:** physically remove the deprecated fields so no new code can
reintroduce them.
**Tasks:**
- [ ] In `proto/ripls/models/community.proto`: delete
      `string conversation_id = 6 [deprecated = true];` from
      `CommunityGear`, `= 4` from `CommunityRequest`, `= 5` from
      `CommunityExperience`. In each message add
      `reserved 6;` / `reserved 4;` / `reserved 5;` and
      `reserved "conversation_id";` (mirror
      `proto/ripls/models/notification.proto:82-84`). Reserving both
      the tag and the name is required by Buf's breaking-change
      linter and prevents accidental reuse of either.
- [ ] In `proto/ripls/api/feed_service.proto` and
      `proto/ripls/api/search_service.proto`: delete the singular
      `community_id` field and reserve the tag and name. If the
      API-level open question above (client-traffic check) surfaces
      old-client usage, split this into a follow-up PR after the
      model-level cut lands.
- [ ] `npm run generate` (regenerates Go bindings under
      `server/gen/` **and** Dart bindings under `app/lib/data/gen/`).
      Generated code is not checked in
      (see CLAUDE.md → "Code Generation"), so no committed files
      change from regen itself; the source `.proto` files are what
      lands in the PR.
- [ ] `go test -tags=integration -timeout 20m ./...` and
      `cd app && flutter test --reporter github` to confirm the
      client compiles against the reduced surface. Include a
      lint pass: `npm run lint` (proto + go + context-map gate).
- [ ] Update `docs/client/chat.md` (drop the "deprecated back-compat"
      language in §"Conversation Scoping per Item Type" and the
      CommunityExperience note at line 57) and any other durable doc
      the freshness gate flags after the change. Sweep
      `docs/proto_conventions.md` for stale references. Do **not**
      hand-edit the `freshness:` `verified_commit` stamps — the
      nightly `claude_refresh_docs` ratchet advances them after
      merge.
**Validates:** no `//nolint:staticcheck` for ConversationId /
CommunityId under `server/` (grep returns zero). CI green,
including `npm run lint:context-map` and the Buf breaking-change
lint against `main`.

## Related Issues

- **#2020** — separate deprecation for `CtaRow.CommunityId`,
  `DailyItem.ItemType`/`ItemId`/`Title`/`BackgroundMediaId`, etc.
  in `proto/ripls/api/portfolio.proto` and
  `proto/ripls/api/workshop_service.proto`. The
  `//nolint:staticcheck` suppressions carrying `// dual-population
  per #2020` are out of scope for this issue and must be preserved.
- **#1679** — earlier work that fixed the "emit the sharing anchor
  only when the gear's conversation is first created" behavior in
  `server/services/community/gear_sharing.go`. Referenced here
  because that fix is the reason the writer at
  `gear_sharing.go:321` is the last remaining write to the
  deprecated column.
- **#1644** — original migration plan for
  `Community.owner_user_id`. Referenced here because
  `InitializeCommunityOwnerUserID`
  (`server/storage/community_schema.go`) is the reference
  backfill shape this plan copies.

---

## Audit notes

Audited against the following selected guidance (`docs/llms.txt`
lens/glob match; see `docs/context_map.md`):

- **Anchors (server + conventions + proto):**
  `docs/proto_conventions.md`, `docs/server/architecture.md`,
  `docs/server/conventions.md`.
- **Engaged dimensions:** `docs/reviews/api_design_review_prompt.md`
  (§ "Areas to Review" — deprecation, reserved tags, breaking
  changes), `docs/reviews/code_quality_review_prompt.md`
  (§ "Areas to Review" — dead code, TODO hygiene),
  `docs/reviews/observability_review_prompt.md` (§ 1 — logging
  levels & structured fields on the backfill).
- **Leaf docs:** `docs/client/chat.md` (conversation-scoping model
  and the completed Experience migration precedent — targeted for
  the doc update in Phase 3), `docs/server/observability.md`
  (structured slog fields on the backfill helper).

Fixes applied during audit:
- **Test-caller sweep** — Phase 2 now includes updating the ~46
  `api.GetFeedRequest{CommunityId: …}` / `api.SearchRequest{
  CommunityId: …}` test constructors so the Phase 3 proto delete
  doesn't break the test build.
- **Reserved-tag-and-name discipline** — Phase 3 explicitly reserves
  both tag and field name (Buf breaking-change lint requires this),
  citing `notification.proto:82-84` as the in-tree precedent.
- **Generated-code paths** — Phase 3 now names both `server/gen/`
  and `app/lib/data/gen/` and calls out that generated code is not
  checked in per CLAUDE.md, so the PR diff is only the `.proto`
  source edits.
- **Observability wiring for the backfill** — Phase 1 now
  explicitly asks for structured slog counters and duration on the
  helper, per `docs/server/observability.md`.
- **Freshness stamp discipline** — Phase 3 clarifies that the
  `verified_commit` stamps are advanced by the nightly
  `claude_refresh_docs` ratchet, not by hand, so this PR leaves
  them alone.
- **Lint gate** — Phase 3 now calls out `npm run lint` (which
  transitively runs `lint:proto`, `lint:go`, and
  `lint:context-map`).
