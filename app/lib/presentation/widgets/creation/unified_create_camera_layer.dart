import 'dart:async';

import 'package:camera/camera.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/creation/camera_coaching_carousel.dart';
import 'package:ripls/presentation/widgets/creation/camera_viewport.dart';

final _log = Logger('UnifiedCreateCameraLayer');

/// Camera layer rendered behind the unified-create input drawer when
/// the user is on the Image tab. Hosts a full-bleed [CameraViewport]
/// (live preview, viewfinder corners, permission / no-hardware
/// fallback), the coaching carousel that teaches first-time users what
/// kinds of subjects the camera can recognise, the centered shutter
/// button, and the lower-left gallery button.
///
/// Owns the [CameraController] reference for the lifetime of the
/// widget. The controller itself is created and disposed by
/// [CameraViewport]; this layer just holds a handle returned via
/// `onCameraReady` so the shutter can call `takePicture()`.
class UnifiedCreateCameraLayer extends ConsumerStatefulWidget {
  const UnifiedCreateCameraLayer({
    super.key,
    required this.bottomDrawerHeight,
    this.showCoachingCarousel = true,
    this.onPhotoCaptured,
    this.onGalleryPick,
  });

  /// Height of the bottom drawer (tab strip) in dp. The shutter +
  /// gallery row is anchored just above this so it never collides with
  /// the drawer; the carousel sits above that. The modal owns the
  /// drawer's height; the layer reads it via this prop rather than
  /// guessing.
  final double bottomDrawerHeight;

  /// Whether to render the [CameraCoachingCarousel] above the shutter.
  /// The unified-create flow shows it (educates users on what subjects
  /// the camera can recognise); narrower flows that re-use this layer
  /// for plain photo capture (e.g. community creation) opt out.
  final bool showCoachingCarousel;

  /// Override for the shutter action. When provided, the shutter calls
  /// this with the captured file path instead of routing into the
  /// unified-create view model. Used by reusable callers (community
  /// creation, etc.) that own their own creation state.
  final Future<void> Function(String filePath)? onPhotoCaptured;

  /// Override for the gallery button. When provided, the gallery button
  /// calls this instead of the unified-create view model's gallery
  /// pick. See [onPhotoCaptured] for the motivation.
  final void Function()? onGalleryPick;

  @override
  ConsumerState<UnifiedCreateCameraLayer> createState() =>
      _UnifiedCreateCameraLayerState();
}

