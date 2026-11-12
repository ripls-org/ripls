# Issue #929: Dev server not generating videos

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/929
**Filed:** 2026-03-27 | **Last activity:** 2026-03-28
**Labels:** bug, user-feedback | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

Experiences on the dev server (and sometimes prod) fail to find stock videos.
Local development works fine. Server logs show HTTP 429 (Too Many Requests)
responses from the Pexels API — both in health checks and in actual
`GetStockVideo` calls. The Pexels free tier allows **200 requests/hour**, and
this quota is being exhausted.

The issue is NOT a missing API key — 429 means the key is valid but rate-limited.
The Terraform configuration correctly retrieves the Pexels API key from GCP
Secret Manager for both dev and prod environments.

## Context from Comments

- **maintainer** confirmed 429 errors in both `ripls-dev` and `ripls-prod`
  Cloud Run logs. Health checks fail with rate limit errors. Actual
  `GetStockVideo` calls also fail with "pexels video search failed: pexels
  video API: pexels API rate limited."
- Local servers work fine — likely because local usage is single-user with
  far fewer requests.
- User raised the possibility that Pexels isn't configured in Terraform —
  but logs confirm the key is present (429 is an authenticated rate-limit
  response, not a 401/403).

## Root Cause Analysis

Multiple factors compound to exhaust the 200 req/hour Pexels quota:

### 1. Health checks consume quota unnecessarily
Each `/health` endpoint call triggers **two** Pexels API calls:
- `stock_imagery` health check → `PexelsProvider.CheckHealth()` (1 API call)
- `stock_video` health check → `PexelsProvider.CheckHealth()` (1 API call)

Each health check uses a unique timestamp-based query
(`healthcheck{UnixNano}`) specifically to bypass CDN caching and force API
validation. GCP uptime monitoring calls `/health` every 5 minutes from the
USA region = **24 Pexels API calls/hour** just from health checks.

### 2. No client-side rate limiting
The `PexelsClient` has no token bucket, sliding window, or any pre-emptive
rate limiting. It fires requests eagerly until Pexels returns 429.

### 3. Shared quota across image and video
Pexels photos and videos share the same 200 req/hour quota. Both
`GetStockImage` (for requests, gear, communities) and `GetStockVideo`
(for experiences) draw from the same pool.

### 4. Cache miss rate on video
The semantic video cache requires similarity ≥ 0.85 for a hit. Experience
queries are often unique enough to miss, leading to live API calls. The
fallback threshold (0.65) helps when rate-limited, but by then the quota is
already gone.

## Potential Solutions

### Option A: Health check caching + client-side rate limiter

- **Approach:** (1) Cache health check results for 5 minutes so repeated
  `/health` calls don't make live Pexels API calls. (2) Add a token bucket
  rate limiter to `PexelsClient` that pre-emptively limits requests to stay
  under 200/hour. (3) Share one health check between `stock_imagery` and
  `stock_video` since they use the same Pexels provider.
- **Pros:** Directly addresses root cause. Simple to implement. No external
  dependencies or cost.
- **Cons:** Still limited to 200/hour total. May not scale if usage grows.
- **Effort:** Medium.

### Option B: Upgrade to Pexels paid tier or add API keys

- **Approach:** Either upgrade the Pexels account for higher rate limits, or
  use separate API keys for dev vs prod (if currently shared). If they're
  already separate keys, get additional keys to increase total quota.
- **Pros:** Simplest fix if the problem is just quota.
- **Cons:** Monthly cost. Doesn't fix the wasteful health check pattern.
  Doesn't scale indefinitely.
- **Effort:** Small.

### Option C: Option A + smarter fallback behavior

- **Approach:** Everything in Option A, plus: (1) Lower the video cache
  fallback threshold from 0.65 to 0.5 so more cached videos are reused when
  rate-limited. (2) Implement a circuit breaker that stops Pexels calls
  entirely for N minutes after a 429, falling back to Pixabay or cached
  content. (3) Skip health check for Pexels entirely — rely on operational
  errors to detect problems.
- **Pros:** Most resilient. Gracefully degrades instead of failing.
- **Cons:** More complex. Lower fallback threshold may return less relevant
  videos.
- **Effort:** Medium-large.

## Recommendation

**Option A** — health check caching + client-side rate limiter. This directly
addresses the two biggest quota drains (health checks and unthrottled
requests) without adding cost or complexity. The health check deduplication
alone saves 24 req/hour (12% of quota), and a token bucket ensures the
remaining quota is distributed across actual user requests.

If quota remains tight after Option A, upgrading the Pexels plan (Option B)
is the simplest escalation.

## Open Questions

- **Do dev and prod use the same Pexels API key?** — Recommendation: Check
  the actual secret values in GCP Secret Manager for `ripls-dev` and
  `ripls-prod`. If they share one key, the combined health checks (48/hour)
  and user traffic from both environments compete for 200 req/hour. Using
  separate keys per environment is a quick win.
- **Is the Pexels account on the free tier?** — Recommendation: Verify the
  account tier. If the project has budget for it, upgrading to a paid tier
  would complement the technical fixes.
- **Should we skip Pexels health checks entirely?** — Recommendation: No.
  Health checks are valuable for alerting. Instead, cache the result and
  deduplicate the check across stock_imagery and stock_video.

## Implementation Plan

### Phase 1: Deduplicate and cache health checks
**Goal:** Eliminate wasteful Pexels API calls from health checks.
**Tasks:**
- [ ] Add a `lastHealthStatus` cache to `PexelsProvider` that stores the last
      health check result and timestamp. Return cached result if less than 5
      minutes old.
- [ ] In `main.go`, register Pexels health check only once (under
      `stock_imagery`). Remove the separate `stock_video` health check
      registration, or have it delegate to the cached result.
- [ ] Add tests for cached health check behavior (fresh call, cached return,
      cache expiry).
**Validates:** Health checks no longer trigger Pexels API calls on every
invocation. Confirms the caching mechanism works correctly.

### Phase 2: Add client-side rate limiter
**Goal:** Pre-emptively throttle Pexels API calls to stay under quota.
**Tasks:**
- [ ] Add a token bucket rate limiter to `PexelsClient` (use
      `golang.org/x/time/rate`). Configure with ~180 tokens/hour (leaving
      headroom below the 200 limit).
- [ ] When the rate limiter denies a request, return `ErrRateLimited`
      immediately without making the HTTP call. This lets the existing
      fallback chain (cache → Pixabay) handle it.
- [ ] Add configuration for the rate limit (flag or constant) so it can be
      tuned.
- [ ] Add tests verifying that the rate limiter returns `ErrRateLimited`
      when quota is exhausted.
**Validates:** Pexels API calls are capped below the 200/hour limit. The
fallback chain activates smoothly when the limiter kicks in.

### Phase 3: Verify and tune
**Goal:** Confirm the fix works in the dev environment.
**Tasks:**
- [ ] Deploy to dev and monitor Pexels 429 errors in Cloud Run logs.
- [ ] Verify that experiences successfully find videos (from cache or
      Pixabay fallback when Pexels is throttled).
- [ ] Check if dev and prod use separate API keys; if not, create separate
      secrets.
- [ ] Lower the video cache fallback similarity threshold from 0.65 to 0.55
      if too many requests still fail (optional tuning).
**Validates:** No more 429 errors in steady-state operation. Experiences
consistently get video content.

## Related Issues
- #811 — Video caching (related infrastructure for stock video caching)
- #847 — Video thumbnails (related video handling)
- #921 — Thumbnail not showing (related media display issues)
