import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

/// Selectable "I'm flexible — any of these works" row shown at the bottom of
/// a poll's voting list. A flexible vote folds into every option's tally
/// server-side (see `LOCATION_VOTE_STATUS_FLEXIBLE` / `TIME_VOTE_STATUS_FLEXIBLE`).
///
/// Implemented as a [Toggle] so assistive tech announces the selected state
/// via the semantics flag rather than baked into the label. Kind-agnostic:
/// the caller passes already-localized [title], [subtitle], and
/// [semanticsLabel] (e.g. "anywhere works" vs "anytime works").
class PollFlexibleVoteRow extends StatelessWidget {
  const PollFlexibleVoteRow({
    super.key,
    required this.title,
    required this.subtitle,
    required this.semanticsLabel,
    required this.selected,
    required this.onTap,
  });

  /// Localized primary line, e.g. "Honestly, anywhere works".
  final String title;

  /// Localized secondary line, e.g. "I'm flexible — go with the crew".
  final String subtitle;

  /// Localized label announced to assistive tech (selected state is
  /// conveyed separately by the [Toggle]).
  final String semanticsLabel;

  /// Whether the viewer has marked themselves flexible.
  final bool selected;

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    final borderColor = selected
        ? accent
        : AppColors.modalTextPrimary.withValues(alpha: 0.22);
    return Toggle(
      semanticsLabel: semanticsLabel,
      selected: selected,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        padding: const EdgeInsets.all(14),
        decoration: BoxDecoration(
          color: selected
              ? accent.withValues(alpha: 0.08)
              : Colors.transparent,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: borderColor),
        ),
        child: Row(
          children: [
            _Check(selected: selected),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    title,
                    style: TextStyle(
                      color: AppColors.modalTextPrimary,
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    subtitle,
                    style: TextStyle(
                      color: AppColors.modalTextSecondary,
                      fontSize: 12.5,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Check extends StatelessWidget {
  const _Check({required this.selected});

  final bool selected;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    return Container(
      width: 24,
      height: 24,
      decoration: BoxDecoration(
        color: selected ? accent : Colors.transparent,
        shape: BoxShape.circle,
        border: Border.all(
          color: selected
              ? accent
              : AppColors.modalTextPrimary.withValues(alpha: 0.3),
          width: 1.5,
        ),
      ),
      child: selected
          ? const Icon(Icons.check, size: 14, color: Color(0xFF0D1A0A))
          : null,
    );
  }
}
