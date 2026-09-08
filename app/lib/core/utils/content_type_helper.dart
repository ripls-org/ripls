import 'package:flutter/material.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart';

/// ContentTypeHelper provides centralized icon and label mappings for all content types.
/// This ensures consistency across the app and provides a single place to update
/// content type display logic.
class ContentTypeHelper {
  /// getGearIcon returns the appropriate icon for gear based on availability.
  static IconData getGearIcon(Availability? availability) {
    switch (availability) {
      case Availability.AVAILABILITY_FOR_LOAN:
        return Icons.handshake;
      case Availability.AVAILABILITY_FOR_GIVEAWAY:
        return Icons.volunteer_activism;
      default:
        return Icons.inventory_2; // Generic gear icon
    }
  }

  /// getGearLabel returns the appropriate label for gear based on availability.
  static String getGearLabel(Availability? availability) {
    switch (availability) {
      case Availability.AVAILABILITY_FOR_LOAN:
        return 'LOAN';
      case Availability.AVAILABILITY_FOR_GIVEAWAY:
        return 'GIVEAWAY';
      default:
        return 'GEAR';
    }
  }

  /// getRequestIcon returns the icon for request content.
  static IconData getRequestIcon() => Icons.help_outline;

  /// getRequestLabel returns the label for request content.
  static String getRequestLabel() => 'REQUEST';

  /// getCommunityIcon returns the icon for community content.
  static IconData getCommunityIcon() => Icons.groups;

  /// getCommunityLabel returns the label for community content.
  static String getCommunityLabel() => 'Community';
    
  
  }
