import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_helper.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/core/utils/gear_helper.dart';
import 'package:ripls/core/utils/location_picker_helper.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show Estimate, TrackedEstimate, TrackedString;
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GearMetadata;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferType;
import 'package:ripls/data/gen/ripls/api/transfer.pbenum.dart'
    show TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;
import 'package:ripls/presentation/screens/gear/widgets/gear_carousel_media_picker.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_conversation_panel.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_details_panel.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_edit_pane.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_interest_panel.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_location_panel.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_manage_menu_sheet.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_menu_items.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_read_shell.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_transfer_handlers_mixin.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_whos_using_calendar.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_whos_using_panel.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/chat/action_dropdown_menu.dart'
    show ActionDropdownItem;
import 'package:ripls/presentation/widgets/content/access_sheet.dart';
import 'package:ripls/presentation/widgets/content/close_item_modal.dart';
import 'package:ripls/presentation/widgets/content/content_edit_bar.dart';
import 'package:ripls/presentation/widgets/content/content_editing_mixin.dart';
import 'package:ripls/presentation/widgets/content/content_gradient_overlay.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel_launcher.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';
import 'package:ripls/presentation/widgets/content/inline_conversation_view.dart';
import 'package:ripls/presentation/widgets/media/media_picker_button.dart';
import 'package:ripls/presentation/widgets/media/video_background_host.dart';
import 'package:ripls/presentation/widgets/media/video_mute_toggle_button.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart';
import 'package:ripls/services/providers.dart';

/// GearContentView displays the full content for a gear item as a
/// bottom-anchored read shell over the hero ([GearReadShell]). The description,
/// specs, location, and who's-using cards compose the scrolling sheet; the
/// discussion card morph-expands into the conversation; edit mode swaps the
/// shell for [GearEditPane] + the edit bar; owner management lives in the
/// top-bar `···` overflow ([GearManageMenuSheet]). The old tabbed
/// Loan/Details/Chat layout has been removed (#2509).
class GearContentView extends ConsumerStatefulWidget {
  final String gearId;
  final bool showEditControls;
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

  /// Server-computed distance in meters from the discover feed, used to
  /// populate the location card subtitle. Null when opened without distance
  /// context (e.g., deep links).
  final double? initialDistanceMeters;

  /// Deep-link hint: a non-zero value (the legacy "chat" tab index) opens the
  /// conversation panel once the gear has loaded. 0 shows the read shell.
  final int initialTab;

