# Issue #2562: Drop legacy invite-link URL query-param handling once old GetOrCreateInviteLink URLs age out

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2562
**Filed:** 2026-06-26 | **Last activity:** 2026-06-26
**Labels:** none | **Assignees:** unassigned
**Context docs:** invitations.md, proto_conventions.md, server/architecture.md, server/conventions.md, server/observability.md, reviews/api_design_review_prompt.md (backwards-compat), reviews/observability_review_prompt.md; referenced issues #2058 (closed), #2048 (closed)

## Problem Statement

This is a **gated cleanup**, not net-new work. The legacy `GetOrCreateInviteLink`
RPC minted invite URLs that carried item context as query params
(`/go/SHORTCODE?gear_id=…` / `?request_id=` / `?experience_id=`). That RPC was
removed in **#2058**, so we no longer *mint* decorated URLs — but every decorated
URL already texted/emailed to someone persists in circulation and can still be
clicked by any client (old or new). Until those URLs age out, the server must keep
reading item context from the query params.

Three read sites keep the legacy path alive, each marked with a `TODO(#2562)` (or a
now-stale `TODO(#2058)`):

1. **`server/services/web/item_preview.go`** — `fetchItemPreview` falls back to
   `query.Get("gear_id"|"request_id"|"experience_id")` when the `ShareLink` row has
   no typed target. The canonical source post-#2048 is the row's `oneof target`; the
   query-param branch serves only legacy URLs. (Legacy invite rows were migrated to
   `ShareLink` with `community_invite_id` set — a plain community invite with **no**
   typed item target — so a decorated legacy URL genuinely falls through to the
   query-param branch.)
2. **`proto/ripls/api/community_service.proto` → `AcceptInvitationLinkRequest.gear_id`**
   (field 2) + its read in `server/services/community/invitations.go`
   `AcceptInvitationLink` (the `auditGearID` fallback). Legacy clients pass `gear_id`
   through for the `INVITATION_LINK_USED` audit record; new flows carry it on the row.
3. **The Flutter client** — `app/lib/core/router/app_router.dart` still parses
   `gear_id`/`request_id`/`experience_id` from the `/go/:code` redirect and the
   `/invite` route, and still passes `gearId:` to `acceptInvitationLink`
   (→ `community_repository.dart` → the `AcceptInvitationLinkRequest.gear_id` field).
   These reads are marked `TODO(#2058)` — a **closed** issue, so they are stale TODOs
   that no one is watching. **The issue text names only the two server sites, but the
   client cannot be ignored**: removing the proto field breaks the client's
   `gearId:` argument at compile time. The client cleanup is gated on the *same*
   aging-out condition and must ship in the same coordinated removal.

**The core gap the issue glosses over:** the "done when" criterion — *"telemetry/heuristic
confirms legacy query-param invite URLs are out of circulation (no `?gear_id=` hits on
`/go/` in a trailing window)"* — **cannot be evaluated today because that telemetry does
not exist.** `HandleInvitePage` logs `item_type` but never records whether the item came
from the query-param fallback vs. the row. Before we can *decide* to remove anything, we
have to *measure*. That measurement step is the real first phase of this issue.

## Context from Comments

**The issue has no comments.** Everything comes from the body and the two referenced
(closed) issues:

- **#2058** ("Remove deprecated GetOrCreateInviteLink RPC after two minor revisions",
  CLOSED) — removed the RPC that *minted* decorated URLs, gated on old-*client* traffic
  dropping to zero. #2562 is its explicit follow-up, gated on old-*URL* traffic instead.
- **#2048** ("ShareLink: polymorphic table + migration", CLOSED) — introduced the
  `ShareLink` model with `oneof target`, migrating legacy `CommunityInvitationLink` rows
  to `community_invite_id` (community invite, no item target). This is *why* the
  query-param fallback is still exercised: the migrated rows have no gear/request/event
  target, so item context for a legacy URL lives only in the query string.

