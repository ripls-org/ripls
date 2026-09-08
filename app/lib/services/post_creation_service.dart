import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/services/providers/cache_providers.dart';

/// PostCreationService handles the consistent UX flow after creating content
/// (gear, requests, experiences). It ensures that:
/// 1. The feed is refreshed to show the new item
/// 2. The user is navigated to the feed tab
/// 3. The feed scrolls to the top to show the new item
///
/// This centralizes post-creation logic that was previously duplicated across
/// multiple modals and screens.
class PostCreationService {
  final Ref _ref;

  PostCreationService(this._ref);

  /// Handles the complete post-creation flow:
  /// 1. Refreshes the feed to fetch the newly created item
  /// 2. Navigates to the feed tab (index 0)
  /// 3. Scrolls to the top of the feed
  ///
  /// This should be called after any content creation (gear, request, experience)
  /// is successfully saved to the database.
  Future<void> handlePostCreation() async {
    // Refresh feed in-place (no loading spinner) so the PageController stays
    // attached. Using refresh() here would set isLoading: true, removing the
    // PageView from the tree and detaching the PageController — causing
    // navigateToTab's scrollToTop to silently fail (hasClients == false).
    await _ref.read(feedProvider.notifier).refreshInPlace();

    // Navigate to feed tab and scroll to top to show new item.
    _ref.read(homeProvider.notifier).navigateToTab(0, scrollToTop: true);
  }

  /// Re-invalidates the feed + portfolio (home) caches after the share sheet
  /// completes. A newly-created item is only added to its communities during
  /// the share, which happens AFTER [handlePostCreation] has already run and
  /// refreshed the feed — so without this the new item is absent from both the
  /// feed and the home "Yours" list until a manual pull-to-refresh. Firing the
  /// invalidation notifiers mirrors the real-time event path and lets each
  /// list's listener refetch on its own.
  void refreshAfterShare() {
    _ref.read(feedListingCacheInvalidationProvider.notifier).notify();
    _ref.read(portfolioCacheInvalidationProvider.notifier).notify();
  }
}

/// Provider for PostCreationService
final postCreationServiceProvider = Provider<PostCreationService>((ref) {
  return PostCreationService(ref);
});
