// Tests for generic type-safe storage wrappers.

package storage

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGetByIDsGeneric(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	g1 := &models.Gear{Name: "Drill", Description: "Power drill"}
	g2 := &models.Gear{Name: "Saw", Description: "Circular saw"}
	g3 := &models.Gear{Name: "Sander", Description: "Belt sander"}

	id1, err := storage.Insert(ctx, g1)
	if err != nil {
		t.Fatalf("insert g1: %v", err)
	}
	id2, err := storage.Insert(ctx, g2)
	if err != nil {
		t.Fatalf("insert g2: %v", err)
	}
	id3, err := storage.Insert(ctx, g3)
	if err != nil {
		t.Fatalf("insert g3: %v", err)
	}

	result, err := GetByIDs[*models.Gear](storage, ctx, []string{id1, id3})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	// No type assertion needed — result values are already *models.Gear.
	if result[id1].Name != "Drill" {
		t.Errorf("expected Drill, got %s", result[id1].Name)
	}
	if result[id3].Name != "Sander" {
		t.Errorf("expected Sander, got %s", result[id3].Name)
	}
	if _, ok := result[id2]; ok {
		t.Error("should not contain id2")
	}
}

func TestGetByIDsGenericEmpty(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	result, err := GetByIDs[*models.Gear](storage, ctx, nil)
	if err != nil {
		t.Fatalf("GetByIDs empty: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d items", len(result))
	}
}

func TestQueryByFieldGeneric(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	_, err := storage.Insert(ctx, &models.Gear{Name: "Drill", OwnerId: "user-1"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = storage.Insert(ctx, &models.Gear{Name: "Saw", OwnerId: "user-1"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = storage.Insert(ctx, &models.Gear{Name: "Hammer", OwnerId: "user-2"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := QueryByField[*models.Gear](storage, ctx, "owner_id", "user-1")
	if err != nil {
		t.Fatalf("QueryByField: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	// Verify typed access without assertion.
	names := make(map[string]bool)
	for _, gear := range result {
		names[gear.Name] = true
	}
	if !names["Drill"] || !names["Saw"] {
		t.Errorf("expected Drill and Saw, got %v", names)
	}
}

func TestQueryByFieldsGeneric(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	_, err := storage.Insert(ctx, &models.CommunityGear{
		CommunityId: "comm-1", GearId: "gear-1", Archived: false,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = storage.Insert(ctx, &models.CommunityGear{
		CommunityId: "comm-1", GearId: "gear-2", Archived: true,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := QueryByFields[*models.CommunityGear](storage, ctx, map[string]any{
		"community_id": "comm-1",
		"archived":     false,
	})
	if err != nil {
		t.Fatalf("QueryByFields: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].GearId != "gear-1" {
		t.Errorf("expected gear-1, got %s", result[0].GearId)
	}
}

func TestQueryByFieldInGeneric(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	_, err := storage.Insert(ctx, &models.Gear{Id: "g1", Name: "Drill"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = storage.Insert(ctx, &models.Gear{Id: "g2", Name: "Saw"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = storage.Insert(ctx, &models.Gear{Id: "g3", Name: "Hammer"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := QueryByFieldIn[*models.Gear](storage, ctx, "id", []string{"g1", "g3"})
	if err != nil {
		t.Fatalf("QueryByFieldIn: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	names := make(map[string]bool)
	for _, gear := range result {
		names[gear.Name] = true
	}
	if !names["Drill"] || !names["Hammer"] {
		t.Errorf("expected Drill and Hammer, got %v", names)
	}
}

func TestQueryByFieldInGenericEmpty(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	result, err := QueryByFieldIn[*models.Gear](storage, ctx, "id", nil)
	if err != nil {
		t.Fatalf("QueryByFieldIn empty: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %d items", len(result))
	}
}

func TestCollectField(t *testing.T) {
	items := []*models.CommunityGear{
		{GearId: "g1", CommunityId: "c1"},
		{GearId: "g2", CommunityId: "c1"},
		{GearId: "g3", CommunityId: "c2"},
	}

	gearIDs := CollectField(items, func(cg *models.CommunityGear) string { return cg.GearId })
	if len(gearIDs) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(gearIDs))
	}
	if gearIDs[0] != "g1" || gearIDs[1] != "g2" || gearIDs[2] != "g3" {
		t.Errorf("expected [g1 g2 g3], got %v", gearIDs)
	}

	// CollectField on empty slice returns empty slice (not nil).
	empty := CollectField([]*models.Gear{}, func(g *models.Gear) string { return g.Id })
	if len(empty) != 0 {
		t.Errorf("expected empty, got %d", len(empty))
	}
}

func TestToMap(t *testing.T) {
	items := []*models.CommunityGear{
		{GearId: "g1", CommunityId: "c1"},
		{GearId: "g2", CommunityId: "c2"},
		{GearId: "g3", CommunityId: "c1"},
	}

	byGear := ToMap(items, func(cg *models.CommunityGear) string { return cg.GearId })
	if len(byGear) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(byGear))
	}
	if byGear["g1"].CommunityId != "c1" {
		t.Errorf("expected c1 for g1, got %s", byGear["g1"].CommunityId)
	}
	if byGear["g2"].CommunityId != "c2" {
		t.Errorf("expected c2 for g2, got %s", byGear["g2"].CommunityId)
	}

	// Last-wins on duplicate keys.
	dupes := []*models.CommunityGear{
		{GearId: "g1", CommunityId: "first"},
		{GearId: "g1", CommunityId: "second"},
	}
	byGear2 := ToMap(dupes, func(cg *models.CommunityGear) string { return cg.GearId })
	if byGear2["g1"].CommunityId != "second" {
		t.Errorf("expected last-wins, got %s", byGear2["g1"].CommunityId)
	}

	// Empty slice returns empty map.
	emptyMap := ToMap([]*models.Gear{}, func(g *models.Gear) string { return g.Id })
	if len(emptyMap) != 0 {
		t.Errorf("expected empty map, got %d", len(emptyMap))
	}
}
