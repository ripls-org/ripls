import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/location_pin_renderer.dart';

void main() {
  // PictureRecorder.toImage needs a binding.
  TestWidgetsFlutterBinding.ensureInitialized();

  group('LocationPinRenderer.renderLetterPin', () {
    test('returns a non-empty PNG bitmap', () async {
      final bytes = await LocationPinRenderer.renderLetterPin(
        letter: 'A',
        fill: const Color(0xFF8FAE7E),
        border: const Color(0xFFFFFFFF),
        textColor: const Color(0xFFFFFFFF),
      );

      expect(bytes, isNotEmpty);
      // PNG magic number: 0x89 'P' 'N' 'G'.
      expect(bytes.sublist(0, 4), [0x89, 0x50, 0x4E, 0x47]);
    });

    test('renders distinct bytes for leader vs normal variants', () async {
      final leader = await LocationPinRenderer.renderLetterPin(
        letter: 'B',
        fill: const Color(0xFF8FAE7E),
        border: const Color(0xFFFFFFFF),
        textColor: const Color(0xFFFFFFFF),
      );
      final normal = await LocationPinRenderer.renderLetterPin(
        letter: 'B',
        fill: const Color(0xFFFFFFFF),
        border: const Color(0xFF8FAE7E),
        textColor: const Color(0xFF8FAE7E),
      );

      expect(leader, isNot(equals(normal)));
    });
  });
}
