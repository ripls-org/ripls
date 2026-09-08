import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/community_avatar.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_sheet.dart';
import 'package:ripls/services/community_service.dart' show CommunityItem;
import 'package:ripls/services/providers.dart';
import 'package:share_plus/share_plus.dart';

class InviteSheet extends ConsumerStatefulWidget {
  final String communityId;
  final String communityName;
  final String? gearId;
  final String? gearName;
  final VoidCallback onClose;
  /// When provided and contains more than one entry, a community selector
  /// dropdown is shown so the user can switch communities without dismissing.
  final List<CommunityItem> communities;

  const InviteSheet({
    super.key,
    required this.communityId,
    required this.communityName,
    this.gearId,
    this.gearName,
    required this.onClose,
    this.communities = const [],
  });

  @override
  ConsumerState<InviteSheet> createState() => _InviteSheetState();
}

class _InviteSheetState extends ConsumerState<InviteSheet> {
  bool _isLoading = true;
  bool _isRevoking = false;
  bool _copied = false;
  String? _inviteUrl;
  String? _error;
  int _numMembers = 0;
  int _maxMembers = 32;

  late String _selectedCommunityId;
  late String _selectedCommunityName;

  @override
  void initState() {
    super.initState();
    _selectedCommunityId = widget.communityId;
    _selectedCommunityName = widget.communityName;
    _loadInviteLink();
  }

