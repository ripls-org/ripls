import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Compact tappable people summary row for content Details tabs.
///
/// Shows an overlapping avatar stack + [summaryText] + chevron. No card
/// wrapper — sits directly in the Details pane layout. Tapping calls [onTap].
///
/// Used by both experience (attendees) and request (helpers) content views.
class ContentPeopleRow extends ConsumerWidget {
  const ContentPeopleRow({
    super.key,
    required this.users,
    required this.summaryText,
    required this.onTap,
  });

  /// Users to display in the avatar stack (ordered: most relevant first).
  final List<User> users;

  /// Summary text shown beside the avatar stack (e.g. "2 going · 1 maybe").
  final String summaryText;

  /// Called when the row is tapped.
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Tappable(
      semanticsLabel: summaryText,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(
          color: Colors.black.withValues(alpha: 0.40),
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: Colors.white.withValues(alpha: 0.10)),
        ),
        child: Row(
          children: [
            if (users.isNotEmpty) ...[
              _AvatarStack(users: users),
              const SizedBox(width: 10),
            ],
            Expanded(
              child: Text(
                summaryText,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: Colors.white.withValues(alpha: 0.65),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Small overlapping avatar stack (max 4, dark theme).
class _AvatarStack extends ConsumerWidget {
  const _AvatarStack({required this.users});

  final List<User> users;

  static const double _size = 28;
  static const int _maxVisible = 4;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final visible = users.length > _maxVisible ? _maxVisible : users.length;
    final overflow = users.length - _maxVisible;
    final itemCount = overflow > 0 ? visible + 1 : visible;
    final totalWidth = itemCount > 0 ? (itemCount - 1) * (_size * 0.7) + _size : 0.0;

    return SizedBox(
      height: _size,
      width: totalWidth,
      child: Stack(
        children: [
          for (int i = 0; i < visible; i++)
            Positioned(
              left: i * (_size * 0.7),
              child: Container(
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  border: Border.all(
                    color: Colors.black.withValues(alpha: 0.60),
                    width: 1.5,
                  ),
                ),
                child: UserAvatar(user: users[i], radius: _size / 2),
              ),
            ),
          if (overflow > 0)
            Positioned(
              left: visible * (_size * 0.7),
              child: Container(
                width: _size,
                height: _size,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: Colors.black.withValues(alpha: 0.40),
                  border: Border.all(
                    color: Colors.black.withValues(alpha: 0.60),
                    width: 1.5,
                  ),
                ),
                child: Center(
                  child: Text(
                    '+$overflow',
                    style: TextStyle(
                      fontSize: _size * 0.35,
                      fontWeight: FontWeight.w600,
                      color: Colors.white.withValues(alpha: 0.65),
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
