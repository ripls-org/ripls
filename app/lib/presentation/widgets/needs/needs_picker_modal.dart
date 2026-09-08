// dart-line-count-allow: NK1/NK3/NK4 picker flows + new edit-mode
// confirm body (TITLE / QUANTITY / COMMENT layout per
// claim-sheet-prototype.html) live here. Splitting per-flow is
// tracked in #2148 alongside the broader needs-v2 cleanup.
import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/gear/gear_pill.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_gear_picker.dart';
import 'package:ripls/presentation/widgets/needs/needs_copy.dart';
import 'package:ripls/presentation/widgets/needs/needs_edit_atoms.dart';
import 'package:ripls/presentation/widgets/needs/needs_suggestion_grid.dart';

/// Result returned from [NeedsPickerModal] when the user confirms.
class NeedsPickerResult {
  final String name;

  /// Total slots needed (>= 1). v1 ships without the total/each unit
  /// toggle from NK3/NK4 — the value is always interpreted as total
  /// slots, matching the existing `Need.slots` semantics. The unit
  /// toggle is tracked as a deferred follow-up; see `docs/client/needs.md`.
  final int slots;

  /// Optional note attached to the Need.
  final String? note;

  /// True when the user tapped **Remove** in the NK4 edit variant
  /// instead of Save. In Add mode (NK3) this is always false.
  final bool removed;

  /// Optional Gear item the contributor linked to this claim. At
  /// most one gear per claim — multi-link is not supported.
  final String? gearId;

  const NeedsPickerResult({
    required this.name,
    required this.slots,
    this.note,
    this.removed = false,
    this.gearId,
  });
}

/// Picker modal — NK1 ("What are we adding?") + NK3 (confirm view).
///
/// NK1 is the default state: search pill + suggestion grid (two-column
/// rectangular tiles). Tapping any suggestion or submitting the search
/// field transitions to NK3 — the chosen item sits at the top with a
/// qty stepper and an optional note. NK3's header back-chip returns to
/// NK1 ("Pick another").
///
/// Saved kits (NK1 right-hand rail in the mock) are intentionally
/// **not** wired in v1 — kits are a separate persistence concept,
/// tracked as a deferred follow-up in `docs/client/needs.md`.
class NeedsPickerModal extends StatefulWidget {
  /// Scope flavour — picks Experience vs. Request copy via [NeedsCopy].
  final NeedsScopeKind scopeKind;

  /// Suggestion tiles for the NK1 grid. Pass `[]` to render
  /// search-only (the search pill is the universal custom-add entry,
  /// so the picker is fully usable with no suggestions).
  final List<NeedsSuggestion> suggestions;

  /// Optional category hint rendered as "Suggested for {hint}" above
  /// the grid (e.g. "a climbing day"). When null the grid renders
  /// under the generic "Suggestions" label.
  final String? categoryHint;

  /// Case-insensitive set of names already on the parent list.
  /// Suggestion tiles whose name matches one of these render in their
  /// "taken" state (sage outline, "on the list" subtitle, sage check)
  /// and are inert. v2 dedup affordance — see NK1 in the redesign.
  final Set<String> takenNames;

  /// NK4 edit-mode flag. When true the picker skips NK1 entirely and
  /// opens straight into the NK3 confirm view, pre-populated with
  /// [initialName] / [initialSlots] / [initialNote]. The bottom
  /// buttons swap Cancel → **Remove** and Add to list → **Save**.
  final bool editMode;

  /// Pre-populated values used by [editMode]. Ignored otherwise.
  final String? initialName;
  final int initialSlots;
  final String? initialNote;

  /// One-line "Already claimed" summary rendered below the selected
  /// card in [editMode]. Caller pre-renders (e.g. "Maya brings 4 ·
  /// Henry brings 2"); the picker just displays it inside a chip.
  final String? existingClaimsSummary;

  /// Community context used to scope the gear-link search (single
  /// community per parent entity). When null or empty, the link
  /// picker degrades to a library-only list with no search.
  final String? communityId;

  /// Pre-selected gear link, used in edit mode (NK4) to surface the
  /// existing link on open.
  final String? initialGearId;

