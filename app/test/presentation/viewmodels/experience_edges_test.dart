import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/content/content_edge_data.dart';

ExperienceState _state({
  required pb.ExperienceState state,
  required User owner,
  List<RSVP> rsvps = const [],
  List<User> invitedIndividuals = const [],
  String? currentUserId,
  double? distanceMeters,
}) {
  return ExperienceState(
    currentUserId: currentUserId,
    locationDistanceMeters: distanceMeters,
    experienceDetails: GetExperienceResponse(
      experience: pb.Experience(state: state, owner: owner),
      rsvps: rsvps,
      invitedIndividuals: invitedIndividuals,
    ),
  );
}

void main() {
  final anna = User(id: 'anna', name: 'Anna Reyes');
  final maya = User(id: 'maya', name: 'Maya Lopez');
  final sam = User(id: 'sam', name: 'Sam');

  group('ExperienceState.edges', () {
    test('returns empty when there are no details', () {
      expect(const ExperienceState().edges(), isEmpty);
    });

    test('lists the host first, then yes/maybe, and skips declines', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        currentUserId: 'maya',
        rsvps: [
          RSVP(user: maya, intention: RSVPIntention.RSVP_INTENTION_YES),
          RSVP(user: sam, intention: RSVPIntention.RSVP_INTENTION_NO),
        ],
      );

      final edges = state.edges();
      expect(edges.map((e) => e.userId), ['anna', 'maya']);
      expect(edges[0].status, EdgeStatus.host);
      expect(edges[0].initials, 'AR');
      expect(edges[1].status, EdgeStatus.going);
      expect(edges[1].isYou, isTrue);
    });

    test('maps a maybe RSVP to the maybe status', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        owner: anna,
        rsvps: [
          RSVP(user: maya, intention: RSVPIntention.RSVP_INTENTION_MAYBE),
        ],
      );
      expect(state.edges().last.status, EdgeStatus.maybe);
    });

    test('does not duplicate the owner when they also have an RSVP', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        rsvps: [RSVP(user: anna, intention: RSVPIntention.RSVP_INTENTION_YES)],
      );
      expect(state.edges().where((e) => e.userId == 'anna'), hasLength(1));
    });

    test('marks every row wrapped once the event has wrapped', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
        owner: anna,
        rsvps: [RSVP(user: maya, intention: RSVPIntention.RSVP_INTENTION_YES)],
      );
      expect(
        state.edges().map((e) => e.status),
        everyElement(EdgeStatus.wrapped),
      );
    });

    test('injects contribution text by user id', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        rsvps: [RSVP(user: maya, intention: RSVPIntention.RSVP_INTENTION_YES)],
      );
      final edges = state.edges(
        contributionsByUserId: {'maya': 'folding chairs ×2'},
      );
      expect(
        edges.firstWhere((e) => e.userId == 'maya').contribution,
        'folding chairs ×2',
      );
      expect(edges.firstWhere((e) => e.userId == 'anna').contribution, '');
    });
  });

  group('ExperienceState.stockedSummaryFrom', () {
    final state = _state(
      state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
      owner: anna,
    );

    test('is empty (no bar) when there are no needs', () {
      final summary = state.stockedSummaryFrom(const []);
      expect(summary.hasNeeds, isFalse);
      expect(summary.total, 0);
      expect(summary.percent, 0);
      expect(summary.isFullyStocked, isFalse);
    });

    test('sums slots and slots_remaining across needs', () {
      final summary = state.stockedSummaryFrom([
        pb.ExperienceNeedResponse(slots: 2, slotsRemaining: 1),
        pb.ExperienceNeedResponse(slots: 10, slotsRemaining: 2),
      ]);
      expect(summary.total, 12);
      expect(summary.remaining, 3);
      expect(summary.covered, 9);
      expect(summary.percent, 75);
      expect(summary.hasNeeds, isTrue);
      expect(summary.isFullyStocked, isFalse);
    });

    test('reports fully stocked when nothing remains', () {
      final summary = state.stockedSummaryFrom([
        pb.ExperienceNeedResponse(slots: 3, slotsRemaining: 0),
      ]);
      expect(summary.isFullyStocked, isTrue);
      expect(summary.percent, 100);
    });
  });

  group('ExperienceState.rosterGroups', () {
    test('is empty when there are no details', () {
      expect(const ExperienceState().rosterGroups().isEmpty, isTrue);
    });

    test('groups host + yes into going, and buckets maybe and declines', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        currentUserId: 'maya',
        rsvps: [
          RSVP(
            user: maya,
            intention: RSVPIntention.RSVP_INTENTION_YES,
            rsvpedAtUnixSec: Int64(2000),
          ),
          RSVP(
            user: sam,
            intention: RSVPIntention.RSVP_INTENTION_MAYBE,
            rsvpedAtUnixSec: Int64(1000),
          ),
          RSVP(
            user: User(id: 'wes', name: 'Wes'),
            intention: RSVPIntention.RSVP_INTENTION_NO,
            rsvpedAtUnixSec: Int64(500),
          ),
        ],
      );

      final groups = state.rosterGroups();
      // Host pinned first in going, then the yes RSVP.
      expect(groups.going.map((e) => e.user.id), ['anna', 'maya']);
      // Host has no RSVP → going bucket; status follows that, isHost flags them.
      expect(groups.going.first.status, RosterStatus.going);
      expect(groups.going.first.isHost, isTrue);
      expect(groups.going.last.status, RosterStatus.going);
      expect(groups.going.last.isYou, isTrue);
      expect(groups.maybe.map((e) => e.user.id), ['sam']);
      expect(groups.notGoing.map((e) => e.user.id), ['wes']);
      // No client-side invitee source today.
      expect(groups.noReply, isEmpty);
    });

    test('orders within the going group most-recent-first after the host', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        rsvps: [
          RSVP(
            user: maya,
            intention: RSVPIntention.RSVP_INTENTION_YES,
            rsvpedAtUnixSec: Int64(1000),
          ),
          RSVP(
            user: sam,
            intention: RSVPIntention.RSVP_INTENTION_YES,
            rsvpedAtUnixSec: Int64(3000),
          ),
        ],
      );
      // Host first, then Sam (t=3000) before Maya (t=1000).
      expect(state.rosterGroups().going.map((e) => e.user.id), [
        'anna',
        'sam',
        'maya',
      ]);
    });

    test('does not duplicate the owner when they also have an RSVP', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        rsvps: [RSVP(user: anna, intention: RSVPIntention.RSVP_INTENTION_YES)],
      );
      final going = state.rosterGroups().going;
      expect(going.where((e) => e.user.id == 'anna'), hasLength(1));
      expect(going.single.status, RosterStatus.going);
      expect(going.single.isHost, isTrue);
    });

    test('moves the host to "not going" when they decline, keeping the flag', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        currentUserId: 'anna',
        rsvps: [RSVP(user: anna, intention: RSVPIntention.RSVP_INTENTION_NO)],
      );
      final groups = state.rosterGroups();
      expect(groups.going, isEmpty);
      expect(groups.notGoing.map((e) => e.user.id), ['anna']);
      expect(groups.notGoing.single.status, RosterStatus.notGoing);
      expect(groups.notGoing.single.isHost, isTrue);
      expect(groups.notGoing.single.isYou, isTrue);
    });

    test('puts a host who answered maybe in the maybe group', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        rsvps: [RSVP(user: anna, intention: RSVPIntention.RSVP_INTENTION_MAYBE)],
      );
      final groups = state.rosterGroups();
      expect(groups.going, isEmpty);
      expect(groups.maybe.map((e) => e.user.id), ['anna']);
      expect(groups.maybe.single.status, RosterStatus.maybe);
      expect(groups.maybe.single.isHost, isTrue);
    });

    test('puts server-supplied invited-no-reply users in the noReply group', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        currentUserId: 'sam',
        rsvps: [RSVP(user: maya, intention: RSVPIntention.RSVP_INTENTION_YES)],
        invitedIndividuals: [sam, User(id: 'wes', name: 'Wes')],
      );
      final groups = state.rosterGroups();
      expect(groups.noReply.map((e) => e.user.id), ['sam', 'wes']);
      expect(groups.noReply.first.status, RosterStatus.noReply);
      expect(groups.noReply.first.isYou, isTrue);
    });

    test('does not double-list a user in both rsvps and invited-no-reply', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        rsvps: [RSVP(user: maya, intention: RSVPIntention.RSVP_INTENTION_YES)],
        invitedIndividuals: [maya], // stale overlap
      );
      expect(state.rosterGroups().noReply, isEmpty);
      expect(state.rosterGroups().going.map((e) => e.user.id), ['anna', 'maya']);
    });
  });

  group('RosterGroups in/invited header counts (#2724)', () {
    test('going and invited are disjoint: host+yes in, no-reply invited', () {
      // Host + one yes + one maybe + one decline + two silent invitees.
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        rsvps: [
          RSVP(user: maya, intention: RSVPIntention.RSVP_INTENTION_YES),
          RSVP(user: sam, intention: RSVPIntention.RSVP_INTENTION_MAYBE),
          RSVP(
            user: User(id: 'wes', name: 'Wes'),
            intention: RSVPIntention.RSVP_INTENTION_NO,
          ),
        ],
        invitedIndividuals: [
          User(id: 'gia', name: 'Gia'),
          User(id: 'rob', name: 'Rob'),
        ],
      );
      final groups = state.rosterGroups();
      // "2 in": the host and Maya. Maybes and declines count in neither.
      expect(groups.goingCount, 2);
      // "2 invited": only the invitees still awaiting a reply.
      expect(groups.invitedNoReplyCount, 2);
    });

    test('the host is never counted as invited', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        // Even a stale overlap listing the host as an invitee is ignored.
        invitedIndividuals: [anna],
      );
      final groups = state.rosterGroups();
      expect(groups.goingCount, 1); // the host, default-going
      expect(groups.invitedNoReplyCount, 0);
    });

    test('a guest who RSVPs moves from invited to in — totals stay disjoint', () {
      final before = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        invitedIndividuals: [maya, sam],
      ).rosterGroups();
      expect(before.goingCount, 1); // host only ("1 in · 2 invited")
      expect(before.invitedNoReplyCount, 2);

      final after = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        owner: anna,
        rsvps: [RSVP(user: maya, intention: RSVPIntention.RSVP_INTENTION_YES)],
        // The server drops responders from invited_individuals; even if it
        // hasn't yet, the client de-dups.
        invitedIndividuals: [maya, sam],
      ).rosterGroups();
      expect(after.goingCount, 2); // "2 in · 1 invited"
      expect(after.invitedNoReplyCount, 1);
    });
  });

  group('ExperienceState.isFarAway', () {
    test('is false when distance is unknown', () {
      expect(const ExperienceState().isFarAway, isFalse);
    });

    test('is false within the threshold', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        owner: anna,
        distanceMeters: 10000,
      );
      expect(state.isFarAway, isFalse);
    });

    test('is true beyond the threshold', () {
      final state = _state(
        state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        owner: anna,
        distanceMeters: kFarAwayThresholdMeters + 1,
      );
      expect(state.isFarAway, isTrue);
    });
  });
}
