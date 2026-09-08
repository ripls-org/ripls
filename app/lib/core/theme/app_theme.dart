import 'package:flutter/material.dart';
import 'app_colors.dart';
import 'gen/design_tokens.gen.dart';

class AppTheme {
  // Font families. The serif comes from the design tokens (#2441); the body
  // font realizes the sans token via its platform fallback — the app ships
  // the system sans (SF Pro on iOS, Roboto on Android) rather than bundling
  // the web-side family, per the fallback stack in design/tokens.json.
  static const String headingFont = DesignTokens.serifFamily;
  static const String bodyFont = 'SF Pro Text'; // System default sans-serif
  // Community name header style — used in FloatingHeader for community name text.
  // No fontFamily specified — uses the system sans-serif (SF Pro on iOS,
  // Roboto on Android). Color is intentionally omitted so callers apply
  // AppColors.textPrimary(context), preserving light/dark theme support.
  static const TextStyle communityHeaderStyle = TextStyle(
    fontSize: 15.5,
    fontWeight: FontWeight.w600,
    letterSpacing: -0.3,
  );

  // Conversation title style — used in ConversationTopBar for the item title.
  // Matches communityHeaderStyle to keep header typography consistent across
  // screens. Color is omitted so callers can adapt to light/dark theme.
  
  // Nudge card text styles — always rendered over dark imagery backgrounds.
  // Colour is baked in (white) since nudge cards are always dark-backgrounded.
  static const TextStyle nudgeHeadlineLargeStyle = TextStyle(
    fontFamily: headingFont,
    fontSize: 32,
    fontWeight: FontWeight.w400,
    color: Colors.white,
    height: 1.15,
    letterSpacing: -0.3,
  );

  static const TextStyle nudgeHeadlineMediumStyle = TextStyle(
    fontFamily: headingFont,
    fontSize: 26,
    fontWeight: FontWeight.w400,
    color: Colors.white,
    height: 1.15,
  );

  static const TextStyle nudgeBodyStyle = TextStyle(
    fontFamily: headingFont,
    fontSize: 16,
    fontWeight: FontWeight.w400,
    color: Colors.white,
    height: 1.6,
  );

  static const TextStyle nudgeStatValueStyle = TextStyle(
    fontFamily: headingFont,
    fontSize: 22,
    fontWeight: FontWeight.w700,
    color: Colors.white,
  );

  // Hero metric text styles — used in the dark hero header on impact screens
  // (ItemMetricsScreen and CommunityMetricsScreen) to ensure visual consistency.
  static const TextStyle heroMetricValueStyle = TextStyle(
    fontFamily: headingFont,
    fontSize: 18,
    fontWeight: FontWeight.w700,
    color: Colors.white,
    height: 1.2,
  );

