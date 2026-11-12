# Issue #2665: Activity digest emails: categorize actions, measure true daily actives, and identify ad-hoc communities

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2665
**Filed:** 2026-07-07 | **Last activity:** 2026-07-07
**Labels:** enhancement, ops, P2 | **Assignees:** unassigned
**Context docs:** `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/proto_conventions.md`, `docs/users/priorities.md`, `docs/users/personas.md` (skimmed), `docs/server/observability.md` (skimmed), `docs/ad_hoc_communities.md`, `docs/registration_and_login.md`, `docs/server/l10n.md` (admin-email exemption), review checklists skimmed: security, performance, observability

> **UI investigation:** skipped. This issue concerns a server-rendered internal
> ops email (Mailgun, English-only per `docs/server/l10n.md` → "Admin emails
> are English-only"); nothing is observable in the app UI. Rendering is
> verified offline via `server/cmd/preview-activity-digest`.

## Problem Statement

The daily/weekly ops activity digests (`server/activity_digest/`, rendered in
`server/email/activity_digest_render.go` + templates, scheduled by
`server/jobs/activity_digest{,_weekly}.go`) have three insight gaps plus a
handful of smaller defects:

1. **Ambiguous "events" and uncategorized actions.** The digest uses "events"
   both for Experiences (correct — the UI Terminology rule renders Experiences
   as "Events") and for raw `CommunityEvent` log rows ("Community events: N",
   per-community "N events / N messages"). Worse, `applyEventToTotals`
   (`server/activity_digest/builder.go`) classifies only 22 of 46
   `CommunityEventType` values; 8 UNDONE retractions are deliberately excluded
   but **16 types are silently uncounted** (all six `PLANNING_*`,
   `TRANSFER_INTEREST_EXPRESSED/WITHDRAWN`, `TRANSFER_RECIPIENT_SELECTED`,
   `TRANSFER_PICKUP_PROPOSED`, `REQUEST_OFFER_SELECTED/WITHDRAWN`,
   `EXPERIENCE_STARTED/UPDATED/ROSTER_CHANGED`, `COMMUNITY_NAMED`). They still
   inflate the totals line and per-community counts, so the ACTIVITY section
   visibly fails to reconcile, and key signals — planning activity, ad-hoc →
   named promotions — are invisible. Nothing stops the next enum value from
   falling into the same gap.

2. **"Returning sign-ins" is credential churn, not people.**
   `CountDistinctSignInsInWindow` counts distinct users with `refresh_token`
   rows *created* in the window. The `RefreshToken` RPC rotates tokens
   (`server/services/login/refresh.go`), and the client refreshes on a
   proactive expiry timer (~hourly access-token TTL, including backgrounded),
   on resume-near-expiry, on cold start, and on 401
   (`app/lib/services/auth_state.dart`). The metric is biased both directions
   and there is no server-side per-user activity signal at all today (no
   `last_*` on `User`, the auth interceptor stamps nothing,
   `UserDevice.last_used_unix_sec` tracks push delivery).

3. **Ad-hoc communities render as "Unknown community."**
   `communityName()` falls back to "Unknown community" for (a) nameless
   ad-hoc communities — despite `Community.origin_item` recording exactly
   which item spawned them (`docs/ad_hoc_communities.md`) — and (b)
   soft-deleted communities, because the builder's `GetByIDs` call excludes
   deleted rows by default even though the digest counts the
   `COMMUNITY_DELETED` event itself. Community rows carry no member counts
   and discard the already-computed `DistinctSenderCount`.

4. **Smaller defects.** `DigestNewUser.PrimaryCommunityName` is rendered by
   both templates but never populated; no period-over-period context on any
   number; `server/activity_digest/README.md` references a deleted
   `events.go`.

Product framing: this digest is how ops watches the fabric-health metrics that
`docs/users/priorities.md` commits to (circles with an active host, actions
per circle, promotions of one-offs into standing things). The enhancements
below make those specific signals legible.

## Context from Comments

No comments on the issue yet — it was filed 2026-07-07 from a full audit of
the digest package, the `CommunityEvent` stream, the auth/token stack, and
client refresh behavior; the three headline concerns (event-word confusion,
sign-ins vs. daily actives, ad-hoc "Unknown community") were stated directly
by the maintainer. The SMS-visibility blind spot discovered during the audit
was split into follow-up #2666 (inbound non-keyword SMS is a no-op today —
`server/services/web/sms_webhook.go` — so SMS-native users are invisible to
every metric; needs its own privacy/retention design).

