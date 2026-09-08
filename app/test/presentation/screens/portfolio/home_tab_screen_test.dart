import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/portfolio/home_tab_screen.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_tab_view_model.dart';
import 'package:ripls/presentation/widgets/home/home_empty_states.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart' show authStateProvider;

/// Serves a fixed [HomeTabState] so the screen renders without a server.
class _FakeHomeTab extends HomeTabNotifier {
  _FakeHomeTab(this._state);

  final HomeTabState _state;

  @override
  HomeTabState build() => _state;

  @override
  Future<void> load() async {}
}

/// A settled, empty feed so the pulse section renders nothing and never
/// touches a repository.
class _FakeFeed extends FeedNotifier {
  @override
  FeedState build() =>
      const FeedState(isLoading: false, hasMore: false, items: []);
}

class _FakeAuth extends AuthStateNotifier {
  @override
  AuthStateData build() => AuthStateData(
        isLoading: false,
        user: User(id: 'u-1', name: 'Wilma Wide'),
      );
}

void main() {
  Future<void> pump(WidgetTester tester, {required Size surface}) async {
    tester.view.physicalSize = surface;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(ProviderScope(
      overrides: [
        homeTabProvider.overrideWith(
          () => _FakeHomeTab(
            HomeTabState(view: GetHomeViewResponse(), isLoading: false),
          ),
        ),
        feedProvider.overrideWith(_FakeFeed.new),
        authStateProvider.overrideWith(_FakeAuth.new),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: const HomeTabScreen(),
      ),
    ));
    await tester.pump();
  }

  group('desktop measure (#2912)', () {
    testWidgets(
        'the zero-state hero and its CTAs hold the centered reading column '
        'at 1440x810', (tester) async {
      const surface = Size(1440, 810);
      await pump(tester, surface: surface);
      expect(tester.takeException(), isNull);

      final bandLeft = (surface.width - Responsive.contentMaxWidth) / 2;
      final bandRight = bandLeft + Responsive.contentMaxWidth;

      final hero = tester.getRect(find.byType(HomeZeroStateCard));
      expect(hero.width, lessThanOrEqualTo(Responsive.contentMaxWidth));
      expect(hero.center.dx, moreOrLessEquals(surface.width / 2, epsilon: 1));

      // The ask pill — window-wide before #2912 — sits inside the measure
      // band. Checked on all three CTAs, which are now identical prompts
      // (#2936), so a regression in any one of them shows up.
      for (final cta in const [
        'Plan an event',
        'Ask for something',
        'Offer something',
      ]) {
        final pill = tester.getRect(find.textContaining(cta));
        expect(pill.left, greaterThanOrEqualTo(bandLeft), reason: cta);
        expect(pill.right, lessThanOrEqualTo(bandRight), reason: cta);
      }

      // The greeting header aligns with the same column.
      final greeting = tester.getRect(find.textContaining('Wilma'));
      expect(greeting.left, greaterThanOrEqualTo(bandLeft));
    });

    testWidgets('phone geometry is unchanged at 390x844', (tester) async {
      const surface = Size(390, 844);
      await pump(tester, surface: surface);
      expect(tester.takeException(), isNull);

      // The hero card keeps its full-width layout: the card's padding is
      // 20 each side, so its box spans the window exactly as before #2912.
      final hero = tester.getRect(find.byType(HomeZeroStateCard));
      expect(hero.width, surface.width);
    });
  });
}
