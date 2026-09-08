import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Builds a section header for settings screens.
Widget buildSettingsSectionHeader(BuildContext context, String title) {
  return Padding(
    padding: const EdgeInsets.only(left: 8, bottom: 4),
    child: Text(
      title,
      style: Theme.of(context).textTheme.titleSmall?.copyWith(
            color: AppColors.textSecondary(context),
            fontWeight: FontWeight.w600,
          ),
    ),
  );
}

/// Builds a card container for settings items.
Widget buildSettingsCard(
  BuildContext context, {
  required List<Widget> children,
}) {
  return Container(
    decoration: BoxDecoration(
      color: AppColors.surface(context),
      borderRadius: BorderRadius.circular(12),
      border: Border.all(
        color: AppColors.border(context),
        width: 1,
      ),
    ),
    child: Column(
      children: children,
    ),
  );
}

/// Builds a divider between settings items.
Widget buildSettingsDivider(BuildContext context) {
  return Divider(
    height: 1,
    thickness: 1,
    indent: 56,
    color: AppColors.border(context),
  );
}

/// Builds a clickable settings action item with icon, title, subtitle, and chevron.
Widget buildSettingsActionItem(
  BuildContext context, {
  required IconData icon,
  required String title,
  required String subtitle,
  required VoidCallback onTap,
}) {
  return Tappable(
    semanticsLabel: title,
    onTap: onTap,
    inkBorderRadius: BorderRadius.zero,
    child: Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: AppColors.primary(context).withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(
              icon,
              color: AppColors.primary(context),
              size: 20,
            ),
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                        fontWeight: FontWeight.w500,
                        color: AppColors.textPrimary(context),
                      ),
                ),
                const SizedBox(height: 2),
                Text(
                  subtitle,
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: AppColors.textSecondary(context),
                      ),
                ),
              ],
            ),
          ),
          Icon(
            Icons.chevron_right,
            color: AppColors.textSecondary(context),
          ),
        ],
      ),
    ),
  );
}

/// Builds a settings toggle item with icon, title, subtitle, and switch.
Widget buildSettingsToggleItem(
  BuildContext context, {
  required IconData icon,
  required String title,
  required String subtitle,
  required bool value,
  required ValueChanged<bool> onChanged,
}) {
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
          child: Icon(
            icon,
            color: AppColors.primary(context),
            size: 20,
          ),
        ),
        const SizedBox(width: 16),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                title,
                style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                      fontWeight: FontWeight.w500,
                      color: AppColors.textPrimary(context),
                    ),
              ),
              const SizedBox(height: 2),
              Text(
                subtitle,
                style: Theme.of(context).textTheme.bodySmall?.copyWith(
                      color: AppColors.textSecondary(context),
                    ),
              ),
            ],
          ),
        ),
        Switch.adaptive(
          value: value,
          onChanged: onChanged,
          activeTrackColor: AppColors.primary(context),
          activeThumbColor: Colors.white,
        ),
      ],
    ),
  );
}
