import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/completion/dark_check_circle.dart'
    show kCompletionAccentLight, CompletionColors;

/// DarkSearchResultTile is a single row inside the search-results container of
/// the completion modals.
///
/// Shows an avatar, a name with optional subtitle, and a trailing action label
/// (defaults to "Add"). The [onAvatarTap] callback is optional — when null the
/// avatar is not separately tappable.
class DarkSearchResultTile extends StatelessWidget {
  const DarkSearchResultTile({
    super.key,
    required this.avatar,
    required this.name,
    this.subtitle,
    required this.onTap,
    this.onAvatarTap,
    this.trailingLabel,
  });

  /// Leading avatar widget.
  final Widget avatar;

  /// Display name.
  final String name;

  /// Optional secondary label (e.g. "Not yet on Ripls", "Create placeholder").
  final String? subtitle;

  /// Called when the whole row is tapped to add the person.
  final VoidCallback onTap;

  /// Called when only the avatar is tapped. Null = not separately tappable.
  final VoidCallback? onAvatarTap;

  /// Label shown on the trailing side. Defaults to "Add".
  final String? trailingLabel;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: name,
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
        child: Row(
          children: [
            Tappable(
              semanticsLabel: context.l10n.a11yMiscViewProfile,
              onTap: onAvatarTap,
              child: avatar,
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    name,
                    style: TextStyle(
                      fontSize: 13,
                      color: CompletionColors.textPrimary(context),
                    ),
                  ),
                  if (subtitle != null)
                    Text(
                      subtitle!,
                      style: TextStyle(
                        fontSize: 10,
                        color: CompletionColors.textDim(context),
                      ),
                    ),
                ],
              ),
            ),
            Text(
              trailingLabel ?? 'Add',
              style: const TextStyle(
                fontSize: 11,
                color: kCompletionAccentLight,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
