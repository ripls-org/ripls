import 'dart:async';

import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart'
    show GearMetadataFormatter;
import 'package:ripls/core/utils/media_picker_helper.dart';
import 'package:ripls/core/utils/video_cache_helper.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show ExperienceTimeExtraction;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show DetectedGearItem;
import 'package:ripls/data/gen/ripls/api/gen_stream.pb.dart'
    show MediaCandidate;
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart'
    show StockImageProvider;
import 'package:ripls/data/gen/ripls/api/time.pb.dart' show ExperienceTime;
import 'package:ripls/data/gen/ripls/api/time.pbenum.dart' show TimeConfidence;
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType, StreamGenUnifiedCreateFinal_Payload;
import 'package:ripls/presentation/viewmodels/replace_media_slot.dart';
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_streaming_actions.dart';
import 'package:ripls/services/providers/auth_providers.dart'
    show observabilityServiceProvider;
import 'package:ripls/services/providers/media_providers.dart';
import 'package:ripls/services/providers/unified_create_providers.dart';
import 'package:video_player/video_player.dart';

final _log = Logger('UnifiedCreateViewModel');

/// View model for the unified-create flow. Owns input collection,
/// streaming dispatch, type-flip re-streaming, and the Lend/Give toggle.
///
/// See `docs/design/unified-create.md` § Architecture (Client).
class UnifiedCreateViewModel extends Notifier<UnifiedCreateState> {
  StreamSubscription<void>? _streamSub;
  UnifiedCreateStreamingActions? _actions;

  /// Owned video controller mirror of [UnifiedCreateState.previewVideoController]
  /// so we can dispose without racing the state. Matches the pattern in
  /// `GenExperienceViewModel._previewVideoController`.
  VideoPlayerController? _previewVideoController;

  /// Tracks the mediaId the current controller was initialised for, so a
  /// `MediaReady` event with a different first id triggers a re-init.
  String? _previewVideoControllerMediaId;

  @override
  UnifiedCreateState build() {
    // Auto-attach the production streaming actions. Tests can override
    // via [attachActions] after constructing the provider container.
    // Wrapped in try/catch so test ProviderContainers without auth /
    // transport overrides don't blow up build() — tests always call
    // attachActions next anyway.
    try {
      _actions = ref.read(unifiedCreateStreamingActionsProvider);
    } catch (_) {
      _actions = null;
    }
    ref.onDispose(() {
      _cancelStream();
      unawaited(_previewVideoController?.dispose());
      _previewVideoController = null;
    });
    return const UnifiedCreateState();
  }

  /// Inject the streaming actions provider. Real callers use the default
  /// (Riverpod-provided) actions; tests pass a fake.
  void attachActions(UnifiedCreateStreamingActions actions) {
    _actions = actions;
  }

  /// Reset state to the initial empty value. Creation no longer picks an
  /// audience — the per-item community is provisioned server-side and the
  /// Share sheet (opened after Save) handles audience expansion — so there
  /// is nothing to pre-seed. Called by the modal on every open since the
  /// provider itself is not autoDispose.
  void reset() {
    _cancelStream();
    unawaited(_previewVideoController?.dispose());
    _previewVideoController = null;
    _previewVideoControllerMediaId = null;
    state = const UnifiedCreateState();
  }

  void setInputMode(CreateInputMode mode) {
    state = state.copyWith(inputMode: mode);
  }

  /// Seed the type the caller already committed to by picking an
  /// intent-specific entry point, so the composer opens in that shape
  /// rather than an undifferentiated blank one (#2936).
  ///
  /// The input mode follows the type: an event and a request are things
  /// you describe, an item is a thing you photograph. Callers can still
  /// switch tabs; this only picks where the drawer lands.
  void seedTargetType(DetectedContentType type) {
    state = state.copyWith(
      targetType: type,
      type: type,
      inputMode: type == DetectedContentType.DETECTED_CONTENT_TYPE_GEAR
          ? CreateInputMode.image
          : CreateInputMode.text,
    );
  }

  void setPrompt(String value) {
    state = state.copyWith(prompt: value);
  }

