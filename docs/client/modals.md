---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client modal architecture — unified frosted-glass bottom-sheet surface, standard glass-modal shape and entry pattern, glass primitives, Navigator result pattern, keyboard handling, and morph-reveal content panels.
  globs: [app/lib/presentation/widgets/modal/**]
  triggers: [modal, bottom-sheet, glass, glass-sheet, drag-handle, morph-reveal, showAccessibleModal]
  lens: [client]
  domain: client
freshness:
  verified_commit: "69c4ab218"
  verified_on: "2026-07-19"
---
# Client Modal Architecture

## Overview

The Ripls client uses modals for focused user interactions that require temporary context switching. Modal types include:

1. **Creation Modals:** AI-powered creation flows with text/image input (gear, request, experience, community)
2. **Preview Modals:** AI-generated content preview before database save (gear, request, experience, community)
3. **Location Modals:** Search-based location picking with saved chips
4. **Action Modals:** Content-specific actions (RSVP, request fulfillment, invite sharing)
5. **Picker Modals:** Media selection, time/date picking, settings
6. **Navigation Modals:** Entry point selection (PlusButtonModal)
7. **Morph-reveal content panels:** Full-screen panels that *grow from a card's footprint* over the hero, replacing the rest of the content (e.g. "Who's pitching in?", the conversation). A different surface from glass bottom sheets — see *Morph-reveal content panels* below.

All modals follow **standardized patterns** for keyboard handling, navigation, and user feedback.

### Design Philosophy

- **Single modal material — frosted glass.** Every in-scope bottom-sheet
  modal (location, time, RSVP, transfer, action sheets) renders on the
  unified glass surface. The intentionally-distinct surfaces are
  creation/preview modals, impact-detail modals (separate aesthetics), and
  the opaque **`SolidSheet`** used by the Me-sheet grammar (#2634 v2) — see
  *Solid sheets* below.
- **Standard modal shape, no exceptions.** All glass modals share the
  same dimensions, corners, and entry pattern (see *Standard glass-modal
  shape* below). New modals must conform; visual divergence requires a
  design ticket, not a per-modal override.
- **Glass primitives over ad-hoc styling.** Modal layouts compose from
  the widgets in [`app/lib/presentation/widgets/modal/glass/`](../../app/lib/presentation/widgets/modal/glass/) — `GlassSheet`, `GlassModalHeader`, `GlassChip`, `GlassFooterButtons`, `GlassSurface`, etc. No `Color()` literals or hardcoded radii in glass modal code; everything flows from `AppColors.modal*` and `ModalTheme`.
- **Navigator Result Pattern:** Modals return data via `Navigator.pop(result)` instead of callbacks.
- **Keyboard-Aware:** `isScrollControlled: true` lets the sheet grow past the default half-screen ceiling so it *can* lift above the keyboard, but content widgets must apply `MediaQuery.of(context).viewInsets.bottom` themselves — Flutter does not move the content automatically. Reference implementations: [`feedback_sheet.dart`](../../app/lib/presentation/widgets/feedback/feedback_sheet.dart) (overrides `GlassSheet`'s padding with the live inset) and [`_sheet_chrome.dart`](../../app/lib/presentation/widgets/experience/needs/_sheet_chrome.dart) (`resolveBatchSheetPadding` stacks the inset on top of the base padding so the home-indicator breathing room is preserved above the keyboard).
- **Architecture Compliance:** Riverpod state management, repository pattern, no manual caching.

---

## Standard glass-modal shape

Every glass modal renders identically in width, top corners, max height,
and entry mechanism. The constants live in `ModalTheme`
([app_theme.dart](../../app/lib/core/theme/app_theme.dart)) and are
enforced by `GlassSheet` itself:

| Constant | Value | Purpose |
|----------|-------|---------|
| `ModalTheme.sheetHorizontalInset` | **12 px** | Inset from each screen edge — sheet never spans full width |
| `ModalTheme.sheetTopRadius` | **28 px** | Top-rounded corners (bottom is square — sits flush at the screen edge) |
| `ModalTheme.sheetMaxHeightFraction` | **0.92** | Sheet caps at 92% of screen height |

**The only canonical entry pattern** for a glass bottom-sheet modal:

```dart
static Future<T?> show(BuildContext context, {/* args */}) {
  return showAccessibleModal<T>(
    context,
    isScrollControlled: true,                 // required for keyboard handling
    backgroundColor: Colors.transparent,       // GlassSheet provides the surface
    barrierColor: AppColors.modalBackdrop,    // static dark scrim outside the sheet
    builder: (context) => MyModal(...),
  );
}

