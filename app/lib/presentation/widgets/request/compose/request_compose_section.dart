// Compose section: the chip strip, paste-list disclosure, pieces list
// with modal-driven rename + pre-claim, and inline-add input. Pure UI
// over [requestComposeProvider] — the caller passes in the chip
// [suggestions] (post-publish: chips ride on
// [requestNeedsProvider]; pre-publish: chips ride on
// [genRequestProvider]).
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/viewmodels/request_compose_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_edit_atoms.dart';

class RequestComposeSection extends ConsumerStatefulWidget {
  /// AI-suggested chip labels (1–4 words each). Empty when the AI
  /// returned nothing — the section then renders without a chip strip
  /// and the user can still type/paste pieces.
  final List<String> suggestions;

  /// When true, the section paints text/borders for a translucent
  /// modal surface (frosted glass / dark gradient) using
  /// [AppColors.modalTextPrimary] / `modalTextSecondary`. Defaults
  /// false for ordinary themed surfaces.
  final bool onGlass;

  /// True when the section plans an event's needs (the experience compose
  /// sheet) rather than a request breakdown. Swaps the suggestion-chip
  /// eyebrow to the event wording — "From your request" is request-flow
  /// copy and leaked onto the event surface (#2724).
  final bool forEvent;

  const RequestComposeSection({
    super.key,
    this.suggestions = const [],
    this.onGlass = false,
    this.forEvent = false,
  });

  @override
  ConsumerState<RequestComposeSection> createState() =>
      _RequestComposeSectionState();
}

class _RequestComposeSectionState extends ConsumerState<RequestComposeSection> {
  final _inlineCtrl = TextEditingController();
  final _pasteCtrl = TextEditingController();
  bool _pasteOpen = false;

  @override
  void dispose() {
    _inlineCtrl.dispose();
    _pasteCtrl.dispose();
    super.dispose();
  }

  void _commitInline() {
    final text = _inlineCtrl.text;
    if (text.trim().isEmpty) return;
    ref.read(requestComposeProvider.notifier).addInlinePiece(text);
    _inlineCtrl.clear();
  }

  void _commitPaste() {
    final text = _pasteCtrl.text;
    if (text.trim().isEmpty) return;
    ref.read(requestComposeProvider.notifier).addPasteList(text);
    _pasteCtrl.clear();
    setState(() => _pasteOpen = false);
  }

  /// Opens the piece-edit modal — same shape as the post-publish
  /// NK4 needs-edit sheet, scoped down to the one field that exists
  /// on a draft piece (its label). Inline rename was removed so all
  /// edit affordances go through the modal regardless of whether the
  /// list has been published yet.
  Future<void> _openEditModal(ComposePiece p) async {
    final newName = await _ComposePieceEditSheet.show(
      context,
      initialLabel: p.label,
    );
    if (newName == null || !mounted) return;
    ref.read(requestComposeProvider.notifier).renamePiece(p.id, newName);
  }

  @override
  Widget build(BuildContext context) {
    final compose = ref.watch(requestComposeProvider);
    final l10n = context.l10n;
    final onGlass = widget.onGlass;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // The sheet chrome already carries the modal title; the
        // section opens directly with the framing subhead so the
        // requester isn't reading two titles back-to-back.
        Text(l10n.composeSectionSubhead,
            style: _subheadStyle(context, onGlass: onGlass)),
        if (widget.suggestions.isNotEmpty) ...[
          const SizedBox(height: 18),
          _SectionEyebrow(
            label: widget.forEvent
                ? l10n.composeSectionEyebrowExperience
                : l10n.composeSectionEyebrow,
            semanticsLabel: widget.forEvent
                ? l10n.a11yComposeSectionEyebrowExperience
                : l10n.a11yComposeSectionEyebrow,
            onGlass: onGlass,
          ),
          const SizedBox(height: 10),
          _SuggestionChips(
            suggestions: widget.suggestions,
            compose: compose,
            onGlass: onGlass,
          ),
          const SizedBox(height: 8),
        ] else
          const SizedBox(height: 16),
        _PasteDisclosure(
          open: _pasteOpen,
          onToggle: () => setState(() => _pasteOpen = !_pasteOpen),
          controller: _pasteCtrl,
          onAdd: _commitPaste,
          onGlass: onGlass,
        ),
        if (compose.pieces.isNotEmpty) ...[
          const SizedBox(height: 18),
          _PiecesEyebrow(
            label: l10n.composePiecesHeader(
              compose.pieces.length,
              compose.preclaimedCount,
            ),
            onGlass: onGlass,
          ),
          const SizedBox(height: 10),
          for (final p in compose.pieces)
            _PieceRow(
              key: ValueKey(p.id),
              piece: p,
              onOpenEditModal: () => _openEditModal(p),
              onGlass: onGlass,
              onTogglePreclaim: () => ref
                  .read(requestComposeProvider.notifier)
                  .togglePreclaim(p.id),
              onRemove: () => ref
                  .read(requestComposeProvider.notifier)
                  .removePiece(p.id),
            ),
        ],
        const SizedBox(height: 12),
        _InlineAdd(
          controller: _inlineCtrl,
          hint: compose.pieces.isEmpty
              ? l10n.composeInlineAddFirstHint
              : l10n.composeInlineAddAnotherHint,
          onSubmit: _commitInline,
          onGlass: onGlass,
        ),
      ],
    );
  }
}

