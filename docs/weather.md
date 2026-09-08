---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: The Home calendar weather substrate — a provider-independent weather subsystem (Open-Meteo MVP) cached per place, with a climate-normals fallback beyond the forecast horizon, surfaced per calendar day on GetHomeView and rendered as a glyph in every grid cell.
  globs: [server/weather/**, server/services/portfolio/home_weather.go, app/lib/presentation/widgets/home/calendar/**]
  triggers: [weather, forecast, open-meteo, climate-normal, temperature, calendar-weather, day-forecast, glyph]
  lens: [server, client, domain]
  domain: server
freshness:
  verified_commit: "d708ceb8c"
  verified_on: "2026-07-06"
---
# Calendar Weather

The Home calendar (#2514) paints **weather in every day cell** — a coarse sky
glyph plus a daily high. Weather is the calendar's *substrate*: it appears on
planned and open days alike, and it fits the open-day suggestions
([client/calendar.md](client/calendar.md), [client/inbox.md](client/inbox.md)).

This doc covers where weather comes from, how it's cached, and how it reaches the
client.

## The subsystem (`server/weather`)

The design mirrors the **stock-imagery** subsystem ([stock_imagery.md](stock_imagery.md)):
a provider-independent interface, a cache wrapper, and a fake provider for tests
— but keyed **geospatially** (a rounded lat/lng grid), not semantically.

```
CachedProvider(            ← per-place ~0.1° grid, 3h TTL, stale-while-revalidate
  OpenMeteoProvider )      ← forecast ≤16 days ∥ climate-normal proxy (parallel)
```

| File | Role |
|------|------|
| [`provider.go`](../server/weather/provider.go) | `Provider` interface, `DayWeather`/`Condition`/`LatLng`, the ~0.1° `Snap` |
| [`open_meteo.go`](../server/weather/open_meteo.go) | `OpenMeteoProvider` — real forecast + archive-based climate-normal fallback |
| [`wmo.go`](../server/weather/wmo.go) | WMO `weather_code` → five-bucket `Condition` |
| [`cached.go`](../server/weather/cached.go) | `CachedProvider` — per-place in-process cache: 3h TTL, stale-while-revalidate, failure backoff (the cost *and latency* lever) |
| [`fake.go`](../server/weather/fake.go) | `FakeProvider` — deterministic weather for tests |

### Five buckets

A daily forecast does not justify more than a handful of sky states, so
`Condition` has five — clear, mostly-sunny, partly-cloudy, overcast, rain — that
map 1:1 onto the client's five glyphs (☀️ 🌤️ ⛅ ☁️ 🌧️). **All precipitation**
(drizzle, rain, snow, showers, thunderstorm) collapses to `rain`; there is no
separate snow glyph.

## Caching is the cost lever, not the API

Weather for a lat/lng is identical for every viewer looking at it, so it is
**never fetched per user or per request**. `CachedProvider`:

- **Snaps the place to a ~0.1° (~11 km) grid** (`LatLng.Snap`) so every viewer
  near a place shares one cache key.
- **Caches per (grid point, timezone, day-span) with a 3-hour TTL.**
- **Serves stale-while-revalidate** (#2646): once an entry outlives the TTL —
  or the daily span rolls over at midnight — the most recent good data for the
  place (up to a **48-hour stale ceiling**) is returned immediately and a
  background refresh (detached from the request context, collapsed via
  singleflight) brings the cache current. A request only waits on the network
  when the place has never been fetched on that instance.
- **Negatively caches failures**: a provider error suppresses refetches for
  that place for **5 minutes**, so an upstream brownout costs one bounded
  fetch per window instead of stalling every request until its timeout.

The hourly cache gets the same stale-while-revalidate and backoff treatment,
but only for the exact hour span — hours for a different span are the wrong
data, not slightly-old data, so there is no cross-span fallback.

A thousand distinct places refreshed every few hours is a few hundred thousand
calls a month total — trivial on any plan. The cache is **in-process**; a
multi-instance deployment that wants a shared cache (Redis/Postgres) swaps
`CachedProvider` for one backed by shared storage without touching the `Provider`
interface or any caller.

## Forecast horizon and the climate-normals fallback

No API forecasts a full month — Open-Meteo reaches ~16 days, Google ~10, NWS ~7.
The calendar paints ~46 days forward, so days past the horizon (and future
months) come from **climate normals** instead of a real forecast.

The MVP approximates normals by reading the **same calendar dates one year
earlier** from Open-Meteo's free archive endpoint, then shifting the result
forward a year and flagging it `IsTypical`. The client styles those days as
**"Typical"**, and the suggestion engine treats them as **lower confidence** — a
suggestion built on a 30-year average is weaker than one on a real forecast. A
true multi-year normal is the production upgrade; the seam is the
`OpenMeteoProvider.fetchRange` archive branch.

## Providers

| Stage | Provider | Notes |
|-------|----------|-------|
| **MVP (now)** | **Open-Meteo** | No API key. `daily=weather_code,temperature_2m_max`. CC-BY attribution. Free tier is **non-commercial only**. |
| **Production** | **Open-Meteo Professional** (flat €99/mo, commercial license) **or Google Weather API** ($0.15/1k, folds into GCP billing, ~10-day forecast, "Powered by Google" attribution; no China + a few gaps — fine for a US launch) | A new `Provider` implementation behind the same interface; pick per cost/ops. |

The endpoint hosts are constant (never user-controlled), so the outbound
requests carry no SSRF surface.

## Wiring and the wire shape

- **Wiring:** `server/wiring.go` builds **one** shared
  `weather.NewCachedProvider(weather.NewOpenMeteoProvider())` and injects it into
  **both** the portfolio service (`SetWeatherProvider`, for the calendar) and the
  experience service (for the event-day forecast), so they share the per-place
  cache. The keyless Open-Meteo MVP is on by default. (`SetWeatherProvider` is
  optional at the service level: unset — e.g. in tests — yields no weather.)
- **Shared conversion:** [`server/weatherapi`](../server/weatherapi/) converts a
  `weather.DayWeather` into the `api.DayForecast` wire type (condition enum,
  locale-formatted temperature, summary) and fetches a single day's forecast
  (`DayForecastFor`). Both the calendar and the event view format weather
  identically through it.
- **Per-request:** `GetHomeView` resolves the viewer's **primary-residence
  location** ([home_weather.go](../server/services/portfolio/home_weather.go) →
  `resolveViewerPlace`), asks the provider for the calendar window, and converts
  each `DayWeather` to `api.DayForecast`. **Best-effort, like the inbox nudge —
  a weather failure never fails the Home view.** The request waits at most
  **1200ms** for the fetch (`fetchForecastWithBudget`, #2646): past the budget
  the view renders without weather while the detached fetch finishes and warms
  the cache for the next request. With the stale-while-revalidate cache, only
  a place's very first fetch on a fresh instance can hit this budget.
- **Units server-side:** the provider works only in Celsius (so the per-place
  cache stays locale-neutral); `weatherapi.ToAPI` formats `temperature_display`.
  The app **defaults to Fahrenheit everywhere** for the US-first launch
  (`weatherapi.UsesFahrenheit`); the function keeps the timezone seam for a future
  locale-based unit choice. The client renders the string verbatim.
- **Proto:** `repeated DayForecast forecast` on `GetHomeViewResponse` — a
  **sibling per-day list**, not a field on `HomeUpNextEntry` (which `up_next`
  reuses, and which open days have no instance of). Each carries
  `date_unix_sec`, a `DayForecastCondition`, `temperature_display`, a `summary`,
  and `is_typical`.

## Client rendering

[`app/lib/presentation/widgets/home/calendar/calendar_weather.dart`](../app/lib/presentation/widgets/home/calendar/calendar_weather.dart)
maps the condition enum to a glyph and a **localized** label. The raw emoji is
never the announced content: every day cell's accessibility label is built from
the localized condition name + temperature (`calendarWeatherSemantic`), and the
glyph itself is wrapped in `ExcludeSemantics`. The day detail shows a weather
pill (the sunny state uses the warm accent; others a glass pill).

## The event view

`GetExperience` carries an `optional DayForecast forecast` — the weather for the
event's **scheduled day at its own coordinates** (`latitude_deg`/`longitude_deg`),
formatted in the event's `SpecificTime.timezone`. The experience service computes
it best-effort via `weatherapi.DayForecastFor`
([get.go](../server/services/experience/get.go) → `attachEventForecast`); it stays
unset when there's no scheduled time, no coordinates, or a provider error. The
client shows it as a `WeatherChip` on the event's WHEN row
([weather_chip.dart](../app/lib/presentation/widgets/weather/weather_chip.dart),
shared with the calendar day detail) and a compact glyph + temperature on the
content view's WHEN fact card.

### Hour-by-hour weather (the "When" screen strip)

The expanded "When" screen paints an **hour-by-hour** strip around the event
window, and repaints it for any day the viewer taps to explore. That granularity
is served on demand:

- **Provider:** `weather.Provider` also exposes `HourlyWeather(place, from, to,
  tz)` returning one `HourWeather` per hour. Unlike the daily path there is **no
  climate-normal fallback** — hours past the ~16-day forecast horizon are simply
  omitted, so a far-future day yields none and the client shows "forecast
  available closer to the day". `CachedProvider` caches hourly per (grid point,
  tz, hour span) on the same 3-hour TTL; `weatherapi.HourlyForecastFor` converts
  a day's hours to the `api.HourForecast` wire type.
- **RPC:** `ExperienceService.GetExperienceDayWeather(experience_id,
  date_unix_sec)` returns `repeated HourForecast` **plus an `optional
  DayForecast day_forecast`** for that local day at the event's coordinates
  ([day_weather.go](../server/services/experience/day_weather.go)). The daily
  field is the fallback the strip renders when there are no hourly entries:
  hourly has no climate-normal fill, so a day beyond the ~16-day horizon returns
  no `hours`, but `day_forecast` still resolves from normals — the strip shows
  the day's coarse weather instead of an empty note. Best-effort and lazy — the
  client
  ([experience_day_weather_provider.dart](../app/lib/presentation/viewmodels/experience_day_weather_provider.dart))
  fetches one day at a time, keyed by `(experienceId, day)`, and the server's
  per-place cache keeps repeated taps cheap.
- **Calendar window:** `ExperienceService.GetExperienceCalendarWeather(experience_id)`
  returns `repeated DayForecast` across the month-grid window (today forward,
  stretched to cover the event, capped) — all at the **experience's** coordinates,
  so the "When" month grid paints weather for where the event is rather than the
  viewer's home. (The Home calendar's `forecast` is the viewer's primary
  residence; the two are intentionally different surfaces.)

## Open-day suggestions

The weather feeds the open-day suggestion engine
([home_suggestions.go](../server/services/portfolio/home_suggestions.go)): each
open day's suggestion reason leads with the forecast summary, and beyond-horizon
("typical") days are flagged `is_low_confidence`. See the calendar docs for the
engine itself.

## Testing

`server/weather` has a fake provider and unit tests for the WMO mapping, the grid
snap, cache collapse/refresh, and the daily-array parse. `server/weatherapi`
tests the unit formatting and condition conversion. No test calls the live API.

---

**Last Updated:** 2026-07-06 — stale-while-revalidate cache, failure backoff,
parallel endpoint fetches, and the GetHomeView fetch budget (#2646)
