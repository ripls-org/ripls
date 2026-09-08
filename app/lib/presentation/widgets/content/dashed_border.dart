import 'package:flutter/material.dart';

/// DashedBorder is a dashed rounded-rectangle [ShapeBorder] — used for the
/// "needed" / "add" chips on the pitching-in surface
/// (docs/issues/2280-pitching-in-expand.md). A small re-implementation so we
/// don't depend on an external dashed-border package.
class DashedBorder extends ShapeBorder {
  final Color color;
  final Radius radius;
  final double dashWidth;
  final double gap;
  final double strokeWidth;

  const DashedBorder({
    required this.color,
    this.radius = const Radius.circular(10),
    this.dashWidth = 5,
    this.gap = 4,
    this.strokeWidth = 1.2,
  });

  @override
  EdgeInsetsGeometry get dimensions => EdgeInsets.all(strokeWidth);

  @override
  ShapeBorder scale(double t) => DashedBorder(
    color: color,
    radius: radius,
    dashWidth: dashWidth * t,
    gap: gap * t,
    strokeWidth: strokeWidth * t,
  );

  @override
  Path getInnerPath(Rect rect, {TextDirection? textDirection}) => Path()
    ..addRRect(RRect.fromRectAndRadius(rect.deflate(strokeWidth), radius));

  @override
  Path getOuterPath(Rect rect, {TextDirection? textDirection}) =>
      Path()..addRRect(RRect.fromRectAndRadius(rect, radius));

  @override
  void paint(Canvas canvas, Rect rect, {TextDirection? textDirection}) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = strokeWidth;
    final outline = Path()
      ..addRRect(RRect.fromRectAndRadius(rect.deflate(strokeWidth / 2), radius));
    final dashed = Path();
    for (final metric in outline.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        final next = distance + dashWidth;
        dashed.addPath(
          metric.extractPath(distance, next.clamp(0, metric.length)),
          Offset.zero,
        );
        distance = next + gap;
      }
    }
    canvas.drawPath(dashed, paint);
  }
}
