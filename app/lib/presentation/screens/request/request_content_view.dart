import 'dart:async';

import 'package:connectrpc/connect.dart' show Code;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_helper.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/models/item_metric_data_base.dart'
    show ItemType;
import 'package:ripls/presentation/screens/item/item_metrics_screen.dart';
import 'package:ripls/presentation/screens/request/mark_fulfilled_modal.dart';
import 'package:ripls/presentation/screens/request/widgets/request_chat_pane.dart';
import 'package:ripls/presentation/screens/request/widgets/request_conversation_panel.dart';
import 'package:ripls/presentation/screens/request/widgets/request_edit_pane.dart';
import 'package:ripls/presentation/screens/request/widgets/request_helpers_panel.dart';
import 'package:ripls/presentation/screens/request/widgets/request_location_panel.dart';
import 'package:ripls/presentation/screens/request/widgets/request_manage_menu_sheet.dart';
import 'package:ripls/presentation/screens/request/widgets/request_read_shell.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_sharing_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/content/access_sheet.dart';
import 'package:ripls/presentation/widgets/content/close_item_modal.dart';
import 'package:ripls/presentation/widgets/content/content_edit_bar.dart';
import 'package:ripls/presentation/widgets/content/content_editing_mixin.dart';
import 'package:ripls/presentation/widgets/content/content_gradient_overlay.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel_launcher.dart';
import 'package:ripls/presentation/widgets/content/content_removed_view.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';
import 'package:ripls/presentation/widgets/media/media_picker_button.dart';
import 'package:ripls/presentation/widgets/media/video_background_host.dart';
import 'package:ripls/presentation/widgets/media/video_mute_toggle_button.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_sheet.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart';
import 'package:ripls/services/providers.dart';

/// RequestContentView displays the full content for a request as a
/// bottom-anchored read shell over the hero (`RequestReadShell`). The
/// discussion, location, and helpers cards morph-expand into full-screen
/// panels; edit mode swaps the shell for `RequestEditPane` + the edit bar. The
/// old tabbed Request/Discuss layout has been removed (#2293).
class RequestContentView extends ConsumerStatefulWidget {
  final String requestId;
  final bool showEditControls;
  final bool showFloatingActions;
  final bool showOwnerInfo;
  final VoidCallback? onDeleted;

  // Feed header parameters (optional, only used in feed context)
  final bool showFeedHeader;
  final User? feedActor;
  final int? feedOccurredAtUnixSec;
  final int? feedLastActivityAtUnixSec;
  final String? feedActionText;

  /// Optional community ID to use for the initial data load, overriding the
  /// global communitiesProvider.
  final String? initialCommunityId;

  /// Server-computed distance in meters from the discover feed.
  final double? initialDistanceMeters;

  /// Deep-link hint: a non-zero value (the legacy "chat" tab index) opens the
  /// conversation panel once the request has loaded. 0 shows the read shell.
  final int initialTab;

  const RequestContentView({
    super.key,
    required this.requestId,
    this.showEditControls = true,
    this.showFloatingActions = true,
    this.showOwnerInfo = true,
    this.onDeleted,
    this.initialCommunityId,
    this.initialDistanceMeters,
    this.initialTab = 0,
    this.showFeedHeader = false,
    this.feedActor,
    this.feedOccurredAtUnixSec,
    this.feedLastActivityAtUnixSec,
    this.feedActionText,
  });

  @override
  ConsumerState<RequestContentView> createState() => _RequestContentViewState();
}