The critical constraint stated in the body: this is **gated on real-world URL aging, not
on old-client app traffic** (the #2058 gate). A clicked old URL exercises this path
regardless of the clicking client's version — even the newest app, handed a decorated
legacy URL, will forward the query params. So "no old clients left" is *not* sufficient;
we need "no old *URLs* still being clicked."

## Potential Solutions

The removal itself is mechanical and non-controversial (the issue prescribes it). The
real design decisions are (A) **how we measure aging-out** and (B) **how far the removal
reaches**. The options below are for those two decisions; the recommended path composes
one option from each into the Implementation Plan.

### Measurement options

### Option A1: Distinctive structured log line + manual Cloud Logging query
- **Approach:** In `HandleInvitePage` (`server/services/web/service.go`), when the raw
  query string contains any of `gear_id`/`request_id`/`experience_id`, emit one
  distinctive `INFO` log (e.g. `legacy_invite_query_param=true` field, or a fixed
  message string) with the param name. After it has been deployed to prod for a full
  aging window, query Cloud Logging (as `user2@example.org`, per the GCP-log-access
  runbook) over the trailing 30–90 days and confirm the count is at/near zero.
- **Pros:** Minimal, no infra. Fires on **every** click of a decorated legacy URL —
  independent of client version, auth state, or whether the item still exists or the
  user completes acceptance. The web-layer invite-page render is the single best
  choke point (it precedes the accept RPC and covers unauthenticated clickers who
  never open the app). Aligns with the project's "minimal dependencies" preference and
  the fact that this is a one-time decision, not an ongoing SLO.
- **Cons:** The decision is a manual log query rather than a dashboard tile; someone has
  to remember to run it. Log retention must exceed the aging window.
- **Effort:** Small.

### Option A2: Permanent log-based metric + monitoring dashboard/alert
- **Approach:** Same log line as A1, plus a log-based counter metric in
  `terraform/modules/monitoring` (the established SLI/SLO stack, #1613) and a dashboard
  tile / alert so the trend is visible over time.
- **Pros:** Durable, glanceable, no manual querying; a stakeholder can watch it trend to
  zero. Fits the existing monitoring module.
- **Cons:** Over-engineered for a value that is meant to reach zero once and then have its
  emitter deleted — we'd stand up a metric only to tear it down. Terraform apply cost on
  shared dev state (see the shared-dev-state gotcha) for a throwaway metric.
- **Effort:** Medium.

### Option A3: Heuristic time-based horizon (no telemetry)
- **Approach:** Pick a fixed horizon (e.g. "URLs older than N months are assumed dead")
  and remove on schedule without instrumentation.
- **Pros:** Zero code before removal.
- **Cons:** Directly contradicts the issue's "done when" (which asks for telemetry/heuristic
  *confirmation*). Invite URLs never expire and live in durable channels (SMS/email threads,
  saved messages) — a blind horizon risks silently breaking a still-shared link's rich
  preview and item deep-link. Not defensible.
- **Effort:** Small, but wrong.

### Removal-scope options

### Option B1: Server-only removal (literal issue scope)
- **Approach:** Drop only the two named server sites — the `fetchItemPreview` fallback and
  the `auditGearID` fallback — but **keep** `AcceptInvitationLinkRequest.gear_id` in the
  proto so the client keeps compiling.
- **Pros:** Smallest diff; matches the issue checklist verbatim.
- **Cons:** Leaves a dead proto field the server ignores, a client still parsing/forwarding
  query params, and the stale `TODO(#2058)` references pointing at a closed issue —
  exactly the "dead deprecation surface" this cleanup exists to remove. Half a cleanup.
- **Effort:** Small.

### Option B2: Coordinated server + proto + client removal (recommended)
- **Approach:** Remove the `fetchItemPreview` fallback; remove
  `AcceptInvitationLinkRequest.gear_id` (reserve field 2) and the `auditGearID` fallback;
  and in the same change remove the client-side query-param reads (`/go` redirect,
  `/invite` route, `RegisterScreen` `gearId`/`requestId`/`experienceId` params) and the
  `gearId:` argument threaded through `community_repository.dart` /
  `community_service.dart`. Delete the now-stale `TODO(#2058)` markers.
- **Pros:** Leaves a single clean primitive with zero dead deprecation surface. The proto
  field removal *requires* the client change anyway (compile break), and the client reads
  are gated on the identical aging-out condition — doing them separately would mean two
  round-trips through the same gate. Retires the stale-closed-issue TODOs.
- **Cons:** Larger diff spanning proto + Go + Dart; needs a coordinated regen and full
  client/server test pass.
- **Effort:** Medium.

## Recommendation

**DECISION (2026-07-05, Bryan): Option A3 + Option B2 — hard cutover now, no telemetry
wait.** Clients have minted only bare `/go/{code}` links for several versions, and the
degradation for any still-circulating legacy decorated URL is **graceful**: a clicked
`/go/CODE?gear_id=…` still resolves the community and joins the user — it only loses the
item-specific rich preview and the post-join item deep-link. Because the failure mode is
"loses an enhancement," not "breaks the join," the A3 objection in the options table
(never-expiring URLs) is outweighed: there is nothing to *break*, only a legacy nicety to
retire. So we skip the instrument-and-wait phases (A1/A2) and cut over immediately.

Server field removal is backwards-safe: proto3 silently ignores unknown fields, so any old
client still sending field 2 keeps working — it just no longer gets the query-param
deep-link, which is the intended end state.

**The short links are already self-sufficient.** The deep-link target lives on the typed
`ShareLink` row (`oneof target { gear_id | transfer_id | request_id | experience_id | … }`).
The SSR web pages resolve the item straight from the row (`gear_page.go`/`event_page.go`/
`request_page.go` → `shareLink.GetGearId()`/`.GetExperienceId()`/`.GetRequestId()`), and
`CheckInvitation` exposes it to the client as `target_kind`/`target_id`
(`login_service.proto`). Nothing about deep linking depends on the *incoming* URL's query
string anymore. So the legacy query-param reads are pure dead weight — a hard cutover
removes them without giving anything up.

**The #2492 phone-first web params are orthogonal, not legacy.** The
`/verify-phone?gear_id=…&n=…&img=…` params are the SSR-page → Flutter-Web-app **handoff**:
`WebItemActionScreen` gets `itemId` from the **route path** (`/item/{gearId}`, resolved by
the SSR page *from the short-link row*) and threads the id + item name + hero image into
`/verify-phone` because none of them is fetchable pre-auth
(`web_item_action_screen.dart:44,113`). Same source of truth (the row), carried across the
auth boundary — not an incoming legacy URL. This plumbing (`/register`, `/verify-phone`,
`RegisterScreen` fields, `PhoneAuthScreen` propagation, `postRegistrationDestination` web
branches) **stays as-is** because it's current and correct, not because we're grandfathering
legacy. The `TODO(#2058)` comments there are stale/misleading (they predate #2492 reusing
the same transport and wrongly call it legacy-only); **correct the comments** in place — no
new follow-up issue is needed, since the code is doing the right thing.

**Genuinely legacy-invite-only client sites — cut now:**
- the `/go/:code` redirect's forwarding of `gear_id`/`request_id`/`experience_id` (drop the
  id at the door; forward only the token), and
