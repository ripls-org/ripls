import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// Bold section-title label for the needs/contributions lists.
class NeedsSectionHeader extends StatelessWidget {
  const NeedsSectionHeader({super.key, required this.title});

  final String title;

  @override
  Widget build(BuildContext context) {
    return Text(
      title,
      style: const TextStyle(
        fontSize: 15,
        fontWeight: FontWeight.w700,
        color: AppColors.onContentImage,
      ),
    );
  }
}
