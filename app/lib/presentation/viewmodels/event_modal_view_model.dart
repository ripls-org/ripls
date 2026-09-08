import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart' show RSVP;
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/data/repositories/provisional_user_repository.dart'
    show ProvisionalUser, ProvisionalUserRepository;
import 'package:ripls/presentation/viewmodels/event_modal_state.dart';
import 'package:ripls/presentation/viewmodels/impact_draft_notifier.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('EventModalViewModel');

/// Provider for the EventModalNotifier.
///
/// This is an autoDispose family provider that manages experience modals
/// (both organizer and joiner perspectives). The family parameter is the
/// experience ID.
final eventModalProvider = NotifierProvider.autoDispose
    .family<EventModalNotifier, EventModalState, String>(
      EventModalNotifier.new,
    );

/// ViewModel for managing experience RSVP/time coordination modals.
///
/// Handles both organizer and joiner workflows with phase-based progression.
/// Manages time proposals, voting, RSVP, and completion with proper
/// disposal safety checks after async operations.
class EventModalNotifier extends Notifier<EventModalState>
    with SafeNotifierMixin<EventModalState> {
  /// Constructor accepts the experienceId parameter from the family modifier
  EventModalNotifier(this._experienceId);

  /// The experienceId for this specific experience instance
  final String _experienceId;

  ExperienceRepository get _repository =>
      ref.read(experienceRepositoryProvider);
  LocationRepository get _locationRepository =>
      ref.read(locationRepositoryProvider);
  ProvisionalUserRepository get _provisionalRepo =>
      ref.read(provisionalUserRepositoryProvider);

  @override
  EventModalState build() {
    ref.onDispose(() => _previewTimer?.cancel());
    return const EventModalState();
  }

  /// The community ID for this experience context (set via [initialize]).
  String _communityId = '';

  /// Debounce timer for live QT preview calls.
  Timer? _previewTimer;

  /// Initializes the ViewModel by loading the experience details.
  ///
  /// [communityId] provides the community context for conversation lookup
  /// and summary generation.
  /// [preTaggedUsers] are participant User objects resolved from AI-mentioned
  /// names before the experience was created. They are added to
  /// [extraMemberAttendees] and the attendance map as attended.
  /// Determines if current user is organizer, loads RSVPs and time proposals,
  /// and derives the current phase from the experience state.
  Future<void> initialize({
    String communityId = '',
    List<User> preTaggedUsers = const [],
    List<ProvisionalUser> preProvisionalUsers = const [],
  }) async {
    _communityId = communityId;
    _log.info(
      'Initializing event modal for experience: $_experienceId, community: $_communityId',
    );

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      final response = await _repository.getExperienceDetails(
        _experienceId,
        communityId: communityId.isNotEmpty ? communityId : null,
      );

      if (!ref.mounted) return;

      final experience = response.experience;
      final authState = ref.read(authStateProvider);
      final currentUserId = authState.user?.id;

      // Determine if current user is the organizer
      final isOrganizer = experience.owner.id == currentUserId;

      // Determine current phase based on experience state
      final currentPhase = _determinePhase(experience, isOrganizer);

      // Extract current user's RSVP intention
      RSVPIntention? currentUserIntention;
      final currentUserRsvp = response.rsvps
          .where((rsvp) => rsvp.user.id == currentUserId)
          .firstOrNull;
      if (currentUserRsvp != null) {
        currentUserIntention = currentUserRsvp.intention;
      }

      // Load location name if available
      String? locationName;
      if (experience.locationId.isNotEmpty) {
        try {
          final location = await _locationRepository.getLocation(
            experience.locationId,
          );
          if (!ref.mounted) return;
          locationName = location.name;
        } catch (e) {
          _log.warning('Failed to load location name: $e');
        }
      }

      // For completed experiences, restore summary and attendance from server data.
      // For non-completed experiences, default all YES RSVPs to attended so the
      // wrap-up modal starts with sensible defaults.
      String? completionSummary;
      final Map<String, bool> attendanceMap = {};
      final isComplete =
          experience.state == ExperienceState.EXPERIENCE_STATE_COMPLETED;

      if (isComplete) {
        // Load completion summary from experience proto
        if (experience.hasCompletionSummary() &&
            experience.completionSummary.isNotEmpty) {
          completionSummary = experience.completionSummary;
        }

        // Rebuild attendance map from RSVP attended status
        for (final rsvp in response.rsvps) {
          if (rsvp.attended == AttendedStatus.ATTENDED_STATUS_YES) {
            attendanceMap[rsvp.user.id] = true;
          } else if (rsvp.attended == AttendedStatus.ATTENDED_STATUS_NO) {
            attendanceMap[rsvp.user.id] = false;
          }
        }
      } else {
        // Default all YES RSVPs to attended for wrap-up
        for (final rsvp in response.rsvps) {
          if (rsvp.intention == RSVPIntention.RSVP_INTENTION_YES) {
            attendanceMap[rsvp.user.id] = true;
          }
        }
        // Pre-tag participants resolved from AI-mentioned names (past events).
        // These may not have RSVPs yet; add them to extraMemberAttendees so
        // they appear in the wrap-up list and mark them attended.
        for (final user in preTaggedUsers) {
          if (!response.rsvps.any((r) => r.user.id == user.id)) {
            attendanceMap[user.id] = true;
          }
        }
      }

      // Seed pre-tagged member attendees (not already in RSVPs).
      final seedExtraMembers = preTaggedUsers
          .where((u) => !response.rsvps.any((r) => r.user.id == u.id))
          .toList();

      // Seed pre-resolved provisional users from AI-mentioned name resolution.
      final seedProvisionalAttendees = List<ProvisionalUser>.from(
        preProvisionalUsers,
      );
      final seedProvisionalMap = <String, bool>{
        for (final s in preProvisionalUsers) s.id: true,
      };

      safeUpdateState(
        (s) => s.copyWith(
          experience: experience,
          rsvps: response.rsvps,
          timeProposals: experience.timeProposals,
          timePollActive: experience.timePollActive,
          locationName: locationName,
          currentPhase: currentPhase,
          isOrganizer: isOrganizer,
          currentUserIntention: currentUserIntention,
          completionSummary: completionSummary,
          extraMemberAttendees: seedExtraMembers,
          provisionalAttendees: seedProvisionalAttendees,
          provisionalAttendanceMap: seedProvisionalMap,
          attendanceMap: attendanceMap,
          isLoading: false,
        ),
      );

      // Load impact data: for active/in-process experiences use a live preview
      // based on current attendance defaults (all YES RSVPs confirmed); for
      // completed experiences use the stored final impact from stats.
      try {
        final isComplete =
            experience.state == ExperienceState.EXPERIENCE_STATE_COMPLETED;
        if (isComplete) {
          final stats = await _repository.getStats(
            _experienceId,
            communityId: experience.communityId,
          );
          if (!ref.mounted) return;
          final impact = stats.hasImpact() ? stats.impact : null;
          safeUpdateState((s) => s.copyWith(impactEstimate: impact));
        } else {
          // For the completion modal, start with a live preview using the
          // default attendance (all YES RSVPs confirmed).
          await previewImpact();
        }
      } catch (e) {
        _log.warning('Failed to load initial impact: $e');
      }

      _log.info('Initialized: phase=$currentPhase, isOrganizer=$isOrganizer');
    } catch (e, stackTrace) {
      _log.severe('Failed to initialize event modal', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState(
        (s) => s.copyWith(isLoading: false, error: RpcErrorHandler.classify(e)),
      );
    }
  }

  /// Determines the current phase based on experience state and user role.
  ///
  /// Organizer phases: Planning (0), Wrap Up (1), Complete (2)
  /// Joiner phases: RSVP (0), Complete (1)
  int _determinePhase(Experience experience, bool isOrganizer) {
    final state = experience.state;

    if (state == ExperienceState.EXPERIENCE_STATE_COMPLETED) {
      // Organizer: step 2 (complete), Joiner: step 1 (complete)
      return isOrganizer ? 2 : 1;
    }

    if (state == ExperienceState.EXPERIENCE_STATE_IN_PROCESS) {
      // Organizer: step 0 (planning) or 1 (wrap up)
      // For now, default to wrap up (1) when IN_PROCESS
      // Joiner stays in RSVP (0)
      return isOrganizer ? 1 : 0;
    }

    if (state == ExperienceState.EXPERIENCE_STATE_ACTIVE ||
        state == ExperienceState.EXPERIENCE_STATE_JOINED) {
      // Both stay in planning/RSVP phase (0)
      // Locked time state is shown via WhenRow appearance, not separate phase
      return 0;
    }

    return 0; // Default to first phase
  }

  /// Schedules a debounced live QT preview call (300 ms).
  ///
  /// Called after any attendance toggle so the impact bar in the completion
  /// modal reflects the current confirmed-attendee set without hammering the server.
  void _schedulePreviewImpact() {
    _previewTimer?.cancel();
    _previewTimer = Timer(const Duration(milliseconds: 250), previewImpact);
  }

  /// previewImpact calls the server's PreviewExperienceImpact RPC and updates
  /// [state.impactEstimate] with the real-formula result for the current
  /// confirmed attendee set. Called debounced on every attendance toggle.
  Future<void> previewImpact() async {
    // Collect confirmed registered-user IDs from all three attendee lists.
    final confirmedIds = <String>[
      for (final entry in state.attendanceMap.entries)
        if (entry.value) entry.key,
    ];

    // Total count includes provisional users.
    final provisionalCount = state.provisionalAttendanceMap.values
        .where((v) => v)
        .length;
    final totalCount = confirmedIds.length + provisionalCount;

    try {
      final impact = await _repository.previewExperienceImpact(
        experienceId: _experienceId,
        confirmedAttendeeIds: confirmedIds,
        confirmedAttendeeCount: totalCount,
      );
      if (!ref.mounted) return;
      safeUpdateState((s) => s.copyWith(impactEstimate: impact));

      // Mirror the live-preview result into the per-metric draft so the
      // QT/Money/CO₂ detail modals see the same group size and composites
      // the user just saw on the top tile bar. The draft notifier no-ops
      // when the user has any active override — their explicit edits stay
      // authoritative.
      ref
          .read(impactDraftProvider(_experienceId).notifier)
          .applyExternalDraft(impact);
    } catch (e) {
      // Preview failure is non-critical: keep showing the last known estimate.
      _log.warning('Live QT preview failed: $e');
    }
  }

  /// Proposes a new time for the experience.
  Future<void> proposeTime(ExperienceTime time) async {
    _log.info('Proposing time for experience: $_experienceId');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      await _repository.proposeTime(experienceId: _experienceId, time: time);

      if (!ref.mounted) return;

      // Refresh experience to get updated proposals
      await _refreshExperience();
    } catch (e, stackTrace) {
      _log.severe('Failed to propose time', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          error: RpcErrorHandler.classify(
            e,
            fallback: 'Could not propose time',
          ),
        ),
      );
    }
  }

  /// Votes on a time proposal.
  Future<void> voteOnTime(String proposalId, TimeVoteStatus status) async {
    _log.info('Voting on proposal: $proposalId, status: $status');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      await _repository.voteOnTime(
        proposalId: proposalId,
        status: status,
        experienceId: _experienceId,
      );

      if (!ref.mounted) return;

      // Mark as voted in local state
      final votedTimes = Map<String, bool>.from(state.votedTimes);
      votedTimes[proposalId] = true;

      safeUpdateState((s) => s.copyWith(votedTimes: votedTimes));

      // Refresh experience to get updated votes
      await _refreshExperience();
    } catch (e, stackTrace) {
      _log.severe('Failed to vote on time', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          error: RpcErrorHandler.classify(
            e,
            fallback: 'Could not vote on time',
          ),
        ),
      );
    }
  }

  /// Confirms a time proposal as the final time (organizer only).
  Future<void> confirmTime(String proposalId) async {
    _log.info('Confirming time proposal: $proposalId');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      await _repository.confirmTime(
        experienceId: _experienceId,
        proposalId: proposalId,
      );

      if (!ref.mounted) return;

      // Refresh experience to get updated state
      await _refreshExperience();

      // Update phase to confirmed
      safeUpdateState((s) => s.copyWith(currentPhase: 1));
    } catch (e, stackTrace) {
      _log.severe('Failed to confirm time', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          error: RpcErrorHandler.classify(
            e,
            fallback: 'Could not confirm time',
          ),
        ),
      );
    }
  }

  /// Submits an RSVP for the current user.
  ///
  /// No-ops when the intention matches the already-recorded value to prevent
  /// duplicate server calls and system chat messages. Uses [isSubmittingRsvp]
  /// instead of [isLoading] so the modal body remains visible during the call.
  /// Applies an optimistic update to [state.rsvps] immediately — inserting a
  /// new entry for first-time RSVPs or rewriting the existing entry — then
  /// replaces with the authoritative server data once the refresh completes.
  /// Rolls back both [state.rsvps] and [state.currentUserIntention] on failure.
  Future<void> submitRsvp(RSVPIntention intention, String communityId) async {
    // Guard: skip if the intention hasn't changed.
    if (state.currentUserIntention == intention) {
      _log.fine('RSVP intention unchanged, skipping submission');
      return;
    }

    _log.info('Submitting RSVP: $intention');

    safeUpdateState((s) => s.copyWith(isSubmittingRsvp: true, error: null));

    // Snapshot before optimistic mutation for symmetric rollback on failure.
    final previousRsvps = state.rsvps;
    final previousIntention = state.currentUserIntention;

    // Optimistic update: reflect the new intention in the RSVP list immediately
    // so the responses section updates without waiting for the server round-trip.
    final currentUser = ref.read(authStateProvider).user;
    final currentUserId = currentUser?.id ?? '';
    final alreadyHasEntry = state.rsvps.any(
      (rsvp) => rsvp.user.id == currentUserId,
    );

    final List<RSVP> optimisticRsvps;
    if (alreadyHasEntry) {
      optimisticRsvps = state.rsvps.map((rsvp) {
        if (rsvp.user.id == currentUserId) {
          return RSVP(
            user: rsvp.user,
            intention: intention,
            attended: rsvp.attended,
            rsvpedAtUnixSec: rsvp.rsvpedAtUnixSec,
            lastUpdatedUnixSec: rsvp.lastUpdatedUnixSec,
          );
        }
        return rsvp;
      }).toList();
    } else if (currentUser != null) {
      // First-time RSVP: append a new entry so the responses section
      // renders immediately without waiting for the server round-trip.
      optimisticRsvps = [
        ...state.rsvps,
        RSVP(user: currentUser, intention: intention),
      ];
    } else {
      optimisticRsvps = state.rsvps;
    }

    safeUpdateState(
      (s) =>
          s.copyWith(currentUserIntention: intention, rsvps: optimisticRsvps),
    );

    try {
      await _repository.rsvp(
        experienceId: _experienceId,
        communityId: communityId,
        intention: intention,
      );

      if (!ref.mounted) return;

      // Refresh to get the authoritative server data.
      await _refreshExperience();
    } catch (e, stackTrace) {
      _log.severe('Failed to submit RSVP', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState(
        (s) => s.copyWith(
          isSubmittingRsvp: false,
          currentUserIntention: previousIntention,
          rsvps: previousRsvps,
          error: RpcErrorHandler.classify(e, fallback: 'Could not submit RSVP'),
        ),
      );
    }
  }

  /// Updates the experience location (organizer only, before confirmation).
  Future<void> updateLocation(String locationId) async {
    _log.info('Updating experience location');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      await _repository.saveExperience(
        id: _experienceId,
        locationId: locationId,
      );

      if (!ref.mounted) return;

      // Refresh experience to get updated location
      await _refreshExperience();
    } catch (e, stackTrace) {
      _log.severe('Failed to update location', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          error: RpcErrorHandler.classify(
            e,
            fallback: 'Could not update location',
          ),
        ),
      );
    }
  }

  
  /// Marks the experience as in process (organizer only).
  Future<void> markInProcess() async {
    _log.info('Marking experience as in process');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      await _repository.markInProcess(_experienceId);

      if (!ref.mounted) return;

      // Refresh experience to get updated state
      await _refreshExperience();
    } catch (e, stackTrace) {
      _log.severe('Failed to mark experience in process', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          error: RpcErrorHandler.classify(
            e,
            fallback: 'Could not start the event',
          ),
        ),
      );
    }
  }

  
  /// Sets the completion summary (organizer wrap-up).
  ///
  /// The recap is whatever the host types. It starts empty and stays empty
  /// until they write something — nothing is drafted on their behalf (#2936).
  void setCompletionSummary(String summary) {
    safeUpdateState((s) => s.copyWith(completionSummary: summary));
  }

  /// Toggles attendance for a user (organizer wrap-up).
  void toggleAttendance(String userId) {
    final attendanceMap = Map<String, bool>.from(state.attendanceMap);
    attendanceMap[userId] = !(attendanceMap[userId] ?? false);
    safeUpdateState((s) => s.copyWith(attendanceMap: attendanceMap));
    _schedulePreviewImpact();
  }

  
  /// Creates or finds a provisional user by name and adds them to the wrap-up
  /// attendance list as attended.
  ///
  /// Returns the created/found [ProvisionalUser], or null on failure.
  Future<ProvisionalUser?> addProvisionalAttendee({
    required String communityId,
    required String name,
  }) async {
    if (name.trim().isEmpty) return null;
    try {
      final prov = await _provisionalRepo.createProvisionalUser(
        communityId: communityId,
        name: name.trim(),
      );
      if (!ref.mounted) return null;
      final updated = [...state.provisionalAttendees, prov];
      final map = Map<String, bool>.from(state.provisionalAttendanceMap)
        ..[prov.id] = true;
      safeUpdateState(
        (s) => s.copyWith(
          provisionalAttendees: updated,
          provisionalAttendanceMap: map,
        ),
      );
      _schedulePreviewImpact();
      return prov;
    } catch (e, st) {
      _log.severe('Failed to add provisional attendee', e, st);
      if (!ref.mounted) return null;
      safeUpdateState((s) => s.copyWith(error: RpcErrorHandler.classify(e)));
      return null;
    }
  }

  /// Toggles attendance for a provisional user (organizer wrap-up).
  void toggleProvisionalAttendance(String provisionalUserId) {
    final map = Map<String, bool>.from(state.provisionalAttendanceMap);
    map[provisionalUserId] = !(map[provisionalUserId] ?? false);
    safeUpdateState((s) => s.copyWith(provisionalAttendanceMap: map));
    _schedulePreviewImpact();
  }

  /// Adds an existing provisional user to the wrap-up attendance list as attended.
  ///
  /// No-op if the provisional user is already in the list.
  void addExistingProvisionalAttendee(ProvisionalUser prov) {
    if (state.provisionalAttendees.any((s) => s.id == prov.id)) return;
    final updated = [...state.provisionalAttendees, prov];
    final map = Map<String, bool>.from(state.provisionalAttendanceMap)
      ..[prov.id] = true;
    safeUpdateState(
      (s) => s.copyWith(
        provisionalAttendees: updated,
        provisionalAttendanceMap: map,
      ),
    );
    _schedulePreviewImpact();
  }

  /// Adds a registered member to the wrap-up attendance list as attended.
  ///
  /// No-op if the user is already in the RSVP list or extra attendees list.
  void addMemberAttendee(User user) {
    if (state.rsvps.any((r) => r.user.id == user.id)) return;
    if (state.extraMemberAttendees.any((u) => u.id == user.id)) return;
    final updated = [...state.extraMemberAttendees, user];
    final map = Map<String, bool>.from(state.attendanceMap)..[user.id] = true;
    safeUpdateState(
      (s) => s.copyWith(extraMemberAttendees: updated, attendanceMap: map),
    );
    _schedulePreviewImpact();
  }

  
  /// Completes the experience with attendance (organizer only). Returns
  /// the community_event_id for undo wiring, or null on error.
  Future<String?> wrapUp() async {
    _log.info('Completing experience with attendance');

    safeUpdateState((s) => s.copyWith(isLoading: true, error: null));

    try {
      // Record attendance first so completeExperience uses the correct attendee
      // count when computing impact (not auto-marked RSVPs).
      final attendanceRecords = [
        ...state.attendanceMap.entries.map(
          (e) => AttendanceRecord(
            userId: e.key,
            attended: e.value
                ? AttendedStatus.ATTENDED_STATUS_YES
                : AttendedStatus.ATTENDED_STATUS_NO,
          ),
        ),
        ...state.provisionalAttendanceMap.entries.map(
          (e) => AttendanceRecord(
            provisionalUserId: e.key,
            attended: e.value
                ? AttendedStatus.ATTENDED_STATUS_YES
                : AttendedStatus.ATTENDED_STATUS_NO,
          ),
        ),
      ];

      if (attendanceRecords.isNotEmpty && state.experience != null) {
        final communityId = state.experience!.communityId;
        if (communityId.isNotEmpty) {
          await _repository.recordAttendance(
            experienceId: _experienceId,
            communityId: communityId,
            attendance: attendanceRecords,
          );
        }
      }

      if (!ref.mounted) return null;

      // Complete the experience with optional summary and any user-supplied overrides.
      final summary = state.completionSummary;
      final draftState = ref.read(impactDraftProvider(_experienceId));
      // Send the exact confirmed set the modal previewed with (same
      // derivation as [previewImpact]) so the committed impact matches the
      // previewed numbers (#2724).
      final confirmedIds = <String>[
        for (final entry in state.attendanceMap.entries)
          if (entry.value) entry.key,
      ];
      final provisionalCount = state.provisionalAttendanceMap.values
          .where((v) => v)
          .length;
      final response = await _repository.completeExperience(
        _experienceId,
        summary: (summary != null && summary.trim().isNotEmpty)
            ? summary
            : null,
        confirmedAttendeeIds: confirmedIds,
        confirmedAttendeeCount: confirmedIds.length + provisionalCount,
        qualityTimeOverrides: draftState.qualityTimeOverrides,
        moneySavingsOverrides: draftState.moneySavingsOverrides,
        emissionsOverrides: draftState.emissionsOverrides,
      );

      if (!ref.mounted) return response.communityEventId;

      // Refresh experience to get completion data
      await _refreshExperience();

      // Update to complete and store impact data
      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          error: null,
          currentPhase: 2,
          impactEstimate: response.impact,
        ),
      );
      return response.communityEventId;
    } catch (e, stackTrace) {
      _log.severe('Failed to complete experience', e, stackTrace);
      if (!ref.mounted) return null;

      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          error: RpcErrorHandler.classify(
            e,
            fallback: 'Could not complete the event',
          ),
        ),
      );
      return null;
    }
  }

  /// Refreshes the experience data from the repository.
  ///
  /// This is useful after external changes (e.g., time modal updates) to
  /// ensure the modal displays the latest data.
  Future<void> refresh() async {
    await _refreshExperience();
  }

  /// Internal method to refresh the experience data from the repository.
  Future<void> _refreshExperience() async {
    try {
      final response = await _repository.getExperienceDetails(
        _experienceId,
        communityId: _communityId.isNotEmpty ? _communityId : null,
      );

      if (!ref.mounted) return;

      // Reload location name if locationId changed
      final newLocationId = response.experience.locationId;
      final currentLocationId = state.experience?.locationId ?? '';
      String? locationName = state.locationName;

      if (newLocationId != currentLocationId && newLocationId.isNotEmpty) {
        try {
          final location = await _locationRepository.getLocation(newLocationId);
          if (!ref.mounted) return;
          locationName = location.name;
        } catch (e) {
          _log.warning('Failed to load location name: $e');
        }
      }

      safeUpdateState(
        (s) => s.copyWith(
          experience: response.experience,
          rsvps: response.rsvps,
          timeProposals: response.experience.timeProposals,
          locationName: locationName,
          isLoading: false,
          isSubmittingRsvp: false,
        ),
      );
    } catch (e, stackTrace) {
      _log.severe('Failed to refresh experience', e, stackTrace);
      if (!ref.mounted) return;

      safeUpdateState(
        (s) => s.copyWith(
          isLoading: false,
          isSubmittingRsvp: false,
          error: RpcErrorHandler.classify(e),
        ),
      );
    }
  }
}
