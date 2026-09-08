import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/presentation/screens/settings/settings_hub_screen.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/viewmodels/me_sheet_view_model.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/feedback/feedback_sheet.dart';
import 'package:ripls/presentation/widgets/modal/solid_sheet.dart';
import 'package:ripls/presentation/widgets/navigation/nav_destination.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers.dart' show authStateProvider;

/// The Me sheet (#2634 v2): the account bottom sheet opened from the
/// avatar chip in every destination header, on the solid (non-glass)
/// sheet surface. Identity block with the communities' impact line, a
/// Profile row, a "Yours" section of filtered views of canonical
/// surfaces (the sheet owns no content of its own), then an "Account"
/// section. Logout stays as the last row until Settings hosts it.
class ProfileMenuModal extends ConsumerWidget {
  const ProfileMenuModal({super.key});

  /// Canonical entry point. Slides up the menu as a solid bottom sheet.
  ///
  /// Uses [useRootNavigator] so the sheet renders above the bottom dock
  /// even when opened from a tab with its own nested Navigator (e.g.
  /// Library) — otherwise the sheet paints into that tab's own Overlay,
  /// which sits *below* the dock in the outer Stack, leaving the dock
  /// visible on top of the sheet instead of covered by it.
  static Future<void> show(BuildContext context) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      useRootNavigator: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => const ProfileMenuModal(),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final user = ref.watch(authStateProvider).user;
    final impactAmount = ref.watch(meSheetImpactProvider).asData?.value;

