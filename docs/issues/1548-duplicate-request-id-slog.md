# Issue #1548: Duplicate `request_id` slog attribute causes concatenated 72-char IDs in Cloud Logging

**Source:** <https://github.com/Type2Fun-Foundation/ripls/issues/1548>
**Filed:** 2026-04-28T20:57:20Z | **Last activity:** 2026-04-29T21:03:44Z
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Production log entries are emitting `request_id` values that look like two
UUIDs glued together with no separator (~72 characters), e.g.
`5d01f6bd-da42-4371-8729-13501d4b63453944c905-3525-4829-a62a-09f41d693a3b`.
The data on disk is fine — this is a logging artifact only — but it breaks
every downstream consumer that filters, pivots, or alerts on `request_id`
(Cloud Logging dashboards, log-based metrics, log groupings).

The root cause is a key collision in `slog`. Two different concepts both
end up under the same `"request_id"` attribute on the same log record:

1. **HTTP correlation ID.** `server/middleware/request_id.go` reads the
   `X-Request-ID` header (or generates a UUID), stores it in the context
   via `logging.WithRequestID`, and `LoggerWithContext` adds it to every
   logger derived from the request context (`server/logging/logger.go:294`).
2. **`Request` entity ID.** RPC handlers in `server/services/request/*` and
   neighboring packages chain `.With("request_id", req.Msg.RequestId, ...)`
   on the context-derived logger to record which `models.Request` is being
   acted on (e.g. `server/services/request/offers.go:34`,
   `lifecycle.go:251`).

`slog`'s JSON handler emits both attributes verbatim, producing a JSON
object with two `"request_id"` keys. RFC 8259 leaves duplicate-key
behavior implementation-defined; Cloud Logging concatenates the values,
which is what we see.

## Context from Comments

The only comment is maintainer pinging `@claude` to triage. The issue
body itself is thorough: it identifies the cause, names the colliding call
sites, suggests two possible new keys (`target_request_id`,
`req_msg_request_id`), proposes a regression test, and explicitly
distinguishes this from #1547 (the offer-help bug whose log lines first
exposed this artifact). Acceptance criteria are:

1. No production log entries with `request_id` longer than 36 characters.
2. Handlers that need the RPC-payload request id use a distinct key.
3. Dashboards / alerts that filter on `request_id` continue to work, or
   are migrated with a note in the PR.

No design ambiguity from the comments — the author has already chosen the
shape of the fix (rename the entity-level usage). Our job is to pick the
new key name, find every site, and add a regression guard.

## Potential Solutions

### Option A: Rename the entity-level key (issue author's proposal)
- **Approach:** Leave the HTTP correlation `request_id` alone. Rename the
  ~80 call sites in `server/services/` and `server/jobs/` that log a
  `models.Request` ID to a new key. Two sub-choices for the name:
  - `target_request_id` — issue author's first suggestion. Emphasizes
    that this is the *target* of the action. Generic and applies
    uniformly to every site that currently logs `req.Msg.RequestId` /
    `request.Id`.
  - `community_request_id` — would *read* like the existing
    `gear_id` / `experience_id` / `transfer_id` pattern, but is actively
    misleading: there is already a `models.CommunityRequest` join entity
    (request-↔-community), so a future reader seeing
    `community_request_id` would assume it refers to that join row, not
    the underlying `models.Request`. Reject.
  - `req_msg_request_id` — issue author's alternate. Exposes "msg" /
    Connect-RPC plumbing in a logging field name. Reject.
- **Pros:** Acceptance criterion (3) is satisfied with zero work — every
  Cloud Logging dashboard / alert that filters on `request_id` keeps
  working unchanged. The HTTP correlation ID is by far the more
  load-bearing query key in ops use. Smallest blast radius for ops.
- **Cons:** ~80 call sites across ~20 files to update. The new key is
  slightly awkward (entity is `Request`, but `request_id` is taken). New
  contributors will keep wanting to write `request_id` and will need to
  be steered.
- **Effort:** Medium (mechanical edit + a guard).

### Option B: Rename the HTTP correlation key
- **Approach:** Rename the middleware-level key to `correlation_id` (or
  `http_request_id`). Touches ~5 production call sites
  (`server/logging/logger.go:174,294`, `server/middleware/logging.go:54`,
  `server/middleware/recovery.go:40`, `server/middleware/query_stats.go:36`)
  plus a handful of tests. All ~80 entity-level call sites remain
  `request_id`, which is the natural Go-convention name for a
  `models.Request` ID (parallels `gear_id`, `experience_id`, etc.).
- **Pros:** The eventually "right" naming — entity IDs are uniformly
  `<entity>_id`. No collision risk for any future entity field.
