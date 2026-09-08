import 'package:cross_file/cross_file.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/content_type_helper.dart';
import 'package:ripls/core/utils/media_picker_helper.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/communities/invite_sheet.dart';
import 'package:ripls/presentation/screens/communities/widgets/community_chat_pane.dart';
import 'package:ripls/presentation/screens/communities/widgets/community_members_pane.dart';
import 'package:ripls/presentation/viewmodels/community_content_state.dart';
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart';
import 'package:ripls/presentation/viewmodels/community_edit_view_model.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/chat/morphing_action_button.dart';
import 'package:ripls/presentation/widgets/community/member_preview_row.dart';
import 'package:ripls/presentation/widgets/content/content_avatar.dart';
import 'package:ripls/presentation/widgets/content/content_edit_bar.dart';
import 'package:ripls/presentation/widgets/content/content_editing_mixin.dart';
import 'package:ripls/presentation/widgets/content/content_gradient_overlay.dart';
import 'package:ripls/presentation/widgets/content/content_info_row.dart';
import 'package:ripls/presentation/widgets/content/content_owner_row.dart';
import 'package:ripls/presentation/widgets/content/content_shared_widgets.dart';
import 'package:ripls/presentation/widgets/content/content_top_rows.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/content/content_view_pane_switcher.dart';
import 'package:ripls/presentation/widgets/content/content_view_tab_bar.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';
import 'package:ripls/presentation/widgets/content/photo_attribution_line.dart';
import 'package:ripls/presentation/widgets/media/media_picker_button.dart';
import 'package:ripls/presentation/widgets/media/video_background_host.dart';
import 'package:ripls/presentation/widgets/media/video_mute_toggle_button.dart';
import 'package:ripls/services/providers.dart';

/// CommunityCreationFeedView displays community creation events in the feed.
/// Shows community details with inline editing capabilities for creators.
class CommunityContentView extends ConsumerStatefulWidget {
  final String communityId;
  final String communityName;
  final String communityDescription;
  final User? actor;
  final String? mediaId;
  final bool canEdit;
  final int occurredAtUnixSec;

  /// Initial tab index (0=Community, 1=Members, 2=Discuss). Used when
  /// navigating from outside the feed, e.g. from the portfolio inbox pin.
  final int initialTab;

  // Feed header parameters (optional, only used in feed context)
  final bool showFeedHeader;
  final User? feedActor;
  final int? feedOccurredAtUnixSec;
  final String? feedActionText;

  const CommunityContentView({
    super.key,
    required this.communityId,
    required this.communityName,
    this.communityDescription = '',
    this.actor,
    this.mediaId,
    this.canEdit = false,
    this.occurredAtUnixSec = 0,
    this.initialTab = 0,
    // Feed header (disabled by default)
    this.showFeedHeader = false,
    this.feedActor,
    this.feedOccurredAtUnixSec,
    this.feedActionText,
  });

  @override
  ConsumerState<CommunityContentView> createState() =>
      _CommunityCreationFeedViewState();
}

