import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/utils/media_helpers.dart';
import 'package:ripls/data/repositories/media_url.dart';
import 'package:ripls/data/repositories/workshop_repository.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';
import 'package:ripls/services/providers/workshop_providers.dart';

part 'workshop_view_model.freezed.dart';

/// State for the Workshop tab.
///
/// Carries the AI-generated daily brief and the synthesis panels that power
/// the SeasonReportScreen deep view.
@freezed
sealed class WorkshopState with _$WorkshopState {
  const factory WorkshopState({
    GetWorkshopSynthesisResponse? synthesis,
    GetWorkshopBriefResponse? briefResponse,
  }) = _WorkshopState;

  const WorkshopState._();

  bool get hasSynthesis => synthesis?.panels.isNotEmpty ?? false;

  /// True when the server returned a daily brief with renderable content.
  bool get hasBrief => briefResponse != null && briefResponse!.hasBrief();

  /// The brief payload, or null when no brief was returned.
  BriefPayload? get brief =>
      (briefResponse != null && briefResponse!.hasBrief())
          ? briefResponse!.brief
          : null;

  /// True when there is anything renderable on the Workshop screen.
  bool get hasContent => hasBrief || hasSynthesis;
}

/// Fetches and exposes the daily brief and synthesis for all of the host's
/// communities.
class WorkshopNotifier extends AsyncNotifier<WorkshopState> {
  WorkshopRepository get _repository => ref.read(workshopRepositoryProvider);

  @override
  Future<WorkshopState> build() async {
    final communityIds = ref.watch(workshopEnabledCommunityIdsProvider);

    if (communityIds.isEmpty) {
      return const WorkshopState();
    }

    // Fetch synthesis + brief in parallel — both are independent reads.
    final results = await Future.wait([
      _repository.getSynthesis(communityIds: communityIds),
      _repository.getBrief(communityIds: communityIds),
    ]);
    if (!ref.mounted) {
      return const WorkshopState();
    }
    return WorkshopState(
      synthesis: results[0] as GetWorkshopSynthesisResponse,
      briefResponse: results[1] as GetWorkshopBriefResponse,
    );
  }

  /// Resolves a media id to a thumbnail URL. Used by [CTARow] to render the
  /// referenced entity's first media. Thin wrapper around [MediaHelpers] to
  /// keep widgets out of the data layer.
  Future<MediaUrl> getMediaUrl(String mediaId) =>
      MediaHelpers.getMediaUrl(ref, mediaId);

  /// Pull-to-refresh entry point. Wipes the brief + synthesis caches so
  /// the next read pulls fresh data from the server, then forces a
  /// rebuild and waits for it to settle before returning so the
  /// `RefreshIndicator` spinner dismisses on completion.
  Future<void> refresh() async {
    await _repository.invalidateAll();
    ref.invalidateSelf();
    await future;
  }
}

/// Provider for the Workshop screen state.
final workshopProvider =
    AsyncNotifierProvider.autoDispose<WorkshopNotifier, WorkshopState>(
  WorkshopNotifier.new,
);