class _RequestContentViewState extends ConsumerState<RequestContentView>
    with ContentEditingMixin {
  // Cached so it can be used in dispose() where ref.read is forbidden.
  HomeNotifier? _homeNotifier;

  @override
  void initState() {
    super.initState();
    if (widget.showFeedHeader) {
      _homeNotifier = ref.read(homeProvider.notifier);
    }

    final authState = ref.read(authStateProvider);

    WidgetsBinding.instance.addPostFrameCallback((_) async {
      if (!mounted) return;
      await ref
          .read(requestProvider(widget.requestId).notifier)
          .initialize(
            requestId: widget.requestId,
            currentUserId: authState.user?.id,
            communityId: widget.initialCommunityId,
            initialDistanceMeters: widget.initialDistanceMeters,
          );

      if (!mounted) return;

      // Deep-link to the conversation (legacy "chat" tab): open the morph
      // conversation panel once the request has loaded.
      if (widget.initialTab != 0) {
        unawaited(_expandConversation(_defaultPanelRect()));
      }

      ref.listenManual(contentCacheInvalidationProvider, (previous, next) {
        if (!mounted || previous == next) return;
        ref
            .read(requestProvider(widget.requestId).notifier)
            .refreshRequestDetails();
      });

      if (widget.showFeedHeader) {
        // Lock the home nav while editing; an open morph panel locks nav itself
        // via openContentMorphPanel.
        ref.listenManual(requestProvider(widget.requestId), (previous, next) {
          if (!mounted) return;
          final shouldLock = next.isEditing;
          final wasLocked = previous?.isEditing ?? false;
          if (shouldLock != wasLocked) {
            if (shouldLock) {
              ref.read(homeProvider.notifier).lockNav();
            } else {
              ref.read(homeProvider.notifier).unlockNav();
            }
          }
        });
      }
    });
  }

  @override
  void dispose() {
    _homeNotifier?.unlockNav(silent: true);
    super.dispose();
  }

  // ── Helpers ──────────────────────────────────────────────────────────────

  Color _accentColor() => AppColors.requestColorOnDark;

  /// A reasonable morph source rect when there's no tapped card to grow from
  /// (e.g. a deep link straight to the conversation): the lower half of the
  /// screen, so the panel still grows upward rather than from a corner.
  Rect _defaultPanelRect() {
    final size = MediaQuery.of(context).size;
    return Rect.fromLTRB(0, size.height * 0.5, size.width, size.height);
  }

  Future<void> _showAccessSheet(RequestState state) async {
    final request = state.requestDetails;
    if (request == null) return;

    final owner = request.requester;
    final createdAt = DateTime.fromMillisecondsSinceEpoch(
      request.createdAtUnixSec.toInt() * 1000,
    );
    final age = DateTime.now().difference(createdAt);
    final agoStr = age.inDays > 0
        ? '${age.inDays}d ago'
        : age.inHours > 0
        ? '${age.inHours}h ago'
        : '${age.inMinutes}m ago';

    final ownerInitials = owner.name
        .split(' ')
        .where((p) => p.isNotEmpty)
        .take(2)
        .map((p) => p[0])
        .join()
        .toUpperCase();

    final invitable = CommunityHelper.invitableSharedCommunities(
      state.sharedCommunities,
      ref.read(communitiesProvider),
    );

    final otherCommunityCount =
        (request.totalSharedCommunityCount - state.sharedCommunities.length)
            .clamp(0, 999999);

    await AccessSheet.show(
      context,
      creator: AccessCreator(
        name: owner.name,
        initials: ownerInitials,
        timeAgo: agoStr,
      ),
      groups: _buildAccessGroups(state),
      totalPeople: request.totalDistinctMemberCount,
      otherCommunityCount: otherCommunityCount,
      onAddCommunity: state.isOwner ? _showAddCommunity : null,
      onInvitePerson: invitable.isEmpty ? null : _inviteFromAccessSheet,
    );

    // Refresh request details after the sheet is dismissed so any community
    // sharing changes are reflected next time the sheet opens.
    if (!mounted) return;
    ref
        .read(requestProvider(widget.requestId).notifier)
        .refreshRequestDetails()
        .ignore();
  }

  /// The access sheet's "Invite someone": opens the share sheet, then returns
  /// the request's refreshed access groups so the still-open sheet reflects any
  /// community/person added (#2492).
  Future<List<AccessGroup>?> _inviteFromAccessSheet() async {
    final request = ref.read(requestProvider(widget.requestId)).requestDetails;
    if (request == null) return null;
    await ItemShareSheet.show(
      context,
      itemType: ShareableItemType.request,
      itemId: request.id,
      itemName: request.title,
    );
    if (!mounted) return null;
    await ref
        .read(requestProvider(widget.requestId).notifier)
        .refreshRequestDetails();
    if (!mounted) return null;
    return _buildAccessGroups(ref.read(requestProvider(widget.requestId)));
  }

  /// _buildAccessGroups converts the shared communities on [state] into the
  /// [AccessGroup] list consumed by [AccessSheet].
  List<AccessGroup> _buildAccessGroups(RequestState state) {
    final owner = state.requestDetails?.requester;
    if (owner == null) return [];
    final ownerInitials = owner.name
        .split(' ')
        .where((w) => w.isNotEmpty)
        .take(2)
        .map((w) => w[0])
        .join()
        .toUpperCase();
    return state.sharedCommunities.map((c) {
      final cSharedAt = DateTime.fromMillisecondsSinceEpoch(
        c.sharedAtUnixSec.toInt() * 1000,
      );
      final cAge = DateTime.now().difference(cSharedAt);
      final cAgo = cAge.inDays > 0
          ? '${cAge.inDays}d ago'
          : cAge.inHours > 0
          ? '${cAge.inHours}h ago'
          : '${cAge.inMinutes}m ago';
      return AccessGroup(
        communityId: c.communityId,
        communityName: c.communityName,
        sharedByName: owner.name.split(' ').first,
        sharedByInitials: ownerInitials,
        sharedTimeAgo: cAgo,
        sharedAtUnixSec: c.sharedAtUnixSec.toInt(),
        memberCount: c.memberCount,
      );
    }).toList();
  }

  /// _showAddCommunity opens the community selection modal, waits for it to
  /// close, then immediately returns an updated [AccessGroup] list derived from
  /// the real-time [requestSharingProvider] state (which is already up-to-date
  /// because each toggle fires the API synchronously). The server sync refresh
  /// is deferred to when the outer [AccessSheet] is dismissed.
  Future<List<AccessGroup>?> _showAddCommunity() async {
    if (!mounted) return null;
    final state = ref.read(requestProvider(widget.requestId));
    final request = state.requestDetails;
    if (request == null) return null;

    ref
        .read(requestSharingProvider.notifier)
        .setSharedCommunities(request.sharedCommunityIds);
    await ref.read(requestSharingProvider.notifier).loadUserCommunities();

    if (!mounted) return null;
    await CommunitySelectionSheet.showForImmediate(
      context,
      itemId: request.id,
      itemType: 'request',
      isOwner: true,
      source: CommunitySelectionSource.requestAccess,
    );

    if (!mounted) return null;

    // Read the sharing provider's optimistic state — it is updated synchronously
    // as each toggle fires, so it is already correct when the modal closes.
    // The server-side refresh is deferred to when the outer AccessSheet dismisses.
    final sharingState = ref.read(requestSharingProvider);
    final sharedIds = sharingState.sharedCommunityIds;
    final currentState = ref.read(requestProvider(widget.requestId));
    final owner = currentState.requestDetails?.requester;
    if (owner == null) return null;

    final ownerInitials = owner.name
        .split(' ')
        .where((w) => w.isNotEmpty)
        .take(2)
        .map((w) => w[0])
        .join()
        .toUpperCase();
    final nowSec = DateTime.now().millisecondsSinceEpoch ~/ 1000;
    final existingById = {
      for (final c in currentState.sharedCommunities) c.communityId: c,
    };

    AccessGroup toGroup(String id) {
      final existing = existingById[id];
      final communityName =
          existing?.communityName ??
          sharingState.userCommunities
              .where((uc) => uc.id == id)
              .firstOrNull
              ?.name ??
          '';
      final sharedAtUnixSec = existing?.sharedAtUnixSec.toInt() ?? nowSec;
      final memberCount = existing?.memberCount ?? 0;
      final sharedAt = DateTime.fromMillisecondsSinceEpoch(
        sharedAtUnixSec * 1000,
      );
      final age = DateTime.now().difference(sharedAt);
      final timeAgo = age.inDays > 0
          ? '${age.inDays}d ago'
          : age.inHours > 0
          ? '${age.inHours}h ago'
          : '${age.inMinutes}m ago';
      return AccessGroup(
        communityId: id,
        communityName: communityName,
        sharedByName: owner.name.split(' ').first,
        sharedByInitials: ownerInitials,
        sharedTimeAgo: timeAgo,
        sharedAtUnixSec: sharedAtUnixSec,
        memberCount: memberCount,
      );
    }

    return sharedIds.map<AccessGroup>(toGroup).toList();
  }

  // ── Build ────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(requestProvider(widget.requestId));

    // Show spinner only while metadata (requestDetails) is still loading.
    // Once details are available, render immediately with thumbnail background
    // while the video controller initializes in the background.
    if (state.isLoading && state.requestDetails == null) {
      return ContentViewBuilders.buildLoadingView();
    }

    if (state.hasError) {
      final err = state.error!;
      final isNotFound =
          err is UserErrorServerMessage && err.code == Code.notFound ||
          (err is UserErrorRpcCode && err.code == Code.notFound);
      if (isNotFound) {
        return ContentRemovedView(
          itemType: 'request',
          onBack: widget.onDeleted,
        );
      }
      return ContentViewBuilders.buildErrorView(
        title: 'Failed to load request',
        errorMessage: RpcErrorHandler.localize(err, context.l10n),
        onRetry: () => ref
            .read(requestProvider(widget.requestId).notifier)
            .loadRequestDetails(),
      );
    }

    final isNavVisible = widget.showFeedHeader
        ? ref.watch(homeProvider).isNavVisible
        : false;

    final accentColor = _accentColor();

    return Stack(
      fit: StackFit.expand,
      children: [
        // 1. Background image/video
        VideoBackgroundHost(
          mediaPath: state.mediaPath,
          mediaId: state.mediaId,
          isVideo: state.isVideo,
          thumbnailUrl: state.backgroundThumbnailUrl,
          isMuted: state.isMuted,
        ),
        // 2. Gradient overlay
        ContentGradientOverlay(mediaCacheKey: state.mediaId),
        // 3. Read shell (read mode) or edit fields (edit mode). The read shell
        //    owns its top bar (attribution + overflow) and hides itself while a
        //    morph panel is open.
        if (state.isEditing)
          _buildEditPane(state)
        else
          _buildReadShell(state, isNavVisible),
        // 4. Edit bar at top (edit mode only)
        if (state.isEditing)
          ContentEditBar(
            accentColor: accentColor,
            isNavVisible: isNavVisible,
            onCancel: () {
              ref
                  .read(requestProvider(widget.requestId).notifier)
                  .toggleEditMode();
              resetEditing();
            },
            onSave: _handleSaveChanges,
          ),
        // 5. Media picker (in edit mode for owners; "Replace background" when
        //    a cover photo already exists, "Select a photo or video" otherwise).
        if (state.canEditCoverPhoto) _buildMediaPickerOverlay(state),
        // 6. Upload overlay
        if (state.isUploadingMedia) ContentViewBuilders.buildUploadOverlay(),
      ],
    );
  }

  // ── Read shell / edit pane ─────────────────────────────────────────────────

  Widget _buildReadShell(RequestState state, bool isNavVisible) {
    final notifier = ref.read(requestProvider(widget.requestId).notifier);
    return RequestReadShell(
      requestId: widget.requestId,
      accentColor: _accentColor(),
      onExpandConversation: _expandConversation,
      onShowLocation: _showLocationPanel,
      onShowHelpers: _showHelpersPanel,
      onShowAccess: () => _showAccessSheet(state),
      onInvite: _handleInvite,
      onManage: _showManageSheet,
      onViewImpact: _openImpactMetrics,
      muteButton: state.isVideo
          ? VideoMuteToggleButton(
              isMuted: state.isMuted,
              onTap: notifier.toggleMute,
            )
          : null,
      // Clear the home nav bar that overlays the bottom in feed context.
      bottomNavInset: isNavVisible ? 80 : 0,
    );
  }

  Widget _buildEditPane(RequestState state) {
    final request = state.requestDetails;
    if (request != null) {
      initializeEditing(
        initialTitle: request.title,
        initialDescription: request.description,
      );
    }
    final maxHeight = MediaQuery.of(context).size.height * 0.6;
    return Positioned(
      left: 0,
      right: 0,
      bottom: 0,
      // The caption column (#2912): edit mode holds the same reading measure
      // as the read shell; no-op at phone widths.
      child: ContentColumn(
        child: HeroContentWash(
          child: SafeArea(
            top: false,
            child: ConstrainedBox(
              constraints: BoxConstraints(maxHeight: maxHeight),
              child: RequestEditPane(
                editingTitle: editingTitle,
                editingDescription: editingDescription,
                onTitleChanged: (v) {
                  if (mounted) updateEditingTitle(v);
                },
                onDescriptionChanged: (v) {
                  if (mounted) updateEditingDescription(v);
                },
              ),
            ),
          ),
        ),
      ),
    );
  }

  /// Expands the discussion card into the full conversation: a morph-reveal
  /// panel that grows from the card's footprint ([sourceRect]) over the
  /// still-playing hero, replacing the old chat tab.
  Future<void> _expandConversation(Rect sourceRect) async {
    final state = ref.read(requestProvider(widget.requestId));
    final request = state.requestDetails;
    if (request == null || request.conversationId.isEmpty) return;
    final notifier = ref.read(requestProvider(widget.requestId).notifier);
    await openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: requestContentExpandedProvider(widget.requestId),
      sourceRect: sourceRect,
      routeName: 'request_conversation',
      screen: RequestConversationPanel(
        child: RequestChatPane(
          state: state,
          requestId: widget.requestId,
          accentColor: _accentColor(),
          forceActive: true,
          onAddMedia: () => _showCarouselMediaPicker(notifier),
          onDeleteMedia: (mediaId) => notifier.deleteMedia(mediaId),
          onReorderMedia: state.isOwner
              ? (mediaIds) => notifier.reorderMedia(mediaIds)
              : null,
        ),
      ),
    );
  }

  // ── Overlays ─────────────────────────────────────────────────────────────

  Widget _buildMediaPickerOverlay(RequestState state) {
    return Positioned(
      bottom: MediaQuery.of(context).size.height * 0.48 + 80,
      left: 24,
      right: 24,
      child: MediaPickerButton(
        hasMedia: state.mediaPath != null,
        isUploading: state.isUploadingMedia,
        onTap: _showMediaPickerDialog,
      ),
    );
  }

  // ── Actions / handlers ───────────────────────────────────────────────────

  Future<void> _showManageSheet() async {
    final state = ref.read(requestProvider(widget.requestId));

    final result = await showAccessibleModal<RequestManageAction>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => RequestManageMenuSheet(
        showSettingsActions: state.showManageSettingsActions,
        showViewImpact: state.isFulfilled,
      ),
    );
    if (result == null || !mounted) return;

    switch (result) {
      case RequestManageAction.editDetails:
        ref.read(requestProvider(widget.requestId).notifier).toggleEditMode();
      case RequestManageAction.markFulfilled:
        await _handleMarkFulfilled();
      case RequestManageAction.closeRequest:
        await CloseItemModal.show(
          context,
          contentType: CloseItemContentType.request,
          onUnshare: state.communityId == null ? null : _handleUnshareRequest,
          onCancel: _handleCancelRequest,
          onDelete: _handleDeleteRequest,
        );
      case RequestManageAction.viewImpact:
        await _openImpactMetrics();
    }
  }

  /// Opens the full impact receipt. Reached from the manage menu and from
  /// the inline post-fulfillment impact row on the read shell (#2724).
  Future<void> _openImpactMetrics() async {
    final state = ref.read(requestProvider(widget.requestId));
    final request = state.requestDetails;
    if (request == null) return;
    await NavigationHelpers.pushWithSlide(
      context: context,
      screen: ItemMetricsScreen(
        itemType: ItemType.request,
        itemId: request.id,
        communityId: state.communityId,
      ),
      routeName: 'item_metrics',
    );
  }

  Future<void> _showCarouselMediaPicker(RequestNotifier notifier) async {
    Future<void>? uploadFuture;

    final hasMedia = ref
        .read(requestProvider(widget.requestId))
        .allMediaItems
        .isNotEmpty;
    await showAccessibleModal<void>(
      context,
      builder: (sheetContext) {
        return Container(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                hasMedia ? 'Replace Media' : 'Add Media',
                style: const TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.bold,
                ),
              ),
              const SizedBox(height: 16),
              ListTile(
                leading: const Icon(Icons.video_library),
                title: const Text('Videos'),
                onTap: () {
                  Navigator.of(sheetContext).pop();
                  uploadFuture = notifier.pickVideoFromGallery();
                },
              ),
              ListTile(
                leading: const Icon(Icons.photo_library),
                title: const Text('Photos'),
                onTap: () {
                  Navigator.of(sheetContext).pop();
                  uploadFuture = notifier.pickMultipleImagesFromGallery();
                },
              ),
              ListTile(
                leading: const Icon(Icons.camera_alt),
                title: const Text('Camera'),
                onTap: () {
                  Navigator.of(sheetContext).pop();
                  uploadFuture = notifier.pickImageFromCamera();
                },
              ),
            ],
          ),
        );
      },
    );

    if (uploadFuture != null) {
      await uploadFuture;
      if (!mounted) return;
      final failedCount = ref
          .read(requestProvider(widget.requestId))
          .batchUploadFailedCount;
      if (failedCount != null && failedCount > 0) {
        ToastHelper.showError(
          context,
          context.l10n.mediaBatchUploadPartialFailure(failedCount),
        );
      }
    }
  }

  void _showMediaPickerDialog() {
    final state = ref.read(requestProvider(widget.requestId));
    final notifier = ref.read(requestProvider(widget.requestId).notifier);
    ContentViewHelpers.showMediaPickerDialog(
      context: context,
      hasMedia: state.mediaPath != null,
      onVideoTap: () => notifier.pickVideoFromGallery(insertAtFront: true),
      onPhotoTap: () => notifier.pickImageFromGallery(insertAtFront: true),
      onCameraTap: () => notifier.pickImageFromCamera(insertAtFront: true),
    );
  }

  Future<void> _handleSaveChanges() async {
    try {
      await ref
          .read(requestProvider(widget.requestId).notifier)
          .saveChanges(title: editingTitle, description: editingDescription);
    } catch (e) {
      if (mounted) ToastHelper.showError(context, 'Failed to save: $e');
    }
  }

  /// Opens the morph-reveal location panel, growing from the tapped row's
  /// [sourceRect]. The panel shows the spot on a map with directions for
  /// everyone and an inline picker for the owner — replacing the old stacked
  /// location-picker bottom sheet.
  Future<void> _showLocationPanel(Rect sourceRect) async {
    final state = ref.read(requestProvider(widget.requestId));
    if (state.requestDetails == null) return;
    await openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: requestContentExpandedProvider(widget.requestId),
      sourceRect: sourceRect,
      routeName: 'request_location',
      screen: RequestLocationPanel(requestId: widget.requestId),
    );
  }

  /// Opens the morph-reveal helpers panel, growing from the offer row's
  /// [sourceRect]. The panel shows offer/withdraw + the helping roster and
  /// carries a `···` overflow that opens the access sheet — replacing the old
  /// stacked HelperListSheet.
  Future<void> _showHelpersPanel(Rect sourceRect) async {
    final state = ref.read(requestProvider(widget.requestId));
    if (state.requestDetails == null) return;
    await openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: requestContentExpandedProvider(widget.requestId),
      sourceRect: sourceRect,
      routeName: 'request_helpers',
      screen: RequestHelpersPanel(
        requestId: widget.requestId,
        onShowAccess: () =>
            _showAccessSheet(ref.read(requestProvider(widget.requestId))),
        onInvite: _handleInvite,
      ),
    );
  }

  /// Opens the share-invite-link flow for the request, scoped to the
  /// communities the viewer can invite into. A no-op snackbar when there's no
  /// invitable community.
  void _handleInvite() {
    final state = ref.read(requestProvider(widget.requestId));
    final invitable = CommunityHelper.invitableSharedCommunities(
      state.sharedCommunities,
      ref.read(communitiesProvider),
    );
    if (invitable.isEmpty) {
      ToastHelper.showError(context, context.l10n.pitchingInInviteUnavailable);
      return;
    }
    _showShareInviteLink(invitable);
  }

  Future<void> _handleMarkFulfilled() async {
    final state = ref.read(requestProvider(widget.requestId));
    final request = state.requestDetails;
    final communityId = state.communityId;
    if (request == null || communityId == null) return;

    // Read cached contributions from the needs viewmodel. Non-blocking: if the
    // provider hasn't loaded yet, it kicks its own initial load and we proceed
    // with an empty contributor list rather than delaying the modal open.
    final needsState = ref.read(requestNeedsProvider(widget.requestId));
    final requesterId = request.requester.id;
    final contributors = needsState.contributions
        .map((c) => c.contributor)
        .where((u) => u.id != requesterId)
        .toList();

    await MarkFulfilledModal.show(
      context,
      requestId: widget.requestId,
      communityId: communityId,
      requestTitle: request.title,
      ownerId: requesterId,
      offerers: request.offerers.where((u) => u.id != requesterId).toList(),
      contributors: contributors,
      sharedCommunityIds: request.sharedCommunityIds.toSet(),
    );
  }

  void _showShareInviteLink(List<SharedCommunity> communities) {
    final state = ref.read(requestProvider(widget.requestId));
    final request = state.requestDetails;

    if (communities.isEmpty || request == null) return;

    ItemShareSheet.show(
      context,
      itemType: ShareableItemType.request,
      itemId: request.id,
      itemName: request.title,
    );
  }

  Future<void> _handleCancelRequest() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          context.l10n.requestCancelDialogTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          context.l10n.requestCancelDialogBody,
          style: TextStyle(color: AppColors.textSecondary(context)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              context.l10n.commonCancel,
              style: TextStyle(color: AppColors.textSecondary(context)),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(
              context.l10n.requestCancelConfirm,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );

    if (confirmed != true || !mounted) return;

    try {
      await ref
          .read(requestProvider(widget.requestId).notifier)
          .cancelRequest();

      if (!mounted) return;
      ToastHelper.showSuccess(context, context.l10n.requestCancelSuccess);
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(
        context,
        RpcErrorHandler.localize(RpcErrorHandler.classify(e), context.l10n),
      );
    }
  }

  Future<void> _handleUnshareRequest() async {
    final state = ref.read(requestProvider(widget.requestId));
    final communityId = state.communityId;
    if (communityId == null) return;

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          context.l10n.commonUnshareDialogTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          context.l10n.commonUnshareDialogBody,
          style: TextStyle(color: AppColors.textSecondary(context)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              context.l10n.commonCancel,
              style: TextStyle(color: AppColors.textSecondary(context)),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(
              context.l10n.commonUnshareConfirm,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );

    if (confirmed != true || !mounted) return;

    try {
      await ref
          .read(requestSharingProvider.notifier)
          .unshareFromCommunity(widget.requestId, communityId);

      if (!mounted) return;
      ToastHelper.showSuccess(context, context.l10n.commonUnshareSuccess);
      await ref
          .read(requestProvider(widget.requestId).notifier)
          .refreshRequestDetails();
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(
        context,
        RpcErrorHandler.localize(RpcErrorHandler.classify(e), context.l10n),
      );
    }
  }

  Future<void> _handleDeleteRequest() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.surface(context),
        title: Text(
          'Delete Request?',
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        content: Text(
          'Are you sure you want to delete this request? It will be permanently removed.',
          style: TextStyle(color: AppColors.textSecondary(context)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              context.l10n.commonCancel,
              style: TextStyle(color: AppColors.textSecondary(context)),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(
              context.l10n.commonDelete,
              style: const TextStyle(color: Colors.red),
            ),
          ),
        ],
      ),
    );

    if (confirmed != true || !mounted) return;

    try {
      await ref
          .read(requestProvider(widget.requestId).notifier)
          .deleteRequest();

      if (!mounted) return;

      unawaited(
        ref
            .read(observabilityServiceProvider)
            .logAnalyticsEvent(
              RequestDeletedEvent(requestId: widget.requestId),
            ),
      );

      ToastHelper.showSuccess(context, 'Request deleted');
      widget.onDeleted?.call();
    } catch (error) {
      if (!mounted) return;
      ToastHelper.showError(context, error.toString());
    }
  }
}
