import 'dart:async';

import 'package:camera/camera.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:gal/gal.dart';
import 'package:keyboard_actions/keyboard_actions.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/screens/request/request_preview_modal.dart';
import 'package:ripls/presentation/viewmodels/gen_request_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_creation_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/creation/camera_viewport.dart';
import 'package:ripls/presentation/widgets/creation/creation_bottom_controls.dart';
import 'package:ripls/presentation/widgets/creation/creation_instruction_overlay.dart';
import 'package:ripls/presentation/widgets/creation/input_mode_toggle.dart';
import 'package:ripls/presentation/widgets/creation/text_input_area.dart';
import 'package:ripls/presentation/widgets/keyboard_actions_config.dart';

/// Request creation modal with camera/text input modes.
///
/// Opens directly into the creation flow; community selection is handled
/// inside `RequestPreviewModal` via `CommunitySelectionSheet.showForDeferred`,
/// matching the pattern used by Create Gear and Create Event.
///
/// Usage:
/// ```dart
/// final requestId = await RequestCreationModal.show(context);
/// if (requestId != null) {
///   // Request was created - requestId is the new request ID
/// }
/// ```
class RequestCreationModal extends ConsumerStatefulWidget {
  const RequestCreationModal({super.key});

  /// Show the request creation modal.
  ///
  /// Returns the created request ID on success, null if user cancels.
  static Future<String?> show(BuildContext context) {
    return showAccessibleModal<String>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      isDismissible: true,
      enableDrag: true,
      builder: (context) => const RequestCreationModal(),
    );
  }

  @override
  ConsumerState<RequestCreationModal> createState() =>
      _RequestCreationModalState();
}

class _RequestCreationModalState extends ConsumerState<RequestCreationModal> {
  final TextEditingController _textController = TextEditingController();
  final FocusNode _textFocusNode = FocusNode();
  CameraController? _cameraController;

  @override
  void dispose() {
    _textController.dispose();
    _textFocusNode.dispose();
    super.dispose();
  }

