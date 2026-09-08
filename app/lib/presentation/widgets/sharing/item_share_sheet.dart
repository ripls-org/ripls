import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_sheet.dart';
import 'package:ripls/presentation/widgets/sharing/invite_members_sheet.dart';
import 'package:ripls/services/providers.dart';
import 'package:share_plus/share_plus.dart';

final logger = Logger('ItemShareSheet');

/// Type of item that can be shared via invite link.
enum ShareableItemType { gear, request, experience }

/// Audience action chosen inside the share sheet that requires a follow-up
/// picker. The sheet pops itself with one of these as its result and the
/// static [ItemShareSheet.show] presents the picker — one sheet at a time,
/// never a picker stacked on the still-open share sheet (#2724).
enum _AudienceFollowUp { invitePeople, addCommunity }

/// Modal bottom sheet for sharing an item (gear, request, experience) via its
/// per-item ad-hoc community (#2492).
///
/// On open the sheet finds-or-provisions the item's per-item community via
/// [CommunityRepository.shareItem] and renders the resulting open link as a QR
/// code, a truncated URL, and Copy/Share buttons. Below the link two action
/// rows expand the audience: "Invite people" (phone/email + host relay) and
/// "Add Community" (multi-select existing communities). Any member of a
/// community the item is shared with can open the sheet and reshare the link;
/// the audience rows are shown only when the server says the caller may manage
/// the audience (the item's owner — #2630).
class ItemShareSheet extends ConsumerStatefulWidget {
  final ShareableItemType itemType;
  final String itemId;
  final String itemName;

  const ItemShareSheet({
    super.key,
    required this.itemType,
    required this.itemId,
    required this.itemName,
  });

