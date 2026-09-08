import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/content/content_lifecycle_phase.dart';

/// Builds an [ExperienceState] whose experience has the given [state] and,
/// optionally, a recorded start time.
ExperienceState _stateFor(pb.ExperienceState state, {Int64? startedAt}) {
  final exp = pb.Experience(state: state);
  if (startedAt != null) {
    exp.startedAtUnixSec = startedAt;
  }
  return ExperienceState(
    experienceDetails: GetExperienceResponse(experience: exp),
  );
}

void main() {
  group('ExperienceState.lifecyclePhaseAt', () {
    const now = 1000000;

    test('null details defaults to sharing', () {
      expect(
        const ExperienceState().lifecyclePhaseAt(now),
        ContentLifecyclePhase.sharing,
      );
    });

    test('ACTIVE maps to sharing', () {
      expect(
        _stateFor(
          pb.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        ).lifecyclePhaseAt(now),
        ContentLifecyclePhase.sharing,
      );
    });

    test('UNSPECIFIED falls back to sharing', () {
      expect(
        _stateFor(
          pb.ExperienceState.EXPERIENCE_STATE_UNSPECIFIED,
        ).lifecyclePhaseAt(now),
        ContentLifecyclePhase.sharing,
      );
    });

    test('JOINED maps to confirmed', () {
      expect(
        _stateFor(
          pb.ExperienceState.EXPERIENCE_STATE_JOINED,
        ).lifecyclePhaseAt(now),
        ContentLifecyclePhase.confirmed,
      );
    });

    test('COMPLETED maps to wrapped', () {
      expect(
        _stateFor(
          pb.ExperienceState.EXPERIENCE_STATE_COMPLETED,
        ).lifecyclePhaseAt(now),
        ContentLifecyclePhase.wrapped,
      );
    });

    test('CANCELLED maps to cancelled', () {
      expect(
        _stateFor(
          pb.ExperienceState.EXPERIENCE_STATE_CANCELLED,
        ).lifecyclePhaseAt(now),
        ContentLifecyclePhase.cancelled,
      );
    });

    test('IN_PROCESS before the start time is leaving', () {
      final state = _stateFor(
        pb.ExperienceState.EXPERIENCE_STATE_IN_PROCESS,
        startedAt: Int64(now + 3600),
      );
      expect(state.lifecyclePhaseAt(now), ContentLifecyclePhase.leaving);
    });

    test('IN_PROCESS at/after the start time is onTheRoad', () {
      final state = _stateFor(
        pb.ExperienceState.EXPERIENCE_STATE_IN_PROCESS,
        startedAt: Int64(now - 3600),
      );
      expect(state.lifecyclePhaseAt(now), ContentLifecyclePhase.onTheRoad);
    });

    test('IN_PROCESS without a start time is leaving', () {
      expect(
        _stateFor(
          pb.ExperienceState.EXPERIENCE_STATE_IN_PROCESS,
        ).lifecyclePhaseAt(now),
        ContentLifecyclePhase.leaving,
      );
    });
  });
}
