package gear

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// bookingFixture builds a community + owner + member + gear shared with the
// community, returning the service and the relevant IDs.
type bookingFixture struct {
	svc         *Service
	store       *storage.ProtoSQLStorage
	communityID string
	gearID      string
	ownerID     string
	memberID    string
}

func setupBookingFixture(t *testing.T) bookingFixture {
	t.Helper()
	store := setupTestStorage(t)
	svc := New(store, &services.MockBucketStorage{})
	ctx := context.Background()

	ownerID := "owner-1"
	memberID := "member-1"
	for _, u := range []string{ownerID, memberID} {
		if _, err := store.Insert(ctx, &models.User{Id: u, Name: "User " + u, Email: u + "@example.com"}); err != nil {
			t.Fatalf("insert user %s: %v", u, err)
		}
	}

	communityID, err := store.Insert(ctx, &models.Community{Name: "Tool Library", CreatorId: ownerID, OwnerUserId: ownerID})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	for _, u := range []string{ownerID, memberID} {
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: u, InviterId: ownerID}); err != nil {
			t.Fatalf("insert membership %s: %v", u, err)
		}
	}

	gearID, err := store.Insert(ctx, &models.Gear{OwnerId: ownerID, Name: "Lawn Mower"})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}); err != nil {
		t.Fatalf("insert community gear: %v", err)
	}

	return bookingFixture{svc: svc, store: store, communityID: communityID, gearID: gearID, ownerID: ownerID, memberID: memberID}
}

const (
	day0 = int64(1_750_000_000)
	day1 = day0 + 86400
	day2 = day1 + 86400
)

func TestClaimGearDays_CreatesAndLists(t *testing.T) {
	f := setupBookingFixture(t)
	ctx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

	resp, err := f.svc.ClaimGearDays(ctx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId:           f.gearID,
		CommunityId:      f.communityID,
		StartDateUnixSec: day0,
		EndDateUnixSec:   day1,
	}))
	if err != nil {
		t.Fatalf("ClaimGearDays failed: %v", err)
	}
	if resp.Msg.Booking.GetBorrower().GetId() != f.memberID {
		t.Errorf("booking borrower = %q, want %q", resp.Msg.Booking.GetBorrower().GetId(), f.memberID)
	}
	if !resp.Msg.Booking.IsMine {
		t.Error("booking should be is_mine for the claimer")
	}

	listed, err := f.svc.ListGearBookings(ctx, connect.NewRequest(&api.ListGearBookingsRequest{
		GearId:      f.gearID,
		CommunityId: f.communityID,
	}))
	if err != nil {
		t.Fatalf("ListGearBookings failed: %v", err)
	}
	if len(listed.Msg.Bookings) != 1 {
		t.Fatalf("expected 1 booking, got %d", len(listed.Msg.Bookings))
	}
	if listed.Msg.Bookings[0].StartDateUnixSec != day0 || listed.Msg.Bookings[0].EndDateUnixSec != day1 {
		t.Errorf("unexpected range: %+v", listed.Msg.Bookings[0])
	}

	// The claim must have created a backing loan Transfer (the booking is a
	// transfer with a reserved window) — recipient is the claimer.
	rows, err := f.store.QueryByField(context.Background(), "gear_id", f.gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("query transfers failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 transfer created by claim, got %d", len(rows))
	}
	tr := rows[0].(*models.Transfer)
	if tr.RecipientId != f.memberID {
		t.Errorf("transfer recipient = %q, want %q", tr.RecipientId, f.memberID)
	}
	if tr.TransferType != models.TransferType_TRANSFER_TYPE_LOAN {
		t.Errorf("transfer type = %v, want LOAN", tr.TransferType)
	}
	if tr.GetEstimatedPickupUnixSec() != day0 || tr.GetExpectedReturnUnixSec() != day1 {
		t.Errorf("transfer window = [%d,%d], want [%d,%d]",
			tr.GetEstimatedPickupUnixSec(), tr.GetExpectedReturnUnixSec(), day0, day1)
	}
}

