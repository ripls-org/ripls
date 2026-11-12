# Issue #1699: No UI to change cover photo for gear, request, or experience after creation

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1699
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A user who created a gear, request, or experience cannot change the cover
photo afterward. Title, description, and other metadata are all editable
through the "Managing" → "Set Details" flow on each content screen, but the
cover photo is not — there is no pencil affordance, no menu item, and no tap
target on the image itself.

The `MediaPickerButton` overlay does exist in edit mode on all three screens,
but the call site is gated on `state.mediaPath == null`. The first photo a
user picks during creation latches the picker into the "hidden" state forever
after, even though the underlying `MediaPickerButton` widget already supports
a "Replace background" mode (see
`app/lib/presentation/widgets/media/media_picker_button.dart:21`).

**Two separate categories of "other types" matter here:**

1. **Communities and profiles already work and need no change.** They use
   dedicated edit screens (`community_edit_screen.dart:131-200`,
   `profile_edit_screen.dart:180-196`) with a tappable image and pencil
   overlay opening a Gallery/Camera sheet. They are the working baseline
   the gear/request/experience screens fail to match.
2. **Gear, request, and experience all share the same broken gate.** The
   identical `state.isEditing && state.mediaPath == null` check exists at
   `gear_content_view.dart:384`, `request_content_view.dart:468`, and
   `experience_content_view.dart:491`. These are inline-edit screens that
   share a common pattern, distinct from community/profile's dedicated
   edit screens. All three are equally un-editable today and all three
   are in scope for this fix.

The product direction is **not** to copy the community/profile small-pencil
UI pixel-for-pixel — it is to make the inline-edit screens let owners
change the cover. The existing centered "Replace background" button
(`MediaPickerButton` in `hasMedia: true` mode) is the chosen affordance.

A loosely related affordance lives in the Chat tab: the "Replace Media" sheet
(`gear_content_view.dart:663`) lets the owner add/delete/reorder media via
`_showGearCarouselMediaPicker`, and the viewmodel's `_setMediaFile` does set
`mediaPath` to the newly added item, so functionally a new photo *does* become
the cover. But (a) it's a multi-media carousel, not a "change cover"
affordance, (b) it's tucked into the chat tab with conversation media, and (c)
discoverable intent is "add", not "change". The reporter of #1697 hit exactly
this — they added a real photo expecting the AI image to be replaced and were
surprised when the AI still showed.

## Context from Comments

The issue was filed today after duplicate triage of #1697 ("Ability to use
second photo for Susan's loan"). After initial filing focused on gear, the
issue was broadened to cover gear, request, and experience together, since
all three share the same broken gate. The investigation established:

- The gap is real on all three inline-edit screens; it is not a regression.
  Git history shows photo-edit UI was never built behind the gate on any
  of them.
- Communities and profiles already work via a different (dedicated edit
  screen) pattern. Their existence proves the bug is inline-edit-specific,
  not app-wide; they themselves need no change.
- User direction: use the existing `MediaPickerButton` "Replace background"
  state (the centered button) rather than introducing a new pencil overlay.
  The existing button is already wired for the case — only the guard is
  blocking it.

## Recommendation

**Drop the `state.mediaPath == null` guard in all three content views.** The
existing `MediaPickerButton` already flips to `Icons.edit` and "Replace
background" copy when `hasMedia: true`, so the lift is one line per file
(`gear_content_view.dart:384`, `request_content_view.dart:468`,
`experience_content_view.dart:491`). All three inline-edit screens are in
scope per user direction; the shared gate makes them a single coherent fix
rather than three separate ones.

User direction explicitly chose this over a community/profile-style pencil
overlay: the existing centered button is the affordance, no new widget
needed.

The new media flows through the existing `MediaPickerDialog` →
`pickImageFromGallery` / `pickImageFromCamera` / `pickVideoFromGallery` →
`_setMediaFile` path, which already promotes the newly picked item to the
cover by setting `mediaPath`/`mediaId` and calling `saveChanges` — no
viewmodel changes needed.

### Audit findings (2026-05-04)

The audit surfaced two issues the implementation must address; the
Implementation Plan below incorporates them.

