package known_for

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return db
}

func insertUser(t *testing.T, db *storage.ProtoSQLStorage, name string) string {
	t.Helper()
	id, err := db.Insert(context.Background(), &models.User{
		Id:   uuid.New().String(),
		Name: name,
	})
	if err != nil {
		t.Fatalf("insertUser: %v", err)
	}
	return id
}

func insertCommunity(t *testing.T, db *storage.ProtoSQLStorage, name string) string {
	t.Helper()
	id, err := db.Insert(context.Background(), &models.Community{
		Id:          uuid.New().String(),
		Name:        name,
		CreatorId:   "test",
		OwnerUserId: "test",
	})
	if err != nil {
		t.Fatalf("insertCommunity: %v", err)
	}
	return id
}

func insertGearWithCategory(t *testing.T, db *storage.ProtoSQLStorage, ownerID, name, category string) string {
	t.Helper()
	g := &models.Gear{
		Id:               uuid.New().String(),
		OwnerId:          ownerID,
		Name:             name,
		State:            models.GearState_GEAR_STATE_AVAILABLE,
		CreatedAtUnixSec: 1,
		Category:         &models.TrackedString{Value: category},
	}
	id, err := db.Insert(context.Background(), g)
	if err != nil {
		t.Fatalf("insertGearWithCategory: %v", err)
	}
	return id
}

// shareGearInCommunity creates the community_gear pivot row that
// lists [gearID] inside [communityID]. Each pivot adds +1 weight to
// the gear's category bucket.
func shareGearInCommunity(t *testing.T, db *storage.ProtoSQLStorage, communityID, gearID string) {
	t.Helper()
	cg := &models.CommunityGear{
		Id:           uuid.New().String(),
		CommunityId:  communityID,
		GearId:       gearID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	if _, err := db.Insert(context.Background(), cg); err != nil {
		t.Fatalf("shareGearInCommunity: %v", err)
	}
}

func insertCompletedLoan(t *testing.T, db *storage.ProtoSQLStorage, communityID, ownerID, recipientID, gearID string) {
	t.Helper()
	pickup := int64(100)
	ret := int64(200)
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		OwnerId:             ownerID,
		RecipientId:         recipientID,
		GearId:              gearID,
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualPickupUnixSec: &pickup,
		ActualReturnUnixSec: &ret,
	}
	if _, err := db.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("insertCompletedLoan: %v", err)
	}
}

func loanGearNTimes(t *testing.T, db *storage.ProtoSQLStorage, community, owner, gearID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		recipient := insertUser(t, db, "Recipient")
		insertCompletedLoan(t, db, community, owner, recipient, gearID)
	}
}

func TestDerive_EmptyCommunityIDs(t *testing.T) {
	db := setupTestStorage(t)
	tags, err := Derive(context.Background(), db, Options{
		Mode:    ModePerUser,
		OwnerID: "any",
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected empty, got %v", tags)
	}
}

func TestDerive_PerUser_MissingOwnerID(t *testing.T) {
	db := setupTestStorage(t)
	_, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		CommunityIDs: []string{"c"},
	})
	if err == nil {
		t.Fatalf("expected error when OwnerID empty in per-user mode")
	}
}

func TestDerive_PerUser_AtThreshold(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")
	gearID := insertGearWithCategory(t, db, target, "Chainsaw", "Power Tools")
	loanGearNTimes(t, db, community, target, gearID, 2)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Power Tools" {
		t.Fatalf("got %v, want [Power Tools]", tags)
	}
}

func TestDerive_PerUser_BelowThreshold(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")
	gearID := insertGearWithCategory(t, db, target, "Chainsaw", "Power Tools")
	loanGearNTimes(t, db, community, target, gearID, 1)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected empty (1 loan < threshold), got %v", tags)
	}
}

func TestDerive_PerUser_DedupesCategoryVariants(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	for _, cat := range []string{"Power Tools", "power tools", "  Power  Tools  "} {
		gearID := insertGearWithCategory(t, db, target, "g-"+cat, cat)
		loanGearNTimes(t, db, community, target, gearID, 1)
	}

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Power Tools" {
		t.Fatalf("got %v, want [Power Tools]", tags)
	}
}

