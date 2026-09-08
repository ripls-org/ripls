import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/value.pb.dart' show ValueEstimate;

/// Helper functions for displaying value estimates in the UI.
class ValueHelpers {
  /// Formats a value in USD to a display string.
  ///
  /// Examples:
  /// - 0.50 -> "$0.50"
  /// - 75.0 -> "$75"
  /// - 1250.0 -> "$1,250"
  /// - 0.0 -> null (returns null for zero/missing values)
  static String? formatValue(double? valueUsd) {
    if (valueUsd == null || valueUsd <= 0) return null;

    if (valueUsd < 1) {
      return '\$${valueUsd.toStringAsFixed(2)}';
    }

    final dollars = valueUsd.toInt();

    // Add thousands separators
    final formatted = _formatWithCommas(dollars);
    return '\$$formatted';
  }

  /// Formats a number with comma separators for thousands.
  static String _formatWithCommas(int number) {
    final str = number.toString();
    final buffer = StringBuffer();
    final len = str.length;

    for (var i = 0; i < len; i++) {
      if (i > 0 && (len - i) % 3 == 0) {
        buffer.write(',');
      }
      buffer.write(str[i]);
    }

    return buffer.toString();
  }

  /// Returns an icon based on confidence level.
  ///
  /// - High confidence (>= 0.8): checkmark
  /// - Medium confidence (>= 0.5): question mark
  /// - Low confidence (< 0.5): warning
  static IconData getConfidenceIcon(double confidence) {
    if (confidence >= 0.8) {
      return Icons.verified;
    } else if (confidence >= 0.5) {
      return Icons.help_outline;
    } else {
      return Icons.warning_amber;
    }
  }

  /// Returns a color based on confidence level, resolved for the ambient
  /// theme of [context].
  static Color getConfidenceColor(BuildContext context, double confidence) {
    if (confidence >= 0.8) {
      return AppColors.statusSuccess(context);
    } else if (confidence >= 0.5) {
      return AppColors.statusWarning(context);
    } else {
      return AppColors.statusError(context);
    }
  }

  /// Returns a human-readable confidence label.
  static String getConfidenceLabel(double confidence) {
    if (confidence >= 0.8) {
      return 'High confidence';
    } else if (confidence >= 0.5) {
      return 'Medium confidence';
    } else {
      return 'Low confidence';
    }
  }

  /// Checks if a ValueEstimate has valid data to display.
  static bool hasValidEstimate(ValueEstimate? estimate) {
    if (estimate == null) return false;
    return estimate.estimatedValueUsd > 0;
  }

  
  /// Formats cost savings from USD (float) to a compact display string.
  ///
  /// Examples:
  /// - 0.0 → "$0"
  /// - 0.50 → "$0.50"
  /// - 50.0 → "$50"
  /// - 1500.0 → "$1.5k"
  static String formatSavingsMoneyUsd(double usd) {
    if (usd == 0) return '\$0';
    if (usd >= 1000) {
      return '\$${(usd / 1000).toStringAsFixed(1)}k';
    }
    if (usd < 1) {
      return '\$${usd.toStringAsFixed(2)}';
    }
    return '\$${usd.toStringAsFixed(0)}';
  }

  
  /// Formats time savings from minutes (float) to a compact display string.
  ///
  /// Auto-scales between minutes, hours, and days.
  /// Examples:
  /// - 0.0 → "0h"
  /// - 15.0 → "15m"
  /// - 120.0 → "2h"
  /// - 2880.0 → "2d" (48 hours)
  static String formatTimeMinutes(double minutes) {
    if (minutes == 0) return '0h';

    // >= 24 hours → use days
    if (minutes >= 1440) {
      final days = minutes / 1440;
      return '${days.round()}d';
    }

    // >= 1 hour → use hours
    if (minutes >= 60) {
      final hours = minutes / 60;
      return '${hours.round()}h';
    }

    // < 1 hour → use minutes
    return '${minutes.round()}m';
  }

  
  /// Formats CO2 savings from grams (float) to a compact display string.
  ///
  /// Auto-scales between grams, kilograms, and tonnes.
  /// Examples:
  /// - 0.0 → "0kg"
  /// - 500.0 → "500g"
  /// - 24000.0 → "24kg"
  /// - 1500000.0 → "1.5t"
  static String formatCO2Grams(double grams) {
    if (grams == 0) return '0kg';

    // >= 1000 kg (1 tonne) → use tonnes
    if (grams >= 1000000) {
      final tonnes = grams / 1000000;
      if (tonnes >= 10) {
        return '${tonnes.toStringAsFixed(0)}t';
      }
      return '${tonnes.toStringAsFixed(1)}t';
    }

    // >= 1 kg → use kilograms
    if (grams >= 1000) {
      final kg = grams / 1000;
      if (kg >= 10) {
        return '${kg.toStringAsFixed(0)}kg';
      }
      return '${kg.toStringAsFixed(1)}kg';
    }

    // < 1 kg → use grams
    return '${grams.toStringAsFixed(0)}g';
  }

  /// Formats CO₂ grams always in kg (no tonnes conversion).
  ///
  /// Examples:
  /// - 500.0 → "0.5 kg"
  /// - 24000.0 → "24 kg"
  /// - 1500000.0 → "1,500 kg"
  static String formatCO2GramsAsKg(double grams) {
    if (grams == 0) return '0 kg';
    final kg = grams / 1000;
    if (kg < 1) return '${kg.toStringAsFixed(1)} kg';
    final kgInt = kg.round();
    if (kgInt >= 1000) {
      final formatted = kgInt.toString().replaceAllMapped(
        RegExp(r'(\d{1,3})(?=(\d{3})+(?!\d))'),
        (m) => '${m[1]},',
      );
      return '$formatted kg';
    }
    return '$kgInt kg';
  }
}
