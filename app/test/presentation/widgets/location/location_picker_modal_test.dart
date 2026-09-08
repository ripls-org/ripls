import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/location_picker_helper.dart';

void main() {
  // Regression for #1991: proto3 string fields default to "" for unset
  // values. Callers reading `locationId` off a proto message (e.g. an
  // experience whose location is still TBD) see `""`, not null. Before
  // this helper, every guard wrote `id == null` and silently fell through
  // for empty strings, leaving the LocationPickerModal in a half-
  // initialized state (read-only modal, no default camera, …).
  group('hasSavedLocationId', () {
    test('returns false for null', () {
      expect(hasSavedLocationId(null), isFalse);
    });

    test('returns false for empty string (proto3 default for unset id)', () {
      expect(hasSavedLocationId(''), isFalse);
    });

    test('returns true for a real id', () {
      expect(hasSavedLocationId('loc-123'), isTrue);
    });

    test('returns true for whitespace (treated as a real id)', () {
      // Whitespace is suspicious but unambiguously non-empty — callers
      // are responsible for trimming before passing through. If you hit
      // this in practice, fix it at the source rather than relaxing the
      // helper.
      expect(hasSavedLocationId(' '), isTrue);
    });
  });
}