  void setUrlInput(String value) {
    state = state.copyWith(urlInput: value);
  }

  
  void setTransferIntent(TransferIntent intent) {
    state = state.copyWith(transferIntent: intent);
  }

  /// Toggle the in-flight Save+Share spinner on the primary button.
  /// Driven by the modal's onShare closure around the save dispatch.
  void setSaving(bool value) {
    state = state.copyWith(saving: value);
  }

  
  /// Photos option for the replace-media dialog.
  Future<void> replaceBackgroundFromPhotos() =>
      _replaceFromPicker(MediaPickerHelper.pickImageFromGallery);

  /// Camera option for the replace-media dialog.
  Future<void> replaceBackgroundFromCamera() =>
      _replaceFromPicker(MediaPickerHelper.pickImageFromCamera);

  /// Video option for the replace-media dialog. The hero backdrop only
  /// renders images, so we still store the resulting mediaId in
  /// `state.mediaIds` — saved gear/event/request carries the video, and
  /// the backdrop falls back to the blurred underlay until the user
  /// replaces with a photo.
  Future<void> replaceBackgroundFromVideo() =>
      _replaceFromPicker(MediaPickerHelper.pickVideoFromGallery);

  Future<void> _replaceFromPicker(Future<XFile?> Function() pick) async {
    try {
      final file = await pick();
      if (file == null) return;
      final mediaRepo = ref.read(mediaRepositoryProvider);
      final mediaId =
          await mediaRepo.addMedia(file: file, description: null);
      state = state.copyWith(
        mediaIds: [mediaId],
        userEditedFields: {...state.userEditedFields, UserEditedField.mediaIds},
      );
      // Drop any previously-loaded preview video controller so the hero
      // falls back to the still-image renderer (or to the new video).
      unawaited(_loadPreviewVideoIfNeeded([mediaId]));
    } catch (e, s) {
      _log.warning('replace background failed', e, s);
      state = state.copyWith(errorMessage: 'Replace background failed: $e'); // dart-error-tostring-allow #1899
    }
  }

