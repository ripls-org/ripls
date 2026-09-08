---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Emissions Prevented methodology — CO2e avoided from manufacturing avoided plus waste reduced, using embodied carbon (material+weight or spend-based EEIO), material emission factors, prevented-purchase rates, and per-transaction-type formulas.
  globs: [server/impact_metrics/estimator/**]
  triggers: [emissions-prevented, co2, carbon, embodied-carbon, emission-factor, waste-reduced, eeio, material-category]
  lens: [domain]
  domain: impact
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Emissions Prevented

## Definition

Kilograms of CO2-equivalent greenhouse gas emissions avoided when items are shared instead of purchased new. The metric has two components: manufacturing emissions avoided (no new item produced) and waste emissions avoided (item diverted from landfill).

## Formula

```
emissions_prevented = manufacture_avoided + waste_reduced

manufacture_avoided = embodied_carbon * prevented_purchase_rate
waste_reduced       = weight_kg * waste_co2e_per_kg * prevented_purchase_rate
```

Embodied carbon is computed at item creation time via one of two methods:

```
# Primary: material + weight lookup
embodied_carbon = weight_grams * material_emission_factor_kg_co2e_per_kg

# Fallback: spend-based EEIO
embodied_carbon = value_usd * spend_emission_factor_kg_co2e_per_usd * 1000
```

## Parameters

| Parameter | Value | Source | Config Key |
|-----------|-------|--------|------------|
| Loan prevented purchase rate | 0.50 | UK sharing library surveys | `loan_prevented_purchase_rate` |
| Giveaway prevented purchase rate | 0.50 | UK sharing library surveys | `giveaway_prevented_purchase_rate` |
| Waste CO2e per kg | 1.0 kg | Landfill diversion factor | `waste_co2e_per_kg` |
| Request default carbon | 5,000 g CO2e | Flat default | `request_default_carbon_grams` |
| Experience default carbon | 0 g CO2e | Disabled | `experience_default_carbon_grams` |

### Material Emission Factors (kg CO2e per kg)

| Material Category | Factor | Assumption |
|-------------------|--------|------------|
| SOLID_METAL | 3.25 | 95% steel, 5% aluminum average |
| SOLID_PLASTIC | 3.22 | 80% plastic, 20% rubber average |
| MIXED_PLASTIC_METAL | 3.23 | 50/50 plastic-metal blend |
| MIXED_WOOD_METAL | 1.87 | 50/50 wood-metal blend |
| MIXED_WOOD_PLASTIC | 1.86 | 50/50 wood-plastic blend |
| WOOD | 0.49 | Average softwood/hardwood |
| ALUMINUM | 7.63 | Trade mix |
| FABRIC | 7.96 | 50% cotton, 50% nylon average |
| CORDLESS_POWER_TOOL | 6.17 | 30% battery, 15% motor, 55% housing |
| CORDED_POWER_TOOL | 3.77 | 30% motor, 70% housing |
| PETROL_TOOL | 4.13 | 50% engine, 50% housing |
| ELECTRONICS_SMALL | 1.76 | Small device (WEEE) average |

### Spend-Based (EEIO) Emission Factors (kg CO2e per USD)

| Spend Category | Factor |
|----------------|--------|
| electronics_appliances | 0.15 |
| tools_hardware | 0.12 |
| sporting_goods | 0.10 |
| clothing_textiles | 0.12 |
| books_media | 0.05 |
| furniture | 0.08 |
| toys_games | 0.10 |
| general_consumer_goods (default) | 0.10 |

## By Transaction Type

| Transaction Type | Calculation | Config Defaults |
|------------------|-------------|-----------------|
| Loan | Embodied carbon from gear metadata + waste from weight, both scaled by loan rate | rate = 0.50 |
| Giveaway | Same as loan, scaled by giveaway rate | rate = 0.50 |
| Request | Flat default `request_default_carbon_grams` | 5,000 g CO2e |
| Experience | Flat default `experience_default_carbon_grams` | 0 g CO2e (disabled) |

## Uncertainty Model

| Source | Relative Stddev | Config Key |
|--------|----------------|------------|
| Weight + material lookup | 0.30 | `method_stddevs` → `weight_material_v1` |
| Spend-based EEIO | 0.50 | `method_stddevs` → `spend_based_v1` |
| Product LCA (future) | 0.15 | `method_stddevs` → `product_lca_v1` |
| Category average (future) | 0.40 | `method_stddevs` → `category_average_v1` |
| LLM inference | 0.50 | `method_stddevs` → `llm_inference_v1` |

## Methodology (User-Facing)

### Summary

When sharing prevents a new purchase, the manufacturing emissions from that product are avoided.

### How It Works

1. **Manufacturing emissions avoided**: AI analyzes item photos and descriptions to determine material composition and weight. These are matched against emission factor tables (12 material categories from the Edinburgh Tool Library and University of Bath ICE Database). When material data is unavailable, a spend-based fallback uses the item's dollar value and EEIO factors.
2. **Waste emissions avoided**: Items diverted from landfill or incineration avoid end-of-life emissions, calculated from item weight.
3. **Prevented purchase rate**: Not every loan prevents a new purchase. A 50% rate is applied based on UK sharing library survey data.

### Sources

- Edinburgh Tool Library (material emission factors)
- University of Bath ICE Database (embodied carbon reference data)
- Library of Things (prevented purchase methodology)
- Decarbon open-source dataset, CC BY-SA 4.0 (spend-based EEIO factors)

### Limitations

- Material categories are broad (12 categories covering all item types).
- Weight is often estimated by AI from photos, introducing additional uncertainty.
- Emission factors are global averages, not region-specific.
- Prevented purchase rate is a population-level average.
- Rebound effect (59-94% of savings re-spent on other consumption) is not subtracted.

## Implementation

| Component | File | Function |
|-----------|------|----------|
| Estimator | `server/impact_metrics/estimator/savings.go` | `EstimateEmissionsPrevented()` |
| Carbon lookups | `server/impact_metrics/estimator/carbon.go` | `LookupMaterialCarbon()`, `LookupSpendCarbon()` |
| Gear carbon | `server/impact_metrics/estimator/gear_carbon.go` | `EstimateGearCarbon()` |
| Config | `server/impact_metrics/estimator/config.textproto` | `material_emission_factors`, `spend_emission_factors` |
| Builder | `server/impact_metrics/builder.go` | `BuildTransferImpactMetrics()`, `BuildGearCumulativeImpactMetrics()` |

## Assumptions

1. **Prevented purchase rate**: 50% of sharing transactions prevent a new manufacture. Research support: Directional (UK sharing library surveys).
2. **Material category mapping**: AI-assigned material categories map accurately to emission factor tables. Research support: Conceptual.
3. **Weight estimation**: AI weight estimates from photos are within ~30% of actual. Research support: Conceptual.
4. **Waste diversion factor**: 1.0 kg CO2e per kg diverted from landfill. Research support: Directional (EPA WARM model ranges).
5. **No rebound effect**: Savings are gross, not net of re-spending. Research support: Direct (literature documents rebound but standard practice is to report gross).

## Research References

| Reference | Support Level | Used For |
|-----------|--------------|----------|
| Edinburgh Tool Library | Directional | Material emission factors for tool/equipment categories |
| University of Bath ICE Database | Direct | Embodied carbon reference data for common materials |
| Library of Things | Directional | Prevented purchase methodology and survey design |
| Decarbon (CC BY-SA 4.0) | Direct | Spend-based EEIO emission factors by category |
| EPA WARM model | Directional | Waste diversion emission factors |

## Per-Input Provenance and Overrides

For experiences and requests, prevented emissions are computed from
structured inputs: per-`PlanningContribution` `ContributionEmissionsInput`
records (with weight + material → embodied carbon and waste carbon),
optional `travel_avoided_carbon`, and `repair_credit_carbon`. Each input
carries its own `Provenance`: `source = LLM` for AI-inferred values,
`source = FORMULA` for table-derived ones (e.g., material → emission
factor), `source = CONFIG_DEFAULT` for fallbacks. Completion-modal
overrides flip the affected input's `Provenance.source` to `USER`;
siblings keep their original source.

The composite `manufacture_avoided_carbon` and `waste_reduced_carbon`
fields are always server-computed from the inputs and are never directly
user-settable. Refresh-gating (`ShouldRefresh()`) skips any input whose
source is `USER`, so user overrides survive algorithm-version bumps.

## Last Reviewed

2026-04-24.
