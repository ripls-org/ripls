import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/observability/settings.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/screens/profile/delete_account_screen.dart';
import 'package:ripls/presentation/screens/profile/manage_memberships_screen.dart';
import 'package:ripls/presentation/screens/profile/profile_edit_screen.dart';
import 'package:ripls/presentation/screens/profile/profile_locations_screen.dart';
import 'package:ripls/presentation/screens/settings/user_notifications_screen.dart';
import 'package:ripls/presentation/viewmodels/locale_view_model.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/settings/language_picker.dart';
import 'package:ripls/presentation/widgets/settings/settings_widgets.dart';
import 'package:ripls/presentation/widgets/settings/timezone_picker.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/services/providers.dart';
import 'package:timezone/timezone.dart' as tz;

/// ProfileSettingsScreen displays app settings and user profile management.
///
/// When userId is provided, displays user-specific profile management options
/// (Update Profile, Manage Memberships) alongside app-wide settings.
/// When userId is null, displays only app-wide settings (Privacy & Data).
class ProfileSettingsScreen extends ConsumerStatefulWidget {
  const ProfileSettingsScreen({super.key, this.userId});

  final String? userId;

  @override
  ConsumerState<ProfileSettingsScreen> createState() =>
      _ProfileSettingsScreenState();
}

class _ProfileSettingsScreenState extends ConsumerState<ProfileSettingsScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  String? get userId => widget.userId;

  @override
  Widget build(BuildContext context) {
    final settings = ref.watch(observabilitySettingsProvider);

    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(
          onPressed: handleClose,
        ),
        title: Text(
          context.l10n.profileSettingsTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
      ),
      body: settings.isLoading
          ? const Center(child: CircularProgressIndicator())
          : ListView(
              padding: const EdgeInsets.all(16),
              children: [
                if (userId != null) ...[
                  _buildProfileSection(context),
                  const SizedBox(height: 24),
                  _buildPreferencesSection(context, ref),
                  const SizedBox(height: 24),
                  _buildNotificationsSection(context),
                  const SizedBox(height: 24),
                ],
                _buildPrivacySection(context, ref, settings),
                const SizedBox(height: 24),
                if (userId != null) ...[
                  _buildAccountSection(context),
                  const SizedBox(height: 24),
                ],
                _buildComingSoonSection(context),
              ],
            ),
    );
  }

  Widget _buildProfileSection(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, context.l10n.commonProfile),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            buildSettingsActionItem(
              context,
              icon: Icons.edit_outlined,
              title: context.l10n.settingsUpdateProfile,
              subtitle: context.l10n.settingsUpdateProfileSubtitle,
              onTap: () {
                NavigationHelpers.pushWithSlide(
                  context: context,
                  screen: ProfileEditScreen(userId: userId!),
                  routeName: 'profile_edit',
                );
              },
            ),
            buildSettingsDivider(context),
            buildSettingsActionItem(
              context,
              icon: Icons.location_on_outlined,
              title: context.l10n.profileLocations,
              subtitle: context.l10n.settingsManageLocationsSubtitle,
              onTap: () {
                NavigationHelpers.pushWithSlide(
                  context: context,
                  screen: const ProfileLocationsScreen(),
                  routeName: 'profile_locations',
                );
              },
            ),
            buildSettingsDivider(context),
            buildSettingsActionItem(
              context,
              icon: Icons.groups_outlined,
              title: context.l10n.profileMemberships,
              subtitle: context.l10n.settingsViewCommunitiesSubtitle,
              onTap: () {
                NavigationHelpers.pushScreen(
                  context: context,
                  screen: ManageMembershipsScreen(userId: userId!),
                  routeName: 'manage_memberships',
                );
              },
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildPreferencesSection(BuildContext context, WidgetRef ref) {
    final timezoneAsync = ref.watch(userTimezoneProvider);
    final themeMode = ref.watch(themeModeProvider);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, context.l10n.settingsPreferences),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            _buildThemeSelector(context, ref, themeMode),
            buildSettingsDivider(context),
            timezoneAsync.when(
              data: (timezone) {
                final systemTz = tz.local.name;
                final subtitle = timezone != null
                    ? _formatTimezoneDisplay(timezone)
                    : context.l10n.settingsTimezoneSystem(
                        _formatTimezoneDisplay(systemTz));
                return buildSettingsActionItem(
                  context,
                  icon: Icons.schedule,
                  title: context.l10n.settingsTimezone,
                  subtitle: subtitle,
                  onTap: () => _showTimezonePicker(context, ref, timezone),
                );
              },
              loading: () => Padding(
                padding: const EdgeInsets.all(16),
                child: Row(
                  children: [
                    Icon(
                      Icons.schedule,
                      color: AppColors.textSecondary(context),
                      size: 24,
                    ),
                    const SizedBox(width: 16),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'Timezone',
                            style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                              color: AppColors.textPrimary(context),
                            ),
                          ),
                          const SizedBox(height: 4),
                          Text(
                            context.l10n.commonLoading,
                            style: Theme.of(context).textTheme.bodySmall?.copyWith(
                              color: AppColors.textSecondary(context),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              error: (_, _) => buildSettingsActionItem(
                context,
                icon: Icons.schedule,
                title: context.l10n.settingsTimezone,
                subtitle: context.l10n.settingsTimezoneError,
                onTap: () => _showTimezonePicker(context, ref, null),
              ),
            ),
            buildSettingsDivider(context),
            buildSettingsActionItem(
              context,
              icon: Icons.language_outlined,
              title: context.l10n.settingsLanguage,
              subtitle: _currentLanguageName(context),
              onTap: () => _showLanguagePicker(context, ref),
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildThemeSelector(
    BuildContext context,
    WidgetRef ref,
    ThemeMode currentMode,
  ) {
    final options = [
      (ThemeMode.system, Icons.brightness_auto, context.l10n.settingsThemeSystem),
      (ThemeMode.light, Icons.light_mode, context.l10n.settingsThemeLight),
      (ThemeMode.dark, Icons.dark_mode, context.l10n.settingsThemeDark),
    ];
    final current = options.firstWhere((o) => o.$1 == currentMode);
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: AppColors.primary(context).withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(current.$2, color: AppColors.primary(context), size: 20),
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  context.l10n.settingsAppearance,
                  style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                    fontWeight: FontWeight.w500,
                    color: AppColors.textPrimary(context),
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  current.$3,
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                    color: AppColors.textSecondary(context),
                  ),
                ),
              ],
            ),
          ),
          DropdownButton<ThemeMode>(
            value: currentMode,
            underline: const SizedBox.shrink(),
            icon: Icon(
              Icons.expand_more,
              color: AppColors.textSecondary(context),
              size: 20,
            ),
            dropdownColor: AppColors.cardBackground(context),
            borderRadius: BorderRadius.circular(12),
            items: [
              for (final (mode, icon, label) in options)
                DropdownMenuItem(
                  value: mode,
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(icon, size: 16, color: AppColors.textSecondary(context)),
                      const SizedBox(width: 8),
                      Text(
                        label,
                        style: TextStyle(
                          color: AppColors.textPrimary(context),
                          fontSize: 14,
                        ),
                      ),
                    ],
                  ),
                ),
            ],
            onChanged: (mode) {
              if (mode != null) {
                ref.read(themeModeProvider.notifier).setThemeMode(mode);
              }
            },
          ),
        ],
      ),
    );
  }

  String _formatTimezoneDisplay(String timezone) {
    return timezone.replaceAll('_', ' ');
  }

  Future<void> _showTimezonePicker(
    BuildContext context,
    WidgetRef ref,
    String? currentTimezone,
  ) async {
    await NavigationHelpers.pushWithSlide(
      context: context,
      routeName: 'timezone_picker',
      screen: TimezonePicker(
        currentTimezone: currentTimezone,
        onTimezoneSelected: (selectedTimezone) async {
          try {
            final userRepository = ref.read(userRepositoryProvider);
            await userRepository.updatePreferredTimezone(
              userId!,
              selectedTimezone,
            );

            // Invalidate timezone providers to refresh all consumers
            ref.invalidate(userTimezoneProvider);
            ref.invalidate(resolvedTimezoneProvider);

            if (context.mounted) {
              final displayName = selectedTimezone != null
                  ? _formatTimezoneDisplay(selectedTimezone)
                  : context.l10n.settingsTimezoneResetToSystem;
              ToastHelper.showSuccess(
                context,
                context.l10n.settingsTimezoneUpdated(displayName),
              );
            }
          } catch (e) {
            if (context.mounted) {
              ToastHelper.showError(
                context,
                context.l10n.settingsTimezoneUpdateFailed,
              );
            }
          }
        },
      ),
    );
  }

  /// _currentLanguageName returns the display name of the currently selected locale.
  ///
  /// Returns 'System default' when no explicit locale is set.
  String _currentLanguageName(BuildContext context) {
    final locale = ref.read(localePreferenceProvider).asData?.value;
    if (locale == null) return context.l10n.settingsLanguageSystemDefault;
    switch (locale.languageCode) {
      case 'en':
        return context.l10n.settingsLanguageEnglish;
      case 'es':
        return context.l10n.settingsLanguageSpanish;
      default:
        return locale.languageCode;
    }
  }

  /// _showLanguagePicker pushes the language picker screen from the right.
  Future<void> _showLanguagePicker(BuildContext context, WidgetRef ref) async {
    final current = ref.read(localePreferenceProvider).asData?.value;
    await NavigationHelpers.pushScreen(
      context: context,
      screen: LanguagePicker(
        currentLanguageCode: current?.languageCode,
        onLanguageSelected: (code) {
          ref.read(localePreferenceProvider.notifier).setLocale(
            code != null ? Locale(code) : null,
          );
        },
      ),
      routeName: 'language_picker',
    );
  }

  Widget _buildNotificationsSection(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, context.l10n.settingsNotifications),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            buildSettingsActionItem(
              context,
              icon: Icons.notifications_outlined,
              title: context.l10n.settingsUserNotificationsTitle,
              subtitle: context.l10n.settingsUserNotificationsDescription,
              onTap: () {
                NavigationHelpers.pushWithSlide(
                  context: context,
                  screen: const UserNotificationsScreen(),
                  routeName: 'user_notifications',
                );
              },
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildPrivacySection(
    BuildContext context,
    WidgetRef ref,
    ObservabilitySettings settings,
  ) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, context.l10n.settingsPrivacyAndData),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            buildSettingsToggleItem(
              context,
              icon: Icons.bug_report_outlined,
              title: context.l10n.settingsCrashReporting,
              subtitle: context.l10n.settingsCrashReportingSubtitle,
              value: settings.crashReporting == ObservabilityConsent.granted,
              onChanged: (value) async {
                await ref
                    .read(observabilitySettingsProvider.notifier)
                    .setCrashReporting(
                      value
                          ? ObservabilityConsent.granted
                          : ObservabilityConsent.denied,
                    );
              },
            ),
            buildSettingsDivider(context),
            buildSettingsToggleItem(
              context,
              icon: Icons.speed_outlined,
              title: context.l10n.settingsPerformanceMonitoring,
              subtitle: context.l10n.settingsPerformanceMonitoringSubtitle,
              value:
                  settings.performanceMonitoring ==
                  ObservabilityConsent.granted,
              onChanged: (value) async {
                await ref
                    .read(observabilitySettingsProvider.notifier)
                    .setPerformanceMonitoring(
                      value
                          ? ObservabilityConsent.granted
                          : ObservabilityConsent.denied,
                    );
              },
            ),
            buildSettingsDivider(context),
            buildSettingsToggleItem(
              context,
              icon: Icons.analytics_outlined,
              title: context.l10n.settingsUsageAnalytics,
              subtitle: context.l10n.settingsUsageAnalyticsSubtitle,
              value: settings.analytics == ObservabilityConsent.granted,
              onChanged: (value) async {
                await ref
                    .read(observabilitySettingsProvider.notifier)
                    .setAnalytics(
                      value
                          ? ObservabilityConsent.granted
                          : ObservabilityConsent.denied,
                    );
              },
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildAccountSection(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, context.l10n.settingsAccount),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            buildSettingsActionItem(
              context,
              icon: Icons.delete_outline,
              title: context.l10n.profileDeleteAccount,
              subtitle: context.l10n.settingsDeleteAccountSubtitle,
              onTap: () {
                NavigationHelpers.pushScreen(
                  context: context,
                  screen: const DeleteAccountScreen(),
                  routeName: 'delete_account',
                );
              },
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildComingSoonSection(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        buildSettingsSectionHeader(context, context.l10n.settingsMoreSettings),
        const SizedBox(height: 8),
        buildSettingsCard(
          context,
          children: [
            Padding(
              padding: const EdgeInsets.all(16),
              child: Row(
                children: [
                  Icon(
                    Icons.construction,
                    color: AppColors.textSecondary(context),
                    size: 24,
                  ),
                  const SizedBox(width: 16),
                  Expanded(
                    child: Text(
                      context.l10n.settingsComingSoon,
                      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        color: AppColors.textSecondary(context),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ],
    );
  }
}
