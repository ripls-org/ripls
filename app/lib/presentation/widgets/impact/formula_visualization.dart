import 'package:flutter/material.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/presentation/models/drill_down_data.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// FormulaVisualization renders the top-level formula bar with operands and operators.
///
/// Shows operands as tappable chips with an operator symbol between them.
/// Tapping an operand scrolls to its component card in Section 2.
class FormulaVisualization extends StatelessWidget {
  final FormulaNode formula;
  final void Function(int index)? onOperandTap;

  const FormulaVisualization({
    super.key,
    required this.formula,
    this.onOperandTap,
  });

  @override
  Widget build(BuildContext context) {
    final hasOperands =
        formula.operands != null && formula.operands!.isNotEmpty;
    final hasPrefix = formula.prefix != null;

    if (!hasOperands && !hasPrefix) {
      // Leaf formula without prefix — value is already shown in the banner
      // header, so don't duplicate it here.
      return const SizedBox.shrink();
    }

    if (!hasOperands && hasPrefix) {
      return _buildPrefixOnly();
    }

    return _buildCompositeFormula();
  }

  /// Renders just "prefix = value" with no operand chips.
  Widget _buildPrefixOnly() {
    return Wrap(
      spacing: 0,
      runSpacing: 6,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        Padding(
          padding: const EdgeInsets.only(right: 6),
          child: Text(
            formula.prefix!,
            style: const TextStyle(
              color: PaperTokens.textMuted,
              fontSize: 14,
              fontWeight: FontWeight.w500,
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6),
          child: Text(
            '=',
            style: const TextStyle(
              color: PaperTokens.textMuted,
              fontSize: 18,
              fontWeight: FontWeight.w400,
            ),
          ),
        ),
        Text(
          formula.formattedValue,
          style: const TextStyle(
            color: PaperTokens.textPrimary,
            fontSize: 20,
            fontWeight: FontWeight.w700,
                      ),
        ),
      ],
    );
  }

  Widget _buildCompositeFormula() {
    final operands = formula.operands!;
    final op = formula.operator ?? '=';
    final children = <Widget>[];

    // Render prefix (e.g. "Σ 30 items") before operands.
    if (formula.prefix != null) {
      children.add(Padding(
        padding: const EdgeInsets.only(right: 6),
        child: Text(
          formula.prefix!,
          style: const TextStyle(
            color: PaperTokens.textMuted,
            fontSize: 14,
            fontWeight: FontWeight.w500,
          ),
        ),
      ));
    }

    for (var i = 0; i < operands.length; i++) {
      if (i > 0) {
        children.add(Padding(
          padding: const EdgeInsets.symmetric(horizontal: 6),
          child: Text(
            op,
            style: const TextStyle(
              color: PaperTokens.textMuted,
              fontSize: 18,
              fontWeight: FontWeight.w400,
            ),
          ),
        ));
      }

      final index = i;
      final operand = operands[i];
      children.add(
        Tappable(
          semanticsLabel: operand.label,
          onTap: onOperandTap != null ? () => onOperandTap!(index) : null,
          child: _buildOperandChip(operand),
        ),
      );
    }

    // Add result: " = value"
    children.add(Padding(
      padding: const EdgeInsets.symmetric(horizontal: 6),
      child: Text(
        '=',
        style: const TextStyle(
          color: PaperTokens.textMuted,
          fontSize: 18,
          fontWeight: FontWeight.w400,
        ),
      ),
    ));
    children.add(Text(
      formula.formattedValue,
      style: const TextStyle(
        color: PaperTokens.textPrimary,
        fontSize: 20,
        fontWeight: FontWeight.w700,
              ),
    ));

    return Wrap(
      spacing: 0,
      runSpacing: 6,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: children,
    );
  }

  Widget _buildOperandChip(FormulaNode operand) {
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 120),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        decoration: BoxDecoration(
          color: PaperTokens.surfaceRaised,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(
            color: PaperTokens.borderSubtle,
            width: 1,
          ),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              operand.formattedValue,
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: PaperTokens.textPrimary,
                fontSize: 16,
                fontWeight: FontWeight.w600,
                              ),
            ),
            const SizedBox(height: 2),
            Text(
              operand.label.toLowerCase(),
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: PaperTokens.textMuted,
                fontSize: 10,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