## Potential Solutions

Most of the issue's five enhancement areas have one clear house pattern to
follow (see Recommendation). The genuine design fork is **how to measure
active users**:

### Option A: `user_active_day` stamp table written by middleware

- **Approach:** A small non-proto table (precedent: `activity_digest_state`,
  `server/storage/protosql_schema.go:530`) holding
  `(user_id, day_utc, first_seen_unix_sec, last_seen_unix_sec)` with
  PK `(user_id, day_utc)`. A new middleware between the authn wrapper and the
  RPC handlers (`server/routes.go:166` wrap chain; authn info is already
  readable from context, as `server/middleware/logging.go` does) upserts
  asynchronously (`logging.GoSafe`), rate-limited to ≤1 write per user per
  ~15 min by an in-process TTL cache. The digest counts distinct users whose
  `[first_seen, last_seen]` overlaps the window; the weekly digest groups per
  local day.
- **Pros:** True "used the product today" DAU/WAU, queryable by the digest
  builder like every other digest input; per-day breakdowns and future
  retention analyses come free; write cost ~zero at current scale; no client
  changes.
- **Cons:** Accrues only from deploy forward (not backfillable); one more
  table + middleware to maintain; needs an explicit retention/prune story.
- **Effort:** Medium (table + middleware + digest queries + tests).

### Option B: Log-based metric / BigQuery export of request logs

- **Approach:** `middleware.Logging` already emits `user_id` per authenticated
  request; define a Cloud Logging log-based metric or BQ sink and count
  distinct users per day there (the same stack as the #1613 SLI/SLO work in
  `terraform/modules/monitoring/`).
- **Pros:** Zero server code; historically accurate from whenever log
  retention starts; the "truest" signal.
- **Cons:** **Not reachable from the digest builder** — the email is
  assembled from storage queries, and pulling Cloud Monitoring time series
  into the Go job adds a GCP API dependency, auth surface, and a new failure
  mode to an email job; log-based metrics also can't do distinct-count
  exactly (they count entries, not cardinality) without a BQ pipeline.
  Belongs to dashboards, not this email.
- **Effort:** Small (infra) but wrong integration point for the digest.

### Option C: Derive actives from refresh-token cadence

- **Approach:** Keep counting `refresh_token` rows but interpret them as
  "app-active" (access tokens live ~1h, so an actively-used app rotates
  roughly hourly).
- **Pros:** No new writes; historical data exists.
- **Cons:** The client's proactive timer refreshes backgrounded apps and
  skips freshly-opened apps with valid tokens — the audit showed bias in both
  directions; stays misleading no matter the label.
- **Effort:** Small, but doesn't fix the problem.

## Recommendation

**Option A**, alongside two zero-instrumentation metrics that ship first and
remain valuable afterward:

- **Active contributors** — distinct union of `CommunityEvent.actor_id` and
  chat `DistinctSenderCount` in the window. One new batch storage query plus
  surfacing a value the chat query already computes; fully backfillable for
  historical windows (the indexes on `community_event.occurred_at_unix_sec`
  already support it).
- **Interactive sign-ins** — add an origin enum (models-proto only, no API
  change) to `RefreshToken`, set at the six interactive `createRefreshToken`
  call sites vs. the rotation site (`refresh.go:91`). Pre-existing rows read
  as UNSPECIFIED and age out within the refresh-token TTL.

The rest of the issue follows established patterns and is not contested:

- **A. Classification registry** mirroring `server/undo/registry.go` +
  its `TestEveryEventTypeClassified` CI gate (the documented convention for
  new `CommunityEventType` values in `docs/server/conventions.md` — this adds
  a second, digest-facing registry with the same enforcement shape).
- **C. Community rows**: fetch with `IncludeDeleted: true`; label ad-hoc rows
  by lifting `resolveOriginItemNames`
  (`server/services/community/membership.go:289`) into the shared
  `server/community` library (services must not be imported by libraries;
  `server/community` is the existing shared home — `provision.go` lives
  there); batch member counts via one `GROUP BY community_id` over
  `community_user`; surface distinct senders.
- **D. Deltas** via a slim prior-window headline pass (not a second full
  `BuildRange`).
