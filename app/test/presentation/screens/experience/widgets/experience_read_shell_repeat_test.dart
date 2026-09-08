// The wrapped-up event's "Schedule the next one" chip — the host's on-ramp to
// the next instance of a recurring rhythm, sitting at the foot of the Who's-in
// card where the crew that just came is listed.
//
// Two gates matter: the event has to have WRAPPED, and the viewer has to own
// it. It is also the only always-present route to the repeat draft (the
// workshop-surface schedule_repeat nudge that used to carry it is filtered out
// of the feed and inbox pools), so losing it is losing the feature.

import 'package:fixnum/fixnum.dart' show Int64;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show ExperienceTime, SpecificTime;
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_read_shell.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/content/content_action_chip.dart';

import '../../../../helpers/l10n_helpers.dart';

const _id = 'exp-1';
const _hostId = 'host-1';
const _runnerId = 'runner-1';

class _FakeExp extends ExperienceNotifier {
  _FakeExp(super.experienceId, this._s);
  final ExperienceState _s;
  @override
  ExperienceState build() => _s;
}

/// The needs notifier's real build() fetches; this one just holds a state so
/// the shell can render its roster without a repository.
class _FakeNeeds extends ExperienceNeedsNotifier {
  _FakeNeeds(super.experienceId);
  @override
  ExperienceNeedsState build() => const ExperienceNeedsState();
}

/// [startsIn] places the event's start relative to now: negative for one that
/// has already happened (which is what unlocks "Wrap up"), positive for one
/// still ahead. Null leaves the time unset.
ExperienceState _state({
  required String viewerId,
  required pb.ExperienceState state,
  Duration? startsIn = const Duration(hours: -2),
}) {
  final experience = pb.Experience(
    id: _id,
    name: 'Wednesday morning run',
    description: '6:30am at the park gate, easy three miles.',
    owner: User(id: _hostId, name: 'Nadia'),
    state: state,
  );
  if (startsIn != null) {
    experience.time = ExperienceTime(
      specific: SpecificTime(
        unixTimestampSec: Int64(
          DateTime.now().add(startsIn).millisecondsSinceEpoch ~/ 1000,
        ),
        timezone: 'America/Los_Angeles',
      ),
    );
  }
  return ExperienceState(
    experienceId: _id,
    currentUserId: viewerId,
    isLoading: false,
    experienceDetails: GetExperienceResponse(
      experience: experience,
      rsvps: [
        RSVP(
          user: User(id: _runnerId, name: 'Priya'),
          intention: RSVPIntention.RSVP_INTENTION_YES,
        ),
      ],
    ),
  );
}

Future<void> _pump(WidgetTester tester, ExperienceState state) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        experienceProvider(_id).overrideWith(() => _FakeExp(_id, state)),
        experienceNeedsProvider(_id).overrideWith(() => _FakeNeeds(_id)),
      ],
      child: localizedApp(
        ExperienceReadShell(
          experienceId: _id,
          accentColor: const Color(0xFF7A9B8C),
          onExpandConversation: (_) {},
          onShowTime: (_) {},
          onShowLocation: (_) {},
          onShowAccess: () {},
          onManage: () {},
        ),
      ),
    ),
  );
  await tester.pump();
}

/// Chips are found by the accessible name they carry, not the visible text,
/// which a locale change would move.
Finder _chipLabelled(String semanticsLabel) => find.byWidgetPredicate(
      (w) => w is ContentActionChip && w.semanticsLabel == semanticsLabel,
    );

Finder _scheduleNextChip() =>
    _chipLabelled('Schedule the next one, pre-filled from this event');
Finder _wrapUpChip() => _chipLabelled('Wrap up this event — confirm who came');

void main() {
  group('ExperienceReadShell — Schedule the next one', () {
    testWidgets('the host of a wrapped-up event gets the chip', (tester) async {
      await _pump(
        tester,
        _state(
          viewerId: _hostId,
          state: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
        ),
      );

      expect(_scheduleNextChip(), findsOneWidget);
      expect(find.text('Schedule the next one'), findsOneWidget);
    });

    testWidgets('a guest of a wrapped-up event does not', (tester) async {
      await _pump(
        tester,
        _state(
          viewerId: _runnerId,
          state: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
        ),
      );

      expect(_scheduleNextChip(), findsNothing);
    });

    testWidgets('a cancelled event does not offer a next one', (tester) async {
      await _pump(
        tester,
        _state(
          viewerId: _hostId,
          state: pb.ExperienceState.EXPERIENCE_STATE_CANCELLED,
        ),
      );

      expect(_scheduleNextChip(), findsNothing);
      expect(_wrapUpChip(), findsNothing);
    });
  });

  group('ExperienceReadShell — Wrap up', () {
    testWidgets('the host gets it once the start time has passed',
        (tester) async {
      await _pump(
        tester,
        _state(
          viewerId: _hostId,
          state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        ),
      );

      expect(_wrapUpChip(), findsOneWidget);
      expect(find.text('Wrap up'), findsOneWidget);
    });

    testWidgets('not before the event has started', (tester) async {
      await _pump(
        tester,
        _state(
          viewerId: _hostId,
          state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
          startsIn: const Duration(days: 3),
        ),
      );

      expect(_wrapUpChip(), findsNothing);
    });

    testWidgets('not when the time is still unset', (tester) async {
      await _pump(
        tester,
        _state(
          viewerId: _hostId,
          state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
          startsIn: null,
        ),
      );

      expect(_wrapUpChip(), findsNothing);
    });

    testWidgets('never for a guest', (tester) async {
      await _pump(
        tester,
        _state(
          viewerId: _runnerId,
          state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        ),
      );

      expect(_wrapUpChip(), findsNothing);
    });

    testWidgets('and it gives way to Schedule the next one once wrapped',
        (tester) async {
      await _pump(
        tester,
        _state(
          viewerId: _hostId,
          state: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
        ),
      );

      expect(_wrapUpChip(), findsNothing);
      expect(_scheduleNextChip(), findsOneWidget);
    });
  });

  group('ExperienceReadShell — the RSVP chip keeps its slot', () {
    testWidgets('a guest who has not replied still gets it', (tester) async {
      // All three chips share one slot; the host-only ones must not swallow
      // the call-to-action a guest needs.
      await _pump(
        tester,
        _state(
          viewerId: 'someone-else',
          state: pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
          startsIn: const Duration(days: 3),
        ),
      );

      expect(_scheduleNextChip(), findsNothing);
      expect(_wrapUpChip(), findsNothing);
      expect(find.byType(ContentActionChip), findsOneWidget);
    });
  });
}
