# Issue #1933: IOS keyboard hides input area

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1933
**Filed:** 2026-05-15T15:08:32Z | **Last activity:** 2026-05-15T15:42:28Z
**Labels:** user-feedback | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

When a user taps the "+" floating action button on the home screen, the
unified-create modal opens with the input drawer at the bottom of the
screen. Tapping the textarea in the **Text** tab brings up the iOS
keyboard, which covers the textarea, the cycling hint, and the
"Generate" primary button — the user can type but cannot see what
they're typing, and cannot tap Generate without dismissing the keyboard.

Reported on iOS 26.4.2 / iPhone, app version 0.20.1 (build 0.20.1, DEV
environment). The same panel structure is used by the **URL** tab, so
the bug presumably reproduces there too.

The drawer already *tries* to handle keyboard insets — there's an
`AnimatedPadding` at
[unified_create_modal.dart:114-118](app/lib/presentation/screens/create/unified_create_modal.dart#L114-L118)
that adds `bottom: viewInsets` padding intended to push the drawer
above the keyboard. The bug is that `viewInsets` is always 0 at that
read site, so the padding is a no-op.

## Context from Comments

No comments on the issue — just the original bug report with a
screenshot showing the keyboard sitting over the input drawer. Per the
"no comments" guidance in the skill template, this is a sole-author
report; the planning decisions below come purely from code analysis.

## Root Cause

[unified_create_modal.dart:34-58](app/lib/presentation/screens/create/unified_create_modal.dart#L34-L58)
opens the bottom sheet with `useSafeArea: false` so the hero image can
render full-bleed under the status bar. The comment block at lines
37-46 explains that
`showModalBottomSheet(useSafeArea: false)` wraps its content in
`MediaQuery.removePadding(removeTop: true)`, which zeroes
`padding.top` and `viewPadding.top` inside the sheet and breaks
`SafeArea` for descendants (like the close button at line 104). The
documented workaround is to restate the outer `MediaQueryData` inside
the builder:

```dart
final outerMediaQuery = MediaQuery.of(context);   // line 47
return showAccessibleModal<bool>(
  context,
  ...
  builder: (_) => MediaQuery(
    data: outerMediaQuery,                         // line 55
    child: const UnifiedCreateModal(),
  ),
);
```

The problem: `outerMediaQuery` is captured **once** at `show()` time —
before the modal opens and before any keyboard is up. It contains
`viewInsets.bottom == 0`. Restating it inside the modal subtree
freezes **all** `MediaQueryData` fields at their open-time values,
not just `padding.top`. So when the user later taps the textarea and
the OS reports a non-zero `viewInsets.bottom`, the modal subtree never
sees the update.

Then [line 64-65](app/lib/presentation/screens/create/unified_create_modal.dart#L64-L65)
reads `viewInsets` from the frozen MediaQuery:

```dart
final media = MediaQuery.of(context);
final viewInsets = media.viewInsets.bottom;        // always 0
```

…and [line 114-118](app/lib/presentation/screens/create/unified_create_modal.dart#L114-L118)
applies it as `AnimatedPadding`:

```dart
padding: EdgeInsets.only(bottom: viewInsets),      // always EdgeInsets.zero
```

Result: the drawer stays anchored to the bottom of the screen and the
keyboard slides over the top of it.

The preview stage at
[unified_create_modal.dart:121-190](app/lib/presentation/screens/create/unified_create_modal.dart#L121-L190)
uses `SingleChildScrollView(reverse: true)` inside the same broken
`AnimatedPadding`, but it doesn't have a TextField so a keyboard
doesn't normally come up there — the bug is masked.

## Potential Solutions

### Option A: Merge live `viewInsets` into the restated MediaQuery

**Approach:** In the `show()` builder, read the current MediaQuery
inside the builder context and merge its live `viewInsets` (and
`viewPadding` for completeness) with the captured outer `padding` so
the modal subtree gets a `MediaQueryData` that has correct top
insets *and* reactive bottom keyboard insets.

```dart
builder: (innerCtx) {
  final innerMq = MediaQuery.of(innerCtx);
  return MediaQuery(
    data: outerMediaQuery.copyWith(
      viewInsets: innerMq.viewInsets,
    ),
    child: const UnifiedCreateModal(),
  );
},
```

- **Pros:** Minimal, surgical, lives in the same `show()` method that
  introduced the workaround. Keeps the existing `AnimatedPadding`
  contract — `media.viewInsets.bottom` becomes the real keyboard
  height, so the existing `padding: EdgeInsets.only(bottom:
  viewInsets)` Just Works. Doesn't touch the drawer's internal
  layout, so the URL tab fix comes free.
- **Cons:** The `builder` closure still uses a captured-at-call-time
  `outerMediaQuery.padding`. If the device rotates while the modal
  is open, top padding stays at the original orientation's value.
  This is the same pre-existing limitation as today and is unrelated
  to #1933.
- **Effort:** Small (≤ 10 lines of code + a test).

### Option B: Replace MediaQuery restatement with a targeted SafeArea fix

**Approach:** Stop restating the entire MediaQuery. Instead, capture
just `outerMediaQuery.padding.top` (or `viewPadding.top`) and pass
that to the close button's `SafeArea` analogue — e.g. wrap the close
button in a `Padding(EdgeInsets.only(top: outerTopInset + 8, ...))`
rather than `SafeArea`. Let the rest of the modal subtree see the
real (unmodified) MediaQuery.

- **Pros:** Removes a fragile workaround. Every MediaQuery read in
  the subtree (current and future) sees correct values automatically.
- **Cons:** Larger blast radius. Any other widget in the modal that
  relies on `SafeArea` or `MediaQuery.padding` would need to be
  re-checked. More risk of follow-up regressions.
- **Effort:** Medium.

### Option C: Make the drawer itself keyboard-aware

**Approach:** Leave `show()` alone. Inside
`UnifiedCreateInputDrawer`, wrap the content in a
`SingleChildScrollView(reverse: true)` and/or read `viewInsets` via
`WidgetsBinding.instance.window` / `View.of(context)` (which bypasses
the restated MediaQuery and goes to the raw window data).

- **Pros:** Localized fix; doesn't touch `show()`.
- **Cons:** Two issues. (1) `View.of(context)` returns physical
  pixels and reading platform-window data directly bypasses Flutter's
  MediaQuery contract — fragile. (2) `SingleChildScrollView(reverse:
  true)` only helps if there *is* a bottom inset to scroll past, and
  the underlying `AnimatedPadding` is still passing 0 — the drawer
  would scroll itself but still render below the keyboard's top edge.
  Doesn't truly fix the bug.
- **Effort:** Small but probably ineffective without also doing A.

## Recommendation

**Option A.** It's the smallest, most targeted change that fixes the
actual root cause, and the existing `AnimatedPadding` infrastructure
becomes correct rather than being replaced. Option B is a better
long-term cleanup but pays for that with broader risk on a
user-reported bug we should ship a fix for quickly. Option C addresses
symptoms, not the cause.

After A lands, Option B is a worthwhile follow-up — but only if we
find another MediaQuery-frozen-value bug. Until then the captured
top-padding behaviour is acceptable (rotating mid-modal is not a
flow we're optimising for).

## Open Questions

- **Does the bug reproduce in the URL tab on iOS?** Confirmed by the
  reporter — yes. Same `_BorderlessGlassField` widget, same drawer,
  same frozen-MediaQuery root cause. The single MediaQuery fix
  resolves both tabs in one change.
- **Should `viewPadding` also be merged in Option A's `copyWith`?**
  `viewPadding` is `padding + viewInsets` semantically (the system
  UI's true insets ignoring keyboard). The pre-existing code didn't
  bother because the only `SafeArea` consumer was for top padding.
  Recommendation: merge `viewInsets` only — leave `viewPadding`
  frozen for symmetry with the existing `padding` freeze. If a
  future use case needs live `viewPadding`, fix it then.
- **Is there an Android equivalent of this bug?** Android keyboard
  insets propagate through the same `MediaQuery.viewInsets.bottom`
  channel and would in principle suffer the same freeze. But the
  reporter explicitly hit it on iOS — possibly because Android's
  default `windowSoftInputMode` resizes the activity differently and
  masks the issue, or because the testing happened on iOS only.
  Recommendation: design the unit test so it doesn't depend on
  platform (see Phase 1 — drive a pure `MediaQueryData` helper) and
  verify on both simulators during Phase 2. The fix applies to both.
- **Should we add a regression check that covers the close-button
  SafeArea path too?** The MediaQuery restatement was added for that
  case originally and our fix changes how the restatement works.
  Recommendation: yes — the same pure helper test should also assert
  `padding.top` comes from the *outer* MediaQuery (the original
  SafeArea fix), so a future refactor can't silently regress it.
- **Should this modal be brought in line with `docs/client/modals.md`
  keyboard-handling standards as part of this fix?** **Yes —
  bundled with the viewInsets fix per the reporter's direction.**
  The viewInsets fix alone would leave the textarea visible but
  still hard to dismiss (iOS multiline keyboards have no Return-key
  Done action). Bundling the full keyboard-standards work — `KeyboardActions`
  + `buildKeyboardActionsConfig` for the multiline text field, `KeyboardDismissWrapper`
  around the input content, `textInputAction: TextInputAction.done`
  on the URL field — produces a "fixed for real" outcome rather
  than a partial patch.

## Implementation Plan

### Phase 1: Fix the keyboard inset propagation

**Goal:** Restore live `viewInsets` to the modal subtree without
losing the SafeArea fix for the status-bar inset.

**Tasks:**
- [ ] Extract a pure helper in
  [unified_create_modal.dart](app/lib/presentation/screens/create/unified_create_modal.dart),
  e.g. `MediaQueryData _mergeForBottomSheet(MediaQueryData outer,
  MediaQueryData inner) => outer.copyWith(viewInsets:
  inner.viewInsets);`. The helper exists so the merge logic is
  unit-testable as a pure function (per `docs/client/testing.md`'s
  "test business logic, not Flutter widgets" rule) and self-documents
  exactly which fields cross the open-time/live boundary.
- [ ] In [unified_create_modal.dart:54-57](app/lib/presentation/screens/create/unified_create_modal.dart#L54-L57),
  change the builder closure to read MediaQuery inside the
  builder context and pipe both `MediaQueryData`s through the helper:
  `MediaQuery(data: _mergeForBottomSheet(outerMediaQuery,
  MediaQuery.of(innerCtx)), child: const UnifiedCreateModal())`.
  Update the comment block at lines 37-46 to explain the merge —
  top padding is captured at open-time (`useSafeArea: false`
  workaround); `viewInsets` flows through live so the keyboard inset
  reaches `AnimatedPadding`.
- [ ] Add a pure unit test under
  `app/test/presentation/screens/create/unified_create_modal_test.dart`
  for the helper: assert that with `outer.padding.top == 47` (a
  typical iPhone status-bar inset) and `inner.viewInsets.bottom ==
  336` (a typical iOS keyboard height), the merged result has
  `padding.top == 47` *and* `viewInsets.bottom == 336`. Also assert
  that `padding.top == 0` on the inner is ignored (locks in the
  original `useSafeArea: false` SafeArea fix). No `ProviderContainer`
  or widget pump needed — the helper is a pure function.
- [ ] Run `npm run test:app` and confirm the new test fails on
  current `main` (catches the bug) before applying the fix, then
  passes after.

**Validates:** A pure unit test proves the merge logic is correct and
locks in both the original SafeArea-restoration intent and the new
keyboard-inset passthrough. Platform-independent and aligned with the
project's ViewModel-first testing philosophy.

### Phase 1.5: Bring the drawer in line with modal keyboard standards

**Goal:** Make the unified-create input drawer fully comply with
`docs/client/modals.md` keyboard-handling patterns so the keyboard
is dismissable on iOS (multiline) and accessibly tappable-outside.

**Tasks:**
- [ ] Convert `UnifiedCreateInputDrawer` from `ConsumerWidget` to
  `ConsumerStatefulWidget` so it can own `FocusNode`s for the two
  text inputs (text-prompt and URL). Dispose them in `dispose()`.
- [ ] Wrap the drawer body in `KeyboardActions(disableScroll: true,
  tapOutsideBehavior: TapOutsideBehavior.translucentDismiss, config:
  buildKeyboardActionsConfig([_textFocusNode, _urlFocusNode]),
  child: KeyboardDismissWrapper(child: ...))`. Pattern matches
  [create_gear_modal.dart:243-246](app/lib/presentation/screens/gear/create_gear_modal.dart#L243-L246).
- [ ] Extend `_BorderlessGlassField` with `focusNode` and
  `textInputAction` named parameters, forwarded to the underlying
  `TextField`. Both default to null/absent to preserve existing
  callers' behaviour.
- [ ] Pass `_textFocusNode` to the Text-tab `_BorderlessGlassField`
  (no `textInputAction` — multiline; the toolbar provides Done).
- [ ] Pass `_urlFocusNode` *and* `textInputAction: TextInputAction.done`
  to the URL-tab `_BorderlessGlassField` (single-line —
  `TextInputAction.done` works because `maxLines == 1`; the toolbar
  is the iOS fallback if the user uses a third-party keyboard).
- [ ] Smoke-build with `cd app && flutter analyze` and
  `flutter test --reporter expanded` to catch any regressions
  from the State-class conversion.

**Validates:** Drawer now matches the documented creation-modal
pattern; iOS multiline textarea has a Done toolbar; URL field has
a Done key on the keyboard; tap-anywhere-outside dismisses focus.

### Phase 2: Manual iOS smoke test

**Goal:** Confirm the bug is gone on the actual reported platform and
configuration.

**Tasks:**
- [ ] Reproduce on iOS simulator on `main` first: launch app, tap
  "+", select Text tab, tap textarea, type — confirm the keyboard
  covers the field and Generate button (per the issue screenshot).
- [ ] Apply the fix locally and repeat — the drawer should animate
  upward to sit immediately above the keyboard, with the textarea
  and Generate button both fully visible.
- [ ] Repeat for the URL tab (single-line field). Confirm same
  behaviour.
- [ ] Repeat on Android emulator to confirm no regression on the
  platform that wasn't reported.

**Validates:** Real-device behaviour matches the test result and the
user's reported symptom is gone.

### Phase 3: Close the loop on #1933

**Goal:** Ship the fix and document the gotcha.

**Tasks:**
- [ ] Open PR referencing #1933. PR description explains the
  MediaQuery-restatement freeze (root cause), the merge fix
  (resolution), and links to this plan.
- [ ] After merge, leave a short comment on the issue with the
  PR/commit link and close.

**Validates:** Issue is closed with a traceable code change and a
future maintainer hitting the same `useSafeArea: false` workaround
elsewhere can find this trail.

## Related Issues

None found directly referencing the unified-create modal or
MediaQuery insets. The unified-create flow itself is gated behind
`Environment.unifiedCreateEnabled`, so this only affects users on
builds where that flag is on — worth noting in the PR but doesn't
change the fix.

## Audit notes (`/ripls-audit`)

Audited 2026-05-15 against `docs/client/architecture.md`,
`docs/client/testing.md`, `docs/client/accessibility.md`,
`docs/client/i18n.md`, and `docs/client/modals.md`.

| Concern | Finding | Resolution in plan |
|---------|---------|--------------------|
| MVVM / Riverpod boundaries | Fix lives in a widget builder closure; no ViewModel or repository change. No business logic moves into widgets. | Compliant — no change. |
| Testing philosophy ("test business logic, not Flutter widgets") | Original Phase 1 proposed a widget test that pumps `UnifiedCreateModal`. Conflicts with the project's ViewModel-first test stance and "widget tests are slower, more brittle, and often test framework behavior rather than app logic" guidance. | **Updated Phase 1** to extract a pure `_mergeForBottomSheet` helper and unit-test it as a pure function — no `ProviderContainer` or widget pump required. |
| Accessibility primitives | Fix doesn't touch the close button (`Tappable` with `context.l10n.a11yClose`), the textarea (`TextField`), or any focus path. No new strings, so no `a11y*` l10n keys needed. | Compliant — no change. |
| i18n | No new user-visible strings. | Compliant — no change. |
| Modal standards (`docs/client/modals.md`) | The drawer currently violates two documented patterns: no `KeyboardDismissWrapper` around the text-input content, and the multiline textarea lacks `KeyboardActions` + `buildKeyboardActionsConfig`. The doc explicitly calls out that iOS ignores `TextInputAction.done` when `maxLines > 1`. These violations don't *cause* #1933 but compound the UX: even after the viewInsets fix, users have a harder-than-needed time dismissing the keyboard. | **Added Open Question** documenting the drift, **added Phase 3 task** to file a follow-up issue. Kept out of scope for this PR to limit blast radius. |
| Caching / observability / server boundary | None applicable — pure client-side rendering fix. | N/A. |