func TestDerive_PerUser_OrdersByCountDescending(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	camping := insertGearWithCategory(t, db, target, "Tent", "Camping")
	tools := insertGearWithCategory(t, db, target, "Chainsaw", "Power Tools")
	loanGearNTimes(t, db, community, target, camping, 3)
	loanGearNTimes(t, db, community, target, tools, 2)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	want := []string{"Camping", "Power Tools"}
	if !equalSlices(tags, want) {
		t.Fatalf("got %v, want %v", tags, want)
	}
}

func TestDerive_PerUser_FiltersOutsideScope(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	shared := insertCommunity(t, db, "Shared")
	other := insertCommunity(t, db, "Other")

	gearID := insertGearWithCategory(t, db, target, "Chainsaw", "Power Tools")
	loanGearNTimes(t, db, shared, target, gearID, 2)
	loanGearNTimes(t, db, other, target, gearID, 5)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{shared},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Power Tools" {
		t.Fatalf("got %v, want [Power Tools]", tags)
	}
}

func TestDerive_PerCommunity_AggregatesAcrossOwners(t *testing.T) {
	db := setupTestStorage(t)
	alice := insertUser(t, db, "Alice")
	bob := insertUser(t, db, "Bob")
	community := insertCommunity(t, db, "Crew")

	// Each owner alone has only 1 completed loan (below per-user
	// threshold), but together the community has 2 in "Power Tools".
	g1 := insertGearWithCategory(t, db, alice, "Chainsaw", "Power Tools")
	g2 := insertGearWithCategory(t, db, bob, "Drill", "Power Tools")
	loanGearNTimes(t, db, community, alice, g1, 1)
	loanGearNTimes(t, db, community, bob, g2, 1)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Power Tools" {
		t.Fatalf("per-community aggregate: got %v, want [Power Tools]", tags)
	}
}

func TestDerive_PerCommunity_MultipleCommunitiesAggregate(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	c1 := insertCommunity(t, db, "C1")
	c2 := insertCommunity(t, db, "C2")

	g1 := insertGearWithCategory(t, db, owner, "Chainsaw", "Power Tools")
	loanGearNTimes(t, db, c1, owner, g1, 1)
	loanGearNTimes(t, db, c2, owner, g1, 1)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{c1, c2},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Power Tools" {
		t.Fatalf("multi-community aggregate: got %v, want [Power Tools]", tags)
	}
}

func TestDerive_QueryBudget(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	for _, cat := range []string{"Power Tools", "Camping", "Cooking"} {
		gid := insertGearWithCategory(t, db, target, cat+" item", cat)
		shareGearInCommunity(t, db, community, gid)
		loanGearNTimes(t, db, community, target, gid, 2)
	}

	// Per-user Derive budget:
	//   loans:        3 (gear-by-owner + community_gear pivot + transfer by owner)
	//   experiences:  4 (2 for hosted + 2 for attended)
	//   requests:     4 (2 for filed + 2 for helped)
	//   total:       11 reads max
	ctx := storage.WithQueryStats(context.Background())
	storage.AssertMaxQueries(t, ctx, 11, func() {
		_, err := Derive(ctx, db, Options{
			Mode:         ModePerUser,
			OwnerID:      target,
			CommunityIDs: []string{community},
		})
		if err != nil {
			t.Fatalf("Derive: %v", err)
		}
	})
}

// ---------------------------------------------------------------------
// Shared-gear-without-loans path (no completion bonus)
// ---------------------------------------------------------------------.

func TestDerive_PerCommunity_SharedGearAloneClearsThreshold(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")

	// Two distinct gear shared with the community, neither ever
	// loaned. The shared-only weight alone (+1 each) is enough to
	// clear the threshold, so "Power Tools" surfaces.
	g1 := insertGearWithCategory(t, db, owner, "Drill", "Power Tools")
	g2 := insertGearWithCategory(t, db, owner, "Saw", "Power Tools")
	shareGearInCommunity(t, db, community, g1)
	shareGearInCommunity(t, db, community, g2)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Power Tools" {
		t.Fatalf("got %v, want [Power Tools]", tags)
	}
}