  /// Import an alternate media candidate the user tapped in the Replace
  /// Media modal. The server fetches the URL and returns a media id we
  /// swap into [UnifiedCreateState.mediaIds]. Shows an inline spinner on
  /// the tapped thumbnail via [UnifiedCreateState.candidateImportingIndex]
  /// while the import resolves; clears in `finally` so a disposal
  /// mid-flight never leaves the spinner stuck.
  Future<void> useCandidate(int index) async {
    if (index < 0 || index >= state.mediaCandidates.length) return;
    final slot = state.mediaCandidates[index];

    // Telemetry: log the tap before the network round-trip so we
    // capture intent regardless of success/failure. ProtoSlot taps
    // carry the most metadata; MediaIdSlot taps are swap-backs.
    final telemetryEvent = switch (slot) {
      ProtoSlot(:final candidate) => ReplaceMediaCandidateTappedEvent(
          index: index,
          provider: _candidateProviderTag(candidate),
          contentType: candidate.contentType,
        ),
      MediaIdSlot() => ReplaceMediaCandidateTappedEvent(
          index: index,
          provider: 'ripls-internal',
          contentType: '',
        ),
    };
    unawaited(
      ref.read(observabilityServiceProvider).logAnalyticsEvent(telemetryEvent),
    );

    // Build a poster URL for the immediate hero render. ProtoSlot has
    // thumbnail_url (or url) directly; MediaIdSlot requires a resolved
    // ripls media URL which we fetch lazily during the swap.
    String? posterUrl;
    if (slot is ProtoSlot) {
      posterUrl = slot.candidate.hasThumbnailUrl()
          ? slot.candidate.thumbnailUrl
          : slot.candidate.url;
    }

    // Capture the previously-active slot before mutating state. We
    // place it back into the row at the tapped index so a second tap
    // reverses the swap.
    final ReplaceMediaSlot? previousActive = state.activeSlot;

    // Drop the existing video controller now so a failed swap doesn't
    // silently leave the previous video playing under the cleared
    // poster.
    unawaited(_previewVideoController?.dispose());
    _previewVideoController = null;
    _previewVideoControllerMediaId = null;

    // Compose the post-tap candidates list: the tapped slot is replaced
    // with the previously-active slot (when we have one); otherwise
    // the tapped slot disappears and the row shrinks. Other slots are
    // kept in place to preserve their relative ordering.
    final List<ReplaceMediaSlot> nextCandidates;
    if (previousActive != null) {
      nextCandidates = List<ReplaceMediaSlot>.from(state.mediaCandidates);
      nextCandidates[index] = previousActive;
    } else {
      nextCandidates = [
        for (int i = 0; i < state.mediaCandidates.length; i++)
          if (i != index) state.mediaCandidates[i],
      ];
    }

    state = state.copyWith(
      candidateImportingIndex: index,
      candidatePreviewPosterUrl: posterUrl,
      mediaSwapInProgress: true,
      previewVideoController: null,
      mediaCandidates: nextCandidates,
      activeSlot: slot,
    );

    try {
      final newMediaId = switch (slot) {
        ProtoSlot(:final candidate) => await _importCandidate(candidate),
        MediaIdSlot(:final mediaId) => mediaId,
      };
      state = state.copyWith(
        mediaIds: [newMediaId],
        userEditedFields: {...state.userEditedFields, UserEditedField.mediaIds},
        // activeSlot already updated above to the tapped slot; if the
        // tapped slot was a MediaIdSlot pointing at this same id, the
        // assignment stays consistent.
      );
      // Re-evaluate the preview video controller: if the new media is
      // a still image this disposes any stale video controller; for a
      // video it builds a new controller and surfaces it on state.
      await _loadPreviewVideoIfNeeded([newMediaId]);
    } catch (e, s) {
      _log.warning('use candidate failed', e, s);
      // Revert the candidate list + active slot since the swap didn't
      // land. mediaIds was never updated to the new media; activeSlot
      // returns to what it was before.
      state = state.copyWith(
        errorMessage: 'Replace media failed: $e', // dart-error-tostring-allow #1899
        mediaCandidates: state.mediaCandidates.length == nextCandidates.length
            ? _restoreOriginalCandidates(nextCandidates, index, slot)
            : state.mediaCandidates,
        activeSlot: previousActive,
      );
    } finally {
      state = state.copyWith(
        candidateImportingIndex: null,
        candidatePreviewPosterUrl: null,
        mediaSwapInProgress: false,
      );
    }
  }

  /// Imports a streamed candidate via AddMediaFromURL. Extracted from
  /// useCandidate so the sealed switch in the caller stays readable.
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

  /// Puts [tapped] back at [index] after a failed swap. Used on the
  /// error path so the user sees the same row of candidates they
  /// started with rather than a row that already swapped the slot.
  List<ReplaceMediaSlot> _restoreOriginalCandidates(
    List<ReplaceMediaSlot> current,
    int index,
    ReplaceMediaSlot tapped,
  ) {
    if (index < 0 || index >= current.length) return current;
    final restored = List<ReplaceMediaSlot>.from(current);
    restored[index] = tapped;
    return restored;
  }

  /// Maps the candidate's provider enum into the lowercase tag used in
  /// analytics. Defaults to "url" when no stock-imagery provider is
  /// attached (generic URL alternates from webpage extraction).
  String _candidateProviderTag(MediaCandidate c) {
    if (!c.hasProvider()) return 'url';
    switch (c.provider) {
      case StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS:
        return 'pexels';
      case StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH:
        return 'unsplash';
      case StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPECIFIED:
        return 'unspecified';
    }
    return 'unspecified';
  }

  void setItemDetails(ItemDetailsValue value) {
    state = state.copyWith(
      itemDetails: value,
      userEditedFields: {...state.userEditedFields, UserEditedField.itemDetails},
    );
  }

