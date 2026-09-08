import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:intl/intl.dart';
import 'package:ripls/data/repositories/portfolio_repository.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/screens/communities/shared_calendar_screen.dart';
import 'package:ripls/services/providers.dart' show portfolioRepositoryProvider;

import '../../../helpers/l10n_helpers.dart';

class _FakePortfolioRepository extends Fake implements PortfolioRepository {
  _FakePortfolioRepository(this.viewToReturn);
  GetHomeViewResponse viewToReturn;

  @override
  Future<GetHomeViewResponse> getHomeView(String timezone) async =>
      viewToReturn;
}

void main() {
  const communityId = 'c1';

  Widget wrap(GetHomeViewResponse view) {
    return ProviderScope(
      overrides: [
        portfolioRepositoryProvider
            .overrideWithValue(_FakePortfolioRepository(view)),
        resolvedTimezoneProvider.overrideWith((ref) async => 'UTC'),
      ],
      child: localizedApp(
        const SharedCalendarScreen(communityIds: [communityId]),
      ),
    );
  }

  group('SharedCalendarScreen initial day', () {
    testWidgets(
        'opens on the soonest planned day instead of an empty today '
        '(regression: initState hardcoded today, ignoring entries)',
        (tester) async {
      // Tall viewport: the month grid + day detail overflow the default
      // 800×600 test surface.
      tester.view.physicalSize = const Size(1170, 2600);
      tester.view.devicePixelRatio = 3.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final today = DateTime.now();
      final future = DateTime(today.year, today.month, today.day)
          .add(const Duration(days: 44));

      await tester.pumpWidget(wrap(GetHomeViewResponse(
        calendar: [
          HomeUpNextEntry(
            id: 'e1',
            kind: HomeUpNextKind.HOME_UP_NEXT_KIND_EVENT,
            title: 'Neighborhood Craft Circle',
            timeUnixSec: Int64(future.millisecondsSinceEpoch ~/ 1000),
            communityId: communityId,
            communityIds: [communityId],
          ),
        ],
      )));
      await tester.pumpAndSettle();

      // The month header jumped to the event's month, not the current one.
      expect(
        find.text(DateFormat('MMMM yyyy').format(future)),
        findsOneWidget,
      );
      // And the event itself is visible without any interaction.
      expect(find.text('Neighborhood Craft Circle'), findsOneWidget);
      expect(find.text('Nothing planned yet'), findsNothing);
    });

    testWidgets('stays on today when today already has something',
        (tester) async {
      tester.view.physicalSize = const Size(1170, 2600);
      tester.view.devicePixelRatio = 3.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final today = DateTime.now();

      await tester.pumpWidget(wrap(GetHomeViewResponse(
        calendar: [
          HomeUpNextEntry(
            id: 'e1',
            kind: HomeUpNextKind.HOME_UP_NEXT_KIND_EVENT,
            title: 'Today Only Event',
            timeUnixSec: Int64(today.millisecondsSinceEpoch ~/ 1000),
            communityId: communityId,
            communityIds: [communityId],
          ),
        ],
      )));
      await tester.pumpAndSettle();

      expect(
        find.text(DateFormat('MMMM yyyy').format(today)),
        findsOneWidget,
      );
      expect(find.text('Today Only Event'), findsOneWidget);
    });

    testWidgets('falls back to today when nothing is planned',
        (tester) async {
      tester.view.physicalSize = const Size(1170, 2600);
      tester.view.devicePixelRatio = 3.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final today = DateTime.now();

      await tester.pumpWidget(wrap(GetHomeViewResponse()));
      await tester.pumpAndSettle();

      expect(
        find.text(DateFormat('MMMM yyyy').format(today)),
        findsOneWidget,
      );
      expect(find.text('Nothing planned yet'), findsOneWidget);
    });
  });
}
