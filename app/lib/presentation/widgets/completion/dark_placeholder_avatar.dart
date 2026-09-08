import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/completion/dark_check_circle.dart'
    show CompletionColors;

/// DarkPlaceholderAvatar renders a gradient circle avatar with initials for
/// the completion modals.
///
/// Used as the leading widget in search result tiles when the person being
/// added does not yet exist as a provisional user. The [initials] string should be
/// pre-computed by the caller (e.g. first letters of first and last name).
class DarkPlaceholderAvatar extends StatelessWidget {
  const DarkPlaceholderAvatar({
    super.key,
    required this.initials,
    required this.radius,
  });

  final String initials;
  final double radius;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: radius * 2,
      height: radius * 2,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        gradient: const LinearGradient(
          colors: [Color(0xFF5A5A5A), Color(0xFF3E3E3E)],
        ),
        border: Border.all(
          color: CompletionColors.glassBorder(context),
          width: 1.5,
        ),
      ),
      alignment: Alignment.center,
      child: Text(
        initials,
        style: TextStyle(color: Colors.white, fontSize: radius * 0.65),
      ),
    );
  }
}
