// Debug-only visual preview harness for the poll morphing sheet.
//
// NOT part of the shipping app — only reachable when the app is launched with
// `--dart-define=POLL_PREVIEW=<kind>` (see main.dart). It renders a poll body
// for a single seeded state over a mock hero, with the view-model + auth
// providers overridden by fakes (no network, no auth). Used to iterate on the
// redesign against `docs/cowork/App Design/*-prototype*.html` by screenshotting
// the simulator and diffing against the rendered prototype — no real data or
// navigation required.
//
// Kinds: `time-tbd`, `time-poll`, `time-set` (host), `time-poll-friend`.
import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/experience/time_poll_finalized_modal.dart';
import 'package:ripls/presentation/screens/experience/time_poll_propose_modal.dart';
import 'package:ripls/presentation/screens/experience/time_poll_vote_modal.dart';
import 'package:ripls/presentation/viewmodels/time_modal_view_model.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart';

const _previewId = 'preview-experience';
const _pollId = 'preview-poll';

User _user(String id, String name) => User(id: id, name: name);

TimeVote _vote(String id, String name, TimeVoteStatus status) =>
    TimeVote(user: _user(id, name), status: status);

TimeProposal _proposal(
  String id,
  List<TimeVote> votes, {
  int unixSec = 1751302800,
  bool confirmed = false,
}) =>
    TimeProposal(
      id: id,
      pollId: _pollId,
      time: ExperienceTime(
        specific: SpecificTime(
          unixTimestampSec: Int64(unixSec),
          timezone: 'America/Denver',
          durationMinutes: 120,
        ),
      ),
      votes: votes,
      isConfirmed: confirmed,
      proposedAtUnixSec: Int64(1751200000),
    );

class _FakeTimeModalNotifier extends TimeModalNotifier {
  _FakeTimeModalNotifier(this._data) : super(_previewId);
  final TimeModalData _data;
  @override
  Future<TimeModalData> build() async => _data;
}

class _FakeAuthStateNotifier extends AuthStateNotifier {
  _FakeAuthStateNotifier(this._user);
  final User? _user;
  @override
  AuthStateData build() =>
      AuthStateData(isLoading: false, accessToken: 'preview', user: _user);
}

/// Seeded data + body widget for a given preview kind.
({TimeModalData data, Widget body}) _seedFor(String kind) {
  const me = 'me';
  switch (kind) {
    case 'time-poll':
      return (
        data: TimeModalData(
          timePollActive: true,
          currentPollId: _pollId,
          isOrganizer: true,
          currentUserId: me,
          proposals: [
            _proposal('p1', [
              _vote(me, 'You', TimeVoteStatus.TIME_VOTE_STATUS_YES),
              _vote('sam', 'Sam', TimeVoteStatus.TIME_VOTE_STATUS_YES),
            ]),
            _proposal('p2', [
              _vote('priya', 'Priya', TimeVoteStatus.TIME_VOTE_STATUS_FLEXIBLE),
            ], unixSec: 1751389200),
          ],
        ),
        body: const TimePollVoteModal(experienceId: _previewId, embedded: true),
      );
    case 'time-poll-friend':
      return (
        data: TimeModalData(
          timePollActive: true,
          currentPollId: _pollId,
          isOrganizer: false,
          currentUserId: me,
          proposals: [
            _proposal('p1', [
              _vote('sam', 'Sam', TimeVoteStatus.TIME_VOTE_STATUS_YES),
            ]),
            _proposal('p2', const [], unixSec: 1751389200),
          ],
        ),
        body: const TimePollVoteModal(experienceId: _previewId, embedded: true),
      );
    case 'time-set':
      return (
        data: TimeModalData(
          timePollActive: false,
          lockedProposalId: 'p1',
          isOrganizer: true,
          currentUserId: me,
          eventTime: ExperienceTime(
            specific: SpecificTime(
              unixTimestampSec: Int64(1751302800),
              timezone: 'America/Denver',
              durationMinutes: 120,
            ),
          ),
          proposals: [
            _proposal('p1', [
              _vote(me, 'You', TimeVoteStatus.TIME_VOTE_STATUS_YES),
              _vote('sam', 'Sam', TimeVoteStatus.TIME_VOTE_STATUS_YES),
            ], confirmed: true),
            _proposal('p2', const [], unixSec: 1751389200),
          ],
        ),
        body:
            const TimePollFinalizedModal(experienceId: _previewId, embedded: true),
      );
    case 'time-tbd':
    default:
      return (
        data: const TimeModalData(
          timePollActive: false,
          isOrganizer: true,
          currentUserId: me,
        ),
        body:
            const TimePollProposeModal(experienceId: _previewId, embedded: true),
      );
  }
}

/// Root widget launched by main.dart when POLL_PREVIEW is set.
class PollPreviewApp extends StatelessWidget {
  const PollPreviewApp({super.key, required this.kind});

  final String kind;

  @override
  Widget build(BuildContext context) {
    final seed = _seedFor(kind);
    return ProviderScope(
      overrides: [
        timeModalProvider(_previewId)
            .overrideWith(() => _FakeTimeModalNotifier(seed.data)),
        authStateProvider
            .overrideWith(() => _FakeAuthStateNotifier(_user('me', 'You'))),
      ],
      child: MaterialApp(
        debugShowCheckedModeBanner: false,
        theme: AppTheme.darkTheme,
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: _PreviewScaffold(body: seed.body),
      ),
    );
  }
}

class _PreviewScaffold extends StatelessWidget {
  const _PreviewScaffold({required this.body});
  final Widget body;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Stack(
        children: [
          // Mock hero behind the sheet, mirroring the experience screen.
          Positioned.fill(
            child: DecoratedBox(
              decoration: const BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment.topCenter,
                  end: Alignment.bottomCenter,
                  colors: [Color(0xFF3C5A36), Color(0xFF172417), Color(0xFF0E160D)],
                ),
              ),
            ),
          ),
          // Mirror the real app: TimePollSheet wraps the body in a frosted
          // GlassSheet (design system #1797), so the preview shows the same
          // surface rather than a faked opaque box.
          Align(alignment: Alignment.bottomCenter, child: GlassSheet(child: body)),
        ],
      ),
    );
  }
}
