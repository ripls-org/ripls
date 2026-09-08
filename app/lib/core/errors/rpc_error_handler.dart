import 'dart:async';
import 'dart:io';

import 'package:connectrpc/connect.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/errors.pb.dart' as errors_pb;
import 'package:ripls/l10n/app_localizations.dart';

/// ServiceException is thrown when a service operation fails.
///
/// The message contains a user-friendly error description suitable for display.
/// When the failure originates from a ConnectRPC call, [code] preserves the
/// structured error code so callers can classify the failure without parsing
/// the message string.
class ServiceException implements Exception {
  final String message;
  final Code? code;

  ServiceException(this.message, {this.code});

  @override
  String toString() => message;
}

/// RpcErrorHandler maps ConnectRPC error codes to user-facing messages and
/// classifies errors for the global error zone.
///
/// Crash reporting is not done here. The zone handlers in `main.dart` decide
/// fatality with [isNetworkError] / [isExpectedRpcError] and hand the error to
/// the observability service; a second, instance-level reporting path used to
/// exist on this class but nothing ever invoked it.
class RpcErrorHandler {
  /// Converts transport-level exceptions to user-friendly error messages.
  ///
  /// Handles common network and I/O errors that occur before the RPC layer.
  /// Pass [l10n] when a BuildContext is available (preferred); when omitted
  /// the method falls back to English. Per the i18n architecture, callers
  /// that display these strings in a widget should resolve them via
  /// AppLocalizations.of(context) and pass it here.
  static String getTransportErrorMessage(Object error, {AppLocalizations? l10n}) {
    if (error is SocketException) {
      return l10n?.rpcErrorTransportSocket ??
          'Unable to connect to the server. Please check your internet connection and try again.';
    }
    if (error is TimeoutException) {
      return l10n?.rpcErrorTransportTimeout ??
          'The request timed out. Please check your internet connection and try again.';
    }
    if (error is HttpException) {
      return l10n?.rpcErrorTransportHttp ?? 'Server error. Please try again later.';
    }
    return l10n?.rpcErrorGeneric ?? 'An unexpected error occurred. Please try again.';
  }

  /// Checks if an error is a network/connection error that the app can recover from.
  ///
  /// These errors are typically transient and don't indicate a bug in the app code.
  /// Use this to determine if errors should be logged as fatal or non-fatal in
  /// crash reporting.
  ///
  /// Returns true for:
  /// - HTTP/2 protocol errors
  /// - Connection errors (socket, network)
  /// - Timeout errors
  /// - Stream protocol errors (missing end-stream)
  static bool isNetworkError(Object error) {
    final errorString = error.toString().toLowerCase();
    return errorString.contains('http2') ||
        errorString.contains('connection') ||
        errorString.contains('socket') ||
        errorString.contains('network') ||
        errorString.contains('timeout') ||
        errorString.contains('end-stream') ||
        errorString.contains('protocol error');
  }

  /// Codes that represent expected, server-driven outcomes — not programmer
  /// errors or unrecoverable state. RPC failures with these codes should never
  /// be reported to Crashlytics as Fatal: they pollute the dashboard and bury
  /// real bugs.
  static const _expectedRpcCodes = {
    Code.permissionDenied,
    Code.unauthenticated,
    Code.notFound,
    Code.failedPrecondition,
    Code.invalidArgument,
    Code.resourceExhausted,
    Code.alreadyExists,
    Code.canceled,
  };

  /// Returns true if [error] is an RPC failure with a code that represents an
  /// expected server-driven decision (e.g. `permission_denied`, `not_found`).
  ///
  /// Use this alongside [isNetworkError] at Crashlytics report sites to decide
  /// the `fatal:` flag — these should be recorded as non-fatal so the fatal
  /// dashboard surfaces only real crashes.
  ///
  /// Recognises both [ServiceException] (the typed wrapper produced by
  /// `RpcUtils.executeRpc`) and raw [ConnectException] (in case an error
  /// bypasses that wrapper).
  static bool isExpectedRpcError(Object error) {
    Code? code;
    if (error is ServiceException) {
      code = error.code;
    } else if (error is ConnectException) {
      code = error.code;
    }
    if (code == null) return false;
    return _expectedRpcCodes.contains(code);
  }

