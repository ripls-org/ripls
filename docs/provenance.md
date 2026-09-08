---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Origin tracking for computed and AI-generated values — the Provenance record (source, version, reasoning), TrackedString/MaterialCategory/Estimate wrappers, version-based staleness via ShouldRefresh, and selective backfill across gear metadata and impact estimates.
  globs: [server/impact_metrics/**, server/services/gear/service.go]
  triggers: [provenance, tracked-types, staleness, should-refresh, backfill, methodology-doc, estimation-method]
  lens: [domain, server]
  domain: impact
freshness:
  verified_commit: "5c2d3e59c"
  verified_on: "2026-07-07"
---
# Metadata Provenance

## Overview

Every computed or AI-generated value in the system carries a `Provenance` record that answers three questions: **who produced it** (user, LLM, formula, config default), **which algorithm version** produced it, and **why** (reasoning and source citations). This enables principled decisions about when to overwrite a value on re-estimation, gives users transparency into how estimates were derived, and provides a foundation for selective backfill as algorithms improve.

Provenance is currently applied to two domains -- gear metadata fields and impact estimate dimensions -- but the same type and principles apply wherever computed values need origin tracking.

## Core Concepts

### The Provenance Record

A `Provenance` message (defined in `proto/ripls/models/common.proto` and mirrored in `proto/ripls/api/common.proto`) contains:

- **Source** (`ProvenanceSource` enum): How the value was produced. Four sources, from most to least authoritative: `USER` (manually entered or corrected), `LLM` (inferred by an LLM -- the input modality is carried in the `name` field, e.g. `genai_from_text`, `genai_from_image`, `genai_from_web`, rather than in separate enum values), `FORMULA` (deterministic computation from other data), and `CONFIG_DEFAULT` (a flat default from the estimator config).
- **Name** (string): A short identifier for the specific algorithm or estimation method, such as `"prevented_purchase"`, `"weight_material_carbon"`, or `"genai_from_image"`. This is the key used for version lookup and methodology doc resolution.
- **Version** (int32): Which revision of that algorithm produced the value. Bumping the version in config causes the staleness checker to flag records produced by older versions for re-estimation.
- **Confidence** (optional float, 0.0-1.0): LLM self-assessed confidence, present only when the source is an LLM type.
- **Reasoning** (optional string): Human-readable explanation of how the value was determined.
- **Sources** (repeated string): URLs, database names, or study citations used to produce the value.

### Tracked Types

Scalar fields that need provenance are wrapped in typed containers rather than adding provenance to the scalar type itself. Three wrapper types are defined in `proto/ripls/models/common.proto`:

- **TrackedString**: Pairs a `string value` with `Provenance`.
- **TrackedMaterialCategory**: Pairs a `MaterialCategory` enum value with `Provenance`.
- **TrackedEstimate**: Pairs an `Estimate` (mean + stddev) with `Provenance`.

Fields that are already message types (like `CarbonEstimate`, `ValueEstimate`, `MoneySavings`, `TimeSavings`) embed `Provenance` directly rather than wrapping.

The `Estimate` type itself remains bare -- it is a pure numerical container (mean + stddev) without provenance. Provenance belongs on the parent message that owns the estimate.

### Version-Based Staleness

Each named algorithm has a current version in the estimator config (`server/impact_metrics/estimator/config.textproto`, under the `provenance_versions` map). When a value's stored version is less than the config's current version for that name, it is considered stale and eligible for re-estimation.

The staleness check (`ShouldRefresh` in `server/impact_metrics/provenance.go`) follows four rules:

1. **No provenance**: Always refresh (legacy data predating provenance tracking).
2. **USER source**: Never refresh. User-entered values are sacrosanct regardless of version.
3. **Unknown name**: Never refresh. If the config has no version entry for a name, the system cannot determine staleness and conservatively leaves the value alone.
4. **Otherwise**: Refresh if `stored_version < config_version`.

This enables a controlled upgrade path: bump a version number in config, and a backfill job (see [Backfill Path](#backfill-path)) can selectively re-estimate only the affected fields while preserving user edits and unrelated algorithms.

## Application: Gear Metadata

Gear items have seven metadata fields that may originate from different sources:

| Field | Type | Typical Source |
|-------|------|---------------|
| `category` | TrackedString | LLM (GenGear) |
| `brand` | TrackedString | LLM (GenGear) |
| `model` | TrackedString | LLM (GenGear) |
| `material_category` | TrackedMaterialCategory | LLM (GenGear) |
| `weight_grams` | TrackedEstimate | LLM (GenGear) |
| `embodied_carbon` | CarbonEstimate (has Provenance) | Formula (from material + weight) |
| `value_estimate` | ValueEstimate (has Provenance) | LLM (GenGear) |

### Creation Path

When gear is created through the AI generation flow, the client passes a `generation_mode` on the `SaveGearRequest` indicating how the item was detected (text description, uploaded image, or product URL). The server maps this to the appropriate LLM provenance name (`genai_from_text`, `genai_from_image`, `genai_from_web`) and applies it to all metadata fields that don't already have provenance. See `server/services/gear/service.go` for the insert path helpers.

The AI detection response (`DetectedGearItem`) intentionally uses bare types without provenance -- provenance is assigned at SaveGear time, not at detection time. This is because the user previews and may edit the AI suggestions before saving.

### Update Path

When a user edits gear metadata, the server compares each field's new value against the stored value (via helpers like `trackedStringChanged` and `trackedEstimateChanged` in `server/services/gear/service.go`). Changed fields get `Provenance{Source: USER}`. Unchanged fields preserve their existing provenance. If material or weight changes, embodied carbon is recomputed with FORMULA provenance.

### Backfill Path

**Status:** No backfill job is currently wired at startup. The original `server/jobs/impact_backfill.go` (`ImpactBackfillJob`) drove this path -- using `ShouldRefresh` on each metadata field to decide whether to overwrite it with a fresh AI estimate, inferring the generation mode from gear attributes (presence of `source_url` implies web mode, `media_ids` implies image mode, otherwise text mode) -- but it was deleted as dead code (#2643): production logs showed the transaction half was always a no-op and the gear half was a stuck retry loop re-enriching the same few gear items with LLM calls on every boot. `ShouldRefresh` (`server/impact_metrics/provenance.go`) and the `provenance_versions` config remain in place as the staleness primitive for a future backfill job; recover the deleted implementation from git history if selective re-estimation is needed again. User-set fields would never be touched by such a job; only stale LLM-produced fields would be refreshed.

## Application: Impact Estimates

Impact estimates quantify the savings produced by sharing transactions (loans, giveaways, requests, experiences). Each transaction's `ImpactEstimate` has three dimensions, each carrying its own provenance:

| Dimension | Message Type | Typical Provenance |
|-----------|-------------|-------------------|
| Money saved | `MoneySavings` | FORMULA / `prevented_purchase` |
| Emissions prevented | `PreventedEmissions` | FORMULA / `weight_material_carbon` |
| Time saved | `TimeSavings` | CONFIG_DEFAULT / `gear_shopping_time` |

The estimator library (`server/impact_metrics/estimator/`) sets provenance on every value it produces. Builder functions in `server/impact_metrics/builder.go` compose estimator outputs into full `ImpactEstimate` messages and propagate provenance through scaling and merging operations. For example, when money saved is computed from a gear item's AI-generated value estimate, `mergeValueSources` copies the AI's reasoning and source citations into the savings provenance while keeping the formula's source type and version.

### Methodology Doc Resolution

Provenance replaces the previously scattered `methodology_doc` field that was stored on each impact dimension -- that field (and the `MethodologyDocs` estimator-config message) was removed in the provenance migration (see the field-history comments in `proto/ripls/models/impact_estimate.proto` and `proto/ripls/models/estimator_config.proto`). The provenance `name` is now the stable key for a value's methodology, and methodology drill-down is resolved **client-side** from that name rather than stored on every transaction record.

## Design Principles for Future Use

When adding provenance to new domains, follow these principles:

1. **Use the existing `Provenance` message.** Don't create domain-specific provenance types. The four `ProvenanceSource` values and the name/version/reasoning/sources fields are general enough for any computed value.

2. **Wrap scalars, embed on messages.** If the field is a scalar or enum, use the appropriate `Tracked*` wrapper. If it's already a message type, add a `Provenance` field directly.

3. **Keep `Estimate` bare.** Numerical estimates (mean + stddev) should never carry their own provenance. Provenance belongs on the parent message that gives the number its meaning.

4. **Provenance is set at the persistence boundary.** Don't set provenance at detection or computation time -- set it when the value is saved. This cleanly separates "what the AI suggested" from "what was actually persisted and why."

5. **USER source is immutable.** No automated process should ever overwrite a user-set value. This is the most important invariant in the system.

6. **Version bumps are the upgrade mechanism.** When an algorithm improves, bump its version in `config.textproto`. The staleness check and backfill infrastructure handle the rest. No ad-hoc migration scripts needed.

7. **Name is a stable key.** Provenance names (like `"prevented_purchase"` or `"genai_from_text"`) are used for version lookup, methodology doc resolution, and display name mapping. Choose them carefully and don't rename them -- add new names instead.

8. **Don't track the obvious.** Fields that are always user-entered (gear name, description, media uploads, location) don't need provenance. Only track fields where the origin is ambiguous or where automated re-estimation might occur.

## Related Documentation

- [docs/ai/provenance_plan.md](./ai/provenance_plan.md) -- Detailed implementation plan with phase-by-phase progress
- [docs/ai/impact_improvements_plan.md](./ai/impact_improvements_plan.md) -- Impact estimation system (Phases 1-10), which provenance builds on
- [docs/impact_metrics.md](./impact_metrics.md) -- Community impact metrics system overview

---

**Last Updated:** 2026-02-19
**Implementation Status:** Complete (Phases 1-6). Provenance tracked on all gear metadata fields and all impact estimate dimensions.
