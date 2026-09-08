import 'package:flutter/material.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;

/// GearHelper provides shared utility functions for gear-related operations.
class GearHelper {
  /// Validates gear fields and shows error toast if invalid.
  /// Returns true if all fields are valid, false otherwise.
  /// Note: Location is optional and not validated here.
  static bool validateGearFields({
    required BuildContext context,
    required String name,
    required String description,
    required String? locationId,
  }) {
    if (name.trim().isEmpty) {
      ToastHelper.showError(context, 'Gear name is required');
      return false;
    }

    if (description.trim().isEmpty) {
      ToastHelper.showError(context, 'Gear description is required');
      return false;
    }

    // Location is optional - don't validate it
    return true;
  }

  /// Gets user-friendly text for availability type.
  /// Returns 'shared' for LOAN, 'given away' for GIVEAWAY.
  static String getAvailabilityActionText(Availability availability) {
    return availability == Availability.AVAILABILITY_FOR_LOAN
        ? 'shared'
        : 'given away';
  }

  /// Gets success message for gear creation/sharing.
  static String getSuccessMessage(Availability availability) {
    final actionText = getAvailabilityActionText(availability);
    return 'Gear $actionText successfully!';
  }

  /// Gets feed action text for gear sharing events.
  /// Returns 'Shared for loan' or 'Shared for giveaway' based on availability.
  static String getFeedActionText(Availability availability) {
    switch (availability) {
      case Availability.AVAILABILITY_FOR_LOAN:
        return 'Shared for loan';
      case Availability.AVAILABILITY_FOR_GIVEAWAY:
        return 'Shared for giveaway';
      default:
        return 'Shared gear'; // Fallback for unspecified
    }
  }
}
