import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' as pb show RequestState;
import 'package:ripls/data/gen/ripls/api/social.pb.dart' show SocialContext;
import 'package:ripls/presentation/viewmodels/request_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('RequestEditActions');

/// RequestEditActionsMixin provides content editing, fulfillment, offer, and
/// cancellation methods for [RequestNotifier].
///
/// Coordination pattern: this mixin is used by the single [RequestNotifier]
/// coordinator class. All state reads and writes go through [state] and
/// [state.copyWith], which are the coordinator's own [Notifier.state] fields.
mixin RequestEditActionsMixin on Notifier<RequestState> {
  // ---------------------------------------------------------------------------
  // Edit mode
  // ---------------------------------------------------------------------------

  /// toggleEditMode flips the edit mode flag.
  void toggleEditMode() {
    state = state.copyWith(isEditing: !state.isEditing);
  }

  // ---------------------------------------------------------------------------
  // Content mutations
  // ---------------------------------------------------------------------------

  /// saveSocialContext saves only the social dimension attributes for the
  /// current request, performing a silent background refresh so impact tiles
  /// update without a loading spinner.
  ///
  /// Only [requestId] and [mediaIds] are sent alongside [socialContext]. Text
  /// fields and location are intentionally omitted to avoid side effects (e.g.
  /// triggering LLM re-inference on the server). [mediaIds] must always be
  /// sent because the server always replaces the repeated field, which would
  /// clear existing media if omitted.
  Future<void> saveSocialContext(SocialContext socialContext) async {
    final details = state.requestDetails;
    if (details == null) return;

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      await requestRepository.updateRequest(
        requestId: details.id,
        mediaIds: details.mediaIds,
        socialContext: socialContext,
      );
      await refreshRequestDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error saving social context: $e', e, stackTrace);
      rethrow;
    }
  }

  /// saveChanges updates the request title, description, and media.
  Future<void> saveChanges({
    required String title,
    required String description,
  }) async {
    if (state.isSaving || state.requestDetails == null) return;

    state = state.copyWith(isSaving: true);

    try {
      final requestDetails = state.requestDetails!;

      final effectiveMediaIds = state.mediaId != null
          ? [state.mediaId!]
          : requestDetails.mediaIds;

      final requestRepository = ref.read(requestRepositoryProvider);
      await requestRepository.updateRequest(
        requestId: requestDetails.id,
        title: title.trim(),
        description: description.trim(),
        mediaIds: effectiveMediaIds.isNotEmpty ? effectiveMediaIds : null,
        locationId: requestDetails.locationId.isNotEmpty
            ? requestDetails.locationId
            : null,
      );

      final updatedRequest = Request(
        id: requestDetails.id,
        requester: requestDetails.requester,
        title: title.trim(),
        description: description.trim(),
        state: requestDetails.state,
        offerers: requestDetails.offerers,
        conversationId: requestDetails.conversationId,
        mediaIds: effectiveMediaIds,
        createdAtUnixSec: requestDetails.createdAtUnixSec,
        locationId: requestDetails.locationId,
        locationName: requestDetails.locationName,
      );

      state = state.copyWith(
        requestDetails: updatedRequest,
        isEditing: false,
        isSaving: false,
      );

      _log.info('✅ Request updated successfully');
    } catch (e, stackTrace) {
      _log.severe('❌ Error saving request: $e', e, stackTrace);
      state = state.copyWith(
        isSaving: false,
        error: const UserError.generic(fallback: 'Could not save changes'),
      );
    }
  }

  /// deleteRequest permanently deletes the request.
  Future<void> deleteRequest() async {
    if (state.requestDetails == null) return;

    _log.info('🗑️ Deleting request: ${state.requestDetails!.id}');

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      await requestRepository.deleteRequest(
        requestId: state.requestDetails!.id,
      );

      _log.info('✅ Request deleted successfully');
    } catch (e, stackTrace) {
      _log.severe('❌ Error deleting request: $e', e, stackTrace);
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Could not delete request'),
      );
      rethrow;
    }
  }

  /// updateLocation sets a new location on the request.
  Future<void> updateLocation(String locationId) async {
    if (state.requestDetails == null) return;

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final request = state.requestDetails!;

      await requestRepository.updateRequest(
        requestId: request.id,
        title: request.title,
        description: request.description,
        mediaIds: request.mediaIds,
        locationId: locationId,
      );

      await loadRequestDetails();
    } catch (e, stackTrace) {
      _log.severe('❌ Error updating location: $e', e, stackTrace);
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Could not update location'),
      );
      rethrow;
    }
  }

  // ---------------------------------------------------------------------------
  // Fulfillment
  // ---------------------------------------------------------------------------

  
  
  /// loadCompletionSavings fetches and caches savings data for a fulfilled
  /// request when it is not already cached in state.
  Future<void> loadCompletionSavings() async {
    if (state.requestDetails == null) return;

    if (state.requestDetails!.state !=
        pb.RequestState.REQUEST_STATE_FULFILLED) {
      return;
    }

    if (state.completionSavings != null) return;

    try {
      final requestRepository = ref.read(requestRepositoryProvider);
      final statsResponse = await requestRepository.getStats(
        state.requestDetails!.id,
        communityId: state.communityId,
      );

      if (!ref.mounted) return;

      if (statsResponse.hasImpact()) {
        state = state.copyWith(completionSavings: statsResponse.impact);
      }
    } catch (e) {
      _log.warning('⚠️ Failed to load completion savings: $e');
    }
  }

  
  // ---------------------------------------------------------------------------
  // Offers
  // ---------------------------------------------------------------------------

  /// offerToFulfill creates an offer from the current user to fulfill the
  /// request and joins the conversation.
  Future<void> offerToFulfill() async {
    if (state.requestDetails == null) return;

    final communityId = state.communityId;
    if (communityId == null || communityId.isEmpty) {
      _log.severe('Cannot offer to fulfill: no community context');
      state = state.copyWith(
        error: const UserError.generic(
            fallback: 'Unable to offer help: no community selected'),
      );
      return;
    }

    _log.info(
      '🙋 Offering to fulfill request: ${state.requestDetails!.id} in community: $communityId',
    );

    state = state.copyWith(isSaving: true, error: null);

    try {
      final requestRepository = ref.read(requestRepositoryProvider);

      final updatedRequest = await requestRepository.offerToFulfill(
        requestId: state.requestDetails!.id,
        communityId: communityId,
      );

      state = state.copyWith(requestDetails: updatedRequest, isSaving: false);

      _log.info('✅ Offer created successfully');

      final requestRepository2 = ref.read(requestRepositoryProvider);
      await requestRepository2.invalidateStats(state.requestDetails!.id);
      unawaited(loadStats());
    } catch (e, stackTrace) {
      _log.severe('❌ Error offering to fulfill: $e', e, stackTrace);
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Could not create offer'),
        isSaving: false,
      );
      rethrow;
    }
  }

  /// withdrawOffer removes the current user's offer to fulfill the request.
  Future<void> withdrawOffer() async {
    if (state.requestDetails == null) return;

    final communityId = state.communityId;
    if (communityId == null || communityId.isEmpty) {
      _log.severe('Cannot withdraw offer: no community context');
      state = state.copyWith(
        error: const UserError.generic(
            fallback: 'Unable to withdraw offer: no community selected'),
      );
      return;
    }

    _log.info('🚫 Withdrawing offer for request: ${state.requestDetails!.id}');

    state = state.copyWith(isSaving: true, error: null);

    try {
      final requestRepository = ref.read(requestRepositoryProvider);

      await requestRepository.withdrawOffer(
        requestId: state.requestDetails!.id,
        communityId: communityId,
      );

      final updatedRequest = await requestRepository.refreshRequest(
        requestId: state.requestDetails!.id,
      );

      state = state.copyWith(requestDetails: updatedRequest, isSaving: false);

      _log.info('✅ Offer withdrawn successfully');

      await requestRepository.invalidateStats(state.requestDetails!.id);
      unawaited(loadStats());
    } catch (e, stackTrace) {
      _log.severe('❌ Error withdrawing offer: $e', e, stackTrace);
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Could not withdraw offer'),
        isSaving: false,
      );
      rethrow;
    }
  }

  // ---------------------------------------------------------------------------
  // Cancellation
  // ---------------------------------------------------------------------------

  /// cancelRequest cancels the request. Only the request owner can call this.
  /// Returns the community_event_id for undo wiring.
  Future<String?> cancelRequest() async {
    if (state.requestDetails == null) return null;

    _log.info('❌ Cancelling request: ${state.requestDetails!.id}');

    state = state.copyWith(isLoading: true, error: null);

    String? communityEventId;
    try {
      final requestRepository = ref.read(requestRepositoryProvider);

      communityEventId = await requestRepository.cancelRequest(
        requestId: state.requestDetails!.id,
      );

      await loadRequestDetails();

      _log.info('✅ Request cancelled successfully');
      return communityEventId;
    } catch (e, stackTrace) {
      _log.severe('❌ Error cancelling request: $e', e, stackTrace);
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Could not cancel request'),
        isLoading: false,
      );
      rethrow;
    }
  }

  // ---------------------------------------------------------------------------
  // Abstract methods that the coordinator must implement
  // ---------------------------------------------------------------------------

  /// loadRequestDetails reloads request details from the repository.
  Future<void> loadRequestDetails();

  /// refreshRequestDetails invalidates caches and reloads details + stats.
  Future<void> refreshRequestDetails();

  /// loadStats fetches impact statistics for the Details tab.
  Future<void> loadStats();
}
