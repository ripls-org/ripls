import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:intl/intl.dart';
import 'package:ripls/data/gen/ripls/api/gear_booking.pb.dart'
    show GearBooking, GearBookingState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_whos_using_calendar.dart';
import 'package:ripls/presentation/viewmodels/gear_booking_view_model.dart'
    as vm;

import '../../../../helpers/l10n_helpers.dart';

/// A booking notifier stub that returns canned bookings and never touches the
/// repository (initialize is a no-op).
class _FakeBookingNotifier extends vm.GearBookingNotifier {
  _FakeBookingNotifier(this._state) : super('gear-1');
  final vm.GearBookingState _state;
  @override
  vm.GearBookingState build() => _state;
  @override
  Future<void> initialize({required String communityId}) async {}
}

int _todayMidnightUnix() {
  final now = DateTime.now();
  return DateTime(now.year, now.month, now.day).millisecondsSinceEpoch ~/ 1000;
}

Widget _harness(vm.GearBookingState state) {
  return ProviderScope(
    overrides: [
      vm.gearBookingProvider('gear-1')
          .overrideWith(() => _FakeBookingNotifier(state)),
    ],
    child: localizedApp(
      GearWhosUsingCalendar(
        gearId: 'gear-1',
        communityId: 'community-1',
        heroMediaId: '',
        onOpenComments: () {},
      ),
    ),
  );
}

void main() {
  group('GearWhosUsingCalendar', () {
    testWidgets('renders the current month header and grid', (tester) async {
      await tester.pumpWidget(_harness(const vm.GearBookingState()));
      await tester.pump();

      final monthLabel = DateFormat('MMMM yyyy').format(DateTime.now());
      expect(find.text(monthLabel), findsOneWidget);
      expect(find.text("Who's using it"), findsOneWidget);
    });

    testWidgets('open future day shows the friendly "Borrow it" dock',
        (tester) async {
      await tester.pumpWidget(_harness(const vm.GearBookingState()));
      await tester.pump();

      // Jump to next month so day 15 is always an open future day (no bookings).
      await tester.tap(find.byIcon(Icons.chevron_right));
      await tester.pump();
      await tester.tap(find.text('15').first);
      await tester.pump();

      expect(find.text('Borrow it'), findsOneWidget);
      expect(find.text('This is what sharing it is for.'), findsOneWidget);
      // The old small "Claim" chip is gone.
      expect(find.text('Claim'), findsNothing);
    });

    testWidgets('tapping a selected day again unselects it', (tester) async {
      await tester.pumpWidget(_harness(const vm.GearBookingState()));
      await tester.pump();

      await tester.tap(find.byIcon(Icons.chevron_right));
      await tester.pump();

      // Select an open future day → the borrow dock appears.
      await tester.tap(find.text('15').first);
      await tester.pump();
      expect(find.text('Borrow it'), findsOneWidget);

      // Tap the same day again → selection clears, back to the open tip.
      await tester.tap(find.text('15').first);
      await tester.pump();
      expect(find.text('Borrow it'), findsNothing);
      expect(find.text('Pick your days · the owner confirms.'), findsOneWidget);
    });

    testWidgets('plots a booking without crashing and shows the dock on tap',
        (tester) async {
      final unix = _todayMidnightUnix();
      final booking = GearBooking(
        id: 'b1',
        gearId: 'gear-1',
        borrower: User(id: 'u1', name: 'Maya Lopez'),
        startDateUnixSec: Int64(unix),
        endDateUnixSec: Int64(unix),
        state: GearBookingState.GEAR_BOOKING_STATE_RESERVED,
        isMine: false,
      );
      await tester
          .pumpWidget(_harness(vm.GearBookingState(bookings: [booking])));
      await tester.pump();

      // Tapping today's cell selects it and surfaces the booked-day dock with
      // the borrower's name.
      final todayNumber = '${DateTime.now().day}';
      await tester.tap(find.text(todayNumber).first);
      await tester.pump();
      expect(find.text('Maya Lopez'), findsWidgets);
    });

    testWidgets('renders the hand-off rows for your own booking with semantics '
        'enabled (no flex/semantics assertion)', (tester) async {
      // Building the semantics tree is what trips the parentData assertion when
      // a Tappable (Semantics) is a direct flex child, so enable it here.
      final handle = tester.ensureSemantics();
      final unix = _todayMidnightUnix();
      final booking = GearBooking(
        id: 'mine',
        gearId: 'gear-1',
        borrower: User(id: 'me', name: 'You There'),
        startDateUnixSec: Int64(unix),
        endDateUnixSec: Int64(unix),
        state: GearBookingState.GEAR_BOOKING_STATE_RESERVED,
        isMine: true,
      );
      await tester
          .pumpWidget(_harness(vm.GearBookingState(bookings: [booking])));
      await tester.pump();

      await tester.tap(find.text('${DateTime.now().day}').first);
      await tester.pump();

      // The PICKUP / DROP-OFF hand-off rows (with PLACE + TIME segments) render.
      expect(find.text('PLACE'), findsWidgets);
      expect(find.text('TIME'), findsWidgets);
      expect(tester.takeException(), isNull);
      handle.dispose();
    });
  });
}