func TestClaimGearDays_MergesAdjacentOwnDaysIntoOneTransfer(t *testing.T) {
	f := setupBookingFixture(t)
	ctx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

	// Claim day0, then the adjacent day1 — these must collapse into a single
	// multi-day transfer rather than two.
	if _, err := f.svc.ClaimGearDays(ctx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day0, EndDateUnixSec: day0,
	})); err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if _, err := f.svc.ClaimGearDays(ctx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day1, EndDateUnixSec: day1,
	})); err != nil {
		t.Fatalf("second (adjacent) claim failed: %v", err)
	}

	listed, err := f.svc.ListGearBookings(ctx, connect.NewRequest(&api.ListGearBookingsRequest{
		GearId: f.gearID, CommunityId: f.communityID,
	}))
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(listed.Msg.Bookings) != 1 {
		t.Fatalf("adjacent claims should merge into 1 booking, got %d", len(listed.Msg.Bookings))
	}
	b := listed.Msg.Bookings[0]
	if b.StartDateUnixSec != day0 || b.EndDateUnixSec != day1 {
		t.Errorf("merged range = [%d,%d], want [%d,%d]",
			b.StartDateUnixSec, b.EndDateUnixSec, day0, day1)
	}
}

func TestClaimGearDays_RejectsOverlap(t *testing.T) {
	f := setupBookingFixture(t)
	memberCtx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(f.ownerID, "owner-1@example.com", models.Role_ROLE_USER)

	if _, err := f.svc.ClaimGearDays(memberCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day0, EndDateUnixSec: day1,
	})); err != nil {
		t.Fatalf("first claim failed: %v", err)
	}

	// Owner tries to claim an overlapping range (day1–day2).
	_, err := f.svc.ClaimGearDays(ownerCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day1, EndDateUnixSec: day2,
	}))
	if err == nil {
		t.Fatal("expected overlap to be rejected")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v", connect.CodeOf(err))
	}
}

func TestReleaseGearBooking_DropsFromSchedule(t *testing.T) {
	f := setupBookingFixture(t)
	ctx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

	claim, err := f.svc.ClaimGearDays(ctx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day0, EndDateUnixSec: day0,
	}))
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	if _, err := f.svc.ReleaseGearBooking(ctx, connect.NewRequest(&api.ReleaseGearBookingRequest{
		BookingId: claim.Msg.Booking.Id,
	})); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	listed, err := f.svc.ListGearBookings(ctx, connect.NewRequest(&api.ListGearBookingsRequest{
		GearId: f.gearID, CommunityId: f.communityID,
	}))
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(listed.Msg.Bookings) != 0 {
		t.Errorf("expected 0 bookings after release, got %d", len(listed.Msg.Bookings))
	}

	// After release the day is free again — re-claiming must succeed.
	if _, err := f.svc.ClaimGearDays(ctx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day0, EndDateUnixSec: day0,
	})); err != nil {
		t.Errorf("re-claim of a released day should succeed, got: %v", err)
	}
}

func TestReleaseGearBooking_OwnerAndRecipientCanDropOthersCannot(t *testing.T) {
	f := setupBookingFixture(t)
	memberCtx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(f.ownerID, "owner-1@example.com", models.Role_ROLE_USER)
	outsiderCtx := createAuthenticatedContext("outsider-1", "outsider@example.com", models.Role_ROLE_USER)

	claim, err := f.svc.ClaimGearDays(memberCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day0, EndDateUnixSec: day0,
	}))
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	// A non-participant cannot drop someone else's booking.
	if _, err := f.svc.ReleaseGearBooking(outsiderCtx, connect.NewRequest(&api.ReleaseGearBookingRequest{
		BookingId: claim.Msg.Booking.Id,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied for a non-participant, got %v", connect.CodeOf(err))
	}

	// The gear owner can cancel any reservation on their item.
	if _, err := f.svc.ReleaseGearBooking(ownerCtx, connect.NewRequest(&api.ReleaseGearBookingRequest{
		BookingId: claim.Msg.Booking.Id,
	})); err != nil {
		t.Errorf("the gear owner should be able to drop a booking, got %v", err)
	}
}

func TestUpdateGearBookingHandoff_SetsPickup(t *testing.T) {
	f := setupBookingFixture(t)
	ctx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

	claim, err := f.svc.ClaimGearDays(ctx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day0, EndDateUnixSec: day0,
	}))
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	locID := "loc-garage"
	pickupAt := day0 + 3600
	resp, err := f.svc.UpdateGearBookingHandoff(ctx, connect.NewRequest(&api.UpdateGearBookingHandoffRequest{
		BookingId:         claim.Msg.Booking.Id,
		PickupLocationId:  &locID,
		PickupTimeUnixSec: &pickupAt,
	}))
	if err != nil {
		t.Fatalf("UpdateGearBookingHandoff failed: %v", err)
	}
	if resp.Msg.Booking.GetPickupLocationId() != locID {
		t.Errorf("pickup location = %q, want %q", resp.Msg.Booking.GetPickupLocationId(), locID)
	}
	if resp.Msg.Booking.GetPickupTimeUnixSec() != pickupAt {
		t.Errorf("pickup time = %d, want %d", resp.Msg.Booking.GetPickupTimeUnixSec(), pickupAt)
	}
}

