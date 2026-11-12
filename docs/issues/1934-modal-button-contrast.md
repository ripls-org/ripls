# Issue #1934: Feedback is hidden and modal is too hard to read

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1934
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The issue conflates two complaints from the same root surface — the glass-modal footer in [PlusButtonModal](../../app/lib/presentation/widgets/plus_button_modal.dart):

1. **"Feedback is hidden"** — Feedback is currently the *secondary* (left) button on the plus-button create modal ([plus_button_modal.dart:501-518](../../app/lib/presentation/widgets/plus_button_modal.dart#L501-L518)). The secondary slot uses a translucent white-on-white style (`modalSecondaryButtonBackground = 0x1AFFFFFF`, 10% white fill + 90% white text) on a frosted glass surface (`modalSurface = 0x26FFFFFF`, 15% white fill). The button reads as a "cancel" affordance and visually disappears against the glass — so users don't notice Feedback exists. Feedback is *also* nested inside the avatar → profile menu modal ([profile_menu_modal.dart:57-62](../../app/lib/presentation/widgets/profile_menu_modal.dart#L57-L62)), which is the only other in-product entry point. (A third exists, but only on `server_error_screen.dart` when the API is down.)

2. **"Modal is too hard to read" / "frosted modal buttons (especially orange)"** — The *primary* (right) button on every glass modal — including PlusButtonModal's "Invite Someone" — uses `modalPrimaryButtonBackground = lightAccent` (`0xFFE89B7E`, light coral) with solid white text ([glass_footer_buttons.dart:200-220](../../app/lib/presentation/widgets/modal/glass/glass_footer_buttons.dart#L200-L220), [app_colors.dart:14, 667](../../app/lib/core/theme/app_colors.dart#L14)). Light-coral + white text yields a contrast ratio of roughly **2.3:1** — well below WCAG AA's 4.5:1 minimum for normal text and below even the 3:1 minimum for large text. On the frosted surface, with arbitrary content visible through the blur, legibility degrades further.

Both complaints have the same root: the glass-modal footer's contrast model is broken. Fixing the colors fixes both perceived problems, and adding a more prominent feedback entry point fixes the discoverability concern that low contrast alone can't solve.

## Context from Comments

**No comments** on the issue. This was filed via the in-app feedback form on iOS (build 0.20.1) by `user10@example.org`. The lack of discussion is normal for user-submitted feedback but means we have no clarification on which screen the user was looking at, what exactly they couldn't read, or where they want Feedback surfaced. The build context (iOS 26.4.2, DEV environment) tells us the user is on a recent app build.

The wording of the title ("Feedback is hidden **and** modal is too hard to read") plus the description ("Need to bring this back to create or somewhere else to make more noticeable. The frosted modal buttons (especially orange) are too hard to see") leaves only mild ambiguity — they could be reporting two unrelated bugs, but both surface in the same widget (`GlassFooterButtons` on the create modal), so I'm treating them as one bundle.

## Potential Solutions

### Option A: Darken the primary CTA color + darken (don't brighten) the secondary pill (single-token fix)

- **Approach:**
  - Change `AppColors.modalPrimaryButtonBackground` from `lightAccent` (`0xFFE89B7E`) to `lightPrimary` (`0xFFE07A5F`, "Coral sunset"). White on `0xFFE07A5F` ≈ **3.3:1** — passes AA for large text (≥18pt or ≥14pt bold). The button label uses `ModalTheme.buttonTextStyle`, which we must confirm meets large-text thresholds; if it doesn't, also bump the font weight to `w700` so the 3:1 threshold applies.
  - **AUDIT NOTE:** the original draft proposed raising `modalSecondaryButtonBackground` from 10% white to 25% white. That is the **wrong direction** — see contrast math in the "Audit notes" section below. Instead, replace the white fill with a *darker* fill so the white text gets more contrast against the pill, not less. Concretely:
    - Change `modalSecondaryButtonBackground` from `0x1AFFFFFF` (10% white) to `0x33000000` (~20% black). On a mid-gray underlying scene composited through the standard scrim + glass surface, this drops the apparent pill luminance from ~0.48 to ~0.27 (vs. 0.95 for the 90%-white text → contrast jumps from ~1.87:1 to **~3.0:1**, crossing WCAG large-text threshold).
    - Raise `modalSecondaryButtonBorder` from `0x4DFFFFFF` (30%) to `0x80FFFFFF` (50%) so the pill outline is unambiguous against the new darker fill.
    - Optionally raise `modalSecondaryButtonText` from `0xE6FFFFFF` (90%) to `0xF2FFFFFF` (95%) to claw back a little more contrast — but only if QA shows the existing 90% reads grey on the darker fill.
- **Pros:** All-token fix in [app_colors.dart](../../app/lib/core/theme/app_colors.dart) — touches one file, instantly cascades to every glass modal in the app, including the feedback sheet itself (which uses the same `GlassFooterButtons`). Stays inside the documented "White-on-glass legibility deviation" exception from [docs/client/modals.md](../client/modals.md) — glass modals are intentionally non-theme-aware. Honors the **Reuse existing theme tokens** memory — no per-component coral constants and no new theme variants.
- **Cons:** Coral-sunset on glass is only 3.3:1 — accessible only if we lock in the large-text rule by bolding the label. The darker secondary pill is a noticeable aesthetic shift from the current near-invisible look; we need design eyes on it. Doesn't address "Feedback is hidden" if the user means *discoverability* rather than *contrast*.
- **Effort:** Small.

### Option B: Option A + add a prominent feedback entry point on the home screen

- **Approach:** Everything in Option A, plus surface Feedback as a first-class entry — e.g.,
  - (a) a small "Send Feedback" tile in `empty_content_state.dart` (Workshop empty state), or
  - (b) a persistent "Feedback" item in the [Workshop tab's settings/profile area](../../app/lib/presentation/widgets/profile_menu_modal.dart) promoted out of the avatar-tap modal into a visible bar/link.
  - Option (a) is the lightest touch; option (b) closer matches "bring this back to create or somewhere else to make more noticeable" since Workshop is where the create flow lives.
- **Pros:** Directly addresses the discoverability complaint ("hidden"). Empty-state placement gets eyes on Feedback when users are most likely to need help. Doesn't require renaming or restructuring.
- **Cons:** Larger scope. Risks crowding the empty state or the Workshop chrome. Needs design alignment on placement.
- **Effort:** Medium.

### Option C: Restructure the plus-modal footer so "Feedback" isn't bundled with "Invite Someone"

- **Approach:** Remove "Feedback" from the plus-modal footer entirely. Promote it to a small text-link or icon row at the bottom of the modal body ("Found a bug? Send us feedback"). The plus-modal footer becomes single-action ("Invite Someone" as full-width primary). Pair with Option A's color fixes so the primary CTA is legible.
- **Pros:** Footer no longer mixes "create a thing" with "tell us about bugs" — those are different mental tasks. Single full-width primary reads cleaner. The text-link feedback affordance is the conventional pattern for "tell us if something's broken."
- **Cons:** Doesn't help discoverability outside the plus-modal — Feedback is still only reachable from the same two screens. May require l10n strings for the new text-link copy. Slight visual churn in a recently-redesigned modal.
- **Effort:** Medium.

## Recommendation

**Ship Option A first as a same-day fix** (with the audit-corrected secondary-button direction — *darken* the pill, don't brighten it). It's the smallest token change, lands in one file, and tackles the objectively-broken WCAG contrast on every glass modal in the app — not just the plus-modal. The color shift from `lightAccent` to `lightPrimary` for the primary CTA was likely an oversight when the glass-modal revamp landed (per [docs/issues/1797-glass-modal-revamp.md](./1797-glass-modal-revamp.md), `modalPrimaryButtonBackground` was matched to `lightAccent` as a brand-palette choice, but the contrast math wasn't checked against white text on the translucent surface). The secondary-button regression dates from the same revamp — the 10%-white-on-15%-white-glass pill is below the noise floor for white text.

**Then decide on Option B vs. Option C in a follow-up conversation with design.** Option B (promote Feedback discoverability) directly answers "bring this back to create or somewhere else to make more noticeable", but lands in surface area we should sanity-check with mocks before changing. Option C is structurally cleaner but ambiguous on what it gives the user beyond a tidier footer. I lean Option B because it's the literal ask in the issue body. The "Feedback is hidden" complaint is, at root, a discoverability problem that contrast alone can't fully solve.

The Option A token change should be done first because it's required either way — Option B and C both leave a glass modal with the same illegible CTA if A isn't applied.

## Open Questions

- **Does the issue author mean "Feedback is visually hidden by low contrast" or "Feedback is buried in the menu tree"?** — Recommendation: assume **both**. Ship Option A (fixes the visual problem). Then bring Option B vs. C to design as a follow-up — don't try to guess from one sentence.
- **What's the target WCAG level — AA or AAA?** Ripls hasn't documented a target. — Recommendation: target **AA (4.5:1 normal, 3:1 large)** as the default. The proposed `lightPrimary` button at 3.3:1 only passes AA for large text, so we should also confirm the button label qualifies (>=18pt or >=14pt bold). If it doesn't, bump to `w700`.
- **Should we add an outline/shadow to the orange button for separation from the glass, in addition to darkening it?** — Recommendation: keep the existing `BoxShadow` ([glass_footer_buttons.dart:208-212](../../app/lib/presentation/widgets/modal/glass/glass_footer_buttons.dart#L208-L212)) — it already gives elevation. Don't add a border; that would mean rebalancing every secondary button border too.
- **Does the secondary button restyle (Option A's white-25%-fill / 50%-border) cascade unwantedly to modals where the secondary really is "Cancel"?** — Recommendation: visually QA the cancel-style use cases (e.g. gear delete, community settings) in browser before merging. If a more muted look is needed for Cancel specifically, add an explicit "destructive vs. neutral secondary" variant to `GlassFooterButtons` — but only if QA shows the unified pill reads wrong as Cancel.
- **Should `FeedbackSheet`'s own primary "Submit" button — which also uses `modalPrimaryButtonBackground` — be exempt from the color change?** — Recommendation: no, apply the global change. A more legible submit button on the feedback form itself is also a bug fix.

## Implementation Plan

### Phase 1: Token-level contrast fix (Option A, revised per audit)
**Goal:** Every glass modal in the app gets a readable primary CTA and a more legible secondary CTA. No per-component overrides; everything lands in [app_colors.dart](../../app/lib/core/theme/app_colors.dart).
**Tasks:**
- [ ] In [app_colors.dart:667](../../app/lib/core/theme/app_colors.dart#L667), change `modalPrimaryButtonBackground = lightAccent` → `modalPrimaryButtonBackground = lightPrimary`. Update the inline doc-comment to reflect the new source (`lightPrimary` / coral sunset) and the WCAG rationale.
- [ ] In [app_colors.dart:657](../../app/lib/core/theme/app_colors.dart#L657), change `modalSecondaryButtonBackground` from `0x1AFFFFFF` (10% white) to `0x33000000` (~20% black). **Direction reversed from the original draft per audit math** — the darker fill increases white-text contrast on the glass surface. Update the inline comment.
- [ ] In [app_colors.dart:660](../../app/lib/core/theme/app_colors.dart#L660), raise `modalSecondaryButtonBorder` from `0x4DFFFFFF` (30%) to `0x80FFFFFF` (50%). Update the inline comment.
- [ ] Confirm `ModalTheme.buttonTextStyle` (in `app/lib/core/theme/app_theme.dart`) meets WCAG large-text thresholds (≥18pt OR ≥14pt at `FontWeight.w700`). If it falls short, raise the weight on the primary button text style. Read the file before editing — don't guess.
- [ ] Manual visual QA on iOS simulator AND Android emulator: PlusButtonModal, FeedbackSheet, ProfileMenuModal, plus the in-scope glass modals in [docs/issues/1797-glass-modal-revamp.md](./1797-glass-modal-revamp.md) (location picker, time modal, RSVP, transfer flows). Compare before/after screenshots over a cream background, a dark scene, AND a colorful underlying screen (where the glass blur dominates).
- [ ] Spot-check the genuinely "Cancel"-style use cases of `_SecondaryButton` (gear delete confirm, community settings exit, etc.). The new darker pill *should* still read as "secondary" because primary uses brand coral — but if QA shows the dark pill reads as a destructive button, defer to Phase 3.
- [ ] Run `npm run lint:dart:i18n`, `npm run lint:dart:a11y`, and `npm run lint:ephemeral-comments` — token changes shouldn't trip them, but the run confirms.
- [ ] Add a token-level regression test under [app/test/core/theme/](../../app/test/core/theme/) (or extend an existing theme test) asserting `AppColors.modalPrimaryButtonBackground == AppColors.lightPrimary` and the new secondary fill/border values. This is the closest thing to a regression guard in absence of golden tests (per [docs/client/testing.md §Weaknesses](../client/testing.md) — "No Golden Tests").
- [ ] Run `npm run test:app --reporter expanded` to ensure no widget tests break on the token change.

**Validates:** A single-file color change eliminates the WCAG contrast bug across the entire glass-modal surface area, both buttons read clearly on the frosted surface, and the existing "Cancel"-style use cases still parse correctly. After this lands, "modal is too hard to read" should be resolved end-to-end.

### Phase 2: Promote Feedback discoverability (Option B, pending design review)
**Goal:** Make Feedback visible from the create surface without burying it as a secondary button on the plus modal.
**Tasks:**
- [ ] Design check-in: pick between (a) Feedback tile in `empty_content_state.dart`, (b) Feedback link in Workshop chrome, or (c) keep Feedback only on PlusButtonModal but make the secondary button's label/icon more distinct (Option C-flavored).
- [ ] Implement the chosen placement. Reuse the existing `FeedbackSheet.show()` entry point — no new business logic, no new ViewModel, no new repository plumbing (per [client architecture](../client/architecture.md), this is a presentation-layer addition only).
- [ ] **i18n compliance.** Any new user-visible string must be added to BOTH `app_en.arb` AND `app_es.arb` with an `@key` metadata block, then accessed via `context.l10n.*`. Do not hardcode strings — `npm run lint:dart:i18n` will reject. If reusing the existing `sidebarFeedback` key, ensure the description still fits the new context; otherwise add a new namespaced key (e.g. `workshopFeedback`).
- [ ] **Accessibility.** If the new entry point is a tap target, use the `Tappable` primitive (or `IconAction` for icon-only) with a `semanticsLabel` from `context.l10n.*` — per [docs/client/accessibility.md](../client/accessibility.md), raw `GestureDetector` and unlabelled icon buttons are lint-rejected.
- [ ] **Color discipline.** If the new entry point is NOT inside a glass modal (e.g. it lives on `empty_content_state.dart` or the Workshop chrome), use theme-aware helpers (`AppColors.primary(context)`, `AppColors.textPrimary(context)`, etc.) — NOT the `modal*` glass tokens. Glass tokens are scoped to glass modals per [docs/client/modals.md](../client/modals.md). If the entry point IS inside a glass modal, use the `modal*` family.
- [ ] Add a ViewModel-level test only if the new entry triggers business logic; otherwise this is purely presentation — visual QA suffices per [docs/client/testing.md](../client/testing.md)'s "ViewModels are primary focus" rule.

**Validates:** That users discover Feedback through a path other than the plus-modal footer or the avatar-tap profile menu. Ships only if design agrees on placement.

### Phase 3: (Optional) Refactor secondary-button variants
**Goal:** If Phase 1 QA shows that the darker secondary pill misreads as destructive (or otherwise wrong) in genuinely-Cancel contexts, introduce a variant flag on `GlassFooterButtons`.
**Tasks:**
- [ ] Add a `secondaryVariant` parameter (`neutral` / `cancel`) to [GlassFooterButtons](../../app/lib/presentation/widgets/modal/glass/glass_footer_buttons.dart#L16). Default to `neutral` (the new darker pill); pass `cancel` from the few sites that need the more muted look.
- [ ] Add tokens in `app_colors.dart` for the cancel variant. Stay in the `modal*` namespace.
- [ ] **i18n cleanup (existing tech debt, optional in scope):** [plus_button_modal.dart:503, 510](../../app/lib/presentation/widgets/plus_button_modal.dart#L503-L510) currently hardcodes `secondaryLabel: 'Feedback'` and `primaryLabel: 'Invite Someone'`. If we're touching the file for Phase 3, replace these with `context.l10n.sidebarFeedback` (or a new key) and `context.l10n.commonInviteSomeone` (or new key), and add both locales to ARB.

**Validates:** Secondary-button styling supports both "neutral secondary" and "Cancel" semantics without per-call-site overrides. Skip this phase entirely if Phase 1 QA shows no regression.

## Audit notes (added by `/ripls-audit`)

The plan was reviewed for compliance with the architectural docs under [docs/client/](../client/) and [docs/server/](../server/). Findings — each addressed in the updated plan above:

1. **Contrast math error in Option A's original secondary-button recommendation.** The first draft proposed raising `modalSecondaryButtonBackground` from 10% white to 25% white, with the intuition that "a more visible pill = more legible." That's backwards on a white-on-glass-on-dark-scrim stack. Compositing `0x59000000` (35% black scrim) → `0x26FFFFFF` (15% white modal surface) → secondary fill → `0xE6FFFFFF` (90% white text) over a mid-gray underlying scene yields an apparent pill luminance of ~0.48 with the original 10%-white fill (text contrast ~1.87:1) and ~0.57 with 25%-white (text contrast ~1.62:1 — **worse**). Switching to a *darker* fill (`0x33000000`, 20% black) drops apparent luminance to ~0.27 and lifts text contrast to ~3.0:1 — over the WCAG large-text threshold for white-on-dark. Option A has been revised to reflect this.

2. **Glass modals are intentionally NOT theme-aware.** [docs/client/modals.md](../client/modals.md) §"White-on-glass legibility deviation" explicitly documents that glass modals render identically in light and dark themes (white-on-translucent over a dark scrim, scrim adapts the theme contrast). This is a documented exception to the "all colors must be theme-aware" rule in [docs/client/architecture.md](../client/architecture.md). The plan's all-static-token approach is correct — no theme-aware `Color Function(BuildContext)` is needed for `modal*` tokens.

3. **No server, no repository, no ViewModel scope.** This is a presentation-layer color-token change. The plan touches only [app/lib/core/theme/app_colors.dart](../../app/lib/core/theme/app_colors.dart) in Phase 1, and at most a few presentation widgets in Phase 2/3. No `FeedbackService` RPC changes, no `MediaRepository` changes, no `feedbackSheetProvider` (the ViewModel) changes. Compliant with the MVVM rule "ViewModels are the ONLY layer that calls Repositories" — we're not touching any of those layers.

4. **Existing i18n violations in `plus_button_modal.dart`.** Lines 503 and 510 hardcode `'Feedback'` and `'Invite Someone'`. This pre-dates this issue and is flagged in the lint check `npm run lint:dart:i18n` ([docs/client/i18n.md](../client/i18n.md)). The plan does not require fixing them as part of #1934, but Phase 3 calls them out as cheap cleanup if we're already editing the file. Any *new* string introduced by Phase 2 must go to both `app_en.arb` and `app_es.arb` and be accessed via `context.l10n.*` — never hardcoded.

5. **Testing approach revised.** [docs/client/testing.md](../client/testing.md) calls out "No Golden Tests" as a known weakness — the codebase has no visual regression coverage. ViewModel-first testing doesn't fit a pure color-token change, and `flutter test --reporter expanded` is required (not the default `compact` reporter — would produce enormous piped output). The revised plan adds a token-level regression test (`expect(AppColors.modalPrimaryButtonBackground, AppColors.lightPrimary)`) as the closest regression guard available and reframes coverage as manual visual QA on both platforms.

6. **Phase 2 color-token discipline.** If the new Feedback entry point lives outside a glass modal (e.g. `empty_content_state.dart`'s ElevatedButton row, or Workshop chrome), it must use theme-aware `AppColors.xxx(context)` helpers — `AppColors.primary(context)`, `AppColors.textPrimary(context)`, etc. Pulling the `modal*` glass tokens onto a non-glass surface would violate the "Glass tokens are scoped to glass modals" rule in [docs/client/modals.md](../client/modals.md) and break dark mode. Added to the Phase 2 task list.

7. **Accessibility primitives are unchanged.** `GlassFooterButtons` already composes `Tappable` with `semanticsLabel` ([glass_footer_buttons.dart:153-162, 224-233](../../app/lib/presentation/widgets/modal/glass/glass_footer_buttons.dart#L153-L162)), so the color-token change introduces no new violations under `npm run lint:dart:a11y`. Phase 2's new entry point must continue this — use `Tappable`/`IconAction` with l10n-sourced semanticsLabels.

8. **Server observability is out of scope.** Issue is client-only UI. No `logging.LoggerWithContext` usage, no request-ID propagation concerns, no masking ([docs/server/observability.md](../server/observability.md) does not apply).

## Related Issues
- #1797 — Glass modal revamp; defines the white-on-translucent contrast model that this issue exposes as broken in the button-token layer. Cited under the "White-on-glass legibility deviation" exception.
- #1802 — Modals glass sweep; in-scope surface list for Phase 1 QA.
- #936 — Earlier dark-mode contrast bug on the same `FeedbackSheet`. Resolved at the input-field layer but did not touch the footer buttons.
