import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart' as tpb;
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_read_shell.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/content/content_edge_avatar.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';

import '../../../helpers/l10n_helpers.dart';

const _id = 'exp-1';

/// Fake that returns a fixed [ExperienceState] from build(), bypassing the real
/// notifier's network load so the shell can be rendered in isolation.
class _FakeExperienceNotifier extends ExperienceNotifier {
  _FakeExperienceNotifier(super.experienceId, this._state);
  final ExperienceState _state;
  @override
  ExperienceState build() => _state;
}

class _FakeNeedsNotifier extends ExperienceNeedsNotifier {
  _FakeNeedsNotifier(super.experienceId);
  @override
  ExperienceNeedsState build() => const ExperienceNeedsState();
}

ExperienceState _state() {
  return ExperienceState(
    experienceId: _id,
    currentUserId: 'viewer',
    locationName: 'Vail Village',
    experienceDetails: GetExperienceResponse(
      experience: pb.Experience(
        id: _id,
        name: 'GoPro Mountain Games',
        description: 'Mountain sports, music, art.',
        owner: User(id: 'anna', name: 'Anna Reyes'),
        state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        conversationId: 'conv-1',
        messageCount: 3,
        unreadCount: 1,
      ),
      rsvps: [
        RSVP(
          user: User(id: 'maya', name: 'Maya Lopez'),
          intention: RSVPIntention.RSVP_INTENTION_YES,
        ),
      ],
    ),
  );
}

/// A completed (wrapped) experience with an AI recap.
ExperienceState _wrappedState() {
  return ExperienceState(
    experienceId: _id,
    currentUserId: 'viewer',
    locationName: 'Vail Village',
    completionSummary: 'A muddy, sunny weekend — everyone made it back safe.',
    experienceDetails: GetExperienceResponse(
      experience: pb.Experience(
        id: _id,
        name: 'GoPro Mountain Games',
        description: 'Mountain sports, music, art.',
        owner: User(id: 'anna', name: 'Anna Reyes'),
        state: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
        completedAtUnixSec: Int64(1700000000),
        conversationId: 'conv-1',
      ),
      rsvps: [
        RSVP(
          user: User(id: 'maya', name: 'Maya Lopez'),
          intention: RSVPIntention.RSVP_INTENTION_YES,
        ),
      ],
    ),
  );
}

/// An owner viewing their own event with neither time nor location set yet.
ExperienceState _ownerUnsetState() {
  return ExperienceState(
    experienceId: _id,
    currentUserId: 'anna',
    experienceDetails: GetExperienceResponse(
      experience: pb.Experience(
        id: _id,
        name: 'GoPro Mountain Games',
        owner: User(id: 'anna', name: 'Anna Reyes'),
        state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        conversationId: 'conv-1',
      ),
      // Sharing auto-RSVPs the owner, so the RSVP CTA should not show them
      // a reply they already have.
      rsvps: [
        RSVP(
          user: User(id: 'anna', name: 'Anna Reyes'),
          intention: RSVPIntention.RSVP_INTENTION_YES,
        ),
      ],
    ),
  );
}

/// More participants than the collapsed card shows, with the folded-away
/// remainder ([extraStatus]) sharing one status.
ExperienceState _overflowState({
  RSVPIntention extraStatus = RSVPIntention.RSVP_INTENTION_MAYBE,
}) {
  RSVP rsvp(String id, String name, RSVPIntention intention) =>
      RSVP(user: User(id: id, name: name), intention: intention);
  return ExperienceState(
    experienceId: _id,
    currentUserId: 'viewer',
    locationName: 'Vail Village',
    experienceDetails: GetExperienceResponse(
      experience: pb.Experience(
        id: _id,
        name: 'GoPro Mountain Games',
        owner: User(id: 'anna', name: 'Anna Reyes'),
        state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        conversationId: 'conv-1',
      ),
      // Anna (host) + these five = six rows; the card shows four, so the last
      // two fold into the overflow.
      rsvps: [
        rsvp('maya', 'Maya Lopez', RSVPIntention.RSVP_INTENTION_YES),
        rsvp('ben', 'Ben Ortiz', RSVPIntention.RSVP_INTENTION_YES),
        rsvp('cara', 'Cara Diaz', RSVPIntention.RSVP_INTENTION_YES),
        rsvp('dan', 'Dan Vega', extraStatus),
        rsvp('eve', 'Eve Chan', RSVPIntention.RSVP_INTENTION_MAYBE),
      ],
    ),
  );
}

