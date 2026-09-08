import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show ExperienceTimeExtraction;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show DetectedGearItem;
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show GeocodedLocation;
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/presentation/viewmodels/replace_media_slot.dart';
import 'package:video_player/video_player.dart';

part 'unified_create_state.freezed.dart';

/// User-editable gear item-details (brand, model, est. value,
/// material, weight). Set via the UnifiedItemDetailsSheet pushed from
/// the preview card's "Item details" row when type=GEAR. Saved into
/// GearMetadata on the SaveGear call.
///
/// [weight] holds the numeric magnitude only (e.g. "3.2"); [weightUnit]
/// is the wire-level identifier ("kg" / "g" / "lbs"). Display labels
/// are localized by the consuming widget.
class ItemDetailsValue {
  const ItemDetailsValue({
    this.brand = '',
    this.model = '',
    this.estValueUsd = '',
    this.material = '',
    this.weight = '',
    this.weightUnit = 'kg',
  });
  final String brand;
  final String model;
  final String estValueUsd;
  final String material;
  final String weight;
  final String weightUnit;

  bool get isEmpty =>
      brand.isEmpty &&
      model.isEmpty &&
      estValueUsd.isEmpty &&
      material.isEmpty &&
      weight.isEmpty;
}

/// Top-level stage of the unified-create modal.
enum CreateStage {
  input,
  preview,
}

/// Input tab inside the bottom drawer.
enum CreateInputMode {
  text,
  image,
  url,
}

/// User-controlled transfer intent for type=gear. Lend by default;
/// flipped via a Lend/Give toggle on the preview card. The server does
/// not classify this — it is only consulted at SaveGear time.
enum TransferIntent {
  lend,
  give,
}

/// Set of fields the user has manually edited so a re-stream after
/// flipType does not overwrite their edits. Membership is checked in
/// the reducer.
enum UserEditedField {
  title,
  description,
  location,
  mediaIds,
  itemDetails,
  eventTime,
  requestSeedNeeds,
}

