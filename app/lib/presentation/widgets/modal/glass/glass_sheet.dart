import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_surface.dart';

/// GlassSheet renders the frosted-glass bottom-sheet container.
///
/// **Important:** GlassSheet is a content widget. It does NOT call
/// [showModalBottomSheet] — that responsibility stays with `showAccessibleModal`
/// or `ModalHelpers.showStandardModal()`. Wrap your modal content in a
/// `GlassSheet(...)` inside the `builder` argument of those functions.
///
/// **Backdrop scrim vs. content scrim.** The dark scrim that dims the rest of
/// the screen is the system modal barrier (`barrierColor:` on
/// `showAccessibleModal`). The barrier is rendered outside the modal's
/// animation hierarchy, so it stays put when the sheet is dragged. Pass
/// `barrierColor: AppColors.modalBackdrop` on the show call to match the
/// established scrim color. Separately, [scrim] paints a darkening layer
/// *inside* the sheet — between the backdrop blur and the content — for
/// sheets shown over bright media where the blurred-through content would
/// otherwise wash out the on-glass text.
///
/// The widget paints (in order, back to front):
///
/// 1. A [GlassSurface] sheet inset from the screen edges by
///    [ModalTheme.sheetHorizontalInset], with [ModalTheme.sheetTopRadius]
///    at the top and square corners at the bottom.
/// 2. A drag handle pill at the top center (omit via [showDragHandle]: false
///    for surfaces where the handle would interfere with content gestures —
///    e.g. a full-bleed map).
/// 3. The provided [child].
///
/// See docs/issues/1797-glass-modal-revamp.md.
class GlassSheet extends StatelessWidget {
  /// Sheet content. Typically a column of `GlassModalHeader`,
  /// field-label-and-chips groups, and a `GlassFooterButtons` at the bottom.
  final Widget child;

  /// Whether to render a [BackdropFilter] inside the sheet. Forwarded to the
  /// underlying [GlassSurface]. Defaults to `true`. Disable on low-end Android
  /// (or via an app-level switch) to skip the blur cost.
  final bool useBlur;

  /// Internal padding around [child]. Defaults to 24px horizontal, 24px top,
  /// 32px bottom (the bottom value adds breathing room above the home indicator).
  final EdgeInsets padding;

  /// Whether the sheet should constrain itself to
  /// [ModalTheme.sheetMaxHeightFraction] of screen height. Set false when the
  /// caller already imposes a height constraint (e.g. via `showStandardModal`'s
  /// `heightFactor`).
  final bool applyMaxHeight;

  /// Whether to render the drag handle at the top of the sheet. Defaults to
  /// `true`. The system modal barrier still handles tap-to-dismiss;
  /// drag-to-dismiss is governed by [showModalBottomSheet]'s `enableDrag`.
  final bool showDragHandle;

  /// When `true`, the drag handle is positioned as a [Stack] overlay on top
  /// of the child (the child fills the sheet's available height). When
  /// `false` (default), the drag handle sits in a [Column] above the child,
  /// occupying its own vertical strip.
  ///
  /// Use overlay mode for content that should bleed to the sheet's top
  /// edge — e.g. a full-bleed map — so the drag handle floats over the
  /// content instead of appearing in a separate band above it. Combine
  /// with `padding: EdgeInsets.zero` to get true edge-to-edge bleed.
  final bool dragHandleOverlay;

  /// Optional darkening layer painted between the sheet's backdrop blur and
  /// its content. Forwarded to [GlassSurface.scrim]. Set to
  /// [AppColors.modalContentScrim] for sheets shown over bright media, where
  /// the blurred-through content would otherwise wash out on-glass text.
  final Color? scrim;

  const GlassSheet({
    super.key,
    required this.child,
    this.useBlur = true,
    this.padding = const EdgeInsets.fromLTRB(20, 20, 20, 32),
    this.applyMaxHeight = true,
    this.showDragHandle = true,
    this.dragHandleOverlay = false,
    this.scrim,
  });

  @override
  Widget build(BuildContext context) {
    final mediaQuery = MediaQuery.of(context);
    final maxHeight =
        mediaQuery.size.height * ModalTheme.sheetMaxHeightFraction;

    final Widget content = dragHandleOverlay
        ? Stack(
            children: [
              child,
              if (showDragHandle)
                const Positioned(
                  top: 8,
                  left: 0,
                  right: 0,
                  child: _OverlayDragHandle(),
                ),
            ],
          )
        : Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              if (showDragHandle) ...[
                const _DragHandle(),
                const SizedBox(height: 12),
              ],
              Flexible(child: child),
            ],
          );

    final sheet = GlassSurface(
      useBlur: useBlur,
      scrim: scrim,
      borderRadius: const BorderRadius.only(
        topLeft: Radius.circular(ModalTheme.sheetTopRadius),
        topRight: Radius.circular(ModalTheme.sheetTopRadius),
      ),
      // Transparent Material between the glass background and the content so
      // descendant ListTiles/InkWells paint ink on a Material ancestor rather
      // than behind GlassSurface's decoration. Flutter 3.44+ asserts when a
      // ListTile's nearest background ancestor is a DecoratedBox, not a
      // Material; this covers every sheet built on GlassSheet in one place.
      child: Material(
        type: MaterialType.transparency,
        child: Padding(
          padding: padding,
          child: content,
        ),
      ),
    );

    // No outer Align: returning just the Padding lets the bottom-sheet
    // route size the modal to its natural content height (up to maxHeight).
    // An Align would expand the modal's render box to the full screen,
    // intercepting taps in the empty area above the sheet and preventing
    // the system ModalBarrier (the dismiss-on-tap scrim) from receiving
    // them. The bottom-sheet route bottom-anchors the result for us.
    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: ModalTheme.sheetHorizontalInset,
      ),
      child: applyMaxHeight
          ? ConstrainedBox(
              constraints: BoxConstraints(maxHeight: maxHeight),
              child: sheet,
            )
          : sheet,
    );
  }
}

class _DragHandle extends StatelessWidget {
  const _DragHandle();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Semantics(
        label: context.l10n.a11yMiscModalDragHandle,
        child: ExcludeSemantics(
          child: Container(
            width: ModalTheme.dragHandleWidth,
            height: ModalTheme.dragHandleHeight,
            decoration: BoxDecoration(
              color: AppColors.modalDragHandle,
              borderRadius: BorderRadius.circular(
                ModalTheme.dragHandleHeight / 2,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Drag handle variant for `dragHandleOverlay: true` mode. The same white
/// pill is rendered inside a small dark capsule so it reads against any
/// background — matching the scrim+light-glass compositing pattern used
/// for chips and the bottom address card on the location modal.
class _OverlayDragHandle extends StatelessWidget {
  const _OverlayDragHandle();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Semantics(
        label: context.l10n.a11yMiscModalDragHandle,
        child: ExcludeSemantics(
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
            decoration: BoxDecoration(
              color: AppColors.modalBackdrop,
              borderRadius: BorderRadius.circular(8),
            ),
            child: Container(
              width: ModalTheme.dragHandleWidth,
              height: ModalTheme.dragHandleHeight,
              decoration: BoxDecoration(
                color: AppColors.modalDragHandle,
                borderRadius: BorderRadius.circular(
                  ModalTheme.dragHandleHeight / 2,
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
