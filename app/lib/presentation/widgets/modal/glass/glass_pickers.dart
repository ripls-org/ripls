import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// Builds a [ThemeData] that re-skins Flutter's [showDatePicker] and
/// [showTimePicker] dialogs to match the modal-glass material — translucent
/// white surfaces, white-on-glass text, no warm/coral tint anywhere.
///
/// We deliberately avoid [ColorScheme.fromSeed]: M3 derives a dozen
/// surface-container shades from the seed, all of which tint the picker's
/// internal widgets if the seed is the brand coral. Building the scheme
/// from scratch keeps every visible surface on the same `modalSurface`
/// translucent-white token used by the rest of the glass modals.
ThemeData _glassPickerTheme(BuildContext context) {
  const onSurface = AppColors.modalTextPrimary;
  // Same translucent-white surface used by the time modal's GlassSheet —
  // composes with the heavy backdrop blur in [_glassWrap] to read as
  // frosted glass rather than a flat white panel.
  const glassSurface = AppColors.modalSurface;

  // Build a complete dark ColorScheme without going through fromSeed so
  // none of the surface-container slots inherit a coral tint.
  //
  // #2764: `primary`, `secondary`, `tertiary` and `inversePrimary` were set to
  // the deep-sage BUTTON FILL. Material resolves those slots as the foreground
  // for text buttons, so Cancel/OK and today's date were painted in a dark
  // brand colour directly on the dark sheet — measured at 1.01:1, i.e.
  // invisible, which is exactly what the reporter photographed. The fill is
  // still correct for FILLED slots (`dayBackgroundColor` below); the foreground
  // slots take the on-glass text token instead.
  const scheme = ColorScheme(
    brightness: Brightness.dark,
    primary: AppColors.modalTextPrimary,
    onPrimary: AppColors.modalChipTextActive,
    primaryContainer: glassSurface,
    onPrimaryContainer: onSurface,
    secondary: AppColors.modalTextPrimary,
    onSecondary: AppColors.modalChipTextActive,
    secondaryContainer: AppColors.modalChipBackgroundActive,
    onSecondaryContainer: AppColors.modalChipTextActive,
    tertiary: AppColors.modalTextPrimary,
    onTertiary: AppColors.modalChipTextActive,
    error: AppColors.statusErrorOnDark,
    onError: Colors.white,
    surface: glassSurface,
    onSurface: onSurface,
    surfaceTint: Colors.transparent,
    surfaceContainerLowest: glassSurface,
    surfaceContainerLow: glassSurface,
    surfaceContainer: glassSurface,
    surfaceContainerHigh: glassSurface,
    surfaceContainerHighest: glassSurface,
    surfaceBright: glassSurface,
    surfaceDim: glassSurface,
    onSurfaceVariant: AppColors.modalTextSecondary,
    outline: AppColors.modalBorder,
    outlineVariant: AppColors.modalBorderSubtle,
    inverseSurface: glassSurface,
    onInverseSurface: onSurface,
    inversePrimary: AppColors.modalTextPrimary,
    shadow: Colors.black,
    scrim: Colors.black,
  );

  return ThemeData(
    useMaterial3: true,
    brightness: Brightness.dark,
    colorScheme: scheme,
    canvasColor: Colors.transparent,
    scaffoldBackgroundColor: Colors.transparent,
    dialogTheme: const DialogThemeData(
      backgroundColor: Colors.transparent,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(24)),
      ),
    ),
    datePickerTheme: DatePickerThemeData(
      backgroundColor: glassSurface,
      surfaceTintColor: Colors.transparent,
      shadowColor: Colors.transparent,
      elevation: 0,
      headerBackgroundColor: Colors.transparent,
      headerForegroundColor: onSurface,
      headerHeadlineStyle: const TextStyle(
        fontSize: 28,
        fontWeight: FontWeight.w400,
        color: onSurface,
      ),
      headerHelpStyle: TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        letterSpacing: 0.5,
        color: AppColors.modalTextMuted,
      ),
      weekdayStyle: TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        color: AppColors.modalTextMuted,
      ),
      dayStyle: const TextStyle(fontSize: 14, color: onSurface),
      dayForegroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalPrimaryButtonText;
        }
        if (states.contains(WidgetState.disabled)) {
          return AppColors.modalTextMuted;
        }
        return onSurface;
      }),
      dayBackgroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalPrimaryButtonBackground;
        }
        return Colors.transparent;
      }),
      dayOverlayColor: WidgetStateProperty.resolveWith(
        (_) => AppColors.modalChipBackground,
      ),
      todayForegroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalPrimaryButtonText;
        }
        // #2764: today's date is UNFILLED, so this is a foreground on the bare
        // sheet — the fill token measured 1.01:1 here. Today reads as today via
        // `todayBorder` below, not via a colour nobody can see.
        return AppColors.modalTextPrimary;
      }),
      todayBackgroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalPrimaryButtonBackground;
        }
        return Colors.transparent;
      }),
      todayBorder: BorderSide(
        color: AppColors.modalPrimaryButtonBackground,
        width: 1.2,
      ),
      yearStyle: const TextStyle(fontSize: 14, color: onSurface),
      yearForegroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalPrimaryButtonText;
        }
        return onSurface;
      }),
      yearBackgroundColor: WidgetStateProperty.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalPrimaryButtonBackground;
        }
        return Colors.transparent;
      }),
      yearOverlayColor: WidgetStateProperty.resolveWith(
        (_) => AppColors.modalChipBackground,
      ),
      dividerColor: AppColors.modalBorderSubtle,
      rangeSelectionBackgroundColor:
          AppColors.modalPrimaryButtonBackground.withValues(alpha: 0.18),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(24)),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: AppColors.modalSearchFieldBackground,
        labelStyle: TextStyle(color: AppColors.modalTextSecondary),
        hintStyle: TextStyle(color: AppColors.modalTextMuted),
        floatingLabelStyle: TextStyle(color: AppColors.modalTextPrimary),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: AppColors.modalSearchFieldBorder),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: AppColors.modalSearchFieldBorder),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: AppColors.modalBorder, width: 1.5),
        ),
      ),
    ),
    timePickerTheme: TimePickerThemeData(
      backgroundColor: glassSurface,
      elevation: 0,
      hourMinuteColor: WidgetStateColor.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalChipBackgroundActive;
        }
        return AppColors.modalChipBackground;
      }),
      hourMinuteTextColor: WidgetStateColor.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalChipTextActive;
        }
        return AppColors.modalTextPrimary;
      }),
      dayPeriodColor: WidgetStateColor.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalChipBackgroundActive;
        }
        return AppColors.modalChipBackground;
      }),
      dayPeriodTextColor: WidgetStateColor.resolveWith((states) {
        if (states.contains(WidgetState.selected)) {
          return AppColors.modalChipTextActive;
        }
        return AppColors.modalTextPrimary;
      }),
      dayPeriodBorderSide:
          BorderSide(color: AppColors.modalChipBorder),
      dialBackgroundColor: AppColors.modalChipBackground,
      dialHandColor: AppColors.modalPrimaryButtonBackground,
      dialTextColor: AppColors.modalTextPrimary,
      entryModeIconColor: AppColors.modalTextSecondary,
      helpTextStyle: TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w600,
        letterSpacing: 0.5,
        color: AppColors.modalTextMuted,
      ),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(24)),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: AppColors.modalSearchFieldBackground,
        hintStyle: TextStyle(color: AppColors.modalTextMuted),
        labelStyle: TextStyle(color: AppColors.modalTextSecondary),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: AppColors.modalSearchFieldBorder),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: AppColors.modalSearchFieldBorder),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: BorderSide(color: AppColors.modalBorder, width: 1.5),
        ),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        // Cancel/OK. This is the label colour on a bare sheet — the reported
        // #2764 surface — so it takes the on-glass text token, not the fill.
        foregroundColor: AppColors.modalTextPrimary,
        textStyle: const TextStyle(
          fontSize: 14,
          fontWeight: FontWeight.w600,
        ),
      ),
    ),
    iconButtonTheme: IconButtonThemeData(
      style: IconButton.styleFrom(foregroundColor: onSurface),
    ),
    iconTheme: const IconThemeData(color: onSurface),
    dividerTheme:
        DividerThemeData(color: AppColors.modalBorderSubtle, space: 0),
    textTheme: Theme.of(context).textTheme.apply(
          bodyColor: onSurface,
          displayColor: onSurface,
        ),
  );
}

