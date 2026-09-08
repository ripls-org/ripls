import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Camera instruction overlay with fade animation.
///
/// Displays instruction text over the camera viewport with:
/// - Initial full opacity (1.0)
/// - Auto-fades to 20% opacity after 3 seconds
/// - Tap to restore full opacity
/// - Proper timer cleanup in dispose
///
/// Uses StatefulWidget for local UI state (animation), not business logic.
class CameraInstructionOverlay extends StatefulWidget {
  final String instruction;

  const CameraInstructionOverlay({
    super.key,
    required this.instruction,
  });

  @override
  State<CameraInstructionOverlay> createState() =>
      _CameraInstructionOverlayState();
}

class _CameraInstructionOverlayState extends State<CameraInstructionOverlay> {
  double _opacity = 1;
  Timer? _fadeTimer;

  @override
  void initState() {
    super.initState();
    // Fade to low opacity after 3 seconds
    _fadeTimer = Timer(const Duration(seconds: 3), () {
      if (mounted) {
        setState(() => _opacity = 0.2);
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: widget.instruction,
      onTap: () {
        // Restore full opacity on tap
        if (mounted) {
          setState(() => _opacity = 1.0);
        }
      },
      excludeChildSemantics: false,
      child: AnimatedOpacity(
        opacity: _opacity,
        duration: accessibleDuration(context, const Duration(milliseconds: 500)),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.9),
            borderRadius: BorderRadius.circular(20),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.camera_alt, size: 18),
              const SizedBox(width: 8),
              Flexible(
                child: Text(
                  widget.instruction,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w500,
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  void dispose() {
    _fadeTimer?.cancel();
    super.dispose();
  }
}