class _UnifiedCreateCameraLayerState
    extends ConsumerState<UnifiedCreateCameraLayer> {
  CameraController? _controller;
  bool _capturing = false;

  Future<void> _onShutter() async {
    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) return;
    if (_capturing) return;
    setState(() => _capturing = true);
    unawaited(HapticFeedback.mediumImpact());
    try {
      final file = await controller.takePicture();
      if (!mounted) return;
      final override = widget.onPhotoCaptured;
      if (override != null) {
        await override(file.path);
      } else {
        await ref
            .read(unifiedCreateViewModelProvider.notifier)
            .captureAndStartFromCamera(file);
      }
    } catch (e, s) {
      _log.warning('takePicture failed', e, s);
    } finally {
      if (mounted) setState(() => _capturing = false);
    }
  }

  void _onGallery() {
    final override = widget.onGalleryPick;
    if (override != null) {
      override();
      return;
    }
    ref.read(unifiedCreateViewModelProvider.notifier).pickAndStartFromGallery();
  }

  @override
  Widget build(BuildContext context) {
    // Vertical anchor: shutter sits 24 dp above the drawer; carousel
    // sits another 24 dp above the shutter row. Using bottom-padding
    // rather than absolute `bottom:` so the layout scales with the
    // drawer's actual height (which may change between Text/URL tabs
    // mounted at a different size than the Image collapsed strip).
    const shutterSize = 72.0;
    const galleryButtonSize = 56.0;
    final shutterRowBottom = widget.bottomDrawerHeight + 24;

    // On web, the `camera` plugin has no working implementation
    // (camera_web exists but is immature on desktop browsers and
    // doesn't integrate with our CameraViewport's lifecycle). We
    // render a gallery-only fallback: large "Pick from gallery"
    // button + a small note that the live camera needs the mobile
    // app. The Text and URL tabs in the same modal are fully
    // web-safe.
    if (kIsWeb) {
      return Stack(
        children: [
          Positioned.fill(
            child: ColoredBox(
              color: AppColors.modalSurface.withValues(alpha: 0.6),
              child: Center(
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 32),
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        Icons.photo_library_outlined,
                        size: 48,
                        color: AppColors.modalTextPrimary
                            .withValues(alpha: 0.85),
                      ),
                      const SizedBox(height: 12),
                      // Just the invitation — no "live camera is mobile-only"
                      // caveat. Platform capability notes are developer
                      // framing, not user guidance; the Text/URL tabs are
                      // right there in the same drawer (#2724).
                      Text(
                        context.l10n.webCreateGalleryOnlyTitle,
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          fontSize: 18,
                          fontWeight: FontWeight.w600,
                          color: AppColors.modalTextPrimary,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
          Positioned(
            left: 0,
            right: 0,
            bottom: shutterRowBottom,
            child: SizedBox(
              height: shutterSize,
              child: Center(
                child: _GalleryButton(
                  size: shutterSize,
                  onTap: _onGallery,
                ),
              ),
            ),
          ),
        ],
      );
    }

    return Stack(
      children: [
        // Full-bleed camera preview with permission / no-hardware
        // fallback handled by CameraViewport. We disable its built-in
        // gallery button (positioned lower-right) because the new
        // design renders our own in the lower-left.
        Positioned.fill(
          child: CameraViewport(
            showGalleryButton: false,
            onCameraReady: (controller) {
              if (!mounted) return;
              setState(() => _controller = controller);
            },
          ),
        ),
        // Coaching carousel — sits above the shutter row.
        if (widget.showCoachingCarousel)
          Positioned(
            left: 0,
            right: 0,
            bottom: shutterRowBottom + shutterSize + 16,
            child: const CameraCoachingCarousel(),
          ),
        // Centered shutter + lower-left gallery button row.
        Positioned(
          left: 0,
          right: 0,
          bottom: shutterRowBottom,
          child: SizedBox(
            height: shutterSize,
            child: Stack(
              alignment: Alignment.center,
              children: [
                Positioned(
                  left: 20,
                  child: _GalleryButton(
                    size: galleryButtonSize,
                    onTap: _onGallery,
                  ),
                ),
                _ShutterButton(
                  size: shutterSize,
                  enabled: _controller != null &&
                      _controller!.value.isInitialized &&
                      !_capturing,
                  onTap: _onShutter,
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _ShutterButton extends StatelessWidget {
  const _ShutterButton({
    required this.size,
    required this.enabled,
    required this.onTap,
  });
  final double size;
  final bool enabled;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final inner = size - 14;
    return Tappable(
      key: const Key('camera-shutter-button'),
      semanticsLabel: context.l10n.a11yUnifiedCreateShutter,
      onTap: enabled ? onTap : null,
      excludeChildSemantics: true,
      child: Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          color: AppColors.modalSurface,
          border: Border.all(
            color: AppColors.modalTextPrimary.withValues(alpha: 0.85),
            width: 2,
          ),
          boxShadow: const [
            BoxShadow(
              color: Color(0x40000000),
              blurRadius: 12,
              offset: Offset(0, 4),
            ),
          ],
        ),
        alignment: Alignment.center,
        child: Container(
          width: inner,
          height: inner,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: enabled
                ? AppColors.modalTextPrimary
                : AppColors.modalTextPrimary.withValues(alpha: 0.6),
          ),
        ),
      ),
    );
  }
}

class _GalleryButton extends StatelessWidget {
  const _GalleryButton({required this.size, required this.onTap});
  final double size;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      key: const Key('camera-gallery-button'),
      semanticsLabel: context.l10n.a11yMiscOpenGallery,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          color: AppColors.modalSurface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: AppColors.modalBorder),
        ),
        alignment: Alignment.center,
        child: Icon(
          Icons.photo_library_outlined,
          color: AppColors.modalTextPrimary,
          size: 26,
        ),
      ),
    );
  }
}