  /// The hero is always dark (see [heroMetricValueStyle]), so the embedded
  /// color is the on-dark accent shade ([AppColors.lightAccent], the lighter
  /// sage variant) — chosen so the label clears contrast against the dark
  /// hero background. The previous coral palette had its primary shade in the
  /// mid-luminance range and read fine on dark; Heritage Sage's primary is
  /// darker (L*≈51), so the lighter accent is used here instead. Callers that
  /// `.copyWith(color: ...)` are unaffected.
  static const TextStyle heroMetricLabelStyle = TextStyle(
    fontSize: 10,
    fontWeight: FontWeight.w600,
    letterSpacing: 0.5,
    color: AppColors.lightAccent,
    height: 1.2,
  );
  static ThemeData get lightTheme => ThemeData(
    useMaterial3: true,
    brightness: Brightness.light,
    colorScheme: const ColorScheme.light(
      primary: AppColors.lightPrimary,
      secondary: AppColors.lightSecondary,
      tertiary: AppColors.lightAccent,
      surface: AppColors.lightCardBackground,
      error: DesignTokens.lightError,
      onPrimary: Colors.white,
      onSecondary: AppColors.lightTextPrimary,
      onSurface: AppColors.lightTextPrimary,
      onError: Colors.white,
    ),
    scaffoldBackgroundColor: AppColors.lightBackground,
    appBarTheme: const AppBarTheme(
      backgroundColor: AppColors.lightAppBarBackground,
      elevation: 0,
      centerTitle: true,
      titleTextStyle: TextStyle(
        color: AppColors.lightTextPrimary,
        fontSize: 20,
        fontWeight: FontWeight.w400,
        letterSpacing: 0.5,
      ),
      iconTheme: IconThemeData(color: AppColors.lightTextPrimary),
    ),
    cardTheme: const CardThemeData(
      color: AppColors.lightCardBackground,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(30)),
      ),
      margin: EdgeInsets.symmetric(horizontal: 16, vertical: 8),
    ),
    bottomSheetTheme: const BottomSheetThemeData(
      backgroundColor: AppColors.lightCardBackground,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
    ),
    bottomNavigationBarTheme: const BottomNavigationBarThemeData(
      backgroundColor: AppColors.lightCardBackground,
      selectedItemColor: AppColors.lightPrimary,
      unselectedItemColor: AppColors.lightTextSecondary,
      elevation: 8,
      type: BottomNavigationBarType.fixed,
    ),
    floatingActionButtonTheme: const FloatingActionButtonThemeData(
      backgroundColor: AppColors.lightPrimary,
      foregroundColor: Colors.white,
      elevation: 4,
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: AppColors.lightPrimary,
        foregroundColor: DesignTokens.lightOnPrimary,
        elevation: 0,
        padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 16),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(50)),
        textStyle: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: AppColors.lightPrimary,
        side: const BorderSide(color: AppColors.lightBorder, width: 2),
        padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 16),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(50)),
        textStyle: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: AppColors.lightSurface,
      labelStyle: const TextStyle(
        color: AppColors.lightTextSecondary,
        fontSize: 14,
      ),
      hintStyle: const TextStyle(
        color: AppColors.lightTextTertiary,
        fontSize: 14,
      ),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.lightBorder, width: 0.5),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.lightBorder, width: 0.5),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.lightPrimary, width: 1),
      ),
      errorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: DesignTokens.lightError, width: 1),
      ),
      focusedErrorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: DesignTokens.lightError, width: 1),
      ),
      isDense: true,
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
    ),
    datePickerTheme: const DatePickerThemeData(
      backgroundColor: AppColors.lightCardBackground,
      headerBackgroundColor: AppColors.lightPrimary,
      headerForegroundColor: Colors.white,
      dayForegroundColor: WidgetStatePropertyAll(AppColors.lightTextPrimary),
      todayForegroundColor: WidgetStatePropertyAll(AppColors.lightPrimary),
      todayBackgroundColor: WidgetStatePropertyAll(Colors.transparent),
    ),
    timePickerTheme: const TimePickerThemeData(
      backgroundColor: AppColors.lightCardBackground,
      hourMinuteColor: AppColors.lightSurface,
      hourMinuteTextColor: AppColors.lightTextPrimary,
      dialBackgroundColor: AppColors.lightSurface,
      dialHandColor: AppColors.lightPrimary,
      dialTextColor: AppColors.lightTextPrimary,
      entryModeIconColor: AppColors.lightTextSecondary,
    ),
    dividerTheme: const DividerThemeData(
      color: AppColors.lightDivider,
      thickness: 1,
      space: 1,
    ),
    // One surface for every toast. ~120 call sites raise a bare
    // `SnackBar(content: ...)` and took the Material default, while
    // `ToastHelper`'s status variants filled the whole bar with green or red;
    // a single flow could show three toasts that looked like three different
    // components. Status is carried by the icon now, so this is the only
    // snack-bar appearance in the app.
    snackBarTheme: SnackBarThemeData(
      backgroundColor: AppColors.lightSurface,
      contentTextStyle: const TextStyle(
        color: AppColors.lightTextPrimary,
        fontSize: 14,
        fontWeight: FontWeight.w500,
      ),
      actionTextColor: AppColors.lightPrimary,
      behavior: SnackBarBehavior.floating,
      insetPadding: const EdgeInsets.all(16),
      elevation: 3,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: const BorderSide(color: AppColors.lightBorder),
      ),
    ),
    textTheme: const TextTheme(
      displayLarge: TextStyle(
        fontFamily: headingFont,
        color: AppColors.lightTextPrimary,
        fontSize: 68,
        fontWeight: FontWeight.w400,
        height: 1.3,
      ),
      displayMedium: TextStyle(
        fontFamily: headingFont,
        color: AppColors.lightTextPrimary,
        fontSize: 42,
        fontWeight: FontWeight.w400,
      ),
      displaySmall: TextStyle(
        fontFamily: headingFont,
        color: AppColors.lightTextPrimary,
        fontSize: 38,
        fontWeight: FontWeight.w400,
      ),
      headlineLarge: TextStyle(
        fontFamily: headingFont,
        color: AppColors.lightTextPrimary,
        fontSize: 32,
        fontWeight: FontWeight.w400,
      ),
      headlineMedium: TextStyle(
        fontFamily: headingFont,
        color: AppColors.lightTextPrimary,
        fontSize: 24,
        fontWeight: FontWeight.w500,
      ),
      headlineSmall: TextStyle(
        fontFamily: headingFont,
        color: AppColors.lightTextPrimary,
        fontSize: 20,
        fontWeight: FontWeight.w500,
      ),
      titleLarge: TextStyle(
        fontFamily: headingFont,
        color: AppColors.lightTextPrimary,
        fontSize: 22,
        fontWeight: FontWeight.w400,
        height: 1.7,
      ),
      titleMedium: TextStyle(
        fontFamily: headingFont,
        color: AppColors.lightTextPrimary,
        fontSize: 16,
        fontWeight: FontWeight.w500,
      ),
      bodyLarge: TextStyle(
        color: AppColors.lightTextSecondary,
        fontSize: 16,
        fontWeight: FontWeight.w400,
        height: 1.6,
      ),
      bodyMedium: TextStyle(
        color: AppColors.lightTextSecondary,
        fontSize: 15,
        fontWeight: FontWeight.w400,
        height: 1.6,
      ),
      bodySmall: TextStyle(
        color: AppColors.lightTextTertiary,
        fontSize: 14,
        fontWeight: FontWeight.w400,
      ),
      labelLarge: TextStyle(
        color: AppColors.lightTextPrimary,
        fontSize: 17,
        fontWeight: FontWeight.w600,
      ),
    ),
  );

  static ThemeData get darkTheme => ThemeData(
    useMaterial3: true,
    brightness: Brightness.dark,
    colorScheme: const ColorScheme.dark(
      primary: AppColors.darkPrimary,
      secondary: AppColors.darkSecondary,
      tertiary: AppColors.darkAccent,
      surface: AppColors.darkCardBackground,
      error: DesignTokens.darkError,
      onPrimary: AppColors.darkTextPrimary,
      onSecondary: AppColors.darkTextPrimary,
      onSurface: AppColors.darkTextPrimary,
      onError: Colors.white,
    ),
    scaffoldBackgroundColor: AppColors.darkBackground,
    appBarTheme: const AppBarTheme(
      backgroundColor: AppColors.darkAppBarBackground,
      elevation: 0,
      centerTitle: true,
      titleTextStyle: TextStyle(
        color: AppColors.darkTextPrimary,
        fontSize: 20,
        fontWeight: FontWeight.w400,
        letterSpacing: 0.5,
      ),
      iconTheme: IconThemeData(color: AppColors.darkTextPrimary),
    ),
    cardTheme: const CardThemeData(
      color: AppColors.darkCardBackground,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(30)),
      ),
      margin: EdgeInsets.symmetric(horizontal: 16, vertical: 8),
    ),
    bottomSheetTheme: const BottomSheetThemeData(
      backgroundColor: AppColors.darkCardBackground,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
    ),
    bottomNavigationBarTheme: const BottomNavigationBarThemeData(
      backgroundColor: AppColors.darkCardBackground,
      selectedItemColor: AppColors.darkPrimary,
      unselectedItemColor: AppColors.darkTextSecondary,
      elevation: 8,
      type: BottomNavigationBarType.fixed,
    ),
    floatingActionButtonTheme: const FloatingActionButtonThemeData(
      backgroundColor: AppColors.darkPrimary,
      foregroundColor: AppColors.darkTextPrimary,
      elevation: 4,
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: AppColors.darkPrimary,
        // `darkOnPrimary`, not `darkTextPrimary`. The dark theme's primary is a
        // LIGHT sage, so the page's near-white body colour on top of it
        // measured 1.82:1 — every ElevatedButton in dark mode, the phone
        // verification flow among them. `on-primary` is the token that exists
        // to answer "what goes on primary", and it measures 8.06:1.
        foregroundColor: DesignTokens.darkOnPrimary,
        elevation: 0,
        padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 16),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(50)),
        textStyle: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: AppColors.darkPrimary,
        side: const BorderSide(color: AppColors.darkBorder, width: 2),
        padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 16),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(50)),
        textStyle: const TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: AppColors.darkSurface,
      labelStyle: const TextStyle(
        color: AppColors.darkTextSecondary,
        fontSize: 14,
      ),
      hintStyle: const TextStyle(
        color: AppColors.darkTextTertiary,
        fontSize: 14,
      ),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.darkBorder, width: 0.5),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.darkBorder, width: 0.5),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: AppColors.darkPrimary, width: 1),
      ),
      errorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: DesignTokens.darkError, width: 1),
      ),
      focusedErrorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: DesignTokens.darkError, width: 1),
      ),
      isDense: true,
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
    ),
    datePickerTheme: const DatePickerThemeData(
      backgroundColor: AppColors.darkCardBackground,
      headerBackgroundColor: AppColors.darkSurface,
      headerForegroundColor: AppColors.darkTextPrimary,
      dayForegroundColor: WidgetStatePropertyAll(AppColors.darkTextPrimary),
      todayForegroundColor: WidgetStatePropertyAll(AppColors.darkPrimary),
      todayBackgroundColor: WidgetStatePropertyAll(Colors.transparent),
    ),
    timePickerTheme: const TimePickerThemeData(
      backgroundColor: AppColors.darkCardBackground,
      hourMinuteColor: AppColors.darkSurface,
      hourMinuteTextColor: AppColors.darkTextPrimary,
      dialBackgroundColor: AppColors.darkSurface,
      dialHandColor: AppColors.darkPrimary,
      dialTextColor: AppColors.darkTextPrimary,
      entryModeIconColor: AppColors.darkTextSecondary,
    ),
    dividerTheme: const DividerThemeData(
      color: AppColors.darkDivider,
      thickness: 1,
      space: 1,
    ),
    // See the light theme's snackBarTheme — one surface for every toast.
    snackBarTheme: SnackBarThemeData(
      backgroundColor: AppColors.darkSurface,
      contentTextStyle: const TextStyle(
        color: AppColors.darkTextPrimary,
        fontSize: 14,
        fontWeight: FontWeight.w500,
      ),
      actionTextColor: AppColors.darkPrimary,
      behavior: SnackBarBehavior.floating,
      insetPadding: const EdgeInsets.all(16),
      elevation: 3,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: const BorderSide(color: AppColors.darkBorder),
      ),
    ),
    textTheme: const TextTheme(
      displayLarge: TextStyle(
        fontFamily: headingFont,
        color: AppColors.darkTextPrimary,
        fontSize: 68,
        fontWeight: FontWeight.w400,
        height: 1.3,
      ),
      displayMedium: TextStyle(
        fontFamily: headingFont,
        color: AppColors.darkTextPrimary,
        fontSize: 42,
        fontWeight: FontWeight.w400,
      ),
      displaySmall: TextStyle(
        fontFamily: headingFont,
        color: AppColors.darkTextPrimary,
        fontSize: 38,
        fontWeight: FontWeight.w400,
      ),
      headlineLarge: TextStyle(
        fontFamily: headingFont,
        color: AppColors.darkTextPrimary,
        fontSize: 32,
        fontWeight: FontWeight.w400,
      ),
      headlineMedium: TextStyle(
        fontFamily: headingFont,
        color: AppColors.darkTextPrimary,
        fontSize: 24,
        fontWeight: FontWeight.w500,
      ),
      headlineSmall: TextStyle(
        fontFamily: headingFont,
        color: AppColors.darkTextPrimary,
        fontSize: 20,
        fontWeight: FontWeight.w500,
      ),
      titleLarge: TextStyle(
        fontFamily: headingFont,
        color: AppColors.darkTextPrimary,
        fontSize: 22,
        fontWeight: FontWeight.w400,
        height: 1.7,
      ),
      titleMedium: TextStyle(
        fontFamily: headingFont,
        color: AppColors.darkTextPrimary,
        fontSize: 16,
        fontWeight: FontWeight.w500,
      ),
      bodyLarge: TextStyle(
        color: AppColors.darkTextSecondary,
        fontSize: 16,
        fontWeight: FontWeight.w400,
        height: 1.6,
      ),
      bodyMedium: TextStyle(
        color: AppColors.darkTextSecondary,
        fontSize: 15,
        fontWeight: FontWeight.w400,
        height: 1.6,
      ),
      bodySmall: TextStyle(
        color: AppColors.darkTextTertiary,
        fontSize: 14,
        fontWeight: FontWeight.w400,
      ),
      labelLarge: TextStyle(
        color: AppColors.darkTextPrimary,
        fontSize: 17,
        fontWeight: FontWeight.w600,
      ),
    ),
  );
}

