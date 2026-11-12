# Issue #1529: [security] No per-resource auth check in GetMedia

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1529
**Filed:** 2026-04-28 | **Last activity:** 2026-04-28
**Labels:** `auto-filed`, `review:security` | **Assignees:** unassigned
**Context docs:** none (issue body + linked `docs/reviews/reports/security_review.md` are sufficient)

## Problem Statement

`GetMedia` (`server/services/media/service.go:80-170`) authenticates the caller but performs **no per-resource authorisation**. Any authenticated user who knows or guesses a media id can call `GetMedia(id)` and receive:

- a presigned bucket URL good for ~35 minutes,
- a presigned thumbnail URL,
- the uploader's name + avatar (the inline `Uploader` block at lines 151-161),
- stock-imagery `Attribution` if applicable.

`DeleteMedia` in the same file (lines 199-202) enforces ownership; `GetMedia` does not. UUIDv4 ids make brute-force enumeration impractical, but ids reach logs, share-link payloads, error reports, and third-party crash collectors — anywhere a leaked id ends up, the privacy boundary collapses to "knowing the id."

The right model is *capability follows surface*: a user can read a media if the user has any legitimate path to discovering its id — owner, community member where it's shared, chat participant, or any user whose avatar this is.

## Context from Comments

Single triage comment from maintainer on 2026-04-28: raised to P1 with rationale "security High: missing per-resource auth check in `GetMedia`." No design discussion, no contested points. The recommendation in the issue body itself ("owner OR member of community where the media is surfaced (gear / experience / chat / profile)") is the design direction.

## Codebase Findings

(Confirmed by code search — file:line refs follow.)

### Where the gap is

- Handler: `server/services/media/service.go:80-170`. Calls `auth.RequireAuth` then `s.storage.GetByID(ctx, req.Msg.Id, mediaRecord)` and proceeds straight to the presign step with no ownership or membership check.
- Contrast `DeleteMedia` at `server/services/media/service.go:199-202`: explicit `media.UserId != authInfo.UserID` → `connect.CodePermissionDenied`.

### Unauthenticated media flows (must not regress)

`GetMedia` already calls `auth.RequireAuth` at the top, so unauthenticated callers are rejected today. But two distinct surfaces show media to *unauthenticated* viewers — and we need to confirm they don't go through `GetMedia`, because the new per-resource rule would deny everyone (no `userID` to check membership against).

1. **Web invite preview pages** (`/go/{code}`). Server renders an HTML preview with Open Graph `<meta>` tags so social crawlers (Slack, Twitter, Signal, etc.) unfurl the link with a thumbnail. Implementation:
   - Handler registration at `server/main.go:1371` (marked "public, no auth required").
   - Page render entry at `server/services/web/service.go:213`.
   - Media-URL minting at `server/services/web/item_preview.go:236-263` (`getMediaImageURL`) and `server/services/web/service.go:262-285` (`getCommunityImageURL`). Both call `s.bucket.GetSignedURL(ctx, bucketKey, ogImageDuration)` directly to mint a 1-hour presigned GCS URL, embed it in the response, and the crawler / browser fetches it from GCS with no further server involvement.
   - **Does not call `GetMedia`.** The presigned URL ships in the rendered HTML; nothing on the unauthenticated side ever hits the RPC.

2. **`CheckInvitation` RPC** (unauthenticated; called during signup to render the "you've been invited to X" screen before the user has an auth token).
   - Handler registration at `server/main.go:1482-1483` — no auth wrapper (comment: "required for registration/login").
   - RPC handler at `server/services/login/registration.go:261-353` — no `RequireAuth` call.
   - Mints a presigned community-avatar URL via the same `getCommunityImageURL` helper at `server/services/login/registration.go:556-580` and returns it in the response.
   - **Does not call `GetMedia`.** Same shape as the web flow — server mints the URL at response time.

**Implication for this design:** the new `GetMedia` rule is orthogonal. Neither unauth surface routes through `GetMedia`, so tightening the RPC doesn't break them. The rule those flows follow today is "the *link-creating* code path is the gatekeeper — it decides which media is safe to expose, mints the URL, and embeds it." That stays the right model.

