# Issue #2514: Revamp inbox & community calendar to match month × backdrop design

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2514
**Filed:** 2026-06-19 | **Last activity:** 2026-06-19
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [client/calendar.md](../client/calendar.md), [client/inbox.md](../client/inbox.md), [client/design.md](../client/design.md), [client/architecture.md](../client/architecture.md), [client/conventions.md](../client/conventions.md), [server/portfolio.md](../server/portfolio.md), [feed/nudges.md](../nudges.md), [content/stock_imagery.md](../stock_imagery.md), [client/geospatial.md](../client/geospatial.md), [proto_conventions.md](../proto_conventions.md), design mockup `docs/cowork/App Design/calendar-month-backdrop-recommendations.html`

## Problem Statement

The issue asks to revamp the **Home-tab inbox** and the **community calendar** to match the interactive mockup at
[`calendar-month-backdrop-recommendations.html`](../cowork/App%20Design/calendar-month-backdrop-recommendations.html)
("Month × Backdrop — get-outside calendar").

The mockup is a **major paradigm shift**, not a re-skin. It proposes:

1. A **month-grid** calendar (7×5 cells) where each day cell is a rounded photo
   thumbnail of that day's marquee event, overlaid with the day number, a
   multi-event count badge, and a **weather glyph in every cell**.
2. A **full-bleed photo backdrop** behind the entire screen, sourced from the
   selected day's event photo, with a layered scrim for contrast and a crossfade
   on day switch (honoring `prefers-reduced-motion`).
3. A **bottom-half day detail** over the same photo: for planned days, the
   time/title/subtitle + attendee avatars (multi-event days list earliest-first
   with a `hero` row highlighted); for **open days, a "smart suggestion"** —
   a recommendation derived from *weather × the user's activity history × the
   people who usually come*, with alternative chips and a "Plan it with them" CTA.

**The critical gap between the mockup and reality** (confirmed by codebase
research):

- **There is no month-grid calendar today.** The current calendar
  ([`home_calendar_screen.dart`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart),
  180 lines) is a **week-strip + day-agenda** view rendering
  [`UpNextWeekView`](../../app/lib/presentation/widgets/home/up_next_week_view.dart).
- **There is no weather data anywhere** in the app, protos, or server. The
  mockup puts a forecast glyph + temperature in *every* cell.
- **There is no recommendation/suggestion engine.** The closest analog is the
  server-driven `NudgePayload` (one contextual card per empty day / zero-state),
  which reuses the feed nudge system — not the weather×history×crew engine the
  mockup implies.
- The mockup's typography (Fraunces/Newsreader/Inter) and raw green+gold palette
  are **not** our design system. Ripls is **Ink & Sage** (deep-sage actions, Libre
  Baskerville serif headings, system-sans body), with all accent pixels routed
  through `AppColors` and enforced by `npm run lint:dart:colors`. The mockup's
  gold accent in particular has no token today.

**Ambiguity to resolve up front:** the issue title says "inbox **and** community
calendar," but the mockup is entirely a *calendar*. The inbox tie-in ("align with
the same visual language") is under-specified — see Open Questions. The bulk of
the concrete work is the calendar.

## Context from Comments