- the `/invite` route's reading of those params + passing them to `RegisterScreen` and to
  `acceptInvitationLink`, plus
- the compile-forced `gearId` argument on the `acceptInvitationLink` RPC chain
  (`community_service.dart`, `community_repository.dart`).

Clicking an old decorated `/go/CODE?gear_id=X` link then drops the `gear_id` at the redirect
and lands on `/invite?token=CODE` — the user still joins the community; only the item
deep-link nicety is gone, which is the intended graceful degradation. The manual-paste
`parseInviteCodeInput`/`_handleManualCodeSubmit` id-extraction is the one remaining
legacy-only-but-shared-route bit (`/register` is also a #2492 web-event landing); simplify it
if clean, else correct its comment and leave it — the primary vector (clicking an old link)
is fully covered by the `/go` + `/invite` changes.

## Open Questions

- **What aging window counts as "aged out"?** — Recommendation: **90 days of zero (or
  de-minimis, e.g. <5/week and trending to zero) legacy-query-param invite-page renders in
  prod**, measured after the Phase-1 log ships. Invite URLs never expire and live in durable
  SMS/email threads, so 30 days is too short; 90 days balances confidence against carrying
  the dead surface. Confirm the prod Cloud Logging retention covers the chosen window before
  relying on it.
- **Should the audit `GearId` for legacy accepts be preserved after removal?** —
  Recommendation: **No.** Once legacy URLs are gone, every accept carries item context on the
  `ShareLink` row (`link.GetGearId()`), and `AcceptInvitationLink` already reads the row when
  the request field is empty. Dropping the request field loses nothing for new URLs; for the
  vanishing tail of old URLs the audit `GearId` simply falls back to the row's value (empty
  for a plain community-invite row), which is acceptable for a historical audit record.
