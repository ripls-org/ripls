---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Time Saved methodology — minutes saved by sharing instead of shopping/researching/solo labor, computed from per-transaction-type default minutes (or an AI duration hint) times a multiplier, with config vs AI-hint uncertainty tiers.
  globs: [server/impact_metrics/estimator/**]
  triggers: [time-saved, time-recovered, shopping-time, labor-time, duration-hint, ai-hint]
  lens: [domain]
  domain: impact
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Time Saved

## Definition

Minutes saved by community members through sharing instead of shopping, researching, or performing tasks alone. Time savings represent the hours of effort avoided when items, skills, and experiences are shared within a community.

## Formula

```
time_saved = default_minutes * multiplier
```

Where `multiplier` is the attendee count for experiences (each person saves time), and 1 for all other transaction types. When an AI-inferred duration hint is available, it replaces `default_minutes`.

## Parameters

| Parameter | Value | Source | Config Key |
|-----------|-------|--------|------------|
| Gear shopping time | 120 min | Consumer research averages | `time_defaults.gear_shopping_time_minutes` |
| Request labor time | 120 min | Task labor averages | `time_defaults.request_labor_time_minutes` |
| Experience duration | 120 min | Activity duration averages | `time_defaults.experience_duration_minutes` |
| Default relative stddev | 0.50 (50%) | Population-level average | `time_defaults.default_relative_stddev` |
| AI hint relative stddev | 0.30 (30%) | Context-specific estimate | `time_defaults.hint_relative_stddev` |

## By Transaction Type

| Transaction Type | Calculation | Config Defaults |
|------------------|-------------|-----------------|
| Loan / Giveaway | `gear_shopping_time_minutes` (fixed) | 120 min, stddev = 0.50 |
| Request | AI hint if available, else `request_labor_time_minutes` | 120 min, stddev = 0.50 (config) or 0.30 (AI) |
| Experience | AI hint if available, else `experience_duration_minutes`; scaled by attendee count | 120 min, stddev = 0.50 (config) or 0.30 (AI) |

## Uncertainty Model

| Source | Relative Stddev | Config Key |
|--------|----------------|------------|
| Config default (all types) | 0.50 | `time_defaults.default_relative_stddev` |
| AI-inferred hint | 0.30 | `time_defaults.hint_relative_stddev` |

The higher default stddev reflects that config defaults are population-level averages. AI-inferred hints from item/task descriptions are more specific, warranting lower uncertainty.

## Methodology (User-Facing)

### Summary

Sharing saves time by avoiding shopping trips, research, and solo task labor. Each transaction type has a default time savings reflecting the effort that would otherwise be spent.

### How It Works

- **Gear loans/giveaways**: Borrowing avoids the research and shopping time to find and purchase a similar item (default: 2 hours).
- **Requests**: Getting help with a task saves the labor time of doing it alone (default: 2 hours). AI can provide more specific estimates based on the request description.
- **Experiences**: Shared activities save individual preparation and execution time, scaled by the number of attendees (default: 2 hours per person). AI can estimate duration from the event description.

### Sources

- Consumer research on average shopping times for durable goods
- AI analysis of item/task descriptions when available

### Limitations

- Defaults are broad averages; actual shopping time varies widely by item type.
- Time savings are harder to verify empirically than monetary or carbon savings.
- Experience scaling assumes each attendee would have spent equivalent time individually.

## Implementation

| Component | File | Function |
|-----------|------|----------|
| Gear time | `server/impact_metrics/estimator/time.go` | `EstimateGearTime()` |
| Request time | `server/impact_metrics/estimator/time.go` | `EstimateRequestTime()` |
| Experience time | `server/impact_metrics/estimator/time.go` | `EstimateExperienceTime()` |
| Config | `server/impact_metrics/estimator/config.textproto` | `time_defaults` section |
| Builder | `server/impact_metrics/builder.go` | `BuildTransferImpactMetrics()`, `BuildRequestImpactMetrics()`, `BuildExperienceImpactMetrics()` |
| Scaling | `server/impact_metrics/estimator/savings.go` | `ScaleTimeSaved()` (scales by multiplier) |

## Assumptions

1. **Default durations**: 2 hours is a reasonable average for shopping, labor, and activity time. Research support: Directional (consumer survey averages).
2. **Linear attendee scaling**: Each experience attendee saves the full default duration. Research support: Conceptual.
3. **AI hint accuracy**: When AI infers duration from descriptions, it is more accurate than the flat default. Research support: Conceptual.

## Research References

| Reference | Support Level | Used For |
|-----------|--------------|----------|
| Consumer shopping time research | Directional | Default gear shopping time (120 min) |
| Task labor surveys | Directional | Default request labor time (120 min) |

## Last Reviewed

2026-03-06.
