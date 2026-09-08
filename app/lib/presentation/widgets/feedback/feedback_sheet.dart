import 'package:flutter/foundation.dart' show Uint8List;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

import '../../../core/theme/app_colors.dart';
import '../../../core/utils/media_picker_helper.dart';
import '../../../core/utils/toast_helper.dart';
import '../../../data/gen/ripls/api/feedback_service.pb.dart';
import '../../viewmodels/feedback_sheet_view_model.dart';

/// FeedbackSheet is a bottom-sheet modal for submitting user feedback.
///
/// Use [FeedbackSheet.show] to display the sheet from any context.
class FeedbackSheet extends ConsumerStatefulWidget {
  const FeedbackSheet({super.key});

  /// show opens the feedback sheet as a modal bottom sheet.
  static void show(BuildContext context) {
    showAccessibleModal<void>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => const FeedbackSheet(),
    );
  }

  @override
  ConsumerState<FeedbackSheet> createState() => _FeedbackSheetState();
}

class _FeedbackSheetState extends ConsumerState<FeedbackSheet> {
  final _titleController = TextEditingController();
  final _descriptionController = TextEditingController();

  @override
  void dispose() {
    _titleController.dispose();
    _descriptionController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(feedbackSheetProvider);
    final notifier = ref.read(feedbackSheetProvider.notifier);

    return GlassSheet(
      padding: EdgeInsets.only(
        bottom: MediaQuery.of(context).viewInsets.bottom,
      ),
      child: state.isSubmitted
          ? _buildSuccessState(context, notifier)
          : _buildForm(context, state, notifier),
    );
  }

