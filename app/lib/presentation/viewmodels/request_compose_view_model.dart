// State and Notifier for the compose stage of Request creation.
//
// The compose stage lives between the AI preview stage (handled by
// [genRequestProvider]) and the final SubmitRequest call. The requester
// breaks their request into discrete "pieces", optionally pre-claims the
// pieces they will handle themselves, and publishes — each piece
// becomes a Need on the new Request; pre-claimed pieces land with the
// requester's contribution already applied.
//
// AI suggestion chips are NOT owned here — they come down the wire on
// [GenRequestState.breakdownPieces] (populated by the existing
// streaming gen flow). The compose ViewModel reads them as input and
// turns selections into committed pieces.
//
// All chip / piece state is screen-scoped — the provider is
// autoDispose so it tears down with the modal. Repository ownership of
// caching is preserved (this VM holds no manual cache map).
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show
        BatchClaimRequestNeedItem,
        BatchClaimRequestNeedResult,
        RequestBatchNeedItem;
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/services/providers.dart';

part 'request_compose_view_model.freezed.dart';

final _log = Logger('RequestComposeViewModel');

/// A single piece in the compose draft. Each published piece becomes a
/// [PlanningNeed] on the new Request with `slots = 1`.
@freezed
sealed class ComposePiece with _$ComposePiece {
  const factory ComposePiece({
    /// Stable client-side id used as a list key and for toggle / rename.
    required String id,

    /// Human-visible label, 1–4 words (rendered as the Need's name).
    required String label,

    /// When set, the requester has marked "I've got this" — on publish, the
    /// new Need gets a pre-claim from the requester.
    @Default(false) bool preclaimed,

    /// When the piece was added by tapping a suggestion chip, this is
    /// the chip's text — so toggling the chip again removes the piece.
    String? fromSuggestionLabel,
  }) = _ComposePiece;
}

/// Outcome of a publish() call. A non-empty [failedAdds] or
/// [failedClaims] indicates partial success.
@freezed
sealed class ComposePublishResult with _$ComposePublishResult {
  const factory ComposePublishResult({
    required String requestId,

    /// Need IDs that were created via BatchAddRequestNeeds.
    @Default([]) List<String> addedNeedIds,

    /// Pieces whose batch-add failed. Surfaced to the user with a retry
    /// affordance.
    @Default([]) List<ComposePiece> failedAdds,

    /// Need IDs that were successfully pre-claimed.
    @Default([]) List<String> claimedNeedIds,

    /// Pre-claims that failed. Each entry carries the offending need_id
    /// and the server-reported error message.
    @Default([]) List<ComposePublishClaimFailure> failedClaims,
  }) = _ComposePublishResult;
}

@freezed
sealed class ComposePublishClaimFailure with _$ComposePublishClaimFailure {
  const factory ComposePublishClaimFailure({
    required String needId,
    required String errorMessage,
  }) = _ComposePublishClaimFailure;
}

/// State for the compose stage of Request creation.
@freezed
sealed class RequestComposeState with _$RequestComposeState {
  const factory RequestComposeState({
    @Default([]) List<ComposePiece> pieces,
    @Default(false) bool isPublishing,
    UserError? error,
  }) = _RequestComposeState;

  const RequestComposeState._();

  /// Number of pieces marked "I've got this".
  int get preclaimedCount => pieces.where((p) => p.preclaimed).length;

  /// True when at least one piece exists (Publish CTA enable check).
  bool get canPublish => pieces.isNotEmpty && !isPublishing;
}

/// Notifier for the compose stage. Reads AI chip suggestions from
/// [genRequestProvider]; writes Needs + pre-claims via the request
/// repository.
class RequestComposeNotifier extends Notifier<RequestComposeState> {
  @override
  RequestComposeState build() => const RequestComposeState();

  RequestRepository get _repo => ref.read(requestRepositoryProvider);

  // ── Piece manipulation ────────────────────────────────────────────────

  void _setPieces(List<ComposePiece> next) {
    state = state.copyWith(pieces: next);
  }

  String _newPieceId() =>
      'p_${DateTime.now().microsecondsSinceEpoch.toRadixString(36)}_${state.pieces.length}';

  /// Toggle a suggestion chip — adds a piece if the chip isn't already
  /// committed, removes it otherwise. Matches the prototype's chip
  /// toggle behavior.
  void toggleSuggestion(String label) {
    final trimmed = label.trim();
    if (trimmed.isEmpty) return;

    final existingIdx =
        state.pieces.indexWhere((p) => p.fromSuggestionLabel == trimmed);
    if (existingIdx >= 0) {
      _setPieces([
        for (var i = 0; i < state.pieces.length; i++)
          if (i != existingIdx) state.pieces[i],
      ]);
      return;
    }
    _setPieces([
      ...state.pieces,
      ComposePiece(
        id: _newPieceId(),
        label: trimmed,
        fromSuggestionLabel: trimmed,
      ),
    ]);
  }

  /// Add a single piece from the inline-add input.
  void addInlinePiece(String label) {
    final trimmed = label.trim();
    if (trimmed.isEmpty) return;
    _setPieces([
      ...state.pieces,
      ComposePiece(id: _newPieceId(), label: trimmed),
    ]);
  }

  /// Split [text] on commas, semicolons, and newlines, then add each
  /// non-empty fragment as a fresh piece. Used by the paste-a-list
  /// disclosure.
  void addPasteList(String text) {
    final fragments = text
        .split(RegExp(r'[,;\n]+'))
        .map((s) => s.trim())
        .where((s) => s.isNotEmpty)
        .toList();
    if (fragments.isEmpty) return;
    final next = [...state.pieces];
    for (final f in fragments) {
      next.add(ComposePiece(id: _newPieceId(), label: f));
    }
    _setPieces(next);
  }

