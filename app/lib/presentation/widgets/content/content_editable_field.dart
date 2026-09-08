import 'package:flutter/material.dart';
import 'package:logging/logging.dart';
import '../../../core/theme/app_colors.dart';

final _log = Logger('ContentEditableField');

/// ContentEditableField provides standardized text input with consistent styling
/// across all content types (gear, requests, communities).
///
/// This widget follows the Ripls architecture:
/// - Uses internal TextEditingController with proper lifecycle management
/// - Syncs with external value prop for controlled component behavior
/// - Uses AppColors for theme-aware colors
/// - Pure presentation component with zero business logic
class ContentEditableField extends StatefulWidget {
  /// The current value of the field (from ViewModel state)
  final String value;

  /// The label/hint text to display
  final String label;

  /// Callback when the text changes
  final ValueChanged<String> onChanged;

  /// Maximum number of lines (null for unlimited)
  final int? maxLines;

  /// Minimum number of lines (defaults to 1)
  final int? minLines;

  /// Whether the field is enabled for editing
  final bool enabled;

  /// Optional hint text (if different from label)
  final String? hintText;

  /// Use overlay style (white text on semi-transparent black background)
  /// for fields displayed over images/media
  final bool useOverlayStyle;

  /// Optional focus node for keyboard action toolbar integration.
  final FocusNode? focusNode;

  /// Optional key to set on the inner [TextField] widget for test targeting.
  final Key? textFieldKey;

  const ContentEditableField({
    super.key,
    required this.value,
    required this.label,
    required this.onChanged,
    this.maxLines,
    this.minLines,
    this.enabled = true,
    this.hintText,
    this.useOverlayStyle = false,
    this.focusNode,
    this.textFieldKey,
  });

  @override
  State<ContentEditableField> createState() => _ContentEditableFieldState();
}

class _ContentEditableFieldState extends State<ContentEditableField> {
  late TextEditingController _controller;

  @override
  void initState() {
    super.initState();
    _controller = TextEditingController(text: widget.value);
  }

  @override
  void didUpdateWidget(ContentEditableField oldWidget) {
    super.didUpdateWidget(oldWidget);
    // Only update controller if value changed externally (e.g., from cancel/reset)
    if (widget.value != oldWidget.value && widget.value != _controller.text) {
      _log.fine('External value change for ${widget.label}: "${oldWidget.value}" -> "${widget.value}"');
      _controller.text = widget.value;
      // Preserve cursor position if possible
      _controller.selection = TextSelection.collapsed(offset: widget.value.length);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // Choose colors based on overlay style flag
    final textColor = widget.useOverlayStyle
        ? AppColors.overlayFieldText()
        : AppColors.editableFieldText(context);
    final hintColor = widget.useOverlayStyle
        ? AppColors.overlayFieldHint()
        : AppColors.editableFieldHint(context);
    final fillColor = widget.useOverlayStyle
        ? AppColors.overlayFieldBackground()
        : AppColors.editableFieldBackground(context);
    final borderColor = widget.useOverlayStyle
        ? AppColors.overlayFieldBorder()
        : AppColors.editableFieldBorder(context);
    final focusedBorderColor = widget.useOverlayStyle
        ? AppColors.overlayFieldFocusedBorder()
        : AppColors.primary(context);
    final borderRadius = widget.useOverlayStyle ? 12.0 : 8.0;
    final borderWidth = widget.useOverlayStyle ? 2.0 : 1.0;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Label — hidden in overlay style (label acts as hint text instead)
        if (!widget.useOverlayStyle)
          Padding(
            padding: const EdgeInsets.only(bottom: 6),
            child: Text(
              widget.label,
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w600,
                letterSpacing: 0.5,
                color: AppColors.textSecondary(context),
              ),
            ),
          ),
        // Text field
        TextField(
          key: widget.textFieldKey,
          controller: _controller,
          focusNode: widget.focusNode,
          onChanged: widget.onChanged,
          enabled: widget.enabled,
          maxLines: widget.maxLines,
          minLines: widget.minLines ?? 1,
          textInputAction: TextInputAction.done,
          onSubmitted: (_) => FocusScope.of(context).unfocus(),
          style: TextStyle(
            fontSize: 16,
            color: textColor,
          ),
          decoration: InputDecoration(
            hintText: widget.hintText ?? widget.label,
            hintStyle: TextStyle(
              color: hintColor,
            ),
            filled: true,
            fillColor: fillColor,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(borderRadius),
              borderSide: BorderSide(
                color: borderColor,
                width: borderWidth,
              ),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(borderRadius),
              borderSide: BorderSide(
                color: borderColor,
                width: borderWidth,
              ),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(borderRadius),
              borderSide: BorderSide(
                color: focusedBorderColor,
                width: borderWidth,
              ),
            ),
            disabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(borderRadius),
              borderSide: BorderSide(
                color: borderColor,
                width: borderWidth,
              ),
            ),
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 12,
            ),
          ),
        ),
      ],
    );
  }
}
