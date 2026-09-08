import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_pitching_in_screen.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';

import '../../../helpers/l10n_helpers.dart';

const _id = 'exp-1';

class _FakeExp extends ExperienceNotifier {
  _FakeExp(super.experienceId, this._s);
  final ExperienceState _s;
  @override
  ExperienceState build() => _s;
}

class _FakeNeeds extends ExperienceNeedsNotifier {
  _FakeNeeds(super.experienceId, this._s);
  final ExperienceNeedsState _s;
  @override
  ExperienceNeedsState build() => _s;
}

ExperienceState _state({
  String currentUserId = 'viewer',
  pb.ExperienceState expState = pb.ExperienceState.EXPERIENCE_STATE_JOINED,
}) => ExperienceState(
  experienceId: _id,
  currentUserId: currentUserId,
  experienceDetails: GetExperienceResponse(
    experience: pb.Experience(
      id: _id,
      name: 'GoPro Mountain Games',
      owner: User(id: 'anna', name: 'Anna Reyes'),
      state: expState,
    ),
    rsvps: [
      RSVP(
        user: User(id: 'maya', name: 'Maya Lopez'),
        intention: RSVPIntention.RSVP_INTENTION_YES,
        rsvpedAtUnixSec: Int64(2000),
      ),
      RSVP(
        user: User(id: 'sam', name: 'Sam Reyes'),
        intention: RSVPIntention.RSVP_INTENTION_MAYBE,
        rsvpedAtUnixSec: Int64(1000),
      ),
      RSVP(
        user: User(id: 'wes', name: 'Wes Park'),
        intention: RSVPIntention.RSVP_INTENTION_NO,
        rsvpedAtUnixSec: Int64(500),
      ),
    ],
  ),
);

// State whose event is shared into a single origin (per-item) community with
// the given name (empty = nameless) and member count. Used to exercise the
// "Name this group" promote nudge (which needs a nameless origin, host viewer,
// and ≥2 members).
ExperienceState _stateWithOrigin({
  required String currentUserId,
  required String originName,
  int memberCount = 2,
}) => ExperienceState(
  experienceId: _id,
  currentUserId: currentUserId,
  experienceDetails: GetExperienceResponse(
    experience: pb.Experience(
      id: _id,
      name: 'GoPro Mountain Games',
      owner: User(id: 'anna', name: 'Anna Reyes'),
      state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
    ),
    sharedCommunities: [
      SharedCommunity(
        communityId: 'origin-1',
        communityName: originName,
        isOriginCommunity: true,
        memberCount: memberCount,
      ),
    ],
  ),
);

ExperienceNeedsState _needs() => ExperienceNeedsState(
  needs: [
    pb.ExperienceNeedResponse(
      id: 'n1',
      proposer: User(id: 'anna', name: 'Anna Reyes'),
      name: 'Camp stove',
      slots: 1,
      slotsRemaining: 1,
    ),
  ],
  contributions: [
    pb.ExperienceContributionResponse(
      id: 'c1',
      contributor: User(id: 'maya', name: 'Maya Lopez'),
      title: 'Folding chairs',
      description: 'Two of them, in my trunk.',
    ),
  ],
);

Future<void> _pump(
  WidgetTester tester, {
  ExperienceState? state,
  VoidCallback? onShowAccess,
}) async {
  // Tall viewport so the lazily-built ListView renders every roster section
  // (the role-aware prompt block pushes the lower groups down).
  tester.view.physicalSize = const Size(1200, 3000);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        experienceProvider(
          _id,
        ).overrideWith(() => _FakeExp(_id, state ?? _state())),
        experienceNeedsProvider(
          _id,
        ).overrideWith(() => _FakeNeeds(_id, _needs())),
      ],
      child: localizedApp(
        ExperiencePitchingInScreen(
          experienceId: _id,
          accentColor: const Color(0xFF7A9B8C),
          onShowAccess: onShowAccess ?? () {},
        ),
      ),
    ),
  );
  // Let the swipe-in slide animation settle so widgets are at rest and
  // hittable for tap().
  await tester.pumpAndSettle();
}

