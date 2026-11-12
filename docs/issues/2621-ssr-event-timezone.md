# Issue #2621: Event SSR page and link preview render event time in UTC instead of the event's local timezone

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2621
**Filed:** 2026-07-01 | **Last activity:** 2026-07-01
**Labels:** bug | **Assignees:** unassigned
**Context docs:** `docs/proto_conventions.md`, `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/client/architecture.md`, `docs/client/conventions.md`, `docs/invitations.md`, `docs/client/time.md`, `docs/workflows/phone_first_rsvp.md`; review dimensions applied: api_design, error_handling, testing, code_quality, dependency

> **DECISION (maintainer, 2026-07-01):** Option C. Schema change + stop the
> bleeding + render fixes. **No location→timezone resolver dependency and no
> backfill** — existing events keep their stored (possibly missing/UTC) data
> and degrade to date-only rendering. Option B below is kept as the record of
> the full-fix design in case it is ever revisited.

## Problem Statement

Server-rendered event surfaces show event times in UTC while the app shows
them correctly. The live example (<https://ripls.app/go/q5M3tYzh>) renders
`og:description = "Thursday, July 2 · 12:00 AM · Magnolia Cafe"` for an event
that actually happens **Wednesday, July 1 at 6:00 PM** local time — the hour,
the day of week, *and* the date are all wrong. Because midnight-UTC events are
common (evening events in US timezones), many previews read as broken
"12:00 AM" placeholders in Messages/WhatsApp unfurls — exactly the surface the
phone-first pivot (#2492) depends on.

Three server-rendered surfaces share the same defect, each with its own copy of
the same "apply stored timezone, else silently fall back to UTC" helper:

| Surface | Function | What renders wrong |
| --- | --- | --- |
| Event SSR landing page + its OG tags | `formatEventWhen()` in `server/services/web/event_page.go:550` | Day label, date, time-of-day (page body **and** `og:description`) |
| `/go` invite link preview for experiences | `formatExperienceDate()` in `server/services/web/item_preview.go:266` | Date (can roll over a day) |
| Off-app SMS/email notification summaries | `formatSummaryDate()` in `server/notifications/off_app_summary.go:225` | Date (can roll over a day) |

The app is unaffected because it converts the stored unix timestamp to the
*device's* timezone at display time (`app/lib/core/utils/date_time_formatter.dart`);
SSR has no device timezone and depends entirely on stored data.

The stored data is deficient in four ways:

1. **`models.TimeRange` has no timezone field** (`proto/ripls/models/time.proto`),
   so range-timed events can never render timezone-aware. Notably, the
   informal-time LLM parser (`server/services/experience/convert_informal_time.go`)
   **already receives an IANA timezone from the LLM and discards it** for
   ranges because there is nowhere to put it. The data exists at creation
   time; the schema drops it.
2. **GenExperience paths persist the literal string `"UTC"`** when the request
   timezone is empty (`server/services/experience/gen_ai.go:66–73`,
   `gen_ai_streaming.go` → `resolveTimezone`), poisoning the stored value —
   indistinguishable from a real UTC-timezone event.
3. **Legacy events** predate the client's timezone plumbing and have empty
   `SpecificTime.Timezone`.
4. **All-day events** (`is_all_day` on both `SpecificTime` and `TimeRange`) are
   ignored by all three formatters, rendering as "12:00 AM".

New manual-picker events are already correct: `time_modal_view_model.dart`
sets `timezone: userTz` on every `SpecificTime` it builds.

## Context from Comments

The issue has no comments. Two pieces of out-of-band maintainer direction:

1. Schema changes are in scope — "it's fine to do the schema changes
   necessary to fix this right."
2. **Scope decision:** Option C — "let old events be wrong with no backfill."
   No new geo dependency, no backfill job; only forward-looking data fixes
   plus render-side degradation for bad legacy data.

## Key Codebase Facts (from research)

- **All API↔storage time conversion funnels through two functions**:
  `convertTimeAPIToModels()` (`server/services/experience/service.go:337`,
  called from `save.go:61`, `save.go:338`, `time_proposals.go:691`) and
  `services.ConvertTimeModelsToAPI()` (`server/services/experience_converters.go`).
  Adding a field means touching exactly these two plus the proto files.
- **No manual DB migration is needed.** protosql flattens nested message
  fields into columns and performs schema evolution at startup:
  `initializeDatabase` diffs expected vs. existing columns and issues
  `ALTER TABLE ... ADD COLUMN` for any missing ones
  (`server/storage/protosql_schema.go:367–386`), alongside the serialized
  proto blob.
- **`server/timezone` already exists** as a shared validation package
  (`ValidateIANA`, `IsValidOrEmpty`). It has no README.md today.
- Two read paths already consume `SpecificTime.Timezone` correctly and will
  benefit from better stored data for free: day-weather forecasting
  (`day_weather.go:163`, `get.go:108`).
- The SSR event page fetches the full `models.Location` row
  (`fetchPlaceName`, `event_page.go:263`) — relevant only to the descoped
  Option B resolver.

## Potential Solutions

### Option A: Read-time resolution only (no schema change)

- **Approach:** At render time, when the stored timezone is empty or `"UTC"`,
  resolve the timezone from the event's location coordinates and use it.
  No proto changes, no backfill.
- **Pros:** Smallest diff; fixes the live example immediately; no client work.
- **Cons:** Every render surface must fetch the location and call the resolver
  (`off_app_summary` and `item_preview` don't currently load locations);
  range events *without* a location stay broken; the `"UTC"` literal keeps
  being written; the fix lives in N render sites instead of the data.
- **Effort:** Small.

### Option B: Fix the data — schema + write-time stamping + backfill (render trusts storage)

*Not chosen — recorded for posterity; the resolver/backfill design lived in
revision 1 of this plan (git history).*

- **Approach:** Everything in Option C, plus: a lat/lng→IANA resolver
  (`github.com/ringsaturn/tzf`, `NewDefaultFinder()`, ~60MB steady-state —
  affordable within the 2Gi Cloud Run limit) behind a `timezone.Resolver`
  interface; write-time stamping of empty/`"UTC"` timezones from the event
  location's coordinates in `SaveExperience` / `ConfirmTime` /
  `ConfirmLocation`; and a one-time startup backfill (impact_backfill
  pattern) for legacy events.
- **Pros:** Fixes existing events (including the live example); every future
  consumer reads correct data.
- **Cons:** New ~60MB-resident dependency and a backfill job to shepherd —
  disproportionate to the residual harm once Option C's date-only degradation
  removes the "confidently wrong 12:00 AM" failure mode.
- **Effort:** Medium.

### Option C: Schema + stop-the-bleeding + render fixes ✅ CHOSEN

- **Approach:**
  1. Add `timezone` to `TimeRange` (api + models protos) and plumb it through
     the two conversion funnels, the informal-time parser (stop discarding the
     LLM's timezone), and the client's range-preserving edit path.
  2. Stop persisting the literal `"UTC"` fallback in GenExperience paths
     (keep UTC for LLM date math; persist empty when unknown).
  3. Fix the three formatters: honor `Range.Timezone`, honor `is_all_day`
     (date-only labels), treat empty **and literal `"UTC"`** stored timezones
     as unknown, and degrade to date-only labels when unknown (never render a
     fake "12:00 AM"); extract the shared localize-an-`ExperienceTime` logic
     into one helper.
- **Pros:** No new dependency; all new events render correctly on every
  surface; legacy/poisoned events degrade to a truthful date-only preview
  instead of a wrong time.
- **Cons:** Existing events (including the motivating example) keep a
  degraded preview, and their date can still be off by one day near midnight
  UTC. Accepted by maintainer decision.
- **Effort:** Small-to-medium.

## Resolved Questions

- **Schema changes allowed?** Yes — explicit maintainer direction.
- **Backfill / resolver?** No — maintainer decision (Option C). If revisited,
  Option B sketches the design; a backfill would need its cleanup issue filed
  before landing, per project convention.
- **Treat stored literal `"UTC"` as unknown at render time?** Yes. Genuine
  UTC-timezone venues are essentially nonexistent on land, while `"UTC"` is
  the known poison value written by the gen paths. Degrading those to
  date-only is strictly less wrong. (Simulation/e2e fixtures that really use
  UTC degrade to date-only previews — acceptable.)
- **What renders when no timezone is known?** Date-only labels computed in
  UTC and no time-of-day string. A possibly off-by-one date is strictly
  better than a confidently wrong "12:00 AM". `WhenSummary` collapses to the
  date-only form; templates must not emit dangling "·" separators.
- **`optional` on the new proto field?** No — plain `string` with `"" =
  unknown`, matching the sibling `SpecificTime.timezone` exactly (naming
  parity and presence semantics; `optional` here would make the two timezone
  fields diverge for no benefit).

## Open Questions

- **Where does the shared localization helper live?** — Recommendation: a new
  small shared package `server/eventtime` (single responsibility: interpreting
  `models.ExperienceTime` — localized time, timezone-known flag, all-day
  flag). Keeps `server/timezone` proto-free. Requires the standard package
  comment (`doc.go`) and a `README.md` (multi-file directory rule). The
  alternative — an exported helper inside `server/timezone` — is acceptable
  but couples a generic tz-string library to the models protos; note that
  `server/timezone` is missing its `README.md` either way and should gain one
  in whichever variant is chosen.

## Implementation Plan

### Phase 1: Render correctness with data we already have

**Goal:** No server-rendered surface ever shows a wrong or fake time again;
the three divergent formatters share one tested helper.

**Tasks:**

- [x] New shared package `server/eventtime` (with `doc.go` package comment,
      `README.md`, unit tests): maps a `models.ExperienceTime` →
      (localized `time.Time`, `hasTimezone bool`, `isAllDay bool`). Treats
      empty and literal-`"UTC"` stored timezones as unknown. Invalid stored
      timezone strings also degrade to unknown (render must never fail on bad
      data; this is display, not validation).
- [x] `formatEventWhen` (`event_page.go`): consume the helper; honor
      `is_all_day` and unknown-timezone by producing date-only labels and an
      empty `TimeFormatted`; `WhenSummary` collapses to the date-only form.
- [x] `templates/event.html`: guard the `TimeFormatted`/`WhenSummary` usages
      so empty time-of-day never leaves a dangling "·" separator in the page
      body or `og:description`.
- [x] Same helper + all-day/date-only treatment in `formatExperienceDate`
      (`item_preview.go`) and `formatSummaryDate` (`off_app_summary.go`).
- [x] Unit tests per surface: tz set / tz empty / tz `"UTC"` / invalid tz /
      all-day / range, including golden assertions on the rendered
      `og:description` in `event_page_test.go` / `item_preview_test.go` and
      the off-app summary tests.
- [x] `npm run build` (lint + compile) green.

**Validates:** The "confidently wrong" failure mode is gone for all existing
data, including the live example (degrades to a date-only preview).

### Phase 2: Schema — `TimeRange.timezone` + stop persisting the `"UTC"` literal

**Goal:** Every write path stores the best timezone it actually knows, so all
new events render fully correct times on every surface.

**Tasks:**

- [x] Add `string timezone` (IANA; `"" = unknown`; declared adjacent to
      `start_unix_sec`/`end_unix_sec`, next free field number) to `TimeRange`
      in **both** `proto/ripls/api/time.proto` and
      `proto/ripls/models/time.proto`. Proto comment describes meaning only
      (no vendor/mechanics). `npm run generate`. (No DB migration — protosql
      adds the flattened column at startup.)
- [x] Plumb through `convertTimeAPIToModels()`
      (`server/services/experience/service.go`) and
      `services.ConvertTimeModelsToAPI()` (`server/services/experience_converters.go`).
- [x] `convert_informal_time.go`: stop discarding `parsed.Timezone` for the
      `"range"` case. When the *request* provided no timezone and the LLM
      merely echoes `"UTC"`, store `""`; keep a real non-UTC timezone the LLM
      inferred from the text.
- [x] GenExperience paths (`gen_ai.go`, `gen_ai_streaming.go`
      `resolveTimezone` and its callers): keep UTC as the *computation* zone
      for LLM relative-date math, but persist `""` (not `"UTC"`) on
      `SpecificTime`/`TimeRange` when the request had no timezone.
- [x] Client: preserve `range.timezone` in the duration-edit path
      (`event_modal_view_model.dart` — the `TimeRange(...)` reconstruction)
      and set `userTz` anywhere the app constructs a `TimeRange` (audit
      confirmed `time_modal_view_model.dart` builds only `SpecificTime`
      today, which already carries `userTz`).
- [x] Validate incoming timezones with the existing
      `timezone.IsValidOrEmpty` where API time enters the service
      (`SaveExperience`, time proposals); reject invalid values with
      `connect.CodeInvalidArgument` (fail fast; user error, not
      `connecterr.Internal`).
- [x] Round-trip coverage for the new field in
      `server/services/experience/roundtrip_test.go` (save → get returns the
      same `range.timezone`), per proto_conventions.
- [x] Full `flutter test --reporter expanded` run (label-formatter/widget-test
      coupling gotcha) and `npm run build` +
      `go build -tags=integration ./...` (cross-package signature hygiene).
- [ ] Post-deploy validation on dev: create a range-timed event via the
      informal-time path and confirm the SSR page + `og:description` show
      venue-local labels; confirm a legacy `"UTC"` event (e.g. the live
      example) renders date-only with no "12:00 AM".

**Validates:** New range events and gen'd events render correctly end-to-end;
no new `"UTC"` poison enters the database.

### Descoped (by maintainer decision)

- Location→timezone resolver (`tzf`) and write-time stamping — Option B.
- Legacy-event backfill — Option B. Existing events keep degraded date-only
  previews indefinitely; if this ever matters (e.g. a marquee event's preview
  looks bare), re-open with Option B's design and file the backfill cleanup
  issue first, per convention.

## Audit Notes

Audited against: `docs/proto_conventions.md` (field parity, `optional`
semantics, round-trip tests, proto comments), `docs/server/architecture.md`
(shared-library pattern, package layout, testing levels, storage
schema-evolution), `docs/server/conventions.md` (fail-fast, CodeInternal
gate, build verification, package comments), `docs/invitations.md` (SSR `/go`
surface), review-prompt checklists for api_design / error_handling / testing /
dependency, plus direct code verification of `protosql_schema.go` column
evolution. Fixes applied in this revision: corrected the "no migration"
rationale (startup `ALTER TABLE`, not just the proto blob), committed to plain
`string` over `optional` with rationale, added the `"UTC"`-as-unknown render
rule (load-bearing under Option C), added `server/eventtime` packaging
requirements (doc.go, README, models-import isolation), added template
dangling-separator task, added invalid-timezone render tolerance vs.
write-path validation split, and restructured Phases to the decided Option C
scope (dependency review dimension is now moot — no new dependency).

## Related Issues

- #2049 — original SSR event landing page (Phase 1) where `formatEventWhen`
  was introduced.
- #2492 — phone-first pivot epic; SMS/OG link previews are its primary
  off-app surface, which is why broken unfurl times matter.
- #2572 — off-app SMS/email `/go` short links (`off_app_summary.go`, the
  third affected formatter).