  /// Persist a user-picked event time. Wraps the picked [ExperienceTime]
  /// in an [ExperienceTimeExtraction] so the existing reducer/state shape
  /// (which already carries an extraction from the AI path) doesn't need
  /// to branch on origin.
  void setEventTime(ExperienceTime time) {
    state = state.copyWith(
      eventTime: ExperienceTimeExtraction(
        suggestedTime: time,
        confidence: TimeConfidence.TIME_CONFIDENCE_EXPLICIT,
      ),
      userEditedFields: {...state.userEditedFields, UserEditedField.eventTime},
    );
  }

  void setLocationFromPicker(String? newLocationId) {
    if (newLocationId == null || newLocationId.isEmpty) return;
    state = state.copyWith(
      locationId: newLocationId,
      userEditedFields: {...state.userEditedFields, UserEditedField.location},
    );
  }

  /// Pick an image from the photo gallery, upload it, and start the
  /// unified-create stream in image mode. Surfaces an error message in
  /// state if anything fails. The in-app camera flow goes through
  /// [captureAndStartFromCamera] instead — the OS-camera picker is no
  /// longer used inside unified-create.
  Future<void> pickAndStartFromGallery() async {
    final file = await MediaPickerHelper.pickImageFromGallery();
    if (file == null) return;
    await _uploadAndStartStream(file, source: 'gallery');
  }

  /// Upload an in-app camera capture (XFile produced by
  /// `CameraController.takePicture()` inside `_CameraLayer`) and start
  /// the unified-create stream in image mode.
  Future<void> captureAndStartFromCamera(XFile file) async {
    _log.info('camera capture: ${file.name}');
    await _uploadAndStartStream(file, source: 'camera');
  }

  /// Shared upload-and-stream path used by both gallery picks and in-app
  /// camera captures. [source] is used only for logging differentiation.
  ///
  /// The modal can be closed while the media upload is in flight (the
  /// user taps the close button after pulling the shutter). When that
  /// happens, the provider's `Ref` is disposed and any state assignment
  /// after the next `await` throws. The `ref.mounted` guards below let
  /// the in-flight upload finish silently rather than crash the app.
  Future<void> _uploadAndStartStream(
    XFile file, {
    required String source,
  }) async {
    final actions = _actions;
    if (actions == null) return;
    try {
      state = state.copyWith(
        stage: CreateStage.preview,
        streaming: true,
        streamComplete: false,
        errorMessage: null,
        selectorEnabled: false,
        type: state.targetType,
        title: null,
        description: null,
        location: null,
        mediaIds: const <String>[],
        eventTime: null,
        userEditedFields: const <UserEditedField>{},
      );
      final mediaRepo = ref.read(mediaRepositoryProvider);
      final mediaId =
          await mediaRepo.addMedia(file: file, description: null);
      if (!ref.mounted) return;
      // Mark mediaIds as user-edited so a flipType re-stream's
      // stock-imagery suggestion does not replace the user's photo (#1957).
      state = state.copyWith(
        mediaId: mediaId,
        mediaIds: [mediaId],
        userEditedFields: {...state.userEditedFields, UserEditedField.mediaIds},
      );
      await _dispatchStream(actions, forceType: state.targetType);
    } catch (e, s) {
      _log.warning('$source upload + dispatch failed', e, s);
      if (!ref.mounted) return;
      state = state.copyWith(
        streaming: false,
        selectorEnabled: true,
        errorMessage: 'Image upload failed: $e', // dart-error-tostring-allow #1899
      );
    }
  }

  /// Mark a field as user-edited so the reducer drops incoming events
  /// for it on a flipType re-stream.
  void editTitle(String value) {
    state = state.copyWith(
      title: value,
      userEditedFields: {...state.userEditedFields, UserEditedField.title},
    );
  }

  void editDescription(String value) {
    state = state.copyWith(
      description: value,
      userEditedFields: {...state.userEditedFields, UserEditedField.description},
    );
  }

  /// Replace the seeded-needs list on a request preview (#2702, #2731). Blank
  /// entries are dropped; an empty list means the request is born with no
  /// needs. Marking the field user-edited stops a later streaming final from
  /// overwriting the requester's pruned/added list.
  void editRequestSeedNeeds(List<String> values) {
    state = state.copyWith(
      requestSeedNeeds: [
        for (final n in values)
          if (n.trim().isNotEmpty) n.trim(),
      ],
      userEditedFields: {
        ...state.userEditedFields,
        UserEditedField.requestSeedNeeds,
      },
    );
  }

