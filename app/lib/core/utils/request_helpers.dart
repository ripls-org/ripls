import 'package:flutter/material.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request, RequestState;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferType;

/// Returns a human-readable label combining request state.
///
/// This helper converts the request state into a descriptive label for
/// consistent display across the UI.
///
/// Examples:
/// - ACTIVE = "Request Active"
/// - OFFERS_RECEIVED = "Request Offered"
/// - FULFILLED = "Request Fulfilled"
/// - CANCELLED = "Request Cancelled"
///
/// The function handles unknown states gracefully by falling back to "Request".
String getRequestStateLabel(Request request) {
  switch (request.state) {
    case RequestState.REQUEST_STATE_ACTIVE:
      return 'Request Active';

    case RequestState.REQUEST_STATE_OFFERS_RECEIVED:
      return 'Request Help Offered';

    case RequestState.REQUEST_STATE_FULFILLED:
      return 'Request Fulfilled ✓';

    case RequestState.REQUEST_STATE_CANCELLED:
      return 'Request Cancelled';

    default:
      // Fallback for unknown states
      return 'Request';
  }
}

// ============================================================================
// Transfer Request Count Labels
// ============================================================================

/// Generates chip label and icon for gear transfer inquiry status.
///
/// This helper provides centralized label generation for transfer inquiries
/// across the UI to ensure consistency. Uses a unified "inquiry" pattern
/// regardless of transfer type (loan vs giveaway).
///
/// Matches RSVP chip pattern: all users see the count in parentheses.
/// The count shows the number of OTHER participants (excluding the owner).
///
/// Examples:
/// - Owner with 4 other participants: {label: "INQUIRIES (4)", icon: Icons.check_circle}
/// - Owner with 0 inquiries: {label: "INQUIRIES (0)", icon: Icons.circle_outlined}
/// - Non-owner with inquiry: {label: "INQUIRIES (3)", icon: Icons.check_circle}
/// - Non-owner without inquiry: {label: "INQUIRIES (3)", icon: Icons.circle_outlined}
///
/// The [isParticipant] parameter indicates whether the current user has already
/// expressed interest (for giveaways) or made a request (for loans).
///
/// The [participantCount] is the total number of participants including the owner.
/// The displayed count excludes the owner to show "other" participants.
({String label, IconData icon}) getTransferChipLabel({
  required bool isOwner,
  required Availability availability,
  int? requestCount,
  bool? hasActiveRequest,
  int? participantCount,
  bool? isParticipant,
}) {
  // Use requestCount which represents active interests (pendingRequests.length)
  // NOT participantCount which includes all users in the modal
  final count = requestCount ?? 0;

  if (isOwner) {
    // Owner: use check_circle if there are inquiries, otherwise circle_outlined
    final icon = count > 0 ? Icons.check_circle : Icons.circle_outlined;
    return (label: 'INQUIRIES ($count)', icon: icon);
  } else {
    // Non-owner: check if user has expressed interest
    final isLoan = availability == Availability.AVAILABILITY_FOR_LOAN;
    final hasInquiry = isLoan
        ? (hasActiveRequest ?? false)
        : (isParticipant ?? false);

    // Use check_circle if user has inquiry, otherwise circle_outlined
    final icon = hasInquiry ? Icons.check_circle : Icons.circle_outlined;
    return (label: 'INQUIRIES ($count)', icon: icon);
  }
}

/// Generates inbox preview label for transfer requests.
///
/// Example: "3 loan requests" or "1 person interested"
String getTransferInboxLabel({
  required TransferType transferType,
  required int count,
}) {
  if (transferType == TransferType.TRANSFER_TYPE_LOAN) {
    return count == 1 ? '1 loan request' : '$count loan requests';
  } else {
    return count == 1 ? '1 person interested' : '$count people interested';
  }
}

/// Generates notification text for new transfer requests.
///
/// Example: "You have 2 new requests for Ladder"
String getTransferNotificationText({
  required String gearName,
  required int count,
}) {
  final suffix = count == 1 ? 'request' : 'requests';
  return 'You have $count new $suffix for $gearName';
}

// ============================================================================
// Feed Action Text
// ============================================================================

/// Gets feed action text for request posting events.
/// Returns 'Posted request' for all request creation events.
String getRequestFeedActionText() {
  return 'Posted request';
}
