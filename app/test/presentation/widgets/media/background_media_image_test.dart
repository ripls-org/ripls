import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/presentation/widgets/media/background_media_image.dart';

MediaUrl _fakeUrl(String id) => MediaUrl(
      url: 'https://example.com/$id.jpg',
      isThumbnail: false,
      mediaId: id,
      contentType: 'image/jpeg',
    );

void main() {
  group('BackgroundMediaImage', () {
    testWidgets(
        'A: getMediaUrl is called exactly once when mediaId is stable across frame pumps',
        (tester) async {
      // Verifies that pumping frames without any widget configuration change
      // does not cause additional getMediaUrl invocations.
      int callCount = 0;

      await tester.pumpWidget(
        ProviderScope(
          child: MaterialApp(
            home: Stack(
              children: [
                BackgroundMediaImage(
                  mediaId: 'id1',
                  getMediaUrl: () async {
                    callCount++;
                    return _fakeUrl('id1');
                  },
                ),
              ],
            ),
          ),
        ),
      );

      // Advance three frames without state changes — no additional calls expected.
      await tester.pump();
      await tester.pump();
      await tester.pump();

      expect(callCount, equals(1));
    });

    testWidgets(
        'B: getMediaUrl is called with the new id when mediaId changes via parent rebuild',
        (tester) async {
      // Verifies that changing the mediaId through a parent state update causes
      // getMediaUrl to be invoked with the updated id.
      String? lastInvokedId;

      final controller = ValueNotifier<String>('id1');

      await tester.pumpWidget(
        ProviderScope(
          child: MaterialApp(
            home: ValueListenableBuilder<String>(
              valueListenable: controller,
              builder: (context, mediaId, _) => Stack(
                children: [
                  BackgroundMediaImage(
                    mediaId: mediaId,
                    getMediaUrl: () async {
                      lastInvokedId = mediaId;
                      return _fakeUrl(mediaId);
                    },
                  ),
                ],
              ),
            ),
          ),
        ),
      );

      expect(lastInvokedId, equals('id1'));

      controller.value = 'id2';
      await tester.pump();

      expect(lastInvokedId, equals('id2'));
    });
  });
}
