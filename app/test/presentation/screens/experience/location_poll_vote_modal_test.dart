import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/experience/location_poll_vote_modal.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

// Mirrors test/presentation/screens/experience/time_poll_vote_modal_test.dart
// — same parity coverage shape so any divergence in poll-modal behavior
// shows up in both test suites at once.

const _experienceId = 'test-experience';
const _activePollId = 'poll-active';
const _priorUserId = 'prior-user';
const _newUserId = 'new-user';

User _user(String id, String name) => User(id: id, name: name);

LocationVote _yesVoteFrom(String userId, String userName) => LocationVote(
      user: _user(userId, userName),
      status: LocationVoteStatus.LOCATION_VOTE_STATUS_YES,
    );

LocationProposal _proposal({
  required String id,
  String pollId = _activePollId,
  List<LocationVote> votes = const [],
  String name = 'Spot',
}) {
  return LocationProposal(
    id: id,
    pollId: pollId,
    location: ProposedLocation(
      geocoded: GeocodedLocation(
        name: name,
        latitudeDeg: 37,
        longitudeDeg: -122,
      ),
    ),
    votes: votes,
  );
}

class _FakeLocationModalNotifier extends LocationModalNotifier {
  _FakeLocationModalNotifier(this._data) : super(_experienceId);

  final LocationModalData _data;

  @override
  Future<LocationModalData> build() async => _data;
}

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

Widget _wrapModal({
  required LocationModalData data,
  required User authUser,
}) {
  return ProviderScope(
    overrides: [
      locationModalProvider(_experienceId)
          .overrideWith(() => _FakeLocationModalNotifier(data)),
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
              builder: (_) => const LocationPollVoteModal(
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

void main() {
  testWidgets(
    'auto-submit binary voting: no "Save" button is rendered',
    (tester) async {
      // Location has always been auto-submit; this asserts the contract
      // that time was migrated to match — keeping it explicit here means
      // any future Save-button regression on either side is caught.
      final data = LocationModalData(
        locationPollActive: true,
        currentLocationPollId: _activePollId,
        currentUserId: _newUserId,
        proposals: [_proposal(id: 'p1')],
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'New Person'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Save'), findsNothing);
    },
  );

  testWidgets(
    'organizer sees a Manage button in the active-poll bottom bar',
    (tester) async {
      final data = LocationModalData(
        locationPollActive: true,
        currentLocationPollId: _activePollId,
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
      final data = LocationModalData(
        locationPollActive: true,
        currentLocationPollId: _activePollId,
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
    'header kicker switches to "YOU\'RE IN" once the viewer has a YES vote',
    (tester) async {
      // Eyebrow contract mirrors time's: idle → "WHICH CAN YOU MAKE?";
      // once the viewer has any YES, switches to the sage submitted
      // kicker ("YOU'RE IN"). Time has the same contract via
      // timePollVoteSubmittedKicker.
      final data = LocationModalData(
        locationPollActive: true,
        currentLocationPollId: _activePollId,
        currentUserId: _newUserId,
        currentUserYesProposalIds: const {'p1'},
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
    'idle viewer (no YES votes) sees the pre-submission kicker',
    (tester) async {
      // Sanity that the eyebrow flips back when the viewer hasn't voted —
      // a regression here would silently lock the modal into the
      // submitted state for first-time voters.
      final data = LocationModalData(
        locationPollActive: true,
        currentLocationPollId: _activePollId,
        currentUserId: _newUserId,
        proposals: [
          _proposal(
            id: 'p1',
            votes: [_yesVoteFrom(_priorUserId, 'Someone Else')],
          ),
        ],
      );

      await tester.pumpWidget(_wrapModal(
        data: data,
        authUser: _user(_newUserId, 'New Person'),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.text('YOU\'RE IN'), findsNothing);
    },
  );
}
