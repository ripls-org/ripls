import 'package:flutter/material.dart';

import 'gen/design_tokens.gen.dart';
import 'gen/glass_tokens.gen.dart';
import 'gen/overlay_tokens.gen.dart';

class AppColors {
  // Available theme-aware helpers: primary, secondary, accent, background,
  // cardBackground, appBarBackground, surface, textPrimary, textSecondary,
  // textTertiary, border, divider, gradient, chatBubble,
  // editableFieldBackground, editableFieldBorder, editableFieldText,
  // editableFieldHint, contentOverlay, fabBackground, metadataChipBackground,
  // metadataChipText, metadataChipIcon.
  //
  // Brand, neutral, and semantic values come from the generated design tokens
  // (design/tokens.json → gen/design_tokens.gen.dart, issue #2441). This class
  // owns the semantic mapping on top: which token a given UI role uses, plus
  // app-level colors (content types, chat glass, modal glass) that are layered
  // on the palette but deliberately not part of the token set.

  // Light Theme Colors — Ink & Sage (see design/tokens.json).
  static const Color lightPrimary = DesignTokens.lightPrimary; // Deep sage
  static const Color lightSecondary =
      DesignTokens.darkPrimary; // Soft sage (beige retired in #2441)
  static const Color lightAccent = DesignTokens.darkPrimary; // Light sage

  static const Color lightBackground = DesignTokens.lightBackground; // White
  static const Color lightCardBackground =
      DesignTokens.lightBackground; // White card, separated by border
  static const Color lightAppBarBackground = DesignTokens.lightBackground;
  static const Color lightSurface =
      DesignTokens.lightSurface; // Green-tinted off-white

  static const Color lightTextPrimary = DesignTokens.lightTextPrimary;
  static const Color lightTextSecondary = DesignTokens.lightTextSecondary;
  static const Color lightTextTertiary = DesignTokens.lightTextFaint;

  static const Color lightBorder = DesignTokens.lightBorder;
  static const Color lightDivider = DesignTokens.lightBorder;

  // Dark Theme Colors — ink-green neutrals (warm brown ramp retired in #2441).
  static const Color darkPrimary =
      DesignTokens.darkPrimary; // Light sage for dark mode
  static const Color darkSecondary =
      DesignTokens.darkPrimaryHover; // Pale sage (beige retired in #2441)
  static const Color darkAccent = DesignTokens.lightPrimary; // Deep sage

  static const Color darkBackground = DesignTokens.darkBackground;
  static const Color darkCardBackground = DesignTokens.darkSurface;
  static const Color darkAppBarBackground = DesignTokens.darkBackground;

  /// Third neutral step for raised fills on cards (editable fields etc.) —
  /// the dark border token doubles as the elevation step above [darkCardBackground].
  static const Color darkSurface = DesignTokens.darkBorder;

  static const Color darkTextPrimary = DesignTokens.darkTextPrimary;
  static const Color darkTextSecondary = DesignTokens.darkTextSecondary;
  static const Color darkTextTertiary = DesignTokens.darkTextFaint;

  static const Color darkBorder = DesignTokens.darkBorder;
  static const Color darkDivider = DesignTokens.darkBorder;

  // Impact tab bar — inactive label color (legible but secondary)
    
  // QR / barcode colors — always black on white regardless of theme
  static const Color qrForeground = Colors.black;
  static const Color qrBackground = Colors.white;

  // Semantic status colors. Per-theme since #2441: a single shared value cannot
  // pass 4.5:1 on both a white page and a dark one.
  //
  // #2445 was that the shared consts pointed at the DARK token values and were
  // used in BOTH themes, so the light theme rendered status text at 1.86-2.76:1
  // while the correct light values sat unused in the palette:
  //
  //     success  #7A9B76 -> 2.76:1   correct #2E7041 -> 5.33:1
  //     warning  #E8A661 -> 1.86:1   correct #8A6200 -> 4.89:1
  //     error    #DB7F63 -> 2.59:1   correct #B5492B -> 4.73:1
  //     info     #7FA0A9 -> 2.49:1   correct #3E6471 -> 5.73:1
  //
  // (measured against the light surface #F2F2EE). The consts are gone, so the
  // compiler names every remaining call site rather than letting one slip
  // through silently.