class MyModal extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return GlassSheet(
      child: /* content column */,
    );
  }
}
```

**The dark scrim is the system modal barrier**, not part of the sheet.
`showModalBottomSheet`'s `barrierColor` paints the scrim **outside** the
modal's animation hierarchy, so the scrim stays put when the sheet is
dragged. `GlassSheet` deliberately does **not** render its own scrim;
adding one inside the sheet would re-introduce the bug where the dim
layer slid up and down with the modal.

**Dismissal** is also handled by the system. `showModalBottomSheet`'s
`isDismissible: true` (the default) makes a tap anywhere outside the
sheet — i.e. on the system barrier — call `Navigator.pop`. Drag-to-
dismiss is governed by `enableDrag` (also default true). New code
does not need a per-modal `onClose` hook for backdrop dismissal.

**Drag handle.** Every glass modal renders a drag-handle pill at the
top — it's both a visual affordance and a touch target for drag-to-
dismiss. Two layout modes:

- **Default (`dragHandleOverlay: false`).** The drag handle sits in a
  `Column` above the child, occupying its own vertical strip. Use this
  for content modals (time, RSVP, info, transfer, etc.) where the
  child is an internal column laid out below the handle.
- **Overlay (`dragHandleOverlay: true`).** The drag handle is
  positioned inside a `Stack` on top of the child, which fills the
  sheet's full available height. Use this when content should bleed
  to the sheet's top edge — the location picker is the canonical
  case (full-bleed map; the handle floats over the map without a
  surface strip above it). Combine with `padding: EdgeInsets.zero`
  for true edge-to-edge bleed.

**Do not** route new glass modals through `ModalHelpers.showStandardModal`.
That helper imposes a 20px ClipRRect and forces a 75/90% height
container, which conflicts with the standard 28px corners and 92% max
height. It survives only for non-glass surfaces such as
[`consent_dialog`](../../app/lib/presentation/widgets/observability/consent_dialog.dart),
which is a centered alert dialog and intentionally out of the
glass-migration scope.

If a modal needs a behavior the canonical pattern doesn't already
provide (e.g. full-bleed map content like the location picker), tune
the *contents*, not the shape — pass a custom `padding:` to `GlassSheet`,
constrain the inner content with a `SizedBox`, or set
`showDragHandle: false`. Keep the 12px inset and 28px top corners
untouched.

---

## Morph-reveal content panels

A **second, deliberately-distinct surface** from the glass bottom sheet:
a full-screen panel that **grows out of the widget the user tapped** (a
clip-reveal "morph"), overlays the content view's **existing hero**
(image/video), and hides everything else. Use it when a widget on a
content view should *expand into its own full detail* rather than slide a
sheet up from the bottom. First shipped for the "Who's pitching in?"
roster and the experience conversation (see
[docs/issues/2280-pitching-in-expand.md](../issues/2280-pitching-in-expand.md)).
**We are standardizing on this surface for content-view expansions**, so
build new ones from the shared pieces below rather than re-rolling the
animation.

### What it looks like / how it behaves

- **Grows from the card.** Tapping the source widget captures its on-screen
  `Rect` and the panel is revealed by a rounded rectangle that interpolates
  from that footprint to full screen (corner radius 18→0). Popping reverses
  the morph — the panel **shrinks back into the card**.
- **Over the hero, not a fresh background.** The route is non-opaque and the
  panel is a transparent scaffold over a 70%-black scrim
  (`AppColors.modalContentScrimStrong`). The content view's
  `VideoBackgroundHost` keeps playing **underneath** — it is never
  re-rendered (that would restart the video).
- **Nothing else shows through.** While open, the read shell's body is
  hidden (via a per-experience flag) and the home top/bottom nav are
  retreated (`HomeNotifier.lockNav`), so only the hero + scrim + panel are
  visible. Both are restored when the panel closes.
- **Three ways to dismiss, all play the shrink:** the in-panel close/back
  control (`Navigator.pop`), the Android system back button, and a
  **horizontal swipe** (either direction, past a velocity threshold).
- **Reduce-motion aware.** The grow/shrink duration comes from
  `accessibleDuration`, so it is instant when the OS reduce-motion setting
  is on.

### Shared pieces (compose these — don't re-roll)

| Piece | Responsibility |
|-------|----------------|
| [`morphRevealRoute`](../../app/lib/presentation/widgets/content/morph_reveal_route.dart) | The transparent `PageRoute` whose destination is clip-revealed from `sourceRect` to full screen, reversing on pop. `scrimMaxOpacity: 0` when the destination supplies its own scrim. Generic — no experience coupling. |
| [`ContentMorphPanel`](../../app/lib/presentation/widgets/content/content_morph_panel.dart) | The shared chrome: transparent `Scaffold` + the strong media scrim + horizontal swipe-to-close. Wrap your panel body in it; it owns the dismiss gesture so children don't repeat it. |
| [`openExperienceContentPanel`](../../app/lib/presentation/screens/experience/widgets/experience_content_panel_launcher.dart) | The launcher that ties it together for an experience: sets the read-shell-hidden flag, locks the nav, pushes `morphRevealRoute` (root navigator, `scrimMaxOpacity: 0`), and restores the flag + nav on close. |
| [`experienceContentExpandedProvider`](../../app/lib/presentation/viewmodels/experience_content_expanded_provider.dart) | Per-experience bool the read shell watches to hide its body while a panel is open. |

The source widget must hand its `Rect` to the launcher. The two content
cards that do this expose a `ValueChanged<Rect>? onTap` that captures
their own `RenderObject` footprint at tap time:
[`ContentEdgesCard`](../../app/lib/presentation/widgets/content/content_edges_card.dart)
and
[`ContentDiscussionCard`](../../app/lib/presentation/widgets/content/content_discussion_card.dart).

### Building a new one

1. Build the panel body as a widget and wrap it in `ContentMorphPanel`
   (add your own header/close control — `IconAction(close)` or a
   `BackButtonWidget` — calling `Navigator.pop`).
2. Give the source card a `ValueChanged<Rect>? onTap` that forwards its
   footprint.
3. In the tap handler call `openExperienceContentPanel(context, ref,
   experienceId, sourceRect, screen, routeName)`.

Reference implementations: the roster
([`ExperiencePitchingInScreen`](../../app/lib/presentation/screens/experience/widgets/experience_pitching_in_screen.dart)),
the conversation
([`ExperienceConversationPanel`](../../app/lib/presentation/screens/experience/widgets/experience_conversation_panel.dart)),
and the location panel
([`ExperienceLocationPanel`](../../app/lib/presentation/screens/experience/widgets/experience_location_panel.dart)).

The **location panel** is the WHERE-card expansion (#2291): a single
multi-state surface (where-screen v3) that absorbed the location-poll bottom
sheets as **inline states** rather than pushed routes. It embeds a map (phone
blue dot + the confirmed spot, or every poll proposal as a lettered marker —
see [geospatial.md](geospatial.md)) and switches between:

- **Poll** — lettered option rows with MOST PICKS / TIED badges, voter avatars,
  the viewer's distance per spot, an unreplied + Nudge strip, inline voting
  (auto-submit, no secondary sheet), and an owner "Set the final spot".
- **Set-final** ([`LocationSetFinalView`](../../app/lib/presentation/screens/experience/widgets/experience_location_set_final_view.dart)) — the absorbed `LocationPollConfirmModal`: the winner card, a tie / not-leader warning, the re-targetable "how everyone picked" breakdown, and lock-in.
- **Single-spot (n=1)** — when one spot is proposed and the viewer didn't add it: "X set the spot · Works for me / Doesn't work / Directions" instead of poll framing.
- **Picker** — picking a spot happens **inline on the panel** (a back arrow in the top bar cancels), not in a pushed sheet. The shared picker body was extracted into [`LocationPickerView`](../../app/lib/presentation/widgets/location/location_picker_modal.dart) (surface-agnostic, reports the saved id via an `onSaved` callback); `LocationPickerModal.show()` is now a thin `GlassSheet` wrapper around it for the other (modal) callers. Save → `proposeSavedLocation` or `setSingleLocation` depending on which control opened it.
- **Confirmed** / **TBD** — directions + change/ask-group, or owner set-a-spot.

The owner's overflow menu opens from a top-bar `···` (in-panel overlay) and is
**always available to the owner** — it carries the poll-management actions
while a poll is running *and* the experience-settings actions ("Mark
Completed", "Close Event") from the content view's Manage sheet, so the owner
can manage the event without leaving the panel. Those two actions reuse
`ExperienceCloseHandlersMixin.markCompleted()` / `closeEvent()`, passed into
both panels from the content view. "Set the final spot" routes to the inline
set-final state. Names, addresses,
and coordinates are resolved in the view-model layer via
`locationPanelGeoProvider`; presentation pieces live in
[`experience_location_panel_widgets.dart`](../../app/lib/presentation/screens/experience/widgets/experience_location_panel_widgets.dart) — so the panel never calls a repository directly. The attendee roster (for the
reply counter + unreplied strip) comes from `experienceProvider`. The card
hands its footprint up through `ContentFactsRow` → `ContentFactData.onTap` (a
`ValueChanged<Rect>`), the only fact card wired to a morph panel; the WHEN/time
card still opens a bottom sheet. The four legacy location-poll bottom sheets
(`LocationPollVoteModal` etc.) survive only for the **edit-mode event pane**,
which still routes through `_showLocationPickerModal`.

**Not yet absorbed (needs backend):** group-average travel time, auto-lock at
the deadline, and opt-in spot suggestions from the v3 spec are deferred — they
require attendee-location data and new RPCs the client doesn't have. The panel
shows the **viewer's** distance only.

The **time panel**
([`ExperienceTimePanel`](../../app/lib/presentation/screens/experience/widgets/experience_time_panel.dart))
is the WHEN-card expansion (when-screen v3) — a deliberate 1:1 mirror of the
location panel for date/time/duration. A **month calendar** — the Home
calendar's [`CalendarMonthGrid`](../../app/lib/presentation/widgets/home/calendar/calendar_month_grid.dart)
(#2514), reused so the app has one calendar surface — replaces the map as the
spatial anchor: the proposed/confirmed days render as photo cells (the event's
image), with the selected day ringed. Same inline states —
poll voting (`voteOnTime`), set-final ([`ExperienceTimeSetFinalView`](../../app/lib/presentation/screens/experience/widgets/experience_time_set_final_view.dart) → `lockTime`), single-time (n=1) accept/decline, confirmed (the agenda dock — event date/time hero with **Add to calendar** as an export icon, a per-day weather strip, an agenda of the viewer's other commitments, an "explore another day" mode, and a host control bar of Mark done · Change time · Ask the group), TBD, and a top-bar manage menu. Driven by `timeModalProvider`; time is stored inline on each `TimeProposal` so no resolution provider is needed. Picking a time happens **inline on the panel** (a back arrow cancels) via the extracted [`DateTimePickerView`](../../app/lib/presentation/widgets/modal/glass/date_time_picker_modal.dart) — the surface-agnostic body (calendar + time wheel) reports its result through an `onSaved` callback; `DateTimePickerModal.show()` is now a thin `GlassSheet` wrapper around it for the other (modal) callers, exactly as `LocationPickerView`/`LocationPickerModal`. Deferred for the same reason as Where: auto-lock, opt-in suggestions, and per-option calendar-conflict evidence (needs calendar access the client doesn't have).

**Shared poll components.** Both panels compose the same domain-agnostic
presentation widgets from
[`widgets/poll/poll_option_widgets.dart`](../../app/lib/presentation/widgets/poll/poll_option_widgets.dart) — `PollOptionRow` (letter chip · name · subtitle · evidence · badge · voter stack · check), `PollVoterStack`, `PollLetterChip`, `PollAddRow`, `PollPanelButton`, `PollUnrepliedStrip`, `PollPickRow`, `PollFinalCard`, plus the `pollOptionLetter` / `pollTiedTopCount` helpers. They take pre-resolved strings + `List<User>` voters, never a location/time vote proto, so the location and time flows stay in lockstep. The manage-menu chrome is likewise shared (`PollManageMenuSheet`).

### When to use which surface

| Use a **glass bottom sheet** when… | Use a **morph-reveal panel** when… |
|------------------------------------|-------------------------------------|
| The interaction is a focused, transient task (pick a time/location, confirm an RSVP, edit a field) that returns a result and dismisses. | A widget on a content view should expand into its **own full screen of detail** over the same hero. |
| It should sit *over* the current screen, which stays visible behind the backdrop. | The rest of the content view should be **replaced**, with only the hero kept. |
| Returns data via `Navigator.pop(result)`. | Is a navigable surface (its own scroll, sub-sheets, conversation) opened from a specific card. |

Do **not** use a morph panel for quick form-style tasks (keep those on the
glass sheet), and do **not** re-render the hero inside the panel.

---

## Modal Infrastructure

### ModalHelpers (legacy — non-glass only)

Centralized utility for showing **non-glass** modals — primarily the
[`consent_dialog`](../../app/lib/presentation/widgets/observability/consent_dialog.dart),
which is a centered alert dialog kept on a non-glass surface.

`showStandardModal` imposes a 20px ClipRRect and a 75/90% height
container, which conflicts with the standard glass-modal shape (28px
corners + 92% max height + 12px horizontal inset). **New glass modals
do not use this helper** — they go through `showAccessibleModal`
directly and wrap content in `GlassSheet`. See *Standard glass-modal
shape* above.

**Reference:** [modal_helpers.dart](../app/lib/presentation/widgets/modal/modal_helpers.dart)

### KeyboardDismissWrapper

Reusable widget that adds tap-to-dismiss keyboard behavior to any content area.

**Purpose:** Wraps content in a `GestureDetector` that calls `FocusScope.of(context).unfocus()` when the user taps outside a text field.

**Usage:**
```dart
body: KeyboardDismissWrapper(
  child: /* existing content */,
),
```

**Applied to:** All screens with text input — preview modals, creation modals, form screens, and embedded form widgets.

**Reference:** [keyboard_dismiss_wrapper.dart](../app/lib/presentation/widgets/keyboard_dismiss_wrapper.dart)

### buildKeyboardActionsConfig

Shared helper that builds a `KeyboardActionsConfig` for the `keyboard_actions` package.

**Purpose:** Provides a "Done" button toolbar above the iOS keyboard for multiline `TextField` widgets where iOS cannot show a native "Done" key (because `TextInputAction.done` is ignored when `maxLines > 1`).

**Usage:**
```dart
// Add a FocusNode to state + dispose():
final _myFocusNode = FocusNode();

