import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/core/utils/user_location_initializer.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/streaming/gen_stream_controller.dart';
import 'package:ripls/presentation/viewmodels/gen_request_view_model.dart';
import 'package:ripls/presentation/widgets/creation/input_mode_toggle.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

part 'request_creation_view_model.freezed.dart';

final _log = Logger('RequestCreationViewModel');

/// State for request creation modal.
@freezed
sealed class RequestCreationState with _$RequestCreationState {
  const factory RequestCreationState({
    @Default(CreationInputMode.text) CreationInputMode inputMode,
    @Default('') String textInput,
    String? selectedImagePath,
    @Default(false) bool isLoading,
    UserError? error,
  }) = _RequestCreationState;
}

/// Notifier for request creation modal.
///
/// Handles:
/// - Switching between text and image input modes
/// - Calling repository for request generation
/// - Managing state for the creation flow
///
/// Architecture: ViewModel → Repository → Service
class RequestCreationNotifier extends Notifier<RequestCreationState>
    with SafeNotifierMixin<RequestCreationState> {
  GenStreamController<StreamGenRequestResponse>? _streamController;

  @override
  RequestCreationState build() {
    ref.onDispose(() {
      _streamController?.dispose();
      _streamController = null;
    });
    return const RequestCreationState();
  }

  RequestRepository get _requestRepository =>
      ref.read(requestRepositoryProvider);

  /// Switch input mode while preserving both text and image data.
  void setInputMode(CreationInputMode mode) {
    if (mode == state.inputMode) return;

    state = state.copyWith(
      inputMode: mode,
      error: null,
    );
  }

  /// Set text input.
  void setTextInput(String text) {
    state = state.copyWith(
      textInput: text,
      error: null,
    );
  }

  /// Set selected image path.
  void setImagePath(String path) {
    state = state.copyWith(
      selectedImagePath: path,
      error: null,
    );
  }

  /// Clear selected image.
  void clearImage() {
    state = state.copyWith(
      selectedImagePath: null,
      error: null,
    );
  }

  /// Check if generation can proceed.
  ///
  /// In text mode: requires non-empty text input
  /// In image mode: always ready - Generate button will capture photo if needed
  bool canGenerate() {
    if (state.inputMode == CreationInputMode.text) {
      return state.textInput.trim().isNotEmpty;
    } else {
      // Image mode is always ready - Generate button will capture photo if needed
      return true;
    }
  }

  /// Open manual creation mode by navigating to the preview modal with empty fields.
  ///
  /// This allows users to manually fill in all fields without AI generation.
  ///
  /// Returns true if manual creation was initiated successfully.
  bool openManualCreation() {
    // This method will be called by the modal, which will then navigate
    // to the preview modal with empty/default values.
    return true;
  }

  /// Kicks off streaming generation and returns immediately. Events are
  /// pushed into [genRequestProvider] as they arrive so the preview modal
  /// can show partial fields while the rest streams in.
  ///
  /// Image mode uploads the selected image first; text mode opens the
  /// stream directly. Returns `false` if prerequisites aren't met (no
  /// input, or image upload fails).
  Future<bool> generateRequestStreaming() async {
    if (!canGenerate()) {
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Please provide either text or an image'),
      );
      return false;
    }

    await _streamController?.dispose();
    _streamController = null;

    state = state.copyWith(isLoading: true, error: null);

    String? prompt;
    String? mediaId;

    if (state.inputMode == CreationInputMode.image) {
      final path = state.selectedImagePath;
      if (path == null || path.isEmpty) {
        safeUpdateState((s) => s.copyWith(
              isLoading: false,
              error: const UserError.generic(fallback: 'No image selected'),
            ));
        return false;
      }
      try {
        mediaId = await ref.read(mediaRepositoryProvider).addMedia(
              file: XFile(path),
              description: 'Request creation image',
            );
      } catch (e) {
        safeUpdateState((s) => s.copyWith(
              isLoading: false,
              error: RpcErrorHandler.classify(e, fallback: 'Image upload failed'),
            ));
        return false;
      }
      if (!ref.mounted) return false;
    } else {
      prompt = state.textInput.trim();
    }

    final locationId =
        await UserLocationInitializer.getPrimaryLocationIfSet(ref);
    if (!ref.mounted) return false;
    final position = ref
        .read(userLocationProvider(LocationIntent.proximityBias))
        .asData
        ?.value;

    final genNotifier = ref.read(genRequestProvider.notifier);
    genNotifier.beginStreaming();

    final stream = _requestRepository.streamGenRequest(
      prompt: prompt,
      mediaId: mediaId,
      locationId: locationId,
      latitudeDeg: position?.latitude,
      longitudeDeg: position?.longitude,
    );

    final probeStart = DateTime.now();
    var firstEventLogged = false;
    void logProbe(String metric) {
      _log.info(
        'gen_probe flow=request mode=streaming metric=$metric '
        'ms=${DateTime.now().difference(probeStart).inMilliseconds}',
      );
    }

    final controller = GenStreamController<StreamGenRequestResponse>(
      onEvent: (event) {
        if (!firstEventLogged &&
            event.whichEvent() != StreamGenRequestResponse_Event.notSet) {
          firstEventLogged = true;
          logProbe('first_event');
        }
        switch (event.whichEvent()) {
          case StreamGenRequestResponse_Event.title:
            logProbe('title_event');
            genNotifier.applyStreamingTitle(event.title);
          case StreamGenRequestResponse_Event.description:
            logProbe('description_event');
            genNotifier.applyStreamingDescription(event.description);
          case StreamGenRequestResponse_Event.geocoded:
            logProbe('geocoded_event');
            genNotifier.applyStreamingGeocoded(event.geocoded);
          case StreamGenRequestResponse_Event.mediaReady:
            logProbe('media_event');
            genNotifier
                .applyStreamingMediaReady(event.mediaReady.mediaIds.toList());
          case StreamGenRequestResponse_Event.final_4:
            logProbe('final');
            final resp = event.final_4;
            genNotifier.finalizeStreaming(
              title: resp.title,
              description: resp.description,
              mediaIds: resp.mediaIds.toList(),
              tags: resp.tags.toList(),
              locationQuery:
                  resp.locationQuery.isNotEmpty ? resp.locationQuery : null,
              geocodedLocation:
                  resp.hasGeocodedLocation() ? resp.geocodedLocation : null,
              metadata: resp.hasMetadata() ? resp.metadata : null,
              additionalAsks: resp.additionalAsks.toList(),
              breakdownPieces: resp.breakdownPieces.toList(),
              offerIdeas: resp.offerIdeas.toList(),
              seedNeeds: resp.seedNeeds.toList(),
            );
            safeUpdateState((s) => s.copyWith(isLoading: false));
          case StreamGenRequestResponse_Event.error:
            logProbe('error');
            genNotifier.failStreaming(event.error.message);
            safeUpdateState((s) => s.copyWith(
                  isLoading: false,
                  error: UserError.generic(fallback: event.error.message),
                ));
          case StreamGenRequestResponse_Event.notSet:
            break;
        }
      },
      onError: (error, _) {
        _log.warning('stream transport error: $error');
        genNotifier.failStreaming(error.toString());
        safeUpdateState((s) => s.copyWith(
              isLoading: false,
              error: RpcErrorHandler.classify(error),
            ));
      },
      onDone: () {
        safeUpdateState((s) => s.copyWith(isLoading: false));
      },
    );
    _streamController = controller;
    controller.start(stream);
    return true;
  }

  /// Reset state for new creation.
  void reset() {
    state = const RequestCreationState();
  }
}

/// Provider for request creation state management.
final requestCreationProvider =
    NotifierProvider.autoDispose<RequestCreationNotifier, RequestCreationState>(
  RequestCreationNotifier.new,
);
