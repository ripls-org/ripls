import 'package:cross_file/cross_file.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/core/utils/user_location_initializer.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/streaming/gen_stream_controller.dart';
import 'package:ripls/presentation/viewmodels/gen_experience_view_model.dart';
import 'package:ripls/presentation/widgets/creation/input_mode_toggle.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';

part 'experience_creation_view_model.freezed.dart';

final _log = Logger('ExperienceCreationViewModel');

/// State for experience creation modal.
@freezed
sealed class ExperienceCreationState with _$ExperienceCreationState {
  const factory ExperienceCreationState({
    @Default(CreationInputMode.text) CreationInputMode inputMode,
    @Default('') String textInput,
    String? selectedImagePath,
    @Default(false) bool isLoading,
    GenExperienceResponse? generatedExperience,
    UserError? error,
  }) = _ExperienceCreationState;
}

/// Notifier for experience creation modal.
///
/// Handles:
/// - Switching between text and image input modes
/// - Calling repository for experience generation
/// - Managing state for the creation flow
///
/// Architecture: ViewModel → Repository → Service
class ExperienceCreationNotifier extends Notifier<ExperienceCreationState>
    with SafeNotifierMixin<ExperienceCreationState> {
  GenStreamController<StreamGenExperienceResponse>? _streamController;

  @override
  ExperienceCreationState build() {
    ref.onDispose(() {
      _streamController?.dispose();
      _streamController = null;
    });
    return const ExperienceCreationState();
  }

  ExperienceRepository get _experienceRepository =>
      ref.read(experienceRepositoryProvider);

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

  /// URL regex pattern for extracting URLs from text.
  /// Matches http:// or https:// followed by domain and optional path.
  static final _urlRegex = RegExp(
    r'https?://[^\s<>"{}|\\^`\[\]]+',
    caseSensitive: false,
  );

  /// Extract a URL from text input if present.
  ///
  /// Returns the first valid HTTP/HTTPS URL found in the text, or null if none.
  String? extractUrl(String text) {
    final match = _urlRegex.firstMatch(text);
    if (match == null) return null;
    final url = match.group(0);
    if (url == null) return null;
    // Validate it's a proper URL
    final uri = Uri.tryParse(url);
    if (uri == null || !uri.hasAuthority) return null;
    return url;
  }

  /// Check if the text input contains a URL.
  bool hasUrl() {
    return extractUrl(state.textInput) != null;
  }

  /// Check if generation can proceed.
  ///
  /// In text mode: requires non-empty text input (can be text or URL)
  /// In image mode: always true (camera will capture on Generate tap)
  bool canGenerate() {
    switch (state.inputMode) {
      case CreationInputMode.text:
        return state.textInput.trim().isNotEmpty;
      case CreationInputMode.image:
        // Image mode is always ready - Generate button will capture photo if needed
        return true;
    }
  }

  /// Open manual creation mode by initializing the preview modal with empty fields.
  ///
  /// This method sets up the genExperienceProvider with default/empty values,
  /// allowing users to manually fill in all fields without AI generation.
  ///
  /// Returns true if manual creation was initiated successfully.
  bool openManualCreation() {
    // This method will be called by the modal, which will then navigate
    // to the preview modal. The preview modal reads from genExperienceProvider.
    // We don't set state here - the modal handles navigation.
    return true;
  }

  /// Kicks off streaming generation and returns immediately. Events are pushed
  /// into [genExperienceProvider] as they arrive so the preview modal can show
  /// partial fields while the rest streams in.
  ///
  /// Image mode uploads the selected image first; URL and text modes open the
  /// stream directly. Returns `false` if prerequisites aren't met (no input,
  /// or image upload fails) — callers should not navigate in that case.
  Future<bool> generateExperienceStreaming() async {
    if (!canGenerate()) {
      state = state.copyWith(
        error: const UserError.generic(fallback: 'Please provide text or select an image'),
      );
      return false;
    }

    // A previous call may have left a stream in flight (user retried before
    // the first stream resolved). Cancel it so callbacks from the old one
    // cannot clobber state populated by the new one.
    await _streamController?.dispose();
    _streamController = null;

    state = state.copyWith(
      isLoading: true,
      error: null,
      generatedExperience: null,
    );

    String? textPrompt;
    String? websiteUrl;
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
              description: 'Experience creation image',
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
      final extracted = extractUrl(state.textInput);
      if (extracted != null) {
        websiteUrl = extracted;
      } else {
        textPrompt = state.textInput;
      }
    }

    final locationId =
        await UserLocationInitializer.getPrimaryLocationIfSet(ref);
    if (!ref.mounted) return false;
    final position = ref
        .read(userLocationProvider(LocationIntent.proximityBias))
        .asData
        ?.value;

    final genNotifier = ref.read(genExperienceProvider.notifier);
    genNotifier.beginStreaming();

    final stream = _experienceRepository.streamGenExperience(
      prompt: textPrompt,
      mediaId: mediaId,
      websiteUrl: websiteUrl,
      locationId: locationId,
      latitudeDeg: position?.latitude,
      longitudeDeg: position?.longitude,
    );

    final probeStart = DateTime.now();
    var firstEventLogged = false;
    void logProbe(String metric) {
      _log.info(
        'gen_probe flow=experience mode=streaming metric=$metric '
        'ms=${DateTime.now().difference(probeStart).inMilliseconds}',
      );
    }

    final controller = GenStreamController<StreamGenExperienceResponse>(
      onEvent: (event) {
        if (!firstEventLogged &&
            event.whichEvent() != StreamGenExperienceResponse_Event.notSet) {
          firstEventLogged = true;
          logProbe('first_event');
        }
        switch (event.whichEvent()) {
          case StreamGenExperienceResponse_Event.title:
            logProbe('title_event');
            genNotifier.applyStreamingTitle(event.title);
          case StreamGenExperienceResponse_Event.description:
            logProbe('description_event');
            genNotifier.applyStreamingDescription(event.description);
          case StreamGenExperienceResponse_Event.time:
            logProbe('time_event');
            genNotifier.applyStreamingTime(
              suggestedTime:
                  event.time.hasSuggestedTime() ? event.time.suggestedTime : null,
              confidence:
                  event.time.hasConfidence() ? event.time.confidence : null,
              extractedTimeUnixSec: event.time.hasExtractedTimeUnixSec()
                  ? event.time.extractedTimeUnixSec
                  : null,
            );
          case StreamGenExperienceResponse_Event.geocoded:
            logProbe('geocoded_event');
            genNotifier.applyStreamingGeocoded(event.geocoded);
          case StreamGenExperienceResponse_Event.mediaReady:
            logProbe('media_event');
            genNotifier
                .applyStreamingMediaReady(event.mediaReady.mediaIds.toList());
          case StreamGenExperienceResponse_Event.final_4:
            logProbe('final');
            genNotifier.finalizeStreaming(event.final_4);
            safeUpdateState((s) => s.copyWith(
                  isLoading: false,
                  generatedExperience: event.final_4,
                ));
          case StreamGenExperienceResponse_Event.error:
            logProbe('error');
            genNotifier.failStreaming(event.error.message);
            safeUpdateState((s) => s.copyWith(
                  isLoading: false,
                  error: UserError.generic(fallback: event.error.message),
                ));
          case StreamGenExperienceResponse_Event.notSet:
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
    state = const ExperienceCreationState();
  }
}

/// Provider for experience creation notifier.
final experienceCreationProvider =
    NotifierProvider.autoDispose<ExperienceCreationNotifier, ExperienceCreationState>(
  ExperienceCreationNotifier.new,
);
