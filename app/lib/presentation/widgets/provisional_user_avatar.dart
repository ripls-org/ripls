import 'package:flutter/material.dart';

/// ProvisionalUserAvatar renders a greyed-out avatar with initials and a small
/// "Not on Ripls" dot badge for visual distinction from registered users.
///
/// Use this widget in place of [UserAvatar] wherever provisional users appear:
/// attendance lists, attendee sheets, story cards, and Manage Members.
class ProvisionalUserAvatar extends StatelessWidget {
  final String name;
  final double radius;

  const ProvisionalUserAvatar({
    super.key,
    required this.name,
    this.radius = 16,
  });

  @override
  Widget build(BuildContext context) {
    final avatar = CircleAvatar(
      radius: radius,
      backgroundColor: const Color(0xFF3E3E3E),
      child: Text(
        _initials(name),
        style: TextStyle(
          color: Colors.white70,
          fontSize: radius * 0.75,
          fontWeight: FontWeight.w500,
        ),
      ),
    );

    final badgeSize = radius * 0.7;

    return SizedBox(
      width: radius * 2,
      height: radius * 2,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          avatar,
          Positioned(
            right: -2,
            bottom: -2,
            child: Container(
              width: badgeSize + 3,
              height: badgeSize + 3,
              decoration: const BoxDecoration(
                color: Color(0xFF1A1210),
                shape: BoxShape.circle,
              ),
              child: Center(
                child: Container(
                  width: badgeSize,
                  height: badgeSize,
                  decoration: const BoxDecoration(
                    color: Color(0xFF6B6B6B),
                    shape: BoxShape.circle,
                  ),
                  child: Icon(
                    Icons.person_off_outlined,
                    size: badgeSize * 0.65,
                    color: Colors.white70,
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// Extracts up to two initials from a name (e.g., "John Doe" → "JD").
  String _initials(String name) {
    if (name.trim().isEmpty) return '?';
    final parts = name.trim().split(RegExp(r'\s+'));
    if (parts.length >= 2) {
      return '${parts.first[0]}${parts.last[0]}'.toUpperCase();
    }
    return name.substring(0, name.length.clamp(0, 2)).toUpperCase();
  }
}
