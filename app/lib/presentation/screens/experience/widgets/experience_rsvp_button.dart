import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pbenum.dart'
    show RSVPIntention;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// ExperienceRsvpButton renders an RSVP action button whose appearance
/// reflects the current user's RSVP intention (Going / Maybe / Not Going /
/// unanswered).
class ExperienceRsvpButton extends StatelessWidget {
  final RSVPIntention? intention;
  final VoidCallback onTap;

  const ExperienceRsvpButton({super.key, required this.onTap, this.intention});

  @override
  Widget build(BuildContext context) {
    final (
      Color background,
      Color foreground,
      IconData? icon,
      String text,
    ) = switch (intention) {
      RSVPIntention.RSVP_INTENTION_YES => (
        AppColors.experienceSageGreen,
        Colors.white,
        Icons.check,
        'Going',
      ),
      RSVPIntention.RSVP_INTENTION_MAYBE => (
        AppColors.rsvpMaybeBackground(context),
        AppColors.rsvpMaybeText(context),
        Icons.help_outline,
        'Maybe',
      ),
      RSVPIntention.RSVP_INTENTION_NO => (
        AppColors.rsvpNoBackground(context),
        AppColors.rsvpNoText(context),
        Icons.close,
        'Not Going',
      ),
      _ => (
        AppColors.transferCoralSoft,
        AppColors.darkBackground,
        null,
        'RSVP',
      ),
    };

    final semantics = switch (intention) {
      RSVPIntention.RSVP_INTENTION_YES => context.l10n.a11yExpRsvpGoing,
      RSVPIntention.RSVP_INTENTION_MAYBE => context.l10n.a11yExpRsvpMaybe,
      RSVPIntention.RSVP_INTENTION_NO => context.l10n.a11yExpRsvpNotGoing,
      _ => context.l10n.a11yExpRsvpNotSet,
    };

    return Tappable(
      semanticsLabel: semantics,
      onTap: onTap,
      child: Container(
        height: 28,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        decoration: BoxDecoration(
          color: background,
          border: Border.all(color: background),
          borderRadius: BorderRadius.circular(14),
        ),
        alignment: Alignment.center,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (icon != null) ...[
              Icon(icon, size: 14, color: foreground),
              const SizedBox(width: 4),
            ],
            Text(
              text,
              style: TextStyle(
                color: foreground,
                fontSize: 11,
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
