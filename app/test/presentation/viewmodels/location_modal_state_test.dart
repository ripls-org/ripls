import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/location_modal_state.dart';

User _user(String id) => User()..id = id;

LocationVote _vote(String userId, LocationVoteStatus status) => LocationVote()
  ..user = _user(userId)
  ..status = status;

LocationProposal _proposal(
  String id,
  String pollId,
  List<LocationVote> votes,
) =>
    LocationProposal()
      ..id = id
      ..pollId = pollId
      ..votes.addAll(votes);

void main() {
  const yes = LocationVoteStatus.LOCATION_VOTE_STATUS_YES;
  const flexible = LocationVoteStatus.LOCATION_VOTE_STATUS_FLEXIBLE;

  LocationModalData buildState(List<LocationProposal> proposals,
          {String? currentUserId = 'me', String pollId = 'poll-1'}) =>
      LocationModalData(
        proposals: proposals,
        currentLocationPollId: pollId,
        currentUserId: currentUserId,
        locationPollActive: true,
      );

  group('currentPollProposals', () {
    test('scopes to the active poll id', () {
      final state = buildState([
        _proposal('a', 'poll-1', []),
        _proposal('old', 'poll-0', []),
        _proposal('b', 'poll-1', []),
      ]);
      expect(
        state.currentPollProposals.map((p) => p.id),
        ['a', 'b'],
      );
    });
  });

  group('flexible folding', () {
    test('a flexible voter counts toward every proposal', () {
      // Sam YES on A only; Priya flexible (stored on A as the representative).
      final state = buildState([
        _proposal('a', 'poll-1', [
          _vote('sam', yes),
          _vote('priya', flexible),
        ]),
        _proposal('b', 'poll-1', []),
      ]);

      // A: Sam (yes) + Priya (flexible) = 2.
      expect(state.effectiveVoteCount('a'), 2);
      // B: no explicit votes, but Priya folds in = 1.
      expect(state.effectiveVoteCount('b'), 1);
      expect(state.currentPollFlexibleVoterIds, {'priya'});
    });

    test('does not double-count a user who is both YES and flexible', () {
      final state = buildState([
        _proposal('a', 'poll-1', [
          _vote('sam', yes),
          _vote('sam', flexible),
        ]),
      ]);
      // Sam appears once despite two vote rows.
      expect(state.effectiveVoteCount('a'), 1);
    });
  });

  group('leadingProposalId', () {
    test('picks the proposal with the most effective support', () {
      final state = buildState([
        _proposal('a', 'poll-1', [_vote('sam', yes)]),
        _proposal('b', 'poll-1', [
          _vote('sam', yes),
          _vote('jess', yes),
        ]),
      ]);
      expect(state.leadingProposalId, 'b');
    });

    test('is null when no proposal has any support', () {
      final state = buildState([
        _proposal('a', 'poll-1', []),
        _proposal('b', 'poll-1', []),
      ]);
      expect(state.leadingProposalId, isNull);
    });

    test('ties resolve to the earliest-listed proposal', () {
      final state = buildState([
        _proposal('a', 'poll-1', [_vote('sam', yes)]),
        _proposal('b', 'poll-1', [_vote('jess', yes)]),
      ]);
      expect(state.leadingProposalId, 'a');
    });
  });

  group('currentUserIsFlexible', () {
    test('true when the current user has a flexible vote', () {
      final state = buildState([
        _proposal('a', 'poll-1', [_vote('me', flexible)]),
      ]);
      expect(state.currentUserIsFlexible, isTrue);
    });

    test('false when the current user only cast a YES', () {
      final state = buildState([
        _proposal('a', 'poll-1', [_vote('me', yes)]),
      ]);
      expect(state.currentUserIsFlexible, isFalse);
    });
  });

  group('pollStatus', () {
    test('is poll while a poll is active', () {
      final state = buildState([_proposal('a', 'poll-1', [])]);
      expect(state.pollStatus, 'poll');
    });

    test('is set when a location is confirmed and no poll runs', () {
      const state = LocationModalData(
        eventLocationId: 'loc-1',
        locationPollActive: false,
      );
      expect(state.pollStatus, 'set');
    });

    test('is tbd with no value and no poll', () {
      const state = LocationModalData(locationPollActive: false);
      expect(state.pollStatus, 'tbd');
    });
  });

  group('repliedUserIds', () {
    test('unions explicit YES voters and flexible voters', () {
      final state = buildState([
        _proposal('a', 'poll-1', [
          _vote('sam', yes),
          _vote('priya', flexible),
        ]),
        _proposal('b', 'poll-1', [_vote('jess', yes)]),
      ]);
      expect(state.repliedUserIds, {'sam', 'priya', 'jess'});
    });
  });
}
