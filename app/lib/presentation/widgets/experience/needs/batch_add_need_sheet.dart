import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/experience/needs/_sheet_chrome.dart';
import 'package:ripls/presentation/widgets/experience/needs/needs_sheet_models.dart';
import 'package:ripls/presentation/widgets/shared/needs/action_button.dart';
import 'package:ripls/presentation/widgets/shared/needs/checklist_row.dart';

// ── BatchAddNeedSheet ────────────────────────────────────────────────────────
//
// The "Request something" button opens this sheet. Users accumulate a list
// of need drafts before saving. Uses sage-green accent.

/// Batch bottom sheet for adding one or more needs to an experience.
class BatchAddNeedSheet extends StatefulWidget {
  const BatchAddNeedSheet({
    super.key,
    this.suggestions = const [],
    this.existingLabels = const [],
    this.initialText,
    this.onChooseTimeTap,
  });

  final List<String> suggestions;
  final List<String> existingLabels;
  final String? initialText;

  /// When provided, shows an "Agree on Time" pill as the first suggestion.
  /// Tapping it dismisses this sheet and runs the callback (typically opens
  /// the time-poll propose modal).
  final VoidCallback? onChooseTimeTap;

  /// Shows the sheet and returns the batch result on confirmation.
  static Future<BatchAddNeedResult?> show(
    BuildContext context, {
    List<String> suggestions = const [],
    List<String> existingLabels = const [],
    String? initialText,
    VoidCallback? onChooseTimeTap,
  }) {
    return showAccessibleModal<BatchAddNeedResult>(context,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      isScrollControlled: true,
      builder: (_) => BatchAddNeedSheet(
        suggestions: suggestions,
        existingLabels: existingLabels,
        initialText: initialText,
        onChooseTimeTap: onChooseTimeTap,
      ),
    );
  }

  @override
  State<BatchAddNeedSheet> createState() => _BatchAddNeedSheetState();
}

class _BatchAddNeedSheetState extends State<BatchAddNeedSheet> {
  late final TextEditingController _nameController;
  final List<NeedDraft> _pending = [];
  bool _showDiscard = false;
  bool _isClosing = false;

  @override
  void initState() {
    super.initState();
    _nameController = TextEditingController(text: widget.initialText ?? '');
    if (widget.initialText != null) {
      _nameController.selection =
          TextSelection.collapsed(offset: _nameController.text.length);
    }
  }

  @override
  void dispose() {
    _nameController.dispose();
    super.dispose();
  }

  List<String> get _filteredSuggestions {
    final lower = {
      ...widget.existingLabels.map((e) => e.toLowerCase()),
      ..._pending.map((d) => d.name.toLowerCase()),
    };
    return widget.suggestions
        .where((s) => !lower.contains(s.toLowerCase()))
        .toList();
  }

  bool get _hasInput => _nameController.text.trim().isNotEmpty;

  /// Lowercased set of titles that already exist (asked-for needs + offers
  /// passed in by the caller, plus drafts already in the pending list).
  /// Used to block typed and tapped duplicates.
  Set<String> get _disallowedTitles => {
        ...widget.existingLabels.map((e) => e.toLowerCase()),
        ..._pending.map((d) => d.name.toLowerCase()),
      };

  bool get _currentInputIsDuplicate =>
      _hasInput &&
      _disallowedTitles.contains(_nameController.text.trim().toLowerCase());

  void _addToList() {
    if (!_hasInput || _currentInputIsDuplicate) return;
    setState(() {
      _pending.add(NeedDraft(
        name: _nameController.text.trim(),
        note: null,
        slots: 1,
      ));
      _nameController.clear();
    });
  }

  /// Pushes [label] onto the pending list directly — used when the user
  /// taps a suggestion. Mirrors [_addToList] but bypasses the input
  /// field so the modal stays open with the suggestion accumulated.
  /// Silently ignores duplicates as a safety net (suggestion list is
  /// already filtered, but a race with state updates could surface a
  /// stale label).
  void _addLabelToList(String label) {
    final trimmed = label.trim();
    if (trimmed.isEmpty) return;
    if (_disallowedTitles.contains(trimmed.toLowerCase())) return;
    setState(() {
      _pending.add(NeedDraft(name: trimmed, note: null, slots: 1));
    });
  }

  void _removeFromList(int index) =>
      setState(() => _pending.removeAt(index));

  void _editFromList(int index) {
    final draft = _pending[index];
    setState(() {
      _pending.removeAt(index);
      _nameController.text = draft.name;
      _nameController.selection =
          TextSelection.collapsed(offset: draft.name.length);
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
    Navigator.of(context).pop(BatchAddNeedResult(List.from(_pending)));
  }

  @override
  Widget build(BuildContext context) {
    final chips = _filteredSuggestions;
    final canCommitInput = _hasInput && !_currentInputIsDuplicate;
    final saveCount = _pending.length + (canCommitInput ? 1 : 0);

    return PopScope(
      canPop: false,
      // Only run the discard-confirmation flow when the framework
      // *blocks* a pop (canPop:false → didPop:false). When the sheet
      // is popping for real (e.g. the time-agreement tap calls
      // Navigator.pop directly, or _save pops with a result),
      // didPop is true and re-entering _attemptClose would cause a
      // recursive Navigator.pop while the first one is still
      // flushing — that's the _debugLocked assertion failure.
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
                  accent: AppColors.experienceSageGreen,
                  statusLabel: _pending.isEmpty
                      ? 'Ask the group'
                      : '${_pending.length} on your list',
                  title: 'Request',
                  accentText: 'something.',
                  onClose: _attemptClose,
                ),
                if (_pending.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  buildPendingList(
                    context: context,
                    accent: AppColors.experienceSageGreen,
                    items: _pending
                        .map((d) => PendingRowData(
                              title: d.name,
                              subtitle: d.note,
                            ))
                        .toList(),
                    onTap: _editFromList,
                    onRemove: _removeFromList,
                  ),
                ],
                const SizedBox(height: 20),
                buildInputField(
                  context: context,
                  controller: _nameController,
                  accent: AppColors.experienceSageGreen,
                  hint: _pending.isEmpty
                      ? context.l10n.experienceBatchNeedInputHint
                      : context.l10n.experienceBatchInputHintSubsequent,
                  onChanged: () => setState(() {}),
                  onSubmitted: _addToList,
                ),
                if (chips.isNotEmpty || widget.onChooseTimeTap != null) ...[
                  const SizedBox(height: 14),
                  NeedsSectionHeader(
                    title: context.l10n.suggestionsHeader,
                  ),
                  const SizedBox(height: 4),
                  // LLM thing-suggestions go in the two-column grid.
                  if (chips.isNotEmpty)
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
                  // "Agree Upon Time" is a distinct, longer-label
                  // affordance — gets its own full-width row at the
                  // bottom of the section so the text isn't truncated
                  // and the action is visually separated.
                  if (widget.onChooseTimeTap != null)
                    SuggestionRow(
                      label: context.l10n.planPollSuggestionTime,
                      chip: PollChip(
                        label: context.l10n.planSuggestionPollChip,
                      ),
                      actionLabel: '',
                      actionSemanticsLabel:
                          context.l10n.planPollSuggestionTime,
                      onAction: () {
                        Navigator.of(context).pop();
                        widget.onChooseTimeTap!();
                      },
                      showCheckbox: false,
                      showDivider: false,
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

