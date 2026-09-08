import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart'
    show Attribution, StockImageProvider;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/widgets/content/photo_attribution_line.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_content_view.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_presentation.dart';
import 'package:ripls/services/feed_service.dart' show NudgePayload, NudgeStat;
import 'package:ripls/services/providers.dart'
    show mediaAttributionProvider, mediaUrlProvider;

// Helper: build a NudgePayload for tests.
NudgePayload makeNudge({
  int variant = 1,
  String headline = 'Test headline',
  String description = 'Test description',
  String ctaLabel = 'Tap me',
  String ctaAction = 'plan_experience',
  String nudgeId = 'nudge-test-1',
  List<String> mediaIds = const [],
  List<NudgeStat> stats = const [],
  String? locationHint,
  String? secondaryCtaLabel,
  String? secondaryCtaAction,
}) {
  final n = NudgePayload()
    ..nudgeVariant = variant
    ..headline = headline
    ..description = description
    ..ctaLabel = ctaLabel
    ..ctaAction = ctaAction
    ..nudgeId = nudgeId
    ..mediaIds.addAll(mediaIds)
    ..stats.addAll(stats);
  if (locationHint != null) n.locationHint = locationHint;
  if (secondaryCtaLabel != null) n.secondaryCtaLabel = secondaryCtaLabel;
  if (secondaryCtaAction != null) n.secondaryCtaAction = secondaryCtaAction;
  return n;
}

// Wraps a NudgeContentView in a localised MaterialApp with an optional
// ProviderScope override list.
Widget buildTestWidget(NudgePayload nudge) {
  return ProviderScope(
    child: _NudgeTestApp(nudge: nudge),
  );
}

Widget buildTestWidgetWithOverrides(NudgePayload nudge, List<Object> overrides) {
  return ProviderScope(
    overrides: overrides.cast(),
    child: _NudgeTestApp(nudge: nudge),
  );
}

// Wraps the card in a fixed-size box, mimicking a height-constrained host
// such as the calendar's fixed-height open-day nudge card.
Widget buildConstrained(
  NudgePayload nudge, {
  required double width,
  required double height,
  NudgePresentation presentation = NudgePresentation.embedded,
  Attribution? attribution,
  TextScaler textScaler = TextScaler.noScaling,
}) {
  return ProviderScope(
    overrides: [
      mediaUrlProvider.overrideWith((ref, id) async => ''),
      mediaAttributionProvider.overrideWith((ref, id) async => attribution),
    ],
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(
        body: MediaQuery(
          data: MediaQueryData(textScaler: textScaler),
          child: Center(
            child: SizedBox(
              width: width,
              height: height,
              child: NudgeContentView(
                nudge: nudge,
                presentation: presentation,
              ),
            ),
          ),
        ),
      ),
    ),
  );
}

class _NudgeTestApp extends StatelessWidget {
  final NudgePayload nudge;
  const _NudgeTestApp({required this.nudge});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(body: NudgeContentView(nudge: nudge)),
    );
  }
}

