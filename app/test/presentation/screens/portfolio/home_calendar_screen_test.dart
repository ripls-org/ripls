import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/portfolio/home_calendar_screen.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_day_cell.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_day_detail.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_month_grid.dart';
import 'package:ripls/presentation/widgets/home/calendar/month_grid_metrics.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_content_view.dart';
import 'package:ripls/services/providers.dart' show mediaUrlProvider;

/// Serves a fixed [HomeTabState] so the screen renders without a server.
class _FakeHomeTab extends HomeTabNotifier {
  _FakeHomeTab(this._state);

  final HomeTabState _state;

  @override
  HomeTabState build() => _state;
}


void main() {
  // Pixel 9 Pro: 448 x 952 logical, 48dp status bar.
  const screen = Size(1344, 2856);
  const dpr = 3.0;

  Future<void> pump(
    WidgetTester tester, {
    required bool embedded,
    Size physicalSize = screen,
    double pixelRatio = dpr,
  }) async {
    tester.view.physicalSize = physicalSize;
    tester.view.devicePixelRatio = pixelRatio;
    tester.view.padding =
        FakeViewPadding(top: 48 * pixelRatio, bottom: 24 * pixelRatio);
    addTearDown(tester.view.reset);

    final view = GetHomeViewResponse();

    await tester.pumpWidget(ProviderScope(
      overrides: [
        homeTabProvider.overrideWith(
          () => _FakeHomeTab(HomeTabState(view: view, isLoading: false)),
        ),
        mediaUrlProvider.overrideWith((ref, id) async => ''),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: HomeCalendarScreen(embedded: embedded),
      ),
    ));
    await tester.pump();
  }

  testWidgets('renders an empty calendar without overflowing', (tester) async {
    await pump(tester, embedded: true);
    expect(tester.takeException(), isNull);
  });

  testWidgets('an open day never renders a nudge card (#2936)',
      (tester) async {
    // The open-day nudge and its whole sizing problem (#2801 — the card had
    // to be bounded by the space under the month grid or it clipped) are
    // gone. An open day falls through to the static "plan an event" prompt,
    // which sizes to its own content.
    await pump(tester, embedded: true);
    expect(find.byType(NudgeContentView), findsNothing);
    expect(find.text('Nothing planned yet'), findsOneWidget);
  });

  group('landscape viewports (#2908)', () {
    // The app was authored against one viewport shape — a tall phone — and no
    // test had ever instantiated a landscape one. On the web that meant every
    // ordinary browser window: past roughly a 1.1:1 width-to-height ratio the
    // month grid's square cells sized themselves from width alone, grew taller
    // than the screen, and starved the day-detail panel to nothing while the
    // month's last row clipped away behind the dock — with no way to scroll to
    // either.
    const landscapes = <String, Size>{
      'laptop 16:9': Size(1440, 810),
      "the reporter's window": Size(1205, 1080),
      'wide and tall': Size(1440, 1400),
      'desktop 1080p': Size(1920, 1080),
      'short and narrow': Size(390, 620),
    };

    landscapes.forEach((name, size) {
      testWidgets('the month and the day detail both fit at $name',
          (tester) async {
        await pump(tester, embedded: true, physicalSize: size, pixelRatio: 1);
        expect(tester.takeException(), isNull);

        // #2912: at expanded widths CalendarPanes puts the day detail in a
        // side rail, which frees the grid from the stacked height share.
        final sideBySide = size.width >= Responsive.expandedBreakpoint;

        // The reporter's symptom: the month's last row sat below the fold.
        final lastCell = tester.getRect(find.byType(CalendarDayCell).last);
        expect(lastCell.bottom, lessThanOrEqualTo(size.height),
            reason: "the month's last row is clipped at $name");

        // The regression proper: the grid must leave the day detail room —
        // stacked, that is the height share; side-by-side, the whole grid
        // simply fits the surface (the rail owns the detail's room).
        final grid = tester.getRect(find.byType(CalendarMonthGrid));
        if (sideBySide) {
          expect(grid.bottom, lessThanOrEqualTo(size.height + 1),
              reason: 'the month grid overruns the surface at $name');
        } else {
          expect(grid.height,
              lessThanOrEqualTo(size.height * monthGridHeightShare + 1),
              reason: 'the month grid overruns its height budget at $name');
        }

        // And the day detail must actually render — at 1440x810 the pre-fix
        // build dropped it from the tree entirely, so a screen-reader user
        // lost the day's events too.
        expect(find.byType(CalendarDayDetail), findsOneWidget,
            reason: 'the day detail is missing at $name');
        final detail = tester.getRect(find.byType(CalendarDayDetail));
        expect(detail.height, greaterThan(0));
        expect(detail.bottom, lessThanOrEqualTo(size.height + 1));

        if (sideBySide) {
          // The two-pane arm proper: the detail sits BESIDE the grid, in a
          // trailing rail, not squeezed under it (#2912). (No stacked twin of
          // this assertion: inside the reversed scroll view the detail's
          // layout rect can legitimately extend above its clip, so a
          // below-the-grid claim on the rect would be unsound.)
          expect(detail.left, greaterThanOrEqualTo(grid.right - 1),
              reason: 'the day detail is not beside the grid at $name');

          // #2926 follow-up. "Beside" was too weak: it passed with 179px of
          // dead space stranded in the seam. It must also be measured against
          // the cells, not against CalendarMonthGrid's rect — under the old
          // Expanded pane that rect spanned the whole pane while the grid
          // *painted* centred inside it, so a seam assertion on the box was
          // blind to exactly the bug it was meant to catch.
          final cells = tester
              .renderObjectList<RenderBox>(find.byType(CalendarDayCell))
              .map((box) =>
                  box.localToGlobal(Offset.zero) & box.size)
              .toList();
          final paintedLeft =
              cells.map((r) => r.left).reduce((a, b) => a < b ? a : b);
          final paintedRight =
              cells.map((r) => r.right).reduce((a, b) => a > b ? a : b);

          // 40 = the grid pane's 18px inset + the detail scroll view's 22.
          expect(detail.left - paintedRight, lessThanOrEqualTo(44),
              reason: 'dead space is stranded between the panes at $name');

          // The rail is where the reclaimed width goes — event titles run two
          // lines then ellipsize, so this is what the surplus is *for*. 336
          // was the old fixed rail's content width (380 − 44 of padding).
          expect(detail.width, greaterThan(336),
              reason: 'the rail did not absorb the surplus at $name');

          // ...and what neither pane needs lands in the outer margins, split
          // evenly, so the pair reads as a centred composition.
          final groupLeft = paintedLeft - 18;
          final groupRight = detail.right + 22;
          expect((groupLeft - (size.width - groupRight)).abs(),
              lessThanOrEqualTo(2),
              reason: 'the grid/rail pair is not centred at $name');
        }
      });
    });

    testWidgets('at the breakpoint the rail keeps its floor, not its max',
        (tester) async {
      // The rail's 560 max must never be taken out of the grid's hide: at
      // 840 there is no surplus to share, so the rail holds the pre-#2926
      // 380 and the grid keeps every pixel it had. Without the floor the
      // grid's cells would collapse to ~30px here.
      const size = Size(840, 900);
      await pump(tester, embedded: true, physicalSize: size, pixelRatio: 1);
      expect(tester.takeException(), isNull);

      final detail = tester.getRect(find.byType(CalendarDayDetail));
      expect(detail.width, closeTo(380 - 44, 1),
          reason: 'the rail should sit at its floor when there is no surplus');

      final grid = tester.getRect(find.byType(CalendarMonthGrid));
      expect(grid.width, greaterThan(300),
          reason: 'a greedy rail starved the month grid at the breakpoint');
    });

    testWidgets(
        'two-pane semantics order reads grid before detail at 1440x810',
        (tester) async {
      // Visual order = reading order = focus order: the Row's children are
      // grid-then-detail, and Flutter's default traversal follows the widget
      // tree, so the day cells must come before the detail region in the
      // semantics tree (#2912 accessibility requirement).
      await pump(tester,
          embedded: true, physicalSize: const Size(1440, 810), pixelRatio: 1);

      final gridNode = tester.getSemantics(find.byType(CalendarDayCell).first);
      final detailNode = tester.getSemantics(find.byType(CalendarDayDetail));
      // Semantics node ids are assigned in traversal order within a build, so
      // the first day cell must precede the detail region.
      expect(gridNode.id, lessThan(detailNode.id),
          reason: 'day cells must precede the day detail in traversal order');
    });
  });
}