  /// Success status, resolved for the ambient theme.
  static Color statusSuccess(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
          ? DesignTokens.lightSuccess
          : DesignTokens.darkSuccess;

  /// Warning status, resolved for the ambient theme.
  static Color statusWarning(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
          ? DesignTokens.lightWarning
          : DesignTokens.darkWarning;

  /// Error status, resolved for the ambient theme.
  static Color statusError(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
          ? DesignTokens.lightError
          : DesignTokens.darkError;

  /// Info status, resolved for the ambient theme.
  static Color statusInfo(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
          ? DesignTokens.lightInfo
          : DesignTokens.darkInfo;

  /// Status colours for surfaces that are dark regardless of app theme —
  /// glass sheets and media overlays. Use these ONLY where the surface is
  /// always dark; on an ordinary page they are the #2445 bug.
  static const Color statusSuccessOnDark = DesignTokens.darkSuccess;
  static const Color statusWarningOnDark = DesignTokens.darkWarning;
  static const Color statusErrorOnDark = DesignTokens.darkError;
  static const Color statusInfoOnDark = DesignTokens.darkInfo;

  /// Status colours for surfaces that are LIGHT regardless of app theme — the
  /// impact-report paper, which `PaperTokens` documents as opaque and
  /// light-only. Without these, a status colour on paper had to choose between
  /// a theme-aware getter (correct in light, ~2.7:1 on the cream card in dark)
  /// and an on-dark const (wrong in both). The surface is fixed, so the colour
  /// should be too.
  static const Color statusSuccessOnLight = DesignTokens.lightSuccess;
  static const Color statusWarningOnLight = DesignTokens.lightWarning;
  static const Color statusErrorOnLight = DesignTokens.lightError;
  static const Color statusInfoOnLight = DesignTokens.lightInfo;

  
  
  
  
  /// Light red text color used inside translucent-red error banners
  /// (e.g. inline form errors in transfer / fulfillment modals). Lighter
  /// than [error] so it reads against a red-tinted background fill.
  static const Color errorBannerText = Color(0xFFFF8A80);

  // `accentButtonBackground` (DesignTokens.lightSurface, #F2F2EE) was deleted in
  // #2798. It promised "a light fill whose text and border are the accent
  // color", which is the pairing that measured 1.79:1 — and 1.12:1 once #2764's
  // sweep moved the label onto the on-glass white ramp. A light-theme *surface*
  // token has no role inside the always-dark glass material. Selected chips use
  // modalChipBackgroundActive/modalChipTextActive; inline actions use
  // GlassInlineAction.

  // Chat bubble colors
  // Sage green - matches success color
  // Darker sage green for dark mode

  // ── Item type colours ────────────────────────────────────────────────
  //
  // These were four consts commented "shared between light/dark themes", and
  // the values were the DARK theme's ramp: giveaway was byte-for-byte
  // `dark.success`, request was `dark.warning`, experience was `dark.primary`.
  // On a dark surface that is correct. On a light one it is #2445 — the home
  // feed measured giveaway at **2.46:1** against a 4.5 requirement, and the
  // request accent read as an orange belonging to no theme because it was
  // `dark.warning` sitting on a light card.
  //
  // Same shape as the status colours above, for the same reason: resolve for
  // the ambient theme on surfaces that follow it, and take the `*OnDark` const
  // where the surface is glass or a hero and is dark whatever the app theme
  // says. Loan moves onto the `info` ramp (its old #5A7A82 was the one value
  // not drawn from the palette at all).

  /// Item type accents. Every surface that uses one of these is dark — hero
  /// content, glass sheets, media overlays — or uses it as a saturated fill
  /// behind an emoji, where the mid-tone reads on either theme. The name says
  /// so, because the previous name did not and the values silently became the
  /// dark ramp everywhere.
  ///
  /// If one of these is ever needed as a FOREGROUND on a theme-following
  /// surface, add a resolved accessor next to the status ones above rather than
  /// reaching for these — that is precisely the #2445 mistake.
  static const Color giveawayColorOnDark = DesignTokens.darkSuccess;
  static const Color loanColorOnDark = DesignTokens.darkInfo;
  static const Color experienceColorOnDark = DesignTokens.darkPrimary;

  /// Request accent.
  ///
  /// Was `dark.warning` — an amber reading as orange — and then briefly
  /// `dark.error`'s terracotta. Neither belongs: Ink & Sage has no warm hue,
  /// and the status ramp's warm end exists to mean "warning" and "error", not
  /// to supply decorative accents. It is the sage, like experience.
  ///
  /// That makes request and experience the same colour. Nothing today shows
  /// them side by side relying on hue to tell them apart — the feed thumbnail
  /// carries a per-type emoji and the mention badge a per-type icon — but a
  /// four-way Ink & Sage ramp for item types is the real answer if one is ever
  /// needed, not a fifth hue borrowed from status.
  static const Color requestColorOnDark = DesignTokens.darkPrimary;

  // Gradient Colors — deep→light sage, both ends from the token palette.
  static const List<Color> lightGradient = [
    DesignTokens.lightPrimary,
    DesignTokens.darkPrimary,
  ];

  static const List<Color> darkGradient = [
    DesignTokens.darkPrimary,
    DesignTokens.darkPrimaryHover,
  ];

  // Theme-aware color getters
  static Color primary(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightPrimary
        : darkPrimary;
  }

  /// Text and icons on a [primary] fill.
  ///
  /// Use this instead of `Colors.white` on a primary-filled button or chip.
  /// White is only correct in the light theme: dark-theme [primary] is light
  /// sage `#9DBFA8`, where white measures **2.01:1** — a WCAG failure — while
  /// this token measures 8.06:1. The palette has always defined the value; it
  /// simply had no accessor, so call sites reached for white and the dark
  /// theme silently paid for it.
  static Color onPrimary(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? DesignTokens.lightOnPrimary
        : DesignTokens.darkOnPrimary;
  }

  /// Text and icons on an [accent] fill. Same reasoning as [onPrimary].
  static Color onAccent(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? DesignTokens.lightOnAccent
        : DesignTokens.darkOnAccent;
  }

  static Color secondary(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightSecondary
        : darkSecondary;
  }

  static Color accent(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightAccent
        : darkAccent;
  }

  static Color background(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightBackground
        : darkBackground;
  }

  static Color cardBackground(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightCardBackground
        : darkCardBackground;
  }

  static Color appBarBackground(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightAppBarBackground
        : darkAppBarBackground;
  }

  static Color surface(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightSurface
        : darkSurface;
  }

  static Color textPrimary(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightTextPrimary
        : darkTextPrimary;
  }

  static Color textSecondary(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightTextSecondary
        : darkTextSecondary;
  }

  static Color textTertiary(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightTextTertiary
        : darkTextTertiary;
  }

  static Color border(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightBorder
        : darkBorder;
  }

  static Color divider(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightDivider
        : darkDivider;
  }

  static List<Color> gradient(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightGradient
        : darkGradient;
  }

  
  
  // Content-specific colors for editable fields
  static Color editableFieldBackground(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightSurface
        : darkSurface;
  }

  static Color editableFieldBorder(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightBorder
        : darkBorder;
  }

  static Color editableFieldText(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightTextPrimary
        : darkTextPrimary;
  }

  static Color editableFieldHint(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightTextTertiary
        : darkTextTertiary;
  }

  // ── Media-overlay material ──────────────────────────────────────────────────
  // #2770: values come from `OverlayTokens` (design/tokens.json). This material
  // is what gets painted onto a user's photo, and it is where the composited
  // gate found most of its failures — the shipped gradients fade to fully
  // transparent between their stops, leaving mid-hero text on bare media.
  //
  // Not theme-aware: the photo is the backdrop, and it does not follow the app
  // theme. `contentOverlay(context)` previously returned 30% in light and 50%
  // in dark for the same photo, which made the wash weakest exactly where the
  // media was brightest.

  /// Flat wash over media where no gradient applies.
  static Color contentOverlay(BuildContext context) => OverlayTokens.scrimFlat;

  /// Top of the hero gradient, behind the status bar and top controls.
  static const Color contentScrimTop = OverlayTokens.scrimTop;

  /// Bottom of the hero gradient, behind the title block and metadata.
  static const Color contentScrimBottom = OverlayTokens.scrimBottom;

  /// Minimum wash under any text on media — the floor that stops a gradient
  /// fading to nothing exactly where the text sits.
  static const Color contentScrimFloor = OverlayTokens.scrimFloor;

  // Floating Action Button colors
  // Overlay text field colors (for fields displayed over images/media)
  /// Text color for overlay text fields.
  static Color overlayFieldText() => OverlayTokens.textPrimary;

  /// Hint text color for overlay text fields.
  static Color overlayFieldHint() => OverlayTokens.textFaint;

  /// Background color for overlay text fields.
  static Color overlayFieldBackground() => OverlayTokens.fieldFill;

  /// Border color for overlay text fields (enabled state).
  static Color overlayFieldBorder() => OverlayTokens.outline;

  /// Border color for overlay text fields (focused state).
  static Color overlayFieldFocusedBorder() => OverlayTokens.textPrimary;

  // Metadata chip colors (for location, status chips over images/media)
  // Availability status colors (for manage/status pills)
  
  
  
  
  /// Warm beige background for experience modal (light-mode constant, use getter below)
  static const Color lightExperienceModalBackground = Color(0xFFF5F1ED);

  /// Sage green for primary actions (Start button, active timeline, selected RSVP)
  static const Color experienceSageGreen = Color(0xFF7A9B8C);

  /// Pale-sage text/glyph color used inside translucent "confirmation"
  /// chips (e.g. the `_RowCallToActionPill` "Voted"/"Helped"/"Chosen"
  /// state). Pairs with [experienceSageGreen] at 22% alpha for the fill
  /// and 35% alpha for the border — matches the redesign mockup in
  /// `docs/cowork/App Design/event-item-4.html`.
  static const Color experienceSageGreenSoftText = Color(0xFFC8D6BF);

  /// Fully-opaque white for text rendered over a darkened content-view
  /// background image. Theme-independent — the underlying surface is
  /// always a dark media + gradient, so this constant never changes
  /// with brightness. Prefer this over `Colors.white` so the intent
  /// (text on content image) is explicit at call sites.
  static const Color onContentImage = OverlayTokens.textPrimary;

  /// Secondary text over content media.
  ///
  /// #2770: use these rather than the palette's `darkText*` steps for anything
  /// drawn on media. The palette ramp is validated against `background` and
  /// `surface`; measured out on a photo the same values landed at 4.40:1 and
  /// 2.93:1, because the backdrop there is a composite the palette never saw.
  static const Color onContentImageSecondary = OverlayTokens.textSecondary;

  /// De-emphasised text over content media. Never body copy.
  static const Color onContentImageFaint = OverlayTokens.textFaint;

  /// Sage-green background for "Going" RSVP status badge.
  static Color rsvpYesBackground(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFE8F3E9)
      : const Color(0xFF1A2E1C);

  /// Sage-green text for "Going" RSVP status badge.
  static Color rsvpYesText(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFF3D7048)
      : const Color(0xFF8BC49A);

  /// Amber background for "Maybe" RSVP status badge.
  static Color rsvpMaybeBackground(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFFFF3E0)
      : const Color(0xFF2E2518);

  /// Amber text/icon color paired with rsvpMaybeBackground.
  static Color rsvpMaybeText(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFF57C00)
      : const Color(0xFFFFAB40);

  /// Red background for "Not Going" RSVP status badge.
  static Color rsvpNoBackground(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFFFEBEE)
      : const Color(0xFF2E1A17);

  /// Red text/icon color paired with rsvpNoBackground.
  static Color rsvpNoText(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFC62828)
      : const Color(0xFFEF9A9A);

  
  
  
  
  
  
  // Theme-aware experience colors.
  static Color experienceModalBackground(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? lightExperienceModalBackground
      : darkBackground;

  
  
  
    
  
  
  
  // Transfer modal colors — accent colors: semantic status colors, same in both themes.
  /// Accent sage for transfer/content primary actions — an **on-media
  /// accent**: the content views, chat glass, and manage sheets that consume
  /// this all sit over dark media or the glass scrim, so the value is the
  /// dark-theme primary token regardless of app theme (the deep light-theme
  /// sage melts into those backdrops — see #2441). Pair fills with a dark
  /// label ([darkBackground]), never white. Theme-following surfaces (e.g.
  /// portfolio inbox pills) must use [primary] instead. Token name is
  /// historical (was coral pre-#1946); rename to `transferAccent` tracked
  /// separately.
  static const Color transferCoral = DesignTokens.darkPrimary;

  /// Soft on-media accent sage. Token name is historical; rename tracked
  /// separately.
  static const Color transferCoralSoft = DesignTokens.darkPrimaryHover;

  /// Sage green for transfer "accepted / confirmed" states. Visually close to
  /// the new brand accent (`lightPrimary` is also a sage); design review is
  /// scheduled to determine whether this needs to shift to a deeper green for
  /// clearer state separation. See #1946.
  static const Color transferSage = Color(0xFF6B8F71);

  
  // Theme-aware transfer colors — use these instead of the light-only consts.
  
  
  static Color transferSageBackground(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFE8F3E9)
      : const Color(0xFF1A2E1C);

  static Color transferTextPrimary(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFF2D2A26)
      : darkTextPrimary;

  static Color transferTextSecondary(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFF7A756D)
      : darkTextSecondary;

  static Color transferTextMuted(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFA9A49C)
      : darkTextTertiary;

  static Color transferBorder(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFEDE8E0)
      : darkBorder;

  static Color transferBorderLight(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFF3EFE8)
      : darkDivider;

  static Color transferCardBackground(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFFEFBF5)
      : darkCardBackground;

  static Color transferHighlight(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFFFF3E6)
      : const Color(0xFF2E2518);

  // ── Chat glass surface colors ────────────────────────────────────────────────
  // Values from the chat2 reference mockup for consistent glass surfaces.

  // rgba(255,255,255,0.7)

  // rgba(42,39,35,0.85)

  // rgba(255,255,255,0.85)

  // rgba(42,39,35,0.88)

  // rgba(253,248,240,0.93)

  // rgba(28,26,23,0.95)

  /// Background for the text input field in the bottom bar (light mode).
  static const Color lightInputFieldBg = Color(0x26000000); // rgba(0,0,0,0.15)

  /// Background for the text input field in the bottom bar (dark mode).
  static const Color darkInputFieldBg = Color(
    0x33FFFFFF,
  ); // rgba(255,255,255,0.2)

  // rgba(255,255,255,0.?25)

  // rgba(0,0,0,0.25)

  // Gradient stop colors for the chat background readability gradient.

  
  
  
  
  
  
  
  
  /// inputFieldBg returns the themed background for the chat input text field.
  static Color inputFieldBg(BuildContext context) {
    return Theme.of(context).brightness == Brightness.light
        ? lightInputFieldBg
        : darkInputFieldBg;
  }

  
  
  // ── Modal glass surface colors ───────────────────────────────────────────────
  // Frosted-glass material for bottom-sheet modals. Identical in light and
  // dark modes by design — the sheet sits on a dark scrim that handles theme
  // adaptation, and on-glass content is white-on-translucent in both modes.
  // See docs/issues/1797-glass-modal-revamp.md for the rationale.
  //
  // #2770: values now come from `GlassTokens` (design/tokens.json), so the
  // material is a swappable, gate-checked token set instead of ~40 hand-tuned
  // constants outside every gate. The `modal*` names below are kept as the
  // semantic layer widgets call; several that used to hold byte-identical
  // values now correctly resolve to ONE canonical token — the previous set had
  // seven names for solid white and three for the sheet fill.

  /// Backdrop scrim drawn behind the glass sheet.
  static const Color modalBackdrop = GlassTokens.scrim;

  /// Sigma value passed to BackdropFilter for the scrim blur (logical pixels).
  static const double modalBackdropBlurSigma = GlassTokens.backdropBlurSigma;

  /// Translucent white fill of the glass sheet.
  static const Color modalSurface = GlassTokens.surface;

  /// Darkening layer painted between the sheet's backdrop blur and its
  /// content. Opt-in via `GlassSheet.scrim` for sheets shown over bright media,
  /// where the blurred-through content would otherwise wash out on-glass text.
  static const Color modalContentScrim = GlassTokens.scrim;

  /// Stronger media scrim for full-screen content panels shown over a hero
  /// image/video, where more dimming is needed than [modalContentScrim] to keep
  /// the panel legible.
  static const Color modalContentScrimStrong = GlassTokens.scrimHeavy;

  /// Sigma value passed to BackdropFilter inside the sheet.
  static const double modalSurfaceBlurSigma = GlassTokens.blurSigma;

  /// Outer border of the glass sheet.
  static const Color modalBorder = GlassTokens.border;

  /// Internal divider/border. Now distinct from [modalSurface] — it previously
  /// held the same value as the surface it divides.
  static const Color modalBorderSubtle = GlassTokens.divider;

  /// Drag handle pill at the top of the sheet.
  static const Color modalDragHandle = GlassTokens.dragHandle;

  /// Primary on-glass text.
  static const Color modalTextPrimary = GlassTokens.textPrimary;

  /// Secondary on-glass text.
  static const Color modalTextSecondary = GlassTokens.textSecondary;

  /// Tertiary on-glass text.
  static const Color modalTextTertiary = GlassTokens.textSecondary;

  /// Muted on-glass text. Was solid white — i.e. it de-emphasised nothing.
  static const Color modalTextMuted = GlassTokens.textFaint;

  /// Header icon-badge background.
  static const Color modalIconBadgeBackground = GlassTokens.fillSubtle;

  /// Header icon-badge border.
  static const Color modalIconBadgeBorder = GlassTokens.divider;

  /// Inactive chip background.
  static const Color modalChipBackground = GlassTokens.fillSubtle;

  /// Inactive chip border.
  static const Color modalChipBorder = GlassTokens.divider;

  /// Inactive chip text — primary line.
  static const Color modalChipText = GlassTokens.textSecondary;

  /// Inactive chip text — secondary line.
  static const Color modalChipTextSecondary = GlassTokens.textFaint;

  /// Active chip background.
  static const Color modalChipBackgroundActive = GlassTokens.fillStrong;

  /// Active chip border.
  static const Color modalChipBorderActive = GlassTokens.fillStrong;

  /// Active chip text — primary line.
  static const Color modalChipTextActive = GlassTokens.onFillStrong;

  /// Active chip text — secondary line.
  static const Color modalChipTextSecondaryActive = Color(0x8C1A1A1A);

  /// Inline action row background.
  static const Color modalInlineActionBackground = GlassTokens.fillSubtle;

  /// Inline action row border.
  static const Color modalInlineActionBorder = GlassTokens.divider;

  /// Inline action row text.
  static const Color modalInlineActionText = GlassTokens.textSecondary;

  /// Inline action row chevron.
  static const Color modalInlineActionChevron = GlassTokens.textFaint;

  /// Footer divider above buttons. Distinct from [modalSurface] now — it
  /// previously held the same value as the sheet it sits on.
  static const Color modalFooterDivider = GlassTokens.divider;

  /// Secondary (cancel) button background.
  ///
  /// Darker than the surrounding [modalSurface] so white text gets real
  /// contrast on the pill. A previous 10%-white fill made the button
  /// near-invisible against the glass — see #1934 for the contrast math.
  static const Color modalSecondaryButtonBackground = GlassTokens.secondaryFill;

  /// Secondary (cancel) button border. The brighter outline keeps the pill
  /// shape unambiguous against the darker fill.
  static const Color modalSecondaryButtonBorder = GlassTokens.secondaryBorder;

  /// Secondary (cancel) button text.
  static const Color modalSecondaryButtonText = GlassTokens.textSecondary;

  /// Primary (save) button background.
  ///
  /// **Fill only.** White on this colour clears 4.5:1 comfortably, which is why
  /// the value was chosen (#1934, #1946, #2441). It is NOT a foreground: as a
  /// `TextButton` foreground on the glass sheet it measures 1.01:1, which is
  /// #2764. `glass_pickers.dart` does exactly that via `ColorScheme.primary`.
  static const Color modalPrimaryButtonBackground = GlassTokens.primary;

  /// Primary (save) button text.
  static const Color modalPrimaryButtonText = GlassTokens.onPrimary;

  /// Search-input field background on the glass sheet.
  static const Color modalSearchFieldBackground = GlassTokens.fillSubtle;

  /// Search-input field border on the glass sheet.
  static const Color modalSearchFieldBorder = GlassTokens.divider;

  /// Solid cream pill background for the primary "What's needed?" input
  /// on the request/offer batch sheets. Mirrors the HTML reference
  /// (#F4EFE4) so the input reads as a distinct cream chip against the
  /// dark glass surface instead of washing out as 8% white.
  static const Color modalSheetInputBackground = Color(0xFFF4EFE4);

  /// Primary text color inside the cream input pill (near-black).
  static const Color modalSheetInputText = Color(0xFF1A1A1A);

  /// Muted placeholder color inside the cream input pill (warm brown
  /// at ~55% opacity, matching the mock).
  static const Color modalSheetInputPlaceholder = Color(0x8C5A4A3D);

  
  /// Inset-card surface used by [GlassInsetCard] — a flat (non-blurred)
  /// glass-tinted card meant to sit inside a [GlassSheet]. Stacking
  /// another [BackdropFilter] inside the sheet causes recursive-blur
  /// ring artifacts on iOS+Metal, so [GlassInsetCard] uses this flat
  /// alpha fill instead.
  ///
  /// White @ 8% alpha.
  static const Color modalInsetCardBg = Color(0x14FFFFFF);

  /// Hairline border for [GlassInsetCard]. White @ 12% alpha.
  static const Color modalInsetCardBorder = Color(0x1FFFFFFF);

  // ── Needs picker suggestion-grid category tints ──────────────────────────────
  // Theme-aware tints for the four suggestion categories shown in the
  // Needs picker (gear / food-drink / hands-help / personal). Each pair
  // returns a soft fill suitable for chip backgrounds plus an accent
  // suitable for the leading icon / text. Tints are kept distinct from
  // the existing item-type colors (giveaway / loan / request / community)
  // so the picker reads as its own information layer.

  static Color needsCategoryGearBg(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFE6EEF5)
      : const Color(0xFF1F2A33);
  static Color needsCategoryGearAccent(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFF3D6B78)
      : const Color(0xFF8BB8C8);

  static Color needsCategoryFoodBg(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFFCEFE0)
      : const Color(0xFF2E2418);
  static Color needsCategoryFoodAccent(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFB67839)
      : const Color(0xFFE8A661);

  static Color needsCategoryHelpBg(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFE8F3E9)
      : const Color(0xFF1A2E1C);
  static Color needsCategoryHelpAccent(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFF3D7048)
      : const Color(0xFF8BC49A);

  static Color needsCategoryPersonalBg(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFFF0E6F5)
      : const Color(0xFF2A1F33);
  static Color needsCategoryPersonalAccent(BuildContext context) =>
      Theme.of(context).brightness == Brightness.light
      ? const Color(0xFF7B68BE)
      : const Color(0xFFB4A3DC);
  }

/// ImpactModalColors provides the warm paper-toned palette used in the
/// Calculation/Inputs detail modals for Money Saved, CO₂ Avoided, and Quality Time.
/// All values are light-mode primaries; callers should use [ImpactModalColors.adaptForContext]
/// when dark-mode support is needed.
class ImpactModalColors {
  ImpactModalColors._();

  // Metric accent colours. Token names `coral` / `coralBg` are historical
  // (pre-#1946) and now hold the sage accent; follow-up issue tracks
  // renaming to `accent` / `accentBg`.
  static const Color coral = DesignTokens.lightPrimary;
  static const Color coralBg = Color(0xFFDDE6DC);
  static const Color green = Color(0xFF6B8758);
  static const Color greenBg = Color(0xFFE5EDD8);

  // CalcCard surface
  static const Color calcBg = Color(0xFFF5EDE0);
  static const Color calcLine = Color(0xFFEAD9C1);
  static const Color chipBg = Color(0xFFF0E8DA);
  static const Color line = Color(0xFFE6DFD3);
  
  // Input chip borders
  static const Color inputBorder = Color(0xFFD8CCB4);
  static const Color inputBorderFocus = Color(0xFFB98B5E);

  // Editable input pill — slightly lighter than calcBg so the tap target
  // reads as distinct from the surrounding card.
  static const Color inputFieldBg = Color(0xFFFCF7EC);

  // Text
  static const Color ink = Color(0xFF1F1C19);
  static const Color inkSoft = Color(0xFF5E574F);
  static const Color inkFade = Color(0xFF95897A);

  // Sheet surface
  static const Color sheetBg = Color(0xFFFFFFFF);
  static const Color hintBg = Color(0xFFFAF3E4);
}
