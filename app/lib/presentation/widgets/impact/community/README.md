# Community Impact Widgets

Community-scoped impact display components.

## Purpose

One widget remains here: the chart card the workshop per-metric detail
screens render their series into. The rest of the community-metrics
surface this directory once served was retired along with those screens.

## Key Files

- **`impact_chart_card.dart`** — `ImpactChartCard`, a `CustomPainter`-based
  chart with two modes: `ChartType.line` for cumulative series (money,
  quality time, time) and `ChartType.bar` for monthly series (CO₂). Also
  defines `ChartDataPoint`, the `(label, value)` pair its callers build.
  Consumed by the four `screens/workshop/workshop_*_detail_screen.dart`
  screens.

## When to add here vs. elsewhere

Community-scoped impact widgets belong here. User-scoped or item-scoped
impact widgets live in the parent `widgets/impact/` directory.