**The issue has no comments.** It was filed programmatically (the body is rich,
but there's been no maintainer discussion or scoping debate). This is a signal
that the Open Questions below need owner input *before* implementation — in
particular the weather-source and recommendation-engine decisions, which are
large enough to be their own projects.

## Codebase grounding (what exists today)

| Concern | Current state | Relevant file |
|---------|---------------|---------------|
| Calendar UI | week-strip + adaptive day agenda (0→nudge, 1–3→hero cards, 4+→time rows) | [`home_calendar_screen.dart`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart), [`up_next_week_view.dart`](../../app/lib/presentation/widgets/home/up_next_week_view.dart) |
| Day card | full-bleed photo + gradient + "when" badge + title + face stack | [`calendar_day_hero.dart`](../../app/lib/presentation/widgets/home/calendar_day_hero.dart) |
| Community calendar | same timeline, filtered to one community | [`workshop_library_calendar_panel.dart`](../../app/lib/presentation/screens/workshop/workshop_library_calendar_panel.dart), `workshopCommunityCalendarProvider` |
| Calendar data | `GetHomeViewResponse.calendar` (`repeated HomeUpNextEntry`), assembled by `assembleHomeCalendar` | [`home_view.go`](../../server/services/portfolio/home_view.go), [`portfolio.proto`](../../proto/ripls/api/portfolio.proto) |
| Entry shape | `kind`, `timeUnixSec`, `timeDisplay`, `title`/`subtitle`, `communityId`, `thumbnailMediaId`, `contentId`/`itemType`, `helpers` | `HomeUpNextEntry` |
| Empty-day prompt | server `NudgePayload`, reuses feed nudge pool | [`nudges.md`](../nudges.md), `assembleHomeCalendar` → `InboxNudge` |
| Photos | user-uploaded media via `CachedMediaImage`, keyed by `thumbnailMediaId`; sage fallback when absent | [`media.md`](../client/media.md) |
| Weather | **none** | — |
| Recommendations | **none** (only server `NudgePayload`) | — |
| Theme | Ink & Sage tokens via `AppColors`/`AppTheme.headingFont`; no gold accent | [`design.md`](../client/design.md), `app/lib/core/theme/` |

Key architectural facts the plan must respect:
- The client **never invents dates/times** — it groups by the server's
  `time_unix_sec` and renders `time_display` verbatim ([calendar.md](../client/calendar.md)).
- One server-assembled `calendar` feeds **both** the inbox calendar (all
  communities) and the Workshop community calendar (filtered). A grid redesign
  should preserve that single-source contract so both surfaces benefit.
- MVVM + Riverpod, async-first immutable Freezed state, repository-cached fetch
  ([architecture.md](../client/architecture.md)). ViewModels are the only layer
  that calls repositories; screens read the cached `homeTabProvider`. The grid's
  **selected-day** is screen-local UI state (acceptable `setState`/local use), not
  app state.
- **Media:** day-cell thumbnails and the backdrop must use `CachedMediaImage`
  (never `Image.network`) with stable media-ID cache keys, thumbnail tier for
  cells ([conventions.md](../client/conventions.md) → Media Display).
- **Colors/typography:** every accent pixel through `AppColors`, headings through
  `AppTheme.headingFont`; `npm run lint:dart:colors` blocks raw hex / `Colors.amber`
  outside `app/lib/core/theme/`. The mockup's gold is exactly the kind of literal
  that fails this gate.
- **i18n:** all new user-facing copy ("Suggested for you", "Plan it with them",
  weather phrasing, temperature) must be ARB-sourced via `context.l10n`;
  viewmodels never resolve strings ([i18n.md](../client/i18n.md)). Apply the
  **"Event" not "Experience"** terminology rule to any visible label.
- `npm run lint:dart:size` fails any non-generated `app/lib/**` file > 1,000 lines.

## Potential Solutions

### Option A: Full mockup parity in one push (grid + weather + recommendations)
- **Approach:** Build the month grid, integrate a real weather provider
  (server-side forecast keyed to the community/user location), and build the
  weather×history×crew recommendation engine — all before shipping.
- **Pros:** Matches the mockup exactly; one cohesive launch.
- **Cons:** Two of the three pillars (weather, recommendations) are net-new
  *backend subsystems*, each with its own data source, caching, cost, privacy,
  and proto surface. Months of work; high risk; nothing ships until all of it
  lands. The recommendation engine especially is a product in its own right.
- **Effort:** Large (XL).

### Option B: Layered delivery — grid first, weather and recommendations as follow-on phases (recommended)
- **Approach:** Treat the mockup as a *visual target* and deliver in shippable
  increments that each stand on their own:
  1. **Month-grid calendar + full-bleed backdrop** over the **existing**
     `HomeUpNextEntry` data (no new backend). Weather glyphs and open-day
     suggestions are simply absent in v1; open days reuse today's `NudgePayload`
     prompt. This alone is a dramatic visual upgrade and is purely client-side.
  2. **Weather substrate** — add a forecast source and surface the per-day glyph
     once data exists.
  3. **Open-day recommendations** — build (or stub then build) the suggestion
     engine.
  4. **Inbox visual alignment** — bring the inbox into the same language once the
     calendar system is settled.
- **Pros:** Ships value early; de-risks the two hard backend bets; each phase is
  independently reviewable and reversible; preserves the single-`calendar`-source
  contract. Lets the owner cut the line after any phase.
- **Cons:** Intermediate states don't fully match the mockup (no weather, generic
  open days) — must be communicated as intentional.
- **Effort:** Phase 1 Medium; Phases 2–3 Large each; Phase 4 Small–Medium.

