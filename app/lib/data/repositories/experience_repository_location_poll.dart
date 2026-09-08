// Part of [ExperienceRepository]. Holds the location-poll mutation methods
// (propose/vote/confirm/unlock/cancel + Manage actions deadline/lock/nudge
// + bulk-paste candidate extraction) so the main repository file stays
// under the 1000-line lint gate.
//
// Mutation methods here intentionally do **not** call
// [_invalidateExperienceAndDaily] or [_invalidateExperience]. Those helpers
// fire a global cache-invalidation cascade
// (`contentCacheInvalidationProvider`, `portfolioCacheInvalidationProvider`)
// whose listeners in turn debounce a second `getExperienceDetails` RPC; that
// second RPC introduced a 300 ms race window that could resolve in the
// logout frame and crash the IndexedStack with a duplicate NavigatorState
// GlobalKey, and also produced an unnecessary round-trip when the client
// already knew the new state.
//
// Instead, callers ([LocationModalNotifier] et al.) apply the well-defined
// local state change — the new proposal, the recorded vote, the confirmed
// winner — directly to their own state and to [ExperienceNotifier] via
// [ExperienceNotifier.applyExperiencePatch]. The next time the location
// modal is reopened, [LocationModalNotifier.build] invalidates the cache
// and refetches; the global cache TTL bounds staleness everywhere else.
part of 'experience_repository.dart';

extension ExperienceRepositoryLocationPoll on ExperienceRepository {
  /// Proposes a new candidate location. Exactly one of [locationId] /
  /// [geocoded] must be set.
  Future<LocationProposal> proposeLocation({
    required String experienceId,
    String? locationId,
    GeocodedLocation? geocoded,
  }) async {
    return _service.proposeLocation(
      experienceId: experienceId,
      locationId: locationId,
      geocoded: geocoded,
    );
  }

  /// Records a vote on a location proposal.
  Future<void> voteOnLocation({
    required String proposalId,
    required LocationVoteStatus status,
    required String experienceId,
  }) async {
    await _service.voteOnLocation(proposalId: proposalId, status: status);
  }

  /// Confirms a location proposal as the final location (owner only).
  /// Returns the resolved `location_id` — either the winning proposal's
  /// existing saved id, or a freshly-materialized one when the winner
  /// carried an inline geocoded location.
  Future<String> confirmLocation({
    required String experienceId,
    required String proposalId,
  }) async {
    return _service.confirmLocation(
      experienceId: experienceId,
      proposalId: proposalId,
    );
  }

  /// Unlocks a previously confirmed location (owner only).
  Future<void> unlockLocation({required String experienceId}) async {
    await _service.unlockLocation(experienceId: experienceId);
  }

  /// Cancels the active location poll (owner only). Proposals are preserved
  /// so the read-only results view stays usable.
  Future<void> cancelLocationPoll({required String experienceId}) async {
    await _service.cancelLocationPoll(experienceId: experienceId);
  }

  /// Deletes a proposal from the location poll. Owner may delete any
  /// proposal; participants may delete only their own.
  Future<void> deleteLocationProposal({
    required String experienceId,
    required String proposalId,
  }) async {
    await _service.deleteLocationProposal(
      experienceId: experienceId,
      proposalId: proposalId,
    );
  }

  /// Sets the reply-by deadline on the active location poll. Pass
  /// `deadlineUnixSec=0` to clear an existing deadline.
  Future<void> setLocationPollDeadline({
    required String experienceId,
    required int deadlineUnixSec,
  }) async {
    await _service.setLocationPollDeadline(
      experienceId: experienceId,
      deadlineUnixSec: deadlineUnixSec,
    );
  }

  /// Locks or unlocks the proposal list on the active poll.
  Future<void> lockLocationProposals({
    required String experienceId,
    required bool locked,
  }) async {
    await _service.lockLocationProposals(
      experienceId: experienceId,
      locked: locked,
    );
  }

  /// Pings RSVPs who have not voted on the current location poll.
  /// Returns the count of users notified.
  Future<int> nudgeLocationPollVoters({required String experienceId}) async {
    return _service.nudgeLocationPollVoters(experienceId: experienceId);
  }

  /// Extracts and geocodes candidate locations from a free-form text
  /// message. Used by the propose modal's bulk-paste shortcut.
  Future<List<GeocodedLocation>> extractLocationCandidates({
    required String text,
    double? latitudeDeg,
    double? longitudeDeg,
  }) {
    return _service.extractLocationCandidates(
      text: text,
      latitudeDeg: latitudeDeg,
      longitudeDeg: longitudeDeg,
    );
  }
}
