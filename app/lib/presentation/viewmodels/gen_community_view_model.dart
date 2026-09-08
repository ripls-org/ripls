import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/media_upload_mixin.dart';
import 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart' show MediaCandidate;
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/streaming/gen_stream_controller.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/viewmodels/replace_media_slot.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/providers.dart';

part 'gen_community_view_model.freezed.dart';

final _log = Logger('GenCommunityViewModel');

/// State for the community creation preview
@freezed
sealed class GenCommunityState with _$GenCommunityState {
  const factory GenCommunityState({
    String? name,
    String? description,
    List<String>? mediaIds,
    @Default(false) bool isCreating,
    @Default(false) bool isUploadingMedia,
    @Default(false) bool isStreaming,
    UserError? uploadError,
    int? batchUploadCompleted,
    int? batchUploadTotal,
    int? batchUploadFailedCount,
    UserError? error,
    String? createdCommunityId,

    /// When non-null, [GenCommunityNotifier.createCommunity] promotes this
    /// existing (nameless / ad-hoc) community in place via UpdateCommunity
    /// instead of creating a new one — the "give it a name + photo +
    /// description" promote path (#2492). Cleared by [GenCommunityNotifier.reset];
    /// seed it *after* reset.
    String? existingCommunityId,

    /// Replace Media row entries for community gen. ProtoSlot entries
    /// come from the server's MediaReady event; MediaIdSlot entries
    /// represent previously-active media kept around for swap-back.
    @Default(<ReplaceMediaSlot>[]) List<ReplaceMediaSlot> mediaCandidates,

    /// The active media's slot form — placed back into the row when
    /// the user taps a candidate.
    ReplaceMediaSlot? activeSlot,

    /// Index in [mediaCandidates] currently being imported.
    int? candidateImportingIndex,
  }) = _GenCommunityState;

  const GenCommunityState._();

  /// Check if workflow has completed successfully
  bool get isCompleted => createdCommunityId != null;

  /// Check if there's an error
  bool get hasError => error != null;
}

