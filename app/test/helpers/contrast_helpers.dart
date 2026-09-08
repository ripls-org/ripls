import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

/// Contrast assertions for a pumped widget tree (#2798).
///
/// ## What this catches, and why it is a separate layer
///
/// `scripts/check_glass_foreground.js` reasons about a token's *position* — it
/// flags a fill token used where a foreground belongs. It never looks at what
/// the element actually paints underneath, so it cannot see the defect in
/// #2798: a control that paints its OWN opaque light fill and then draws an
/// on-glass (white) foreground on it. Worse, its remedy text told the author to
/// reach for the on-glass text ramp, which is what took that label from 1.79:1
/// to 1.12:1 in `af66dadd4`.
///
/// This helper closes that gap from the other direction: it walks the pumped
/// tree, pairs each `Text`/`Icon` with the nearest ancestor that paints a
/// **fully opaque** flat fill, and measures.
///
/// ## What it deliberately does not do
///
/// It skips any element whose nearest ancestor fill is translucent, a gradient,
/// or an image. On the glass material those composite against whatever sits
/// behind the sheet, so no value read out of the widget tree is the value a
/// user sees — asserting one would be a vacuous test that passes while meaning
/// nothing. That case belongs to the composited pixel gate
/// (`e2e/scripts/contrast_surfaces.mjs`), which renders real frames over
/// synthetic worst-case backdrops.
///
/// So: opaque pairs are gated here, on every `flutter test` run; translucent
/// pairs are gated there. Neither layer subsumes the other.

/// WCAG AA threshold for body text (1.4.3).
const double kContrastBodyText = 4.5;

/// WCAG AA threshold for large text (1.4.3) and non-text content (1.4.11).
const double kContrastLargeOrGraphic = 3;

/// A foreground/background pairing discovered in a pumped tree.
class ContrastPair {
  /// Human-readable identifier — the text content, or the icon's code point.
  final String label;

  /// The resolved foreground colour actually painted.
  final Color foreground;

  /// The nearest opaque ancestor fill.
  final Color background;

  /// The threshold this pair must clear.
  ///
  /// WCAG sets this per element, not globally: 4.5:1 for body text (1.4.3),
  /// but 3:1 for large text and for non-text content such as icons (1.4.11).
  /// Holding icons to 4.5:1 reports passing designs as failures, which is how a
  /// contrast gate earns a reputation for noise and stops being read.
  final double requiredRatio;

  const ContrastPair({
    required this.label,
    required this.foreground,
    required this.background,
    required this.requiredRatio,
  });

  /// WCAG 2.x contrast ratio for this pair.
  double get ratio => wcagContrast(foreground, background);

  /// Whether this pair clears its own threshold.
  bool get passes => ratio >= requiredRatio;

  @override
  String toString() =>
      '"$label" ${_hex(foreground)} on ${_hex(background)} = '
      '${ratio.toStringAsFixed(2)}:1 (needs $requiredRatio:1)';
}

/// The WCAG threshold for text of a given size and weight.
///
/// "Large" is >=18pt, or >=14pt bold — WCAG's own definition, in CSS points,
/// which map 1:1 to Flutter logical pixels for this purpose.
double thresholdForText(double? fontSize, FontWeight? fontWeight) {
  final size = fontSize ?? 14.0;
  final bold = (fontWeight?.value ?? FontWeight.w400.value) >=
      FontWeight.w700.value;
  if (size >= 18 || (size >= 14 && bold)) return kContrastLargeOrGraphic;
  return kContrastBodyText;
}

/// WCAG 2.x relative-luminance contrast ratio between two opaque colours.
double wcagContrast(Color a, Color b) {
  final la = _luminance(a);
  final lb = _luminance(b);
  final hi = math.max(la, lb);
  final lo = math.min(la, lb);
  return (hi + 0.05) / (lo + 0.05);
}

double _luminance(Color c) {
  double channel(double v) =>
      v <= 0.03928 ? v / 12.92 : math.pow((v + 0.055) / 1.055, 2.4).toDouble();
  return 0.2126 * channel(c.r) + 0.7152 * channel(c.g) + 0.0722 * channel(c.b);
}