/// Wraps the picker tree in:
///   1. A heavily-blurred backdrop scrim covering the whole screen. We use
///      the same sigma as the time modal's *sheet* backdrop
///      ([AppColors.modalSurfaceBlurSigma]) rather than the lighter sheet
///      *outer* scrim — Flutter's centered Dialog widgets don't expose a
///      hook to apply [BackdropFilter] inside their own clip, so we make
///      the entire screen behind the picker frosted instead. The picker's
///      translucent-white surface (15%) sits on top, reading as the same
///      frosted-glass material as the time modal's sheet.
///   2. The glass [ThemeData] override so every widget in the picker
///      reads from the modal-glass color tokens.
Widget _glassWrap(BuildContext context, Widget? child) {
  if (child == null) return const SizedBox.shrink();
  return Stack(
    fit: StackFit.expand,
    children: [
      Positioned.fill(
        child: BackdropFilter(
          filter: ui.ImageFilter.blur(
            sigmaX: AppColors.modalSurfaceBlurSigma,
            sigmaY: AppColors.modalSurfaceBlurSigma,
          ),
          child: Container(color: AppColors.modalBackdrop),
        ),
      ),
      Theme(data: _glassPickerTheme(context), child: child),
    ],
  );
}

/// Glass-themed replacement for [showDatePicker]. Forwards every parameter
/// unchanged and wraps the dialog tree so the calendar paints with
/// translucent white surfaces and white-on-glass text.
Future<DateTime?> showGlassDatePicker({
  required BuildContext context,
  required DateTime initialDate,
  required DateTime firstDate,
  required DateTime lastDate,
  DateTime? currentDate,
  String? helpText,
  String? cancelText,
  String? confirmText,
  String? errorFormatText,
  String? errorInvalidText,
  String? fieldHintText,
  String? fieldLabelText,
  TextInputType? keyboardType,
  Locale? locale,
  bool useRootNavigator = true,
  RouteSettings? routeSettings,
  TextDirection? textDirection,
  DatePickerEntryMode initialEntryMode = DatePickerEntryMode.calendar,
  SelectableDayPredicate? selectableDayPredicate,
  DatePickerMode initialDatePickerMode = DatePickerMode.day,
}) {
  return showDatePicker(
    context: context,
    initialDate: initialDate,
    firstDate: firstDate,
    lastDate: lastDate,
    currentDate: currentDate,
    helpText: helpText,
    cancelText: cancelText,
    confirmText: confirmText,
    errorFormatText: errorFormatText,
    errorInvalidText: errorInvalidText,
    fieldHintText: fieldHintText,
    fieldLabelText: fieldLabelText,
    keyboardType: keyboardType,
    locale: locale,
    useRootNavigator: useRootNavigator,
    routeSettings: routeSettings,
    textDirection: textDirection,
    initialEntryMode: initialEntryMode,
    selectableDayPredicate: selectableDayPredicate,
    initialDatePickerMode: initialDatePickerMode,
    barrierColor: Colors.transparent, // we paint our own scrim in _glassWrap
    builder: _glassWrap,
  );
}

/// Glass-themed replacement for [showTimePicker]. Forwards every parameter
/// unchanged and wraps the dialog tree.
Future<TimeOfDay?> showGlassTimePicker({
  required BuildContext context,
  required TimeOfDay initialTime,
  TimePickerEntryMode initialEntryMode = TimePickerEntryMode.dial,
  String? cancelText,
  String? confirmText,
  String? helpText,
  String? errorInvalidText,
  String? hourLabelText,
  String? minuteLabelText,
  RouteSettings? routeSettings,
  EntryModeChangeCallback? onEntryModeChanged,
  Orientation? orientation,
  bool useRootNavigator = true,
}) {
  return showTimePicker(
    context: context,
    initialTime: initialTime,
    initialEntryMode: initialEntryMode,
    cancelText: cancelText,
    confirmText: confirmText,
    helpText: helpText,
    errorInvalidText: errorInvalidText,
    hourLabelText: hourLabelText,
    minuteLabelText: minuteLabelText,
    routeSettings: routeSettings,
    onEntryModeChanged: onEntryModeChanged,
    orientation: orientation,
    useRootNavigator: useRootNavigator,
    barrierColor: Colors.transparent, // we paint our own scrim in _glassWrap
    builder: _glassWrap,
  );
}
