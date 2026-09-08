import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';

/// NeedQuantityBadge is the shared "×N" quantity affordance for needs and
/// contributions. Every chip and row that shows how many of a thing is
/// needed/brought composes this widget, so the quantity reads identically
/// across surfaces (the pitching-in roster, the claim composers, and the
/// collapsed participation cards on both the request and experience content
/// views).
///
/// It renders nothing when [quantity] is 1 or less — a quantity of one is the
/// default and adds no information. The visible glyph is "×N"; screen readers
/// hear the localized "quantity N" instead so the multiplication sign is never
/// read literally.
class NeedQuantityBadge extends StatelessWidget {
  /// How many of the thing — slots needed, or copies a person is bringing.
  final int quantity;

  /// Text style for the glyph. Callers pass the surface's chip/row text style
  /// (usually a bolder weight) so the badge sits flush with its label.
  final TextStyle style;

  const NeedQuantityBadge({
    super.key,
    required this.quantity,
    required this.style,
  });

  @override
  Widget build(BuildContext context) {
    if (quantity <= 1) return const SizedBox.shrink();
    return Semantics(
      label: context.l10n.needsChipQuantitySemantics(quantity),
      excludeSemantics: true,
      child: Text('×$quantity', style: style),
    );
  }
}