String _hex(Color c) {
  int ch(double v) => (v * 255).round().clamp(0, 255);
  final a = ch(c.a);
  final rgb = '#'
      '${ch(c.r).toRadixString(16).padLeft(2, '0')}'
      '${ch(c.g).toRadixString(16).padLeft(2, '0')}'
      '${ch(c.b).toRadixString(16).padLeft(2, '0')}';
  final pct = (c.a * 100).round();
  return a == 255 ? rgb.toUpperCase() : '$rgb@$pct%'.toUpperCase();
}

/// Every `Text`/`Icon` in the pumped tree that sits on a fully opaque fill,
/// paired with that fill.
///
/// Elements with no ancestor fill, or whose nearest ancestor fill is
/// translucent, are omitted — see the class doc for why.
List<ContrastPair> opaqueContrastPairs(WidgetTester tester) {
  final pairs = <ContrastPair>[];

  for (final element in tester.allElements) {
    final widget = element.widget;

    String label;
    Color? foreground;
    double requiredRatio;

    if (widget is Text) {
      final content = widget.data ?? widget.textSpan?.toPlainText() ?? '';
      if (content.trim().isEmpty) continue;
      label = content;
      final inherited = DefaultTextStyle.of(element).style;
      final style = inherited.merge(widget.style);
      foreground = style.color;
      requiredRatio = thresholdForText(style.fontSize, style.fontWeight);
    } else if (widget is Icon) {
      label = 'Icon(${widget.icon?.codePoint})';
      foreground = widget.color ?? IconTheme.of(element).color;
      // Non-text content — WCAG 1.4.11.
      requiredRatio = kContrastLargeOrGraphic;
    } else {
      continue;
    }

    // Fully transparent foregrounds are not painted at all.
    if (foreground == null || foreground.a == 0) continue;

    final background = _nearestAncestorFill(element);
    // No fill found, or a fill we cannot evaluate from the tree alone.
    if (background == null || background.a != 1.0) continue;

    pairs.add(ContrastPair(
      label: label,
      foreground: foreground,
      background: background,
      requiredRatio: requiredRatio,
    ));
  }

  return pairs;
}

/// The colour of the nearest ancestor that paints a flat fill, or null.
///
/// Returns the fill even when translucent so the caller can distinguish "no
/// fill at all" from "a fill I must not reason about" — only the latter is
/// evidence that the pixel gate should be covering this surface.
Color? _nearestAncestorFill(Element start) {
  Color? found;

  start.visitAncestorElements((ancestor) {
    final widget = ancestor.widget;

    if (widget is ColoredBox) {
      found = widget.color;
      return false;
    }
    if (widget is DecoratedBox) {
      final decoration = widget.decoration;
      // A gradient or image fill has no single colour to measure against.
      if (decoration is BoxDecoration) {
        if (decoration.gradient != null || decoration.image != null) {
          return false;
        }
        if (decoration.color != null) {
          found = decoration.color;
          return false;
        }
      }
      return true;
    }
    if (widget is Material && widget.color != null) {
      found = widget.color;
      return false;
    }
    return true;
  });

  return found;
}

/// Asserts every opaque foreground/background pair in the pumped tree clears
/// its own WCAG threshold — 4.5:1 for body text, 3:1 for large text and icons.
///
/// [skipLabels] drops pairs by exact label — use it only with a stated reason,
/// never to quiet a real finding.
void expectOpaqueContrast(
  WidgetTester tester, {
  Set<String> skipLabels = const {},
}) {
  final failures = opaqueContrastPairs(tester)
      .where((p) => !skipLabels.contains(p.label))
      .where((p) => !p.passes)
      .toList();

  expect(
    failures,
    isEmpty,
    reason: 'Foreground painted on an opaque fill below its WCAG threshold:\n'
        '${failures.map((f) => '  - $f').join('\n')}\n'
        'Pair the foreground against the fill the element actually paints, not '
        'against the sheet behind it (#2798).',
  );
}
