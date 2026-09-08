import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/experience/needs/_shared_sheet_widgets.dart';
import 'package:ripls/presentation/widgets/experience/needs/_sheet_chrome.dart';
import 'package:ripls/presentation/widgets/experience/needs/needs_sheet_models.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_footer_buttons.dart';

// ── AddContributionSheet (claim-a-need) ─────────────────────────────────────
//
// "Bring \n extra layers?" — the canonical editorial claim sheet. Opened when
// a user taps "I got this" on a need from [ViewNeedSheet].

/// Bottom sheet for claiming a specific need.
class AddContributionSheet extends StatefulWidget {
  const AddContributionSheet({
    super.key,
    required this.needName,
    this.needNote,
    this.proposerName,
    this.isRsvped = true,
    this.isRsvpedMaybe = false,
    this.experienceName = '',
    this.canRemove = false,
  });

  final String needName;
  final String? needNote;
  final String? proposerName;
  final bool isRsvped;
  final bool isRsvpedMaybe;
  final String experienceName;

  /// When true, the secondary button reads "Remove" and emits a
  /// remove-flagged result instead of the passive "Maybe later"
  /// dismiss. Should only be set when the current user is the need's
  /// proposer.
  final bool canRemove;

  /// Shows the sheet and returns the result when the user confirms or dismisses.
  static Future<AddContributionResult?> show(
    BuildContext context, {
    required String needName,
    String? needNote,
    String? proposerName,
    bool isRsvped = true,
    bool isRsvpedMaybe = false,
    String experienceName = '',
    bool canRemove = false,
  }) {
    return showAccessibleModal<AddContributionResult>(context,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      isScrollControlled: true,
      builder: (_) => AddContributionSheet(
        needName: needName,
        needNote: needNote,
        proposerName: proposerName,
        isRsvped: isRsvped,
        isRsvpedMaybe: isRsvpedMaybe,
        experienceName: experienceName,
        canRemove: canRemove,
      ),
    );
  }

  @override
  State<AddContributionSheet> createState() => _AddContributionSheetState();
}

class _AddContributionSheetState extends State<AddContributionSheet> {
  final _commentController = TextEditingController();
  late bool _preferMaybe;

  bool get _showDisclosure => !widget.isRsvped;

  @override
  void initState() {
    super.initState();
    _preferMaybe = widget.isRsvpedMaybe;
  }

  @override
  void dispose() {
    _commentController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final statusLabel = [
      'Open',
      if (widget.proposerName != null && widget.proposerName!.isNotEmpty)
        'asked by ${widget.proposerName}',
    ].join(' · ');
    final hasNote = widget.needNote != null && widget.needNote!.isNotEmpty;

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
            dotColor: AppColors.transferCoral,
            label: statusLabel,
          ),
          const SizedBox(height: 20),
          buildSerifTitle(
            context,
            primary: 'Bring',
            accent: '${widget.needName}?',
          ),
          const SizedBox(height: 20),
          if (hasNote) ...[
            buildNoteSection(context, widget.needNote!),
            const SizedBox(height: 16),
          ],
          buildNoteInputField(
            context: context,
            controller: _commentController,
            hint: context.l10n.experienceBatchNoteHint,
          ),
          if (_showDisclosure) ...[
            const SizedBox(height: 14),
            RsvpDisclosure(
              isRsvpedMaybe: widget.isRsvpedMaybe,
              preferMaybe: _preferMaybe,
              experienceName: widget.experienceName,
              onToggle: () => setState(() => _preferMaybe = !_preferMaybe),
            ),
          ],
          const SizedBox(height: 18),
          GlassFooterButtons(
            primaryLabel: "I'll bring it",
            primaryEnabled: true,
            onPrimary: () => Navigator.of(context).pop(
              AddContributionResult(
                title: widget.needName,
                description: _commentController.text.trim().isEmpty
                    ? null
                    : _commentController.text.trim(),
                preferMaybe: _showDisclosure && _preferMaybe,
              ),
            ),
            secondaryLabel: widget.canRemove ? 'Remove' : 'Maybe later',
            onSecondary: widget.canRemove
                ? () => Navigator.of(context).pop(
                      const AddContributionResult(title: '', remove: true),
                    )
                : () => Navigator.of(context).pop(null),
          ),
        ],
      ),
    );
  }
}
