# Issue #1026: @ mentions are not working

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1026
**Filed:** 2026-04-04 | **Last activity:** 2026-04-04
**Labels:** bug, user-feedback | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

The @-mention feature (originally shipped in PR #611) is not functional in the
primary chat UI — the Chat tab of item detail screens (gear, request,
experience). These screens use `InlineConversationView` for chat, which uses a
plain `TextField` instead of the `MentionableTextField` that provides the
autocomplete overlay. Additionally, the `MessageList` in `InlineConversationView`
does not pass an `onMentionTap` callback, so even if someone sends an encoded
mention from the old `ConversationScreen`, the chips render but are not tappable.

The `ConversationScreen` (opened from the inbox) still has full @-mention
support — it uses `MessageInputBar` which conditionally renders
`MentionableTextField`. But the item-detail chat tab is now the primary chat
surface, so mentions are effectively broken for most users.

### Root cause summary

Two gaps in `InlineConversationView` (`app/lib/presentation/widgets/content/inline_conversation_view.dart`):

1. **Input**: Both `_buildSimpleInput` (line 239) and `_buildPillInput` (line
   375) use a plain `TextField` with `_inputController` (a
   `MentionTextController`). There is no `MentionableTextField`, so typing `@`
   does not trigger the suggestion overlay.
2. **Display**: The `MessageList` at line 484 does not receive `onMentionTap`,
   so mention chips in received messages are not tappable.

## Context from Comments

One comment from the maintainer (maintainer):

> PR #611 added at-mentions for people and gear in chats, but we don't see
> them anymore. There is a new item-oriented chat interface, and it's possible
> they just never got ported over. We should bring them into the new chat
> interface while respecting its new aesthetics.

Key takeaway: the mention feature must integrate with the dark, pill-style
aesthetic of the inline chat pane — not just be dropped in from the old
`MessageInputBar`.

## Potential Solutions

### Option A: Wire MentionableTextField into InlineConversationView

- **Approach:** Replace the plain `TextField` in `_buildPillInput` (expanded
  mode) with `MentionableTextField`. Pass `onFetchMentionSuggestions` from the
  conversation view model and a `mediaUrlLoader`. Pass `onMentionTap` to
  `MessageList`. Leave collapsed mode (`_buildSimpleInput`) as-is since it
  auto-expands on tap/focus.
- **Pros:** Reuses all existing mention infrastructure (controller, overlay,
  suggestion converter, chip rendering). Minimal new code. The overlay already
  positions itself via `CompositedTransformFollower` which works in any layout.
- **Cons:** The `MentionSuggestionOverlay` was designed for the old light-theme
  chat. Its colors and styling may need adjustment for the dark immersive
  content-view background. The overlay renders via Flutter's `Overlay` so it
  won't be clipped by the pill container, but its visual style may clash.
- **Effort:** Small.

### Option B: Build a new inline mention experience

- **Approach:** Instead of the popup overlay, build a new mention suggestion
  widget that renders inline within the chat tab (e.g., a suggestion row between
  the message list and input bar).
- **Pros:** Could feel more native to the content-view layout. No overlay
  clipping concerns.
- **Cons:** Significant new widget code. Duplicates logic already in
  `MentionableTextField`. Higher effort for the same functionality.
- **Effort:** Medium–large.

### Option C: Redirect to ConversationScreen for typing

- **Approach:** When the user taps the input field or types `@`, open the full
  `ConversationScreen` which already has mentions working.
- **Pros:** Zero new mention UI code.
- **Cons:** Terrible UX — users would lose their context in the item detail
  screen just to type a mention. Does not fix mention display.
- **Effort:** Small but poor outcome.

## Recommendation

**Option A.** The existing `MentionableTextField` and overlay infrastructure is
well-tested and fully functional in `ConversationScreen`. Wiring it into
`InlineConversationView` is straightforward. The overlay styling may need minor
tweaks to harmonize with the dark theme, but the `MentionSuggestionOverlay`
already accepts enough parameters to adjust. This is the right balance of
effort and quality.

## Open Questions

- **Should collapsed mode also support mentions?** — Recommendation: No. The
  collapsed `_buildSimpleInput` auto-expands on focus, so the user will always
  be in expanded mode when typing. Adding mentions to collapsed mode adds
  complexity for no benefit.
- **Does the overlay need dark-theme styling?** — Recommendation: Yes, but
  this can be a fast follow. The overlay uses `Material` and `Container` with
  explicit colors. Passing a brightness flag or checking `Theme.of(context)`
  should suffice. If it looks acceptable as-is against the dark background,
  skip it entirely.
- **Should mention taps navigate away from the item screen?** — Recommendation:
  Yes, using the same navigation as `ConversationScreen._handleMentionTap`.
  Push the user/gear/request/experience screen. The user can swipe back. This
  is consistent behavior.

## Implementation Plan

### Phase 1: Wire mention autocomplete into expanded input
**Goal:** Typing `@` in the expanded pill input shows the suggestion overlay and
inserting a mention encodes it correctly.
**Tasks:**
- [ ] Import `MentionableTextField`, `MentionSuggestion`, and `MentionTypes` in `inline_conversation_view.dart`
- [ ] In `_buildPillInput`, replace the plain `TextField` (line 375) with `MentionableTextField`, passing `_inputController`, `_inputFocusNode`, and an `onFetchSuggestions` callback that calls `conversationProvider.notifier.fetchMentionSuggestions`
- [ ] Pass a `mediaUrlLoader` using `mediaRepositoryProvider` (same pattern as `ConversationScreen`)
- [ ] Preserve existing pill styling by passing `decoration` and `style` params to `MentionableTextField`
- [ ] Verify mention encoding works end-to-end: type `@`, select suggestion, send message, confirm `@[type:id:name]` appears in server logs
**Validates:** Core mention input works in the inline chat.

### Phase 2: Wire mention tap handling in message display
**Goal:** Mention chips in messages are tappable and navigate to the mentioned entity.
**Tasks:**
- [ ] Add an `onMentionTap` callback to `InlineConversationView` (or handle internally with navigation helpers)
- [ ] Pass `onMentionTap` to the `MessageList` widget (line 484)
- [ ] Implement navigation for each `MentionType` (user → UserScreen, loan/giveaway → GearScreen, request → RequestScreen, experience → ExperienceScreen) using `NavigationHelpers.pushScreen`
**Validates:** Tapping a mention chip navigates to the correct screen.

### Phase 3: Theme harmonization (if needed)
**Goal:** Mention overlay and chips look good against the dark immersive background.
**Tasks:**
- [ ] Test overlay appearance on dark background; if it looks off, add dark-mode color overrides to `MentionSuggestionOverlay`
- [ ] Verify `MentionChip` colors are legible on dark message bubbles (the `onDarkBackground` flag is already passed as `false` in `MessageList` — this may need to be `true` for immersive mode)
- [ ] Adjust `MentionableTextField` hint text styling if it clashes with the pill container's aesthetic
**Validates:** Mentions look native to the item-detail chat tab.

## Architecture Audit

Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md`.

**Compliant:**
- Suggestion fetching routes through ViewModel (`conversationProvider.notifier.fetchMentionSuggestions`), not directly to a service or repository from the widget.
- Navigation uses `NavigationHelpers.pushScreen` (not raw `Navigator.push`). Target screens already have `SwipeToCloseMixin`.
- Dark-theme overlay uses `Theme` wrapper so `AppColors.xxx(context)` resolves to dark variants — no hardcoded colors in the overlay.
- `MentionChip.onDarkBackground` is set per-bubble based on immersive mode + isCurrentUser, matching the bubble text color logic.
- No server changes needed — existing `server/chat/mentions.go` parsing and `mention_handler.go` processing are unchanged.
- Caching is unaffected — search results flow through `SearchRepository` which uses `CacheService`.

**Known deviations (pre-existing, not introduced by this change):**
- `mediaUrlLoader` accesses `mediaRepositoryProvider` directly from the widget rather than routing through the ViewModel. This matches the identical pattern in `ConversationScreen` (line 444-448). Fixing would require adding a `getMediaUrl` pass-through on the ViewModel, which is out of scope for this bug fix.
- Hint text `"Type '@' to mention things"` is a hardcoded English string. Per i18n guidelines, it should use `context.l10n`. The same string is already hardcoded in `MessageInputBar` (line 505). Both should be moved to ARB files in a follow-up i18n pass.

## Related Issues

- PR #611 — Original @-mention implementation (merged 2026-01-20)
