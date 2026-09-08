import 'dart:async';
import 'dart:io';

import 'package:camera/camera.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:image_picker/image_picker.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/creation/camera_instruction_overlay.dart';

/// Camera viewport component for creation modals.
///
/// Shows live camera preview, selected image preview, or error/loading states.
/// Integrates with ImagePicker for gallery selection and Camera for capture.
///
/// Example:
/// ```dart
/// CameraViewport(
///   selectedImagePath: state.selectedImagePath,
///   onImageCaptured: (path) => notifier.setImagePath(path),
///   onImageCleared: () => notifier.clearImage(),
/// )
/// ```
class CameraViewport extends StatefulWidget {
  final String? selectedImagePath;
  final Function(String)? onImageCaptured;
  final VoidCallback? onImageCleared;
  final bool showGalleryButton;
  final Function(CameraController?)? onCameraReady;
  final String? instructionText;

  const CameraViewport({
    super.key,
    this.selectedImagePath,
    this.onImageCaptured,
    this.onImageCleared,
    this.showGalleryButton = true,
    this.onCameraReady,
    this.instructionText,
  });

  @override
  State<CameraViewport> createState() => _CameraViewportState();
}

class _CameraViewportState extends State<CameraViewport> {
  CameraController? _cameraController;
  List<CameraDescription>? _cameras;
  bool _isInitializingCamera = false;
  String? _cameraErrorMessage;
  final ImagePicker _imagePicker = ImagePicker();