class _CommunityCreationFeedViewState
    extends ConsumerState<CommunityContentView>
    with ContentEditingMixin {
  int _slideDirection = 1;

  void _onTabChanged(int index) {
    final current = ref.read(communityContentProvider).activeTab;
    _slideDirection = index > current ? 1 : -1;
    ref.read(communityContentProvider.notifier).setActiveTab(index);
    if (index == 2) {
      ref
          .read(communityContentProvider.notifier)
          .loadConversation(widget.communityId);
    }
  }

  @override
  void initState() {
    super.initState();
    initializeEditing(
      initialTitle: widget.communityName,
      initialDescription: widget.communityDescription,
    );

    WidgetsBinding.instance.addPostFrameCallback((_) {
      // Load members/gear/events for the Members tab.
      ref
          .read(communityContentProvider.notifier)
          .initialize(widget.communityId);

      if (widget.initialTab != 0) {
        _onTabChanged(widget.initialTab);
      }

      _loadCommunityMedia();
    });

    if (widget.showFeedHeader) {
      ref.listenManual(communityEditProvider(widget.communityId), (
        previous,
        next,
      ) {
        if (!mounted) return;
        if (previous?.isEditing != next.isEditing) {
          if (next.isEditing) {
            ref.read(homeProvider.notifier).hideNav();
          } else {
            ref.read(homeProvider.notifier).showNav();
          }
        }
      });
    }
  }

  /// Fetches the community from the repository and loads all its media items
  /// for the carousel + the first item as the background. The widget receives
  /// only the legacy singular [widget.mediaId], but a community can accumulate
  /// multiple media via [CommunityEditNotifier.uploadMediaAndAppend]; relying
  /// on `widget.mediaId` alone would shrink the carousel back to one item
  /// every time the autoDispose state rebuilds.
  ///
  /// `CommunityRepository.updateCommunity` invalidates the per-community cache,
  /// so this `get()` will refetch fresh `mediaIds` after any upload/delete/
  /// reorder.
  Future<void> _loadCommunityMedia() async {
    final notifier = ref.read(
      communityEditProvider(widget.communityId).notifier,
    );
    try {
      final community = await ref
          .read(communityRepositoryProvider)
          .get(widget.communityId);
      if (!mounted) return;
      final ids = community.mediaIds;
      if (ids.isEmpty) return;
      await notifier.loadAllMedia(ids);
      if (!mounted) return;
      await notifier.loadBackgroundMedia(ids.first);
      if (!mounted) return;
      await notifier.loadBackgroundAttribution(ids.first);
    } catch (_) {
      // Fall back to the singular legacy mediaId if the community fetch
      // fails (e.g. offline). This preserves prior behavior for that path.
      if (!mounted) return;
      final fallback = widget.mediaId;
      if (fallback != null && fallback.isNotEmpty) {
        await notifier.loadAllMedia([fallback]);
        if (!mounted) return;
        await notifier.loadBackgroundMedia(fallback);
        if (!mounted) return;
        await notifier.loadBackgroundAttribution(fallback);
      }
    }
  }

  @override
  void didUpdateWidget(CommunityContentView oldWidget) {
    super.didUpdateWidget(oldWidget);

    // Reinitialize editing buffer if name or description changed
    if (widget.communityName != oldWidget.communityName ||
        widget.communityDescription != oldWidget.communityDescription) {
      resetEditing();
      initializeEditing(
        initialTitle: widget.communityName,
        initialDescription: widget.communityDescription,
      );
    }
  }

  void _toggleEditMode() {
    final notifier = ref.read(
      communityEditProvider(widget.communityId).notifier,
    );
    final state = ref.read(communityEditProvider(widget.communityId));

    if (state.isEditing) {
      // Cancel editing - reset editing buffer
      resetEditing();
      initializeEditing(
        initialTitle: widget.communityName,
        initialDescription: widget.communityDescription,
      );
      notifier.cancelEditing(); // This also clears newMediaId in state
    } else {
      // Start editing
      notifier.startEditing();
    }
  }

  void _showMediaPickerDialog() {
    ContentViewHelpers.showMediaPickerDialog(
      context: context,
      hasMedia: widget.mediaId != null && widget.mediaId!.isNotEmpty,
      onVideoTap: _pickVideoFromGallery,
      onPhotoTap: _pickImageFromGallery,
      onCameraTap: _pickImageFromCamera,
    );
  }

  Future<void> _pickImageFromGallery() async {
    final file = await MediaPickerHelper.pickImageFromGallery();
    if (file != null) {
      await _uploadMedia(file);
    }
  }

  Future<void> _pickVideoFromGallery() async {
    final file = await MediaPickerHelper.pickVideoFromGallery();
    if (file != null) {
      await _uploadMedia(file);
    }
  }

  Future<void> _pickImageFromCamera() async {
    final file = await MediaPickerHelper.pickImageFromCamera();
    if (file != null) {
      await _uploadMedia(file);
    }
  }

  Future<void> _uploadMedia(XFile file) async {
    final notifier = ref.read(
      communityEditProvider(widget.communityId).notifier,
    );

    try {
      await notifier.uploadMedia(file);
    } catch (e) {
      rethrow;
    }
  }

  Future<void> _saveChanges() async {
    final notifier = ref.read(
      communityEditProvider(widget.communityId).notifier,
    );

    final success = await notifier.saveCommunity(
      communityId: widget.communityId,
      name: editingTitle.trim(),
      description: editingDescription.trim(),
    );

    if (mounted) {
      if (success) {
        // Refresh communities list to update sidebar and home screen title
        final communities = await ref
            .read(communityRepositoryProvider)
            .listUserCommunities();
        await ref
            .read(communitiesProvider.notifier)
            .setCommunities(communities);

        // Invalidate feed cache and refresh to show updated content
        await ref.read(feedRepositoryProvider).invalidateFeed();
        await ref.read(feedProvider.notifier).refresh();
      } else {
        final error = ref.read(communityEditProvider(widget.communityId)).error;
        ToastHelper.showError(
          context,
          error == null
              ? 'Failed to update community'
              : RpcErrorHandler.localize(error, context.l10n),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final editState = ref.watch(communityEditProvider(widget.communityId));
    final contentState = ref.watch(communityContentProvider);
    final isEditing = editState.isEditing;
    final isSaving = editState.isSaving;
    final newMediaId = editState.newMediaId;
    final isUploadingMedia = editState.isUploadingMedia;
    final effectiveMediaId = newMediaId ?? widget.mediaId;

    final isNavVisible = widget.showFeedHeader
        ? ref.watch(homeProvider).isNavVisible
        : false;

    return Stack(
      fit: StackFit.expand,
      children: [
        // 1. Background media
        VideoBackgroundHost(
          mediaPath: editState.mediaPath,
          mediaId: effectiveMediaId,
          isVideo: editState.isVideo,
          thumbnailUrl: editState.backgroundThumbnailUrl,
          isMuted: editState.isMuted,
        ),
        // 2. Gradient overlay
        ContentGradientOverlay(mediaCacheKey: effectiveMediaId),
        // 3. Bottom content (tab bar + panes)
        _buildBottomContent(
          editState,
          contentState,
          isEditing,
          isSaving,
          isNavVisible,
        ),
        // 4. Owner row or Edit bar at top (hidden in Discuss tab)
        if (isEditing)
          ContentEditBar(
            accentColor: AppColors.primary(context),
            isNavVisible: isNavVisible,
            onCancel: _toggleEditMode,
            onSave: _saveChanges,
          )
        else if (!widget.showFeedHeader &&
            contentState.activeTab != 1 &&
            widget.actor != null)
          ContentOwnerRow(
            owner: widget.actor!,
            subtitle: context.l10n.communityContentCreatedSubtitle,
            accentColor: AppColors.primary(context),
          ),
        // 5. Feed header
        if (!isEditing &&
            widget.showFeedHeader &&
            contentState.activeTab != 1 &&
            widget.feedActor != null &&
            widget.feedOccurredAtUnixSec != null &&
            widget.feedActionText != null)
          ContentViewBuilders.buildFeedHeader(
            context: context,
            actor: widget.feedActor!,
            occurredAtUnixSec: widget.feedOccurredAtUnixSec!,
            actionText: widget.feedActionText!,
            isNavVisible: isNavVisible,
          ),
        // 6. Media picker overlay (edit mode, no media yet)
        if (isEditing &&
            editState.mediaPath == null &&
            effectiveMediaId == null)
          _buildMediaPickerOverlay(editState),
        // 7. Upload overlay
        if (isUploadingMedia) ContentViewBuilders.buildUploadOverlay(),
      ],
    );
  }

  // ── Bottom content ──────────────────────────────────────────────────────

  Widget _buildBottomContent(
    CommunityEditState editState,
    CommunityContentState contentState,
    bool isEditing,
    bool isSaving,
    bool isNavVisible,
  ) {
    final bottomPadding = isNavVisible ? 80.0 : 24.0;
    final screenHeight = MediaQuery.of(context).size.height;
    final chatPaneHeight = (screenHeight * 0.73).clamp(380.0, 700.0);

    return Positioned(
      left: 0,
      right: 0,
      bottom: 0,
      // The caption column (#2912): the panel holds the reading measure over
      // the full-bleed hero on desktop-wide windows; no-op at phone widths.
      child: ContentColumn(
        child: HeroContentWash(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (isEditing)
                _buildEditPane(isSaving)
              else ...[
                if (editState.backgroundAttribution != null ||
                    _buildVolumeButton(editState) != null)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(16, 0, 16, 4),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        if (editState.backgroundAttribution != null)
                          Expanded(
                            child: PhotoAttributionLine(
                              attribution: editState.backgroundAttribution!,
                            ),
                          )
                        else
                          const Spacer(),
                        if (_buildVolumeButton(editState) != null) ...[
                          const SizedBox(width: 8),
                          _buildVolumeButton(editState)!,
                        ],
                      ],
                    ),
                  ),
                ContentViewTabBar(
                  firstTabLabel: ContentTypeHelper.getCommunityLabel(),
                  secondTabLabel: context.l10n.communityTabMembers,
                  thirdTabLabel: context.l10n.communityTabDiscuss,
                  conversationId:
                      contentState.communityConversation?.conversationId,
                  serverUnreadCount:
                      contentState.communityConversation?.unreadCount ?? 0,
                  totalMessageCount: 0,
                  activeIndex: contentState.activeTab,
                  onChanged: _onTabChanged,
                  accentColor: AppColors.primary(context),
                  isEditing: false,
                ),
                ContentViewPaneSwitcher(
                  activeIndex: contentState.activeTab,
                  slideDirection: _slideDirection,
                  onTabChanged: _onTabChanged,
                  paneBuilders: [
                    (_) => _buildReadPane(),
                    (_) =>
                        CommunityMembersPane(communityId: widget.communityId),
                    (_) => SizedBox(
                      height: chatPaneHeight,
                      child: CommunityChatPane(
                        communityId: widget.communityId,
                        conversationId:
                            contentState.communityConversation?.conversationId,
                        paneHeight: chatPaneHeight,
                        unreadCount:
                            contentState.communityConversation?.unreadCount ??
                            0,
                        mediaItems: editState.allMediaItems,
                        onAddMedia: _showCommunityCarouselMediaPicker,
                        onDeleteMedia: _handleDeleteMedia,
                        onReorderMedia: widget.canEdit
                            ? _handleReorderMedia
                            : null,
                        isOwner: widget.canEdit,
                      ),
                    ),
                  ],
                ),
              ],
              SizedBox(height: bottomPadding),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildReadPane() {
    final actor = widget.actor;
    final firstName = actor?.name.split(' ').first ?? '';
    final relative = widget.occurredAtUnixSec > 0
        ? _relativeAgo(widget.occurredAtUnixSec)
        : '';
    final rows = <ContentInfoRow>[];
    if (actor != null && firstName.isNotEmpty) {
      rows.add(
        ContentInfoRow(
          leading: ContentAvatar(
            user: actor,
            size: 18,
            backgroundColor: AppColors.transferCoral,
          ),
          value: firstName,
          rowSemanticsLabel: firstName,
          trailingText: relative.isNotEmpty
              ? context.l10n.contentRowsSharedRelativeAgo(relative)
              : null,
          onTap: () => ContentViewHelpers.openUserScreen(context, actor.id),
          showDivider: false,
        ),
      );
    }
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          ContentTopRows(
            title: widget.communityName,
            descriptionSlot: ContentExpandableDescription(
              description: widget.communityDescription,
            ),
            rows: rows,
          ),
          const SizedBox(height: 14),
          _buildTypeActions(),
        ],
      ),
    );
  }

  String _relativeAgo(int unixSec) {
    if (unixSec <= 0) return '';
    final dt = DateTime.fromMillisecondsSinceEpoch(unixSec * 1000);
    final age = DateTime.now().difference(dt);
    if (age.inDays > 0) return '${age.inDays}d';
    if (age.inHours > 0) return '${age.inHours}h';
    if (age.inMinutes > 0) return '${age.inMinutes}m';
    return '${age.inSeconds}s';
  }

  Widget _buildEditPane(bool isSaving) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 32, 16, 8),
      child: SafeArea(
        top: false,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            ContentViewBuilders.buildEditableTitle(
              value: editingTitle,
              onChanged: updateEditingTitle,
              enabled: !isSaving,
              label: context.l10n.communityEditNameLabel,
              hintText: context.l10n.communityEditNameHint,
            ),
            const SizedBox(height: 8),
            ContentViewBuilders.buildEditableDescription(
              value: editingDescription,
              onChanged: updateEditingDescription,
              enabled: !isSaving,
              maxLines: 8,
              minLines: 3,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildTypeActions() {
    final members = ref.watch(communityContentProvider).members;
    return Row(
      children: [
        MemberPreviewRow(
          members: members,
          totalCount: members.length,
          onTap: () => _onTabChanged(1),
          avatarSize: 24,
          avatarOverlap: 8,
          namesFontSize: 12,
          overflowFontSize: 10,
          textColor: OverlayTokens.textPrimary,
          borderColor: Colors.black.withValues(alpha: 0.4),
          overflowBackgroundColor: OverlayTokens.chipFill,
          semanticsLabel: context.l10n.communityMemberCount(members.length),
        ),
        const Spacer(),
        MorphingActionButton(
          preActionLabel: context.l10n.communityInvite,
          postActionLabel: context.l10n.communityInvite,
          status: ActionButtonStatus.preAction,
          onAction: _showShareInviteLink,
          menuItems: const [],
          flat: false,
        ),
      ],
    );
  }

  // ── Overlays ────────────────────────────────────────────────────────────

  Widget? _buildVolumeButton(CommunityEditState editState) {
    if (editState.isEditing || !editState.isVideo) {
      return null;
    }
    return VideoMuteToggleButton(
      isMuted: editState.isMuted,
      onTap: () => ref
          .read(communityEditProvider(widget.communityId).notifier)
          .toggleMute(),
    );
  }

  Widget _buildMediaPickerOverlay(CommunityEditState editState) {
    return Positioned(
      bottom: MediaQuery.of(context).size.height * 0.55 + 20,
      left: 24,
      right: 24,
      child: MediaPickerButton(
        hasMedia: widget.mediaId != null && widget.mediaId!.isNotEmpty,
        isUploading: editState.isUploadingMedia,
        onTap: _showMediaPickerDialog,
      ),
    );
  }

  // ── Actions ─────────────────────────────────────────────────────────────

  void _showShareInviteLink() {
    showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (context) => InviteSheet(
        communityId: widget.communityId,
        communityName: widget.communityName,
        onClose: () => Navigator.of(context).pop(),
      ),
    );
  }

  Future<void> _handleDeleteMedia(String mediaId) async {
    final notifier = ref.read(
      communityEditProvider(widget.communityId).notifier,
    );
    final editState = ref.read(communityEditProvider(widget.communityId));
    final currentMediaIds = editState.allMediaItems
        .map((item) => item.id)
        .toList();
    await notifier.deleteMedia(mediaId, currentMediaIds);
  }

  Future<void> _handleReorderMedia(List<String> mediaIds) async {
    final notifier = ref.read(
      communityEditProvider(widget.communityId).notifier,
    );
    await notifier.reorderMedia(mediaIds);
  }

  /// Shows the carousel-style media picker bottom sheet. Mirrors gear's
  /// _showGearCarouselMediaPicker: each option pops the sheet and assigns
  /// uploadFuture, which is awaited after the modal closes. Used by both the
  /// chat pane's media-strip "+ Add" and the gallery carousel's add page;
  /// any community member can append (not creator-gated).
  Future<void> _showCommunityCarouselMediaPicker() async {
    final notifier = ref.read(
      communityEditProvider(widget.communityId).notifier,
    );
    Future<void>? uploadFuture;

    await showAccessibleModal<void>(
      context,
      builder: (sheetContext) {
        return Container(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text(
                'Add Media',
                style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
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
                  uploadFuture = notifier.pickImageFromGallery();
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
      try {
        await uploadFuture;
      } catch (e) {
        if (mounted) {
          ToastHelper.showError(context, 'Failed to upload media: $e');
        }
      }
    }
  }
}