  Future<void> _handleGenerate() async {
    // Haptic feedback on button tap
    unawaited(HapticFeedback.mediumImpact());

    final viewModel = ref.read(requestCreationProvider.notifier);
    final state = ref.read(requestCreationProvider);

    // Capture photo if no image is selected and in image mode
    if (state.inputMode == CreationInputMode.image &&
        state.selectedImagePath == null &&
        _cameraController != null) {
      try {
        final image = await _cameraController!.takePicture();
        viewModel.setImagePath(image.path);

        // Save photo to device gallery
        try {
          await Gal.putImage(image.path);
        } catch (e) {
          // Don't block generation if gallery save fails, just log it
          debugPrint('Failed to save photo to gallery: $e');
        }
      } catch (e) {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('Error capturing photo: $e')),
          );
        }
        return;
      }
    }

    ref.read(genRequestProvider.notifier).reset();
    final ok = await viewModel.generateRequestStreaming();
    if (!ok) {
      if (mounted) {
        final errState = ref.read(requestCreationProvider);
        if (errState.error != null) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(
                RpcErrorHandler.localize(errState.error!, context.l10n),
              ),
            ),
          );
        }
      }
      return;
    }
    if (!mounted) return;
    final requestId = await _showPreview();
    if (!mounted) return;
    if (requestId != null) {
      Navigator.of(context).pop(requestId);
    } else if (state.inputMode == CreationInputMode.image) {
      viewModel.clearImage();
    }
  }

  /// Show preview modal as a full-screen overlay
  Future<String?> _showPreview() async {
    return showDialog<String>(
      context: context,
      barrierDismissible: false,
      builder: (context) => const RequestPreviewModal(),
    );
  }

  /// Handle manual creation by showing preview modal with empty fields.
  Future<void> _handleManualCreate() async {
    // Haptic feedback on button tap
    unawaited(HapticFeedback.mediumImpact());

    if (!mounted) return;

    // Reset genRequestProvider for manual entry
    ref.read(genRequestProvider.notifier).reset();

    // Show the preview modal with empty state
    final requestId = await _showPreview();

    if (requestId != null && mounted) {
      // Request was created successfully - close creation modal and return request ID
      Navigator.of(context).pop(requestId);
    }
    // If requestId is null, user cancelled from preview - stay in creation modal
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(requestCreationProvider);

    // Sync text controller with state
    if (_textController.text != state.textInput) {
      _textController.text = state.textInput;
      _textController.selection = TextSelection.collapsed(
        offset: state.textInput.length,
      );
    }

    final screenHeight = MediaQuery.of(context).size.height;

    // Image mode fills the screen; text mode is fixed at 60%.
    // The "Done" toolbar handles keyboard dismissal — no expansion needed.
    final isImageMode = state.inputMode == CreationInputMode.image;
    final modalHeight = screenHeight * (isImageMode ? 1.0 : 0.60);

    return KeyboardActions(
      disableScroll: true,
      tapOutsideBehavior: TapOutsideBehavior.translucentDismiss,
      config: buildKeyboardActionsConfig([_textFocusNode]),
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 300)),
        curve: Curves.easeInOut,
        height: modalHeight,
        child: Container(
          decoration: BoxDecoration(
            color: AppColors.experienceModalBackground(context),
            borderRadius: isImageMode
                ? BorderRadius.zero
                : const BorderRadius.vertical(top: Radius.circular(20)),
            border: isImageMode
                ? null
                : Border.all(color: AppColors.border(context)),
          ),
          child: SafeArea(
            // Disable top padding in image mode so camera fills screen
            top: !isImageMode,
            child: Stack(
              children: [
                // Main content with full-height camera/text area
                Column(children: [Expanded(child: _buildMainContent(state))]),
                // Instruction overlay above bottom controls (camera mode only)
                if (isImageMode)
                  Positioned(
                    left: 0,
                    right: 0,
                    bottom: 140,
                    child: CreationInstructionOverlay(
                      icon: Icons.camera_alt,
                      text: 'Capture what you need',
                    ),
                  ),
                // Bottom controls overlay (semi-transparent)
                Positioned(
                  left: 0,
                  right: 0,
                  bottom: 0,
                  child: _buildBottomControls(state),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  /// Main content area - shows camera viewport or text input.
  Widget _buildMainContent(RequestCreationState state) {
    if (state.inputMode == CreationInputMode.image) {
      return CameraViewport(
        selectedImagePath: state.selectedImagePath,
        onImageCaptured: (path) =>
            ref.read(requestCreationProvider.notifier).setImagePath(path),
        onImageCleared: () =>
            ref.read(requestCreationProvider.notifier).clearImage(),
        onCameraReady: (controller) => _cameraController = controller,
      );
    } else {
      return _buildTextInputMode(state);
    }
  }

  /// Text input mode with multi-line input.
  Widget _buildTextInputMode(RequestCreationState state) {
    return TextInputArea(
      controller: _textController,
      focusNode: _textFocusNode,
      hintText: context.l10n.creationRequestHintText,
      minLines: 4,
      maxLines: 4,
      onChanged: (value) =>
          ref.read(requestCreationProvider.notifier).setTextInput(value),
    );
  }

  // Widget async exception: confirmation dialog before mode switch.
  Future<void> _handleModeChanged(CreationInputMode mode) async {
    final state = ref.read(requestCreationProvider);
    final hasText = state.textInput.isNotEmpty;
    if (mode == CreationInputMode.image && hasText) {
      final confirmed = await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: Text(context.l10n.creationSwitchToImageTitle),
          content: Text(context.l10n.creationSwitchToImageBody),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(false),
              child: Text(context.l10n.commonCancel),
            ),
            TextButton(
              onPressed: () => Navigator.of(context).pop(true),
              child: Text(context.l10n.creationSwitchToImageConfirm),
            ),
          ],
        ),
      );
      if (!mounted) return;
      if (confirmed != true) return;
    }
    ref.read(requestCreationProvider.notifier).setInputMode(mode);
  }

  /// Bottom controls with toggle and generate button.
  Widget _buildBottomControls(RequestCreationState state) {
    final canGenerate = ref
        .read(requestCreationProvider.notifier)
        .canGenerate();

    return CreationBottomControls(
      inputMode: state.inputMode,
      onModeChanged: _handleModeChanged,
      isLoading: state.isLoading,
      canGenerate: canGenerate,
      onGenerate: _handleGenerate,
      onManualCreate: _handleManualCreate,
      buttonLabel: 'Create Request',
    );
  }
}
