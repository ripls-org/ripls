import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/auth_repository.dart';
import 'package:ripls/data/repositories/share_link_repository.dart';

import 'share_link_repository_test.mocks.dart';

@GenerateMocks([AuthRepository])
void main() {
  group('ShareLinkRepository', () {
    late ShareLinkRepository repository;
    late MockAuthRepository mockAuthRepository;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockAuthRepository = MockAuthRepository();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = ShareLinkRepository(cacheManager, mockAuthRepository);
    });

    InvitationCheckResult eventResult({String code = 'abc12345'}) {
      return InvitationCheckResult(
        isValid: true,
        communityId: 'comm-1',
        communityName: 'Two Oaks Block',
        inviterName: 'Alice',
        errorMessage: '',
        numMembers: 5,
        maxMembers: 32,
        targetKind: ShareLinkTargetKind.SHARE_LINK_TARGET_KIND_EVENT,
        targetId: 'exp-$code',
      );
    }

    test('describe fetches from service on cache miss', () async {
      const code = 'abcd1234';
      final fixture = eventResult(code: code);
      when(mockAuthRepository.checkInvitation(shortCode: code))
          .thenAnswer((_) async => fixture);

      final result = await repository.describe(code);

      expect(result.isEvent, isTrue);
      expect(result.targetId, 'exp-$code');
      verify(mockAuthRepository.checkInvitation(shortCode: code)).called(1);
    });

    test('describe returns cached value on cache hit', () async {
      const code = 'abcd1234';
      final fixture = eventResult(code: code);
      when(mockAuthRepository.checkInvitation(shortCode: code))
          .thenAnswer((_) async => fixture);

      await repository.describe(code);
      await repository.describe(code); // second call

      // Second describe should hit the cache, not the service.
      verify(mockAuthRepository.checkInvitation(shortCode: code)).called(1);
    });

    test('invalidate clears the cache so the next describe re-fetches',
        () async {
      const code = 'abcd1234';
      when(mockAuthRepository.checkInvitation(shortCode: code))
          .thenAnswer((_) async => eventResult(code: code));

      await repository.describe(code);
      await repository.invalidate(code);
      await repository.describe(code);

      verify(mockAuthRepository.checkInvitation(shortCode: code)).called(2);
    });
  });
}
