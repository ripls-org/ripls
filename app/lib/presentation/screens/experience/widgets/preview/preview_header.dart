import 'package:flutter/material.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';

/// PreviewHeader displays the "Generated Preview" badge and subtitle shown
/// at the top of the experience preview modal.
class PreviewHeader extends StatelessWidget {
  const PreviewHeader({super.key});

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Row(
          children: [
            Icon(
              Icons.auto_awesome,
              color: GlassTokens.textPrimary,
              size: 20,
            ),
            SizedBox(width: 8),
            Text(
              'Generated Preview',
              style: TextStyle(
                color: GlassTokens.textPrimary,
                fontSize: 14,
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
        ),
        const SizedBox(height: 4),
        const Text(
          'Review and edit before sharing',
          style: TextStyle(
            color: GlassTokens.textMuted,
            fontSize: 12,
          ),
        ),
      ],
    );
  }
}
