import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/home/home_empty_states.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_content_view.dart';
import 'package:ripls/services/feed_service.dart' show NudgePayload;

const _planTitle = 'Got something to plan?';
const _askTitle = 'Need a hand with something?';
const _offerTitle = 'Have something to share?';
const _planCta = 'Plan an event';
const _askCta = 'Ask for something';
const _offerCta = 'Offer something';

const _allTitles = [_planTitle, _askTitle, _offerTitle];
const _allCtas = [_planCta, _askCta, _offerCta];

/// Every prompt title renders at this size. There is no hero size any more —
/// the three are peer choices, so none of them is scaled up.
const _promptTitleSize = 16.0;

Future<void> _pump(
  WidgetTester tester, {
  VoidCallback? onPlan,
  VoidCallback? onAsk,
  VoidCallback? onOffer,
  NudgePayload? nudge,
  VoidCallback? onNudgeConsumed,
}) {
  return tester.pumpWidget(
    ProviderScope(
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(
          // Mirror the real host (a scrolling ListView) so a tall nudge card
          // doesn't overflow the fixed test surface.
          body: SingleChildScrollView(
            child: HomeZeroStateCard(
              onPlan: onPlan ?? () {},
              onAsk: onAsk ?? () {},
              onOffer: onOffer ?? () {},
              nudge: nudge,
              onNudgeConsumed: onNudgeConsumed,
            ),
          ),
        ),
      ),
    ),
  );
}

NudgePayload _nudge() => NudgePayload(
      nudgeId: 'n-1',
      headline: 'Get the crew together for an adventure',
      description: 'Doug and Ali are ready for a new outing.',
      ctaLabel: 'Plan something',
      ctaAction: 'plan_experience',
    );

/// The font size of the [Text] rendering [text].
double _titleSize(WidgetTester tester, String text) =>
    tester.widget<Text>(find.text(text)).style!.fontSize!;

/// The top edge of the first widget matching [finder].
double _topOf(WidgetTester tester, Finder finder) =>
    tester.getTopLeft(finder).dy;

/// The decoration of the pill wrapping the CTA labelled [text]. Two prompts
/// styled the same way produce equal decorations.
Decoration? _ctaDecoration(WidgetTester tester, String text) => tester
    .widget<Container>(
      find
          .ancestor(of: find.text(text), matching: find.byType(Container))
          .first,
    )
    .decoration;

/// Taps [text] after scrolling it into view. The real host is a scrolling
/// ListView; the 800×600 test surface is shorter than the three prompts plus
/// a host prompt, so a bare tap() misses the last one.
Future<void> _tapCta(WidgetTester tester, String text) async {
  await tester.ensureVisible(find.text(text));
  await tester.pumpAndSettle();
  await tester.tap(find.text(text));
}

