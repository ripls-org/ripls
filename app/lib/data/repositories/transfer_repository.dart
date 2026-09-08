import 'package:flutter/foundation.dart' show VoidCallback;
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show Transfer, TransferType, TransferState, GearTransferContext;
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart' show GetUserTransferStatusResponse, CompleteTransferResponse, CreateTransferResponse;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/services/transfer_service.dart';

/// Repository for transfer data (loans and giveaways) with transparent caching.
///
/// This repository wraps TransferService and provides caching for transfer
/// requests using the global TTL from environment (CACHE_TTL_MINUTES).
///
/// **Cache Keys:**
/// - `'my:*'` - User's transfers as sharer (list operations)
/// - `'received:*'` - User's transfers as recipient (list operations)
/// - `'community:{communityId}:*'` - Community transfers (list operations)
/// - `'{transferId}'` - Single transfer (item operations)
///
/// **Invalidation Strategy:**
/// - List keys: Pattern-based invalidation (`'my:*'`, `'received:*'`)
/// - Item keys: Specific transfer ID invalidated after mutations
/// - All mutations invalidate both list patterns AND the specific item key
///
/// **Chat Integration:**
/// - Single-item cache enables efficient real-time updates in conversations
/// - Push notifications trigger `getTransfer()` to refetch single item
/// - Avoids refetching entire lists when one transfer updates
/// - See [ConversationViewModel] for usage in chat contexts
class TransferRepository {
  final CacheService _cache;
  final TransferService _service;
  final ChatRepository _chatRepository;
  final GearRepository _gearRepository;
  final CommunityRepository _communityRepository;
  final UserRepository _userRepository;
  final VoidCallback? _onDailyInvalidated;

  TransferRepository(
    CacheManager cacheManager,
    this._service,
    this._chatRepository,
    this._gearRepository,
    this._communityRepository,
    this._userRepository, {
    VoidCallback? onDailyInvalidated,
  })  : _cache = CacheService(cacheManager, 'transfer'),
        _onDailyInvalidated = onDailyInvalidated;

  /// Gets a single transfer by ID with caching.
  ///
  /// Cache key: 'transfer:{transferId}'
  /// Use [refreshTransfer] to force a refresh.
  Future<Transfer?> getTransfer(String transferId) async {
    return _cache.get(
      key: transferId,
      fetch: () => _service.getTransfer(transferId: transferId),
    );
  }

  /// Lists transfers made by the current user (as sharer).
  ///
  /// Optionally filter by [transferType] and [state].
  /// Use [refreshMyTransfers] to force a refresh.
  Future<List<Transfer>> listMyTransfers({
    TransferType? transferType,
    TransferState? state,
  }) async {
    final listKey = _buildListKey('my', transferType, state);

    return _cache.getList(
      listKey: listKey,
      fetch: () => _service.listMyTransfers(
        transferType: transferType,
        state: state,
      ),
    );
  }

  /// Lists transfers received by the current user (as recipient).
  ///
  /// Optionally filter by [transferType] and [state].
  /// Use [refreshReceivedTransfers] to force a refresh.
  Future<List<Transfer>> listReceivedTransfers({
    TransferType? transferType,
    TransferState? state,
  }) async {
    final listKey = _buildListKey('received', transferType, state);

    return _cache.getList(
      listKey: listKey,
      fetch: () => _service.listReceivedTransfers(
        transferType: transferType,
        state: state,
      ),
    );
  }

  /// Lists available transfers in a community.
  ///
  /// Optionally filter by [transferType] and [state].
  /// Use [refreshTransfers] to force a refresh.
  Future<List<Transfer>> listTransfers({
    required String communityId,
    TransferType? transferType,
    TransferState? state,
  }) async {
    final listKey = _buildListKey('community:$communityId', transferType, state);

    return _cache.getList(
      listKey: listKey,
      fetch: () => _service.listTransfers(
        communityId: communityId,
        transferType: transferType,
        state: state,
      ),
    );
  }

