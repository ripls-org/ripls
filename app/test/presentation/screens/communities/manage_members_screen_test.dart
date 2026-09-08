import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/communities/manage_members_screen.dart';
import 'package:ripls/services/community_service.dart' show CommunityItem;
import 'package:ripls/services/providers.dart'
    show
        communityRepositoryProvider,
        communitiesProvider,
        provisionalUserRepositoryProvider,
        CommunitiesNotifier,
        CommunitiesState;

import '../../viewmodels/manage_members_view_model_test.mocks.dart';

// Stub notifier that returns a fixed CommunitiesState without
// any SharedPreferences or network activity.
class _StubCommunitiesNotifier extends CommunitiesNotifier {
  _StubCommunitiesNotifier(this._fixedState);
  final CommunitiesState _fixedState;

  @override
  CommunitiesState build() => _fixedState;
}

Widget _harness({
  required String communityId,
  required CommunitiesState communityState,
  required MockCommunityRepository mockCommunityRepo,
  required MockProvisionalUserRepository mockProvisionalRepo,
}) {
  return ProviderScope(
    overrides: [
      communitiesProvider.overrideWith(
        () => _StubCommunitiesNotifier(communityState),
      ),
      communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
      provisionalUserRepositoryProvider.overrideWithValue(mockProvisionalRepo),
    ],
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: ManageMembersScreen(communityId: communityId),
    ),
  );
}

void main() {
  const communityId = 'community-1';

  late MockCommunityRepository mockCommunityRepo;
  late MockProvisionalUserRepository mockProvisionalRepo;

  setUp(() {
    mockCommunityRepo = MockCommunityRepository();
    mockProvisionalRepo = MockProvisionalUserRepository();

    // Default stubs so load() resolves without error.
    when(
      mockCommunityRepo.listCommunityUsers(communityId),
    ).thenAnswer((_) async => []);
    when(
      mockProvisionalRepo.listProvisionalUsers(communityId),
    ).thenAnswer((_) async => []);
  });

  tearDown(() {
    reset(mockCommunityRepo);
    reset(mockProvisionalRepo);
  });

  group('ManageMembersScreen FAB (#2000)', () {
    testWidgets(
      'FAB is enabled when community is found by ID in communities list',
      (tester) async {
        final community = CommunityItem(
          id: communityId,
          name: 'Test Community',
        );
        final communityState = CommunitiesState(communities: [community]);

        await tester.pumpWidget(
          _harness(
            communityId: communityId,
            communityState: communityState,
            mockCommunityRepo: mockCommunityRepo,
            mockProvisionalRepo: mockProvisionalRepo,
          ),
        );
        await tester.pumpAndSettle();

        final fab = find.byType(FloatingActionButton);
        expect(fab, findsOneWidget);

        final fabWidget = tester.widget<FloatingActionButton>(fab);
        expect(
          fabWidget.onPressed,
          isNotNull,
          reason:
              'FAB must be enabled when the community is found by communityId '
              'in communitiesProvider.communities',
        );
      },
    );

    testWidgets(
      'FAB is disabled when community is absent from the communities list',
      (tester) async {
        // Graceful degrade: community not found → FAB stays disabled.
        final communityState = CommunitiesState(communities: const []);

        await tester.pumpWidget(
          _harness(
            communityId: communityId,
            communityState: communityState,
            mockCommunityRepo: mockCommunityRepo,
            mockProvisionalRepo: mockProvisionalRepo,
          ),
        );
        await tester.pumpAndSettle();

        final fab = find.byType(FloatingActionButton);
        expect(fab, findsOneWidget);

        final fabWidget = tester.widget<FloatingActionButton>(fab);
        expect(
          fabWidget.onPressed,
          isNull,
          reason:
              'FAB must stay disabled when the community cannot be resolved '
              'from the communities list',
        );
      },
    );
  });
}