- **Cons:** **Breaks every downstream Cloud Logging consumer** that
  queries on `request_id` to correlate log lines for a single HTTP
  request. We would need to inventory and migrate dashboards and alerts
  before merging, which is exactly what the issue author was trying to
  avoid. Higher coordination cost than the larger code change.
- **Effort:** Small in code, medium-to-large in ops migration.

### Option C: Detect the collision instead of preventing it
- **Approach:** Add a runtime guard in `LoggerWithContext` that strips
  any caller-supplied `"request_id"` from a `.With(...)` chain when the
  logger already has one from context, and a unit test that fails on
  duplicate keys. Or: make `LoggerWithContext` use a fixed
  pre-allocated key set and reject collisions.
- **Pros:** No call-site renames needed. Self-healing.
- **Cons:** Either silently drops information the caller intended to
  log (worse than the original bug — now `request_id` is sometimes the
  HTTP one, sometimes the entity one, depending on order), or panics at
  runtime. Doesn't solve the underlying ambiguity. Ergonomic mess.
- **Effort:** Small, but the result is fragile.

## Recommendation

**Option A with the new key `target_request_id`.**

The issue author has already signaled the direction; respect that. Among
the sub-choices, `target_request_id` is the right pick:

- It does not collide with the existing `models.CommunityRequest` join
  entity. (`community_request_id` would read like an ID for that entity,
  which it isn't — it's a `models.Request` ID.)
- It applies uniformly to every site we'd rename: in each one the
  `models.Request` is the *subject the RPC operates on*. "Target"
  captures that semantics across `OfferToFulfill`, `MarkFulfilled`,
  `CancelRequest`, `SubmitRequestNeeds`, etc. without forcing us to
  vary the key by handler.
- The "subject vs target" distinction floated earlier doesn't bite in
  practice: there are no log lines in `server/services/request/` or
  `server/jobs/` that log a `Request` ID as anything but the action's
  subject. If one ever appears, the appropriate key is still
  `target_request_id` — the action being logged is operating on it.

Options B and C are worse: B punts the work onto the ops team and risks
silently breaking alerting, and C trades a logging artifact for a
silent-data-loss footgun.

To prevent recurrence we add a small unit test in
`server/logging/logger_test.go` that exercises the
`LoggerWithContext(ctx).With("request_id", ...)` path and asserts the
emitted JSON contains exactly one `request_id` field. We also add a
lint-style test that walks the AST of `server/services/` and
`server/jobs/` and fails if any string-literal `"request_id"` appears as
the *first* element of a `.With(...)` call following
`logging.LoggerWithContext` — a tight, opt-in regression guard.

## Open Questions

- **Pick `target_request_id`, `community_request_id`, or
  `req_msg_request_id`?** — Recommendation: `target_request_id`. Avoids
  collision with the existing `models.CommunityRequest` join entity,
  doesn't expose RPC plumbing in the field name, and applies uniformly
  to every renamed site. A reviewer who prefers a different name can
  flip it; the rename is mechanical and string-search driven.
- **Are there map-style `slog.Group`-style log sites that also need
  renaming?** — Recommendation: **no.** I initially flagged sites like
  `server/services/request/offers.go:76,90,161,254,277,298`,
  `lifecycle.go:319,533,727,780`, `helpers.go:33,70`, and `gen.go:427`
  as candidates because they pass `map[string]any{"request_id": ...}`
  literals — but on closer reading every one of those is a
  `storage.QueryByFields` filter (SQL column → value). They are *not*
  log attributes. The Phase 1 inventory has been updated to label each
  site LOG vs. SQL; SQL sites stay as `"request_id"` (column name).
  If a future map literal really does feed into a logger
  (`logger.Info("...", slog.Group("...", ...))`-shape), the regression
  guard in Phase 2 will catch it.
- **Update `docs/server/observability.md` field table?** —
  Recommendation: yes. Add a row for `target_request_id` next to the
  existing `request_id` row, and clarify in the prose that `request_id`
  is the HTTP correlation ID specifically. The current table at
  `docs/server/observability.md:104-113` lists `request_id` and an
  "entity-specific fields follow patterns: `gear_id`, `community_id`,
  `transfer_id`, `location_id`, `media_id`" line — the new convention
  should be explicit there.
- **Backfill or migrate the bad logs already in Cloud Logging?** —
  Recommendation: no. The bad data is small, time-bounded, and only
  affects log triage — not a production user surface. Note in the PR
  body that historical entries before the merge will still show 72-char
  IDs.
