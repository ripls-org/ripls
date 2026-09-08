---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client undo pattern — reversing destructive actions via ToastHelper.showUndo's floating SnackBar, why it replaces raw SnackBar inside an IndexedStack, the optimistic-state viewmodel pattern, and margin handling.
  globs: [app/lib/core/utils/toast_helper.dart, app/lib/presentation/screens/portfolio/**, app/lib/presentation/viewmodels/home_tab_view_model.dart, app/lib/presentation/widgets/needs/needs_actions.dart]
  triggers: [undo, snackbar, toast-helper, optimistic, swipe-to-dismiss, indexedstack]
  lens: [client]
  domain: client
freshness:
  verified_commit: "1774bf648"
  verified_on: "2026-07-22"
---
# Undo Pattern

## Overview

Undo allows users to reverse a destructive action — most commonly a swipe-to-dismiss — via a floating SnackBar with an "Undo" button that auto-dismisses after 4 seconds if not acted on.

The canonical example is the Home tab's **Needs you** queue: accepting or
completing a decision optimistically removes its card and offers an Undo — see
[home_decision_routing.dart](../../app/lib/presentation/screens/portfolio/home_decision_routing.dart)
(`ToastHelper.showServerUndo`) and
[home_tab_view_model.dart](../../app/lib/presentation/viewmodels/home_tab_view_model.dart)
(`acceptDecision` / `undoAcceptDecision`).

---

## How to Use

Call `ToastHelper.showUndo` from [toast_helper.dart](../../app/lib/core/utils/toast_helper.dart) after performing the optimistic mutation:

1. Apply the optimistic state change immediately (hide the item, remove the row, etc.).
2. Fire the server call in the background (fire-and-forget or awaited — either works).
3. Call `ToastHelper.showUndo` with the message, undo label, and an `onUndo` callback.
4. In `onUndo`, reverse the optimistic state and re-issue the server call to restore the item.

---

## Why `ToastHelper.showUndo` Instead of `SnackBar` Directly

Flutter's built-in SnackBar auto-dismiss timer only starts after the show animation reaches `AnimationStatus.completed`. In an `IndexedStack` (used by `HomeScreen` to host the bottom-nav tabs), non-active tabs are wrapped in `TickerMode(enabled: false)`. Any `Scaffold` registered from within those tabs has its animation tickers muted, so the show animation never completes and the timer never starts — the snackbar hangs indefinitely.

`ToastHelper.showUndo` works around this by:

- Setting `duration: Duration(days: 365)` on the `SnackBar` itself, disabling Flutter's internal timer entirely.
- Dismissing via `Future.delayed` + `ScaffoldMessengerState.hideCurrentSnackBar`, which is independent of ticker state.
- Capturing the `ScaffoldMessenger` reference before the delay, so it stays valid even if the calling widget unmounts.

Do **not** use `ScaffoldMessenger.of(context).showSnackBar` directly for undo toasts in screens that live inside an `IndexedStack`. Always go through `ToastHelper.showUndo`.

---

## Optimistic State

Undo only works cleanly when the mutation is applied optimistically — the item disappears from the UI immediately, before the server responds. If you wait for the server before updating state, the row is still visible when the SnackBar appears and the interaction feels broken.

The Home tab viewmodel is the model to follow:

- A `removedDecisionIds` set is added to the Freezed state (`HomeTabState`).
- The derived getter (`visibleDecisions`) filters out removed IDs when building the display list.
- On undo, the ID is removed from the set and the card reappears immediately.
- On the next server refresh, `_prunedRemovals` keeps a removed ID hidden only while the server still returns it, so ground truth takes over without a one-by-one flicker.

See [home_tab_view_model.dart](../../app/lib/presentation/viewmodels/home_tab_view_model.dart) for the full implementation: `acceptDecision`, `undoAcceptDecision`, and the `removedDecisionIds` field on `HomeTabState`.

---

## Margin

The `margin` parameter on `ToastHelper.showUndo` controls where the floating SnackBar sits. Screens with a `FloatingHeader` bottom bar should offset the bottom margin to sit above it:

```
bottom: FloatingHeader.contentBottom(context) - 20
```

See [floating_header.dart](../../app/lib/presentation/widgets/floating_header.dart) for `contentBottom`.

Screens without a floating bottom bar can omit `margin` and accept the default (`16px` horizontal, `12px` vertical).

---

## Key Files

| File | Role |
|------|------|
| [toast_helper.dart](../../app/lib/core/utils/toast_helper.dart) | `ToastHelper.showUndo` / `showServerUndo` — the shared undo SnackBar helpers |
| [home_decision_routing.dart](../../app/lib/presentation/screens/portfolio/home_decision_routing.dart) | Home "Needs you" undo caller — `ToastHelper.showServerUndo` |
| [home_tab_view_model.dart](../../app/lib/presentation/viewmodels/home_tab_view_model.dart) | Optimistic decision removal + undo state (`removedDecisionIds`) |
| [needs_actions.dart](../../app/lib/presentation/widgets/needs/needs_actions.dart) | `ToastHelper.showUndo` caller — gear-offer undo |
| [floating_header.dart](../../app/lib/presentation/widgets/floating_header.dart) | `contentBottom` for margin calculation |