Color _textPrimary(BuildContext context, {required bool onGlass}) =>
    onGlass ? AppColors.modalTextPrimary : AppColors.textPrimary(context);

Color _textSecondary(BuildContext context, {required bool onGlass}) => onGlass
    ? AppColors.modalTextPrimary.withValues(alpha: 0.72)
    : AppColors.textSecondary(context);

Color _surface(BuildContext context, {required bool onGlass}) => onGlass
    ? AppColors.modalTextPrimary.withValues(alpha: 0.06)
    : AppColors.surface(context);

Color _border(BuildContext context, {required bool onGlass}) => onGlass
    ? AppColors.modalTextPrimary.withValues(alpha: 0.18)
    : AppColors.border(context);

TextStyle _subheadStyle(BuildContext context, {required bool onGlass}) =>
    TextStyle(
      fontSize: 14,
      color: _textSecondary(context, onGlass: onGlass),
      height: 1.45,
    );

TextStyle _eyebrowStyle(BuildContext context, {required bool onGlass}) =>
    TextStyle(
      fontSize: 11,
      fontWeight: FontWeight.w700,
      letterSpacing: 1.4,
      color: _textSecondary(context, onGlass: onGlass),
    );

/// Small uppercase eyebrow with a leading sparkle icon — used above the
/// AI suggestion chip strip to telegraph "these came from your request."
class _SectionEyebrow extends StatelessWidget {
  const _SectionEyebrow({
    required this.label,
    required this.semanticsLabel,
    required this.onGlass,
  });
  final String label;
  final String semanticsLabel;
  final bool onGlass;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: semanticsLabel,
      container: true,
      child: ExcludeSemantics(
        child: Row(
          children: [
            Icon(
              Icons.auto_awesome,
              size: 14,
              color: AppColors.lightAccent,
            ),
            const SizedBox(width: 8),
            Flexible(
              child: Text(
                label.toUpperCase(),
                style: _eyebrowStyle(context, onGlass: onGlass),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Uppercase pieces-header eyebrow with a leading pulse dot. Mirrors
/// the section eyebrow above so the two divider points read as one
/// design system.
class _PiecesEyebrow extends StatelessWidget {
  const _PiecesEyebrow({required this.label, required this.onGlass});
  final String label;
  final bool onGlass;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(
            color: AppColors.lightAccent,
            shape: BoxShape.circle,
            boxShadow: [
              BoxShadow(
                color: AppColors.lightAccent.withValues(alpha: 0.35),
                blurRadius: 6,
                spreadRadius: 2,
              ),
            ],
          ),
        ),
        const SizedBox(width: 10),
        Flexible(
          child: Text(
            label.toUpperCase(),
            style: _eyebrowStyle(context, onGlass: onGlass),
          ),
        ),
      ],
    );
  }
}

class _SuggestionChips extends ConsumerWidget {
  const _SuggestionChips({
    required this.suggestions,
    required this.compose,
    required this.onGlass,
  });
  final List<String> suggestions;
  final RequestComposeState compose;
  final bool onGlass;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final committedLabels = {
      for (final p in compose.pieces)
        if (p.fromSuggestionLabel != null) p.fromSuggestionLabel!,
    };
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (final label in suggestions)
          _SuggestionChip(
            label: label,
            added: committedLabels.contains(label),
            onTap: () => ref
                .read(requestComposeProvider.notifier)
                .toggleSuggestion(label),
            semanticsLabel: l10n.a11yComposeChip(label),
            onGlass: onGlass,
          ),
      ],
    );
  }
}

