# Parity Audit — Issue #2188

**Phase 0 of the Google Maps migration.** Two purposes:

1. **Quality calibration.** Confirm Google's POI dataset + geocoder is at
   least as good as Mapbox's on real Ripls-shaped queries, and surface
   any sharp regressions before we depend on it.
2. **Cost projection.** Apply Google's published 2024+ pricing to current
   Ripls request volume; produce a defensible monthly-spend number.

This is **not** a go/no-go gate — the migration is committed (see issue
#2188 author note). The audit's job is to surface surprises early
enough that we can adjust the rollout: budget alerts, request-volume
mitigations (caching, session tokens), or scope cuts.

## How to run

Prerequisites:

1. A Google Maps Platform API key with **Places API (New)** and
   **Geocoding API** enabled. See
   [`docs/issues/2188-google-maps-provisioning.md`](2188-google-maps-provisioning.md)
   for the full setup.
2. The existing Mapbox token in GSM as `mapbox-access-token`.

Run:

```bash
export GOOGLE_MAPS_API_KEY=$(gcloud secrets versions access latest \
  --secret=google-maps-api-key-server --project=ripls-dev)
export MAPBOX_ACCESS_TOKEN=$(gcloud secrets versions access latest \
  --secret=mapbox-access-token --project=ripls-dev)

npm run parity:2188
```

Output:

- `docs/issues/2188-parity-audit-results.json` — full per-query records
  (gitignored: don't commit a result file with vendor IDs).
- `docs/issues/2188-parity-audit-summary.md` — auto-generated markdown
  with presence-rate tables, latency percentiles, and a top-15 result
  preview for human judgment.

The 50-query starter sample takes ~30 seconds and costs well under $1.

## Quality assessment

The harness can answer two of the three quality dimensions automatically:

- **Result presence.** Did either provider return *any* result for the
  query? Mechanical comparison.
- **Latency.** Wall-clock per call. Mechanical comparison.

The third dimension requires human judgment:

- **Top-result relevance.** Did the first result match user intent? A
  query like "tea house pearl" could legitimately resolve to "Boulder
  Dushanbe Tea House" or to "Pearl Tea Lounge" or to "Pearl St" or
  nothing — the harness can't tell which the user wanted.

**The reviewer must read through the top-15 preview in the generated
summary** and mark each row as: Google better, Mapbox better, or
equivalent. Record the count in the table below. A representative sample
of 50 with proximate-equivalent or Google-better on >85% is a positive
signal; <70% is a yellow flag worth investigating.

### Results (audit run 2026-05-27)

**Source data:** `npm run parity:2188` against the 50-query starter
set, with both providers' dev keys. Raw results in
`docs/issues/2188-parity-audit-results.json` (gitignored); auto-
generated summary in `docs/issues/2188-parity-audit-summary.md`
(also gitignored). 50 queries finished in 3 seconds total.

| Dimension | Mapbox | Google | Delta |
|---|---|---|---|
| Search presence rate (n=38) | 82% (31/38) | 100% (38/38) | Google +18%, but see ⚠ below |
| Reverse-geocode presence rate (n=12) | 100% | 100% | tied |
| Search p50 latency | 181 ms | 96 ms (43 + 53) | Google ~2× faster |
| Search p95 latency | 300 ms | 211 ms (135 + 76) | Google ~30% faster |
| Reverse p50 latency | 181 ms | 113 ms | Google ~40% faster |
| Reverse p95 latency | 300 ms | 220 ms | Google ~30% faster |
| Top-result relevance (Google better) | — | 4 / 50 | — |
| Top-result relevance (Mapbox better) | 12 / 50 | — | — |
| Top-result relevance (equivalent) | 27 / 50 | 27 / 50 | — |
| Mapbox rate-limit errors during the run | 7 / 50 | n/a | ⚠ artifact |

⚠ **Mapbox's 82% presence rate is partly an artifact of rate limiting.**
The harness sent 50 sequential queries with concurrency=4. Mapbox
returned `Too Many Requests` on 7 queries between #11 and #26.
Excluding rate-limit failures, Mapbox's real presence rate is 31/31 =
100% on autocomplete. The rate-limit issue itself is real (production
will hit it under enough load) but it's not a quality signal.

### Notable per-query findings

**Google's proximity handling is looser than Mapbox's, and even
`locationRestriction` doesn't fully fix it.** This was the original
audit's top finding; the re-run with both bias modes refines the
picture.

`locationBias.circle` (default in the first run) failed on 4 / 19
proximity-aware queries:

| Query | Proximity | Google (bias) pick | Distance |
|---|---|---|---|
| `The Cup Boulder` | Boulder, CO | "The Coffee Cup Cafe", Boulder City, NV | 929 km |
| `tea house pearl` | Boulder, CO | "Super Bowl Asian Cuisine & Tea House", Pearland, TX | 1,440 km |
| `near me` | Boulder, CO | "NearMe Mobile Massage", Greeley, CO | 67 km |
| `park` | Boulder, CO | "Parker, CO" (a town) | 70 km |

Re-running with `locationRestriction.circle` at the same 50 km radius:

| Query | Google (bias) | Google (restriction) |
|---|---|---|
| `The Cup Boulder` | Nevada, 929 km | **no results** ✓ |
| `tea house pearl` | Pearland TX, 1,440 km | **no results** ✓ |
| `near me` | Greeley CO, 67 km | Greeley CO, **67 km** ✗ |
| `park` | Parker CO, 70 km | Parker CO, **70 km** ✗ |

**`locationRestriction` fixes the catastrophic cases but not the
marginal ones.** Google interprets `locationRestriction.circle.radius`
as a "search region" that includes localities whose *polygon overlaps*
the region — Parker (city) was kept because part of Parker's land area
reaches inside the 50 km circle, even though Parker's centroid (used
for the Place Details lat/lng we save) is 70 km out. Same with
"NearMe" (a Greeley business — Google's restriction isn't strict even
for non-locality results; the docs say "Region to search" but the
implementation is loose).

**Implication:** post-filter by computed distance for proximity-strict
flows. After Place Details resolves a placeId → coords, drop results
whose distance from the user's proximity exceeds some multiple of the
requested radius (e.g. 1.5× — keeps Parker if the user genuinely wants
it, drops a Nevada cafe). This catches the 2 marginal cases that
Google's own restriction misses.

**Also: Google's relevance ranking doesn't bias toward the nearest
result within the radius.** "Whole Foods" with Boulder proximity:
both modes returned the Denver Whole Foods (37 km, within the 50 km
radius). Mapbox returned the Boulder Whole Foods (1 km). Both Google
picks are valid, but Mapbox is "more local." For chain-store queries,
the harness can't fix this — it's a relevance-ranking difference, not
a parameter we can tune.

**Mapbox returned international garbage on famous global landmarks.**

| Query | Mapbox's pick | Correct |
|---|---|---|
| `Shibuya Crossing` | "Shibuya Xing", Bengaluru, India | Tokyo |
| `Plaza de Mayo` | "Plaza de Armas", Sucre, Bolivia | Buenos Aires |

Google nailed both. For Ripls' US user base today this is a minor
concern, but it's a real quality gap.

**Google returns Plus Codes for rural reverse-geocodes.**

For the two sparse-coverage reverse-geocode tests (rural Colorado
mountains, Alaska interior), Google returned only Plus Codes
(`H629+25 Keystone, CO`, `93PG6G24+8G`). Mapbox returned real street
addresses or locality names (`124 E Colorado Ave, Grant, CO`,
`Healy, Alaska 99743`). For our use case — saving reverse-geocoded
results as user-visible location names — Plus Codes are unusable.

**Reverse geocoding favors Mapbox 6:2 on substance.**

Across the 12 reverse queries, Mapbox more reliably returned a real
street address vs. Google's tendency to return a nearby business name
or Plus Code. Both providers handled urban centers (SF, Paris, Tokyo)
equivalently; the gap was in rural areas and at coordinates close to
landmarks but not at them (e.g. Big Ben coords → Mapbox: "6 Bridge
Street", Google: "81 London Bridge St" — wrong street).

**Latency: Google wins on every percentile**, but the Google
autocomplete-with-coords flow is two sequential calls. The user-facing
latency to pick a place from a Google autocomplete suggestion is
~100ms (autocomplete + details). The same operation via Mapbox is
~180ms (search returns coords directly, no second call).

## Cost projection

Google Maps Platform pricing as of late 2024 (verify against
`cloud.google.com/maps-platform/pricing` before relying on these
numbers; rates are billed monthly per API per project).

### SKUs we use

| API | SKU | Unit price (USD per 1,000) |
|---|---|---|
| Places API (New) — Autocomplete | Autocomplete (per session) | $2.83 |
| Places API (New) — Place Details (Basic) | Place Details Basic | $5.00 |
| Geocoding API | Geocoding | $5.00 |
| Maps SDK for Android | Maps SDK Android (per load) | $7.00 |
| Maps SDK for iOS | Maps SDK iOS (per load) | $7.00 |
| Maps JavaScript API | Maps Dynamic (per load) | $7.00 |

> Mapbox comparable (for the line we're replacing): Search Box API
> Forward Search $1.00/1k, Geocoding v6 $0.75/1k forward and reverse,
> static map loads vary. Mapbox is materially cheaper at the per-call
> level; Google's value is dataset quality.

> **The Maps SDK SKU is what dominates the bill at any non-trivial
> traffic.** A user opening Discover loads the map once → $0.007. At
> 10k DAUs each opening Discover twice a day, that's 600k loads/month
> = **$4,200/month** just for the map renders. The session-token model
> for autocomplete keeps that part cheap; the map loads are what to
> watch.

### Per-month projection — fill in from production logs

Pull current Ripls volume from Cloud Logging:

```bash
# Mapbox SearchPlaces calls in the last 30 days (server-side)
gcloud logging read \
  'resource.labels.service_name="server" jsonPayload.external_service="mapbox" jsonPayload.operation="SearchPlaces"' \
  --project=ripls-prod --freshness=30d --limit=200000 \
  --format="value(timestamp)" | wc -l

# Reverse-geocode calls
gcloud logging read \
  'resource.labels.service_name="server" jsonPayload.external_service="mapbox" jsonPayload.operation="ReverseGeocode"' \
  --project=ripls-prod --freshness=30d --limit=200000 \
  --format="value(timestamp)" | wc -l

# Forward-geocode calls
gcloud logging read \
  'resource.labels.service_name="server" jsonPayload.external_service="mapbox" jsonPayload.operation="ForwardGeocode"' \
  --project=ripls-prod --freshness=30d --limit=200000 \
  --format="value(timestamp)" | wc -l
```

Client-side autocomplete volume is harder to pull directly — there are
no server logs for client-only Mapbox calls. Estimate from:

- DAU × (probability of triggering an autocomplete) × (avg keystrokes
  past the 4-character threshold per session). Rough rule: a typical
  search session is 3–6 typeahead requests once the user starts typing.
- Pull DAU from Metabase
  ([`ripls-analytics`](../analytics) — pick the daily active users
  chart for the last 30 days).

Plug numbers in:

| Input | Value | Notes |
|---|---|---|
| Monthly server `SearchPlaces` calls | _TBD_ | Replaces 1 Google Autocomplete + 1 Place Details |
| Monthly server `ForwardGeocode` calls | _TBD_ | Replaces 1 Google Geocoding call |
| Monthly server `ReverseGeocode` calls | _TBD_ | Replaces 1 Google Geocoding call |
| Monthly client autocomplete sessions | _TBD_ | A "session" = sequence of keystrokes leading to one selection; with a session token, the Google session is billed at 1 × Autocomplete + 1 × Place Details |
| Monthly client reverse-geocodes (map-tap) | _TBD_ | Replaces 1 Google Geocoding call |
| Monthly mobile map loads | _TBD_ | Per-instance, every time a MapWidget mounts |
| Monthly web map loads | _TBD_ | Same |

Cost = (server `SearchPlaces`) × ($0.00283 + $0.00500)
     + (server `ForwardGeocode` + `ReverseGeocode`) × $0.00500
     + (client autocomplete sessions) × ($0.00283 + $0.00500)
     + (client reverse-geocodes) × $0.00500
     + (mobile + web map loads) × $0.00700

### Projection — fill in

| Line item | Volume / month | Unit | Cost / month |
|---|---|---|---|
| Server `SearchPlaces` | _TBD_ | $0.00783 | $_TBD_ |
| Server geocoding (forward + reverse) | _TBD_ | $0.00500 | $_TBD_ |
| Client autocomplete sessions | _TBD_ | $0.00783 | $_TBD_ |
| Client reverse-geocodes | _TBD_ | $0.00500 | $_TBD_ |
| Mobile map loads | _TBD_ | $0.00700 | $_TBD_ |
| Web map loads | _TBD_ | $0.00700 | $_TBD_ |
| **Total** | | | **$_TBD_** |

Compare against current Mapbox spend (check the Mapbox account billing
dashboard for the last 30 days). Net delta = Google projection −
Mapbox actual.

### Mitigations if the projection is high

If the projected cost is uncomfortably high, the most impactful levers
in priority order:

1. **Session tokens for autocomplete.** A Google "session token" wraps
   N autocomplete predictions + 1 Place Details call into a single
   billable session at the Place Details rate. Without a session
   token, each call is billed independently. **Always pass a session
   token from the client.** Implementation: the client generates a
   UUID per autocomplete session, sends it in `sessionToken` on the
   Autocomplete request, sends the same token on the Place Details
   request when the user picks a result.
2. **Server-side debouncing.** The current client triggers autocomplete
   at ≥4 characters. Move that threshold higher (5–6) or add a 250ms
   client-side debounce — both reduce per-session keystrokes by ~30%.
3. **Cache map style + tiles where possible.** Google's Maps SDK
   already does aggressive client-side tile caching. The map *load*
   counter increments per `GoogleMap()` widget instantiation. Avoid
   remounting the map widget on every screen rebuild (use
   `AutomaticKeepAliveClientMixin` or `Offstage` instead of disposing
   and recreating).
4. **Cache Place Details on the server.** Once we've resolved a
   placeId → coords, store the result in Postgres keyed by placeId.
   Subsequent saves of the same place skip the Place Details call.
5. **Reverse-geocode result caching.** Coordinates round to ~6 decimal
   places of precision in our storage; ratios of users tap-and-confirm
   the same location repeatedly. Cache reverse-geocodes server-side
   by rounded coordinate.

## Decision

**Recommendation: Proceed with mitigations.**

The migration is committed per #2188's author note, so this isn't a
go/no-go. But the audit surfaced three quality concerns that need
mitigation **before** flipping the provider switch in Phase 5, not
after:

### Required mitigations before cutover

1. **Use `locationRestriction` + post-filter by computed distance for
   proximity-strict flows.** The re-run audit showed that
   `locationRestriction.circle` alone isn't enough — Google still
   returned Parker (70 km) when the radius was 50 km, because Google
   treats restriction as "region containing overlapping localities"
   rather than a strict centroid-in-circle filter. Combine the API
   parameter with a server-side distance check on the resolved Place
   Details centroid; drop results > N× the requested radius. This is
   what's needed to make AI-extracted vague queries ("rec center")
   reliably resolve locally.
   - Implementation: in Phase 3's `GoogleLocationSearchService`,
     expose proximity as a typed parameter (`ProximityHint` vs
     `ProximityBound`). For `ProximityBound`, send
     `locationRestriction.circle` AND post-filter the resolved
     Place Details against the radius. For `ProximityHint`, send
     `locationBias.circle` and don't post-filter (lets famous-but-far
     answers through, like "Bondi Beach" from a Boulder context).
   - See Phase 3 §"Typed proximity semantics" in the migration plan
     (`2188-google-maps-migration.md`) for the per-call-site
     mapping.
2. **Filter Plus Codes out of reverse-geocode results.** Inspect the
   `types` array on each result; if the only result is a Plus Code
   (`plus_code` type with no `street_address` / `route` / `locality`
   in the same payload), fall back to a Mapbox call for that
   coordinate — or surface "address unavailable" to the user. Don't
   save a Plus Code as a user-visible location string.
3. **Prefer `street_address` / `route` over `establishment` results
   for reverse-geocode.** Google's default behavior picks the
   topmost result, which is often a nearby business. For our use
   case (saving the user's chosen location as an address) we want the
   address, not the business. The Geocoding API supports a
   `result_type` filter — use it.

### Recommended mitigations for cost

These weren't blocking quality findings — they're cost-management
levers. Apply them as part of Phase 3:

4. **Session tokens for autocomplete.** Without a session token, each
   autocomplete keystroke + the follow-up Place Details call is billed
   independently. With a session token, the whole session bills at the
   Place Details rate only (autocomplete becomes free within the
   session). Client-side: generate a UUID per autocomplete session,
   pass on every Autocomplete request, pass the same on the Place
   Details request when the user picks a result.
5. **Server-side Place Details cache.** Once a placeId resolves to
   coords + address components, store the result keyed by placeId in
   Postgres. Hits skip the Place Details call entirely (free).
6. **Server-side reverse-geocode cache.** Coordinates round to ~6
   decimals in our storage; map-tap users frequently re-tap the same
   spot. Cache reverse-geocode results keyed by rounded coordinate.

### Cost projection

Still needs production volume input — see the line items below. To
unblock: pull 30-day Mapbox call counts from Cloud Logging using the
queries in the "Per-month projection" section above, paste into the
table, multiply.

A rough estimate using order-of-magnitude DAU guess (1,000) and
typical engagement (one autocomplete session + one map-tap per active
session, 2 sessions/day per active user): ~60,000 autocomplete
sessions/month + ~60,000 reverse-geocodes/month. With session tokens
and the Place Details cache hitting ~70% of saved places:

- Autocomplete sessions: 60k × $0.00500 (Place Details rate, session
  token covers autocomplete) × 0.30 cache-miss = ~$90/month
- Reverse-geocodes: 60k × $0.00500 × 0.50 cache-miss = ~$150/month
- Server-side `SearchPlaces` (today's Mapbox volume from logs is
  unknown — fill in): _TBD_
- Mobile + web map loads: _TBD_ (this is the line item to watch — at
  10k DAU × 2 map opens/day = 600k loads/month = $4,200/month)

The map-load line item dominates. The audit didn't measure it (the
harness doesn't load maps), but it's the budget number to watch.
Phase 4's instrumentation should track per-instance MapWidget mount
counts so we can dial in real volume before flipping prod.

### What this means for sequencing

- Phase 3 (server `GoogleMapsClient`): implement with the mitigations
  baked in from the start — locationRestriction-vs-locationBias
  parameter, Plus Code filter, result_type filter, server-side
  Place Details cache.
- Phase 4 (client maps): instrument map-load counters in the first
  cut; revisit the cost projection with real numbers before any prod
  rollout.
- Phase 5 (cutover): keep Mapbox as the fallback for any flow where
  Google's quality regresses (the audit's `--map-provider` flag
  already supports this; use it as a kill switch).

### What this means for the harness

The harness used `locationBias` rather than `locationRestriction`,
which faithfully reproduces the bug a naive port would have. Fixing
the harness to use `locationRestriction` and re-running would likely
move the "Google ignored proximity" findings to zero — that's the
expected confirmation that mitigation #1 works. Leave the harness as
written; the data it produced is the right "before mitigations"
baseline.
