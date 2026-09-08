# itemkind

Shared utility for the #2012 item-consolidation work. Every response message
in the API that
surfaces a tappable thing — `BriefCTARow`, `DailyItem`, the workshop
and profile "Available Now" rails — embeds the shared `api.Item`
shape with its unified `api.ItemKind` discriminator. This package owns
the conversions that map the legacy per-surface enums and strings onto
`ItemKind`.

## Why this lives in its own package

Services should not depend on other services (per
[`docs/server/architecture.md`](../../docs/server/architecture.md)
§"Service Independence"). The aggregator that emits a workshop
postcard (in `server/services/workshop/`) and the aggregator that
emits a portfolio inbox row (in `server/services/portfolio/`) both
need the same mapping from a surface-specific discriminator to
`ItemKind`. Factoring the mapping into this shared library avoids
reaching across service boundaries.

The conversions are pure functions with no external dependencies, so
the package exposes plain functions rather than an interface.

## Discriminator table

| Legacy discriminator | Source surface | `ItemKind` |
|---|---|---|
| `DailyItemType.TRANSFER` | Portfolio inbox | `ITEM_KIND_TRANSFER` |
| `DailyItemType.EXPERIENCE` | Portfolio inbox | `ITEM_KIND_EXPERIENCE` |
| `DailyItemType.REQUEST` | Portfolio inbox | `ITEM_KIND_REQUEST` |
| `DailyItemType.GIVEAWAY` | Portfolio inbox | `ITEM_KIND_GIVEAWAY` |
| `DailyItemType.COMMUNITY` | Portfolio inbox | `ITEM_KIND_COMMUNITY` |
| `cta_action` string `"gear"` | Workshop / profile postcards | `ITEM_KIND_GEAR` |
| `cta_action` string `"experience"` | Workshop / profile postcards | `ITEM_KIND_EXPERIENCE` |
| `cta_action` string `"request"` | Workshop / profile postcards | `ITEM_KIND_REQUEST` |
| any other `cta_action` string | Workshop / profile postcards | not an entity — caller routes to non-entity action handling |

The Available Now rail historically used its own `AvailableNowKind`
enum; that enum was retired during the #2012 consolidation when
`AvailableNowItem` was renamed to the universal `Item`, so its
conversion no longer lives here.

## Files

- `kinds.go` — entry points: `FromDailyItemType`,
  `FromCTAActionString`, `ToScreenRouteName`.
- `kinds_test.go` — table-driven tests covering every legacy
  discriminator value.

## When to add code here

- A new legacy discriminator surfaces and needs mapping onto
  `ItemKind`. Add a conversion arm + a test case.
- A new `ItemKind` value lands. Update `ToScreenRouteName` to give it
  a tap-routing string (if applicable) and extend the tests.

## When NOT to add code here

- Surface-specific overlay field translation (e.g. composing the
  `mini_label` string for an Available Now tile). That belongs in the
  aggregator that emits the tile.
- Anything that needs storage, an RPC client, or a clock. Those keep
  this package impure and unreasonable to test as a pure function.