  Future<void> _loadInviteLink() async {
    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      final response = await communityRepository.getOrCreateShareLink(
        communityId: _selectedCommunityId,
        gearId: widget.gearId,
      );

      if (mounted) {
        setState(() {
          _inviteUrl = response.shareUrl;
          _numMembers = response.numMembers;
          _maxMembers = response.maxMembers;
          _isLoading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = 'Failed to load invite link: $e';
          _isLoading = false;
        });
      }
    }
  }

  void _selectCommunity(CommunityItem opt) {
    setState(() {
      _selectedCommunityId = opt.id;
      _selectedCommunityName = opt.name;
    });
    _loadInviteLink();
  }

  Future<void> _openCommunityPicker() async {
    final pickedId = await CommunitySelectionSheet.showForInvite(
      context,
      initialSelectedId: _selectedCommunityId,
    );
    if (pickedId == null || !mounted) return;
    // Look up against the live community list — `widget.communities` is a
    // construction-time snapshot and won't contain a community that was
    // just created via the picker's "Create new community" affordance.
    final liveCommunities =
        ref.read(communitiesProvider).communities;
    final picked = liveCommunities
        .where((c) => c.id == pickedId)
        .firstOrNull;
    if (picked != null) {
      _selectCommunity(picked);
    }
  }

  /// Name to use in text that LEAVES the group — the share body and its
  /// subject, both delivered to people who are not members.
  ///
  /// A nameless (ad-hoc) community is labelled in-app by its members ("You,
  /// Alex, and Sam"); putting that in an invite would hand members' first names
  /// to a stranger, so a generic label stands in (#2937). The server draws the
  /// same line on the public `/go/` landing. Anything the *viewer* reads uses
  /// [communityDisplayName] instead.
  String get _outboundName {
    final trimmed = _selectedCommunityName.trim();
    return trimmed.isEmpty ? context.l10n.communityGenericGroupLabel : trimmed;
  }

  /// Name the VIEWER reads — the selector row and the "spots left in …" line.
  /// Here a nameless community reads as its members, which is what makes it
  /// identifiable to someone already inside it. Build-time only (it watches).
  String get _viewerName {
    final item = ref
        .watch(communitiesProvider)
        .communities
        .where((c) => c.id == _selectedCommunityId)
        .firstOrNull;
    return item != null
        ? communityDisplayName(item, context.l10n)
        : _outboundName;
  }

  Future<void> _shareInviteLink() async {
    if (_inviteUrl == null) return;

    final outboundName = _outboundName;
    final String text = widget.gearId != null && widget.gearName != null
        ? 'Join $outboundName to check out ${widget.gearName}! $_inviteUrl'
        : 'Join $outboundName! $_inviteUrl';

    try {
      await SharePlus.instance.share(
        ShareParams(text: text, subject: 'Invitation to $outboundName'),
      );

      unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
            CommunityInviteSharedEvent(communityId: _selectedCommunityId),
          ));
    } catch (e) {
      _showMessage('Failed to share: $e', isError: true);
    }
  }

  Future<void> _copyToClipboard() async {
    if (_inviteUrl == null) return;

    try {
      await Clipboard.setData(ClipboardData(text: _inviteUrl!));

      if (!mounted) return;
      setState(() => _copied = true);
      Future.delayed(const Duration(seconds: 2), () {
        if (mounted) setState(() => _copied = false);
      });

      unawaited(ref.read(observabilityServiceProvider).logAnalyticsEvent(
            CommunityInviteCopiedEvent(communityId: _selectedCommunityId),
          ));
    } catch (e) {
      _showMessage('Failed to copy: $e', isError: true);
    }
  }

  Future<void> _revokeInviteLink() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppColors.cardBackground(context),
        title: Text(
          'Revoke Invite Link?',
          style: TextStyle(color: AppColors.modalTextPrimary),
        ),
        content: Text(
          'This will invalidate the current link. You can create a new one afterwards.',
          style: TextStyle(color: AppColors.modalTextSecondary),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(
              context.l10n.commonCancel,
              style: TextStyle(color: AppColors.modalTextSecondary),
            ),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: Text('Revoke', style: TextStyle(color: Colors.red.shade400)),
          ),
        ],
      ),
    );

    if (confirmed != true || !mounted) return;

    setState(() => _isRevoking = true);

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      await communityRepository.revokeShareLink(
        communityId: _selectedCommunityId,
      );

      if (mounted) {
        _showMessage('Invite link revoked', isError: false);
        await _loadInviteLink();
      }
    } catch (e) {
      if (mounted) {
        _showMessage('Failed to revoke link: $e', isError: true);
      }
    } finally {
      if (mounted) setState(() => _isRevoking = false);
    }
  }

  void _showMessage(String message, {required bool isError}) {
    if (!mounted) return;
    if (isError) {
      ToastHelper.showError(context, message);
    } else {
      ToastHelper.showSuccess(context, message);
    }
  }

  // ---------------------------------------------------------------------------
  // Build helpers
  // ---------------------------------------------------------------------------

  Widget _buildHeader() {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(
          'Invite Link',
          style: TextStyle(
            fontSize: 18,
            fontWeight: FontWeight.bold,
            color: AppColors.modalTextPrimary,
          ),
        ),
        Tappable(
          semanticsLabel: context.l10n.a11yClose,
          onTap: widget.onClose,
          child: Icon(
            Icons.close,
            color: AppColors.modalTextSecondary,
            size: 28,
          ),
        ),
      ],
    );
  }

  Widget _buildCommunitySelector() {
    if (widget.communities.length <= 1) return const SizedBox.shrink();
    // Use the live community list (watched) so the selector reflects the
    // most recent community list — including any community the user just
    // created from the picker's "Create new community" action.
    final liveCommunities =
        ref.watch(communitiesProvider).communities;
    final selectedItem = liveCommunities
        .where((c) => c.id == _selectedCommunityId)
        .firstOrNull;
    final spotsLeft = !_isLoading && _inviteUrl != null
        ? _maxMembers - _numMembers
        : null;
    final selectorName = _viewerName;
    return Tappable(
      semanticsLabel: context.l10n.a11yMediaCommunityPicker,
      onTap: _openCommunityPicker,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(
            color: AppColors.divider(context),
            width: 1.5,
          ),
        ),
        child: Row(
          children: [
            CommunityAvatar(
              community: selectedItem,
              name: selectedItem == null ? selectorName : null,
              radius: 16,
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    selectorName,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: AppColors.textPrimary(context),
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                  if (spotsLeft != null)
                    Text(
                      '$spotsLeft spots left',
                      style: TextStyle(
                        fontSize: 12,
                        color: AppColors.primary(context),
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                ],
              ),
            ),
            Icon(
              Icons.keyboard_arrow_down,
              color: AppColors.textSecondary(context),
              size: 20,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildSpotsLeft() {
    final spotsLeft = _maxMembers - _numMembers;
    return RichText(
      text: TextSpan(
        style: TextStyle(
          color: AppColors.modalTextSecondary,
          fontSize: 16,
        ),
        children: [
          TextSpan(
            text: '$spotsLeft spots left',
            style: TextStyle(
              color: AppColors.primary(context),
              fontWeight: FontWeight.w600,
            ),
          ),
          TextSpan(
            text: ' in $_viewerName',
          ),
        ],
      ),
    );
  }

  Widget _buildQrCode() {
    if (_inviteUrl == null) return const SizedBox.shrink();

    return Center(
      child: Container(
        padding: const EdgeInsets.all(17),
        decoration: BoxDecoration(
          color: AppColors.qrBackground,
          borderRadius: BorderRadius.circular(16),
        ),
        child: QrImageView(
          data: _inviteUrl!,
          version: QrVersions.auto,
          size: 168,
          backgroundColor: AppColors.qrBackground,
          eyeStyle: const QrEyeStyle(
            eyeShape: QrEyeShape.square,
            color: AppColors.qrForeground,
          ),
          dataModuleStyle: const QrDataModuleStyle(
            dataModuleShape: QrDataModuleShape.square,
            color: AppColors.qrForeground,
          ),
        ),
      ),
    );
  }

  Widget _buildUrlDisplay() {
    if (_inviteUrl == null) return const SizedBox.shrink();

    final uri = Uri.tryParse(_inviteUrl!);
    String displayUrl;
    if (uri != null) {
      final base = '${uri.host}${uri.path}';
      displayUrl = uri.query.isNotEmpty ? '$base?...' : base;
    } else {
      displayUrl = _inviteUrl!;
    }

    return Text(
      displayUrl,
      style: TextStyle(
        color: AppColors.modalTextMuted,
        fontSize: 14,
      ),
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

  Widget _buildFooter() {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Text(
          'Anyone with link can join',
          style: TextStyle(
            color: AppColors.modalTextMuted,
            fontSize: 13,
          ),
        ),
        Text(
          ' · ',
          style: TextStyle(
            color: AppColors.modalTextMuted,
            fontSize: 13,
          ),
        ),
        Tappable(
          semanticsLabel: context.l10n.a11yMiscRevokeInvite,
          onTap: _isRevoking ? null : _revokeInviteLink,
          child: Text(
            _isRevoking ? 'Revoking...' : 'Revoke',
            style: TextStyle(
              color: _isRevoking
                  ? AppColors.modalTextMuted
                  : AppColors.primary(context),
              fontSize: 13,
              fontWeight: FontWeight.w500,
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildLoadingState() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(48),
        child: CircularProgressIndicator(
          color: AppColors.primary(context),
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
            _error!,
            style: TextStyle(color: Colors.red.shade700),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: _loadInviteLink,
            style: ElevatedButton.styleFrom(
              backgroundColor: AppColors.primary(context),
            ),
            child: Text(
              'Retry',
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

                _buildCommunitySelector(),

                if (_isLoading)
                  _buildLoadingState()
                else if (_error != null)
                  _buildErrorState()
                else if (_inviteUrl != null) ...[
                  if (widget.communities.length <= 1) ...[
                    _buildSpotsLeft(),
                    const SizedBox(height: 24),
                  ] else
                    const SizedBox(height: 8),
                  _buildQrCode(),
                  const SizedBox(height: 16),
                  _buildUrlDisplay(),
                  const SizedBox(height: 24),
                  _buildActionButtons(),
                  const SizedBox(height: 16),
                  _buildFooter(),
                ],
              ],
            ),
      ),
    );
  }
}
