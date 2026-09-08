import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/experience/needs/_shared_sheet_widgets.dart';
import 'package:ripls/presentation/widgets/experience/needs/_sheet_chrome.dart';
import 'package:ripls/presentation/widgets/experience/needs/needs_sheet_models.dart';
import 'package:ripls/presentation/widgets/shared/needs/action_button.dart';
import 'package:ripls/presentation/widgets/shared/needs/checklist_row.dart';

// ── BatchAddContributionSheet ────────────────────────────────────────────────
//
// The "Offer something" button opens this sheet. Users accumulate a list of
// contribution drafts before saving. Uses coral accent and shows the RSVP
// disclosure when the user isn't already RSVPed.

/// Batch bottom sheet for adding one or more free-form contributions.
class BatchAddContributionSheet extends StatefulWidget {
  const BatchAddContributionSheet({
    super.key,
    this.suggestions = const [],
    this.existingLabels = const [],
    this.isRsvped = true,
    this.isRsvpedMaybe = false,
    this.experienceName = '',
    this.initialText,
    this.onChooseTimeTap,
  });

  final List<String> suggestions;
  final List<String> existingLabels;
  final bool isRsvped;
  final bool isRsvpedMaybe;
  final String experienceName;
  final String? initialText;

  /// When provided, shows an "Agree on Time" pill as the first suggestion.
  /// Tapping it dismisses this sheet and runs the callback.
  final VoidCallback? onChooseTimeTap;

  /// Shows the sheet and returns the batch result on confirmation.
  static Future<BatchAddContributionResult?> show(
    BuildContext context, {
    List<String> suggestions = const [],
    List<String> existingLabels = const [],
    bool isRsvped = true,
    bool isRsvpedMaybe = false,
    String experienceName = '',
    String? initialText,
    VoidCallback? onChooseTimeTap,
  }) {
    return showAccessibleModal<BatchAddContributionResult>(context,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      isScrollControlled: true,
      builder: (_) => BatchAddContributionSheet(
        suggestions: suggestions,
        existingLabels: existingLabels,
        isRsvped: isRsvped,
        isRsvpedMaybe: isRsvpedMaybe,
        experienceName: experienceName,
        initialText: initialText,
        onChooseTimeTap: onChooseTimeTap,
      ),
    );
  }

  @override
  State<BatchAddContributionSheet> createState() =>
      _BatchAddContributionSheetState();
}