  const GearContentView({
    super.key,
    required this.gearId,
    this.showEditControls = true,
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
  ConsumerState<GearContentView> createState() => _GearContentViewState();
}

class _GearContentViewState extends ConsumerState<GearContentView>
    with ContentEditingMixin, GearTransferHandlersMixin {
  @override
  String get gearId => widget.gearId;

  @override
  VoidCallback? get onDeleted => widget.onDeleted;

  // Metadata editing buffers (specs card, edit mode).
  String _editingBrand = '';
  String _editingModel = '';
  String _editingValueUsd = '';
  String _editingWeightGrams = '';
  String _editingWebsite = '';
  bool _hasInitializedMetadata = false;
  // Cached so it can be used in dispose() where ref.read is forbidden.
  HomeNotifier? _homeNotifier;
  // Debounce timer for auto-saving metadata when owner edits inline fields.
  Timer? _metadataSaveTimer;
  static const _metadataSaveDelay = Duration(milliseconds: 800);

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
          .read(gearProvider(widget.gearId).notifier)
          .initialize(
            gearId: widget.gearId,
            currentUserId: authState.user?.id,
            communityId: widget.initialCommunityId,
            initialDistanceMeters: widget.initialDistanceMeters,
          );

      if (!mounted) return;

      // Deep-link to the conversation (legacy "chat" tab): open the morph
      // conversation panel once the gear has loaded.
      if (widget.initialTab != 0) {
        unawaited(_expandConversation(_defaultPanelRect()));
      }

      ref.listenManual(contentCacheInvalidationProvider, (previous, next) {
        if (!mounted || previous == next) return;
        ref
            .read(gearProvider(widget.gearId).notifier)
            .refreshGearDetails(communityId: null);
      });

      ref.listenManual(transferCacheInvalidationProvider, (previous, next) {
        if (!mounted || previous == next) return;
        ref
            .read(gearProvider(widget.gearId).notifier)
            .refreshGearDetails(communityId: null);
      });

      if (widget.showFeedHeader) {
        // Lock the home nav while editing; an open morph panel locks nav itself
        // via openContentMorphPanel.
        ref.listenManual(gearProvider(widget.gearId), (previous, next) {
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
    _metadataSaveTimer?.cancel();
    _homeNotifier?.unlockNav(silent: true);
    super.dispose();
  }

  // ── Helpers ──────────────────────────────────────────────────────────────

  Color _accentColor(GearState state) {
    final isGiveaway =
        state.gearDetails?.availability ==
        Availability.AVAILABILITY_FOR_GIVEAWAY;
    return isGiveaway
        ? AppColors.giveawayColorOnDark
        : AppColors.loanColorOnDark;
  }

  /// A reasonable morph source rect when there's no tapped card to grow from
  /// (e.g. a deep link straight to the conversation): the lower half of the
  /// screen, so the panel still grows upward rather than from a corner.
  Rect _defaultPanelRect() {
    final size = MediaQuery.of(context).size;
    return Rect.fromLTRB(0, size.height * 0.5, size.width, size.height);
  }

  // ── Build ────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(gearProvider(widget.gearId));

    // Hold the loading view through the whole FIRST hydration pass, not just
    // until gearDetails lands: the location name, owner profile, and transfer
    // context resolve after the details, and painting the read shell in that
    // window shows placeholder facts — "WHERE / TBD", an empty owner avatar,
    // and a false "no one's raised their hand yet" empty state on an already
    // claimed item (#2724). Reloads after the first pass keep the previous
    // hydrated state on screen (hasHydrated stays true), so they never drop
    // back to the spinner.
    // (The gearDetails == null clause keeps the spinner up during a
    // post-error retry, where hasHydrated is already true but there is no
    // previous content to keep on screen.)
    if (state.isLoading && (!state.hasHydrated || state.gearDetails == null)) {
      return ContentViewBuilders.buildLoadingView();
    }

    if (state.error != null) {
      return ContentViewBuilders.buildErrorView(
        title: 'Failed to load gear details',
        errorMessage: RpcErrorHandler.localize(state.error!, context.l10n),
        onRetry: () =>
            ref.read(gearProvider(widget.gearId).notifier).loadGearDetails(),
      );
    }

    final isNavVisible = widget.showFeedHeader
        ? ref.watch(homeProvider).isNavVisible
        : false;

    final accentColor = _accentColor(state);

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
        // 4. Edit bar at top (only in edit mode)
        if (state.isEditing)
          ContentEditBar(
            accentColor: accentColor,
            isNavVisible: isNavVisible,
            onCancel: () {
              ref.read(gearProvider(widget.gearId).notifier).cancelEdit();
              resetEditing();
              _hasInitializedMetadata = false;
            },
            onSave: _handleSaveChanges,
          ),
        // 5. Media picker button (in edit mode for owners).
        if (state.canEditCoverPhoto) _buildMediaPickerOverlay(state),
        // 6. Upload overlay
        if (state.isUploadingMedia) ContentViewBuilders.buildUploadOverlay(),
      ],
    );
  }

  // ── Read shell / edit pane ─────────────────────────────────────────────────

  Widget _buildReadShell(GearState state, bool isNavVisible) {
    final notifier = ref.read(gearProvider(widget.gearId).notifier);
    return GearReadShell(
      gearId: widget.gearId,
      accentColor: _accentColor(state),
      onExpandConversation: _expandConversation,
      onShowLocation: _expandLocation,
      onShowWhosUsing: _expandWhosUsing,
      onShowDetails: _expandDetails,
      onShowCalendar: _expandCalendar,
      onMarkReturned: _markActiveLoanReturned,
      onShowAccess: () => _showAccessSheet(state),
      onInvite: _handleInvite,
      onManage: _showManageSheet,
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

  /// Opens the access sheet ("who can see this") for the gear, mirroring the
  /// request/experience access surfaces so sharing reads the same across item
  /// types (#2492). Built from the gear's shared communities; Invite is
  /// owner-only. Adding communities is handled inside the share sheet, so the
  /// sheet's secondary Add-community action is omitted here.
  Future<void> _showAccessSheet(GearState state) async {
    final gear = state.gearDetails;
    if (gear == null) return;

    final owner = gear.owner;
    final agoStr =
        DateTimeFormatter.formatTimeAgo(gear.createdAtUnixSec.toInt()) ?? '';
    final ownerInitials = _initials(owner.name);

    final otherCommunityCount =
        (gear.totalSharedCommunityCount - state.sharedCommunities.length).clamp(
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
      totalPeople: gear.totalDistinctMemberCount,
      otherCommunityCount: otherCommunityCount,
      onInvitePerson: state.isOwner ? _inviteFromAccessSheet : null,
    );

    if (!mounted) return;
    ref
        .read(gearProvider(widget.gearId).notifier)
        .refreshGearDetails()
        .ignore();
  }

  /// The access sheet's "Invite someone": opens the share sheet, then returns
  /// the gear's refreshed access groups so the still-open sheet reflects any
  /// community/person added (#2492).
  Future<List<AccessGroup>?> _inviteFromAccessSheet() async {
    final gear = ref.read(gearProvider(widget.gearId)).gearDetails;
    if (gear == null) return null;
    await ItemShareSheet.show(
      context,
      itemType: ShareableItemType.gear,
      itemId: gear.id,
      itemName: gear.name,
    );
    if (!mounted) return null;
    await ref.read(gearProvider(widget.gearId).notifier).refreshGearDetails();
    if (!mounted) return null;
    return _buildAccessGroups(ref.read(gearProvider(widget.gearId)));
  }

  /// Converts the gear's shared communities into the [AccessGroup] list the
  /// [AccessSheet] renders.
  List<AccessGroup> _buildAccessGroups(GearState state) {
    final owner = state.gearDetails?.owner;
    if (owner == null) return [];
    final ownerInitials = _initials(owner.name);
    return state.sharedCommunities.map((c) {
      return AccessGroup(
        communityId: c.communityId,
        communityName: c.communityName,
        sharedByName: owner.name.split(' ').first,
        sharedByInitials: ownerInitials,
        sharedTimeAgo:
            DateTimeFormatter.formatTimeAgo(c.sharedAtUnixSec.toInt()) ?? '',
        sharedAtUnixSec: c.sharedAtUnixSec.toInt(),
        memberCount: c.memberCount,
      );
    }).toList();
  }

  /// Opens the gear share sheet (QR + link + invite people / community), after
  /// confirming there's a community the viewer can invite into.
  void _handleInvite() {
    final state = ref.read(gearProvider(widget.gearId));
    final gear = state.gearDetails;
    if (gear == null) return;
    final invitable = CommunityHelper.invitableSharedCommunities(
      state.sharedCommunities,
      ref.read(communitiesProvider),
    );
    if (invitable.isEmpty) {
      ToastHelper.showError(context, context.l10n.pitchingInInviteUnavailable);
      return;
    }
    ItemShareSheet.show(
      context,
      itemType: ShareableItemType.gear,
      itemId: gear.id,
      itemName: gear.name,
    );
  }

  /// "2d ago" / "3h ago" / "5m ago" from a Unix-seconds timestamp.
  /// Up-to-two-letter uppercase initials from a display name.
  String _initials(String name) => name
      .split(' ')
      .where((p) => p.isNotEmpty)
      .take(2)
      .map((p) => p[0])
      .join()
      .toUpperCase();

  Widget _buildEditPane(GearState state) {
    final gear = state.gearDetails;
    if (gear != null) {
      initializeEditing(
        initialTitle: gear.name,
        initialDescription: gear.description,
      );
      _seedMetadataBuffers(state);
    }
    final maxHeight = MediaQuery.of(context).size.height * 0.66;
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
              child: GearEditPane(
                state: state,
                gearId: widget.gearId,
                editingTitle: editingTitle,
                editingDescription: editingDescription,
                onTitleChanged: (v) {
                  if (mounted) updateEditingTitle(v);
                },
                onDescriptionChanged: (v) {
                  if (mounted) updateEditingDescription(v);
                },
                isEditingMetadataInitialized: _hasInitializedMetadata,
                editingBrand: _editingBrand,
                editingModel: _editingModel,
                editingValueUsd: _editingValueUsd,
                editingWeightGrams: _editingWeightGrams,
                editingWebsite: _editingWebsite,
                onBrandChanged: (v) {
                  setState(() => _editingBrand = v);
                  _scheduleMetadataSave();
                },
                onModelChanged: (v) {
                  setState(() => _editingModel = v);
                  _scheduleMetadataSave();
                },
                onValueChanged: (v) {
                  setState(() => _editingValueUsd = v);
                  _scheduleMetadataSave();
                },
                onWeightChanged: (v) {
                  setState(() => _editingWeightGrams = v);
                  _scheduleMetadataSave();
                },
                onWebsiteChanged: (v) => setState(() => _editingWebsite = v),
                onLocationTap: _showLocationModal,
              ),
            ),
          ),
        ),
      ),
    );
  }

  /// Seeds the metadata edit buffers once per edit session.
  void _seedMetadataBuffers(GearState state) {
    if (_hasInitializedMetadata) return;
    final gear = state.gearDetails;
    if (gear == null) return;
    final metadata = gear.hasMetadata() ? gear.metadata : null;
    final valueEstimate = gear.hasValueEstimate() ? gear.valueEstimate : null;
    _hasInitializedMetadata = true;
    _editingBrand = metadata?.brand.value ?? '';
    _editingModel = metadata?.model.value ?? '';
    _editingValueUsd = valueEstimate?.hasEstimatedValueUsd() ?? false
        ? valueEstimate!.estimatedValueUsd.toStringAsFixed(0)
        : '';
    final weightMean = metadata?.weightGrams.value.mean ?? 0;
    _editingWeightGrams = weightMean > 0 ? weightMean.toStringAsFixed(0) : '';
    _editingWebsite = gear.sourceUrl;
  }

  /// Expands the discussion card into the full conversation: a morph-reveal
  /// panel that grows from the card's footprint ([sourceRect]) over the
  /// still-playing hero, replacing the old chat tab. Creates the perpetual gear
  /// conversation on demand if it does not exist yet.
  Future<void> _expandConversation(Rect sourceRect) async {
    final notifier = ref.read(gearProvider(widget.gearId).notifier);
    var state = ref.read(gearProvider(widget.gearId));
    var gear = state.gearDetails;
    if (gear == null) return;

    if (gear.conversationId.isEmpty) {
      await notifier.ensureGearConversation();
      if (!mounted) return;
      state = ref.read(gearProvider(widget.gearId));
      gear = state.gearDetails;
      if (gear == null || gear.conversationId.isEmpty) return;
    }

    final accentColor = _accentColor(state);
    final conversationId = gear.conversationId;
    await openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: gearContentExpandedProvider(widget.gearId),
      sourceRect: sourceRect,
      routeName: 'gear_conversation',
      screen: GearConversationPanel(
        child: InlineConversationView(
          key: ValueKey(conversationId),
          conversationId: conversationId,
          accentColor: accentColor,
          isActive: true,
          isExpanded: true,
          mediaItems: state.allMediaItems,
          onAddMedia: () => _showGearCarouselMediaPicker(notifier),
          onDeleteMedia: (mediaId) => notifier.deleteMedia(mediaId),
          isOwner: state.isOwner,
          onReorderMedia: state.isOwner
              ? (mediaIds) => notifier.reorderMedia(mediaIds)
              : null,
          onMessagesMarkedAsRead: (cid) => resetUnreadForConversation(ref, cid),
        ),
      ),
    );
  }

  /// Morph-expands the WHERE card into the full-screen location panel, growing
  /// from the tapped card's [sourceRect].
  Future<void> _expandLocation(Rect sourceRect) async {
    if (ref.read(gearProvider(widget.gearId)).gearDetails == null) return;
    await openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: gearContentExpandedProvider(widget.gearId),
      sourceRect: sourceRect,
      routeName: 'gear_location',
      screen: GearLocationPanel(gearId: widget.gearId),
    );
  }

  /// Morph-expands the WHO'S-USING / WHO'S-INTERESTED card. A giveaway opens the
  /// interest sign-up sheet ([GearInterestPanel]); a loan opens the roster/story
  /// panel, which renders the per-borrower workflow checklist inline by reusing
  /// [_buildActionMenuItems] (so the actions stay live as state changes).
  Future<void> _expandWhosUsing(Rect sourceRect) async {
    final state = ref.read(gearProvider(widget.gearId));
    if (state.gearDetails == null) return;
    final isGiveaway =
        state.gearDetails!.availability ==
        Availability.AVAILABILITY_FOR_GIVEAWAY;
    await openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: gearContentExpandedProvider(widget.gearId),
      sourceRect: sourceRect,
      routeName: isGiveaway ? 'gear_interest' : 'gear_whos_using',
      screen: isGiveaway
          ? GearInterestPanel(
              gearId: widget.gearId,
              communityId: state.communityId,
              onOpenConversation: () =>
                  _expandConversation(_defaultPanelRect()),
            )
          : GearWhosUsingPanel(
              gearId: widget.gearId,
              accentColor: _accentColor(state),
              actionItemsBuilder: _buildActionMenuItems,
            ),
    );
  }

  /// Morph-expands the specs card into the full-screen details panel.
  Future<void> _expandDetails(Rect sourceRect) async {
    if (ref.read(gearProvider(widget.gearId)).gearDetails == null) return;
    await openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: gearContentExpandedProvider(widget.gearId),
      sourceRect: sourceRect,
      routeName: 'gear_details',
      screen: GearDetailsPanel(gearId: widget.gearId),
    );
  }

  /// Marks the viewer's active loan returned from the who-card CTA. The
  /// transfer id comes from gear.activeLoan — the card only shows this CTA
  /// when the viewer IS the active borrower (#2638 follow-up).
  Future<void> _markActiveLoanReturned() async {
    final gear = ref.read(gearProvider(widget.gearId)).gearDetails;
    if (gear == null || !gear.hasActiveLoan()) return;
    await handleCompleteLoan(gear.activeLoan.transferId);
  }

  /// Opens the full-screen who's-using booking calendar (loan gear). Grows from
  /// the lower half of the screen — there's no single card to morph from (it's
  /// reached from the who's-using card and the sticky CTA).
  Future<void> _expandCalendar() async {
    final state = ref.read(gearProvider(widget.gearId));
    final communityId = state.communityId;
    if (state.gearDetails == null || communityId == null) return;
    await openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: gearContentExpandedProvider(widget.gearId),
      sourceRect: _defaultPanelRect(),
      routeName: 'gear_calendar',
      screen: GearWhosUsingCalendar(
        gearId: widget.gearId,
        communityId: communityId,
        heroMediaId: state.mediaId ?? '',
        onOpenComments: () => _expandConversation(_defaultPanelRect()),
      ),
    );
  }

  // ── Overlays ─────────────────────────────────────────────────────────────

  Widget _buildMediaPickerOverlay(GearState state) {
    return Positioned(
      bottom: MediaQuery.of(context).size.height * 0.55 + 20,
      left: 24,
      right: 24,
      child: MediaPickerButton(
        hasMedia: state.mediaPath != null,
        isUploading: state.isUploadingMedia,
        onTap: _showMediaPickerDialog,
      ),
    );
  }

  // ── Manage sheet ───────────────────────────────────────────────────────────

  Future<void> _showManageSheet() async {
    final state = ref.read(gearProvider(widget.gearId));
    final result = await showAccessibleModal<GearManageAction>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => GearManageMenuSheet.forState(state),
    );
    if (result == null || !mounted) return;

    final isGiveaway =
        state.gearDetails?.availability ==
        Availability.AVAILABILITY_FOR_GIVEAWAY;

    switch (result) {
      case GearManageAction.editDetails:
        ref.read(gearProvider(widget.gearId).notifier).toggleEditMode();
      case GearManageAction.setLocation:
        await _showLocationModal();
      case GearManageAction.viewImpact:
        showGearEquityModal();
      case GearManageAction.logPast:
        await openPastTransferModal(
          state,
          isGiveaway
              ? TransferType.TRANSFER_TYPE_GIVEAWAY
              : TransferType.TRANSFER_TYPE_LOAN,
        );
      case GearManageAction.closeItem:
        await _showCloseItem(state, isGiveaway: isGiveaway);
    }
  }

  Future<void> _showCloseItem(
    GearState state, {
    required bool isGiveaway,
  }) async {
    final ctx = state.transferContext;
    final transfer = ctx?.hasUserTransfer() ?? false ? ctx!.userTransfer : null;
    await CloseItemModal.show(
      context,
      contentType: isGiveaway
          ? CloseItemContentType.item
          : CloseItemContentType.loan,
      onUnshare: state.communityId == null
          ? null
          : handleUnshareGearFromCommunity,
      onCancel: transfer == null
          ? null
          : () => isGiveaway
                ? handleCancelGiveaway(transfer.id)
                : handleCancelLoan(transfer.id),
      onDelete: handleDeleteGear,
    );
  }

  // ── Menu item builders ────────────────────────────────────────────────────

  /// _buildActionMenuItems returns the dropdown items for the sticky action
  /// button — the per-borrower workflow actions (owner confirms / borrower
  /// actions) appropriate to the current user's role and transfer state.
  List<ActionDropdownItem> _buildActionMenuItems(GearState state) {
    final gear = state.gearDetails;
    if (gear == null) return const [];

    final isOwner = state.isOwner;
    final transferContext = state.transferContext;
    final isGiveaway =
        gear.availability == Availability.AVAILABILITY_FOR_GIVEAWAY;

    if (isGiveaway) {
      if (isOwner) {
        return GearMenuItems.buildGiveawayOwnerItems(
          context: context,
          state: state,
          onToggleEditMode: () =>
              ref.read(gearProvider(widget.gearId).notifier).toggleEditMode(),
          onLocationTap: _showLocationModal,
          onOpenTransferModal: openTransferModal,
          onOpenPastTransferModal: () =>
              openPastTransferModal(state, TransferType.TRANSFER_TYPE_GIVEAWAY),
          onShowEquityModal: showGearEquityModal,
          onMarkGiveawayPickedUp: handleMarkGiveawayPickedUp,
          onDeleteGear: handleDeleteGear,
          onCancelTransfer: handleCancelGiveaway,
          onUnshareFromCommunity: state.communityId == null
              ? null
              : handleUnshareGearFromCommunity,
        );
      }
      final isSelectedRecipient =
          (transferContext?.hasUserTransfer() ?? false) &&
          transferContext!.userTransfer.state ==
              TransferState.TRANSFER_STATE_RECIPIENT_SELECTED;
      if (isSelectedRecipient && transferContext.hasUserTransfer()) {
        return GearMenuItems.buildGiveawayRecipientItems(
          context: context,
          transfer: transferContext.userTransfer,
          gearName: gear.name,
          state: state,
          onMarkGiveawayPickedUp: handleMarkGiveawayPickedUp,
          onCancelTransfer: handleCancelLoan,
        );
      }
      return const [];
    }

    // Loan workflow.
    if (isOwner) {
      return GearMenuItems.buildLoanOwnerItems(
        context: context,
        state: state,
        onToggleEditMode: () =>
            ref.read(gearProvider(widget.gearId).notifier).toggleEditMode(),
        onLocationTap: _showLocationModal,
        onOpenPastTransferModal: () =>
            openPastTransferModal(state, TransferType.TRANSFER_TYPE_LOAN),
        onShowEquityModal: showGearEquityModal,
        onStartLoan: handleStartLoan,
        onCompleteLoan: handleCompleteLoan,
        onDeleteGear: handleDeleteGear,
        onCancelTransfer: handleCancelLoan,
        onUnshareFromCommunity: state.communityId == null
            ? null
            : handleUnshareGearFromCommunity,
      );
    }

    final hasRequested = transferContext?.hasUserTransfer() ?? false;
    if (!hasRequested) return const [];

    final userTransfer = transferContext!.userTransfer;
    final activeLoan = gear.hasActiveLoan() ? gear.activeLoan : null;
    final otherBorrowers = transferContext.pendingRequests
        .where((r) => r.transferId != userTransfer.id)
        .length;
    final canAct =
        userTransfer.state == TransferState.TRANSFER_STATE_ACTIVE ||
        (otherBorrowers == 0 && activeLoan == null);

    return GearMenuItems.buildBorrowerLoanItems(
      context: context,
      transfer: userTransfer,
      gearName: gear.name,
      canAct: canAct,
      onStartLoan: handleStartLoan,
      onCompleteLoan: handleCompleteLoan,
      onCancelTransfer: handleCancelLoan,
    );
  }

  // ── Actions / handlers ───────────────────────────────────────────────────

  Future<void> _showGearCarouselMediaPicker(GearNotifier notifier) {
    return showGearCarouselMediaPicker(
      context: context,
      ref: ref,
      gearId: widget.gearId,
      notifier: notifier,
    );
  }

  void _showMediaPickerDialog() {
    final state = ref.read(gearProvider(widget.gearId));
    final notifier = ref.read(gearProvider(widget.gearId).notifier);
    ContentViewHelpers.showMediaPickerDialog(
      context: context,
      hasMedia: state.mediaPath != null,
      onVideoTap: () => notifier.pickVideoFromGallery(insertAtFront: true),
      onPhotoTap: () =>
          notifier.pickMultipleImagesFromGallery(insertAtFront: true),
      onCameraTap: () => notifier.pickImageFromCamera(insertAtFront: true),
    );
  }

  /// _scheduleMetadataSave debounces the inline specs auto-save. Fires 800ms
  /// after the last keystroke so impact metrics update live. The save itself is
  /// routed through the view-model (no direct repository call from the widget).
  void _scheduleMetadataSave() {
    _metadataSaveTimer?.cancel();
    _metadataSaveTimer = Timer(_metadataSaveDelay, () {
      if (!mounted) return;
      ref
          .read(gearProvider(widget.gearId).notifier)
          .saveMetadataSilent(
            brand: _editingBrand,
            model: _editingModel,
            valueUsd: _editingValueUsd,
            weightGrams: _editingWeightGrams,
            website: _editingWebsite,
          );
    });
  }

  Future<void> _handleSaveChanges() async {
    final state = ref.read(gearProvider(widget.gearId));

    if (!hasInitializedEditing && state.gearDetails != null) {
      initializeEditing(
        initialTitle: state.gearDetails!.name,
        initialDescription: state.gearDetails!.description,
      );
    }

    if (!GearHelper.validateGearFields(
      context: context,
      name: editingTitle,
      description: editingDescription,
      locationId: state.gearDetails?.locationId,
    )) {
      return;
    }

    // Build metadata from local edit buffers.
    final brand = _editingBrand.trim();
    final model = _editingModel.trim();
    final weightMean = double.tryParse(_editingWeightGrams.trim());
    final valueUsd = double.tryParse(_editingValueUsd.trim());
    final hasBrand = brand.isNotEmpty;
    final hasModel = model.isNotEmpty;
    final hasWeight = weightMean != null && weightMean > 0;
    final hasValue = valueUsd != null && valueUsd > 0;

    GearMetadata? metadata;
    if (hasBrand || hasModel || hasWeight || hasValue) {
      metadata = GearMetadata(
        brand: hasBrand ? TrackedString(value: brand) : null,
        model: hasModel ? TrackedString(value: model) : null,
        weightGrams: hasWeight
            ? TrackedEstimate(value: Estimate(mean: weightMean))
            : null,
        valueEstimate: hasValue
            ? ValueEstimate(estimatedValueUsd: valueUsd)
            : null,
      );
    }

    final sourceUrl = _editingWebsite.trim().isEmpty
        ? null
        : _editingWebsite.trim();

    try {
      await ref
          .read(gearProvider(widget.gearId).notifier)
          .saveChanges(
            name: editingTitle,
            description: editingDescription,
            metadata: metadata,
            sourceUrl: sourceUrl,
          );
      _hasInitializedMetadata = false;
    } catch (e) {
      if (mounted) ToastHelper.showError(context, 'Failed to save: $e');
    }
  }

  Future<void> _showLocationModal() async {
    final state = ref.read(gearProvider(widget.gearId));
    final gear = state.gearDetails;
    if (gear == null) return;

    final newLocationId = await LocationPickerHelper.showLocationPicker(
      context: context,
      ref: ref,
      locationId: gear.locationId,
      checkIsOwner: () async => ref.read(gearProvider(widget.gearId)).isOwner,
      showDirections: true,
      allowNonOwnerEdit: false,
    );

    if (newLocationId != null && mounted) {
      await ref
          .read(gearProvider(widget.gearId).notifier)
          .updateLocation(newLocationId);
    }
  }
}