  /// Gets a user-friendly message for a ConnectRPC error code.
  ///
  /// Pass [l10n] when a BuildContext is available (preferred); when omitted
  /// the method falls back to English. Per the i18n architecture, callers in
  /// viewmodels should expose typed error states (Code or enums) and let the
  /// widget layer resolve them via AppLocalizations.of(context).
  String getUserMessageForCode(Code code, {AppLocalizations? l10n}) {
    return _localizeCode(code, l10n);
  }

  /// Classifies any caught exception into a typed [UserError] suitable for
  /// storing on viewmodel state.
  ///
  /// Precedence: [ServiceException] (preserves server message + code) >
  /// [ConnectException] (raw RPC error that bypassed `executeRpc`) — when
  /// a `LocalizedErrorDetail` is attached the classifier surfaces it via
  /// [UserError.localizedCode] so widgets can resolve to a locale-specific
  /// ARB key instead of the generic per-Code fallback — >
  /// [SocketException] / [TimeoutException] / [HttpException] (transport) >
  /// `generic(fallback)` for anything else. Reads `e.message` and `e.code`
  /// directly — never `e.toString()` — so a future `ServiceException`
  /// `toString()` override cannot leak.
  ///
  /// [fallback] is an optional English context string surfaced only when the
  /// classifier hits the generic bucket. The typed branches still win and
  /// remain localized via [localize] at render time.
  ///
  /// This is a pure transformation — it does not log. Logging stays in
  /// `RpcUtils.executeRpc` and the viewmodel-level `_log.severe(...)` calls
  /// surrounding each catch.
  static UserError classify(Object e, {String? fallback}) {
    if (e is ServiceException) {
      return UserError.serverMessage(message: e.message, code: e.code);
    }
    if (e is ConnectException) {
      final detail = _extractLocalizedErrorDetail(e);
      if (detail != null) {
        return UserError.localizedCode(
          code: detail.code,
          params: Map<String, String>.from(detail.params),
        );
      }
      return UserError.rpcCode(e.code);
    }
    if (e is SocketException) {
      return const UserError.transport(TransportKind.socket);
    }
    if (e is TimeoutException) {
      return const UserError.transport(TransportKind.timeout);
    }
    if (e is HttpException) {
      return const UserError.transport(TransportKind.http);
    }
    return UserError.generic(fallback: fallback);
  }

  /// Walks the Connect error's details and returns the first
  /// attached [errors_pb.LocalizedErrorDetail], or null when none is
  /// present. The server attaches one via `connecterr.UserVisible`
  /// (`server/connecterr/user_visible.go`); errors without a detail
  /// fall through to the per-Code branch in [classify].
  static errors_pb.LocalizedErrorDetail? _extractLocalizedErrorDetail(
    ConnectException e,
  ) {
    for (final detail in e.details) {
      if (detail.type == 'ripls.api.LocalizedErrorDetail') {
        try {
          return errors_pb.LocalizedErrorDetail.fromBuffer(detail.value);
        } catch (parseError, stack) {
          // A type-matched detail that fails to decode is a real
          // server bug worth seeing in logs — the client falls
          // back to per-Code rendering so the user still sees
          // something, but the malformed bytes shouldn't be silent.
          Logger('RpcErrorHandler').warning(
            'malformed LocalizedErrorDetail; falling back to per-Code rendering',
            parseError,
            stack,
          );
        }
      }
    }
    return null;
  }

