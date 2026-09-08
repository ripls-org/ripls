import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// ChartDataPoint represents a single (label, value) pair for chart rendering.
class ChartDataPoint {
  final String label;
  final double value;

  const ChartDataPoint({required this.label, required this.value});
}

/// ImpactChartCard renders a simple line chart or bar chart using CustomPainter.
///
/// Supports two modes:
/// - [ChartType.line]: cumulative line chart (for Money, QT, Time)
/// - [ChartType.bar]: monthly bar chart (for CO₂)
///
/// Charts are decorative — they show trend shape, not interactive data points.
///
/// When [showYAxisLabels] is true, three Y-axis value labels (max, mid, zero)
/// are rendered to the left of the chart area. Use [valueFormatter] to control
/// the label format (e.g. "$5K" for money, "45 kg" for CO₂).
///
/// When [secondaryData] is provided, a secondary line series is drawn behind
/// the primary series in a muted color. Use [primaryLabel] and [secondaryLabel]
/// to show a legend below the chart.
class ImpactChartCard extends StatelessWidget {
  final String title;
  final List<ChartDataPoint> data;
  final Color accentColor;
  final ChartType chartType;
  final bool showYAxisLabels;
  final String Function(double)? valueFormatter;

  /// Optional secondary data series drawn as a muted line behind the primary.
  final List<ChartDataPoint>? secondaryData;

  /// Legend label for the primary series. Shown only when [secondaryData] is set.
  final String? primaryLabel;

  /// Legend label for the secondary series. Shown only when [secondaryData] is set.
  final String? secondaryLabel;

  /// Message shown in place of the chart when [data] is empty.
  /// When null and data is empty, the widget returns [SizedBox.shrink].
  final String? emptyMessage;

  /// Background color of the card container. Defaults to [AppColors.cardBackground].
  /// Pass [Colors.transparent] to blend with the parent surface.
  final Color? backgroundColor;

  const ImpactChartCard({
    super.key,
    required this.title,
    required this.data,
    required this.accentColor,
    this.chartType = ChartType.line,
    this.showYAxisLabels = false,
    this.valueFormatter,
    this.secondaryData,
    this.primaryLabel,
    this.secondaryLabel,
    this.emptyMessage,
    this.backgroundColor,
  });

  TextStyle _yLabelStyle(BuildContext context) => TextStyle(
    fontSize: 8,
    color: AppColors.textTertiary(context),
  );

