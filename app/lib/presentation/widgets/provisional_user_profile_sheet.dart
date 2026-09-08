import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/repositories/provisional_user_repository.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/provisional_user_avatar.dart';
import 'package:ripls/services/providers.dart';
import 'package:share_plus/share_plus.dart';

/// ProvisionalUserProfileSheet shows a bottom sheet with a provisional user's activity
/// history and an option to send them an invite link.
///
/// Displays the provisional user's name, total activity count, attended experience
/// list (most-recent first), and an "Invite to Ripls" button.
class ProvisionalUserProfileSheet extends ConsumerStatefulWidget {
  final ProvisionalUser provisionalUser;
  final String communityId;

  const ProvisionalUserProfileSheet({
    super.key,
    required this.provisionalUser,
    required this.communityId,
  });

  /// Shows the profile sheet as a bottom sheet.
  static Future<void> show(
    BuildContext context, {
    required ProvisionalUser provisionalUser,
    required String communityId,
  }) {
    return showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => FractionallySizedBox(
        heightFactor: 0.75,
        child: ProvisionalUserProfileSheet(
          provisionalUser: provisionalUser,
          communityId: communityId,
        ),
      ),
    );
  }

  @override
  ConsumerState<ProvisionalUserProfileSheet> createState() =>
      _ProvisionalUserProfileSheetState();
}

class _ProvisionalUserProfileSheetState
    extends ConsumerState<ProvisionalUserProfileSheet> {
  bool _isLoadingActivities = true;
  bool _isInviting = false;
  List<ProvisionalUserActivityItem> _activities = [];
  int _totalCount = 0;
  String? _error;

  @override
  void initState() {
    super.initState();
    _loadActivities();
  }

  Future<void> _loadActivities() async {
    setState(() {
      _isLoadingActivities = true;
      _error = null;
    });
    try {
      final repo = ref.read(provisionalUserRepositoryProvider);
      final response = await repo.getActivities(
        communityId: widget.communityId,
        provisionalUserId: widget.provisionalUser.id,
      );
      if (mounted) {
        setState(() {
          _activities = response.activities;
          _totalCount = response.totalActivityCount;
          _isLoadingActivities = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e.toString();
          _isLoadingActivities = false;
        });
      }
    }
  }

  Future<void> _sendInvite() async {
    setState(() => _isInviting = true);
    try {
      final repo = ref.read(provisionalUserRepositoryProvider);
      final response = await repo.getInviteLink(
        communityId: widget.communityId,
        provisionalUserId: widget.provisionalUser.id,
      );
      if (mounted) {
        await SharePlus.instance.share(ShareParams(text: response.inviteUrl));
      }
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, e.toString());
      }
    } finally {
      if (mounted) setState(() => _isInviting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return GlassSheet(
      padding: EdgeInsets.zero,
      child: Column(
        children: [
          _buildHeader(context),
          Container(height: 1, color: AppColors.modalFooterDivider),
          Expanded(child: _buildBody(context)),
          _buildInviteButton(context),
        ],
      ),
    );
  }

  Widget _buildHeader(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
      child: Row(
        children: [
          ProvisionalUserAvatar(name: widget.provisionalUser.name, radius: 24),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  widget.provisionalUser.name,
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    color: AppColors.modalTextPrimary,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  _isLoadingActivities
                      ? '...'
                      : context.l10n.provisionalProfileTotalActivities(
                          _totalCount,
                        ),
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                    color: AppColors.modalTextSecondary,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildBody(BuildContext context) {
    if (_isLoadingActivities) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return Center(
        child: Text(
          _error!,
          style: TextStyle(color: AppColors.modalTextSecondary),
        ),
      );
    }
    if (_activities.isEmpty) {
      return Center(
        child: Text(
          context.l10n.provisionalProfileEmptyActivities,
          style: TextStyle(color: AppColors.modalTextSecondary),
        ),
      );
    }
    return _buildActivitiesList(context);
  }

  Widget _buildActivitiesList(BuildContext context) {
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
      itemCount: _activities.length,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (context, index) =>
          _buildActivityTile(context, _activities[index]),
    );
  }

  Widget _buildActivityTile(
    BuildContext context,
    ProvisionalUserActivityItem item,
  ) {
    final exp = item.experience;
    final completedAt = item.completedAtUnixSec.toInt();
    final dateStr = completedAt > 0
        ? _formatDate(DateTime.fromMillisecondsSinceEpoch(completedAt * 1000))
        : null;

    return ListTile(
      contentPadding: const EdgeInsets.symmetric(vertical: 4),
      title: Text(
        exp.name,
        style: Theme.of(
          context,
        ).textTheme.bodyMedium?.copyWith(color: AppColors.modalTextPrimary),
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
      ),
      subtitle: dateStr != null
          ? Text(
              dateStr,
              style: TextStyle(
                color: AppColors.modalTextSecondary,
                fontSize: 12,
              ),
            )
          : null,
      leading: const Icon(Icons.check_circle_outline, size: 20),
    );
  }

  Widget _buildInviteButton(BuildContext context) {
    if (widget.provisionalUser.isClaimed) return const SizedBox.shrink();
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
        child: GlassFooterButtons(
          showSecondary: false,
          primaryEnabled: !_isInviting,
          primaryLabel: context.l10n.provisionalProfileInviteButton,
          onPrimary: _isInviting ? null : _sendInvite,
        ),
      ),
    );
  }

  String _formatDate(DateTime dt) {
    final now = DateTime.now();
    final diff = now.difference(dt).inDays;
    if (diff == 0) return 'Today';
    if (diff == 1) return 'Yesterday';
    if (diff < 7) return '$diff days ago';
    return '${dt.month}/${dt.day}/${dt.year}';
  }
}
