import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show Transfer;
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/repositories/share_link_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/web_action_handoff_view_model.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/gear_providers.dart';
import 'package:ripls/services/providers/request_providers.dart';

import 'web_action_handoff_view_model_test.mocks.dart';

@GenerateMocks([TransferRepository, RequestRepository, ShareLinkRepository])
void main() {
  late MockTransferRepository mockTransferRepo;
  late MockRequestRepository mockRequestRepo;
  late MockShareLinkRepository mockShareLinkRepo;
  late ProviderContainer container;

  const gearId = 'gear-123';
  const requestId = 'request-123';
  const shortCode = 'TESTCODE';
  const communityId = 'community-456';

  InvitationCheckResult okInvitation() {
    return InvitationCheckResult(
      isValid: true,
      communityId: communityId,
      communityName: 'Test Community',
      inviterName: 'Inviter',
      numMembers: 5,
      maxMembers: 32,
      targetId: requestId,
      errorMessage: '',
    );
  }

  setUp(() {
    mockTransferRepo = MockTransferRepository();
    mockRequestRepo = MockRequestRepository();
    mockShareLinkRepo = MockShareLinkRepository();

    container = ProviderContainer(
      overrides: [
        transferRepositoryProvider.overrideWithValue(mockTransferRepo),
        requestRepositoryProvider.overrideWithValue(mockRequestRepo),
        shareLinkRepositoryProvider.overrideWithValue(mockShareLinkRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('WebGearInterestHandoffNotifier', () {
    test('attempt() fires ExpressInterest and lands in succeeded', () async {
      when(
        mockTransferRepo.expressInterest(gearId: anyNamed('gearId')),
      ).thenAnswer((_) async => Transfer());

      final notifier = container.read(
        webGearInterestHandoffProvider(gearId).notifier,
      );
      await notifier.attempt(shortCode: shortCode);

      expect(
        container.read(webGearInterestHandoffProvider(gearId)).hasSucceeded,
        isTrue,
      );
      verify(mockTransferRepo.expressInterest(gearId: gearId)).called(1);
      // Gear interest derives the community server-side — no share-link lookup.
      verifyNever(mockShareLinkRepo.describe(any));
    });

    test('attempt() is a no-op once succeeded', () async {
      when(
        mockTransferRepo.expressInterest(gearId: anyNamed('gearId')),
      ).thenAnswer((_) async => Transfer());

      final notifier = container.read(
        webGearInterestHandoffProvider(gearId).notifier,
      );
      await notifier.attempt(shortCode: shortCode);
      await notifier.attempt(shortCode: shortCode);

      verify(mockTransferRepo.expressInterest(gearId: gearId)).called(1);
    });

    test('exhausting retries lands in failed, then retry() recovers', () async {
      var calls = 0;
      when(
        mockTransferRepo.expressInterest(gearId: anyNamed('gearId')),
      ).thenAnswer((_) async {
        calls++;
        if (calls <= 3) throw Exception('network down');
        return Transfer();
      });

      // Keep the autoDispose family provider mounted through the retry loop.
      container.listen(webGearInterestHandoffProvider(gearId), (_, _) {});
      final notifier = container.read(
        webGearInterestHandoffProvider(gearId).notifier,
      );

      await notifier.attempt(shortCode: shortCode);
      final failed = container.read(webGearInterestHandoffProvider(gearId));
      expect(failed.hasFailed, isTrue, reason: 'attempts=${failed.attempts}');
      expect(failed.attempts, 3);

      await notifier.retry(shortCode: shortCode);
      expect(
        container.read(webGearInterestHandoffProvider(gearId)).hasSucceeded,
        isTrue,
      );
    });
  });

  group('WebRequestOfferHandoffNotifier', () {
    test('attempt() resolves the community then fires OfferToFulfill', () async {
      when(
        mockShareLinkRepo.describe(shortCode),
      ).thenAnswer((_) async => okInvitation());
      when(
        mockRequestRepo.offerToFulfill(
          requestId: anyNamed('requestId'),
          communityId: anyNamed('communityId'),
        ),
      ).thenAnswer((_) async => Request());

      final notifier = container.read(
        webRequestOfferHandoffProvider(requestId).notifier,
      );
      await notifier.attempt(shortCode: shortCode);

      expect(
        container.read(webRequestOfferHandoffProvider(requestId)).hasSucceeded,
        isTrue,
      );
      verify(
        mockRequestRepo.offerToFulfill(
          requestId: requestId,
          communityId: communityId,
        ),
      ).called(1);
    });

    test('a share link with no community_id fails the attempt', () async {
      when(mockShareLinkRepo.describe(shortCode)).thenAnswer(
        (_) async => InvitationCheckResult(
          isValid: true,
          communityId: '', // missing — OfferToFulfill needs an explicit one
          communityName: '',
          inviterName: '',
          numMembers: 0,
          maxMembers: 32,
          errorMessage: '',
        ),
      );

      container.listen(webRequestOfferHandoffProvider(requestId), (_, _) {});
      final notifier = container.read(
        webRequestOfferHandoffProvider(requestId).notifier,
      );
      await notifier.attempt(shortCode: shortCode);

      expect(
        container.read(webRequestOfferHandoffProvider(requestId)).hasFailed,
        isTrue,
      );
      verifyNever(
        mockRequestRepo.offerToFulfill(
          requestId: anyNamed('requestId'),
          communityId: anyNamed('communityId'),
        ),
      );
    });
  });
}
