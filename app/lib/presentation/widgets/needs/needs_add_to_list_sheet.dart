import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_gear_picker.dart';
import 'package:ripls/presentation/widgets/needs/needs_copy.dart';

/// Result returned from [NeedsAddToListSheet] when the requester confirms.
///
/// [preClaim] is set when the requester flipped the "I'll bring this myself"
/// toggle. The caller is responsible for chaining `addNeed` + `claimNeed`
/// in that order (passing [gearId] to the claim call). The sheet itself
/// never dispatches mutations.
class NeedsAddToListResult {
  final String name;
  final String? note;
  final bool preClaim;
  final String? gearId;

  const NeedsAddToListResult({
    required this.name,
    this.note,
    this.preClaim = false,
    this.gearId,
  });
}

/// "Add to the list" sheet — single-screen variant of the legacy
/// NK1+NK3 picker flow. Combines the name input, suggestion chips, a
/// note field, and an optional pre-claim toggle into one editorial
/// surface that publishes a Need in one tap.
///
/// Wired from the Volunteer sheet's "Add something" affordance for
/// both Experience and Request scopes; [scopeKind] drives the few
/// strings whose tone changes between the collaborative-event and
/// single-help-request framings via [NeedsCopy].
class NeedsAddToListSheet extends ConsumerStatefulWidget {
  /// Scope flavour — picks the Experience vs Request copy variants
  /// via [NeedsCopy].
  final NeedsScopeKind scopeKind;

  /// Suggestion labels rendered as compact chips below the name input.
  /// Caller filters out anything already on the list; the sheet
  /// defensively re-filters in case state moves while the modal is
  /// open. Capped at six visible chips per the reference design.
  final List<String> suggestions;

  /// Lowercased names of needs already on the list — used to gate
  /// chips that would create a duplicate. Defensive only; the caller
  /// is expected to pre-filter [suggestions].
  final Set<String> takenNames;

  /// Community context forwarded to [NeedsClaimGearPicker] so the
  /// suggested gear section intersects with community-scoped search.
  /// Null/empty falls back to a library-only picker.
  final String? communityId;

  const NeedsAddToListSheet({
    super.key,
    required this.scopeKind,
    this.suggestions = const [],
    this.takenNames = const {},
    this.communityId,
  });

  static Future<NeedsAddToListResult?> show(
    BuildContext context, {
    required NeedsScopeKind scopeKind,
    List<String> suggestions = const [],
    Set<String> takenNames = const {},
    String? communityId,
  }) {
    return showAccessibleModal<NeedsAddToListResult>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => NeedsAddToListSheet(
        scopeKind: scopeKind,
        suggestions: suggestions,
        takenNames: takenNames,
        communityId: communityId,
      ),
    );
  }

  @override
  ConsumerState<NeedsAddToListSheet> createState() =>
      _NeedsAddToListSheetState();
}