    return SolidSheet(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (user != null)
            Padding(
              padding: const EdgeInsets.fromLTRB(0, 4, 0, 10),
              child: Row(
                children: [
                  UserAvatar(user: user, radius: 31),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          user.name,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            fontFamily: AppTheme.headingFont,
                            fontSize: 22,
                            fontWeight: FontWeight.w600,
                            color: AppColors.textPrimary(context),
                          ),
                        ),
                        if (impactAmount != null) ...[
                          const SizedBox(height: 3),
                          Text(
                            l10n.meSheetImpactLine(impactAmount),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontSize: 11.5,
                              color: AppColors.textSecondary(context),
                            ),
                          ),
                        ],
                      ],
                    ),
                  ),
                ],
              ),
            ),
          _MeSheetRow(
            icon: Icons.account_circle_outlined,
            title: l10n.commonProfile,
            subtitle: l10n.meSheetProfileSubtitle,
            onTap: () => _openProfile(context, ref),
          ),
          _sectionLabel(context, l10n.meSheetSectionYours),
          _MeSheetRow(
            icon: Icons.inventory_2_outlined,
            title: l10n.meSheetMyGear,
            subtitle: l10n.meSheetMyGearSubtitle,
            onTap: () => _openMyGear(context, ref),
          ),
          _MeSheetRow(
            icon: Icons.calendar_month_outlined,
            title: l10n.meSheetMyPlans,
            subtitle: l10n.meSheetMyPlansSubtitle,
            showDivider: false,
            onTap: () => _openMyPlans(context, ref),
          ),
          _sectionLabel(context, l10n.meSheetSectionAccount),
          _MeSheetRow(
            icon: Icons.notifications_none,
            title: l10n.meSheetNotifications,
            onTap: () => _openSettings(context),
          ),
          _MeSheetRow(
            icon: Icons.mail_outline,
            title: l10n.meSheetInvitations,
            subtitle: l10n.meSheetInvitationsSubtitle,
            onTap: () => _openHome(context, ref),
          ),
          _MeSheetRow(
            icon: Icons.help_outline,
            title: l10n.meSheetHelp,
            subtitle: l10n.meSheetHelpSubtitle,
            onTap: () => _openFeedback(context),
          ),
          _MeSheetRow(
            icon: Icons.settings_outlined,
            title: l10n.commonSettings,
            onTap: () => _openSettings(context),
          ),
          _MeSheetRow(
            icon: Icons.logout,
            title: l10n.commonLogout,
            showDivider: false,
            onTap: () => _logout(context, ref),
          ),
        ],
      ),
    );
  }

  Widget _sectionLabel(BuildContext context, String label) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(0, 14, 0, 6),
      child: Text(
        label.toUpperCase(),
        style: TextStyle(
          fontSize: 10.5,
          fontWeight: FontWeight.w700,
          letterSpacing: 1.2,
          color: AppColors.textSecondary(context),
        ),
      ),
    );
  }

  void _openProfile(BuildContext context, WidgetRef ref) {
    final userId = ref.read(authStateProvider).user?.id;
    Navigator.of(context).pop();
    if (userId == null || userId.isEmpty) return;
    ContentViewHelpers.openUserScreen(context, userId);
  }

  /// "Yours" row: opens the Library tab narrowed to the current user's own
  /// items — the map pins, category shelves and card rail all filter to
  /// their gear, asks and events via the search person filter.
  void _openMyGear(BuildContext context, WidgetRef ref) {
    final userId = ref.read(authStateProvider).user?.id;
    Navigator.of(context).pop();
    ref
        .read(homeProvider.notifier)
        .navigateToTab(RiplsNavDestination.library.stackIndex);
    if (userId != null && userId.isNotEmpty) {
      ref.read(searchProvider.notifier).filterByPerson(userId);
    }
  }

  /// "Yours" row: lands on the canonical Plans tab.
  void _openMyPlans(BuildContext context, WidgetRef ref) {
    Navigator.of(context).pop();
    ref
        .read(homeProvider.notifier)
        .navigateToTab(RiplsNavDestination.plans.stackIndex);
  }

  /// Invitations row: waiting invites live in the Home queue — one
  /// badge, one queue.
  void _openHome(BuildContext context, WidgetRef ref) {
    Navigator.of(context).pop();
    ref
        .read(homeProvider.notifier)
        .navigateToTab(RiplsNavDestination.home.stackIndex);
  }

  void _openSettings(BuildContext context) {
    Navigator.of(context).pop();
    NavigationHelpers.pushScreen(
      context: context,
      screen: const SettingsHubScreen(),
      useRootNavigator: true,
      routeName: 'settings_hub',
    );
  }

  void _openFeedback(BuildContext context) {
    Navigator.of(context).pop();
    FeedbackSheet.show(context);
  }

  Future<void> _logout(BuildContext context, WidgetRef ref) async {
    Navigator.of(context).pop();
    await ref.read(authStateProvider.notifier).logout();
  }
}

/// The Me-sheet row on the solid surface: a glyph box, title with
/// optional subtitle, trailing chevron, and a hairline divider below.
class _MeSheetRow extends StatelessWidget {
  final IconData icon;
  final String title;
  final String? subtitle;
  final bool showDivider;
  final VoidCallback onTap;

  const _MeSheetRow({
    required this.icon,
    required this.title,
    this.subtitle,
    this.showDivider = true,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: title,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(12),
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 9),
        decoration: BoxDecoration(
          border: showDivider
              ? Border(
                  bottom: BorderSide(color: AppColors.divider(context)),
                )
              : null,
        ),
        child: Row(
          children: [
            Container(
              width: 34,
              height: 34,
              decoration: BoxDecoration(
                color: AppColors.surface(context),
                borderRadius: BorderRadius.circular(11),
              ),
              child: Icon(icon,
                  size: 17, color: AppColors.textPrimary(context)),
            ),
            const SizedBox(width: 11),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 13.5,
                      fontWeight: FontWeight.w600,
                      color: AppColors.textPrimary(context),
                    ),
                  ),
                  if (subtitle != null && subtitle!.isNotEmpty)
                    Text(
                      subtitle!,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 11.5,
                        color: AppColors.textSecondary(context),
                      ),
                    ),
                ],
              ),
            ),
            Icon(
              Icons.chevron_right,
              size: 18,
              color: AppColors.textSecondary(context),
            ),
          ],
        ),
      ),
    );
  }
}
