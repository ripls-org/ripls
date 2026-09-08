import 'package:fixnum/fixnum.dart' show Int64;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show Transfer, TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/gear_sharing_view_model.dart';
import 'package:ripls/services/chat_service.dart';
import 'package:ripls/services/providers.dart';

import 'gear_sharing_view_model_test.mocks.dart';

@GenerateMocks([
  TransferRepository,
  ChatService,
  ChatRepository,
  CommunityRepository,
])
void main() {
  late ProviderContainer container;
  late MockTransferRepository mockTransferRepository;
  late MockChatService mockChatService;
  late MockChatRepository mockChatRepository;
  late MockCommunityRepository mockCommunityRepository;

  setUp(() {
    mockTransferRepository = MockTransferRepository();
    mockChatService = MockChatService();
    mockChatRepository = MockChatRepository();
    mockCommunityRepository = MockCommunityRepository();

    container = ProviderContainer(
      overrides: [
        transferRepositoryProvider.overrideWithValue(mockTransferRepository),
        chatServiceProvider.overrideWithValue(mockChatService),
        chatRepositoryProvider.overrideWithValue(mockChatRepository),
        communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockTransferRepository);
    reset(mockChatService);
    reset(mockChatRepository);
    reset(mockCommunityRepository);
  });

  group('GearSharingState', () {
    test('initial state is correct', () {
      final state = container.read(gearSharingProvider);

      expect(state.gear, isNull);
      expect(state.isOwner, isFalse);
      expect(state.communityId, isNull);
      expect(state.isSubmitting, isFalse);
      expect(state.isCanceling, isFalse);
      expect(state.error, isNull);
      expect(state.submittedTransferId, isNull);
      expect(state.receivedRequests, isEmpty);
      expect(state.extendedRequests, isEmpty);
      expect(state.isLoadingRequests, isFalse);
      expect(state.requestsError, isNull);
    });

    test('hasActiveTransferRequest returns correct value', () {
      final notifier = container.read(gearSharingProvider.notifier);

      // No active transfer request initially
      expect(container.read(gearSharingProvider).hasActiveTransferRequest, isFalse);

      // Set transfer ID
      notifier.state = notifier.state.copyWith(submittedTransferId: 'transfer-123');
      expect(container.read(gearSharingProvider).hasActiveTransferRequest, isTrue);
    });

    test('hasError returns correct value', () {
      final notifier = container.read(gearSharingProvider.notifier);

      // No error initially
      expect(container.read(gearSharingProvider).hasError, isFalse);

      // Set error
      notifier.state = notifier.state.copyWith(
          error: const UserError.generic(fallback: 'Test error'));
      expect(container.read(gearSharingProvider).hasError, isTrue);
    });

    test('canWithdraw returns false when owner', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: true,
        submittedTransferId: 'transfer-123',
        submittedTransferState: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );

      expect(container.read(gearSharingProvider).canWithdraw, isFalse);
    });

    test('canWithdraw returns false when no transfer', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        submittedTransferId: null,
      );

      expect(container.read(gearSharingProvider).canWithdraw, isFalse);
    });

    test('canWithdraw returns false when transfer state is not INTEREST_EXPRESSED', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        submittedTransferId: 'transfer-123',
        submittedTransferState: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );

      expect(container.read(gearSharingProvider).canWithdraw, isFalse);
    });

    test('canWithdraw returns true when conditions are met', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        submittedTransferId: 'transfer-123',
        submittedTransferState: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );

      expect(container.read(gearSharingProvider).canWithdraw, isTrue);
    });
  });


  group('expressInterest', () {
    test('successfully expresses interest', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      final transfer = Transfer(
        id: 'transfer-123',
        gearId: 'gear-123',
        conversationId: 'conv-123',
        gearName: 'Camping Tent',
      );

      when(mockTransferRepository.expressInterest(
        gearId: anyNamed('gearId'),
      )).thenAnswer((_) async => transfer);
      when(mockTransferRepository.listReceivedTransfers())
        .thenAnswer((_) async => []);

      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        gear: gear,
        communityId: 'community-123',
      );

      final result = await notifier.expressInterest('Need this gear');

      expect(result, isA<Transfer>());
      expect(result.conversationId, 'conv-123');
      expect(result.id, 'transfer-123');
      final state = container.read(gearSharingProvider);
      expect(state.isSubmitting, isFalse);
      expect(state.error, isNull);
    });

    test('throws error when gear not initialized', () async {
      final notifier = container.read(gearSharingProvider.notifier);

      expect(
        () => notifier.expressInterest('Need this gear'),
        throwsA(isA<StateError>().having(
          (e) => e.message,
          'message',
          'Gear not initialized',
        )),
      );
    });
  });

  group('withdrawInterest', () {
    test('returns error when no active transfer request', () async {
      final notifier = container.read(gearSharingProvider.notifier);

      final error = await notifier.withdrawInterest();

      expect(error, 'No active transfer request to withdraw');
    });

    test('returns error when canWithdraw is false', () async {
      final notifier = container.read(gearSharingProvider.notifier);
      // Set up a transfer that cannot be withdrawn (e.g., already selected)
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        submittedTransferId: 'transfer-123',
        submittedTransferState: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );

      final error = await notifier.withdrawInterest();

      expect(error, 'Cannot withdraw at this stage');
    });

    test('successfully withdraws interest', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockTransferRepository.withdrawInterest(
        transferId: anyNamed('transferId'),
      )).thenAnswer((_) async {});

      when(mockTransferRepository.listReceivedTransfers())
          .thenAnswer((_) async => []);

      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        gear: gear,
        isOwner: false,
        submittedTransferId: 'transfer-123',
        submittedTransferState: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );

      final error = await notifier.withdrawInterest();

      expect(error, isNull);
      final state = container.read(gearSharingProvider);
      expect(state.submittedTransferId, isNull);
      expect(state.submittedTransferState, isNull);
      expect(state.isWithdrawing, isFalse);
      verify(mockTransferRepository.withdrawInterest(transferId: 'transfer-123')).called(1);
    });

    test('handles service error during withdrawal', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockTransferRepository.withdrawInterest(
        transferId: anyNamed('transferId'),
      )).thenThrow(Exception('Network error'));

      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        gear: gear,
        isOwner: false,
        submittedTransferId: 'transfer-123',
        submittedTransferState: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );

      final error = await notifier.withdrawInterest();

      expect(error, contains('Failed to withdraw interest'));
      final state = container.read(gearSharingProvider);
      // Transfer ID should still be set since withdrawal failed
      expect(state.submittedTransferId, 'transfer-123');
      expect(state.isWithdrawing, isFalse);
    });
  });

  group('cancelTransferRequest', () {
    test('returns error when no active transfer request', () async {
      final notifier = container.read(gearSharingProvider.notifier);

      final error = await notifier.cancelTransferRequest();

      expect(error, 'No active transfer request to cancel');
    });
  });

  group('selectRecipient', () {
    test('returns error when not owner', () async {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(isOwner: false);

      final error = await notifier.selectRecipient('transfer-123', 'user-456');

      expect(error, 'Only the owner can select recipients');
    });
  });

  group('declineTransferRequest', () {
    test('returns error when not owner', () async {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(isOwner: false);

      final error = await notifier.declineTransferRequest('transfer-123');

      expect(error, 'Only the owner can decline transfer requests');
    });
  });

  group('loadTransferConversations', () {
    test('loads conversations successfully for all transfers', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      final transfer1 = Transfer(id: 'transfer-1', gearId: 'gear-123');
      final transfer2 = Transfer(id: 'transfer-2', gearId: 'gear-123');
      final conversation1 = ConversationItem(conversationId: 'conv-1');
      final conversation2 = ConversationItem(conversationId: 'conv-2');

      when(mockTransferRepository.getGearTransfers(gearId: 'gear-123'))
          .thenAnswer((_) async => [transfer1, transfer2]);
      when(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-1',
      )).thenAnswer((_) async => conversation1);
      when(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-2',
      )).thenAnswer((_) async => conversation2);

      final notifier = container.read(gearSharingProvider.notifier);
      await notifier.initialize(
        gear: gear,
        isOwner: true,
        communityId: 'community-123',
      );

      final state = container.read(gearSharingProvider);
      expect(state.conversations.length, 2);
      expect(state.conversations['transfer-1'], conversation1);
      expect(state.conversations['transfer-2'], conversation2);
      expect(state.isLoadingConversations, false);

      verify(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-1',
      )).called(1);
      verify(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-2',
      )).called(1);
    });

    test('handles partial conversation load failures gracefully', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      final transfer1 = Transfer(id: 'transfer-1', gearId: 'gear-123');
      final transfer2 = Transfer(id: 'transfer-2', gearId: 'gear-123');
      final conversation1 = ConversationItem(conversationId: 'conv-1');

      when(mockTransferRepository.getGearTransfers(gearId: 'gear-123'))
          .thenAnswer((_) async => [transfer1, transfer2]);
      when(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-1',
      )).thenAnswer((_) async => conversation1);
      when(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-2',
      )).thenThrow(Exception('Network error'));

      final notifier = container.read(gearSharingProvider.notifier);
      await notifier.initialize(
        gear: gear,
        isOwner: true,
        communityId: 'community-123',
      );

      final state = container.read(gearSharingProvider);
      expect(state.conversations.length, 2);
      expect(state.conversations['transfer-1'], conversation1);
      expect(state.conversations['transfer-2'], null); // Failed to load
      expect(state.isLoadingConversations, false);
    });

    test('sets loading state correctly during conversation fetch', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      final transfer = Transfer(id: 'transfer-1', gearId: 'gear-123');
      final conversation = ConversationItem(conversationId: 'conv-1');

      when(mockTransferRepository.getGearTransfers(gearId: 'gear-123'))
          .thenAnswer((_) async => [transfer]);
      when(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-1',
      )).thenAnswer((_) async {
        // Verify loading state during conversation fetch
        final state = container.read(gearSharingProvider);
        expect(state.isLoadingConversations, true);
        return conversation;
      });

      final notifier = container.read(gearSharingProvider.notifier);
      await notifier.initialize(
        gear: gear,
        isOwner: true,
        communityId: 'community-123',
      );

      final state = container.read(gearSharingProvider);
      expect(state.isLoadingConversations, false);
    });

    test('does not load conversations for non-owners', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockTransferRepository.listReceivedTransfers())
          .thenAnswer((_) async => []);

      final notifier = container.read(gearSharingProvider.notifier);
      await notifier.initialize(
        gear: gear,
        isOwner: false, // Borrower view
        communityId: 'community-123',
      );

      final state = container.read(gearSharingProvider);
      expect(state.conversations.isEmpty, true);
      verifyNever(mockChatRepository.getConversationForTransfer(
        transferId: anyNamed('transferId'),
      ));
    });

    test('handles empty transfer list without errors', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      when(mockTransferRepository.getGearTransfers(gearId: 'gear-123'))
          .thenAnswer((_) async => []);

      final notifier = container.read(gearSharingProvider.notifier);
      await notifier.initialize(
        gear: gear,
        isOwner: true,
        communityId: 'community-123',
      );

      final state = container.read(gearSharingProvider);
      expect(state.conversations.isEmpty, true);
      expect(state.isLoadingConversations, false);
      verifyNever(mockChatRepository.getConversationForTransfer(
        transferId: anyNamed('transferId'),
      ));
    });

    test('uses ChatRepository not ChatService (architecture compliance)', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        description: 'Test Description',
        owner: User(id: 'owner-123', name: 'Test Owner'),
        mediaIds: [],
      );

      final transfer = Transfer(id: 'transfer-1', gearId: 'gear-123');
      final conversation = ConversationItem(conversationId: 'conv-1');

      when(mockTransferRepository.getGearTransfers(gearId: 'gear-123'))
          .thenAnswer((_) async => [transfer]);
      when(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-1',
      )).thenAnswer((_) async => conversation);

      final notifier = container.read(gearSharingProvider.notifier);
      await notifier.initialize(
        gear: gear,
        isOwner: true,
        communityId: 'community-123',
      );

      // Verify ChatRepository was called, not ChatService
      verify(mockChatRepository.getConversationForTransfer(
        transferId: 'transfer-1',
      )).called(1);
      verifyNever(mockChatService.getConversationForTransfer(
        transferId: anyNamed('transferId'),
      ));
    });
  });

  group('Phase 5: Computed Properties', () {
    Transfer createTransfer({
      required String id,
      required TransferState state,
      String? recipientId,
      String recipientName = 'Recipient',
      int latestRequestUnixSec = 1000,
    }) {
      return Transfer(
        id: id,
        gearId: 'gear-123',
        state: state,
        recipient: recipientId != null
            ? User(id: recipientId, name: recipientName)
            : User(),
        latestRequestUnixSec: Int64(latestRequestUnixSec),
      );
    }

    test('displayTransfers returns non-cancelled transfers for owner', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: true,
        receivedRequests: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
          ),
          createTransfer(
            id: 't2',
            state: TransferState.TRANSFER_STATE_CANCELLED,
          ),
          createTransfer(
            id: 't3',
            state: TransferState.TRANSFER_STATE_ACTIVE,
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.displayTransfers.length, 2);
      expect(state.displayTransfers.map((t) => t.id), ['t1', 't3']);
    });

    test('displayTransfers returns non-cancelled transfers for non-owner', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        allGearTransfers: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
          ),
          createTransfer(
            id: 't2',
            state: TransferState.TRANSFER_STATE_CANCELLED,
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.displayTransfers.length, 1);
      expect(state.displayTransfers.first.id, 't1');
    });

    test('activeTransfer returns transfer past INTEREST_EXPRESSED', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: true,
        receivedRequests: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
          ),
          createTransfer(
            id: 't2',
            state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.activeTransfer, isNotNull);
      expect(state.activeTransfer!.id, 't2');
    });

    test('activeTransfer returns null when all are pending', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: true,
        receivedRequests: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.activeTransfer, isNull);
    });

    test('pendingRequests returns only INTEREST_EXPRESSED transfers', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: true,
        receivedRequests: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
          ),
          createTransfer(
            id: 't2',
            state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
          ),
          createTransfer(
            id: 't3',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.pendingRequests.length, 2);
      expect(state.pendingRequests.map((t) => t.id), ['t1', 't3']);
    });

    test('currentUserTransfer returns transfer for current user', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        currentUserId: 'user-123',
        allGearTransfers: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
            recipientId: 'other-user',
          ),
          createTransfer(
            id: 't2',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
            recipientId: 'user-123',
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.currentUserTransfer, isNotNull);
      expect(state.currentUserTransfer!.id, 't2');
    });

    test('currentUserTransfer returns null when user has no transfer', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        currentUserId: 'user-123',
        allGearTransfers: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
            recipientId: 'other-user',
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.currentUserTransfer, isNull);
    });

    test('isSelectedRecipient returns true when user is active recipient', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        currentUserId: 'user-123',
        allGearTransfers: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
            recipientId: 'user-123',
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.isSelectedRecipient, isTrue);
    });

    test('isSelectedRecipient returns false when user is not selected', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        currentUserId: 'user-123',
        allGearTransfers: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
            recipientId: 'other-user',
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.isSelectedRecipient, isFalse);
    });

    test('hasExpressedInterest returns true when user has transfer', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: false,
        currentUserId: 'user-123',
        allGearTransfers: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
            recipientId: 'user-123',
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.hasExpressedInterest, isTrue);
    });

    test('orderedTransfers puts active first, then pending by timestamp', () {
      final notifier = container.read(gearSharingProvider.notifier);
      notifier.state = notifier.state.copyWith(
        isOwner: true,
        receivedRequests: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
            latestRequestUnixSec: 3000, // Later
          ),
          createTransfer(
            id: 't2',
            state: TransferState.TRANSFER_STATE_ACTIVE,
            latestRequestUnixSec: 1000,
          ),
          createTransfer(
            id: 't3',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
            latestRequestUnixSec: 2000, // Earlier
          ),
        ],
      );

      final state = container.read(gearSharingProvider);
      expect(state.orderedTransfers.length, 3);
      // Active first
      expect(state.orderedTransfers[0].id, 't2');
      // Then pending by timestamp (oldest first)
      expect(state.orderedTransfers[1].id, 't3');
      expect(state.orderedTransfers[2].id, 't1');
    });

    test('hasActiveTransferInProgress depends on activeTransfer', () {
      final notifier = container.read(gearSharingProvider.notifier);

      // No active transfer
      notifier.state = notifier.state.copyWith(
        isOwner: true,
        receivedRequests: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
          ),
        ],
      );
      expect(container.read(gearSharingProvider).hasActiveTransferInProgress, isFalse);

      // With active transfer
      notifier.state = notifier.state.copyWith(
        receivedRequests: [
          createTransfer(
            id: 't1',
            state: TransferState.TRANSFER_STATE_ACTIVE,
          ),
        ],
      );
      expect(container.read(gearSharingProvider).hasActiveTransferInProgress, isTrue);
    });
  });

  group('loadUserCommunities', () {
    // Regression for #1674: when the modal is opened from a context with no
    // selected community (e.g. All Circles), the notifier may not have been
    // initialize()d with gear context yet. The user's community list still
    // needs to load so the "share with another community" picker isn't empty.
    test('loads communities even when notifier is uninitialized', () async {
      final communities = [
        CommunityItem(id: 'c1', name: 'A'),
        CommunityItem(id: 'c2', name: 'B'),
        CommunityItem(id: 'c3', name: 'C'),
      ];
      when(mockCommunityRepository.listUserCommunities())
          .thenAnswer((_) async => communities);

      final notifier = container.read(gearSharingProvider.notifier);

      // Note: no initialize() call — gear is null, isOwner is false.
      await notifier.loadUserCommunities();

      final state = container.read(gearSharingProvider);
      expect(state.userCommunities, hasLength(3));
      expect(state.userCommunities.map((c) => c.id), ['c1', 'c2', 'c3']);
      expect(state.isLoadingCommunities, isFalse);
      expect(state.communitiesError, isNull);
      verify(mockCommunityRepository.listUserCommunities()).called(1);
    });

    test('loads communities after initialize and triggers sharing status', () async {
      final gear = GetGearResponse(
        id: 'gear-123',
        name: 'Test Gear',
        owner: User(id: 'owner-123', name: 'Owner'),
      );
      final communities = [CommunityItem(id: 'c1', name: 'A')];

      when(mockTransferRepository.getGearTransfers(gearId: 'gear-123'))
          .thenAnswer((_) async => []);
      when(mockCommunityRepository.listUserCommunities())
          .thenAnswer((_) async => communities);
      when(mockCommunityRepository.listCommunityGear('c1'))
          .thenAnswer((_) async => []);

      final notifier = container.read(gearSharingProvider.notifier);
      await notifier.initialize(
        gear: gear,
        isOwner: true,
        communityId: '',
      );
      await notifier.loadUserCommunities();

      final state = container.read(gearSharingProvider);
      expect(state.userCommunities, hasLength(1));
      verify(mockCommunityRepository.listCommunityGear('c1')).called(1);
    });
  });
}
