import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';

/// Editable list of the claimable needs a request will be born with (#2731).
///
/// The create flow extracts the things a request's text plainly names (one, a
/// few, or none) and seeds one need per entry. This field surfaces that list on
/// the preview so extraction is never invisible: each need is a removable chip,
/// and an inline field adds more (splitting a pasted comma/newline list, like
/// the compose sheet). An empty list is valid — the request is then born with
/// no needs.
class SeedNeedsField extends StatefulWidget {
  const SeedNeedsField({
    super.key,
    required this.needs,
    required this.onChanged,
    this.enabled = true,
    this.labelColor,
    this.textColor,
    this.borderColor,
  });

  /// The current seeded-need labels, in priority order.
  final List<String> needs;

  /// Called with the full next list whenever a need is added or removed.
  final ValueChanged<List<String>> onChanged;

  final bool enabled;

  /// Theming hooks so the field reads correctly on both the glass overlay
  /// (GenRequest preview) and the modal card (unified-create preview).
  final Color? labelColor;
  final Color? textColor;
  final Color? borderColor;

  @override
  State<SeedNeedsField> createState() => _SeedNeedsFieldState();
}

class _SeedNeedsFieldState extends State<SeedNeedsField> {
  final _addController = TextEditingController();
  final _addFocus = FocusNode();

  @override
  void dispose() {
    _addController.dispose();
    _addFocus.dispose();
    super.dispose();
  }

  Color get _text => widget.textColor ?? AppColors.modalTextPrimary;
  Color get _label =>
      widget.labelColor ?? AppColors.modalTextPrimary.withValues(alpha: 0.55);
  Color get _border => widget.borderColor ?? AppColors.modalBorderSubtle;

  void _commitAdd() {
    final raw = _addController.text;
    // Mirror the compose paste-a-list split so a requester can drop several at
    // once ("bins, art supplies, glue").
    final additions = raw
        .split(RegExp(r'[,;\n]+'))
        .map((s) => s.trim())
        .where((s) => s.isNotEmpty)
        .toList();
    if (additions.isEmpty) {
      _addController.clear();
      return;
    }
    widget.onChanged([...widget.needs, ...additions]);
    _addController.clear();
    // Keep focus so the requester can keep listing.
    _addFocus.requestFocus();
  }

  void _removeAt(int index) {
    widget.onChanged([
      for (var i = 0; i < widget.needs.length; i++)
        if (i != index) widget.needs[i],
    ]);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Icon(Icons.add_task, size: 18, color: _text),
            const SizedBox(width: 10),
            Text(
              l10n.unifiedCreateSeedNeedLabel,
              style: TextStyle(color: _label, fontSize: 14),
            ),
          ],
        ),
        if (widget.needs.isNotEmpty) ...[
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              for (var i = 0; i < widget.needs.length; i++)
                _NeedChip(
                  label: widget.needs[i],
                  textColor: _text,
                  borderColor: _border,
                  onRemove: widget.enabled ? () => _removeAt(i) : null,
                ),
            ],
          ),
        ],
        const SizedBox(height: 8),
        Row(
          children: [
            Expanded(
              child: TextField(
                controller: _addController,
                focusNode: _addFocus,
                enabled: widget.enabled,
                style: TextStyle(color: _text, fontSize: 14),
                textCapitalization: TextCapitalization.sentences,
                textInputAction: TextInputAction.done,
                onSubmitted: (_) => _commitAdd(),
                inputFormatters: [LengthLimitingTextInputFormatter(120)],
                decoration: InputDecoration(
                  isDense: true,
                  hintText: l10n.unifiedCreateSeedNeedHint,
                  hintStyle: TextStyle(
                    color: _text.withValues(alpha: 0.4),
                    fontSize: 14,
                  ),
                  border: InputBorder.none,
                  contentPadding: EdgeInsets.zero,
                ),
              ),
            ),
            IconAction(
              icon: Icons.add_circle_outline,
              semanticsLabel: l10n.a11ySeedNeedAdd,
              semanticsIdentifier: 'seed-need-add',
              onPressed: widget.enabled ? _commitAdd : null,
              iconSize: 20,
              color: _text.withValues(alpha: 0.7),
            ),
          ],
        ),
      ],
    );
  }
}

class _NeedChip extends StatelessWidget {
  const _NeedChip({
    required this.label,
    required this.textColor,
    required this.borderColor,
    required this.onRemove,
  });

  final String label;
  final Color textColor;
  final Color borderColor;
  final VoidCallback? onRemove;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Container(
      padding: const EdgeInsets.only(left: 12, right: 4, top: 4, bottom: 4),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: borderColor),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Flexible(
            child: Text(
              label,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: textColor,
                fontSize: 13,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          if (onRemove != null)
            IconAction(
              icon: Icons.close,
              semanticsLabel: l10n.a11yComposeRemovePiece(label),
              semanticsIdentifier: 'seed-need-remove',
              onPressed: onRemove,
              iconSize: 16,
              color: textColor.withValues(alpha: 0.6),
            ),
        ],
      ),
    );
  }
}