// Wrap the content section:
KeyboardActions(
  disableScroll: true,
  config: buildKeyboardActionsConfig([_myFocusNode]),
  child: KeyboardDismissWrapper(
    child: /* content with TextField */,
  ),
),

// Attach to the multiline TextField:
TextField(
  focusNode: _myFocusNode,
  maxLines: 4,
  ...
),
```

**Applied to:** All multiline text inputs — `ContentEditableField` in preview modals, reasoning/description fields in embedded widgets.

**Reference:** [keyboard_actions_config.dart](../app/lib/presentation/widgets/keyboard_actions_config.dart)

### Footer button row — `GlassFooterButtons`

Every glass modal renders its bottom action row through
`GlassFooterButtons`: a hairline `modalFooterDivider` above, a
white-glass secondary on the left, and a coral primary on the right.
Pass `showSecondary: false` for a single-action footer. The primitive
animates `ModalTheme.pressScale` on press and disables both buttons
when their callbacks are null.

The legacy `ModalActionButtons` widget was retired in #1802 Phase 7;
the location picker's trailing directions icon button was dropped to
unify on the canonical 2-button shape.

**Reference:** [glass_footer_buttons.dart](../app/lib/presentation/widgets/modal/glass/glass_footer_buttons.dart)

### ChipData

Data structure for chips that need an icon and a single label (e.g.
location-picker saved-location chips with a home/place icon).

**Fields:** `label`, `icon`, `value`, `isPrimary`

For chips with a primary + optional secondary text line and **no
icon** (e.g. the time modal's "Today / May 8" chips), use
`GlassChip` directly — it owns its own data shape, styling, and
selected-state semantics via `Toggle`.

**Reference:** [chip_data.dart](../app/lib/presentation/widgets/modal/chip_data.dart)

---

## Modal Types

### Creation Modals

**Purpose:** AI-powered content creation with text/image input modes

**Common Structure:**
- Modal bottom sheet (60% height, 95% with keyboard)
- Warm beige background (`AppColors.experienceModalBackground`)
- No headers - clean, minimal design
- Text/Image toggle for input mode switching
- Camera viewport with gallery button (image mode)
- Text input area with inline hint instructions (text mode)
- Floating instruction overlay in camera mode (positioned 140px from bottom)
- Bottom controls with Generate and Manual Create buttons
- Full-screen camera mode (100% height, no SafeArea top padding)
- Loading overlay during AI generation

**Shared Components:**
- [CreationBottomControls](../app/lib/presentation/widgets/creation/creation_bottom_controls.dart) - Toggle, generate, manual buttons
- [CreationInstructionOverlay](../app/lib/presentation/widgets/creation/creation_instruction_overlay.dart) - Floating instruction badge
- [CameraViewport](../app/lib/presentation/widgets/creation/camera_viewport.dart) - Camera preview with gallery
- [TextInputArea](../app/lib/presentation/widgets/creation/text_input_area.dart) - Multi-line text input
- [LoadingOverlay](../app/lib/presentation/widgets/creation/loading_overlay.dart) - AI generation loading state

**Implementations:**
- [CreateGearModal](../app/lib/presentation/screens/gear/create_gear_modal.dart) - "Capture what you want to share"
- [ExperienceCreationModal](../app/lib/presentation/screens/experience/experience_creation_modal.dart) - "Capture an activity or flyer"
- [CommunityCreationModal](../app/lib/presentation/screens/communities/community_creation_modal.dart) - "Capture something about your community"
- [RequestCreationModal](../app/lib/presentation/screens/request/request_creation_modal.dart) - "Capture what you need"

**Pattern:**
- Input state managed by simple ViewModels (Freezed)
- AI generation via Repository methods (no DB save)
- Success: Opens preview modal with generated content
- Manual creation: Opens preview modal with empty state
- See [docs/client/create.md](create.md) for detailed creation flow documentation

### Navigation Modals

**Primary Modal:** [PlusButtonModal](../app/lib/presentation/widgets/plus_button_modal.dart)

**Purpose:** Central navigation hub for creation flows

**Features:**
- Three category groups — **Ask**, **Do**, **Share** — each a grid of circular typed items (e.g. Ask → Help / Babysit / Donation / Ride / …; Do → Eat / Hike / Carpool / …) with per-category gradient accents
- Each item maps to a creation action (`requestSomething`, `inviteExperience`, share gear, invite user, …) and opens the appropriate creation modal
- Used in bottom navigation plus button and empty states

**Used in:**
- [HomeScreen](../app/lib/presentation/screens/home/home_screen.dart) - Bottom nav plus button
- [EmptyContentState](../app/lib/presentation/widgets/empty_content_state.dart) - "Share" button

### Location Modals

**Primary Modal:** [LocationPickerModal](../../app/lib/presentation/widgets/location/location_picker_modal.dart)

**Features:**
- Search-based location picker with autocomplete
- Saved-location chips (home, recent locations) — individual dark-glass pills, edge-to-edge horizontal scroll
- Primary location highlighted with home icon
- Distance calculation from user location
- Owner-only edit mode with read-only view for non-owners

**Modal shape:** Standard glass-modal — `showAccessibleModal` + `GlassSheet`
(12px inset, 28px top corners, 92% max height). Uses
`padding: EdgeInsets.fromLTRB(0, 8, 0, 0)` so the map bleeds full-width
within the sheet; the 8px top breathes the drag handle above the map.

**Floating overlays compositing.** The search wrapper, chips, and
bottom address card are stacked over the (vibrant) map texture — they
do **not** sit on the GlassSheet's frosted surface. To match the time
modal's white-on-glass legibility, each overlay layers a local
`AppColors.modalBackdrop` scrim under a default light `GlassSurface`
(white @ 15% + BackdropFilter blur). This replicates the time modal's
full-screen scrim + frosted sheet compositing **scoped to the
overlay's bounds**, so white text and chip pills read against any map
background.

**Architecture:**
- ViewModel: LocationPickerViewModel with Freezed state
- Repository: LocationRepository with cache namespace 'location'
- Navigator result pattern: Returns location ID via `Navigator.pop(locationId)`
- Static show method: `LocationPickerModal.show(context, ...)`

**Usage:**
- Preview modals: Setting location during gear/request/experience creation
- Content views: Editing location on existing content
- User settings: Setting primary residence

**Reference:** [location_picker_modal.dart](../../app/lib/presentation/widgets/location/location_picker_modal.dart)

### Time (WHEN) surface

The WHEN card no longer opens a standalone glass bottom-sheet
(`UnifiedTimeModal` was removed). It now expands into the
[`ExperienceTimePanel`](../../app/lib/presentation/screens/experience/widgets/experience_time_panel.dart)
**morph-reveal panel** — see *Morph-reveal content panels* → *The time
panel* above for the full state set. State + mutations come from
`timeModalProvider` ([`TimeModalNotifier`](../../app/lib/presentation/viewmodels/time_modal_view_model.dart),
an `AsyncNotifier`).

Inline date/time/duration picking happens **on the panel** via the
extracted [`DateTimePickerView`](../../app/lib/presentation/widgets/modal/glass/date_time_picker_modal.dart);
`DateTimePickerModal.show()` is a thin `GlassSheet` wrapper around it for
the remaining (modal) callers. Both follow the standard glass-modal shape.

### RSVP surface

RSVP is composed through
[`ExperienceRsvpComposerSheet`](../../app/lib/presentation/screens/experience/widgets/experience_rsvp_composer_sheet.dart)
(a `GlassSheet`-based bottom sheet opened via its static `show()`), plus
the segmented RSVP controls on the content view
([`content_rsvp_segmented.dart`](../../app/lib/presentation/widgets/content/content_rsvp_segmented.dart),
[`experience_rsvp_controls.dart`](../../app/lib/presentation/screens/experience/widgets/experience_rsvp_controls.dart)).

**Features:**
- RSVP selection via `RSVPIntention` (Yes / No / Maybe)
- Attendee context and (for the composer) a note field
- Owner vs. attendee affordances

**Architecture:**
- State backed by Freezed/Notifier providers
- The composer returns through its `show()` future / `Navigator.pop`

### Transfer Modals

**Purpose:** Recipient selection and pickup coordination for giveaways (the
loan flow is inline/menu-driven and does not use a dedicated modal — see
`gear_transfer_handlers_mixin.dart`).

**Common Structure:**
- Glass bottom sheet via [TransferModalShell](../app/lib/presentation/widgets/transfer/transfer_modal_shell.dart) (wraps `GlassSheet`)
- 60% screen-height cap on both surviving giveaway flows
- Loading and error rendering handled inside the shell
- Footer action row via `GlassFooterButtons` (or `TransferPhaseBottomBar` for
  the phase-driven cases)

**Live Modal Types:**
- **Giveaway Receiver** ([giveaway_receiver_modal.dart](../app/lib/presentation/screens/gear/giveaway_receiver_modal.dart)) — read-only interest list with the receiver's status
- **Giveaway Giver** ([giveaway_giver_modal.dart](../app/lib/presentation/screens/gear/giveaway_giver_modal.dart)) — picker that selects a recipient from the interest list

The four legacy loan modals (`loan_borrower_modal.dart`, `loan_owner_modal.dart`,
`loan_list_modal.dart`, `loan_manage_modal.dart`) were deleted in #1802 Phase 0
after grep confirmed zero callers.

**Shared Infrastructure:**
- **TransferModalShell** — `GlassSheet` wrapper with loading/error states
- **TransferPhaseBottomBar** — phase-driven action row built on `GlassFooterButtons`
- **RecipientSelectionCard** — selectable user card

**Design System:**
- Uses the standard `modal*` glass tokens (`modalSurface`, `modalChipBackground`,
  `modalPrimaryButtonBackground`, etc.) — same family as the time and location modals
- `transferSage` and `transferCoral` retained as semantic accents only

**Architecture Pattern:**
- ViewModels with Freezed state classes
- Modal widgets are presentation-only; `context.mounted` checks after async

**Benefits:**
- **Code Reduction**: ~37% fewer lines vs. custom implementations
- **Consistency**: All transfer flows use same components and styling
- **Type Safety**: ImpactMetric data class, transfer color constants
- **Maintainability**: Single source of truth for common patterns
- **Testability**: Shared widgets have comprehensive test coverage

**Reference:** [transfer2.md](../docs/ai/transfer2.md), [transfer2_phase15_refactoring_example.md](../docs/ai/transfer2_phase15_refactoring_example.md)

### Preview Modals

**Purpose:** AI-generated content preview before database creation

**Common Structure:**
- Full-screen Dialog with Stack layout
- Background media with gradient overlay
- MediaPickerButton for media upload
- ContentEditableField for text inputs
- Location/time pickers as needed
- ContentErrorBanner for validation errors
- Single action button (Share/Create)
- Close button (X) in top-right corner

**Implementations:**
- [gear_preview_modal.dart](../app/lib/presentation/screens/gear/gear_preview_modal.dart) - Gear creation with lend/giveaway options
- [request_preview_modal.dart](../app/lib/presentation/screens/request/request_preview_modal.dart) - Request creation with required location
- [experience_preview_modal.dart](../app/lib/presentation/screens/experience/experience_preview_modal.dart) - Experience creation with time/RSVP
- [community_preview_modal.dart](../app/lib/presentation/screens/communities/community_preview_modal.dart) - Community creation

**Pattern:**
- Editing buffer pattern: `late String` variables for local state
- Media upload: Stores media ID, not file paths
- Validation: Client-side checks before submission
- Success: Navigator returns created content ID
- Keyboard: `KeyboardActions` + `KeyboardDismissWrapper` wraps the full-screen content; each `ContentEditableField` has its own `FocusNode` passed to `buildKeyboardActionsConfig()`

---

## Common Patterns

### Navigation Pattern

**Standard:** Navigator Result Pattern

All modals use `Navigator.pop(result)` to return data instead of callbacks. This pattern:
- Simplifies modal code (no callback parameters)
- Follows Flutter best practices
- Enables better type safety
- Supports async/await at call sites

**Reference:** [preview_modal_navigation.md](../docs/ai/preview_modal_navigation.md)

### Keyboard Handling

The app uses two complementary mechanisms to ensure text input never blocks action buttons:

#### 1. Tap-to-Dismiss (`KeyboardDismissWrapper`)

Wrap any content area containing text input with `KeyboardDismissWrapper`. Tapping outside a focused field dismisses the keyboard.

```dart
body: KeyboardDismissWrapper(
  child: /* content */,
),
```

Applied universally: preview modals, creation modals, form screens, and embedded form widgets.

#### 2. "Done" Toolbar for Multiline Fields (`keyboard_actions`)

iOS ignores `TextInputAction.done` when `maxLines > 1` — the keyboard shows a Return key instead. For any multiline `TextField`, use `KeyboardActions` + `buildKeyboardActionsConfig()` to display a "Done" button in a toolbar directly above the keyboard.

```dart
KeyboardActions(
  disableScroll: true,
  config: buildKeyboardActionsConfig([_descriptionFocusNode]),
  child: KeyboardDismissWrapper(
    child: /* content with multiline TextField */,
  ),
),
```

Applied to: description/reasoning/summary fields in preview modals and embedded widgets.

#### 3. Automatic Expansion (ModalHelpers)

Standard picker/action modals shown via `ModalHelpers.showStandardModal()` get automatic height expansion:

1. Modal opens at 75% height
2. User taps input field → keyboard appears
3. Modal animates to 90% height (200ms)
4. Keyboard dismisses → modal returns to 75% height

**Reference:** [keyboard_dismiss_wrapper.dart](../app/lib/presentation/widgets/keyboard_dismiss_wrapper.dart), [keyboard_actions_config.dart](../app/lib/presentation/widgets/keyboard_actions_config.dart), [modal_helpers.dart](../app/lib/presentation/widgets/modal/modal_helpers.dart)

### Edit Mode Pattern

**Two Modes:** Read-only view (non-owners) vs. Edit mode (owners)

**Read-Only Features:**
- Summary display with icon and text
- "View Only" indicator
- No save button, only close button

**Edit Mode Features:**
- Input fields enabled
- Quick suggestion chips
- Save button appears when changes detected
- Unsaved changes tracking

**Reference:** Location, time, and RSVP modals all implement this pattern

### Unsaved Changes Tracking

**Pattern:** Track initial state vs. current state to enable save button

**Used by:** RSVP modal, time modal, location modal

---

## Best Practices

### Do's

- **Use `showAccessibleModal` + `GlassSheet`** for every glass bottom sheet (canonical entry pattern)
- **Use shared creation components** for creation modals (CreationBottomControls, CreationInstructionOverlay, etc.)
- **Use TransferModalShell** for transfer modals (loading/error handling on top of `GlassSheet`)
- **Use the modal token family** (`AppColors.modal*` — `modalSurface`, `modalChipBackground`, `modalPrimaryButtonBackground`, `modalTextPrimary`, etc.) for every glass surface; reach for `transferSage` / `transferCoral` only for semantic accents
- **Use transfer_utils helpers** for date/time formatting (formatPickupDate, formatPickupTime, formatRelativeTimestamp)
- **Return data via Navigator.pop(result)** instead of callbacks
- **Add static show() method** to modal classes for clean call sites
- **Use `GlassFooterButtons`** for every footer action row — never hand-roll OutlinedButton/ElevatedButton pairs
- **Wrap all text-input content with `KeyboardDismissWrapper`** for tap-to-dismiss keyboard behavior
- **Use `keyboard_actions` + `buildKeyboardActionsConfig()`** for any multiline `TextField` (`maxLines > 1`) — iOS ignores `TextInputAction.done` for multiline fields
- **Add `FocusNode` per multiline field** and pass it to both `buildKeyboardActionsConfig([node])` and `TextField(focusNode: node)`
- **Add textInputAction: TextInputAction.done** to single-line TextFields
- **Track unsaved changes** to enable/disable save button
- **Support read-only mode** for non-owner views
- **Use ContentEditableField** for preview modal text inputs
- **Use TextInputArea** for creation modal text inputs
- **Add ContentErrorBanner** for inline validation errors
- **Test keyboard behavior** (expansion, dismiss, Done button)
- **Use context.mounted** instead of mounted for BuildContext checks after async operations

### Don'ts

- **Don't use manual showModalBottomSheet()** - use ModalHelpers for picker/action modals, or follow creation modal patterns
- **Don't use FractionallySizedBox** - ModalHelpers handles sizing for picker modals
- **Don't use callback parameters** - use Navigator result pattern
- **Don't duplicate keyboard handling** - use `KeyboardDismissWrapper` and `buildKeyboardActionsConfig()` centrally
- **Don't use raw `GestureDetector` for keyboard dismiss** - use `KeyboardDismissWrapper` instead
- **Don't leave multiline fields without a keyboard dismiss mechanism** - always add `keyboard_actions` toolbar for `maxLines > 1` fields
- **Don't hard-code modal heights** - use default 75% (expands to 90%) for pickers, 60%/95% for creation modals, 90% for transfer modals
- **Don't skip read-only mode** - always support non-owner views where applicable
- **Don't forget close button** - always provide X button in top-right for preview modals
- **Don't auto-save** - require explicit save button tap
- **Don't block keyboard dismiss** - always allow tap-outside-to-dismiss
- **Don't use TextEditingControllers** - use editing buffer pattern in preview modals
- **Don't create custom creation UIs** - use shared creation components for consistency
- **Don't create custom transfer layouts** - use TransferModalShell and shared transfer components
- **Don't hard-code transfer colors** - use AppColors.transfer* constants
- **Don't duplicate date/time formatting** - use transfer_utils helpers
- **Don't use Container for whitespace** - use SizedBox when only setting width/height
- **Don't use mounted for BuildContext checks after async** - use context.mounted instead

---

## Weaknesses

### Modal Fragmentation (resolved for bottom sheets)

**Status:** Bottom-sheet modals migrated to a single material —
frosted glass — under [`#1797`](../issues/1797-glass-modal-revamp.md)
and [`#1802`](../issues/1802-modals-glass-sweep.md). Every glass modal
now shares the same shape (12px inset, 28px corners, 92% max height)
and entry pattern (`showAccessibleModal` + `GlassSheet`). Multiple
historical action-modal variants (RequestActionModal, etc.) and
duplicate location/time modal files were either consolidated or
deleted as dead code in Phase 0.