class _SuggestionChip extends StatelessWidget {
  const _SuggestionChip({
    required this.label,
    required this.added,
    required this.onTap,
    required this.semanticsLabel,
    required this.onGlass,
  });
  final String label;
  final bool added;
  final VoidCallback onTap;
  final String semanticsLabel;
  final bool onGlass;

  @override
  Widget build(BuildContext context) {
    return Toggle(
      semanticsLabel: semanticsLabel,
      selected: added,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(999),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 8),
        decoration: BoxDecoration(
          color: added
              ? AppColors.accent(context).withValues(alpha: 0.18)
              : _surface(context, onGlass: onGlass),
          borderRadius: BorderRadius.circular(999),
          border: Border.all(
            color: added
                ? AppColors.accent(context)
                : _border(context, onGlass: onGlass),
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              added ? Icons.check : Icons.add,
              size: 14,
              color: _textPrimary(context, onGlass: onGlass),
            ),
            const SizedBox(width: 6),
            Text(
              label,
              style: TextStyle(
                fontSize: 13.5,
                color: _textPrimary(context, onGlass: onGlass),
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _PasteDisclosure extends StatelessWidget {
  const _PasteDisclosure({
    required this.open,
    required this.onToggle,
    required this.controller,
    required this.onAdd,
    required this.onGlass,
  });
  final bool open;
  final VoidCallback onToggle;
  final TextEditingController controller;
  final VoidCallback onAdd;
  final bool onGlass;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Tappable(
          semanticsLabel: open
              ? l10n.a11yComposePasteCollapse
              : l10n.a11yComposePasteExpand,
          onTap: onToggle,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 6),
            // Muted "Not quite right?" prefix + underlined "Paste a
            // list" link + chevron — matches the reference's subtle
            // power-user affordance below the main chip strip.
            child: Row(
              children: [
                Text(
                  l10n.composePastePromptPrefix,
                  style: TextStyle(
                    fontSize: 13,
                    color: _textSecondary(context, onGlass: onGlass),
                  ),
                ),
                const SizedBox(width: 6),
                Text(
                  l10n.composePasteTrigger,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: AppColors.lightAccent,
                    decoration: TextDecoration.underline,
                    decorationColor:
                        AppColors.lightAccent.withValues(alpha: 0.55),
                    decorationThickness: 1.2,
                  ),
                ),
                const SizedBox(width: 2),
                Icon(
                  open ? Icons.expand_more : Icons.chevron_right,
                  size: 14,
                  color: AppColors.lightAccent,
                ),
              ],
            ),
          ),
        ),
        if (open) ...[
          const SizedBox(height: 6),
          TextField(
            controller: controller,
            maxLines: 4,
            minLines: 2,
            style: TextStyle(color: _textPrimary(context, onGlass: onGlass)),
            decoration: InputDecoration(
              hintText: context.l10n.composePastePlaceholder,
              hintStyle:
                  TextStyle(color: _textSecondary(context, onGlass: onGlass)),
              filled: true,
              fillColor: _surface(context, onGlass: onGlass),
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
                borderSide:
                    BorderSide(color: _border(context, onGlass: onGlass)),
              ),
              enabledBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
                borderSide:
                    BorderSide(color: _border(context, onGlass: onGlass)),
              ),
            ),
          ),
          const SizedBox(height: 8),
          Align(
            alignment: Alignment.centerRight,
            child: Tappable(
              semanticsLabel: context.l10n.a11yComposePasteAdd,
              onTap: onAdd,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                decoration: BoxDecoration(
                  color: AppColors.accent(context).withValues(alpha: 0.22),
                  borderRadius: BorderRadius.circular(10),
                  border: Border.all(
                    color: AppColors.accent(context).withValues(alpha: 0.45),
                  ),
                ),
                child: Text(
                  context.l10n.composePasteAdd,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: _textPrimary(context, onGlass: onGlass),
                  ),
                ),
              ),
            ),
          ),
        ],
      ],
    );
  }
}

class _PieceRow extends StatelessWidget {
  const _PieceRow({
    super.key,
    required this.piece,
    required this.onOpenEditModal,
    required this.onTogglePreclaim,
    required this.onRemove,
    required this.onGlass,
  });

