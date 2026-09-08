import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/communities/invite_sheet.dart';
import 'package:ripls/services/providers.dart';

import 'invite_sheet_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  late MockCommunityRepository mockRepository;

  const communityId = 'comm1';
  const communityName = 'Test Community';
  const firstShortCode = 'aaaaaaaa';
  const secondShortCode = 'bbbbbbbb';

  setUp(() {
    mockRepository = MockCommunityRepository();
  });

  Widget buildWidget({VoidCallback? onClose}) {
    return ProviderScope(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockRepository),
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
          body: InviteSheet(
            communityId: communityId,
            communityName: communityName,
            onClose: onClose ?? () {},
          ),
        ),
      ),
    );
  }

  group('InviteSheet — revoke', () {
    setUp(() {
      // Default stub: first call returns firstShortCode, second returns secondShortCode.
      var callCount = 0;
      when(mockRepository.getOrCreateShareLink(
        communityId: communityId,
        gearId: null,
        transferId: null,
        requestId: null,
        experienceId: null,
      )).thenAnswer((_) async {
        callCount++;
        final code = callCount == 1 ? firstShortCode : secondShortCode;
        return GetOrCreateShareLinkResponse(
          shareUrl: 'https://example.com/go/$code',
          shortCode: code,
          communityId: communityId,
          numMembers: 3,
          maxMembers: 32,
        );
      });

      when(mockRepository.revokeShareLink(
        communityId: communityId,
        gearId: anyNamed('gearId'),
        transferId: anyNamed('transferId'),
        requestId: anyNamed('requestId'),
        experienceId: anyNamed('experienceId'),
      )).thenAnswer((_) async {});
    });

    testWidgets('after revoke the sheet re-fetches and shows the new URL',
        (WidgetTester tester) async {
      await tester.pumpWidget(buildWidget());
      await tester.pumpAndSettle();

      // Initial render shows the first share URL.
      expect(find.textContaining(firstShortCode), findsOneWidget);

      // Tap Revoke and confirm.
      await tester.tap(find.text('Revoke'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Revoke').last);
      await tester.pumpAndSettle();

      // After revoke, _loadInviteLink() re-fetches and the URL updates.
      expect(find.textContaining(secondShortCode), findsOneWidget);
      expect(find.textContaining(firstShortCode), findsNothing);
    });

    testWidgets('revokeShareLink is called with communityId on revoke',
        (WidgetTester tester) async {
      await tester.pumpWidget(buildWidget());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Revoke'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Revoke').last);
      await tester.pumpAndSettle();

      verify(mockRepository.revokeShareLink(
        communityId: communityId,
        gearId: null,
        transferId: null,
        requestId: null,
        experienceId: null,
      )).called(1);
    });
  });
}
