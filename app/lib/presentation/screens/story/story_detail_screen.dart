import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart' show StoryPayload;
import 'package:ripls/presentation/screens/story/story_content_view.dart';
import 'package:ripls/presentation/widgets/back_button.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';

/// StoryDetailScreen wraps [StoryContentView] in a pushable screen with
/// swipe-to-close. The Workshop tab pushes this when a season-hero
/// [WorkshopTile] is tapped.
///
/// Pattern B (overlay screen): no AppBar; a floating [BackButtonWidget]
/// over the content per the navigation conventions in
/// [docs/client/architecture.md].
class StoryDetailScreen extends ConsumerStatefulWidget {
  final StoryPayload story;

  const StoryDetailScreen({super.key, required this.story});

  @override
  ConsumerState<StoryDetailScreen> createState() => _StoryDetailScreenState();
}

class _StoryDetailScreenState extends ConsumerState<StoryDetailScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  Widget build(BuildContext context) {
    return buildSwipeableContent(
      child: Scaffold(
        backgroundColor: AppColors.background(context),
        body: Stack(
          children: [
            Positioned.fill(
              child: StoryContentView(story: widget.story),
            ),
            Positioned(
              top: MediaQuery.of(context).padding.top + 12,
              left: 12,
              child: BackButtonWidget(onPressed: handleClose),
            ),
          ],
        ),
      ),
    );
  }
}
