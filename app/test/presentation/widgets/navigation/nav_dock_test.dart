import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/navigation/nav_destination.dart';
import 'package:ripls/presentation/widgets/navigation/nav_dock.dart';

void main() {
  RiplsNavDestination? tappedDestination;
  var createTaps = 0;

  setUp(() {
    tappedDestination = null;
    createTaps = 0;
  });

  Widget host({
    int selectedStackIndex = 1,
    int badge = 0,
    bool highContrast = false,
    bool disableAnimations = false,
  }) {
    return MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: MediaQuery(
        data: MediaQueryData(
          highContrast: highContrast,
          disableAnimations: disableAnimations,
        ),
        child: Scaffold(
          body: Align(
            alignment: Alignment.bottomCenter,
            child: NavDock(
              selectedStackIndex: selectedStackIndex,
              homeBadgeCount: badge,
              onDestinationTap: (d) => tappedDestination = d,
              onCreateTap: () => createTaps++,
            ),
          ),
        ),
      ),
    );
  }

  testWidgets(
      'renders four tabs with the label only on the selected pill '
      '(NavVariations-v3 · liquid glass, light)', (tester) async {
    await tester.pumpWidget(host()); // stack index 1 = Home selected

    // Only the selected tab carries a visible label; the rest are
    // icon-only (their names live in semantics).
    expect(find.text('Home'), findsOneWidget);
    expect(find.text('Plans'), findsNothing);
    expect(find.text('Library'), findsNothing);
    expect(find.text('People'), findsNothing);
    expect(find.byIcon(Icons.add), findsOneWidget);
    // No search in the bar — it moved to the destination header.
    expect(find.byIcon(Icons.search), findsNothing);
  });

  testWidgets('taps route to destinations and Create', (tester) async {
    await tester.pumpWidget(host());

    await tester.tap(find.bySemanticsLabel('Plans'));
    expect(tappedDestination, RiplsNavDestination.plans);

    await tester.tap(find.bySemanticsLabel('Library'));
    expect(tappedDestination, RiplsNavDestination.library);

    await tester.tap(find.byIcon(Icons.add));
    expect(createTaps, 1);
  });

  testWidgets('home badge renders its count and hides at zero',
      (tester) async {
    await tester.pumpWidget(host(badge: 5));
    expect(find.text('5'), findsOneWidget);

    await tester.pumpWidget(host(badge: 0));
    expect(find.text('5'), findsNothing);
  });

  testWidgets('orphaned stack index renders with no selected tab',
      (tester) async {
    // Index 0 is the retired Feed; the dock must not crash nor highlight —
    // and with no selection, no tab shows a label and no bubble renders.
    await tester.pumpWidget(host(selectedStackIndex: 0));
    expect(find.text('Home'), findsNothing);
    expect(find.bySemanticsLabel('Home'), findsOneWidget);
    expect(find.byKey(NavDock.selectionPillKey), findsNothing);
  });

  testWidgets('selection bubble slides from the old tab to the new one',
      (tester) async {
    await tester.pumpWidget(host()); // Home selected
    final pill = find.byKey(NavDock.selectionPillKey);
    final homeLeft = tester.getTopLeft(pill).dx;

    // Select Library (stack index 2, third tab) and freeze mid-flight.
    await tester.pumpWidget(host(selectedStackIndex: 2));
    await tester.pump(const Duration(milliseconds: 140));
    final midFlightLeft = tester.getTopLeft(pill).dx;

    await tester.pumpAndSettle();
    final libraryLeft = tester.getTopLeft(pill).dx;

    // The bubble travelled right, passing through an intermediate spot —
    // it did not vanish on Home and reappear on Library.
    expect(libraryLeft, greaterThan(homeLeft));
    expect(midFlightLeft, greaterThan(homeLeft));
    expect(midFlightLeft, lessThan(libraryLeft));

    // The label follows the bubble: only the new destination's shows.
    expect(find.text('Library'), findsOneWidget);
    expect(find.text('Home'), findsNothing);
  });

  testWidgets('reduce motion snaps the bubble with no travel frames',
      (tester) async {
    await tester.pumpWidget(host(disableAnimations: true));
    final pill = find.byKey(NavDock.selectionPillKey);

    await tester.pumpWidget(host(
      selectedStackIndex: 2,
      disableAnimations: true,
    ));
    await tester.pump();
    final snappedLeft = tester.getTopLeft(pill).dx;

    // Already settled: further pumping does not move the bubble, and no
    // animation is left running.
    expect(tester.binding.transientCallbackCount, 0);
    await tester.pump(const Duration(milliseconds: 140));
    expect(tester.getTopLeft(pill).dx, snappedLeft);
    expect(find.text('Library'), findsOneWidget);
  });

  testWidgets('high contrast swaps blur for the solid fallback',
      (tester) async {
    await tester.pumpWidget(host());
    expect(find.byType(BackdropFilter), findsWidgets);

    await tester.pumpWidget(host(highContrast: true));
    expect(find.byType(BackdropFilter), findsNothing);
  });

  testWidgets('every dock target has a >=48dp touch target', (tester) async {
    await tester.pumpWidget(host());

    for (final label in ['Home', 'Plans', 'Library', 'People']) {
      final size = tester.getSize(find.bySemanticsLabel(label));
      expect(size.height, greaterThanOrEqualTo(48),
          reason: '$label tab height');
    }

    final createSize = tester.getSize(
      find
          .ancestor(
            of: find.byIcon(Icons.add),
            matching: find.byType(SizedBox),
          )
          .first,
    );
    expect(createSize.height, greaterThanOrEqualTo(48));
    expect(createSize.width, greaterThanOrEqualTo(48));
  });

  testWidgets('dock geometry is identical between glass and solid',
      (tester) async {
    await tester.pumpWidget(host());
    final glassSize = tester.getSize(find.byType(NavDock));

    await tester.pumpWidget(host(highContrast: true));
    final solidSize = tester.getSize(find.byType(NavDock));

    expect(solidSize, glassSize);
  });

  group('capsule width cap (#2912)', () {
    testWidgets('caps and centers the capsule on a desktop-wide window',
        (tester) async {
      tester.view.physicalSize = const Size(1440, 810);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);

      await tester.pumpWidget(host());

      // The NavDock widget spans whatever its parent gives it; the visible
      // capsule inside is the capped, centered pill.
      final capsule = tester.getRect(find.byType(GlassSurface));
      expect(capsule.width, Responsive.dockMaxWidth);
      expect(capsule.center.dx, moreOrLessEquals(1440 / 2, epsilon: 1));
    });

    testWidgets('never binds at a phone width', (tester) async {
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);

      await tester.pumpWidget(host());

      final capsule = tester.getRect(find.byType(GlassSurface));
      expect(capsule.width, 390);
    });
  });
}
