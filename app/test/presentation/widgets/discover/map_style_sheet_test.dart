import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/discover/map_style_sheet.dart';
import 'package:ripls/services/providers.dart';

import '../../../presentation/viewmodels/search_view_model_test.mocks.dart';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

late MockSearchRepository _mockRepo;

/// Builds a full ProviderScope that overrides the search repository so
/// MapStyleSheet can watch searchProvider without making network calls.
Widget _wrapSheet() {
  return ProviderScope(
    overrides: [
      searchRepositoryProvider.overrideWithValue(_mockRepo),
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
            onPressed: () => MapStyleSheet.show(context),
            child: const Text('Open'),
          ),
        ),
      ),
    ),
  );
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

void main() {
  setUp(() {
    _mockRepo = MockSearchRepository();
    when(
      _mockRepo.search(
        query: anyNamed('query'),
        communityIds: anyNamed('communityIds'),
        latitudeDeg: anyNamed('latitudeDeg'),
        longitudeDeg: anyNamed('longitudeDeg'),
        maxResults: anyNamed('maxResults'),
        includeCompleted: anyNamed('includeCompleted'),
      ),
    ).thenAnswer((_) async => []);
    when(
      _mockRepo.invalidateSearchesForCommunity(any),
    ).thenAnswer((_) async {});
    when(
      _mockRepo.invalidateAll(),
    ).thenAnswer((_) async {});
  });

  Future<void> openSheet(WidgetTester tester) async {
    await tester.pumpWidget(_wrapSheet());
    await tester.pump();
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
  }

  group('MapStyleSheet', () {
    testWidgets('renders the map-style title and all 4 options',
        (tester) async {
      await openSheet(tester);

      expect(find.text('Map Style'), findsOneWidget);
      expect(find.text('Default'), findsOneWidget);
      expect(find.text('Outdoor'), findsOneWidget);
      expect(find.text('Satellite'), findsOneWidget);
      expect(find.text('Dark'), findsOneWidget);
    });

    testWidgets('tapping a map style applies it and dismisses the sheet',
        (tester) async {
      await openSheet(tester);

      await tester.tap(find.text('Dark'));
      await tester.pumpAndSettle();

      // Selecting a style closes the sheet.
      expect(find.text('Map Style'), findsNothing);
    });

    testWidgets('close button dismisses the sheet', (tester) async {
      await openSheet(tester);

      await tester.tap(find.byIcon(Icons.close));
      await tester.pumpAndSettle();

      expect(find.text('Map Style'), findsNothing);
    });
  });
}
