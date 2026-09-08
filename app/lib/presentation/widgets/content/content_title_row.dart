import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_theme.dart';

/// ContentTitleRow renders the serif title shown at the top of every
/// content view. An optional [trailing] widget is rendered to the right of
/// the title — used by the experience pane for the owner edit-pencil.
class ContentTitleRow extends StatelessWidget {
  final String title;
  final Widget? trailing;

  const ContentTitleRow({
    super.key,
    required this.title,
    this.trailing,
  });

  @override
  Widget build(BuildContext context) {
    const textStyle = TextStyle(
      fontFamily: AppTheme.headingFont,
      fontSize: 26,
      fontWeight: FontWeight.bold,
      color: Colors.white,
      height: 1.1,
      shadows: [
        Shadow(
          blurRadius: 12,
          color: Color(0x73000000),
          offset: Offset(0, 2),
        ),
      ],
    );
    if (trailing == null) {
      return Text(title, style: textStyle);
    }
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(child: Text(title, style: textStyle)),
        const SizedBox(width: 8),
        trailing!,
      ],
    );
  }
}