1. **Pre-existing i18n violations exposed by the fix.** Today the
   `MediaPickerButton` visible text label
   (`media_picker_button.dart:21,59`: `'Replace background'` /
   `'Select a photo or video'`) and `MediaPickerDialog` titles and list
   tiles (`media_picker_dialog.dart:22,31,39,47`: `'Replace Media'`,
   `'Add Media'`, `'Videos'`, `'Photos'`, `'Camera'`) are hardcoded
   English literals. The semantic labels are correctly l10n'd
   (`a11yMediaReplaceBackground` / `a11yMediaSelectMedia` exist in both
   `app_en.arb` and `app_es.arb`), but the visible `Text(...)` widgets
   are not. These currently surface only during item creation when no
   photo exists; once the gate drops, they will surface to existing-cover
   edits as well, including for Spanish-locale users. Per
   `docs/client/i18n.md` ("Strings belong in ARB files, not Dart code"),
   this PR must l10n the visible labels — not as scope creep but because
   the fix promotes a latent bug into a visible one.

2. **Testing layer alignment.** `docs/client/testing.md` is explicit:
   "Test ViewModels, not widgets." The visibility decision in this PR is
   pure widget-level boolean composition (`state.isEditing &&
   state.isOwner`), so a small widget smoke test is justified, but the
   plan should not stop there. The cleaner alternative is to lift the
   decision into a derived getter on each state class
   (e.g. `bool get canEditCoverPhoto => isEditing && isOwner`) and unit
   test the getter at the viewmodel layer with `ProviderContainer` and
   mocked repositories — matching the MVVM testing pattern documented in
   `docs/client/testing.md`. The widget then reads
   `state.canEditCoverPhoto`, and the widget test is reduced to a thin
   assertion that the bool is honored by the rendered tree.

No changes needed for caching, server boundaries, or observability —
none of those are touched by this fix. Accessibility primitives in
`MediaPickerButton` (`Tappable` + `semanticsLabel` from `context.l10n`)
and `MediaPickerDialog` (`showAccessibleModal`) already comply with
`docs/client/accessibility.md`; no new a11y violations are introduced.

## Open Questions

- **Should the picker be available to non-owners who reach edit mode through
  some indirect path?** —
  Recommendation: **no — gate on `state.isOwner` alongside
  `state.isEditing`**. The existing edit-mode entry points already require
  ownership, so this is just defensive against future code paths that might
  not.

- **Should "change cover" replace the existing media item or just add a new
  one (latest-wins for `mediaPath`)?** —
  Recommendation: **add a new one**, matching today's `_setMediaFile`
  behavior. This is the path of least resistance — zero viewmodel changes,
  preserves the previous image in the carousel so the user can flip back via
  reorder, and stays consistent with the chat-tab "Replace Media" flow. If a
  user wants to drop the old image, they can delete it from the carousel.

- **Does dropping the guard cause the centered `MediaPickerButton` to
  collide with anything in the populated-cover case?** —
  Recommendation: **verify on device before merging.** The button renders
  centered, ~55% up the screen, on top of the cover. With a populated cover
  it's now a bright "Replace background" call-to-action floating over an
  image. That's the intended UX for option A — but it's worth a manual look
  to confirm it doesn't fight with the edit bar, info button, or tab bar.
  If it does, we can shift positioning, but anything beyond that is
  out-of-scope creep into option B.

## Implementation Plan