  /// Opens the share sheet (QR code + invite link) for [itemType]/[itemId]. The
  /// sheet finds-or-provisions the item's per-item community itself, so callers
  /// only pass the item.
  ///
  /// The audience rows ("Invite people" / "Share to communities") dismiss the
  /// sheet and hand back a [_AudienceFollowUp]; the picker is then presented
  /// here, from the caller's context, so exactly one sheet is on screen at a
  /// time (#2724). The returned future completes once the whole flow —
  /// share sheet plus any follow-up picker — is done.
  static Future<void> show(
    BuildContext context, {
    required ShareableItemType itemType,
    required String itemId,
    required String itemName,
  }) async {
    final followUp = await showAccessibleModal<_AudienceFollowUp>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => ItemShareSheet(
        itemType: itemType,
        itemId: itemId,
        itemName: itemName,
      ),
    );
    if (followUp == null || !context.mounted) return;
    switch (followUp) {
      case _AudienceFollowUp.invitePeople:
        await _handleInvitePeople(context, itemType: itemType, itemId: itemId);
      case _AudienceFollowUp.addCommunity:
        await _handleAddCommunity(context, itemType: itemType, itemId: itemId);
    }
  }

  /// Follow-up for "Invite people": opens the member invite sheet (the share
  /// sheet is already dismissed) and refreshes the item detail if anyone was
  /// invited.
  static Future<void> _handleInvitePeople(
    BuildContext context, {
    required ShareableItemType itemType,
    required String itemId,
  }) async {
    final invited = await InviteMembersSheet.show(
      context,
      itemType: itemType,
      itemId: itemId,
    );
    if ((invited ?? false) && context.mounted) {
      _refreshItemDetailsIn(
        ProviderScope.containerOf(context, listen: false),
        itemType: itemType,
        itemId: itemId,
      );
    }
  }

  /// Follow-up for "Share to communities": opens the additive community
  /// picker (the share sheet is already dismissed) — communities the item is
  /// already in are hidden, so the host only checks off new ones — then
  /// shares the item to the chosen communities. Removal lives on the Who's In
  /// roster, not here.
  static Future<void> _handleAddCommunity(
    BuildContext context, {
    required ShareableItemType itemType,
    required String itemId,
  }) async {
    final container = ProviderScope.containerOf(context, listen: false);
    final ids = await CommunitySelectionSheet.showForDeferred(
      context,
      source: CommunitySelectionSource.invite,
      excludeCommunityIds:
          _alreadySharedCommunityIdsIn(container, itemType: itemType, itemId: itemId),
    );
    if (ids == null || ids.isEmpty || !context.mounted) return;

    try {
      await container.read(communityRepositoryProvider).shareItem(
            experienceId:
                itemType == ShareableItemType.experience ? itemId : null,
            gearId: itemType == ShareableItemType.gear ? itemId : null,
            requestId: itemType == ShareableItemType.request ? itemId : null,
            shareToCommunityIds: ids,
          );
      if (!context.mounted) return;
      _refreshItemDetailsIn(container, itemType: itemType, itemId: itemId);
      // Outcome-first, named toast when exactly one community was chosen
      // (#2724): "Shared to Cedar Court Neighbors". The name lookup is
      // cached and best-effort; if it comes back empty the count is bumped
      // past 1 so the ICU plural falls back to the unnamed copy instead of
      // rendering "Shared to ".
      var namedCount = ids.length;
      var communityName = '';
      if (ids.length == 1) {
        try {
          communityName =
              (await container.read(communityRepositoryProvider).get(ids.first))
                  .name
                  .trim();
        } catch (_) {
          // Fall through to the unnamed plural copy.
        }
        if (communityName.isEmpty) namedCount = 2;
      }
      if (!context.mounted) return;
      ToastHelper.showSuccess(
        context,
        context.l10n.itemShareSharedToCommunities(namedCount, communityName),
      );
    } catch (e, stackTrace) {
      logger.severe('Failed to share to communities', e, stackTrace);
      if (!context.mounted) return;
      ToastHelper.showError(
        context,
        RpcErrorHandler.localize(RpcErrorHandler.classify(e), context.l10n),
      );
    }
  }

  /// Every community this item is already shared with — including its own
  /// ad-hoc origin community — so the picker hides them all and only offers
  /// new ones. Read from the item's already-loaded view model; empty if it
  /// hasn't loaded yet.
  static List<String> _alreadySharedCommunityIdsIn(
    ProviderContainer container, {
    required ShareableItemType itemType,
    required String itemId,
  }) {
    final shared = switch (itemType) {
      ShareableItemType.experience =>
        container.read(experienceProvider(itemId)).sharedCommunities,
      ShareableItemType.gear =>
        container.read(gearProvider(itemId)).sharedCommunities,
      ShareableItemType.request =>
        container.read(requestProvider(itemId)).sharedCommunities,
    };
    return shared.map((c) => c.communityId).toList();
  }

  /// Refreshes the item's loaded detail so the content view (the Who's In
  /// roster, the "Shared with" card, and the shared-community list) reflects a
  /// just-added person or community without an app restart.
  static void _refreshItemDetailsIn(
    ProviderContainer container, {
    required ShareableItemType itemType,
    required String itemId,
  }) {
    switch (itemType) {
      case ShareableItemType.experience:
        container
            .read(experienceProvider(itemId).notifier)
            .refreshExperienceDetails();
      case ShareableItemType.gear:
        container.read(gearProvider(itemId).notifier).refreshGearDetails();
      case ShareableItemType.request:
        container
            .read(requestProvider(itemId).notifier)
            .refreshRequestDetails();
    }
  }

  @override
  ConsumerState<ItemShareSheet> createState() => _ItemShareSheetState();
}

class _ItemShareSheetState extends ConsumerState<ItemShareSheet> {
  bool _isLoading = true;
  bool _copied = false;
  bool _canManageAudience = false;
  String? _communityId;
  String? _shareUrl;
  UserError? _error;

  @override
  void initState() {
    super.initState();
    _ensureShareTarget();
  }

  /// The per-type item id, routed to the matching [shareItem] parameter.
  String? get _experienceId =>
      widget.itemType == ShareableItemType.experience ? widget.itemId : null;
  String? get _gearId =>
      widget.itemType == ShareableItemType.gear ? widget.itemId : null;
  String? get _requestId =>
      widget.itemType == ShareableItemType.request ? widget.itemId : null;