- **Is the client cleanup in-scope for #2562 or a separate issue?** — Recommendation:
  **In-scope** (Option B2). It's a hard compile-time dependency of the proto change and shares
  the exact gate. If the team prefers to keep #2562 server-only for tracking hygiene, file a
  companion client issue and land the two PRs together — but do **not** merge the proto field
  removal without the client change in the same release train, or the client build breaks.
- **Do we need a log-based metric after all?** — Recommendation: **No**, unless a stakeholder
  wants to watch the trend. Default to A1's manual query; revisit only if the first read is
  non-zero and we want to monitor the decline.
- **`request_id` / `experience_id` symmetry.** — The proto only ever had `gear_id` on
  `AcceptInvitationLinkRequest` (there was never a `request_id`/`experience_id` request field;
  those only ever traveled as query params to the invite page). So the proto removal is just
  field 2. Recommendation: confirm this during Phase 3 with a `grep`, and reserve only what
  actually exists — don't invent reserved fields for names the message never had.

## Implementation Plan

**Single phase — hard cutover now (A3 + B2).** No instrument-and-wait; clients mint bare
links and the short-link row carries all deep-link info, so the legacy query-param reads are
dead weight and their loss degrades gracefully (join still works). The former Phase 1/2
(instrument, confirm aging-out) are intentionally skipped per the recorded decision.

### Phase 1: Remove the legacy read paths (server + proto + client)
**Goal:** Single clean primitive; zero dead deprecation surface.
**Tasks:**
- [ ] **Server web:** delete the query-param fallback block in `fetchItemPreview`
  (`item_preview.go`); read the `ShareLink` row exclusively. Remove the Phase-1
  instrumentation log (its job is done) — or keep it briefly as a regression tripwire, then
  remove.
- [ ] **Proto:** remove `AcceptInvitationLinkRequest.gear_id` and add `reserved 2;` +
  `reserved "gear_id";` (matching the existing reserved-field style in this proto). Confirm no
  `request_id`/`experience_id` request fields ever existed before reserving anything else.
- [ ] **Server community:** delete the `auditGearID` request-field fallback in
  `AcceptInvitationLink` (`invitations.go`); publish the `INVITATION_LINK_USED` event with
  `GearId: link.GetGearId()` (row-only). Update the surrounding comment.
- [ ] **Client (legacy-only, cut):** in `app_router.dart`, stop the `/go/:code` redirect from
  forwarding `gear_id`/`request_id`/`experience_id` (forward only `token`), and stop the
  `/invite` route from reading those params + passing them to `RegisterScreen` / to
  `acceptInvitationLink`. Remove the `gearId:` argument through `community_repository.dart` and
  `community_service.dart` (compile-forced by the proto field removal). Delete the `TODO(#2058)`
  markers on these removed sites.
