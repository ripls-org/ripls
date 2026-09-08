import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/content/content_view_pane_switcher.dart';
import 'package:ripls/presentation/widgets/content/content_view_tab_bar.dart';

/// ExperienceBottomContent renders the bottom panel of an experience: the tab
/// bar with trailing access ring, and the animated tab pane switcher.
///
/// The caller is responsible for maintaining [slideDirection] as UI state and
/// passing pre-built pane widgets.
class ExperienceBottomContent extends StatelessWidget {
  final ExperienceState state;
  final bool isNavVisible;
  final Color accentColor;
  final Color? firstTabColor;
  final int slideDirection;
  final Widget accessRingTrailing;
  final void Function(int index) onTabChanged;
  final Widget chatPane;
  final Widget eventPane;

  /// Optional credit line rendered just above the tab bar (e.g. "Photo
  /// by … on Unsplash"). Null when there is no attribution to show.
  final Widget? attributionLine;

  /// Optional mute/unmute toggle for video backgrounds. Rendered to the
  /// right of [attributionLine] in the same row, just above the tab bar.
  /// Null when the background is not a video or when editing.
  final Widget? muteButton;

  const ExperienceBottomContent({
    super.key,
    required this.state,
    required this.isNavVisible,
    required this.accentColor,
    required this.firstTabColor,
    required this.slideDirection,
    required this.accessRingTrailing,
    required this.onTabChanged,
    required this.chatPane,
    required this.eventPane,
    this.attributionLine,
    this.muteButton,
  });

  @override
  Widget build(BuildContext context) {
    final bottomPadding = isNavVisible ? 80.0 : 24.0;
    final screenHeight = MediaQuery.of(context).size.height;
    final safeAreaTop = MediaQuery.of(context).padding.top;
    const topOffset = 68.0;
    const buttonHeight = 44.0;
    const belowButton = 0.0;
    const tabBarHeight = 56.0;
    final paneHeight =
        (screenHeight -
                safeAreaTop -
                topOffset -
                buttonHeight -
                belowButton -
                tabBarHeight -
                bottomPadding)
            .clamp(0.0, 720.0);

    final chatTabIndex = state.chatTabIndex;

    return Positioned(
      left: 0,
      right: 0,
      bottom: 0,
      // The caption column (#2912): the panel holds the reading measure over
      // the full-bleed hero on desktop-wide windows; no-op at phone widths.
      child: ContentColumn(
        child: Container(
          decoration: BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [
                Colors.transparent,
                Colors.black.withValues(alpha: 0.65),
                Colors.black.withValues(alpha: 0.95),
              ],
              stops: const [0.0, 0.2, 1.0],
            ),
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (attributionLine != null || muteButton != null)
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 0, 16, 4),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.center,
                    children: [
                      if (attributionLine != null)
                        Expanded(child: attributionLine!)
                      else
                        const Spacer(),
                      if (muteButton != null) ...[
                        const SizedBox(width: 8),
                        muteButton!,
                      ],
                    ],
                  ),
                ),
              ContentViewTabBar(
                firstTabLabel: context.l10n.experienceTabEvent,
                showSecondTab: false,
                conversationId:
                    state.experienceDetails?.experience.conversationId,
                serverUnreadCount:
                    state.experienceDetails?.experience.unreadCount ?? 0,
                totalMessageCount:
                    state.experienceDetails?.experience.messageCount ?? 0,
                activeIndex: state.activeTab.clamp(0, chatTabIndex),
                onChanged: onTabChanged,
                accentColor: accentColor,
                firstTabColor: firstTabColor,
                isEditing: state.isEditing,
                trailing: accessRingTrailing,
              ),
              ConstrainedBox(
                constraints: BoxConstraints(
                  minHeight: state.activeTab == chatTabIndex ? paneHeight : 0.0,
                ),
                child: ContentViewPaneSwitcher(
                  activeIndex: state.activeTab.clamp(0, chatTabIndex),
                  slideDirection: slideDirection,
                  onTabChanged: onTabChanged,
                  paneBuilders: [
                    (_) => ConstrainedBox(
                      constraints: BoxConstraints(maxHeight: paneHeight),
                      child: eventPane,
                    ),
                    (_) => SizedBox(height: paneHeight, child: chatPane),
                  ],
                ),
              ),
              SizedBox(height: bottomPadding),
            ],
          ),
        ),
      ),
    );
  }
}