- **Do call sites that log a `request_id` *without* first calling
  `LoggerWithContext` need to be renamed?** A handful of sites do
  `storage.QueryByField(ctx, "request_id", ...)` — that's a SQL column
  name, not a log key, and stays unchanged. Test files that
  *construct* a logger themselves and add `"request_id"` (e.g.
  `server/logging/logger_test.go`) test the HTTP-correlation key and
  also stay unchanged. — Recommendation: limit the rename to (a)
  log-attribute keys whose value is a `models.Request` ID, and (b)
  identify each site by checking that the variable being passed is
  `req.Msg.RequestId`, `request.Id`, `requestID` (locally derived from
  one of the above), or `topic.RequestId` from a chat conversation
  topic. SQL column names and tests that exercise the HTTP key stay as
  `"request_id"`.
- **Web handler `server/services/web/item_preview.go:44,111`** logs a
  `request_id` from a query-string parameter that is itself the
  `models.Request` entity ID. — Recommendation: rename the *log*
  attribute (line 111) to `target_request_id`. Leave the
  query-string parameter name (`?request_id=...` on line 44) unchanged
  — that's a public URL contract.

## Implementation Plan

### Phase 1: Rename entity-level log key in `server/services/` and `server/jobs/`
**Goal:** Eliminate the duplicate key in production logs.

The inventory below was hand-verified by reading each site. Sites
labeled **LOG** are real log-attribute keys (passed to `.With(...)`,
`.Info(...)`, `.Warn(...)`, `.Error(...)`, etc. on a `*logging.Logger`)
and need renaming to `"target_request_id"`. Every other `"request_id"`
literal in the same files is a SQL column name passed to
`QueryByField` / `QueryByFields` / `QueryByFieldIn` /
`deleteByFieldIn` / `loadContributions`, a URL query-string parameter,
or a `proto/...` field accessor — those stay unchanged. The
implementation agent should re-verify each site before editing (line
numbers may shift if other PRs land first).

**Tasks:**
- [ ] Rename `"request_id"` → `"target_request_id"` at these LOG sites:
  - `server/services/request/offers.go`: lines 34, 228 (LOG). Lines
    76, 90, 161, 254, 277, 298 are `QueryByFields` SQL keys — leave.
  - `server/services/request/lifecycle.go`: lines 93, 100, 109, 123,
    145, 162, 192, 198, 251, 489, 597, 688, 765 (LOG). Lines 319,
    533, 727, 780 are `QueryByFields` SQL keys — leave.
  - `server/services/request/contributions.go`: lines 33, 89, 152,
    206 (all LOG).
  - `server/services/request/needs.go`: lines 38, 110, 161, 252, 339
    (LOG). Line 413 is a `QueryByFields` SQL key — leave.
  - `server/services/request/sharing.go`: lines 30, 151 (LOG). Lines
    51, 172 are `QueryByFields` SQL keys — leave.
  - `server/services/request/preview.go`: line 32 (LOG). Line 71 is
    a `QueryByField` SQL key — leave.
  - `server/services/request/queries.go`: lines 29, 209, 291 (LOG).
    Line 65 is a `QueryByField` SQL key — leave.
  - `server/services/request/stats.go`: line 30 (LOG). Lines 47, 81,
    91 are `QueryByField`/`QueryByFields` SQL keys — leave.
  - `server/services/request/people.go`: line 29 (LOG). Line 62 is a
    `QueryByFields` SQL key — leave.
  - `server/services/request/stock_imagery.go`: line 20 (LOG).
  - `server/services/request/gen.go`: line 397 (LOG). Line 427 is a
    `QueryByFields` SQL key — leave.
  - `server/services/request/events.go`: lines 53, 73, 92 (all LOG).
  - `server/services/request/helpers.go`: line 22 (LOG). Lines 33, 70
    are `QueryByFields` SQL keys — leave. Line 59 is a `QueryByField`
    SQL key — leave.
  - `server/services/request/undo.go`: lines 109, 195 (LOG). Lines
    90, 176, 205 are `QueryByField` SQL keys — leave.
  - `server/services/chat/conversations.go`: lines 246, 660 (LOG).
    Line 127 is a `QueryByField` SQL key — leave.
  - `server/services/chat/media_append.go`: line 177 (LOG).
  - `server/services/chat/messages.go`: line 358 (LOG).
  - `server/services/feed/service.go`: line 871 (LOG). Lines 344, 395
    are `QueryByFieldIn` SQL keys — leave.
  - `server/services/web/item_preview.go`: line 111 (LOG). Line 44 is
    a public URL query-string parameter — leave.
  - `server/jobs/impact_backfill.go`: line 486 (LOG).
  - `server/jobs/community_cascade_backfill.go`: line 98 (LOG).