- **E. Cleanups** (README, `PrimaryCommunityName`, deliberate
  `AssertMaxQueries` bumps).

## Open Questions

- **Day-bucketing timezone for `user_active_day`** — Recommendation: key the
  upsert on the **UTC day** (pure dedup key) and store first/last-seen unix
  timestamps; the digest counts by timestamp overlap with its local-TZ
  window. This keeps the table timezone-agnostic so a future change to the
  digest's configured timezone (or a second consumer) never invalidates
  stored rows. Accept ≤15 min slop at window edges from the write
  rate-limit.
- **Retention for `user_active_day`** — Recommendation: prune rows older
  than 400 days inside the weekly digest job tick (one `DELETE`, keeps a full
  year for year-over-year context). The table holds only `user_id` + dates —
  the same sensitivity class as existing `refresh_token.created_at_unix_sec`
  rows — and is never exposed on the API. Rows for a deleted user are inert
  telemetry; include them in the existing hard-delete/GDPR cascade list
  (`docs/deleting_things.md`) when that lands.
- **Fate of "returning sign-ins"** — Recommendation: once the origin enum
  lands, replace it with "signed in interactively: N" and drop the old
  number entirely (two actives metrics + interactive sign-ins is enough;
  a relabeled credential-churn count adds noise). If Phase 3 ships before
  the enum, interim-relabel to "N users' apps refreshed credentials".
- **Per-community row depth & ad-hoc rollup threshold** — Recommendation:
  show the top 8 communities as
  `Name (14 members) — 3 loans, 1 request, 12 messages from 5 people`
  (top 3 categories by count), annotate ad-hoc rows as
  `Ad hoc · around event "Board-game night"`, annotate same-window deletions,
  and roll up any remaining ad-hoc communities into one line by origin kind.
  Host-only (member count ≤ 1) ad-hoc communities are *not* suppressed here
  (unlike `ListCommunities`) — if one generated events, ops wants to see it.
- **Surface excluded/undone counts?** — Recommendation: yes, one muted footer
  line — "N bookkeeping events excluded; M actions were undone" — so the
  reconciliation invariant is visible in the email itself, and undo volume
  (a UX-health signal) gets a pulse.
- **`PrimaryCommunityName` populate or delete?** — Recommendation: populate.
  One batch `community_user` lookup for the (small) new-user list joined to
  the communities already fetched; the field and template branch already
  exist. Rendering follows the ad-hoc labeling rules from C.

## Implementation Plan

**Every phase:** `npm run build` (golangci-lint + compile) before commit;
`go build -tags=integration ./...` when exported signatures change
(`docs/server/conventions.md`); render daily + weekly through
`server/cmd/preview-activity-digest` whenever templates or
`ActivityDigestInput` change; keep `AssertMaxQueries` budgets tight with an
itemized comment at each bump.

### Phase 1: Classification registry + reconciling copy

**Goal:** Every `CommunityEventType` has an explicit digest disposition; the
ACTIVITY section reconciles with the totals; "events" means Experiences only.
**Tasks:**
- [ ] Add `server/activity_digest/registry.go`: map every `CommunityEventType`
      to `{Category, CounterLabel}` or an explicit exclusion
      (`ExcludedRetraction` for `*_UNDONE`, `ExcludedInternal` for
      `EXPERIENCE_ROSTER_CHANGED`). Categories: Gear, Loans, Giveaways,
      Requests, Events (experiences), Planning, Membership, Community
      lifecycle.
- [ ] CI gate `TestEveryDigestEventTypeClassified` mirroring
      `server/undo/registry_test.go`; runtime fallback counts an unknown type
      as "other" and logs at Warn (defense in depth only — the test makes it
      unreachable).
- [ ] Rewrite `applyEventToTotals` to be registry-driven; extend
      `ActivityTotals` with the 16 missing counters (transfer funnel:
      interest expressed/withdrawn, recipient selected, pickup proposed;
      request offer selected/withdrawn; Planning row; experiences
      started/updated; **"ad-hoc groups named"** from `COMMUNITY_NAMED`).
- [ ] Invariant unit test: categorized + excluded == total fetched rows, on a
      fixture containing every enum value.
- [ ] Copy: "Community events" → "Member actions" in both templates; footer
      line with excluded + undone counts; keep "Events" wording exclusively
      for experiences. **Structured log field names
      (`total_community_event_count` etc.) stay unchanged** — they are not
      user-facing and stable field names are an observability convention;
      add new fields additively if needed.