### Option C: Re-skin only (keep week view, adopt backdrop + palette)
- **Approach:** Keep the week-strip agenda; just adopt the full-bleed backdrop
  treatment, gold-accent tokens, and richer day detail without the month grid,
  weather, or recommendations.
- **Pros:** Smallest effort; low risk.
- **Cons:** Misses the defining idea of the mockup (the month × backdrop grid and
  the self-suggesting open days). Likely to be seen as not delivering the ask.
- **Effort:** Small.

## Recommendation

**Option B (layered delivery), with Phase 1 as the committed deliverable.** The
former open questions are now resolved (see [Resolved Decisions](#resolved-decisions-owner-2026-06-19));
only the *production* weather-provider choice remains deferred and it gates
nothing before the Phase 2 Open-Meteo MVP.

Rationale: the mockup conflates a UI redesign (the grid + backdrop — squarely a
client task we can do now against existing data) with two net-new backend
subsystems (weather, recommendations) that the issue hand-waves as "confirm we
have / decide whether." Shipping the grid first delivers the headline visual
change, proves the layout against real `HomeUpNextEntry` data, and buys time to
make the weather and recommendation calls deliberately rather than under redesign
pressure. It also keeps each phase under the architectural contracts (single
calendar source, no client-invented dates, `AppColors` routing, 1k-line file cap).

If the owner wants the *feeling* of the mockup fastest, Phase 1 plus the
gold-accent tokens gets ~80% of the visual impact at ~20% of the cost.

## Resolved Decisions (owner, 2026-06-19)

1. **Scope / "the inbox."** The target is the **calendar screen reached from the
   Home tab's Calendar button** — today that opens the week-view
   ([`home_calendar_screen.dart`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart)).
   **Replace that week view outright with the new month × backdrop design.** No
   structural redesign of the Home root or the separate portfolio inbox is in
   scope; Phase 4 narrows to making the entry point / surrounding chrome feel
   consistent. The Workshop community calendar (same shared `calendar` source)
   inherits the new grid too.

2. **Weather — server-side, provider-abstracted, mirror the stock-imagery
   architecture.**
   - **MVP provider: Open-Meteo.** No key/signup, free non-commercial up to 10k
     calls/day, `daily=weather_code,temperature_2m_max`. WMO `weather_code` maps
     onto the five-glyph scheme. CC-BY attribution required.
   - **Production: Open-Meteo Professional (flat €99/mo, 1M calls, commercial
     license) or Google Weather API ($0.15/1k, 10k/mo free, folds into our GCP
     bill, DeepMind-enhanced; "Powered by Google" attribution; no China and a few
     other gaps — fine for a US/Colorado launch).** The free Open-Meteo tier is
     **non-commercial only**, so the moment the app monetizes we must move to a
     paid plan. Decision deferred to production but the abstraction below makes it
     a config swap.
   - **Forecast horizon is the real gotcha.** No API forecasts a full month —
     Open-Meteo ~16 days, Google 10, NWS 7. For days beyond the forecast horizon
     (and any future month) fall back to **climate normals** (30-yr typical),
     which Open-Meteo also serves free via its Historical/Climate API. Style/label
     those cells as **"typical"** (visually distinct from a real forecast).
   - **Caching is the cost lever, not the API.** Weather for a lat/lng is identical
     for every viewer, so **never call per-user or per-request.** Fetch per
     location **rounded to ~0.1° (~11 km) grid**, cache server-side (Postgres/Redis),
     refresh every few hours. ~1k locations × every-3h ≈ 240k calls/mo — trivial
     (~$35 on Google, within Open-Meteo Pro).
   - **Architecture: reuse the stock-imagery shape** ([stock_imagery.md](../stock_imagery.md)):
     a provider-independent `WeatherProvider` interface, a cache wrapper, a
     fallback chain, a **fake provider** for tests, and CLI-flag/env-var key config
     (`--openmeteo-*` / `OPEN_METEO_*`, `--google-weather-*`, etc.) — exactly like
     `StockImageryProvider` / `cached_stock_imagery.go` / `fake_provider.go`.
   - **Client stays a dumb renderer** of a server-formatted glyph token + temp
     string; temperature unit formatted **server-side** per recipient locale
     (mockup is °F).
   - **Document it:** once weather support exists, write **`docs/weather.md`**
     (provider interface, caching/grid, horizon + climate-normals fallback,
     attribution, config) and add its `context:` frontmatter so `docs/llms.txt`
     picks it up. *(Not created now — it documents code that doesn't exist yet.)*

3. **Recommendation engine — placeholder in Phase 1, real engine in Phase 3,
   server-side.** Confirmed. Reuse the nudge/feed assembly or workshop
   momentum/needs detector chain rather than a new subsystem. **Far-future days
   are lower-confidence:** a suggestion built on a real forecast is meaningfully
   stronger than one built on a 30-yr average, so the engine must treat
   beyond-horizon ("typical") days as reduced confidence.

4. **Backdrop photos, gold accent, photo-per-cell performance** — all confirmed as
   previously recommended: event `thumbnailMediaId` + sage fallback in v1
   (stock-imagery backdrops for open days only if/when recommendations land); **no
   raw gold literal** — map onto an existing token or add one centrally in
   `design/tokens.json` with design sign-off; thumbnail-tier `CachedMediaImage`
   with a hard fallback color and deferred decode, verified against the performance
   checklist.

No open questions remain blocking; the production weather-provider choice
(Open-Meteo Pro vs. Google) is the only deferred decision and does not gate Phase 1
or the Phase 2 MVP.

## Implementation Plan

### Phase 1: Month-grid calendar + full-bleed backdrop (client-only, existing data)
**Goal:** Replace the Home-tab Calendar-button week view
([`home_calendar_screen.dart`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart))
with the mockup's month × backdrop grid, rendering today's `HomeUpNextEntry` data.
No weather, no recommendations — open days reuse the existing nudge prompt.
**Tasks:**
- [ ] Build a `CalendarMonthGrid` widget (Mon-first 7×N grid) that buckets the
      cached `calendar` entries by local day (reuse the existing day-grouping
      logic from `UpNextWeekView`; do not re-derive dates).
- [ ] Day cell: rounded thumbnail of the day's marquee event (`hero` if present,
      else earliest), day number, multi-event count badge, today/selected states.
      Sage fallback when no `thumbnailMediaId`.
- [ ] Full-bleed backdrop layer behind the screen sourced from the selected day's
      photo, layered scrim for contrast, crossfade on day switch gated by
      `MediaQuery.disableAnimations` / reduce-motion.
- [ ] Bottom day-detail panel over the backdrop: single-event (time/title/subtitle
      + face stack) and multi-event (earliest-first list, `hero` row emphasized),
      reading `timeDisplay` verbatim.
- [ ] Open day → existing `NudgePayload`/"Nothing planned" prompt in the new
      panel styling (placeholder for Phase 3).
- [ ] Wire into both surfaces: the inbox Calendar
      ([`home_calendar_screen.dart`](../../app/lib/presentation/screens/portfolio/home_calendar_screen.dart))
      and the Workshop community calendar (filtered via
      `workshopCommunityCalendarProvider`) — preserving the single-source contract.
- [ ] Keep new widgets < 1,000 lines (split the grid, cell, and detail panel);
      route all accent colors through `AppColors`, headings through
      `AppTheme.headingFont`. Resolve the gold-accent question (no raw hex) before
      writing cell/selection styling.
- [ ] All new copy via ARB/`context.l10n` (no hardcoded strings); "Event" not
      "Experience" in any visible label.
- [ ] Accessibility: a **descriptive semantic label per day cell** (date +
      event/empty + later, weather) — the raw weather **emoji must not be the
      announced content**; reduce-motion honored (crossfade disabled via
      `MediaQuery.disableAnimations`); 44px touch targets; follow
      `reviews/accessibility_review_prompt.md`.
- [ ] Performance: thumbnail-tier `CachedMediaImage` per cell, hard fallback color,
      avoid decoding 35 full images; spot-check against
      `reviews/performance_review_prompt.md`.
- [ ] Widget tests for grouping, multi-event hero selection, empty-day fallback,
      reduce-motion path.
**Validates:** Whether the month × backdrop layout works against real,
variable-density calendar data and on both surfaces — the riskiest *UI* assumption
— without committing to any backend work.

### Phase 2: Weather substrate (server-side, Open-Meteo MVP)
**Goal:** Add the per-day forecast glyph + temperature to every cell, sourced
server-side, cached per location, with a climate-normals fallback beyond the
forecast horizon.
**Tasks:**
- [ ] **Provider abstraction mirroring stock-imagery** ([stock_imagery.md](../stock_imagery.md)):
      a provider-independent `WeatherProvider` interface (`server/weather/`), a
      **cache wrapper**, a **fallback chain**, and a **fake provider** for tests —
      same shape as `StockImageryProvider` / `cached_stock_imagery.go` /
      `stock_imagery_fallback.go` / `fake_provider.go`.
- [ ] **MVP provider: Open-Meteo** (`daily=weather_code,temperature_2m_max`, no
      key). Map WMO `weather_code` → the five glyph tokens. CC-BY attribution
      surfaced where the design shows it.
- [ ] **Config like the imagery providers:** CLI flags + env vars
      (`--openmeteo-*` / `OPEN_METEO_*`; later `--google-weather-*` /
      `GOOGLE_WEATHER_*`), at least one provider enabling the feature. New deps →
      `reviews/dependency_review_prompt.md`.
- [ ] **Caching is the cost control.** Key by location **rounded to ~0.1° (~11 km)
      grid**, store in Postgres/Redis, refresh every few hours; **never per-user /
      per-request.** Drive off the community/user place already available via
      [`geospatial.md`](../client/geospatial.md) / `server/location/`.
- [ ] **Forecast horizon + climate-normals fallback.** Real forecast only ~16 days
      out (Open-Meteo); for days beyond the horizon and future months, fall back to
      **climate normals** (Open-Meteo Historical/Climate API). Mark those days as
      **"typical"** in the response so the client can style/label them distinctly.
- [ ] **Proto:** surface as a **sibling per-day forecast list** on
      `GetHomeViewResponse` (e.g. `repeated DayForecast forecast`, each with
      `date_unix_sec`, `optional` glyph token, server-formatted temp string, and an
      `is_typical`/confidence flag) — **not** a field on `HomeUpNextEntry`
      (`HomeUpNextEntry` is reused by `up_next` → unused-field reuse smell; and open
      days carry no entry to hang weather off). Use `optional` for presence;
      vendor-neutral proto comments.
- [ ] **Assembly:** follow the portfolio **pure-function** pattern (I/O in the
      `fetchAll`/extras phase, pure join) and **never fail the home view** — a
      forecast error degrades to no-glyph, mirroring the nudge contract.
- [ ] **Temperature unit** formatted **server-side** per recipient locale/region
      (mockup is °F), consistent with the l10n render boundary; client renders the
      string verbatim.
- [ ] **Client:** render the glyph in every cell + the weather pill in the day
      detail, "typical" cells styled distinctly, with a **descriptive semantic
      label** (not the bare emoji).
- [ ] **Write `docs/weather.md`** (provider interface, grid/caching, horizon +
      climate-normals fallback, attribution, config) with `context:` frontmatter;
      run `npm run generate:context-map`.
- [ ] Tests: fake-provider flow, cache hit/refresh, grid rounding, climate-normals
      fallback path, round-trip for the new proto field, client glyph + no-data
      fallback.
**Validates:** That weather can be sourced cheaply per place (forecast + normals)
and rendered without a per-day client round-trip.

### Phase 3: Open-day recommendations
**Goal:** Open days propose a real plan (weather × history × usual crew) with
alternative chips and a "Plan it with them" CTA.
**Tasks:**
- [ ] Decide engine shape: extend the nudge/feed assembly or the workshop
      momentum/needs detector chain vs. a new subsystem (prefer reuse). If it uses
      an LLM, go through the existing `ai.Provider` interface ([server/llm.md](../server/llm.md)),
      not a bespoke client.
- [ ] Server: assemble per-open-day suggestions (activity history + recurring
      participants + weather-fit), exposed on the calendar response as their own
      dedicated message (no cross-RPC reuse; `optional` presence). Must follow the
      pure-function assembly pattern and **never fail the home view**.
- [ ] **Confidence by horizon:** a suggestion built on a real forecast is stronger
      than one built on climate normals — consume the Phase 2 `is_typical`/
      confidence flag and down-weight (or soften the copy for) beyond-horizon days
      rather than presenting a 30-yr average as a firm recommendation.
- [ ] Privacy: "who usually comes" is participant data — keep history/crew
      derivation server-side, expose only the avatars/names the viewer is already
      entitled to see (mirror the profile shared-community privacy invariants); no
      cross-community leakage. Check against `reviews/security_review_prompt.md`.
- [ ] Client: suggestion panel with chips to flip alternatives and a CTA that
      routes into the existing create/plan flow; all copy ARB-sourced.
- [ ] Tests: server suggestion assembly + round-trip for new fields; client
      chip-flip + CTA routing; disposal-safety test for any new autoDispose VM.
**Validates:** Whether self-suggesting open days are useful enough to justify the
engine — the riskiest *product* bet, deliberately last.

### Phase 4: Entry-point / chrome alignment (narrowed)
**Goal:** Make the Home-tab Calendar entry point and the surrounding chrome feel
consistent with the new calendar — **not** a structural redesign of the Home root
or portfolio inbox (per the resolved scope).
**Tasks:**
- [ ] Align the Home-tab Calendar button affordance and any shared
      typography/accent tokens with the new calendar surface.
- [ ] Verify no regressions to the Home-root sections, decisions, and routing.
- [ ] Tests/visual checks for the touched rows.
**Validates:** That the entry point and the new calendar read as one system. This
phase is intentionally small and may fold into Phase 1's PR if trivial.

## Related Issues
- #2435 — the Home tab "v4.1" inbox redesign this builds on (current product).
- #2487 — workshop v4.1 redesign (recently merged; adjacent surface).
- The in-progress `inbox-nudges` branch (commits "First pass at nudges and inbox
  improvements", "Add nudge for hero card") — this work should land on top of /
  coordinate with it.
- #2441 — Ink & Sage tokenization (the design-system constraints the gold-accent
  question must respect).

---

## Audit notes (`/ripls-audit`, 2026-06-19)

Audited against the selected standards below. The plan was already well-grounded;
the audit added/sharpened these compliance points (all applied in-place):

1. **Proto shape for weather (most material).** Original wording floated putting a
   forecast field on `HomeUpNextEntry`. Corrected to a **sibling per-day forecast
   list on `GetHomeViewResponse`** because `HomeUpNextEntry` is reused by `up_next`
   (unused-field-in-one-direction reuse smell) and because **open days carry no
   entry** yet still need weather. Specified `optional` presence per
   `proto_conventions.md`.
2. **Server assembly discipline.** Phases 2–3 must follow the portfolio
   **pure-function** assembly (I/O in `fetchAll`/extras, pure join) and the
   **"never fail the home view"** degradation rule (mirroring the nudge contract).
3. **i18n.** Added explicit ARB/`context.l10n` tasks for all new copy and the
   **"Event" not "Experience"** terminology rule — was missing.
4. **Accessibility.** Day-cell semantic labels must be **descriptive, not the bare
   weather emoji**; reduce-motion via `MediaQuery.disableAnimations`.
5. **Media + color + size gates.** Made explicit: `CachedMediaImage` (never
   `Image.network`), thumbnail tier, `AppColors`/`AppTheme.headingFont` routing
   (the mockup's gold literal fails `lint:dart:colors`), 1k-line file cap.
6. **Privacy.** Phase 3 "who usually comes" is participant data — keep derivation
   server-side, expose only entitled avatars, no cross-community leakage.
7. **Temperature units.** Format server-side per recipient locale (mockup is °F),
   consistent with the l10n render boundary; client stays a dumb renderer.

**Docs consulted:** `client/architecture.md`, `client/conventions.md`,
`client/design.md`, `client/calendar.md`, `client/inbox.md`, `proto_conventions.md`,
`server/portfolio.md`, `feed/nudges.md`, `content/stock_imagery.md`,
`client/geospatial.md`, and the review prompts for accessibility, performance,
api_design, security, dependency, and testing (`docs/reviews/*`). Not yet read in
full — defer to implementation time — `client/i18n.md`, `server/l10n.md`,
`server/llm.md`, and `client/media.md` (their relevant rules are cited above).

**No blocking conflicts** with existing documented decisions were found. The plan's
main risk remains scope (two net-new backend subsystems), which the phased approach
deliberately isolates.

### Update (owner direction, 2026-06-19)

The Open Questions section was replaced by [Resolved Decisions](#resolved-decisions-owner-2026-06-19).
Key locks: (a) scope = **replace the Home-tab Calendar-button week view** with the
new design (Phase 4 narrowed to entry-point/chrome); (b) weather = **server-side,
Open-Meteo MVP**, provider-abstracted **mirroring the stock-imagery architecture**,
cached per ~0.1° location grid with a **climate-normals fallback** beyond the
~16-day forecast horizon ("typical" days, lower recommendation confidence),
temperature formatted server-side; (c) a **`docs/weather.md`** is required once
weather lands. Production weather provider (Open-Meteo Pro vs. Google Weather) is
the only deferred choice. Phase 2 rewritten accordingly.