/// A viewer who hasn't voted in a running time poll.
ExperienceState _timePollState() {
  return ExperienceState(
    experienceId: _id,
    currentUserId: 'viewer',
    locationName: 'Vail Village',
    experienceDetails: GetExperienceResponse(
      experience: pb.Experience(
        id: _id,
        name: 'GoPro Mountain Games',
        owner: User(id: 'anna', name: 'Anna Reyes'),
        state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        conversationId: 'conv-1',
        timePollActive: true,
        currentPollId: 'p1',
        timeProposals: [tpb.TimeProposal(id: 'tp1', pollId: 'p1')],
      ),
      rsvps: [
        RSVP(
          user: User(id: 'viewer', name: 'Vi Ewer'),
          intention: RSVPIntention.RSVP_INTENTION_YES,
        ),
      ],
    ),
  );
}

Future<void> _pump(
  WidgetTester tester, {
  ExperienceState? state,
  VoidCallback? onManage,
}) async {
  final s = state ?? _state();
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        experienceProvider(
          _id,
        ).overrideWith(() => _FakeExperienceNotifier(_id, s)),
        experienceNeedsProvider(
          _id,
        ).overrideWith(() => _FakeNeedsNotifier(_id)),
      ],
      child: localizedApp(
        ExperienceReadShell(
          experienceId: _id,
          accentColor: const Color(0xFF7A9B8C),
          onExpandConversation: (_) {},
          onShowTime: (_) {},
          onShowLocation: (_) {},
          onShowAccess: () {},
          onManage: onManage ?? () {},
        ),
      ),
    ),
  );
  await tester.pump();
}

