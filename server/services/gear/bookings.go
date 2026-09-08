package gear

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// secondsPerDay bounds how long a single booking may run.
const secondsPerDay = int64(86400)

// maxBookingDays caps a single claimed range so one member can't reserve the
// gear indefinitely.
const maxBookingDays = 30

// A gear booking is just a loan Transfer with a reserved date window
// (estimated_pickup_unix_sec .. expected_return_unix_sec). Claiming days creates
// the transfer, so the schedule, "who's using it" people, and impact all flow
// through the existing loan model rather than a parallel concept.

// bookingTransferStates are the transfer states that represent a live booking on
// the calendar (a reserved or in-progress loan).
func isLiveBookingState(s models.TransferState) bool {
	return s == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED ||
		s == models.TransferState_TRANSFER_STATE_ACTIVE
}

// hasBookingWindow reports whether a transfer carries a reserved date range
// (set by ClaimGearDays). Interest-only transfers from the legacy flow do not.
func hasBookingWindow(t *models.Transfer) bool {
	return t.EstimatedPickupUnixSec != nil && t.ExpectedReturnUnixSec != nil
}

// isBookingTransfer reports whether a transfer should appear in the gear's
// "who's using it" schedule: a live loan with a reserved date window (the
// calendar bookings), or the selected recipient of a giveaway (so the owner and
// recipient can coordinate the pickup hand-off — a giveaway has no return, so it
// needs no date window).
func isBookingTransfer(t *models.Transfer) bool {
	if !isLiveBookingState(t.State) {
		return false
	}
	switch t.TransferType {
	case models.TransferType_TRANSFER_TYPE_LOAN:
		return hasBookingWindow(t)
	case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
		return true
	default:
		return false
	}
}

// ListGearBookings returns the who-has-it-which-days schedule for a gear item:
// every live loan transfer with a reserved date window, with the recipient
// resolved and is_mine set for the caller.
func (s *Service) ListGearBookings(
	ctx context.Context,
	req *connect.Request[api.ListGearBookingsRequest],
) (*connect.Response[api.ListGearBookingsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListGearBookings",
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.GearId,
		"community_id", req.Msg.CommunityId,
	)

	if _, err := s.requireBookingReadAccess(ctx, authInfo.UserID, req.Msg.GearId); err != nil {
		return nil, err
	}

	fields := map[string]any{"gear_id": req.Msg.GearId}
	if req.Msg.CommunityId != "" {
		fields["community_id"] = req.Msg.CommunityId
	}
	rows, err := s.storage.QueryByFields(ctx, fields, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query gear transfers", "error", err)
		return nil, connecterr.Internal(ctx, "ListGearBookings", err)
	}

	transfers := make([]*models.Transfer, 0, len(rows))
	recipientIDSet := make(map[string]bool)
	for _, row := range rows {
		t := row.(*models.Transfer)
		if !isBookingTransfer(t) {
			continue
		}
		transfers = append(transfers, t)
		recipientIDSet[t.RecipientId] = true
	}

	recipientIDs := make([]string, 0, len(recipientIDSet))
	for id := range recipientIDSet {
		recipientIDs = append(recipientIDs, id)
	}
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, recipientIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to fetch booking recipients", "error", err)
		return nil, connecterr.Internal(ctx, "ListGearBookings", err)
	}

	out := make([]*api.GearBooking, 0, len(transfers))
	for _, t := range transfers {
		out = append(out, transferToBooking(t, authInfo.UserID, services.ResolveUserOrFormer(userMap, t.RecipientId)))
	}
	return connect.NewResponse(&api.ListGearBookingsResponse{Bookings: out}), nil
}

