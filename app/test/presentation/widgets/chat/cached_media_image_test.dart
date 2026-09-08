import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';

void main() {
  Widget buildTestWidget(CachedMediaImage widget) {
    return MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(
        body: Center(child: widget),
      ),
    );
  }

  group('CachedMediaImage', () {
    group('empty URL (Phase 1 guard)', () {
      testWidgets('shows empty placeholder for empty URL without attempting network load',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: '',
              width: 100,
              height: 100,
            ),
          ),
        );

        // Should show a Container, not a CachedNetworkImage
        expect(find.byType(CachedMediaImage), findsOneWidget);
        // No network image widget should be present
        expect(find.byType(CircularProgressIndicator), findsNothing);
      });

      testWidgets('applies borderRadius to empty placeholder',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: '',
              width: 100,
              height: 100,
              borderRadius: BorderRadius.all(Radius.circular(12)),
            ),
          ),
        );

        expect(find.byType(ClipRRect), findsOneWidget);
      });

      testWidgets('empty placeholder has no spinner for video without thumbnail',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: '',
              contentType: 'video/mp4',
              width: 100,
              height: 100,
            ),
          ),
        );

        // No loading spinner — nothing is loading
        expect(find.byType(CircularProgressIndicator), findsNothing);
      });
    });

    group('semanticsLabel', () {
      testWidgets('exposes Semantics with image: true and provided label when set',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: '',
              semanticsLabel: 'Photo of a chainsaw',
              width: 100,
              height: 100,
            ),
          ),
        );

        final node = tester.getSemantics(find.byType(CachedMediaImage));
        expect(node.label, 'Photo of a chainsaw');
        expect(node.flagsCollection.isImage, isTrue);
      });

      testWidgets('omits Semantics node when semanticsLabel is null (decorative)',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: '',
              width: 100,
              height: 100,
            ),
          ),
        );

        // Without a semantic label the image is treated as decorative —
        // ExcludeSemantics drops the subtree from the accessibility tree.
        final node = tester.getSemantics(find.byType(CachedMediaImage));
        expect(node.label, isEmpty);
        expect(node.flagsCollection.isImage, isFalse);
      });
    });

    group('semanticsIdentifier', () {
      testWidgets('flows through to the Semantics node alongside the label',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: '',
              semanticsLabel: 'Hero video',
              semanticsIdentifier: 'event-hero-video-player',
              width: 100,
              height: 100,
            ),
          ),
        );

        final node = tester.getSemantics(find.byType(CachedMediaImage));
        expect(node.identifier, 'event-hero-video-player');
        expect(node.label, 'Hero video');
        expect(node.flagsCollection.isImage, isTrue);
      });

      testWidgets(
          'identifier-only (no label) still exposes the Semantics node silently',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: '',
              semanticsIdentifier: 'event-hero-video-player',
              width: 100,
              height: 100,
            ),
          ),
        );

        // Test-only identifier with no spoken label: node carries the
        // identifier but isImage:false so screen readers don't announce
        // it as an image without context.
        final node = tester.getSemantics(find.byType(CachedMediaImage));
        expect(node.identifier, 'event-hero-video-player');
        expect(node.label, isEmpty);
        expect(node.flagsCollection.isImage, isFalse);
      });
    });

    group('play icon overlay (Phase 3)', () {
      testWidgets('shows play icon overlay for video with non-empty URL',
          (WidgetTester tester) async {
        // We can't load the actual network image in tests, but we can verify
        // the play overlay structure is in the widget tree by checking for Stack
        // and the play icon.
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: 'https://example.com/thumb.jpg',
              contentType: 'video/mp4',
              width: 100,
              height: 100,
            ),
          ),
        );

        // Stack should be used for the overlay
        expect(find.byType(Stack), findsWidgets);
        // Play arrow icon should be present
        expect(find.byIcon(Icons.play_arrow), findsOneWidget);
      });

      testWidgets('does not show play icon for image content type',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: 'https://example.com/image.jpg',
              contentType: 'image/jpeg',
              width: 100,
              height: 100,
            ),
          ),
        );

        expect(find.byIcon(Icons.play_arrow), findsNothing);
      });

      testWidgets('does not show play icon when contentType is null',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: 'https://example.com/image.jpg',
              width: 100,
              height: 100,
            ),
          ),
        );

        expect(find.byIcon(Icons.play_arrow), findsNothing);
      });

      testWidgets('does not show play icon for empty URL even with video contentType',
          (WidgetTester tester) async {
        // Empty URL = video without thumbnail = show placeholder, no play icon
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: '',
              contentType: 'video/mp4',
              width: 100,
              height: 100,
            ),
          ),
        );

        // Play icon should NOT appear on the empty placeholder
        expect(find.byIcon(Icons.play_arrow), findsNothing);
      });

      testWidgets('applies borderRadius around image and play overlay',
          (WidgetTester tester) async {
        await tester.pumpWidget(
          buildTestWidget(
            const CachedMediaImage(
              imageUrl: 'https://example.com/thumb.jpg',
              contentType: 'video/mp4',
              width: 100,
              height: 100,
              borderRadius: BorderRadius.all(Radius.circular(8)),
            ),
          ),
        );

        // ClipRRect wraps the Stack (image + overlay)
        expect(find.byType(ClipRRect), findsOneWidget);
        expect(find.byIcon(Icons.play_arrow), findsOneWidget);
      });
    });
  });
}
