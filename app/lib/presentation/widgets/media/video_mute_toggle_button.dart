import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// VideoMuteToggleButton is the shared mute/unmute affordance for any
/// content view with a video background. A 32×32 translucent-black
/// circle with a white volume icon, designed to overlay video without
/// fighting the gradient.
///
/// Accessibility labels come from `context.l10n.a11yMiscMute` /
/// `a11yMiscUnmute` — the label describes the action the next tap will
/// perform, mirroring the password-toggle convention in
/// `docs/client/accessibility.md`.
class VideoMuteToggleButton extends StatelessWidget {
  final bool isMuted;
  final VoidCallback onTap;

  const VideoMuteToggleButton({
    super.key,
    required this.isMuted,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel:
          isMuted ? context.l10n.a11yMiscUnmute : context.l10n.a11yMiscMute,
      onTap: onTap,
      child: Container(
        width: 32,
        height: 32,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          color: Colors.black.withValues(alpha: 0.4),
          border: Border.all(color: Colors.white.withValues(alpha: 0.2)),
        ),
        child: Icon(
          isMuted ? Icons.volume_off : Icons.volume_up,
          color: Colors.white,
          size: 16,
        ),
      ),
    );
  }
}