void main() {
  group('ExperienceReadShell', () {
    testWidgets('renders headline, discussion, facts and edges', (
      tester,
    ) async {
      await _pump(tester);

      expect(find.text('GoPro Mountain Games'), findsOneWidget);
      // The description now reads as the discussion's opening quote.
      expect(find.text('"Mountain sports, music, art."'), findsOneWidget);
      expect(find.text('WHERE'), findsOneWidget);
      // The location surfaces only in the WHERE fact card now (the redundant
      // chip row was removed); the fact value is a RichText span.
      expect(find.text('Vail Village', findRichText: true), findsOneWidget);
      // The messages card became the discussion reply count (a RichText span
      // in the attribution line). Of the 3 messages one is the seeded
      // description shown as the quote itself, so 2 read as replies — a
      // fresh event otherwise claimed "1 reply" before anyone replied
      // (#2724).
      expect(find.textContaining('2 replies'), findsOneWidget);
      // Anna (owner) authors the opening comment AND appears as host in edges.
      expect(find.text('Anna Reyes'), findsWidgets);
      expect(find.text('Maya Lopez'), findsOneWidget);
    });

    testWidgets('shows the quiet "you haven\'t replied" status row', (
      tester,
    ) async {
      await _pump(tester);
      // The collapsed widget is quiet — the viewer's status, not a loud CTA.
      // The actual RSVP controls live in the expanded panel.
      expect(find.text("You haven't replied yet"), findsOneWidget);
    });

    testWidgets(
      'wrapped state shows the recap and status, hides the RSVP CTA',
      (tester) async {
        await _pump(tester, state: _wrappedState());

        // Terminal status pill + past-tense edge pills both read WRAPPED.
        expect(find.textContaining('WRAPPED'), findsWidgets);
        // The AI recap renders inline.
        expect(
          find.text('A muddy, sunny weekend — everyone made it back safe.'),
          findsOneWidget,
        );
        // No RSVP CTA or you-gap on a finished event.
        expect(find.text('RSVP'), findsNothing);
        expect(find.text('RSVP — are you in?'), findsNothing);
      },
    );

    testWidgets('shows the RSVP CTA when the viewer has not replied', (
      tester,
    ) async {
      await _pump(tester);
      expect(find.text('RSVP — are you in?'), findsOneWidget);
    });

    testWidgets(
      'who\'s-in header counts disjoint buckets: host+yes in, no-reply invited (#2724)',
      (tester) async {
        final s = _state();
        s.experienceDetails!.invitedIndividuals.addAll([
          User(id: 'gia', name: 'Gia'),
          User(id: 'rob', name: 'Rob'),
        ]);
        await _pump(tester, state: s);

        // Host (Anna) + Maya are in; Gia and Rob haven't replied. The host is
        // never counted as invited, and RSVP'd guests aren't either.
        expect(find.text('2 in · 2 invited'), findsOneWidget);
      },
    );

    testWidgets(
      'who\'s-in header drops the invited tail when nobody is awaiting reply',
      (tester) async {
        await _pump(tester);
        expect(find.text('2 in'), findsOneWidget);
        expect(find.textContaining('invited'), findsNothing);
      },
    );

    testWidgets('roster rows show faces, not identical check circles (#2724)', (
      tester,
    ) async {
      await _pump(tester);

      // Anna and Maya each get their own avatar. Neither has a photo in this
      // fixture, so the initials fallback stands in — the point is that the
      // row is keyed to the person, not to a shared status glyph.
      expect(find.byType(ContentEdgeAvatar), findsNWidgets(2));
      expect(find.text('AR'), findsOneWidget);
      expect(find.text('ML'), findsOneWidget);
    });

    testWidgets('overflow row names the hidden bucket when it is uniform', (
      tester,
    ) async {
      await _pump(tester, state: _overflowState());

      // Four rows shown (Anna, Maya, Ben, Cara); Dan and Eve are both maybes.
      expect(find.text('2 maybe ›'), findsOneWidget);
    });

    testWidgets('overflow row falls back to a plain count when mixed', (
      tester,
    ) async {
      await _pump(
        tester,
        state: _overflowState(extraStatus: RSVPIntention.RSVP_INTENTION_YES),
      );

      // Dan is going and Eve is a maybe — no single label is true, so the
      // row must not claim one.
      expect(find.text('2 more ›'), findsOneWidget);
    });

    testWidgets('owner with no time/location sees Set time / Set location', (
      tester,
    ) async {
      await _pump(tester, state: _ownerUnsetState());

      expect(find.text('Set time'), findsOneWidget);
      expect(find.text('Set location'), findsOneWidget);
      // Owner is already in, so no RSVP prompt.
      expect(find.text('RSVP — are you in?'), findsNothing);
    });

    testWidgets('owner sees the top-right manage overflow; tap opens it', (
      tester,
    ) async {
      var managed = 0;
      await _pump(
        tester,
        state: _ownerUnsetState(),
        onManage: () => managed++,
      );

      final overflow = find.byIcon(Icons.more_horiz);
      expect(overflow, findsOneWidget);
      await tester.tap(overflow);
      expect(managed, 1);
    });

    testWidgets('non-owner sees no manage overflow', (tester) async {
      await _pump(tester);
      expect(find.byIcon(Icons.more_horiz), findsNothing);
    });

    testWidgets('a running time poll the viewer has not voted in shows Vote', (
      tester,
    ) async {
      await _pump(tester, state: _timePollState());

      // The Vote CTA on the When card.
      expect(find.text('Vote'), findsOneWidget);
      // Location is set, so no Where CTA / no Set location.
      expect(find.text('Set location'), findsNothing);
    });
  });

  group('desktop caption column (#2912)', () {
    testWidgets('the sheet holds the centered measure at 1440x810',
        (tester) async {
      const surface = Size(1440, 810);
      tester.view.physicalSize = surface;
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);

      await _pump(tester);

      final wash = tester.getRect(find.byType(HeroContentWash));
      expect(wash.width, Responsive.contentMaxWidth);
      expect(wash.center.dx, moreOrLessEquals(surface.width / 2, epsilon: 1));
    });

    testWidgets('phone geometry is unchanged at 390x844', (tester) async {
      const surface = Size(390, 844);
      tester.view.physicalSize = surface;
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);

      await _pump(tester);

      expect(tester.getRect(find.byType(HeroContentWash)).width, surface.width);
    });
  });
}
