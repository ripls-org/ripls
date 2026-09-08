import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/giveaway_receiver_state.dart';
import 'package:ripls/presentation/viewmodels/giveaway_receiver_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'giveaway_receiver_view_model_test.mocks.dart';

@GenerateMocks([TransferRepository])
void main() {
  late MockTransferRepository mockRepo;
  late ProviderContainer container;

  final owner = User(id: 'owner1', name: 'Owner');
  final me = User(id: 'me', name: 'Me');
  final otherUser = User(id: 'other', name: 'Other');

  Transfer createTransfer({
    String id = 't1',
    String gearId = 'gear1',
    TransferState state = TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
    User? recipient,
  }) {
    return Transfer(
      id: id,
      gearId: gearId,
      gearName: 'Test Gear',
      owner: owner,
      recipient: recipient ?? me,
      transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      state: state,
      conversationId: 'conv1',
    );
  }

  setUp(() {
    mockRepo = MockTransferRepository();
    container = ProviderContainer(
      overrides: [
        transferRepositoryProvider.overrideWithValue(mockRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockRepo);
  });

  group('initialize', () {
    test('loads transfer and sets waiting phase', () async {
      final transfer = createTransfer();
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.getGearTransfers(gearId: 'gear1'))
          .thenAnswer((_) async => [transfer]);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1', gearId: 'gear1');

      final state = container.read(giveawayReceiverProvider);
      expect(state.currentPhase, GiveawayReceiverPhase.waiting);
      expect(state.transfer, isNotNull);
      expect(state.isLoading, isFalse);
    });

    test('loads interested transfers list', () async {
      final myTransfer = createTransfer(id: 't1', recipient: me);
      final otherTransfer = createTransfer(id: 't2', recipient: otherUser);
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => myTransfer);
      when(mockRepo.getGearTransfers(gearId: 'gear1'))
          .thenAnswer((_) async => [myTransfer, otherTransfer]);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1', gearId: 'gear1');

      final state = container.read(giveawayReceiverProvider);
      expect(state.interestedTransfers, hasLength(2));
    });

    test('filters out cancelled transfers from interest list', () async {
      final myTransfer = createTransfer(id: 't1', recipient: me);
      final cancelled = createTransfer(
        id: 't2',
        recipient: otherUser,
        state: TransferState.TRANSFER_STATE_CANCELLED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => myTransfer);
      when(mockRepo.getGearTransfers(gearId: 'gear1'))
          .thenAnswer((_) async => [myTransfer, cancelled]);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1', gearId: 'gear1');

      final state = container.read(giveawayReceiverProvider);
      expect(state.interestedTransfers, hasLength(1));
    });

    test('sets recipientSelected phase when selected', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.getGearTransfers(gearId: 'gear1'))
          .thenAnswer((_) async => [transfer]);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1', gearId: 'gear1');

      final state = container.read(giveawayReceiverProvider);
      expect(state.currentPhase, GiveawayReceiverPhase.recipientSelected);
    });

    test('sets complete phase when completed', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_COMPLETED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.getGearTransfers(gearId: 'gear1'))
          .thenAnswer((_) async => []);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1', gearId: 'gear1');

      final state = container.read(giveawayReceiverProvider);
      expect(state.currentPhase, GiveawayReceiverPhase.complete);
    });

    test('sets error when transfer not found', () async {
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => null);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(giveawayReceiverProvider);
      expect(state.error, isNotNull);
      expect(state.isLoading, isFalse);
    });

    test('sets error on exception', () async {
      when(mockRepo.getTransfer('t1')).thenThrow(Exception('Network error'));

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(giveawayReceiverProvider);
      expect(state.error, isNotNull);
    });
  });

  group('withdrawInterest', () {
    test('clears transfer on success', () async {
      final transfer = createTransfer();
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.getGearTransfers(gearId: 'gear1'))
          .thenAnswer((_) async => [transfer]);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1', gearId: 'gear1');

      when(mockRepo.withdrawInterest(transferId: 't1'))
          .thenAnswer((_) async {});

      await notifier.withdrawInterest();

      final state = container.read(giveawayReceiverProvider);
      expect(state.transfer, isNull);
      expect(state.isLoading, isFalse);
    });

    test('sets error on failure', () async {
      final transfer = createTransfer();
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.getGearTransfers(gearId: 'gear1'))
          .thenAnswer((_) async => [transfer]);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1', gearId: 'gear1');

      when(mockRepo.withdrawInterest(transferId: 't1'))
          .thenThrow(Exception('Server error'));

      await notifier.withdrawInterest();

      final state = container.read(giveawayReceiverProvider);
      expect(state.error, isNotNull);
    });
  });

  group('markPickedUp', () {
    test('transitions to complete phase', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.getGearTransfers(gearId: 'gear1'))
          .thenAnswer((_) async => [transfer]);

      final notifier = container.read(giveawayReceiverProvider.notifier);
      await notifier.initialize(transferId: 't1', gearId: 'gear1');

      final completed = createTransfer(
        state: TransferState.TRANSFER_STATE_COMPLETED,
      );
      when(mockRepo.completeTransfer(transferId: 't1'))
          .thenAnswer((_) async => CompleteTransferResponse());
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => completed);

      await notifier.markPickedUp();

      final state = container.read(giveawayReceiverProvider);
      expect(state.currentPhase, GiveawayReceiverPhase.complete);
    });
  });
}