  /// Begin the initial unified-create stream. Resets state and dispatches
  /// `StreamGenUnifiedCreate` with the current input.
  Future<void> start() async {
    final actions = _actions;
    if (actions == null) {
      _log.warning('start() called without attached actions');
      return;
    }
    state = state.copyWith(
      stage: CreateStage.preview,
      streaming: true,
      streamComplete: false,
      errorMessage: null,
      selectorEnabled: false,
      // A seeded target type survives the clear: the caller already chose
      // it, so the card should open in that shape rather than flickering
      // through "no type" until the first wire event lands (#2936).
      type: state.targetType,
      title: null,
      description: null,
      location: null,
      mediaIds: const <String>[],
      eventTime: null,
      userEditedFields: const <UserEditedField>{},
    );
    await _dispatchStream(actions, forceType: state.targetType);
  }

  /// Flip the inferred type. Cancels the in-flight stream, drops
  /// type-specific fields the user has not edited, and re-issues the
  /// stream with `force_type` set. Subject to [UnifiedCreateState.selectorEnabled]
  /// — no-op when the selector is currently locked out.
  Future<void> flipType(DetectedContentType newType) async {
    if (!state.selectorEnabled) {
      _log.fine('flipType ignored: selector locked');
      return;
    }
    final actions = _actions;
    if (actions == null) {
      return;
    }
    _cancelStream();
    state = state.copyWith(
      selectorEnabled: false,
      streaming: true,
      streamComplete: false,
      errorMessage: null,
      // Clear type-specific fields unless the user has edited them.
      eventTime: state.userEditedFields.contains(UserEditedField.eventTime)
          ? state.eventTime
          : null,
    );
    await _dispatchStream(actions, forceType: newType);
  }

  /// Hard ceiling on a single stream. Cancels the subscription and
  /// surfaces an error so the spinner can't hang forever when the
  /// AI backend is unreachable (most common in local dev: Vertex
  /// can't reach `LocalBucketStorage` URLs for image mode).
  static const _streamTimeout = Duration(seconds: 45);

  Future<void> _dispatchStream(
    UnifiedCreateStreamingActions actions, {
    DetectedContentType? forceType,
  }) async {
    final stream = actions.stream(state: state, forceType: forceType);
    _streamSub = stream.timeout(_streamTimeout, onTimeout: (sink) {
      sink.addError('AI request timed out');
      sink.close();
    }).listen(
      _applyEvent,
      onError: (Object e, StackTrace s) {
        _log.warning('stream error', e, s);
        state = state.copyWith(
          streaming: false,
          selectorEnabled: true,
          errorMessage: e.toString(), // dart-error-tostring-allow #1899
        );
      },
      onDone: () {
        // Reducer marks streamComplete on the terminal `final` event.
        state = state.copyWith(streaming: false, selectorEnabled: true);
      },
    );
  }

