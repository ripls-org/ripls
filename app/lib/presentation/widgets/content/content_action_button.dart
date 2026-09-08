import 'package:flutter/material.dart';

/// ContentActionButton provides a standardized button for primary content actions.
///
/// Features:
/// - Consistent styling across all content types
/// - Built-in loading state with spinner
/// - Customizable colors and text
/// - Full-width or flexible sizing
/// - Disabled state support
///
/// Usage:
/// ```dart
/// ContentActionButton(
///   label: 'Lend',
///   backgroundColor: Colors.blue,
///   onPressed: () => _handleLend(),
///   isLoading: state.isLoading,
/// )
/// ```
class ContentActionButton extends StatelessWidget {
  /// Button label text
  final String label;

  /// Background color when enabled
  final Color backgroundColor;

  /// Callback when button is pressed (null to disable)
  final VoidCallback? onPressed;

  /// Whether to show loading spinner instead of label
  final bool isLoading;

  /// Optional icon to show before label
  final IconData? icon;

  /// Text color (defaults to white)
  final Color foregroundColor;

  /// Button elevation (defaults to 4)
  final double elevation;

  /// Border radius (defaults to 12)
  final double borderRadius;

  /// Vertical padding (defaults to 12)
  final double verticalPadding;

  const ContentActionButton({
    super.key,
    required this.label,
    required this.backgroundColor,
    this.onPressed,
    this.isLoading = false,
    this.icon,
    this.foregroundColor = Colors.white,
    this.elevation = 4,
    this.borderRadius = 12,
    this.verticalPadding = 12,
  });

  @override
  Widget build(BuildContext context) {
    // Determine if button should be disabled
    final isDisabled = onPressed == null || isLoading;

    if (icon != null) {
      return ElevatedButton.icon(
        onPressed: isDisabled ? null : onPressed,
        icon: isLoading
            ? const SizedBox.shrink()
            : Icon(icon, size: 18),
        label: _buildButtonContent(),
        style: _buildButtonStyle(),
      );
    }

    return ElevatedButton(
      onPressed: isDisabled ? null : onPressed,
      style: _buildButtonStyle(),
      child: _buildButtonContent(),
    );
  }

  Widget _buildButtonContent() {
    if (isLoading) {
      return const SizedBox(
        width: 20,
        height: 20,
        child: CircularProgressIndicator(
          strokeWidth: 2,
          valueColor: AlwaysStoppedAnimation<Color>(Colors.white),
        ),
      );
    }

    return Text(
      label,
      style: const TextStyle(
        fontSize: 16,
        fontWeight: FontWeight.w600,
      ),
    );
  }

  ButtonStyle _buildButtonStyle() {
    return ElevatedButton.styleFrom(
      backgroundColor: backgroundColor,
      foregroundColor: foregroundColor,
      padding: EdgeInsets.symmetric(vertical: verticalPadding),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(borderRadius),
      ),
      elevation: elevation,
    );
  }
}
