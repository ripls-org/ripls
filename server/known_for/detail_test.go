package known_for

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// insertGearWithCreated is a test helper that inserts a gear with the
// given created_at timestamp so the Detail recency ranking can be
// exercised.
func insertGearWithCreated(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	ownerID, name, category string,
	createdAt int64,
) string {
	t.Helper()
	g := &models.Gear{
		Id:               uuid.New().String(),
		OwnerId:          ownerID,
		Name:             name,
		State:            models.GearState_GEAR_STATE_AVAILABLE,
		CreatedAtUnixSec: createdAt,
		Category:         &models.TrackedString{Value: category},
	}
	id, err := db.Insert(context.Background(), g)
	if err != nil {
		t.Fatalf("insertGearWithCreated: %v", err)
	}
	return id
}

// insertCompletedExperienceReturningID is like
// insertCompletedExperience but returns the Experience id so callers
// can attach RSVPs to it.
func insertCompletedExperienceReturningID(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, ownerID, category string,
) string {
	t.Helper()
	e := &models.Experience{
		Id:       uuid.New().String(),
		OwnerId:  ownerID,
		Name:     category + " session",
		State:    models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		Category: category,
	}
	id, err := db.Insert(context.Background(), e)
	if err != nil {
		t.Fatalf("insertCompletedExperienceReturningID: %v", err)
	}
	ce := &models.CommunityExperience{
		Id:           uuid.New().String(),
		CommunityId:  communityID,
		ExperienceId: id,
	}
	if _, err := db.Insert(context.Background(), ce); err != nil {
		t.Fatalf("insertCommunityExperience: %v", err)
	}
	return id
}

// insertAttendedRSVP records [userID] as having attended
// (RSVP.attended == "YES") the given experience in the given
// community.
func insertAttendedRSVP(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	experienceID, communityID, userID string,
) {
	t.Helper()
	rsvp := &models.ExperienceRSVP{
		Id:           uuid.New().String(),
		ExperienceId: experienceID,
		UserId:       userID,
		CommunityId:  communityID,
		Intention:    models.RSVPIntention_RSVP_INTENTION_YES,
		Attended:     models.AttendedStatus_ATTENDED_STATUS_YES,
	}
	if _, err := db.Insert(context.Background(), rsvp); err != nil {
		t.Fatalf("insertAttendedRSVP: %v", err)
	}
}

// insertFulfilledRequestWithHelper inserts a FULFILLED Request with
// [helperID] in its confirmed_helper_ids list.
func insertFulfilledRequestWithHelper(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, requesterID, category, helperID string,
) {
	t.Helper()
	r := &models.Request{
		Id:                 uuid.New().String(),
		RequesterId:        requesterID,
		Title:              category + " help",
		State:              models.RequestState_REQUEST_STATE_FULFILLED,
		Category:           category,
		ConfirmedHelperIds: []string{helperID},
	}
	id, err := db.Insert(context.Background(), r)
	if err != nil {
		t.Fatalf("insertFulfilledRequestWithHelper: %v", err)
	}
	cr := &models.CommunityRequest{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		RequestId:   id,
	}
	if _, err := db.Insert(context.Background(), cr); err != nil {
		t.Fatalf("insertCommunityRequest: %v", err)
	}
}

func TestDetail_EmptyCommunityIDs(t *testing.T) {
	db := setupTestStorage(t)
	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:     ModePerCommunity,
		Category: "Power Tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != 0 || len(res.MemberIDs) != 0 || res.Suppressed {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestDetail_PerUser_MissingOwnerID(t *testing.T) {
	db := setupTestStorage(t)
	_, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerUser,
		CommunityIDs: []string{"c"},
		Category:     "Power Tools",
	})
	if err == nil {
		t.Fatalf("expected error when OwnerID empty in per-user mode")
	}
}

func TestDetail_EmptyCategory(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")
	insertGearWithCategory(t, db, target, "Saw", "Power Tools")

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
		Category:     "   ", // whitespace normalizes to empty
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected empty items for empty category, got %d", len(res.Items))
	}
}

func TestDetail_Suppressed_ShortCircuits(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")
	gearID := insertGearWithCategory(t, db, target, "Saw", "Power Tools")
	shareGearInCommunity(t, db, community, gearID)

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "Power Tools",
		SuppressedKeys: map[string]struct{}{
			"power tools": {},
		},
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !res.Suppressed {
		t.Fatalf("expected Suppressed=true")
	}
	if len(res.Items) != 0 || len(res.MemberIDs) != 0 {
		t.Fatalf("expected no items/members when suppressed, got %+v", res)
	}
}

