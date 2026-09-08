# Impact Metrics

The `impact_metrics` package is the core library for computing, formatting, and aggregating community and user impact metrics — money saved, carbon emissions prevented, and time saved. It provides the `Calculator` and `MetricDetailCalculator` types consumed by the impact metrics RPC service, as well as conversion helpers between API and storage representations.

## Key files

```
impact_metrics/
├── calculator.go              # Calculator: activity counts and aggregate impact per community
├── user_calculator.go         # Per-user impact aggregation
├── metric_detail_calculator.go # MetricDetailCalculator: breakdowns by dimension (time-series, rankings, comparisons)
├── metric_detail_series.go    # Time-series data for metric detail views
├── metric_detail_rankings.go  # Ranked member/gear lists for metric detail views
├── metric_detail_comparison.go # Peer comparison helpers
├── metric_detail_factors.go   # Methodology factor display
├── builder.go                 # Builder functions: BuildTransferImpact, BuildGearCumulativeImpact, etc.
├── calculator.go              # APIImpactToModels / modelsImpactToAPI conversion
├── format.go                  # Human-readable formatting helpers
├── insights.go                # High-level insight generation
├── provenance.go              # ShouldRefresh: staleness check for provenance-tracked fields
├── social_context.go          # ConnectionContextResolver: social relationship context for impact attribution
└── estimator/                 # Sub-package: per-item estimation config and formulas (see estimator/README.md)
```

## When to add code here vs. elsewhere

Calculation and aggregation logic lives here. RPC handler code lives in `server/services/impact_metrics`. Per-item estimation formulas (carbon, savings, time) and config live in `server/impact_metrics/estimator`. Background jobs that backfill impact estimates live in `server/jobs`.
