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
import 'package:ripls/presentation/screens/experience/experience_preview_modal.dart';
import 'package:ripls/presentation/viewmodels/experience_creation_view_model.dart';
import 'package:ripls/presentation/viewmodels/gen_experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/creation/camera_viewport.dart';
import 'package:ripls/presentation/widgets/creation/creation_bottom_controls.dart';
import 'package:ripls/presentation/widgets/creation/creation_instruction_overlay.dart';
import 'package:ripls/presentation/widgets/creation/input_mode_toggle.dart';
import 'package:ripls/presentation/widgets/creation/text_input_area.dart';
import 'package:ripls/presentation/widgets/keyboard_actions_config.dart';

/// Experience creation modal with camera/text input modes.
///
/// Features:
/// - Clean modal design matching RSVP modal (no header, warm beige background)
/// - Text/Image toggle (similar to metrics screen tabs)
/// - Camera viewport with gallery button in lower-right
/// - Text input mode with inline instructions
/// - Semi-transparent bottom area showing camera through
/// - Generate button with enabled/disabled states
/// - Integrates with ExperienceService via ViewModel
///
/// Usage:
/// ```dart
/// final result = await ExperienceCreationModal.show(context);
/// if (result != null) {
///   // Experience was created - result is the experience ID
/// }
/// ```
class ExperienceCreationModal extends ConsumerStatefulWidget {
  const ExperienceCreationModal({super.key});

  /// Show the experience creation modal.
  ///
  /// Modal height is dynamic:
  /// - Text mode: 75% screen height
  /// - Image mode: 95% screen height
  /// - Auto-expands to 90% when keyboard is visible
  static Future<Object?> show(BuildContext context) {
    return showAccessibleModal<Object?>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      isDismissible: true,
      enableDrag: true,
      builder: (context) => const ExperienceCreationModal(),
    );
  }

  @override
  ConsumerState<ExperienceCreationModal> createState() =>
      _ExperienceCreationModalState();
}

class _ExperienceCreationModalState
    extends ConsumerState<ExperienceCreationModal> {
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

    final viewModel = ref.read(experienceCreationProvider.notifier);
    final state = ref.read(experienceCreationProvider);

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
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(SnackBar(content: Text('Error capturing photo: $e')));
        }
        return;
      }
    }

    await _handleGenerateStreaming();
  }

  Future<void> _handleGenerateStreaming() async {
    final viewModel = ref.read(experienceCreationProvider.notifier);
    // Reset the preview provider before starting so stale state from a prior
    // generation doesn't leak into the skeleton.
    ref.read(genExperienceProvider.notifier).reset();

    final ok = await viewModel.generateExperienceStreaming();
    if (!ok) {
      if (mounted) {
        final state = ref.read(experienceCreationProvider);
        if (state.error != null) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(
                RpcErrorHandler.localize(state.error!, context.l10n),
              ),
            ),
          );
        }
      }
      return;
    }

    if (!mounted) return;
    final previewResult = await showDialog<Object?>(
      context: context,
      barrierDismissible: false,
      builder: (context) => const ExperiencePreviewModal(),
    );

    if (!mounted) return;
    if (previewResult != null) {
      Navigator.of(context).pop(previewResult);
    } else {
      final state = ref.read(experienceCreationProvider);
      if (state.inputMode == CreationInputMode.image) {
        ref.read(experienceCreationProvider.notifier).clearImage();
      }
    }
  }

  Future<void> _handleManualCreate() async {
    // Haptic feedback on button tap
    unawaited(HapticFeedback.mediumImpact());

    if (!mounted) return;

    // Reset genExperienceProvider and navigate to preview modal with empty fields
    final genNotifier = ref.read(genExperienceProvider.notifier);
    genNotifier.reset();

    // Close this modal first
    Navigator.of(context).pop();

    // Wait a frame for animation
    await Future.delayed(const Duration(milliseconds: 100));

    if (!mounted) return;

    // Show preview modal with empty state for manual entry
    await showDialog<String>(
      context: context,
      barrierDismissible: false,
      builder: (context) => const ExperiencePreviewModal(),
    );

    // Just close the creation modal - all post-creation work is done in preview modal
    if (mounted) {
      Navigator.of(context).pop();
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(experienceCreationProvider);

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
                    bottom: 140, // Above bottom controls (~160px for controls)
                    child: _buildInstructionOverlay(state),
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

  /// Main content area - shows camera viewport or text/link input.
  Widget _buildMainContent(ExperienceCreationState state) {
    switch (state.inputMode) {
      case CreationInputMode.image:
        return CameraViewport(
          selectedImagePath: state.selectedImagePath,
          onImageCaptured: (path) =>
              ref.read(experienceCreationProvider.notifier).setImagePath(path),
          onImageCleared: () =>
              ref.read(experienceCreationProvider.notifier).clearImage(),
          onCameraReady: (controller) => _cameraController = controller,
          // No instruction text - now shown in bottom overlay
        );
      case CreationInputMode.text:
        return _buildTextInputMode(state);
    }
  }

  /// Builds instruction overlay positioned above bottom controls.
  Widget _buildInstructionOverlay(ExperienceCreationState state) {
    return CreationInstructionOverlay(
      icon: Icons.camera_alt,
      text: 'Capture an activity or flyer',
    );
  }

  /// Builds text/link input mode with instructions integrated into hint text.
  Widget _buildTextInputMode(ExperienceCreationState state) {
    return TextInputArea(
      controller: _textController,
      focusNode: _textFocusNode,
      hintText: context.l10n.creationExperienceHintText,
      minLines: 5,
      maxLines: 5,
      onChanged: (value) =>
          ref.read(experienceCreationProvider.notifier).setTextInput(value),
    );
  }

  // Widget async exception: confirmation dialog before mode switch.
  Future<void> _handleModeChanged(CreationInputMode mode) async {
    final state = ref.read(experienceCreationProvider);
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
    ref.read(experienceCreationProvider.notifier).setInputMode(mode);
  }

  /// Bottom controls with toggle and generate button.
  Widget _buildBottomControls(ExperienceCreationState state) {
    final canGenerate = ref
        .read(experienceCreationProvider.notifier)
        .canGenerate();

    return CreationBottomControls(
      inputMode: state.inputMode,
      onModeChanged: _handleModeChanged,
      isLoading: state.isLoading,
      canGenerate: canGenerate,
      onGenerate: _handleGenerate,
      onManualCreate: _handleManualCreate,
      buttonLabel: 'Create Experience',
      textModeLabel: 'Text / Link',
    );
  }
}