void main() {
  group('NudgeContentView – Variant 1 (Magazine Spread)', () {
    testWidgets('renders headline and description centered', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(
        variant: 1,
        headline: 'Big headline',
        description: 'Some italic prose',
        ctaLabel: 'Join Now',
      )));
      await tester.pump();

      expect(find.text('Big headline'), findsOneWidget);
      expect(find.text('Some italic prose'), findsOneWidget);
      expect(find.text('Join Now'), findsOneWidget);
    });

    testWidgets('shows location pill when locationHint is set', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(
        variant: 1,
        locationHint: '20 min from you',
      )));
      await tester.pump();

      expect(find.text('20 min from you'), findsOneWidget);
    });

    testWidgets('no location pill when locationHint is absent', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(variant: 1)));
      await tester.pump();

      // No pill text visible
      expect(find.byType(NudgeContentView), findsOneWidget);
      expect(find.text('Test headline'), findsOneWidget);
    });

    testWidgets('CTA tap does not consume nudge (card stays in feed)', (tester) async {
      bool consumed = false;

      final nudge = makeNudge(
        variant: 1,
        nudgeId: 'nudge-abc',
        ctaAction: 'plan_experience',
        ctaLabel: 'Tap me',
      );

      await tester.pumpWidget(buildTestWidgetWithOverrides(nudge, [
        feedProvider.overrideWith(() => _SpyFeedNotifier(
          onConsumeNudge: (_, _) => consumed = true,
        )),
      ]));
      await tester.pump();

      await tester.tap(find.text('Tap me'));
      await tester.pump();

      // CTA opens the action modal but does NOT consume the nudge — the card
      // remains visible behind the modal.
      expect(consumed, isFalse);
    });
  });

  group('NudgeContentView – Variant 2 (Ticker Tape)', () {
    testWidgets('renders headline and description', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(
        variant: 2,
        headline: 'Alfred is on a roll',
        description: 'He shared 3 items this month.',
        ctaLabel: 'List an item',
      )));
      await tester.pump();

      expect(find.text('Alfred is on a roll'), findsOneWidget);
      expect(find.text('He shared 3 items this month.'), findsOneWidget);
      expect(find.text('List an item'), findsOneWidget);
    });

    testWidgets('renders stats in bottom bar', (tester) async {
      final stat = NudgeStat()
        ..value = '12'
        ..label = 'items shared';

      await tester.pumpWidget(buildTestWidget(makeNudge(
        variant: 2,
        headline: 'Stats headline',
        stats: [stat],
      )));
      await tester.pump();

      expect(find.text('12'), findsOneWidget);
      // Label is uppercased in _StatTile
      expect(find.text('ITEMS SHARED'), findsOneWidget);
    });

    testWidgets('shows secondary CTA button when set', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(
        variant: 2,
        ctaLabel: 'List an item',
        secondaryCtaLabel: 'Offer time',
        secondaryCtaAction: 'offer_time',
      )));
      await tester.pump();

      expect(find.text('List an item'), findsOneWidget);
      expect(find.text('Offer time'), findsOneWidget);
    });

    testWidgets('no secondary button when secondaryCtaLabel absent', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(
        variant: 2,
        ctaLabel: 'Plan a trip',
      )));
      await tester.pump();

      expect(find.text('Plan a trip'), findsOneWidget);
      // No secondary button text rendered
      expect(find.text('Offer time'), findsNothing);
    });
  });

  group('NudgeContentView – Variant 3 (Terminator)', () {
    testWidgets('renders headline and description without badge', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(
        variant: 3,
        headline: "You're all caught up",
        description: 'Close the app and go outside.',
        ctaLabel: 'Plan something',
      )));
      await tester.pump();

      expect(find.text("You're all caught up"), findsOneWidget);
      expect(find.text('Close the app and go outside.'), findsOneWidget);
      expect(find.text('Plan something'), findsOneWidget);
    });

    testWidgets('CTA tap does not consume nudge (card stays in feed)', (tester) async {
      bool consumed = false;

      final nudge = makeNudge(
        variant: 3,
        nudgeId: 'term-1',
        ctaAction: 'plan_experience',
        ctaLabel: 'Plan something',
      );

      await tester.pumpWidget(buildTestWidgetWithOverrides(nudge, [
        feedProvider.overrideWith(() => _SpyFeedNotifier(
          onConsumeNudge: (_, _) => consumed = true,
        )),
      ]));
      await tester.pump();

      await tester.tap(find.text('Plan something'));
      await tester.pump();

      expect(consumed, isFalse);
    });
  });

  group('NudgeContentView – height-constrained host (#2801)', () {
    // The reporter's copy, weather-woven the way CalendarDayDetail weaves it.
    const longHeadline = 'Get the Space Team together for an adventure';
    const longDescription =
        'Overcast but dry, 100° — Doug and Ali are ready for a new outing, '
        'so pick a trail and send the word out before the weekend slips away.';

    // Every height an embedded host can hand the card: the calendar's floor,
    // its preferred height, and the Home zero state's hero.
    const heights = [200.0, 280.0, 320.0];

    NudgePayload longCopy(int variant) => makeNudge(
          variant: variant,
          headline: longHeadline,
          description: longDescription,
          ctaLabel: 'Plan it',
        );

    for (final variant in [1, 2, 3]) {
      for (final height in heights) {
        testWidgets(
            'variant $variant at ${height.toInt()}px neither clips nor '
            'overflows', (tester) async {
          await tester.pumpWidget(
            buildConstrained(longCopy(variant), width: 404, height: height),
          );
          await tester.pump();

          // The bug: a RenderFlex that overflowed its box, hidden in
          // production only by the host's ClipRRect.
          expect(tester.takeException(), isNull);

          // The description either renders whole lines or steps aside — never
          // the fraction of a line that produced the sliced glyphs. It is the
          // sole Flexible, so it is the one that gets squeezed. Above the
          // floor it must actually be there: an "if it exists" check would
          // pass on the broken build too.
          final description = find.text(longDescription);
          if (height > 200) {
            expect(description, findsOneWidget,
                reason: 'description should survive at ${height.toInt()}px');
          }
          if (description.evaluate().isNotEmpty) {
            final descHeight = tester.getSize(description).height;
            // One line of nudgeBodyStyle is 16 * 1.6 = 25.6.
            expect(descHeight, greaterThanOrEqualTo(25.0),
                reason: 'description got a partial line box');
          }

          // The CTA survives at every height and stays inside the card.
          final cta = find.text('Plan it');
          expect(cta, findsOneWidget);
          final card = tester.getRect(find.byType(NudgeContentView));
          final ctaRect = tester.getRect(cta);
          expect(card.contains(ctaRect.topLeft), isTrue);
          expect(card.contains(ctaRect.bottomRight), isTrue);
        });
      }

      testWidgets(
          'variant $variant survives 200% text scaling at the 200px floor',
          (tester) async {
        // A fixed line reserve would silently reintroduce the clip here:
        // every line box doubles while the card does not.
        await tester.pumpWidget(buildConstrained(
          longCopy(variant),
          width: 404,
          height: 200,
          textScaler: const TextScaler.linear(2),
        ));
        await tester.pump();

        expect(tester.takeException(), isNull);
      });
    }

    testWidgets('a long headline no longer starves the description',
        (tester) async {
      await tester.pumpWidget(
        buildConstrained(longCopy(1), width: 404, height: 280),
      );
      await tester.pump();

      // Before #2801 the headline took all three of its lines and left the
      // description a 15px box against a 25.6px line. Both must be readable.
      expect(find.text(longHeadline), findsOneWidget);
      expect(find.text(longDescription), findsOneWidget);
      expect(tester.getSize(find.text(longDescription)).height,
          greaterThanOrEqualTo(25.0));
    });

    testWidgets('full-screen presentation still gets its full line budget',
        (tester) async {
      await tester.pumpWidget(buildConstrained(
        longCopy(1),
        width: 404,
        height: 900,
        presentation: NudgePresentation.fullScreen,
      ));
      await tester.pump();

      // 3 headline lines * 32 * 1.15 = 110.4; 3 description lines * 25.6.
      expect(tester.getSize(find.text(longHeadline)).height,
          greaterThan(100.0));
      expect(tester.getSize(find.text(longDescription)).height,
          greaterThan(70.0));
      expect(tester.takeException(), isNull);
    });
  });

  group('NudgeContentView – photo attribution (#2801)', () {
    Attribution unsplash() => Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH,
          creatorName: 'Jane Doe',
          creatorUsername: 'janedoe',
          originalUrl: 'https://unsplash.com/photos/abc',
        );

    testWidgets('renders the credit inline instead of behind an info button',
        (tester) async {
      await tester.pumpWidget(buildConstrained(
        makeNudge(variant: 1, mediaIds: const ['m-1']),
        width: 404,
        height: 280,
        attribution: unsplash(),
      ));
      await tester.pumpAndSettle();

      expect(find.byType(PhotoAttributionLine), findsOneWidget);
      expect(find.text('Jane Doe'), findsOneWidget);
      // The button that opened a bottom sheet at the far edge of the screen
      // is gone — the credit it hid is simply on the card.
      expect(find.byIcon(Icons.info_outline), findsNothing);
    });

    testWidgets('the credit clears the headline and the CTA at 200px',
        (tester) async {
      await tester.pumpWidget(buildConstrained(
        makeNudge(
          variant: 1,
          headline: 'Get the Space Team together for an adventure',
          ctaLabel: 'Plan it',
          mediaIds: const ['m-1'],
        ),
        width: 404,
        height: 200,
        attribution: unsplash(),
      ));
      await tester.pumpAndSettle();

      final credit = tester.getRect(find.byType(PhotoAttributionLine));
      final headline =
          tester.getRect(find.text('Get the Space Team together for an adventure'));
      final cta = tester.getRect(find.text('Plan it'));
      expect(credit.overlaps(headline), isFalse);
      expect(credit.overlaps(cta), isFalse);
    });

    testWidgets('no credit when the media carries no attribution',
        (tester) async {
      await tester.pumpWidget(buildConstrained(
        makeNudge(variant: 1, mediaIds: const ['m-1']),
        width: 404,
        height: 280,
      ));
      await tester.pumpAndSettle();

      expect(find.byType(PhotoAttributionLine), findsNothing);
    });
  });

  group('NudgeContentView – unknown variant falls back to Variant 1', () {
    testWidgets('renders headline for unknown variant', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(variant: 99, headline: 'Fallback test')));
      await tester.pump();

      expect(find.text('Fallback test'), findsOneWidget);
    });
  });

  group('NudgeContentView – media resolution', () {
    testWidgets('renders without crashing when media ID is absent', (tester) async {
      await tester.pumpWidget(buildTestWidget(makeNudge(variant: 1, mediaIds: [])));
      await tester.pump();

      expect(find.text('Test headline'), findsOneWidget);
    });
  });
}

// _SpyFeedNotifier is a minimal FeedNotifier that records consumeNudge calls.
class _SpyFeedNotifier extends FeedNotifier {
  final void Function(String nudgeId, String action) onConsumeNudge;

  _SpyFeedNotifier({required this.onConsumeNudge});

  @override
  void consumeNudge(String nudgeId, String action) {
    onConsumeNudge(nudgeId, action);
  }
}