The remaining pockets of fragmentation are intentional, separate
aesthetics: creation/preview modals (own redesign track) and
impact-detail modals (warm-paper `ImpactModalColors` palette).

### State Management Complexity

**Issue:** Dual context support (provider vs. conversation) adds complexity

**Example:** ExperienceRSVPModal supports both ExperienceProvider and ConversationProvider

**Impact:** More conditional logic, harder to test

### Keyboard Behavior Edge Cases

**Issue:** Keyboard handling doesn't account for all edge cases

**Examples:**
- iPad split keyboard
- Third-party keyboards with custom heights
- Landscape orientation keyboard

**Impact:** Occasional layout issues on non-standard configurations

---

## Future Improvements

| Improvement | Cost | Impact | Priority | Description |
|-------------|------|--------|----------|-------------|
| **Unified action modal** | 3-4 days | Medium | Medium | Extract a reusable ActionModal pattern for content-specific action sheets |
| **Modal animation library** | 2-3 days | Low | Low | Centralize slide/fade animations for consistent transitions |
| **Keyboard height detection** | 2 days | Low | Low | Better handling of custom keyboard heights and orientations |
| **Form validation framework** | 3-5 days | Medium | Low | Reusable form validation with error display |
| **Modal state persistence** | 2-3 days | Low | Low | Preserve modal state across app backgrounding |

