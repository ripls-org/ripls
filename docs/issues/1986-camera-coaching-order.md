# Issue #1986: Reorder the camera suggestions

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1986
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Audit Notes

Audited 2026-05-16 against `docs/client/{architecture,caching,i18n,
accessibility,testing}.md` and `docs/server/{architecture,observability}.md`.
Server/caching/observability sections don't apply — this is a pure
client-side UI reorder. Compliance findings and the resulting plan
adjustments:

- **Accessibility:** No new primitives or labels needed. The widget already
  composes `Tappable`, `LiveRegion`, and `accessibleDuration`. Semantic
  labels and asset paths are switched on enum *values*, not order, so a
  reorder doesn't break the mapping. The accessibility allowlist is
  unaffected.
- **i18n:** All four nudges and a11y labels already exist in both
  `app_en.arb` and `app_es.arb` (`unifiedCreateCameraCoaching{Gear,Clothes,Food,Flyer}`
  + `a11yUnifiedCreateCoaching*`). No new keys, no new translations.
  Added a one-line locale-verification task to Phase 1.
- **Testing:** Per `docs/client/testing.md`, business logic belongs in
  viewmodels and is the primary test target. This widget has no
  viewmodel — its only state is `_currentIndex` and `_userLocked`, which
  are presentation-layer cycling concerns. The existing widget-test
  layer (`app/test/presentation/widgets/creation/`) is the right home
  for the reorder coverage. The plan already targets it correctly.
