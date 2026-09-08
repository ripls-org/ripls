import 'package:connectrpc/connect.dart' as connect;
import 'package:fixnum/fixnum.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show Transfer, TransferType, TransferState;
import 'package:ripls/data/gen/ripls/api/transfer_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;

final _log = ObservableLogger.named('TransferService');

/// TransferService handles transfer-related operations (loans and giveaways) using the TransferService API.
class TransferService {
  final TransferServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  TransferService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  }) : _client = TransferServiceClient(transport),
       _getAccessToken = getAccessToken,
       _errorHandler = errorHandler ?? RpcErrorHandler(),
       _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// ExpressInterest expresses interest in a transfer (loan or giveaway).
  ///
  /// Returns the full Transfer object with all context including conversation ID.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Transfer> expressInterest({required String gearId}) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Expressing interest in gear: $gearId');

        final request = ExpressInterestRequest(gearId: gearId);

        _log.info('🌐 Sending ExpressInterest RPC...');
        final response = await _client.expressInterest(
          request,
          headers: _buildHeaders(),
        );

        _log.info(
          '✅ Interest expressed! Transfer ID: ${response.transfer.id}, Conversation ID: ${response.transfer.conversationId}',
        );
        return response.transfer;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ExpressInterest',
    );
  }

  /// SelectRecipient selects a recipient from interested users (sharer only).
  ///
  /// Returns a record of (conversationId, communityEventId). The
  /// community_event_id is what the caller passes to
  /// [undoSelectRecipient] to wire an undo snackbar.
  Future<({String conversationId, String communityEventId})> selectRecipient({
    required String transferId,
    required String recipientId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info(
          '📤 Selecting recipient $recipientId for transfer: $transferId',
        );

        final request = SelectRecipientRequest(
          transferId: transferId,
          recipientId: recipientId,
        );

        _log.info('🌐 Sending SelectRecipient RPC...');
        final response = await _client.selectRecipient(
          request,
          headers: _buildHeaders(),
        );

        _log.info(
          '✅ Recipient selected! Conversation ID: ${response.conversationId}',
        );
        return (
          conversationId: response.conversationId,
          communityEventId: response.communityEventId,
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SelectRecipient',
    );
  }

  /// CancelTransfer cancels a transfer. Returns the community_event_id
  /// of the cancel so the caller can wire an undo snackbar.
  Future<String> cancelTransfer({required String transferId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CancelTransferRequest(transferId: transferId);
        final response =
            await _client.cancelTransfer(request, headers: _buildHeaders());
        return response.communityEventId;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CancelTransfer',
    );
  }

  /// StartLoan starts an approved loan. Returns the community_event_id
  /// of the action so the caller can wire an undo snackbar.
  Future<String> startLoan({required String transferId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = StartLoanRequest(transferId: transferId);
        final response =
            await _client.startLoan(request, headers: _buildHeaders());
        return response.communityEventId;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'StartLoan',
    );
  }

  /// CompleteTransfer completes a transfer (loan returned or giveaway ownership transferred).
  ///
  /// Returns the completion response including savings metrics.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<CompleteTransferResponse> completeTransfer({required String transferId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = CompleteTransferRequest(transferId: transferId);

        return _client.completeTransfer(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CompleteTransfer',
    );
  }

  /// Undo a prior SelectRecipient call. Throws [ServiceException] (with
  /// an attached UndoErrorDetail on the underlying ConnectException)
  /// when the action cannot be reversed.
  Future<void> undoSelectRecipient({required String communityEventId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UndoSelectRecipientRequest(
          communityEventId: communityEventId,
        );
        await _client.undoSelectRecipient(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UndoSelectRecipient',
    );
  }

  /// Undo a prior StartLoan call.
  Future<void> undoStartLoan({required String communityEventId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UndoStartLoanRequest(communityEventId: communityEventId);
        await _client.undoStartLoan(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UndoStartLoan',
    );
  }

  /// Undo a prior CompleteTransfer call on a loan.
  Future<void> undoCompleteLoan({required String communityEventId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request =
            UndoCompleteLoanRequest(communityEventId: communityEventId);
        await _client.undoCompleteLoan(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UndoCompleteLoan',
    );
  }

  /// Undo a prior CompleteTransfer call on a giveaway.
  Future<void> undoCompleteGiveaway({required String communityEventId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request =
            UndoCompleteGiveawayRequest(communityEventId: communityEventId);
        await _client.undoCompleteGiveaway(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UndoCompleteGiveaway',
    );
  }

  /// Undo a prior CancelTransfer call.
  Future<void> undoCancelTransfer({required String communityEventId}) async {
    return RpcUtils.executeRpc(
      () async {
        final request =
            UndoCancelTransferRequest(communityEventId: communityEventId);
        await _client.undoCancelTransfer(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UndoCancelTransfer',
    );
  }

  /// ListMyTransfers lists transfers made by the current user (as sharer).
  ///
  /// Optionally filter by [transferType] and [state].
  /// Returns a list of transfer items.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<Transfer>> listMyTransfers({
    TransferType? transferType,
    TransferState? state,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info(
          '📤 Listing my transfers${transferType != null ? ' (type: $transferType)' : ''}',
        );

        final request = ListMyTransfersRequest(
          transferType: transferType ?? TransferType.TRANSFER_TYPE_UNSPECIFIED,
          state: state ?? TransferState.TRANSFER_STATE_UNSPECIFIED,
        );

        _log.info('🌐 Sending ListMyTransfers RPC...');
        final response = await _client.listMyTransfers(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Received ${response.transfers.length} transfers');
        return response.transfers;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListMyTransfers',
    );
  }

  /// ListReceivedTransfers lists transfers received by the current user (as recipient).
  ///
  /// Optionally filter by [transferType] and [state].
  /// Returns a list of transfer items.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<Transfer>> listReceivedTransfers({
    TransferType? transferType,
    TransferState? state,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info(
          '📤 Listing received transfers${transferType != null ? ' (type: $transferType)' : ''}',
        );

        final request = ListReceivedTransfersRequest(
          transferType: transferType ?? TransferType.TRANSFER_TYPE_UNSPECIFIED,
          state: state ?? TransferState.TRANSFER_STATE_UNSPECIFIED,
        );

        _log.info('🌐 Sending ListReceivedTransfers RPC...');
        final response = await _client.listReceivedTransfers(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Received ${response.transfers.length} transfers');
        return response.transfers;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListReceivedTransfers',
    );
  }

  /// GetTransfer retrieves a single transfer by ID.
  ///
  /// Returns the transfer with full details including owner, recipient, and gear info.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Transfer> getTransfer({required String transferId}) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Fetching transfer: $transferId');

        final request = GetTransferRequest(transferId: transferId);

        _log.info('🌐 Sending GetTransfer RPC...');
        final response = await _client.getTransfer(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Transfer fetched: ${response.transfer.id}');
        return response.transfer;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetTransfer',
    );
  }

  /// UpdateTransfer updates a transfer.
  ///
  /// Can update transfer type (in INTEREST_EXPRESSED state) or
  /// estimated pickup time and loan duration (in RECIPIENT_SELECTED state).
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> updateTransfer({
    required String transferId,
    TransferType? transferType,
    int? estimatedPickupUnixSec,
    int? loanDurationDays,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UpdateTransferRequest(transferId: transferId);

        if (transferType != null) {
          request.transferType = transferType;
        }
        if (estimatedPickupUnixSec != null) {
          request.estimatedPickupUnixSec = Int64(estimatedPickupUnixSec);
        }
        if (loanDurationDays != null) {
          request.loanDurationDays = loanDurationDays;
        }

        await _client.updateTransfer(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UpdateTransfer',
    );
  }

  /// ListTransfers lists available transfers in a community.
  ///
  /// Optionally filter by [transferType] and [state].
  /// Returns a list of transfer items.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<Transfer>> listTransfers({
    required String communityId,
    TransferType? transferType,
    TransferState? state,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info(
          '📤 Listing transfers for community: $communityId${transferType != null ? ' (type: $transferType)' : ''}',
        );

        final request = ListTransfersRequest(
          communityId: communityId,
          transferType: transferType ?? TransferType.TRANSFER_TYPE_UNSPECIFIED,
          state: state ?? TransferState.TRANSFER_STATE_UNSPECIFIED,
        );

        _log.info('🌐 Sending ListTransfers RPC...');
        final response = await _client.listTransfers(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Received ${response.transfers.length} transfers');
        return response.transfers;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'ListTransfers',
    );
  }

  /// GetGearTransfers retrieves all active transfers for a gear item.
  ///
  /// Returns a list of all non-archived transfers for the specified gear.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<Transfer>> getGearTransfers({required String gearId}) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Fetching transfers for gear: $gearId');

        final request = GetGearTransfersRequest(gearId: gearId);

        _log.info('🌐 Sending GetGearTransfers RPC...');
        final response = await _client.getGearTransfers(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Received ${response.transfers.length} transfers');
        return response.transfers;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetGearTransfers',
    );
  }

  /// GetUserTransferStatus checks the current user's transfer status for a gear item.
  ///
  /// Returns the user's active transfer (if any) and a boolean indicating if they have one.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetUserTransferStatusResponse> getUserTransferStatus({
    required String gearId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Checking transfer status for gear: $gearId');

        final request = GetUserTransferStatusRequest(gearId: gearId);

        _log.info('🌐 Sending GetUserTransferStatus RPC...');
        final response = await _client.getUserTransferStatus(
          request,
          headers: _buildHeaders(),
        );

        _log.info(
          '✅ Transfer status: ${response.hasActiveTransfer() ? "Has active transfer (${response.activeTransfer.id})" : "No active transfer"}',
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetUserTransferStatus',
    );
  }

  /// GetGearTransferContext retrieves transfer context for a gear item.
  ///
  /// Returns user's transfer (if any), available actions, and pending requests.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<GetGearTransferContextResponse> getGearTransferContext({
    required String gearId,
    required String communityId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Getting transfer context for gear: $gearId');

        final request = GetGearTransferContextRequest(
          gearId: gearId,
          communityId: communityId,
        );

        _log.info('🌐 Sending GetGearTransferContext RPC...');
        final response = await _client.getGearTransferContext(
          request,
          headers: _buildHeaders(),
        );

        _log.info(
          '✅ Retrieved transfer context: ${response.context.pendingRequests.length} pending requests, ${response.context.availableActions.length} available actions',
        );
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetGearTransferContext',
    );
  }

  /// CreateTransfer creates a past-tense transfer directly in the target state.
  ///
  /// Bypasses the interest/selection state machine when completedAtUnixSec is
  /// in the past. Exactly one of recipientUserId and provisionalUserId must be set.
  /// For loans, returnedAtUnixSec may be provided to mark the loan as already returned.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<CreateTransferResponse> createTransfer({
    required String gearId,
    required String communityId,
    required TransferType transferType,
    required int completedAtUnixSec,
    String? recipientUserId,
    String? provisionalUserId,
    int? returnedAtUnixSec,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Creating past transfer for gear: $gearId');

        final request = CreateTransferRequest(
          gearId: gearId,
          communityId: communityId,
          transferType: transferType,
          completedAtUnixSec: Int64(completedAtUnixSec),
        );
        if (recipientUserId != null) {
          request.recipientUserId = recipientUserId;
        }
        if (provisionalUserId != null) {
          request.provisionalUserId = provisionalUserId;
        }
        if (returnedAtUnixSec != null) {
          request.returnedAtUnixSec = Int64(returnedAtUnixSec);
        }

        _log.info('🌐 Sending CreateTransfer RPC...');
        final response = await _client.createTransfer(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Transfer created: ${response.transfer.id}');
        return response;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'CreateTransfer',
    );
  }

  /// OfferTransfer offers the caller's gear toward a community request
  /// (#2702): creates a live transfer targeted at the requester (born in
  /// RECIPIENT_SELECTED) and shares the gear into the community with the
  /// availability matching [transferType].
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Transfer> offerTransfer({
    required String gearId,
    required TransferType transferType,
    required String recipientUserId,
    required String communityId,
    required String originRequestId,
    required String contributionId,
    int? loanDurationDays,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Offering gear $gearId toward request $originRequestId');

        final request = OfferTransferRequest(
          gearId: gearId,
          transferType: transferType,
          recipientUserId: recipientUserId,
          communityId: communityId,
          originRequestId: originRequestId,
          contributionId: contributionId,
        );
        if (loanDurationDays != null) {
          request.loanDurationDays = loanDurationDays;
        }

        final response = await _client.offerTransfer(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Gear offer created: ${response.transfer.id}');
        return response.transfer;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'OfferTransfer',
    );
  }

  /// OfferExperienceTransfer brings the caller's gear to an event (#2708):
  /// creates a live transfer to the event's host for the event's duration
  /// (born in RECIPIENT_SELECTED) and shares the gear into the community with
  /// the availability matching [transferType]. The recipient (host) is derived
  /// server-side and is not supplied by the caller.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<Transfer> offerExperienceTransfer({
    required String gearId,
    required TransferType transferType,
    required String communityId,
    required String originExperienceId,
    required String contributionId,
    int? loanDurationDays,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Bringing gear $gearId to event $originExperienceId');

        final request = OfferExperienceTransferRequest(
          gearId: gearId,
          transferType: transferType,
          communityId: communityId,
          originExperienceId: originExperienceId,
          contributionId: contributionId,
        );
        if (loanDurationDays != null) {
          request.loanDurationDays = loanDurationDays;
        }

        final response = await _client.offerExperienceTransfer(
          request,
          headers: _buildHeaders(),
        );

        _log.info('✅ Event gear offer created: ${response.transfer.id}');
        return response.transfer;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'OfferExperienceTransfer',
    );
  }

  /// WithdrawInterest withdraws the user's interest in a transfer.
  ///
  /// For loans: archives the user's individual transfer request.
  /// For giveaways: removes user from group conversation.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<void> withdrawInterest({required String transferId}) async {
    return RpcUtils.executeRpc(
      () async {
        _log.info('📤 Withdrawing interest from transfer: $transferId');

        final request = WithdrawInterestRequest(transferId: transferId);

        _log.info('🌐 Sending WithdrawInterest RPC...');
        await _client.withdrawInterest(request, headers: _buildHeaders());

        _log.info('✅ Interest withdrawn successfully');
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'WithdrawInterest',
    );
  }
}
