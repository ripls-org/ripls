# Impact Estimator

The `estimator` sub-package contains the per-item impact estimation formulas and configuration. It computes money saved, carbon emissions prevented, and time saved for individual gear loans, giveaways, and experiences using a combination of material emission-factor tables, LLM confidence interpolation, and configurable defaults.

## Key files

- `config.go` — `Config` wrapper around the `EstimatorConfig` proto; provides `LoadConfig` (from file) and `LoadConfigFromEmbed` (embedded `config.textproto`).
- `config.textproto` — embedded configuration: material emission factors, provenance version strings, methodology doc filenames, and default savings parameters.
- `savings.go` — money-saved estimation from gear value and category.
- `carbon.go`, `gear_carbon.go` — embodied carbon estimation from material category and weight.
- `time.go` — time-saved estimation from category and loan duration.
- `social.go` — social-footprint estimation (connection context).
- `uncertainty.go` — quadrature uncertainty propagation for `Estimate` fields.

## When to add code here vs. elsewhere

New estimation formulas and their parameters belong here. Builder functions that compose multiple estimation calls into a full `ImpactEstimate` live in `server/impact_metrics/builder.go`. Configuration values go in `config.textproto`, not hardcoded in Go. RPC handlers that expose impact data live in `server/services/impact_metrics`.
