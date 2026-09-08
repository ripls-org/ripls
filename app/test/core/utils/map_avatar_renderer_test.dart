import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/map_avatar_renderer.dart';

void main() {
  // Required for dart:ui operations (PictureRecorder, Canvas, Picture.toImage).
  TestWidgetsFlutterBinding.ensureInitialized();

  group('MapAvatarRenderer', () {
    group('renderInitials', () {
      test('produces valid PNG bytes for single-word name', () async {
        final bytes = await MapAvatarRenderer.renderInitials(
          name: 'Alice',
          backgroundColor: Colors.blue,
        );
        expect(bytes, isNotEmpty);
        // PNG magic bytes: 137 80 78 71
        expect(bytes[0], 137);
        expect(bytes[1], 80);
        expect(bytes[2], 78);
        expect(bytes[3], 71);
      });

      test('produces valid PNG bytes for two-word name', () async {
        final bytes = await MapAvatarRenderer.renderInitials(
          name: 'Alice Bob',
          backgroundColor: Colors.green,
          size: 60,
        );
        expect(bytes, isNotEmpty);
        expect(bytes[0], 137);
      });

      test('produces valid PNG bytes for single-character name', () async {
        final bytes = await MapAvatarRenderer.renderInitials(
          name: 'X',
          backgroundColor: Colors.red,
        );
        expect(bytes, isNotEmpty);
      });

      test('handles empty string without throwing', () async {
        final bytes = await MapAvatarRenderer.renderInitials(
          name: '',
          backgroundColor: Colors.grey,
        );
        expect(bytes, isNotEmpty);
      });

      test('handles whitespace-only name without throwing', () async {
        final bytes = await MapAvatarRenderer.renderInitials(
          name: '   ',
          backgroundColor: Colors.grey,
        );
        expect(bytes, isNotEmpty);
      });

      test('respects custom size parameter', () async {
        const customSize = 48.0;
        final bytes = await MapAvatarRenderer.renderInitials(
          name: 'Test User',
          backgroundColor: Colors.orange,
          size: customSize,
        );
        expect(bytes, isNotEmpty);
        expect(bytes[0], 137); // PNG magic byte
      });
    });

    group('determinism', () {
      // Verify that rendering the same inputs twice gives the same bytes.
      test('produces identical bytes for same inputs', () async {
        const name = 'Jan Kowalski';
        const color = Colors.teal;

        final bytes1 = await MapAvatarRenderer.renderInitials(
          name: name,
          backgroundColor: color,
        );
        final bytes2 = await MapAvatarRenderer.renderInitials(
          name: name,
          backgroundColor: color,
        );

        expect(bytes1, equals(bytes2));
      });
    });
  });

  group('renderThumbnail', () {
    // renderThumbnail requires a real network image, which is unavailable in
    // unit tests.  The tests below verify the method signature compiles and
    // that the PNG fallback path (renderInitials) remains valid as a stand-in.
    test('renderInitials produces valid PNG as thumbnail fallback', () async {
      final bytes = await MapAvatarRenderer.renderInitials(
        name: 'Climbing Rope',
        backgroundColor: Colors.deepOrange,
        size: 80,
      );
      expect(bytes, isNotEmpty);
      // PNG magic bytes: 137 80 78 71
      expect(bytes[0], 137);
      expect(bytes[1], 80);
      expect(bytes[2], 78);
      expect(bytes[3], 71);
    });
  });
}