  /// Resolves a [UserError] to a localized user-visible string.
  static String localize(UserError error, AppLocalizations l10n) {
    return switch (error) {
      UserErrorServerMessage(:final message) => message,
      UserErrorRpcCode(:final code) => _localizeCode(code, l10n),
      UserErrorLocalizedCode(:final code, :final params) =>
        _localizeServerCode(code, params, l10n),
      UserErrorTransport(:final kind) => switch (kind) {
          TransportKind.socket => l10n.rpcErrorTransportSocket,
          TransportKind.timeout => l10n.rpcErrorTransportTimeout,
          TransportKind.http => l10n.rpcErrorTransportHttp,
        },
      UserErrorGeneric(:final fallback) => fallback ?? l10n.rpcErrorGeneric,
    };
  }

  /// Maps a server-emitted [LocalizedErrorDetail] code to its ARB key,
  /// substituting [params] into ICU placeholders.
  ///
  /// Unknown codes (server emitted a code newer than this client)
  /// fall back to the generic RPC error string so the user still
  /// sees something readable. Add a new case here whenever the
  /// server starts emitting a new code via
  /// `connecterr.UserVisible`.
  static String _localizeServerCode(
    String code,
    Map<String, String> params,
    AppLocalizations l10n,
  ) {
    switch (code) {
      case 'internal_error_generic':
        return l10n.rpcErrorInternal;
      case 'experience_owner_required_for_delete':
        return l10n.rpcErrorExperienceOwnerRequiredForDelete;
      case 'experience_owner_required_for_share':
        return l10n.rpcErrorExperienceOwnerRequiredForShare;
      case 'experience_owner_required_for_unshare':
        return l10n.rpcErrorExperienceOwnerRequiredForUnshare;
      case 'experience_owner_required_for_update':
        return l10n.rpcErrorExperienceOwnerRequiredForUpdate;
      case 'gear_owner_required_for_delete':
        return l10n.rpcErrorGearOwnerRequiredForDelete;
      case 'gear_owner_required_for_update':
        return l10n.rpcErrorGearOwnerRequiredForUpdate;
      case 'gear_owner_required_for_share':
        return l10n.rpcErrorGearOwnerRequiredForShare;
      case 'gear_owner_required_for_unshare':
        return l10n.rpcErrorGearOwnerRequiredForUnshare;
      case 'gear_owner_required_for_sharing_settings':
        return l10n.rpcErrorGearOwnerRequiredForSharingSettings;
      case 'gear_owner_required_for_past_transfer':
        return l10n.rpcErrorGearOwnerRequiredForPastTransfer;
      case 'request_creator_required_for_cancel':
        return l10n.rpcErrorRequestCreatorRequiredForCancel;
      case 'request_creator_required_for_delete':
        return l10n.rpcErrorRequestCreatorRequiredForDelete;
      case 'request_creator_required_for_update':
        return l10n.rpcErrorRequestCreatorRequiredForUpdate;
      case 'request_creator_required_for_fulfill':
        return l10n.rpcErrorRequestCreatorRequiredForFulfill;
      case 'request_creator_required_for_share':
        return l10n.rpcErrorRequestCreatorRequiredForShare;
      case 'request_creator_required_for_unshare':
        return l10n.rpcErrorRequestCreatorRequiredForUnshare;
      case 'transfer_owner_required_for_select_recipient':
        return l10n.rpcErrorTransferOwnerRequiredForSelectRecipient;
      case 'transfer_owner_required_for_update_type':
        return l10n.rpcErrorTransferOwnerRequiredForUpdateType;
      case 'transfer_owner_or_recipient_required_for_modify':
        return l10n.rpcErrorTransferOwnerOrRecipientRequiredForModify;
      case 'transfer_recipient_must_be_member':
        return l10n.rpcErrorTransferRecipientMustBeMember;
      case 'community_owner_required_for_delete':
        return l10n.rpcErrorCommunityOwnerRequiredForDelete;
      case 'community_owner_required_for_region_override':
        return l10n.rpcErrorCommunityOwnerRequiredForRegionOverride;
      case 'community_not_member':
        return l10n.rpcErrorCommunityNotMember;
      case 'transfer_invalid_state_for_select_recipient':
        return l10n.rpcErrorTransferInvalidStateForSelectRecipient;
      case 'transfer_invalid_state_for_pickup_details':
        return l10n.rpcErrorTransferInvalidStateForPickupDetails;
      case 'transfer_invalid_state_for_update_type':
        return l10n.rpcErrorTransferInvalidStateForUpdateType;
      case 'transfer_loan_in_progress':
        return l10n.rpcErrorTransferLoanInProgress;
      case 'request_invalid_state_for_fulfill':
        return l10n.rpcErrorRequestInvalidStateForFulfill;
      case 'request_invalid_state_for_update':
        return l10n.rpcErrorRequestInvalidStateForUpdate;
      case 'request_invalid_state_for_withdraw_offer':
        return l10n.rpcErrorRequestInvalidStateForWithdrawOffer;
      case 'request_not_accepting_offers':
        return l10n.rpcErrorRequestNotAcceptingOffers;
      case 'gear_unavailable':
        return l10n.rpcErrorGearUnavailable;
      case 'gear_availability_not_set':
        return l10n.rpcErrorGearAvailabilityNotSet;
      case 'gear_not_shared_to_any_community':
        return l10n.rpcErrorGearNotSharedToAnyCommunity;
      case 'gear_last_community_cannot_unshare':
        return l10n.rpcErrorGearLastCommunityCannotUnshare;
      case 'gear_owner_required_for_availability':
        return l10n.rpcErrorGearOwnerRequiredForAvailability;
      case 'gear_availability_invalid':
        return l10n.rpcErrorGearAvailabilityInvalid;
      case 'gear_availability_change_blocked_active_transfer':
        return l10n.rpcErrorGearAvailabilityChangeBlockedActiveTransfer;
      case 'media_delete_not_authorized':
        return l10n.rpcErrorMediaDeleteNotAuthorized;
      case 'media_add_requires_shared_community':
        return l10n.rpcErrorMediaAddRequiresSharedCommunity;
      case 'esm_prompt_expired':
        return l10n.rpcErrorEsmPromptExpired;
      default:
        return l10n.rpcErrorGeneric;
    }
  }

