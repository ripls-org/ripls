import 'package:logging/logging.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' as pb;
import 'package:ripls/presentation/viewmodels/experience_view_model.dart'
    show ExperienceState;
import 'package:ripls/services/providers.dart';

final _log = Logger('ExperienceCompletionActions');

/// Completion-flow methods for ExperienceNotifier. Split out of
/// `experience_view_model.dart` to keep that file under the 1000-line cap.
mixin ExperienceCompletionActionsMixin on SafeNotifierMixin<ExperienceState> {
  /// Provided by ExperienceNotifier — full reload after state mutation.
  Future<void> reloadExperienceDetails();

  
  
  
  /// Loads savings data for a completed experience.
  ///
  /// Used when displaying a previously completed experience that doesn't
  /// have completionSavings cached in state. Fetches stats from the server
  /// and populates the completionSavings field.
  Future<void> loadCompletionSavings() async {
    if (state.experienceDetails == null) return;

    if (state.experienceDetails!.experience.state !=
        pb.ExperienceState.EXPERIENCE_STATE_COMPLETED) {
      return;
    }

    if (state.completionSavings != null) return;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final statsResponse = await experienceRepository.getStats(
        state.experienceDetails!.experience.id,
        communityId: state.communityId,
      );

      if (!ref.mounted) return;

      if (statsResponse.hasImpact()) {
        state = state.copyWith(completionSavings: statsResponse.impact);
      }
    } catch (e) {
      _log.warning('⚠️ Failed to load completion savings: $e');
    }
  }
}
