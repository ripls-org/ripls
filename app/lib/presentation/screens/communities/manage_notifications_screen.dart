import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/community_notifications_view_model.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/settings/settings_widgets.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';

/// Per-community notification preferences screen. Reached from
/// Settings → Communities → [Community] → Membership → Manage Notifications.
class ManageNotificationsScreen extends ConsumerStatefulWidget {
  final String communityId;

  const ManageNotificationsScreen({
    super.key,
    required this.communityId,
  });

  @override
  ConsumerState<ManageNotificationsScreen> createState() =>
      _ManageNotificationsScreenState();
}

class _ManageNotificationsScreenState
    extends ConsumerState<ManageNotificationsScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  ProviderSubscription<AsyncValue<CommunityNotificationsState>>? _errorSub;

  @override
  void initState() {
    super.initState();
    // Surface persistence errors as a snackbar without rebuilding on every
    // update. We listen rather than watch because the AsyncError → AsyncData
    // recovery handled in the viewmodel re-emits both states in sequence.
    _errorSub = ref.listenManual<AsyncValue<CommunityNotificationsState>>(
      communityNotificationsProvider(widget.communityId),
      (prev, next) {
        if (next.hasError && prev?.hasError != true && mounted) {
          ToastHelper.showError(
            context,
            next.error?.toString() ?? 'Failed to update preferences',
          );
        }
      },
    );
  }

  @override
  void dispose() {
    _errorSub?.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final asyncState =
        ref.watch(communityNotificationsProvider(widget.communityId));

    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        title: Text(
          context.l10n.settingsManageNotificationsTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        leading: AppBarBackButton(onPressed: handleClose),
      ),
      body: asyncState.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => _buildErrorView(context, e),
        data: (state) => _buildToggles(context, state),
      ),
    );
  }

  Widget _buildErrorView(BuildContext context, Object error) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Text(
          error.toString(),
          style: TextStyle(color: AppColors.textSecondary(context)),
          textAlign: TextAlign.center,
        ),
      ),
    );
  }

  Widget _buildToggles(
    BuildContext context,
    CommunityNotificationsState state,
  ) {
    final notifier =
        ref.read(communityNotificationsProvider(widget.communityId).notifier);

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
          child: Text(
            context.l10n.settingsManageNotificationsDescription,
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                  color: AppColors.textSecondary(context),
                ),
          ),
        ),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: _toggleEntries(context, state, notifier),
        ),
      ],
    );
  }

  List<Widget> _toggleEntries(
    BuildContext context,
    CommunityNotificationsState state,
    CommunityNotificationsNotifier notifier,
  ) {
    final l10n = context.l10n;
    // Order intentionally groups by item type: experiences first, then
    // requests, then loans/giveaways, then chat — matches how the rest
    // of the app is organized.
    final entries = <_Entry>[
      // Experience-related.
      _Entry(
        Icons.event_outlined,
        l10n.settingsNotifyNewExperiences,
        l10n.settingsNotifyNewExperiencesSubtitle,
        state.notifyNewExperiences,
        notificationCategoryNewExperiences,
      ),
      _Entry(
        Icons.event_available_outlined,
        l10n.settingsNotifyExperienceRsvps,
        l10n.settingsNotifyExperienceRsvpsSubtitle,
        state.notifyExperienceRsvps,
        notificationCategoryExperienceRsvps,
      ),
      _Entry(
        Icons.checklist_rtl,
        l10n.settingsNotifyPlanningUpdates,
        l10n.settingsNotifyPlanningUpdatesSubtitle,
        state.notifyPlanningUpdates,
        notificationCategoryPlanningUpdates,
      ),
      _Entry(
        Icons.celebration_outlined,
        l10n.settingsNotifyExperienceCompleted,
        l10n.settingsNotifyExperienceCompletedSubtitle,
        state.notifyExperienceCompleted,
        notificationCategoryExperienceCompleted,
      ),
      // Request-related.
      _Entry(
        Icons.help_outline,
        l10n.settingsNotifyNewRequests,
        l10n.settingsNotifyNewRequestsSubtitle,
        state.notifyNewRequests,
        notificationCategoryNewRequests,
      ),
      _Entry(
        Icons.assignment_outlined,
        l10n.settingsNotifyRequestUpdates,
        l10n.settingsNotifyRequestUpdatesSubtitle,
        state.notifyRequestUpdates,
        notificationCategoryRequestUpdates,
      ),
      _Entry(
        Icons.schedule_outlined,
        l10n.notifyRequestFollowupPromptsTitle,
        l10n.notifyRequestFollowupPromptsDescription,
        state.notifyRequestFollowupPrompts,
        notificationCategoryRequestFollowupPrompts,
      ),
      // Loan / giveaway-related.
      _Entry(
        Icons.handyman_outlined,
        l10n.settingsNotifyGearShared,
        l10n.settingsNotifyGearSharedSubtitle,
        state.notifyGearShared,
        notificationCategoryGearShared,
      ),
      _Entry(
        Icons.swap_horiz,
        l10n.settingsNotifyTransferUpdates,
        l10n.settingsNotifyTransferUpdatesSubtitle,
        state.notifyTransferUpdates,
        notificationCategoryTransferUpdates,
      ),
      // Chat.
      _Entry(
        Icons.chat_bubble_outline,
        l10n.settingsNotifyChats,
        l10n.settingsNotifyChatsSubtitle,
        state.notifyChats,
        notificationCategoryChats,
      ),
      // Membership.
      _Entry(
        Icons.person_add_alt_outlined,
        l10n.settingsNotifyNewMembers,
        l10n.settingsNotifyNewMembersSubtitle,
        state.notifyNewMembers,
        notificationCategoryNewMembers,
      ),
    ];

    final widgets = <Widget>[];
    for (var i = 0; i < entries.length; i++) {
      final e = entries[i];
      widgets.add(
        buildSettingsToggleItem(
          context,
          icon: e.icon,
          title: e.title,
          subtitle: e.subtitle,
          value: e.value,
          onChanged: (v) => notifier.setCategory(e.category, v),
        ),
      );
      if (i < entries.length - 1) {
        widgets.add(buildSettingsDivider(context));
      }
    }
    return widgets;
  }
}

class _Entry {
  final IconData icon;
  final String title;
  final String subtitle;
  final bool value;
  final NotificationCategoryToggle category;

  _Entry(this.icon, this.title, this.subtitle, this.value, this.category);
}