/// ModalTheme groups layout, radius, and typography constants for the
/// frosted-glass bottom-sheet modal material. Colors live in [AppColors]
/// (`modal*`); this class holds the geometry and text styles so each glass
/// primitive pulls from a single source of truth.
///
/// See docs/issues/1797-glass-modal-revamp.md for the rationale.
class ModalTheme {
  ModalTheme._();

  /// Top-corner radius of the glass sheet.
  static const double sheetTopRadius = 28;

  /// Horizontal inset of the sheet from the screen edges.
  static const double sheetHorizontalInset = 12;

  /// Maximum height of the sheet as a fraction of screen height.
  static const double sheetMaxHeightFraction = 0.92;

  /// Drag handle dimensions.
  static const double dragHandleWidth = 40;
  static const double dragHandleHeight = 4;

  /// Header icon-badge size.
  static const double headerIconBadgeSize = 40;

  /// Chip corner radius and minimum vertical padding.
  static const double chipRadius = 16;
  static const double chipPaddingVertical = 8;
  static const double chipPaddingHorizontal = 8;

  /// Inline-action row radius and height.
  static const double inlineActionRadius = 16;
  static const double inlineActionHeight = 44;

  /// Footer button height and radius (fully rounded pill).
  static const double buttonHeight = 44;

