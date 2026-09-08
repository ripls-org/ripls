import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/impact_estimate.pb.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/provisional_user_repository.dart'
    show ProvisionalUser;

export 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
export 'package:ripls/data/repositories/provisional_user_repository.dart'
    show ProvisionalUser;

part 'event_modal_state.freezed.dart';

/// State for EventModalViewModel.
///
/// Manages the state of experience RSVP/time coordination modals
/// with separate workflows for organizers and joiners.
@freezed
sealed class EventModalState with _$EventModalState {
  const factory EventModalState({
    // Core data
    Experience? experience,
    @Default([]) List<RSVP> rsvps,
    @Default([]) List<TimeProposal> timeProposals,
    String? locationName,

    // Phase management
    @Default(0) int currentPhase,
    @Default(false) bool isOrganizer,

    // Time proposal state
    @Default(false) bool timePollActive,
    String? selectedProposalId,

    // RSVP state (joiner)
    RSVPIntention? currentUserIntention,
    @Default({}) Map<String, bool> votedTimes, // proposalId -> voted
    // Wrap-up state (organizer)
    String? completionSummary,
    @Default({}) Map<String, bool> attendanceMap, // userId -> attended
    /// Provisional users added as attendees during wrap-up (not in RSVP list).
    @Default([]) List<ProvisionalUser> provisionalAttendees,
    @Default({})
    Map<String, bool> provisionalAttendanceMap, // provisionalUserId -> attended
    /// Registered members added via the search sheet (not in RSVP list).
    @Default([]) List<User> extraMemberAttendees,

    // Impact metrics (populated after completion)
    ImpactEstimate? impactEstimate,

    // Loading/error
    @Default(true) bool isLoading,
    // True only while an RSVP submission is in flight. Unlike isLoading, this
    // does not replace the entire modal body — it only disables the RSVP buttons.
    @Default(false) bool isSubmittingRsvp,
    UserError? error,
  }) = _EventModalState;

  const EventModalState._();

  bool get hasError => error != null;

  bool get hasExperience => experience != null;
  
  
  
  
  }