func TestDetail_PerCommunity_FilterByCategory(t *testing.T) {
	db := setupTestStorage(t)
	alice := insertUser(t, db, "Alice")
	community := insertCommunity(t, db, "Crew")

	powerTools := insertGearWithCategory(t, db, alice, "Saw", "Power Tools")
	camping := insertGearWithCategory(t, db, alice, "Tent", "Camping")
	shareGearInCommunity(t, db, community, powerTools)
	shareGearInCommunity(t, db, community, camping)

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "Power Tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if res.Items[0].Kind != DetailKindGear || res.Items[0].Gear.Id != powerTools {
		t.Fatalf("expected the power-tools gear, got %+v", res.Items[0])
	}
}

func TestDetail_RecencyRanking(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")

	// Three gear with distinct created-at timestamps.
	a := insertGearWithCreated(t, db, owner, "A", "Power Tools", 100)
	b := insertGearWithCreated(t, db, owner, "B", "Power Tools", 300)
	c := insertGearWithCreated(t, db, owner, "C", "Power Tools", 200)
	for _, id := range []string{a, b, c} {
		shareGearInCommunity(t, db, community, id)
	}

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "Power Tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(res.Items))
	}
	wantOrder := []string{b, c, a}
	for i, want := range wantOrder {
		if res.Items[i].Gear.Id != want {
			t.Fatalf("item %d: got %s want %s", i, res.Items[i].Gear.Id, want)
		}
	}
}

func TestDetail_TenItemCap(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")

	for i := 0; i < 11; i++ {
		id := insertGearWithCreated(t, db, owner, "Gear", "Power Tools", int64(i+1))
		shareGearInCommunity(t, db, community, id)
	}

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "Power Tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != MaxDetailItems {
		t.Fatalf("expected %d items, got %d", MaxDetailItems, len(res.Items))
	}
	// Newest (timestamp 11) should be first; oldest cut off (timestamp
	// 1 would be 11th in recency order).
	if res.Items[0].RecencySecs != 11 {
		t.Fatalf("expected newest first; got recency %d", res.Items[0].RecencySecs)
	}
}

func TestDetail_PerUser_OmitsOwnerFromMembers(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	borrower := insertUser(t, db, "Borrower")
	community := insertCommunity(t, db, "Crew")

	gearID := insertGearWithCategory(t, db, target, "Saw", "Power Tools")
	shareGearInCommunity(t, db, community, gearID)
	insertCompletedLoan(t, db, community, target, borrower, gearID)

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
		Category:     "Power Tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	for _, id := range res.MemberIDs {
		if id == target {
			t.Fatalf("per-user mode should not surface the target in MemberIDs")
		}
		// Borrowers must never appear — a borrower's own per-user
		// known_for chips don't credit borrowing, so listing them
		// here would surface a contributor with no matching chip.
		if id == borrower {
			t.Fatalf("borrowers should not appear in MemberIDs, got %v", res.MemberIDs)
		}
	}
}

func TestDetail_PerCommunity_OmitsBorrowersFromMembers(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	borrower := insertUser(t, db, "Borrower")
	community := insertCommunity(t, db, "Crew")

	gearID := insertGearWithCategory(t, db, owner, "Saw", "Power Tools")
	shareGearInCommunity(t, db, community, gearID)
	insertCompletedLoan(t, db, community, owner, borrower, gearID)

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "Power Tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	for _, id := range res.MemberIDs {
		if id == borrower {
			t.Fatalf("borrowers should not appear in MemberIDs, got %v", res.MemberIDs)
		}
	}
	foundOwner := false
	for _, id := range res.MemberIDs {
		if id == owner {
			foundOwner = true
		}
	}
	if !foundOwner {
		t.Fatalf("expected owner in MemberIDs, got %v", res.MemberIDs)
	}
}

func TestDetail_PerCommunity_IncludesAttendees(t *testing.T) {
	db := setupTestStorage(t)
	host := insertUser(t, db, "Host")
	attendee := insertUser(t, db, "Attendee")
	community := insertCommunity(t, db, "Crew")
	expID := insertCompletedExperienceReturningID(t, db, community, host, "Cooking")
	insertAttendedRSVP(t, db, expID, community, attendee)

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "Cooking",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	foundAttendee := false
	for _, id := range res.MemberIDs {
		if id == attendee {
			foundAttendee = true
		}
	}
	if !foundAttendee {
		t.Fatalf("expected attendee in MemberIDs, got %v", res.MemberIDs)
	}
}