void main() {
  // All three affordances are unconditional (#2936) — the zero state can
  // never be empty, and none of them needs a server round trip.
  group('HomeZeroStateCard — no host prompt', () {
    testWidgets('offers plan, ask, and offer', (tester) async {
      await _pump(tester);
      for (final title in _allTitles) {
        expect(find.text(title), findsOneWidget);
      }
      for (final cta in _allCtas) {
        expect(find.text(cta), findsOneWidget);
      }
    });

    // The three are peer choices, so they render identically. Ask used to be
    // a hero — a bordered sage box carrying a rotating canned example and a
    // filled pill reading "Ask the crew ›" — which made one of the three look
    // broken next to the others.
    testWidgets('the three prompts are visually uniform', (tester) async {
      await _pump(tester);

      for (final title in _allTitles) {
        expect(_titleSize(tester, title), _promptTitleSize,
            reason: '"$title" is scaled differently from its peers');
      }

      // Same pill treatment: one decoration shared by all three, so a
      // difference in fill or border shows up as unequal decorations.
      final decorations = _allCtas.map((cta) => _ctaDecoration(tester, cta));
      expect(decorations.toSet(), hasLength(1),
          reason: 'the three CTA pills do not share one treatment');

      // No CTA carries a decorative chevron the others lack.
      for (final cta in _allCtas) {
        expect(cta, isNot(contains('›')));
      }
    });

    testWidgets('renders no host-prompt card', (tester) async {
      await _pump(tester);
      expect(find.byType(NudgeContentView), findsNothing);
    });

    testWidgets('the plan CTA fires onPlan', (tester) async {
      var planned = false;
      await _pump(tester, onPlan: () => planned = true);
      await _tapCta(tester, _planCta);
      expect(planned, isTrue);
    });

    testWidgets('the request CTA fires onAsk', (tester) async {
      var asked = false;
      await _pump(tester, onAsk: () => asked = true);
      await _tapCta(tester, _askCta);
      expect(asked, isTrue);
    });

    testWidgets('the offer CTA fires onOffer', (tester) async {
      var offered = false;
      await _pump(tester, onOffer: () => offered = true);
      await _tapCta(tester, _offerCta);
      expect(offered, isTrue);
    });
  });

  // A host prompt rides a real signal from the momentum engine, so it still
  // earns the lead slot. The three affordances render below it regardless.
  group('HomeZeroStateCard — with a host prompt', () {
    testWidgets('the host prompt leads and the three stay uniform below it',
        (tester) async {
      await _pump(tester, nudge: _nudge());

      expect(find.byType(NudgeContentView), findsOneWidget);

      // #2796: the host prompt is the only hero. On the reported arrangement
      // the request stayed a 22pt hero *and* wrapped the card, giving the box
      // two heroes.
      for (final title in _allTitles) {
        expect(_titleSize(tester, title), _promptTitleSize);
      }
    });

    testWidgets('the host prompt renders above all three prompts, not inside '
        'one', (tester) async {
      await _pump(tester, nudge: _nudge());

      // #2796 regression guard. When the card was nested inside the request
      // hero, the request title painted *above* it; ordering the card first
      // is what "one hero" looks like from the outside.
      final nudgeTop = _topOf(tester, find.byType(NudgeContentView));
      final planTop = _topOf(tester, find.text(_planTitle));
      final askTop = _topOf(tester, find.text(_askTitle));
      final offerTop = _topOf(tester, find.text(_offerTitle));

      expect(nudgeTop, lessThan(planTop));
      expect(planTop, lessThan(askTop));
      expect(askTop, lessThan(offerTop));
    });

    testWidgets('all three prompts keep working alongside the host prompt',
        (tester) async {
      var planned = false;
      var asked = false;
      var offered = false;
      await _pump(
        tester,
        nudge: _nudge(),
        onPlan: () => planned = true,
        onAsk: () => asked = true,
        onOffer: () => offered = true,
      );

      expect(find.text(_planCta), findsOneWidget);
      expect(find.text(_askCta), findsOneWidget);
      expect(find.text(_offerCta), findsOneWidget);

      await _tapCta(tester, _planCta);
      expect(planned, isTrue);
      await _tapCta(tester, _askCta);
      expect(asked, isTrue);
      await _tapCta(tester, _offerCta);
      expect(offered, isTrue);
    });
  });

  group('HomeZeroStateCard — touch targets', () {
    testWidgets('every CTA clears the 48dp minimum', (tester) async {
      for (final nudge in [null, _nudge()]) {
        await _pump(tester, nudge: nudge);
        for (final cta in [_planCta, _askCta, _offerCta]) {
          final size = tester.getSize(
            find.ancestor(
              of: find.text(cta),
              matching: find.byType(ConstrainedBox),
            ).first,
          );
          expect(
            size.height,
            greaterThanOrEqualTo(kMinInteractiveDimension),
            reason: '"$cta" pill is below the 48dp touch floor',
          );
        }
      }
    });
  });
}