func TestDerive_PerCommunity_SingleSharedGearWithCompletedLoanClearsThreshold(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")

	// One shared gear (+1) + one completed loan of it (+1) = 2 → at
	// threshold. Completion materially raises the chance the chip
	// surfaces compared to "shared but never loaned".
	g := insertGearWithCategory(t, db, owner, "Drill", "Power Tools")
	shareGearInCommunity(t, db, community, g)
	loanGearNTimes(t, db, community, owner, g, 1)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Power Tools" {
		t.Fatalf("got %v, want [Power Tools]", tags)
	}
}

func TestDerive_PerCommunity_SingleSharedGearWithoutLoanIsBelowThreshold(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")

	// One shared gear, never loaned → +1 weight → below threshold.
	g := insertGearWithCategory(t, db, owner, "Drill", "Power Tools")
	shareGearInCommunity(t, db, community, g)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected empty (1 shared gear, no loans), got %v", tags)
	}
}

// ---------------------------------------------------------------------
// Experience-host source
// ---------------------------------------------------------------------.

// insertExperienceInState adds an Experience row with [state] owned
// by [ownerID] with the given category, plus a CommunityExperience
// row that lists it inside [communityID].
func insertExperienceInState(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, ownerID, category string,
	state models.ExperienceState,
) {
	t.Helper()
	e := &models.Experience{
		Id:       uuid.New().String(),
		OwnerId:  ownerID,
		Name:     category + " session",
		State:    state,
		Category: category,
	}
	id, err := db.Insert(context.Background(), e)
	if err != nil {
		t.Fatalf("insertExperience: %v", err)
	}
	ce := &models.CommunityExperience{
		Id:           uuid.New().String(),
		CommunityId:  communityID,
		ExperienceId: id,
	}
	if _, err := db.Insert(context.Background(), ce); err != nil {
		t.Fatalf("insertCommunityExperience: %v", err)
	}
}

// insertCompletedExperience is the convenience wrapper for the
// completed-state case. Each completed experience adds +2 weight
// (1 for shared, 1 for the completion bonus).
func insertCompletedExperience(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, ownerID, category string,
) {
	insertExperienceInState(t, db, communityID, ownerID, category,
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED)
}

func TestDerive_PerUser_ExperiencesProduceHostTags(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	// Two completed Cooking experiences clears the threshold.
	insertCompletedExperience(t, db, community, target, "Cooking")
	insertCompletedExperience(t, db, community, target, "Cooking")

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

func TestDerive_PerUser_SingleActiveExperienceIsBelowThreshold(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	// One shared but non-completed experience → +1 weight → below
	// threshold. (A single completed experience would clear the
	// threshold via the +1 completion bonus; this case proves the
	// bonus is what makes the difference.)
	insertExperienceInState(t, db, community, target, "Cooking",
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE)

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected empty (1 active experience < threshold), got %v", tags)
	}
}

func TestDerive_PerUser_SingleCompletedExperienceClearsThreshold(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	// One shared + COMPLETED experience = 1 (shared) + 1 (bonus) = 2
	// → at threshold. Completion alone is enough to surface a chip.
	insertCompletedExperience(t, db, community, target, "Cooking")

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

func TestDerive_PerCommunity_ExperiencesAggregateAcrossHosts(t *testing.T) {
	db := setupTestStorage(t)
	alice := insertUser(t, db, "Alice")
	bob := insertUser(t, db, "Bob")
	community := insertCommunity(t, db, "Crew")

	// Each host alone has 1 (sub-threshold), together 2 hits the bar.
	insertCompletedExperience(t, db, community, alice, "Cooking")
	insertCompletedExperience(t, db, community, bob, "Cooking")

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Cooking" {
		t.Fatalf("got %v, want [Cooking]", tags)
	}
}

