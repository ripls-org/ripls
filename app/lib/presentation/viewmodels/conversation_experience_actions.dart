import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart'
    show ExperienceState;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart'
    show ImpactEstimate;
import 'package:ripls/data/gen/ripls/api/time.pb.dart' show TimeVoteStatus;
import 'package:ripls/presentation/viewmodels/conversation_state.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart'
    show experienceProvider;
import 'package:ripls/services/providers.dart';

final _log = Logger('ConversationExperienceActions');

/// ConversationExperienceActionsMixin provides experience lifecycle methods
/// for [ConversationNotifier].
mixin ConversationExperienceActionsMixin on Notifier<ConversationState> {
  /// setCachedImpact is implemented by the coordinator. Action mixins call it
  /// to store the impact estimate from a terminal action.
  void setCachedImpact(ImpactEstimate? impact);

  // ── Experience status getters ────────────────────────────────────────────

  /// isOrganizer returns true when the current user owns the experience.
  bool get isOrganizer {
    final experience = state.cachedExperience;
    if (experience == null) return false;
    return experience.experience.owner.id == state.currentUserId;
  }

  /// timePollActive returns true when the experience has an active time poll.
  bool get timePollActive {
    final experience = state.cachedExperience;
    if (experience == null) return false;
    return experience.experience.timePollActive;
  }

  /// currentUserRsvp returns the current user's RSVP intention.
  ///
  /// Returns null when the user has not RSVP'd or the map is empty.
  RSVPIntention? get currentUserRsvp {
    final rsvp = state.rsvpStatusMap[state.currentUserId];
    if (rsvp == RSVPIntention.RSVP_INTENTION_UNSPECIFIED) return null;
    return rsvp;
  }

  
  
  // ── Experience data fetching ─────────────────────────────────────────────

  /// fetchExperienceDetails loads experience details from the repository.
  Future<void> fetchExperienceDetails() async {
    final conversation = state.conversation;
    if (conversation == null || !conversation.topic.hasExperienceId()) {
      return;
    }

    final experienceId = conversation.topic.experienceId;
    if (experienceId.isEmpty) {
      return;
    }

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final communityId = conversation.communityId;
      final experience = await experienceRepository.getExperienceDetails(
        experienceId,
        communityId: communityId.isNotEmpty ? communityId : null,
      );

      final rsvpMap = <String, RSVPIntention>{};
      for (final rsvp in experience.rsvps) {
        rsvpMap[rsvp.user.id] = rsvp.intention;
      }

      state = state.copyWith(
        cachedExperience: experience,
        rsvpStatusMap: rsvpMap,
      );
      _log.info(
        'Fetched experience details: ${experience.experience.id} '
        'with ${rsvpMap.length} RSVPs',
      );
      await resolveExperienceLocation(experience.experience.locationId);

      if (experience.experience.state ==
          ExperienceState.EXPERIENCE_STATE_COMPLETED) {
        await fetchImpactEstimate(
          experience.experience.id,
          experience.experience.communityId,
        );
      }
    } catch (e) {
      _log.severe('Failed to fetch experience details: $e');
    }
  }

  /// resolveExperienceLocation looks up the location name and stores it in state.
  Future<void> resolveExperienceLocation(String locationId) async {
    if (locationId.isEmpty) return;
    try {
      final locationRepository = ref.read(locationRepositoryProvider);
      final location = await locationRepository.getLocation(locationId);
      if (!ref.mounted) return;
      final name = LocationFormatter.formatLocationNameShort(location);
      state = state.copyWith(experienceLocationName: name);
    } catch (e) {
      _log.fine('Could not resolve experience location: $e');
    }
  }

  /// fetchImpactEstimate fetches and caches the impact estimate for a
  /// completed experience via the stats endpoint.
  ///
  /// Silently ignores failures — the completion card renders without metrics
  /// when the stats are unavailable.
  Future<void> fetchImpactEstimate(
    String experienceId,
    String communityId,
  ) async {
    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      final stats = await experienceRepository.getStats(
        experienceId,
        communityId: communityId.isNotEmpty ? communityId : null,
      );
      if (stats.hasImpact()) {
        setCachedImpact(stats.impact);
      }
    } catch (e) {
      _log.warning('Failed to fetch impact estimate: $e');
    }
  }

  /// fetchExperienceMedia loads the experience's first media item URL.
  Future<void> fetchExperienceMedia() async {
    final experience = state.cachedExperience;
    if (experience == null || experience.experience.mediaIds.isEmpty) {
      _log.info('No media available for experience');
      return;
    }

    try {
      _log.info(
        'Fetching experience media for: ${experience.experience.mediaIds.first}',
      );

      final mediaRepository = ref.read(mediaRepositoryProvider);
      final mediaUrl = await mediaRepository.getMediaUrl(
        experience.experience.mediaIds.first,
      );

      state = state.copyWith(experienceMediaUrl: mediaUrl.url);

      _log.info(
        'Experience media URL fetched - using '
        '${mediaUrl.isThumbnail ? "THUMBNAIL" : "FULL"}: ${mediaUrl.url}',
      );
    } catch (e) {
      _log.warning('Failed to fetch experience media: $e');
    }
  }

  // ── Experience mutations ─────────────────────────────────────────────────

  
  /// voteOnTime toggles the current user's yes-vote on a time proposal.
  ///
  /// If the user has already voted YES, clears their vote. Otherwise casts YES.
  Future<void> voteOnTime(String proposalId) async {
    final experience = state.cachedExperience;
    if (experience == null) return;

    final experienceId = experience.experience.id;
    if (experienceId.isEmpty) return;

    final proposal = experience.experience.timeProposals
        .where((p) => p.id == proposalId)
        .firstOrNull;

    final alreadyVotedYes =
        proposal?.votes.any(
          (v) =>
              v.user.id == state.currentUserId &&
              v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES,
        ) ??
        false;

    final newStatus = alreadyVotedYes
        ? TimeVoteStatus.TIME_VOTE_STATUS_UNSPECIFIED
        : TimeVoteStatus.TIME_VOTE_STATUS_YES;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.voteOnTime(
        proposalId: proposalId,
        status: newStatus,
        experienceId: experienceId,
      );
      invalidateExperienceViewModelIfActive();
      await fetchExperienceDetails();
      _log.info('✅ Voted on time proposal: $proposalId → $newStatus');
    } catch (e) {
      _log.severe('Failed to vote on time proposal: $e');
      rethrow;
    }
  }

  /// lockTime confirms the best available time proposal, advancing the
  /// experience from Planning to Confirmed phase.
  Future<void> lockTime() async {
    final experience = state.cachedExperience;
    if (experience == null) return;

    final experienceId = experience.experience.id;
    if (experienceId.isEmpty) return;

    final proposals = experience.experience.timeProposals;

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);

      if (proposals.isNotEmpty) {
        final sorted = [...proposals]..sort((a, b) {
            final aYes = a.votes
                .where(
                  (v) => v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES,
                )
                .length;
            final bYes = b.votes
                .where(
                  (v) => v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES,
                )
                .length;
            return bYes.compareTo(aYes);
          });
        final bestProposal = sorted.first;
        await experienceRepository.confirmTime(
          experienceId: experienceId,
          proposalId: bestProposal.id,
        );
      } else {
        // No proposals — save the experience time as-is to lock it.
        final time = experience.experience.time;
        if (time.hasSpecific()) {
          await experienceRepository.proposeTime(
            experienceId: experienceId,
            time: time,
          );
          if (!ref.mounted) return;
          await fetchExperienceDetails();
          final updated = state.cachedExperience;
          if (updated == null || !ref.mounted) return;
          final newProposal =
              updated.experience.timeProposals.lastOrNull;
          if (newProposal != null) {
            await experienceRepository.confirmTime(
              experienceId: experienceId,
              proposalId: newProposal.id,
            );
          }
        }
      }

      if (!ref.mounted) return;

      invalidateExperienceViewModelIfActive();
      await fetchExperienceDetails();
      await refreshMessagesAfterMutation();

      _log.info('✅ Locked time for experience: $experienceId');
    } catch (e) {
      _log.severe('Failed to lock time: $e');
      rethrow;
    }
  }

  
  
  
  /// markExperienceInProcess marks the experience as in-process (owner only).
  Future<void> markExperienceInProcess() async {
    final experienceId = state.cachedExperience?.experience.id ?? '';
    if (experienceId.isEmpty) {
      throw Exception('Experience ID not found');
    }

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.markInProcess(experienceId);

      invalidateExperienceViewModelIfActive();
      await fetchExperienceDetails();
      await refreshMessagesAfterMutation();

      _log.info('✅ Marked experience in process: $experienceId');
    } catch (e) {
      _log.severe('Failed to mark experience in process: $e');
      rethrow;
    }
  }

  /// completeExperience completes an experience (owner only).
  Future<void> completeExperience() async {
    final experienceId = state.cachedExperience?.experience.id ?? '';
    if (experienceId.isEmpty) {
      throw Exception('Experience ID not found');
    }

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.completeExperience(experienceId);

      invalidateExperienceViewModelIfActive();
      await fetchExperienceDetails();
      await refreshMessagesAfterMutation();

      _log.info('✅ Completed experience: $experienceId');
    } catch (e) {
      _log.severe('Failed to complete experience: $e');
      rethrow;
    }
  }

  /// cancelExperience cancels an experience (owner only).
  Future<void> cancelExperience() async {
    final experienceId = state.cachedExperience?.experience.id ?? '';
    if (experienceId.isEmpty) {
      throw Exception('Experience ID not found');
    }

    try {
      final experienceRepository = ref.read(experienceRepositoryProvider);
      await experienceRepository.cancelExperience(experienceId);

      invalidateExperienceViewModelIfActive();
      await fetchExperienceDetails();
      await refreshMessagesAfterMutation();

      _log.info('✅ Cancelled experience: $experienceId');
    } catch (e) {
      _log.severe('Failed to cancel experience: $e');
      rethrow;
    }
  }

  
  /// invalidateAndRefreshExperience invalidates the experience cache and
  /// triggers a fresh fetch plus ExperienceViewModel refresh.
  void invalidateAndRefreshExperience() {
    final expId =
        state.conversation?.topic.experienceId.isNotEmpty ?? false
            ? state.conversation!.topic.experienceId
            : (state.cachedExperience?.experience.id ?? '');
    if (expId.isEmpty) return;

    ref.read(experienceRepositoryProvider).invalidate(expId).then((_) {
      if (!ref.mounted) return;
      fetchExperienceDetails();
      invalidateExperienceViewModelIfActive();
    }).catchError((Object error) {
      _log.warning('Failed to invalidate experience cache: $error');
    });
  }

  /// invalidateExperienceViewModelIfActive refreshes the experienceProvider
  /// family instance so ExperienceContentView reflects the latest data.
  ///
  /// No-op when the experience provider isn't currently alive — otherwise
  /// `ref.read` would resurrect an auto-disposed provider, kick off a
  /// background refresh, and race the auto-dispose timer, producing
  /// "Cannot use the Ref... after it has been disposed" warnings.
  void invalidateExperienceViewModelIfActive() {
    final experienceId = state.cachedExperience?.experience.id ?? '';
    if (experienceId.isEmpty) return;
    if (!ref.exists(experienceProvider(experienceId))) return;
    ref.read(experienceProvider(experienceId).notifier).refresh();
  }

  // ── Cross-mixin hooks ─────────────────────────────────────────────────────

  Future<void> refreshMessagesAfterMutation();
}
