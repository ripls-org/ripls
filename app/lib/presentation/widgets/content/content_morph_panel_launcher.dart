import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/content/morph_reveal_route.dart';

/// Opens [screen] as a full-screen morph-reveal content panel that grows from
/// [sourceRect] over the content's existing hero (docs/client/modals.md —
/// Morph-reveal content panels). This is the content-type-agnostic launcher
/// behind the experience ("Who's pitching in?", conversation) and request
/// (location, helpers) panel interactions.
///
/// It performs the surface bookkeeping every such panel needs:
/// - hides the read surface via [expandedProvider] so only the hero shows
///   behind the panel,
/// - retreats the home top/bottom nav ([HomeNotifier.lockNav]),
/// - pushes [morphRevealRoute] on the root navigator with its own backdrop
///   scrim disabled (`scrimMaxOpacity: 0`) since the panel supplies one,
/// - restores the read surface and nav once the panel fully closes.
///
/// [expandedProvider] is the content item's per-id expanded flag — pass
/// `experienceContentExpandedProvider(experienceId)` or
/// `requestContentExpandedProvider(requestId)`.
///
/// [screen] should wrap its body in `ContentMorphPanel` for the shared scrim +
/// swipe-to-close chrome. Returns the route's pop future.
Future<void> openContentMorphPanel({
  required BuildContext context,
  required WidgetRef ref,
  required NotifierProvider<ContentExpandedNotifier, bool> expandedProvider,
  required Rect sourceRect,
  required Widget screen,
  required String routeName,
}) {
  final flag = ref.read(expandedProvider.notifier);
  final home = ref.read(homeProvider.notifier);
  flag.set(true);
  home.lockNav();
  return Navigator.of(context, rootNavigator: true)
      .push(
        morphRevealRoute<void>(
          screen: screen,
          sourceRect: sourceRect,
          duration: accessibleDuration(
            context,
            const Duration(milliseconds: 420),
          ),
          routeName: routeName,
          scrimMaxOpacity: 0,
        ),
      )
      .whenComplete(() {
        flag.set(false);
        home.unlockNav();
      });
}
