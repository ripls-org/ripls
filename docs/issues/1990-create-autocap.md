## Issue #1990: Creation modal text input

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1990
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A user reported (via in-app feedback, iOS 26.4.2, app v0.21.1) that the text
input in the creation modal does not automatically capitalize the first letter
of a sentence. The expected iOS keyboard behavior (`TextCapitalization.sentences`)
is missing.

The "creation modal" in current product nomenclature is the **unified-create
flow** opened from the home tab. Its primary text inputs live in:

- The input drawer's Text panel — the prompt textarea where users type
  something like "anyone have a hammer drill I could borrow this weekend?"
  ([unified_create_input_drawer.dart:375](app/lib/presentation/screens/create/unified_create_input_drawer.dart#L375),
  the shared `_BorderlessGlassField`).
- The preview card's editable Title and Description fields shown after AI
  generation ([unified_preview_card.dart:373](app/lib/presentation/widgets/create/unified_preview_card.dart#L373),
  `_SyncedTextField`).
- The item-details push sheet's Brand / Model / Material rows
  ([unified_item_details_sheet.dart:134](app/lib/presentation/widgets/create/unified_item_details_sheet.dart#L134)).

None of these set `textCapitalization`, so they fall back to Flutter's default
`TextCapitalization.none`, which disables iOS sentence auto-capitalization. By
contrast, the older per-type creation modals (`TextInputArea` used by
experience / community / gear / request flows) already set
`TextCapitalization.sentences`
([text_input_area.dart:49](app/lib/presentation/widgets/creation/text_input_area.dart#L49)),
which is why this regression is specific to the unified-create surface.

The URL tab in the same drawer should **not** auto-capitalize — URLs are
case-sensitive — so any fix needs to discriminate per-panel rather than flip a
single global on the shared `_BorderlessGlassField` widget.

## Context from Comments

The issue has no comments. It was filed via the in-app feedback flow on
2026-05-16 with a screenshot attached and has not been triaged. The description
is one sentence, so the scope is narrow but the issue does not specify *which*
text input in the modal — see the Open Questions for how I'm interpreting that.

## Potential Solutions

### Option A: Plumb `textCapitalization` through `_BorderlessGlassField`, fix three call sites

- **Approach:** Add an optional `TextCapitalization textCapitalization` parameter
  to `_BorderlessGlassField` (defaulting to `none` for safety), pass
  `TextCapitalization.sentences` from `_TextPanel`, leave the URL panel on the
  default. In a follow-up step, also pass `sentences` to the title and
  description `_SyncedTextField` instances in `unified_preview_card.dart` and
  to the brand / model / material rows in `unified_item_details_sheet.dart`.
  (Value and weight stay numeric and don't need it.) Brand and model can argue
  for `TextCapitalization.words`, but `sentences` is the lowest-surprise and
  matches what `TextInputArea` does for every other modal.
- **Pros:** Targeted fix that respects the URL exception. Keeps the shared
  widget shared. Brings all unified-create text entry to parity with the older
  per-type modals.
- **Cons:** Touches three files. Need to be careful about which fields get
  `sentences` vs `words` vs `none` in the item-details sheet.
- **Effort:** Small.

### Option B: Only fix the prompt textarea in the Text panel

- **Approach:** Smallest possible change — add `textCapitalization:
  TextCapitalization.sentences` only to the Text-panel call site by exposing
  the parameter on `_BorderlessGlassField` and passing it from `_TextPanel`.
  Leave the preview card and item-details sheet untouched.
- **Pros:** Minimal blast radius. Directly addresses the most likely repro
  path (typing the initial prompt).
- **Cons:** Title / description editing in the preview card still won't
  auto-capitalize, so the user (or another user) will likely re-file the same
  feedback for those fields. We have the fix in hand; deferring it is busywork.
- **Effort:** Small.

### Option C: Default `_BorderlessGlassField` to `sentences`, opt out for URL

- **Approach:** Flip the parameter default to
  `TextCapitalization.sentences` and have `_UrlPanel` pass
  `TextCapitalization.none` explicitly.
- **Pros:** Future call sites of `_BorderlessGlassField` get the right default
  for free.
- **Cons:** `_BorderlessGlassField` is a file-private widget (leading
  underscore) with two call sites today, so "future call sites" is hypothetical.
  Defaults that quietly enable iOS behavior also surprise readers more than
  explicit per-call settings. Not worth the implicit-default cost.
- **Effort:** Small.

## Recommendation

**Option A.** The reported bug ("should automatically start with a capital
letter") is almost certainly about the prompt textarea, but the title and
description fields in the preview card have the exact same defect and the
same user will encounter both within seconds of each other. Fixing all three
unified-create text-input surfaces in one pass is cheaper than three round
trips, and parity with the existing `TextInputArea` standard is the goal.

For the item-details sheet, default brand / model / material to
`TextCapitalization.sentences` (matching `TextInputArea`'s standard). Brand
names often want word-capitalization ("DeWalt", "REI") but `sentences` still
capitalizes the first letter without forcing every word — close enough, and
consistent. Value and weight stay numeric inputs with no capitalization
setting.

## Open Questions

- **Does "creation modal" refer to the unified-create flow specifically, or
  could the user mean an older per-type creation modal (experience, community,
  gear, request) accessed via a different entry point?** — Recommendation:
  treat this as a unified-create issue. The older modals all use `TextInputArea`
  which already sets `TextCapitalization.sentences`, so they cannot exhibit
  this bug; only the unified-create surface can. The reporter is internal
  (`user10@example.org`) and the unified-create modal is the default entry point
  on iOS, so this is the most likely repro path.
- **Should brand / model in the item-details sheet use
  `TextCapitalization.words` instead of `sentences`?** — Recommendation: no,
  stay with `sentences` for consistency with `TextInputArea`. `words` is
  reasonable for brand specifically, but mixing capitalization modes across
  the same form is more jarring than helpful, and iOS users can manually shift
  to capital for the next word anyway.
- **Should we add a regression test?** — Recommendation: **no, with one
  narrow exception**. `docs/client/testing.md` is emphatic that the project
  tests business logic in ViewModels and explicitly lists "Limited UI Testing"
  as a weakness of the current approach. `textCapitalization` is a pure
  widget prop with no ViewModel involvement, so there is no business-logic
  hook to assert against — a widget test here would be the kind of UI-config
  assertion the doc steers away from. The exception worth keeping is **one**
  widget test in `app/test/presentation/screens/create/` that pumps the
  drawer in URL mode and asserts the URL field is **not** `sentences`. That
  invariant (URLs stay case-sensitive) is the one regression that would not
  be obvious from manual smoke testing — autocapitalizing a URL would silently
  corrupt host names. Skip tests for the prompt textarea, the preview card,
  and the item-details sheet; rely on the audit + manual run-through.

## Implementation Plan

### Phase 1: Fix the prompt textarea (the reported defect)

**Goal:** Eliminate the user-reported bug — typing in the unified-create Text
tab auto-capitalizes the first letter on iOS.

**Tasks:**
- [ ] Add `TextCapitalization textCapitalization` parameter to
      `_BorderlessGlassField`, defaulting to `TextCapitalization.none`.
- [ ] Pass `TextCapitalization.sentences` from `_TextPanel`'s
      `_BorderlessGlassField` call site.
- [ ] Pass `TextCapitalization.none` explicitly from `_UrlPanel` (matches
      default, but make the URL-is-case-sensitive intent visible to readers).
- [ ] One narrow widget test: pump drawer in URL mode, assert the URL field's
      `TextCapitalization` is **not** `sentences`. This is the only assertion
      worth automating per the testing-doc guidance (see Open Questions);
      manual smoke covers the text-mode side.

**Validates:** The original feedback report (typing the initial prompt) is
fixed, and the URL field remains unmodified — confirming the per-panel
discrimination works.

### Phase 2: Fix the editable preview card fields

**Goal:** Title and description editing on the preview card auto-capitalize
the same way as the prompt.

**Tasks:**
- [ ] Add `TextCapitalization textCapitalization` parameter to
      `_SyncedTextField` in `unified_preview_card.dart` (default
      `TextCapitalization.none` for safety).
- [ ] Pass `TextCapitalization.sentences` from both title and description
      call sites.
- [ ] No widget test — pure UI-prop change with no business logic; verify
      manually by opening the unified-create modal, generating content, and
      editing the title.

**Validates:** Edit flows after AI generation match prompt-entry behavior. No
mode-switch surprise for the user.

### Phase 3: Fix the item-details push sheet

**Goal:** Brand / model / material rows in the gear-item details sheet
auto-capitalize.

**Tasks:**
- [ ] Update the `_row()` helper in `unified_item_details_sheet.dart` to set
      `textCapitalization: TextCapitalization.sentences` on its inner
      `TextField`.
- [ ] Leave `_weightRow` untouched (numeric input).
- [ ] No widget test — verify manually via the gear-item details push sheet.

**Validates:** Every editable text field in the unified-create flow has
`TextCapitalization.sentences`. The unified-create surface reaches parity with
the older `TextInputArea`-based modals.

## Audit Notes (post-/ripls-audit)

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/client/modals.md`, `docs/client/create.md`,
`docs/server/architecture.md`, and `docs/server/observability.md`. Changes:

- **Testing strategy revised (Phase 1–3 + Open Questions).** Original plan
  recommended widget tests across all three phases. `docs/client/testing.md`
  is explicit that ViewModel tests are the focus and lists "Limited UI
  Testing" as a project weakness. Since `textCapitalization` is a pure
  widget prop with no ViewModel hook, broad widget testing here would push
  against the doc's guidance. Retained exactly **one** narrow widget test —
  the URL-mode "must not be `sentences`" assertion — because that invariant
  (URLs stay case-sensitive) is a silent-corruption risk that manual smoke
  testing would miss. Dropped all other test tasks; rely on manual
  verification.
- **MVVM architecture (architecture.md):** plan is compliant. The change
  touches only View-layer widget props on a file-private wrapper
  (`_BorderlessGlassField`, `_SyncedTextField`, `_row()`). No ViewModel,
  Repository, or Service layer is affected. The
  "ViewModels are the ONLY layer that calls Repositories" rule and adjacent
  invariants stand untouched.
- **Caching (caching.md):** not applicable — no data access or invalidation
  changes.
- **i18n (i18n.md):** not applicable — no new user-visible strings; iOS
  auto-capitalization happens on the soft keyboard, not in our string
  table. ARB files untouched.
- **Accessibility (accessibility.md):** not applicable — no new `Semantics`
  labels, no `Tappable` / `IconAction` / `LiveRegion` use, no l10n keys.
  Auto-capitalization is invisible to VoiceOver / TalkBack (which read the
  actual entered character regardless of keyboard hint). The
  `scripts/check_a11y.js` allowlist stays empty.
- **Modal architecture (modals.md):** not applicable — the unified-create
  modal is explicitly listed as one of the "intentionally-distinct surfaces"
  (not GlassSheet-based), and this plan does not touch the modal shell,
  drag handle, scrim, or dismissal pattern.
- **Creation flow doc (create.md):** docs/client/create.md predates the
  unified-create surface (it documents the legacy per-type modals using
  `TextInputArea`). The plan's parity argument is correct: `TextInputArea`
  already sets `TextCapitalization.sentences`; the unified-create surface
  must match. Not flagging a docs update as in-scope for this fix, but the
  unified-create flow's absence from `create.md` is a pre-existing gap
  worth noting.
- **Server architecture / observability (architecture.md, observability.md):**
  not applicable — pure client-side keyboard property change. No proto
  changes, no RPC changes, no log fields, no request-correlation impact.

## Related Issues

- #1933 — keyboard / `viewInsets` plumbing in the unified-create modal; the
  fix in this plan is orthogonal but touches the same file
  (`unified_create_modal.dart` parent / `unified_create_input_drawer.dart`),
  so worth a quick re-read of the `mergeForBottomSheet` rationale before
  editing.
- #1980 — dismiss semantics on the unified-create modal. Same surface,
  unrelated change.