**Total Estimated Cost:** 12-17 developer days

**Expected Benefits:**
- Reduced modal code duplication
- More consistent UX across action flows
- Better keyboard handling edge cases
- Easier form validation with reusable patterns

---

## File References

### Modal Infrastructure

- [modal_helpers.dart](../app/lib/presentation/widgets/modal/modal_helpers.dart) - Centralized modal utilities
- [chip_data.dart](../app/lib/presentation/widgets/modal/chip_data.dart) - Standardized chip data structure

### Location System

- [location_picker_modal.dart](../app/lib/presentation/widgets/location/location_picker_modal.dart) - Main location picker
- [location_autocomplete_field.dart](../app/lib/presentation/widgets/location/location_autocomplete_field.dart) - Search field
- [location_picker_view_model.dart](../app/lib/presentation/viewmodels/location_picker_view_model.dart) - State management
- [location_repository.dart](../app/lib/data/repositories/location_repository.dart) - Data access

### Time System

- [experience_time_panel.dart](../app/lib/presentation/screens/experience/widgets/experience_time_panel.dart) - WHEN-card morph panel
- [date_time_picker_modal.dart](../app/lib/presentation/widgets/modal/glass/date_time_picker_modal.dart) - Inline/modal date-time picker (`DateTimePickerView` + `DateTimePickerModal`)
- [time_modal_view_model.dart](../app/lib/presentation/viewmodels/time_modal_view_model.dart) - State management (`timeModalProvider`)