  @override
  void initState() {
    super.initState();
    // Auto-initialize camera on mount
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _initializeCamera();
    });
  }

  @override
  void dispose() {
    _cameraController?.dispose();
    super.dispose();
  }

  Future<void> _initializeCamera() async {
    if (_isInitializingCamera) return;
    if (!mounted) return;

    setState(() {
      _isInitializingCamera = true;
      _cameraErrorMessage = null;
    });

    try {
      _cameras = await availableCameras();
      if (!mounted) return;
      if (_cameras == null || _cameras!.isEmpty) {
        setState(() {
          _cameraErrorMessage = 'No cameras available';
          _isInitializingCamera = false;
        });
        return;
      }

      _cameraController = CameraController(
        _cameras!.first,
        ResolutionPreset.high,
        enableAudio: false,
      );

      await _cameraController!.initialize();

      if (!mounted) {
        // Widget disposed during initialize() — release the controller we
        // just created so the camera hardware isn't held.
        unawaited(_cameraController?.dispose());
        _cameraController = null;
        return;
      }

      setState(() {
        _isInitializingCamera = false;
      });

      // Notify parent that camera is ready
      widget.onCameraReady?.call(_cameraController);
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _cameraErrorMessage = 'Failed to initialize camera: $e';
        _isInitializingCamera = false;
      });
    }
  }

  Future<void> _pickImageFromGallery() async {
    try {
      final image = await _imagePicker.pickImage(
        source: ImageSource.gallery,
        maxWidth: 1920,
        maxHeight: 1080,
        imageQuality: 85,
      );

      if (image != null && widget.onImageCaptured != null) {
        widget.onImageCaptured!(image.path);
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Error selecting image: $e')));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    // Show selected image with clear button
    if (widget.selectedImagePath != null) {
      return Stack(
        children: [
          // Image preview
          Positioned.fill(
            child: ClipRRect(
              borderRadius: BorderRadius.zero,
              child: Image.file(
                File(widget.selectedImagePath!),
                fit: BoxFit.cover,
              ),
            ),
          ),
          // Clear button in top-right
          if (widget.onImageCleared != null)
            Positioned(
              top: 16,
              right: 16,
              child: Tappable(
                semanticsLabel: context.l10n.a11yMiscClearImage,
                onTap: () {
                  HapticFeedback.lightImpact();
                  widget.onImageCleared!();
                },
                child: Container(
                  padding: const EdgeInsets.all(8),
                  decoration: const BoxDecoration(
                    color: OverlayTokens.scrimFloor,
                    shape: BoxShape.circle,
                  ),
                  child: const Icon(
                    Icons.close,
                    color: GlassTokens.textPrimary,
                    size: 20,
                  ),
                ),
              ),
            ),
        ],
      );
    }

    // Show live camera preview, loading, or error
    final showCorners = _cameraController != null &&
        _cameraController!.value.isInitialized;
    return Stack(
      children: [
        // Main camera area
        _buildCameraPreview(),
        // Viewfinder corner brackets — anchored to the live preview's
        // actual on-screen rect (NOT the outer container) so the frame
        // they imply genuinely matches what `controller.takePicture()`
        // captures. Mount only when the live preview is up so they
        // don't render on top of the error / loading states.
        if (showCorners)
          Positioned.fill(
            child: LayoutBuilder(
              builder: (context, constraints) {
                final rect = _previewRectIn(constraints.biggest);
                return Stack(children: _buildViewfinderCorners(rect));
              },
            ),
          ),
        // Instruction overlay at top (only if instruction provided and camera is ready)
        if (widget.instructionText != null && showCorners)
          Positioned(
            top: 16,
            left: 16,
            right: 16,
            child: Center(
              child: CameraInstructionOverlay(
                instruction: widget.instructionText!,
              ),
            ),
          ),
        // Gallery button in lower-right
        if (widget.showGalleryButton)
          Positioned(bottom: 135, right: 16, child: _buildGalleryButton()),
      ],
    );
  }

  /// Compute the on-screen rect occupied by `Center(child: CameraPreview)`
  /// inside a container of [containerSize]. `CameraPreview` wraps its
  /// platform texture in an `AspectRatio` whose ratio is
  /// `1 / controller.aspectRatio` in portrait (camera package convention),
  /// so the rendered widget's size is bounded by both the container
  /// dimensions and that ratio. We reproduce the BoxFit.contain math
  /// here so the viewfinder brackets can be anchored to the same rect.
  Rect _previewRectIn(Size containerSize) {
    // Reported aspect ratio is for landscape (W/H of the preview stream).
    // In portrait the rendered AspectRatio uses the reciprocal.
    final rawAR = _cameraController!.value.aspectRatio;
    final previewAR =
        MediaQuery.of(context).orientation == Orientation.portrait
            ? 1 / rawAR
            : rawAR;
    final containerAR = containerSize.width / containerSize.height;
    double width;
    double height;
    if (previewAR > containerAR) {
      // Preview is wider than container → fit by width.
      width = containerSize.width;
      height = width / previewAR;
    } else {
      // Preview is taller than container → fit by height.
      height = containerSize.height;
      width = height * previewAR;
    }
    final left = (containerSize.width - width) / 2;
    final top = (containerSize.height - height) / 2;
    return Rect.fromLTWH(left, top, width, height);
  }

  List<Widget> _buildViewfinderCorners(Rect rect) {
    final color = AppColors.modalTextPrimary.withValues(alpha: 0.85);
    // Brackets sit a few dp INSIDE the preview rect so they hug the
    // photo's frame without being clipped by it.
    const inset = 8.0;
    const size = 24.0;
    const thickness = 2.0;
    const radius = Radius.circular(6);
    final tl = Offset(rect.left + inset, rect.top + inset);
    final tr = Offset(rect.right - inset - size, rect.top + inset);
    final bl = Offset(rect.left + inset, rect.bottom - inset - size);
    final br = Offset(rect.right - inset - size, rect.bottom - inset - size);
    return [
      Positioned(
        left: tl.dx,
        top: tl.dy,
        child: _CornerBracket(
          size: size,
          thickness: thickness,
          color: color,
          borderRadius: const BorderRadius.only(topLeft: radius),
          showTop: true,
          showLeft: true,
        ),
      ),
      Positioned(
        left: tr.dx,
        top: tr.dy,
        child: _CornerBracket(
          size: size,
          thickness: thickness,
          color: color,
          borderRadius: const BorderRadius.only(topRight: radius),
          showTop: true,
          showRight: true,
        ),
      ),
      Positioned(
        left: bl.dx,
        top: bl.dy,
        child: _CornerBracket(
          size: size,
          thickness: thickness,
          color: color,
          borderRadius: const BorderRadius.only(bottomLeft: radius),
          showBottom: true,
          showLeft: true,
        ),
      ),
      Positioned(
        left: br.dx,
        top: br.dy,
        child: _CornerBracket(
          size: size,
          thickness: thickness,
          color: color,
          borderRadius: const BorderRadius.only(bottomRight: radius),
          showBottom: true,
          showRight: true,
        ),
      ),
    ];
  }

  Widget _buildCameraPreview() {
    // Debug logging for aspect ratio analysis
    if (_cameraController != null && _cameraController!.value.isInitialized) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        final renderBox = context.findRenderObject() as RenderBox?;
        if (renderBox != null) {
          final size = renderBox.size;
          final cameraAspectRatio = _cameraController!.value.aspectRatio;
          debugPrint(
            '[CameraViewport] Container: ${size.width.toStringAsFixed(0)}px × ${size.height.toStringAsFixed(0)}px',
          );
          debugPrint(
            '[CameraViewport] Container aspect ratio: ${(size.width / size.height).toStringAsFixed(2)}:1',
          );
          debugPrint(
            '[CameraViewport] Camera aspect ratio: ${cameraAspectRatio.toStringAsFixed(2)}:1',
          );
        }
      });
    }

    // Show error state
    if (_cameraErrorMessage != null) {
      return Container(
        color: OverlayTokens.scrimBottom,
        child: Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(
                Icons.camera_alt_outlined,
                size: 64,
                color: GlassTokens.textFaint,
              ),
              const SizedBox(height: 16),
              Text(
                _cameraErrorMessage!,
                style: TextStyle(
                  color: GlassTokens.textMuted,
                  fontSize: 16,
                ),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 16),
              Text(
                'Use the gallery button to select a photo',
                style: TextStyle(
                  color: GlassTokens.textFaint,
                  fontSize: 14,
                ),
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      );
    }

    // Show loading state
    if (_isInitializingCamera ||
        _cameraController == null ||
        !_cameraController!.value.isInitialized) {
      return Container(
        color: OverlayTokens.scrimBottom,
        child: Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              CircularProgressIndicator(color: AppColors.primary(context)),
              const SizedBox(height: 16),
              Text(
                'Initializing camera...',
                style: TextStyle(
                  color: GlassTokens.textMuted,
                  fontSize: 16,
                ),
              ),
            ],
          ),
        ),
      );
    }

    // Show live camera preview - simple centered approach (matches create_gear_screen.dart)
    // CameraPreview handles its own aspect ratio and will letterbox naturally
    return Center(child: CameraPreview(_cameraController!));
  }

  Widget _buildGalleryButton() {
    return Tappable(
      semanticsLabel: context.l10n.a11yMiscOpenGallery,
      onTap: _pickImageFromGallery,
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: OverlayTokens.scrimFloor,
          borderRadius: BorderRadius.circular(12),
        ),
        child: const Icon(
          Icons.photo_library,
          color: GlassTokens.textPrimary,
          size: 28,
        ),
      ),
    );
  }
}

/// L-shaped bracket used as a viewfinder corner over the live camera
/// preview. Renders only the edges named via [showTop] / [showBottom]
/// / [showLeft] / [showRight] so the four corners share a single
/// widget definition.
class _CornerBracket extends StatelessWidget {
  const _CornerBracket({
    required this.size,
    required this.thickness,
    required this.color,
    required this.borderRadius,
    this.showTop = false,
    this.showBottom = false,
    this.showLeft = false,
    this.showRight = false,
  });

  final double size;
  final double thickness;
  final Color color;
  final BorderRadius borderRadius;
  final bool showTop;
  final bool showBottom;
  final bool showLeft;
  final bool showRight;

  @override
  Widget build(BuildContext context) {
    BorderSide edge() => BorderSide(color: color, width: thickness);
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        border: Border(
          top: showTop ? edge() : BorderSide.none,
          bottom: showBottom ? edge() : BorderSide.none,
          left: showLeft ? edge() : BorderSide.none,
          right: showRight ? edge() : BorderSide.none,
        ),
        borderRadius: borderRadius,
      ),
    );
  }
}
