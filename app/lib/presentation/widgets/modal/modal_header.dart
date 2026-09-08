import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// ModalHeader displays a centered title at the top of modals.
///
/// This widget provides consistent header styling across all modals
/// (request management, experience RSVP, etc.). It uses the app's
/// headlineMedium text style with custom weight and spacing.
///
/// Example:
/// ```dart
/// ModalHeader(title: experience.name)
/// ```
class ModalHeader extends StatelessWidget {
  final String title;
  final TextStyle? style;

  const ModalHeader({
    super.key,
    required this.title,
    this.style,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 24),
      child: Semantics(
        header: true,
        child: Text(
          title,
          textAlign: TextAlign.center,
          style: style ??
              Theme.of(context).textTheme.headlineMedium?.copyWith(
                    color: AppColors.textPrimary(context),
                    fontWeight: FontWeight.w600,
                    letterSpacing: -0.5,
                  ),
        ),
      ),
    );
  }
}
