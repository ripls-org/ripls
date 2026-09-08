import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/settings/restore_community_screen.dart';
import 'package:ripls/services/providers.dart';

import '../../viewmodels/restore_community_view_model_test.mocks.dart';

void main() {
  group('RestoreCommunityScreen', () {
    late MockCommunityRepository mockCommunity;
    late MockGearRepository mockGear;
    late MockRequestRepository mockRequest;
    late MockExperienceRepository mockExperience;
    late MockTransferRepository mockTransfer;
    late MockSearchRepository mockSearch;
    late MockFeedRepository mockFeed;
    late MockStoryRepository mockStory;
    late MockImpactMetricsRepository mockImpact;

    setUp(() {
      mockCommunity = MockCommunityRepository();
      mockGear = MockGearRepository();
      mockRequest = MockRequestRepository();
      mockExperience = MockExperienceRepository();
      mockTransfer = MockTransferRepository();
      mockSearch = MockSearchRepository();
      mockFeed = MockFeedRepository();
      mockStory = MockStoryRepository();
      mockImpact = MockImpactMetricsRepository();
    });

    Widget pumpHarness(Widget child) {
      return ProviderScope(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunity),
          gearRepositoryProvider.overrideWithValue(mockGear),
          requestRepositoryProvider.overrideWithValue(mockRequest),
          experienceRepositoryProvider.overrideWithValue(mockExperience),
          transferRepositoryProvider.overrideWithValue(mockTransfer),
          searchRepositoryProvider.overrideWithValue(mockSearch),
          feedRepositoryProvider.overrideWithValue(mockFeed),
          storyRepositoryProvider.overrideWithValue(mockStory),
          impactMetricsRepositoryProvider.overrideWithValue(mockImpact),
        ],
        child: MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: child,
        ),
      );
    }

    testWidgets('renders body with the community name and the CTA',
        (tester) async {
      await tester.pumpWidget(pumpHarness(
        const RestoreCommunityScreen(
          communityId: 'comm-1',
          communityName: 'Tide Pool',
        ),
      ));

      expect(
        find.text("You'll become the new owner of Tide Pool. "
            'Members will be re-added.'),
        findsOneWidget,
      );
      expect(find.text('Restore Community'), findsOneWidget);
      verifyNever(mockCommunity.restoreCommunity(any));
    });

    testWidgets('cancel via back button does not fire the restore RPC',
        (tester) async {
      await tester.pumpWidget(pumpHarness(
        const RestoreCommunityScreen(
          communityId: 'comm-1',
          communityName: 'Tide Pool',
        ),
      ));

      // Tap the AppBar back button (BackButtonIcon in the leading slot).
      // We tap the ancestor IconButton via tooltip so it's stable.
      await tester.tap(find.byTooltip('Back'));
      await tester.pumpAndSettle();

      verifyNever(mockCommunity.restoreCommunity(any));
    });
  });
}
