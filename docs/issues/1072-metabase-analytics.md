# Issue #1072: Add Metabase analytics dashboards on Postgres

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1072
**Filed:** 2026-04-09 | **Last activity:** 2026-04-09
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/users/personas.md, docs/users/priorities.md

## Problem Statement

We have no visibility into server-side business metrics. Firebase/GA4 covers
client-side behavior (retention, funnels, engagement), but questions like "how
many communities exist?", "what's our transfer completion rate?", "how many
users per community?" require manual SQL queries against production. We need a
self-hosted dashboard layer that non-engineers can use to answer these questions
on existing Postgres data, without introducing a separate analytics pipeline.

## Context from Comments

No comments on this issue. The issue body is comprehensive and includes a
pre-discussed plan. The plan is well-scoped but some deployment details need
resolution (see Open Questions).

## Potential Solutions

### Option A: Self-hosted Metabase on Cloud Run

- **Approach:** Deploy Metabase as a second Cloud Run service alongside the
  existing server. Connect it to our Cloud SQL Postgres via a read-only user.
  Use a separate Cloud SQL database (or a second database on the same instance)
  for Metabase's internal metadata.
- **Pros:** Consistent with our existing infrastructure (Cloud Run + Cloud SQL).
  Single Docker container. No new infrastructure types. VPC connector already
  exists for private DB access. Metabase is mature, open source, and has a
  visual query builder for non-engineers.