/// Notifier for managing community creation preview and submission
class GenCommunityNotifier extends Notifier<GenCommunityState>
    with MediaUploadMixin<GenCommunityState> {
  /// Active background-image stream subscription. Disposed before each new
  /// [fetchBackground] and on notifier disposal so no late events land.
  GenStreamController<StreamGenCommunityResponse>? _streamController;

  /// True once the user has explicitly chosen a background (Replace
  /// Background, candidate swap, or manual upload). Suppresses the
  /// automatic [fetchBackground] so a typed name change can't clobber a
  /// hand-picked image.
  bool _backgroundManuallyReplaced = false;

  /// True once an automatic background image has loaded. After the first
  /// auto-loaded image we **leave it** — later name edits don't re-search
  /// (it felt jarring to watch the image swap as the name was refined and the
  /// description filled in). The user can still change it via Replace
  /// Background. Cleared by [reset].
  bool _backgroundAutoLoaded = false;

  /// Debounce timer that coalesces name keystrokes into a single background
  /// fetch shortly after the user stops typing the name — so a relevant image
  /// loads automatically without the user having to submit the field.
  Timer? _backgroundDebounce;

  @override
  GenCommunityState build() {
    ref.onDispose(() {
      _backgroundDebounce?.cancel();
      _backgroundDebounce = null;
      _streamController?.dispose();
      _streamController = null;
    });
    return const GenCommunityState();
  }

  CommunityRepository get _communityRepository =>
      ref.read(communityRepositoryProvider);

  // MediaUploadMixin implementation
  @override
  MediaRepository get mediaRepository => ref.read(mediaRepositoryProvider);

  @override
  Logger get log => _log;

  @override
  String get mediaUploadDescription => 'Community media';

  @override
  void onMediaUploadStarted() {
    state = state.copyWith(isUploadingMedia: true, uploadError: null);
  }

  @override
  void onMediaUploaded(String mediaId) {
    _backgroundManuallyReplaced = true;
    state = state.copyWith(
      mediaIds: [mediaId],
      isUploadingMedia: false,
    );
  }

  @override
  void onMediaUploadFailed(String error) {
    state = state.copyWith(
      uploadError: UserError.generic(fallback: error),
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: null,
    );
  }

  @override
  void onBatchUploadProgress(int completed, int total) {
    state = state.copyWith(
      batchUploadCompleted: completed,
      batchUploadTotal: total,
    );
  }

  @override
  void onBatchUploadCompleted(List<String> mediaIds) {
    _backgroundManuallyReplaced = true;
    state = state.copyWith(
      mediaIds: mediaIds,
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: null,
    );
  }

  @override
  void onBatchUploadPartialFailure(List<String> uploadedIds, int failedCount) {
    _backgroundManuallyReplaced = true;
    state = state.copyWith(
      mediaIds: uploadedIds.isNotEmpty ? uploadedIds : state.mediaIds,
      isUploadingMedia: false,
      batchUploadCompleted: null,
      batchUploadTotal: null,
      batchUploadFailedCount: failedCount,
    );
  }

  /// Initialize the state with AI-generated or manual preview data
  void setPreviewData({
    required String name,
    required String description,
    List<String>? mediaIds,
  }) {
    _log.info('Setting preview data: name=$name, mediaIds=${mediaIds?.length ?? 0}');
    state = state.copyWith(
      name: name,
      description: description,
      mediaIds: mediaIds,
      error: null,
      createdCommunityId: null,
    );
  }

  /// Reset preview state at the start of a streaming generation. Clears
  /// any prior name/description/media so skeleton placeholders render
  /// in the preview modal until streaming events populate fields.
  void beginStreaming() {
    state = state.copyWith(
      name: null,
      description: null,
      mediaIds: null,
      error: null,
      createdCommunityId: null,
      isStreaming: true,
    );
  }

  /// Fetch a background image for the community by streaming
  /// [CommunityRepository.streamGenCommunity] keyed on the typed
  /// name/description. The server only finds a background image — name and
  /// description are owned by the user — so this updates [mediaIds] and the
  /// Replace Media candidates, never the text fields.
  ///
  /// No-ops when the name is empty or the user has already chosen a
  /// background manually. Background fetching is best-effort: stream errors
  /// only clear the spinner; they never surface a blocking error banner.
  Future<void> fetchBackground() async {
    _backgroundDebounce?.cancel();

    final name = state.name?.trim() ?? '';
    if (name.isEmpty) {
      _log.info('fetchBackground skipped: name empty');
      return;
    }
    if (_backgroundManuallyReplaced || _backgroundAutoLoaded) {
      _log.info('fetchBackground skipped: background already set');
      return;
    }

    final desc = state.description?.trim() ?? '';
    final prompt = desc.isEmpty ? name : '$name. $desc';
    _log.info('fetchBackground: streaming for prompt="$prompt"');

    await _streamController?.dispose();
    _streamController = null;

    state = state.copyWith(isStreaming: true, error: null);

    final stream = _communityRepository.streamGenCommunity(prompt: prompt);

    final controller = GenStreamController<StreamGenCommunityResponse>(
      onEvent: (event) {
        switch (event.whichEvent()) {
          case StreamGenCommunityResponse_Event.mediaReady:
            _log.info(
                'fetchBackground: media_ready (${event.mediaReady.mediaIds.length} media, ${event.mediaReady.candidates.length} candidates)');
            if (event.mediaReady.mediaIds.isNotEmpty) {
              _backgroundAutoLoaded = true;
            }
            applyStreamingMediaReady(
              event.mediaReady.mediaIds.toList(),
              event.mediaReady.candidates.toList(),
            );
          case StreamGenCommunityResponse_Event.final_4:
            _log.info(
                'fetchBackground: final (${event.final_4.mediaIds.length} media)');
            if (event.final_4.mediaIds.isNotEmpty) {
              _backgroundAutoLoaded = true;
            }
            finalizeStreaming(mediaIds: event.final_4.mediaIds.toList());
          case StreamGenCommunityResponse_Event.error:
            _log.warning(
              'community background stream error: ${event.error.message}',
            );
            state = state.copyWith(isStreaming: false);
          case StreamGenCommunityResponse_Event.notSet:
            break;
        }
      },
      onError: (error, _) {
        _log.warning('community background stream transport error: $error');
        state = state.copyWith(isStreaming: false);
      },
      onDone: () {
        state = state.copyWith(isStreaming: false);
      },
    );
    _streamController = controller;
    controller.start(stream);
  }

  /// Debounced trigger for [fetchBackground]. Call on every name/description
  /// keystroke; the actual fetch fires shortly after the user stops typing,
  /// so a relevant background loads automatically without a submit gesture.
  void scheduleBackgroundFetch() {
    // Fetch at most once: after the first auto-loaded image (or a manual
    // pick) we leave the background alone, so it doesn't keep swapping as the
    // name is refined or the description is filled in.
    if (_backgroundAutoLoaded || _backgroundManuallyReplaced) return;
    _backgroundDebounce?.cancel();
    _backgroundDebounce =
        Timer(const Duration(milliseconds: 800), fetchBackground);
  }

  /// Apply media IDs and candidate alternates from a streaming
  /// `media_ready` event. Mirrors the unified-create reducer:
  /// candidates become ProtoSlot entries, the chosen active media is
  /// captured as a MediaIdSlot for swap-back, the row is replaced
  /// each event (server-authoritative ordering).
  void applyStreamingMediaReady(
    List<String> mediaIds, [
    List<MediaCandidate> candidates = const [],
  ]) {
    if (mediaIds.isEmpty && candidates.isEmpty) return;
    final slots = candidates
        .map<ReplaceMediaSlot>((c) => ProtoSlot(c))
        .toList(growable: false);
    state = state.copyWith(
      mediaIds: mediaIds.isNotEmpty ? mediaIds : state.mediaIds,
      mediaCandidates: slots,
      activeSlot:
          mediaIds.isNotEmpty ? MediaIdSlot(mediaIds.first) : state.activeSlot,
    );
  }

  /// Import an alternate candidate the user tapped in Replace Media.
  /// Mirrors UnifiedCreateViewModel.useCandidate: ProtoSlot → server
  /// import; MediaIdSlot → just swap the active media id. Either way,
  /// the previously-active slot is placed back into the row at the
  /// tapped index so swaps are reversible.
  Future<void> useCandidate(int index) async {
    _backgroundManuallyReplaced = true;
    if (index < 0 || index >= state.mediaCandidates.length) return;
    final slot = state.mediaCandidates[index];
    final previousActive = state.activeSlot;

    final nextCandidates = List<ReplaceMediaSlot>.from(state.mediaCandidates);
    if (previousActive != null) {
      nextCandidates[index] = previousActive;
    } else {
      nextCandidates.removeAt(index);
    }

    state = state.copyWith(
      candidateImportingIndex: index,
      mediaCandidates: nextCandidates,
      activeSlot: slot,
    );

    try {
      final newMediaId = switch (slot) {
        ProtoSlot(:final candidate) => await _importCandidate(candidate),
        MediaIdSlot(:final mediaId) => mediaId,
      };
      state = state.copyWith(mediaIds: [newMediaId]);
    } catch (e, s) {
      _log.warning('community use candidate failed', e, s);
      state = state.copyWith(
        uploadError: UserError.generic(fallback: 'Replace media failed: $e'), // dart-error-tostring-allow #1899
        mediaCandidates: state.mediaCandidates,
        activeSlot: previousActive,
      );
    } finally {
      state = state.copyWith(candidateImportingIndex: null);
    }
  }

  Future<String> _importCandidate(MediaCandidate candidate) async {
    final mediaRepo = ref.read(mediaRepositoryProvider);
    return mediaRepo.addMediaFromUrl(
      url: candidate.url,
      description: null,
      provider: candidate.hasProvider() ? candidate.provider : null,
      providerPhotoId:
          candidate.hasProviderPhotoId() ? candidate.providerPhotoId : null,
    );
  }

  /// Finalize background streaming with the terminal `final` event's
  /// authoritative media IDs. Only updates [mediaIds] when the event
  /// carries any; name and description are user-owned and untouched.
  void finalizeStreaming({List<String>? mediaIds}) {
    state = state.copyWith(
      mediaIds:
          (mediaIds != null && mediaIds.isNotEmpty) ? mediaIds : state.mediaIds,
      error: null,
      isStreaming: false,
    );
  }

  /// Surface a streaming-side error onto the preview state.
  void failStreaming(String message) {
    state = state.copyWith(
      error: UserError.generic(fallback: message),
      isStreaming: false,
    );
  }

  /// Update the name in the preview
  void setName(String name) {
    state = state.copyWith(name: name);
  }

  /// Update the description in the preview
  void setDescription(String description) {
    state = state.copyWith(description: description);
  }

  /// Update the media IDs in the preview
  void setMediaIds(List<String>? mediaIds) {
    state = state.copyWith(mediaIds: mediaIds);
  }

  /// Seed promote mode (see [GenCommunityState.existingCommunityId]). A
  /// non-null id makes the next [createCommunity] update that community in
  /// place rather than creating a new one. Set this *after* [reset], which
  /// clears it.
  void setExistingCommunityId(String? id) {
    state = state.copyWith(existingCommunityId: id);
  }

  /// Submit the community: create a new one, or — when [GenCommunityState.existingCommunityId]
  /// is set — promote that existing nameless community in place.
  ///
  /// Returns the community ID on success, null on error. The name is required;
  /// the description is optional (may be empty). A brand-new community refreshes
  /// the community list, invalidates the feed, and navigates to the Feed tab; a
  /// promote updates in place via UpdateCommunity and only refreshes the list
  /// (no navigation — the user stays where they invoked it).
  Future<String?> createCommunity() async {
    final name = state.name?.trim();
    final description = state.description?.trim() ?? '';

    if (name == null || name.isEmpty) {
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Community name is required'),
      );
      return null;
    }

    final promoteId = state.existingCommunityId;
    _log.info(
        'Submitting community: name=$name, promote=${promoteId != null}, mediaIds=${state.mediaIds?.length ?? 0}');

    state = state.copyWith(
      isCreating: true,
      error: null,
    );

    try {
      final String communityId;
      if (promoteId != null) {
        // Promote an existing nameless (ad-hoc / per-item) community in place:
        // a name + optional photo/description turn it into a real, persistent
        // community (the server emits COMMUNITY_NAMED) (#2492). No create-only
        // landing side effects (feed invalidate / tab navigation) — the user
        // stays where they invoked the promote from.
        await _communityRepository.updateCommunity(
          id: promoteId,
          name: name,
          description: description,
          mediaIds: state.mediaIds,
        );
        communityId = promoteId;
        _log.info('Community promoted successfully: id=$communityId');
        // Refresh the list so the promoted community re-renders with its new
        // name (e.g. the Workshop carousel pill flips from group-text to name).
        await ref.read(communitiesProvider.notifier).reloadCommunities();
      } else {
        // Create a brand-new community via repository.
        communityId = await _communityRepository.createCommunity(
          name: name,
          description: description,
          mediaIds: state.mediaIds,
        );
        _log.info('Community created successfully: id=$communityId');

        // Refresh the community list so the new community is visible.
        await ref.read(communitiesProvider.notifier).reloadCommunities();

        // Invalidate feed cache so the feed reinitializes for the new community.
        await ref.read(feedRepositoryProvider).invalidateFeed();

        // Land on Home (#2634: the Feed tab is retired; Home's pulse
        // section now surfaces new community activity).
        ref.read(homeProvider.notifier).navigateToTab(1);
      }

      state = state.copyWith(
        isCreating: false,
        createdCommunityId: communityId,
      );

      return communityId;
    } catch (e) {
      _log.severe('Failed to submit community: $e');
      state = state.copyWith(
        isCreating: false,
        error: RpcErrorHandler.classify(e),
      );
      return null;
    }
  }

  /// Reset the state to initial values
  void reset() {
    _log.info('Resetting state');
    _backgroundDebounce?.cancel();
    _backgroundDebounce = null;
    _streamController?.dispose();
    _streamController = null;
    _backgroundManuallyReplaced = false;
    _backgroundAutoLoaded = false;
    state = const GenCommunityState();
  }
}

/// Provider for the GenCommunity workflow
final genCommunityProvider =
    NotifierProvider<GenCommunityNotifier, GenCommunityState>(
  GenCommunityNotifier.new,
);
