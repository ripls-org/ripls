# weather

Per-day weather for the Home calendar — a coarse sky condition and a daily high
for each day at a place. Weather is the calendar's substrate: every day cell
carries a glyph, planned or open, and open-day suggestions are weather-fitted.

The package mirrors the stock-imagery subsystem (`server/media`): a
provider-independent interface, a cache wrapper, and a fake provider for tests.

## Key files

| File | Purpose |
|------|---------|
| `provider.go` | `Provider` interface, `DayWeather`/`Condition`/`LatLng` types, the ~0.1° grid `Snap` |
| `open_meteo.go` | `OpenMeteoProvider` — real forecast (forecast endpoint) + climate-normal fallback (archive endpoint) beyond the horizon |
| `wmo.go` | WMO `weather_code` → five-bucket `Condition` mapping |
| `cached.go` | `CachedProvider` — per-place, in-process cache: 3h TTL, stale-while-revalidate with a 48h ceiling, per-place failure backoff, singleflight (the cost and latency lever) |
| `fake.go` | `FakeProvider` — deterministic weather for tests |

## How it fits together

```
CachedProvider(  ← per-place ~0.1° grid, 3h TTL, stale-while-revalidate; never per-user
  OpenMeteoProvider )  ← forecast ≤16d ∥ climate-normal proxy (parallel; IsTypical)
```

Expired entries (up to 48h old) are served immediately while a background
refresh runs; provider failures back off for 5 minutes per place. A request
only waits on the network for a place never fetched on the instance (#2646).

The caller (`server/services/portfolio`) resolves the viewer's place
(primary-residence location), asks for a date window, and formats the
user-facing temperature unit per locale — the provider works only in Celsius so
the per-place cache stays locale-neutral.

## When to add code here vs. elsewhere

Add a new `Provider` implementation here (e.g. a Google Weather provider for
production) behind the same interface. Keep proto/wire concerns out — this
package is transport-agnostic; the portfolio service converts `DayWeather` to
`api.DayForecast`. User-facing formatting (units, summary strings, l10n) lives in
the portfolio service, not here.

See [`docs/weather.md`](../../docs/weather.md) for the full design (providers,
the grid/cache, the horizon fallback, config, and the production upgrade path).