**Design rule (load-bearing, repeat in code comment on the new helper):** *Any future unauthenticated media flow must mint its presigned URL server-side at link/render time, like the two above. It must NOT route the viewer through `GetMedia` — there is no `userID` available, and the helper has no logic for unauthenticated callers. If a future ticket needs unauth viewing of an arbitrary media id, the right shape is a new dedicated RPC (e.g., `GetMediaForShareLink(shortCode, mediaId)`) that authorises against the share-link capability, not the user.*

**Acceptable asymmetry to call out:** an unauthenticated user holding a valid share link will see media for which an *authenticated* non-member would be denied. That's correct — the share link is the capability proof for the unauth viewer; an authenticated stranger has no equivalent proof. If, after registration, a former unauth recipient becomes a community member (the expected accept-invite flow), `GetMedia` starts working for them as a normal authenticated member. There is no flow today where an authenticated non-member receives a media id via these unauth surfaces and would expect `GetMedia` to succeed on it.

### Where media is surfaced (proto + storage)

- `User.media_id` (`proto/ripls/models/user.proto:62`) + `User.media_ids[]` (line 69) — avatars.
- `Gear.media_ids[]` (`proto/ripls/models/gear.proto:35`).
- `Experience.media_ids[]` (`proto/ripls/models/experience.proto:34`).
- `Request.media_ids[]` (`proto/ripls/models/request.proto:45`).
- `Community.media_ids[]` (`proto/ripls/models/community.proto:33`).
- `Story.media_ids[]` (`proto/ripls/models/story.proto:29`) — has direct `community_id` field on the row (not a join table).
- `Message…user_message.media_ids[]` (`proto/ripls/models/chat.proto:113`) — chat attachments.

Gear / Experience / Request reach a Community via join rows: `CommunityGear`, `CommunityExperience`, `CommunityRequest`. These are many-to-one *per share* — a single gear can be shared to multiple communities, one join row per (community, gear) pair. Story is the odd one out — `community_id` lives directly on the row.

### Storage-layer capabilities

- Repeated proto fields land as Postgres `TEXT[]`. Containment query is raw SQL `WHERE $1 = ANY(field_name)`. Pattern confirmed at `server/storage/chat_conversation.go:104` (the `ListByParticipant` query).
- GIN indices exist on at least one such field already (`server/storage/chat_conversation_schema.go:82-84`).
- `ProtoSQLStorage.QueryByField` does **not** expose a generic "array contains" verb — for `media_ids` containment we'll need either a dedicated helper method (e.g., `QueryByArrayContains(field, value, model)`) or raw SQL in the access helper.

### Existing patterns to mirror

- **`auth.IsMemberOfCommunity(ctx, storage, communityID, userID) (bool, error)`** at `server/auth/community.go:14-39`. The canonical "is this user in this community?" check.
- **`requireConversationAccess(ctx, conversationID, userID)`** at `server/services/chat/helpers.go:12-54`. The right architectural template — it short-circuits on direct participation, then falls through to a per-community check for cross-community topics. This is exactly the hierarchical-fallthrough shape we want for media.

### Indices we'll touch (or need)

To keep the access check fast, every `media_ids` column we probe needs a GIN index. Worth auditing in Phase 1 (and adding migrations where missing). Same for `User.media_ids`.

## Potential Solutions

### Option A: Inline access check inside GetMedia

- **Approach:** Open-code the access rules inside the `GetMedia` handler, similar to how `DeleteMedia` open-codes the owner check.
- **Pros:** Smallest diff; no new files.
- **Cons:** The full rule set is ~5 branches and several SQL probes. Inlining buries the security logic in a 170-line handler, makes it hard to unit-test, and guarantees we'll never reuse it for the other read endpoints (`ListMedia`, `GetSignedURL`-style) we'll need eventually.
- **Effort:** small surface, but worse for long-term maintenance.

### Option B: Dedicated authorization helper — recommended