### RSVP System

- [experience_rsvp_composer_sheet.dart](../app/lib/presentation/screens/experience/widgets/experience_rsvp_composer_sheet.dart) - RSVP composer bottom sheet
- [content_rsvp_segmented.dart](../app/lib/presentation/widgets/content/content_rsvp_segmented.dart) - Segmented RSVP control

### Preview Modals

- [gear_preview_modal.dart](../app/lib/presentation/screens/gear/gear_preview_modal.dart)
- [request_preview_modal.dart](../app/lib/presentation/screens/request/request_preview_modal.dart)
- [experience_preview_modal.dart](../app/lib/presentation/screens/experience/experience_preview_modal.dart)
- [community_preview_modal.dart](../app/lib/presentation/screens/communities/community_preview_modal.dart)

### Creation Modal Components

- [creation_bottom_controls.dart](../app/lib/presentation/widgets/creation/creation_bottom_controls.dart) - Toggle, generate, manual buttons
- [creation_instruction_overlay.dart](../app/lib/presentation/widgets/creation/creation_instruction_overlay.dart) - Floating instruction badge
- [camera_viewport.dart](../app/lib/presentation/widgets/creation/camera_viewport.dart) - Camera preview with gallery
- [text_input_area.dart](../app/lib/presentation/widgets/creation/text_input_area.dart) - Multi-line text input
- [loading_overlay.dart](../app/lib/presentation/widgets/creation/loading_overlay.dart) - AI generation loading state
- [input_mode_toggle.dart](../app/lib/presentation/widgets/creation/input_mode_toggle.dart) - Text/Image toggle

