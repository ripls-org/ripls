import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// ContentAccessPill is the trailing affordance in a content-view tab bar
/// (event / request / gear). Visually mirrors the inactive [ContentTabBar]
/// tab pills — same height, padding, font, glass blur — and shows
/// "{count} Invites".
///
/// Shared between the Experience and Request content views; tapping the
/// pill opens the same [AccessSheet] used elsewhere. The ARB key is
/// `experienceInvitesPill` for now — it's an internal identifier and the
/// pluralization is identical for both content types.
class ContentAccessPill extends StatelessWidget {
  final int totalPeople;
  final VoidCallback? onTap;

  const ContentAccessPill({
    super.key,
    required this.totalPeople,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final label = context.l10n.experienceInvitesPill(totalPeople);
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(20),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 12, sigmaY: 12),
          child: Container(
            padding:
                const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
            decoration: BoxDecoration(
              color: OverlayTokens.fieldFill,
              borderRadius: BorderRadius.circular(20),
              border: Border.all(color: GlassTokens.hairline),
            ),
            child: Text(
              label,
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w500,
                color: OverlayTokens.textFaint,
              ),
            ),
          ),
        ),
      ),
    );
  }
}
