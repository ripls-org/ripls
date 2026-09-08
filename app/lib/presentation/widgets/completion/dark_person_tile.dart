import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/completion/dark_check_circle.dart';

/// DarkPersonTile is a glassmorphic attendee/helper row for the completion
/// and fulfillment modals.
///
/// Tapping the tile toggles [included]. Pass [subtitle] to show a secondary
/// label (e.g. "Not yet on Ripls" for provisional users). The [onAvatarTap]
/// callback is optional — when null the avatar is not tappable.
class DarkPersonTile extends StatelessWidget {
  const DarkPersonTile({
    super.key,
    required this.leading,
    required this.name,
    this.subtitle,
    required this.included,
    required this.onTap,
    this.onAvatarTap,
  });

  /// Leading avatar widget (e.g. UserAvatar or ProvisionalUserAvatar).
  final Widget leading;

  /// Display name of the person.
  final String name;

  /// Optional secondary label shown below [name] (e.g. "Not yet on Ripls").
  final String? subtitle;

  /// Whether this person is currently confirmed/included.
  final bool included;

  /// Called when the tile is tapped to toggle inclusion.
  final VoidCallback onTap;

  /// Called when the avatar is tapped (opens profile). Null = not tappable.
  final VoidCallback? onAvatarTap;

  @override
  Widget build(BuildContext context) {
    return Toggle(
      semanticsLabel: name,
      selected: included,
      onTap: onTap,
      child: AnimatedContainer(
        duration: accessibleDuration(
          context,
          const Duration(milliseconds: 200),
        ),
        margin: const EdgeInsets.only(bottom: 6),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(
          color: CompletionColors.tileBackground(context, included: included),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: CompletionColors.tileBorder(context, included: included),
          ),
        ),
        child: Row(
          children: [
            Tappable(
              semanticsLabel: context.l10n.a11yMiscViewProfile,
              onTap: onAvatarTap,
              child: leading,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    name,
                    style: TextStyle(
                      fontSize: 14,
                      color: CompletionColors.tileText(
                        context,
                        included: included,
                      ),
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
            DarkCheckCircle(checked: included),
          ],
        ),
      ),
    );
  }
}
