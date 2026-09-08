import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_avatar.dart';

/// ContentOwnerRow displays the owner avatar, name, and subtitle at the top
/// of a content view.
///
/// Shown in normal (non-editing) mode. Replaced by [ContentEditBar] when
/// the user enters edit mode. Hidden in feed context (the feed header takes
/// over that role).
///
/// When [compactMode] is true, only the back button is rendered. Used when
/// the Chat tab is active from the discover screen so the back button remains
/// visible without showing owner info.
class ContentOwnerRow extends ConsumerWidget {
  final User owner;
  final String subtitle;
  final Color accentColor;

  /// When true, only the back button is rendered. Used on the Chat tab
  /// when opened from the discover screen (not in feed context).
  final bool compactMode;

  /// Called when the (i) info button is tapped. If null the button is not shown.
  final VoidCallback? onInfoTap;

  /// Optional volume toggle widget placed to the left of the info button.
  /// Only shown for content with video backgrounds.
  final Widget? volumeButton;

  const ContentOwnerRow({
    super.key,
    required this.owner,
    required this.subtitle,
    required this.accentColor,
    this.compactMode = false,
    this.onInfoTap,
    this.volumeButton,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final topPadding = MediaQuery.of(context).padding.top;
    return Positioned(
      top: 0,
      left: 0,
      right: 0,
      child: Padding(
        padding: EdgeInsets.only(top: topPadding + 12, left: 16, right: 16),
        child: compactMode ? _buildCompact(context) : _buildFull(context),
      ),
    );
  }

  /// _buildCompact renders only the back button for the Chat tab from discover.
  Widget _buildCompact(BuildContext context) {
    return Row(children: [_BackButton()]);
  }

  /// _buildFull renders the complete owner row with avatar, name, and buttons.
  Widget _buildFull(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        ContentAvatar(user: owner, size: 36, backgroundColor: accentColor),
        const SizedBox(width: 10),
        Expanded(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                owner.name,
                style: const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              Text(
                subtitle,
                style: TextStyle(
                  fontSize: 11,
                  color: Colors.white.withValues(alpha: 0.55),
                ),
              ),
            ],
          ),
        ),
        if (volumeButton != null) ...[const SizedBox(width: 8), volumeButton!],
        if (onInfoTap != null) ...[
          const SizedBox(width: 8),
          _InfoButton(onTap: onInfoTap!),
        ],
      ],
    );
  }
}

/// _BackButton renders a circular back arrow button used in compact mode.
class _BackButton extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yBack,
      onTap: () => Navigator.of(context).maybePop(),
      child: Container(
        width: 32,
        height: 32,
        decoration: BoxDecoration(
          color: Colors.black.withValues(alpha: 0.22),
          shape: BoxShape.circle,
          border: Border.all(color: Colors.white.withValues(alpha: 0.18)),
        ),
        child: const Icon(Icons.arrow_back, color: Colors.white, size: 16),
      ),
    );
  }
}

/// _InfoButton renders the (i) info icon button shown in the owner row.
class _InfoButton extends StatelessWidget {
  final VoidCallback onTap;
  const _InfoButton({required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yShowDetails,
      onTap: onTap,
      child: Container(
        width: 32,
        height: 32,
        decoration: BoxDecoration(
          color: Colors.black.withValues(alpha: 0.22),
          shape: BoxShape.circle,
          border: Border.all(color: Colors.white.withValues(alpha: 0.18)),
        ),
        child: const Icon(Icons.info_outline, color: Colors.white, size: 24),
      ),
    );
  }
}