  Widget _buildForm(
    BuildContext context,
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
  ) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        // Handle bar
        Container(
          margin: const EdgeInsets.only(top: 12, bottom: 4),
          width: 40,
          height: 4,
          decoration: BoxDecoration(
            color: AppColors.modalFooterDivider,
            borderRadius: BorderRadius.circular(2),
          ),
        ),
        Flexible(
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(24, 16, 24, 24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _buildHeader(context),
                const SizedBox(height: 20),
                _buildTypeSelector(context, state, notifier),
                const SizedBox(height: 20),
                _buildTitleField(context, state, notifier),
                const SizedBox(height: 16),
                _buildDescriptionField(context, state, notifier),
                const SizedBox(height: 16),
                _buildScreenshotSection(context, state, notifier),
                if (state.error != null) ...[
                  const SizedBox(height: 16),
                  _buildErrorBanner(
                    context,
                    RpcErrorHandler.localize(state.error!, context.l10n),
                  ),
                ],
                const SizedBox(height: 24),
                _buildSubmitButton(context, state, notifier),
                const SizedBox(height: 12),
                _buildPrivacyFooter(context),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildHeader(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          "We'd love to hear from you!",
          style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                color: AppColors.modalTextPrimary,
                fontWeight: FontWeight.bold,
                fontSize: 22,
              ),
        ),
        const SizedBox(height: 6),
        Text(
          'Share your ideas, report issues, or let us know how we can improve.',
          style: TextStyle(
            fontSize: 14,
            color: AppColors.modalTextSecondary,
          ),
        ),
      ],
    );
  }

  Widget _buildTypeSelector(
    BuildContext context,
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
  ) {
    // Wrap, not Row: three chips on a fixed row overflow once the system text
    // scale climbs (accessibility review §4 asks for 200%).
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        _buildTypeChip(
          state,
          notifier,
          FeedbackType.FEEDBACK_TYPE_BUG_REPORT,
          context.l10n.feedbackTypeBug,
        ),
        _buildTypeChip(
          state,
          notifier,
          FeedbackType.FEEDBACK_TYPE_FEATURE_REQUEST,
          context.l10n.feedbackTypeFeature,
        ),
        _buildTypeChip(
          state,
          notifier,
          FeedbackType.FEEDBACK_TYPE_UNSPECIFIED,
          context.l10n.feedbackTypeOther,
        ),
      ],
    );
  }

  Widget _buildTypeChip(
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
    FeedbackType type,
    String label,
  ) {
    return GlassChip(
      primary: label,
      semanticsLabel: label,
      selected: state.feedbackType == type,
      onTap: () => notifier.setFeedbackType(type),
    );
  }

  Widget _buildTitleField(
    BuildContext context,
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _buildFieldLabel('Title'),
        const SizedBox(height: 6),
        TextField(
          controller: _titleController,
          textCapitalization: TextCapitalization.sentences,
          maxLength: 100,
          onChanged: notifier.setTitle,
          decoration: InputDecoration(
            hintText: 'Brief summary of the issue or feature',
            counterText: '',
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide(color: AppColors.modalPrimaryButtonBackground),
            ),
          ),
        ),
        Align(
          alignment: Alignment.centerRight,
          child: Text(
            '${state.title.length}/100',
            style: TextStyle(
              fontSize: 11,
              color: AppColors.modalTextMuted,
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildDescriptionField(
    BuildContext context,
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _buildFieldLabel('Description'),
        const SizedBox(height: 6),
        TextField(
          controller: _descriptionController,
          textCapitalization: TextCapitalization.sentences,
          maxLines: 5,
          maxLength: 5000,
          onChanged: notifier.setDescription,
          decoration: InputDecoration(
            hintText: 'Describe the issue or feature in detail',
            counterText: '',
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide(color: AppColors.modalPrimaryButtonBackground),
            ),
          ),
        ),
        Align(
          alignment: Alignment.centerRight,
          child: Text(
            '${state.description.length}/5000',
            style: TextStyle(
              fontSize: 11,
              color: AppColors.modalTextMuted,
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildFieldLabel(String label) => GlassFieldLabel(text: label);

  Widget _buildScreenshotSection(
    BuildContext context,
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (state.screenshots.isNotEmpty) ...[
          _buildScreenshotThumbnails(context, state, notifier),
          const SizedBox(height: 8),
        ],
        if (state.screenshots.length < 3) _buildScreenshotButton(context, state, notifier),
      ],
    );
  }

  Widget _buildScreenshotThumbnails(
    BuildContext context,
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
  ) {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: state.screenshots.asMap().entries.map((entry) {
        final index = entry.key;
        final file = entry.value;
        return Stack(
          clipBehavior: Clip.none,
          children: [
            Container(
              width: 80,
              height: 80,
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: AppColors.modalInsetCardBorder),
              ),
              child: ClipRRect(
                borderRadius: BorderRadius.circular(8),
                child: FutureBuilder<Uint8List>(
                  // XFile.readAsBytes works on every platform — including
                  // Flutter Web, where the path is a blob: URL and
                  // Image.file/dart:io File would throw.
                  future: file.readAsBytes(),
                  builder: (context, snapshot) {
                    if (snapshot.hasData) {
                      return Image.memory(snapshot.data!, fit: BoxFit.cover);
                    }
                    return const ColoredBox(color: Colors.black12);
                  },
                ),
              ),
            ),
            Positioned(
              top: -8,
              right: -8,
              child: Tappable(
                semanticsLabel: context.l10n.a11yMiscRemoveScreenshot,
                onTap: () => notifier.removeScreenshot(index),
                child: Container(
                  padding: const EdgeInsets.all(4),
                  decoration: const BoxDecoration(
                    color: Colors.red,
                    shape: BoxShape.circle,
                  ),
                  child: const Icon(Icons.close,
                      color: GlassTokens.textPrimary, size: 14),
                ),
              ),
            ),
          ],
        );
      }).toList(),
    );
  }

  Widget _buildScreenshotButton(
    BuildContext context,
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
  ) {
    // Widget-level async exception: image picking uses platform UI (image_picker)
    // and cannot be driven through the Notifier. The picked file is immediately
    // forwarded to the Notifier for state management.
    //
    // GlassInlineAction is the established treatment for a row that opens a
    // picker — media_picker_dialog.dart uses it for Photos/Videos/Camera. It
    // carries the on-glass fill/foreground pair, so this control no longer
    // invents one.
    return GlassInlineAction(
      icon: Icons.add_photo_alternate_outlined,
      text: state.screenshots.isEmpty
          ? 'Add Screenshots (optional)'
          : 'Add Another (${state.screenshots.length}/3)',
      semanticsLabel: context.l10n.a11yMiscAddScreenshot,
      onTap: () async {
        try {
          final file = await MediaPickerHelper.pickImageFromGallery();
          if (file != null && context.mounted) {
            notifier.addScreenshot(file);
          }
        } catch (e) {
          if (context.mounted) {
            ToastHelper.showError(context, 'Failed to pick image');
          }
        }
      },
    );
  }

  Widget _buildErrorBanner(BuildContext context, String message) {
    return GlassInsetCard(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      borderRadius: BorderRadius.circular(8),
      child: Row(
        children: [
          Icon(Icons.error_outline, color: AppColors.modalTextSecondary, size: 18),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              message,
              style: TextStyle(
                color: AppColors.modalTextSecondary,
                fontSize: 13,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildSubmitButton(
    BuildContext context,
    FeedbackSheetState state,
    FeedbackSheetNotifier notifier,
  ) {
    return GlassFooterButtons(
      showSecondary: false,
      primaryEnabled: state.canSubmit && !state.isSubmitting,
      primaryLabel: 'Submit Feedback',
      onPrimary:
          state.canSubmit && !state.isSubmitting ? notifier.submitFeedback : null,
    );
  }

  Widget _buildPrivacyFooter(BuildContext context) {
    return Text(
      'Your feedback will be submitted as a GitHub issue. Device information '
      'will be included to help us diagnose and improve the app.',
      style: TextStyle(
        fontSize: 12,
        color: AppColors.modalTextMuted,
      ),
      textAlign: TextAlign.center,
    );
  }

  Widget _buildSuccessState(
    BuildContext context,
    FeedbackSheetNotifier notifier,
  ) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 48),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 72,
            height: 72,
            decoration: BoxDecoration(
              color: AppColors.transferSageBackground(context),
              shape: BoxShape.circle,
            ),
            child: Icon(
              Icons.check_rounded,
              color: AppColors.transferSage,
              size: 40,
            ),
          ),
          const SizedBox(height: 20),
          Text(
            'Thanks for your feedback!',
            style: Theme.of(context).textTheme.titleLarge?.copyWith(
                  color: AppColors.modalTextPrimary,
                  fontWeight: FontWeight.bold,
                ),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 12),
          Text(
            "We'll take a look and follow up if needed. Your input helps make "
            'the app better for everyone.',
            style: TextStyle(
              fontSize: 14,
              color: AppColors.modalTextSecondary,
            ),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 20),
          GlassFooterButtons(
            showSecondary: false,
            primaryEnabled: true,
            primaryLabel: context.l10n.commonDone,
            onPrimary: () => Navigator.of(context).pop(),
          ),
        ],
      ),
    );
  }
}
