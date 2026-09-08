import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/discover/library_location_sheet.dart';
import 'package:ripls/services/providers.dart';

import '../../../presentation/viewmodels/search_view_model_test.mocks.dart';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

late MockSearchRepository _mockRepo;

/// Manual community notifier so the sheet can watch communitiesProvider
/// without hitting the network.
class _ManualCommunityNotifier extends CommunitiesNotifier {
  @override
  CommunitiesState build() => const CommunitiesState();
}

Widget _wrapSheet() {
  return ProviderScope(
    overrides: [
      searchRepositoryProvider.overrideWithValue(_mockRepo),
      communitiesProvider.overrideWith(_ManualCommunityNotifier.new),
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
            onPressed: () => LibraryLocationSheet.show(context),
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
    when(_mockRepo.invalidateAll()).thenAnswer((_) async {});
  });

  Future<void> openSheet(WidgetTester tester) async {
    await tester.pumpWidget(_wrapSheet());
    await tester.pump();
    await tester.tap(find.text('Open'));
    await tester.pumpAndSettle();
  }

  group('LibraryLocationSheet filters', () {
    testWidgets('renders the inline result-filter sections', (tester) async {
      await openSheet(tester);

      // The re-homed filter content lives directly on this sheet now.
      expect(find.text('SHOW ON MAP'), findsOneWidget);
      expect(find.text('Requests'), findsOneWidget);
      expect(find.text('Events'), findsOneWidget);
      expect(find.text('Sharing'), findsOneWidget);
      expect(find.text('Giving'), findsOneWidget);
      expect(find.text('Include Completed'), findsOneWidget);
    });

    testWidgets('does not render the dropped Distance / Sort / Map Style '
        'sections', (tester) async {
      await openSheet(tester);

      expect(find.text('DISTANCE'), findsNothing);
      expect(find.text('SORT BY'), findsNothing);
      expect(find.text('MAP STYLE'), findsNothing);
    });

    testWidgets('tapping a category chip toggles it and reveals Reset',
        (tester) async {
      await openSheet(tester);

      // No active filters yet → Reset hidden.
      expect(find.text('Reset'), findsNothing);

      // Deselecting a category makes activeFilterCount > 0.
      await tester.tap(find.text('Requests'));
      await tester.pumpAndSettle();

      expect(find.text('Reset'), findsOneWidget);
      expect(find.text('Requests'), findsOneWidget);
    });
  });
}
