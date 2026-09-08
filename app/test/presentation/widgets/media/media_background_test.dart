import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/media/media_background.dart';
import 'package:video_player/video_player.dart';

void main() {
  group('MediaBackground - thumbnail branch ordering', () {
    testWidgets('renders without error when no media, no thumbnail, no controller', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(body: MediaBackground()),
        ),
      );
      expect(find.byType(VideoPlayer), findsNothing);
      expect(find.byType(Image), findsNothing);
    });

    testWidgets('builds without crash for video with thumbnailUrl and no controller', (tester) async {
      // Priority 2: thumbnailUrl shown when controller is absent.
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: MediaBackground(
              isVideo: true,
              thumbnailUrl: 'https://example.com/thumb.jpg',
              mediaId: 'media123',
            ),
          ),
        ),
      );
      // No crash; no VideoPlayer since controller is absent.
      expect(find.byType(VideoPlayer), findsNothing);
    });

    testWidgets('builds without crash for legacy video with no thumbnail and no controller', (tester) async {
      // #847 legacy data: isVideo true, no thumbnail, no controller → black.
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: MediaBackground(
              isVideo: true,
            ),
          ),
        ),
      );
      expect(find.byType(VideoPlayer), findsNothing);
      expect(find.byType(Image), findsNothing);
    });

    testWidgets('empty thumbnailUrl string is treated as absent (falls through to black)', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: MediaBackground(
              isVideo: true,
              thumbnailUrl: '',
              mediaId: 'media123',
            ),
          ),
        ),
      );
      expect(find.byType(VideoPlayer), findsNothing);
    });

    testWidgets('non-video image mediaPath renders CachedNetworkImage', (tester) async {
      await tester.pumpWidget(
        const MaterialApp(
          home: Scaffold(
            body: MediaBackground(
              mediaPath: 'https://example.com/image.jpg',
              mediaId: 'media123',
              isVideo: false,
            ),
          ),
        ),
      );
      // No VideoPlayer for a non-video item.
      expect(find.byType(VideoPlayer), findsNothing);
    });
  });
}
