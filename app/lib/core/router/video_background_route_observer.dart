import 'package:flutter/widgets.dart';

/// Global RouteObserver that [VideoBackgroundHost] subscribes to so it can
/// pause playback when another route is pushed on top, and resume when that
/// route is popped. Registered in the GoRouter `observers:` list in
/// `app_router.dart` so every navigation through the rooted Navigator
/// dispatches to subscribers.
///
/// Typed on [PageRoute] so transient overlays like dialogs and snackbars
/// don't trigger spurious pauses — only full-screen page pushes (modal
/// sheets, content screens, MediaCarousel) count.
final RouteObserver<PageRoute<dynamic>> videoBackgroundRouteObserver =
    RouteObserver<PageRoute<dynamic>>();