void main() {
  group('ExperiencePitchingInScreen', () {
    testWidgets('lists everyone in one flat roster (no per-status sections)', (
      tester,
    ) async {
      await _pump(tester);

      // Everyone is listed in a single roster — host, going, maybe, not going.
      expect(find.text('Anna Reyes'), findsOneWidget);
      expect(find.text('Maya Lopez'), findsOneWidget);
      expect(find.text('Sam Reyes'), findsOneWidget);
      expect(find.text('Wes Park'), findsOneWidget);

      // No per-status section headers — status now renders as a per-row pill.
      expect(find.text('GOING · 2'), findsNothing);
      expect(find.text('MAYBE · 1'), findsNothing);
      expect(find.text('NOT GOING · 1'), findsNothing);

      // Contributions still render under the person's row.
      expect(find.text('Folding chairs'), findsOneWidget);
    });

    testWidgets('host gets a tappable status pill on each attendee', (
      tester,
    ) async {
      // The host (Anna) can change anyone's RSVP, so each pill is tappable and
      // carries a "Change status, currently <state>" semantics label. Anna
      // (host) + Maya (yes) are Going; Sam is Maybe; Wes is Not going.
      await _pump(tester, state: _state(currentUserId: 'anna'));
      expect(
        find.bySemanticsLabel(RegExp('Change status.*Going')),
        findsNWidgets(2),
      );
      expect(
        find.bySemanticsLabel(RegExp('Change status.*Maybe')),
        findsOneWidget,
      );
      expect(
        find.bySemanticsLabel(RegExp('Change status.*Not going')),
        findsOneWidget,
      );
    });

    testWidgets("highlights the viewer's own row with a YOU tag", (
      tester,
    ) async {
      // The viewer is a participant (Maya), so her row carries the bordered
      // "YOU" badge (and a ringed avatar).
      await _pump(tester, state: _state(currentUserId: 'maya'));
      expect(find.text('YOU'), findsOneWidget);
      expect(find.text('Maya Lopez'), findsOneWidget);
    });

    testWidgets('lists open needs + the top action row', (tester) async {
      await _pump(tester);

      // One unclaimed need -> the "Needed · 1 open" header + the need chip.
      expect(find.text('NEEDED · 1 OPEN'), findsOneWidget);
      expect(find.text('Camp stove'), findsOneWidget);
      // The add affordances now live in the top action row.
      expect(find.text("I'll bring"), findsOneWidget);
      expect(find.text('We need'), findsOneWidget);
      expect(find.text('Invite'), findsOneWidget);
      // Maya's contribution shows under her row.
      expect(find.text('Folding chairs'), findsOneWidget);
    });

    testWidgets('the header overflow opens the access / invites sheet', (
      tester,
    ) async {
      var opened = 0;
      await _pump(tester, onShowAccess: () => opened++);

      await tester.tap(find.bySemanticsLabel("Manage who's invited"));
      await tester.pump();
      expect(opened, 1);
    });

    testWidgets('tapping a contribution pill opens the shared claim sheet', (
      tester,
    ) async {
      await _pump(tester);

      expect(find.text("I'll bring this"), findsNothing);

      // The viewer isn't bringing Maya's chairs -> claim mode, same shared
      // NeedsClaimConfirmSheet as the RSVP composer's chips.
      await tester.tap(find.text('Folding chairs'));
      await tester.pumpAndSettle();

      expect(find.text('PITCH IN'), findsOneWidget);
      expect(find.text("I'll bring this"), findsOneWidget);
    });

    testWidgets('a horizontal flick pops (closes) the panel', (tester) async {
      // Push the panel onto a navigator so there is something to pop back to.
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            experienceProvider(_id).overrideWith(() => _FakeExp(_id, _state())),
            experienceNeedsProvider(
              _id,
            ).overrideWith(() => _FakeNeeds(_id, _needs())),
          ],
          child: localizedApp(
            Builder(
              builder: (context) => Scaffold(
                body: Center(
                  child: ElevatedButton(
                    onPressed: () => Navigator.of(context).push(
                      MaterialPageRoute<void>(
                        builder: (_) => ExperiencePitchingInScreen(
                          experienceId: _id,
                          accentColor: const Color(0xFF7A9B8C),
                          onShowAccess: () {},
                        ),
                      ),
                    ),
                    child: const Text('open'),
                  ),
                ),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();
      expect(find.text('GoPro Mountain Games'), findsNothing); // home gone
      expect(find.text('Maya Lopez'), findsOneWidget); // panel shown

      // Flick horizontally → the panel pops.
      await tester.fling(find.text('Maya Lopez'), const Offset(400, 0), 1200);
      await tester.pumpAndSettle();
      expect(find.text('Maya Lopez'), findsNothing); // panel dismissed
      expect(find.text('open'), findsOneWidget); // back at home
    });

    group('wrapped (completed) event freezes the pane (#2724)', () {
      testWidgets('hides the We need / I\'ll bring / Invite action tiles', (
        tester,
      ) async {
        await _pump(
          tester,
          state: _state(
            expState: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
          ),
        );

        expect(find.text('We need'), findsNothing);
        expect(find.text("I'll bring"), findsNothing);
        expect(find.text('Invite'), findsNothing);
        // The roster itself still renders as a record.
        expect(find.text('Maya Lopez'), findsOneWidget);
      });

      testWidgets('open need chips render inert — no "+", no claim sheet', (
        tester,
      ) async {
        await _pump(
          tester,
          state: _state(
            expState: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
          ),
        );

        // The chip still tells the story…
        expect(find.text('Camp stove'), findsOneWidget);
        // …but carries no claim affordance (the action row's "+" icons are
        // gone with the tiles, so none remain anywhere).
        expect(find.byIcon(Icons.add_rounded), findsNothing);

        // Tapping it must NOT open the claim sheet.
        await tester.tap(find.text('Camp stove'));
        await tester.pumpAndSettle();
        expect(find.text('PITCH IN'), findsNothing);
        expect(find.text("I'll bring this"), findsNothing);
      });

      testWidgets('RSVP pills go read-only, even for the host', (
        tester,
      ) async {
        await _pump(
          tester,
          state: _state(
            currentUserId: 'anna',
            expState: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
          ),
        );

        // No manage affordance on any attendee's status pill.
        expect(
          find.bySemanticsLabel(RegExp('Change status')),
          findsNothing,
        );
        // And the viewer's own RSVP control is gone too.
        expect(find.text("You're invited — are you in?"), findsNothing);
      });

      testWidgets('a live event keeps all three action tiles', (tester) async {
        await _pump(tester);
        expect(find.text('We need'), findsOneWidget);
        expect(find.text("I'll bring"), findsOneWidget);
        expect(find.text('Invite'), findsOneWidget);
      });
    });

    testWidgets(
      'host sees the "Name this group" link when the origin community is nameless',
      (tester) async {
        await _pump(
          tester,
          state: _stateWithOrigin(currentUserId: 'anna', originName: ''),
        );
        expect(find.text('Name this group…'), findsOneWidget);
      },
    );

    testWidgets(
      'no "Name this group" link once the origin community has a name',
      (tester) async {
        await _pump(
          tester,
          state: _stateWithOrigin(
              currentUserId: 'anna', originName: 'Trail Crew'),
        );
        expect(find.text('Name this group…'), findsNothing);
      },
    );

    testWidgets(
      'a non-host does not see the "Name this group" link',
      (tester) async {
        await _pump(
          tester,
          state: _stateWithOrigin(currentUserId: 'viewer', originName: ''),
        );
        expect(find.text('Name this group…'), findsNothing);
      },
    );

    testWidgets(
      'no "Name this group" link until the per-item community has ≥2 members',
      (tester) async {
        // Host-only (just the host) → naming an audience of one is premature.
        await _pump(
          tester,
          state: _stateWithOrigin(
              currentUserId: 'anna', originName: '', memberCount: 1),
        );
        expect(find.text('Name this group…'), findsNothing);
      },
    );

    testWidgets(
      'a NAMED per-item community shows in the Communities section (not hidden)',
      (tester) async {
        // A nameless origin is hidden from the Communities section; once named,
        // it shows like any community, and the "Name this group" nudge is gone.
        await _pump(
          tester,
          state: _stateWithOrigin(
              currentUserId: 'anna', originName: 'Trail Crew'),
        );
        expect(find.text('Trail Crew'), findsOneWidget);
        expect(find.text('Name this group…'), findsNothing);
      },
    );
  });
}