  /// Finds-or-provisions the item's per-item community and its open link.
  Future<void> _ensureShareTarget() async {
    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final result = await ref.read(communityRepositoryProvider).shareItem(
            experienceId: _experienceId,
            gearId: _gearId,
            requestId: _requestId,
          );

      if (!mounted) return;

      setState(() {
        _communityId = result.adhocCommunityId;
        _shareUrl = result.shareUrl;
        _canManageAudience = result.canManageAudience;
        _isLoading = false;
      });

      unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
            ItemShareQrDisplayedEvent(
              communityId: _communityId ?? '',
              itemType: widget.itemType.name,
              itemId: widget.itemId,
            ),
          ));
    } catch (e, stackTrace) {
      logger.severe('Failed to prepare share link', e, stackTrace);
      if (!mounted) return;
      setState(() {
        _error = RpcErrorHandler.classify(e);
        _isLoading = false;
      });
    }
  }

  Future<void> _shareInviteLink() async {
    if (_shareUrl == null) return;

    try {
      await SharePlus.instance.share(
        ShareParams(text: _getShareMessage(), subject: _getShareSubject()),
      );

      unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
            ItemShareSharedEvent(
              communityId: _communityId ?? '',
              itemType: widget.itemType.name,
              itemId: widget.itemId,
            ),
          ));
    } catch (e, stackTrace) {
      logger.severe('Failed to share invite link', e, stackTrace);
      _showMessage('Failed to share: $e', isError: true);
    }
  }

  Future<void> _copyToClipboard() async {
    if (_shareUrl == null) return;

    try {
      await Clipboard.setData(ClipboardData(text: _shareUrl!));

      if (!mounted) return;
      setState(() => _copied = true);
      Future.delayed(const Duration(seconds: 2), () {
        if (mounted) setState(() => _copied = false);
      });

      unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
            ItemShareCopiedEvent(
              communityId: _communityId ?? '',
              itemType: widget.itemType.name,
              itemId: widget.itemId,
            ),
          ));
    } catch (e, stackTrace) {
      logger.severe('Failed to copy to clipboard', e, stackTrace);
      _showMessage('Failed to copy: $e', isError: true);
    }
  }

  /// "Share to communities": pops this sheet with the follow-up marker; the
  /// static [ItemShareSheet.show] then presents the community picker on its
  /// own (#2724 — one sheet at a time).
  void _addCommunity() {
    Navigator.of(context).pop(_AudienceFollowUp.addCommunity);
  }

  void _showMessage(String message, {required bool isError}) {
    if (!mounted) return;

    if (isError) {
      ToastHelper.showError(context, message);
    } else {
      ToastHelper.showSuccess(context, message);
    }
  }

  String _getShareMessage() {
    switch (widget.itemType) {
      case ShareableItemType.gear:
        return 'Check out ${widget.itemName}! $_shareUrl';
      case ShareableItemType.request:
        return 'Help needed: ${widget.itemName}! $_shareUrl';
      case ShareableItemType.experience:
        return 'Join ${widget.itemName}! $_shareUrl';
    }
  }

  String _getShareSubject() {
    switch (widget.itemType) {
      case ShareableItemType.gear:
        return 'Check out ${widget.itemName}';
      case ShareableItemType.request:
        return 'Help needed: ${widget.itemName}';
      case ShareableItemType.experience:
        return 'Join ${widget.itemName}';
    }
  }

  Widget _buildHeader() {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Expanded(
          child: Text(
            'Share ${widget.itemName}',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.bold,
              color: AppColors.modalTextPrimary,
            ),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
        const SizedBox(width: 12),
        Tappable(
          semanticsLabel: context.l10n.a11yClose,
          onTap: () => Navigator.of(context).pop(),
          child: Icon(
            Icons.close,
            color: AppColors.modalTextSecondary,
            size: 28,
          ),
        ),
      ],
    );
  }

  Widget _buildQrCode() {
    if (_shareUrl == null) return const SizedBox.shrink();

    return Center(
      child: Container(
        padding: const EdgeInsets.all(17),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(16),
        ),
        child: QrImageView(
          data: _shareUrl!,
          version: QrVersions.auto,
          size: 168,
          backgroundColor: Colors.transparent,
          eyeStyle: QrEyeStyle(
            eyeShape: QrEyeShape.square,
            color: Colors.black,
          ),
          dataModuleStyle: QrDataModuleStyle(
            dataModuleShape: QrDataModuleShape.square,
            color: Colors.black,
          ),
        ),
      ),
    );
  }

  Widget _buildUrlDisplay() {
    if (_shareUrl == null) return const SizedBox.shrink();

    // Strip scheme, truncate query params (e.g. "ripls.app/go/r5BcpDed?gear_id=...")
    final uri = Uri.tryParse(_shareUrl!);
    String displayUrl;
    if (uri != null) {
      final base = '${uri.host}${uri.path}';
      displayUrl = uri.query.isNotEmpty ? '$base?...' : base;
    } else {
      displayUrl = _shareUrl!;
    }

    return Text(
      displayUrl,
      style: TextStyle(color: AppColors.modalTextMuted, fontSize: 14),
      textAlign: TextAlign.center,
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }

  Widget _buildActionButtons() {
    // The "Copied ✓" flip on the button itself is the copy confirmation —
    // a snackbar would render behind this sheet and be invisible (#2724).
    return GlassFooterButtons(
      secondaryLabel:
          _copied ? context.l10n.shareSheetCopied : context.l10n.shareSheetCopyLink,
      secondaryEnabled: !_copied,
      onSecondary: _copied ? null : _copyToClipboard,
      primaryLabel: context.l10n.commonShare,
      primaryEnabled: true,
      onPrimary: _shareInviteLink,
    );
  }

  // Additive invite actions. "Invite people" adds existing Ripls members
  // directly (no consent / 10DLC needed — they're already on the app). Inviting
  // off-app contacts by text is still gated on platform SMS (A2P 10DLC; #2492
  // SMS-2/3) and surfaced as "coming soon" inside the member picker.
  Widget _buildAudienceActions() {
    return Column(
      children: [
        GlassInlineAction(
          text: context.l10n.shareSheetInvitePeople,
          semanticsLabel: context.l10n.shareSheetInvitePeople,
          icon: Icons.person_add_alt_1_outlined,
          onTap: _invitePeople,
        ),
        const SizedBox(height: 10),
        GlassInlineAction(
          text: context.l10n.shareSheetInviteCommunity,
          semanticsLabel: context.l10n.shareSheetInviteCommunity,
          icon: Icons.groups_outlined,
          onTap: _addCommunity,
        ),
      ],
    );
  }

  /// "Invite people": pops this sheet with the follow-up marker; the static
  /// [ItemShareSheet.show] then presents the member invite sheet on its own
  /// (#2724 — one sheet at a time).
  void _invitePeople() {
    Navigator.of(context).pop(_AudienceFollowUp.invitePeople);
  }

  /// Per-entity link caption (#2724): only events are "joined" — an item
  /// is seen and claimed, a request is seen and helped.
  Widget _buildAnyoneWithLink() {
    final caption = switch (widget.itemType) {
      ShareableItemType.experience => context.l10n.shareSheetLinkCaptionEvent,
      ShareableItemType.gear => context.l10n.shareSheetLinkCaptionGear,
      ShareableItemType.request => context.l10n.shareSheetLinkCaptionRequest,
    };
    return Text(
      caption,
      style: TextStyle(color: AppColors.modalTextMuted, fontSize: 13),
      textAlign: TextAlign.center,
    );
  }

  Widget _buildLoadingState() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(48),
        child: CircularProgressIndicator(
          color: AppColors.modalPrimaryButtonBackground,
        ),
      ),
    );
  }

  Widget _buildErrorState() {
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: Colors.red.shade50,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.red.shade200),
      ),
      child: Column(
        children: [
          Icon(Icons.error_outline, color: Colors.red.shade700, size: 48),
          const SizedBox(height: 16),
          Text(
            RpcErrorHandler.localize(_error!, context.l10n),
            style: TextStyle(color: Colors.red.shade700),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: _ensureShareTarget,
            style: ElevatedButton.styleFrom(
              backgroundColor: AppColors.modalPrimaryButtonBackground,
            ),
            child: Text(
              context.l10n.commonRetry,
              style: TextStyle(color: AppColors.modalTextPrimary),
            ),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return GlassSheet(
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _buildHeader(),
            const SizedBox(height: 16),
            if (_isLoading)
              _buildLoadingState()
            else if (_error != null)
              _buildErrorState()
            else if (_shareUrl != null) ...[
              _buildQrCode(),
              const SizedBox(height: 16),
              _buildUrlDisplay(),
              const SizedBox(height: 6),
              _buildAnyoneWithLink(),
              const SizedBox(height: 24),
              _buildActionButtons(),
              // Audience management (inviting people, adding communities) is
              // the owner's; members only reshare the link (#2630).
              if (_canManageAudience) ...[
                const SizedBox(height: 24),
                _buildAudienceActions(),
              ],
            ],
          ],
        ),
      ),
    );
  }
}