- [ ] Fix `server/activity_digest/README.md` (stale `events.go` reference;
      document the registry).
- [ ] Extend `docs/server/conventions.md` → "Adding New CommunityEventType
      Values": a new enum value now needs entries in **two** registries
      (undo + digest), each with its own CI gate.
- [ ] Verify with `server/cmd/preview-activity-digest` (daily + weekly).
**Validates:** The registry gate catches unclassified types in CI; the email
reconciles exactly; no template line uses "events" for log rows.

### Phase 2: Community rows with identity, size, and ad-hoc context

**Goal:** No more "Unknown community"; every row shows size and, for ad-hoc
groups, what they formed around.
**Tasks:**
- [ ] Lift `resolveOriginItemNames` from
      `server/services/community/membership.go` into the shared
      `server/community` library (per the shared-functionality rule in
      `docs/server/architecture.md`); the community service delegates to it.
      Include the transfer → gear-name hop.
- [ ] New storage helper `CountMembersByCommunity(ctx, communityIDs)` — one
      `GROUP BY community_id` over `community_user` (no N+1).
- [ ] Builder: fetch communities with `IncludeDeleted: true`; annotate
      deleted rows; label nameless rows `Ad hoc · around <kind> "<name>"`
      (name-presence is the ad-hoc discriminator per
      `docs/ad_hoc_communities.md` — no new enum); add member counts and
      `DistinctSenderCount` to `DigestCommunity`; per-community top-3
      category breakdown from Phase 1's registry counts; roll up the ad-hoc
      tail beyond the top 8 by origin kind.
- [ ] Populate `DigestNewUser.PrimaryCommunityName` via one batch
      `community_user` lookup for new users.
- [ ] Update both templates; bump `AssertMaxQueries` in `builder_test.go`
      deliberately (current 11; expect ≈ +7 for member counts + origin
      resolution + new-user communities) with a comment itemizing the budget.
- [ ] Unit tests: ad-hoc labeling per origin kind, deleted-community
      annotation, rollup threshold; preview-tool verification.
**Validates:** A fixture window containing an ad-hoc community, a promoted
community, and a same-window-deleted community renders all three correctly
with sizes; query budget is accounted line-by-line.

### Phase 3: Honest active-user metrics

**Goal:** The digest reports people, not credential churn.
**Tasks:**
- [ ] Storage query: distinct `actor_id` over `community_event` in window
      (uses `idx_community_event_occurred`); union with chat distinct senders
      in the builder → **"Active contributors"** headline. Weekly per-day
      contributor counts are bucketed in Go from the already-fetched events
      (no extra query); the chat-sender union applies to the window total
      only. Backfill-friendly: works for any historical window.
- [ ] `user_active_day` table (plain-table pattern next to
      `activity_digest_state` in `protosql_schema.go`) + new
      `server/middleware/activitystamp` middleware (package doc comment
      required — `revive.package-comments`) wrapped inside the authn chain in
      `routes.go`: async `GoSafe` upsert on `context.Background()` (the
      request context ends at response), in-process TTL cache
      (≤1 write/user/15 min; bounded by daily-active users, TTL-evicted),
      Warn-level log on write failure with an explicit safe-to-ignore
      rationale (ops telemetry must never fail a request — documented
      exception to fail-fast).
- [ ] Digest queries: distinct-active count for the window (+ per-local-day
      breakdown and WAU for weekly); render "Active users (opened the app)"
      alongside "Active contributors (did something)", with an em-dash note
      the first year that app-active accrues from the feature's deploy date.
- [ ] Models-proto change: `RefreshToken.origin` enum
      (`UNSPECIFIED/INTERACTIVE/ROTATION`), declared adjacent to
      `created_at_unix_sec` (field order near semantic kin, not appended)
      with a "Server-only; not exposed on the API" comment; set at all seven
      `createRefreshToken` call sites; digest metric becomes "Signed in
      interactively: N" (distinct users, `origin = INTERACTIVE`), replacing
      `ReturningSignInCount`. No API-proto change; no client change; no
      round-trip test needed (models-only field).
- [ ] Retention: weekly prune of `user_active_day` rows older than 400 days
      in the weekly job tick.
