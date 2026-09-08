import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show TransferType, Transfer, TransferState;
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart'
    show CompleteTransferResponse;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/services/transfer_service.dart';

import 'transfer_repository_test.mocks.dart';

@GenerateMocks([TransferService, ChatRepository, GearRepository, CommunityRepository, UserRepository])
void main() {
  group('TransferRepository', () {
    late TransferRepository repository;
    late MockTransferService mockService;
    late MockChatRepository mockChatRepository;
    late MockGearRepository mockGearRepository;
    late MockCommunityRepository mockCommunityRepository;
    late MockUserRepository mockUserRepository;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockTransferService();
      mockChatRepository = MockChatRepository();
      mockGearRepository = MockGearRepository();
      mockCommunityRepository = MockCommunityRepository();
      mockUserRepository = MockUserRepository();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = TransferRepository(
        cacheManager,
        mockService,
        mockChatRepository,
        mockGearRepository,
        mockCommunityRepository,
        mockUserRepository,
      );
    });

    group('listMyTransfers', () {
      test('fetches transfers from service', () async {
        final mockTransfers = <Transfer>[
          Transfer(id: 'transfer1', gearName: 'Tent'),
          Transfer(id: 'transfer2', gearName: 'Kayak'),
        ];

        when(
          mockService.listMyTransfers(),
        ).thenAnswer((_) async => mockTransfers);

        final result = await repository.listMyTransfers();

        expect(result.length, 2);
        expect(result[0].id, 'transfer1');
        expect(result[1].id, 'transfer2');
        verify(mockService.listMyTransfers()).called(1);
      });

      test('caches transfers', () async {
        final mockTransfers = <Transfer>[
          Transfer(id: 'transfer1', gearName: 'Tent'),
        ];

        when(
          mockService.listMyTransfers(),
        ).thenAnswer((_) async => mockTransfers);

        // First call - should fetch
        await repository.listMyTransfers();

        // Second call - should use cache
        await repository.listMyTransfers();

        // Service should only be called once
        verify(mockService.listMyTransfers()).called(1);
      });

      test('filters by transferType when provided', () async {
        final mockTransfers = <Transfer>[
          Transfer(
            id: 'transfer1',
            gearName: 'Tent',
            transferType: TransferType.TRANSFER_TYPE_LOAN,
          ),
        ];

        when(
          mockService.listMyTransfers(
            transferType: TransferType.TRANSFER_TYPE_LOAN,
          ),
        ).thenAnswer((_) async => mockTransfers);

        final result = await repository.listMyTransfers(
          transferType: TransferType.TRANSFER_TYPE_LOAN,
        );

        expect(result.length, 1);
        expect(result[0].transferType, TransferType.TRANSFER_TYPE_LOAN);
        verify(
          mockService.listMyTransfers(
            transferType: TransferType.TRANSFER_TYPE_LOAN,
          ),
        ).called(1);
      });

      test('caches separately by transferType', () async {
        final allTransfers = <Transfer>[
          Transfer(id: 'transfer1', gearName: 'Tent'),
          Transfer(id: 'transfer2', gearName: 'Kayak'),
        ];
        final loanTransfers = <Transfer>[
          Transfer(
            id: 'transfer1',
            gearName: 'Tent',
            transferType: TransferType.TRANSFER_TYPE_LOAN,
          ),
        ];

        when(
          mockService.listMyTransfers(),
        ).thenAnswer((_) async => allTransfers);
        when(
          mockService.listMyTransfers(
            transferType: TransferType.TRANSFER_TYPE_LOAN,
          ),
        ).thenAnswer((_) async => loanTransfers);

        // Fetch all transfers
        final all = await repository.listMyTransfers();
        expect(all.length, 2);

        // Fetch loan-specific transfers (should be separate cache)
        final loans = await repository.listMyTransfers(
          transferType: TransferType.TRANSFER_TYPE_LOAN,
        );
        expect(loans.length, 1);

        // Each should have been called once
        verify(mockService.listMyTransfers()).called(1);
        verify(
          mockService.listMyTransfers(
            transferType: TransferType.TRANSFER_TYPE_LOAN,
          ),
        ).called(1);
      });
    });

    group('listReceivedTransfers', () {
      test('fetches received transfers from service', () async {
        final mockTransfers = <Transfer>[
          Transfer(id: 'transfer1', gearName: 'Tent'),
          Transfer(id: 'transfer2', gearName: 'Kayak'),
        ];

        when(
          mockService.listReceivedTransfers(),
        ).thenAnswer((_) async => mockTransfers);

        final result = await repository.listReceivedTransfers();

        expect(result.length, 2);
        expect(result[0].id, 'transfer1');
        expect(result[1].id, 'transfer2');
        verify(mockService.listReceivedTransfers()).called(1);
      });

      test('caches received transfers', () async {
        final mockTransfers = <Transfer>[
          Transfer(id: 'transfer1', gearName: 'Tent'),
        ];

        when(
          mockService.listReceivedTransfers(),
        ).thenAnswer((_) async => mockTransfers);

        // First call - should fetch
        await repository.listReceivedTransfers();

        // Second call - should use cache
        await repository.listReceivedTransfers();

        // Service should only be called once
        verify(mockService.listReceivedTransfers()).called(1);
      });
    });

    group('getTransfer', () {
      test('fetches transfer by ID from service', () async {
        const transferId = 'transfer123';
        final mockTransfer = Transfer(
          id: transferId,
          gearName: 'Tent',
        );

        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => mockTransfer);

        final result = await repository.getTransfer(transferId);

        expect(result, isNotNull);
        expect(result!.id, transferId);
        expect(result.gearName, 'Tent');
        verify(mockService.getTransfer(transferId: transferId)).called(1);
      });

      test('caches transfer by ID', () async {
        const transferId = 'transfer123';
        final mockTransfer = Transfer(
          id: transferId,
          gearName: 'Tent',
        );

        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => mockTransfer);

        // First call - should fetch from service
        await repository.getTransfer(transferId);

        // Second call - should use cache
        await repository.getTransfer(transferId);

        // Service should only be called once
        verify(mockService.getTransfer(transferId: transferId)).called(1);
      });

      test('handles error when transfer not found', () async {
        const transferId = 'nonexistent';

        when(
          mockService.getTransfer(transferId: transferId),
        ).thenThrow(Exception('Transfer not found'));

        expect(
          () => repository.getTransfer(transferId),
          throwsException,
        );
      });

      test('cache is invalidated after mutations', () async {
        const transferId = 'transfer123';
        final originalTransfer = Transfer(
          id: transferId,
          gearName: 'Tent',
        );
        final updatedTransfer = Transfer(
          id: transferId,
          gearName: 'Updated Tent',
        );

        // Initial fetch
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => originalTransfer);

        final result1 = await repository.getTransfer(transferId);
        expect(result1!.gearName, 'Tent');

        // Perform mutation
        when(
          mockService.startLoan(transferId: transferId),
        ).thenAnswer((_) async => 'event-id');

        await repository.startLoan(transferId: transferId);

        // Fetch again - should get fresh data because cache was invalidated
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => updatedTransfer);

        final result2 = await repository.getTransfer(transferId);
        expect(result2!.gearName, 'Updated Tent');

        // Service should have been called twice (once before mutation, once after)
        verify(mockService.getTransfer(transferId: transferId)).called(2);
      });
    });

    group('mutation operations', () {
      test('offerTransfer creates the gear-backed offer and invalidates gear',
          () async {
        const gearId = 'gear123';
        const transferId = 'transfer123';

        final transfer = Transfer(
          id: transferId,
          gearId: gearId,
          transferType: TransferType.TRANSFER_TYPE_LOAN,
          state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
          originRequestId: 'request1',
        );

        when(
          mockService.offerTransfer(
            gearId: gearId,
            transferType: TransferType.TRANSFER_TYPE_LOAN,
            recipientUserId: 'asker1',
            communityId: 'community1',
            originRequestId: 'request1',
            contributionId: 'contribution1',
            loanDurationDays: null,
          ),
        ).thenAnswer((_) async => transfer);
        when(mockGearRepository.invalidate(gearId)).thenAnswer((_) async {});

        final result = await repository.offerTransfer(
          gearId: gearId,
          transferType: TransferType.TRANSFER_TYPE_LOAN,
          recipientUserId: 'asker1',
          communityId: 'community1',
          originRequestId: 'request1',
          contributionId: 'contribution1',
        );

        expect(result.id, transferId);
        expect(result.state, TransferState.TRANSFER_STATE_RECIPIENT_SELECTED);
        expect(result.originRequestId, 'request1');
        verify(
          mockService.offerTransfer(
            gearId: gearId,
            transferType: TransferType.TRANSFER_TYPE_LOAN,
            recipientUserId: 'asker1',
            communityId: 'community1',
            originRequestId: 'request1',
            contributionId: 'contribution1',
            loanDurationDays: null,
          ),
        ).called(1);
        verify(mockGearRepository.invalidate(gearId)).called(1);
      });

      test('expressInterest creates transfer and invalidates cache', () async {
        const gearId = 'gear123';
        const conversationId = 'conv456';
        const transferId = 'transfer123';

        final transfer = Transfer(
          id: transferId,
          gearId: gearId,
          conversationId: conversationId,
        );

        when(
          mockService.expressInterest(gearId: gearId),
        ).thenAnswer((_) async => transfer);

        // Mock the list calls that will be made by refreshReceivedTransfers
        when(mockService.listReceivedTransfers()).thenAnswer((_) async => []);

        final result = await repository.expressInterest(gearId: gearId);

        expect(result.id, transferId);
        expect(result.conversationId, conversationId);
        verify(mockService.expressInterest(gearId: gearId)).called(1);
      });

      test('expressInterest invalidates conversation cache', () async {
        const gearId = 'gear123';
        const conversationId = 'conv456';
        const transferId = 'transfer123';

        final transfer = Transfer(
          id: transferId,
          gearId: gearId,
          conversationId: conversationId,
        );

        when(
          mockService.expressInterest(gearId: gearId),
        ).thenAnswer((_) async => transfer);
        when(
          mockChatRepository.refreshConversations(),
        ).thenAnswer((_) async {});

        await repository.expressInterest(gearId: gearId);

        // Verify that conversation cache was invalidated
        verify(mockChatRepository.refreshConversations()).called(1);
      });

      test('selectRecipient approves and invalidates cache', () async {
        const transferId = 'transfer123';
        const recipientId = 'user456';

        // Mock getTransfer call (needed by selectRecipient)
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(id: transferId, gearId: 'gear123'));

        when(
          mockService.selectRecipient(
            transferId: transferId,
            recipientId: recipientId,
          ),
        ).thenAnswer(
          (_) async => (conversationId: '', communityEventId: 'event-id'),
        );

        // Mock the list calls that will be made by refresh methods
        when(mockService.listMyTransfers()).thenAnswer((_) async => []);

        await repository.selectRecipient(
          transferId: transferId,
          recipientId: recipientId,
        );

        verify(
          mockService.selectRecipient(
            transferId: transferId,
            recipientId: recipientId,
          ),
        ).called(1);
      });

      test('selectRecipient invalidates conversation cache', () async {
        const transferId = 'transfer123';
        const recipientId = 'user456';

        // Mock getTransfer call (needed by selectRecipient)
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(id: transferId, gearId: 'gear123'));

        when(
          mockService.selectRecipient(
            transferId: transferId,
            recipientId: recipientId,
          ),
        ).thenAnswer(
          (_) async => (conversationId: 'conv789', communityEventId: 'event-id'),
        );
        when(
          mockChatRepository.refreshConversations(),
        ).thenAnswer((_) async {});

        await repository.selectRecipient(
          transferId: transferId,
          recipientId: recipientId,
        );

        // Verify that conversation cache was invalidated
        verify(mockChatRepository.refreshConversations()).called(1);
      });

      test('cancelTransfer cancels and invalidates both caches', () async {
        const transferId = 'transfer123';

        // Mock getTransfer call (needed by cancelTransfer)
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(id: transferId, gearId: 'gear123'));

        when(
          mockService.cancelTransfer(transferId: transferId),
        ).thenAnswer((_) async => 'event-id');

        // Mock the list calls that will be made by refresh methods
        when(mockService.listMyTransfers()).thenAnswer((_) async => []);
        when(mockService.listReceivedTransfers()).thenAnswer((_) async => []);

        await repository.cancelTransfer(transferId: transferId);

        verify(mockService.cancelTransfer(transferId: transferId)).called(1);
      });

      test('startLoan starts and invalidates both caches', () async {
        const transferId = 'transfer123';

        // Mock getTransfer call (needed to get gearId for cache invalidation)
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(id: transferId, gearId: 'gear123'));

        when(
          mockService.startLoan(transferId: transferId),
        ).thenAnswer((_) async => 'event-id');

        // Mock the list calls that will be made by refresh methods
        when(mockService.listMyTransfers()).thenAnswer((_) async => []);
        when(mockService.listReceivedTransfers()).thenAnswer((_) async => []);

        await repository.startLoan(transferId: transferId);

        verify(mockService.startLoan(transferId: transferId)).called(1);
      });

      test('completeTransfer completes and invalidates both caches', () async {
        const transferId = 'transfer123';

        // Mock getTransfer call (needed to get gearId for cache invalidation)
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(id: transferId, gearId: 'gear123'));

        when(
          mockService.completeTransfer(transferId: transferId),
        ).thenAnswer((_) async => CompleteTransferResponse());

        // Mock the list calls that will be made by refresh methods
        when(mockService.listMyTransfers()).thenAnswer((_) async => []);
        when(mockService.listReceivedTransfers()).thenAnswer((_) async => []);

        await repository.completeTransfer(transferId: transferId);

        verify(mockService.completeTransfer(transferId: transferId)).called(1);
      });

      test('completeTransfer invalidates past giveaways cache for giveaways', () async {
        const transferId = 'transfer123';

        // Mock getTransfer call with GIVEAWAY type
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(
          id: transferId,
          gearId: 'gear123',
          transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
        ));

        when(
          mockService.completeTransfer(transferId: transferId),
        ).thenAnswer((_) async => CompleteTransferResponse());

        // Mock the invalidateCompletedGiveawaysAll call
        when(mockCommunityRepository.invalidateCompletedGiveawaysAll())
            .thenAnswer((_) async => 'event-id');

        // Mock refreshUserGear so it doesn't throw
        when(mockGearRepository.refreshUserGear()).thenAnswer((_) async => []);

        await repository.completeTransfer(transferId: transferId);

        // Verify past giveaways cache was invalidated
        verify(mockCommunityRepository.invalidateCompletedGiveawaysAll()).called(1);
      });

      test(
        'completeTransfer calls refreshUserGear for giveaway so list cache is cleared',
        () async {
          const transferId = 'transfer123';
          const gearId = 'gear123';

          when(
            mockService.getTransfer(transferId: transferId),
          ).thenAnswer((_) async => Transfer(
            id: transferId,
            gearId: gearId,
            transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
          ));

          when(
            mockService.completeTransfer(transferId: transferId),
          ).thenAnswer((_) async => CompleteTransferResponse());

          when(mockCommunityRepository.invalidateCompletedGiveawaysAll())
              .thenAnswer((_) async => 'event-id');

          when(mockGearRepository.refreshUserGear()).thenAnswer((_) async => []);

          await repository.completeTransfer(transferId: transferId);

          verify(mockGearRepository.refreshUserGear()).called(1);
        },
      );

      test(
        'completeTransfer does not call refreshUserGear for loans',
        () async {
          const transferId = 'transfer123';
          const gearId = 'gear123';

          when(
            mockService.getTransfer(transferId: transferId),
          ).thenAnswer((_) async => Transfer(
            id: transferId,
            gearId: gearId,
            transferType: TransferType.TRANSFER_TYPE_LOAN,
          ));

          when(
            mockService.completeTransfer(transferId: transferId),
          ).thenAnswer((_) async => CompleteTransferResponse());

          await repository.completeTransfer(transferId: transferId);

          verifyNever(mockGearRepository.refreshUserGear());
        },
      );

      test('completeTransfer does not invalidate past giveaways for loans', () async {
        const transferId = 'transfer123';

        // Mock getTransfer call with LOAN type
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(
          id: transferId,
          gearId: 'gear123',
          transferType: TransferType.TRANSFER_TYPE_LOAN,
        ));

        when(
          mockService.completeTransfer(transferId: transferId),
        ).thenAnswer((_) async => CompleteTransferResponse());

        await repository.completeTransfer(transferId: transferId);

        // Verify past giveaways cache was NOT invalidated for loans
        verifyNever(mockCommunityRepository.invalidateCompletedGiveawaysAll());
      });

      test('completeTransfer invalidates gear cache to refresh activeLoan', () async {
        const transferId = 'transfer123';
        const gearId = 'gear123';

        // Mock getTransfer call
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(
          id: transferId,
          gearId: gearId,
          transferType: TransferType.TRANSFER_TYPE_LOAN,
        ));

        when(
          mockService.completeTransfer(transferId: transferId),
        ).thenAnswer((_) async => CompleteTransferResponse());

        // Mock gear repository invalidate
        when(mockGearRepository.invalidate(any))
            .thenAnswer((_) async => 'event-id');

        await repository.completeTransfer(transferId: transferId);

        // Verify gear cache was invalidated so activeLoan is refreshed
        verify(mockGearRepository.invalidate(gearId)).called(1);
      });

      test('startLoan invalidates gear cache to set activeLoan', () async {
        const transferId = 'transfer123';
        const gearId = 'gear123';

        // Mock getTransfer call
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(
          id: transferId,
          gearId: gearId,
          transferType: TransferType.TRANSFER_TYPE_LOAN,
        ));

        when(
          mockService.startLoan(transferId: transferId),
        ).thenAnswer((_) async => 'event-id');

        // Mock gear repository invalidate
        when(mockGearRepository.invalidate(any))
            .thenAnswer((_) async => 'event-id');

        await repository.startLoan(transferId: transferId);

        // Verify gear cache was invalidated so activeLoan is set
        verify(mockGearRepository.invalidate(gearId)).called(1);
      });

      test('cancelTransfer invalidates gear cache when transfer was ACTIVE', () async {
        const transferId = 'transfer123';
        const gearId = 'gear123';

        // Mock getTransfer call with ACTIVE state
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(
          id: transferId,
          gearId: gearId,
          state: TransferState.TRANSFER_STATE_ACTIVE,
        ));

        when(
          mockService.cancelTransfer(transferId: transferId),
        ).thenAnswer((_) async => 'event-id');

        // Mock gear repository invalidate
        when(mockGearRepository.invalidate(any))
            .thenAnswer((_) async => 'event-id');

        await repository.cancelTransfer(transferId: transferId);

        // Verify gear cache was invalidated since transfer was ACTIVE
        verify(mockGearRepository.invalidate(gearId)).called(1);
      });

      test('cancelTransfer does not invalidate gear cache when transfer was not ACTIVE', () async {
        const transferId = 'transfer123';
        const gearId = 'gear123';

        // Mock getTransfer call with RECIPIENT_SELECTED state (not ACTIVE)
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(
          id: transferId,
          gearId: gearId,
          state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
        ));

        when(
          mockService.cancelTransfer(transferId: transferId),
        ).thenAnswer((_) async => 'event-id');

        await repository.cancelTransfer(transferId: transferId);

        // Verify gear cache was NOT invalidated since transfer was not ACTIVE
        verifyNever(mockGearRepository.invalidate(gearId));
      });
    });

    group('error handling', () {
      test('handles service errors in listMyTransfers', () {
        when(
          mockService.listMyTransfers(),
        ).thenThrow(Exception('Network error'));

        expect(() => repository.listMyTransfers(), throwsException);
      });

      test('handles service errors in listReceivedTransfers', () {
        when(
          mockService.listReceivedTransfers(),
        ).thenThrow(Exception('Network error'));

        expect(() => repository.listReceivedTransfers(), throwsException);
      });

      test('handles service errors in mutation operations', () {
        const transferId = 'transfer123';

        // Mock getTransfer call (needed by cancelTransfer)
        when(
          mockService.getTransfer(transferId: transferId),
        ).thenAnswer((_) async => Transfer(id: transferId, gearId: 'gear123'));

        when(
          mockService.cancelTransfer(transferId: transferId),
        ).thenThrow(Exception('Network error'));

        expect(
          () => repository.cancelTransfer(transferId: transferId),
          throwsException,
        );
      });
    });
  });
}