class _NeedsAddToListSheetState extends ConsumerState<NeedsAddToListSheet> {
  final _nameCtrl = TextEditingController();
  final _noteCtrl = TextEditingController();
  bool _preClaim = false;
  String? _selectedGearId;
  String? _selectedGearName;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _noteCtrl.dispose();
    super.dispose();
  }

  bool get _canSubmit => _nameCtrl.text.trim().isNotEmpty;

  void _selectChip(String label) {
    setState(() {
      _nameCtrl.text = label;
      _nameCtrl.selection =
          TextSelection.collapsed(offset: _nameCtrl.text.length);
    });
  }

  void _submit() {
    final name = _nameCtrl.text.trim();
    if (name.isEmpty) return;
    final note = _noteCtrl.text.trim();
    Navigator.of(context).pop(NeedsAddToListResult(
      name: name,
      note: note.isEmpty ? null : note,
      preClaim: _preClaim,
      gearId: _preClaim ? _selectedGearId : null,
    ));
  }

  void _toggleMyself() {
    setState(() {
      _preClaim = !_preClaim;
      // Drop any cached gear link when the user disables the toggle so
      // it doesn't silently re-attach on re-enable.
      if (!_preClaim) {
        _selectedGearId = null;
        _selectedGearName = null;
      }
    });
  }

  Future<void> _openGearPicker() async {
    final result = await NeedsClaimGearPicker.show(
      context,
      communityId: widget.communityId,
      initialQuery: _nameCtrl.text.trim().isEmpty
          ? null
          : _nameCtrl.text.trim(),
      initialGearId: _selectedGearId,
    );
    if (!mounted || result == null) return;
    setState(() {
      _selectedGearId = result.gearId;
      _selectedGearName = result.gearId == null ? null : result.gear?.name;
    });
  }

  /// "From your gear" entry point on the What's-needed row. Opens the gear
  /// picker — which lets the contributor pick a library item *or* snap/create
  /// one from the camera (the unified create flow) — then fills the item name,
  /// links the gear, and flips on "I'll bring this myself" so the new gear is
  /// pre-claimed.
  Future<void> _pickFromGearAndFill() async {
    final result = await NeedsClaimGearPicker.show(
      context,
      communityId: widget.communityId,
      initialQuery:
          _nameCtrl.text.trim().isEmpty ? null : _nameCtrl.text.trim(),
      initialGearId: _selectedGearId,
    );
    if (!mounted || result?.gearId == null) return;
    setState(() {
      _selectedGearId = result!.gearId;
      _selectedGearName = result.gear?.name;
      final gearName = result.gear?.name.trim() ?? '';
      if (_nameCtrl.text.trim().isEmpty && gearName.isNotEmpty) {
        _nameCtrl.text = gearName;
        _nameCtrl.selection =
            TextSelection.collapsed(offset: _nameCtrl.text.length);
      }
      // Linking a gear item means the viewer is bringing it.
      _preClaim = true;
    });
  }

  /// Clears the pre-claim's linked gear directly from the tile —
  /// short-circuits the picker round-trip so unlinking is a one-tap
  /// action.
  void _unlinkGear() {
    setState(() {
      _selectedGearId = null;
      _selectedGearName = null;
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, widget.scopeKind);
    final takenLower = {
      for (final n in widget.takenNames) n.trim().toLowerCase(),
    };
    final visibleChips = [
      for (final s in widget.suggestions)
        if (!takenLower.contains(s.trim().toLowerCase())) s,
    ].take(6).toList();

    return GlassSheet(
      padding: EdgeInsets.zero,
      showDragHandle: true,
      child: SafeArea(
        top: false,
        child: Padding(
          padding: EdgeInsets.fromLTRB(
            20,
            8,
            20,
            24 + MediaQuery.of(context).viewInsets.bottom,
          ),
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _Eyebrow(
                  text: l10n.needsAddToListEyebrow.toUpperCase(),
                  closeLabel: l10n.a11yNeedsAddToListClose,
                  onClose: () => Navigator.of(context).pop(),
                ),
                const SizedBox(height: 16),
                Row(
                  children: [
                    Expanded(
                      child: _SectionLabel(
                        label: l10n.needsAddToListWhatsNeededLabel,
                      ),
                    ),
                    _FromGearAffordance(
                      label: l10n.rsvpComposerFromGear,
                      semanticsLabel: l10n.a11yRsvpComposerFromGear,
                      onTap: _pickFromGearAndFill,
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                _NameInput(
                  controller: _nameCtrl,
                  hint: copy.addToListInputHint,
                  onChanged: (_) => setState(() {}),
                ),
                const SizedBox(height: 10),
                Text(
                  copy.addToListHelperText,
                  style: const TextStyle(
                    fontSize: 13,
                    color: AppColors.modalTextSecondary,
                    height: 1.4,
                  ),
                ),
                if (visibleChips.isNotEmpty) ...[
                  const SizedBox(height: 18),
                  _SectionLabel(
                    label: l10n.needsAddToListSuggestionsLabel,
                    leading: Icons.auto_awesome,
                  ),
                  const SizedBox(height: 10),
                  _ChipsGrid(
                    chips: visibleChips,
                    onSelect: _selectChip,
                    chipSemanticsBuilder: l10n.a11yNeedsAddToListChip,
                  ),
                ],
                const SizedBox(height: 18),
                _SectionLabel(label: l10n.needsAddToListNoteLabel),
                const SizedBox(height: 8),
                _NoteInput(
                  controller: _noteCtrl,
                  hint: copy.addToListNoteHint,
                ),
                const SizedBox(height: 18),
                _MyselfToggleRow(
                  selected: _preClaim,
                  title: l10n.needsAddToListMyselfTitle,
                  subtitle: l10n.needsAddToListMyselfSubtitle,
                  semanticsLabel: l10n.a11yNeedsAddToListMyselfToggle,
                  onTap: _toggleMyself,
                ),
                if (_preClaim) ...[
                  const SizedBox(height: 10),
                  _LinkGearTile(
                    isLinked: _selectedGearId != null,
                    linkedName: _selectedGearName,
                    onTap: _openGearPicker,
                    unlinkSemanticsLabel:
                        l10n.a11yNeedsClaimSheetUnlinkGear,
                    onUnlink: _unlinkGear,
                  ),
                ],
                const SizedBox(height: 20),
                _PrimaryCta(
                  label: l10n.needsAddToListCta,
                  enabled: _canSubmit,
                  onTap: _canSubmit ? _submit : null,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _Eyebrow extends StatelessWidget {
  const _Eyebrow({
    required this.text,
    required this.closeLabel,
    required this.onClose,
  });
  final String text;
  final String closeLabel;
  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Expanded(
          child: Text(
            text,
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
          semanticsLabel: closeLabel,
          color: AppColors.modalTextPrimary,
          onPressed: onClose,
        ),
      ],
    );
  }
}

class _SectionLabel extends StatelessWidget {
  const _SectionLabel({required this.label, this.leading});
  final String label;
  final IconData? leading;

  @override
  Widget build(BuildContext context) {
    const style = TextStyle(
      fontSize: 10.5,
      fontWeight: FontWeight.w700,
      letterSpacing: 1.4,
      color: GlassTokens.textMuted,
    );
    if (leading == null) {
      return Text(label.toUpperCase(), style: style);
    }
    return Row(
      children: [
        Icon(
          leading,
          size: 12,
          color: AppColors.experienceSageGreen,
        ),
        const SizedBox(width: 6),
        Text(label.toUpperCase(), style: style),
      ],
    );
  }
}

/// Trailing "From your gear" affordance on the What's-needed row. Opens the
/// gear picker (library pick *or* snap/create from camera).
class _FromGearAffordance extends StatelessWidget {
  const _FromGearAffordance({
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
  });
  final String label;
  final String semanticsLabel;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(999),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 2),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.add_a_photo_outlined,
              size: 13,
              color: AppColors.experienceSageGreen,
            ),
            const SizedBox(width: 4),
            Text(
              label.toUpperCase(),
              style: const TextStyle(
                color: AppColors.experienceSageGreen,
                fontSize: 10,
                fontWeight: FontWeight.w700,
                letterSpacing: 1,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _NameInput extends StatelessWidget {
  const _NameInput({
    required this.controller,
    required this.hint,
    required this.onChanged,
  });
  final TextEditingController controller;
  final String hint;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        border: Border.all(color: GlassTokens.borderSoft),
        borderRadius: BorderRadius.circular(14),
      ),
      child: TextField(
        controller: controller,
        onChanged: onChanged,
        textCapitalization: TextCapitalization.sentences,
        cursorColor: AppColors.experienceSageGreen,
        style: const TextStyle(
          color: AppColors.modalTextPrimary,
          fontSize: 15,
          height: 1.3,
        ),
        decoration: InputDecoration(
          hintText: hint,
          hintStyle: const TextStyle(
            color: GlassTokens.textFaint,
            fontSize: 15,
            height: 1.3,
          ),
          // Explicit transparent fill so Material's default
          // InputDecorationTheme can't overlay a white fill on focus.
          filled: true,
          fillColor: Colors.transparent,
          hoverColor: Colors.transparent,
          border: InputBorder.none,
          enabledBorder: InputBorder.none,
          focusedBorder: InputBorder.none,
          disabledBorder: InputBorder.none,
          isCollapsed: true,
          contentPadding: EdgeInsets.zero,
        ),
      ),
    );
  }
}

class _NoteInput extends StatelessWidget {
  const _NoteInput({required this.controller, required this.hint});
  final TextEditingController controller;
  final String hint;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        border: Border.all(color: GlassTokens.borderSoft),
        borderRadius: BorderRadius.circular(14),
      ),
      child: TextField(
        controller: controller,
        minLines: 2,
        maxLines: 4,
        textCapitalization: TextCapitalization.sentences,
        cursorColor: AppColors.experienceSageGreen,
        style: const TextStyle(
          color: AppColors.modalTextPrimary,
          fontSize: 14,
          height: 1.45,
        ),
        decoration: InputDecoration(
          hintText: hint,
          hintStyle: const TextStyle(
            color: GlassTokens.textFaint,
            fontSize: 14,
            height: 1.45,
            fontStyle: FontStyle.italic,
          ),
          filled: true,
          fillColor: Colors.transparent,
          hoverColor: Colors.transparent,
          border: InputBorder.none,
          enabledBorder: InputBorder.none,
          focusedBorder: InputBorder.none,
          disabledBorder: InputBorder.none,
          isCollapsed: true,
          contentPadding: EdgeInsets.zero,
        ),
      ),
    );
  }
}

class _ChipsGrid extends StatelessWidget {
  const _ChipsGrid({
    required this.chips,
    required this.onSelect,
    required this.chipSemanticsBuilder,
  });
  final List<String> chips;
  final ValueChanged<String> onSelect;
  final String Function(String label) chipSemanticsBuilder;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(builder: (context, constraints) {
      const gap = 8.0;
      final tileWidth = (constraints.maxWidth - gap) / 2;
      return Wrap(
        spacing: gap,
        runSpacing: gap,
        children: [
          for (final label in chips)
            SizedBox(
              width: tileWidth,
              child: _Chip(
                label: label,
                semanticsLabel: chipSemanticsBuilder(label),
                onTap: () => onSelect(label),
              ),
            ),
        ],
      );
    });
  }
}

class _Chip extends StatelessWidget {
  const _Chip({
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
  });
  final String label;
  final String semanticsLabel;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(999),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: GlassTokens.fillFaint,
          border: Border.all(color: GlassTokens.borderSoft),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 22,
              height: 22,
              decoration: const BoxDecoration(
                color: GlassTokens.fillSubtle,
                shape: BoxShape.circle,
              ),
              alignment: Alignment.center,
              child: const Icon(
                Icons.add,
                size: 14,
                color: GlassTokens.textSecondary,
              ),
            ),
            const SizedBox(width: 8),
            Flexible(
              child: Text(
                label,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 13.5,
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _MyselfToggleRow extends StatelessWidget {
  const _MyselfToggleRow({
    required this.selected,
    required this.title,
    required this.subtitle,
    required this.semanticsLabel,
    required this.onTap,
  });
  final bool selected;
  final String title;
  final String subtitle;
  final String semanticsLabel;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.experienceSageGreen;
    return Toggle(
      semanticsLabel: semanticsLabel,
      selected: selected,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        padding: const EdgeInsets.fromLTRB(12, 12, 16, 12),
        decoration: BoxDecoration(
          color: selected
              ? accent.withValues(alpha: 0.10)
              : GlassTokens.fillFaint,
          border: Border.all(
            color: selected
                ? accent.withValues(alpha: 0.45)
                : GlassTokens.borderSoft,
          ),
          borderRadius: BorderRadius.circular(14),
        ),
        child: Row(
          children: [
            _SwitchVisual(selected: selected, accent: accent),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    title,
                    style: const TextStyle(
                      fontSize: 14.5,
                      fontWeight: FontWeight.w700,
                      color: AppColors.modalTextPrimary,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    subtitle,
                    style: const TextStyle(
                      fontSize: 12,
                      color: GlassTokens.textFaint,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _SwitchVisual extends StatelessWidget {
  const _SwitchVisual({required this.selected, required this.accent});
  final bool selected;
  final Color accent;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: accessibleDuration(context, const Duration(milliseconds: 200)),
      width: 38,
      height: 22,
      padding: const EdgeInsets.all(2),
      decoration: BoxDecoration(
        color: selected ? accent : GlassTokens.borderSoft,
        borderRadius: BorderRadius.circular(999),
      ),
      child: AnimatedAlign(
        duration: accessibleDuration(context, const Duration(milliseconds: 220)),
        curve: Curves.easeOut,
        alignment: selected ? Alignment.centerRight : Alignment.centerLeft,
        child: Container(
          width: 18,
          height: 18,
          decoration: const BoxDecoration(
            color: AppColors.modalTextPrimary,
            shape: BoxShape.circle,
          ),
        ),
      ),
    );
  }
}

class _LinkGearTile extends StatelessWidget {
  const _LinkGearTile({
    required this.isLinked,
    required this.linkedName,
    required this.onTap,
    required this.unlinkSemanticsLabel,
    required this.onUnlink,
  });
  final bool isLinked;
  final String? linkedName;
  final VoidCallback onTap;
  final String unlinkSemanticsLabel;

  /// Clears the linked gear directly from the tile when the user taps
  /// the trailing ×. Only rendered when [isLinked] is true.
  final VoidCallback onUnlink;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.experienceSageGreen;
    return Tappable(
      semanticsLabel: linkedName ?? 'Link gear',
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: isLinked
              ? accent.withValues(alpha: 0.10)
              : GlassTokens.fillFaint,
          border: Border.all(
            color: isLinked
                ? accent.withValues(alpha: 0.45)
                : GlassTokens.borderSoft,
          ),
          borderRadius: BorderRadius.circular(14),
        ),
        child: Row(
          children: [
            const Icon(
              Icons.chevron_right,
              size: 18,
              color: GlassTokens.textMuted,
            ),
            const SizedBox(width: 6),
            Expanded(
              child: Text(
                isLinked && linkedName != null && linkedName!.isNotEmpty
                    ? linkedName!
                    : 'Link gear',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ),
            if (isLinked) ...[
              const SizedBox(width: 4),
              IconAction(
                icon: Icons.close,
                semanticsLabel: unlinkSemanticsLabel,
                color: AppColors.modalTextPrimary,
                iconSize: 16,
                padding: const EdgeInsets.all(4),
                constraints: const BoxConstraints(
                  minWidth: 28,
                  minHeight: 28,
                ),
                onPressed: onUnlink,
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _PrimaryCta extends StatelessWidget {
  const _PrimaryCta({
    required this.label,
    required this.enabled,
    required this.onTap,
  });
  final String label;
  final bool enabled;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.experienceSageGreen;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(16),
      child: Container(
        height: 52,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: enabled ? accent : accent.withValues(alpha: 0.30),
          borderRadius: BorderRadius.circular(16),
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
        child: Text(
          label,
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.1,
            color: enabled
                ? const Color(0xFF0F1A14)
                : AppColors.modalTextMuted,
          ),
        ),
      ),
    );
  }
}
