import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/community_avatar.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_switcher_dropdown.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers/auth_providers.dart'
    show authStateProvider;
import 'package:ripls/services/providers/community_providers.dart';

class _TestCommunitiesNotifier extends CommunitiesNotifier {
  _TestCommunitiesNotifier(this._communities);
  final List<CommunityItem> _communities;

  @override
  CommunitiesState build() => CommunitiesState(communities: _communities);
}

/// Auth-state override reporting a fixed signed-in user id, so the switcher can
/// tell which nameless communities the viewer owns (and may promote).
class _FakeAuthState extends AuthStateNotifier {
  _FakeAuthState(this._userId);
  final String _userId;

  @override
  AuthStateData build() =>
      AuthStateData(user: User(id: _userId), isLoading: false);
}

CommunityItem _community(
  String id,
  String name, {
  String ownerUserId = '',
  int memberCount = 0,
  List<String> preview = const [],
  String originItemName = '',
}) {
  return CommunityItem()
    ..id = id
    ..name = name
    ..ownerUserId = ownerUserId
    ..memberCount = memberCount
    ..originItemName = originItemName
    ..memberPreviewFirstNames.addAll(preview);
}

Widget _wrap(Widget child, ProviderContainer container) {
  return UncontrolledProviderScope(
    container: container,
    child: MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(body: child),
    ),
  );
}

WorkshopSwitcherDropdown _dropdown({void Function(String)? onSelect}) =>
    WorkshopSwitcherDropdown(
      query: '',
      onSelect: onSelect ?? (_) {},
      onCreate: () {},
    );

void main() {
  testWidgets(
    'tapping a nameless community row selects it (does not force-name)',
    (tester) async {
      String? selected;
      final container = ProviderContainer(
        overrides: [
          authStateProvider.overrideWith(() => _FakeAuthState('owner-1')),
          communitiesProvider.overrideWith(
            () => _TestCommunitiesNotifier([
              _community('c-2', '',
                  ownerUserId: 'owner-1', memberCount: 2, preview: ['Ada']),
            ]),
          ),
        ],
      );
      addTearDown(container.dispose);

      await tester.pumpWidget(
        _wrap(_dropdown(onSelect: (id) => selected = id), container),
      );
      await tester.pumpAndSettle();

      // No pencil / force-name affordance — naming moved to the overview pill.
      expect(find.byIcon(Icons.edit_outlined), findsNothing);
      // Tapping the nameless row just selects it, like any community.
      await tester.tap(find.text('You and Ada'));
      await tester.pumpAndSettle();
      expect(selected, 'c-2');
    },
  );

  testWidgets('tapping a named community row selects it', (tester) async {
    String? selected;
    final container = ProviderContainer(
      overrides: [
        authStateProvider.overrideWith(() => _FakeAuthState('owner-1')),
        communitiesProvider.overrideWith(
          () => _TestCommunitiesNotifier([
            _community('c-1', 'Boulder BC', ownerUserId: 'owner-1'),
          ]),
        ),
      ],
    );
    addTearDown(container.dispose);

    await tester.pumpWidget(
      _wrap(_dropdown(onSelect: (id) => selected = id), container),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Boulder BC'));
    await tester.pumpAndSettle();

    expect(selected, 'c-1');
  });

  testWidgets(
    'a nameless community renders a group-avatar cluster (viewer + preview faces)',
    (tester) async {
      final container = ProviderContainer(
        overrides: [
          authStateProvider.overrideWith(() => _FakeAuthState('owner-1')),
          communitiesProvider.overrideWith(
            () => _TestCommunitiesNotifier([
              _community('c-2', '',
                  ownerUserId: 'owner-1', memberCount: 3, preview: ['Ada', 'Sam']),
            ]),
          ),
        ],
      );
      addTearDown(container.dispose);

      await tester.pumpWidget(_wrap(_dropdown(), container));
      await tester.pumpAndSettle();

      // The nameless community is depicted as a cluster, not a single circle.
      expect(find.byType(GroupAvatar), findsOneWidget);
      expect(find.byType(CommunityAvatar), findsNothing);
      // Viewer + Ada + Sam = three member faces inside the cluster.
      expect(
        find.descendant(
          of: find.byType(GroupAvatar),
          matching: find.byType(UserAvatar),
        ),
        findsNWidgets(3),
      );
    },
  );

  testWidgets('a named community uses a single community avatar', (tester) async {
    final container = ProviderContainer(
      overrides: [
        authStateProvider.overrideWith(() => _FakeAuthState('owner-1')),
        communitiesProvider.overrideWith(
          () => _TestCommunitiesNotifier([
            _community('c-1', 'Boulder BC', ownerUserId: 'owner-1'),
          ]),
        ),
      ],
    );
    addTearDown(container.dispose);

    await tester.pumpWidget(_wrap(_dropdown(), container));
    await tester.pumpAndSettle();

    expect(find.byType(CommunityAvatar), findsOneWidget);
    expect(find.byType(GroupAvatar), findsNothing);
  });

  testWidgets(
    'a nameless community identifies itself by the item that spawned it',
    (tester) async {
      final container = ProviderContainer(
        overrides: [
          authStateProvider.overrideWith(() => _FakeAuthState('owner-1')),
          communitiesProvider.overrideWith(
            () => _TestCommunitiesNotifier([
              _community('c-2', '',
                  ownerUserId: 'owner-1',
                  memberCount: 2,
                  preview: ['Ada'],
                  originItemName: 'Dinner at Este'),
            ]),
          ),
        ],
      );
      addTearDown(container.dispose);

      await tester.pumpWidget(_wrap(_dropdown(), container));
      await tester.pumpAndSettle();

      // The row reads "from {item}" rather than a "Name this group" nudge.
      expect(find.text('from Dinner at Este'), findsOneWidget);
      expect(find.byIcon(Icons.edit_outlined), findsNothing);
    },
  );
}