  /// Refreshes the my transfers cache by invalidating it.
  ///
  /// Call this after selecting recipients, canceling, or otherwise modifying transfers
  /// to ensure the next fetch gets fresh data.
  Future<List<Transfer>> refreshMyTransfers({
    TransferType? transferType,
    TransferState? state,
  }) async {
    await _cache.invalidatePattern('my:*');
    return listMyTransfers(transferType: transferType, state: state);
  }

  /// Refreshes the received transfers cache by invalidating it.
  ///
  /// Call this after expressing interest, canceling, or otherwise modifying transfers
  /// to ensure the next fetch gets fresh data.
  Future<List<Transfer>> refreshReceivedTransfers({
    TransferType? transferType,
    TransferState? state,
  }) async {
    await _cache.invalidatePattern('received:*');
    return listReceivedTransfers(transferType: transferType, state: state);
  }

  /// Refreshes the community transfers cache by invalidating it.
  ///
  /// Call this after any transfer modifications to ensure the next fetch gets fresh data.
  Future<List<Transfer>> refreshTransfers({
    required String communityId,
    TransferType? transferType,
    TransferState? state,
  }) async {
    await _cache.invalidatePattern('community:$communityId:*');
    return listTransfers(
      communityId: communityId,
      transferType: transferType,
      state: state,
    );
  }

  /// Expresses interest in a transfer (loan or giveaway).
  ///
  /// Returns the full Transfer object with all context including conversation ID.
  /// Automatically invalidates relevant caches.
  Future<Transfer> expressInterest({
    required String gearId,
  }) async {
    final transfer = await _service.expressInterest(
      gearId: gearId,
    );

    // Invalidate received transfers cache since we just expressed interest
    await _cache.invalidatePattern('received:*');
    // Invalidate owner's transfers cache since a new transfer was created on their gear
    await _cache.invalidatePattern('my:*');
    // Invalidate conversations cache since a new conversation was created or joined
    await _chatRepository.refreshConversations();
    // Invalidate conversation context cache so action buttons update
    if (transfer.hasConversationId()) {
      await _chatRepository.refreshConversationContext(transfer.conversationId);
    }
    // Invalidate gear request count cache since the count changed
    await _gearRepository.invalidateRequestCount(gearId);
    // Invalidate new transfer caches
    await invalidateGearTransfers(gearId);
    await invalidateUserTransferStatus(gearId);

    _onDailyInvalidated?.call();
    return transfer;
  }

  /// Selects a recipient from interested users (sharer only).
  ///
  /// Returns a record of (conversationId, communityEventId).
  /// communityEventId is what the caller passes to
  /// TransferService.undoSelectRecipient for snackbar undo.
  /// Automatically invalidates relevant caches.
  Future<({String conversationId, String communityEventId})> selectRecipient({
    required String transferId,
    required String recipientId,
  }) async {
    // Fetch transfer to get gearId for cache invalidation
    final transfer = await getTransfer(transferId);

    final result = await _service.selectRecipient(
      transferId: transferId,
      recipientId: recipientId,
    );

    // Invalidate my transfers cache since we just selected a recipient
    await _cache.invalidatePattern('my:*');
    // Invalidate the specific transfer item cache
    await _cache.invalidate(transferId);
    // Invalidate conversations cache since a new 1:1 conversation was created
    await _chatRepository.refreshConversations();
    // Invalidate gear request count cache since the count changed
    if (transfer != null) {
      await _gearRepository.invalidateRequestCount(transfer.gearId);
      // Invalidate new transfer caches
      await invalidateGearTransfers(transfer.gearId);
      await invalidateUserTransferStatus(transfer.gearId);
    }

    _onDailyInvalidated?.call();
    return result;
  }

