import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// Dashed sage card shown on a poll's TBD/default body, communicating that
/// no spot/time is locked yet (e.g. "Where: TBD" + a reassuring subtitle).
///
/// Matches the prototype's TBD card: a dashed sage rounded border, a "~"
/// tilde glyph in a rounded square, the headline, and a reassuring subtitle.
/// Kind-agnostic — the caller passes the already-localized [title] and
/// [subtitle]. The whole card is a single semantics container so the status
/// reads as one announcement.
class PollTbdCard extends StatelessWidget {
  const PollTbdCard({
    super.key,
    required this.title,
    required this.subtitle,
    this.icon,
  });

  /// Localized headline, e.g. "Where: TBD".
  final String title;

  /// Localized supporting line.
  final String subtitle;

  /// Retained for source compatibility with existing call sites; ignored.
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.lightAccent;
    return Semantics(
      label: title,
      value: subtitle,
      container: true,
      child: CustomPaint(
        painter: _DashedRRectPainter(
          color: accent.withValues(alpha: 0.45),
          radius: 16,
        ),
        child: Container(
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(
            color: accent.withValues(alpha: 0.07),
            borderRadius: BorderRadius.circular(16),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                width: 34,
                height: 34,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: accent.withValues(alpha: 0.18),
                  borderRadius: BorderRadius.circular(10),
                ),
                child: Text(
                  '~',
                  style: TextStyle(
                    color: accent,
                    fontSize: 20,
                    fontWeight: FontWeight.w700,
                    height: 1,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      title,
                      style: TextStyle(
                        color: AppColors.modalTextPrimary,
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      subtitle,
                      style: TextStyle(
                        color: AppColors.modalTextSecondary,
                        fontSize: 11.5,
                        height: 1.4,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Paints a dashed rounded-rectangle border, since Flutter's [Border] has no
/// dashed style. Stroke hugs the inside edge of the [radius] corners.
class _DashedRRectPainter extends CustomPainter {
  _DashedRRectPainter({required this.color, required this.radius});

  final Color color;
  final double radius;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.2;
    final rrect = RRect.fromRectAndRadius(
      Offset.zero & size,
      Radius.circular(radius),
    );
    final path = Path()..addRRect(rrect);
    const dash = 6.0;
    const gap = 4.0;
    for (final metric in path.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        canvas.drawPath(
          metric.extractPath(distance, math.min(distance + dash, metric.length)),
          paint,
        );
        distance += dash + gap;
      }
    }
  }

  @override
  bool shouldRepaint(covariant _DashedRRectPainter old) =>
      old.color != color || old.radius != radius;
}
