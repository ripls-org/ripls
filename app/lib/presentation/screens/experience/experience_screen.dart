import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/screens/experience/experience_content_view.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/back_button.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';

/// ExperienceScreen displays detailed information about a specific experience.
/// It slides in from the right when opened and animates out when closed.
/// Uses ExperienceContentView to display the experience content.
class ExperienceScreen extends ConsumerStatefulWidget {
  final String experienceId;

  /// Optional community ID to use for the initial data load, overriding the
  /// global communitiesProvider. Used when opening from the portfolio
  /// inbox so the item loads in the community it was grouped under.
  final String? initialCommunityId;
  final double? initialDistanceMeters;

  /// Initial tab index to show when the screen opens (0=Event, 1=Details, 2=Chat).
  final int initialTab;

  const ExperienceScreen({
    super.key,
    required this.experienceId,
    this.initialCommunityId,
    this.initialDistanceMeters,
    this.initialTab = 0,
  });

  @override
  ConsumerState<ExperienceScreen> createState() => _ExperienceScreenState();
}

class _ExperienceScreenState extends ConsumerState<ExperienceScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  Widget build(BuildContext context) {
    return buildSwipeableContent(
      child: Scaffold(
        backgroundColor: Colors.black,
        body: SafeArea(
          child: Stack(
            fit: StackFit.expand,
            children: [
              ExperienceContentView(
                experienceId: widget.experienceId,
                showEditControls: true,
                showOwnerInfo: false,
                onDeleted: handleClose,
                initialCommunityId: widget.initialCommunityId,
                initialDistanceMeters: widget.initialDistanceMeters,
                initialTab: widget.initialTab,
              ),
              // The inline conversation carousel owns its own back button while
              // it is up, so hide the screen's to avoid two stacked back arrows.
              // Likewise while a morph-reveal panel (conversation, roster,
              // time, location) is open — the panel supplies its own close (✕),
              // and a dimmed chevron under its scrim reads as a second, broken
              // dismiss control.
              if (!_isConversationOpen() && !_isPanelExpanded()) _buildAppBar(),
            ],
          ),
        ),
      ),
    );
  }

  /// Whether the inline discussion carousel is up (active tab == chat), in
  /// which case the carousel renders its own back button.
  bool _isConversationOpen() => ref.watch(
    experienceProvider(widget.experienceId).select(
      (s) => !s.isEditing && s.activeTab == s.chatTabIndex,
    ),
  );

  /// Whether a full-screen morph-reveal panel is open over this experience.
  bool _isPanelExpanded() =>
      ref.watch(experienceContentExpandedProvider(widget.experienceId));

  Widget _buildAppBar() {
    return Positioned(
      top: 0,
      left: 0,
      right: 0,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        child: Row(
          children: [
            BackButtonWidget(onPressed: handleClose),
          ],
        ),
      ),
    );
  }
}