func TestUpdateGearBookingHandoff_OwnerOrRecipientCanEdit(t *testing.T) {
	f := setupBookingFixture(t)
	memberCtx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(f.ownerID, "owner-1@example.com", models.Role_ROLE_USER)

	claim, err := f.svc.ClaimGearDays(memberCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day0, EndDateUnixSec: day0,
	}))
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	// The gear owner (not the booking's recipient) may also set the hand-off, so
	// owner and recipient can coordinate the pickup together.
	locID := "loc-porch"
	resp, err := f.svc.UpdateGearBookingHandoff(ownerCtx, connect.NewRequest(&api.UpdateGearBookingHandoffRequest{
		BookingId:        claim.Msg.Booking.Id,
		PickupLocationId: &locID,
	}))
	if err != nil {
		t.Fatalf("owner UpdateGearBookingHandoff failed: %v", err)
	}
	if resp.Msg.Booking.GetPickupLocationId() != locID {
		t.Errorf("pickup location = %q, want %q", resp.Msg.Booking.GetPickupLocationId(), locID)
	}

	// An unrelated member cannot.
	outsiderCtx := createAuthenticatedContext("outsider-1", "outsider@example.com", models.Role_ROLE_USER)
	_, err = f.svc.UpdateGearBookingHandoff(outsiderCtx, connect.NewRequest(&api.UpdateGearBookingHandoffRequest{
		BookingId: claim.Msg.Booking.Id, PickupLocationId: &locID,
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied for a non-participant, got %v", connect.CodeOf(err))
	}
}

func TestListGearBookings_SurfacesSelectedGiveaway(t *testing.T) {
	f := setupBookingFixture(t)
	ctx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

	// A selected giveaway recipient is surfaced as a booking (no date window —
	// a giveaway has no return) so the owner and recipient can coordinate pickup.
	if _, err := f.store.Insert(context.Background(), &models.Transfer{
		GearId:       f.gearID,
		OwnerId:      f.ownerID,
		RecipientId:  f.memberID,
		CommunityId:  f.communityID,
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
	}); err != nil {
		t.Fatalf("insert giveaway transfer: %v", err)
	}
	// An interest-only giveaway transfer must NOT appear.
	if _, err := f.store.Insert(context.Background(), &models.Transfer{
		GearId:       f.gearID,
		OwnerId:      f.ownerID,
		RecipientId:  "interested-1",
		CommunityId:  f.communityID,
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:        models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
	}); err != nil {
		t.Fatalf("insert interest transfer: %v", err)
	}

	listed, err := f.svc.ListGearBookings(ctx, connect.NewRequest(&api.ListGearBookingsRequest{
		GearId: f.gearID, CommunityId: f.communityID,
	}))
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(listed.Msg.Bookings) != 1 {
		t.Fatalf("expected only the selected giveaway as a booking, got %d", len(listed.Msg.Bookings))
	}
	if listed.Msg.Bookings[0].GetBorrower().GetId() != f.memberID {
		t.Errorf("booking borrower = %q, want %q", listed.Msg.Bookings[0].GetBorrower().GetId(), f.memberID)
	}
}

func TestClaimGearDays_OwnerReservesForPerson(t *testing.T) {
	f := setupBookingFixture(t)
	ownerCtx := createAuthenticatedContext(f.ownerID, "owner-1@example.com", models.Role_ROLE_USER)

	resp, err := f.svc.ClaimGearDays(ownerCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId:           f.gearID,
		CommunityId:      f.communityID,
		StartDateUnixSec: day0,
		EndDateUnixSec:   day1,
		RecipientId:      &f.memberID,
	}))
	if err != nil {
		t.Fatalf("owner ClaimGearDays for person failed: %v", err)
	}
	b := resp.Msg.Booking
	if b.GetBorrower().GetId() != f.memberID {
		t.Errorf("booking borrower = %q, want %q", b.GetBorrower().GetId(), f.memberID)
	}
	if b.IsMine {
		t.Error("a booking reserved for someone else is not is_mine for the owner")
	}
	if b.IsPending || b.IsOwnerBlock {
		t.Errorf("unexpected flags: pending=%v block=%v", b.IsPending, b.IsOwnerBlock)
	}
}