- **Approach:** New file `server/services/media/authorization.go` (file name matches the convention documented in `docs/server/architecture.md` § "File naming convention" — service-specific auth helpers live in `authorization.go`). Two exported entry points:
  - `canUserAccessMedia(ctx, storage, userID string, media *models.Media) (allowed bool, reason string, err error)` — pure predicate used in tests and Phase 1 log-only mode. `reason` captures which rule matched (or `"deny"`) for telemetry.
  - `requireMediaReadAccess(ctx, storage, userID string, media *models.Media) error` — thin wrapper used by the handler in Phase 2. Calls the predicate and returns `connect.NewError(connect.CodePermissionDenied, …)` on `false, _, nil`. Mirrors the `requireOwnership` / `requireConversationAccess` shape documented in `docs/server/architecture.md` § "Authorization Patterns" — Option 2.
- **Short-circuit order (cheapest first):** owner → avatar (single `User` probe — high-throughput surface) → community direct (`Community.media_ids` containment, no join) → story (direct `community_id` field, no join) → gear / experience / request (each via its `Community*` join row) → chat (`Message.media_ids` containment then `ChatConversation.participant_ids` check).
- **Pros:** Testable in isolation. Reusable when other read endpoints land. Mirrors `requireConversationAccess` (the right architectural model — confirmed at `server/services/chat/helpers.go:12-54`). Each branch is one SQL probe or fewer for the common cases.
- **Cons:** New file, one more indirection in the handler. Modest test-fixture cost (need a community/gear/conversation built per test, but `setupTestStorage` already supports this in `service_test.go`).
- **Effort:** medium — most of the work is the SQL probes + tests, not the wiring.

### Option C: Token-bound access — capability tokens issued by the *surfacing* endpoints

- **Approach:** `GetMedia` requires a signed capability token alongside the media id. The token is minted by `GetGear`, `ListMessages`, `GetCommunity`, etc., proving the bearer reached the media through a legitimate surface. `GetMedia` validates the token instead of reasoning about cross-table state.
- **Pros:** No cross-table SQL in the read path; the surfacing endpoint already did the auth work. Conceptually clean.
- **Cons:** Substantial protocol change — every media-rendering surface in the client has to thread a new token. Breaks any cached `GetMedia` calls that don't have a token (every existing client release). Doesn't match how the codebase has done access checks anywhere else. Bigger change for the same security outcome.
- **Effort:** large.

## Recommendation

**Option B.** It's the right shape for this codebase: a focused helper that mirrors the existing `requireConversationAccess` pattern, leaves the handler readable, and is unit-testable without spinning up the full RPC stack. Option A's "save 30 lines" doesn't pay for the cost of making the rule untestable; Option C is a bigger protocol change than the issue justifies.

The most important design choice inside Option B is the **rollout shape**: rather than ship full enforcement in one step (and risk a deny-storm if any access path is missed), the helper lands in *log-only* mode first. The handler still allows the request, but if the helper would have denied, we log a structured warning. After a day of production traffic, we read the logs, confirm zero legitimate denials, and flip to enforce. See Phase 1 / Phase 2 below.

## Open Questions

- **Q1: Does `ProtoSQLStorage` need a generic `QueryByArrayContains` helper, or do we drop raw SQL into `access.go`?**
  — **Recommendation:** add a small helper. The pattern (`WHERE $1 = ANY(text_array_col)`) already exists in `server/storage/chat_conversation.go:104` for `participant_ids`; we'll repeat it for `media_ids` in 5+ models. A reusable `QueryByArrayContains(ctx, field, value, model)` method on `ProtoSQLStorage` is ~15 lines, makes the helper readable, and saves the next person from re-inventing it. File the helper in `server/storage/` alongside the existing query verbs.

- **Q2: What's the avatar rule — anyone-can-fetch-anyone's-avatar, or only co-community-members?**
  — **Recommendation:** anyone can fetch any avatar. Avatars surface in cross-community contexts (search results, who-RSVPed, notification senders) and tightening this introduces an auth check on the highest-throughput surface in the app. The CLAUDE.md issue itself groups "profile" with the public-ish surfaces. If telemetry later shows this is a leak vector (e.g., an unbounded scraping pattern), we'd revisit then; not now.

- **Q3: Do we need GIN indices on every `media_ids` column we'll probe?**
  — **Recommendation:** yes, audit + add as needed in Phase 1. Without them, every non-owner GetMedia turns into 5 sequential scans. Migration cost is small; carrying it forward is not.

