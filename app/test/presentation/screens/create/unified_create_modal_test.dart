import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/screens/create/unified_create_modal.dart';

void main() {
  group('mergeForBottomSheet', () {
    test('keeps outer.padding (preserves status-bar SafeArea workaround)', () {
      const outer = MediaQueryData(
        padding: EdgeInsets.only(top: 47),
        viewPadding: EdgeInsets.only(top: 47),
      );
      const inner = MediaQueryData(
        // showModalBottomSheet(useSafeArea: false) zeroes inner.padding.top.
        padding: EdgeInsets.zero,
        viewPadding: EdgeInsets.zero,
      );

      final merged = mergeForBottomSheet(outer, inner);

      expect(merged.padding.top, 47);
      expect(merged.viewPadding.top, 47);
    });

    test('takes viewInsets from inner so keyboard inset propagates live', () {
      const outer = MediaQueryData(
        padding: EdgeInsets.only(top: 47),
        // Captured before the keyboard opened — bottom is 0.
        viewInsets: EdgeInsets.zero,
      );
      const inner = MediaQueryData(
        padding: EdgeInsets.zero,
        // Live viewInsets — keyboard is up, ~336 dp on a typical iPhone.
        viewInsets: EdgeInsets.only(bottom: 336),
      );

      final merged = mergeForBottomSheet(outer, inner);

      expect(merged.viewInsets.bottom, 336);
    });

    test('viewInsets.bottom of 0 from inner (keyboard dismissed) propagates',
        () {
      const outer = MediaQueryData(
        padding: EdgeInsets.only(top: 47),
        // Stale value from before the keyboard ever came up.
        viewInsets: EdgeInsets.zero,
      );
      const inner = MediaQueryData(viewInsets: EdgeInsets.zero);

      final merged = mergeForBottomSheet(outer, inner);

      expect(merged.viewInsets.bottom, 0);
    });
  });
}
