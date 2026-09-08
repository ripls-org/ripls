import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Circular Close (X) affordance used at the trailing edge of search
/// pill surfaces. Shared between the Feed's idle/active search pill
/// and the Discover screen's results header so the affordance reads
/// the same across surfaces.
class CloseSearchButton extends StatelessWidget {
  const CloseSearchButton({
    super.key,
    required this.onTap,
    required this.semanticsLabel,
  });

  final VoidCallback onTap;
  final String semanticsLabel;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(14),
      child: SizedBox(
        width: 28,
        height: 28,
        child: Icon(
          Icons.close,
          size: 16,
          color: AppColors.textSecondary(context),
        ),
      ),
    );
  }
}
