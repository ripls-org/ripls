import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate;
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/request.pbenum.dart' as proto;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show GetRequestStatsResponse;

part 'request_state.freezed.dart';

/// RequestState holds the full UI state for a request detail screen.
///
/// Owned by [RequestNotifier]. All action mixins read and write this state
/// via [Notifier.state] and [Notifier.state.copyWith].
@freezed
sealed class RequestState with _$RequestState {
  const factory RequestState({
    String? requestId,
    String? currentUserId,
    String? communityId,
    Request? requestDetails,
    String? requesterName,
    String? locationName,
    double? locationLatitude,
    double? locationLongitude,
    double? locationDistanceMeters,
    @Default(true) bool isLoading,
    @Default(false) bool isEditing,
    @Default(false) bool isSaving,
    @Default(false) bool isUploadingMedia,
    @Default(false) bool isResolvingRequest,
    @Default(false) bool isLoadingSummary,
    @Default(false) bool isEditingSummary,
    String? resolutionSummary,
    ImpactEstimate? completionSavings,
    GetRequestStatsResponse? requestStats,
    UserError? error,
    // Media state
    String? mediaPath,
    String? mediaId,
    @Default(false) bool isVideo,
    @Default(true) bool isMuted,
    @Default([]) List<MediaItemData> allMediaItems,
    @Default(0) int currentMediaIndex,
    Attribution? backgroundAttribution,
    // True while the video controller is being initialized; the view shows
    // backgroundThumbnailUrl as a static first frame during this window.
    @Default(false) bool isBackgroundMediaLoading,
    // Server-generated thumbnail URL displayed as a static background while
    // the video controller initializes. Cleared once the controller is ready.
    String? backgroundThumbnailUrl,
    // All communities this request is shared with.
    @Default([]) List<SharedCommunity> sharedCommunities,
    // Tab navigation state
    @Default(0) int selectedMediaIndex,
    // Number of images that failed during a batch upload (null when no batch in progress).
    int? batchUploadFailedCount,
  }) = _RequestState;

  const RequestState._();

  /// Returns whether the current user is the requester (owner) of the request.
  bool get isOwner =>
      currentUserId != null &&
      requestDetails != null &&
      currentUserId == requestDetails!.requester.id;

  /// canEditCoverPhoto reports whether the cover-photo picker overlay should be
  /// shown. Owners may swap the cover photo whenever the request is in edit
  /// mode.
  bool get canEditCoverPhoto => isEditing && isOwner;

  /// Returns whether the state has an error.
  bool get hasError => error != null;

  /// isFulfilled reports whether the request is in the FULFILLED terminal state.
  /// Drives the inline Impact section on the first tab.
  bool get isFulfilled =>
      requestDetails?.state == proto.RequestState.REQUEST_STATE_FULFILLED;

  /// isCancelled reports whether the request is in the CANCELLED terminal state.
  bool get isCancelled =>
      requestDetails?.state == proto.RequestState.REQUEST_STATE_CANCELLED;

  /// isTerminal reports whether the request is in any terminal state.
  /// Used to gate the inline edit pencil, row CTAs, and Offer-row trailing pill.
  bool get isTerminal => isFulfilled || isCancelled;

  /// showManageOverflow reports whether the always-on top-right `···` overflow
  /// should be shown for this viewer. Only the owner sees it, and never on a
  /// cancelled request (which has no remaining manage actions). A fulfilled
  /// request keeps it for the "View Impact" entry.
  bool get showManageOverflow => isOwner && !isCancelled;

  /// showManageSettingsActions reports whether the editing/settings rows (Edit
  /// Details, Mark Fulfilled, Close Request) belong in the Manage sheet. They
  /// only apply while the request is still active; terminal states hide them.
  bool get showManageSettingsActions => isOwner && !isTerminal;
}
