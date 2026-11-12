# Issue #924: [Bug Report] conversation thread on wrong item

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/924
**Filed:** 2026-03-27 | **Last activity:** 2026-03-27
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** docs/client/content.md, docs/client/chat.md, docs/ai/content4.md

## Problem Statement

When a user has the Chat tab selected on a content view and scrolls through the feed from one item to the next, the conversation from the first item bleeds into the second item's Chat tab. The user sees the wrong conversation thread displayed under a different item.

This is a state management bug caused by the `conversationProvider` being a global singleton rather than a per-conversation parameterized provider. When the feed scrolls to a new item, the Chat tab's `InlineConversationView` either reuses stale state from the previous item or fails to reinitialize for the new conversation ID.

## Context from Comments

No comments on this issue. The bug report is straightforward from user feedback — the description and screenshot are sufficient to reproduce.

## Root Cause Analysis

Three interacting issues cause this bug:

### 1. Global `conversationProvider` (primary cause)

`conversation_view_model.dart:2862` defines the provider as a non-parameterized singleton:

```dart
final conversationProvider =
    NotifierProvider.autoDispose<ConversationNotifier, ConversationState>(
      ConversationNotifier.new,
    );
```

Every content view in the feed shares the same provider instance. When Item A initializes the conversation, Item B reads that same state. In contrast, `gearProvider`, `requestProvider`, and `experienceProvider` are all `.family` providers parameterized by ID — the conversation provider should follow the same pattern.

### 2. `_initialized` flag not reset on conversation ID change

`inline_conversation_view.dart:76-81`:

```dart
void didUpdateWidget(InlineConversationView old) {
  super.didUpdateWidget(old);
  if (widget.isActive && !_initialized) {
    _initialize();
  }
}
```

When the widget updates with a new `conversationId`, `didUpdateWidget` only checks `!_initialized`. If already initialized from Item A, it skips initialization for Item B entirely.

### 3. Missing `ValueKey` on `InlineConversationView`

All three content views render `InlineConversationView` without a `key`:

- `gear_content_view.dart:776`
- `experience_content_view.dart:793`
- `request_content_view.dart:675`

Without a key tied to `conversationId`, Flutter reuses the stateful widget across different items in the feed, preserving stale `_initialized = true` and the `AutomaticKeepAliveClientMixin` state.

## Potential Solutions

### Option A: Add ValueKey + reset `_initialized` on conversation ID change

- **Approach:** Add `key: ValueKey(conversationId)` to every `InlineConversationView` instantiation. Also update `didUpdateWidget` to detect when `conversationId` changes and reset `_initialized`.
- **Pros:** Minimal change. Forces Flutter to create a fresh widget state when the conversation ID changes.
- **Cons:** The global `conversationProvider` is still shared — two items visible simultaneously (during a scroll transition) could still conflict. The key forces a dispose/recreate cycle, but a brief window of state collision remains.
- **Effort:** Small.

### Option B: Parameterize `conversationProvider` as a `.family` provider

- **Approach:** Change `conversationProvider` to `NotifierProvider.autoDispose.family<ConversationNotifier, ConversationState, String>` keyed by conversation ID. Update all consumers to pass the conversation ID when reading/watching. Combine with `ValueKey` on the widget.
- **Pros:** Correct architectural fix. Each conversation gets its own isolated state. Matches the pattern used by gear/request/experience providers. Eliminates the state collision window entirely.
- **Cons:** Larger change — every site that reads `conversationProvider` must be updated to pass the conversation ID. This includes `InlineConversationView`, `ConversationScreen`, and any other consumers.
- **Effort:** Medium.

### Option C: ValueKey only (simplest fix)

- **Approach:** Add `key: ValueKey(conversationId)` to `InlineConversationView` in all three content views. Do not change the provider or `didUpdateWidget`.
- **Pros:** One-line change per content view. The `ValueKey` forces Flutter to dispose the old widget and create a new one when the conversation ID changes, which disposes the old provider subscription and starts fresh.
- **Cons:** Relies on Flutter's widget lifecycle for correctness rather than fixing the underlying architectural issue. The global provider remains a latent risk.
- **Effort:** Small.

## Recommendation

**Option B** (parameterize `conversationProvider` + add `ValueKey`) is the correct fix. The global provider is architecturally wrong — it was likely an oversight from when the conversation view was only used in `ConversationScreen` (one at a time) and not embedded inline in feed items. Making it a `.family` provider aligns with the existing pattern for all other content providers and eliminates the class of bugs entirely.

However, **Option C** (ValueKey only) is a pragmatic quick fix if Option B is too large for now. The `ValueKey` forces widget disposal/recreation on ID change, which triggers `autoDispose` on the provider, effectively resetting state. This works because only one `InlineConversationView` can be active at a time in the feed.