  /// Press-feedback scale used on chips and buttons.
  static const double pressScale = 0.97;

  /// Press-animation duration. Wrap with `accessibleDuration(context, ...)`
  /// at the call site so reduce-motion is respected.
  static const Duration pressDuration = Duration(milliseconds: 120);

  
  /// UPPERCASE kicker label above the header value (e.g. "WHEN").
  /// Color must be applied at the call site via `AppColors.modalTextMuted`.
  static const TextStyle kickerStyle = TextStyle(
    fontSize: 10,
    fontWeight: FontWeight.w700,
    letterSpacing: 1.6, // ≈ 0.16em on 10px
    height: 1.2,
  );

  /// UPPERCASE field label (e.g. "DATE", "START TIME"). Same shape as kicker
  /// but rendered in a slightly higher-contrast white (`modalTextTertiary`).
  static const TextStyle fieldLabelStyle = TextStyle(
    fontSize: 10,
    fontWeight: FontWeight.w700,
    letterSpacing: 1.6,
    height: 1.2,
  );

  /// Header value (e.g. "Fri, May 8 · 9:00 AM · 1 hr").
  static const TextStyle headerValueStyle = TextStyle(
    fontSize: 15.5,
    fontWeight: FontWeight.w600,
    height: 1.25,
  );

  /// Chip primary line.
  static const TextStyle chipPrimaryStyle = TextStyle(
    fontSize: 13,
    fontWeight: FontWeight.w600,
    height: 1.2,
  );

  /// Chip secondary line.
  static const TextStyle chipSecondaryStyle = TextStyle(
    fontSize: 10.5,
    fontWeight: FontWeight.w500,
    height: 1.2,
  );

  /// Inline action row text.
  static const TextStyle inlineActionTextStyle = TextStyle(
    fontSize: 13,
    fontWeight: FontWeight.w500,
    height: 1.2,
  );

  /// Footer button text.
  ///
  /// 14pt bold meets the WCAG large-text threshold so the primary CTA's
  /// white-on-coral text clears the 3:1 large-text contrast minimum. See
  /// #1934.
  static const TextStyle buttonTextStyle = TextStyle(
    fontSize: 14,
    fontWeight: FontWeight.w700,
    height: 1.2,
  );
}