  @override
  Widget build(BuildContext context) {
    // Treat all-zero data the same as empty — painters skip rendering anyway.
    final hasData = data.isNotEmpty && data.any((d) => d.value > 0);
    if (!hasData) {
      final msg = emptyMessage;
      if (msg == null) return const SizedBox.shrink();
      return Container(
        padding: const EdgeInsets.all(13),
        decoration: BoxDecoration(
          color: AppColors.cardBackground(context),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: AppColors.border(context)),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              title,
              style: TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w700,
                letterSpacing: 1.5,
                color: AppColors.textTertiary(context),
              ),
            ),
            const SizedBox(height: 9),
            SizedBox(
              height: 80,
              child: Container(
                decoration: BoxDecoration(
                  color: AppColors.border(context).withValues(alpha: 0.4),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Center(
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        Icons.lock_outline,
                        size: 14,
                        color: AppColors.textTertiary(context),
                      ),
                      const SizedBox(width: 6),
                      Text(
                        msg,
                        style: TextStyle(
                          fontSize: 13,
                          color: AppColors.textTertiary(context),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ],
        ),
      );
    }

    final maxVal = data.map((d) => d.value).reduce(math.max);
    final fmt = valueFormatter ?? (v) => v.toStringAsFixed(0);
    final secondary = secondaryData;
    final secondaryColor = AppColors.textTertiary(context);

    final Widget chartPaint = CustomPaint(
      painter: secondary != null && secondary.isNotEmpty
          ? _LineBarPainter(
              lineSeries: data,
              barSeries: secondary,
              lineColor: accentColor,
              barColor: secondaryColor,
            )
          : (chartType == ChartType.line
              ? _LinePainter(data: data, color: accentColor)
              : _BarPainter(data: data, color: accentColor)),
      size: Size.infinite,
    );

    Widget chartArea;
    if (showYAxisLabels && maxVal > 0) {
      chartArea = Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SizedBox(
            width: 38,
            child: Column(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Text(fmt(maxVal), style: _yLabelStyle(context)),
                Text(fmt(maxVal / 2), style: _yLabelStyle(context)),
                Text(fmt(0), style: _yLabelStyle(context)),
              ],
            ),
          ),
          const SizedBox(width: 4),
          Expanded(child: chartPaint),
        ],
      );
    } else {
      chartArea = chartPaint;
    }

    final bgColor = backgroundColor ?? AppColors.cardBackground(context);
    return Container(
      padding: const EdgeInsets.all(13),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: bgColor == Colors.transparent
            ? null
            : BorderRadius.circular(14),
        border: bgColor == Colors.transparent
            ? null
            : Border.all(color: AppColors.border(context)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.5,
              color: AppColors.textTertiary(context),
            ),
          ),
          const SizedBox(height: 9),
          SizedBox(height: 80, child: chartArea),
          if (data.length > 1) ...[
            const SizedBox(height: 4),
            Padding(
              padding: showYAxisLabels
                  ? const EdgeInsets.only(left: 42)
                  : EdgeInsets.zero,
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(
                    data.first.label,
                    style: TextStyle(
                      fontSize: 9,
                      color: AppColors.textTertiary(context),
                    ),
                  ),
                  Text(
                    data.last.label,
                    style: TextStyle(
                      fontSize: 9,
                      color: AppColors.textTertiary(context),
                    ),
                  ),
                ],
              ),
            ),
          ],
          if (secondary != null &&
              primaryLabel != null &&
              secondaryLabel != null) ...[
            const SizedBox(height: 8),
            Row(
              children: [
                _LegendDot(color: accentColor),
                const SizedBox(width: 4),
                Text(
                  primaryLabel!,
                  style: TextStyle(
                    fontSize: 9,
                    color: AppColors.textTertiary(context),
                  ),
                ),
                const SizedBox(width: 12),
                _LegendDot(color: secondaryColor),
                const SizedBox(width: 4),
                Text(
                  secondaryLabel!,
                  style: TextStyle(
                    fontSize: 9,
                    color: AppColors.textTertiary(context),
                  ),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}

/// Chart rendering type.
enum ChartType { line, bar }

class _LinePainter extends CustomPainter {
  final List<ChartDataPoint> data;
  final Color color;

  _LinePainter({required this.data, required this.color});

  @override
  void paint(Canvas canvas, Size size) {
    if (data.length < 2) return;

    final maxVal = data.map((d) => d.value).reduce(math.max);
    if (maxVal == 0) return;

    final points = <Offset>[];
    for (int i = 0; i < data.length; i++) {
      final x = i / (data.length - 1) * size.width;
      final y = size.height - (data[i].value / maxVal) * size.height * 0.9;
      points.add(Offset(x, y));
    }

    // Fill area under curve
    final fillPath = Path()..moveTo(points.first.dx, size.height);
    for (final p in points) {
      fillPath.lineTo(p.dx, p.dy);
    }
    fillPath.lineTo(points.last.dx, size.height);
    fillPath.close();

    canvas.drawPath(
      fillPath,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            color.withValues(alpha: 0.18),
            color.withValues(alpha: 0.02),
          ],
        ).createShader(Rect.fromLTWH(0, 0, size.width, size.height)),
    );

    // Draw line
    final linePath = Path()..moveTo(points.first.dx, points.first.dy);
    for (int i = 1; i < points.length; i++) {
      linePath.lineTo(points[i].dx, points[i].dy);
    }

    canvas.drawPath(
      linePath,
      Paint()
        ..color = color
        ..strokeWidth = 2
        ..style = PaintingStyle.stroke
        ..strokeCap = StrokeCap.round
        ..strokeJoin = StrokeJoin.round,
    );

    // Draw endpoint dot
    canvas.drawCircle(
      points.last,
      3.5,
      Paint()..color = color,
    );
  }

  @override
  bool shouldRepaint(_LinePainter oldDelegate) =>
      data != oldDelegate.data || color != oldDelegate.color;
}

class _BarPainter extends CustomPainter {
  final List<ChartDataPoint> data;
  final Color color;

  _BarPainter({required this.data, required this.color});

  @override
  void paint(Canvas canvas, Size size) {
    if (data.isEmpty) return;

    final maxVal = data.map((d) => d.value).reduce(math.max);
    if (maxVal == 0) return;

    final barWidth = (size.width / data.length) * 0.6;
    final gap = (size.width / data.length) * 0.4;

    for (int i = 0; i < data.length; i++) {
      final barHeight = (data[i].value / maxVal) * size.height * 0.9;
      final x = i * (size.width / data.length) + gap / 2;
      final y = size.height - barHeight;

      final rrect = RRect.fromRectAndRadius(
        Rect.fromLTWH(x, y, barWidth, barHeight),
        const Radius.circular(3),
      );
      canvas.drawRRect(
        rrect,
        Paint()..color = color.withValues(alpha: 0.7),
      );
    }
  }

  @override
  bool shouldRepaint(_BarPainter oldDelegate) =>
      data != oldDelegate.data || color != oldDelegate.color;
}

/// _DualLinePainter renders two line series on the same chart area.
///
/// _LineBarPainter draws bars for [barSeries] (behind) and a line for
/// [lineSeries] (in front). Each series is normalized to its own max so both
/// are always fully visible regardless of their scale difference.
class _LineBarPainter extends CustomPainter {
  final List<ChartDataPoint> lineSeries;
  final List<ChartDataPoint> barSeries;
  final Color lineColor;
  final Color barColor;

  _LineBarPainter({
    required this.lineSeries,
    required this.barSeries,
    required this.lineColor,
    required this.barColor,
  });

  @override
  void paint(Canvas canvas, Size size) {
    // Draw bars first (behind the line).
    if (barSeries.isNotEmpty) {
      final maxBar = barSeries.map((d) => d.value).reduce(math.max);
      if (maxBar > 0) {
        final slotWidth = size.width / barSeries.length;
        final barWidth = slotWidth * 0.5;
        final gap = (slotWidth - barWidth) / 2;
        final barPaint = Paint()..color = barColor.withValues(alpha: 0.45);
        for (int i = 0; i < barSeries.length; i++) {
          final barH = (barSeries[i].value / maxBar) * size.height * 0.88;
          final x = i * slotWidth + gap;
          final y = size.height - barH;
          canvas.drawRRect(
            RRect.fromRectAndRadius(
              Rect.fromLTWH(x, y, barWidth, barH),
              const Radius.circular(3),
            ),
            barPaint,
          );
        }
      }
    }

    // Draw line on top.
    if (lineSeries.length >= 2) {
      final maxLine = lineSeries.map((d) => d.value).reduce(math.max);
      if (maxLine > 0) {
        final points = [
          for (int i = 0; i < lineSeries.length; i++)
            Offset(
              i / (lineSeries.length - 1) * size.width,
              size.height - (lineSeries[i].value / maxLine) * size.height * 0.88,
            ),
        ];

        // Gradient fill under line.
        final fillPath = Path()..moveTo(points.first.dx, size.height);
        for (final p in points) {
          fillPath.lineTo(p.dx, p.dy);
        }
        fillPath.lineTo(points.last.dx, size.height);
        fillPath.close();
        canvas.drawPath(
          fillPath,
          Paint()
            ..shader = LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [
                lineColor.withValues(alpha: 0.18),
                lineColor.withValues(alpha: 0.02),
              ],
            ).createShader(Rect.fromLTWH(0, 0, size.width, size.height)),
        );

        // Line stroke.
        final linePath = Path()..moveTo(points.first.dx, points.first.dy);
        for (int i = 1; i < points.length; i++) {
          linePath.lineTo(points[i].dx, points[i].dy);
        }
        canvas.drawPath(
          linePath,
          Paint()
            ..color = lineColor
            ..strokeWidth = 2
            ..style = PaintingStyle.stroke
            ..strokeCap = StrokeCap.round
            ..strokeJoin = StrokeJoin.round,
        );
        canvas.drawCircle(points.last, 3.5, Paint()..color = lineColor);
      }
    }
  }

  @override
  bool shouldRepaint(_LineBarPainter old) =>
      lineSeries != old.lineSeries ||
      barSeries != old.barSeries ||
      lineColor != old.lineColor ||
      barColor != old.barColor;
}

/// _LegendDot is a small colored circle used in the chart legend.
class _LegendDot extends StatelessWidget {
  final Color color;

  const _LegendDot({required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 7,
      height: 7,
      decoration: BoxDecoration(color: color, shape: BoxShape.circle),
    );
  }
}
