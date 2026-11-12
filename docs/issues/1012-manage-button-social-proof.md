# Issue #1012: Use manage button for conveying other people's interest

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1012
**Filed:** 2026-04-03 | **Last activity:** 2026-04-03
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The "Manage" button on experiences, help requests, and gear is a missed opportunity for social proof. Once other users engage with content (RSVP to an event, offer to help, express interest in gear), the button still shows a static "Manage" or "Managing" label. The issue asks the owner's action button to surface real-time engagement counts — attendees, offers, interested users — as the label, so owners immediately see at a glance that others are involved.

## Context from Comments

No comments. The issue description is concise and sufficient: it lists the three workflows and the desired label patterns.

## Current State

Gear (both giveaway and loan) already implements this:
- **Giveaway** (`buildGiveaway` in [workflow_action_buttons.dart](app/lib/presentation/widgets/workflow_action_buttons.dart:60)): shows `'$interestCount interested'` when `interestCount > 0`, otherwise `'Managing'`.
- **Loan** ([gear_content_view.dart](app/lib/presentation/screens/gear/gear_content_view.dart:536)): shows `gearPillInterestedCount(interestCount)` when `interestCount > 0`, otherwise `gearPillManaging`.

The two gaps are:

| Workflow | Current owner label | Desired owner label |
|---|---|---|
| Experience | `'Manage'` (hardcoded) | `'N attendee(s)'` when N > 0 (YES RSVPs only) |
| Request | `'Managing'` (hardcoded) | `'N helping'` when N > 0 (reuses `requestHelpingCount`) |

## Potential Solutions

### Option A: Add count parameters to `WorkflowActionButtons` factory methods (do not use)

Add `attendeeCount`/`helperCount` parameters and compute the label string _inside_ the factory, mirroring `buildGiveaway`'s existing `interestCount` handling.

- **Approach:** Two parameter additions; factory computes the plural label internally.
- **Pros:** All label logic in one place.
- **Cons:** `WorkflowActionButtons` has no `BuildContext`. Implementing pluralization inside the factory requires either inline Dart ternaries (`attendeeCount == 1 ? '1 attendee' : …`) or a `BuildContext` parameter. Inline English strings in Dart code are **i18n violations** — `npm run lint:dart:i18n` will fail. `buildGiveaway` already uses this pattern (`'$interestCount interested'`) and is a known pre-existing violation; do not repeat it.
- **Effort:** Small, but introduces a CI lint failure.

### Option B: Pass pre-formatted label string from the call site ✅ Recommended

Format the label at the call site (where `BuildContext` is available), then pass it as an optional `String? ownerPostLabel` parameter to each factory method. The factory falls back to its existing default when the parameter is null.

- **Approach:** Add `String? ownerPostLabel` to `buildExperience` and `buildRequest`. Call sites compute `yesCount > 0 ? context.l10n.experienceAttendeeCount(yesCount) : null` and pass the result. Add one new ARB plural key (`experienceAttendeeCount`); the request call site reuses the existing `requestHelpingCount` key.
- **Pros:** l10n strings are accessed where `context` exists; no lint violation. No `BuildContext` threading into the factory. Centralizes the "count > 0? show count : show default" branching without touching the internals of `MorphingActionButton`.
- **Cons:** Label logic is split: the count-formatting lives at the call site, the fallback default stays in the factory. Acceptable given the i18n constraint.
- **Effort:** Small.

### Option C: Store counts in the state models and derive the label in a ViewModel

Surface `attendeeCount`/`offerCount` as computed getters in the ViewModel, then consume them in the view.

- **Approach:** Extend `EventModalState` / `RequestState` with count properties.
- **Pros:** Counts become reusable across the screen.
- **Cons:** Over-engineered. Counts are trivially derivable at the call site; this adds boilerplate for no architectural gain.
- **Effort:** Medium.

## Recommendation