  void _applyEvent(UnifiedCreateEvent ev) {
    switch (ev) {
      case UnifiedCreateEventType(:final type):
        state = state.copyWith(type: type, selectorEnabled: true);
      case UnifiedCreateEventTitle(:final title):
        if (state.userEditedFields.contains(UserEditedField.title)) return;
        state = state.copyWith(title: title);
      case UnifiedCreateEventDescription(:final description):
        if (state.userEditedFields.contains(UserEditedField.description)) return;
        state = state.copyWith(description: description);
      case UnifiedCreateEventLocation(:final geocoded):
        if (state.userEditedFields.contains(UserEditedField.location)) return;
        state = state.copyWith(location: geocoded);
      case UnifiedCreateEventMedia(:final mediaIds, :final candidates):
        if (state.userEditedFields.contains(UserEditedField.mediaIds)) return;
        // Wrap each wire candidate in a ProtoSlot so the row renderer
        // and useCandidate handler can treat streamed candidates and
        // previously-active media (MediaIdSlot) uniformly.
        final slots = candidates
            .map<ReplaceMediaSlot>((c) => ProtoSlot(c))
            .toList(growable: false);
        // The initial active media is referenced by its ripls media id;
        // when the user taps a candidate, this slot is placed back into
        // the row so they can swap back.
        final ReplaceMediaSlot? newActiveSlot = mediaIds.isNotEmpty
            ? MediaIdSlot(mediaIds.first)
            : state.activeSlot;
        state = state.copyWith(
          mediaIds: List.unmodifiable(mediaIds),
          mediaCandidates: slots,
          activeSlot: newActiveSlot,
        );
        // Stock-imagery fan-out can emit a video media_id (event mode).
        // Mirror gen_experience_view_model._loadPreviewVideoIfNeeded so
        // the hero plays the video instead of showing only its
        // thumbnail as a still.
        unawaited(_loadPreviewVideoIfNeeded(mediaIds));
      case UnifiedCreateEventTime(:final time):
        if (state.userEditedFields.contains(UserEditedField.eventTime)) return;
        state = state.copyWith(eventTime: time);
      case UnifiedCreateEventFinal(:final finalPayload):
        ItemDetailsValue? itemDetails = state.itemDetails;
        // Keep the raw detection regardless of user edits — it is the
        // save-time source for full GearMetadata (value, material,
        // weight); user edits overlay it in the save actions.
        DetectedGearItem? detectedGear = state.detectedGear;
        if (finalPayload.whichPayload() ==
            StreamGenUnifiedCreateFinal_Payload.gear) {
          detectedGear = finalPayload.gear.detectedGear;
          if (!state.userEditedFields.contains(UserEditedField.itemDetails)) {
            final mapped =
                _itemDetailsFromDetectedGear(finalPayload.gear.detectedGear);
            if (mapped != null) itemDetails = mapped;
          }
        }
        // Terminal-payload fallback for the preview's text fields:
        // native-streaming providers emit title/description mid-stream,
        // but a unary-wrapped provider may deliver them only in the final
        // payload — without this the Save gate can never open in image
        // mode (#2687). Same precedent as itemDetails above: fill only
        // what the stream never set and the user never edited.
        final (String finalTitle, String finalDescription) =
            switch (finalPayload.whichPayload()) {
          StreamGenUnifiedCreateFinal_Payload.gear => (
              finalPayload.gear.detectedGear.title,
              finalPayload.gear.detectedGear.description,
            ),
          StreamGenUnifiedCreateFinal_Payload.experience => (
              finalPayload.experience.name,
              finalPayload.experience.description,
            ),
          StreamGenUnifiedCreateFinal_Payload.request => (
              finalPayload.request.title,
              finalPayload.request.description,
            ),
          _ => ('', ''),
        };
        final title = ((state.title ?? '').isEmpty &&
                finalTitle.isNotEmpty &&
                !state.userEditedFields.contains(UserEditedField.title))
            ? finalTitle
            : state.title;
        final description = ((state.description ?? '').isEmpty &&
                finalDescription.isNotEmpty &&
                !state.userEditedFields.contains(UserEditedField.description))
            ? finalDescription
            : state.description;
        state = state.copyWith(
          streamComplete: true,
          streaming: false,
          selectorEnabled: true,
          // type comes from the final's type field for consistency.
          type: finalPayload.type,
          title: title,
          description: description,
          itemDetails: itemDetails,
          detectedGear: detectedGear,
          requestSeedNeeds: finalPayload.whichPayload() ==
                      StreamGenUnifiedCreateFinal_Payload.request &&
                  finalPayload.request.seedNeeds.isNotEmpty &&
                  !state.userEditedFields
                      .contains(UserEditedField.requestSeedNeeds)
              ? finalPayload.request.seedNeeds
              : state.requestSeedNeeds,
        );
      case UnifiedCreateEventError(:final message):
        state = state.copyWith(
          streamComplete: false,
          streaming: false,
          selectorEnabled: true,
          errorMessage: message,
        );
    }
  }

  void _cancelStream() {
    _streamSub?.cancel();
    _streamSub = null;
  }

