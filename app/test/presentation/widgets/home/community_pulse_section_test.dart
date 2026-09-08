import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/users/user_screen.dart';
import 'package:ripls/presentation/viewmodels/feed_view_model.dart';
import 'package:ripls/presentation/viewmodels/profile_sheet_view_model.dart';
import 'package:ripls/presentation/viewmodels/viewer_profile_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/home/community_pulse_section.dart';
import 'package:ripls/services/providers.dart'
    show communitiesProvider, CommunitiesNotifier, CommunitiesState;

/// Feed notifier seeded with fixed items; never touches the repository.
class _SeededFeedNotifier extends FeedNotifier {
  _SeededFeedNotifier(this._items);

  final List<FeedItem> _items;

  @override
  FeedState build() => FeedState(items: _items);
}

class _SeededCommunitiesNotifier extends CommunitiesNotifier {
  @override
  CommunitiesState build() => CommunitiesState(
    communities: [CommunityItem(id: 'c1', name: 'Maple Street')],
  );
}

/// Profile stubs so the who-strip's pushed [UserScreen] renders without
/// repositories.
class _StubProfile extends ViewerProfileNotifier {
  _StubProfile(super.targetUserId);

  @override
  Future<ViewerProfileState> build() async =>
      ViewerProfileState(targetUserId: targetUserId, targetName: 'Carmen Ruiz');
}

class _StubSheet extends ProfileSheetNotifier {
  _StubSheet(super.targetUserId);

  @override
  Future<ProfileSheetState> build() async => const QuietSheet();
}

