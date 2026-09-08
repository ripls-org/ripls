import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// PreviewLocationSelector renders a tappable location row used in the
/// experience preview modal.
///
/// [displayText] is the human-readable location name (or prompt string) built
/// by the caller. [extractedLocationQuery] is the raw AI-extracted query;
/// when non-empty and no location has been geocoded yet it is shown as a
/// subtle hint above [displayText]. [showExtractedQuery] controls whether that
/// hint is visible (true when we have a query but no geocoded/selected location
/// yet). [isLoading] disables the tap target during submission.
class PreviewLocationSelector extends StatelessWidget {
  const PreviewLocationSelector({
    super.key,
    required this.displayText,
    required this.isLoading,
    required this.onTap,
    this.showExtractedQuery = false,
    this.extractedLocationQuery,
  });

  final String displayText;
  final bool isLoading;
  final VoidCallback onTap;
  final bool showExtractedQuery;
  final String? extractedLocationQuery;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yExpSelectLocation,
      onTap: isLoading ? null : onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          color: GlassTokens.fillSubtle,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: GlassTokens.borderSoft),
        ),
        child: Row(
          children: [
            const Icon(
              Icons.location_on,
              size: 20,
              color: GlassTokens.textSecondary,
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (showExtractedQuery &&
                      extractedLocationQuery != null) ...[
                    Text(
                      '📍 $extractedLocationQuery',
                      style: const TextStyle(
                        fontSize: 12,
                        color: GlassTokens.textMuted,
                      ),
                    ),
                    const SizedBox(height: 2),
                  ],
                  Text(
                    displayText,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 14,
                      color: GlassTokens.textSecondary,
                    ),
                  ),
                ],
              ),
            ),
            const Icon(
              Icons.chevron_right,
              size: 20,
              color: GlassTokens.textFaint,
            ),
          ],
        ),
      ),
    );
  }
}
