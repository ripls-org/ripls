# Issue #1064: [Bug] RequestRepository.listMyRequests() stale after submitRequest due to glob mismatch

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1064
**Filed:** 2026-04-07 | **Last activity:** 2026-04-07
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The cache invalidation glob pattern `'my:*'` does not match bare keys like `'my'`. When `listMyRequests()` is called without a `communityId`, it caches under the key `'my'` (full key: `'request:my'`). After a mutation like `submitRequest`, the invalidation call `invalidatePattern('my:*')` converts to regex `^request:my:.*$`, which requires a colon after `my` — so the bare key is never cleared. Users see stale request lists for up to 30 minutes (the TTL).

The root cause is in `stash_cache_manager.dart:122-123`: the glob-to-regex conversion treats `*` as "one or more characters after the preceding text" rather than "zero or more segments including the prefix itself."

This same bug affects `transfer_repository.dart` — `_buildListKey('my', null, null)` produces bare `'my'`, and `invalidatePattern('my:*')` won't match it either. The same applies to `_buildListKey('received', null, null)`.

## Context from Comments

- **maintainer** asked the Claude GitHub bot to fix this and create a PR.
- **Claude bot** implemented Option 2 (fix glob semantics in `stash_cache_manager.dart`) by changing `.replaceAll('*', '.*')` to `.replaceAll(':*', '(:.*)?').replaceAll('*', '.*')`. This makes `:*` an optional suffix, so `my:*` matches both `my` and `my:communityId`.
- The bot could not push due to GitHub App permissions. The fix is in commit `99db79d` on branch `claude/issue-1064-20260407-2353` but never reached a PR.
- The bot's approach is sound but we should verify the fix locally and also address the transfer repository.

## Potential Solutions

### Option A: Fix glob semantics in stash_cache_manager.dart (recommended)

- **Approach:** Change the glob-to-regex conversion so `:*` becomes `(:.*)?` (optional suffix) before the remaining `*` becomes `.*`. This makes `'my:*'` match both `'request:my'` and `'request:my:communityA'`.
- **Pros:** Fixes the bug at the root — all repositories benefit. No per-repository code changes needed. The `:*` semantics become "this prefix and everything under it."
- **Cons:** Slightly broader blast radius — any other pattern using `:*` will now also match the bare prefix. This is almost certainly the desired behavior, but needs verification via existing tests.
- **Effort:** Small.

### Option B: Add explicit bare-key invalidation at each call site

- **Approach:** Add `await _cache.invalidate('my');` next to every `invalidatePattern('my:*')` call.
- **Pros:** Zero risk of unintended side effects in glob matching.
- **Cons:** Requires updating ~8 call sites in request_repository and ~6 in transfer_repository. Easy to miss one. Doesn't fix the conceptual bug — future repositories will hit the same trap.
- **Effort:** Small, but tedious and fragile.

### Option C: Always use suffixed cache keys (no bare keys)

- **Approach:** Change `listMyRequests()` to always produce a suffixed key like `'my:all'` instead of bare `'my'`. Same for `_buildListKey` in transfer_repository.
- **Pros:** Avoids the glob edge case entirely. Cache key structure becomes uniform.
- **Cons:** Invalidates existing cached data on deploy (one-time, harmless). Requires changes in request_repository and transfer_repository.
- **Effort:** Small.

## Recommendation

**Option A** — fix the glob semantics. This is the right fix because:

1. The glob contract (`'my:*'` = "everything starting with `my:`") naturally should include the prefix itself. A glob like `dir/*` in a filesystem matches files inside `dir/`, and our cache hierarchy is analogous — `my` is the "directory" containing `my:communityA`, `my:communityB`, etc.
2. It prevents the same bug from recurring in any future repository.
3. The Claude bot already implemented this exact fix and it's straightforward.
4. Existing tests in `stash_cache_manager_test.dart` can be extended to cover this case.

## Open Questions

- **Does any caller depend on `:*` NOT matching the bare prefix?** Recommendation: No. Grep of all `invalidatePattern` calls shows they all intend to clear "everything under this prefix." Matching the prefix itself is always correct. Verified by reviewing all repositories.
- **Does the fix affect patterns without a colon before `*`?** No. Some repositories use patterns like `'experience:$experienceId*'` (no colon before `*`). The `replaceAll(':*', ...)` step doesn't match these — the subsequent `replaceAll('*', '.*')` handles them identically to the current behavior.

## Implementation Plan

### Phase 1: Fix glob-to-regex conversion

**Goal:** Make `:*` patterns match both the bare prefix and suffixed keys.

**Tasks:**
- [ ] In `stash_cache_manager.dart:121-123`, change the regex conversion:
  ```dart
  // Before:
  final regexPattern = pattern.replaceAll('*', '.*');
  // After:
  final regexPattern = pattern.replaceAll(':*', '(:.*)?').replaceAll('*', '.*');
  ```
- [ ] Add regression test in `stash_cache_manager_test.dart`: put keys `'request:my'`, `'request:my:communityA'`, and `'request:other'`; clear with pattern `'request:my:*'`; assert both `request:my` keys are gone and `request:other` survives.
- [ ] Add a test for the `'transfer:my:*'` pattern with bare `'transfer:my'` key to confirm the fix generalizes.
- [ ] Run existing cache tests to confirm no regressions: `cd app && flutter test test/data/cache/ --reporter expanded`

**Validates:** The glob fix works correctly and doesn't break existing patterns.

### Phase 2: Verify repository behavior

**Goal:** Confirm the fix resolves the issue end-to-end for request and transfer repositories.

**Tasks:**
- [ ] Run request repository tests: `cd app && flutter test test/data/repositories/request_repository_test.dart --reporter expanded`
- [ ] Run transfer repository tests: `cd app && flutter test test/data/repositories/transfer_repository_test.dart --reporter expanded`
- [ ] Run full app test suite: `cd app && flutter test --reporter expanded`

**Validates:** No regressions across the app.

## Audit Notes

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`

- **Layer compliance:** Fix is in `data/cache/` (StashCacheManager), the correct layer per the caching architecture. No viewmodel or service changes needed.
- **Cache key convention:** The `'namespace:identifier'` convention (caching doc) is preserved. The fix makes `invalidatePattern` semantics match the documented intent of pattern-based clearing.
- **No server impact:** Purely client-side cache infrastructure bug. No proto, API, or server changes.
- **Testing:** Plan includes cache-manager-level regression tests and full test suite verification, consistent with the "Test cache invalidation behavior" guideline.
- **No i18n impact:** No user-visible strings affected.
- **Pattern edge cases verified:** Patterns without `:` before `*` (e.g., `experience:$id*`) are unaffected by the `replaceAll(':*', ...)` step, preserving existing behavior. Bare `*` patterns (e.g., `invalidatePattern('*')`) are also unaffected.

**No compliance issues found.**

## Related Issues

- #1057 — Impact screen "start your first request" nudge gating; cache invalidation verification in Phase 4 of that work is what discovered this bug.
