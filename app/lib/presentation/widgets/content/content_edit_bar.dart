import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';

/// ContentEditBar is the top-of-screen bar shown when content is in edit mode.
///
/// Replaces the owner row during editing. Displays a Cancel pill on the left,
/// an "Editing" label in the center, and a Save pill on the right using the
/// provided accent color.
class ContentEditBar extends StatelessWidget {
  final VoidCallback onCancel;
  final VoidCallback onSave;
  final Color accentColor;

  /// When true the bar is offset below the floating community header,
  /// matching the vertical position used by [buildFeedHeader].
  final bool isNavVisible;

  const ContentEditBar({
    super.key,
    required this.onCancel,
    required this.onSave,
    required this.accentColor,
    this.isNavVisible = false,
  });

  @override
  Widget build(BuildContext context) {
    final topOffset = isNavVisible
        ? FloatingHeader.headerHeight + FloatingHeader.headerSpacing
        : 12.0;
    return Positioned(
      top: 0,
      left: 0,
      right: 0,
      child: SafeArea(
        bottom: false,
        child: Padding(
          padding: EdgeInsets.fromLTRB(16, topOffset, 16, 0),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              _GlassPill(
                label: context.l10n.contentEditBarCancel,
                semanticsLabel: context.l10n.contentEditBarCancel,
                onTap: onCancel,
                textColor: GlassTokens.textMuted,
                bold: true,
              ),
              ClipRRect(
                borderRadius: BorderRadius.circular(20),
                child: BackdropFilter(
                  filter: ImageFilter.blur(sigmaX: 10, sigmaY: 10),
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 14,
                      vertical: 6,
                    ),
                    decoration: BoxDecoration(
                      color: OverlayTokens.fieldFill,
                      borderRadius: BorderRadius.circular(20),
                    ),
                    child: Text(
                      context.l10n.contentEditBarEditing,
                      style: const TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w700,
                        color: GlassTokens.textPrimary,
                      ),
                    ),
                  ),
                ),
              ),
              _AccentPill(
                label: context.l10n.contentEditBarSave,
                semanticsLabel: context.l10n.contentEditBarSave,
                onTap: onSave,
                accentColor: accentColor,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _GlassPill extends StatelessWidget {
  final String label;
  final String semanticsLabel;
  final VoidCallback onTap;
  final Color textColor;
  final bool bold;

  const _GlassPill({
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
    required this.textColor,
    this.bold = false,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(20),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 10, sigmaY: 10),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 7),
            decoration: BoxDecoration(
              color: OverlayTokens.fieldFill,
              borderRadius: BorderRadius.circular(20),
            ),
            child: Text(
              label,
              style: TextStyle(
                fontSize: 13,
                fontWeight: bold ? FontWeight.w600 : FontWeight.w500,
                color: textColor,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _AccentPill extends StatelessWidget {
  final String label;
  final String semanticsLabel;
  final VoidCallback onTap;
  final Color accentColor;

  const _AccentPill({
    required this.label,
    required this.semanticsLabel,
    required this.onTap,
    required this.accentColor,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 7),
        decoration: BoxDecoration(
          color: accentColor,
          borderRadius: BorderRadius.circular(20),
        ),
        child: Text(
          label,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: GlassTokens.textPrimary,
          ),
        ),
      ),
    );
  }
}
