import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/experience/time_poll_vote_modal.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/experience/time_proposal_tile.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

// ---------------------------------------------------------------------------
// Test data
// ---------------------------------------------------------------------------

const _experienceId = 'test-experience';
const _activePollId = 'poll-active';
const _priorUserId = 'prior-user';
const _newUserId = 'new-user';

User _user(String id, String name) => User(id: id, name: name);

TimeVote _yesVoteFrom(String userId, String userName) => TimeVote(
      user: _user(userId, userName),
      status: TimeVoteStatus.TIME_VOTE_STATUS_YES,
    );

TimeProposal _proposal({
  required String id,
  String pollId = _activePollId,
  List<TimeVote> votes = const [],
  int unixTimestampSec = 1743148800,
}) {
  return TimeProposal(
    id: id,
    pollId: pollId,
    time: ExperienceTime(
      specific: SpecificTime(
        unixTimestampSec: Int64(unixTimestampSec),
        timezone: 'UTC',
        durationMinutes: 60,
      ),
    ),
    votes: votes,
    isConfirmed: false,
    proposedAtUnixSec: Int64(1743100000),
  );
}

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

class _FakeTimeModalNotifier extends TimeModalNotifier {
  _FakeTimeModalNotifier(this._data) : super(_experienceId);

  final TimeModalData _data;

  @override
  Future<TimeModalData> build() async => _data;
}

/// Stub that bypasses AuthStateNotifier.build()'s lifecycle wiring (secure
/// storage, RPC handlers, lifecycle observers) and just returns a fixed user.
class _FakeAuthStateNotifier extends AuthStateNotifier {
  _FakeAuthStateNotifier(this._user);

  final User? _user;

