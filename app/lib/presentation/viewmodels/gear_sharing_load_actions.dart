import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart'
    show ConversationItem;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferState;
import 'package:ripls/presentation/viewmodels/gear_sharing_state.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('GearSharingLoadActions');

/// GearSharingLoadMixin provides initialization and data-loading methods for
/// [GearSharingNotifier].
///
/// Covers the full initialize sequence plus individual reload helpers for
/// transfers and conversations.
mixin GearSharingLoadMixin on Notifier<GearSharingState> {

  /// initialize sets up the notifier with gear and user context and kicks off
  /// the initial data loads.
  Future<void> initialize({
    required GetGearResponse gear,
    required bool isOwner,
    required String communityId,
  }) async {
    // Get current user ID for role determination (may be null in tests)
    String? currentUserId;
    try {
      currentUserId = ref.read(authStateProvider).user?.id;
    } catch (_) {
      // Auth provider may not be available in tests
    }

    state = state.copyWith(
      gear: gear,
      isOwner: isOwner,
      communityId: communityId,
      currentUserId: currentUserId,
    );

    // Load all gear transfers for all users (needed for queue display)
    await loadAllGearTransfers();

    // Load additional data based on user role
    if (isOwner) {
      // For owners, receivedRequests is already populated by loadAllGearTransfers
      await loadTransferConversations();
    } else {
      // For non-owners, also load their specific extended requests
      await loadExtendedRequests();
      // Note: Giveaway summary is loaded on-demand by modal, not during init
    }
  }

  /// loadAllGearTransfers fetches every transfer for this gear so all users
  /// can see the full queue.
  Future<void> loadAllGearTransfers() async {
    final gear = state.gear;
    if (gear == null) return;

    _log.info('📥 Loading all transfers for gear: ${gear.id}');

    state = state.copyWith(isLoadingRequests: true, requestsError: null);

    try {
      final transferRepository = ref.read(transferRepositoryProvider);
      final transfers = await transferRepository.getGearTransfers(
        gearId: gear.id,
      );

      _log.info('✅ Loaded ${transfers.length} transfers for gear ${gear.id}');

      // For owners, receivedRequests and allGearTransfers are the same
      if (state.isOwner) {
        state = state.copyWith(
          receivedRequests: transfers,
          allGearTransfers: transfers,
          isLoadingRequests: false,
        );
      } else {
        state = state.copyWith(
          allGearTransfers: transfers,
          isLoadingRequests: false,
        );
      }
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException loading gear transfers: ${e.message}', e);
      state = state.copyWith(
        requestsError: RpcErrorHandler.classify(e),
        isLoadingRequests: false,
      );
    } catch (e, stackTrace) {
      _log.severe(
        '❌ Unexpected exception loading gear transfers: $e',
        e,
        stackTrace,
      );
      state = state.copyWith(
        requestsError: RpcErrorHandler.classify(e),
        isLoadingRequests: false,
      );
    }
  }

  /// loadReceivedRequests reloads all transfers for this gear.
  /// Called after actions to refresh the transfer list.
  Future<void> loadReceivedRequests() async {
    if (state.gear == null) return;

    _log.info('📥 Reloading transfers for gear: ${state.gear!.id}');

    state = state.copyWith(isLoadingRequests: true, requestsError: null);

    try {
      // Use getGearTransfers to get all transfers for this specific gear
      final transferRepository = ref.read(transferRepositoryProvider);
      final requests = await transferRepository.getGearTransfers(
        gearId: state.gear!.id,
      );

      _log.info(
        '✅ Loaded ${requests.length} transfers for gear ${state.gear!.id}',
      );

      // Update both receivedRequests (for owners) and allGearTransfers (for all)
      state = state.copyWith(
        receivedRequests: requests,
        allGearTransfers: requests,
        isLoadingRequests: false,
      );

      // After loading transfer requests, load conversations for each (owner only)
      if (state.isOwner) {
        await loadTransferConversations();
      }
    } on ServiceException catch (e) {
      _log.severe('❌ ServiceException loading transfers: ${e.message}', e);
      state = state.copyWith(
        requestsError: RpcErrorHandler.classify(e),
        isLoadingRequests: false,
      );
    } catch (e, stackTrace) {
      _log.severe(
        '❌ Unexpected exception loading transfers: $e',
        e,
        stackTrace,
      );
      state = state.copyWith(
        requestsError: RpcErrorHandler.classify(e),
        isLoadingRequests: false,
      );
    }
  }

  /// loadTransferConversations fetches conversation data for all transfer
  /// requests owned by this gear.
  ///
  /// Conversations are stored in the state's conversations map keyed by
  /// transfer ID. Only runs for owners.
  Future<void> loadTransferConversations() async {
    if (!state.isOwner) return;

    _log.info(
      '💬 Loading conversations for ${state.receivedRequests.length} transfer requests',
    );

    state = state.copyWith(isLoadingConversations: true);

    final conversations = <String, ConversationItem?>{};

    // Load conversations for each received request
    for (final transfer in state.receivedRequests) {
      try {
        final chatRepository = ref.read(chatRepositoryProvider);
        final conversation = await chatRepository.getConversationForTransfer(
          transferId: transfer.id,
        );
        conversations[transfer.id] = conversation;
        _log.fine('✅ Loaded conversation for transfer ${transfer.id}');
      } catch (e) {
        // If a conversation fails to load, set it as null
        conversations[transfer.id] = null;
        _log.warning(
          '⚠️ Failed to load conversation for transfer ${transfer.id}: $e',
        );
      }
    }

    _log.info('✅ Loaded ${conversations.length} conversations');

    state = state.copyWith(
      conversations: conversations,
      isLoadingConversations: false,
    );
  }

  /// loadExtendedRequests fetches loan requests extended by the borrower for
  /// this specific gear.
  Future<void> loadExtendedRequests() async {
    if (state.isOwner || state.gear == null) return;

    _log.info(
      '📥 Loading loan requests extended by borrower for gear: ${state.gear!.id}',
    );

    state = state.copyWith(isLoadingRequests: true, requestsError: null);

    try {
      // Use repository for cached transfer requests
      final transferRepository = ref.read(transferRepositoryProvider);
      final requests = await transferRepository.listReceivedTransfers();

      // Filter to only requests for this specific gear
      final gearId = state.gear?.id ?? '';
      final gearRequests = requests.where((r) => r.gearId == gearId).toList();

      _log.info(
        '✅ Loaded ${gearRequests.length} extended requests for this gear',
      );

      // Check if there's a request for this gear
      String? transferId;
      TransferState? transferState;
      if (gearRequests.isNotEmpty) {
        // Store the first request for this gear
        final req = gearRequests.first;
        transferId = req.id;
        transferState = req.state;
        _log.info(
          '✅ Found existing request for this gear: $transferId (state: $transferState)',
        );
      }

      state = state.copyWith(
        extendedRequests: gearRequests,
        submittedTransferId: transferId,
        submittedTransferState: transferState,
        isLoadingRequests: false,
      );
    } on ServiceException catch (e) {
      _log.severe(
        '❌ ServiceException loading extended requests: ${e.message}',
        e,
      );
      state = state.copyWith(
        requestsError: RpcErrorHandler.classify(e),
        isLoadingRequests: false,
      );
    } catch (e, stackTrace) {
      _log.severe(
        '❌ Unexpected exception loading extended requests: $e',
        e,
        stackTrace,
      );
      state = state.copyWith(
        requestsError: RpcErrorHandler.classify(e),
        isLoadingRequests: false,
      );
    }
  }
}