- [ ] Tests: middleware unit tests (dedup cache, async write, auth-info
      extraction), storage query tests, integration test that an
      authenticated RPC stamps exactly one row per user/day under concurrent
      requests, registry-unaffected `AssertMaxQueries` bump (≈ +4).
- [ ] Update `docs/registration_and_login.md` (RefreshToken origin field) —
      the nightly doc-freshness ratchet will demand it anyway.
**Validates:** Stamp writes are invisible in request latency (async), exactly
one row per user/day; DAU/contributor numbers match a seeded fixture;
interactive vs. rotation separation matches the login-flow call sites.

### Phase 4: Period-over-period deltas

**Goal:** Every headline number carries context.
**Tasks:**
- [ ] `buildHeadlineMetrics(ctx, start, end)` slim pass (events fetch +
      registry classification, message count, active counts, new-user count —
      ≈4 queries) run for the prior window (previous day / previous ISO
      week).
- [ ] Render deltas next to headline numbers (`Loans started: 12 (+4)`) in
      both templates; suppress deltas when the prior window predates
      available data (e.g. `user_active_day` before its deploy).
- [ ] Weekly digest: 7-row per-day actives/contributors line.
- [ ] Tests over synthetic two-window fixtures; final `AssertMaxQueries`
      budget (expected ceiling ≈26, all batched); preview-tool pass on daily
      + weekly.
**Validates:** Deltas are correct across window boundaries (incl. ISO-week
and DST edges, which `BuildRange`'s location math already handles); the
digest's total query count stays flat-bounded regardless of community count.

## Related Issues

- #1924 — original daily digest to ops (closed; established recipient +
  Metabase-vs-server decision this issue builds on).
- #2492 — phone-first pivot epic; source of ad-hoc communities,
  `COMMUNITY_NAMED`, and the SMS-native users the digest currently can't see.
- #2666 — follow-up: record inbound SMS as countable user activity
  (deliberately out of scope here; needs privacy/retention design).
- #2657 / #2662 — recent ad-hoc-community race work; unrelated mechanically
  but the same ad-hoc rendering rules apply.

## Audit Notes

Audited 2026-07-07 against the context map (`docs/llms.txt`). Changes applied
by the audit:

1. **Log-field stability** (observability): dropped the plan's original
   intent to rename `SendActivityDigest` structured log fields alongside the
   email copy. Verified nothing in `terraform/modules/monitoring/` references
   them today, but stable field names are the convention — copy changes are
   email-only; new log fields are additive.
2. **Two-registry convention** (conventions): added a Phase 1 task to extend
   `docs/server/conventions.md` → "Adding New CommunityEventType Values",
   since the digest registry adds a second CI-gated classification
   obligation for every new enum value.
3. **Middleware hygiene** (code_quality / error_handling): pinned down
   `activitystamp` details — package doc comment (revive gate), upsert on
   `context.Background()` inside `GoSafe`, TTL-cache boundedness.
4. **Proto field placement** (api_design): `RefreshToken.origin` declared
   next to `created_at_unix_sec` with a server-only comment; explicitly no
   round-trip test (models-only).
5. **Weekly per-day contributors** (performance): bucketed in Go from
   already-fetched events instead of new GROUP BY queries; chat-sender union
   stays window-total.
6. **Cross-phase verification note**: `npm run build`, integration-tag build
   on signature changes, preview-tool render, itemized `AssertMaxQueries`
   bumps.

Docs consulted: `docs/llms.txt`; anchors `server/architecture.md`,
`server/conventions.md`, `proto_conventions.md`, `server/observability.md`,
`testing/architecture.md`, `users/priorities.md`, `users/personas.md`
(skimmed); review rubrics ("Areas to Review"): security, performance,
observability, error_handling, testing, code_quality, api_design; leaf docs
`ad_hoc_communities.md`, `registration_and_login.md`, `server/l10n.md`
(admin-email English-only exemption), `deleting_things.md` (description-level,
for the GDPR-cascade note), `server/profiling.md` (AssertMaxQueries,
description-level). Verified against code: `server/activity_digest/*`,
`server/storage/activity_digest.go`, `server/storage/protosql_schema.go`
(plain-table + index precedents), `server/services/community/membership.go`
(`resolveOriginItemNames`, member counts), `server/services/login/refresh.go`,
`server/auth/tokens.go`, `server/routes.go` (authn wrap chain),
`terraform/environments/prod/main.tf` (digest job config).