  /// Reverses a prior [selectRecipient] using the community_event_id returned
  /// by it. Invalidates the same caches (and fires the daily/home invalidation)
  /// so the reverted state — including the re-surfaced Needs-you decision —
  /// reflects everywhere, not just on the screen that triggered the undo.
  Future<void> undoSelectRecipient({required String communityEventId}) async {
    await _service.undoSelectRecipient(communityEventId: communityEventId);
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('received:*');
    await _chatRepository.refreshConversations();
    _onDailyInvalidated?.call();
  }

  /// Cancels a transfer. Returns the community_event_id for undo wiring.
  ///
  /// Automatically invalidates relevant caches.
  Future<String> cancelTransfer({required String transferId}) async {
    // Fetch transfer to get gearId for cache invalidation
    final transfer = await getTransfer(transferId);

    final communityEventId =
        await _service.cancelTransfer(transferId: transferId);

    // Invalidate all caches since either side could cancel
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('received:*');
    await _cache.invalidatePattern('community:*');
    // Invalidate the specific transfer item cache
    await _cache.invalidate(transferId);
    // Invalidate gear request count cache since the count changed
    if (transfer != null) {
      await _gearRepository.invalidateRequestCount(transfer.gearId);
      // Invalidate new transfer caches
      await invalidateGearTransfers(transfer.gearId);
      await invalidateUserTransferStatus(transfer.gearId);

      // If was ACTIVE, invalidate gear cache so activeLoan is cleared
      if (transfer.state == TransferState.TRANSFER_STATE_ACTIVE) {
        await _gearRepository.invalidate(transfer.gearId);
      }
    }
    _onDailyInvalidated?.call();
    return communityEventId;
  }

  /// Starts a loan (doesn't apply to giveaways). Returns the
  /// community_event_id for undo wiring.
  ///
  /// Automatically invalidates relevant caches.
  Future<String> startLoan({required String transferId}) async {
    // Fetch transfer to get gearId for cache invalidation
    final transfer = await getTransfer(transferId);

    final communityEventId = await _service.startLoan(transferId: transferId);

    // Invalidate both caches since status changed
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('received:*');
    // Invalidate the specific transfer item cache
    await _cache.invalidate(transferId);
    // Invalidate gear-specific caches
    if (transfer != null) {
      await invalidateGearTransfers(transfer.gearId);
      await invalidateUserTransferStatus(transfer.gearId);

      // Invalidate gear cache and stats so impact data is refreshed
      await _gearRepository.invalidate(transfer.gearId);
      await _gearRepository.invalidateStats(transfer.gearId);

      // Invalidate user stats for both parties (IE is persisted at loan start)
      if (transfer.hasOwner() && transfer.owner.id.isNotEmpty) {
        await _userRepository.invalidateStats(transfer.owner.id);
      }
      if (transfer.hasRecipient() && transfer.recipient.id.isNotEmpty) {
        await _userRepository.invalidateStats(transfer.recipient.id);
      }
    }
    _onDailyInvalidated?.call();
    return communityEventId;
  }

  /// Completes a transfer (loan returned or giveaway ownership transferred).
  ///
  /// Returns the completion response including savings metrics.
  /// Automatically invalidates relevant caches.
  Future<CompleteTransferResponse> completeTransfer({required String transferId}) async {
    // Fetch transfer to get gearId for cache invalidation
    final transfer = await getTransfer(transferId);

    final response = await _service.completeTransfer(transferId: transferId);

    // Invalidate both caches since status changed
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('received:*');
    // Invalidate the specific transfer item cache
    await _cache.invalidate(transferId);
    // Invalidate gear-specific caches
    if (transfer != null) {
      await invalidateGearTransfers(transfer.gearId);
      await invalidateUserTransferStatus(transfer.gearId);

      // Invalidate gear cache and stats so impact/timesLoaned are refreshed
      await _gearRepository.invalidate(transfer.gearId);
      await _gearRepository.invalidateStats(transfer.gearId);

      // Invalidate user stats for both owner and recipient so profile
      // savings reflect the newly persisted ImpactEstimate.
      if (transfer.hasOwner() && transfer.owner.id.isNotEmpty) {
        await _userRepository.invalidateStats(transfer.owner.id);
      }
      if (transfer.hasRecipient() && transfer.recipient.id.isNotEmpty) {
        await _userRepository.invalidateStats(transfer.recipient.id);
      }

      // If this is a giveaway, invalidate completed giveaways cache for all communities
      // (gear may have been shared in multiple communities) and refresh the user
      // gear list so the given-away item is not retained until the global TTL.
      if (transfer.transferType == TransferType.TRANSFER_TYPE_GIVEAWAY) {
        await _communityRepository.invalidateCompletedGiveawaysAll();
        await _gearRepository.refreshUserGear();
      }
    }

    _onDailyInvalidated?.call();
    return response;
  }

