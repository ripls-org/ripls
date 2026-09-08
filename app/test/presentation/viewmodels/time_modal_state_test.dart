import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/time_modal_state.dart';

User _user(String id) => User()..id = id;

TimeVote _vote(String userId, TimeVoteStatus status) => TimeVote()
  ..user = _user(userId)
  ..status = status;

TimeProposal _proposal(String id, String pollId, List<TimeVote> votes) =>
    TimeProposal()
      ..id = id
      ..pollId = pollId
      ..votes.addAll(votes);

void main() {
  const yes = TimeVoteStatus.TIME_VOTE_STATUS_YES;
  const flexible = TimeVoteStatus.TIME_VOTE_STATUS_FLEXIBLE;

  TimeModalData buildState(List<TimeProposal> proposals,
          {String? currentUserId = 'me', String pollId = 'poll-1'}) =>
      TimeModalData(
        proposals: proposals,
        currentPollId: pollId,
        currentUserId: currentUserId,
        timePollActive: true,
      );

  test('currentPollProposals scopes to the active poll id', () {
    final state = buildState([
      _proposal('a', 'poll-1', []),
      _proposal('old', 'poll-0', []),
      _proposal('b', 'poll-1', []),
    ]);
    expect(state.currentPollProposals.map((p) => p.id), ['a', 'b']);
  });

  test('a flexible voter counts toward every proposal', () {
    final state = buildState([
      _proposal('a', 'poll-1', [_vote('sam', yes), _vote('priya', flexible)]),
      _proposal('b', 'poll-1', []),
    ]);
    expect(state.effectiveVoteCount('a'), 2);
    expect(state.effectiveVoteCount('b'), 1);
    expect(state.currentPollFlexibleVoterIds, {'priya'});
  });

  test('leadingProposalId picks the proposal with most effective support', () {
    final state = buildState([
      _proposal('a', 'poll-1', [_vote('sam', yes)]),
      _proposal('b', 'poll-1', [_vote('sam', yes), _vote('jess', yes)]),
    ]);
    expect(state.leadingProposalId, 'b');
  });

  test('leadingProposalId is null when nothing has support', () {
    final state = buildState([
      _proposal('a', 'poll-1', []),
      _proposal('b', 'poll-1', []),
    ]);
    expect(state.leadingProposalId, isNull);
  });

  test('currentUserIsFlexible reflects the viewer flexible vote', () {
    final flex = buildState([
      _proposal('a', 'poll-1', [_vote('me', flexible)]),
    ]);
    final notFlex = buildState([
      _proposal('a', 'poll-1', [_vote('me', yes)]),
    ]);
    expect(flex.currentUserIsFlexible, isTrue);
    expect(notFlex.currentUserIsFlexible, isFalse);
  });

  test('pollStatus reflects poll / set / tbd', () {
    expect(
      buildState([_proposal('a', 'poll-1', [])]).pollStatus,
      'poll',
    );
    expect(
      const TimeModalData(
        lockedProposalId: 'a',
        timePollActive: false,
      ).pollStatus,
      'set',
    );
    expect(const TimeModalData(timePollActive: false).pollStatus, 'tbd');
  });

  test('repliedUserIds unions YES and flexible voters', () {
    final state = buildState([
      _proposal('a', 'poll-1', [_vote('sam', yes), _vote('priya', flexible)]),
      _proposal('b', 'poll-1', [_vote('jess', yes)]),
    ]);
    expect(state.repliedUserIds, {'sam', 'priya', 'jess'});
  });
}
