import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart'
    show Attribution, StockImageProvider;
import 'package:ripls/presentation/widgets/content/photo_attribution_line.dart';

import '../../../helpers/l10n_helpers.dart';

Attribution _unsplash({
  String creatorName = 'Jane Doe',
  String creatorUsername = 'janedoe',
  String originalUrl = 'https://unsplash.com/photos/abc',
}) =>
    Attribution()
      ..provider = StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH
      ..creatorName = creatorName
      ..creatorUsername = creatorUsername
      ..originalUrl = originalUrl;

void main() {
  group('PhotoAttributionLine', () {
    testWidgets('renders "Photo by {artist} on {source}"', (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            backgroundColor: Colors.black,
            body: PhotoAttributionLine(attribution: _unsplash()),
          ),
        ),
      );

      expect(find.textContaining('Photo by'), findsOneWidget);
      expect(find.text('Jane Doe'), findsOneWidget);
      expect(find.text('Unsplash'), findsOneWidget);
    });

    testWidgets('falls back to the provider name when creator is empty',
        (tester) async {
      await tester.pumpWidget(
        localizedApp(
          Scaffold(
            backgroundColor: Colors.black,
            body: PhotoAttributionLine(
              attribution: _unsplash(creatorName: ''),
            ),
          ),
        ),
      );

      // Both "Photo by Unsplash" and "on Unsplash" — the artist slot
      // collapses to the provider name.
      expect(find.text('Unsplash'), findsNWidgets(2));
    });
  });
}