- [ ] Confirm these "no-touch" sites stay `"request_id"`:
  - SQL: `server/services/admin/cleanup.go:187`,
    `server/services/portfolio/fetch.go:315`,
    `server/services/feed/stories.go:289` (column-name dispatch
    helper), `server/services/impact_metrics/authorization.go:77`,
    `server/services/impact_metrics/drafting.go:244`.
  - Proto/URL: `server/services/community/invitations.go:131`
    (URL query parameter for share links),
    `server/services/web/item_preview.go:44` (URL query parameter),
    `server/services/web/templates/invite.html:232` (HTML/JS query
    parameter).
- [ ] Update assertion-side tests where they assert on the
  entity-level log key:
  - `server/services/request/offers_test.go`: lines 281, 368 (test
    fixture maps describing log payloads — flip to
    `"target_request_id"`). Line 616 asserts the connect error
    message contains `"request_id"` — leave (error message wording
    is a separate concern).
  - `server/services/request/sharing_test.go`: lines 58, 112, 193,
    246, 259 (flip to `"target_request_id"`).
  - Any other test that constructs an expected log-payload map
    against a renamed handler (re-grep after the rename).
- [ ] Run `npm run build` and `go test -tags=integration -timeout 20m
  ./server/services/... ./server/jobs/... ./server/logging/...`. Fix
  any failures.

**Validates:** No more duplicate-key emissions from the most-trafficked
paths; existing tests still green.

### Phase 2: Add regression guard
**Goal:** Prevent the collision from creeping back in.
**Tasks:**
- [ ] Add `TestLoggerWithContextNoDuplicateRequestID` to
  `server/logging/logger_test.go`. Build a context with
  `WithRequestID`, call `LoggerWithContext`, chain
  `.With("request_id", "entity-id")`, log a record, parse the JSON
  output, and assert that exactly one `request_id` key is present and
  its value matches the *first* one set (so the test pins the bug to
  "duplicate keys are forbidden," not to a specific resolution).
- [ ] Add `TestNoEntityRequestIDLogKey` (location:
  `server/logging/lint_test.go` or alongside the existing test) that
  walks `server/services/` and `server/jobs/` with `go/parser` /
  `go/ast` and fails for any string literal `"request_id"` that
  appears in an attribute-key position on a logger call. Concretely:
  - For `.With(args...)` / `.WithGroup(args...)`: every
    even-indexed (0, 2, 4, ...) string-literal argument is an
    attribute key; flag if any equals `"request_id"`.
  - For `.Info(msg, args...)` / `.Warn(...)` / `.Error(...)` /
    `.Debug(...)` and their `*Context` variants: skip the first
    argument (the message) and apply the same rule to the rest.
  - Restrict the receiver-type check to `*logging.Logger` /
    `*slog.Logger` (or just match by package — anything imported as
    `logging` in the file is in scope).
  The test should accept a small allowlist file
  (`server/logging/request_id_log_key_allowlist.txt`) for the handful
  of intentional HTTP-correlation usages, but populate it with only
  the middleware paths if any leak through; ideally the allowlist
  stays empty after Phase 1.
- [ ] Run `npm run build` and `go test ./server/logging/...`. Fix any
  failures the guard catches in code that wasn't on the Phase 1 list.
**Validates:** Future PRs cannot reintroduce the collision; the test
suite documents the rule.

### Phase 3: Update docs and call out the migration
**Goal:** Make the new convention discoverable.
**Tasks:**
- [ ] Update `docs/server/observability.md` (lines 100-113):
  - In the standard-fields table, clarify the `request_id` row reads
    "HTTP correlation ID (X-Request-ID header or auto-generated UUID)".
  - Add a `target_request_id` row immediately below it: "ID of a
    `models.Request` that an RPC is operating on".
  - Update the "entity-specific fields follow patterns" line to
    include `target_request_id` next to `gear_id`, `community_id`,
    `transfer_id`, `location_id`, `media_id`, and add a one-line note
    that the entity ID for `models.Request` uses `target_request_id`
    (not `request_id`) to avoid collision with the HTTP correlation
    key.
- [ ] Update `CLAUDE.md` under "Structured Logging (Go Server)" /
  "Standard field names" to add `target_request_id` to the entity IDs
  list and add a one-line note that `request_id` is reserved for the
  HTTP correlation key.
- [ ] In the PR body, list:
  - the rename (`request_id` → `target_request_id` for entity IDs);
  - that `request_id` continues to mean HTTP correlation ID and
    Cloud Logging dashboards / alerts on `request_id` keep working;
  - that historical log entries before the merge will still contain
    72-char concatenated values and won't be backfilled.
**Validates:** Anyone reading the docs / CLAUDE.md after this lands
arrives at the new convention without re-deriving it.

## Related Issues
- #1547 — The OfferToFulfill 404 bug whose logs first surfaced these
  concatenated IDs. Independent root cause (stale community context on
  the client); plan in `docs/issues/1547-offer-help-stale-community.md`.
