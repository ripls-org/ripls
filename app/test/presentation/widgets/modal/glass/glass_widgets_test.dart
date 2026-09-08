import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

Widget _wrap(Widget child) {
  return MaterialApp(
    theme: AppTheme.lightTheme,
    localizationsDelegates: const [
      AppLocalizations.delegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(
      backgroundColor: Colors.black,
      body: Center(child: child),
    ),
  );
}

void main() {
  group('GlassSurface', () {
    testWidgets('renders child', (tester) async {
      await tester.pumpWidget(
        _wrap(const GlassSurface(child: Text('hello'))),
      );
      expect(find.text('hello'), findsOneWidget);
    });

    testWidgets('omits BackdropFilter when useBlur is false', (tester) async {
      await tester.pumpWidget(
        _wrap(const GlassSurface(useBlur: false, child: Text('flat'))),
      );
      expect(find.byType(BackdropFilter), findsNothing);
    });

    testWidgets('renders BackdropFilter when useBlur is true', (tester) async {
      await tester.pumpWidget(
        _wrap(const GlassSurface(child: Text('blurry'))),
      );
      expect(find.byType(BackdropFilter), findsAtLeast(1));
    });
  });

  group('GlassSheet', () {
    testWidgets('renders drag handle and child', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const GlassSheet(
            child: SizedBox(height: 100, child: Text('sheet body')),
          ),
        ),
      );
      expect(find.text('sheet body'), findsOneWidget);
      // Drag handle has its own semantics label.
      expect(find.bySemanticsLabel('Drag handle'), findsOneWidget);
    });

    testWidgets('omits drag handle when showDragHandle is false', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const GlassSheet(
            showDragHandle: false,
            child: SizedBox(height: 100, child: Text('no-handle body')),
          ),
        ),
      );
      expect(find.text('no-handle body'), findsOneWidget);
      expect(find.bySemanticsLabel('Drag handle'), findsNothing);
    });
  });

  group('GlassModalHeader', () {
    testWidgets('renders kicker (uppercased) and value', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const GlassModalHeader(
            kicker: 'When',
            value: 'Fri, May 8 · 9:00 AM · 1 hr',
          ),
        ),
      );
      expect(find.text('WHEN'), findsOneWidget);
      expect(find.text('Fri, May 8 · 9:00 AM · 1 hr'), findsOneWidget);
    });

    testWidgets('value is announced as a header', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const GlassModalHeader(
            kicker: 'When',
            value: 'header value',
          ),
        ),
      );
      final headerNode = tester.getSemantics(find.text('header value'));
      expect(
        headerNode.flagsCollection.isHeader,
        isTrue,
        reason: 'Header value should expose isHeader semantics flag',
      );
    });

    testWidgets('omits icon badge when icon is null', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const GlassModalHeader(kicker: 'X', value: 'y'),
        ),
      );
      expect(find.byType(Icon), findsNothing);
    });
  });

  group('GlassFieldLabel', () {
    testWidgets('uppercases the text', (tester) async {
      await tester.pumpWidget(_wrap(const GlassFieldLabel(text: 'date')));
      expect(find.text('DATE'), findsOneWidget);
    });
  });

  group('GlassChip', () {
    testWidgets('renders primary and secondary lines', (tester) async {
      await tester.pumpWidget(
        _wrap(
          GlassChip(
            primary: 'Today',
            secondary: 'May 8',
            selected: false,
            onTap: () {},
            semanticsLabel: 'Today, May 8',
          ),
        ),
      );
      expect(find.text('Today'), findsOneWidget);
      expect(find.text('May 8'), findsOneWidget);
    });

    testWidgets('exposes selected semantic flag', (tester) async {
      await tester.pumpWidget(
        _wrap(
          GlassChip(
            primary: 'Today',
            selected: true,
            onTap: () {},
            semanticsLabel: 'Today',
          ),
        ),
      );
      final node = tester.getSemantics(find.bySemanticsLabel('Today'));
      // flagsCollection.isSelected is a Tristate, not a plain bool.
      // toString() is "Tristate.isTrue" / "Tristate.isFalse" / "Tristate.unset".
      expect(
        node.flagsCollection.isSelected.toString(),
        contains('isTrue'),
        reason:
            'Selected GlassChip should expose isSelected via Toggle semantics.',
      );
    });

    testWidgets('fires onTap', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        _wrap(
          GlassChip(
            primary: 'Today',
            selected: false,
            onTap: () => taps++,
            semanticsLabel: 'Today',
          ),
        ),
      );
      await tester.tap(find.text('Today'));
      await tester.pump();
      expect(taps, 1);
    });

    testWidgets('shows a ×N badge only when quantity > 1', (tester) async {
      await tester.pumpWidget(
        _wrap(
          GlassChip(
            primary: 'Chairs',
            selected: false,
            quantity: 1,
            onTap: () {},
            semanticsLabel: 'Chairs',
          ),
        ),
      );
      expect(find.text('×1'), findsNothing);

      await tester.pumpWidget(
        _wrap(
          GlassChip(
            primary: 'Chairs',
            selected: false,
            quantity: 3,
            onTap: () {},
            semanticsLabel: 'Chairs',
          ),
        ),
      );
      expect(find.text('×3'), findsOneWidget);
    });

    testWidgets('shows a comment glyph only when hasComment', (tester) async {
      await tester.pumpWidget(
        _wrap(
          GlassChip(
            primary: 'Chairs',
            selected: false,
            onTap: () {},
            semanticsLabel: 'Chairs',
          ),
        ),
      );
      expect(find.byIcon(Icons.mode_comment_outlined), findsNothing);

      await tester.pumpWidget(
        _wrap(
          GlassChip(
            primary: 'Chairs',
            selected: false,
            hasComment: true,
            onTap: () {},
            semanticsLabel: 'Chairs',
          ),
        ),
      );
      expect(find.byIcon(Icons.mode_comment_outlined), findsOneWidget);
    });
  });

  group('GlassInlineAction', () {
    testWidgets('renders text and chevron', (tester) async {
      await tester.pumpWidget(
        _wrap(
          GlassInlineAction(
            text: 'Pick another date…',
            semanticsLabel: 'Pick another date',
            icon: Icons.calendar_today,
            onTap: () {},
          ),
        ),
      );
      expect(find.text('Pick another date…'), findsOneWidget);
      expect(find.byIcon(Icons.chevron_right), findsOneWidget);
      expect(find.byIcon(Icons.calendar_today), findsOneWidget);
    });
  });

  group('GlassFooterButtons', () {
    testWidgets('renders cancel + save by default labels', (tester) async {
      await tester.pumpWidget(
        _wrap(
          GlassFooterButtons(
            primaryEnabled: true,
            onPrimary: () {},
            onSecondary: () {},
          ),
        ),
      );
      expect(find.text('Save'), findsOneWidget);
      expect(find.text('Cancel'), findsOneWidget);
    });

    testWidgets('disables primary tap when primaryEnabled is false',
        (tester) async {
      var primaryTaps = 0;
      await tester.pumpWidget(
        _wrap(
          GlassFooterButtons(
            primaryEnabled: false,
            onPrimary: () => primaryTaps++,
            onSecondary: () {},
          ),
        ),
      );
      await tester.tap(find.text('Save'));
      await tester.pump();
      expect(primaryTaps, 0);
    });

    testWidgets('omits secondary when showSecondary is false', (tester) async {
      await tester.pumpWidget(
        _wrap(
          GlassFooterButtons(
            primaryEnabled: true,
            showSecondary: false,
            onPrimary: () {},
          ),
        ),
      );
      expect(find.text('Cancel'), findsNothing);
      expect(find.text('Save'), findsOneWidget);
    });
  });

  group('GlassSearchInput', () {
    testWidgets('renders hint and leading icon', (tester) async {
      final controller = TextEditingController();
      final node = FocusNode();
      addTearDown(() {
        controller.dispose();
        node.dispose();
      });
      await tester.pumpWidget(
        _wrap(
          GlassSearchInput(
            controller: controller,
            focusNode: node,
            hintText: 'Search…',
            onChanged: (_) {},
            onSubmitted: (_) {},
          ),
        ),
      );
      expect(find.text('Search…'), findsOneWidget);
      expect(find.byIcon(Icons.search), findsOneWidget);
    });
  });

  group('GlassInsetCard', () {
    testWidgets('renders child', (tester) async {
      await tester.pumpWidget(
        _wrap(
          const GlassInsetCard(child: Text('inset body')),
        ),
      );
      expect(find.text('inset body'), findsOneWidget);
    });

    testWidgets('does not render a BackdropFilter (no recursive blur)',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const GlassInsetCard(child: Text('flat')),
        ),
      );
      // GlassInsetCard's whole purpose is to be a flat-fill alternative
      // to GlassSurface inside a parent glass sheet — nesting blurs
      // produces ring artifacts on iOS+Metal. Confirm no blur layer.
      expect(find.byType(BackdropFilter), findsNothing);
    });

    testWidgets('is presentational without onTap (no Tappable wrapper)',
        (tester) async {
      await tester.pumpWidget(
        _wrap(
          const GlassInsetCard(child: Text('static')),
        ),
      );
      // No Tappable should be inserted when onTap is null — the card
      // is purely presentational and shouldn't claim a button role.
      expect(find.byType(Tappable), findsNothing);
    });

    testWidgets('wraps in Tappable when onTap is provided', (tester) async {
      await tester.pumpWidget(
        _wrap(
          GlassInsetCard(
            onTap: () {},
            semanticsLabel: 'Open',
            child: const Text('tappable'),
          ),
        ),
      );
      expect(find.byType(Tappable), findsOneWidget);
    });

    testWidgets('fires onTap when tapped', (tester) async {
      var tapped = 0;
      await tester.pumpWidget(
        _wrap(
          GlassInsetCard(
            onTap: () => tapped += 1,
            semanticsLabel: 'Open detail',
            child: const SizedBox(
              width: 200,
              height: 80,
              child: Text('tappable card'),
            ),
          ),
        ),
      );
      await tester.tap(find.text('tappable card'));
      await tester.pump();
      expect(tapped, 1);
    });
  });
}