- **Q4: How do we want to handle media that legitimately has no owner record reachable (e.g., system-owned imports)?**
  — **Recommendation:** check by `mediaRecord.UserId` first; if the owner check fails and *no other rule matches*, deny. There's no current code path that creates ownerless media — stock-imagery copies go through `CopyStockImageForUser` and end up owned by the importer (verified for `AddMediaFromURL` in PR #2062). If a future surface needs anonymous media (e.g., system-published assets), it can add an explicit `is_public` flag on the media row.

- **Q5: Does the helper signature take `*models.Media` or just `mediaID`?**
  — **Recommendation:** `*models.Media`. The handler already fetched the row for its own use; passing it in saves a duplicate `GetByID` and gives the helper access to `media.UserId` without a query. Wrap the load + check in a tiny `loadAndAuthorize` if we end up using it from more than one handler later.

- **Q6: Does any existing unauthenticated flow break under the new rule?**
  — **Recommendation:** No. Confirmed by audit (see "Unauthenticated media flows" above): the two unauth surfaces — web `/go/{code}` previews and the `CheckInvitation` RPC — both mint presigned URLs server-side at response time and never call `GetMedia`. The new rule is orthogonal. A regression test belongs in Phase 1 anyway: assert that an unauthenticated caller still gets `Unauthenticated` (not `PermissionDenied`) from `GetMedia` — same outcome as today, but pinned so a future refactor can't silently flip it.

## Implementation Plan

### Phase 1: Helper + log-only mode

**Goal:** Ship the access logic and observe what it would do, without changing user-visible behaviour.

**Tasks:**
- [ ] Add `ProtoSQLStorage.QueryByArrayContains(ctx, field string, value string, prototype proto.Message) ([]proto.Message, error)` in `server/storage/`. Implementation: `SELECT … WHERE $1 = ANY(field) AND deleted_at IS NULL`. Cover with a unit test against the test-container Postgres. (Per `docs/server/architecture.md` § "Storage Best Practices": "Don't write raw SQL in services. Add new storage methods for complex queries.")
- [ ] Audit existing GIN indices on `media_ids` columns (Gear, Experience, Request, Community, Story, Message, User). Add migration for any that are missing — mirror `chat_conversation_schema.go:82-84`.
- [ ] Create `server/services/media/authorization.go` with two functions:
  - `canUserAccessMedia(ctx, storage, userID, media) (allowed bool, reason string, err error)` — pure predicate. `reason` captures which rule matched (`"owner" | "system" | "avatar" | "community" | "story" | "gear" | "experience" | "request" | "chat"` plus per-surface deny codes; see `server/services/media/authorization.go`). The `system` rule was added in #2081 to allow stock images (`UserId == media.SystemUserID`); it sits between owner and avatar as the second zero-SQL short-circuit.
  - `requireMediaReadAccess(ctx, storage, userID, media) error` — wrapper that returns `connect.NewError(connect.CodePermissionDenied, …)` on deny. Used by the handler in Phase 2; not used in Phase 1 (which calls the predicate directly).
  Short-circuit order, cheapest first:
  1. Owner — `media.UserId == userID`. Zero SQL.
  2. Avatar — `QueryByArrayContains("media_ids", media.Id, &User{})` returns any row, OR a `User` with `media_id == media.Id` exists. Single probe.
  3. Community direct — `QueryByArrayContains("media_ids", media.Id, &Community{})`; for each hit, `auth.IsMemberOfCommunity(community.Id, userID)`.
  4. Story — `QueryByArrayContains("media_ids", media.Id, &Story{})`; for each hit, `auth.IsMemberOfCommunity(story.CommunityId, userID)`.
  5. Gear — `QueryByArrayContains("media_ids", media.Id, &Gear{})`; for each gear, query `CommunityGear` rows by `gear_id`, then `auth.IsMemberOfCommunity` on each.
  6. Experience — same shape as Gear via `CommunityExperience`.
  7. Request — same shape as Gear via `CommunityRequest`.
  8. Chat — `QueryByArrayContains("media_ids", media.Id, &Message{})`; for each message, fetch the `ChatConversation` and check `participant_ids`.
  On no match, return `false, "deny", nil`. On storage error, return `false, "", err` and let the handler map via `connecterr.Internal` (do not return `PermissionDenied` from the helper for a system failure).