### Phase 1: Localize visible strings on the picker widgets
**Goal:** Bring `MediaPickerButton` and `MediaPickerDialog` visible text
into compliance with `docs/client/i18n.md` *before* exposing them to
populated-cover edits, so non-English users don't see English labels
the moment Phase 2 ships.
**Tasks:**
- [ ] Add ARB keys to both `app_en.arb` and `app_es.arb` for the
      visible `MediaPickerButton` labels (current literals: "Replace
      background", "Select a photo or video"). The existing
      `a11yMediaReplaceBackground` / `a11yMediaSelectMedia` keys are
      a11y-only — visible-text keys are separate. Suggested keys:
      `mediaPickerButtonReplace` / `mediaPickerButtonSelect` per the
      `gear*` / `loan*` / `mediaPickerButton*` naming convention in
      `docs/client/i18n.md`.
- [ ] Add ARB keys for `MediaPickerDialog` strings: title (`"Replace
      Media"` / `"Add Media"`) and list tiles (`"Videos"`, `"Photos"`,
      `"Camera"`). Suggested: `mediaPickerDialogTitleReplace`,
      `mediaPickerDialogTitleAdd`, `mediaPickerDialogVideos`,
      `mediaPickerDialogPhotos`, `mediaPickerDialogCamera`.
- [ ] Replace the hardcoded literals in
      `media_picker_button.dart:21,59` and
      `media_picker_dialog.dart:22,31,39,47` with `context.l10n.*`
      lookups.
- [ ] Run `npm run generate` and `npm run lint:dart:i18n` to confirm
      no hardcoded-string violations remain on these widgets.

**Validates:** The picker UI surfaces correct Spanish strings the
moment Phase 2 makes it reachable for existing covers.

### Phase 2: Lift the guard on all three content views
**Goal:** Owners can change the cover photo on gear, request, and
experience items in edit mode using the existing `MediaPickerButton`
"Replace background" state.
**Tasks:**
- [ ] Add a derived getter on each of `GearState`, `RequestState`, and
      `ExperienceState`: `bool get canEditCoverPhoto => isEditing &&
      isOwner;`. (Lifts the visibility decision out of widget code so
      it can be unit-tested at the viewmodel layer per
      `docs/client/testing.md`.)
- [ ] Change the guard at `gear_content_view.dart:384` from
      `state.isEditing && state.mediaPath == null` to
      `state.canEditCoverPhoto`.
- [ ] Make the matching change at `request_content_view.dart:468` and
      `experience_content_view.dart:491`.
- [ ] Confirm `_showMediaPickerDialog` (or its equivalent) on each
      screen routes to gallery / camera / video picks and that
      `_setMediaFile` promotes the new item to the cover. This is a
      verification, not a code change — these flows already work via
      the chat-tab carousel.
- [ ] Manual smoke test on a populated item in edit mode for each of
      gear, request, and experience: confirm the centered "Replace
      background" button appears, opens the picker, uploads, and the
      cover swaps. If the centered button visually collides with the
      edit bar or info button on small screens, file a follow-up — do
      not expand this PR into a layout rework.

**Validates:** Owners can swap the cover photo on all three inline-edit
entity types without going through the chat tab.

### Phase 3: Tests
**Goal:** Lock in the behavior so the gate doesn't silently re-appear,
following the ViewModel-first testing convention from
`docs/client/testing.md`.
**Tasks:**
- [ ] Add ViewModel unit tests under
      `app/test/presentation/viewmodels/` for `GearViewModel`,
      `RequestViewModel`, and `ExperienceViewModel` using
      `ProviderContainer` and Mockito-mocked repositories per the
      conventions in `docs/client/testing.md`. Each test should drive
      state through:
      - Initial load (`isOwner: true`, `isEditing: false`) →
        `canEditCoverPhoto == false`
      - `toggleEditMode()` → `canEditCoverPhoto == true`
      - Non-owner case (`isOwner: false`, `isEditing: true`) →
        `canEditCoverPhoto == false`
      Reset mocks and dispose containers in `tearDown` per the
      pitfalls section in the testing doc.
- [ ] Add a thin widget smoke test per content view under
      `app/test/presentation/screens/{gear,request,experience}/` that
      pumps the screen with a state where `canEditCoverPhoto == true`
      and asserts the `MediaPickerButton` is present, plus a negative
      case where it should be absent. Wrap the `MaterialApp` with
      `localizationsDelegates` and `supportedLocales` per
      `app/test/helpers/l10n_helpers.dart` (referenced in
      `docs/client/i18n.md`).
- [ ] Run `npm run test:app` (or `flutter test --reporter expanded`
      per CLAUDE.md) and confirm no regressions.

**Validates:** Future refactors that re-introduce a `mediaPath ==
null` gate or remove `canEditCoverPhoto` will fail at the viewmodel
test layer (fast, deterministic), with the widget smoke test catching
incorrect wiring of the bool to the widget tree.

### Phase 4: Close the loop on #1697
**Goal:** Confirm the original user-reported bug is actually fixed (or
identify a residual bug if not).
**Tasks:**
- [ ] Reproduce #1697's flow: cover starts as an AI image, add a real
      photo via the new "Replace background" affordance, confirm the
      cover flips to the new photo immediately.
- [ ] If the cover flips correctly, leave #1697 closed; the
      discoverability fix from Phases 1-3 is the resolution.
- [ ] If the cover does NOT flip even with the new affordance, the
      `_setMediaFile` promote-to-cover path is broken and that's a
      separate bug. File it and link from #1697.

**Validates:** The original user-facing problem is resolved (or
re-scoped to a clean follow-up bug).

## Related Issues

- #1697 — duplicate; the original user-reported flavor of this gap.
  Closed pointing here. Phase 4 verifies whether the underlying
  behavior is also broken or whether the discoverability fix alone
  closes it.