func TestDetail_PerUser_IncludesAttendedExperiences(t *testing.T) {
	db := setupTestStorage(t)
	host := insertUser(t, db, "Host")
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")
	// Target attends Host's Cooking event; target hosted nothing.
	expID := insertCompletedExperienceReturningID(t, db, community, host, "Cooking")
	insertAttendedRSVP(t, db, expID, community, target)

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
		Category:     "Cooking",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item (the attended experience), got %d", len(res.Items))
	}
}

func TestDerive_PerUser_AttendedExperienceClearsThreshold(t *testing.T) {
	db := setupTestStorage(t)
	host := insertUser(t, db, "Host")
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")
	expID := insertCompletedExperienceReturningID(t, db, community, host, "Cooking")
	insertAttendedRSVP(t, db, expID, community, target)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Cooking" {
		t.Fatalf("attended completed experience should produce a per-user chip; got %v", tags)
	}
}

func TestDerive_PerUser_HelpedRequestClearsThreshold(t *testing.T) {
	db := setupTestStorage(t)
	requester := insertUser(t, db, "Requester")
	helper := insertUser(t, db, "Helper")
	community := insertCommunity(t, db, "Crew")
	insertFulfilledRequestWithHelper(t, db, community, requester, "Power Tools", helper)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      helper,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Power Tools" {
		t.Fatalf("helped fulfilled request should produce a per-user chip; got %v", tags)
	}
}

func TestDerive_PerUser_HostedAndAttendedCountOnce(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")
	// Target hosts one completed cooking event AND RSVP-attends it
	// (host self-RSVP). The chip should still surface (weight 2)
	// but not double-count to weight 4.
	expID := insertCompletedExperienceReturningID(t, db, community, target, "Cooking")
	insertAttendedRSVP(t, db, expID, community, target)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Cooking" {
		t.Fatalf("got %v, want [Cooking]", tags)
	}
}

func TestDetail_PerUser_IncludesHelpedRequests(t *testing.T) {
	db := setupTestStorage(t)
	requester := insertUser(t, db, "Requester")
	helper := insertUser(t, db, "Helper")
	community := insertCommunity(t, db, "Crew")
	insertFulfilledRequestWithHelper(t, db, community, requester, "Power Tools", helper)

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerUser,
		OwnerID:      helper,
		CommunityIDs: []string{community},
		Category:     "Power Tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item (the helped request), got %d", len(res.Items))
	}
}

func TestDetail_PerCommunity_CategoryNormalization(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")
	id := insertGearWithCategory(t, db, owner, "Saw", "  Power  Tools  ")
	shareGearInCommunity(t, db, community, id)

	// Lookup uses different casing/whitespace from storage.
	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "power tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
}

func TestDetail_QueryBudget(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")

	id := insertGearWithCategory(t, db, owner, "Saw", "Power Tools")
	shareGearInCommunity(t, db, community, id)
	loanGearNTimes(t, db, community, owner, id, 1)
	insertCompletedExperience(t, db, community, owner, "Cooking")
	insertFulfilledRequest(t, db, community, owner, "Power Tools")

	// per-community budget: gear (3) + experiences (2 +1 attendees) +
	// requests (2) = 8 reads.
	ctx := storage.WithQueryStats(context.Background())
	storage.AssertMaxQueries(t, ctx, 8, func() {
		_, err := Detail(ctx, db, DetailOptions{
			Mode:         ModePerCommunity,
			CommunityIDs: []string{community},
			Category:     "Power Tools",
		})
		if err != nil {
			t.Fatalf("Detail: %v", err)
		}
	})
}

func TestDetail_PerCommunity_IncludesCompletedExperience(t *testing.T) {
	db := setupTestStorage(t)
	host := insertUser(t, db, "Host")
	community := insertCommunity(t, db, "Crew")
	insertCompletedExperience(t, db, community, host, "Cooking")

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "Cooking",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Kind != DetailKindExperience {
		t.Fatalf("expected one experience item, got %+v", res.Items)
	}
}

func TestDetail_PerCommunity_RequestFulfilledShowsUp(t *testing.T) {
	db := setupTestStorage(t)
	requester := insertUser(t, db, "Requester")
	community := insertCommunity(t, db, "Crew")
	insertFulfilledRequest(t, db, community, requester, "Power Tools")

	res, err := Detail(context.Background(), db, DetailOptions{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
		Category:     "Power Tools",
	})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	foundRequest := false
	for _, it := range res.Items {
		if it.Kind == DetailKindRequest {
			foundRequest = true
		}
	}
	if !foundRequest {
		t.Fatalf("expected a fulfilled request to surface, got %+v", res.Items)
	}
}
