import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/screens/gear/gear_content_view.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/widgets/back_button.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';

/// GearScreen displays detailed information about a specific gear item.
/// It slides in from the right when opened and animates out when closed.
/// Uses GearContentView to display the gear content.
class GearScreen extends ConsumerStatefulWidget {
  final String gearId;

  /// Optional community ID to use for the initial data load, overriding the
  /// global communitiesProvider. Used when opening from the portfolio
  /// inbox so the item loads in the community it was grouped under.
  final String? initialCommunityId;
  final double? initialDistanceMeters;

  /// Initial tab index to show when the screen opens (0=Type, 1=Details, 2=Chat).
  final int initialTab;

  const GearScreen({
    super.key,
    required this.gearId,
    this.initialCommunityId,
    this.initialDistanceMeters,
    this.initialTab = 0,
  });

  @override
  ConsumerState<GearScreen> createState() => _GearScreenState();
}

class _GearScreenState extends ConsumerState<GearScreen>
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
              GearContentView(
                gearId: widget.gearId,
                showEditControls: true,
                showOwnerInfo: false,
                onDeleted: handleClose,
                initialCommunityId: widget.initialCommunityId,
                initialDistanceMeters: widget.initialDistanceMeters,
                initialTab: widget.initialTab,
              ),
              // A morph-reveal panel (conversation, details, …) supplies its
              // own close (✕); hide the screen's chevron while one is open so
              // there's a single dismiss affordance instead of a dimmed
              // chevron under the panel scrim.
              if (!ref.watch(gearContentExpandedProvider(widget.gearId)))
                Positioned(
                  top: 0,
                  left: 0,
                  right: 0,
                  child: Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                    child: Row(
                      children: [BackButtonWidget(onPressed: handleClose)],
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
