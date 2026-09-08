import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/calendar_helper.dart';
import 'package:ripls/core/utils/community_helper.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/experience.pbenum.dart' as proto;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart' show RSVP;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/experience/location_poll_confirm_modal.dart';
import 'package:ripls/presentation/screens/experience/location_poll_finalized_modal.dart';
import 'package:ripls/presentation/screens/experience/location_poll_propose_modal.dart';
import 'package:ripls/presentation/screens/experience/location_poll_vote_modal.dart';
import 'package:ripls/presentation/screens/experience/time_poll_sheet.dart';
import 'package:ripls/presentation/screens/experience/widgets/carousel_media_picker_sheet.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_bottom_content.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_chat_pane.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_close_handlers_mixin.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_content_panel_launcher.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_conversation_carousel.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_conversation_panel.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_event_pane.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_location_panel.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_time_panel.dart';
import 'package:ripls/presentation/viewmodels/experience_sharing_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/content/access_sheet.dart';
import 'package:ripls/presentation/widgets/content/content_access_pill.dart';
import 'package:ripls/presentation/widgets/content/content_edit_bar.dart';
import 'package:ripls/presentation/widgets/content/content_editing_mixin.dart';
import 'package:ripls/presentation/widgets/content/content_gradient_overlay.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/content/photo_attribution_line.dart';
import 'package:ripls/presentation/widgets/experience/attendee_list_sheet.dart';
import 'package:ripls/presentation/widgets/media/media_picker_button.dart';
import 'package:ripls/presentation/widgets/media/video_background_host.dart';
import 'package:ripls/presentation/widgets/media/video_mute_toggle_button.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_sheet.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart';
import 'package:ripls/presentation/widgets/web/web_unsupported.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('ExperienceContentView');

/// ExperienceContentView displays the full content for an experience using a
/// four-tab layout: Event, Details, Chat, and Media.
class ExperienceContentView extends ConsumerStatefulWidget {
  final String experienceId;
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

  /// Initial tab index to show (0=Event, 1=Details, 2=Chat).
  final int initialTab;

