import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/services/providers/media_providers.dart';

import 'full_media_url_provider_test.mocks.dart';

@GenerateMocks([MediaRepository])
void main() {
  group('fullMediaUrlProvider', () {
    late MockMediaRepository mockRepo;
    late ProviderContainer container;

    setUp(() {
      mockRepo = MockMediaRepository();
      container = ProviderContainer(
        overrides: [mediaRepositoryProvider.overrideWithValue(mockRepo)],
      );
    });

    tearDown(() {
      container.dispose();
    });

    test('returns the MediaUrl from getFullMediaUrl', () async {
      const mediaId = 'm-1';
      const url = MediaUrl(
        url: 'https://example.com/full.jpg?sig=abc',
        isThumbnail: false,
        mediaId: mediaId,
        contentType: 'image/jpeg',
      );
      when(mockRepo.getFullMediaUrl(mediaId)).thenAnswer((_) async => url);

      final result =
          await container.read(fullMediaUrlProvider(mediaId).future);

      expect(result, url);
      expect(result.cacheKey, 'media:full:$mediaId');
      verify(mockRepo.getFullMediaUrl(mediaId)).called(1);
    });

    test('invalidate forces a refetch with a fresh URL', () async {
      const mediaId = 'm-2';
      const stale = MediaUrl(
        url: 'https://example.com/full.jpg?sig=stale',
        isThumbnail: false,
        mediaId: mediaId,
      );
      const fresh = MediaUrl(
        url: 'https://example.com/full.jpg?sig=fresh',
        isThumbnail: false,
        mediaId: mediaId,
      );
      final responses = <MediaUrl>[stale, fresh];
      when(mockRepo.getFullMediaUrl(mediaId))
          .thenAnswer((_) async => responses.removeAt(0));

      final first =
          await container.read(fullMediaUrlProvider(mediaId).future);
      expect(first.url, stale.url);

      container.invalidate(fullMediaUrlProvider(mediaId));

      final second =
          await container.read(fullMediaUrlProvider(mediaId).future);
      expect(second.url, fresh.url);
      verify(mockRepo.getFullMediaUrl(mediaId)).called(2);
    });

    test('repository errors surface as AsyncError on the provider state',
        () async {
      const mediaId = 'm-3';
      when(mockRepo.getFullMediaUrl(mediaId))
          .thenAnswer((_) => Future<MediaUrl>.error(Exception('boom')));

      // Hold a listener so the autoDispose family doesn't tear down before
      // we read the resolved state.
      final sub =
          container.listen(fullMediaUrlProvider(mediaId), (_, _) {}, fireImmediately: true);

      // Wait one microtask cycle for the future error to surface.
      await Future<void>.delayed(Duration.zero);

      final value = container.read(fullMediaUrlProvider(mediaId));
      expect(value.hasError, isTrue);
      expect(value.error, isA<Exception>());

      sub.close();
    });

    test('different mediaIds are isolated', () async {
      const mediaA = MediaUrl(
        url: 'https://example.com/a.jpg',
        isThumbnail: false,
        mediaId: 'a',
      );
      const mediaB = MediaUrl(
        url: 'https://example.com/b.jpg',
        isThumbnail: false,
        mediaId: 'b',
      );
      when(mockRepo.getFullMediaUrl('a')).thenAnswer((_) async => mediaA);
      when(mockRepo.getFullMediaUrl('b')).thenAnswer((_) async => mediaB);

      final results = await Future.wait([
        container.read(fullMediaUrlProvider('a').future),
        container.read(fullMediaUrlProvider('b').future),
      ]);

      expect(results[0], mediaA);
      expect(results[1], mediaB);
      verify(mockRepo.getFullMediaUrl('a')).called(1);
      verify(mockRepo.getFullMediaUrl('b')).called(1);
    });

    test('handles disposal during an in-flight fetch gracefully', () async {
      const mediaId = 'm-4';
      final completer = Completer<MediaUrl>();
      when(mockRepo.getFullMediaUrl(mediaId))
          .thenAnswer((_) => completer.future);

      final future = container.read(fullMediaUrlProvider(mediaId).future);
      container.dispose();
      completer.complete(const MediaUrl(
        url: 'https://example.com/full.jpg',
        isThumbnail: false,
        mediaId: mediaId,
      ));

      // The pending future may resolve or be ignored, but the test must
      // not throw "Cannot use Ref after it has been disposed".
      await expectLater(future.catchError((_) =>
          const MediaUrl(url: '', isThumbnail: false, mediaId: '')),
          completes);
    });
  });
}