  @override
  AuthStateData build() {
    return AuthStateData(
      isLoading: false,
      accessToken: 'test-token',
      user: _user,
    );
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

Widget _wrapModal({
  required TimeModalData data,
  required User authUser,
}) {
  return ProviderScope(
    overrides: [
      timeModalProvider(_experienceId)
          .overrideWith(() => _FakeTimeModalNotifier(data)),
      authStateProvider.overrideWith(() => _FakeAuthStateNotifier(authUser)),
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
        body: Builder(
          builder: (context) => ElevatedButton(
            onPressed: () => showModalBottomSheet(
              context: context,
              isScrollControlled: true,
              backgroundColor: Colors.transparent,
              builder: (_) => const TimePollVoteModal(
                experienceId: _experienceId,
              ),
            ),
            child: const Text('Open'),
          ),
        ),
      ),
    ),
  );
}

/// Counts the "selected" indicator (filled circle with white check) inside
/// proposal tiles in the open vote modal — one per pre-selected tile.
int _selectedTileCount(WidgetTester tester) {
  return tester
      .widgetList(find.descendant(
        of: find.byType(TimeProposalTile),
        matching: find.byWidgetPredicate((w) =>
            w is Icon && w.icon == Icons.check && w.color == Colors.white),
      ))
      .length;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  testWidgets(
    'a freshly-signed-in user does not inherit the prior user\'s YES votes',
    (tester) async {
      // Regression for the user-switch bug: TimeModalData's cached
      // `currentUserVotes` map carried the prior user's selections when a
      // different user signed in. Pre-selection must derive from the live
      // auth user against proposal.votes, not from the cached map.
      final data = TimeModalData(
        timePollActive: true,
        currentPollId: _activePollId,
        currentUserId: _priorUserId,
        proposals: [
          _proposal(
            id: 'p1',
            votes: [_yesVoteFrom(_priorUserId, 'Prior Person')],
          ),
          _proposal(
            id: 'p2',
            votes: [_yesVoteFrom(_priorUserId, 'Prior Person')],
          ),
          _proposal(id: 'p3'),
        ],
        // Worst-case stale state: the cached map even includes the prior
        // user's YES votes keyed by proposal id.
        currentUserVotes: const {
          'p1': TimeVoteStatus.TIME_VOTE_STATUS_YES,
          'p2': TimeVoteStatus.TIME_VOTE_STATUS_YES,
        },
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'New Person'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(
        _selectedTileCount(tester),
        0,
        reason: 'Pre-selection must be derived from the live auth user, not '
            "from the cached `currentUserVotes` map (which can carry the prior "
            "user's votes after a logout/login swap).",
      );
    },
  );

  testWidgets(
    'the current user\'s own YES votes are still pre-selected',
    (tester) async {
      // Sanity: the user-switch fix didn't regress the happy path. When the
      // auth user *has* voted YES, those tiles come up pre-selected.
      final data = TimeModalData(
        timePollActive: true,
        currentPollId: _activePollId,
        currentUserId: _newUserId,
        proposals: [
          _proposal(
            id: 'p1',
            votes: [_yesVoteFrom(_newUserId, 'New Person')],
          ),
          _proposal(id: 'p2'),
        ],
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'New Person'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(_selectedTileCount(tester), 1);
    },
  );

  testWidgets(
    'auto-submit binary voting: no "Save" button is rendered',
    (tester) async {
      // Phase 2 of the migration replaced the batch-and-save flow with
      // tap-to-toggle auto-submit. There should be no Save affordance in
      // the bottom bar.
      final data = TimeModalData(
        timePollActive: true,
        currentPollId: _activePollId,
        currentUserId: _newUserId,
        proposals: [_proposal(id: 'p1')],
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'New Person'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      // Save was the old batching CTA; absence is the contract that the
      // auto-submit migration introduced.
      expect(find.textContaining('Save'), findsNothing);
    },
  );

  testWidgets(
    'organizer sees a Manage button in the active-poll bottom bar',
    (tester) async {
      final data = TimeModalData(
        timePollActive: true,
        currentPollId: _activePollId,
        isOrganizer: true,
        currentUserId: _newUserId,
        proposals: [_proposal(id: 'p1')],
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'Organizer'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('Manage'), findsOneWidget);
    },
  );

  testWidgets(
    'non-organizer does not see the Manage button',
    (tester) async {
      final data = TimeModalData(
        timePollActive: true,
        currentPollId: _activePollId,
        isOrganizer: false,
        currentUserId: _newUserId,
        proposals: [_proposal(id: 'p1')],
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'Member'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('Manage'), findsNothing);
    },
  );

  testWidgets(
    'header eyebrow switches to "YOU\'RE IN" once the viewer has a YES vote',
    (tester) async {
      // Eyebrow contract: idle → "N of M voted" upper-case; once the viewer
      // has any YES on the active poll, switches to the sage submitted
      // kicker ("YOU'RE IN").
      final data = TimeModalData(
        timePollActive: true,
        currentPollId: _activePollId,
        currentUserId: _newUserId,
        proposals: [
          _proposal(
            id: 'p1',
            votes: [_yesVoteFrom(_newUserId, 'New Person')],
          ),
          _proposal(id: 'p2'),
        ],
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'New Person'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('YOU\'RE IN'), findsOneWidget);
    },
  );

  testWidgets(
    'read-only branch (poll ended, no winner) shows the Pick winning time CTA',
    (tester) async {
      // timePollActive=false + timePollCompleted=true + no eventTime means
      // the poll ended without a winner. Owner sees a recovery CTA.
      final data = TimeModalData(
        timePollActive: false,
        timePollCompleted: true,
        isOrganizer: true,
        currentUserId: _newUserId,
        proposals: [_proposal(id: 'p1')],
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'Organizer'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('Pick winning time'), findsOneWidget);
      expect(find.text('Close'), findsNothing);
    },
  );

  testWidgets(
    'read-only branch with a confirmed time shows Close, not the recovery CTA',
    (tester) async {
      final confirmedTime = ExperienceTime(
        specific: SpecificTime(
          unixTimestampSec: Int64(1743148800),
          timezone: 'UTC',
          durationMinutes: 60,
        ),
      );
      final data = TimeModalData(
        timePollActive: false,
        timePollCompleted: true,
        isOrganizer: true,
        currentUserId: _newUserId,
        proposals: [_proposal(id: 'p1')],
        eventTime: confirmedTime,
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'Organizer'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('Close'), findsOneWidget);
      expect(find.text('Pick winning time'), findsNothing);
    },
  );
}