// ClaimGearDays reserves an inclusive range of days for the authenticated user
// by creating a loan transfer, rejecting ranges that overlap an existing live
// booking.
func (s *Service) ClaimGearDays(
	ctx context.Context,
	req *connect.Request[api.ClaimGearDaysRequest],
) (*connect.Response[api.ClaimGearDaysResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ClaimGearDays",
		"user_id", authInfo.UserID,
		"gear_id", req.Msg.GearId,
		"community_id", req.Msg.CommunityId,
	)

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}
	start, end := req.Msg.StartDateUnixSec, req.Msg.EndDateUnixSec
	if start <= 0 || end <= 0 || end < start {
		return nil, connecterr.UserVisible(ctx, connect.CodeInvalidArgument,
			"gear_booking_invalid_range", "pick a valid date range", nil)
	}
	// Range-check the span as int64 BEFORE narrowing to int32. start and end are
	// caller-supplied and only checked for sign and ordering above, so narrowing
	// first let a huge span wrap and defeat this very guard: start=1,
	// end=371085174374401 is a 4,294,967,297-day span that truncates to int32(1)
	// and sails past `> maxBookingDays`. Compute wide, reject, then narrow.
	daySpan := (end-start)/secondsPerDay + 1
	if daySpan > maxBookingDays {
		return nil, connecterr.UserVisible(ctx, connect.CodeInvalidArgument,
			"gear_booking_range_too_long", "that range is too long", nil)
	}
	days := int32(daySpan)

	// The gear must be shared with the community and the caller a member of it.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}
	gear, err := s.requireBookingAccess(ctx, authInfo.UserID, req.Msg.GearId)
	if err != nil {
		return nil, err
	}
	// A given-away item has permanently left the owner's hands — no new
	// bookings, regardless of surviving share rows (#2695).
	if gear.State == models.GearState_GEAR_STATE_GIVEN_AWAY {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("gear has been given away"))
	}
	if shared, err := s.gearSharedWithCommunity(ctx, req.Msg.GearId, req.Msg.CommunityId); err != nil {
		return nil, connecterr.Internal(ctx, "ClaimGearDays", err)
	} else if !shared {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("gear is not shared with this community"))
	}

	// Resolve who the days are reserved for. By default the caller is the
	// recipient (a borrower booking). The gear owner may instead reserve for
	// someone else, block the days for personal use (recipient = the owner), or
	// hold them unassigned behind an accept link (pending).
	recipient := authInfo.UserID
	pending := req.Msg.Pending
	acceptToken := ""
	if pending || (req.Msg.RecipientId != nil && *req.Msg.RecipientId != "") {
		if gear.OwnerId != authInfo.UserID {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("only the gear owner can reserve on someone's behalf"))
		}
		if pending {
			recipient = "" // held until a link recipient accepts
			tok, err := newAcceptToken()
			if err != nil {
				return nil, connecterr.Internal(ctx, "ClaimGearDays", err)
			}
			acceptToken = tok
		} else {
			recipient = *req.Msg.RecipientId
		}
	}

	// Walk existing live bookings: reject overlap with *other* recipients, and
	// collect this recipient's own adjacent/overlapping bookings so contiguous
	// days merge into a single multi-day transfer rather than fragmenting. A
	// pending hold never merges — each accept link is its own reservation.
	rows, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id":      req.Msg.GearId,
		"community_id": req.Msg.CommunityId,
	}, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query existing transfers", "error", err)
		return nil, connecterr.Internal(ctx, "ClaimGearDays", err)
	}
	var mergeable []*models.Transfer
	for _, row := range rows {
		t := row.(*models.Transfer)
		if t.TransferType != models.TransferType_TRANSFER_TYPE_LOAN ||
			!isLiveBookingState(t.State) || !hasBookingWindow(t) {
			continue
		}
		tStart, tEnd := t.GetEstimatedPickupUnixSec(), t.GetExpectedReturnUnixSec()
		if !pending && t.RecipientId == recipient {
			// Mergeable when touching (within one day) or overlapping.
			if tStart <= end+secondsPerDay && tEnd >= start-secondsPerDay {
				mergeable = append(mergeable, t)
			}
		} else if tStart <= end && tEnd >= start {
			return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition,
				"gear_booking_overlap", "those days are already taken", nil)
		}
	}

	// Union the new range with any of the caller's mergeable bookings.
	for _, t := range mergeable {
		if v := t.GetEstimatedPickupUnixSec(); v < start {
			start = v
		}
		if v := t.GetExpectedReturnUnixSec(); v > end {
			end = v
		}
	}
	// Same widening rule as above: merging pulled start/end outward from stored
	// bookings, so re-check the union in int64 before narrowing.
	daySpan = (end-start)/secondsPerDay + 1
	if daySpan > maxBookingDays {
		return nil, connecterr.UserVisible(ctx, connect.CodeInvalidArgument,
			"gear_booking_range_too_long", "that range is too long", nil)
	}
	days = int32(daySpan)

	var booking *models.Transfer
	if len(mergeable) > 0 {
		// Extend the earliest mergeable booking to the union; cancel the rest so
		// the contiguous span is represented by a single transfer.
		booking = mergeable[0]
		for _, t := range mergeable[1:] {
			if t.GetEstimatedPickupUnixSec() < booking.GetEstimatedPickupUnixSec() {
				booking = t
			}
		}
		booking.EstimatedPickupUnixSec = &start
		booking.ExpectedReturnUnixSec = &end
		booking.LoanDurationDays = &days
		if err := s.storage.Update(ctx, booking); err != nil {
			return nil, connecterr.Internal(ctx, "ClaimGearDays", err)
		}
		for _, t := range mergeable {
			if t.Id == booking.Id {
				continue
			}
			t.State = models.TransferState_TRANSFER_STATE_CANCELLED
			if err := s.storage.Update(ctx, t); err != nil {
				return nil, connecterr.Internal(ctx, "ClaimGearDays", err)
			}
		}
	} else {
		booking = &models.Transfer{
			GearId:                 req.Msg.GearId,
			OwnerId:                gear.OwnerId,
			RecipientId:            recipient,
			CommunityId:            req.Msg.CommunityId,
			TransferType:           models.TransferType_TRANSFER_TYPE_LOAN,
			State:                  models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
			EstimatedPickupUnixSec: &start,
			ExpectedReturnUnixSec:  &end,
			LoanDurationDays:       &days,
			LatestRequestUnixSec:   clock.UnixSec(ctx),
		}
		if acceptToken != "" {
			booking.AcceptToken = &acceptToken
		}
		id, err := s.storage.Insert(ctx, booking)
		if err != nil {
			logger.ErrorContext(ctx, "failed to insert booking transfer", "error", err)
			return nil, connecterr.Internal(ctx, "ClaimGearDays", err)
		}
		booking.Id = id
	}

	// Resolve the booking's recipient for display (nil for a pending hold).
	var borrower *api.User
	if booking.RecipientId != "" {
		borrower, _ = services.FetchAPIUser(ctx, s.storage, booking.RecipientId)
	}
	return connect.NewResponse(&api.ClaimGearDaysResponse{
		Booking: transferToBooking(booking, authInfo.UserID, borrower),
	}), nil
}