  const ExperienceContentView({
    super.key,
    required this.experienceId,
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
  ConsumerState<ExperienceContentView> createState() =>
      _ExperienceContentViewState();
}

class _ExperienceContentViewState extends ConsumerState<ExperienceContentView>
    with ContentEditingMixin, ExperienceCloseHandlersMixin {
  @override
  String get experienceId => widget.experienceId;
  @override
  VoidCallback? get onDeleted => widget.onDeleted;
  String _editingSourceUrl = '';
  bool _hasInitializedSourceUrl = false;

  // Cached so it can be used in dispose() where ref.read is forbidden.
  HomeNotifier? _homeNotifier;
  // Slide direction for tab transition animation: 1 = left-to-right (going
  // to a higher-index tab), -1 = right-to-left (going to a lower-index tab).
  int _slideDirection = 1;

  @override
  void initState() {
    super.initState();
    if (widget.showFeedHeader) {
      _homeNotifier = ref.read(homeProvider.notifier);
    }

    WidgetsBinding.instance.addPostFrameCallback((_) async {
      if (!mounted) return;
      // Read auth state here, at initialize time, rather than capturing it
      // in initState: a guest completing phone-OTP registration can land
      // auth between mount and this frame, and the early capture would seed
      // a stale null viewer id — the RSVP recorded during registration then
      // never matches the "You" row (#2724).
      final authState = ref.read(authStateProvider);
      await ref
          .read(experienceProvider(widget.experienceId).notifier)
          .initialize(
            experienceId: widget.experienceId,
            currentUserId: authState.user?.id,
            communityId: widget.initialCommunityId,
            initialDistanceMeters: widget.initialDistanceMeters,
          );

      if (!mounted) return;

      // Deep-link (legacy "2 == chat") → open the expanded conversation panel.
      if (widget.initialTab != 0) {
        final details = ref
            .read(experienceProvider(widget.experienceId))
            .experienceDetails;
        if (details != null && details.experience.conversationId.isNotEmpty) {
          final size = MediaQuery.of(context).size;
          _expandConversation(
            Rect.fromLTRB(0, size.height * 0.5, size.width, size.height),
          );
        }
      }

      ref.listenManual(contentCacheInvalidationProvider, (previous, next) {
        if (!mounted || previous == next) return;
        // Skip the refresh once auth is gone — the widget tree is being
        // torn down and a listener-triggered refresh would otherwise race
        // logout and crash the IndexedStack with a duplicate NavigatorState
        // GlobalKey.
        if (ref.read(authStateProvider).user == null) return;
        _log.info(
          '🔔 contentCacheInvalidation fired for ${widget.experienceId}',
        );
        // Debounce per docs/client/caching.md Pattern 7 — chat-side
        // mutations (sendMessage, RSVP, etc.) can fire this listener in
        // bursts. Without debouncing, the cache's stampede-dedup pins the
        // result of the first stale fetch and content stays inconsistent
        // until a manual refresh. Location-poll mutations no longer route
        // through this listener — they apply local patches via
        // [ExperienceNotifier.applyExperiencePatch] instead.
        ref
            .read(experienceProvider(widget.experienceId).notifier)
            .scheduleRefresh();
      });

      if (widget.showFeedHeader) {
        ref.listenManual(experienceProvider(widget.experienceId), (
          previous,
          next,
        ) {
          if (!mounted) return;
          final shouldLock =
              next.isEditing || next.activeTab == next.chatTabIndex;
          final wasLocked =
              (previous?.isEditing ?? false) ||
              (previous != null && previous.activeTab == previous.chatTabIndex);
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

  // ── Tab helpers ──────────────────────────────────────────────────────────

  Color _accentColor() => AppColors.experienceColorOnDark;

  /// _firstTabColor returns the color for the first tab's active pill based on
  /// the state of the action buttons on that tab. The accent color (transferCoral)
  /// means there is something the user still needs to do; the sage-green
  /// completion color means all actions are complete.
  /// Returns null for owners and terminal events (use default accentColor).
  Color? _firstTabColor(ExperienceState state) {
    final exp = state.experienceDetails?.experience;
    if (exp == null || state.isOwner) return null;

    final expState = exp.state;
    final isTerminal =
        expState == proto.ExperienceState.EXPERIENCE_STATE_COMPLETED ||
        expState == proto.ExperienceState.EXPERIENCE_STATE_CANCELLED;
    if (isTerminal) return null;

    final currentUserRsvp = state.experienceDetails!.rsvps.firstWhere(
      (r) => r.user.id == state.currentUserId,
      orElse: () => RSVP(),
    );
    final hasRsvped = currentUserRsvp.hasIntention();
    // Scope to the currently-active poll only — votes from past polls
    // (still in `timeProposals` so users can browse history) would otherwise
    // make this think the user has voted on the new poll already.
    final activePollId = exp.hasCurrentPollId() ? exp.currentPollId : '';
    final hasVoted =
        exp.timePollActive &&
        exp.timeProposals
            .where(
              (p) =>
                  activePollId.isEmpty ||
                  (p.hasPollId() && p.pollId == activePollId),
            )
            .any((p) => p.votes.any((v) => v.user.id == state.currentUserId));
    final needsVote = exp.timePollActive && !hasVoted;

    // Accent color if anything still needs doing; sage-green completion color
    // if all actions are complete.
    if (!hasRsvped || needsVote) return AppColors.transferCoral;
    return AppColors.experienceSageGreen;
  }

  void _onTabChanged(int index, ExperienceState state) {
    setState(() => _slideDirection = index > state.activeTab ? 1 : -1);
    ref
        .read(experienceProvider(widget.experienceId).notifier)
        .setActiveTab(index);
    if (widget.showFeedHeader && index == state.chatTabIndex) {
      ref.read(homeProvider.notifier).hideNav();
    }
  }

  Widget _buildAccessPill(ExperienceState state) {
    final totalPeople = state.experienceDetails?.totalDistinctMemberCount ?? 0;
    return Padding(
      padding: const EdgeInsets.only(left: 8),
      child: ContentAccessPill(
        totalPeople: totalPeople,
        onTap: () => _showAccessSheet(state),
      ),
    );
  }

  Future<void> _showAccessSheet(ExperienceState state) async {
    final exp = state.experienceDetails?.experience;
    if (exp == null) return;

    final owner = exp.owner;
    final agoStr =
        DateTimeFormatter.formatTimeAgo(exp.createdAtUnixSec.toInt()) ?? '';

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

    final totalSharedCommunityCount =
        state.experienceDetails?.totalSharedCommunityCount ?? 0;
    final otherCommunityCount =
        (totalSharedCommunityCount - state.sharedCommunities.length).clamp(
          0,
          999999,
        );

    await AccessSheet.show(
      context,
      creator: AccessCreator(
        name: owner.name,
        initials: ownerInitials,
        timeAgo: agoStr,
      ),
      groups: _buildAccessGroups(state),
      totalPeople: state.experienceDetails?.totalDistinctMemberCount ?? 0,
      otherCommunityCount: otherCommunityCount,
      onAddCommunity: state.isOwner ? _showAddCommunity : null,
      onInvitePerson: invitable.isEmpty ? null : _inviteFromAccessSheet,
    );

    // Clear the sharing notifier's initialization marker so the next
    // access-sheet session re-seeds from fresh server state.
    if (!mounted) return;
    ref.read(experienceSharingNotifierProvider.notifier).clearInitialization();

    // Refresh experience details after the sheet is dismissed so any community
    // sharing changes are reflected next time the sheet opens.
    final communityId = ref
        .read(experienceProvider(widget.experienceId))
        .communityId;
    ref
        .read(experienceProvider(widget.experienceId).notifier)
        .refreshExperienceDetails(communityId: communityId)
        .ignore();
  }

  /// _buildAccessGroups converts the shared communities on [state] into the
  /// [AccessGroup] list consumed by [AccessSheet].
  List<AccessGroup> _buildAccessGroups(ExperienceState state) {
    final owner = state.experienceDetails?.experience.owner;
    if (owner == null) return [];
    final ownerInitials = owner.name
        .split(' ')
        .where((w) => w.isNotEmpty)
        .take(2)
        .map((w) => w[0])
        .join()
        .toUpperCase();
    return state.sharedCommunities.map((c) {
      final sharedAt = DateTime.fromMillisecondsSinceEpoch(
        c.sharedAtUnixSec.toInt() * 1000,
      );
      final sharedAge = DateTime.now().difference(sharedAt);
      final sharedAgo = sharedAge.inDays > 0
          ? '${sharedAge.inDays}d ago'
          : sharedAge.inHours > 0
          ? '${sharedAge.inHours}h ago'
          : '${sharedAge.inMinutes}m ago';
      return AccessGroup(
        communityId: c.communityId,
        communityName: c.communityName,
        sharedByName: owner.name.split(' ').first,
        sharedByInitials: ownerInitials,
        sharedTimeAgo: sharedAgo,
        sharedAtUnixSec: c.sharedAtUnixSec.toInt(),
        memberCount: c.memberCount,
      );
    }).toList();
  }

  /// _showAddCommunity opens the community selection modal, waits for it to
  /// close, then immediately returns an updated [AccessGroup] list derived from
  /// the real-time [experienceSharingNotifierProvider] state (which is already
  /// up-to-date because each toggle fires the API synchronously). The server
  /// sync refresh is deferred to when the outer [AccessSheet] is dismissed.
  Future<List<AccessGroup>?> _showAddCommunity() async {
    if (!mounted) return null;
    final state = ref.read(experienceProvider(widget.experienceId));
    final experience = state.experienceDetails;
    if (experience == null) return null;

    final notifier = ref.read(experienceSharingNotifierProvider.notifier);
    // Only seed from server state on the first open. Subsequent opens within
    // the same access-sheet session preserve the accumulated optimistic state
    // so the modal and sheet stay in sync across repeated trips.
    final currentSharingState = ref.read(experienceSharingNotifierProvider);
    if (currentSharingState.initializedForItemId != experience.experience.id) {
      notifier.setSharedCommunities(
        experience.experience.id,
        experience.experience.sharedCommunityIds,
      );
    }
    await notifier.loadUserCommunities();

    if (!mounted) return null;
    await CommunitySelectionSheet.showForImmediate(
      context,
      itemId: experience.experience.id,
      itemType: 'experience',
      isOwner: true,
      source: CommunitySelectionSource.experienceAccess,
    );

    if (!mounted) return null;

    // Read the sharing provider's optimistic state — it is updated synchronously
    // as each toggle fires, so it is already correct when the modal closes.
    // The server-side refresh is deferred to when the outer AccessSheet dismisses.
    final sharingState = ref.read(experienceSharingNotifierProvider);
    final sharedIds = sharingState.sharedCommunityIds;
    final currentState = ref.read(experienceProvider(widget.experienceId));
    final owner = currentState.experienceDetails?.experience.owner;
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
    final state = ref.watch(experienceProvider(widget.experienceId));

    // Show spinner only while metadata (experienceDetails) is still loading.
    // Once details are available, render immediately with thumbnail background
    // while the video controller initializes in the background.
    if (state.isLoading && state.experienceDetails == null) {
      return ContentViewBuilders.buildLoadingView();
    }

    if (state.error != null) {
      return ContentViewBuilders.buildErrorView(
        title: 'Failed to load experience details',
        errorMessage: RpcErrorHandler.localize(state.error!, context.l10n),
        onRetry: () => ref
            .read(experienceProvider(widget.experienceId).notifier)
            .loadExperienceDetails(),
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
        // 3. Content. Read mode renders the redesigned sheet-over-hero shell
        //    (#2278); edit mode keeps the existing tabbed event pane so the
        //    inline edit flow is unchanged.
        if (state.isEditing)
          _buildBottomContent(state, isNavVisible, accentColor)
        else
          ExperienceConversationCarousel(
            experienceId: widget.experienceId,
            accentColor: accentColor,
            onOpenConversation: _openConversation,
            onExpandConversation: _expandConversation,
            onShowTime: _expandTime,
            onShowLocation: _expandLocation,
            onShowAccess: () => _showAccessSheet(state),
            onManage: showManageSheet,
            onAddMedia: () => _showCarouselMediaPicker(
              ref.read(experienceProvider(widget.experienceId).notifier),
              state.allMediaItems.isNotEmpty,
            ),
            onDeleteMedia: (mediaId) => ref
                .read(experienceProvider(widget.experienceId).notifier)
                .deleteMedia(mediaId),
            onReorderMedia: state.isOwner
                ? (mediaIds) => ref
                      .read(experienceProvider(widget.experienceId).notifier)
                      .reorderMedia(mediaIds)
                : null,
            // Clear the home nav bar that overlays the bottom in feed context.
            bottomNavInset: isNavVisible ? 80 : 0,
          ),
        // 4. Edit bar at top (only in edit mode)
        if (state.isEditing)
          ContentEditBar(
            accentColor: accentColor,
            isNavVisible: isNavVisible,
            onCancel: () {
              ref
                  .read(experienceProvider(widget.experienceId).notifier)
                  .cancelEdit();
              resetEditing();
              setState(() {
                _editingSourceUrl = '';
                _hasInitializedSourceUrl = false;
              });
            },
            onSave: _handleSaveChanges,
          ),
        // 7. Media picker (in edit mode for owners; "Replace background" when
        //    a cover photo already exists, "Select a photo or video" otherwise).
        if (state.canEditCoverPhoto) _buildMediaPickerOverlay(state),
        // 8. Upload overlay
        if (state.isUploading) ContentViewBuilders.buildUploadOverlay(),
        // Mute/unmute toggle (issue #1250) is rendered inside
        // ExperienceBottomContent, just above the tabs alongside the
        // attribution line.
      ],
    );
  }

  // ── Bottom content ───────────────────────────────────────────────────────

  Widget _buildBottomContent(
    ExperienceState state,
    bool isNavVisible,
    Color accentColor,
  ) {
    final attribution = ContentViewHelpers.getBackgroundAttribution(
      mediaId: state.mediaId,
      allMediaItems: state.allMediaItems,
    );
    return ExperienceBottomContent(
      state: state,
      isNavVisible: isNavVisible,
      accentColor: accentColor,
      firstTabColor: _firstTabColor(state),
      slideDirection: _slideDirection,
      accessRingTrailing: _buildAccessPill(state),
      onTabChanged: (i) => _onTabChanged(i, state),
      chatPane: _buildChatPane(state, accentColor),
      eventPane: _buildEventPane(state, accentColor),
      attributionLine: attribution != null
          ? PhotoAttributionLine(attribution: attribution)
          : null,
      muteButton: state.isVideo && !state.isEditing
          ? VideoMuteToggleButton(
              isMuted: state.isMuted,
              onTap: () => ref
                  .read(experienceProvider(widget.experienceId).notifier)
                  .toggleMute(),
            )
          : null,
    );
  }

  // ── Pane 0: Event ────────────────────────────────────────────────────────

  Widget _buildEventPane(ExperienceState state, Color accentColor) {
    final exp = state.experienceDetails?.experience;
    if (exp != null && state.isEditing) {
      initializeEditing(
        initialTitle: exp.name,
        initialDescription: exp.description,
      );
      if (!_hasInitializedSourceUrl) {
        _editingSourceUrl = exp.sourceUrl;
        _hasInitializedSourceUrl = true;
      }
    }
    return ExperienceEventPane(
      experienceId: widget.experienceId,
      state: state,
      accentColor: accentColor,
      editingTitle: editingTitle,
      editingDescription: editingDescription,
      editingSourceUrl: _editingSourceUrl,
      hasInitializedSourceUrl: _hasInitializedSourceUrl,
      onTitleChanged: (v) {
        if (mounted) updateEditingTitle(v);
      },
      onDescriptionChanged: (v) {
        if (mounted) updateEditingDescription(v);
      },
      onSourceUrlChanged: (v) {
        if (mounted) setState(() => _editingSourceUrl = v);
      },
      onShowTimeModal: _showTimeModal,
      onShowTimeModalNonOwner: _showTimeModalNonOwner,
      onShowLocationPickerModal: _showLocationPickerModal,
      onShowAttendeeSheet: () => _showAttendeeSheet(state),
      onDeleteExperience: handleDeleteExperience,
      onCancelExperience: handleCancelExperience,
      onUnshareExperience:
          ref.read(experienceProvider(widget.experienceId)).communityId == null
          ? null
          : handleUnshareExperience,
      onToggleEditMode: () => ref
          .read(experienceProvider(widget.experienceId).notifier)
          .toggleEditMode(),
      onAddToCalendar: _addToCalendar,
    );
  }

  Future<void> _addToCalendar() async {
    // add_2_calendar has no web implementation; show a notice pointing
    // the visitor at the mobile app. See #2157 plugin audit.
    if (WebUnsupported.showCalendarNotice(context)) return;

    final result = await ref
        .read(experienceProvider(widget.experienceId).notifier)
        .exportToCalendar();
    if (result == null) return;
    if (!mounted) return;

    // Widget async exception: platform calendar UI.
    bool success;
    try {
      success = await CalendarHelper.addToCalendar(
        result.experience,
        riplsUrl: result.riplsUrl,
        locationDisplay: result.locationDisplay,
      );
    } catch (e) {
      _log.warning('Calendar export threw', e);
      success = false;
    }
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          success
              ? context.l10n.timeExportOpeningCalendar
              : context.l10n.timeExportFailed,
        ),
      ),
    );
  }

  // ── Pane 1: Chat ─────────────────────────────────────────────────────────

  Widget _buildChatPane(
    ExperienceState state,
    Color accentColor, {
    bool? forceActive,
  }) {
    final notifier = ref.read(experienceProvider(widget.experienceId).notifier);
    return ExperienceChatPane(
      experienceId: widget.experienceId,
      accentColor: accentColor,
      paneHeight: 0, // paneHeight is managed by the parent layout
      forceActive: forceActive,
      onAddMedia: () =>
          _showCarouselMediaPicker(notifier, state.allMediaItems.isNotEmpty),
      onDeleteMedia: (mediaId) => notifier.deleteMedia(mediaId),
      onReorderMedia: state.isOwner
          ? (mediaIds) => notifier.reorderMedia(mediaIds)
          : null,
    );
  }

  // ── Overlays ─────────────────────────────────────────────────────────────

  Widget _buildMediaPickerOverlay(ExperienceState state) {
    return Positioned(
      bottom: MediaQuery.of(context).size.height * 0.48 + 80,
      left: 24,
      right: 24,
      child: MediaPickerButton(
        hasMedia: state.mediaPath != null,
        isUploading: state.isUploading,
        onTap: _showMediaPickerDialog,
      ),
    );
  }

  // ── Volume button ─────────────────────────────────────────────────────────

  // ── Attendee sheet helper ─────────────────────────────────────────────────

  void _showAttendeeSheet(ExperienceState state) {
    if (state.experienceDetails?.experience == null) return;
    final expState = state.experienceDetails!.experience.state;
    AttendeeListSheet.show(
      context,
      widget.experienceId,
      isReadOnly: isExperienceRsvpReadOnly(expState),
    );
  }

  // ── Redesigned read-shell handlers (#2278) ─────────────────────────────────

  /// Opens the discussion inline by switching the active tab to chat. The
  /// carousel ([ExperienceConversationCarousel]) watches the active tab and
  /// slides the conversation in over the persistent hero — no new route, so the
  /// background video keeps playing uninterrupted. In feed context the home nav
  /// bar is hidden while the conversation is up.
  void _openConversation() {
    final state = ref.read(experienceProvider(widget.experienceId));
    ref
        .read(experienceProvider(widget.experienceId).notifier)
        .setActiveTab(state.chatTabIndex);
    if (widget.showFeedHeader) {
      ref.read(homeProvider.notifier).hideNav();
    }
  }

  /// Expands the discussion ("comments") card into the full conversation: a
  /// morph-reveal panel that grows from the card's footprint ([rect]) over the
  /// still-playing hero, replacing the rest of the content (docs/client/modals.md).
  /// Reuses the same shared launcher as the "Who's pitching in?" roster.
  void _expandConversation(Rect rect) {
    final state = ref.read(experienceProvider(widget.experienceId));
    openExperienceContentPanel(
      context: context,
      ref: ref,
      experienceId: widget.experienceId,
      sourceRect: rect,
      routeName: 'experience_conversation',
      screen: ExperienceConversationPanel(
        // On its own surface (no active-tab signal) → force conversation init.
        child: _buildChatPane(state, _accentColor(), forceActive: true),
      ),
    );
  }

  /// Expands the location ("WHERE") card into the full-screen location panel —
  /// a morph-reveal panel grown from the card's footprint ([rect]) over the
  /// hero (docs/client/modals.md), via the shared content-panel launcher.
  void _expandLocation(Rect rect) {
    // Refresh the panel view-model so it sees current poll + proposal state.
    ref.invalidate(locationModalProvider(widget.experienceId));
    openExperienceContentPanel(
      context: context,
      ref: ref,
      experienceId: widget.experienceId,
      sourceRect: rect,
      routeName: 'experience_location',
      screen: ExperienceLocationPanel(
        experienceId: widget.experienceId,
        accentColor: _accentColor(),
        onMarkCompleted: markCompleted,
        onCloseEvent: closeEvent,
      ),
    );
  }

  /// Expands the time ("WHEN") card into the full-screen time panel, morphing
  /// from the card's footprint ([rect]) over the hero (docs/client/modals.md).
  void _expandTime(Rect rect) {
    ref.invalidate(timeModalProvider(widget.experienceId));
    openExperienceContentPanel(
      context: context,
      ref: ref,
      experienceId: widget.experienceId,
      sourceRect: rect,
      routeName: 'experience_time',
      screen: ExperienceTimePanel(
        experienceId: widget.experienceId,
        accentColor: _accentColor(),
        onMarkCompleted: markCompleted,
        onCloseEvent: closeEvent,
      ),
    );
  }

  // ── Actions / handlers ───────────────────────────────────────────────────

  void _showMediaPickerDialog() {
    final state = ref.read(experienceProvider(widget.experienceId));
    final hasMedia =
        state.experienceDetails?.experience.mediaIds.isNotEmpty ?? false;
    final notifier = ref.read(experienceProvider(widget.experienceId).notifier);

    ContentViewHelpers.showMediaPickerDialog(
      context: context,
      hasMedia: hasMedia,
      onVideoTap: () => notifier.pickVideoFromGallery(insertAtFront: true),
      onPhotoTap: () =>
          notifier.pickMultipleImagesFromGallery(insertAtFront: true),
      onCameraTap: () => notifier.pickImageFromCamera(insertAtFront: true),
    );
  }

  /// _showCarouselMediaPicker shows the media picker from within the carousel
  /// and awaits the upload so that the carousel can refresh its thumbnail strip.
  Future<void> _showCarouselMediaPicker(
    ExperienceNotifier notifier,
    bool hasMedia,
  ) async {
    Future<void>? uploadFuture;

    await showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (sheetContext) {
        return CarouselMediaPickerSheet(
          hasMedia: hasMedia,
          onVideoTap: () {
            Navigator.of(sheetContext).pop();
            uploadFuture = notifier.pickVideoFromGallery();
          },
          onPhotoTap: () {
            Navigator.of(sheetContext).pop();
            uploadFuture = notifier.pickMultipleImagesFromGallery();
          },
          onCameraTap: () {
            Navigator.of(sheetContext).pop();
            uploadFuture = notifier.pickImageFromCamera();
          },
        );
      },
    );

    // Await the upload that was started (if any) so the caller's Future only
    // completes once the upload is done and state has been updated.
    if (uploadFuture != null) {
      await uploadFuture;
      if (!mounted) return;
      final failedCount = ref
          .read(experienceProvider(widget.experienceId))
          .batchUploadFailedCount;
      if (failedCount != null && failedCount > 0) {
        ToastHelper.showError(
          context,
          context.l10n.mediaBatchUploadPartialFailure(failedCount),
        );
      }
    }
  }

  Future<void> _handleSaveChanges() async {
    if (editingTitle.trim().isEmpty) {
      ToastHelper.showError(context, 'Experience name is required');
      return;
    }
    if (editingDescription.trim().isEmpty) {
      ToastHelper.showError(context, 'Description is required');
      return;
    }
    final state = ref.read(experienceProvider(widget.experienceId));
    if (state.experienceDetails?.experience.locationId.isEmpty ?? true) {
      ToastHelper.showError(context, 'Location is required');
      return;
    }

    try {
      await ref
          .read(experienceProvider(widget.experienceId).notifier)
          .saveChanges(
            name: editingTitle,
            description: editingDescription,
            sourceUrl: _editingSourceUrl.trim().isEmpty
                ? null
                : _editingSourceUrl.trim(),
          );
    } catch (e) {
      if (mounted) ToastHelper.showError(context, 'Failed to save: $e');
    }
  }

  void _showLocationPickerModal(String? locationId) async {
    _log.info(
      '📍 Opening location modal for experience: ${widget.experienceId}',
    );

    // Refresh the modal view-model so it sees current poll + proposal state.
    ref.invalidate(locationModalProvider(widget.experienceId));

    final state = ref.read(experienceProvider(widget.experienceId));
    final exp = state.experienceDetails?.experience;

    // Read-mode taps expand the location panel; this dispatch is reached only
    // from the owner-only edit pane (no non-owner fallthrough needed).
    if (!mounted) return;

    final hasConfirmedLocation = exp != null && exp.locationId.isNotEmpty;

    if (hasConfirmedLocation && exp.locationPollActive != true) {
      // Confirmed spot, no active re-vote → "It's a plan." (LX2).
      await LocationPollFinalizedModal.show(context, widget.experienceId);
    } else if (exp?.locationPollActive ?? false) {
      // Active poll → vote.
      await LocationPollVoteModal.show(context, widget.experienceId);
    } else if ((exp?.hasLocationPollCompleted() ?? false) &&
        exp!.locationPollCompleted &&
        exp.locationId.isEmpty) {
      // Poll ended without a confirmed winner → owner picks one. If they
      // do, land on "It's a plan" so they don't fall back to the
      // intermediate confirm-list view.
      final confirmed = await LocationPollConfirmModal.show(
        context,
        widget.experienceId,
      );
      if ((confirmed ?? false) && mounted) {
        await LocationPollFinalizedModal.show(context, widget.experienceId);
      }
    } else {
      // Empty location (or any other organizer-add path) → "Pick a spot".
      await LocationPollProposeModal.show(context, widget.experienceId);
    }

    if (mounted) {
      await ref
          .read(experienceProvider(widget.experienceId).notifier)
          .loadExperienceDetails();
    }
  }

  /// State-aware time-chip dispatch — mirrors the location chip exactly.
  /// Active poll → vote modal. Confirmed time → "It's Set" finalized
  /// modal. Otherwise → propose modal, which exposes both the unified
  /// calendar+time picker (via the big "Add your first time" card) and
  /// the LLM bulk-paste hatch. The Finalized modal's "Change → Pick a
  /// different time" path is the only place that opens the picker
  /// directly, matching location's "Pick a different spot" flow.
  void _showTimeModal() async {
    ref.invalidate(timeModalProvider(widget.experienceId));
    // One morphing sheet handles every state (tbd / poll / set) and morphs
    // in place as the poll state changes.
    await TimePollSheet.show(context, widget.experienceId);

    if (mounted) {
      await ref
          .read(experienceProvider(widget.experienceId).notifier)
          .loadExperienceDetails();
    }
  }

  /// Non-owner time-chip dispatch from the edit pane — same morphing sheet as
  /// the owner path (the [pollActive] hint is unused).
  void _showTimeModalNonOwner(bool pollActive) => _showTimeModal();

  /// The access sheet's "Invite someone": opens the share sheet, then returns
  /// the event's refreshed access groups so the still-open sheet reflects any
  /// community/person added (#2492).
  Future<List<AccessGroup>?> _inviteFromAccessSheet() async {
    final experience = ref
        .read(experienceProvider(widget.experienceId))
        .experienceDetails
        ?.experience;
    if (experience == null) return null;
    await ItemShareSheet.show(
      context,
      itemType: ShareableItemType.experience,
      itemId: experience.id,
      itemName: experience.name,
    );
    if (!mounted) return null;
    final communityId =
        ref.read(experienceProvider(widget.experienceId)).communityId;
    await ref
        .read(experienceProvider(widget.experienceId).notifier)
        .refreshExperienceDetails(communityId: communityId);
    if (!mounted) return null;
    return _buildAccessGroups(
      ref.read(experienceProvider(widget.experienceId)),
    );
  }
}
