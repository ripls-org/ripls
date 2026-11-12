# Issue #991: Request should let helper withdraw help from pill menu

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/991
**Filed:** 2026-04-01 | **Last activity:** 2026-04-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a helper has offered to fulfill a request, the "Helping" pill in the
request content view (the full-screen request detail page) has no way to
withdraw. Tapping the sage "Helping" pill simply navigates to the Chat tab
instead of opening a dropdown menu with a "Withdraw Offer" option.

The backend RPC (`WithdrawOffer`), the request view model method
(`withdrawOffer()`), and the conversation-screen action bar all already
support withdrawal. The gap is solely in the request content view's action
bar for non-owner helpers.

## Context from Comments

No comments on the issue. The description is clear and the feature gap is
well-defined.

## Root Cause

In `app/lib/presentation/screens/request/request_content_view.dart`, the
`_buildRequestActions()` method (line 464) has two code paths:

1. **Non-owner, active request (line 464):** Uses `_OfferButton` — a plain
   button with no dropdown. When `isCurrentUserHelping` is true, tapping
   navigates to the Chat tab via `_onTabChanged(2, state)`.

2. **Owner or terminal (line 555):** Uses `WorkflowActionButtons.buildRequest()`
   which renders a `MorphingActionButton` with a dropdown menu.

The non-owner path never passes through the `MorphingActionButton`/dropdown
system, so there's no menu and no way to withdraw.

By contrast, the conversation screen's action bar (`conversation_action_button.dart`
line 270) correctly shows a "Helping" sage pill with a "Withdraw Offer" dropdown
item for non-owner helpers. Giveaway and loan content views also correctly handle
the non-owner withdrawal case.

## Potential Solutions

### Option A: Unify non-owner path through WorkflowActionButtons.buildRequest

- **Approach:** Remove the early-return `_OfferButton` code path for non-owners
  (lines 464-478). Instead, build a helper menu with "Withdraw Offer" and fall
  through to the existing `WorkflowActionButtons.buildRequest()` call at
  line 558, passing the helper menu items instead of empty `ownerMenuItems`.
- **Pros:** Uses the existing `MorphingActionButton` pattern already proven in
  giveaway/loan/experience workflows. Consistent pill appearance across content
  view and conversation screen. Minimal code — removes ~15 lines and adds ~10.
- **Cons:** None significant. This is the established pattern.
- **Effort:** Small.

### Option B: Add a dropdown to the existing _OfferButton widget

- **Approach:** Modify `_OfferButton` to accept menu items and show a dropdown
  when tapped in the "helping" state.
- **Pros:** Smaller diff.
- **Cons:** Duplicates dropdown logic already in `MorphingActionButton`.
  Diverges from the pattern used by every other workflow type. Creates a
  maintenance burden.
- **Effort:** Small, but adds tech debt.

## Recommendation

**Option A.** The `WorkflowActionButtons.buildRequest()` + `MorphingActionButton`
pattern is already used for the owner path in this same method, and for
non-owners in giveaway and loan content views. Reusing it for the request
non-owner path is the natural fix.

## Open Questions

- **Should tapping the "Helping" pill still navigate to Chat when there's a
  menu?** — Recommendation: No. The pill should open the dropdown menu (which
  could include a "Go to Chat" item if desired, but the primary purpose is
  the manage menu). This matches giveaway/loan behavior where the sage pill
  opens a menu, not a nav action. The Chat tab is already available via the
  tab bar.

## Implementation Plan

### Phase 1: Add withdraw to request content view pill menu

**Goal:** Helper can withdraw offer from the request content view's "Helping" pill.

**Tasks:**
- [ ] In `request_content_view.dart` `_buildRequestActions()`: remove the
      early-return non-owner block (lines 464-478) that uses `_OfferButton`
- [ ] Build a `helperMenuItems` list for non-owner helpers containing a
      "Withdraw Offer" `ActionDropdownItem` (icon: `Icons.undo_outlined`,
      `isDestructive: true`). Use `context.l10n.requestWithdraw` for the label
      (the key already exists in both `app_en.arb` and `app_es.arb`)
- [ ] Add a `_handleWithdrawOffer()` method to `RequestContentView` that calls
      `ref.read(requestProvider(widget.requestId).notifier).withdrawOffer()`
      with success/error toasts (matching `request_management_card.dart:906`)
- [ ] In the existing `WorkflowActionButtons.buildRequest()` call (line 558),
      use `helperMenuItems` when `!state.isOwner` instead of `ownerMenuItems`
- [ ] Set `onConfirmedTap` to `null` for non-owner helpers (pill opens dropdown
      instead of navigating to chat)
- [ ] Delete the `_OfferButton` widget class — it is only referenced in the
      early-return block being removed and `MorphingActionButton` renders its own
      pre-action state internally via `preActionLabel`

**Validates:** Helper taps "Helping" pill → dropdown opens with "Withdraw Offer"
→ tapping it withdraws the offer and shows a success toast. Pill morphs back to
"I'll Help" coral CTA.

## Related Issues

None referenced.

---

## Audit

**Audited:** 2026-04-02

**Changes made:**

1. **i18n compliance (i18n.md):** The original plan used a hardcoded `'Withdraw Offer'`
   label string. Updated to use `context.l10n.requestWithdraw` — the key already
   exists in both ARB files. Note: `conversation_action_button.dart:241` has the
   same hardcoded string but that is out of scope for this issue.

2. **Dead code cleanup:** The original plan said `_OfferButton` "can remain" because
   it's "used for the pre-offer CTA state via the MorphingActionButton onAction
   path." This is incorrect — `MorphingActionButton` renders its own pre-action
   button via `preActionLabel`. `_OfferButton` is only used at line 468 inside
   the block being removed, making it dead code. Updated to delete it.

**No issues found with:**
- Architecture layering (Screen → ViewModel → Repository pattern followed correctly)
- Caching (withdrawal mutation goes through `requestRepository.withdrawOffer()` which
  handles cache invalidation)
- Server/client boundary (no server changes needed; `WithdrawOffer` RPC exists)
