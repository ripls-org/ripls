import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';

class AppBarBackButton extends StatelessWidget {
  final VoidCallback? onPressed;
  final Color? color;

  const AppBarBackButton({super.key, this.onPressed, this.color});

  @override
  Widget build(BuildContext context) {
    return IconAction(
      icon: Icons.arrow_back_ios_new,
      semanticsLabel: context.l10n.a11yBack,
      color: color ?? AppColors.textPrimary(context),
      onPressed: onPressed ?? () => Navigator.of(context).pop(),
    );
  }
}