- [ ] Wire into `GetMedia` at `server/services/media/service.go:103` in **log-only mode**: call `canUserAccessMedia` directly, log a `would_deny` warning if `allowed == false && err == nil`, but still allow the request through. Use the standard field names from `docs/server/observability.md` § "Standard Field Names":
  - `operation=GetMedia` (already set on the logger above)
  - `media_id` (already set)
  - `media_owner_id` (matches `DeleteMedia:200`'s shape)
  - `reason=<rule>` from the helper
  - `phase=log-only`
  Logger context already carries `user_id` and masked `user_email` from `auth.RequireAuth`, so don't re-attach them on the log line.
- [ ] Unit tests in `server/services/media/authorization_test.go`. Use table-driven cases per `docs/server/architecture.md` § "Testing Strategy":
  - owner allowed (rule short-circuits at step 1 — assert zero SQL via `storage.AssertMaxQueries`)
  - user's own avatar allowed; another user's avatar allowed (Q2 rule)
  - co-community member via Gear / Experience / Request / Story / Community-direct (one case per surface)
  - chat participant allowed; non-participant denied
  - unrelated user denied
  - soft-deleted gear / soft-deleted community-user → denied (don't let stale rows grant access — covers the read-filter contract from #1619)
  - storage error inside the helper bubbles up as `(false, "", err)`, not `(false, "deny", nil)`
  - **unauthenticated caller** (no `auth.RequireAuth` populated context) still receives `Unauthenticated` from the handler — pins the existing behaviour so a future refactor can't accidentally route an unauth caller through the new helper. The two documented unauth surfaces (`/go/{code}`, `CheckInvitation`) don't go through `GetMedia`; this test guards that invariant.
- [ ] `npm run build` (covers `golangci-lint` + `check_codeinternal.js` + `check_goroutines`) + `go test ./...`. CI green before merge.

**Validates:** The access logic is correct on the test corpus. Production log-only telemetry tells us whether the rule covers every real access path before we enforce.

### Phase 2: Flip to enforce

**Goal:** Turn deny on. This is the actual security fix.

**Tasks:**
- [ ] Read 24-48 hours of log-only output. Confirm `would_deny` count is ~0 per legitimate user (some scraping / probing is fine; large rates from real users indicate a missed access path).
- [ ] If would-deny rate is non-zero on any real surface, fix the missing access rule in the helper and re-soak in log-only mode. Do not skip this step.
- [ ] Flip the handler from `canUserAccessMedia` to `requireMediaReadAccess`. The wrapper returns `connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not authorized"))` directly — `PermissionDenied`'s error string is not gated by `check_codeinternal.js` (only `CodeInternal` is, per `docs/server/architecture.md` § "Error wrapping"). Mirror `DeleteMedia:201`'s shape exactly.
- [ ] Log fields stay the same except `phase=enforce` and the message becomes `unauthorized read attempt` (matching `DeleteMedia:200`'s `WarnContext` level — WARN is the right level per `docs/server/observability.md` § "Appropriate Log Levels": "unexpected but handled situations" — not ERROR, since this is a user-visible auth denial, not a system failure).
- [ ] Update the issue's own acceptance check by writing the requested test: "a non-participating user receives `PermissionDenied`." This already lands as part of the Phase 1 test suite — confirm it's present and asserting the right code.
- [ ] Close #1529.

**Validates:** The leak is closed. `DeleteMedia` and `GetMedia` now have parity on resource-level auth.

### Phase 3 (deferred — file as follow-up if Phase 2 telemetry warrants)

**Goal:** Performance hardening if the SQL count from the access check becomes a problem.

**Tasks (only if needed):**
- [ ] Per-request memoisation of access decisions — the same `media_id` is often fetched multiple times per RPC chain (e.g., gear card + thumbnail + avatar in one response build).
- [ ] Consider a denormalised `media_access` cache row if telemetry shows the joined query is a hot path.

**Validates:** GetMedia stays cheap at scale.

## Out of Scope (file as follow-ups when Phase 2 ships)

- **Avatar rule hardening** — restricting "anyone can fetch any avatar" to "uploader and requester share at least one community." Adds an auth check to the highest-throughput render in the app; defer until we have a signal it's a real leak vector (Q2).
- **Caching / per-request memoisation** — `GetMedia` is on the hot path; if Phase 2 log counts show the SQL probes adding noticeable latency, add a memo per request context (Phase 3 above).
- **`ListMedia` / other read endpoints** — the helper is designed to be reusable, but no other media-read endpoint exists today; this is a follow-up if and when one lands.

## Related Issues / Context

- **PR #2062** — Replace Media candidates. Introduced `AddMediaFromURL`, which creates media rows owned by the importing user. Those rows fall under rule 1 (owner) of this access model; no special-case needed.
- **#1953** — Image upload sanitization. Independent; #1953 governs *what* gets stored, #1529 governs *who can read it*. Both are security-review-filed P1s.
- **#1766** — Security umbrella (deferred to 2026-08-20). #1529 is the kind of work that umbrella tracks but is not a sub-issue of it.
- **`docs/reviews/reports/security_review.md`** — the source review that filed #1529.

---

## Audit notes (added by `/ripls-audit` on 2026-05-20)

Reviewed against `docs/server/architecture.md` and `docs/server/observability.md`. Client docs (`docs/client/*.md`) were not consulted — this is a server-only change with no client surface.

Changes applied to the plan:

- **File naming.** Renamed `access.go` → `authorization.go` to match `docs/server/architecture.md` § "File naming convention" (service-specific auth helpers live in `authorization.go`). Same rename on the test file.
- **Two-function shape.** Added `requireMediaReadAccess(ctx, …) error` as a thin wrapper around the predicate `canUserAccessMedia`. Mirrors the documented `requireOwnership` / `requireConversationAccess` pattern (architecture doc § "Authorization Patterns" — Option 2). The handler in Phase 2 calls the wrapper; Phase 1's log-only path calls the predicate directly so it can observe-and-allow.
- **Storage layer boundary.** Made it explicit that `QueryByArrayContains` lives in `server/storage/`, not as raw SQL in the helper. Citation added to architecture doc § "Storage Best Practices".
- **Log field hygiene.** Spelled out which standard field names the log line uses (`media_id`, `media_owner_id`, `reason`, `phase`, plus the ambient `user_id` / `user_email` already injected by `auth.RequireAuth`). Removed the implicit duplication of `user_id` on the log call. Added an explicit citation to `docs/server/observability.md` § "Standard Field Names".
- **Log level.** Phase 2's `unauthorized read attempt` is `WarnContext`, not `ErrorContext` — auth denial is a "user error" per `docs/server/observability.md` § "Appropriate Log Levels" ("User errors (invalid input, not found) are INFO or WARN, not ERROR. ERROR is reserved for system failures").
- **Error wrapping.** Clarified that `PermissionDenied` doesn't go through `connecterr.Internal` — the `check_codeinternal.js` gate only governs `CodeInternal`. The helper's wrapper uses `connect.NewError(connect.CodePermissionDenied, …)` directly, matching `DeleteMedia:201`. System failures *inside* the helper (storage errors during a probe) DO go through `connecterr.Internal` at the handler layer.
- **Short-circuit ordering.** Re-ordered the rule chain so direct-field probes (Community, Story) come before join-table probes (Gear, Experience, Request). Owner and avatar stay first/second since they're the highest-throughput cases. Doesn't change correctness; trims expected SQL on the common cross-user paths.
- **Test fixtures.** Recommended table-driven test cases (architecture doc § "Testing Strategy") and added a specific case for "soft-deleted gear / community-user → denied" — this is the contract from #1619's read-filter work that we need to be sure the new helper inherits. Added a case for "storage error bubbles up cleanly" so the helper's error semantics are pinned.
- **`npm run build` coverage.** Noted that the build command transitively runs `check_codeinternal.js` and `check_goroutines` — relevant because the helper introduces new storage probes that must respect both gates.

**No changes needed in other areas:**
- The plan's recommendation (Option B) already matches the documented service-architecture pattern (auth helpers live in the service package; storage layer carries the array-containment verb).
- Observability shape (WARN on auth denial, standard field names, `LoggerWithContext` propagation, no plaintext emails) was correct in the original plan; this audit just made the citations explicit.
- The avatar rule (Q2) is documented as a deferrable tradeoff; the audit does not override it.