  const NeedsPickerModal({
    super.key,
    required this.scopeKind,
    required this.suggestions,
    this.categoryHint,
    this.takenNames = const {},
    this.editMode = false,
    this.initialName,
    this.initialSlots = 1,
    this.initialNote,
    this.existingClaimsSummary,
    this.communityId,
    this.initialGearId,
  });

  static Future<NeedsPickerResult?> show(
    BuildContext context, {
    required NeedsScopeKind scopeKind,
    required List<NeedsSuggestion> suggestions,
    String? categoryHint,
    Set<String> takenNames = const {},
    String? communityId,
  }) {
    return showAccessibleModal<NeedsPickerResult>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => NeedsPickerModal(
        scopeKind: scopeKind,
        suggestions: suggestions,
        categoryHint: categoryHint,
        takenNames: takenNames,
        communityId: communityId,
      ),
    );
  }

  /// NK4 entry point — opens the picker straight into the confirm
  /// view with the supplied need pre-populated. The result carries
  /// `removed: true` when the user tapped Remove instead of Save.
  static Future<NeedsPickerResult?> showEdit(
    BuildContext context, {
    required NeedsScopeKind scopeKind,
    required String name,
    int slots = 1,
    String? note,
    String? existingClaimsSummary,
    String? communityId,
    String? initialGearId,
  }) {
    return showAccessibleModal<NeedsPickerResult>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => NeedsPickerModal(
        scopeKind: scopeKind,
        suggestions: const [],
        editMode: true,
        initialName: name,
        initialSlots: slots,
        initialNote: note,
        existingClaimsSummary: existingClaimsSummary,
        communityId: communityId,
        initialGearId: initialGearId,
      ),
    );
  }

  @override
  State<NeedsPickerModal> createState() => _NeedsPickerModalState();
}

class _NeedsPickerModalState extends State<NeedsPickerModal> {
  final TextEditingController _search = TextEditingController();
  String? _confirmedName;
  int _slots = 1;
  late final TextEditingController _note;

  /// Live editable title used by the NK4 edit-variant TITLE input.
  /// In add mode this controller is created but unused — the confirm
  /// view there renders the chosen name as static text.
  late final TextEditingController _nameCtrl;

  /// Currently-selected gear-link. Null means "no link." Carried
  /// through the confirm view's "Link gear" disclosure.
  String? _gearId;

  bool get _isConfirming => _confirmedName != null;

  @override
  void initState() {
    super.initState();
    _note = TextEditingController(text: widget.initialNote ?? '');
    _nameCtrl = TextEditingController(text: widget.initialName ?? '');
    _gearId = widget.initialGearId;
    if (widget.editMode) {
      _confirmedName = widget.initialName;
      _slots = widget.initialSlots;
    }
  }

  @override
  void dispose() {
    _search.dispose();
    _note.dispose();
    _nameCtrl.dispose();
    super.dispose();
  }

  void _confirm(String name) {
    setState(() {
      _confirmedName = name;
      _slots = 1;
      _note.clear();
    });
  }

  void _backToPick() {
    // In edit mode the back chip is hidden — there's no NK1 to return
    // to. Defensive guard for callers that try to invoke it anyway.
    if (widget.editMode) return;
    setState(() {
      _confirmedName = null;
      _search.clear();
    });
  }

  void _submitConfirm() {
    // In edit mode the TITLE input is editable, so prefer the live
    // controller value. In add mode the name is locked to the row
    // tapped on NK1 / submitted on the search pill.
    final name = widget.editMode
        ? _nameCtrl.text.trim()
        : (_confirmedName ?? '').trim();
    if (name.isEmpty) return;
    final noteText = _note.text.trim();
    Navigator.of(context).pop(NeedsPickerResult(
      name: name,
      slots: _slots,
      note: noteText.isEmpty ? null : noteText,
      gearId: _gearId,
    ));
  }

  void _submitRemove() {
    final name = (widget.editMode
            ? _nameCtrl.text
            : (_confirmedName ?? widget.initialName ?? ''))
        .trim();
    Navigator.of(context).pop(NeedsPickerResult(
      name: name,
      slots: _slots,
      removed: true,
      gearId: _gearId,
    ));
  }

