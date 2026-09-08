import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/governance/owner_leave_member_picker_screen.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/providers.dart';

import 'owner_leave_member_picker_screen_test.mocks.dart';

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

@GenerateMocks([CommunityService])
void main() {
  group('OwnerLeaveMemberPickerScreen', () {
    late MockCommunityService mockService;

    CommunityMember member(String id, String name) {
      return CommunityMember(
        user: User(id: id, name: name),
        joinedAtUnixSec: Int64(0),
      );
    }

    setUp(() {
      mockService = MockCommunityService();
    });

    Widget pumpHarness({required String callerId, required Widget child}) {
      return ProviderScope(
        overrides: [
          communityServiceProvider.overrideWithValue(mockService),
          authStateProvider.overrideWith(
            () => _AuthStateOverride(callerId),
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
          home: child,
        ),
      );
    }

    testWidgets('filters out the caller and renders the rest of the members',
        (tester) async {
      when(mockService.listCommunityUsers('comm-1')).thenAnswer(
        (_) async => [
          member('me', 'Caller'),
          member('alice', 'Alice'),
          member('bob', 'Bob'),
        ],
      );

      await tester.pumpWidget(pumpHarness(
        callerId: 'me',
        child: OwnerLeaveMemberPickerScreen(
          community: CommunityItem(id: 'comm-1', name: 'Tide Pool'),
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.text('Alice'), findsOneWidget);
      expect(find.text('Bob'), findsOneWidget);
      expect(find.text('Caller'), findsNothing);
    });

    testWidgets('renders sole-member empty state when only the caller exists',
        (tester) async {
      when(mockService.listCommunityUsers('comm-1')).thenAnswer(
        (_) async => [member('me', 'Caller')],
      );

      await tester.pumpWidget(pumpHarness(
        callerId: 'me',
        child: OwnerLeaveMemberPickerScreen(
          community: CommunityItem(id: 'comm-1', name: 'Tide Pool'),
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.textContaining('only member'), findsOneWidget);
    });

    testWidgets('renders error state on listCommunityUsers failure',
        (tester) async {
      when(mockService.listCommunityUsers('comm-1'))
          .thenThrow(Exception('network'));

      await tester.pumpWidget(pumpHarness(
        callerId: 'me',
        child: OwnerLeaveMemberPickerScreen(
          community: CommunityItem(id: 'comm-1', name: 'Tide Pool'),
        ),
      ));
      await tester.pumpAndSettle();

      expect(find.textContaining("Couldn't load"), findsOneWidget);
    });
  });
}
