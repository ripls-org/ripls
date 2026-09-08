---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Money Saved methodology — dollars saved estimated as item replacement value times a prevented-purchase rate, with per-transaction-type formulas, a 30% relative stddev uncertainty model, and user-facing explanation copy.
  globs: [server/impact_metrics/estimator/**]
  triggers: [money-saved, cost-savings, prevented-purchase-rate, value-usd, replacement-value]
  lens: [domain]
  domain: impact
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Money Saved

## Definition

Dollars saved by community members who share items instead of purchasing new ones. Each sharing transaction avoids some fraction of a retail purchase, and the savings are estimated from the item's replacement value scaled by a prevented-purchase rate.

## Formula

```
money_saved = item_value_usd * prevented_purchase_rate
```

## Parameters

| Parameter | Value | Source | Config Key |
|-----------|-------|--------|------------|
| Loan prevented purchase rate | 0.50 (50%) | UK sharing library surveys | `loan_prevented_purchase_rate` |
| Giveaway prevented purchase rate | 0.50 (50%) | UK sharing library surveys | `giveaway_prevented_purchase_rate` |
| Money saved relative stddev | 0.30 (30%) | Modeling assumption | `money_saved_relative_stddev` |

## By Transaction Type

| Transaction Type | Calculation | Config Defaults |
|------------------|-------------|-----------------|
| Loan | `value_usd * loan_prevented_purchase_rate` | rate = 0.50, stddev = 0.30 |
| Giveaway | `value_usd * giveaway_prevented_purchase_rate` | rate = 0.50, stddev = 0.30 |
| Request | `value_usd * 1.0` (pass-through, value estimated at creation) | stddev = 0.30 |
| Experience | `value_usd * 1.0` (commercial equivalent value) | stddev = 0.30 |

## Uncertainty Model

| Source | Relative Stddev | Config Key |
|--------|----------------|------------|
| All money estimates | 0.30 | `money_saved_relative_stddev` |

Aggregation uses quadrature: `stddev_total = sqrt(sum(stddev_i^2))`.

## Methodology (User-Facing)

### Summary

When you borrow or receive an item instead of buying new, you save the cost of that purchase.

### How It Works

- For loans and giveaways, savings equal the item's estimated retail value scaled by a prevented-purchase rate (50% default). Not every loan prevents a purchase -- some borrowers would have gone without -- so the rate reflects the population-level average from sharing library surveys.
- For requests, the value represents labor or services provided.
- For experiences, the value represents the commercial equivalent of the shared activity.

### Sources

- Library of Things impact methodology (prevented purchase surveying)
- Benthyg Cymru research comparing 6 UK sharing libraries

### Limitations

- Item values are AI estimates and may not reflect actual market prices.
- Prevented purchase rate is a population-level average; individual behavior varies.
- Does not account for maintenance or transaction costs.
- Uses US market pricing; values may differ in other regions.

## Implementation

| Component | File | Function |
|-----------|------|----------|
| Estimator | `server/impact_metrics/estimator/savings.go` | `EstimateMoneySaved()` |
| Config | `server/impact_metrics/estimator/config.textproto` | `loan_prevented_purchase_rate`, `giveaway_prevented_purchase_rate` |
| Builder | `server/impact_metrics/builder.go` | `BuildTransferImpactMetrics()`, `BuildRequestImpactMetrics()`, `BuildExperienceImpactMetrics()` |

## Assumptions

1. **Prevented purchase rate**: 50% of sharing transactions prevent a new purchase. Research support: Directional (UK sharing library surveys).
2. **AI value accuracy**: Item values from AI photo/text analysis approximate retail replacement cost. Research support: Conceptual.
3. **Uniform rate across items**: The same prevented purchase rate applies to all item types. Research support: Conceptual (surveys report aggregate rates).

## Research References

| Reference | Support Level | Used For |
|-----------|--------------|----------|
| Library of Things impact methodology | Directional | Prevented purchase rate methodology |
| Benthyg Cymru (6 UK sharing libraries) | Directional | Prevented purchase rate calibration |

## Per-Input Provenance and Overrides

For experiences and requests, money savings is computed from structured
inputs: a `hire_equivalent_value` plus per-`PlanningContribution`
`ContributionValueInput` records. Each input carries its own `Provenance`:
`source = LLM` for AI-inferred values, `source = FORMULA` for derived ones,
`source = CONFIG_DEFAULT` for fallbacks. Completion-modal overrides flip
the affected input's `Provenance.source` to `USER`; siblings keep their
original source.

The composite `value_usd` is always server-computed from the inputs by the
estimator and is never directly user-settable. Refresh-gating
(`ShouldRefresh()`) skips any input whose source is `USER`, so user
overrides survive algorithm-version bumps. Contribution-aware estimation
applies a co-use discount (~30–40%) so each shared item contributes a
fraction of its commercial value rather than the full retail price.

## Last Reviewed

2026-04-24.
