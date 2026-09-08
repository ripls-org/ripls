import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/esm_repository.dart';
import 'package:ripls/services/esm_service.dart';

import 'esm_repository_test.mocks.dart';

@GenerateMocks([EsmService])
void main() {
  late MockEsmService mockService;
  late StashCacheManager cacheManager;
  late int feedListingNotifications;
  late int esmNotifications;
  late EsmRepository repository;

  setUp(() async {
    mockService = MockEsmService();
    cacheManager = StashCacheManager();
    await cacheManager.initialize();
    feedListingNotifications = 0;
    esmNotifications = 0;
    repository = EsmRepository(
      cacheManager,
      mockService,
      onFeedListingInvalidated: () => feedListingNotifications++,
      onEsmInvalidated: () => esmNotifications++,
    );
  });

  group('respond', () {
    test('calls service and notifies both invalidation channels', () async {
      when(mockService.respondToESMPrompt(
        promptId: anyNamed('promptId'),
        responseOptionKey: anyNamed('responseOptionKey'),
      )).thenAnswer((_) async => RespondToESMPromptResponse());

      await repository.respond(promptId: 'p1', responseOptionKey: 'do_again');

      verify(mockService.respondToESMPrompt(
        promptId: 'p1',
        responseOptionKey: 'do_again',
      )).called(1);
      expect(feedListingNotifications, 1);
      expect(esmNotifications, 1);
    });

    test('propagates service error and does not notify on failure', () async {
      when(mockService.respondToESMPrompt(
        promptId: anyNamed('promptId'),
        responseOptionKey: anyNamed('responseOptionKey'),
      )).thenThrow(Exception('boom'));

      await expectLater(
        repository.respond(promptId: 'p1', responseOptionKey: 'do_again'),
        throwsException,
      );
      expect(feedListingNotifications, 0);
      expect(esmNotifications, 0);
    });
  });
}
