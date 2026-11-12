# Issue #1073: Upgrade dev and prod server configurations to be always-on

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1073
**Filed:** 2026-04-09 | **Last activity:** 2026-04-09
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Both the dev (`ripls-dev`) and prod (`ripls-prod`) Cloud Run environments are currently configured with `min_instances = 0` and `cpu_idle = true`, meaning servers scale to zero when there are no active requests and CPU is only allocated during request processing. This prevents two categories of functionality:

1. **Realtime / responsive use cases**: Cold starts when scaling from zero add multi-second latency to the first request after idle periods.
2. **Server-initiated actions**: Background goroutines (like the impact backfill job) only run while a request is active because `cpu_idle = true` deallocates CPU between requests.

Additionally, the issue requests increasing memory from 1Gi to 2Gi.

## Context from Comments

No comments on this issue.

## Current Configuration

| Setting | Dev | Prod |
|---|---|---|
| `min_instances` | 0 | 0 |
| `max_instances` | 2 | 5 |
| `cpu_idle` | true (hardcoded) | true (hardcoded) |
| `memory_limit` | 1Gi | 1Gi |
| `cpu_limit` | 1000m | 1000m |

Relevant files:
- `terraform/modules/containers/main.tf:169-175` -- `cpu_idle` hardcoded to `true`
- `terraform/environments/dev/main.tf:165-226` -- dev container module call
- `terraform/environments/prod/main.tf:159-218` -- prod container module call

## Recommendation

Direct changes to the Terraform files -- update `min_instances`, `memory_limit`, and `cpu_idle` with no new variables or abstractions.

### Cost Impact

With `min_instances = 1` and `cpu_idle = false`, you pay for 1 always-on vCPU + 2 GiB memory per environment, 24/7. Estimated increase: ~$56/month per environment, ~$112/month total.

## Open Questions

- **What "I/O bandwidth" improvements are desired?** -- Recommendation: Skip for now. The main I/O path is Cloud Run -> VPC Connector -> Cloud SQL, limited by network RTT and connection pool size, not CPU. If throughput is a concern, increasing `max_instances` or the Cloud SQL tier would have more impact.

## Implementation Plan

### Phase 1: Update Terraform configuration
**Goal:** Both environments become always-on with increased memory.
**Tasks:**
- [ ] In `terraform/modules/containers/main.tf`: change `cpu_idle = true` to `cpu_idle = false`
- [ ] In `terraform/environments/dev/main.tf`: set `min_instances = 1`, `memory_limit = "2Gi"`
- [ ] In `terraform/environments/prod/main.tf`: set `min_instances = 1`, `memory_limit = "2Gi"`

### Phase 2: Apply and verify
**Goal:** Changes are live in both environments.
**Tasks:**
- [ ] Apply to dev first: `terraform apply` in `terraform/environments/dev/`
- [ ] Verify dev server stays running with no traffic
- [ ] Apply to prod: `terraform apply` in `terraform/environments/prod/`
- [ ] Verify prod server stays running
- [ ] Monitor cost dashboard for expected increase

## Related Issues

None identified.