func TestClaimGearDays_OwnerBlocksForSelf(t *testing.T) {
	f := setupBookingFixture(t)
	ownerCtx := createAuthenticatedContext(f.ownerID, "owner-1@example.com", models.Role_ROLE_USER)

	resp, err := f.svc.ClaimGearDays(ownerCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId:           f.gearID,
		CommunityId:      f.communityID,
		StartDateUnixSec: day0,
		EndDateUnixSec:   day0,
		RecipientId:      &f.ownerID,
	}))
	if err != nil {
		t.Fatalf("owner self-block failed: %v", err)
	}
	b := resp.Msg.Booking
	if !b.IsOwnerBlock {
		t.Error("a self-reservation should be is_owner_block")
	}
	if !b.IsMine {
		t.Error("a self-block is is_mine for the owner")
	}
}

func TestClaimGearDays_NonOwnerCannotReserveForOthers(t *testing.T) {
	f := setupBookingFixture(t)
	memberCtx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

	_, err := f.svc.ClaimGearDays(memberCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId:           f.gearID,
		CommunityId:      f.communityID,
		StartDateUnixSec: day0,
		EndDateUnixSec:   day0,
		RecipientId:      &f.ownerID,
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied for a non-owner reserving for others, got %v", connect.CodeOf(err))
	}
}

