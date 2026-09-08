---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Ripls app and website design language — Ink & Sage token consumption, light/dark themes, typography, semantic and content-type colors, and AppColors routing discipline enforced by lint.
  globs: [app/lib/core/theme/**]
  triggers: [design, color-palette, theme, app-colors, typography, dark-mode, sage, branding, tokens]
  lens: [client]
  domain: client
freshness:
  verified_commit: "3faa32b81"
  verified_on: "2026-06-12"
---
# Client Design Language

## Overview

This document describes the Ripls app and website design language: how the app consumes the canonical design tokens, plus the UI patterns, component design, and best practices layered on top. The visual identity is **Ink & Sage** (#2441): white/ink-green neutrals, deep sage reserved for actions, ink as the emphasis accent.

> **Brand, neutral, and semantic color values and font families are NOT defined here or in `app_colors.dart` — they live in [`design/tokens.json`](../../design/tokens.json)**, the single source of truth (#2441, superseding the hand-mirrored tokens.css promised in #2049). Generated consumers: `app/lib/core/theme/gen/design_tokens.gen.dart` (app), `website/content/css/gen/tokens.gen.css` (web), and the human-viewable [`design/guidelines.html`](../../design/guidelines.html). `npm run lint:design-tokens` fails CI on drift; generation itself fails if any declared WCAG contrast pair regresses. Routing discipline (every accent-tinted pixel through `AppColors`) remains enforced by `npm run lint:dart:colors`.
>
> History: coral → Heritage Sage `#5B8268` in #1946; Heritage Sage → tokenized Ink & Sage (deep sage `#3E5A47` light primary) in #2441.

## Color Palette

### Token consumption (brand, neutral, semantic)

See [`design/guidelines.html`](../../design/guidelines.html) for every value with swatches, usage notes, and contrast ratios. In the app, `AppColors` maps UI roles onto `DesignTokens` constants:

- **Primary/accent:** `lightPrimary`/`darkPrimary` are the token primaries (deep sage light, light sage dark). `lightAccent`/`darkAccent` mirror them (each theme's accent is the other theme's primary). The beige `secondary` retired in #2441 — it now holds soft sage.
- **Backgrounds:** page and cards are the token `background` (white) in light; dark uses token `background`/`surface` (ink-green ramp). The dark `border` token doubles as the third neutral step (`AppColors.darkSurface`) for raised fills on cards.
- **Text:** `textPrimary`/`textSecondary`/`textTertiary` map to the token text ramp (`text-primary`/`text-secondary`/`text-faint`). The faint step clears 3:1, not 4.5:1 — placeholders and footnotes only.
- **Borders and dividers:** both map to the single token `border` per theme.

### Semantic Colors

**Per-theme since #2441** — a single shared value cannot pass 4.5:1 on both white and dark surfaces. Use the theme-aware getters `AppColors.statusSuccess/statusWarning/statusError/statusInfo(context)`; values come from the tokens and pass 4.5:1 as text on both `background` and `surface` in their theme.

The legacy shared consts (`AppColors.success/warning/error/info`) point at the **dark** token values (success/warning unchanged from the pre-#2441 palette; error/info slightly lightened) and under-deliver on light surfaces — the call-site migration to the getters is tracked in **#2445**.

- Error banner text: `#FF8A80` (light salmon, paired with translucent red banner backgrounds in transfer / fulfillment modals) — app-level, not a token.

`warning` and `requestColor` stay warm-orange/amber so a warning surface never collides with the sage brand.

### Content Type Colors

Item-specific colors that work in both themes:
- Giveaway: `#7A9B76` (Sage green)
- Loan: `#5A7A82` (Stone blue)
- Request: `#E8A661` (Warm orange)
- Experience: `#9B7AA6` (Soft purple)

### Chat Colors

- Light Chat Bubble: `#7B9FB5` (Dusty blue - complements warm palette)
- Dark Chat Bubble: `#5A7A8A` (Darker dusty blue)

### Functional Colors

**Overlay Colors:**
- Light Overlay: `black @ 0.3 alpha`
- Dark Overlay: `black @ 0.5 alpha`

**Metadata Chips (over media):**
- Background: `white @ 0.2 alpha`
- Border: `white @ 0.3 alpha`

**Floating Action Buttons:**
- Background: `black @ 0.16 alpha` (semi-transparent)
- Foreground: `white`

**Overlay Text Fields (over media):**
- Text: `white`
- Hint: `white @ 0.7 alpha`
- Background: `black @ 0.3 alpha`
- Border Enabled: `white @ 0.3 alpha`
- Border Focused: `white`

**Availability Status:**
- Loan Available: `#5A7A82 @ 0.6 alpha`
- Giveaway Available: `#7A9B76 @ 0.6 alpha`
- Unavailable: `red @ 0.6 alpha`
- Available/Unspecified: `green @ 0.6 alpha`

### Gradients

**Light Theme Gradient (`AppColors.lightGradient`):**
- token `light primary` → `dark primary` (deep sage → light sage)

**Dark Theme Gradient (`AppColors.darkGradient`):**
- token `dark primary` → `dark primary-hover` (light sage → pale sage)

**Content Overlay Gradient:**
- Top: `transparent`
- 40%: `black @ 0.3 alpha`
- 70%: `black @ 0.7 alpha`
- Bottom: `black @ 0.9 alpha`

## Typography

### Font Families

Families are defined in [`design/tokens.json`](../../design/tokens.json): **Libre Baskerville** (serif, display/headings, weights 400+700, no italic display) and **Public Sans** (sans, body/UI) with a platform fallback stack.

**Headings:** Libre Baskerville — `AppTheme.headingFont` reads `DesignTokens.serifFamily`; bundled as app font assets.

**Body Text:** the app realizes the sans token via its platform fallback (SF Pro on iOS, Roboto on Android) rather than bundling Public Sans; the web self-hosts Public Sans (`website/content/css/fonts.css`).

### Text Styles (Flutter)

**Display Styles (Libre Baskerville):**
- Display Large: 68px, w400, 1.3 line height
- Display Medium: 42px, w400
- Display Small: 38px, w400
- Headline Large: 32px, w400
- Headline Medium: 24px, w500

**Title Styles (Libre Baskerville):**
- Title Large: 22px, w400, 1.7 line height
- Title Medium: 16px, w500

**Body Styles (System Sans-Serif):**
- Body Large: 19px, w400, 1.7 line height, secondary text color
- Body Medium: 17px, w400, 1.7 line height, secondary text color
- Body Small: 14px, w400, tertiary text color

**Label Styles (System Sans-Serif):**
- Label Large: 17px, w600, primary text color

### Text Styles (Web)

**Headings (Libre Baskerville):**
- H1: clamp(36px, 5vw, 48px), w400, 1.3 line height
- H2: clamp(28px, 4vw, 38px), w400, 1.3 line height
- H3: 22px, w500, system sans-serif

**Body (System Sans-Serif):**
- Paragraph: 18px, 1.7 line height
- List Items: 17px, 1.7 line height

**Hero Text (marketing landing):**
- `clamp(2.4rem, 5.6vw, 4.6rem)`, ~1.08 line height, Libre Baskerville

Source: [shared.css](../../website/content/css/shared.css) (base typography), [marketing.css](../../website/content/css/marketing.css) (hero / landing)

## Spacing and Layout

### Border Radius Standards

- **Cards:** 30px (large, soft)
- **Buttons:** 50px (pill-shaped)
- **Input Fields (Light):** 8px
- **Input Fields (Dark):** 12px
- **Modals:** 20px (top corners only)
- **Status Chips:** 20px (pill)
- **Floating Actions:** 20px
- **Feature Cards (Web):** 30px

### Card Margins

- Horizontal: 16px
- Vertical: 8px

### Content Padding

- Standard Section: 24px horizontal
- Modal Header: 24px horizontal, 24px top, 16px bottom
- Bottom Content Section: 24px horizontal, 32px top, 24px bottom

## Component Patterns

### Content View Pattern

Full-screen content views (gear, request, experience) follow a consistent pattern:

**Structure:**
1. Background media (image or video)
2. `ContentGradientOverlay` — frame-anchored, darkens the top strip for the
   controls only
3. Content overlay (text, chips, controls), wrapped in `HeroContentWash` —
   content-sized, and the layer that makes the text legible
4. Floating action buttons (top-right)

**Content Overlay Layout:**
- Spacer (pushes content to bottom)
- Media picker button (when editing or no media)
- Bottom section with gradient:
  - Content type header (icon + label)
  - Title (editable field or display text)
  - Description (expandable or editable)
  - Status chips row (location, time, RSVP, etc.)
  - Owner section
  - Action buttons (when editing)

Source: [experience_content_view.dart](../app/lib/presentation/screens/experience/experience_content_view.dart)

### Modal Bottom Sheets — Glass Material

Bottom-sheet modals are unifying around a frosted-glass material. The
primitives live under [`app/lib/presentation/widgets/modal/glass/`](../../app/lib/presentation/widgets/modal/glass/). See [`docs/issues/1797-glass-modal-revamp.md`](../issues/1797-glass-modal-revamp.md) for the migration plan.

**Tokens:**
- Color tokens: `AppColors.modal*` (e.g. `modalSurface`, `modalBackdrop`,
  `modalChipBackground`, `modalPrimaryButtonBackground`). Since #2770 these are
  **semantic aliases** onto `GlassTokens` — the values live in
  [`design/tokens.json`](../../design/tokens.json) under `glass`, not here.
- Geometry / typography: `ModalTheme` in [`app_theme.dart`](../../app/lib/core/theme/app_theme.dart).

**Documented exception to the theme-aware-color rule:** the glass
sheet and its on-glass content are intentionally **identical in light
and dark modes** — white text at varying alphas on a translucent
white sheet over a dark scrim. The scrim handles theme adaptation.
Making modal text theme-aware would be wrong for this material.

**Fills are not foregrounds — but `glass.primary` is both** (corrected in
#2806). It shipped as the deep sage `#3E5A47` and measured **1.66:1** painted on
the sheet, which is what #2764 was. #2770 flipped it to the light sage
`#9DBFA8`, which measures **~6.3:1** there, so an accent icon or label painted
with it on glass is now correct — the token's `$description` in
[`tokens.json`](../../design/tokens.json) calls it "the on-glass action colour".
Body text still comes from `GlassTokens.textPrimary` / `textSecondary` /
`textFaint`; reach for `primary` only when you want an *accent* foreground.

What stays banned, and did not relax with the value change, is handing it to
`ColorScheme.primary`. Material resolves one slot into a TextButton foreground
**and** a selected-day fill at once, so the author picks neither — that is
#2764's actual mechanism, and with today's light sage the day cell would be
light sage under white text at 2.01:1.

`GlassTokens.fillStrong` and `AppColors.modalChipBackgroundActive` (solid white)
remain fill-only in every position: solid white as a foreground is
`textPrimary`'s job, and the wrong name buys nothing but a silent break the day
one of the two moves. Enforced by `npm run lint:glass-foreground`.

**None of this can tell you whether a given pair is legible**, because an
element may paint its own fill and the lint never looks underneath — that is
#2798, where a control painted `#F2F2EE` and drew the on-glass white ramp on it
at 1.12:1. Contrast is *measured*, in two places:
[`app/test/helpers/contrast_helpers.dart`](../../app/test/helpers/contrast_helpers.dart)
for opaque pairs on every `flutter test`, and
[`e2e/scripts/contrast_surfaces.mjs`](../../e2e/scripts/contrast_surfaces.mjs)
for translucent composites.

This matches Apple's guidance for the same material: iOS defines vibrancy values
for labels, fills and separators designed to work with each material, and
*standard system colors aren't available in vibrant versions* — the system
simply does not offer a brand colour on a material.

### Media Overlay Material

The wash painted **onto** a user's photo — a different material from glass,
which floats *above* content. Values come from `OverlayTokens`
(`design/tokens.json` → `overlay`); the semantic accessors are
`AppColors.contentOverlay`, `contentScrimTop/Bottom/Floor`,
`metadataChip*`, `overlayField*`, `floatingAction*`.

**Not theme-aware, deliberately.** `contentOverlay(context)` used to return 30%
in light and 50% in dark *for the same photo*, which made the wash weakest
exactly when the media was brightest. The photo is the backdrop; it does not
follow the app theme.

**Two washes, and only one of them protects text.**

[`ContentGradientOverlay`](../../app/lib/presentation/widgets/content/content_gradient_overlay.dart)
is anchored to the **frame**, so the only thing it can express is "N% down the
screen". It darkens the top strip for the back and overflow controls and clears
completely below that. Nothing legibility-critical belongs here.

[`HeroContentWash`](../../app/lib/presentation/widgets/content/hero_content_wash.dart)
is sized to the **content**. A hero's text lives in a bottom-anchored sheet
whose height depends on how much content the item has — a title alone on one, a
title plus four metadata rows plus a discussion card on another — so no fixed
fraction of the screen describes where text begins. It wraps that sheet and
inherits its height, with a fixed `runUp` above it that ramps transparent →
`wash-top`, so the sheet's first pixel is already at full wash.

The run-up is the part that was missing. Six hand-rolled copies of this gradient
put `Colors.transparent` at stop 0.0 of the sheet's own box and reached 65% a
fifth of the way down; text starts ~16px below that edge, so the title sat on
roughly 10% black and measured **3.07:1**. The wash was covering the metadata
and missing the headline.

**Don't fix media contrast by washing the frame.** The obvious repair — hold a
floor across the whole gradient — passes the measurement and costs the
photograph: it measured **26-37% of mean brightness on every hero photo**, spent
mostly on bare backdrop and on pixels the sheet already covers. If text on media
fails, the wash under *that text* is what should move.

Media text must also come from the overlay ramp (`OverlayTokens.textPrimary` /
`textSecondary` / `textFaint`), never the palette's neutral steps: those are
validated against `background`/`surface` and measure as low as 2.71:1 out on
media.

**Scope:** Only **bottom-sheet** modals are migrated. Right-side detail
screens (`pushScreen` + `SwipeToCloseMixin`), creation/preview screens,
centered `AlertDialog`s, and the warm-paper impact-detail modals are
intentionally on different surfaces.

### Modal Bottom Sheets — Legacy builder pattern (being retired)

Standardized modal presentation using the modal builder pattern:
- Height: 75% of screen by default, expands to 100% with keyboard
- Background: Card background color
- Border radius: 20px (top corners only)
- Smooth 200ms animation for height changes
- Always dismissible (tap outside, swipe down)

**Architecture:**

The modal system uses a two-layer architecture:

1. **Modal Helpers** ([modal_helpers.dart](../app/lib/presentation/widgets/modal/modal_helpers.dart)) - Provides `showStandardModal()` for displaying modals with automatic keyboard handling
2. **Modal Builders** ([modal_builders.dart](../app/lib/presentation/widgets/modal/modal_builders.dart)) - Provides reusable UI components for modal content

**Standard Modal Structure:**

Modals follow a consistent builder pattern with three main sections:

1. **Header** - Built with `buildModalHeader()`:
   - Title text (20px, bold)
   - Close button (top-right)
   - Optional action buttons via `headerActionButtons` parameter
   - Padding: 24px horizontal, 24px top, 16px bottom

2. **Content** - Built with `buildReadModeContent()` or `buildEditModeContent()`:
   - Read mode: Display-only content with optional header content
   - Edit mode: Editable form fields
   - Automatic mode switching based on state

3. **Interactive Elements** - Built with specialized builders:
   - `buildChipsRow()` - Horizontal scrollable chips with selection
   - `buildInfoDisplayRow()` - Single-line text display
   - `buildHeaderActionButtons()` - Edit/Done button pairs

**Modal Lifecycle:**

- Opening: Modal slides up from bottom
- Keyboard appears: Modal expands to 100% height
- Keyboard dismisses: Modal returns to 75% height
- Closing: Modal slides down, can show unsaved changes warning

**Example Modals:**

- Location picker: Uses edit mode with search field and chips
- Time picker: Uses edit mode with time selection controls

Sources: [modal_helpers.dart](../app/lib/presentation/widgets/modal/modal_helpers.dart), [modal_builders.dart](../app/lib/presentation/widgets/modal/modal_builders.dart), [location_picker_modal.dart](../app/lib/presentation/widgets/location/location_picker_modal.dart)

### Conversation Headers

Compact header for chat screens:
- Thumbnail: 48x48, 8px border radius
- Avatar overlay: 10px radius, bottom-right, with 1.5px border
- Type label: Small, color-coded
- Title: Below type label
- Border: Bottom divider line
- Background: Card background
- Padding: 16px horizontal, 12px vertical

Source: [conversation_compact_header.dart](../app/lib/presentation/widgets/chat/conversation_compact_header.dart)

### Discover Cards

Swipeable cards for gear/request discovery:
- Media thumbnail on left side
- Title at top
- Truncated description
- Owner info at bottom
- Distance indicator
- Supports PageView for horizontal swiping

Source: [discover_gear_card.dart](../app/lib/presentation/screens/discover/discover_gear_card.dart)

## Form Controls

### Text Input Fields

**Light Theme:**
- Fill color: `grey[50]`
- Label: `grey[800]`, 14px
- Hint: `grey[600]`, 14px
- Border radius: 8px
- Border width: 0.5px
- Border color (enabled): `grey[600]`
- Border color (focused): Primary color, 1px
- Border color (error): Error color, 1px
- Content padding: 8px horizontal, 10px vertical

**Dark Theme:**
- Fill color: `grey[850]`
- Label: `grey[400]`, 13px
- Hint: `grey[600]`, 13px
- Border radius: 12px
- Border width: 0.5px
- Border color (enabled): `grey[700]`
- Border color (focused): Primary color, 1px
- Border color (error): Error color, 1px
- Content padding: 12px horizontal, 10px vertical

Source: [app_theme.dart](../app/lib/core/theme/app_theme.dart)

### Keyboard Handling

iOS does not provide a universal keyboard dismiss gesture. Two mechanisms are applied to all screens with text input:

**Tap-to-dismiss (`KeyboardDismissWrapper`):**

Wrap the `Scaffold.body` (or modal content area) with `KeyboardDismissWrapper`. Tapping anywhere outside a focused field dismisses the keyboard.

```dart
body: KeyboardDismissWrapper(
  child: SafeArea(child: SingleChildScrollView(...)),
),
```

**"Done" toolbar for multiline fields (`keyboard_actions`):**

iOS ignores `TextInputAction.done` when `maxLines > 1`. Use the `keyboard_actions` package with `buildKeyboardActionsConfig()` to show a "Done" button in a toolbar directly above the keyboard. Each multiline `TextField` needs its own `FocusNode`.

```dart
// In State:
final _descriptionFocusNode = FocusNode();
// In dispose():
_descriptionFocusNode.dispose();

// In build:
KeyboardActions(
  disableScroll: true,
  config: buildKeyboardActionsConfig([_descriptionFocusNode]),
  child: KeyboardDismissWrapper(
    child: /* content */,
  ),
),
TextField(
  focusNode: _descriptionFocusNode,
  maxLines: 4,
  ...
),
```

**When to apply each:**

| Field type | `KeyboardDismissWrapper` | `keyboard_actions` toolbar |
|---|---|---|
| Single-line (email, name, search) | ✅ Always | ❌ Not needed — iOS shows "Done"/"Next" |
| Multiline (description, reasoning, summary) | ✅ Always | ✅ Required |

**Reference:** [keyboard_dismiss_wrapper.dart](../app/lib/presentation/widgets/keyboard_dismiss_wrapper.dart), [keyboard_actions_config.dart](../app/lib/presentation/widgets/keyboard_actions_config.dart)

### Buttons

**Elevated Buttons (Primary):**
- Background: Primary color
- Foreground: White (light) or primary text (dark)
- Border radius: 50px (pill-shaped)
- Padding: 32px horizontal, 16px vertical
- Text: 17px, w600
- Elevation: 0 (flat design)

**Outlined Buttons:**
- Foreground: Primary color
- Border: 2px solid border color
- Border radius: 50px
- Padding: 32px horizontal, 16px vertical
- Text: 17px, w600

Source: [app_theme.dart](../app/lib/core/theme/app_theme.dart)

### Web Buttons

**Primary CTA:**
- Background: `var(--color-cta-primary)`, aliased to the token `primary` (sage owns actions) in [`tokens.css`](../../website/content/css/tokens.css); consumed by `.hero-cta` / `.event-cta-primary` / the deep-link card CTA.
- Color: the token `on-primary`
- Pill-shaped, lift-on-hover

Source: [tokens.css](../../website/content/css/tokens.css) (derived aliases over the generated `gen/tokens.gen.css`), [marketing.css](../../website/content/css/marketing.css)

> Since #2441 the web tokens are **generated**, not hand-mirrored: `tokens.css` imports `gen/tokens.gen.css` and defines only derived aliases and app-level extras (item-type accents, type/spacing scales, shadows). Coral is gone entirely — `--color-error` is the per-theme error token and `--color-experience` aliases the sage primary.

## Navigation Patterns

### Transitions

**Standard Navigation:**
- Slide from right (1.0, 0.0) to (0.0, 0.0)
- Curve: easeInOut
- Used for: User profiles, content detail screens

**Modal Presentation:**
- Bottom sheet with curved top corners
- 75% screen height for complex modals
- Transparent background with scrim

### App Bar

- Background: App bar background color
- Elevation: 0 (flat)
- Center title: true
- Title: 20px, w400, 0.5 letter spacing
- Icon theme: Primary text color

Source: [app_theme.dart](../app/lib/core/theme/app_theme.dart)

### Bottom Navigation

- Background: Card background
- Selected: Primary color
- Unselected: Secondary text color
- Elevation: 8
- Type: Fixed (always shows labels)

### Back Buttons and Swipe Gestures

The app uses two distinct back button widgets for different contexts:

**AppBarBackButton** ([app_bar_back_button.dart](../app/lib/presentation/widgets/app_bar_back_button.dart)):
- Usage: Standard AppBar back buttons
- Style: IconButton with iOS-style chevron (`Icons.arrow_back_ios_new`)
- Color: Theme-aware via `AppColors.textPrimary(context)`
- Behavior: Defaults to `Navigator.pop()` if no `onPressed` provided

**BackButtonWidget** ([back_button.dart](../app/lib/presentation/widgets/back_button.dart)):
- Usage: Overlay screens (gear_screen, experience_screen, request_screen)
- Style: Circular button with semi-transparent background
- Color: White icon on `black @ 0.16 alpha` background
- Behavior: Requires explicit `onPressed` callback

**SwipeToCloseWrapper** ([swipe_to_close_wrapper.dart](../app/lib/presentation/widgets/swipe_to_close_wrapper.dart)):
- Usage: Wraps full-screen overlay/detail views for swipe-right-to-close gesture
- Velocity threshold: 300 px/s (default)
- Applied to: User profiles, content detail screens, community public view, settings
- Not applied to: Conversation screens (text input conflict), list/menu screens

## Design Best Practices

### ✅ Do's

**Color Usage:**
- Use theme-aware color getters from AppColors (e.g., `AppColors.primary(context)`)
- Apply semantic colors consistently (success = sage green, warning = warm orange, error = darker coral red, info = stone blue)
- Use semi-transparent overlays for text on media (`black @ 0.3-0.9 alpha`)
- Ensure white text on dark overlays for readability
- `npm run lint:dart:colors` (CI) blocks new `Colors.orange` / `Colors.deepOrange` / `Colors.amber` and coral/sage hex literals outside `app/lib/core/theme/`. The ratchet allowlist at `scripts/color_allowlist.txt` may shrink but never grow.

**Typography:**
- Use Libre Baskerville for all headings and titles
- Use system sans-serif for body text and UI labels
- Maintain 1.7 line height for body text
- Apply proper text shadows for text over images

**Layout:**
- Use consistent border radius (30px cards, 50px buttons, 20px chips)
- Maintain standard spacing (24px horizontal padding for content)
- Apply gradient overlays on full-screen media backgrounds
- Use SafeArea for bottom content sections

**Components:**
- Reuse standard components (ContentStatusChip, ContentFloatingActions, etc.)
- Follow the content view pattern for full-screen displays
- Use ContentActionButtons for edit mode controls
- Apply ContentOwnerSection for user attribution

**Interaction:**
- Use GestureDetector for tap interactions on custom widgets
- Apply proper loading states (CircularProgressIndicator, shimmer)
- Show clear error states with retry actions
- Provide visual feedback for button presses
- **Wrap all text-input screens/widgets with `KeyboardDismissWrapper`** for tap-to-dismiss keyboard behavior
- **Add `keyboard_actions` toolbar via `buildKeyboardActionsConfig()`** for every multiline `TextField` (`maxLines > 1`)

### ❌ Don'ts

**Color Usage:**
- Don't hardcode colors — use AppColors constants. `Colors.orange` / `Colors.deepOrange` / `Colors.amber` and raw coral/sage hex literals outside `app/lib/core/theme/` are blocked in CI.
- Don't use overly bright or saturated colors
- Don't place dark text on dark backgrounds
- Don't mix color systems (stick to the palette)
- Don't treat `AppColors.success`, `AppColors.transferSage`, or `AppColors.lightChatBubble` as interchangeable with the brand accent — they are all sage-ish but carry distinct semantics (success/completion vs. brand). Pending design review will sharpen the visual separation.

**Typography:**
- Don't use random font sizes - stick to text theme
- Don't use sans-serif for headings
- Don't use serif fonts for body text
- Don't ignore line height (maintain 1.7 for readability)

**Layout:**
- Don't create inconsistent border radius values
- Don't ignore safe areas on content overlays
- Don't overcrowd UI with too many elements
- Don't use sharp corners where soft edges are expected

**Components:**
- Don't create duplicate components - reuse existing ones
- Don't embed business logic in presentation widgets
- Don't skip loading/error states
- Don't create deeply nested widget trees (extract methods/widgets)

**Interaction:**
- Don't use InkWell on overlay components (use GestureDetector)
- Don't forget to disable buttons during loading
- Don't skip confirmation dialogs for destructive actions
- Don't navigate without proper transitions
- Don't use raw `GestureDetector` for keyboard dismiss — use `KeyboardDismissWrapper`
- Don't leave multiline `TextField` widgets without a "Done" toolbar — iOS Return key inserts a newline instead of dismissing

## Current Inconsistencies

### Color Inconsistencies

1. **Historical token names that no longer match their values:** Several `AppColors` tokens have names that pre-date the #1946 sage migration but now hold sage values — `transferCoral`, `transferCoralSoft`, `experienceCoral`, `ImpactModalColors.coral`, `ImpactModalColors.coralBg`. Treat the *name* as opaque; the *value* is sage. A follow-up issue tracks renaming these to neutral `*Accent` names.

2. **Sage-on-sage visual collision risk:** `lightPrimary` (#3E5A47), `success` (#7A9B76), `experienceSageGreen` (#7A9B8C), `transferSage` (#6B8F71), and `lightChatBubble` (#7A9B76) are all in the same hue family. They are semantically distinct (brand / status / completion / chat) but visually close — though #2441's deeper light primary widened the gap. A design pass to sharpen separation is pending.

3. **Web token mirroring** — resolved in #2441: `tokens.css` now imports the generated `gen/tokens.gen.css`; `npm run lint:design-tokens` fails CI on drift.

4. **CircularProgressIndicator Colors:** Some screens use hardcoded `Colors.purple` instead of primary color
   - Source: [conversation_screen.dart](../app/lib/presentation/screens/inbox/conversation_screen.dart)

5. **Mixed Overlay Alpha Values:** Some components use `.withOpacity()` while others use `.withValues(alpha:)`
   - Should standardize on `.withValues(alpha:)` (newer API)

### Typography Inconsistencies

1. **Input Field Font Sizes:** Light theme uses 14px labels, dark theme uses 13px
   - Source: [app_theme.dart](../app/lib/core/theme/app_theme.dart)
   - Should be consistent across themes

2. **Border Radius Inconsistencies:** Input fields use 8px (light) vs 12px (dark)
   - Source: [app_theme.dart](../app/lib/core/theme/app_theme.dart)
   - Consider unifying for consistency

### Component Inconsistencies

1. **Avatar Sizes:** Inconsistent avatar radius values across components
   - Compact header: 10px overlay
   - Owner section: 20px default
   - Should define standard sizes (small, medium, large)

## Future Improvements

### Design System Enhancements

1. **Standardized Color System:**
   - Create a comprehensive design token system
   - Document all color usage contexts
   - Audit and fix hardcoded colors throughout codebase
   - Add color contrast checking for accessibility

2. **Typography Scale:**
   - Define clear typography scale with named sizes
   - Create text style utilities for common patterns
   - Ensure accessibility with proper contrast and sizing
   - Add responsive typography for web

3. **Spacing System:**
   - Define spacing scale (4px, 8px, 12px, 16px, 24px, 32px)
   - Create spacing constants to replace magic numbers
   - Document when to use each spacing value

4. **Component Library:**
   - Create comprehensive component documentation
   - Build Storybook/Widgetbook for Flutter components
   - Add usage examples for each component
   - Document component composition patterns

### Accessibility Improvements

1. **Color Contrast:**
   - Audit all color combinations for WCAG AA compliance
   - Improve contrast on semi-transparent overlays
   - Add high-contrast mode support

2. **Touch Targets:**
   - Ensure minimum 44x44px touch targets
   - Add proper spacing between tappable elements
   - Document touch target requirements

3. **Text Sizing:**
   - Support dynamic type/font scaling
   - Test all text at larger sizes
   - Ensure layouts don't break with increased font sizes

### Animation and Motion

1. **Standard Transitions:**
   - Define standard animation durations (200ms, 300ms, etc.)
   - Create reusable transition builders
   - Document when to use different transitions

2. **Micro-interactions:**
   - Add subtle animations to button presses
   - Implement loading state transitions
   - Create smooth state changes

3. **Performance:**
   - Optimize image loading and caching
   - Reduce unnecessary rebuilds
   - Profile and improve animation performance

### Responsive Design

1. **Breakpoints:**
   - Define standard breakpoints for web/tablet
   - Create responsive layout utilities
   - Test all screens at different sizes

2. **Adaptive Components:**
   - Create components that adapt to screen size
   - Implement responsive navigation patterns
   - Support landscape orientation properly

## References

### Primary Sources

**Theme and Colors:**
- [app_theme.dart](../app/lib/core/theme/app_theme.dart)
- [app_colors.dart](../app/lib/core/theme/app_colors.dart)

**Web Styles:**
- [tokens.css](../../website/content/css/tokens.css) — design tokens mirrored from the app
- [shared.css](../../website/content/css/shared.css) — base typography
- [marketing.css](../../website/content/css/marketing.css) — landing-page layout

**Content Components:**
- [content_gradient_overlay.dart](../app/lib/presentation/widgets/content/content_gradient_overlay.dart)
- [content_action_button.dart](../app/lib/presentation/widgets/content/content_action_button.dart)
- [content_action_chip.dart](../app/lib/presentation/widgets/content/content_action_chip.dart)
- [content_access_pill.dart](../app/lib/presentation/widgets/content/content_access_pill.dart)
- [content_top_rows.dart](../app/lib/presentation/widgets/content/content_top_rows.dart)

**Screens:**
- [experience_content_view.dart](../app/lib/presentation/screens/experience/experience_content_view.dart)
- [conversation_screen.dart](../app/lib/presentation/screens/inbox/conversation_screen.dart)
- [discover_gear_card.dart](../app/lib/presentation/screens/discover/discover_gear_card.dart)

**Chat Components:**
- [conversation_compact_header.dart](../app/lib/presentation/widgets/chat/conversation_compact_header.dart)

**Sharing:**
- [unified_sharing_modal.dart](../app/lib/presentation/widgets/sharing/unified_sharing_modal.dart)

### Related Documentation

- [architecture.md](architecture.md) - Client architecture patterns
- [client_caching.md](client_caching.md) - Caching architecture
- [client_testing.md](client_testing.md) - Testing patterns

---

**Last Updated:** 2026-06-12 — palette/typography tokenized as Ink & Sage; this doc defers raw values to `design/tokens.json` (#2441)
**Design Version:** Ink & Sage (white/ink-green neutrals, deep sage actions)
