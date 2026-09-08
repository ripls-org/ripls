import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:protobuf/protobuf.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart'
    show experienceProvider;
import 'package:ripls/services/providers.dart';

import 'location_modal_state.dart';

export 'location_modal_state.dart';

final _log = Logger('LocationModalViewModel');

/// AsyncNotifier managing the location-poll modal state for a single
/// experience. Mirrors [TimeModalNotifier] but for the location flow.
///
/// Disposal is automatic in [build()]. Methods that mutate state manually
/// after an async gap must guard with `if (!ref.mounted) return;`.
class LocationModalNotifier extends AsyncNotifier<LocationModalData> {
  LocationModalNotifier(this.experienceId);

  final String experienceId;

  ExperienceRepository get _repository =>
      ref.read(experienceRepositoryProvider);

  @override
  Future<LocationModalData> build() async {
    ref.onDispose(() {
      _log.fine('LocationModalNotifier disposed for experience: $experienceId');
    });

    // Bail before any async work if the user is not authenticated. A rebuild
    // can be triggered by `ref.invalidateSelf()` from a mutation whose RPC
    // races logout; firing another RPC here would write state into the
    // logout frame and crash the IndexedStack with a duplicate NavigatorState
    // GlobalKey.
    if (ref.read(authStateProvider).user == null) {
      return const LocationModalData();
    }

    // Always refresh from server so vote counts reflect votes cast via
    // other surfaces (e.g. inline chat banner).
    await _repository.invalidate(experienceId);

    final response = await _repository.getExperienceDetails(experienceId);
    final authState = ref.read(authStateProvider);
    if (authState.user == null) {
      return const LocationModalData();
    }
    final exp = response.experience;
    final currentUserId = authState.user?.id;

    // Reconcile against Experience.locationId — the source of truth for
    // the confirmed spot. The LX2 "Change → Pick a Different Spot" path
    // calls SaveExperience to swap the locationId without going through
    // ConfirmLocation, so a proposal's is_confirmed flag can be stale.
    // Prefer the proposal whose location matches the experience; fall back
    // to is_confirmed only when no locationId has been set yet (poll
    // completed but pre-confirmation flow).
    LocationProposal? locked;
    if (exp.locationId.isNotEmpty) {
      locked = exp.locationProposals
          .where((p) => p.location.locationId == exp.locationId)
          .firstOrNull;
    } else {
      locked = exp.locationProposals
          .where((p) => p.isConfirmed)
          .firstOrNull;
    }

    final yesIds = <String>{};
    for (final p in exp.locationProposals) {
      final mine = p.votes
          .where((v) =>
              v.user.id == currentUserId &&
              v.status == LocationVoteStatus.LOCATION_VOTE_STATUS_YES)
          .firstOrNull;
      if (mine != null) yesIds.add(p.id);
    }

    final experienceState = exp.state;
    final isReadOnly =
        experienceState == ExperienceState.EXPERIENCE_STATE_COMPLETED ||
            experienceState == ExperienceState.EXPERIENCE_STATE_CANCELLED;

    final mode =
        (locked != null || exp.locationId.isNotEmpty) ? 'confirmed' : 'default';
    final isOrganizer = exp.owner.id == currentUserId;

    return LocationModalData(
      proposals: exp.locationProposals,
      lockedProposalId: locked?.id,
      isOrganizer: isOrganizer,
      isReadOnly: isReadOnly,
      currentUserYesProposalIds: yesIds,
      currentUserId: currentUserId,
      eventLocationId: exp.locationId.isEmpty ? null : exp.locationId,
      mode: mode,
      locationPollActive: exp.locationPollActive,
      locationPollCompleted: exp.hasLocationPollCompleted() &&
          exp.locationPollCompleted,
      currentLocationPollId: exp.hasCurrentLocationPollId()
          ? exp.currentLocationPollId
          : null,
      locationPollDeadlineUnixSec: exp.hasLocationPollDeadlineUnixSec()
          ? exp.locationPollDeadlineUnixSec.toInt()
          : null,
      locationProposalsLocked: exp.hasLocationProposalsLocked() &&
          exp.locationProposalsLocked,
    );
  }

  /// Applies a patch to [ExperienceNotifier]'s cached experience proto so
  /// every surface that reads it (Plan-tab row, banners, FAB color) sees
  /// the same update we just applied to our own state — without firing the
  /// `contentCacheInvalidationProvider` cascade that previously needed a
  /// 300 ms debounce and could race logout.
  void _syncExperience(void Function(Experience exp) patch) {
    ref.read(experienceProvider(experienceId).notifier).applyExperiencePatch(
      patch,
    );
  }