void main() {
  FeedItem gearItem(String id, String name, {bool unread = false}) => FeedItem(
    id: id,
    itemType: FeedItemType.FEED_ITEM_TYPE_GEAR_SHARED,
    isUnread: unread,
    gearShared: GearSharedPayload(
      gearId: 'gear-$id',
      gearName: name,
      actor: User(id: 'u1', name: 'Carmen Ruiz'),
    ),
  );

  FeedItem requestItem(
    String id,
    String title, {
    bool? viewerHasOffered,
    bool canEdit = false,
  }) => FeedItem(
    id: id,
    itemType: FeedItemType.FEED_ITEM_TYPE_REQUEST_CREATED,
    requestCreated: RequestCreatedPayload(
      requestId: 'req-$id',
      title: title,
      requester: User(id: 'u2', name: 'Gary Soto'),
      viewerHasOffered: viewerHasOffered,
      canEdit: canEdit,
    ),
  );

  FeedItem experienceItem(
    String id,
    String name, {
    FeedRSVPIntention? viewerRsvp,
    bool canEdit = false,
  }) => FeedItem(
    id: id,
    itemType: FeedItemType.FEED_ITEM_TYPE_EXPERIENCE_CREATED,
    experienceCreated: ExperienceCreatedPayload(
      experienceId: 'exp-$id',
      name: name,
      creator: User(id: 'u3', name: 'Betty Cho'),
      viewerRsvp: viewerRsvp,
      canEdit: canEdit,
    ),
  );

  FeedItem nudgeItem(String id) => FeedItem(
    id: id,
    itemType: FeedItemType.FEED_ITEM_TYPE_NUDGE,
    nudge: NudgePayload(nudgeId: 'n-$id'),
  );

  Widget host(List<FeedItem> items) {
    return ProviderScope(
      overrides: [
        feedProvider.overrideWith(() => _SeededFeedNotifier(items)),
        communitiesProvider.overrideWith(_SeededCommunitiesNotifier.new),
        viewerProfileProvider.overrideWith2(_StubProfile.new),
        profileSheetProvider.overrideWith2(_StubSheet.new),
      ],
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: const Scaffold(
          body: SingleChildScrollView(child: CommunityPulseSection()),
        ),
      ),
    );
  }

  testWidgets('renders posts for gear, asks, and events with kind tags', (
    tester,
  ) async {
    await tester.pumpWidget(
      host([
        gearItem('1', 'Extension ladder'),
        requestItem('2', 'Need a stand mixer'),
        experienceItem('3', 'Walk under the redwoods'),
      ]),
    );
    await tester.pump();

    expect(find.text('IN YOUR COMMUNITIES'), findsOneWidget);
    expect(find.text('Extension ladder'), findsOneWidget);
    expect(find.text('Need a stand mixer'), findsOneWidget);
    expect(find.text('Walk under the redwoods'), findsOneWidget);
    // v3 post headers: the owner fronts gear, the asker fronts the
    // request ("people ask"), the host fronts the event.
    expect(find.text('Carmen Ruiz'), findsOneWidget);
    expect(find.text('Gary Soto asks'), findsOneWidget);
    expect(find.text('Betty Cho'), findsOneWidget);
    // Header-corner CTAs replace the kind tags: asks carry the offer
    // CTA, events the Join CTA. Neither viewer-state field is set here,
    // so both cards still invite the viewer to act.
    expect(find.text('I have one'), findsOneWidget);
    expect(find.text('Join'), findsOneWidget);
  });

  group('event CTA reflects the viewer', () {
    // #2800: the card counted the viewer among "3 going" and still asked
    // them to join.
    testWidgets('says Going when the viewer said yes', (tester) async {
      await tester.pumpWidget(
        host([
          experienceItem(
            '1',
            'Love Your Neighbor 5K',
            viewerRsvp: FeedRSVPIntention.FEED_RSVP_INTENTION_YES,
          ),
        ]),
      );
      await tester.pump();

      expect(find.text('Going'), findsOneWidget);
      expect(find.text('Join'), findsNothing);
    });

    testWidgets('says Maybe when the viewer is tentative', (tester) async {
      await tester.pumpWidget(
        host([
          experienceItem(
            '1',
            'Love Your Neighbor 5K',
            viewerRsvp: FeedRSVPIntention.FEED_RSVP_INTENTION_MAYBE,
          ),
        ]),
      );
      await tester.pump();

      expect(find.text('Maybe'), findsOneWidget);
      expect(find.text('Join'), findsNothing);
    });

    testWidgets('says Not going when the viewer declined — the card stays '
        'so they can change their mind', (tester) async {
      await tester.pumpWidget(
        host([
          experienceItem(
            '1',
            'Love Your Neighbor 5K',
            viewerRsvp: FeedRSVPIntention.FEED_RSVP_INTENTION_NO,
          ),
        ]),
      );
      await tester.pump();

      expect(find.text('Love Your Neighbor 5K'), findsOneWidget);
      expect(find.text('Not going'), findsOneWidget);
      expect(find.text('Join'), findsNothing);
    });

    testWidgets('says Hosting for the host, whatever their RSVP', (
      tester,
    ) async {
      await tester.pumpWidget(
        host([
          experienceItem(
            '1',
            'Love Your Neighbor 5K',
            canEdit: true,
            viewerRsvp: FeedRSVPIntention.FEED_RSVP_INTENTION_YES,
          ),
        ]),
      );
      await tester.pump();

      expect(find.text('Hosting'), findsOneWidget);
      expect(find.text('Going'), findsNothing);
    });

    testWidgets('falls back to Join when the RSVP is unknown', (tester) async {
      // An absent viewer_rsvp means the server could not tell — which is
      // not the same as "no", so the card must not assert either status.
      await tester.pumpWidget(
        host([experienceItem('1', 'Love Your Neighbor 5K')]),
      );
      await tester.pump();

      expect(find.text('Join'), findsOneWidget);
    });
  });

  group('request CTA reflects the viewer', () {
    testWidgets('says Offered when the viewer already offered', (tester) async {
      await tester.pumpWidget(
        host([requestItem('1', 'Need a stand mixer', viewerHasOffered: true)]),
      );
      await tester.pump();

      expect(find.text('Offered'), findsOneWidget);
      expect(find.text('I have one'), findsNothing);
    });

    testWidgets('says Your request on the viewer\'s own ask', (tester) async {
      await tester.pumpWidget(
        host([requestItem('1', 'Need a stand mixer', canEdit: true)]),
      );
      await tester.pump();

      expect(find.text('Your request'), findsOneWidget);
      expect(find.text('I have one'), findsNothing);
    });

    testWidgets('still offers when the viewer has not offered', (tester) async {
      await tester.pumpWidget(
        host([requestItem('1', 'Need a stand mixer', viewerHasOffered: false)]),
      );
      await tester.pump();

      expect(find.text('I have one'), findsOneWidget);
    });

    testWidgets('falls back to the offer CTA when offer state is unknown', (
      tester,
    ) async {
      await tester.pumpWidget(host([requestItem('1', 'Need a stand mixer')]));
      await tester.pump();

      expect(find.text('I have one'), findsOneWidget);
    });
  });

  testWidgets('a status chip is not a tap target, but the card still opens '
      'the item', (tester) async {
    // A chip reading "Going" that announces as a button named "Going"
    // promises an action it does not perform, so the status variant is
    // rendered outside any Tappable.
    await tester.pumpWidget(
      host([
        experienceItem(
          '1',
          'Love Your Neighbor 5K',
          viewerRsvp: FeedRSVPIntention.FEED_RSVP_INTENTION_YES,
        ),
      ]),
    );
    await tester.pump();

    expect(
      find.ancestor(of: find.text('Going'), matching: find.byType(Tappable)),
      findsNothing,
      reason: 'the status chip must not sit inside a tap target',
    );
    // The snapshot/caption target is still there and still opens the item.
    expect(
      find.ancestor(
        of: find.text('Love Your Neighbor 5K'),
        matching: find.byType(Tappable),
      ),
      findsOneWidget,
    );
  });

  testWidgets('renders every loaded item and skips system items', (
    tester,
  ) async {
    await tester.pumpWidget(
      host([
        nudgeItem('n1'), // system item — never a card
        for (var i = 0; i < 8; i++) gearItem('$i', 'Gear $i'),
      ]),
    );
    await tester.pump();

    // The pulse is the Home scroll's long tail: no cap, all loaded feed
    // items become cards.
    for (var i = 0; i < 8; i++) {
      expect(
        find.text('Gear $i'),
        findsOneWidget,
        reason: 'Gear $i should render',
      );
    }
  });

  testWidgets('renders nothing when the feed has no card-able items', (
    tester,
  ) async {
    await tester.pumpWidget(host([nudgeItem('n1')]));
    await tester.pump();

    expect(find.text('IN YOUR COMMUNITIES'), findsNothing);
  });

  testWidgets('tapping the who-strip (avatar + name) opens the person profile, '
      'not the item', (tester) async {
    await tester.pumpWidget(host([gearItem('1', 'Extension ladder')]));
    await tester.pump();

    await tester.tap(find.text('Carmen Ruiz'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 16));
    await tester.pumpAndSettle();

    expect(find.byType(UserScreen), findsOneWidget);
  });
}
