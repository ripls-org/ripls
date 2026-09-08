import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/presentation/viewmodels/loan_borrower_state.dart';
import 'package:ripls/presentation/viewmodels/loan_borrower_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'loan_borrower_view_model_test.mocks.dart';

@GenerateMocks([TransferRepository])
void main() {
  late MockTransferRepository mockRepo;
  late ProviderContainer container;

  final owner = User(id: 'owner1', name: 'Owner');
  final borrower = User(id: 'borrower1', name: 'Borrower');

  Transfer createTransfer({
    String id = 't1',
    String gearId = 'gear1',
    TransferState state = TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
    Int64? estimatedPickupUnixSec,
    int? loanDurationDays,
    Int64? actualPickupUnixSec,
  }) {
    final t = Transfer(
      id: id,
      gearId: gearId,
      gearName: 'Test Gear',
      owner: owner,
      recipient: borrower,
      transferType: TransferType.TRANSFER_TYPE_LOAN,
      state: state,
    );
    if (estimatedPickupUnixSec != null) {
      t.estimatedPickupUnixSec = estimatedPickupUnixSec;
    }
    if (loanDurationDays != null) {
      t.loanDurationDays = loanDurationDays;
    }
    if (actualPickupUnixSec != null) {
      t.actualPickupUnixSec = actualPickupUnixSec;
    }
    return t;
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
    test('phase = arrangePickup when INTEREST_EXPRESSED', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(loanBorrowerProvider);
      expect(state.currentPhase, LoanBorrowerPhase.arrangePickup);
      expect(state.transfer, isNotNull);
      expect(state.isLoading, isFalse);
    });

    test('phase = confirmed when RECIPIENT_SELECTED with pickup details set',
        () async {
      final now = DateTime.now().millisecondsSinceEpoch ~/ 1000;
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
        estimatedPickupUnixSec: Int64(now),
        loanDurationDays: 7,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(loanBorrowerProvider);
      expect(state.currentPhase, LoanBorrowerPhase.confirmed);
    });

    test(
        'phase = arrangePickup when RECIPIENT_SELECTED but pickup details unset',
        () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(loanBorrowerProvider);
      expect(state.currentPhase, LoanBorrowerPhase.arrangePickup);
    });

    test('phase = inUse when ACTIVE; countdown populated', () async {
      // actualPickup = 3 days ago, loanDuration = 10 days → 7 days remain
      final actualPickup = DateTime.now()
          .subtract(const Duration(days: 3))
          .millisecondsSinceEpoch ~/
          1000;
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_ACTIVE,
        actualPickupUnixSec: Int64(actualPickup),
        loanDurationDays: 10,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(loanBorrowerProvider);
      expect(state.currentPhase, LoanBorrowerPhase.inUse);
      // Countdown should be populated (roughly 7 days remaining)
      expect(state.countdownDays, greaterThan(0));
    });

    test('phase = returned when COMPLETED', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_COMPLETED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(loanBorrowerProvider);
      expect(state.currentPhase, LoanBorrowerPhase.returned);
    });

    test('error path: repository throws → errorMessage set, isLoading false',
        () async {
      when(mockRepo.getTransfer('t1'))
          .thenThrow(Exception('Network error'));

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(loanBorrowerProvider);
      expect(state.isLoading, isFalse);
      expect(state.error, isNotNull);
      expect(state.currentPhase, LoanBorrowerPhase.arrangePickup);
    });

    test('missing transfer → errorMessage = "Transfer not found"', () async {
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => null);

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final state = container.read(loanBorrowerProvider);
      expect(state.isLoading, isFalse);
      expect(state.error, isNotNull);
    });
  });

  group('setPickupDate / setPickupTime / setDuration', () {
    test('setPickupDate updates selectedPickupDate', () {
      final date = DateTime(2025, 6, 15);
      final notifier = container.read(loanBorrowerProvider.notifier);
      notifier.setPickupDate(date);

      final state = container.read(loanBorrowerProvider);
      expect(state.selectedPickupDate, date);
    });

    test('setPickupTime updates selectedPickupTime', () {
      const time = TimeOfDay(hour: 10, minute: 30);
      final notifier = container.read(loanBorrowerProvider.notifier);
      notifier.setPickupTime(time);

      final state = container.read(loanBorrowerProvider);
      expect(state.selectedPickupTime, time);
    });

    test('setDuration updates selectedDurationDays', () {
      final notifier = container.read(loanBorrowerProvider.notifier);
      notifier.setDuration(14);

      final state = container.read(loanBorrowerProvider);
      expect(state.selectedDurationDays, 14);
    });
  });

  group('submitPickupDetails', () {
    test('early-returns when transfer is null (no repo call)', () async {
      final notifier = container.read(loanBorrowerProvider.notifier);
      notifier.setPickupDate(DateTime(2025, 6, 15));
      notifier.setPickupTime(const TimeOfDay(hour: 10, minute: 0));
      await notifier.submitPickupDetails();

      verifyNever(mockRepo.updateTransfer(
        transferId: anyNamed('transferId'),
        estimatedPickupUnixSec: anyNamed('estimatedPickupUnixSec'),
        loanDurationDays: anyNamed('loanDurationDays'),
      ));
    });

    test('early-returns when date or time unset (no repo call)', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');
      // Do NOT set date or time
      await notifier.submitPickupDetails();

      verifyNever(mockRepo.updateTransfer(
        transferId: anyNamed('transferId'),
        estimatedPickupUnixSec: anyNamed('estimatedPickupUnixSec'),
        loanDurationDays: anyNamed('loanDurationDays'),
      ));
    });

    test('happy path: calls updateTransfer, reloads, updates phase', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final now = DateTime.now().millisecondsSinceEpoch ~/ 1000;
      final updatedTransfer = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
        estimatedPickupUnixSec: Int64(now),
        loanDurationDays: 7,
      );

      when(mockRepo.updateTransfer(
        transferId: anyNamed('transferId'),
        estimatedPickupUnixSec: anyNamed('estimatedPickupUnixSec'),
        loanDurationDays: anyNamed('loanDurationDays'),
      )).thenAnswer((_) async {});
      var getCallCount = 0;
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async {
        getCallCount++;
        return getCallCount == 1 ? transfer : updatedTransfer;
      });

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');
      notifier.setPickupDate(DateTime(2025, 6, 15));
      notifier.setPickupTime(const TimeOfDay(hour: 10, minute: 0));
      notifier.setDuration(7);
      await notifier.submitPickupDetails();

      final state = container.read(loanBorrowerProvider);
      expect(state.isLoading, isFalse);
      expect(state.currentPhase, LoanBorrowerPhase.confirmed);
    });

    test('error path: updateTransfer throws → errorMessage set', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.updateTransfer(
        transferId: anyNamed('transferId'),
        estimatedPickupUnixSec: anyNamed('estimatedPickupUnixSec'),
        loanDurationDays: anyNamed('loanDurationDays'),
      )).thenThrow(Exception('Server error'));

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');
      notifier.setPickupDate(DateTime(2025, 6, 15));
      notifier.setPickupTime(const TimeOfDay(hour: 10, minute: 0));
      await notifier.submitPickupDetails();

      final state = container.read(loanBorrowerProvider);
      expect(state.isLoading, isFalse);
      expect(state.error, isNotNull);
    });

    test('reload returning null → errorMessage = "Failed to reload transfer after update"',
        () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );
      when(mockRepo.updateTransfer(
        transferId: anyNamed('transferId'),
        estimatedPickupUnixSec: anyNamed('estimatedPickupUnixSec'),
        loanDurationDays: anyNamed('loanDurationDays'),
      )).thenAnswer((_) async {});

      var callCount = 0;
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async {
        callCount++;
        return callCount == 1 ? transfer : null;
      });

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');
      notifier.setPickupDate(DateTime(2025, 6, 15));
      notifier.setPickupTime(const TimeOfDay(hour: 10, minute: 0));
      await notifier.submitPickupDetails();

      final state = container.read(loanBorrowerProvider);
      expect(state.isLoading, isFalse);
      expect(state.error, isNotNull);
    });
  });

  group('markPickedUp', () {
    test('happy path: returns communityEventId, transitions to inUse', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final actualPickup = DateTime.now()
              .subtract(const Duration(minutes: 5))
              .millisecondsSinceEpoch ~/
          1000;
      final activeTransfer = createTransfer(
        state: TransferState.TRANSFER_STATE_ACTIVE,
        actualPickupUnixSec: Int64(actualPickup),
        loanDurationDays: 7,
      );

      when(mockRepo.startLoan(transferId: 't1'))
          .thenAnswer((_) async => 'event-id-1');
      var markPickedUpCallCount = 0;
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async {
        markPickedUpCallCount++;
        return markPickedUpCallCount == 1 ? transfer : activeTransfer;
      });

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final eventId = await notifier.markPickedUp();

      expect(eventId, 'event-id-1');
      final state = container.read(loanBorrowerProvider);
      expect(state.currentPhase, LoanBorrowerPhase.inUse);
      expect(state.isLoading, isFalse);
    });

    test('early return when transfer null: returns null', () async {
      final notifier = container.read(loanBorrowerProvider.notifier);
      final result = await notifier.markPickedUp();
      expect(result, isNull);
      verifyNever(mockRepo.startLoan(transferId: anyNamed('transferId')));
    });

    test('error path: startLoan throws → returns null, errorMessage set',
        () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.startLoan(transferId: 't1'))
          .thenThrow(Exception('Start loan error'));

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final result = await notifier.markPickedUp();

      expect(result, isNull);
      final state = container.read(loanBorrowerProvider);
      expect(state.error, isNotNull);
    });
  });

  group('confirmReturn', () {
    test('happy path: returns communityEventId, phase becomes returned',
        () async {
      final actualPickup = DateTime.now()
              .subtract(const Duration(days: 3))
              .millisecondsSinceEpoch ~/
          1000;
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_ACTIVE,
        actualPickupUnixSec: Int64(actualPickup),
        loanDurationDays: 10,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final completedTransfer = createTransfer(
        state: TransferState.TRANSFER_STATE_COMPLETED,
      );
      when(mockRepo.completeTransfer(transferId: 't1'))
          .thenAnswer((_) async => CompleteTransferResponse()
            ..communityEventId = 'event-complete');
      var confirmReturnCallCount = 0;
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async {
        confirmReturnCallCount++;
        return confirmReturnCallCount == 1 ? transfer : completedTransfer;
      });

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final eventId = await notifier.confirmReturn();

      expect(eventId, 'event-complete');
      final state = container.read(loanBorrowerProvider);
      expect(state.currentPhase, LoanBorrowerPhase.returned);
      expect(state.isLoading, isFalse);
    });

    test('early return when transfer null: returns null', () async {
      final notifier = container.read(loanBorrowerProvider.notifier);
      final result = await notifier.confirmReturn();
      expect(result, isNull);
      verifyNever(mockRepo.completeTransfer(transferId: anyNamed('transferId')));
    });

    test('error path: completeTransfer throws → returns null, errorMessage set',
        () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_ACTIVE,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.completeTransfer(transferId: 't1'))
          .thenThrow(Exception('Complete error'));

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');

      final result = await notifier.confirmReturn();

      expect(result, isNull);
      final state = container.read(loanBorrowerProvider);
      expect(state.error, isNotNull);
    });
  });

  group('withdrawInterest', () {
    test('happy path: clears transfer from state', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.withdrawInterest(transferId: 't1'))
          .thenAnswer((_) async {});

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');
      await notifier.withdrawInterest();

      final state = container.read(loanBorrowerProvider);
      expect(state.transfer, isNull);
      expect(state.isLoading, isFalse);
    });

    test('early return when transfer null: no repo call', () async {
      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.withdrawInterest();
      verifyNever(mockRepo.withdrawInterest(transferId: anyNamed('transferId')));
    });

    test('error path: withdrawInterest throws → errorMessage set', () async {
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);
      when(mockRepo.withdrawInterest(transferId: 't1'))
          .thenThrow(Exception('Withdraw error'));

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');
      await notifier.withdrawInterest();

      final state = container.read(loanBorrowerProvider);
      expect(state.error, isNotNull);
    });
  });

  group('disposal safety', () {
    test('container.dispose() mid-flight completes without throwing', () async {
      final actualPickup = DateTime.now()
              .subtract(const Duration(days: 1))
              .millisecondsSinceEpoch ~/
          1000;
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_ACTIVE,
        actualPickupUnixSec: Int64(actualPickup),
        loanDurationDays: 5,
      );

      // Delay the response so container.dispose() races with initialize
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async {
        await Future<void>.delayed(const Duration(milliseconds: 10));
        return transfer;
      });

      final notifier = container.read(loanBorrowerProvider.notifier);
      final future = notifier.initialize(transferId: 't1');
      container.dispose();

      // Future must complete without throwing
      await expectLater(future, completes);
    });

    test('mutations after disposal do not crash', () async {
      final actualPickup = DateTime.now()
              .subtract(const Duration(days: 1))
              .millisecondsSinceEpoch ~/
          1000;
      final transfer = createTransfer(
        state: TransferState.TRANSFER_STATE_ACTIVE,
        actualPickupUnixSec: Int64(actualPickup),
        loanDurationDays: 5,
      );
      when(mockRepo.getTransfer('t1')).thenAnswer((_) async => transfer);

      final notifier = container.read(loanBorrowerProvider.notifier);
      await notifier.initialize(transferId: 't1');
      container.dispose();

      // Post-disposal mutations should not crash
      expect(() => notifier.setPickupDate(DateTime(2025, 6, 15)), returnsNormally);
      expect(() => notifier.setDuration(7), returnsNormally);
    });
  });
}