// AcceptGearBooking lets a link recipient claim a pending owner-created
// reservation: it assigns them as the recipient and clears the accept token.
func (s *Service) AcceptGearBooking(
	ctx context.Context,
	req *connect.Request[api.AcceptGearBookingRequest],
) (*connect.Response[api.AcceptGearBookingResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.BookingId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("booking_id is required"))
	}
	transfer := &models.Transfer{}
	if err := s.storage.GetByID(ctx, req.Msg.BookingId, transfer); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("booking not found"))
		}
		return nil, connecterr.Internal(ctx, "AcceptGearBooking", err)
	}
	// Only a still-open pending hold can be accepted.
	if transfer.GetAcceptToken() == "" || transfer.RecipientId != "" ||
		!isLiveBookingState(transfer.State) {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition,
			"gear_booking_not_open", "those days aren't open to claim anymore", nil)
	}
	if req.Msg.AcceptToken == "" || req.Msg.AcceptToken != transfer.GetAcceptToken() {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("invalid accept token"))
	}
	// The accepter must be a member of the booking's community.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, transfer.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	transfer.RecipientId = authInfo.UserID
	transfer.AcceptToken = nil
	transfer.LatestRequestUnixSec = clock.UnixSec(ctx)
	if err := s.storage.Update(ctx, transfer); err != nil {
		return nil, connecterr.Internal(ctx, "AcceptGearBooking", err)
	}
	borrower, _ := services.FetchAPIUser(ctx, s.storage, authInfo.UserID)
	return connect.NewResponse(&api.AcceptGearBookingResponse{
		Booking: transferToBooking(transfer, authInfo.UserID, borrower),
	}), nil
}

