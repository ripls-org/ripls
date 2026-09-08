import 'package:flutter/material.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/presentation/screens/experience/experience_screen.dart';
import 'package:ripls/presentation/screens/gear/gear_screen.dart';
import 'package:ripls/presentation/screens/request/request_screen.dart';

/// NavigationHelpers provides reusable navigation patterns.
class NavigationHelpers {
  /// Builds a slide-from-right transition for PageRouteBuilder.
  ///
  /// Use this builder for consistent slide animations across the app.
  /// Back button automatically works with this transition.
  static Widget slideFromRightTransition(
    BuildContext context,
    Animation<double> animation,
    Animation<double> secondaryAnimation,
    Widget child,
  ) {
    const begin = Offset(1, 0); // Slide from right
    const end = Offset.zero;
    const curve = Curves.easeInOut;
    final tween = Tween(begin: begin, end: end)
        .chain(CurveTween(curve: curve));
    return SlideTransition(
      position: animation.drive(tween),
      child: child,
    );
  }

  /// Navigates to a screen with slide-from-right animation.
  ///
  /// Use this for consistent navigation across content views.
  /// The optional [routeName] is recorded in [RouteSettings] for analytics.
  static Future<T?> pushWithSlide<T>({
    required BuildContext context,
    required Widget screen,
    bool useRootNavigator = false,
    bool opaque = true,
    String? routeName,
  }) {
    return Navigator.of(context, rootNavigator: useRootNavigator).push<T>(
      PageRouteBuilder(
        opaque: opaque,
        pageBuilder: (context, animation, secondaryAnimation) => screen,
        transitionsBuilder: slideFromRightTransition,
        transitionDuration: const Duration(milliseconds: 250),
        reverseTransitionDuration: const Duration(milliseconds: 200),
        settings: routeName != null ? RouteSettings(name: routeName) : null,
      ),
    );
  }

  /// Navigates to an item detail screen (gear, experience, or request).
  ///
  /// This consolidates the repeated item navigation pattern used across
  /// skill detail views. The [itemType] determines which screen to open.
  /// Uses pushScreen for SwipeToCloseMixin-enabled screens.
  ///
  /// [initialTab] selects the content screen's starting tab (0 = default;
  /// 2 = Discuss, used when opening a conversation thread directly).
  static Future<void> pushToItemScreen({
    required BuildContext context,
    required String itemId,
    required String itemType,
    int initialTab = 0,
  }) {
    final Widget screen;
    final String routeName;
    switch (itemType) {
      case 'gear':
        screen = GearScreen(gearId: itemId, initialTab: initialTab);
        routeName = 'gear_detail';
      case 'experience':
        screen =
            ExperienceScreen(experienceId: itemId, initialTab: initialTab);
        routeName = 'experience_detail';
      case 'request':
        screen = RequestScreen(requestId: itemId, initialTab: initialTab);
        routeName = 'request_detail';
      default:
        return Future.value();
    }
    return pushScreen(context: context, screen: screen, routeName: routeName);
  }

  /// Routes a tap on an [Item] to the matching detail screen. The
  /// unified entry point for the #2012 consolidation — every surface
  /// that surfaces tappable items (Available Now rails, postcards,
  /// inbox rows) collapses onto this helper instead of maintaining
  /// its own kind-to-screen switch.
  ///
  /// `GEAR` / `GIVEAWAY` / `TRANSFER` all route to `GearScreen`
  /// because they share the gear detail surface (a giveaway is gear
  /// in a terminal state; a transfer is an active loan whose subject
  /// is gear). `EXPERIENCE` routes to `ExperienceScreen`, `REQUEST`
  /// to `RequestScreen`. `COMMUNITY` and `UNSPECIFIED` are no-ops —
  /// the inbox surfaces community-milestone rows but they have no
  /// drill-in surface today.
  ///
  /// [initialCommunityId] and [initialTab] are surface-specific
  /// navigation hints used by the portfolio inbox to land on the
  /// right community / tab; other surfaces leave them null.
  static Future<void> pushToItem({
    required BuildContext context,
    required Item item,
    String? initialCommunityId,
    int? initialTab,
    bool useRootNavigator = false,
  }) {
    final contextId = item.contextId;
    if (contextId.isEmpty) return Future.value();
    final Widget screen;
    final String routeName;
    switch (item.kind) {
      case ItemKind.ITEM_KIND_GEAR:
      case ItemKind.ITEM_KIND_GIVEAWAY:
      case ItemKind.ITEM_KIND_TRANSFER:
        screen = GearScreen(
          gearId: contextId,
          initialCommunityId: initialCommunityId,
          initialTab: initialTab ?? 0,
        );
        routeName = 'gear_detail';
      case ItemKind.ITEM_KIND_EXPERIENCE:
        screen = ExperienceScreen(
          experienceId: contextId,
          initialCommunityId: initialCommunityId,
          initialTab: initialTab ?? 0,
        );
        routeName = 'experience_detail';
      case ItemKind.ITEM_KIND_REQUEST:
        screen = RequestScreen(
          requestId: contextId,
          initialCommunityId: initialCommunityId,
          initialTab: initialTab ?? 0,
        );
        routeName = 'request_detail';
      default:
        return Future.value();
    }
    return pushScreen(
      context: context,
      screen: screen,
      routeName: routeName,
      useRootNavigator: useRootNavigator,
    );
  }

  /// Pushes a screen that manages its own slide animation via [SwipeToCloseMixin].
  ///
  /// The route is non-opaque by default so the previous screen remains visible
  /// while the mixin slides content in/out. The identity [transitionsBuilder]
  /// (returns child as-is) prevents double-animation conflicts, and zero
  /// durations ensure the route is added/removed instantly -- all visual
  /// animation is handled by the mixin.
  /// The optional [routeName] is recorded in [RouteSettings] for analytics.
  static Future<T?> pushScreen<T>({
    required BuildContext context,
    required Widget screen,
    bool useRootNavigator = false,
    bool opaque = false,
    String? routeName,
  }) {
    return Navigator.of(context, rootNavigator: useRootNavigator).push<T>(
      PageRouteBuilder(
        opaque: opaque,
        pageBuilder: (context, animation, secondaryAnimation) => screen,
        transitionsBuilder: (context, animation, secondaryAnimation, child) =>
            child,
        // Zero durations because SwipeToCloseMixin manages all animation.
        // Non-zero durations cause a black flash: the mixin slides the screen
        // off-screen, then pop() keeps the opaque route present during its
        // own reverse transition, showing an empty background.
        transitionDuration: Duration.zero,
        reverseTransitionDuration: Duration.zero,
        settings: routeName != null ? RouteSettings(name: routeName) : null,
      ),
    );
  }
}