  /// Maps an AI-detected gear item into the editable [ItemDetailsValue]
  /// shape consumed by `UnifiedItemDetailsSheet`. Emits raw values only
  /// (number + wire-level unit identifier) — display formatting is the
  /// widget's job. Returns null when every field would be empty so the
  /// "item details unset" semantic is preserved.
  ItemDetailsValue? _itemDetailsFromDetectedGear(DetectedGearItem gear) {
    final brand =
        GearMetadataFormatter.isUnknownOrEmpty(gear.brand) ? '' : gear.brand;
    final model =
        GearMetadataFormatter.isUnknownOrEmpty(gear.model) ? '' : gear.model;
    final estValueUsd = gear.hasValueEstimate() &&
            gear.valueEstimate.estimatedValueUsd > 0
        ? gear.valueEstimate.estimatedValueUsd.toStringAsFixed(2)
        : '';
    String weight = '';
    String weightUnit = 'kg';
    if (gear.hasWeightGrams() &&
        gear.weightGrams.hasMean() &&
        gear.weightGrams.mean > 0) {
      final grams = gear.weightGrams.mean;
      if (grams >= 1000) {
        weight = (grams / 1000).toStringAsFixed(1);
        weightUnit = 'kg';
      } else {
        weight = grams.toStringAsFixed(0);
        weightUnit = 'g';
      }
    }
    if (brand.isEmpty &&
        model.isEmpty &&
        estValueUsd.isEmpty &&
        weight.isEmpty) {
      return null;
    }
    return ItemDetailsValue(
      brand: brand,
      model: model,
      estValueUsd: estValueUsd,
      weight: weight,
      weightUnit: weightUnit,
    );
  }

  /// Loads a [VideoPlayerController] for the preview hero when the first
  /// media in [mediaIds] has a `video/*` contentType. Mirrors
  /// `GenExperienceViewModel._loadPreviewVideoIfNeeded`: get the full
  /// (non-thumb) URL, build a cached file-backed controller, surface it
  /// on state for the hero widget to render via [VideoPlayer]. Safe to
  /// call on every MediaReady — short-circuits when the controller
  /// already matches the first id, and disposes/replaces when the id
  /// changes (re-stream or replace-background).
  Future<void> _loadPreviewVideoIfNeeded(List<String> mediaIds) async {
    if (mediaIds.isEmpty) return;
    final mediaId = mediaIds.first;
    if (_previewVideoControllerMediaId == mediaId &&
        _previewVideoController != null) {
      return;
    }
    try {
      final repo = ref.read(mediaRepositoryProvider);
      final mediaUrl = await repo.getFullMediaUrl(mediaId);
      if (!ref.mounted) return;
      if (mediaUrl.contentType?.startsWith('video/') != true) {
        // Image media — drop any previously-loaded video controller so
        // the hero falls back to the still-image renderer.
        if (_previewVideoController != null) {
          unawaited(_previewVideoController?.dispose());
          _previewVideoController = null;
          _previewVideoControllerMediaId = null;
          state = state.copyWith(previewVideoController: null);
        }
        return;
      }
      final controller =
          await createCachedVideoController(repo, mediaId, mediaUrl.url);
      if (!ref.mounted) {
        await controller.dispose();
        return;
      }
      unawaited(_previewVideoController?.dispose());
      _previewVideoController = controller;
      _previewVideoControllerMediaId = mediaId;
      state = state.copyWith(previewVideoController: controller);
    } catch (e) {
      _log.warning('failed to load preview video for $mediaId', e);
      // Non-fatal — hero falls back to the still image.
    }
  }
}

/// Riverpod provider for the unified-create view model. The provider
/// is NOT autoDispose because the modal's view model is read by both
/// the modal widget tree and the save-dispatch closure that runs after
/// dismiss — autoDispose would recreate the notifier mid-flow. State
/// reset on modal open happens in [UnifiedCreateViewModel.reset].
final unifiedCreateViewModelProvider =
    NotifierProvider<UnifiedCreateViewModel, UnifiedCreateState>(
  UnifiedCreateViewModel.new,
);
