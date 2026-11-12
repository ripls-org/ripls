# Issue #2048: ShareLink — polymorphic table + migration (1195 sub-issue 1)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2048
**Filed:** 2026-05-19 | **Last activity:** 2026-05-19
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** `docs/issues/1195-web-guest-rsvp.md` (parent plan; sub-issue 1 section)

> **Audit pass — 2026-05-19**
>
> Audited against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/client/accessibility.md`, `docs/client/testing.md`, `docs/server/architecture.md`, `docs/server/observability.md`. Findings threaded into the relevant phases:
> - **Phase 3 / server independence:** the new `GetOrCreateEventShareLink` reads `Experience` directly via storage (`s.storage.GetByID`), not via the experience service. The short-code lookup is inlined in each service (community + login) instead of factored into a shared library, mirroring the existing `lookupInvitation` shape and preserving service independence.
> - **Phase 3 / log levels:** capacity / validation refusals log at `WARN` (user errors, not system failures); storage and external-service failures log at `ERROR`. Aligns with `observability.md`'s "ERROR is reserved for system failures" rule.
> - **Phase 3 / proto wire change:** `CheckInvitationResponse` adds an optional field that surfaces which `oneof target` is set on the looked-up `ShareLink`, so the Dart client can branch — a wire-compatible additive change (Compliance H).
> - **Phase 4 / client layering:** the new code splits cleanly into `ShareLinkService` (Connect client under `app/lib/services/`) and `ShareLinkRepository` (under `app/lib/data/repositories/`). ViewModels read the repository; the service is invisible to the presentation layer.
> - **Phase 4 / async safety:** ViewModel uses `AsyncNotifier.autoDispose` per `architecture.md`'s preferred pattern; methods that manually set state include `ref.mounted` checks after every `await`; a disposal test is required.
> - **Phase 4 / navigation:** in-app forward navigation uses `NavigationHelpers.pushScreen()`, not `Navigator.push(MaterialPageRoute(...))`. The new screen mixes in `SwipeToCloseMixin` and uses `AppBarBackButton(onPressed: handleClose)`.
> - **Phase 4 / theming + a11y:** all colors come from `AppColors.xxx(context)`; all animations wrap durations in `accessibleDuration(context, ...)`; semantic labels and tooltips read from `a11y*` ARB keys (separate from the content `eventShareLanding*` keys). The `scripts/a11y_allowlist.txt` is empty on `main`, so any new violation must be fixed in the same PR.
> - **Phase 4 / caching:** `ShareLinkRepository` uses `CacheService(cacheManager, 'share_link')` with item keys `'share_link:<short_code>'` and `invalidate(shortCode)` on revoke.
> - **Phase 4 / i18n:** every new ARB key lands in both `app_en.arb` and `app_es.arb` with `@key` metadata; `npm run lint:dart:l10n-parity` is the enforcement gate.
> - **All client tests use `--reporter expanded`** per Flutter testing memory.

## Problem Statement

Today every shareable invite is a `CommunityInvitationLink`: it points at exactly one thing — a community — and there is no other kind of share link in the system. The parent plan (#1195) wants a single shortcode to encode "what to do" as well as "who to do it with" so that, in v1, a recipient can RSVP to a specific event in the link target instead of just joining the community. The product roadmap already has the other shareable item types on the whiteboard — gear give-aways, transfers (loans + giveaways are both `Transfer` rows in this codebase), pinned-request shares — so the data layer needs to support all of them today even though only the community + event branches get exercised in v1.

This sub-issue does the data-layer move only: replace `CommunityInvitationLink` with a polymorphic `ShareLink` row that carries a `oneof target` discriminator covering **every** current shareable entity type (community, gear, transfer, request, experience), migrate existing rows in place, keep the four existing community-invite RPCs wire-compatible, add a new `GetOrCreateEventShareLink` RPC, and add a minimal event-aware native deep-link landing screen that the v1 web work in sub-issues 2-3 will piggyback on. No user-visible web surface yet.

The parent plan for #1195 specifies the storage shape direction (Decision 4) and the migration intent ("idempotent, single transaction"). This document extends that proto sketch from a two-variant oneof to a five-variant oneof — trivial to do today, costly to add later — and decomposes the work into concrete tasks that name the files, RPC contracts, indexes, backfill, and deep-link UI affected.

## Context from Comments

The issue has no comments of its own. All design context lives in the parent plan `docs/issues/1195-web-guest-rsvp.md`, specifically Decision 4 (ShareLink shape), Compliance H (proto conventions), Compliance G (storage indexes), Compliance F (file split), Compliance D/E (logging/error wrapping), and Compliance I (testing layers). The task list in the GitHub issue body is a direct extract from that document.

One thing worth flagging: an issue with no discussion is normally a signal to verify alignment before writing code. The parent plan is detailed and recent, so the source of decisions is well-anchored — but any disagreement on the open questions below should land in the issue's comments before implementation starts.

## Codebase context (from investigation)

Findings that shape the plan:

- **`CommunityInvitationLink` is defined in `proto/ripls/models/community.proto:203-235`.** Eight fields. The parent plan's sketch is missing two of them: `shadow_user_id` (`optional string`, for shadow-account merge during accept) and `deleted` (`optional DeletedMetadata`, for soft-delete cascade). `ShareLink` must keep parity or migration loses data.
- **Four RPCs all live in one file: `server/services/community/invitations.go`** (`GetOrCreateInviteLink`, `RevokeInviteLink`, `AcceptInvitationLink`) plus `server/services/login/registration.go:261-359` (`CheckInvitation`). All four call a single helper `lookupInvitation()` at `server/services/community/invitations.go:372-382`, which is the only short-code → row resolver in the codebase. Centralized; easy to swap.
- **`GetOrCreateInviteLinkRequest` already accepts `gear_id`, `request_id`, and `experience_id`** (`proto/ripls/api/community_service.proto:280-307`), but today those values are *transport-only* — they flow through to the response URL as query params and are not persisted on the `CommunityInvitationLink` row. That existing contract is a useful constraint: we can keep the same request fields and start *actually storing* the target without a wire break.
- **`/go/{code}` is served by `server/services/web/service.go:105` (`HandleInvitePage`)**, which calls `lookupInvitation()` then renders `templates/invite.html`. Branching on `oneof target` to choose between an invite template and an event template happens in sub-issue 2; this sub-issue must leave the existing community-only branch byte-identical.
- **Protosql auto-creates tables from proto messages** registered in `server/storage/protosql_schema.go:250-313` (`TypeConfig` entries) and handles oneof fields by skipping unset variants (`protosql_schema.go:207`, `:418`). Indexes are *not* declared from proto annotations — they're imperative `CREATE INDEX IF NOT EXISTS …` statements in `protosql_schema.go:395-435`. Several indexes need to land in that block in this sub-issue (one per oneof variant, plus a unique-on-short_code). Partial indexes are the established pattern (`protosql_schema.go:411` for `community_event.request_id WHERE request_id <> ''`), which is exactly what each oneof variant needs.
- **`CommunityEvent.invitation_short_code`** (referenced by `CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED`, `proto/ripls/models/community.proto:240`) is the audit-trail link from the join event back to the invitation. It carries the short code, not the row id, so it survives the schema change without proto changes — but the assertion that the audit event still resolves to the right `ShareLink` row is a regression test.
- **Migration / backfill pattern.** The closest analog is `server/jobs/community_cascade_backfill.go` (iterates a table, applies a per-row mutation, idempotent). MEMORY.md's "Impact Estimation Phase 6" notes also point at `server/jobs/impact_backfill.go` as a working template. There is no dedicated migration framework — backfill jobs run as startup goroutines from `main.go`, gated by idempotency (skip rows that look already-migrated). The `ShareLink` backfill follows the same shape.
- **Native deep-link handler is `app/lib/core/router/app_router.dart:221-303`** (`/go/:code` → redirect to `/invite?token=…&experience_id=…`). The route already reads `experience_id`; registration + post-accept paths pass it through but never display it. An "event-aware landing screen" is purely a new screen + an `if shareLink.hasExperienceId()` branch in the post-lookup builder.
- **All five target entity types already exist in the codebase:** `Community`, `Gear`, `Transfer`, `Request`, `Experience` — each with stable `id` fields under `proto/ripls/models/`. None require new storage to be the target of a ShareLink; the oneof variants are pure foreign-key strings into existing tables.

## Potential Solutions

Two meaningful forks. The first is about how to *get rows into the new table*. The second is about how to handle the native-app event-aware landing UI that the issue tasks call for.

### Option A: New `share_link` table + one-shot backfill, drop legacy table later

Create a brand-new `share_link` table from the new proto. On server startup, run a backfill job that iterates `community_invitation_link` and inserts mirroring `ShareLink` rows with `community_invite_id` set. The four existing RPCs read/write `share_link` going forward; reads of `community_invitation_link` happen only inside the backfill job. The legacy table is dropped in a follow-up issue (already filed as #2056) once the new table is verified in production.

- **Pros:** Cleanest separation. Atomic cutover at process restart. Idempotent backfill is easy — skip if the row's `short_code` is already in `share_link`. Easy to roll back: flip the RPCs back to read the legacy table without touching data. Lines up with the established backfill goroutine pattern from `impact_backfill.go` / `community_cascade_backfill.go`.
- **Cons:** Two tables exist simultaneously for a window (one release cycle). Care needed that no other code path writes to the legacy table during the transition.
- **Effort:** Medium. The backfill is straightforward; the larger surface is the four RPC rewrites.

### Option B: Rename `community_invitation_link` to `share_link`, add columns

Use a SQL `ALTER TABLE RENAME` to rename the existing table and add the new oneof columns. Protosql has a `TableName` field in `TypeConfig`, so registering `ShareLink` with `TableName: "share_link"` plus a one-shot rename SQL in `protosql_schema.go` could cover it.

- **Pros:** Zero data movement. Single transaction. No window where two tables coexist.
- **Cons:** Protosql currently has no ALTER-RENAME path; we'd be inventing one. Tests use testcontainers with fresh DBs, so any rename logic only exercises on real deploy — riskier. Rollback requires another rename. Doesn't compose cleanly if a future sub-issue introduces a third storage shape.
- **Effort:** Medium-low for the rename itself, but high for confidence — no existing migration tooling to lean on, and the failure mode (orphaned table on a partial migration) is bad.

### Option C: Native-app event-aware UI in sub-issue 1 vs. defer to sub-issue 3

The issue tasks list both the data-layer move *and* the native-app event-aware deep-link landing screen. The parent plan in `docs/issues/1195-web-guest-rsvp.md:283` justifies this with "same Flutter widgets as the post-RSVP Plan-claim screen from sub-issue 3 to keep the codebase tight." Sub-issue 3's widgets don't exist yet, so building them here means either inventing them now (and then sub-issue 3 has to integrate, not build) or shipping a *minimal* event-aware screen here that sub-issue 3 replaces.

- **C1 (build the widgets here):** Ship the full event hero + time + place + host + RSVP CTA card now. Sub-issue 3 reuses it in the post-RSVP confirmation. Widget gets stress-tested once.
- **C2 (minimal screen here, full screen in sub-issue 3):** Stripped event-aware screen here, just enough to prove the routing works, with a TODO pointing at sub-issue 3 for the polished version.
- **C3 (defer entirely to sub-issue 3):** Sub-issue 1 keeps the existing community-only landing for everything. The native app routes an event-flavored shortcode through today's `AcceptInvitationLink` path (which already accepts an experience_id). Event-aware UI moves entirely into sub-issue 3.

## Recommendation

**Option A + Approach C2.**

For migration: a new `share_link` table with a one-shot startup backfill is the lowest-risk path. The backfill follows the established `community_cascade_backfill.go` / `impact_backfill.go` patterns, and the testcontainers-based test setup actually exercises the cutover (every test starts with an empty DB and goes through both creates). The cost — a single release window where two tables coexist — is a price we routinely pay for storage shape changes elsewhere. Option B's ALTER-TABLE-RENAME path is faster on production but requires inventing migration tooling the codebase doesn't yet have; that's a foundation worth building, but not as a side-effect of this sub-issue.

For the native-app UI: a minimal event-aware landing screen here (C2) is the right amount of work for sub-issue 1. The data layer needs the deep-link path to do *something* observable when `oneof target = experience_id` arrives, otherwise the migration is untestable end-to-end on a physical device — a Compliance I gap. But building the full Plan-claim widget tree this far ahead of sub-issue 3's actual UI work risks designing widgets without their real surrounding context. The minimal screen reads "Event invitation: {name} hosted by {host}. Open in app to RSVP." with a `TODO(#1195-sub3)` and a single button that routes to the existing event-detail screen. Sub-issue 3 replaces it with the polished version that shares the real Plan-claim widgets.

**On the data model:** include all five oneof variants today — `community_invite_id`, `gear_id`, `transfer_id`, `request_id`, `experience_id`. Adding a oneof variant later is wire-compatible, but adding it *now* costs nothing extra (a few proto field numbers + one partial index per variant) and avoids the trap of designing for v1 and discovering at v2 that we picked a shape that doesn't generalize. Per the existing `feedback_prefer_generic_primitives` memory: when the other use cases are already on the roadmap, generic > special-case. Per Compliance H, only the variants whose RPCs ship in this sub-issue (community + event) get *populated*; the rest are reserved-shape-only until their RPCs land.

The remaining open questions below cover the smaller decisions inside this recommendation.

## Open Questions

- **Q1: Should `ShareLink` mirror `CommunityInvitationLink`'s `shadow_user_id` and `deleted` fields, or are those legacy?** — Recommendation: keep both, verbatim, with the same semantics. `shadow_user_id` is referenced in the existing acceptance flow and removing it is a separate concern. `deleted` (soft-delete metadata) is required for cascade-delete parity — the existing `cascade_delete.go` helpers walk `community_invitation_link` and need rewiring otherwise. Carrying them forward is a one-line cost; removing them is a separate sub-issue.
- **Q2: Where does the backfill run — startup goroutine in `main.go`, or one-shot script?** — Recommendation: startup goroutine, consistent with `impact_backfill.go` and `community_cascade_backfill.go`. Idempotent backfills as goroutines is the established codebase pattern; a one-shot script needs its own deploy story.
- **Q3: Field numbers for the five oneof variants?** — Recommendation: keep `community_invite_id = 5` (matches the parent plan's Decision 4 sketch so the wire shape doesn't drift between docs), then `gear_id = 6`, `transfer_id = 7`, `request_id = 8`, `experience_id = 9`. The parent plan calls field 6 `event_id`; per the project's "experience in code, event in UI" rule (`CLAUDE.md`), the proto field is `experience_id`. Flagged for the implementer — a fix-up to the parent plan doc should follow.
- **Q4: Does `CreateEventShareLink` need a "get-or-create" version like `GetOrCreateInviteLink`, or is "create fresh each time" acceptable?** — Recommendation: get-or-create, per `(inviter_id, experience_id)`. Matches the existing pattern (`GetOrCreateInviteLink` is `(inviter_id, community_id)`-keyed) and avoids accumulating dead share links every time the host taps Share. The issue body names the RPC `CreateEventShareLink`, but the precedent is stronger — rename to `GetOrCreateEventShareLink` for symmetry. Decision: ship as `GetOrCreateEventShareLink` in this sub-issue; mention the rename in the PR description.
- **Q5: Uniqueness constraint shape?** — Recommendation: one share link per `(inviter_id, populated_target_variant)`. Encoded as partial unique indexes at the SQL level — one per oneof variant — using the project's existing partial-index pattern from `protosql_schema.go:411`. Example: `CREATE UNIQUE INDEX … ON share_link (inviter_id, experience_id) WHERE experience_id <> ''`. Five partial unique indexes total, one per oneof variant.
- **Q6: When a parent entity (Community, Experience, Gear, Transfer, Request) is soft-deleted, what happens to its `ShareLink` rows?** — Recommendation: cascade-delete share links for any soft-deleted target, matching the existing `cascade_delete.go` behavior. Add a `CascadeDeleteShareLinksBy*` helper per oneof variant in the same PR. The five branches are mechanical and small.
- **Q7: Does the migration delete `community_invitation_link` rows after copying, or leave them in place?** — Recommendation: leave them in place for the rollback window. Soft-delete the legacy rows after backfill confirms each one was migrated (mirrors the existing soft-delete + indexed `deleted_at` pattern). The legacy-table drop is tracked separately in #2056.
- **Q8: Does sub-issue 1 also ship `GetOrCreate*ShareLink` RPCs for gear/transfer/request, since their storage variants land in the proto?** — Recommendation: no. The data shape supports them; the RPCs ship with the sub-issues that exercise them. Leaving the API surface tight today avoids dead RPCs in the wire interface. The proto definitions for those oneof variants serve as a published intent, not a contract — clients can't construct rows of those shapes without a server RPC.

## Implementation Plan

Five phases. Phases 1–3 are server-only and can ship behind a feature flag if desired. Phase 4 is the native-app deep-link UI. Phase 5 is regression and the legacy-cleanup hooks (cleanup itself is tracked in #2056).

### Phase 1: ShareLink proto + protosql registration

**Goal:** New table exists in dev/test; no production reads or writes yet.

**Tasks:**
- [ ] Create `proto/ripls/models/share_link.proto` with the message shape below. All five oneof variants populated today; comments name the entity each variant references without naming any specific consumer service (Compliance H).

  ```proto
  message ShareLink {
    string id = 1;
    string short_code = 2;
    string inviter_id = 3;
    string community_id = 4;            // the community the share link is scoped to (always set)

    oneof target {
      string community_invite_id = 5;   // value mirrors community_id; row is a plain community invite
      string gear_id = 6;
      string transfer_id = 7;
      string request_id = 8;
      string experience_id = 9;
    }

    bool is_revoked = 10;
    int64 created_at_unix_sec = 11;
    optional string shadow_user_id = 12;       // carried over from CommunityInvitationLink
    optional DeletedMetadata deleted = 13;     // carried over from CommunityInvitationLink
  }
  ```
- [ ] Per `CLAUDE.md`, do **not** add a corresponding API-side message yet. RPC-side request/response types land in Phase 3 with dedicated shapes (Compliance H — no reuse). Field comments in the new `.proto` file describe semantics for downstream clients only — no vendor names, no transport plumbing, no server-side mechanics, no internal sentinel values, per the proto-comment conventions in `CLAUDE.md`.
- [ ] Register `ShareLink` in `server/storage/protosql_schema.go` (`TypeConfig` block at `:250-313`) with `TableName: "share_link"`.
- [ ] Add the index block to `protosql_schema.go:395-435`:
  - `CREATE UNIQUE INDEX IF NOT EXISTS idx_share_link_short_code ON share_link (short_code)`
  - `CREATE INDEX IF NOT EXISTS idx_share_link_community ON share_link (community_id)`
  - One partial index per oneof variant: `CREATE INDEX IF NOT EXISTS idx_share_link_<variant> ON share_link (<variant>) WHERE <variant> <> ''` for each of `community_invite_id`, `gear_id`, `transfer_id`, `request_id`, `experience_id`. Pattern matches `idx_community_event_request_id` at `protosql_schema.go:411` (Compliance G).
  - Five partial unique indexes per Open Question 5: `CREATE UNIQUE INDEX IF NOT EXISTS idx_share_link_inviter_<variant> ON share_link (inviter_id, <variant>) WHERE <variant> <> ''` for each variant.
- [ ] Run `npm run generate`; confirm `server/gen/ripls/models/share_link.pb.go` and `app/lib/data/gen/ripls/models/share_link.pb.dart` materialize.
- [ ] `npm run build` (Go) and `flutter analyze` (client) clean.

**Validates:** New table is created by testcontainer startup; protosql schema initialization passes; no behavior change.

### Phase 2: Backfill job — copy CommunityInvitationLink rows into ShareLink

**Goal:** Existing community invites are mirrored into `share_link` with `community_invite_id` set. Original rows untouched.

**Tasks:**
- [ ] Create `server/jobs/share_link_backfill.go` modeled on `server/jobs/community_cascade_backfill.go` and `server/jobs/impact_backfill.go`. Run as a startup goroutine in `main.go` via `logging.GoSafe(ctx, "share_link_backfill", fn)`.
- [ ] Top of the file: a `TODO(#2056):` block (Compliance: `CLAUDE.md` "TODO Comments") naming what the code does today, what would let the TODO be removed, and the constraint that blocked doing it now. Roughly:

  ```go
  // TODO(#2056): Delete this backfill job once #2048's ShareLink rollout has been
  // verified in production for ≥1 release. The job mirrors legacy
  // community_invitation_link rows into share_link with community_invite_id set so
  // that the new RPC paths can read share_link exclusively. It is intentionally
  // idempotent (lookup-by-short_code skip) so it remains safe across restarts. We
  // kept it instead of an ALTER-TABLE-RENAME because the codebase has no migration
  // framework today and we wanted a reversible cutover for the first polymorphic
  // share-link rollout.
  ```
- [ ] Iteration: `ListAll(ctx, &models.CommunityInvitationLink{}, QueryOptions{IncludeDeleted: true})`. For each row, check whether a corresponding `ShareLink` exists (lookup by `short_code` — guaranteed unique). If absent, insert a `ShareLink` with `community_invite_id` set to `community_id` (per Decision 4: "value mirrors community_id"), carrying over `id`, `short_code`, `inviter_id`, `community_id`, `is_revoked`, `created_at_unix_sec`, `shadow_user_id`, and `deleted` verbatim. One row per transaction (keeps idempotency obvious; revisit batching if the legacy table grows past 10k rows).
- [ ] Log levels per `docs/server/observability.md`: `Info` for each insert (normal operation worth recording), `Debug` for skips (verbose detail), `Warn` for malformed legacy rows that the backfill skips, `Error` for storage failures (system failure requiring attention). All log lines use `logging.LoggerWithContext(ctx)` with `operation: "share_link_backfill"`, masked short codes via `logging.MaskToken`, and structured `community_id` / `share_link_id` fields. Entity-ID field names match the standard table in `observability.md` (no `request_id` for any entity — only the HTTP correlation ID uses that key).
- [ ] Test: `server/jobs/share_link_backfill_test.go` with cases for (a) empty legacy table, (b) populated legacy table with mix of revoked / soft-deleted / active rows, (c) re-running the job (idempotency), (d) partial prior migration (some rows already in `share_link`).
- [ ] Integration test in `server/main_integration_test.go` exercising the migration on a populated test database — populate `community_invitation_link`, start the server, assert `share_link` has parity rows (Compliance I).

**Validates:** Existing data lifts cleanly into the new table; rerunning is a no-op; #2056 is the single tracked cleanup destination.

### Phase 3: Rewrite the four existing RPCs on top of ShareLink storage + add GetOrCreateEventShareLink

**Goal:** `GetOrCreateInviteLink`, `RevokeInviteLink`, `AcceptInvitationLink`, `CheckInvitation` read and write `share_link` underneath. Wire contracts unchanged. `GetOrCreateEventShareLink` ships as a new RPC.

**Tasks:**
- [ ] Replace `lookupInvitation()` in `server/services/community/invitations.go:372-382` with a `lookupShareLink()` helper that queries `share_link` by `short_code`. Add an internal `requireCommunityInviteTarget(*models.ShareLink) error` helper that returns `InvalidArgument` if the row's `oneof target` is not `community_invite_id` — keeps the existing four RPCs scoped to community invites only. Event-flavored share links go through `GetOrCreateEventShareLink` (this phase) and `AcceptEventInvite` (sub-issue 3).
- [ ] Update `GetOrCreateInviteLink` (`invitations.go:24-150`) to write into `share_link` with `community_invite_id` set. The existing request's optional `gear_id`/`request_id`/`experience_id` fields stay valid and the validation logic at `:43-56` stays — but per Open Question 8, this RPC does **not** branch into creating non-community share-link rows. Those branches return `connect.CodeUnimplemented` with a clear message pointing at the future per-target RPCs. (Today's community-only behavior unchanged.)
- [ ] Update `RevokeInviteLink` (`invitations.go:153-200`) to mark the `share_link` row revoked. Non-community-invite rows refused.
- [ ] Update `AcceptInvitationLink` (`invitations.go:203-315`) to read the `share_link` row, refuse non-community-invite rows with `connect.CodeInvalidArgument` ("event share links accepted via AcceptEventInvite — see #1195 sub-issue 3"), and otherwise follow the existing membership + event-emission path unchanged.
- [ ] Update `CheckInvitation` in `server/services/login/registration.go:261-359` similarly. Login service does the short-code lookup against `share_link` via its own direct storage access (matches today's pattern at `registration.go:273`) — it must not call into the community service. This preserves service independence per `docs/server/architecture.md` "Core Design Principle 1".
- [ ] Extend `CheckInvitationResponse` (`proto/ripls/api/login_service.proto`) with a new optional field that surfaces which `oneof target` variant is populated on the looked-up `ShareLink`. Wire-compatible additive change; dedicated request/response types per Compliance H. The Dart client uses this to branch between the community-invite landing screen (today's flow) and the event-aware landing screen added in Phase 4. Today's behavior continues to surface community + inviter metadata; later sub-issues extend the response with event-specific fields.
- [ ] Update `server/services/web/service.go:105` (`HandleInvitePage`) to look up via `share_link`, branch on `oneof target`, and for now only render the existing `templates/invite.html` for `community_invite_id` rows. Event-flavored rows render a minimal `templates/event_invite_stub.html` saying "Open the app to RSVP" with a Universal/App Link — the actual SSR event landing template ships in sub-issue 2. Non-community / non-event variants render a "this link is not yet supported on the web" stub. Logging masks the short code via `logging.MaskToken` per Compliance D.
- [ ] Add `GetOrCreateEventShareLink` (per Open Question 4) in a new file `server/services/community/share_links.go` (Compliance F). Dedicated request/response types in `proto/ripls/api/community_service.proto` (Compliance H — no reuse from `GetOrCreateInviteLink*`). Verifies the caller is a member of the hosting community, and verifies the `Experience` exists and is in the same community **via direct storage access** (`s.storage.GetByID(ctx, "experience", id, &models.Experience{})`) — the community service must not call the experience service per `docs/server/architecture.md` "Service Independence". Then `GetOrCreate`s a `share_link` row with `experience_id` set.
- [ ] Internal errors go through `connecterr.Internal(ctx, "<op>.<step>", err, ...)` (Compliance E). User-visible errors (caller not a member, event doesn't exist, capacity refused) use `InvalidArgument` / `FailedPrecondition` and never wrap with `Internal`.
- [ ] Log levels in the new RPC follow `docs/server/observability.md`: `Info` for normal RPC entry/exit, `Warn` for handled user errors (caller not a member, event doesn't exist), `Error` only for system failures (storage / external-service errors). `Warn` (not `Error`) on user-input refusals — otherwise these pollute the auto-filed-issue pipeline described in `observability.md` Part 4.
- [ ] Update all test files that referenced `CommunityInvitationLink` directly — searches in `server/integration_tests/`, `server/cmd/seed-purge-fixture/`, `server/storage/cascade_delete*.go`. Most switch to `ShareLink`; the seed/fixture inserts rows directly, so it carries both legacy and new inserts during the transition (legacy inserts removed in the #2056 cleanup, marked with `TODO(#2056):`).
- [ ] Cascade-delete (Open Question 6): extend `server/storage/cascade_delete.go` with helpers that walk `share_link` for soft-deleted `Community`, `Experience`, `Gear`, `Transfer`, and `Request` rows. The community-side cascade subsumes the existing `CommunityInvitationLink` cascade; the four target-side cascades are new branches that defend against orphaned share links even though the populated branches today are only `community_invite_id` (post-backfill) and `experience_id` (post-`GetOrCreateEventShareLink`).
- [ ] Tests:
  - Unit tests next to each new/modified file (Compliance I).
  - Integration test in `server/main_integration_test.go` covering: existing community-invite creation/acceptance/revocation flow works end-to-end against `share_link`; `GetOrCreateEventShareLink` writes the correct row and is idempotent on `(inviter_id, experience_id)`; cascade-delete from `Community` and `Experience` cleans up share links; `CommunityEvent.invitation_short_code` audit trail resolves to the correct `ShareLink` row.
  - Regression test: hit `/go/{code}` with a known community-invite shortcode, assert the response is byte-identical to today's `invite.html` (golden file).
  - `storage.AssertMaxQueries(t, ctx, max, fn)` budget on `/go/{code}` rendering (Compliance G — target ≤5 queries).
- [ ] `npm run build` and `go build -tags=integration ./...` clean (Compliance: integration build).

**Validates:** Production behavior for community invites unchanged; new event-flavored share links can be created via RPC; cascade-delete and audit-trail invariants hold.

### Phase 4: Native-app event-aware deep-link landing (minimal version)

**Goal:** When the installed app receives `/go/{code}` for an event-flavored share link, it shows a minimal event-aware screen instead of the generic community accept-invitation spinner.

**Tasks:**
- [ ] **Service layer.** Add a `ShareLinkService` (Connect client wrapper) under `app/lib/services/` that exposes the existing `CheckInvitation` RPC for share-link inspection. Provided via Riverpod `Provider<ShareLinkService>`. ViewModels never call this directly per `docs/client/architecture.md` ("Repositories are the ONLY layer that calls Services").
- [ ] **Repository layer.** Add `ShareLinkRepository` under `app/lib/data/repositories/`. It depends on `ShareLinkService` and uses `CacheService(cacheManager, 'share_link')` per `docs/client/caching.md`. Item key: `'share_link:<short_code>'`. Methods: `Future<ShareLinkInfo> describe(String shortCode)` (cached); `Future<void> invalidate(String shortCode)` (called on revoke or accept). `ShareLinkInfo` is a UI-friendly model that surfaces the oneof variant + community + event metadata that the extended `CheckInvitationResponse` carries.
- [ ] **Screen.** Add `EventShareLinkLandingScreen` under `app/lib/presentation/screens/share_link/` (new directory; add `README.md` per the multi-file-directory rule). Mixes in `SwipeToCloseMixin` per `docs/client/architecture.md` Navigation & Gestures; uses `buildSwipeableScaffold(appBar: AppBar(leading: AppBarBackButton(onPressed: handleClose)), body: ...)`.
- [ ] **ViewModel.** Use `AsyncNotifierProvider.autoDispose` per `architecture.md`'s "Default Pattern: Use AsyncNotifier/FutureProvider" — `build()` calls `ref.read(shareLinkRepositoryProvider).describe(shortCode)` and returns the resolved `ShareLinkInfo`. Any methods that manually set state (e.g., a refresh hook) include `if (!ref.mounted) return;` checks after every `await` before `state = ...`. Freezed state class for any non-`AsyncValue` fields.
- [ ] **Routing.** Update `app/lib/core/router/app_router.dart:244-282` (`/invite` builder): after the share link is resolved (via the new repository), if the target oneof variant is `experience_id`, call `NavigationHelpers.pushScreen()` to push `EventShareLinkLandingScreen` with the experience id, hosting community id, and short code — **not** `Navigator.push(MaterialPageRoute(...))` or `context.push()` per `architecture.md` Navigation & Gestures. If `community_invite_id`, keep today's behavior (`acceptInvitationLink` → community home). For unsupported variants (gear/transfer/request — reserved-only today), render a friendly "this link type isn't supported in your app version yet" message with an "update app" CTA.
- [ ] **Screen content.** Event hero via `CachedMediaImage(semanticsLabel: context.l10n.a11yEventHeroForName(eventName))` — `semanticsLabel` is required by the `require_semantic_label_on_cached_media_image` lint rule (Compliance C). Event name + time + place, host name + hosting community name, and a single "RSVP — open RSVP screen" CTA that pushes onto the existing event-detail route. Add a `TODO(#1195-sub3):` for the polished post-RSVP widgets (file the sub-issue 3 ticket first per the standing TODO convention so the number is real).
- [ ] **Theming.** All colors via `AppColors.xxx(context)` per `architecture.md` "Colors and Theming" — no hardcoded `Colors.white`, `Colors.grey[*]`, or `Color(0xFF...)` literals. Card backgrounds use `AppColors.cardBackground(context)`; text uses `AppColors.textPrimary/textSecondary(context)`.
- [ ] **i18n.** All strings via `context.l10n.*` per `docs/client/i18n.md`. Two prefix groups added to both `app_en.arb` and `app_es.arb` with `@key` metadata: `eventShareLanding*` for user-facing content strings (the user-visible "event" terminology — never "experience" — per the `CLAUDE.md` UI terminology rule), and `a11y*` for `semanticsLabel` / `tooltip` strings per the accessibility doc's a11y key conventions. `npm run lint:dart:l10n-parity` enforces both locales carry every key.
- [ ] **Accessibility primitives.** All taps use `Tappable` (not `GestureDetector` / `InkWell`); the RSVP CTA — if it has a tap-to-toggle aspect — uses `Toggle`; any icon-only control uses `IconAction` with both `semanticsLabel:` and `tooltip:` (both required by `require_semantic_label_on_icon_button`). Any inline status announcement (e.g., "Loading event details…") wraps in `LiveRegion` or fires `SemanticAnnouncer.announce`. Any `Duration(...)` on `AnimatedX` / `AnimationController` is wrapped in `accessibleDuration(context, ...)`. **The `scripts/a11y_allowlist.txt` is empty on `main`** — any new violation must be fixed in the same PR; the allowlist may not grow.
- [ ] **Code generation.** Run `flutter pub run build_runner build` after adding `@GenerateMocks` annotations to test files; run `npm run generate` after ARB and proto changes.
- [ ] **Tests.** Per `docs/client/testing.md`:
  - ViewModel test under `app/test/presentation/viewmodels/event_share_link_landing_view_model_test.dart` using `ProviderContainer` with `shareLinkRepositoryProvider.overrideWithValue(mockRepo)`. Cover: initial load (cache miss), error state (RPC fails), and the unsupported-variant path. Use `flutter test --reporter expanded`.
  - **Disposal test required** per `architecture.md` "Disposal Testing": start the async load, dispose the container before completion, assert no thrown error.
  - Repository test under `app/test/data/repositories/share_link_repository_test.dart`: cache hit (no service call), cache miss (service call + cache populated), invalidate-then-refetch.
  - No widget tests required; client-side integration tests are out of scope (Patrol removed in #1887 per `testing.md`).

**Validates:** An event-flavored shortcode arriving via Universal/App Link surfaces an event-aware UI on a physical device.

### Phase 5: Regression + cleanup hooks

**Goal:** Confidence the existing community-invite flow is unchanged in production; legacy-table drop is one well-defined follow-up away (already tracked in #2056).

**Tasks:**
- [ ] End-to-end regression: from a clean test DB, populate `community_invitation_link` with mixed rows (active, revoked, soft-deleted), start the server, run the backfill, and verify that every legacy row's user-observable behavior (`/go/{code}` rendering, `CheckInvitation`, `AcceptInvitationLink`) is identical pre- and post-migration. Sub-test in `server/main_integration_test.go`.
- [ ] Any code that still touches the legacy table (the seed fixture, cascade-delete branches, the backfill itself) carries `TODO(#2056):` markers with the inline-actionable context CLAUDE.md requires. No silent legacy-table references — every one must point at the cleanup issue.
- [ ] Manual canary plan documented in the PR description: deploy to dev → verify migration log lines → smoke-test invite acceptance → verify `share_link` row counts match `community_invitation_link` → deploy to prod → repeat. Reviewers know what production verification looks like before approving.

**Validates:** Production cutover is reversible; the legacy-table drop is one tracked issue away with no orphaned references.

## Related Issues

- **#1195** — Parent plan ("Web UI for shared event planning that allows non-members to participate"). This sub-issue is the data-layer foundation for sub-issues 2-5 of that plan.
- **#2056** — Tracked cleanup of the ShareLink backfill job and the legacy `community_invitation_link` table once #2048 is verified in production. Filed alongside this plan so every `TODO(#2056):` reference has a real target from day one.
- **#1195's other sub-issues (to-be-filed):** sub-issue 2 (Web build target + SSR event landing), 3 (Core actions — RSVP yes/maybe/no + Plan claim), 4 (Host views + at-cap UX), 5 (Rate limits + approval-queue schema). None should start until this sub-issue lands.
- **#690** — Existing TODO marker convention from the invitation_token → short_code rename. The proto-deprecation pattern from that work is the model for any deprecated-but-still-referenced field comments in this sub-issue.
- **#1620** — Daily purge job. Backfill jobs in this codebase share that issue's idempotency conventions.