  Future<void> _openGearLinkPicker() async {
    final result = await NeedsClaimGearPicker.show(
      context,
      communityId: widget.communityId,
      initialQuery: _confirmedName,
      initialGearId: _gearId,
    );
    if (result == null) return;
    if (!mounted) return;
    setState(() => _gearId = result.gearId);
  }

  void _onSearchSubmit(String value) {
    final trimmed = value.trim();
    if (trimmed.isEmpty) return;
    _confirm(trimmed);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, widget.scopeKind);
    final keyboardInset = MediaQuery.of(context).viewInsets.bottom;
    return GlassSheet(
      padding: EdgeInsets.zero,
      child: SafeArea(
        top: false,
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.of(context).size.height * 0.85,
          ),
          child: _isConfirming
              ? (widget.editMode
                  ? _buildEditConfirmBody(context, l10n, copy, keyboardInset)
                  : _buildConfirmBody(context, l10n, copy, keyboardInset))
              : _buildPickBody(context, l10n, copy, keyboardInset),
        ),
      ),
    );
  }

  // ── NK1 — default picker ────────────────────────────────────────────────

  Widget _buildPickBody(
    BuildContext context,
    AppLocalizations l10n,
    NeedsCopy copy,
    double keyboardInset,
  ) {
    final sectionLabel = widget.categoryHint != null
        ? copy.pickerSuggestionsForCategory(widget.categoryHint!)
        : copy.pickerSuggestionsHeader;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 12, 0),
          child: Row(
            children: [
              Expanded(
                child: _Eyebrow(text: copy.pickerTitle),
              ),
              IconAction(
                icon: Icons.close,
                tooltip: l10n.a11yClose,
                semanticsLabel: l10n.a11yClose,
                onPressed: () => Navigator.of(context).pop(),
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              _Title(text: copy.pickerHeadline),
              const SizedBox(height: 6),
              _Subtitle(text: copy.pickerSubtitle),
            ],
          ),
        ),
        Flexible(
          child: SingleChildScrollView(
            padding: EdgeInsets.fromLTRB(20, 18, 20, 24 + keyboardInset),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              mainAxisSize: MainAxisSize.min,
              children: [
                _SearchPill(
                  controller: _search,
                  hint: copy.pickerSearchHint,
                  onSubmitted: _onSearchSubmit,
                ),
                if (widget.suggestions.isNotEmpty) ...[
                  const SizedBox(height: 18),
                  _SectionLabel(label: sectionLabel),
                  const SizedBox(height: 8),
                  _SuggestTileGrid(
                    suggestions: widget.suggestions,
                    takenNames: widget.takenNames,
                    takenSubtitle: copy.pickerTakenSubtitle,
                    onSelect: (s) => _confirm(s.name),
                  ),
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }

  // ── NK3 — confirm view ─────────────────────────────────────────────────

  Widget _buildConfirmBody(
    BuildContext context,
    AppLocalizations l10n,
    NeedsCopy copy,
    double keyboardInset,
  ) {
    final name = _confirmedName ?? '';
    final isEdit = widget.editMode;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(12, 4, 12, 0),
          child: Row(
            children: [
              if (!isEdit)
                IconAction(
                  icon: Icons.arrow_back,
                  tooltip: copy.pickerBackToPick,
                  semanticsLabel: copy.pickerBackToPick,
                  onPressed: _backToPick,
                ),
              Expanded(
                child: Padding(
                  padding: EdgeInsets.only(left: isEdit ? 8 : 0),
                  child: _Eyebrow(
                    text: isEdit
                        ? copy.pickerEditEyebrow
                        : copy.pickerBackToPick,
                  ),
                ),
              ),
              IconAction(
                icon: Icons.close,
                tooltip: l10n.a11yClose,
                semanticsLabel: l10n.a11yClose,
                onPressed: () => Navigator.of(context).pop(),
              ),
            ],
          ),
        ),
        Flexible(
          child: SingleChildScrollView(
            padding: EdgeInsets.fromLTRB(20, 14, 20, 12 + keyboardInset),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              mainAxisSize: MainAxisSize.min,
              children: [
                _SelectedTopCard(
                  scopeKind: widget.scopeKind,
                  name: name,
                  slots: _slots,
                  noteController: _note,
                  onSlotsChanged: (v) => setState(() => _slots = v),
                ),
                const SizedBox(height: 12),
                _GearLinkDisclosure(
                  label: l10n.needsClaimLinkGear,
                  selectedGearId: _gearId,
                  onTap: _openGearLinkPicker,
                ),
                if (isEdit &&
                    widget.existingClaimsSummary != null &&
                    widget.existingClaimsSummary!.isNotEmpty) ...[
                  const SizedBox(height: 12),
                  _AlreadyClaimedChip(
                    eyebrow: copy.pickerEditAlreadyClaimed,
                    summary: widget.existingClaimsSummary!,
                  ),
                ],
              ],
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
          child: Row(
            children: [
              Expanded(
                flex: 1,
                child: isEdit
                    ? _DangerButton(
                        label: copy.pickerEditRemove,
                        onTap: _submitRemove,
                      )
                    : _SecondaryButton(
                        label: MaterialLocalizations.of(context)
                            .cancelButtonLabel,
                        onTap: () => Navigator.of(context).pop(),
                      ),
              ),
              const SizedBox(width: 10),
              Expanded(
                flex: 2,
                child: _PrimaryButton(
                  label: isEdit ? copy.pickerEditSave : copy.pickerAddToList,
                  enabled: name.isNotEmpty,
                  onTap: name.isNotEmpty ? _submitConfirm : null,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  // ── NK4 — redesigned edit-mode confirm view ─────────────────────────────
  //
  // Replaces the legacy single-card layout with three labeled sections
  // (TITLE / QUANTITY NEEDED / COMMENT) per `claim-sheet-prototype.html`.
  // The TITLE input is editable so the user can rename the need; the
  // QUANTITY stepper exposes "person / item" suffix copy; the COMMENT
  // textarea takes a richer placeholder ("Context for whoever picks
  // this up."). Bottom row keeps the existing Remove + Save split per
  // the user's preference, but Save adopts the sage "Save changes ✓"
  // treatment from the reference.
  Widget _buildEditConfirmBody(
    BuildContext context,
    AppLocalizations l10n,
    NeedsCopy copy,
    double keyboardInset,
  ) {
    final eyebrowText = copy.pickerEditEyebrow.toUpperCase();
    final nameText = _nameCtrl.text.trim();

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 8, 12, 0),
          child: Row(
            children: [
              Expanded(
                child: Text(
                  eyebrowText,
                  style: const TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1.6,
                    color: AppColors.modalTextSecondary,
                  ),
                ),
              ),
              IconAction(
                icon: Icons.close,
                tooltip: l10n.a11yClose,
                semanticsLabel: l10n.a11yClose,
                onPressed: () => Navigator.of(context).pop(),
              ),
            ],
          ),
        ),
        Flexible(
          child: SingleChildScrollView(
            padding: EdgeInsets.fromLTRB(20, 12, 20, 12 + keyboardInset),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              mainAxisSize: MainAxisSize.min,
              children: [
                NeedsEditFieldLabel(label: l10n.needsPickerEditTitleLabel),
                const SizedBox(height: 6),
                NeedsEditTitleInput(
                  controller: _nameCtrl,
                  onChanged: (_) => setState(() {}),
                ),
                const SizedBox(height: 16),
                NeedsEditFieldLabel(label: l10n.needsPickerEditQuantityLabel),
                const SizedBox(height: 6),
                NeedsEditQuantityStepper(
                  value: _slots,
                  suffix: l10n.needsPickerEditQuantitySuffix(_slots),
                  decrementLabel: l10n.a11yNeedsDecrement,
                  incrementLabel: l10n.a11yNeedsIncrement,
                  onChanged: (v) => setState(() => _slots = v),
                ),
                const SizedBox(height: 16),
                NeedsEditFieldLabel(label: l10n.needsPickerEditCommentLabel),
                const SizedBox(height: 6),
                NeedsEditCommentField(
                  controller: _note,
                  placeholder: l10n.needsPickerEditCommentHint,
                ),
                if (widget.existingClaimsSummary != null &&
                    widget.existingClaimsSummary!.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  _AlreadyClaimedChip(
                    eyebrow: copy.pickerEditAlreadyClaimed,
                    summary: widget.existingClaimsSummary!,
                  ),
                ],
              ],
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
          child: Row(
            children: [
              Expanded(
                flex: 1,
                child: _DangerButton(
                  label: copy.pickerEditRemove,
                  onTap: _submitRemove,
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                flex: 2,
                child: NeedsEditSaveChangesButton(
                  label: copy.pickerEditSave,
                  enabled: nameText.isNotEmpty,
                  onTap: nameText.isNotEmpty ? _submitConfirm : null,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

/// Inline disclosure row on the confirm view that opens the gear
/// picker. Shows the selected gear as an inline [GearPill] when one
/// is linked; otherwise prompts the user to link an item.
class _GearLinkDisclosure extends StatelessWidget {
  final String label;
  final String? selectedGearId;
  final VoidCallback onTap;

  const _GearLinkDisclosure({
    required this.label,
    required this.selectedGearId,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: GlassTokens.fillFaint,
          border: Border.all(color: AppColors.modalChipBorder),
          borderRadius: BorderRadius.circular(12),
        ),
        child: Row(
          children: [
            const Icon(
              Icons.inventory_2_outlined,
              size: 18,
              color: AppColors.modalTextMuted,
            ),
            const SizedBox(width: 10),
            Expanded(
              child: selectedGearId == null
                  ? Text(
                      label,
                      style: const TextStyle(
                        color: AppColors.modalTextPrimary,
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                      ),
                    )
                  : Align(
                      alignment: Alignment.centerLeft,
                      child: GearPill(gearId: selectedGearId!),
                    ),
            ),
            const Icon(
              Icons.chevron_right,
              size: 18,
              color: AppColors.modalTextMuted,
            ),
          ],
        ),
      ),
    );
  }
}

class _AlreadyClaimedChip extends StatelessWidget {
  final String eyebrow;
  final String summary;
  const _AlreadyClaimedChip({required this.eyebrow, required this.summary});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        border: Border.all(color: AppColors.modalChipBorder),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            eyebrow.toUpperCase(),
            style: const TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.8,
              color: AppColors.modalTextMuted,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            summary,
            style: const TextStyle(
              fontSize: 12,
              color: AppColors.modalTextSecondary,
            ),
          ),
        ],
      ),
    );
  }
}

class _DangerButton extends StatelessWidget {
  final String label;
  final VoidCallback onTap;
  const _DangerButton({required this.label, required this.onTap});

  @override
  Widget build(BuildContext context) {
    // Solid red fill with white foreground — the previous 10%-alpha
    // tint was unreadable against the frosted-glass backdrop.
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        height: 50,
        decoration: BoxDecoration(
          color: AppColors.statusErrorOnDark,
          borderRadius: BorderRadius.circular(14),
        ),
        alignment: Alignment.center,
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.close, size: 14, color: Colors.white),
            const SizedBox(width: 6),
            Text(
              label,
              style: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w700,
                color: Colors.white,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Atoms ───────────────────────────────────────────────────────────────────

class _Eyebrow extends StatelessWidget {
  final String text;
  const _Eyebrow({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text.toUpperCase(),
      style: const TextStyle(
        color: AppColors.modalTextPrimary,
        fontSize: 11,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.6,
      ),
    );
  }
}

class _Title extends StatelessWidget {
  final String text;
  const _Title({required this.text});

  @override
  Widget build(BuildContext context) {
    return Semantics(
      header: true,
      child: Text(
        text,
        style: const TextStyle(
          color: AppColors.modalTextPrimary,
          fontSize: 28,
          fontWeight: FontWeight.w800,
          height: 1.1,
          letterSpacing: -0.4,
        ),
      ),
    );
  }
}

class _Subtitle extends StatelessWidget {
  final String text;
  const _Subtitle({required this.text});

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        color: AppColors.modalTextSecondary,
        fontSize: 14,
        height: 1.4,
      ),
    );
  }
}

class _SectionLabel extends StatelessWidget {
  final String label;
  const _SectionLabel({required this.label});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Icon(Icons.auto_awesome,
            size: 11, color: AppColors.lightAccent),
        const SizedBox(width: 6),
        Flexible(
          child: Text(
            label.toUpperCase(),
            style: const TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.8,
              color: AppColors.modalTextMuted,
            ),
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }
}

class _SearchPill extends StatelessWidget {
  final TextEditingController controller;
  final String hint;
  final ValueChanged<String> onSubmitted;

  const _SearchPill({
    required this.controller,
    required this.hint,
    required this.onSubmitted,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 52,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: GlassTokens.fillSubtle,
        border: Border.all(color: AppColors.modalChipBorder),
        borderRadius: BorderRadius.circular(14),
      ),
      child: Row(
        children: [
          const Icon(Icons.search,
              size: 16, color: AppColors.modalTextMuted),
          const SizedBox(width: 10),
          Expanded(
            child: TextField(
              controller: controller,
              autofocus: false,
              cursorColor: AppColors.modalTextPrimary,
              style: const TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 14,
                height: 1.2,
              ),
              decoration: InputDecoration(
                hintText: hint,
                hintStyle: const TextStyle(
                  color: AppColors.modalTextMuted,
                  fontSize: 14,
                ),
                filled: false,
                border: InputBorder.none,
                enabledBorder: InputBorder.none,
                focusedBorder: InputBorder.none,
                isCollapsed: true,
                contentPadding: EdgeInsets.zero,
              ),
              textInputAction: TextInputAction.go,
              onSubmitted: onSubmitted,
            ),
          ),
        ],
      ),
    );
  }
}

/// NK1 suggestion grid — two-column rectangular tiles. Each tile is a
/// `Tappable` (button semantics) carrying the suggestion name; tap
/// transitions the picker to the NK3 confirm view. Suggestions whose
/// name appears in [takenNames] are filtered out entirely so the
/// requester only ever sees actionable tiles — taking a no-op tile out of
/// the grid is the v2 dedup behaviour (the prior "disabled tile"
/// treatment confused users into thinking the row was broken).
class _SuggestTileGrid extends StatelessWidget {
  final List<NeedsSuggestion> suggestions;
  final Set<String> takenNames;
  final String takenSubtitle;
  final ValueChanged<NeedsSuggestion> onSelect;

  const _SuggestTileGrid({
    required this.suggestions,
    required this.takenNames,
    required this.takenSubtitle,
    required this.onSelect,
  });

  @override
  Widget build(BuildContext context) {
    final takenLower = {
      for (final n in takenNames) n.trim().toLowerCase(),
    };
    final visible = [
      for (final s in suggestions)
        if (!takenLower.contains(s.name.trim().toLowerCase())) s,
    ];
    return LayoutBuilder(builder: (context, constraints) {
      const gap = 8.0;
      final tileWidth = (constraints.maxWidth - gap) / 2;
      return Wrap(
        spacing: gap,
        runSpacing: gap,
        children: [
          for (final s in visible)
            SizedBox(
              width: tileWidth,
              child: _SuggestTile(
                suggestion: s,
                taken: false,
                takenSubtitle: takenSubtitle,
                onTap: () => onSelect(s),
              ),
            ),
        ],
      );
    });
  }
}

class _SuggestTile extends StatelessWidget {
  final NeedsSuggestion suggestion;
  final bool taken;
  final String takenSubtitle;
  final VoidCallback onTap;

  const _SuggestTile({
    required this.suggestion,
    required this.taken,
    required this.takenSubtitle,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final accent = AppColors.experienceSageGreen;
    return Tappable(
      semanticsLabel: l10n.a11yNeedsPickerSuggestion(suggestion.name),
      onTap: taken ? null : onTap,
      child: Opacity(
        opacity: taken ? 0.75 : 1,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          decoration: BoxDecoration(
            color: taken
                ? accent.withValues(alpha: 0.08)
                : GlassTokens.fillFaint,
            border: Border.all(
              color: taken
                  ? accent.withValues(alpha: 0.45)
                  : AppColors.modalChipBorder,
            ),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      suggestion.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        color:
                            taken ? accent : AppColors.modalTextPrimary,
                        fontSize: 13.5,
                        fontWeight: FontWeight.w700,
                        height: 1.2,
                      ),
                    ),
                    if (taken) ...[
                      const SizedBox(height: 1),
                      Text(
                        takenSubtitle,
                        style: TextStyle(
                          color: accent,
                          fontSize: 11,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Icon(
                taken ? Icons.check : Icons.add,
                size: 14,
                color: taken ? accent : AppColors.modalTextPrimary,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// NK3 selected-top card: chosen name + qty stepper + note. Sits at the
/// top of the confirm view; the qty stepper is total-slots only in v1
/// (the total/each unit toggle is a deferred follow-up).
class _SelectedTopCard extends StatelessWidget {
  final NeedsScopeKind scopeKind;
  final String name;
  final int slots;
  final TextEditingController noteController;
  final ValueChanged<int> onSlotsChanged;

  const _SelectedTopCard({
    required this.scopeKind,
    required this.name,
    required this.slots,
    required this.noteController,
    required this.onSlotsChanged,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, scopeKind);
    return Container(
      padding: const EdgeInsets.fromLTRB(14, 14, 14, 12),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        border: Border.all(color: AppColors.modalChipBorder),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            name,
            style: const TextStyle(
              color: AppColors.modalTextPrimary,
              fontSize: 18,
              fontWeight: FontWeight.w800,
              height: 1.2,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              Text(
                copy.slotsLabel,
                style: const TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.8,
                  color: AppColors.modalTextMuted,
                ),
              ),
              const Spacer(),
              IconAction(
                icon: Icons.remove_circle_outline,
                semanticsLabel: l10n.a11yNeedsDecrement,
                onPressed: slots > 1 ? () => onSlotsChanged(slots - 1) : null,
                color: AppColors.modalTextPrimary,
                iconSize: 22,
                padding: EdgeInsets.zero,
                constraints:
                    const BoxConstraints(minWidth: 32, minHeight: 32),
              ),
              SizedBox(
                width: 32,
                child: Text(
                  '$slots',
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: AppColors.modalTextPrimary,
                  ),
                ),
              ),
              IconAction(
                icon: Icons.add_circle_outline,
                semanticsLabel: l10n.a11yNeedsIncrement,
                onPressed: slots < 20 ? () => onSlotsChanged(slots + 1) : null,
                color: AppColors.modalTextPrimary,
                iconSize: 22,
                padding: EdgeInsets.zero,
                constraints:
                    const BoxConstraints(minWidth: 32, minHeight: 32),
              ),
            ],
          ),
          const SizedBox(height: 10),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            decoration: BoxDecoration(
              color: GlassTokens.fillFaint,
              border: Border.all(color: AppColors.modalChipBorder),
              borderRadius: BorderRadius.circular(12),
            ),
            child: TextField(
              controller: noteController,
              minLines: 1,
              maxLines: 3,
              style: const TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 13.5,
                height: 1.4,
              ),
              decoration: InputDecoration(
                hintText: copy.noteLabel,
                hintStyle: const TextStyle(
                  color: AppColors.modalTextMuted,
                  fontSize: 13.5,
                ),
                filled: false,
                border: InputBorder.none,
                enabledBorder: InputBorder.none,
                focusedBorder: InputBorder.none,
                isCollapsed: true,
                contentPadding: EdgeInsets.zero,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _PrimaryButton extends StatelessWidget {
  final String label;
  final bool enabled;
  final VoidCallback? onTap;

  const _PrimaryButton({
    required this.label,
    required this.enabled,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        height: 50,
        decoration: BoxDecoration(
          color: enabled ? accent : accent.withValues(alpha: 0.30),
          borderRadius: BorderRadius.circular(14),
          boxShadow: enabled
              ? [
                  BoxShadow(
                    color: accent.withValues(alpha: 0.30),
                    offset: const Offset(0, 8),
                    blurRadius: 20,
                  ),
                ]
              : null,
        ),
        alignment: Alignment.center,
        child: Text(
          label,
          style: TextStyle(
            color: enabled
                ? AppColors.cardBackground(context)
                : AppColors.modalTextMuted,
            fontWeight: FontWeight.w700,
            fontSize: 14.5,
          ),
        ),
      ),
    );
  }
}

class _SecondaryButton extends StatelessWidget {
  final String label;
  final VoidCallback onTap;

  const _SecondaryButton({required this.label, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        height: 50,
        decoration: BoxDecoration(
          color: GlassTokens.fillSubtle,
          border: Border.all(color: AppColors.modalChipBorder),
          borderRadius: BorderRadius.circular(14),
        ),
        alignment: Alignment.center,
        child: Text(
          label,
          style: const TextStyle(
            color: AppColors.modalTextPrimary,
            fontSize: 14,
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
    );
  }
}
