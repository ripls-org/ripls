import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Sage-circle pill used to open the location picker from the propose
/// and vote modals.
///
/// Two variants:
///   - `big`: centered column with a 52px sage circle, a bold label, and
///     an optional hint paragraph. Used on the propose modal's empty
///     state ("Add your first spot").
///   - small (default): a compact horizontal row with a 20px sage circle
///     and a single label. Used everywhere else ("Add another spot").
///
/// The widget is presentation-only; callers wire the picker open in
/// [onTap]. [onTap] is nullable so callers can disable it during async
/// work (e.g. submitting proposals).
class AddSpotCard extends StatelessWidget {
  final bool big;
  final String label;
  final String? hint;
  final VoidCallback? onTap;

  const AddSpotCard({
    super.key,
    required this.label,
    required this.onTap,
    this.big = false,
    this.hint,
  });

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        padding: EdgeInsets.symmetric(
          horizontal: big ? 18 : 16,
          vertical: big ? 22 : 12,
        ),
        decoration: BoxDecoration(
          color: big
              ? AppColors.surface(context).withValues(alpha: 0.04)
              : Colors.transparent,
          border: Border.all(
            color: accent.withValues(alpha: 0.40),
            width: 1.5,
          ),
          borderRadius: BorderRadius.circular(big ? 18 : 14),
        ),
        child: big
            ? Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Container(
                    width: 52,
                    height: 52,
                    decoration: BoxDecoration(
                      color: accent.withValues(alpha: 0.22),
                      border: Border.all(
                        color: accent.withValues(alpha: 0.40),
                        width: 1.5,
                      ),
                      shape: BoxShape.circle,
                    ),
                    alignment: Alignment.center,
                    child: Icon(Icons.place, size: 22, color: accent),
                  ),
                  const SizedBox(height: 10),
                  Text(
                    label,
                    style: TextStyle(
                      color: AppColors.modalTextPrimary,
                      fontWeight: FontWeight.w700,
                      fontSize: 17,
                    ),
                  ),
                  if (hint != null) ...[
                    const SizedBox(height: 6),
                    SizedBox(
                      width: 250,
                      child: Text(
                        hint!,
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          color: AppColors.modalTextSecondary,
                          fontSize: 12.5,
                          height: 1.4,
                        ),
                      ),
                    ),
                  ],
                ],
              )
            : Row(
                mainAxisAlignment: MainAxisAlignment.center,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Container(
                    width: 20,
                    height: 20,
                    decoration: BoxDecoration(
                      color: accent.withValues(alpha: 0.22),
                      border: Border.all(
                        color: accent.withValues(alpha: 0.40),
                        width: 1,
                      ),
                      shape: BoxShape.circle,
                    ),
                    alignment: Alignment.center,
                    child: Icon(Icons.place, size: 11, color: accent),
                  ),
                  const SizedBox(width: 8),
                  Text(
                    label,
                    style: TextStyle(
                      color: AppColors.modalTextSecondary,
                      fontWeight: FontWeight.w600,
                      fontSize: 13,
                    ),
                  ),
                ],
              ),
      ),
    );
  }
}
