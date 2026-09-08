import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart';
import 'package:ripls/services/community_service.dart' show ShareItemResult;
import 'package:ripls/services/providers.dart';

import '../../../core/observability/analytics_test_helper.dart';
import 'item_share_sheet_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  late MockCommunityRepository mockCommunityRepository;
  late MockObservabilityService mockObservabilityService;

  setUp(() {
    mockCommunityRepository = MockCommunityRepository();
    mockObservabilityService = MockObservabilityService();
  });

  // Helper to create test widget with mocked providers.
  Widget createTestWidget({
    required ShareableItemType itemType,
    String itemId = 'item123',
    String itemName = 'Test Item',
  }) {
    return ProviderScope(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
        observabilityServiceProvider.overrideWithValue(
          mockObservabilityService,
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
        home: Scaffold(
          body: ItemShareSheet(
            itemType: itemType,
            itemId: itemId,
            itemName: itemName,
          ),
        ),
      ),
    );
  }

  // The default success answer for shareItem (the load-time find-or-provision).
  // Defaults to the owner's perspective (canManageAudience: true); pass false
  // for a non-owner member who may only reshare the link (#2630).
  void stubShareItemSuccess({
    String shareUrl = 'https://example.com/go/abc12345?gear_id=item123',
    String communityId = 'adhoc123',
    bool canManageAudience = true,
  }) {
    when(mockCommunityRepository.shareItem(
      experienceId: anyNamed('experienceId'),
      gearId: anyNamed('gearId'),
      requestId: anyNamed('requestId'),
      invitees: anyNamed('invitees'),
      consent: anyNamed('consent'),
      shareToCommunityIds: anyNamed('shareToCommunityIds'),
    )).thenAnswer((_) async => ShareItemResult(
          adhocCommunityId: communityId,
          shareUrl: shareUrl,
          canManageAudience: canManageAudience,
        ));
  }

  group('ItemShareSheet - Loading State', () {
    testWidgets('displays loading indicator while preparing the link',
        (WidgetTester tester) async {
      stubShareItemSuccess();

      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      // Don't settle — observe the loading state before the async completes.

      expect(find.byType(CircularProgressIndicator), findsOneWidget);

      await tester.pumpAndSettle();
    });
  });

  group('ItemShareSheet - Success State', () {
    setUp(stubShareItemSuccess);

    testWidgets('displays QR code and link actions after successful load',
        (WidgetTester tester) async {
      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      await tester.pumpAndSettle();

      expect(find.byType(CircularProgressIndicator), findsNothing);
      expect(find.text('Share Test Item'), findsOneWidget);
      expect(find.text('Copy Link'), findsOneWidget);
      expect(find.text('Share'), findsOneWidget);
      // The per-entity link caption sits directly under the link — gear says
      // "see and claim", not "join" (#2724).
      expect(
        find.text('Anyone with the link can see and claim this item'),
        findsOneWidget,
      );
      // Revoke is removed (no per-link revoke affordance for now).
      expect(find.text('Revoke'), findsNothing);
    });

    testWidgets('offers both Invite people and Share to communities actions',
        (WidgetTester tester) async {
      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      await tester.pumpAndSettle();

      // The additive-invite redesign (#2492) offers inviting individual
      // members and adding whole communities side by side.
      expect(find.text('Invite people'), findsOneWidget);
      expect(find.text('Share to communities'), findsOneWidget);
    });

    testWidgets(
        'hides audience actions for a member who cannot manage the audience',
        (WidgetTester tester) async {
      stubShareItemSuccess(canManageAudience: false);

      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      await tester.pumpAndSettle();

      // A non-owner member still gets the full reshare surface (#2630)…
      expect(find.text('Share Test Item'), findsOneWidget);
      expect(find.text('Copy Link'), findsOneWidget);
      expect(find.text('Share'), findsOneWidget);
      expect(
        find.text('Anyone with the link can see and claim this item'),
        findsOneWidget,
      );
      // …but no audience-management rows.
      expect(find.text('Invite people'), findsNothing);
      expect(find.text('Share to communities'), findsNothing);
    });

    testWidgets('displays correct header for request',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        createTestWidget(
          itemType: ShareableItemType.request,
          itemName: 'Help Request',
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Share Help Request'), findsOneWidget);
    });

    testWidgets('displays correct header for experience',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        createTestWidget(
          itemType: ShareableItemType.experience,
          itemName: 'Community Event',
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Share Community Event'), findsOneWidget);
    });

    testWidgets('truncates long item names with ellipsis in header',
        (WidgetTester tester) async {
      const longName = 'This is a very long item name that should be truncated';
      await tester.pumpWidget(
        createTestWidget(
          itemType: ShareableItemType.gear,
          itemName: longName,
        ),
      );
      await tester.pumpAndSettle();

      final textFinder = find.text('Share $longName');
      expect(textFinder, findsOneWidget);

      final textWidget = tester.widget<Text>(textFinder);
      expect(textWidget.overflow, TextOverflow.ellipsis);
      expect(textWidget.maxLines, 1);
    });

    testWidgets('displays truncated URL', (WidgetTester tester) async {
      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      await tester.pumpAndSettle();

      expect(find.textContaining('example.com/go/abc12345?...'), findsOneWidget);
    });

    testWidgets('logs analytics event on successful load',
        (WidgetTester tester) async {
      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      await tester.pumpAndSettle();

      expect(mockObservabilityService.loggedEvents, hasLength(1));
      expect(
        mockObservabilityService.loggedEvents.first,
        isA<ItemShareQrDisplayedEvent>(),
      );
      final event =
          mockObservabilityService.loggedEvents.first as ItemShareQrDisplayedEvent;
      expect(event.communityId, 'adhoc123');
      expect(event.itemType, 'gear');
      expect(event.itemId, 'item123');
    });

    testWidgets('calls shareItem with gearId for gear',
        (WidgetTester tester) async {
      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      await tester.pumpAndSettle();

      verify(mockCommunityRepository.shareItem(
        experienceId: null,
        gearId: 'item123',
        requestId: null,
        invitees: anyNamed('invitees'),
        consent: anyNamed('consent'),
        shareToCommunityIds: anyNamed('shareToCommunityIds'),
      )).called(1);
    });

    testWidgets('calls shareItem with requestId for request',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        createTestWidget(itemType: ShareableItemType.request),
      );
      await tester.pumpAndSettle();

      verify(mockCommunityRepository.shareItem(
        experienceId: null,
        gearId: null,
        requestId: 'item123',
        invitees: anyNamed('invitees'),
        consent: anyNamed('consent'),
        shareToCommunityIds: anyNamed('shareToCommunityIds'),
      )).called(1);
    });

    testWidgets('calls shareItem with experienceId for experience',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        createTestWidget(itemType: ShareableItemType.experience),
      );
      await tester.pumpAndSettle();

      verify(mockCommunityRepository.shareItem(
        experienceId: 'item123',
        gearId: null,
        requestId: null,
        invitees: anyNamed('invitees'),
        consent: anyNamed('consent'),
        shareToCommunityIds: anyNamed('shareToCommunityIds'),
      )).called(1);
    });
  });

  group('ItemShareSheet - Error State', () {
    testWidgets('displays error message when load fails',
        (WidgetTester tester) async {
      when(mockCommunityRepository.shareItem(
        experienceId: anyNamed('experienceId'),
        gearId: anyNamed('gearId'),
        requestId: anyNamed('requestId'),
        invitees: anyNamed('invitees'),
        consent: anyNamed('consent'),
        shareToCommunityIds: anyNamed('shareToCommunityIds'),
      )).thenThrow(Exception('Network error'));

      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      await tester.pumpAndSettle();

      // The error is classified via RpcErrorHandler.classify; a generic
      // Exception lands in the UserError.generic bucket → rpcErrorGeneric.
      expect(find.byIcon(Icons.error_outline), findsOneWidget);
      expect(
        find.text('Something went wrong. Please try again.'),
        findsOneWidget,
      );
      expect(find.text('Retry'), findsOneWidget);
    });

    testWidgets('retry button re-prepares the link',
        (WidgetTester tester) async {
      var callCount = 0;
      when(mockCommunityRepository.shareItem(
        experienceId: anyNamed('experienceId'),
        gearId: anyNamed('gearId'),
        requestId: anyNamed('requestId'),
        invitees: anyNamed('invitees'),
        consent: anyNamed('consent'),
        shareToCommunityIds: anyNamed('shareToCommunityIds'),
      )).thenAnswer((_) async {
        callCount++;
        if (callCount == 1) {
          throw Exception('Network error');
        }
        return const ShareItemResult(
          adhocCommunityId: 'adhoc123',
          shareUrl: 'https://example.com/go/abc12345',
        );
      });

      await tester
          .pumpWidget(createTestWidget(itemType: ShareableItemType.gear));
      await tester.pumpAndSettle();

      expect(find.text('Retry'), findsOneWidget);

      await tester.tap(find.text('Retry'));
      await tester.pumpAndSettle();

      expect(find.text('Share Test Item'), findsOneWidget);
      expect(find.text('Copy Link'), findsOneWidget);
      expect(callCount, 2);
    });
  });

  group('ItemShareSheet - Close Button', () {
    setUp(() => stubShareItemSuccess(shareUrl: 'https://example.com/go/abc12345'));

    testWidgets('pops the sheet when the close icon is tapped',
        (WidgetTester tester) async {
      final navigatorKey = GlobalKey<NavigatorState>();

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            communityRepositoryProvider
                .overrideWithValue(mockCommunityRepository),
            observabilityServiceProvider
                .overrideWithValue(mockObservabilityService),
          ],
          child: MaterialApp(
            navigatorKey: navigatorKey,
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
                  onPressed: () => ItemShareSheet.show(
                    context,
                    itemType: ShareableItemType.gear,
                    itemId: 'item123',
                    itemName: 'Test Item',
                  ),
                  child: const Text('open'),
                ),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      expect(find.text('Share Test Item'), findsOneWidget);

      await tester.tap(find.byIcon(Icons.close));
      await tester.pumpAndSettle();

      // Sheet is dismissed.
      expect(find.text('Share Test Item'), findsNothing);
    });
  });
}
