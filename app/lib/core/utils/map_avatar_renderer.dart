import 'dart:async';
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/painting.dart';

/// MapAvatarRenderer converts user profile images into circular PNG bitmaps
/// suitable for use as Google Maps Marker icons via BitmapDescriptor.bytes.
///
/// All methods are pure async utilities — no repository access occurs here.
/// The caller (ViewModel) is responsible for resolving image URLs before
/// passing them in.
class MapAvatarRenderer {
  // Prevent instantiation; all methods are static.
  const MapAvatarRenderer._();

  static const double _borderWidth = 3;

  /// renderInitials produces a circular PNG bitmap with a colored background
  /// and white initials text, used as a fallback when no avatar URL is available.
  ///
  /// [name] is used to derive 1-2 initials (first letter of each word, max 2).
  /// [backgroundColor] is the circle fill color.
  /// [size] is the diameter of the final bitmap in logical pixels.
  static Future<Uint8List> renderInitials({
    required String name,
    required Color backgroundColor,
    double size = 80.0,
  }) async {
    final initials = _extractInitials(name);
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    final double radius = size / 2;
    final center = Offset(radius, radius);

    // Draw filled circle background
    canvas.drawCircle(
      center,
      radius,
      Paint()..color = backgroundColor,
    );

    // Draw white border
    canvas.drawCircle(
      center,
      radius - _borderWidth / 2,
      Paint()
        ..color = const Color(0xFFFFFFFF)
        ..style = PaintingStyle.stroke
        ..strokeWidth = _borderWidth,
    );

    // Draw initials text
    final textPainter = TextPainter(
      text: TextSpan(
        text: initials,
        style: TextStyle(
          color: const Color(0xFFFFFFFF),
          fontSize: size * 0.38,
          fontWeight: FontWeight.w600,
        ),
      ),
      textDirection: TextDirection.ltr,
    );
    textPainter.layout();
    textPainter.paint(
      canvas,
      Offset(
        center.dx - textPainter.width / 2,
        center.dy - textPainter.height / 2,
      ),
    );

    final picture = recorder.endRecording();
    final img = await picture.toImage(size.toInt(), size.toInt());
    final byteData = await img.toByteData(format: ui.ImageByteFormat.png);
    return byteData!.buffer.asUint8List();
  }

  /// _loadNetworkImage fetches [url] via Flutter's image infrastructure
  /// and returns the decoded [ui.Image].
  static Future<ui.Image> _loadNetworkImage(String url) async {
    final completer = Completer<ui.Image>();
    final imageProvider = NetworkImage(url);
    final stream = imageProvider.resolve(ImageConfiguration.empty);
    late ImageStreamListener listener;
    listener = ImageStreamListener(
      (info, _) {
        if (!completer.isCompleted) {
          completer.complete(info.image);
        }
        stream.removeListener(listener);
      },
      onError: (error, stack) {
        if (!completer.isCompleted) {
          completer.completeError(error, stack);
        }
        stream.removeListener(listener);
      },
    );
    stream.addListener(listener);
    return completer.future;
  }

  /// renderThumbnail downloads the image at [imageUrl], clips it to a circle
  /// with a white border, and returns the result as PNG [Uint8List].
  ///
  /// [size] is the diameter of the final bitmap in logical pixels.
  static Future<Uint8List> renderThumbnail({
    required String imageUrl,
    double size = 112.0,
  }) async {
    final image = await _loadNetworkImage(imageUrl);
    return _renderCircularBitmap(
      image: image,
      size: size,
      borderColor: const Color(0xFFFFFFFF),
      borderWidth: _borderWidth,
    );
  }

  /// _renderCircularBitmap clips [image] to a circle with a border ring
  /// and returns the result as PNG bytes.
  static Future<Uint8List> _renderCircularBitmap({
    required ui.Image image,
    required double size,
    required Color borderColor,
    required double borderWidth,
  }) async {
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    final double radius = size / 2;
    final center = Offset(radius, radius);
    final innerRadius = radius - borderWidth;

    // Draw white border circle
    canvas.drawCircle(
      center,
      radius,
      Paint()..color = borderColor,
    );

    // Clip to inner circle, then draw the image
    final clipPath = Path()
      ..addOval(Rect.fromCircle(center: center, radius: innerRadius));
    canvas.clipPath(clipPath);

    final srcRect = Rect.fromLTWH(
      0,
      0,
      image.width.toDouble(),
      image.height.toDouble(),
    );
    final dstRect = Rect.fromCircle(center: center, radius: innerRadius);
    canvas.drawImageRect(image, srcRect, dstRect, Paint());

    final picture = recorder.endRecording();
    final img = await picture.toImage(size.toInt(), size.toInt());
    final byteData = await img.toByteData(format: ui.ImageByteFormat.png);
    return byteData!.buffer.asUint8List();
  }

  /// _extractInitials returns up to 2 uppercase initials from [name].
  static String _extractInitials(String name) {
    final words = name.trim().split(RegExp(r'\s+'));
    if (words.isEmpty || words.first.isEmpty) return '?';
    if (words.length == 1) return words.first[0].toUpperCase();
    return (words.first[0] + words[1][0]).toUpperCase();
  }
}