/// State for the unified-create flow. See
/// `docs/design/unified-create.md` § Architecture (Client).
@freezed
sealed class UnifiedCreateState with _$UnifiedCreateState {
  const factory UnifiedCreateState({
    @Default(CreateStage.input) CreateStage stage,
    @Default(CreateInputMode.image) CreateInputMode inputMode,
    @Default('') String prompt,
    @Default('') String urlInput,
    String? mediaId,

    /// The type the caller declared by choosing an intent-specific entry
    /// point ("Plan an event", "Ask for help", "Offer something"). Non-null
    /// means the classifier is skipped: [type] starts here, the stream is
    /// dispatched with `force_type`, and the input drawer shows the
    /// type-specific hint instead of the mixed example tour (#2936).
    ///
    /// Null for the generic `+` create, where the server infers the type.
    DetectedContentType? targetType,
    DetectedContentType? type,
    String? title,
    String? description,
    GeocodedLocation? location,
    String? locationId,
    @Default(<String>[]) List<String> mediaIds,

    /// Alternate media options for the Replace Media row. Initially
    /// populated by the MediaReady event with [ProtoSlot] entries
    /// (server-streamed candidates). When the user taps an entry, the
    /// previously-active media is captured as a slot and placed back
    /// at the tapped index — so swaps are reversible (one tap to swap
    /// to a candidate, second tap on the now-replaced slot brings the
    /// original back).
    @Default(<ReplaceMediaSlot>[]) List<ReplaceMediaSlot> mediaCandidates,

    /// The active media's "slot form" — used to put the active back
    /// into [mediaCandidates] when the user taps a candidate. Tracked
    /// as a slot rather than just a media id so a candidate-imported
    /// active retains its provider metadata for re-import on swap-back
    /// (round-trip preserves attribution).
    ReplaceMediaSlot? activeSlot,

    /// Index in [mediaCandidates] currently being imported via
    /// [UnifiedCreateViewModel.useCandidate]. Null when no import is
    /// in flight. The Replace Media modal shows an inline spinner on
    /// the tapped thumbnail while the import resolves.
    int? candidateImportingIndex,

    /// Transient poster URL the hero renders while a video candidate
    /// is being imported in the background. Set to the candidate's
    /// `thumbnail_url` immediately on tap so the user sees the new
    /// image right away; cleared once the imported video controller
    /// initializes (or the import fails).
    String? candidatePreviewPosterUrl,

    /// True while a media swap is in flight (AddMediaFromURL + video
    /// controller load). Drives a loading-spinner overlay on the hero
    /// so the user has feedback after the Replace Media modal
    /// dismisses.
    @Default(false) bool mediaSwapInProgress,
    ExperienceTimeExtraction? eventTime,
    ItemDetailsValue? itemDetails,

    /// The raw AI-detected gear from the streaming final — full fidelity
    /// (category, material category, weight distribution, value estimate
    /// with provenance), unlike the reduced editable [itemDetails]. Save
    /// persists this whole detection into GearMetadata; dropping it (and
    /// saving only brand/model) is what produced $0-value gear from photo
    /// captures (#2702).
    DetectedGearItem? detectedGear,
    @Default(TransferIntent.lend) TransferIntent transferIntent,

    /// The claimable things a request's text plainly names ("Lawn mower"; or
    /// "Picture books", "Whiteboard", …) — AI-extracted from the streaming
    /// final, then editable on the preview card so the requester sees (and can
    /// prune or add to) the needs their request will be born with. Passed to
    /// SubmitRequest as seed_need_names so the request starts with one need per
    /// entry (#2702, #2731). Empty when the type isn't request or the text
    /// named nothing concrete — the request is then born with no needs.
    @Default(<String>[]) List<String> requestSeedNeeds,
    @Default(<UserEditedField>{}) Set<UserEditedField> userEditedFields,

    /// Selector enabled state per the flip-timing rule (design doc § Decisions #10).
    /// False until the first `type` event arrives, false again while a
    /// flip-triggered re-stream is in flight.
    @Default(false) bool selectorEnabled,

    /// True while the StreamGenUnifiedCreate RPC is in flight.
    @Default(false) bool streaming,

    /// True once the stream has reached its terminal `final` event.
    @Default(false) bool streamComplete,

    /// True while the per-type Save* + Share* RPCs (and the post-save
    /// feed refresh) are in flight after the user taps Share. The
    /// preview UI shows a spinner on the primary button and disables
    /// further edits.
    @Default(false) bool saving,

    /// Surface a terminal error from the stream to the UI.
    String? errorMessage, // dart-error-tostring-allow #1899 (migrate to UserError follow-up)

    /// Initialized [VideoPlayerController] for the preview hero
    /// background when the streamed media is a video (stock-imagery
    /// fan-out emits videos for events). Null for image-only media.
    /// Owned and disposed by [UnifiedCreateViewModel]. Mirrors
    /// `GenExperienceState.previewVideoController`.
    VideoPlayerController? previewVideoController,
  }) = _UnifiedCreateState;

  const UnifiedCreateState._();

  /// Returns true when the item is ready to Save. Creation no longer picks
  /// an audience — the per-item community is provisioned server-side and the
  /// Share sheet opens after Save — so this is exactly [isContentValid].
  bool get isSaveable => isContentValid;

  /// True when the item's own content is ready to save, independent of any
  /// audience. Mirrors the per-type content checks the existing single-create
  /// preview modals apply:
  ///   * Event   — name ≥3 chars + description ≥10 chars
  ///               (experience_preview_modal._handleCreateExperience)
  ///   * Gear    — name + description
  ///               (GearHelper.validateGearFields)
  ///   * Request — title + description
  ///               (request_preview_modal._handleCreate)
  /// All types also require the AI stream to have produced a final event.
  bool get isContentValid {
    if (!streamComplete || type == null) return false;
    final t = (title ?? '').trim();
    final d = (description ?? '').trim();
    switch (type!) {
      case DetectedContentType.DETECTED_CONTENT_TYPE_EVENT:
        return t.length >= 3 && d.length >= 10;
      case DetectedContentType.DETECTED_CONTENT_TYPE_GEAR:
      case DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST:
        return t.isNotEmpty && d.isNotEmpty;
      default:
        return false;
    }
  }
}
