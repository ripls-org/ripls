// Private widgets shared by two or more needs/contributions sheets.

import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// RSVP disclosure shown in contribution sheets when the user isn't already
/// RSVPed yes.
///
/// Explains the implicit RSVP and lets the user toggle to "maybe".
class RsvpDisclosure extends StatelessWidget {
  const RsvpDisclosure({
    super.key,
    required this.isRsvpedMaybe,
    required this.preferMaybe,
    required this.experienceName,
    required this.onToggle,
  });

  final bool isRsvpedMaybe;
  final bool preferMaybe;
  final String experienceName;
  final VoidCallback onToggle;

  @override
  Widget build(BuildContext context) {
    final name = experienceName.isNotEmpty ? experienceName : 'this event';

    final String message;
    final String toggleLabel;
    if (isRsvpedMaybe) {
      if (preferMaybe) {
        message = "You'll stay marked as a maybe for $name.";
        toggleLabel = "Actually, I'm in";
      } else {
        message = "We'll mark you as going to $name.";
        toggleLabel = 'Make it a maybe instead';
      }
    } else {
      if (preferMaybe) {
        message = "We'll mark you as a maybe for ${name.toLowerCase()}.";
        toggleLabel = "Actually, I'm in";
      } else {
        message = "We'll mark you as going to ${name.toLowerCase()}.";
        toggleLabel = 'Make it a maybe instead';
      }
    }

    final Color bgColor = preferMaybe
        ? const Color(0xFFFBF2E0)
        : AppColors.experienceSageGreen.withValues(alpha: 0.10);
    final Color borderColor = preferMaybe
        ? const Color(0xFFF1E4C6)
        : AppColors.experienceSageGreen.withValues(alpha: 0.25);
    final Color textColor = preferMaybe
        ? const Color(0xFFB08850)
        : AppColors.experienceSageGreen;
    final Color toggleColor = preferMaybe
        ? AppColors.experienceSageGreen
        : const Color(0xFFB08850);

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: borderColor),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            message,
            style: TextStyle(
              fontSize: 12,
              color: textColor,
              fontWeight: FontWeight.w500,
              height: 1.5,
            ),
          ),
          const SizedBox(height: 6),
          Tappable(
            semanticsLabel: toggleLabel,
            onTap: onToggle,
            child: Text(
              toggleLabel,
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: toggleColor,
                decoration: TextDecoration.underline,
                decorationColor: toggleColor.withValues(alpha: 0.4),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