  final ComposePiece piece;

  /// Opens the piece-edit modal — replaces the previous inline rename
  /// flow so all edits go through the same modal-driven surface as
  /// post-publish needs.
  final VoidCallback onOpenEditModal;
  final VoidCallback onTogglePreclaim;
  final VoidCallback onRemove;
  final bool onGlass;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: piece.preclaimed
            ? AppColors.accent(context).withValues(alpha: 0.10)
            : _surface(context, onGlass: onGlass),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: piece.preclaimed
              ? AppColors.accent(context).withValues(alpha: 0.40)
              : _border(context, onGlass: onGlass),
        ),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          // Leading badge mirrors the prototype's small sage-tinted
          // square at the start of every piece row. Generic + glyph
          // (no kind-specific icon — kind tags are out of scope).
          Container(
            width: 28,
            height: 28,
            margin: const EdgeInsets.only(right: 12),
            decoration: BoxDecoration(
              color: AppColors.lightAccent.withValues(alpha: 0.18),
              borderRadius: BorderRadius.circular(8),
            ),
            alignment: Alignment.center,
            child: Icon(
              Icons.add,
              size: 14,
              color: AppColors.lightAccent,
            ),
          ),
          Expanded(
            child: Tappable(
              semanticsLabel: l10n.a11yComposePieceRename(piece.label),
              onTap: onOpenEditModal,
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Text(
                  piece.label,
                  style: TextStyle(
                    fontSize: 14.5,
                    fontWeight: FontWeight.w500,
                    color: _textPrimary(context, onGlass: onGlass),
                  ),
                ),
              ),
            ),
          ),
          Toggle(
            semanticsLabel: l10n.a11yComposePreclaimToggle(piece.label),
            selected: piece.preclaimed,
            onTap: onTogglePreclaim,
            inkBorderRadius: BorderRadius.circular(999),
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
              decoration: BoxDecoration(
                color: piece.preclaimed
                    ? AppColors.accent(context).withValues(alpha: 0.20)
                    : Colors.transparent,
                borderRadius: BorderRadius.circular(999),
                border: Border.all(
                  color: AppColors.accent(context).withValues(alpha: 0.55),
                ),
              ),
              child: Text(
                piece.preclaimed
                    ? l10n.composeIveGotThisShort
                    : l10n.composeIveGotThis,
                style: TextStyle(
                  fontSize: 11.5,
                  fontWeight: FontWeight.w700,
                  color: _textPrimary(context, onGlass: onGlass),
                ),
              ),
            ),
          ),
          // Tap target sized to Material's 44px minimum so the remove
          // control reliably absorbs the touch on small phones. A
          // 24px-bounded IconAction was eating taps.
          IconAction(
            icon: Icons.close,
            semanticsLabel: l10n.a11yComposeRemovePiece(piece.label),
            onPressed: onRemove,
            iconSize: 18,
            color: _textSecondary(context, onGlass: onGlass),
          ),
        ],
      ),
    );
  }
}

/// Tap-to-edit modal opened from each piece row. Mirrors the
/// post-publish NK4 needs-edit sheet — uppercase eyebrow, TITLE
/// label, serif input, Cancel + Save changes pair — so the experience
/// is consistent regardless of whether the list has been published.
/// Pre-publish pieces only have a label (no slots, no note, no gear
/// link) so the sheet is just one field.
class _ComposePieceEditSheet extends StatefulWidget {
  const _ComposePieceEditSheet({required this.initialLabel});
  final String initialLabel;

  static Future<String?> show(
    BuildContext context, {
    required String initialLabel,
  }) {
    return showAccessibleModal<String>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => _ComposePieceEditSheet(initialLabel: initialLabel),
    );
  }

  @override
  State<_ComposePieceEditSheet> createState() =>
      _ComposePieceEditSheetState();
}

