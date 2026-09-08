import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_rsvp_controls.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';

import '../../../helpers/l10n_helpers.dart';

const _id = 'exp-1';

class _FakeExp extends ExperienceNotifier {
  _FakeExp(super.experienceId, this._s);
  final ExperienceState _s;
  @override
  ExperienceState build() => _s;
}

ExperienceState _state({
  required String currentUserId,
  RSVPIntention? viewerIntention,
  pb.ExperienceState state = pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
}) {
  return ExperienceState(
    experienceId: _id,
    currentUserId: currentUserId,
    experienceDetails: GetExperienceResponse(
      experience: pb.Experience(
        id: _id,
        name: 'Hike',
        owner: User(id: 'anna', name: 'Anna Reyes'),
        state: state,
      ),
      // The viewer's reply reaches the view the way the server sends it —
      // as their row in the roster.
      rsvps: [
        if (viewerIntention != null)
          RSVP(
            user: User(id: currentUserId, name: 'Viewer'),
            intention: viewerIntention,
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
      ],
      child: localizedApp(
        const ExperienceRsvpControls(
          experienceId: _id,
          accentColor: Color(0xFF7A9B8C),
        ),
      ),
    ),
  );
  await tester.pump();
}

void main() {
  group('ExperienceRsvpControls', () {
    testWidgets("a guest gets the Going / Maybe / Can't go control", (
      tester,
    ) async {
      await _pump(tester, _state(currentUserId: 'viewer'));
      expect(find.text('Going'), findsOneWidget);
      expect(find.text('Maybe'), findsOneWidget);
      expect(find.text("Can't go"), findsOneWidget);
    });

    testWidgets('the host gets the same control (they may not attend)', (
      tester,
    ) async {
      await _pump(tester, _state(currentUserId: 'anna'));
      // Hosts don't always attend, so they get the RSVP control too.
      expect(find.text('Going'), findsOneWidget);
      expect(find.text("Can't go"), findsOneWidget);
    });

    testWidgets('a replied viewer sees a status line, not the buttons', (
      tester,
    ) async {
      await _pump(
        tester,
        _state(
          currentUserId: 'viewer',
          viewerIntention: RSVPIntention.RSVP_INTENTION_YES,
        ),
      );
      expect(find.text("You're going"), findsOneWidget);
      expect(find.text('Tap to change ›'), findsOneWidget);
      // The segmented buttons are hidden until the viewer taps to change.
      expect(find.text('Going'), findsNothing);
      expect(find.text("Can't go"), findsNothing);
    });

    testWidgets('tapping the status line reveals the buttons', (tester) async {
      await _pump(
        tester,
        _state(
          currentUserId: 'viewer',
          viewerIntention: RSVPIntention.RSVP_INTENTION_MAYBE,
        ),
      );
      expect(find.text("You're a maybe"), findsOneWidget);

      await tester.tap(find.text("You're a maybe"));
      await tester.pump();

      expect(find.text('Going'), findsOneWidget);
      expect(find.text('Maybe'), findsOneWidget);
      expect(find.text("Can't go"), findsOneWidget);
    });

    testWidgets('renders nothing once the event is terminal', (tester) async {
      await _pump(
        tester,
        _state(
          currentUserId: 'anna',
          state: pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
        ),
      );
      expect(find.text('Going'), findsNothing);
      expect(find.text("Can't go"), findsNothing);
    });
  });
}