  /// Reverses a prior [startLoan] using its community_event_id. Mirrors the
  /// forward invalidations (and fires the daily/home invalidation) so the
  /// reverted ACTIVE→RECIPIENT_SELECTED state — including the re-surfaced
  /// "Mark picked up" Needs-you decision — reflects everywhere.
  Future<void> undoStartLoan({required String communityEventId}) async {
    await _service.undoStartLoan(communityEventId: communityEventId);
    await _invalidateAfterTransferUndo();
  }

  /// Reverses a prior loan [completeTransfer] using its community_event_id, so
  /// the loan returns to ACTIVE and its "Mark returned" decision re-surfaces.
  Future<void> undoCompleteLoan({required String communityEventId}) async {
    await _service.undoCompleteLoan(communityEventId: communityEventId);
    await _invalidateAfterTransferUndo();
  }

  /// Reverses a prior giveaway [completeTransfer] using its community_event_id,
  /// so the giveaway returns to RECIPIENT_SELECTED and its "Mark picked up"
  /// decision re-surfaces.
  Future<void> undoCompleteGiveaway({required String communityEventId}) async {
    await _service.undoCompleteGiveaway(communityEventId: communityEventId);
    await _invalidateAfterTransferUndo();
  }

  /// Shared cache invalidation for the transfer-undo wrappers. The undo only
  /// carries a community_event_id (no gearId), so this clears the transfer and
  /// gear caches broadly and fires the home/daily invalidation that re-surfaces
  /// the reverted decision.
  Future<void> _invalidateAfterTransferUndo() async {
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('received:*');
    await _gearRepository.invalidateAll();
    _onDailyInvalidated?.call();
  }

  /// Updates a transfer.
  ///
  /// Can update transfer type (in INTEREST_EXPRESSED state) or
  /// estimated pickup time and loan duration (in RECIPIENT_SELECTED state).
  /// Automatically invalidates relevant caches.
  Future<void> updateTransfer({
    required String transferId,
    TransferType? transferType,
    int? estimatedPickupUnixSec,
    int? loanDurationDays,
  }) async {
    // Fetch transfer to get gearId for cache invalidation
    final transfer = await getTransfer(transferId);

    await _service.updateTransfer(
      transferId: transferId,
      transferType: transferType,
      estimatedPickupUnixSec: estimatedPickupUnixSec,
      loanDurationDays: loanDurationDays,
    );

    // Invalidate all caches since the transfer was updated
    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('received:*');
    await _cache.invalidatePattern('community:*');
    // Invalidate the specific transfer item cache
    await _cache.invalidate(transferId);
    // Invalidate gear-specific caches
    if (transfer != null) {
      await invalidateGearTransfers(transfer.gearId);
      await invalidateUserTransferStatus(transfer.gearId);
    }
    _onDailyInvalidated?.call();
  }

  /// Gets all active transfers for a gear item.
  ///
  /// Returns a list of all non-archived transfers for the specified gear.
  /// Uses short-lived cache (global TTL). Call [invalidateGearTransfers] after
  /// any transfer mutations to ensure fresh data.
  Future<List<Transfer>> getGearTransfers({required String gearId}) async {
    return _cache.getList(
      listKey: 'gear:$gearId',
      fetch: () => _service.getGearTransfers(gearId: gearId),
    );
  }

