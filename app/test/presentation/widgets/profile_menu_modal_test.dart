import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/viewmodels/me_sheet_view_model.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/presentation/widgets/navigation/nav_destination.dart';
import 'package:ripls/presentation/widgets/profile_menu_modal.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

/// Spy SearchNotifier that records the person filter without running the
/// real (network-backed) build/search, so the My-gear row can be tested
/// in isolation.
class _SpySearchNotifier extends SearchNotifier {
  String? filteredPersonId;

  @override
  Future<SearchState> build() async => const SearchState();

  @override
  void filterByPerson(String? userId) => filteredPersonId = userId;
}

/// Test notifier that records whether logout was called without performing
/// any of the real teardown work (secure storage, cache clears, etc.).
class _TestAuthStateNotifier extends AuthStateNotifier {
  _TestAuthStateNotifier(this._initial);

  final AuthStateData _initial;
  int logoutCount = 0;

  @override
  AuthStateData build() => _initial;

  @override
  Future<void> logout() async {
    logoutCount += 1;
    state = AuthStateData.empty;
  }
}

void main() {
  late _TestAuthStateNotifier authNotifier;
  late _SpySearchNotifier spySearch;

  AuthStateData seedAuthState({String userId = 'user-1'}) {
    return AuthStateData(
      accessToken: 'token',
      user: User()..id = userId,
      isLoading: false,
    );
  }

  setUp(() {
    authNotifier = _TestAuthStateNotifier(seedAuthState());
    spySearch = _SpySearchNotifier();
  });

  Widget createHostApp() {
    return ProviderScope(
      overrides: [
        authStateProvider.overrideWith(() => authNotifier),
        // The identity impact line fetches portfolio metrics; the tests
        // exercise rows, not metrics.
        meSheetImpactProvider.overrideWith((ref) async => null),
        // Keep the Library search off the network — the My-gear row reads
        // its notifier to apply the person filter.
        searchProvider.overrideWith(() => spySearch),
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
          body: Builder(
            builder: (context) => ElevatedButton(
              onPressed: () => ProfileMenuModal.show(context),
              child: const Text('open'),
            ),
          ),
        ),
      ),
    );
  }

  Future<void> openModal(WidgetTester tester) async {
    await tester.pumpWidget(createHostApp());
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
  }

  testWidgets('renders all menu rows with localized labels', (
    tester,
  ) async {
    await openModal(tester);

    expect(find.text('Profile'), findsOneWidget);
    expect(find.text('My gear'), findsOneWidget);
    expect(find.text('My plans'), findsOneWidget);
    // Borrowing was removed from the Me sheet.
    expect(find.text('Borrowing'), findsNothing);
    expect(find.text('Notifications'), findsOneWidget);
    expect(find.text('Community invitations'), findsOneWidget);
    // #2797: the row opens the feedback sheet and nothing else, so the label
    // has to say so — "Help" alone hid the app's only in-product feedback path.
    expect(find.text('Help & Feedback'), findsOneWidget);
    expect(find.text('Report a bug or share an idea'), findsOneWidget);
    expect(find.text('Settings'), findsOneWidget);
    expect(find.text('Logout'), findsOneWidget);
  });

  testWidgets(
      'tapping My gear switches to the Library tab, filters to the current '
      'user, and dismisses', (tester) async {
    await openModal(tester);
    expect(find.byType(ProfileMenuModal), findsOneWidget);

    await tester.ensureVisible(find.text('My gear'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('My gear'));
    await tester.pumpAndSettle();

    expect(find.byType(ProfileMenuModal), findsNothing);
    final container =
        ProviderScope.containerOf(tester.element(find.text('open')));
    expect(
      container.read(homeProvider).selectedIndex,
      RiplsNavDestination.library.stackIndex,
    );
    expect(spySearch.filteredPersonId, 'user-1');
  });

  testWidgets('tapping My plans switches to the Plans tab and dismisses', (
    tester,
  ) async {
    await openModal(tester);
    expect(find.byType(ProfileMenuModal), findsOneWidget);

    await tester.ensureVisible(find.text('My plans'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('My plans'));
    await tester.pumpAndSettle();

    expect(find.byType(ProfileMenuModal), findsNothing);
  });

  testWidgets('tapping Logout dispatches logout and dismisses the modal', (
    tester,
  ) async {
    await openModal(tester);

    expect(authNotifier.logoutCount, 0);
    expect(find.byType(ProfileMenuModal), findsOneWidget);

    await tester.ensureVisible(find.text('Logout'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Logout'));
    await tester.pumpAndSettle();

    expect(authNotifier.logoutCount, 1);
    expect(find.byType(ProfileMenuModal), findsNothing);
  });

  testWidgets('tapping Help & Feedback dismisses the modal', (tester) async {
    await openModal(tester);
    expect(find.byType(ProfileMenuModal), findsOneWidget);

    await tester.ensureVisible(find.text('Help & Feedback'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Help & Feedback'));
    await tester.pumpAndSettle();

    expect(find.byType(ProfileMenuModal), findsNothing);
  });

  testWidgets('tapping Settings dismisses the modal', (tester) async {
    await openModal(tester);
    expect(find.byType(ProfileMenuModal), findsOneWidget);

    await tester.ensureVisible(find.text('Settings'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    expect(find.byType(ProfileMenuModal), findsNothing);
  });

  testWidgets('tapping Profile dismisses the modal', (tester) async {
    await openModal(tester);
    expect(find.byType(ProfileMenuModal), findsOneWidget);

    await tester.tap(find.text('Profile'));
    // UserScreen pushes via NavigationHelpers and starts async data loads
    // that never settle inside the test environment. We only care that
    // the modal itself was dismissed, so step time forward enough for the
    // pop + push animations and stop.
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 400));

    expect(find.byType(ProfileMenuModal), findsNothing);
  });

  testWidgets('Profile row is a no-op when the user is not loaded', (
    tester,
  ) async {
    authNotifier = _TestAuthStateNotifier(AuthStateData.empty);

    await tester.pumpWidget(createHostApp());
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Profile'));
    await tester.pumpAndSettle();

    // Modal dismisses; no exception is thrown when userId is missing.
    expect(find.byType(ProfileMenuModal), findsNothing);
  });

  testWidgets(
      'opened from inside a nested Navigator, escapes onto the root '
      '(covers chrome painted outside that Navigator, e.g. the dock)', (
    tester,
  ) async {
    // Mirrors the Library tab's shape: HomeScreen's outer Stack paints
    // the persistent dock *after* (i.e. above) the tab body, and the
    // Library tab body is itself a nested Navigator. A modal opened
    // with the default useRootNavigator:false would push its route onto
    // that nested Navigator's own Overlay, which renders *inside* the
    // tab body — visually beneath the dock — leaving the dock on top of
    // the sheet instead of covered by it. useRootNavigator:true escapes
    // onto the app's root Navigator, whose Overlay sits above everything.
    final innerNavigatorKey = GlobalKey<NavigatorState>();

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authStateProvider.overrideWith(() => authNotifier),
          meSheetImpactProvider.overrideWith((ref) async => null),
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
            body: Navigator(
              key: innerNavigatorKey,
              onGenerateRoute: (settings) => MaterialPageRoute(
                builder: (innerContext) => Center(
                  child: ElevatedButton(
                    onPressed: () => ProfileMenuModal.show(innerContext),
                    child: const Text('open'),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );

    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();

    expect(find.byType(ProfileMenuModal), findsOneWidget);
    // The sheet's route was NOT pushed onto the tab's own nested
    // Navigator — its back stack is untouched.
    expect(innerNavigatorKey.currentState!.canPop(), isFalse);
  });
}
