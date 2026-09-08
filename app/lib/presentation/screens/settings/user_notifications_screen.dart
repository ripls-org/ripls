import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/user_notifications_view_model.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/settings/settings_widgets.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';

/// User-scoped notification preferences screen. Toggles here follow the
/// user across every community — for per-community categories, see
/// [ManageNotificationsScreen] (Settings → Communities → [Community] →
/// Membership → Manage Notifications).
class UserNotificationsScreen extends ConsumerStatefulWidget {
  const UserNotificationsScreen({super.key});

  @override
  ConsumerState<UserNotificationsScreen> createState() =>
      _UserNotificationsScreenState();
}

class _UserNotificationsScreenState extends ConsumerState<UserNotificationsScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  ProviderSubscription<AsyncValue<UserNotificationsState>>? _errorSub;

  @override
  void initState() {
    super.initState();
    // Surface persistence errors as a snackbar without rebuilding on every
    // update. We listen rather than watch because the AsyncError → AsyncData
    // recovery handled in the viewmodel re-emits both states in sequence.
    _errorSub = ref.listenManual<AsyncValue<UserNotificationsState>>(
      userNotificationsProvider,
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
    final asyncState = ref.watch(userNotificationsProvider);

    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        title: Text(
          context.l10n.settingsUserNotificationsTitle,
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

  Widget _buildToggles(BuildContext context, UserNotificationsState state) {
    final notifier = ref.read(userNotificationsProvider.notifier);
    final l10n = context.l10n;

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
          child: Text(
            l10n.settingsUserNotificationsDescription,
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                  color: AppColors.textSecondary(context),
                ),
          ),
        ),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            buildSettingsToggleItem(
              context,
              icon: Icons.assignment_return_outlined,
              title: l10n.settingsNotifyLoanReturnReminders,
              subtitle: l10n.settingsNotifyLoanReturnRemindersSubtitle,
              value: state.notifyLoanReturnReminders,
              onChanged: (v) => notifier.setCategory(
                userNotificationCategoryLoanReturnReminders,
                v,
              ),
            ),
            buildSettingsToggleItem(
              context,
              icon: Icons.event_available_outlined,
              title: l10n.settingsNotifyEventClosePrompts,
              subtitle: l10n.settingsNotifyEventClosePromptsSubtitle,
              value: state.notifyEventClosePrompts,
              onChanged: (v) => notifier.setCategory(
                userNotificationCategoryEventClosePrompts,
                v,
              ),
            ),
          ],
        ),
      ],
    );
  }
}
