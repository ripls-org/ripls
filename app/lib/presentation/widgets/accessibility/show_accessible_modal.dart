import 'package:flutter/material.dart';
import 'package:ripls/core/utils/responsive.dart';

/// Drop-in replacement for [showModalBottomSheet] that restores focus to
/// [returnFocusTo] (or, when null, the focused node at the time of the call)
/// after the modal is dismissed. Without this, screen-reader and keyboard
/// users lose their focus position when a modal closes.
///
/// Sheets are capped at [Responsive.sheetMaxWidth] unless the caller passes
/// its own [constraints] (#2912): on a desktop-wide window
/// [showModalBottomSheet] centers a constrained sheet natively, keeping the
/// tap-outside dismiss barrier intact — which an `Align`/`Center` wrapper
/// inside the sheet would break (see `glass_sheet.dart`). On phone viewports
/// the cap never binds, so nothing changes there.
///
/// All other parameters are forwarded to [showModalBottomSheet] unchanged.
Future<T?> showAccessibleModal<T>(
  BuildContext context, {
  required WidgetBuilder builder,
  FocusNode? returnFocusTo,
  Color? backgroundColor,
  double? elevation,
  ShapeBorder? shape,
  Clip? clipBehavior,
  BoxConstraints? constraints,
  Color? barrierColor,
  bool isScrollControlled = false,
  bool useRootNavigator = false,
  bool isDismissible = true,
  bool enableDrag = true,
  bool? showDragHandle,
  bool useSafeArea = false,
  RouteSettings? routeSettings,
  AnimationController? transitionAnimationController,
  Offset? anchorPoint,
}) async {
  final returnTarget = returnFocusTo ?? FocusManager.instance.primaryFocus;
  final result = await showModalBottomSheet<T>(
    context: context,
    builder: builder,
    backgroundColor: backgroundColor,
    elevation: elevation,
    shape: shape,
    clipBehavior: clipBehavior,
    constraints:
        constraints ?? const BoxConstraints(maxWidth: Responsive.sheetMaxWidth),
    barrierColor: barrierColor,
    isScrollControlled: isScrollControlled,
    useRootNavigator: useRootNavigator,
    isDismissible: isDismissible,
    enableDrag: enableDrag,
    showDragHandle: showDragHandle,
    useSafeArea: useSafeArea,
    routeSettings: routeSettings,
    transitionAnimationController: transitionAnimationController,
    anchorPoint: anchorPoint,
  );
  returnTarget?.requestFocus();
  return result;
}
