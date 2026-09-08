import 'package:flutter/material.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';

/// Reusable instruction overlay for creation modals.
///
/// Displays instruction text with an icon in a semi-transparent badge,
/// positioned above bottom controls. Used consistently across all creation
/// modals (Experience, Community, Request, Gear).
///
/// Example:
/// ```dart
/// Positioned(
///   left: 0,
///   right: 0,
///   bottom: 140,
///   child: CreationInstructionOverlay(
///     icon: Icons.camera_alt,
///     text: 'Capture an activity or flyer',
///   ),
/// )
/// ```
class CreationInstructionOverlay extends StatelessWidget {
  final IconData icon;
  final String text;

  const CreationInstructionOverlay({
    super.key,
    required this.icon,
    required this.text,
  });

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
        margin: const EdgeInsets.symmetric(horizontal: 16),
        decoration: BoxDecoration(
          color: GlassTokens.scrimTint,
          borderRadius: BorderRadius.circular(20),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              icon,
              size: 18,
              color: GlassTokens.textPrimary,
            ),
            const SizedBox(width: 8),
            Flexible(
              child: Text(
                text,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                  color: GlassTokens.textPrimary,
                ),
                textAlign: TextAlign.center,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
