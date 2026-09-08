import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/gear_repository.dart'
    hide GearBookingState;
import 'package:ripls/presentation/viewmodels/gear_booking_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'gear_booking_view_model_test.mocks.dart';

@GenerateMocks([GearRepository])
void main() {
  group('GearBookingNotifier', () {
    const gearId = 'gear-1';
    const communityId = 'community-1';

    late MockGearRepository mockRepo;
    late ProviderContainer container;
    // Holds an active subscription so the autoDispose family provider stays
    // alive across the async calls under test.
    late ProviderSubscription<GearBookingState> sub;

    setUp(() {
      mockRepo = MockGearRepository();
      container = ProviderContainer(
        overrides: [
          gearRepositoryProvider.overrideWithValue(mockRepo),
        ],
      );
      sub = container.listen(gearBookingProvider(gearId), (_, _) {});
    });

    tearDown(() {
      sub.close();
      container.dispose();
    });

    GearBookingNotifier notifier() =>
        container.read(gearBookingProvider(gearId).notifier);
    GearBookingState read() => container.read(gearBookingProvider(gearId));

    // Stubs the post-mutation refresh (invalidate + invalidateStats + reload)
    // so success-path mutations can complete.
    void stubRefresh({List<GearBooking> reloaded = const []}) {
      when(mockRepo.invalidate(any, communityId: anyNamed('communityId')))
          .thenAnswer((_) async {});
      when(mockRepo.invalidateStats(any, communityId: anyNamed('communityId')))
          .thenAnswer((_) async {});
      when(mockRepo.listBookings(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => reloaded);
    }

    test('initial state is empty and idle', () {
      final state = read();
      expect(state.bookings, isEmpty);
      expect(state.isLoading, isFalse);
      expect(state.isMutating, isFalse);
      expect(state.error, isNull);
    });

    test('load before initialize is a no-op (no community context)', () async {
      await notifier().load();
      verifyNever(mockRepo.listBookings(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
      ));
      expect(read().isLoading, isFalse);
    });

    test('initialize loads bookings and clears loading/error', () async {
      final bookings = [GearBooking(), GearBooking()];
      when(mockRepo.listBookings(
        gearId: gearId,
        communityId: communityId,
      )).thenAnswer((_) async => bookings);

      await notifier().initialize(communityId: communityId);

      final state = read();
      expect(state.bookings, hasLength(2));
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
    });

    test('load failure classifies the error and clears loading', () async {
      when(mockRepo.listBookings(
        gearId: gearId,
        communityId: communityId,
      )).thenThrow(Exception('boom'));

      await notifier().initialize(communityId: communityId);

      final state = read();
      expect(state.isLoading, isFalse);
      // The raw exception is classified to a typed UserError, not stringified.
      expect(state.error, isNotNull);
    });

    test('claim returns the booking and clears mutating on success', () async {
      when(mockRepo.listBookings(
        gearId: gearId,
        communityId: communityId,
      )).thenAnswer((_) async => []);
      await notifier().initialize(communityId: communityId);

      final claimed = GearBooking();
      when(mockRepo.claimDays(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
        startDateUnixSec: anyNamed('startDateUnixSec'),
        endDateUnixSec: anyNamed('endDateUnixSec'),
        recipientId: anyNamed('recipientId'),
        pending: anyNamed('pending'),
      )).thenAnswer((_) async => claimed);
      stubRefresh();

      final result = await notifier().claim(
        startDateUnixSec: 1000,
        endDateUnixSec: 2000,
      );

      expect(result, same(claimed));
      final state = read();
      expect(state.isMutating, isFalse);
      expect(state.error, isNull);
    });

    test('claim failure returns null and sets a classified error', () async {
      when(mockRepo.listBookings(
        gearId: gearId,
        communityId: communityId,
      )).thenAnswer((_) async => []);
      await notifier().initialize(communityId: communityId);

      when(mockRepo.claimDays(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
        startDateUnixSec: anyNamed('startDateUnixSec'),
        endDateUnixSec: anyNamed('endDateUnixSec'),
        recipientId: anyNamed('recipientId'),
        pending: anyNamed('pending'),
      )).thenThrow(Exception('no slots'));

      final result = await notifier().claim(
        startDateUnixSec: 1000,
        endDateUnixSec: 2000,
      );

      expect(result, isNull);
      final state = read();
      expect(state.isMutating, isFalse);
      expect(state.error, isNotNull);
    });

    test('claim before initialize returns null without calling the repo',
        () async {
      final result = await notifier().claim(
        startDateUnixSec: 1000,
        endDateUnixSec: 2000,
      );
      expect(result, isNull);
      verifyNever(mockRepo.claimDays(
        gearId: anyNamed('gearId'),
        communityId: anyNamed('communityId'),
        startDateUnixSec: anyNamed('startDateUnixSec'),
        endDateUnixSec: anyNamed('endDateUnixSec'),
        recipientId: anyNamed('recipientId'),
        pending: anyNamed('pending'),
      ));
    });

    test('accept returns true on success', () async {
      stubRefresh();
      when(mockRepo.acceptBooking(
        bookingId: anyNamed('bookingId'),
        acceptToken: anyNamed('acceptToken'),
      )).thenAnswer((_) async => GearBooking());

      final ok = await notifier()
          .accept(bookingId: 'b1', acceptToken: 'token');

      expect(ok, isTrue);
      expect(read().error, isNull);
    });

    test('accept failure returns false and sets a classified error', () async {
      when(mockRepo.acceptBooking(
        bookingId: anyNamed('bookingId'),
        acceptToken: anyNamed('acceptToken'),
      )).thenThrow(Exception('bad token'));

      final ok = await notifier()
          .accept(bookingId: 'b1', acceptToken: 'token');

      expect(ok, isFalse);
      expect(read().error, isNotNull);
      expect(read().isMutating, isFalse);
    });

    test('release returns true on success', () async {
      stubRefresh();
      when(mockRepo.releaseBooking(any)).thenAnswer((_) async {});

      final ok = await notifier().release('b1');

      expect(ok, isTrue);
      expect(read().error, isNull);
    });

    test('release failure returns false and sets a classified error', () async {
      when(mockRepo.releaseBooking(any)).thenThrow(Exception('cannot release'));

      final ok = await notifier().release('b1');

      expect(ok, isFalse);
      expect(read().error, isNotNull);
    });

    test('updateHandoff returns true on success', () async {
      stubRefresh();
      when(mockRepo.updateBookingHandoff(
        bookingId: anyNamed('bookingId'),
        pickupLocationId: anyNamed('pickupLocationId'),
        pickupTimeUnixSec: anyNamed('pickupTimeUnixSec'),
        dropoffLocationId: anyNamed('dropoffLocationId'),
        dropoffTimeUnixSec: anyNamed('dropoffTimeUnixSec'),
      )).thenAnswer((_) async => GearBooking());

      final ok = await notifier().updateHandoff(bookingId: 'b1');

      expect(ok, isTrue);
      expect(read().error, isNull);
    });

    test('updateHandoff failure returns false and sets a classified error',
        () async {
      when(mockRepo.updateBookingHandoff(
        bookingId: anyNamed('bookingId'),
        pickupLocationId: anyNamed('pickupLocationId'),
        pickupTimeUnixSec: anyNamed('pickupTimeUnixSec'),
        dropoffLocationId: anyNamed('dropoffLocationId'),
        dropoffTimeUnixSec: anyNamed('dropoffTimeUnixSec'),
      )).thenThrow(Exception('handoff failed'));

      final ok = await notifier().updateHandoff(bookingId: 'b1');

      expect(ok, isFalse);
      expect(read().error, isNotNull);
    });
  });
}