  /// Inline rename. A no-op when the new label is empty (the widget
  /// guards on commit, but defense-in-depth here).
  void renamePiece(String id, String newLabel) {
    final trimmed = newLabel.trim();
    if (trimmed.isEmpty) return;
    _setPieces([
      for (final p in state.pieces)
        if (p.id == id) p.copyWith(label: trimmed) else p,
    ]);
  }

  /// Flip the requester's "I've got this" pre-claim on a piece.
  void togglePreclaim(String id) {
    _setPieces([
      for (final p in state.pieces)
        if (p.id == id) p.copyWith(preclaimed: !p.preclaimed) else p,
    ]);
  }

  /// Remove a piece from the draft.
  void removePiece(String id) {
    _setPieces([
      for (final p in state.pieces)
        if (p.id != id) p,
    ]);
  }

  /// Clear the draft. Called by the discard flow.
  void reset() {
    state = const RequestComposeState();
  }

  // ── Publish ───────────────────────────────────────────────────────────

  /// Publish the draft: assumes the parent flow has already created the
  /// Request via SubmitRequest and supplies its id. Adds one Need per
  /// piece, then pre-claims the pieces marked "I've got this".
  ///
  /// Partial success on either leg is recoverable — the result lists
  /// failed entries; the ViewModel state is preserved so the user can
  /// retry without re-entering pieces.
  Future<ComposePublishResult> publish({
    required String requestId,
    String communityId = '',
  }) async {
    state = state.copyWith(isPublishing: true, error: null);

    final pieces = state.pieces;
    if (pieces.isEmpty) {
      state = state.copyWith(isPublishing: false);
      return ComposePublishResult(requestId: requestId);
    }

    // ── Step 1: batch-add Needs ───────────────────────────────────────
    final items = [
      for (final p in pieces) RequestBatchNeedItem(name: p.label, slots: 1),
    ];

    List<RequestNeedResponse> addedNeedResps;
    try {
      addedNeedResps = await _repo.addNeedsBatch(
        requestId: requestId,
        items: items,
      );
    } catch (e, st) {
      _log.warning('publish: batch add failed', e, st);
      if (!ref.mounted) {
        return ComposePublishResult(requestId: requestId, failedAdds: pieces);
      }
      state = state.copyWith(
        isPublishing: false,
        error: RpcErrorHandler.classify(e),
      );
      return ComposePublishResult(requestId: requestId, failedAdds: pieces);
    }
    if (!ref.mounted) {
      return ComposePublishResult(requestId: requestId);
    }

    // The batch-add response order matches the input order, so we can
    // line up pieces with the returned need IDs by index.
    final addedNeedIds = [for (final n in addedNeedResps) n.id];

    // ── Step 2: batch-claim pre-claimed pieces ────────────────────────
    final claims = <BatchClaimRequestNeedItem>[];
    final preclaimedNeedIds = <String>[];
    for (var i = 0; i < pieces.length; i++) {
      if (!pieces[i].preclaimed) continue;
      final needId = addedNeedIds[i];
      if (needId.isEmpty) continue;
      preclaimedNeedIds.add(needId);
      claims.add(BatchClaimRequestNeedItem(needId: needId));
    }

    if (claims.isEmpty) {
      state = state.copyWith(isPublishing: false);
      return ComposePublishResult(
        requestId: requestId,
        addedNeedIds: addedNeedIds,
      );
    }

    List<BatchClaimRequestNeedResult> claimResults;
    try {
      claimResults = await _repo.batchClaimNeeds(
        requestId: requestId,
        claims: claims,
        communityId: communityId,
      );
    } catch (e, st) {
      _log.warning('publish: batch claim failed', e, st);
      if (!ref.mounted) {
        return ComposePublishResult(
          requestId: requestId,
          addedNeedIds: addedNeedIds,
          failedClaims: [
            for (final id in preclaimedNeedIds)
              ComposePublishClaimFailure(needId: id, errorMessage: '$e'),
          ],
        );
      }
      state = state.copyWith(
        isPublishing: false,
        error: RpcErrorHandler.classify(e),
      );
      return ComposePublishResult(
        requestId: requestId,
        addedNeedIds: addedNeedIds,
        failedClaims: [
          for (final id in preclaimedNeedIds)
            ComposePublishClaimFailure(needId: id, errorMessage: '$e'),
        ],
      );
    }
    if (!ref.mounted) {
      return ComposePublishResult(
        requestId: requestId,
        addedNeedIds: addedNeedIds,
      );
    }

    final claimedNeedIds = <String>[];
    final failedClaims = <ComposePublishClaimFailure>[];
    for (final r in claimResults) {
      if (r.hasErrorMessage()) {
        failedClaims.add(ComposePublishClaimFailure(
          needId: r.needId,
          errorMessage: r.errorMessage,
        ));
      } else {
        claimedNeedIds.add(r.needId);
      }
    }

    state = state.copyWith(isPublishing: false);
    return ComposePublishResult(
      requestId: requestId,
      addedNeedIds: addedNeedIds,
      claimedNeedIds: claimedNeedIds,
      failedClaims: failedClaims,
    );
  }
}

/// Screen-scoped provider for the compose stage. Disposed on modal
/// teardown.
final requestComposeProvider =
    NotifierProvider.autoDispose<RequestComposeNotifier, RequestComposeState>(
  RequestComposeNotifier.new,
);
