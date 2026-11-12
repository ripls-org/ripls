# Issue #1113: Registration screen shows duplicate 'Sign Up' heading

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1113
**Filed:** 2026-04-13 | **Last activity:** 2026-04-13
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

The registration screen displays "Sign Up" twice — once in the app bar
(`AppBar(title: ...)` at line 674) and again as an inline heading via
`_buildHeader(context)` at line 685. Only the app bar title should be shown.

## Context from Comments

No comments on this issue.

## Potential Solutions

### Option A: Remove `_buildHeader` call
- **Approach:** Delete the `_buildHeader(context)` call at line 685 and the
  `SizedBox(height: 32)` spacer below it. Remove the `_buildHeader` method
  entirely if nothing else references it.
- **Pros:** Simplest fix. The app bar already shows the title.
- **Cons:** None — this is dead UI.
- **Effort:** Small (< 5 minutes).

### Option B: Replace header with contextual content
- **Approach:** Replace the duplicate heading with something useful, like a
  subtitle ("Create your account") or move the invitation status banner up.
- **Pros:** Uses the space for something helpful.
- **Cons:** Unnecessary scope creep for a simple bug fix.
- **Effort:** Small.

## Recommendation

**Option A.** Just remove the duplicate. The invitation status banner already
provides context below the app bar. No need to invent new content.

## Open Questions

None — this is clear-cut.

## Implementation Plan

### Phase 1: Remove duplicate heading
**Goal:** Fix the duplicate "Sign Up" display.
**Tasks:**
- [ ] Remove `_buildHeader(context)` call and its spacer at line 685-686
- [ ] Remove the `_buildHeader` method (lines 199-205) if unused elsewhere
- [ ] Verify on both code paths (with and without invite token)
**Validates:** "Sign Up" appears only once — in the app bar.

## Audit Notes

No architectural concerns. This is a presentation-layer-only change (removing
a duplicate widget call). No caching, i18n, server/client boundary, or
ViewModel implications.

## Related Issues

- #1099 — Deferred deep linking added the registration screen flow where
  this was noticed