  /// Gets the current user's transfer status for a gear item.
  ///
  /// Returns the user's active transfer (if any) and whether they have one.
  /// Uses short-lived cache (global TTL). Call [invalidateUserTransferStatus] after
  /// any transfer mutations to ensure fresh data.
  Future<GetUserTransferStatusResponse> getUserTransferStatus({
    required String gearId,
  }) async {
    return _cache.get(
      key: 'user_status:$gearId',
      fetch: () => _service.getUserTransferStatus(gearId: gearId),
    );
  }

  /// Invalidates the gear transfers cache for a specific gear item.
  ///
  /// Call this after any transfer mutations (express interest, approve,
  /// decline, cancel) to ensure the next fetch gets fresh data.
  Future<void> invalidateGearTransfers(String gearId) async {
    await _cache.invalidate('gear:$gearId');
  }

  /// Invalidates the user transfer status cache for a specific gear item.
  ///
  /// Call this after any transfer mutations to ensure the next fetch gets fresh data.
  Future<void> invalidateUserTransferStatus(String gearId) async {
    await _cache.invalidate('user_status:$gearId');
  }

  /// Gets transfer context for a gear item (includes user's transfer, available actions, and pending requests).
  ///
  /// Returns the full context needed for the transfer management modal.
  /// **NOT CACHED** because this contains user-specific data (ownership, available actions).
  /// Each user gets fresh data on every call to ensure correct permissions and state.
  Future<GearTransferContext> getGearTransferContext({
    required String gearId,
    required String communityId,
  }) async {
    // NO CACHING - this is user-specific data that must be fresh
    final response = await _service.getGearTransferContext(
      gearId: gearId,
      communityId: communityId,
    );
    return response.context;
  }

  /// Refreshes the gear transfer context cache by invalidating it.
  ///
  /// **DEPRECATED**: This method is now a no-op since getGearTransferContext
  /// no longer uses caching (user-specific data must always be fresh).
  /// Kept for backward compatibility.
  Future<void> refreshGearTransferContext(String gearId) async {
    // No-op: getGearTransferContext is no longer cached
  }

  /// Invalidates a specific transfer's cache.
  ///
  /// Call this when a transfer has been modified to ensure fresh data on next fetch.
  Future<void> invalidate(String transferId) async {
    await _cache.invalidate(transferId);
  }

  /// Invalidates all cached transfer data.
  Future<void> invalidateAll() async {
    await _cache.invalidateAll();
  }

  /// Invalidates every cached entry scoped to a single community.
  ///
  /// Use this after a community-level lifecycle change (e.g. restoring
  /// a previously soft-deleted community) so the next read fetches
  /// the freshly un-soft-deleted transfers rather than the cached
  /// "no transfers visible" snapshot from the deleted period.
  Future<void> invalidateForCommunity(String communityId) async {
    await _cache.invalidatePattern('community:$communityId:*');
  }

  /// Creates a past-tense transfer directly in the target state.
  ///
  /// Bypasses the interest/selection state machine. Invalidates my-transfers
  /// and gear caches so the gear detail screen reflects the new transfer state.
  Future<CreateTransferResponse> createTransfer({
    required String gearId,
    required String communityId,
    required TransferType transferType,
    required int completedAtUnixSec,
    String? recipientUserId,
    String? provisionalUserId,
    int? returnedAtUnixSec,
  }) async {
    final response = await _service.createTransfer(
      gearId: gearId,
      communityId: communityId,
      transferType: transferType,
      completedAtUnixSec: completedAtUnixSec,
      recipientUserId: recipientUserId,
      provisionalUserId: provisionalUserId,
      returnedAtUnixSec: returnedAtUnixSec,
    );

    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('community:$communityId:*');
    await invalidateGearTransfers(gearId);
    await invalidateUserTransferStatus(gearId);
    await _gearRepository.invalidate(gearId);
    await _gearRepository.invalidateStats(gearId);

    _onDailyInvalidated?.call();
    return response;
  }

