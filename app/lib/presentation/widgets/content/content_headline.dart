import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';

/// ContentHeadline renders the serif title and supporting description that open
/// the content sheet (see docs/issues/2278-experience-content-redesign.md).
///
/// The title uses [AppTheme.headingFont] (the app's existing serif), matching
/// the editorial treatment in the mockups without introducing a new font. This
/// is an always-on-dark surface, so text colors come from the `dark*` tokens.
/// Role-agnostic: it takes plain strings the caller resolves.
class ContentHeadline extends StatelessWidget {
  /// The headline text (e.g. the event or request title).
  final String title;

  /// Optional supporting description shown below the title.
  final String? description;

  const ContentHeadline({super.key, required this.title, this.description});

  @override
  Widget build(BuildContext context) {
    final desc = description?.trim();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          title,
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 28,
            fontWeight: FontWeight.w600,
            color: AppColors.onContentImage,
            height: 1.08,
            letterSpacing: -0.4,
          ),
        ),
        if (desc != null && desc.isNotEmpty) ...[
          const SizedBox(height: 10),
          Text(
            desc,
            style: const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w400,
              color: AppColors.onContentImage,
              height: 1.5,
            ),
          ),
        ],
      ],
    );
  }
}
