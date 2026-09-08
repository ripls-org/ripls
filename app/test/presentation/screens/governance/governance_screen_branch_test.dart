import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/governance/governance_screen.dart';
import 'package:ripls/presentation/screens/governance/leave_community_screen.dart';
import 'package:ripls/presentation/screens/governance/owner_leave_member_picker_screen.dart';
import 'package:ripls/presentation/screens/governance/sole_member_leave_explainer_screen.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/providers.dart';

import 'governance_screen_branch_test.mocks.dart';

class _AuthStateOverride extends AuthStateNotifier {
  _AuthStateOverride(this._userId);
  final String _userId;

  @override
  AuthStateData build() {
    return AuthStateData(
      accessToken: 'test-token',
      user: User(id: _userId, name: 'Caller'),
      isLoading: false,
    );
  }
}

@GenerateMocks([CommunityRepository])
void main() {
  group('GovernanceScreen Leave branch detection', () {
    late MockCommunityRepository mockRepo;

    const callerId = 'me';
    const communityId = 'comm-1';

    setUp(() {
      mockRepo = MockCommunityRepository();
      when(mockRepo.invalidate(any)).thenAnswer((_) async {});
    });

    Widget pumpHarness({
      required CommunityItem community,
      required int numMembers,
    }) {
      return ProviderScope(
        overrides: [
          authStateProvider.overrideWith(() => _AuthStateOverride(callerId)),
          communityRepositoryProvider.overrideWithValue(mockRepo),
          communityProvider(community.id).overrideWith(
            (ref) async => GetCommunityResponse(numMembers: numMembers),
          ),
        ],
        child: MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: GovernanceScreen(community: community),
        ),
      );
    }

    Future<void> tapLeave(WidgetTester tester) async {
      // The Leave tile lives in a ListView; ensure it's on screen
      // before tapping (Settings hub renders many tiles vertically).
      final leaveFinder = find.text('Leave Community');
      await tester.ensureVisible(leaveFinder);
      await tester.tap(leaveFinder);
      // pump once to fire the route push + run any
      // post-frame callbacks. Avoid pumpAndSettle: the picker's
      // CircularProgressIndicator is a continuous animation that
      // never settles, and we only need the navigator to mount the
      // pushed screen — its inner state is out of scope here.
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
    }

    testWidgets(
      'sole-member owner taps Leave → SoleMemberLeaveExplainerScreen',
      (tester) async {
        final community = CommunityItem(
          id: communityId,
          name: 'Tide Pool',
          ownerUserId: callerId,
        );
        await tester.pumpWidget(
          pumpHarness(community: community, numMembers: 1),
        );
        await tester.pumpAndSettle();

        await tapLeave(tester);

        expect(find.byType(SoleMemberLeaveExplainerScreen), findsOneWidget);
        expect(find.byType(OwnerLeaveMemberPickerScreen), findsNothing);
        expect(find.byType(LeaveCommunityScreen), findsNothing);
      },
    );

    testWidgets(
      'sole-member non-owner taps Leave → SoleMemberLeaveExplainerScreen',
      (tester) async {
        final community = CommunityItem(
          id: communityId,
          name: 'Tide Pool',
          ownerUserId: 'someone-else',
        );
        await tester.pumpWidget(
          pumpHarness(community: community, numMembers: 1),
        );
        await tester.pumpAndSettle();

        await tapLeave(tester);

        expect(find.byType(SoleMemberLeaveExplainerScreen), findsOneWidget);
      },
    );

    testWidgets(
      'multi-member owner taps Leave → OwnerLeaveMemberPickerScreen',
      (tester) async {
        final community = CommunityItem(
          id: communityId,
          name: 'Tide Pool',
          ownerUserId: callerId,
        );
        await tester.pumpWidget(
          pumpHarness(community: community, numMembers: 2),
        );
        await tester.pumpAndSettle();

        await tapLeave(tester);

        expect(find.byType(OwnerLeaveMemberPickerScreen), findsOneWidget);
        expect(find.byType(SoleMemberLeaveExplainerScreen), findsNothing);
      },
    );

    testWidgets(
      'multi-member non-owner taps Leave → LeaveCommunityScreen',
      (tester) async {
        final community = CommunityItem(
          id: communityId,
          name: 'Tide Pool',
          ownerUserId: 'someone-else',
        );
        await tester.pumpWidget(
          pumpHarness(community: community, numMembers: 2),
        );
        await tester.pumpAndSettle();

        await tapLeave(tester);

        expect(find.byType(LeaveCommunityScreen), findsOneWidget);
        expect(find.byType(SoleMemberLeaveExplainerScreen), findsNothing);
      },
    );
  });
}