- **Stale-doc risk caught during audit:** The class docstring at
  [camera_coaching_carousel.dart:13](app/lib/presentation/widgets/creation/camera_coaching_carousel.dart#L13)
  hard-codes the current order ("gear, clothes, food, event flyer"). A
  reorder leaves this stale. Phase 1 now includes updating the docstring.
- **Comment hygiene per CLAUDE.md:** The inline test comment
  `// Tap the food thumbnail (index 2)` at
  [camera_coaching_carousel_test.dart:72](app/test/presentation/widgets/creation/camera_coaching_carousel_test.dart#L72)
  is the kind of trivial what-comment CLAUDE.md says to omit. Phase 1
  now says to remove the parenthetical (not rewrite it) so the comment
  becomes order-independent.
- **Cosmetic test cleanup:** The `thumbs` key list at
  [camera_coaching_carousel_test.dart:37-42](app/test/presentation/widgets/creation/camera_coaching_carousel_test.dart#L37-L42)
  happens to be in current render order. Order doesn't matter for the
  assertion, but matching it to the new enum order keeps the file
  self-consistent. Added as an optional cleanup task.
- **CI checks unaffected:** No new strings (`lint:dart:i18n` passes),
  no widget/key changes (`lint:dart:a11y` passes), file size unchanged
  (`lint:dart:size` passes), no Go changes (server build untouched), no
  proto/buf changes.

No contradictions with architecture decisions or existing plans.

## Problem Statement

The camera-preview coaching carousel on the unified-create modal cycles
through four example subjects to teach new users what the camera is good at
capturing: **gear → clothes → food → flyer**. Each example shows for 3
seconds (with a nudge sentence and tappable thumbnail) before advancing.

The submitter — the product owner, filing via the in-app feedback form on
iOS DEV build 0.21.1 — wants **flyer** moved up to slot #1 or #2 so it's
either the first thing a new user sees on the camera screen, or seen
within the first cycle tick.

This is a pure UX-ordering tweak. No new copy, no new asset, no new
behaviour — just a re-shuffle of the enum that drives both the thumbnail
row order and the auto-cycle order. The four user-facing nudge strings,
their semantics labels, and the four image assets all already exist and
are unchanged by this issue.

## Context from Comments

There are no comments on the issue. The body is a single sentence
("Make move flyer up to #1 or #2 suggestion.") submitted via the in-app
feedback form — so there's no maintainer discussion, no debate, and no
follow-up clarification. Because the submitter is the product owner
(user10@example.org), the directive itself is authoritative, but the
choice between slot #1 and slot #2 is left open.

## Potential Solutions

### Option A: Flyer to slot #1
- **Approach:** Reorder the enum to `{ flyer, gear, clothes, food }`. Flyer becomes the first thumbnail rendered on the left of the carousel and the first nudge shown on mount ("Get the whole flyer in the frame").
- **Pros:** Maximum prominence — the very first impression of the camera screen highlights events / flyers, which is the most "social planner" use case and the one most likely to convert a new user into an event creator. Matches the literal "move to #1" reading of the issue.
- **Cons:** Flyer is the narrowest of the four subjects (most users don't have a flyer to photograph at the moment they first open the camera). Leading with a niche example may feel less welcoming than leading with the universal "gear, games, or anything you'd lend" nudge.
- **Effort:** Small (one enum line + two test expectations).

### Option B: Flyer to slot #2
- **Approach:** Reorder the enum to `{ gear, flyer, clothes, food }`. The carousel still opens on the universal "gear / games / anything you'd lend" nudge, but the very next cycle (3 seconds later) advances to flyer, well ahead of clothes and food.
- **Pros:** Keeps the warm, universally-applicable opener as the first impression, while still giving flyer prominent visibility — every user who watches the carousel past the first tick will see flyer second. The left half of the thumbnail row visually pairs the two highest-signal subjects (gear, flyer).
- **Cons:** Slightly less prominent than slot #1. A user who taps the shutter inside the first 3 seconds never sees flyer.
- **Effort:** Small (one enum line + two test expectations).

### Option C: Defer — gather data first
- **Approach:** Leave the order alone, instrument which example is active at shutter time, and revisit with data after a week.
- **Pros:** Data-driven. Avoids re-relitigating the order later if the chosen slot turns out to be wrong.
- **Cons:** Heavier than the change warrants — this is a 4-line code change with low risk and an authoritative submitter. Instrumenting a fleeting carousel state is also non-trivial. Likely overkill.
- **Effort:** Medium (analytics event + dashboard + waiting period).

## Recommendation

**Option B — flyer to slot #2.**

The product owner asked for "#1 or #2," which signals they're indifferent
between the two slots and want a judgment call. The reason to prefer #2:

1. **Universality on first impression.** "Try gear, games, or anything
   you'd lend" applies to nearly every user; "Get the whole flyer in the
   frame" applies only to users who happen to be holding a flyer.
   Leading with the universal nudge gives the warmest first impression
   without sacrificing flyer visibility.
2. **Flyer still gets first-cycle prominence.** Slot #2 means flyer is
   the second nudge any user sees, after just 3 seconds. The user is
   still framing the shot during this window, so they're almost
   guaranteed to see it.
3. **Symmetry of the thumbnail row.** Slot #2 puts the two
   highest-signal examples (gear, flyer) on the left half of the row,
   which reads naturally left-to-right.

If the product owner reviews this plan and prefers slot #1, the change
is identical except for the enum line — switch to Option A.

## Open Questions

- **Slot #1 vs slot #2?** — Recommendation: slot #2 (Option B), for the reasons above. If the answer is "actually #1," only the enum line differs.
- **Should we also reconsider the order of the *other* examples (clothes vs food)?** — Recommendation: no. The issue is scoped to flyer, the other examples currently appear in a deliberate order that the carousel was built with, and re-litigating it would expand scope. Leave clothes ahead of food.
- **Should the *initial index* differ from the cycle order — e.g., start on flyer but cycle gear → clothes → food → flyer?** — Recommendation: no. The current design ties initial display to enum order via `_currentIndex = 0`, and decoupling them would add code complexity for no clear win. A reorder of the enum is the right primitive.
- **Any analytics?** — Recommendation: not needed for this change. Add only if a follow-up debate about the order arises.

## Implementation Plan

### Phase 1: Reorder the enum and update tests
**Goal:** Ship the reorder as a single, small, low-risk change.

**Tasks:**
- [ ] In [camera_coaching_carousel.dart:33](app/lib/presentation/widgets/creation/camera_coaching_carousel.dart#L33), change `enum _CoachingExample { gear, clothes, food, flyer }` to `enum _CoachingExample { gear, flyer, clothes, food }` (Option B). The `_nudgeFor`, `_semanticsLabelFor`, and `_assetPathFor` switches stay as-is — they're keyed on enum values, not order, and don't need to change.
- [ ] Update the class docstring at [camera_coaching_carousel.dart:10-22](app/lib/presentation/widgets/creation/camera_coaching_carousel.dart#L10-L22). Line 13 reads "(gear, clothes, food, event flyer)" and is now stale; change to "(gear, event flyer, clothes, food)" so the docstring matches the actual cycle order.
- [ ] Update [camera_coaching_carousel_test.dart:50-64](app/test/presentation/widgets/creation/camera_coaching_carousel_test.dart#L50-L64) — the "cycles to the next nudge after 3 seconds" test expects clothes after gear; change the expected second-tick text to the flyer nudge `'Get the whole flyer in the frame'`.
- [ ] The "first nudge on initial mount" test ([camera_coaching_carousel_test.dart:10-18](app/test/presentation/widgets/creation/camera_coaching_carousel_test.dart#L10-L18)) still passes — gear stays in slot #1. No change required.
- [ ] The "renders exactly four thumbnails" test ([camera_coaching_carousel_test.dart:20-48](app/test/presentation/widgets/creation/camera_coaching_carousel_test.dart#L20-L48)) still passes — it asserts presence of all four keyed thumbnails, not their order. No required change; *optionally* reorder the `thumbs` key-list at lines 37-42 to match the new enum order (gear, flyer, clothes, food) so the file stays self-consistent. Cosmetic only.
- [ ] The "tapping food thumbnail locks the active example" test ([camera_coaching_carousel_test.dart:66-84](app/test/presentation/widgets/creation/camera_coaching_carousel_test.dart#L66-L84)) still passes — it taps by key (`camera-coaching-thumb-food`) and asserts the food nudge, both order-independent. Remove the inline parenthetical `// Tap the food thumbnail (index 2)` at line 72 (per CLAUDE.md no-trivial-comments rule); leave the comment as `// Tap the food thumbnail.` so it stays correct regardless of enum order.
- [ ] Run `npm run test:app` (from project root) with `--reporter expanded`; confirm all four tests pass.
- [ ] Run `npm run lint:dart:a11y` and `npm run lint:dart:i18n` to confirm no a11y or hardcoded-string regressions (expected: no change — keys/strings are untouched).
- [ ] Manually verify on iOS DEV build: open the unified-create modal, watch the carousel — gear nudge first, flyer nudge after 3 s, then clothes, then food, then back to gear. Verify a second time with the device locale switched to Spanish (`es`) so the Spanish nudges (`unifiedCreateCameraCoaching*` in `app_es.arb`) render in the new order.

**Validates:** A single enum-order line drives the user-visible behaviour change, the test suite still covers the carousel's invariants, the documentation stays in sync with the code, and the in-app camera screen shows flyer prominently within the first cycle tick in both supported locales.

### Phase 2 (optional, only if reviewer prefers Option A)
**Goal:** Switch to flyer-in-slot-#1 if that's the chosen reading.

**Tasks:**
- [ ] Change the enum to `{ flyer, gear, clothes, food }`.
- [ ] Update "first nudge on initial mount" test to expect the flyer nudge.
- [ ] Update "cycles after 3 s" test to expect the gear nudge.

**Validates:** Same as Phase 1, with flyer as the immediate first impression.

## Related Issues

None found. This is a small, self-contained UX tweak with no upstream
dependency.