### Preview Modal Components

- [content_editable_field.dart](../app/lib/presentation/widgets/content/content_editable_field.dart) - Text input
- [content_error_banner.dart](../app/lib/presentation/widgets/content/content_error_banner.dart) - Error display
- [content_action_button.dart](../app/lib/presentation/widgets/content/content_action_button.dart) - Action button
- [media_picker_button.dart](../app/lib/presentation/widgets/media/media_picker_button.dart) - Media upload button
- [media_picker_dialog.dart](../app/lib/presentation/widgets/media/media_picker_dialog.dart) - Media source picker

### Navigation Components

- [plus_button_modal.dart](../app/lib/presentation/widgets/plus_button_modal.dart) - Creation flow selector
- [empty_content_state.dart](../app/lib/presentation/widgets/empty_content_state.dart) - Empty state with share button

### Transfer Modal Infrastructure

- [transfer_modal_shell.dart](../app/lib/presentation/widgets/transfer/transfer_modal_shell.dart) - Modal container with loading/error handling
- [transfer_modal_header.dart](../app/lib/presentation/widgets/transfer/transfer_modal_header.dart) - Progress bar header with back button
- [transfer_modal_title.dart](../app/lib/presentation/widgets/transfer/transfer_modal_title.dart) - Title with description
- [transfer_phase_bottom_bar.dart](../app/lib/presentation/widgets/transfer/transfer_phase_bottom_bar.dart) - Action buttons
- [transfer_progress_bar.dart](../app/lib/presentation/widgets/transfer/transfer_progress_bar.dart) - Multi-step progress indicator
- [transfer_error_display.dart](../app/lib/presentation/widgets/transfer/transfer_error_display.dart) - Error state display

### Transfer Modal Components

- [pickup_details_card.dart](../app/lib/presentation/widgets/transfer/pickup_details_card.dart) - Pickup/return date display
- [impact_metrics_card.dart](../app/lib/presentation/widgets/transfer/impact_metrics_card.dart) - Impact metrics display
- [impact_metric_row.dart](../app/lib/presentation/widgets/transfer/impact_metric_row.dart) - Individual metric row
- [countdown_display.dart](../app/lib/presentation/widgets/transfer/countdown_display.dart) - Countdown timer boxes
- [waiting_on_user_card.dart](../app/lib/presentation/widgets/transfer/waiting_on_user_card.dart) - Waiting state card
- [what_happens_next_card.dart](../app/lib/presentation/widgets/transfer/what_happens_next_card.dart) - Step checklist
- [what_they_see_card.dart](../app/lib/presentation/widgets/transfer/what_they_see_card.dart) - Other party view
- [other_party_sees.dart](../app/lib/presentation/widgets/transfer/other_party_sees.dart) - Other party status
- [recipient_selection_card.dart](../app/lib/presentation/widgets/transfer/recipient_selection_card.dart) - User selection card
- [quick_date_chips.dart](../app/lib/presentation/widgets/transfer/quick_date_chips.dart) - Date selection chips
- [quick_time_chips.dart](../app/lib/presentation/widgets/transfer/quick_time_chips.dart) - Time selection chips
- [duration_chips.dart](../app/lib/presentation/widgets/transfer/duration_chips.dart) - Duration selection chips
- [step_checklist_item.dart](../app/lib/presentation/widgets/transfer/step_checklist_item.dart) - Checklist item
- [transfer_timeline.dart](../app/lib/presentation/widgets/transfer/transfer_timeline.dart) - Timeline display
- [confirmation_dialog.dart](../app/lib/presentation/widgets/transfer/confirmation_dialog.dart) - Confirmation dialog

### Transfer Modal Utilities

- [transfer_utils.dart](../app/lib/presentation/widgets/transfer/transfer_utils.dart) - Date/time formatting helpers