func TestClaimGearDays_PendingLinkAndAccept(t *testing.T) {
	f := setupBookingFixture(t)
	ownerCtx := createAuthenticatedContext(f.ownerID, "owner-1@example.com", models.Role_ROLE_USER)
	memberCtx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

	// Owner mints a pending accept link holding the days.
	resp, err := f.svc.ClaimGearDays(ownerCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId:           f.gearID,
		CommunityId:      f.communityID,
		StartDateUnixSec: day0,
		EndDateUnixSec:   day1,
		Pending:          true,
	}))
	if err != nil {
		t.Fatalf("pending ClaimGearDays failed: %v", err)
	}
	pending := resp.Msg.Booking
	if !pending.IsPending {
		t.Fatal("expected a pending booking")
	}
	if pending.GetAcceptToken() == "" {
		t.Fatal("owner should receive the accept token")
	}
	if pending.GetBorrower() != nil {
		t.Error("a pending booking has no borrower yet")
	}

	// A non-owner cannot see the token but the held days still appear.
	listed, err := f.svc.ListGearBookings(memberCtx, connect.NewRequest(&api.ListGearBookingsRequest{
		GearId: f.gearID, CommunityId: f.communityID,
	}))
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(listed.Msg.Bookings) != 1 || !listed.Msg.Bookings[0].IsPending {
		t.Fatalf("expected one pending booking visible to members, got %+v", listed.Msg.Bookings)
	}
	// The token travels with the booking so a link recipient can accept.
	if listed.Msg.Bookings[0].GetAcceptToken() == "" {
		t.Error("a pending booking should carry its accept token")
	}

	// Wrong token is rejected.
	if _, err := f.svc.AcceptGearBooking(memberCtx, connect.NewRequest(&api.AcceptGearBookingRequest{
		BookingId: pending.Id, AcceptToken: "nope",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected PermissionDenied for a bad token, got %v", connect.CodeOf(err))
	}

	// The link recipient accepts and becomes the booking's recipient.
	accepted, err := f.svc.AcceptGearBooking(memberCtx, connect.NewRequest(&api.AcceptGearBookingRequest{
		BookingId: pending.Id, AcceptToken: pending.GetAcceptToken(),
	}))
	if err != nil {
		t.Fatalf("AcceptGearBooking failed: %v", err)
	}
	if accepted.Msg.Booking.GetBorrower().GetId() != f.memberID {
		t.Errorf("accepted borrower = %q, want %q", accepted.Msg.Booking.GetBorrower().GetId(), f.memberID)
	}
	if accepted.Msg.Booking.IsPending {
		t.Error("an accepted booking is no longer pending")
	}

	// Accepting again is rejected (no longer open).
	if _, err := f.svc.AcceptGearBooking(memberCtx, connect.NewRequest(&api.AcceptGearBookingRequest{
		BookingId: pending.Id, AcceptToken: pending.GetAcceptToken(),
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("expected FailedPrecondition re-accepting a claimed booking, got %v", connect.CodeOf(err))
	}
}

func TestListGearBookings_DeniesNonMember(t *testing.T) {
	f := setupBookingFixture(t)
	outsiderCtx := createAuthenticatedContext("outsider-1", "outsider@example.com", models.Role_ROLE_USER)

	_, err := f.svc.ListGearBookings(outsiderCtx, connect.NewRequest(&api.ListGearBookingsRequest{
		GearId: f.gearID, CommunityId: f.communityID,
	}))
	if err == nil {
		t.Fatal("expected non-member to be denied")
	}
}

// TestReleaseGearBooking_RejectsStartedLoan verifies that Drop is refused once
// the loan is underway: an ACTIVE loan must be completed (marked returned),
// never silently cancelled (#2638 follow-up), and terminal states have nothing
// to drop.
func TestReleaseGearBooking_RejectsStartedLoan(t *testing.T) {
	f := setupBookingFixture(t)
	ctx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

	claim, err := f.svc.ClaimGearDays(ctx, connect.NewRequest(&api.ClaimGearDaysRequest{
		GearId: f.gearID, CommunityId: f.communityID, StartDateUnixSec: day0, EndDateUnixSec: day1,
	}))
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	bookingID := claim.Msg.Booking.Id

	for _, state := range []models.TransferState{
		models.TransferState_TRANSFER_STATE_ACTIVE,
		models.TransferState_TRANSFER_STATE_COMPLETED,
	} {
		transfer := &models.Transfer{}
		if err := f.store.GetByID(ctx, bookingID, transfer); err != nil {
			t.Fatalf("get transfer: %v", err)
		}
		transfer.State = state
		if err := f.store.Update(ctx, transfer); err != nil {
			t.Fatalf("update transfer to %s: %v", state, err)
		}

		_, err := f.svc.ReleaseGearBooking(ctx, connect.NewRequest(&api.ReleaseGearBookingRequest{
			BookingId: bookingID,
		}))
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("state %s: expected FailedPrecondition, got %v (err=%v)", state, connect.CodeOf(err), err)
		}

		// The state must be untouched — no silent cancellation.
		after := &models.Transfer{}
		if err := f.store.GetByID(ctx, bookingID, after); err != nil {
			t.Fatalf("re-get transfer: %v", err)
		}
		if after.State != state {
			t.Errorf("state %s: transfer state changed to %s after rejected release", state, after.State)
		}
	}
}

// TestClaimGearDays_RejectsOverlongRange covers the maxBookingDays guard,
// including the spans that used to defeat it.
//
// The day count was narrowed to int32 BEFORE being compared against
// maxBookingDays. start and end are caller-supplied and validated only for sign
// and ordering, so a span whose day count wrapped int32 landed inside the
// accepted range: start=1, end=371085174374401 is 4,294,967,297 days, and
// int32 of that is 1. The guard waved it through and the booking was created
// spanning ~11.8 million years. Found by gosec G115.
func TestClaimGearDays_RejectsOverlongRange(t *testing.T) {
	cases := []struct {
		name       string
		start, end int64
	}{
		{"just over the limit", day0, day0 + maxBookingDays*secondsPerDay},
		// (end-start)/86400+1 == 2^32+1, which truncates to int32(1).
		{"span that wraps int32 to 1", 1, 371085174374401},
		// (end-start)/86400+1 == 2^31, which truncates to int32 min (negative).
		{"span that wraps int32 negative", 1, (int64(1)<<31-1)*secondsPerDay + 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupBookingFixture(t)
			ctx := createAuthenticatedContext(f.memberID, "member-1@example.com", models.Role_ROLE_USER)

			_, err := f.svc.ClaimGearDays(ctx, connect.NewRequest(&api.ClaimGearDaysRequest{
				GearId:           f.gearID,
				CommunityId:      f.communityID,
				StartDateUnixSec: tc.start,
				EndDateUnixSec:   tc.end,
			}))
			// Assert the specific rejection, not just "some InvalidArgument":
			// several other validations in ClaimGearDays also return
			// InvalidArgument, and a test that accepts any of them would pass
			// even with the overflow guard removed.
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Fatalf("expected InvalidArgument for a %d-second span, got %v (err=%v)",
					tc.end-tc.start, connect.CodeOf(err), err)
			}
			if !strings.Contains(err.Error(), "that range is too long") {
				t.Fatalf("expected the range-too-long rejection, got: %v", err)
			}

			// No booking may have been created.
			listed, lerr := f.svc.ListGearBookings(ctx, connect.NewRequest(&api.ListGearBookingsRequest{
				GearId:      f.gearID,
				CommunityId: f.communityID,
			}))
			if lerr != nil {
				t.Fatalf("ListGearBookings failed: %v", lerr)
			}
			if len(listed.Msg.Bookings) != 0 {
				t.Errorf("rejected claim still created %d booking(s)", len(listed.Msg.Bookings))
			}
		})
	}
}
