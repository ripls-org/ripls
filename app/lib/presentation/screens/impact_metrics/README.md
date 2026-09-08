# Impact Metrics Screens

Drill-down screens that explain how an impact number was arrived at,
across money saved, time recovered, and CO₂ prevented.

## Purpose

This directory holds the "show your work" surface behind a metric — not
the metric displays themselves. Both screens here are pushed from a
metric surface elsewhere (`screens/item/item_metrics_screen.dart`,
`widgets/content/content_metric_sheet.dart`) rather than being reached
from a tab.

## Key Files

- **`metric_drill_down_screen.dart`** — formula-based breakdown of a
  metric: a formula banner with real values, a recursive component tree,
  and numbered research citations.
- **`audit_trail_screen.dart`** — per-attribute provenance breakdown for
  an `ImpactEstimate`, badging each contributing attribute as USER / AI /
  CALCULATED / DEFAULT so the calculation is transparent.

## When to add here vs. elsewhere

A screen belongs here when its job is explaining a metric's derivation.
Widgets that render metric cards, charts, and hero sections live in
`widgets/impact/`; community-scoped variants live in
`widgets/impact/community/`. Screens that *display* a metric in a feature
context (item, content, community) belong with that feature.
