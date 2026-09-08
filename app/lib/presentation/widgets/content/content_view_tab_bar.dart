import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/content/content_tab_bar.dart';
import 'package:ripls/services/providers.dart';

/// ContentViewTabBar is the standard tab bar used by all content views
/// (gear, request, experience). It watches [unreadCountProvider] internally
/// so callers only need to pass raw server values; the live unread count and
/// badge state are computed here, keeping the logic consistent across views.
///
/// Tab order is [firstTabLabel] | Details | Chat. When [showSecondTab] is
/// false the middle tab is omitted, collapsing the strip to two tabs.
///
/// An optional [trailing] widget is rendered to the right of the tab strip
/// (used by ExperienceContentView to show the mute/unmute volume button).
class ContentViewTabBar extends ConsumerWidget {
  /// Label for the first (type-specific) tab, e.g. "Gear", "Request", "Event".
  final String firstTabLabel;

  /// The conversation ID used to look up live unread counts.
  final String? conversationId;

  /// Server-reported unread count for this conversation.
  final int serverUnreadCount;

  /// Total message count — shown as a gray badge when there are no unread
  /// messages, matching the gear content view behavior.
  final int totalMessageCount;

  final int activeIndex;
  final ValueChanged<int> onChanged;
  final Color accentColor;
  final bool isEditing;

  /// Optional color override for the first tab's active pill. When set, the
  /// first tab pill uses this color instead of [accentColor]. Useful for
  /// conveying action-state — typically [AppColors.transferCoral] when
  /// something needs doing, [AppColors.transferSage] when the user has
  /// completed all actions on that tab.
  final Color? firstTabColor;

  /// Optional override for the second tab label (defaults to "Details").
  final String? secondTabLabel;

  /// Optional override for the third tab label (defaults to "Chat").
  final String? thirdTabLabel;

  /// When false the middle (details/plan/results) tab is omitted entirely
  /// and the chat tab moves into index 1. Used by experience for active
  /// events, which expose all planning surface through the inline rows.
  final bool showSecondTab;

  /// Optional widget placed to the right of the tab strip (e.g. volume button).
  final Widget? trailing;

  const ContentViewTabBar({
    super.key,
    required this.firstTabLabel,
    required this.conversationId,
    required this.serverUnreadCount,
    required this.totalMessageCount,
    required this.activeIndex,
    required this.onChanged,
    required this.accentColor,
    this.isEditing = false,
    this.firstTabColor,
    this.secondTabLabel,
    this.thirdTabLabel,
    this.showSecondTab = true,
    this.trailing,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final unreadCounts = ref
        .watch(unreadCountProvider)
        .conversationIdToUnreadCount;
    final liveUnread = unreadCounts[conversationId ?? ''];
    final unreadCount =
        serverUnreadCount == 0 ? 0 : (liveUnread ?? serverUnreadCount);
    final hasUnread = unreadCount > 0;
    final chatBadge = hasUnread
        ? '$unreadCount'
        : (totalMessageCount > 0 ? '$totalMessageCount' : null);

    final tabs = <ContentTab>[
      ContentTab(label: firstTabLabel, color: firstTabColor),
      if (showSecondTab)
        ContentTab(label: secondTabLabel ?? context.l10n.contentTabDetails),
      ContentTab(
        label: thirdTabLabel ?? context.l10n.contentTabChat,
        badge: chatBadge,
        showBadgeDot: hasUnread,
      ),
    ];

    final tabBar = ContentTabBar(
      tabs: tabs,
      activeIndex: activeIndex,
      onChanged: onChanged,
      accentColor: accentColor,
      isEditing: isEditing,
    );

    if (trailing != null) {
      return Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Expanded(child: tabBar),
          Padding(
            padding: const EdgeInsets.only(right: 16),
            child: trailing!,
          ),
        ],
      );
    }

    return tabBar;
  }
}
