import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';

/// Lazily renders + caches lettered map-pin `BitmapDescriptor`s for a poll's
/// option markers, keyed by letter + leader state. Until a pin's bitmap is
/// ready it falls back to a plain colored Google marker; once rendered, the
/// cache invokes `onReady` so the host can rebuild with the lettered pin.
class LocationPinCache {
  final Map<String, BitmapDescriptor> _cache = {};
  final Set<String> _rendering = {};

  /// The lettered pin for an option. [onReady] is called (e.g. `setState`) when
  /// the bitmap finishes rendering so the marker can swap in.
  BitmapDescriptor letteredIcon(
    String letter, {
    required bool leader,
    required Color accent,
    required VoidCallback onReady,
  }) {
    final key = '$letter:${leader ? 'L' : 'N'}';
    final cached = _cache[key];
    if (cached != null) return cached;
    if (!_rendering.contains(key)) {
      _rendering.add(key);
      const white = Color(0xFFFFFFFF);
      LocationPinRenderer.renderLetterPin(
        letter: letter,
        fill: leader ? accent : white,
        border: leader ? white : accent,
        textColor: leader ? white : accent,
      ).then((bytes) {
        _cache[key] = BitmapDescriptor.bytes(bytes, width: 40);
        _rendering.remove(key);
        onReady();
      }, onError: (_) {
        // Block body, not an arrow: `Set.remove` returns bool, and `onError`
        // must return `FutureOr<Null>` to match this `then`'s value type.
        _rendering.remove(key);
      });
    }
    return BitmapDescriptor.defaultMarkerWithHue(
      leader ? BitmapDescriptor.hueOrange : BitmapDescriptor.hueGreen,
    );
  }
}

/// Renders teardrop map-pin PNG bitmaps with a letter in the head, suitable for
/// a Google Maps `BitmapDescriptor.bytes` marker. Used by the location panel so
/// each poll option's pin carries its A / B / C label (where-screen v3).
///
/// Pins anchor at their tip: pair with `Marker(anchor: Offset(0.5, 1.0))`.
class LocationPinRenderer {
  const LocationPinRenderer._();

  /// renderLetterPin draws a teardrop pin filled with [fill], a [border] ring,
  /// and the centered [letter] in [textColor]. [size] is the head diameter in
  /// logical px; the bitmap is `size` wide and `size * 1.4` tall (head + tip).
  static Future<Uint8List> renderLetterPin({
    required String letter,
    required Color fill,
    required Color border,
    required Color textColor,
    double size = 76,
  }) async {
    // Render at 3x for crisp edges on high-density displays.
    const double scale = 3;
    final double w = size * scale;
    final double headR = w / 2;
    final double cx = headR;
    final double cy = headR; // head circle centered at (R, R)
    final double tipY = w * 1.4; // pin tip below the circle
    // Inset a hair so the stroke isn't clipped at the bitmap edges.
    final double inset = 2 * scale;
    final double r = headR - inset;
    final double h = tipY;

    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);

    // Build the pin as the union of the circular head and a triangular tail
    // that tapers from inside the circle down to the tip — robust against the
    // arc-direction pitfalls of a hand-rolled teardrop path.
    final head = Path()
      ..addOval(Rect.fromCircle(center: Offset(cx, cy), radius: r));
    final tail = Path()
      ..moveTo(cx, h)
      ..lineTo(cx - r * 0.68, cy + r * 0.35)
      ..lineTo(cx + r * 0.68, cy + r * 0.35)
      ..close();
    final pin = Path.combine(PathOperation.union, head, tail);

    canvas.drawShadow(pin, const Color(0x66000000), 3 * scale, true);
    canvas.drawPath(pin, Paint()..color = fill);
    canvas.drawPath(
      pin,
      Paint()
        ..color = border
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2.6 * scale,
    );

    final tp = TextPainter(
      text: TextSpan(
        text: letter,
        style: TextStyle(
          color: textColor,
          fontSize: r * 0.95,
          fontWeight: FontWeight.w800,
          height: 1,
        ),
      ),
      textDirection: TextDirection.ltr,
    );
    tp.layout();
    tp.paint(canvas, Offset(cx - tp.width / 2, cy - tp.height / 2));

    final img = await recorder.endRecording().toImage(w.toInt(), h.toInt());
    final bytes = await img.toByteData(format: ui.ImageByteFormat.png);
    return bytes!.buffer.asUint8List();
  }
}
