import 'package:connectrpc/connect.dart';
import 'package:freezed_annotation/freezed_annotation.dart';

part 'user_error.freezed.dart';

/// TransportKind enumerates transport-level failures that occur before the
/// RPC layer (socket, timeout, HTTP).
enum TransportKind { socket, timeout, http }

/// UserError is a typed representation of an error that should be shown to the
/// user. Viewmodels expose this on state instead of a resolved English string;
/// widgets resolve it to localized text via [RpcErrorHandler.localize] at
/// render time.
///
/// See `docs/client/i18n.md` ("Viewmodels never resolve strings") and
/// `app/lib/core/errors/README.md`.
@freezed
sealed class UserError with _$UserError {
  /// Produced from a [ServiceException]. Carries both the server-supplied
  /// message and the structured [Code] so widgets can switch on the code
  /// (e.g. show a sign-in CTA on [Code.unauthenticated]) without losing the
  /// server's exact wording.
  const factory UserError.serverMessage({
    required String message,
    Code? code,
  }) = UserErrorServerMessage;

  /// Produced from a raw [ConnectException] that did not pass through
  /// `RpcUtils.executeRpc` and therefore lacks a [ServiceException] wrap.
  const factory UserError.rpcCode(Code code) = UserErrorRpcCode;

  /// Produced from a [ConnectException] that carried a
  /// `ripls.api.LocalizedErrorDetail`. [code] is the stable
  /// server-emitted identifier (snake_case); [params] is the ICU
  /// substitution map (often empty). The widget layer resolves
  /// `code` to an ARB key — typically `rpcError<UpperCamelCase>` —
  /// and substitutes from `params`.
  ///
  /// See `docs/server/l10n.md` § "User-visible RPC errors" and
  /// `proto/ripls/api/errors.proto`.
  const factory UserError.localizedCode({
    required String code,
    @Default(<String, String>{}) Map<String, String> params,
  }) = UserErrorLocalizedCode;

  /// Transport-level failure (socket, timeout, HTTP).
  const factory UserError.transport(TransportKind kind) = UserErrorTransport;

  /// Catch-all for any non-RPC, non-transport exception. [fallback] is an
  /// optional English context string surfaced when the action is non-obvious
  /// from surrounding UI; otherwise the localizer falls back to the generic
  /// "Something went wrong" message.
  const factory UserError.generic({String? fallback}) = UserErrorGeneric;
}
