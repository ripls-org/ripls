import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/data/repositories/media_url.dart';

part 'home_view_model.freezed.dart';

/// State for the Home screen
@freezed
sealed class HomeState with _$HomeState {
  const factory HomeState({
    // Stack index 1 is the Inbox — now the first/landing tab.
    @Default(1) int selectedIndex,
    @Default(true) bool isNavVisible,
  }) = _HomeState;

  const HomeState._();

  /// Helper to get user initials from their name
  String getInitials(String? name) {
    if (name == null || name.isEmpty) return 'U';
    final parts = name.trim().split(' ').where((p) => p.isNotEmpty).toList();
    if (parts.isEmpty) return 'U';
    if (parts.length >= 2) {
      return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
    }
    return parts[0][0].toUpperCase();
  }
}

/// Notifier for managing the Home screen state
class HomeNotifier extends Notifier<HomeState> {
  // Internal lock counter: while > 0, showNav() is suppressed.
  // Content views increment this when entering chat/edit mode and decrement
  // when leaving, preventing the feed scroll listener from re-showing the nav
  // and causing a rapid show/hide oscillation.
  int _navLockCount = 0;

  @override
  HomeState build() {
    return const HomeState();
  }

  /// Navigate to a specific tab index
  ///
  /// [index] - The tab index to navigate to
  /// [scrollToTop] - If true, scrolls the tab to top after navigation (defaults to false)
  void navigateToTab(int index, {bool scrollToTop = false}) {
    final wasOnTab = state.selectedIndex == index;

    if (!wasOnTab) {
      state = state.copyWith(selectedIndex: index, isNavVisible: true);
    } else {
      // Always show nav when re-tapping current tab
      showNav();
    }

    if (scrollToTop) {
      if (wasOnTab) {
        // Already on tab, scroll immediately
        _scrollToTop(index);
      } else {
        // Switched to tab, schedule scroll after render
        WidgetsBinding.instance.addPostFrameCallback((_) {
          _scrollToTop(index);
        });
      }
    }
  }

  /// Internal helper to scroll a tab to the top. Indices are IndexedStack
  /// positions: 0=Feed, 1=Home, 2=Library (discover map), 3=People/Workshop,
  /// 4=Plans. Home, People, and Plans manage their own scroll state and
  /// have no shared controller here.
  void _scrollToTop(int index) {
    switch (index) {
      case 0: // Feed (orphaned stack entry, kept for legacy routing)
        final feedController = ref.read(feedScrollControllerProvider);
        if (feedController.hasClients) {
          feedController.animateToPage(
            0,
            duration: const Duration(milliseconds: 300),
            curve: Curves.easeInOut,
          );
        }
        break;
      case 2: // Library — the discover map's results sheet
        final discoverController = ref.read(discoverScrollControllerProvider);
        if (discoverController.hasClients) {
          discoverController.animateTo(
            0,
            duration: const Duration(milliseconds: 300),
            curve: Curves.easeInOut,
          );
        }
        break;
      case 3: // Workshop (legacy flag-off tab)
        final workshopController = ref.read(workshopScrollControllerProvider);
        if (workshopController.hasClients) {
          workshopController.animateToPage(
            0,
            duration: const Duration(milliseconds: 300),
            curve: Curves.easeInOut,
          );
        }
        break;
    }
  }

  /// Show the navigation (header and bottom nav).
  ///
  /// No-op when [lockNav] has been called without a matching [unlockNav].
  void showNav() {
    if (_navLockCount == 0 && !state.isNavVisible) {
      state = state.copyWith(isNavVisible: true);
    }
  }

  /// Hide the navigation (header and bottom nav).
  void hideNav() {
    if (state.isNavVisible) {
      state = state.copyWith(isNavVisible: false);
    }
  }

  /// Prevent [showNav] from making the nav visible.
  ///
  /// Content views call this when entering chat or edit mode so that the feed
  /// scroll listener's [showNav] calls do not cause a show/hide oscillation.
  /// Each [lockNav] must be paired with a corresponding [unlockNav].
  void lockNav() {
    _navLockCount++;
    hideNav();
  }

  /// Release a nav lock acquired with [lockNav].
  ///
  /// When the lock count reaches zero the nav is automatically restored
  /// unless [silent] is true.  Content view `dispose()` methods pass
  /// `silent: true` so that scrolling to a new feed page (which unmounts
  /// the old content view) does not flash the nav bars back on.
  void unlockNav({bool silent = false}) {
    if (_navLockCount > 0) {
      _navLockCount--;
    }
    if (!silent && _navLockCount == 0 && !state.isNavVisible) {
      try {
        state = state.copyWith(isNavVisible: true);
      } catch (_) {
        // Container may be disposed during logout — safe to ignore.
      }
    }
  }

  /// Toggle the navigation visibility (header and bottom nav).
  ///
  /// No-op when [lockNav] has been called without a matching [unlockNav].
  void toggleNav() {
    if (_navLockCount > 0) return;
    state = state.copyWith(isNavVisible: !state.isNavVisible);
  }

  /// Gets media URL for a given media ID via MediaRepository.
  ///
  /// The repository handles caching, so this is just a thin but necessary
  /// wrapper to maintain MVVM architecture - widgets should call ViewModels,
  /// not repositories or utilities directly.
  Future<MediaUrl> getMediaUrl(String mediaId) =>
      MediaHelpers.getMediaUrl(ref, mediaId);
}

/// Provider for the Home screen state
final homeProvider = NotifierProvider<HomeNotifier, HomeState>(
  HomeNotifier.new,
);

/// Scroll controller providers for each tab
/// These are kept separate from the state because they're Flutter widgets
/// that need to be disposed properly and shouldn't be part of immutable state

final feedScrollControllerProvider = Provider<PageController>((ref) {
  final controller = PageController();
  ref.onDispose(() => controller.dispose());
  return controller;
});

final workshopScrollControllerProvider = Provider<PageController>((ref) {
  final controller = PageController();
  ref.onDispose(() => controller.dispose());
  return controller;
});

final discoverScrollControllerProvider = Provider<ScrollController>((ref) {
  final controller = ScrollController();
  ref.onDispose(() => controller.dispose());
  return controller;
});

final metricsScrollControllerProvider = Provider<ScrollController>((ref) {
  final controller = ScrollController();
  ref.onDispose(() => controller.dispose());
  return controller;
});

final governanceScrollControllerProvider = Provider<ScrollController>((ref) {
  final controller = ScrollController();
  ref.onDispose(() => controller.dispose());
  return controller;
});

/// Discover screen key notifier for forcing rebuilds
class DiscoverKeyNotifier extends Notifier<Key> {
  @override
  Key build() => UniqueKey();

  void refresh() {
    state = UniqueKey();
  }
}

final discoverKeyProvider = NotifierProvider<DiscoverKeyNotifier, Key>(
  DiscoverKeyNotifier.new,
);

/// Discover navigator key provider
final discoverNavigatorKeyProvider = Provider<GlobalKey<NavigatorState>>((ref) {
  return GlobalKey<NavigatorState>();
});

/// Helper function to scroll a tab to the top
/// This is called from the bottom nav when re-tapping the current tab
void scrollToTop(WidgetRef ref, int index) {
  ref.read(homeProvider.notifier)._scrollToTop(index);
}
