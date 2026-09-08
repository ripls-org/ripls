package available_now

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
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

func insertAvailableGear(t *testing.T, db *storage.ProtoSQLStorage, owner, name string) string {
	t.Helper()
	g := &models.Gear{
		Id:               uuid.New().String(),
		OwnerId:          owner,
		Name:             name,
		State:            models.GearState_GEAR_STATE_AVAILABLE,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	id, err := db.Insert(context.Background(), g)
	if err != nil {
		t.Fatalf("insertAvailableGear: %v", err)
	}
	return id
}

func listGearInCommunity(t *testing.T, db *storage.ProtoSQLStorage, community, gearID string) {
	t.Helper()
	cg := &models.CommunityGear{
		Id:           uuid.New().String(),
		CommunityId:  community,
		GearId:       gearID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	if _, err := db.Insert(context.Background(), cg); err != nil {
		t.Fatalf("listGearInCommunity: %v", err)
	}
}

func TestGather_EmptyCommunityIDs(t *testing.T) {
	db := setupTestStorage(t)
	items, err := Gather(context.Background(), db, Options{
		Mode:         ModePerUser,
		TargetUserID: "u",
	})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty, got %d", len(items))
	}
}

func TestGather_PerUser_MissingTargetID(t *testing.T) {
	db := setupTestStorage(t)
	_, err := Gather(context.Background(), db, Options{
		Mode:         ModePerUser,
		CommunityIDs: []string{"c"},
	})
	if err == nil {
		t.Fatalf("expected error when TargetUserID empty in per-user mode")
	}
}

func TestGather_PerUser_SurfacesGearInSharedCommunity(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")
	gearID := insertAvailableGear(t, db, owner, "Chainsaw")
	listGearInCommunity(t, db, community, gearID)

	items, err := Gather(context.Background(), db, Options{
		Mode:         ModePerUser,
		TargetUserID: owner,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Chainsaw" {
		t.Fatalf("per-user gear: got %v, want one Chainsaw", items)
	}
	if items[0].GetMiniLabel() != "BORROW" {
		t.Errorf("mini-label: got %q, want BORROW", items[0].GetMiniLabel())
	}
}

func TestGather_PerCommunity_SurfacesGearFromAnyOwner(t *testing.T) {
	db := setupTestStorage(t)
	alice := insertUser(t, db, "Alice")
	bob := insertUser(t, db, "Bob")
	community := insertCommunity(t, db, "Crew")
	g1 := insertAvailableGear(t, db, alice, "Chainsaw")
	g2 := insertAvailableGear(t, db, bob, "Drill")
	listGearInCommunity(t, db, community, g1)
	listGearInCommunity(t, db, community, g2)

	items, err := Gather(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("per-community gear: got %d items, want 2", len(items))
	}
	titles := map[string]bool{items[0].Title: true, items[1].Title: true}
	if !titles["Chainsaw"] || !titles["Drill"] {
		t.Fatalf("got titles %v, want Chainsaw and Drill", titles)
	}
}

func TestGather_PerCommunity_ExcludesGearOutsideScope(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	inScope := insertCommunity(t, db, "In Scope")
	outOfScope := insertCommunity(t, db, "Out of Scope")
	g1 := insertAvailableGear(t, db, owner, "Visible")
	g2 := insertAvailableGear(t, db, owner, "Hidden")
	listGearInCommunity(t, db, inScope, g1)
	listGearInCommunity(t, db, outOfScope, g2)

	items, err := Gather(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{inScope},
	})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Visible" {
		t.Fatalf("got %v, want one Visible", items)
	}
}

// TestGather_PopulatesItem verifies that emitted items carry the
// unified context_id + kind + title + mini_label fields the client
// reads for routing and rendering.
func TestGather_PopulatesItem(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")
	gearID := insertAvailableGear(t, db, owner, "Chainsaw")
	listGearInCommunity(t, db, community, gearID)

	items, err := Gather(context.Background(), db, Options{
		Mode:         ModePerCommunity,
		CommunityIDs: []string{community},
	})
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item.ContextId != gearID {
		t.Errorf("ContextId=%q, want %q", item.ContextId, gearID)
	}
	if item.Title != "Chainsaw" {
		t.Errorf("Title=%q, want Chainsaw", item.Title)
	}
	if item.Kind != api.ItemKind_ITEM_KIND_GEAR {
		t.Errorf("Kind=%v, want ITEM_KIND_GEAR", item.Kind)
	}
	if item.GetMiniLabel() != "BORROW" {
		t.Errorf("MiniLabel=%q, want BORROW", item.GetMiniLabel())
	}
}

func TestGather_QueryBudget(t *testing.T) {
	db := setupTestStorage(t)
	owner := insertUser(t, db, "Owner")
	community := insertCommunity(t, db, "Crew")
	for i := 0; i < 4; i++ {
		g := insertAvailableGear(t, db, owner, "g")
		listGearInCommunity(t, db, community, g)
	}
	ctx := storage.WithQueryStats(context.Background())
	storage.AssertMaxQueries(t, ctx, 6, func() {
		_, err := Gather(ctx, db, Options{
			Mode:         ModePerCommunity,
			CommunityIDs: []string{community},
		})
		if err != nil {
			t.Fatalf("Gather: %v", err)
		}
	})
}