class _ComposePieceEditSheetState extends State<_ComposePieceEditSheet> {
  late final TextEditingController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = TextEditingController(text: widget.initialLabel);
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  void _save() {
    final next = _ctrl.text.trim();
    if (next.isEmpty) return;
    Navigator.of(context).pop(next);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final nameText = _ctrl.text.trim();
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
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      l10n.needsPickerEditEyebrow.toUpperCase(),
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
                    color: AppColors.modalTextPrimary,
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              NeedsEditFieldLabel(label: l10n.needsPickerEditTitleLabel),
              const SizedBox(height: 6),
              NeedsEditTitleInput(
                controller: _ctrl,
                onChanged: (_) => setState(() {}),
              ),
              const SizedBox(height: 16),
              Row(
                children: [
                  Expanded(
                    flex: 1,
                    child: _CancelButton(
                      label: MaterialLocalizations.of(context)
                          .cancelButtonLabel,
                      onTap: () => Navigator.of(context).pop(),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    flex: 2,
                    child: NeedsEditSaveChangesButton(
                      label: l10n.needsPickerEditSave,
                      enabled: nameText.isNotEmpty,
                      onTap: nameText.isNotEmpty ? _save : null,
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _CancelButton extends StatelessWidget {
  const _CancelButton({required this.label, required this.onTap});
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        height: 50,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: GlassTokens.fillFaint,
          border: Border.all(color: AppColors.modalChipBorder),
          borderRadius: BorderRadius.circular(14),
        ),
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

/// Minimal dashed-rounded-rectangle border used by the inline-add
/// row so it visually echoes the prototype's "tap a suggestion above"
/// dashed treatment without pulling in an extra package. Kept private
/// to this file.
class _DashedRoundedRectangleBorder extends ShapeBorder {
  const _DashedRoundedRectangleBorder({
    required this.color,
    required this.radius,
    this.gap = 4,
    this.dashWidth = 4,
    this.strokeWidth = 1,
  });

  final Color color;
  final Radius radius;
  final double gap;
  final double dashWidth;
  final double strokeWidth;

  @override
  EdgeInsetsGeometry get dimensions => EdgeInsets.all(strokeWidth);

  @override
  ShapeBorder scale(double t) => _DashedRoundedRectangleBorder(
        color: color,
        radius: radius,
        gap: gap * t,
        dashWidth: dashWidth * t,
        strokeWidth: strokeWidth * t,
      );

  @override
  Path getInnerPath(Rect rect, {TextDirection? textDirection}) =>
      Path()..addRRect(RRect.fromRectAndRadius(rect.deflate(strokeWidth), radius));

  @override
  Path getOuterPath(Rect rect, {TextDirection? textDirection}) =>
      Path()..addRRect(RRect.fromRectAndRadius(rect, radius));

  @override
  void paint(Canvas canvas, Rect rect, {TextDirection? textDirection}) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = strokeWidth;
    final path = Path()
      ..addRRect(RRect.fromRectAndRadius(
        rect.deflate(strokeWidth / 2),
        radius,
      ));
    final dashed = Path();
    for (final metric in path.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        final next = distance + dashWidth;
        dashed.addPath(
          metric.extractPath(distance, next.clamp(0, metric.length)),
          Offset.zero,
        );
        distance = next + gap;
      }
    }
    canvas.drawPath(dashed, paint);
  }
}

class _InlineAdd extends StatelessWidget {
  const _InlineAdd({
    required this.controller,
    required this.hint,
    required this.onSubmit,
    required this.onGlass,
  });
  final TextEditingController controller;
  final String hint;
  final VoidCallback onSubmit;
  final bool onGlass;

  @override
  Widget build(BuildContext context) {
    return Container(
      // Dashed surround echoes the prototype's "tap to add" affordance
      // and — critically — prevents the underlying scroll Material from
      // bleeding a solid surface fill behind the TextField (which was
      // rendering as a cream / off-white rectangle on the dark glass).
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
      decoration: ShapeDecoration(
        color: Colors.transparent,
        shape: _DashedRoundedRectangleBorder(
          color: _border(context, onGlass: onGlass),
          radius: const Radius.circular(14),
        ),
      ),
      child: Row(
        children: [
          Icon(Icons.add, size: 18, color: AppColors.lightAccent),
          const SizedBox(width: 10),
          Expanded(
            child: TextField(
              controller: controller,
              onSubmitted: (_) => onSubmit(),
              style: TextStyle(
                color: _textPrimary(context, onGlass: onGlass),
                fontSize: 14,
              ),
              decoration: InputDecoration(
                hintText: hint,
                hintStyle: TextStyle(
                  color: _textSecondary(context, onGlass: onGlass),
                  fontSize: 14,
                ),
                filled: false,
                border: InputBorder.none,
                enabledBorder: InputBorder.none,
                focusedBorder: InputBorder.none,
                isCollapsed: true,
                contentPadding: const EdgeInsets.symmetric(vertical: 10),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