### Transfer Modal Implementations

- [giveaway_receiver_modal.dart](../app/lib/presentation/screens/gear/giveaway_receiver_modal.dart) - Receiver giveaway flow
- [giveaway_giver_modal.dart](../app/lib/presentation/screens/gear/giveaway_giver_modal.dart) - Giver giveaway flow

(The four legacy loan modals were deleted in #1802 Phase 0 — see *Transfer Modals* above.)

---

## Documentation

- [create.md](create.md) - Creation flow architecture and patterns
- [keyboard.md](../docs/ai/keyboard.md) - Keyboard handling implementation plan
- [preview_modal_navigation.md](../docs/ai/preview_modal_navigation.md) - Navigator result pattern migration
- [content.md](content.md) - Content view architecture (complementary patterns)

---

## Glass modal primitives

The authoritative widget set for bottom-sheet modals. Every in-scope
glass modal composes from these — no per-modal forks, no parallel
cream/sage code paths, no `glass: bool` toggles. Primitives live
under [`app/lib/presentation/widgets/modal/glass/`](../../app/lib/presentation/widgets/modal/glass/) — see the README in that directory.

| Widget | Purpose |
|--------|---------|
| `GlassSheet` | Bottom-sheet container — drag handle (toggleable via `showDragHandle`, position via `dragHandleOverlay` for column-vs-stack layout), 28px top corners, 12px horizontal inset, 92% max height. **Does not render its own scrim** — the dark scrim is the system modal barrier (pass `barrierColor: AppColors.modalBackdrop` on the show call). **Content-only**; does not call `showModalBottomSheet`. Wrap inside `showAccessibleModal`. |
| `GlassModalHeader` | Icon badge + UPPERCASE kicker + value (the value is wrapped in `Semantics(header: true)`). |
| `GlassFieldLabel` | UPPERCASE field label (e.g. "DATE"). |
| `GlassChip` | Two-line selectable chip (primary + optional secondary). Uses `Toggle` semantics. |
| `GlassInlineAction` | 44px translucent row with leading icon + chevron. |
| `GlassFooterButtons` | Cancel ghost + coral primary action bar separated by a hairline divider. The canonical action-row widget — reach for this before falling back to a custom row. |
| `GlassSearchInput` | White-on-glass `TextField`. |
| `GlassSurface` | Foundational blur + fill + border primitive (set `useBlur: false` to skip the `BackdropFilter`; pass `fill:` to override the default `modalSurface` tint). |

**Tokens.** Colors flow from `AppColors.modal*`; geometry and typography
flow from `ModalTheme` ([app_theme.dart](../../app/lib/core/theme/app_theme.dart)). No `Color()` literals or hardcoded radii in glass modal code.

### Solid sheets

[`SolidSheet`](../../app/lib/presentation/widgets/modal/solid_sheet.dart) is a
third, deliberately-**opaque** bottom-sheet surface introduced for the
Me-sheet grammar (#2634 v2): full-width, 28px top radius, a grabber, and a
solid theme-aware fill (`AppColors.cardBackground`) — no blur, no
translucency, and no `modal*` on-glass tokens (content uses the regular
`AppColors.textPrimary` family instead). It follows the same
content-only-widget contract as `GlassSheet` (wrap it in `showAccessibleModal`
yourself) but is a separate widget, not a `GlassSheet` variant — do not pass
`glass: bool` toggles or otherwise try to unify the two. Current callers:
`ProfileMenuModal` (the "Me sheet"), `ProfileRenameSheet`, and the Library
tab's `LibraryLocationSheet` / `MapStyleSheet`. New bottom sheets should
default to `GlassSheet`; reach for `SolidSheet` only for surfaces explicitly
in the solid Me-sheet/Library visual language, not as a shortcut to skip the
glass primitives.

**White-on-glass legibility deviation.** Glass modals deliberately
render identically in light and dark themes — white-on-translucent
over a dark scrim (the scrim handles theme adaptation). This is a
documented exception to the "all colors must be theme-aware" rule in
[architecture.md](architecture.md), recorded in
[design.md](design.md). The exception applies only inside the glass
modal stack.

**Compositing for over-vibrant content (`AppColors.modalOverlayDarkSurface`
+ scrim layering).** When a glass element sits over vibrant content
rather than over the standard dark scrim — e.g. the location picker's
chips and address card sit over a colorful map — `modalSurface` (white
@ 15%) is too transparent to give white-on-glass text the contrast it
needs. The fix is to layer a local `AppColors.modalBackdrop` scrim
under a default light `GlassSurface` (replicates the time modal's
full-screen-scrim-plus-frosted-sheet compositing scoped to the
overlay's rect). The standalone token
`AppColors.modalOverlayDarkSurface` is available as a shorthand when
the layered approach is overkill.

**Scope.** Only **bottom-sheet** modals are on the glass material.
Right-side detail screens, creation modals, preview modals, centered
alert dialogs, and impact-detail modals are intentionally on different
surfaces. See
[`docs/issues/1797-glass-modal-revamp.md`](../issues/1797-glass-modal-revamp.md)
and [`docs/issues/1802-modals-glass-sweep.md`](../issues/1802-modals-glass-sweep.md)
for the migration plan, rationale, and out-of-scope rules.

**Retired.** `ModalBuilders` and `ModalActionButtons` were deleted in
#1802 Phase 7. Compose `GlassModalHeader` + `GlassFooterButtons` inside
a `GlassSheet` for new modals.

**Different surfaces, intentionally.** Two modal families sit outside
the glass aesthetic and stay on their own surfaces:

- **Creation and preview modals** (`community_creation_modal`,
  `experience_creation_modal`, `create_gear_modal`,
  `request_creation_modal`, plus the four `*_preview_modal` files) —
  parallel redesign effort. They still use
  `AppColors.experienceModalBackground` / `lightExperienceModalBackground`
  and the warm-beige aesthetic; that delete is gated on the parallel
  effort landing.
- **Impact-detail and completion modals** (`co2_detail_modal`,
  `value_detail_modal`, `quality_time_detail_modal`,
  `mark_completed_modal`, `mark_fulfilled_modal`, `past_transfer_modal`)
  — `ImpactModalColors` / `CompletionColors` warm-paper celebration
  aesthetic. Migrating these to glass would be a redesign, not a re-skin.
- **`SolidSheet`-based sheets** (`ProfileMenuModal`, `ProfileRenameSheet`,
  `LibraryLocationSheet`, `MapStyleSheet`) — the opaque Me-sheet grammar
  (#2634 v2). See *Solid sheets* above.

---

**Last Updated:** 2026-05-10