**Option B.** `WorkflowActionButtons` has no `BuildContext`, so computing labels inside the factory means hardcoded English strings — a CI lint violation per `docs/client/i18n.md` ("Strings belong in ARB files, not Dart code. The CI lint check `npm run lint:dart:i18n` catches violations"). Passing a pre-formatted `String?` from the call site is the correct pattern: l10n is resolved where context exists, and the factory stays context-free. The attendee and offer counts are a single expression at each call site (`yesCount` already computed at [experience_content_view.dart:763](app/lib/presentation/screens/experience/experience_content_view.dart#L763); `request.offerers.length` already used at [request_content_view.dart:616](app/lib/presentation/screens/request/request_content_view.dart#L616)). Only one new ARB key is needed (`experienceAttendeeCount`); the request call site reuses the existing `requestHelpingCount`.

## Decisions

- **Which RSVP intentions count as "attendees"?** YES RSVPs only. MAYBE is uncertain; this matches how `attendees.length` is used in the mark-completed flow.
- **Request label terminology?** Reuse `requestHelpingCount` ("1 helping / N helping"). No new ARB key needed for requests.
- **Threshold?** Start at 1.
- **Fix `buildGiveaway` inline string in this PR?** Yes — fix as housekeeping alongside the new changes.

## Implementation Plan

### Phase 1: l10n strings

**Goal:** Add the one new plural ARB key before any Dart changes. No new key is needed for requests — `requestHelpingCount` already exists.

**Tasks:**
- [ ] Add `experienceAttendeeCount` ICU plural key to [app_en.arb](app/lib/l10n/app_en.arb):
  `"{count, plural, =1{1 attendee} other{{count} attendees}}"` with `@experienceAttendeeCount` metadata and a `count` int placeholder.
- [ ] Add matching `experienceAttendeeCount` key to [app_es.arb](app/lib/l10n/app_es.arb).
- [ ] Run `npm run generate` to regenerate `AppLocalizations`.

**Validates:** `context.l10n.experienceAttendeeCount(n)` is available and correctly pluralized.

### Phase 2: `WorkflowActionButtons` API — add optional owner label override

**Goal:** Let call sites inject the pre-formatted count label without threading `BuildContext` into the factory.

**Tasks:**
- [ ] Add optional `String? ownerPostLabel` parameter to `WorkflowActionButtons.buildExperience` in [workflow_action_buttons.dart](app/lib/presentation/widgets/workflow_action_buttons.dart). When `isOwner`, use `ownerPostLabel ?? 'Manage'` as `postLabel`.
- [ ] Add optional `String? ownerPostLabel` parameter to `WorkflowActionButtons.buildRequest`. When `isOwner`, use `ownerPostLabel ?? 'Managing'` as `postLabel`.
- [ ] (Housekeeping) Fix the pre-existing i18n violation in `buildGiveaway`: replace the inline `'$interestCount interested'` Dart string with an `String? ownerPostLabel` parameter so callers can pass `context.l10n.gearPillInterestedCount(interestCount)`. Remove the `interestCount` int parameter from the factory signature.

**Validates:** Factory signatures accept optional owner labels; behavior is unchanged when `ownerPostLabel` is null.

### Phase 3: Wire up call sites

**Goal:** Pass the l10n-formatted count labels from each content view.

**Tasks:**
- [ ] In [experience_content_view.dart](app/lib/presentation/screens/experience/experience_content_view.dart) `_buildActionBar`: move `yesCount` computation above the `buildExperience` call (it's currently at line 763, below the call at line 726). Pass `ownerPostLabel: yesCount > 0 ? context.l10n.experienceAttendeeCount(yesCount) : null`.
- [ ] In [request_content_view.dart](app/lib/presentation/screens/request/request_content_view.dart) `_buildRequestActions`: pass `ownerPostLabel: request.offerers.isNotEmpty ? context.l10n.requestHelpingCount(request.offerers.length) : null` to `buildRequest`.
- [ ] In [gear_content_view.dart](app/lib/presentation/screens/gear/gear_content_view.dart) `_buildGiveawayOwner`: update `buildGiveaway` call site to pass `ownerPostLabel: giveawayInterestCount > 0 ? context.l10n.gearPillInterestedCount(giveawayInterestCount) : null` (companion to Phase 2 housekeeping fix).
- [ ] Run `npm run build` to verify lint and compilation pass.

**Validates:** All three workflow buttons show the count label in the owner's language; fall back to default when count is zero.

### Phase 4: Tests

**Goal:** Regression coverage for the new label behavior.

**Tasks:**
- [ ] Add unit test for `buildExperience` with non-null `ownerPostLabel` — assert `postActionLabel` equals the supplied string.
- [ ] Add unit test for `buildRequest` with non-null `ownerPostLabel` — assert `postActionLabel` equals the supplied string.
- [ ] Add widget test for [experience_content_view.dart](app/lib/presentation/screens/experience/experience_content_view.dart): mock state with one YES RSVP, verify the action button label contains "1 attendee".

**Validates:** Count-label logic is exercised and won't regress silently.

## Related Issues

- No directly related issues found in the open backlog.

---

## Audit Notes

_Added by `/ripls-audit` on 2026-04-03._

**i18n violation in original plan (fixed):**
The original Phase 1/2 tasks called for inline Dart ternary strings (`'$attendeeCount ${attendeeCount == 1 ? "attendee" : "attendees"}'`, `helperCount == 1 ? '1 offer' : '$helperCount offers'`) computed inside `WorkflowActionButtons`, and suggested an `interestCount` int parameter following `buildGiveaway`'s existing pattern.

Per `docs/client/i18n.md`: _"Strings belong in ARB files, not Dart code. The CI lint check (`npm run lint:dart:i18n`) catches violations."_ `WorkflowActionButtons` has no `BuildContext`, so it cannot call `context.l10n.*`. Implementing pluralization inside the factory would require hardcoded English strings — a lint failure.

**Resolution:** Plan rewritten to use Option B (pre-formatted label string passed from call site). New tasks:
1. Add `experienceAttendeeCount` ARB plural key (only new key needed; requests reuse `requestHelpingCount`).
2. Change factory API to `String? ownerPostLabel` (replaces int count parameters).
3. Call sites format the label with `context.l10n.*` and pass the result.

**Pre-existing violation flagged:** `buildGiveaway`'s `'$interestCount interested'` inline string is the same violation. A housekeeping fix is included in Phase 2 to bring it into compliance in the same PR.