  /// Offers the caller's gear toward a community request (#2702): creates a
  /// live transfer targeted at the requester (born in RECIPIENT_SELECTED) and
  /// shares the gear into the community with the availability matching
  /// [transferType]. Invalidates my-transfers, community, and gear caches so
  /// the gear and request surfaces reflect the new offer.
  Future<Transfer> offerTransfer({
    required String gearId,
    required TransferType transferType,
    required String recipientUserId,
    required String communityId,
    required String originRequestId,
    required String contributionId,
    int? loanDurationDays,
  }) async {
    final transfer = await _service.offerTransfer(
      gearId: gearId,
      transferType: transferType,
      recipientUserId: recipientUserId,
      communityId: communityId,
      originRequestId: originRequestId,
      contributionId: contributionId,
      loanDurationDays: loanDurationDays,
    );

    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('community:$communityId:*');
    await invalidateGearTransfers(gearId);
    await invalidateUserTransferStatus(gearId);
    await _gearRepository.invalidate(gearId);

    _onDailyInvalidated?.call();
    return transfer;
  }

  /// Brings the caller's gear to an event (#2708): creates a live transfer to
  /// the event's host for the event's duration (born in RECIPIENT_SELECTED) and
  /// shares the gear into the community with the availability matching
  /// [transferType]. The recipient (host) is derived server-side. Invalidates
  /// my-transfers, community, and gear caches so the surfaces reflect the offer.
  Future<Transfer> offerExperienceTransfer({
    required String gearId,
    required TransferType transferType,
    required String communityId,
    required String originExperienceId,
    required String contributionId,
    int? loanDurationDays,
  }) async {
    final transfer = await _service.offerExperienceTransfer(
      gearId: gearId,
      transferType: transferType,
      communityId: communityId,
      originExperienceId: originExperienceId,
      contributionId: contributionId,
      loanDurationDays: loanDurationDays,
    );

    await _cache.invalidatePattern('my:*');
    await _cache.invalidatePattern('community:$communityId:*');
    await invalidateGearTransfers(gearId);
    await invalidateUserTransferStatus(gearId);
    await _gearRepository.invalidate(gearId);

    _onDailyInvalidated?.call();
    return transfer;
  }

  /// Withdraws the user's interest in a transfer.
  ///
  /// For loans: archives the user's individual transfer request.
  /// For giveaways: removes user from group conversation.
  /// Automatically invalidates relevant caches.
  Future<void> withdrawInterest({required String transferId}) async {
    // Fetch transfer to get gearId for cache invalidation
    final transfer = await getTransfer(transferId);

    await _service.withdrawInterest(transferId: transferId);

    // Invalidate relevant caches
    await _cache.invalidatePattern('received:*');
    await _cache.invalidate(transferId);
    await _chatRepository.refreshConversations();

    if (transfer != null) {
      // Invalidate conversation context cache so action buttons update
      if (transfer.hasConversationId()) {
        await _chatRepository.refreshConversationContext(transfer.conversationId);
      }
      await _gearRepository.invalidateRequestCount(transfer.gearId);
      await invalidateGearTransfers(transfer.gearId);
      await invalidateUserTransferStatus(transfer.gearId);
    }
    _onDailyInvalidated?.call();
  }

  // Helper method to build consistent cache keys
  String _buildListKey(String prefix, TransferType? transferType, TransferState? state) {
    final parts = [prefix];

    if (transferType != null && transferType != TransferType.TRANSFER_TYPE_UNSPECIFIED) {
      parts.add(transferType.name);
    }

    if (state != null && state != TransferState.TRANSFER_STATE_UNSPECIFIED) {
      parts.add(state.name);
    }

    return parts.join(':');
  }
}
