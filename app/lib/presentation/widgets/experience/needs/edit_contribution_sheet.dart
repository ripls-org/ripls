import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart'
    show ExperienceContributionResponse;
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/experience/needs/_sheet_chrome.dart';
import 'package:ripls/presentation/widgets/experience/needs/needs_sheet_models.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_footer_buttons.dart';
import 'package:timeago/timeago.dart' as timeago;

// ── EditContributionSheet ───────────────────────────────────────────────────
//
// Opened when the user taps their own contribution chip. Two layouts share
// the same chrome (status dot, serif title, NOTE section, primary coral
// button, secondary link) but differ in action:
//
//   - claimed   → "You're bringing X." with Done / Release-back-to-group
//   - freeform  → "You're bringing X." with editable title/note fields and
//                 Save / Remove. Editing reveals the form; by default the
//                 user sees the same read-like display as a receipt.

/// Bottom sheet for editing or removing an owned contribution.
class EditContributionSheet extends StatefulWidget {
  const EditContributionSheet({super.key, required this.contribution});

  final ExperienceContributionResponse contribution;

  /// Shows the sheet and returns the result when the user acts or dismisses.
  static Future<EditContributionResult?> show(
    BuildContext context,
    ExperienceContributionResponse contribution,
  ) {
    return showAccessibleModal<EditContributionResult>(context,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      isScrollControlled: true,
      builder: (_) => EditContributionSheet(contribution: contribution),
    );
  }

  @override
  State<EditContributionSheet> createState() => _EditContributionSheetState();
}

class _EditContributionSheetState extends State<EditContributionSheet> {
  late final TextEditingController _titleController;
  late final TextEditingController _descriptionController;

  bool _editing = false;
  bool _confirming = false;

  String get _originalTitle => widget.contribution.title;
  String get _originalNote => widget.contribution.hasDescription()
      ? widget.contribution.description
      : '';

  bool get _isClaimed =>
      widget.contribution.hasFromNeedId() &&
      widget.contribution.fromNeedId.isNotEmpty;

  bool get _isDirty =>
      _titleController.text.trim() != _originalTitle.trim() ||
      _descriptionController.text.trim() != _originalNote.trim();

  bool get _canSave =>
      _titleController.text.trim().isNotEmpty && _isDirty;

  @override
  void initState() {
    super.initState();
    _titleController = TextEditingController(text: _originalTitle);
    _descriptionController = TextEditingController(text: _originalNote);
  }