## Open Questions (Resolved)

- **Is `ConversationScreen` (standalone) affected?** — **No.** Only one instance exists at a time since it navigates away from the current screen. Phase 2's `.family` conversion will make it safer for future use cases regardless.
- **Are there other consumers of `conversationProvider` beyond `InlineConversationView` and `ConversationScreen`?** — Grep for all `conversationProvider` references to ensure all sites are updated in Phase 2.
- **Should the `AutomaticKeepAliveClientMixin` be removed from `InlineConversationView`?** — **Keep it.** The mixin preserves state when switching between tabs (Loan/Details/Chat) within a single item. The bug is about switching between items, not tabs. The `ValueKey` fix handles item switches correctly by forcing a new widget instance per conversation ID.

## Implementation Plan

### Phase 1: Quick fix with ValueKey
**Goal:** Stop the bug immediately with minimal risk.
**Tasks:**
- [ ] Add `key: ValueKey(gear.conversationId)` to `InlineConversationView` in `gear_content_view.dart` (use the local variable, not `widget.*`)
- [ ] Add the equivalent `ValueKey` in `experience_content_view.dart` and `request_content_view.dart` (use each view's local conversation ID variable)
- [ ] Add a defensive `didUpdateWidget` guard in `InlineConversationView`: if `widget.conversationId != old.conversationId`, reset `_initialized = false` and re-initialize. This guard is redundant when `ValueKey` is present (Flutter disposes and recreates the widget on key change, so `didUpdateWidget` never sees a different ID), but it protects against callers that forget the key.
- [ ] Test: scroll between items with Chat tab active — verify correct conversation loads
**Validates:** The bug is fixed for the user's reported scenario.

> **Audit note (architecture.md — async safety):** `InlineConversationView._initialize()` is an async method called from `initState` via `addPostFrameCallback` and from `didUpdateWidget`. Verify that every `await` in `_initialize()` is followed by a `mounted` check before calling `setState` or accessing `ref`. The existing code already does this (`if (!mounted) return` at line 115), so no changes needed — just confirm during implementation.

### Phase 2: Parameterize conversationProvider (architectural fix)
**Goal:** Eliminate the root cause so the global provider can never cause state collisions.
**Tasks:**
- [ ] Change `conversationProvider` to `.family` parameterized by conversation ID:
  ```dart
  final conversationProvider = NotifierProvider.autoDispose
      .family<ConversationNotifier, ConversationState, String>(
    ConversationNotifier.new,
  );
  ```
- [ ] Update `ConversationNotifier` to access the conversation ID via the family `arg` parameter. Evaluate whether `initialize()` can fold the conversation ID from `arg` instead of accepting it as a parameter (reducing the initialize-pattern surface).
- [ ] Update `InlineConversationView` to use `ref.watch(conversationProvider(widget.conversationId))`
- [ ] Update `ConversationScreen` to use `ref.watch(conversationProvider(conversationId))`
- [ ] Grep for **all** `conversationProvider` references (including `.notifier`, `.select`, and any `ref.listen` calls) and update them to pass the conversation ID
- [ ] Update existing conversation view model tests for the new family provider signature — ensure disposal safety tests still pass (per architecture.md: "All async Notifiers must have disposal tests")
- [ ] Run full test suite: `flutter test --reporter expanded`
**Validates:** Multiple conversations can coexist in memory without state collisions.

> **Audit note (architecture.md — provider patterns):** The `.family` conversion aligns with the architecture recommendation to "consider family providers for parameterized state." The other content providers (`gearProvider`, `requestProvider`, `experienceProvider`) already use `.family`, so this brings `conversationProvider` into consistency. The `autoDispose` modifier is already present, which is correct for screen-scoped state per architecture.md.

## Audit Notes

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`

1. **No caching issues.** This bug is purely a provider/widget lifecycle issue. No cache keys, repository patterns, or cache invalidation are involved.
2. **No i18n impact.** No new user-visible strings are introduced by either phase.
3. **No server changes.** The bug is entirely client-side state management. Server architecture is unaffected.
4. **Async safety verified.** The existing `_initialize()` method in `InlineConversationView` already has `mounted` checks after async gaps (line 115). Phase 2's `.family` conversion must preserve `ref.mounted` checks in `ConversationNotifier` async methods — these already exist in the current code.
5. **`InlineConversationView` accesses `chatRepositoryProvider` directly (lines 107-113).** This violates the architecture rule "Screens only interact with ViewModels" (architecture.md). However, this is a pre-existing issue and out of scope for this bug fix. The direct repository access feeds data into the `ConversationNotifier.initialize()` call, which is the ViewModel layer. Fixing this layering violation is a separate concern.

## Related Issues

- No directly related issues identified.
