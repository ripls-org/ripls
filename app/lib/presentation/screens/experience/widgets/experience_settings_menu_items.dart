import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/poll/poll_manage_menu_sheet.dart';

/// The owner event-settings menu items ("Mark Completed", "Close Event") shared
/// by the location and time panel overflow menus. They mirror the content
/// view's Manage sheet and are only shown when no poll is open (the poll must
/// complete first), so the caller decides when to include them. Generic over
/// the panel's own action enum [T].
List<PollManageMenuItem<T>> experienceSettingsMenuItems<T>(
  BuildContext context, {
  required T markCompleted,
  required T closeEvent,
}) {
  final l10n = context.l10n;
  return [
    PollManageMenuItem(
      icon: Icons.check_circle_outline,
      label: l10n.experienceMenuMarkCompleted,
      description: l10n.experienceMenuMarkCompletedDesc,
      action: markCompleted,
    ),
    PollManageMenuItem(
      icon: Icons.delete_outline,
      label: l10n.experienceMenuCloseEvent,
      description: l10n.experienceMenuCloseEventDesc,
      destructive: true,
      action: closeEvent,
    ),
  ];
}
