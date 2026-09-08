import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

/// Two-segment Lend / Give choice for a gear-backed request offer (#2702),
/// rendered under the linked-gear tile on the request claim sheet. Lend is
/// the default; the caption under the segments spells out what confirming
/// will start (a loan offer that comes back vs. a giveaway).
class NeedsLendGiveChoice extends StatelessWidget {
  const NeedsLendGiveChoice({
    super.key,
    required this.give,
    required this.onChanged,
  });

  /// False = lend (default), true = give.
  final bool give;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            _segment(
              context,
              label: l10n.needsClaimSheetLendLabel,
              semanticsLabel: l10n.a11yNeedsClaimSheetLend,
              semanticsIdentifier: 'claim-offer-lend',
              selected: !give,
              onTap: () => onChanged(false),
            ),
            const SizedBox(width: 8),
            _segment(
              context,
              label: l10n.needsClaimSheetGiveLabel,
              semanticsLabel: l10n.a11yNeedsClaimSheetGive,
              semanticsIdentifier: 'claim-offer-give',
              selected: give,
              onTap: () => onChanged(true),
            ),
          ],
        ),
        const SizedBox(height: 6),
        Text(
          give ? l10n.needsClaimSheetGiveHint : l10n.needsClaimSheetLendHint,
          style: const TextStyle(
            fontSize: 12.5,
            color: AppColors.modalTextSecondary,
          ),
        ),
      ],
    );
  }

  Widget _segment(
    BuildContext context, {
    required String label,
    required String semanticsLabel,
    required String semanticsIdentifier,
    required bool selected,
    required VoidCallback onTap,
  }) {
    const accent = AppColors.experienceSageGreen;
    return Expanded(
      child: Toggle(
        semanticsLabel: semanticsLabel,
        semanticsIdentifier: semanticsIdentifier,
        selected: selected,
        inMutuallyExclusiveGroup: true,
        inkBorderRadius: BorderRadius.circular(999),
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 9),
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: selected
                ? accent.withValues(alpha: 0.18)
                : Colors.transparent,
            border: Border.all(
              color: selected ? accent : GlassTokens.border,
            ),
            borderRadius: BorderRadius.circular(999),
          ),
          child: Text(
            label,
            style: TextStyle(
              fontSize: 13.5,
              fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
              color: selected ? accent : AppColors.modalTextSecondary,
            ),
          ),
        ),
      ),
    );
  }
}