class _BatchAddContributionSheetState
    extends State<BatchAddContributionSheet> {
  late final TextEditingController _titleController;
  final List<ContributionDraft> _pending = [];
  bool _showDiscard = false;
  bool _isClosing = false;
  late bool _preferMaybe;

  @override
  void initState() {
    super.initState();
    _preferMaybe = widget.isRsvpedMaybe;
    _titleController = TextEditingController(text: widget.initialText ?? '');
    if (widget.initialText != null) {
      _titleController.selection =
          TextSelection.collapsed(offset: _titleController.text.length);
    }
  }

  @override
  void dispose() {
    _titleController.dispose();
    super.dispose();
  }

  List<String> get _filteredSuggestions {
    final lower = {
      ...widget.existingLabels.map((e) => e.toLowerCase()),
      ..._pending.map((d) => d.title.toLowerCase()),
    };
    return widget.suggestions
        .where((s) => !lower.contains(s.toLowerCase()))
        .toList();
  }

  bool get _hasInput => _titleController.text.trim().isNotEmpty;
  bool get _showDisclosure => !widget.isRsvped;

  /// Lowercased set of titles already on the list — used to block
  /// typed and tapped duplicates.
  Set<String> get _disallowedTitles => {
        ...widget.existingLabels.map((e) => e.toLowerCase()),
        ..._pending.map((d) => d.title.toLowerCase()),
      };

  bool get _currentInputIsDuplicate =>
      _hasInput &&
      _disallowedTitles
          .contains(_titleController.text.trim().toLowerCase());

  void _addToList() {
    if (!_hasInput || _currentInputIsDuplicate) return;
    setState(() {
      _pending.add(ContributionDraft(
        title: _titleController.text.trim(),
        description: null,
      ));
      _titleController.clear();
    });
  }

  /// Pushes [label] onto the pending list directly — used when the user
  /// taps a suggestion. Silently skips duplicates.
  void _addLabelToList(String label) {
    final trimmed = label.trim();
    if (trimmed.isEmpty) return;
    if (_disallowedTitles.contains(trimmed.toLowerCase())) return;
    setState(() {
      _pending.add(ContributionDraft(title: trimmed, description: null));
    });
  }

  void _removeFromList(int index) =>
      setState(() => _pending.removeAt(index));

  void _editFromList(int index) {
    final draft = _pending[index];
    setState(() {
      _pending.removeAt(index);
      _titleController.text = draft.title;
      _titleController.selection =
          TextSelection.collapsed(offset: draft.title.length);
    });
  }

  void _attemptClose() {
    if (_isClosing) return;
    if (_pending.isNotEmpty || _hasInput) {
      setState(() => _showDiscard = true);
    } else {
      _isClosing = true;
      Navigator.of(context).pop();
    }
  }

  void _save() {
    if (_hasInput) _addToList();
    if (_pending.isEmpty) return;
    Navigator.of(context).pop(BatchAddContributionResult(
      List.from(_pending),
      preferMaybe: _showDisclosure && _preferMaybe,
    ));
  }

  @override
  Widget build(BuildContext context) {
    final chips = _filteredSuggestions;
    final canCommitInput = _hasInput && !_currentInputIsDuplicate;
    final saveCount = _pending.length + (canCommitInput ? 1 : 0);

    return PopScope(
      canPop: false,
      // Only run the discard-confirmation flow when the framework
      // blocks a pop. See batch_add_need_sheet.dart for the full
      // explanation — short version: re-entering _attemptClose while
      // a programmatic Navigator.pop is still flushing trips the
      // _debugLocked assertion.
      onPopInvokedWithResult: (didPop, result) {
        if (didPop) return;
        _attemptClose();
      },
      child: Stack(
        children: [
          buildSheetContainer(
            context: context,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                buildDragHandle(context),
                const SizedBox(height: 8),
                buildBatchHeader(
                  context: context,
                  accent: AppColors.transferCoral,
                  statusLabel: _pending.isEmpty
                      ? "What you'll bring"
                      : '${_pending.length} on your list',
                  title: 'Offer',
                  accentText: 'something.',
                  onClose: _attemptClose,
                ),
                if (_pending.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  buildPendingList(
                    context: context,
                    accent: AppColors.transferCoral,
                    items: _pending
                        .map((d) => PendingRowData(
                              title: d.title,
                              subtitle: d.description,
                            ))
                        .toList(),
                    onTap: _editFromList,
                    onRemove: _removeFromList,
                  ),
                ],
                const SizedBox(height: 20),
                buildInputField(
                  context: context,
                  controller: _titleController,
                  accent: AppColors.transferCoral,
                  hint: _pending.isEmpty
                      ? context.l10n.experienceBatchOfferInputHint
                      : context.l10n.experienceBatchInputHintSubsequent,
                  onChanged: () => setState(() {}),
                  onSubmitted: _addToList,
                ),
                if (chips.isNotEmpty) ...[
                  const SizedBox(height: 14),
                  NeedsSectionHeader(
                    title: context.l10n.suggestionsHeader,
                  ),
                  const SizedBox(height: 4),
                  buildChecklistTwoColumns<String>(
                    items: chips,
                    buildRow: (label, showDivider) => SuggestionRow(
                      label: label,
                      actionLabel: '',
                      actionSemanticsLabel: label,
                      onAction: () => _addLabelToList(label),
                      showCheckbox: false,
                      showDivider: showDivider,
                    ),
                  ),
                ],
                if (_showDisclosure) ...[
                  const SizedBox(height: 14),
                  RsvpDisclosure(
                    isRsvpedMaybe: widget.isRsvpedMaybe,
                    preferMaybe: _preferMaybe,
                    experienceName: widget.experienceName,
                    onToggle: () =>
                        setState(() => _preferMaybe = !_preferMaybe),
                  ),
                ],
                const SizedBox(height: 20),
                buildBatchActionRow(
                  context: context,
                  addLabel: context.l10n.experienceBatchAddToList,
                  saveLabel: context.l10n.experienceBatchSaveLabel(saveCount),
                  canAdd: canCommitInput,
                  canSave: saveCount > 0,
                  onAdd: _addToList,
                  onSave: _save,
                ),
              ],
            ),
          ),
          if (_showDiscard)
            buildDiscardOverlay(
              context: context,
              count: _pending.length,
              onKeep: () => setState(() => _showDiscard = false),
              onDiscard: () => Navigator.of(context).pop(),
            ),
        ],
      ),
    );
  }
}