  /// Toggles a YES vote on a single proposal with optimistic UI.
  ///
  /// Both the YES-id set (drives the checkbox state) and the per-proposal
  /// `votes` list (drives the avatar stack + count chip) update before the
  /// RPC fires. On RPC failure the prior snapshot is restored — the
  /// proposals list is rebuilt fresh on each call instead of mutating the
  /// proto in place, so the rollback restores the exact pre-vote view.
  Future<void> voteOnLocation(String proposalId) async {
    final previous = state.requireValue;
    final voter = ref.read(authStateProvider).user;
    final isYes = previous.currentUserYesProposalIds.contains(proposalId);
    final newStatus = isYes
        ? LocationVoteStatus.LOCATION_VOTE_STATUS_UNSPECIFIED
        : LocationVoteStatus.LOCATION_VOTE_STATUS_YES;

    final updatedYesIds = {...previous.currentUserYesProposalIds};
    if (isYes) {
      updatedYesIds.remove(proposalId);
    } else {
      updatedYesIds.add(proposalId);
    }
    final optimisticProposals = _projectVote(
      previous.proposals,
      proposalId: proposalId,
      voter: voter,
      newStatus: newStatus,
    );
    state = AsyncData(previous.copyWith(
      proposals: optimisticProposals,
      currentUserYesProposalIds: updatedYesIds,
    ));

    try {
      await _repository.voteOnLocation(
        proposalId: proposalId,
        status: newStatus,
        experienceId: experienceId,
      );
      if (!ref.mounted) return;
      _syncExperience((exp) => _applyVoteToProto(
            exp,
            proposalId: proposalId,
            voter: voter,
            newStatus: newStatus,
          ));
    } catch (e) {
      _log.warning('Failed to vote on location: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  
  /// Marks (or clears) the current user as flexible — "any spot works" — on
  /// the active poll, with optimistic UI.
  ///
  /// A flexible vote is recorded as `FLEXIBLE` on a single representative
  /// proposal (the first in the current poll); [LocationModalData] folds
  /// flexible voters into every option's effective tally and the leader
  /// calc, mirroring the server's nudge bookkeeping. No-op when the poll has
  /// no proposals to attach the marker to.
  Future<void> setFlexibleOnLocation(bool flexible) async {
    final previous = state.requireValue;
    final representative = previous.currentPollProposals.firstOrNull;
    if (representative == null) return;
    final voter = ref.read(authStateProvider).user;
    final newStatus = flexible
        ? LocationVoteStatus.LOCATION_VOTE_STATUS_FLEXIBLE
        : LocationVoteStatus.LOCATION_VOTE_STATUS_UNSPECIFIED;

    final optimisticProposals = _projectVote(
      previous.proposals,
      proposalId: representative.id,
      voter: voter,
      newStatus: newStatus,
    );
    state = AsyncData(previous.copyWith(proposals: optimisticProposals));

    try {
      await _repository.voteOnLocation(
        proposalId: representative.id,
        status: newStatus,
        experienceId: experienceId,
      );
      if (!ref.mounted) return;
      _syncExperience((exp) => _applyVoteToProto(
            exp,
            proposalId: representative.id,
            voter: voter,
            newStatus: newStatus,
          ));
    } catch (e) {
      _log.warning('Failed to set flexible vote on location: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Proposes a location identified by a saved location id.
  Future<void> proposeSavedLocation(String locationId) async {
    if (locationId.isEmpty) {
      throw ArgumentError('locationId is required');
    }
    try {
      final proposal = await _repository.proposeLocation(
        experienceId: experienceId,
        locationId: locationId,
      );
      if (!ref.mounted) return;
      _applyProposalAdded(proposal);
    } catch (e) {
      _log.warning('Failed to propose saved location: $e');
      rethrow;
    }
  }

  
  /// Sets a single candidate as the actual event location, bypassing the
  /// poll flow. Exactly one of [locationId] / [geocoded] must be provided.
  ///
  /// When [locationId] is provided (saved-location case), calls
  /// `SaveExperience` directly — the same path as the LX2 "Change" flow —
  /// so no transient `ProposeLocation` poll is opened and no spurious "Poll
  /// ended" chat banner is written (see #2598).
  ///
  /// When [geocoded] is provided, `ProposeLocation` is still required to
  /// materialize the drop-pin into a canonical `location_id`; this path
  /// still writes the transient poll system message (residual banner leak,
  /// filed as #2607).
  ///
  /// Precondition: there must not already be an active location poll —
  /// callers in live-edit mode should hit [proposeSavedLocation] /
  /// [proposeGeocodedLocation] instead.
  Future<void> setSingleLocation({
    String? locationId,
    GeocodedLocation? geocoded,
  }) async {
    assert(
      (locationId != null) ^ (geocoded != null),
      'Exactly one of locationId / geocoded must be provided',
    );
    final previous = state.requireValue;
    assert(
      !previous.locationPollActive,
      'setSingleLocation must not be called while a poll is active',
    );
    if (locationId != null) {
      // Saved-location direct-set: use SaveExperience to avoid opening a
      // transient poll that leaks a "Poll ended" chat banner (#2598).
      try {
        final response = await _repository.getExperienceDetails(experienceId);
        if (!ref.mounted) return;
        final experience = response.experience;
        await _repository.saveExperience(
          id: experienceId,
          name: experience.name,
          description: experience.description,
          mediaIds: experience.mediaIds,
          locationId: locationId,
          time: experience.hasTime() ? experience.time : null,
          maxParticipants: experience.maxParticipants > 0
              ? experience.maxParticipants
              : null,
          sourceUrl: experience.sourceUrl.isNotEmpty
              ? experience.sourceUrl
              : null,
        );
        if (!ref.mounted) return;
        ref.invalidateSelf();
      } catch (e) {
        _log.warning('Failed to set single location: $e');
        if (!ref.mounted) return;
        state = AsyncData(previous);
        rethrow;
      }
    } else {
      // Geocoded drop-pin: ProposeLocation materializes a canonical
      // location_id; ConfirmLocation resolves it. This path still leaks the
      // "poll ended" chat banner (residual from #2598, filed as #2607).
      try {
        final proposal = await _repository.proposeLocation(
          experienceId: experienceId,
          geocoded: geocoded,
        );
        if (!ref.mounted) return;
        _applyProposalAdded(proposal);

        final resolvedLocationId = await _repository.confirmLocation(
          experienceId: experienceId,
          proposalId: proposal.id,
        );
        if (!ref.mounted) return;
        state = AsyncData(state.requireValue.copyWith(
          lockedProposalId: proposal.id,
          mode: 'confirmed',
          locationPollActive: false,
          locationPollCompleted: true,
          eventLocationId: resolvedLocationId,
        ));
        _syncExperience((exp) {
          exp.locationPollActive = false;
          exp.locationPollCompleted = true;
          exp.locationId = resolvedLocationId;
          for (final p in exp.locationProposals) {
            if (p.id == proposal.id) {
              p.isConfirmed = true;
              p.location.locationId = resolvedLocationId;
            } else {
              p.isConfirmed = false;
            }
          }
        });
      } catch (e) {
        _log.warning('Failed to set single location: $e');
        if (!ref.mounted) return;
        state = AsyncData(previous);
        rethrow;
      }
    }
  }

  /// Adds multiple proposals sequentially. Sequential rather than parallel:
  /// the server emits a "poll opened" system message only when the
  /// location-poll flag flips from false to true. Parallel requests would
  /// race and emit duplicate messages.
  Future<void> proposeLocationsSequentially(List<String> locationIds) async {
    if (locationIds.isEmpty) return;
    try {
      for (final id in locationIds) {
        final proposal = await _repository.proposeLocation(
          experienceId: experienceId,
          locationId: id,
        );
        if (!ref.mounted) return;
        _applyProposalAdded(proposal);
      }
    } catch (e) {
      _log.warning('Failed to add locations to poll: $e');
      rethrow;
    }
  }

  /// Adds multiple drop-pin / search-result proposals sequentially. Mirrors
  /// [proposeLocationsSequentially] for the geocoded path.
  Future<void> proposeGeocodedLocationsSequentially(
    List<GeocodedLocation> geocoded,
  ) async {
    if (geocoded.isEmpty) return;
    try {
      for (final g in geocoded) {
        final proposal = await _repository.proposeLocation(
          experienceId: experienceId,
          geocoded: g,
        );
        if (!ref.mounted) return;
        _applyProposalAdded(proposal);
      }
    } catch (e) {
      _log.warning('Failed to add geocoded locations to poll: $e');
      rethrow;
    }
  }

  /// Updates state + the cached experience proto with the new proposal.
  /// Replaces the previous `ref.invalidateSelf()`-then-refetch pattern.
  void _applyProposalAdded(LocationProposal proposal) {
    final previous = state.requireValue;
    final newPollId = proposal.hasPollId()
        ? proposal.pollId
        : previous.currentLocationPollId;
    state = AsyncData(previous.copyWith(
      proposals: [...previous.proposals, proposal],
      locationPollActive: true,
      currentLocationPollId: newPollId,
    ));
    _syncExperience((exp) {
      exp.locationProposals.add(proposal);
      exp.locationPollActive = true;
      if (proposal.hasPollId()) {
        exp.currentLocationPollId = proposal.pollId;
      }
    });
  }

  /// Confirms a winning proposal as the final location (owner only).
  ///
  /// The optimistic write only carries the saved [locationId] when the
  /// winning proposal already references one; if the winner is an inline
  /// geocoded drop-pin, the canonical `location_id` only exists after the
  /// server materializes it. We backfill `eventLocationId` with the value
  /// returned from `ConfirmLocation` so the finalized modal's map and
  /// address card resolve against the real saved location.
  Future<void> confirmLocation(String proposalId) async {
    final previous = state.requireValue;
    final winningProposal = previous.proposals
        .where((p) => p.id == proposalId)
        .firstOrNull;
    final preliminaryLocationId = winningProposal?.location.locationId;
    state = AsyncData(previous.copyWith(
      lockedProposalId: proposalId,
      mode: 'confirmed',
      locationPollActive: false,
      locationPollCompleted: true,
      eventLocationId: preliminaryLocationId?.isNotEmpty ?? false
          ? preliminaryLocationId
          : previous.eventLocationId,
    ));
    try {
      final resolvedLocationId = await _repository.confirmLocation(
        experienceId: experienceId,
        proposalId: proposalId,
      );
      if (!ref.mounted) return;
      // Server response carries the canonical location_id — either the
      // existing reference or a freshly-materialized one for an inline
      // geocoded winner. Use it so a drop-pin winner doesn't leave the
      // finalized modal without a map.
      state = AsyncData(state.requireValue.copyWith(
        eventLocationId: resolvedLocationId,
      ));
      _syncExperience((exp) {
        exp.locationPollActive = false;
        exp.locationPollCompleted = true;
        exp.locationId = resolvedLocationId;
        for (final p in exp.locationProposals) {
          if (p.id == proposalId) {
            p.isConfirmed = true;
            // Patch the proposal's location_id too so subsequent reads
            // (e.g. _resolveWinner in the finalized modal) match the
            // experience's locationId.
            p.location.locationId = resolvedLocationId;
          } else {
            p.isConfirmed = false;
          }
        }
      });
    } catch (e) {
      _log.warning('Failed to confirm location: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Unlocks the confirmed location (owner only).
  Future<void> unlockLocation() async {
    final previous = state.requireValue;
    state = AsyncData(previous.copyWith(
      lockedProposalId: null,
      mode: 'default',
    ));
    try {
      await _repository.unlockLocation(experienceId: experienceId);
      if (!ref.mounted) return;
      _syncExperience((exp) {
        for (final p in exp.locationProposals) {
          p.isConfirmed = false;
        }
      });
    } catch (e) {
      _log.warning('Failed to unlock location: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Deletes a single proposal from the active poll (owner can delete any;
  /// participants only their own).
  Future<void> deleteProposal(String proposalId) async {
    final previous = state.requireValue;
    final remaining =
        previous.proposals.where((p) => p.id != proposalId).toList();
    final remainingYes = {
      ...previous.currentUserYesProposalIds,
    }..remove(proposalId);
    state = AsyncData(previous.copyWith(
      proposals: remaining,
      currentUserYesProposalIds: remainingYes,
    ));
    try {
      await _repository.deleteLocationProposal(
        experienceId: experienceId,
        proposalId: proposalId,
      );
      if (!ref.mounted) return;
      _syncExperience((exp) {
        exp.locationProposals
            .removeWhere((p) => p.id == proposalId);
      });
    } catch (e) {
      _log.warning('Failed to delete proposal: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Ends the active location poll (owner only). The server clears the
  /// poll flags entirely — `locationPollCompleted`, `currentLocationPollId`,
  /// the deadline, and the proposal-lock — so the next tap on the location
  /// row lands on the fresh propose flow (not on the "owner picks a winner"
  /// confirm flow which is reserved for natural-deadline completions).
  /// Proposals themselves are preserved for history; the propose modal
  /// scopes its existing-options list to `locationPollActive`, so they
  /// won't bleed into the next poll.
  Future<void> endLocationPoll() async {
    final previous = state.requireValue;
    state = AsyncData(previous.copyWith(
      locationPollActive: false,
      locationPollCompleted: false,
      currentLocationPollId: null,
      locationPollDeadlineUnixSec: null,
      locationProposalsLocked: false,
    ));
    try {
      await _repository.cancelLocationPoll(experienceId: experienceId);
      if (!ref.mounted) return;
      _syncExperience((exp) {
        exp.locationPollActive = false;
        exp.clearLocationPollCompleted();
        exp.clearCurrentLocationPollId();
        exp.clearLocationPollDeadlineUnixSec();
        exp.clearLocationProposalsLocked();
      });
    } catch (e) {
      _log.warning('Failed to end location poll: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Sets (or clears) the reply-by deadline on the active poll. Passing
  /// `null` clears the deadline server-side.
  Future<void> setLocationPollDeadline(int? deadlineUnixSec) async {
    final previous = state.requireValue;
    state = AsyncData(previous.copyWith(
      locationPollDeadlineUnixSec: deadlineUnixSec,
    ));
    try {
      await _repository.setLocationPollDeadline(
        experienceId: experienceId,
        deadlineUnixSec: deadlineUnixSec ?? 0,
      );
      if (!ref.mounted) return;
      _syncExperience((exp) {
        if (deadlineUnixSec == null) {
          exp.clearLocationPollDeadlineUnixSec();
        } else {
          exp.locationPollDeadlineUnixSec = Int64(deadlineUnixSec);
        }
      });
    } catch (e) {
      _log.warning('Failed to set location-poll deadline: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Locks or unlocks the proposal list on the active poll.
  Future<void> setLocationProposalsLocked(bool locked) async {
    final previous = state.requireValue;
    state = AsyncData(previous.copyWith(locationProposalsLocked: locked));
    try {
      await _repository.lockLocationProposals(
        experienceId: experienceId,
        locked: locked,
      );
      if (!ref.mounted) return;
      _syncExperience((exp) {
        exp.locationProposalsLocked = locked;
      });
    } catch (e) {
      _log.warning('Failed to update lock state: $e');
      if (!ref.mounted) return;
      state = AsyncData(previous);
      rethrow;
    }
  }

  /// Applies a vote change to the cached experience proto. Used by
  /// [voteOnLocation] / [voteOnLocations] so the Plan tab and other
  /// surfaces reflect the vote without a server refetch.
  void _applyVoteToProto(
    Experience exp, {
    required String proposalId,
    required User? voter,
    required LocationVoteStatus newStatus,
  }) {
    if (voter == null || voter.id.isEmpty) return;
    for (final p in exp.locationProposals) {
      if (p.id != proposalId) continue;
      p.votes.removeWhere((v) => v.user.id == voter.id);
      if (newStatus != LocationVoteStatus.LOCATION_VOTE_STATUS_UNSPECIFIED) {
        final vote = LocationVote()
          ..status = newStatus
          ..user = voter;
        p.votes.add(vote);
      }
    }
  }

  /// Returns a new proposals list with the voter's vote replaced. Used by
  /// the optimistic-UI path: cloning the affected proposal (and copying
  /// the list) leaves [LocationModalData.proposals] from the previous
  /// snapshot untouched, so a rollback on RPC failure restores the exact
  /// pre-vote avatar stack and count.
  List<LocationProposal> _projectVote(
    List<LocationProposal> proposals, {
    required String proposalId,
    required User? voter,
    required LocationVoteStatus newStatus,
  }) {
    if (voter == null || voter.id.isEmpty) return proposals;
    return proposals.map((p) {
      if (p.id != proposalId) return p;
      final cloned = p.deepCopy();
      cloned.votes.removeWhere((v) => v.user.id == voter.id);
      if (newStatus != LocationVoteStatus.LOCATION_VOTE_STATUS_UNSPECIFIED) {
        final vote = LocationVote()
          ..status = newStatus
          ..user = voter;
        cloned.votes.add(vote);
      }
      return cloned;
    }).toList();
  }

  /// Pings RSVPs who have not voted on the active poll. Returns the count.
  Future<int> nudgeUnreplied() async {
    return _repository.nudgeLocationPollVoters(experienceId: experienceId);
  }

  /// Bulk-paste candidate extraction. Returns geocoded candidates.
  Future<List<GeocodedLocation>> extractLocationCandidates(String text) {
    return _repository.extractLocationCandidates(text: text);
  }
}

/// Provider for the location modal view model. Use:
/// ```dart
/// final dataAsync = ref.watch(locationModalProvider(experienceId));
/// ```
final locationModalProvider =
    AsyncNotifierProvider.family<LocationModalNotifier, LocationModalData, String>(
  LocationModalNotifier.new,
);