// newAcceptToken mints a cryptographically random token for a pending
// reservation's accept link.
func newAcceptToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate accept token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ReleaseGearBooking drops a booking the authenticated user owns by cancelling
// its transfer.
func (s *Service) ReleaseGearBooking(
	ctx context.Context,
	req *connect.Request[api.ReleaseGearBookingRequest],
) (*connect.Response[api.ReleaseGearBookingResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	// The booking's recipient (the borrower) or the gear owner may drop it — the
	// owner cancels reservations they made on someone's behalf, blocks, or
	// pending holds (which have no recipient yet).
	transfer, err := s.requireHandoffParticipant(ctx, authInfo.UserID, req.Msg.BookingId)
	if err != nil {
		return nil, err
	}
	// Only a not-yet-started booking can be dropped. Once the loan is ACTIVE
	// the item is in the borrower's hands — the loan must be completed
	// (marked returned), not cancelled out from under the impact accounting;
	// terminal states have nothing to drop. (#2638 follow-up: the calendar
	// day-dock offered Drop on active loans and this silently cancelled them.)
	if transfer.State != models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED &&
		transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition,
			"gear_booking_not_droppable",
			"this loan is already underway — mark it returned instead", nil)
	}
	transfer.State = models.TransferState_TRANSFER_STATE_CANCELLED
	if err := s.storage.Update(ctx, transfer); err != nil {
		return nil, connecterr.Internal(ctx, "ReleaseGearBooking", err)
	}
	return connect.NewResponse(&api.ReleaseGearBookingResponse{}), nil
}

// UpdateGearBookingHandoff sets or clears the pickup / drop-off hand-off on a
// booking the authenticated user owns. The hand-off times reuse the transfer's
// estimated-pickup / expected-return window.
func (s *Service) UpdateGearBookingHandoff(
	ctx context.Context,
	req *connect.Request[api.UpdateGearBookingHandoffRequest],
) (*connect.Response[api.UpdateGearBookingHandoffResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	transfer, err := s.requireHandoffParticipant(ctx, authInfo.UserID, req.Msg.BookingId)
	if err != nil {
		return nil, err
	}

	if req.Msg.PickupTimeUnixSec != nil {
		v := *req.Msg.PickupTimeUnixSec
		transfer.EstimatedPickupUnixSec = &v
	}
	if req.Msg.DropoffTimeUnixSec != nil {
		v := *req.Msg.DropoffTimeUnixSec
		transfer.ExpectedReturnUnixSec = &v
	}
	if req.Msg.PickupLocationId != nil {
		if *req.Msg.PickupLocationId == "" {
			transfer.PickupLocationId = nil
		} else {
			v := *req.Msg.PickupLocationId
			transfer.PickupLocationId = &v
		}
	}
	if req.Msg.DropoffLocationId != nil {
		if *req.Msg.DropoffLocationId == "" {
			transfer.DropoffLocationId = nil
		} else {
			v := *req.Msg.DropoffLocationId
			transfer.DropoffLocationId = &v
		}
	}

	if err := s.storage.Update(ctx, transfer); err != nil {
		return nil, connecterr.Internal(ctx, "UpdateGearBookingHandoff", err)
	}
	borrower, _ := services.FetchAPIUser(ctx, s.storage, transfer.RecipientId)
	return connect.NewResponse(&api.UpdateGearBookingHandoffResponse{
		Booking: transferToBooking(transfer, authInfo.UserID, borrower),
	}), nil
}

// ── helpers ─────────────────────────────────────────────────────────────────.

// requireBookingAccess fetches the gear and verifies the caller may mutate
// its schedule (owner, or a member of a community it is ACTIVELY shared
// with). Booking mutations must keep the strict gate: an archived share
// (item unshared or given away) never authorizes new bookings (#2695).
func (s *Service) requireBookingAccess(ctx context.Context, userID, gearID string) (*models.Gear, error) {
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
		}
		return nil, connecterr.Internal(ctx, "GearBooking", err)
	}
	if _, _, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, userID, auth.EntityGear, gear.Id, gear.OwnerId,
	); err != nil {
		return nil, err
	}
	return gear, nil
}

