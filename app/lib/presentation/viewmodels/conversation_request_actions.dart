import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate;
import 'package:ripls/presentation/viewmodels/conversation_state.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart'
    show requestProvider;
import 'package:ripls/services/providers.dart';

final _log = Logger('ConversationRequestActions');

/// ConversationRequestActionsMixin provides request management methods for
/// [ConversationNotifier].
mixin ConversationRequestActionsMixin on Notifier<ConversationState> {
  /// setCachedImpact is implemented by the coordinator. Action mixins call it
  /// to store the impact estimate from a terminal action.
  void setCachedImpact(ImpactEstimate? impact);

  // ── Request status getters ───────────────────────────────────────────────

  
  
  
  
  
  
  // ── Request data fetching ────────────────────────────────────────────────

  /// fetchRequestDetails loads request details from the repository.
  Future<void> fetchRequestDetails() async {
    final conversation = state.conversation;
    if (conversation == null || !conversation.topic.hasRequestId()) {
      return;
    }

    final requestId = conversation.topic.requestId;
    if (requestId.isEmpty) {
      return;
    }

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final request =
          await requestRepository.getRequest(requestId: requestId);

      state = state.copyWith(cachedRequest: request);
      _log.info('Fetched request details: ${request.id}');
    } catch (e) {
      _log.severe('Failed to fetch request details: $e');
    }
  }

  /// fetchRequestImpact fetches impact stats for the request and caches them.
  ///
  /// Called when a FULFILLED system message is received by non-requester
  /// participants who don't have impact data from the fulfillment response.
  Future<void> fetchRequestImpact() async {
    final requestId = state.cachedRequest?.id ?? '';
    if (requestId.isEmpty) return;

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final stats = await requestRepository.getStats(requestId);
      if (!ref.mounted) return;
      if (stats.hasImpact()) {
        setCachedImpact(stats.impact);
      }
    } catch (e) {
      _log.warning('Failed to fetch request impact: $e');
    }
  }

  /// fetchRequestMedia loads the request's stock image URL.
  Future<void> fetchRequestMedia() async {
    final request = state.cachedRequest;
    final mediaId = request?.mediaIds.firstOrNull ?? '';
    if (request == null || mediaId.isEmpty) {
      _log.info('No media ID available for request');
      return;
    }

    try {
      _log.info('Fetching request media for: $mediaId');

      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaUrl = await mediaRepository.getMediaUrl(mediaId);

      state = state.copyWith(requestMediaUrl: mediaUrl.url);

      _log.info(
        'Request media URL fetched - using '
        '${mediaUrl.isThumbnail ? "THUMBNAIL" : "FULL"}: ${mediaUrl.url}',
      );
    } catch (e) {
      _log.warning('Failed to fetch request media: $e');
    }
  }

  // ── Request mutations ────────────────────────────────────────────────────

  /// offerToFulfill creates an offer on a request.
  Future<void> offerToFulfill() async {
    final request = state.cachedRequest;
    if (request == null || request.id.isEmpty) {
      throw Exception('Request not found');
    }

    final communityId = request.communityId;
    if (communityId.isEmpty) {
      throw Exception('Community ID not found for request');
    }

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      await requestRepository.offerToFulfill(
        requestId: request.id,
        communityId: communityId,
      );

      invalidateRequestViewModelIfActive();
      await fetchRequestDetails();
    } catch (e) {
      _log.severe('Failed to offer to fulfill request: $e');
      rethrow;
    }
  }

  /// markRequestFulfilled marks the request as fulfilled.
  Future<void> markRequestFulfilled() async {
    final requestId = state.cachedRequest?.id ?? '';
    if (requestId.isEmpty) {
      throw Exception('Request ID not found');
    }

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final response = await requestRepository.markRequestFulfilled(
        requestId: requestId,
      );

      if (response.hasImpact()) {
        setCachedImpact(response.impact);
      }

      invalidateRequestViewModelIfActive();
      await fetchRequestDetails();
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to mark request as fulfilled: $e');
      rethrow;
    }
  }

  /// cancelRequest cancels the request.
  Future<void> cancelRequest() async {
    final requestId = state.cachedRequest?.id ?? '';
    if (requestId.isEmpty) {
      throw Exception('Request ID not found');
    }

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      await requestRepository.cancelRequest(requestId: requestId);

      invalidateRequestViewModelIfActive();
      await fetchRequestDetails();
      await refreshMessagesAfterMutation();
    } catch (e) {
      _log.severe('Failed to cancel request: $e');
      rethrow;
    }
  }

  /// withdrawOffer removes the current user's offer to fulfill a request.
  ///
  /// After withdrawal, the user is removed from the conversation.
  Future<void> withdrawOffer() async {
    final request = state.cachedRequest;
    if (request == null || request.id.isEmpty) {
      throw Exception('Request ID not found');
    }

    final communityId = request.communityId;
    if (communityId.isEmpty) {
      throw Exception('Community ID not found for request');
    }

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      await requestRepository.withdrawOffer(
        requestId: request.id,
        communityId: communityId,
      );

      invalidateRequestViewModelIfActive();
      await refreshMessagesAfterMutation();

      _log.info('✅ Offer withdrawn from request: ${request.id}');
    } catch (e) {
      _log.severe('Failed to withdraw offer: $e');
      rethrow;
    }
  }

  /// invalidateAndRefreshRequest invalidates the request cache and triggers
  /// a refetch of both the conversation VM state and RequestContentView.
  void invalidateAndRefreshRequest(String requestId) {
    if (requestId.isEmpty) return;

    ref.read(requestRepositoryProvider).invalidate(requestId).then((_) {
      if (!ref.mounted) return;
      fetchRequestDetails();
      invalidateRequestViewModelIfActive();
    }).catchError((Object error) {
      _log.warning('Failed to invalidate/refresh request: $error');
    });
  }

  /// invalidateRequestViewModelIfActive refreshes the requestProvider family
  /// instance so RequestContentView reflects the latest data.
  ///
  /// No-op when the request provider isn't currently alive — otherwise
  /// `ref.read` would resurrect an auto-disposed provider, kick off a
  /// background refresh, and race the auto-dispose timer.
  void invalidateRequestViewModelIfActive() {
    final requestId = state.cachedRequest?.id ?? '';
    if (requestId.isEmpty) return;
    if (!ref.exists(requestProvider(requestId))) return;
    ref.read(requestProvider(requestId).notifier).refresh();
  }

  // ── Cross-mixin hooks ─────────────────────────────────────────────────────

  Future<void> refreshMessagesAfterMutation();
}