// ---------------------------------------------------------------------
// Request-asker source
// ---------------------------------------------------------------------.

// insertFulfilledRequest adds a Request row in FULFILLED state filed
// by [requesterID] with the given category, plus a CommunityRequest
// row that shares it into [communityID].
func insertFulfilledRequest(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, requesterID, category string,
) {
	t.Helper()
	r := &models.Request{
		Id:          uuid.New().String(),
		RequesterId: requesterID,
		Title:       category + " help",
		State:       models.RequestState_REQUEST_STATE_FULFILLED,
		Category:    category,
	}
	id, err := db.Insert(context.Background(), r)
	if err != nil {
		t.Fatalf("insertRequest: %v", err)
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

func TestDerive_PerUser_RequestsProduceAskerTags(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	// Two fulfilled Home Repair requests clears the threshold.
	insertFulfilledRequest(t, db, community, target, "Home Repair")
	insertFulfilledRequest(t, db, community, target, "Home Repair")

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Home Repair" {
		t.Fatalf("got %v, want [Home Repair]", tags)
	}
}

func TestDerive_PerCommunity_RequestsAggregateAcrossRequesters(t *testing.T) {
	db := setupTestStorage(t)
	alice := insertUser(t, db, "Alice")
	bob := insertUser(t, db, "Bob")
	community := insertCommunity(t, db, "Crew")

	insertFulfilledRequest(t, db, community, alice, "Home Repair")
	insertFulfilledRequest(t, db, community, bob, "Home Repair")

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(tags) != 1 || tags[0] != "Home Repair" {
		t.Fatalf("got %v, want [Home Repair]", tags)
	}
}

// ---------------------------------------------------------------------
// Cross-source merge
// ---------------------------------------------------------------------.

func TestDerive_PerUser_MergesAllThreeSources(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	// 1 shared gear (+1) + 3 completed loans (+3) → "Power Tools" weight 4.
	gearID := insertGearWithCategory(t, db, target, "Drill", "Power Tools")
	shareGearInCommunity(t, db, community, gearID)
	loanGearNTimes(t, db, community, target, gearID, 3)

	// 2 completed Cooking experiences → 2×(1 shared + 1 bonus) = 4
	// weight for "Cooking".
	insertCompletedExperience(t, db, community, target, "Cooking")
	insertCompletedExperience(t, db, community, target, "Cooking")

	// 2 fulfilled Home Repair requests → 2×(1 + 1) = 4 weight for
	// "Home Repair".
	insertFulfilledRequest(t, db, community, target, "Home Repair")
	insertFulfilledRequest(t, db, community, target, "Home Repair")

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	// All three tags weigh 4 — alphabetic tie-break decides the
	// order (Cooking < Home Repair < Power Tools).
	want := []string{"Cooking", "Home Repair", "Power Tools"}
	if !equalSlices(tags, want) {
		t.Fatalf("got %v, want %v", tags, want)
	}
}

func TestDerive_PerUser_CollapsesSameCategoryAcrossSources(t *testing.T) {
	db := setupTestStorage(t)
	target := insertUser(t, db, "Target")
	community := insertCommunity(t, db, "Crew")

	// "Power Tools" surfaces from both loans and requests. Bare
	// category labels mean both contributions collapse into a single
	// "Power Tools" chip whose weight is the sum across sources
	// (2 completed loans = 2 + 2 fulfilled requests × (1+1) = 4 → 6).
	gearID := insertGearWithCategory(t, db, target, "Drill", "Power Tools")
	loanGearNTimes(t, db, community, target, gearID, 2)
	insertFulfilledRequest(t, db, community, target, "Power Tools")
	insertFulfilledRequest(t, db, community, target, "Power Tools")

	tags, err := Derive(context.Background(), db, Options{
		Mode:         ModePerUser,
		OwnerID:      target,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	want := []string{"Power Tools"}
	if !equalSlices(tags, want) {
		t.Fatalf("got %v, want %v (sources collapse to one chip)", tags, want)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