// requireBookingReadAccess mirrors requireBookingAccess with the read gate:
// archived shares still grant schedule VISIBILITY, so the booking history of
// a completed giveaway stays viewable to its communities (#2695).
func (s *Service) requireBookingReadAccess(ctx context.Context, userID, gearID string) (*models.Gear, error) {
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
		}
		return nil, connecterr.Internal(ctx, "GearBooking", err)
	}
	if _, _, err := auth.RequireReadAccessToCommunityScopedEntity(
		ctx, s.storage, userID, auth.EntityGear, gear.Id, gear.OwnerId,
	); err != nil {
		return nil, err
	}
	return gear, nil
}

// requireHandoffParticipant fetches a booking transfer and verifies the caller
// is either its recipient (who claimed it) or the gear owner. Both coordinate
// the hand-off — for a giveaway the owner and recipient arrange the pickup
// together once the owner selects the recipient.
func (s *Service) requireHandoffParticipant(ctx context.Context, userID, bookingID string) (*models.Transfer, error) {
	if bookingID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("booking_id is required"))
	}
	transfer := &models.Transfer{}
	if err := s.storage.GetByID(ctx, bookingID, transfer); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("booking not found"))
		}
		return nil, connecterr.Internal(ctx, "GearBooking", err)
	}
	if transfer.RecipientId != userID && transfer.OwnerId != userID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the booking owner or recipient can change the hand-off"))
	}
	return transfer, nil
}

// gearSharedWithCommunity reports whether the gear has a CommunityGear row in
// the given community.
func (s *Service) gearSharedWithCommunity(ctx context.Context, gearID, communityID string) (bool, error) {
	rows, err := s.storage.QueryByFields(ctx, map[string]any{
		"gear_id":      gearID,
		"community_id": communityID,
	}, &models.CommunityGear{})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// bookingStateFromTransfer maps a transfer state onto the booking-facing state.
func bookingStateFromTransfer(s models.TransferState) api.GearBookingState {
	switch s {
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		return api.GearBookingState_GEAR_BOOKING_STATE_ACTIVE
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		return api.GearBookingState_GEAR_BOOKING_STATE_COMPLETED
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		return api.GearBookingState_GEAR_BOOKING_STATE_CANCELLED
	default:
		return api.GearBookingState_GEAR_BOOKING_STATE_RESERVED
	}
}

// transferToBooking converts a booking transfer into its API representation.
func transferToBooking(t *models.Transfer, viewerID string, borrower *api.User) *api.GearBooking {
	isPending := t.RecipientId == "" && t.GetAcceptToken() != ""
	out := &api.GearBooking{
		Id:               t.Id,
		GearId:           t.GearId,
		CommunityId:      t.CommunityId,
		Borrower:         borrower,
		StartDateUnixSec: t.GetEstimatedPickupUnixSec(),
		EndDateUnixSec:   t.GetExpectedReturnUnixSec(),
		State:            bookingStateFromTransfer(t.State),
		IsMine:           t.RecipientId != "" && t.RecipientId == viewerID,
		IsPending:        isPending,
		IsOwnerBlock:     t.RecipientId != "" && t.RecipientId == t.OwnerId,
	}
	// The accept token travels with the pending booking so a link recipient who
	// reaches the gear can claim the held days. Acceptance still requires the
	// token plus community membership.
	if isPending {
		v := t.GetAcceptToken()
		out.AcceptToken = &v
	}
	if t.EstimatedPickupUnixSec != nil {
		v := *t.EstimatedPickupUnixSec
		out.PickupTimeUnixSec = &v
	}
	if t.ExpectedReturnUnixSec != nil {
		v := *t.ExpectedReturnUnixSec
		out.DropoffTimeUnixSec = &v
	}
	if t.PickupLocationId != nil {
		v := *t.PickupLocationId
		out.PickupLocationId = &v
	}
	if t.DropoffLocationId != nil {
		v := *t.DropoffLocationId
		out.DropoffLocationId = &v
	}
	return out
}
