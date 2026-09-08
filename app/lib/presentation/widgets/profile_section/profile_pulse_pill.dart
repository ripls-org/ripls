import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// The identity header's live-pulse pill (v11 synthesis): a small
/// green-dot chip carrying the freshest sign of life — the next
/// gathering together, the last one, whatever the host surface has.
/// Hosts hide it entirely when they have nothing live to say.
class ProfilePulsePill extends StatelessWidget {
  const ProfilePulsePill({super.key, required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.primary(context);
    return Container(
      padding: const EdgeInsets.fromLTRB(9, 6, 12, 6),
      decoration: BoxDecoration(
        color: accent.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: accent.withValues(alpha: 0.3)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: AppColors.statusSuccess(context),
              boxShadow: [
                BoxShadow(
                  color: AppColors.statusSuccess(context)
                      .withValues(alpha: 0.22),
                  spreadRadius: 3,
                ),
              ],
            ),
          ),
          const SizedBox(width: 7),
          Flexible(
            child: Text(
              text,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w600,
                color: Color.lerp(accent, Colors.white, 0.35),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
