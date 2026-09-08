import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// Shared checklist status icon builders for content view manage menus.
///
/// Used by experience and request content views to render status circles
/// in the owner's action dropdown menu.

/// buildStatusCircle returns a 36x36 circle with a colored icon inside.
Widget buildStatusCircle({
  required BuildContext context,
  required Color backgroundColor,
  required Color iconColor,
  required IconData icon,
  String? semanticLabel,
}) {
  return Container(
    width: 36,
    height: 36,
    decoration: BoxDecoration(
      color: backgroundColor,
      shape: BoxShape.circle,
    ),
    child: Icon(
      icon,
      size: 18,
      color: iconColor,
      semanticLabel: semanticLabel,
    ),
  );
}

/// buildDetailsStatusIcon returns green check when title, description, and at
/// least one media item are all present; default edit icon otherwise.
Widget buildDetailsStatusIcon({
  required BuildContext context,
  required bool hasTitle,
  required bool hasDescription,
  required bool hasMedia,
}) {
  if (hasTitle && hasDescription && hasMedia) {
    return buildStatusCircle(
      context: context,
      backgroundColor: AppColors.rsvpYesBackground(context),
      iconColor: AppColors.rsvpYesText(context),
      icon: Icons.check,
      semanticLabel: context.l10n.a11yContentChecklistDetailsComplete,
    );
  }
  return buildStatusCircle(
    context: context,
    backgroundColor: AppColors.surface(context),
    iconColor: AppColors.textSecondary(context),
    icon: Icons.edit_outlined,
    semanticLabel: context.l10n.a11yContentChecklistDetailsIncomplete,
  );
}

/// buildPickupTimeStatusIcon returns green check when estimated pickup time is
/// set; default calendar icon otherwise.
Widget buildPickupTimeStatusIcon({
  required BuildContext context,
  required bool hasPickupTime,
}) {
  if (hasPickupTime) {
    return buildStatusCircle(
      context: context,
      backgroundColor: AppColors.rsvpYesBackground(context),
      iconColor: AppColors.rsvpYesText(context),
      icon: Icons.check,
      semanticLabel: context.l10n.a11yContentChecklistPickupTimeConfirmed,
    );
  }
  return buildStatusCircle(
    context: context,
    backgroundColor: AppColors.surface(context),
    iconColor: AppColors.textSecondary(context),
    icon: Icons.calendar_today_outlined,
    semanticLabel: context.l10n.a11yContentChecklistPickupTimeNotSet,
  );
}

/// buildPickupStatusIcon returns green check when actual pickup is confirmed;
/// default icon otherwise.
Widget buildPickupStatusIcon({
  required BuildContext context,
  required bool isPickedUp,
}) {
  if (isPickedUp) {
    return buildStatusCircle(
      context: context,
      backgroundColor: AppColors.rsvpYesBackground(context),
      iconColor: AppColors.rsvpYesText(context),
      icon: Icons.check,
      semanticLabel: context.l10n.a11yContentChecklistPickupConfirmed,
    );
  }
  return buildStatusCircle(
    context: context,
    backgroundColor: AppColors.surface(context),
    iconColor: AppColors.textSecondary(context),
    icon: Icons.inventory_2_outlined,
    semanticLabel: context.l10n.a11yContentChecklistPickupNotConfirmed,
  );
}

/// buildReturnStatusIcon returns green check when actual return is confirmed;
/// default icon otherwise.
Widget buildReturnStatusIcon({
  required BuildContext context,
  required bool isReturned,
}) {
  if (isReturned) {
    return buildStatusCircle(
      context: context,
      backgroundColor: AppColors.rsvpYesBackground(context),
      iconColor: AppColors.rsvpYesText(context),
      icon: Icons.check,
      semanticLabel: context.l10n.a11yContentChecklistReturnConfirmed,
    );
  }
  return buildStatusCircle(
    context: context,
    backgroundColor: AppColors.surface(context),
    iconColor: AppColors.textSecondary(context),
    icon: Icons.assignment_return_outlined,
    semanticLabel: context.l10n.a11yContentChecklistReturnNotConfirmed,
  );
}

/// buildReceivedStatusIcon returns green check when giveaway is completed;
/// default icon otherwise.
Widget buildReceivedStatusIcon({
  required BuildContext context,
  required bool isCompleted,
}) {
  if (isCompleted) {
    return buildStatusCircle(
      context: context,
      backgroundColor: AppColors.rsvpYesBackground(context),
      iconColor: AppColors.rsvpYesText(context),
      icon: Icons.check,
      semanticLabel: context.l10n.a11yContentChecklistReceivedConfirmed,
    );
  }
  return buildStatusCircle(
    context: context,
    backgroundColor: AppColors.surface(context),
    iconColor: AppColors.textSecondary(context),
    icon: Icons.card_giftcard,
    semanticLabel: context.l10n.a11yContentChecklistNotYetReceived,
  );
}

/// buildRecipientLeadingWidget returns a 36x36 CircleAvatar with the user's
/// initials. When [avatarWidget] is provided, it is used instead of initials.
Widget buildRecipientLeadingWidget({
  required BuildContext context,
  required String displayName,
  Widget? avatarWidget,
}) {
  if (avatarWidget != null) {
    return SizedBox(width: 36, height: 36, child: avatarWidget);
  }
  final initials = displayName.isNotEmpty ? displayName[0].toUpperCase() : '?';
  return CircleAvatar(
    radius: 18,
    backgroundColor: AppColors.surface(context),
    child: Text(
      initials,
      style: TextStyle(
        fontSize: 14,
        fontWeight: FontWeight.w600,
        color: AppColors.textPrimary(context),
      ),
    ),
  );
}

/// buildLocationStatusIcon returns a status circle for the location state.
///
/// When [requiredLocation] is true (default), a missing location shows a red X.
/// When false, a missing location shows a neutral default icon (for optional
/// location fields like requests).
Widget buildLocationStatusIcon({
  required BuildContext context,
  required String? locationName,
  bool requiredLocation = true,
}) {
  if (locationName != null && locationName.isNotEmpty) {
    return buildStatusCircle(
      context: context,
      backgroundColor: AppColors.rsvpYesBackground(context),
      iconColor: AppColors.rsvpYesText(context),
      icon: Icons.check,
      semanticLabel: context.l10n.a11yContentChecklistLocationSet,
    );
  }
  if (requiredLocation) {
    return buildStatusCircle(
      context: context,
      backgroundColor: AppColors.rsvpNoBackground(context),
      iconColor: AppColors.rsvpNoText(context),
      icon: Icons.close,
      semanticLabel: context.l10n.a11yContentChecklistNoLocation,
    );
  }
  return buildStatusCircle(
    context: context,
    backgroundColor: AppColors.surface(context),
    iconColor: AppColors.textSecondary(context),
    icon: Icons.location_on_outlined,
    semanticLabel: context.l10n.a11yContentChecklistNoLocation,
  );
}