- [ ] **Client (#2492-shared, keep + correct):** leave `RegisterScreen`'s
  `gearId`/`requestId`/`experienceId` fields, `PhoneAuthScreen` propagation, the `/register` and
  `/verify-phone` routes, and `postRegistrationDestination` (web branches) intact — they carry
  the #2492 SSR→SPA handoff, not legacy invite params. **Correct** their stale/misleading
  `TODO(#2058)` comments (they wrongly call the plumbing legacy-only) rather than removing code.
  Simplify the manual-paste `parseInviteCodeInput`/`_handleManualCodeSubmit` id-extraction only
  if it's clean and doesn't disturb #2492's `experience_id`+`rsvp` on `/register`.
- [ ] **Regen + compile:** `npm run generate`; fix Go and Dart compile fallout; ensure the app
  builds against a `gearId`-free `AcceptInvitationLinkRequest`.
- [ ] **Tests:** migrate the `?gear_id=` query-param tests in
  `server/services/web/item_preview_test.go` (`TestHandleInvitePage_GearLoan`,
  `_GearGiveaway`, `_GearGivenAway`, `_GearMissing`, `_GearDefaultsToLoan`,
  `_GearUsesFirstName`) to the typed-target row form (mirror the existing
  `TestHandleInvitePage_TypedGearShareLink`); remove any test asserting the legacy fallback.
  Update/verify the `AcceptInvitationLink` audit tests in `server/services/community/` for
  row-only `GearId`. Run server (`go test`, unit+integration) and Flutter tests locally.
- [ ] **Docs:** update `docs/invitations.md` — remove the "Optional deep-link query
  parameters" list (§2), the Deep-Linking query-param table (§Deep Linking), and the
  `TODO(#2562)` note in the ShareLink data-model section; advance the `freshness:` stamp.
  Update the `AcceptInvitationLinkRequest` proto comment referencing the field.
- [ ] **Close the loop:** close #2562 and grep the tree for any lingering `#2562`/`#2058`
  markers per the TODO-hygiene rule (stale TODOs pointing at closed issues must go).
**Validates:** The codebase exposes one `ShareLink`-row-driven invite path, the proto has no
dead field, the client no longer parses legacy query params, and no TODO points at a closed
issue.

## Audit Notes (2026-07-05)

Audited against the selected standards below.

- **api_design §4 (Backwards Compatibility → Field Removal):** ✅ Removing
  `AcceptInvitationLinkRequest.gear_id` reserves both the number and the name
  (`reserved 2; reserved "gear_id";`), matching the existing reserved style in this
  proto. Wire-compatible — proto3 silently drops unknown field 2 from any old client
  still sending it; the server no longer reads it. Not a breaking change.
- **proto_conventions "Proto comments":** ⚠️→✅ The initial reserved-field tombstone
  comment leaked server mechanics and internal names (`GetOrCreateInviteLink`, the
  `ShareLink` storage model, and a `link.GetGearId()` read). Trimmed to a
  client-safe, mechanics-free note. Reserved-field comments still name the field and
  the historical reason (`?gear_id=` query param for the audit record) — appropriate
  for a tombstone aimed at future proto maintainers.
- **testing/architecture:** The `?gear_id=`-driven cases in
  `server/services/web/item_preview_test.go` must be re-pointed at typed-target rows
  (mirror `TestHandleInvitePage_TypedGearShareLink`); after the change
  `fetchItemPreview` no longer reads query params, so those cases would otherwise
  assert against an empty preview. Captured in Phase 1.
- **code_quality / tech_debt (TODO hygiene):** ✅ The removed sites drop their
  `TODO(#2562)`/`TODO(#2058)` markers; the #2492-shared sites keep their code but get
  their stale/misleading `TODO(#2058)` comments corrected (no dangling reference to a
  closed issue).
- **observability:** The `INVITATION_LINK_USED` audit event's `GearId` now derives
  solely from the share-link row (empty for a plain community invite). Accepted
  reduction for the vanishing legacy-URL tail; no ERROR-log or alert surface is
  affected.

**Docs consulted:** `proto_conventions.md`, `docs/reviews/api_design_review_prompt.md`,
`server/architecture.md` + `server/conventions.md` (anchors), `client/architecture.md` +
`client/conventions.md` (anchors), `server/observability.md`, `testing/architecture.md`,
`invitations.md`, plus the phone-first web-flow code (`web_item_action_screen.dart`,
`gear_page.go`/`event_page.go`/`request_page.go`, `login_service.proto`) to confirm the
#2492 transport is orthogonal to the legacy invite query params.

## Related Issues
- #2058 — CLOSED; removed the `GetOrCreateInviteLink` RPC that minted these decorated URLs.
  This issue is its real-world-URL-aging follow-up, and it also owns the stale `TODO(#2058)`
  client-side query-param reads that #2058 left behind.
- #2048 — CLOSED; introduced the `ShareLink` `oneof target` and migrated legacy invite rows to
  `community_invite_id`, which is why the query-param fallback is still exercised for old URLs.
- #2056 — the `share_link_backfill.go` job (`TODO(#2056)`) still mirroring legacy
  `CommunityInvitationLink` rows into `ShareLink`; adjacent legacy-invite cleanup, worth
  checking for shared removal timing.
