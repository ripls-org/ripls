import 'dart:math' as math;

import 'package:flutter/material.dart';

/// ContentDashedAvatar is a dashed ring with optional [initials] inside — the
/// "invited / not replied yet" status circle on the pitching-in surface
/// (docs/issues/2280-pitching-in-expand.md). Pure; the caller resolves [color].
class ContentDashedAvatar extends StatelessWidget {
  /// Up-to-two letters shown inside the ring; null draws an empty ring.
  final String? initials;

  /// Ring (and initials) color.
  final Color color;

  /// Diameter.
  final double size;

  const ContentDashedAvatar({
    super.key,
    required this.color,
    this.initials,
    this.size = 26,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: size,
      height: size,
      child: CustomPaint(
        painter: _DashedRingPainter(color),
        child: initials == null
            ? null
            : Center(
                child: Text(
                  initials!,
                  style: TextStyle(
                    color: color,
                    fontSize: size * 0.34,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
      ),
    );
  }
}

class _DashedRingPainter extends CustomPainter {
  final Color color;

  const _DashedRingPainter(this.color);

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.5
      ..strokeCap = StrokeCap.round;
    final rect = Rect.fromCircle(
      center: Offset(size.width / 2, size.height / 2),
      radius: size.width / 2 - 1,
    );
    const dashes = 11;
    const sweep = (2 * math.pi) / dashes;
    for (var i = 0; i < dashes; i++) {
      canvas.drawArc(rect, i * sweep, sweep * 0.6, false, paint);
    }
  }

  @override
  bool shouldRepaint(_DashedRingPainter oldDelegate) =>
      oldDelegate.color != color;
}