- **Cons:** Cloud Run's request-based billing model is awkward for a dashboard
  tool that needs to stay warm. Metabase startup is slow (~30s) which means
  cold starts will be painful. Need to solve persistent storage for Metabase
  metadata (H2 default won't survive container restarts).
- **Effort:** Medium.

### Option B: Metabase on a persistent VM (Compute Engine)

- **Approach:** Run Metabase on a small, always-on Compute Engine VM instead of
  Cloud Run. Connect to Cloud SQL via private IP.
- **Pros:** No cold start issues. Always-on is natural for a dashboard tool.
  Simpler persistent storage (local disk). Can run Metabase + its own Postgres
  metadata DB on the same VM.
- **Cons:** Introduces a new infrastructure type (VM) that we don't currently
  manage. Requires OS patching, monitoring, SSH access. More operational burden
  than Cloud Run. Costs more if underutilized.
- **Effort:** Medium-large (new infrastructure pattern).

### Option C: Managed Metabase (Metabase Cloud)

- **Approach:** Use Metabase's hosted offering instead of self-hosting. Point
  it at our Cloud SQL instance via a secure connection.
- **Pros:** Zero operational burden. No infrastructure to manage. Automatic
  updates.
- **Cons:** Requires exposing Cloud SQL to external connections (currently
  private-only, no public IP). Monthly SaaS cost (~$85/mo for Starter plan).
  Vendor dependency. Goes against the project's "no vendor lock-in" preference
  stated in the issue.
- **Effort:** Small (but ongoing cost and security tradeoff).

## Recommendation

**Option A: Self-hosted Metabase on Cloud Run.** It keeps our infrastructure
homogeneous (everything is Cloud Run + Cloud SQL) and avoids introducing VMs or
SaaS dependencies. The cold start issue is acceptable for an internal dashboard
tool that's used infrequently — keep `min_instances = 0` to minimize cost and
accept the ~30s cold start when accessing dashboards.

**Dashboard management via API skill.** Metabase's built-in serialization
(YAML export/import) is an Enterprise-only feature. Instead, dashboards are
managed through the `/ripls-analytics` Claude Code skill, which uses the
Metabase REST API to create, update, and list charts and dashboards. The skill
serves as the operational runbook — charts can be recreated from scratch in
seconds via API calls. Dashboard state lives in Metabase's metadata DB, backed
up via Cloud SQL automated backups.

**Metabase metadata** (user accounts, database connection settings) uses a
dedicated database on the same Cloud SQL instance. This is the minimal state
that can't be serialized — it's tiny, read-heavy, and free on the shared
instance.

## Open Questions

- **How should Metabase be authenticated?** Metabase has its own user accounts.
  For an internal tool with a small team, Metabase's built-in auth is probably
  sufficient. But should it be behind IAP (Identity-Aware Proxy) for an extra
  layer? — Recommendation: Start with Metabase built-in auth. Add IAP later if
  the team grows or if the dashboard becomes externally accessible.

- **Should the read-only user be created via Terraform or manually?** Currently
  only one DB user exists (`gear_user` in `terraform/modules/storage/main.tf:110-116`).
  — Recommendation: Add the read-only user to Terraform so it's reproducible
  across environments. Create it as a `google_sql_user` resource with a
  post-creation `GRANT SELECT` via a provisioner or Cloud SQL admin API.

- **Should Metabase metadata live on the same Cloud SQL instance?** Resolved:
  yes. Dashboard definitions are managed via the Metabase REST API (through
  the `/ripls-analytics` skill) and can be recreated from scratch in seconds.
  User accounts, DB connection settings, and dashboard state live in a separate
  `metabase` database on the shared Cloud SQL instance at zero extra cost,
  backed up via Cloud SQL automated backups.

- **Which dashboards should we build first?** The issue lists many metrics.
  — Recommendation: Start with entity counts (communities, users, gear,
  transfers) and the transfer funnel from `community_event`. These directly
  answer the questions in the issue body and validate the setup end-to-end.

- **Do we need a `user_event` table in Phase 1?** The issue puts this in
  Phase 2. — Recommendation: Agree. Phase 1 should use only existing data.
  The `community_event` table already has 22 event types covering transfer
  lifecycle, request lifecycle, experience RSVPs, and community membership.
  That's enough for meaningful dashboards.

## Implementation Plan

### Phase 1: Infrastructure — read-only user and Metabase deployment

**Goal:** Metabase running in dev and prod, connected to Postgres, with one
working dashboard proving the setup end-to-end.

**Tasks:**

*Local development (PR #1078 — merged):*
- [x] Add Metabase to `docker-local-postgres.yaml` for local development
  - Metabase container behind `metabase` Docker Compose profile
  - Use embedded H2 locally (simpler for dev, metadata is disposable)
  - Expose on port 3000
  - Added npm scripts: `metabase`, `metabase:stop`, `metabase:init`
- [x] Create an init SQL script (`scripts/metabase-readonly-user.sql`) for
  local dev that creates the read-only user on the local Postgres
- [x] Verify Metabase connects and can see all tables locally

*Dev environment (PR #1079 — merged):*
- [x] Add a read-only Postgres user to Terraform
  (`terraform/modules/metabase/main.tf`)
  - `google_sql_user.metabase_reader` resource created
  - SELECT grants are a manual one-time step after deploy (see comment in
    module and `scripts/metabase-readonly-user.sql`)
- [x] Create a `metabase` database on the same Cloud SQL instance for Metabase
  internal metadata (`google_sql_database.metabase`)
- [x] Create a `metabase_admin` user for Metabase to use for its own metadata
  database
- [x] Add Metabase as a Cloud Run service in a new Terraform module
  (`terraform/modules/metabase/main.tf`)
  - Uses `metabase/metabase:latest` image
  - Configured `MB_DB_TYPE=postgres`, `MB_DB_HOST`, `MB_DB_DBNAME=metabase`,
    `MB_DB_USER=metabase_admin`
  - `min_instances = 0` to minimize cost (accept cold starts)
  - Connects via existing VPC connector for private DB access
  - Public access enabled for custom domain (Metabase handles its own auth)
- [x] Create secrets in GCP Secret Manager: `metabase-reader-password`,
  `metabase-admin-password`
- [x] Run `terraform apply` and verify Metabase starts on Cloud Run
- [x] Run SELECT grants on Cloud SQL via Cloud Console SQL Studio
- [x] Set up domain mapping: `analytics-dev.ripls.org` → Metabase Cloud Run
- [x] Set up Cloudflare CNAME for `analytics-dev`
- [x] Verify end-to-end: Metabase accessible at https://analytics-dev.ripls.org
- [x] Create first chart (communities over time) to validate setup

*Production environment (PR #1090):*
- [x] Create secrets in GCP Secret Manager (ripls-prod project):
  `metabase-reader-password`, `metabase-admin-password`
- [x] Add Metabase module to `terraform/environments/prod/main.tf`
  - Same pattern as dev: shared Cloud SQL instance, VPC connector, public
    access for custom domain
  - Domain mapping: `analytics.ripls.org`
  - Cloudflare CNAME for `analytics`
- [x] Run `terraform apply` for prod
- [x] Run SELECT grants on prod Cloud SQL as `ripls_user`
- [x] Verify Metabase accessible at https://analytics.ripls.org
- [x] Complete Metabase setup wizard (admin account, DB connection)

**Validates:** Metabase can connect read-only to our Postgres, see all
auto-generated tables, and the deployment pipeline works.

### Phase 2: Core dashboards on existing data

**Goal:** Dashboards answering the business questions from the issue, built
entirely on existing tables with no schema changes. Dashboards are managed
via the `/ripls-analytics` skill using the Metabase REST API.

**Tasks:**

*Completed:*
- [x] Configure all `_unix_sec` fields as `UNIXSeconds->DateTime` coercion
  (68 fields on both dev and prod, via Metabase API)
- [x] Create `/ripls-analytics` Claude Code skill for API-based dashboard
  management (replaces serialization workflow — Enterprise-only feature)
- [x] Build "Main" dashboard on dev and prod with:
  - Communities over time (cumulative, monthly)
  - Loans over time (cumulative, monthly, transfer_type="1")
  - Giveaways over time (cumulative, monthly, transfer_type="2")
  - Experiences over time (cumulative, monthly, community_experience)
  - Requests over time (cumulative, monthly, community_request)

- [x] Add milestone annotations (Product Milestones timeline) on dev and prod:
  - Alpha Launch: January 27, 2026 (first prod data)
  - Private Beta Launch: April 1, 2026
- [x] Build "Overview" dashboard: Total Communities, Members, Gear,
  Transfers, Requests, Experiences (scalar cards in 3x2 grid)
- [x] Build "Community Activity" dashboard:
  - Events over time (monthly count)
  - Most active communities (bar chart, top 20)
  - Event type distribution (pie chart with human-readable labels)
- [x] Build "Transfer Funnel" dashboard:
  - Transfer funnel (bar: interest → selected → active → completed +
    cancelled/withdrawn)
  - Avg days between transfer stages
- [x] Build "Request Funnel" dashboard:
  - Request funnel (bar: created → offer → selected → fulfilled +
    cancelled/withdrawn)
  - Avg days between request stages
- [x] Build "Growth" dashboard:
  - Communities over time (cumulative)
  - Members over time (cumulative, from community_user)
  - Gear shared over time (cumulative, from community_gear)
- [x] Build "Engagement" dashboard:
  - Feed views over time (monthly count)
  - Views by item type (pie chart)
  - Views per user (bar chart)
- [x] Build "Experiences" dashboard:
  - Experiences over time (cumulative)
  - RSVP breakdown (pie: yes/maybe/no)
  - RSVPs over time

- [x] Organize dashboards into collections on both dev and prod:
  - Root: Overview
  - Growth: Growth
  - Funnels: Transfer Funnel, Request Funnel
  - Activity: Community Activity, Engagement, Experiences

*Dashboard polish (best practices from Metabase docs + BI design principles):*
- [x] Overview: replace static scalars with smartscalar trend cards showing
  % change vs. prior month
- [x] Add dashboard-level date range filter to all dashboards (except
  Overview — trend cards handle their own time comparison)
- [x] Add community filter to Growth and Activity dashboards (Growth,
  Community Activity, Engagement, Experiences)
- [x] Funnel dashboards: switch from bar to funnel display type
- [x] Event Type Distribution: switch from pie to horizontal bar chart
  (15+ categories is unreadable as pie)
- [x] Growth dashboard: add text cards as section dividers ("Entity Growth",
  "Action Growth")
- [x] Add descriptive text cards to all dashboards explaining purpose,
  questions answered, and usage tips
- [x] Funnel charts: full width on separate rows for better readability

**Dashboard organization and placement rules are maintained in the
`/ripls-analytics` skill** (`.claude/skills/ripls-analytics/SKILL.md`).
New dashboards should follow the collection hierarchy defined there.

**Validates:** Non-engineers can answer the core business questions listed in
the issue without writing SQL.

### Phase 3: User lifecycle instrumentation (future)

**Goal:** Track user lifecycle events not currently captured in
`community_event` — registration, login, email verification, profile
completion, first action.

**Tasks:**
- [ ] Design `user_event` proto model in `proto/ripls/models/user.proto`
  - Event types: registered, logged_in, email_verified, profile_completed,
    first_gear_shared, first_transfer, first_request, first_experience
  - Fields: user_id, event_type, timestamp, metadata
  - Use `optional` keyword for conditionally-present fields per proto
    conventions (e.g., metadata that only exists for certain event types)
- [ ] Register `UserEvent` in `DefaultStorageTypes()` in
  `server/storage/protosql_schema.go` so the table auto-creates on startup
- [ ] Implement event-writing in the Go server as a shared library
  (`server/events/user_events.go`), not as a service-to-service call. Services
  that need to emit user events (login, community, gear) inject this library
  via constructor — following the "shared library" pattern from the server
  architecture guide
- [ ] Instrument registration, login, and verification flows to emit events
- [ ] Use structured logging for event emission: `logger.InfoContext(ctx,
  "user event emitted", "operation", "EmitUserEvent", "user_id", userID,
  "event_type", eventType)` — following standard field names from logging
  conventions
- [ ] Add unit tests for the event-writing library, including table-driven
  tests for each event type
- [ ] Build "User Lifecycle" dashboard in Metabase
- [ ] Evaluate whether a `client_event` table + `TrackEvent` RPC is needed or
  if Firebase covers client-side sufficiently. If a `TrackEvent` RPC is added:
  - Define dedicated request/response types in `proto/ripls/api/` (never reuse
    across RPCs)
  - The API proto must NOT import from `proto/ripls/models/` — keep the
    API/models package boundary clean
  - The client should only reference `ripls/api` generated code, never
    `ripls/models`

**Validates:** We can track the full user journey from registration through
first meaningful action, enabling cohort analysis and activation metrics.

## Audit Notes

The following changes were made during architectural review:

1. **Phase 3 — proto conventions:** Added `optional` keyword guidance for
   `user_event` fields, per CLAUDE.md proto field conventions.

2. **Phase 3 — storage registration:** Added explicit task to register
   `UserEvent` in `DefaultStorageTypes()` so the table auto-creates. The
   storage layer auto-generates tables from proto models registered there
   (`server/storage/protosql_schema.go`); omitting this step would mean the
   table doesn't exist.

3. **Phase 3 — shared library pattern:** Changed event-writing from implicit
   "instrument the server" to an explicit shared library
   (`server/events/user_events.go`). The server architecture guide requires
   that shared functionality across services lives in standalone libraries
   injected via constructors, not via service-to-service calls.

4. **Phase 3 — API/models boundary:** Added guidance that any future
   `TrackEvent` RPC must use dedicated request/response types in
   `proto/ripls/api/` and must not import from `proto/ripls/models/`. The
   client must never reference `ripls/models` generated code. This is a core
   architectural constraint from CLAUDE.md.

5. **Phase 3 — logging conventions:** Added structured logging guidance using
   `logging.LoggerWithContext(ctx)` and standard field names per the server
   logging documentation.

6. **Phase 3 — testing:** Added unit test requirement for the event-writing
   library per server architecture guide ("No new functionality without test
   coverage").

7. **No client-side changes in any phase.** This plan is purely server-side
   infrastructure and Metabase configuration. No Flutter architecture patterns
   (repositories, viewmodels, caching, i18n) are affected. If Phase 3's
   `TrackEvent` RPC is eventually built, a separate plan should cover the
   client integration following the repository pattern and i18n guidelines.

## Related Issues

- No directly related issues found. This is a new infrastructure initiative.
