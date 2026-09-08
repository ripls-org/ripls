import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show DayForecast, HourForecast;
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/experience/widgets/experience_time_panel.dart';
import 'package:ripls/presentation/viewmodels/experience_day_weather_provider.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';

import '../../../../helpers/l10n_helpers.dart';

const _id = 'exp-1';

/// Returns fixed [TimeModalData] from build(), bypassing the network load.
class _FakeTimeNotifier extends TimeModalNotifier {
  _FakeTimeNotifier(this._data) : super(_id);
  final TimeModalData _data;

  @override
  Future<TimeModalData> build() async => _data;
}

ProviderScope _host(TimeModalData data) {
  return ProviderScope(
    overrides: [
      timeModalProvider(_id).overrideWith(() => _FakeTimeNotifier(data)),
      // Stub the day weather so the strip never sits on a spinner (which would
      // hang pumpAndSettle); the network is never hit in tests.
      experienceDayWeatherProvider.overrideWith(
          (ref, key) async => const DayWeatherResult(hours: <HourForecast>[])),
      experienceCalendarWeatherProvider
          .overrideWith((ref, id) async => const <DayForecast>[]),
    ],
    child: localizedApp(
      ExperienceTimePanel(
        experienceId: _id,
        accentColor: const Color(0xFF7A9B8C),
        onMarkCompleted: () {},
        onCloseEvent: () {},
      ),
    ),
  );
}

/// The panel always renders a full month calendar, so the default 800×600 test
/// surface overflows. Use a tall, phone-width surface so the grid + dock lay out
/// like a real device.
void _phoneSurface(WidgetTester tester) {
  tester.view.physicalSize = const Size(390, 2000);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}

TimeProposal _proposal(String id, DateTime at, {User? by}) {
  final p = TimeProposal()
    ..id = id
    ..pollId = 'poll-1'
    ..time = (ExperienceTime()
      ..specific = (SpecificTime()
        ..unixTimestampSec = Int64(at.millisecondsSinceEpoch ~/ 1000)
        ..durationMinutes = 120));
  if (by != null) p.proposedBy = by;
  return p;
}

