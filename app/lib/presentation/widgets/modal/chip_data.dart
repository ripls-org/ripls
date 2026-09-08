import 'package:flutter/material.dart';

/// Data class for configuring a chip in a modal. Consumed by the glass-modal
/// `GlassChip` widget and by the location-picker chip carousel — pass
/// [icon] + [label] for an icon-and-label chip, or [primary] + optional
/// [secondary] for a two-line chip.
///
/// Example:
/// ```dart
/// final chips = [
///   ChipData(value: 'today', primary: 'Today', secondary: 'May 8'),
///   ChipData(value: 'tomorrow', primary: 'Tomorrow', secondary: 'May 9'),
/// ];
/// ```
class ChipData {
  /// Display label for the legacy icon chip.
  /// For glass chips, prefer [primary] (which falls back to this field).
  final String label;

  /// Optional icon. Required by legacy chips, omitted by glass chips.
  final IconData? icon;

  /// Value used for selection tracking (must be unique within the row).
  final String value;

  /// Whether this chip represents a primary/featured option (legacy styling).
  final bool isPrimary;

  /// Optional primary line for glass-style chips. When null, [label] is used.
  final String? primary;

  /// Optional secondary line for glass-style chips. Rendered smaller below
  /// [primary]. Null on legacy chips.
  final String? secondary;

  const ChipData({
    this.label = '',
    this.icon,
    required this.value,
    this.isPrimary = false,
    this.primary,
    this.secondary,
  });

  /// Resolves the primary line — prefers [primary], falls back to [label].
  String get primaryText => primary ?? label;

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is ChipData &&
          runtimeType == other.runtimeType &&
          label == other.label &&
          icon == other.icon &&
          value == other.value &&
          isPrimary == other.isPrimary &&
          primary == other.primary &&
          secondary == other.secondary;

  @override
  int get hashCode =>
      label.hashCode ^
      icon.hashCode ^
      value.hashCode ^
      isPrimary.hashCode ^
      primary.hashCode ^
      secondary.hashCode;

  @override
  String toString() {
    return 'ChipData(value: $value, primary: $primaryText, secondary: $secondary, isPrimary: $isPrimary)';
  }
}
