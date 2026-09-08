import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/giveaway_giver_state.dart';
import 'package:ripls/presentation/viewmodels/giveaway_giver_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'giveaway_giver_view_model_test.mocks.dart';

@GenerateMocks([TransferRepository])
void main() {
  late MockTransferRepository mockRepo;
  late ProviderContainer container;

  final owner = User(id: 'owner1', name: 'Owner');
  final recipient1 = User(id: 'r1', name: 'Alice');
  final recipient2 = User(id: 'r2', name: 'Bob');

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
      recipient: recipient ?? recipient1,
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
    test('loads pending requests and sets selectRecipient phase', () async {
      final t1 = createTransfer(id: 't1', recipient: recipient1);
      final t2 = createTransfer(id: 't2', recipient: recipient2);
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenAnswer((_) async => [t1, t2]);

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1');

      final state = container.read(giveawayGiverProvider);
      expect(state.currentPhase, GiveawayGiverPhase.selectRecipient);
      expect(state.pendingRequests, hasLength(2));
      expect(state.isLoading, isFalse);
    });

    test('sets recipientSelected phase when transfer is RECIPIENT_SELECTED', () async {
      final selected = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenAnswer((_) async => [selected]);
      when(mockRepo.getTransfer(selected.id))
          .thenAnswer((_) async => selected);

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1', transferId: selected.id);

      final state = container.read(giveawayGiverProvider);
      expect(state.currentPhase, GiveawayGiverPhase.recipientSelected);
      expect(state.activeTransfer, isNotNull);
    });

    test('sets complete phase when transfer is COMPLETED', () async {
      final completed = createTransfer(
        state: TransferState.TRANSFER_STATE_COMPLETED,
      );
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenAnswer((_) async => [completed]);
      when(mockRepo.getTransfer(completed.id))
          .thenAnswer((_) async => completed);

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1', transferId: completed.id);

      final state = container.read(giveawayGiverProvider);
      expect(state.currentPhase, GiveawayGiverPhase.complete);
    });

    test('sets error on failure', () async {
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenThrow(Exception('Network error'));

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1');

      final state = container.read(giveawayGiverProvider);
      expect(state.isLoading, isFalse);
      expect(state.error, isNotNull);
    });

    test('empty pending requests when no transfers exist', () async {
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenAnswer((_) async => []);

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1');

      final state = container.read(giveawayGiverProvider);
      expect(state.currentPhase, GiveawayGiverPhase.selectRecipient);
      expect(state.pendingRequests, isEmpty);
      expect(state.hasPendingRequests, isFalse);
    });
  });

  group('selectRecipient', () {
    test('transitions to recipientSelected phase', () async {
      final t1 = createTransfer(id: 't1');
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenAnswer((_) async => [t1]);

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1');

      final selected = createTransfer(
        id: 't1',
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.selectRecipient(
        transferId: 't1',
        recipientId: 'r1',
      )).thenAnswer(
        (_) async => (conversationId: 'conv1', communityEventId: 'event-id'),
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => selected);

      await notifier.selectRecipient('t1', 'r1');

      final state = container.read(giveawayGiverProvider);
      expect(state.currentPhase, GiveawayGiverPhase.recipientSelected);
      expect(state.pendingRequests, isEmpty);
      expect(state.selectedRecipientId, isNull);
    });

    test('sets error on failure', () async {
      final t1 = createTransfer(id: 't1');
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenAnswer((_) async => [t1]);

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1');

      when(mockRepo.selectRecipient(
        transferId: 't1',
        recipientId: 'r1',
      )).thenThrow(Exception('Server error'));

      await notifier.selectRecipient('t1', 'r1');

      final state = container.read(giveawayGiverProvider);
      expect(state.error, isNotNull);
    });
  });

  group('setSelectedRecipient', () {
    test('updates selectedRecipientId', () {
      final notifier = container.read(giveawayGiverProvider.notifier);
      notifier.setSelectedRecipient('r1');

      final state = container.read(giveawayGiverProvider);
      expect(state.selectedRecipientId, 'r1');
      expect(state.hasSelectedRecipient, isTrue);
    });
  });

  group('completeGiveaway', () {
    test('transitions to complete phase', () async {
      final active = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenAnswer((_) async => [active]);
      when(mockRepo.getTransfer(active.id))
          .thenAnswer((_) async => active);

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1', transferId: active.id);

      final completed = createTransfer(
        state: TransferState.TRANSFER_STATE_COMPLETED,
      );
      when(mockRepo.completeTransfer(transferId: active.id))
          .thenAnswer((_) async => CompleteTransferResponse());
      when(mockRepo.getTransfer(active.id))
          .thenAnswer((_) async => completed);

      await notifier.completeGiveaway(active.id);

      final state = container.read(giveawayGiverProvider);
      expect(state.currentPhase, GiveawayGiverPhase.complete);
    });
  });

  group('cancelTransfer', () {
    test('clears active transfer', () async {
      final active = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.listMyTransfers(
        transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
      )).thenAnswer((_) async => [active]);
      when(mockRepo.getTransfer(active.id))
          .thenAnswer((_) async => active);

      final notifier = container.read(giveawayGiverProvider.notifier);
      await notifier.initialize(gearId: 'gear1', transferId: active.id);

      when(mockRepo.cancelTransfer(transferId: active.id))
          .thenAnswer((_) async => 'event-id');

      await notifier.cancelTransfer(active.id);

      final state = container.read(giveawayGiverProvider);
      expect(state.activeTransfer, isNull);
      expect(state.isLoading, isFalse);
    });
  });
}
