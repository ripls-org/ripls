# Issue #988: Loan owner "arrange pickup" modal erroneously asks "How long do you need it?"

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/988
**Filed:** 2026-04-01 | **Last activity:** 2026-04-02
**Labels:** none | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

The "Arrange Pickup" modal uses the same borrower-perspective label for the
duration question regardless of who is viewing it. When the gear owner opens
the modal, they see "How long do you need it?" — which makes no sense from
their perspective. It should say something like "How long can they borrow it?"

The modal already accepts an `isOwner` parameter and uses it to customize the
notification preview text (`_notificationDescription()`), but the duration
label at line 289 is hardcoded to the borrower perspective.

## Context from Comments

No comments on the issue. The problem and fix direction are clear from the
description.

## Root Cause

In `app/lib/presentation/screens/gear/arrange_pickup_modal.dart` line 289,
the `DurationChips` label is a hardcoded string:

```dart
DurationChips(
  label: 'How long do you need it?',  // always borrower perspective
  ...
),
```

The `widget.isOwner` flag is available but not consulted here. The same modal
already branches on `isOwner` for the notification description (lines
389-394), so the pattern exists — it just wasn't applied to this label.

Additionally, this string is not localized (no l10n key), violating the i18n
convention.

## Potential Solutions

### Option A: Inline role branch + add l10n keys

- **Approach:** Add two l10n keys (`loanDurationQuestionBorrower` /
  `loanDurationQuestionOwner`) and select based on `widget.isOwner` at the
  call site.
- **Pros:** Simple, follows existing `isOwner` branching pattern already in
  this file. Fixes the i18n gap at the same time.
- **Cons:** None.
- **Effort:** Small.

### Option B: Pass role into DurationChips

- **Approach:** Add an `isOwner` flag to `DurationChips` and let it choose
  its own label internally.
- **Pros:** Self-contained widget.
- **Cons:** `DurationChips` is a generic widget — injecting role awareness
  couples it to the loan domain. The label is already a parameter, so
  choosing it at the call site is the cleaner pattern.
- **Effort:** Small, but adds unnecessary coupling.

## Recommendation

**Option A.** The label is already passed as a parameter to `DurationChips`,
so the call site should choose the right label based on role. Adding l10n
keys also fixes the hardcoded-string violation.

## Open Questions

- **What exact text should the owner see?** — Recommendation:
  "How long can they borrow it?" This matches the third-person perspective
  used elsewhere in the modal (e.g., notification preview).

## Implementation Plan

### Phase 1: Fix duration label and localize

**Goal:** Owner sees owner-appropriate text; strings are localized.

**Tasks:**
- [ ] Add l10n keys to `app_en.arb` and `app_es.arb`:
  - `loanDurationQuestionBorrower`: "How long do you need it?"
  - `loanDurationQuestionOwner`: "How long can they borrow it?"
- [ ] In `arrange_pickup_modal.dart` line 289, replace hardcoded string with:
  ```dart
  label: widget.isOwner
      ? context.l10n.loanDurationQuestionOwner
      : context.l10n.loanDurationQuestionBorrower,
  ```
- [ ] Run `npm run generate` to regenerate l10n code

**Validates:** Owner opens arrange pickup modal and sees "How long can they
borrow it?" instead of "How long do you need it?"

## Related Issues

None referenced.

---

## Audit

**Audited:** 2026-04-02

**No issues found.** The plan:
- Correctly keeps role logic in the screen (call site), not the generic `DurationChips` widget (architecture.md: widgets don't contain business logic)
- Uses `context.l10n` for new strings with proper `scopeContextDescription` naming (i18n.md)
- Requires both ARB files with `@key` metadata (i18n.md)
- No caching or server/client boundary concerns — this is a UI-only text change

**Note:** The modal has ~10 other hardcoded strings (line 145 "Arrange Pickup",
line 282 "Pickup Time", etc.) that also lack l10n keys. These are out of scope
for this issue but worth a follow-up i18n sweep.