  @override
  void dispose() {
    _titleController.dispose();
    _descriptionController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.contribution;
    final claimedAt = c.createdAtUnixSec.toInt() > 0
        ? timeago.format(DateTime.fromMillisecondsSinceEpoch(
            c.createdAtUnixSec.toInt() * 1000))
        : null;
    final hasAskNote = c.originalNeedNote.isNotEmpty;

    final statusLabel = [
      _isClaimed ? 'Claimed' : 'Your offer',
      ?claimedAt,
    ].join(' · ');

    return buildSheetContainer(
      context: context,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          buildDragHandle(context),
          const SizedBox(height: 8),
          buildStatusBadge(
            context: context,
            dotColor: AppColors.experienceSageGreen,
            label: statusLabel,
          ),
          const SizedBox(height: 20),
          buildSerifTitle(
            context,
            primary: "You're bringing",
            accent: _editing ? null : '${c.title}.',
          ),
          const SizedBox(height: 20),
          if (_isClaimed && hasAskNote) ...[
            buildNoteSection(context, c.originalNeedNote),
            const SizedBox(height: 16),
          ],
          if (_editing) ...[
            buildInputField(
              context: context,
              controller: _titleController,
              accent: AppColors.transferCoral,
              hint: context.l10n.experienceContributionsTitleLabel,
              onChanged: () => setState(() {}),
            ),
            const SizedBox(height: 10),
            buildNoteInputField(
              context: context,
              controller: _descriptionController,
              hint: context.l10n.experienceBatchNoteHint,
            ),
          ] else if (!_confirming && _originalNote.isNotEmpty) ...[
            buildNoteSection(context, _originalNote, label: 'YOUR NOTE'),
            const SizedBox(height: 8),
          ],
          if (_confirming) ...[
            _buildRemoveConfirmation(context, c.title),
          ] else ...[
            const SizedBox(height: 18),
            if (_editing)
              GlassFooterButtons(
                primaryLabel: _primaryLabel(),
                primaryEnabled: _primaryAction() != null,
                onPrimary: _primaryAction(),
                secondaryLabel: 'Cancel',
                onSecondary: () => setState(() {
                  _editing = false;
                  _titleController.text = _originalTitle;
                  _descriptionController.text = _originalNote;
                }),
              )
            else if (_isClaimed)
              // Claimed contributions collapse the old
              // "Release back to the group" + "Remove this contribution"
              // pair into a single Remove. UnclaimNeed both deletes the
              // contribution and frees the slot on the need.
              GlassFooterButtons(
                primaryLabel: _primaryLabel(),
                primaryEnabled: true,
                onPrimary: _primaryAction(),
                secondaryLabel: 'Remove',
                onSecondary: () => Navigator.of(context).pop(
                  const EditContributionResult(action: EditAction.unclaim),
                ),
              )
            else
              GlassFooterButtons(
                primaryLabel: _primaryLabel(),
                primaryEnabled: true,
                onPrimary: _primaryAction(),
                secondaryLabel: 'Edit',
                onSecondary: () => setState(() => _editing = true),
              ),
            // Freeform offers (no underlying need) still need a Remove
            // affordance — kept as a destructive tertiary below.
            if (!_editing && !_isClaimed)
              buildSecondaryLink(
                context: context,
                label: 'Remove this contribution',
                onPressed: () => setState(() => _confirming = true),
                color: AppColors.statusErrorOnDark,
              ),
          ],
        ],
      ),
    );
  }

  String _primaryLabel() {
    if (_editing) return _canSave ? 'Save changes' : 'No changes';
    return 'Done';
  }

  VoidCallback? _primaryAction() {
    if (_editing) {
      if (!_canSave) return null;
      return () => Navigator.of(context).pop(EditContributionResult(
            action: EditAction.save,
            title: _titleController.text.trim(),
            description: _descriptionController.text.trim().isEmpty
                ? null
                : _descriptionController.text.trim(),
          ));
    }
    return () => Navigator.of(context).pop(null);
  }

  Widget _buildRemoveConfirmation(BuildContext context, String title) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: AppColors.statusErrorOnDark.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            'Remove this contribution?',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: AppColors.statusErrorOnDark,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            '"$title" will be removed. You can add it back any time.',
            style: TextStyle(
              fontSize: 12,
              color: AppColors.modalTextSecondary,
              height: 1.5,
            ),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              Expanded(
                child: OutlinedButton(
                  onPressed: () => setState(() => _confirming = false),
                  style: OutlinedButton.styleFrom(
                    side: BorderSide(color: AppColors.modalSecondaryButtonBorder),
                    shape: const StadiumBorder(),
                    padding: const EdgeInsets.symmetric(vertical: 10),
                  ),
                  child: Text(
                    'Keep it',
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                      color: AppColors.modalTextSecondary,
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: ElevatedButton(
                  onPressed: () => Navigator.of(context).pop(
                    const EditContributionResult(action: EditAction.delete),
                  ),
                  style: ElevatedButton.styleFrom(
                    backgroundColor: AppColors.statusErrorOnDark,
                    shape: const StadiumBorder(),
                    padding: const EdgeInsets.symmetric(vertical: 10),
                  ),
                  child: const Text(
                    'Remove',
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w700,
                      color: GlassTokens.textPrimary,
                    ),
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
