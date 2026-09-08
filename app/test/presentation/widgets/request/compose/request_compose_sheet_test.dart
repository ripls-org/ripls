// Unit tests for the pure suggestion-strip helper used by
// `RequestComposeSheet`. The helper is intentionally top-level so we
// can exercise it without spinning up a `ProviderScope` /
// `requestNeedsProvider` family / etc. — it's just a list-shape fn.
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/request_compose_view_model.dart';
import 'package:ripls/presentation/widgets/request/compose/request_compose_sheet.dart';

ComposePiece _piece(String label, {String? fromSuggestionLabel}) =>
    ComposePiece(id: label, label: label, fromSuggestionLabel: fromSuggestionLabel);

void main() {
  group('composeSuggestionsFor', () {
    test('returns server suggestions when nothing has been added', () {
      final result = composeSuggestionsFor(
        breakdownPieces: ['Chainsaw', 'Bar oil'],
        additionalAsks: ['Safety goggles'],
        pieces: const [],
      );
      expect(result, ['Chainsaw', 'Bar oil', 'Safety goggles']);
    });

    test('caps at six even when the server returns more', () {
      final result = composeSuggestionsFor(
        breakdownPieces: ['A', 'B', 'C', 'D'],
        additionalAsks: ['E', 'F', 'G', 'H'],
        pieces: const [],
      );
      expect(result, ['A', 'B', 'C', 'D', 'E', 'F']);
      expect(result.length, 6);
    });

    test('respects an override limit', () {
      final result = composeSuggestionsFor(
        breakdownPieces: ['A', 'B', 'C', 'D'],
        additionalAsks: const [],
        pieces: const [],
        limit: 2,
      );
      expect(result, ['A', 'B']);
    });

    test('filters out suggestions that match a committed piece label', () {
      final result = composeSuggestionsFor(
        breakdownPieces: ['Chainsaw', 'Bar oil'],
        additionalAsks: ['Safety goggles', 'Work gloves'],
        pieces: [
          _piece('Chainsaw', fromSuggestionLabel: 'Chainsaw'),
          _piece('Safety goggles'),
        ],
      );
      expect(result, ['Bar oil', 'Work gloves']);
    });

    test('match is case-insensitive and trims whitespace', () {
      final result = composeSuggestionsFor(
        breakdownPieces: ['  Chainsaw  ', 'BAR OIL'],
        additionalAsks: ['safety GOGGLES'],
        pieces: [_piece('chainsaw'), _piece('  Safety Goggles  ')],
      );
      expect(result, ['BAR OIL']);
    });

    test('drops empty / whitespace-only entries from the server', () {
      final result = composeSuggestionsFor(
        breakdownPieces: ['', '   ', 'Chainsaw'],
        additionalAsks: ['\t', 'Bar oil'],
        pieces: const [],
      );
      expect(result, ['Chainsaw', 'Bar oil']);
    });

    test('dedupes between breakdownPieces and additionalAsks', () {
      final result = composeSuggestionsFor(
        breakdownPieces: ['Chainsaw', 'Bar oil'],
        additionalAsks: ['chainsaw', 'BAR OIL', 'Goggles'],
        pieces: const [],
      );
      expect(result, ['Chainsaw', 'Bar oil', 'Goggles']);
    });

    test('still caps at six after filtering wipes out earlier entries', () {
      final result = composeSuggestionsFor(
        breakdownPieces: ['A', 'B', 'C', 'D'],
        additionalAsks: ['E', 'F', 'G', 'H', 'I'],
        // Knock out the first three breakdown chips; the strip
        // should fill back up to six from the remaining server hits.
        pieces: [_piece('A'), _piece('B'), _piece('C')],
      );
      expect(result, ['D', 'E', 'F', 'G', 'H', 'I']);
      expect(result.length, 6);
    });
  });
}
