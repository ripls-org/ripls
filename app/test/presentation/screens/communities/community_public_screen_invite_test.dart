// The group page's Invite action. This screen is where a host lands from the
// People tab when they want to add someone to a group, and until now the only
// routes to InviteSheet hung off other surfaces (the feed's community card,
// the home header, Settings → Manage members) — so the test that matters is
// that the header action row offers Invite and that tapping it opens the
// sheet.

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart' show Item;
import 'package:ripls/presentation/screens/communities/community_public_screen.dart';
import 'package:ripls/presentation/screens/communities/invite_sheet.dart';
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart'
    show communityMembersProvider;
import 'package:ripls/presentation/viewmodels/community_impact_view_model.dart';
import 'package:ripls/presentation/viewmodels/profile_sheet_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

import '../../../helpers/l10n_helpers.dart';

const _communityId = 'comm-1';
const _communityName = 'Hump Day Milers';

/// The header's data comes from five independent reads. Only the community
/// itself needs a value here — the rest stay pending, which is the state the
/// screen already renders around, and keeps the test off the network.
class _PendingImpact extends CommunityImpactMetricsNotifier {
  _PendingImpact(super.communityId);
  @override
  Future<CommunityImpactData> build() => Completer<CommunityImpactData>().future;
}

class _PendingSheet extends CommunitySheetNotifier {
  _PendingSheet(super.communityId);
  @override
  Future<ProfileSheetState> build() => Completer<ProfileSheetState>().future;
}

Future<void> _pump(WidgetTester tester) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        // No mediaIds → the hero media read never happens.
        getCommunityProvider(_communityId).overrideWith(
          (ref) async => GetCommunityResponse(
            id: _communityId,
            name: _communityName,
          ),
        ),
        communityOpenItemsProvider(_communityId).overrideWith(
          (ref) async =>
              (items: <Item>[], specialties: <String>[], eventsCount: 0),
        ),
        communityImpactProvider(_communityId)
            .overrideWith(() => _PendingImpact(_communityId)),
        communitySheetProvider(_communityId)
            .overrideWith(() => _PendingSheet(_communityId)),
        communityMembersProvider(_communityId).overrideWith((ref) async => []),
      ],
      child: localizedApp(
        const CommunityPublicScreen(communityId: _communityId),
      ),
    ),
  );
  // Resolve getCommunityProvider so the body renders, then run out the
  // screen's slide-in (SwipeToCloseMixin) — until it finishes the header sits
  // a full screen width to the right and no tap can reach it.
  await tester.pump();
  await tester.pumpAndSettle();
}

Finder _inviteAction() => find.byWidgetPredicate(
      (w) => w is Tappable && w.semanticsLabel == 'Invite',
    );

void main() {
  group('CommunityPublicScreen — Invite', () {
    testWidgets('the header action row offers Invite', (tester) async {
      await _pump(tester);

      expect(_inviteAction(), findsOneWidget);
    });

    testWidgets('tapping Invite opens the group\'s invite sheet',
        (tester) async {
      await _pump(tester);
      expect(find.byType(InviteSheet), findsNothing);

      await tester.ensureVisible(_inviteAction());
      await tester.pump();
      await tester.tap(_inviteAction());
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 400));

      expect(find.byType(InviteSheet), findsOneWidget);
      // The sheet is opened for THIS group, not a blank one — a wrong id here
      // would silently mint an invite link into someone else's community.
      final sheet = tester.widget<InviteSheet>(find.byType(InviteSheet));
      expect(sheet.communityId, _communityId);
      expect(sheet.communityName, _communityName);
    });
  });
}