void main() {
  group('ExperienceTimePanel (TBD state)', () {
    testWidgets('owner sees the set / ask-the-group controls', (tester) async {
      _phoneSurface(tester);
      await tester.pumpWidget(_host(const TimeModalData(isOrganizer: true)));
      await tester.pumpAndSettle();

      expect(find.text('When'), findsOneWidget);
      expect(find.text('Set the time'), findsOneWidget);
      expect(find.text('Ask the group'), findsOneWidget);
    });

    testWidgets('non-owner sees the read-only host note', (tester) async {
      _phoneSurface(tester);
      await tester.pumpWidget(_host(const TimeModalData(isOrganizer: false)));
      await tester.pumpAndSettle();

      expect(find.text("The host hasn't picked a time yet."), findsOneWidget);
      expect(find.text('Set the time'), findsNothing);
    });
  });

  group('ExperienceTimePanel (active poll)', () {
    testWidgets('owner sees options, add-another, and opens set-final', (
      tester,
    ) async {
      // Tall surface so the lazy ListView builds the bottom action button.
      tester.view.physicalSize = const Size(1000, 2400);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final p1 = _proposal('p1', DateTime(2026, 6, 9, 8));
      final p2 = _proposal('p2', DateTime(2026, 6, 13, 9));
      await tester.pumpWidget(
        _host(TimeModalData(
          isOrganizer: true,
          timePollActive: true,
          currentPollId: 'poll-1',
          proposals: [p1, p2],
        )),
      );
      await tester.pumpAndSettle();

      expect(find.text('Add another time'), findsOneWidget);

      await tester.tap(find.text('SET THE FINAL TIME').last);
      await tester.pumpAndSettle();

      expect(find.text('Lock in & notify group'), findsOneWidget);
      expect(find.text('HOW EVERYONE PICKED'), findsOneWidget);
    });
  });

  group('ExperienceTimePanel (overflow menu)', () {
    testWidgets('owner overflow is shown with no poll and includes the '
        'experience-settings actions', (tester) async {
      _phoneSurface(tester);
      await tester.pumpWidget(_host(const TimeModalData(isOrganizer: true)));
      await tester.pumpAndSettle();

      // The ... overflow is present even though no poll is active.
      final manage = find.bySemanticsLabel('Manage poll');
      expect(manage, findsOneWidget);
      await tester.tap(manage);
      await tester.pumpAndSettle();

      // The menu carries the experience-settings actions from the content view.
      expect(find.text('Wrap up'), findsOneWidget);
      expect(find.text('Close Event'), findsOneWidget);
    });

    testWidgets('event-settings actions are hidden while a poll is open', (
      tester,
    ) async {
      _phoneSurface(tester);
      final p1 = _proposal('p1', DateTime(2026, 6, 9, 8));
      await tester.pumpWidget(
        _host(TimeModalData(
          isOrganizer: true,
          timePollActive: true,
          currentPollId: 'poll-1',
          proposals: [p1],
        )),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.bySemanticsLabel('Manage poll'));
      await tester.pumpAndSettle();

      // Poll must complete first — no mark-completed / close-event here.
      expect(find.text('Wrap up'), findsNothing);
      expect(find.text('Close Event'), findsNothing);
      // The poll-management actions are present instead.
      expect(find.text('Cancel poll'), findsOneWidget);
    });

    testWidgets('non-owner sees no overflow', (tester) async {
      _phoneSurface(tester);
      await tester.pumpWidget(_host(const TimeModalData(isOrganizer: false)));
      await tester.pumpAndSettle();
      expect(find.bySemanticsLabel('Manage poll'), findsNothing);
    });
  });

  group('ExperienceTimePanel (inline picker)', () {
    testWidgets('owner picks a time on the panel, not a modal route', (
      tester,
    ) async {
      tester.view.physicalSize = const Size(1000, 2400);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(_host(const TimeModalData(isOrganizer: true)));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Set the time'));
      await tester.pumpAndSettle();

      // The picker is now inline on the panel (its Save footer is present) and
      // the TBD controls are gone — no pushed sheet.
      expect(find.text('Save'), findsOneWidget);
      expect(find.text('Set the time'), findsNothing);
    });
  });

  group('ExperienceTimePanel (confirmed)', () {
    testWidgets('renders the agenda dock: export icon + host controls', (
      tester,
    ) async {
      _phoneSurface(tester);
      final t = ExperienceTime()
        ..specific = (SpecificTime()
          ..unixTimestampSec =
              Int64(DateTime(2026, 6, 24, 14).millisecondsSinceEpoch ~/ 1000)
          ..durationMinutes = 60);
      await tester.pumpWidget(
        _host(TimeModalData(isOrganizer: true, eventTime: t)),
      );
      await tester.pumpAndSettle();

      // Export is now an icon with a caption (not a full-width button).
      expect(find.text('Export'), findsOneWidget);
      // The three host controls replace the old two-up button row.
      expect(find.text('Mark done'), findsOneWidget);
      expect(find.text('Change time'), findsOneWidget);
      expect(find.text('Ask the group'), findsOneWidget);
    });
  });

  group('ExperienceTimePanel (single time, n=1)', () {
    testWidgets('non-owner sees works / doesn\'t-work, not poll framing', (
      tester,
    ) async {
      _phoneSurface(tester);
      final p1 = _proposal('p1', DateTime(2026, 6, 13, 9),
          by: User(id: 'maya', name: 'Maya'));
      await tester.pumpWidget(
        _host(TimeModalData(
          isOrganizer: false,
          currentUserId: 'viewer',
          timePollActive: true,
          currentPollId: 'poll-1',
          proposals: [p1],
        )),
      );
      await tester.pumpAndSettle();

      expect(find.text('Works for me'), findsOneWidget);
      expect(find.text("Doesn't work"), findsOneWidget);
    });
  });
}