  /// Maps a ConnectRPC [Code] to a localized string.
  ///
  /// Falls back to English strings when [l10n] is null (e.g. called without
  /// BuildContext). Prefer passing AppLocalizations when available.
  static String _localizeCode(Code code, AppLocalizations? l10n) {
    switch (code) {
      case Code.invalidArgument:
        return l10n?.rpcErrorInvalidArgument ??
            'Invalid input. Please check your information and try again.';
      case Code.unauthenticated:
        return l10n?.rpcErrorUnauthenticated ?? 'You need to sign in to continue.';
      case Code.permissionDenied:
        return l10n?.rpcErrorPermissionDenied ??
            "You don't have permission to perform this action.";
      case Code.notFound:
        return l10n?.rpcErrorNotFound ?? 'The requested item could not be found.';
      case Code.alreadyExists:
        return l10n?.rpcErrorAlreadyExists ?? 'This item already exists.';
      case Code.resourceExhausted:
        return l10n?.rpcErrorResourceExhausted ?? 'Too many requests. Please try again later.';
      case Code.unavailable:
        return l10n?.rpcErrorUnavailable ??
            'Service is temporarily unavailable. Please try again.';
      case Code.deadlineExceeded:
        return l10n?.rpcErrorDeadlineExceeded ??
            'Request timed out. Please check your connection and try again.';
      case Code.canceled:
        return l10n?.rpcErrorCanceled ?? 'Request was canceled.';
      case Code.unimplemented:
        return l10n?.rpcErrorUnimplemented ?? 'This feature is not yet available.';
      case Code.internal:
        return l10n?.rpcErrorInternal ?? 'An internal error occurred. Please try again.';
      case Code.dataLoss:
        return l10n?.rpcErrorDataLoss ?? 'Data error occurred. Please contact support.';
      case Code.failedPrecondition:
        return l10n?.rpcErrorFailedPrecondition ??
            'This action cannot be completed right now. Please check the item status.';
      case Code.unknown:
      default:
        return l10n?.rpcErrorGeneric ?? 'Something went wrong. Please try again.';
    }
  }
}
