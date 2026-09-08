// Batch storage tests: GetByIDs, QueryByFieldIn, and InsertBatch.

package storage

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGetByIDs(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("returns matching items by ID", func(t *testing.T) {
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

		result, err := storage.GetByIDs(ctx, []string{id1, id3}, &models.Gear{})
		if err != nil {
			t.Fatalf("GetByIDs: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result))
		}
		if result[id1].(*models.Gear).Name != "Drill" {
			t.Errorf("expected Drill, got %s", result[id1].(*models.Gear).Name)
		}
		if result[id3].(*models.Gear).Name != "Sander" {
			t.Errorf("expected Sander, got %s", result[id3].(*models.Gear).Name)
		}
		if _, ok := result[id2]; ok {
			t.Error("should not contain id2")
		}
	})

	t.Run("empty IDs returns empty map", func(t *testing.T) {
		result, err := storage.GetByIDs(ctx, nil, &models.Gear{})
		if err != nil {
			t.Fatalf("GetByIDs empty: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty map, got %d entries", len(result))
		}
	})

	t.Run("missing IDs silently omitted", func(t *testing.T) {
		g := &models.Gear{Name: "Hammer"}
		id, err := storage.Insert(ctx, g)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		result, err := storage.GetByIDs(ctx, []string{id, "nonexistent-id"}, &models.Gear{})
		if err != nil {
			t.Fatalf("GetByIDs: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		if result[id].(*models.Gear).Name != "Hammer" {
			t.Errorf("expected Hammer, got %s", result[id].(*models.Gear).Name)
		}
	})

	t.Run("excludes soft-deleted by default", func(t *testing.T) {
		user := &models.User{Email: "batch-del@example.com", Name: "Batch Del", Role: models.Role_ROLE_USER}
		userID, err := storage.Insert(ctx, user)
		if err != nil {
			t.Fatalf("insert user: %v", err)
		}

		active := &models.Request{Title: "Active", RequesterId: userID, State: models.RequestState_REQUEST_STATE_ACTIVE}
		deleted := &models.Request{
			Title: "Deleted", RequesterId: userID, State: models.RequestState_REQUEST_STATE_ACTIVE,
			Deleted: &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: 1234567890},
		}

		activeID, err := storage.Insert(ctx, active)
		if err != nil {
			t.Fatalf("insert active: %v", err)
		}
		deletedID, err := storage.Insert(ctx, deleted)
		if err != nil {
			t.Fatalf("insert deleted: %v", err)
		}

		// Without IncludeDeleted — should only return active.
		result, err := storage.GetByIDs(ctx, []string{activeID, deletedID}, &models.Request{})
		if err != nil {
			t.Fatalf("GetByIDs: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		if _, ok := result[activeID]; !ok {
			t.Error("expected active request in results")
		}

		// With IncludeDeleted — should return both.
		result2, err := storage.GetByIDs(ctx, []string{activeID, deletedID}, &models.Request{}, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("GetByIDs with IncludeDeleted: %v", err)
		}
		if len(result2) != 2 {
			t.Fatalf("expected 2 results with IncludeDeleted, got %d", len(result2))
		}
	})

	t.Run("unregistered type returns error", func(t *testing.T) {
		_, err := storage.GetByIDs(ctx, []string{"a"}, &emptypb.Empty{})
		if err == nil {
			t.Fatal("expected error for unregistered type")
		}
	})
}

func TestQueryByFieldIn(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("returns items matching field values", func(t *testing.T) {
		user1 := &models.User{Email: "qbfi-1@example.com", Name: "User1", Role: models.Role_ROLE_USER}
		user2 := &models.User{Email: "qbfi-2@example.com", Name: "User2", Role: models.Role_ROLE_USER}
		user3 := &models.User{Email: "qbfi-3@example.com", Name: "User3", Role: models.Role_ROLE_USER}

		u1id, err := storage.Insert(ctx, user1)
		if err != nil {
			t.Fatalf("insert user1: %v", err)
		}
		_, err = storage.Insert(ctx, user2)
		if err != nil {
			t.Fatalf("insert user2: %v", err)
		}
		u3id, err := storage.Insert(ctx, user3)
		if err != nil {
			t.Fatalf("insert user3: %v", err)
		}

		g1 := &models.Gear{Name: "Item1", OwnerId: u1id}
		g2 := &models.Gear{Name: "Item2", OwnerId: u1id}
		g3 := &models.Gear{Name: "Item3", OwnerId: u3id}

		_, err = storage.Insert(ctx, g1)
		if err != nil {
			t.Fatalf("insert g1: %v", err)
		}
		_, err = storage.Insert(ctx, g2)
		if err != nil {
			t.Fatalf("insert g2: %v", err)
		}
		_, err = storage.Insert(ctx, g3)
		if err != nil {
			t.Fatalf("insert g3: %v", err)
		}

		results, err := storage.QueryByFieldIn(ctx, "owner_id", []string{u1id, u3id}, &models.Gear{})
		if err != nil {
			t.Fatalf("QueryByFieldIn: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("empty values returns nil", func(t *testing.T) {
		results, err := storage.QueryByFieldIn(ctx, "owner_id", nil, &models.Gear{})
		if err != nil {
			t.Fatalf("QueryByFieldIn empty: %v", err)
		}
		if results != nil {
			t.Errorf("expected nil, got %d results", len(results))
		}
	})

	t.Run("excludes soft-deleted by default", func(t *testing.T) {
		user := &models.User{Email: "qbfi-del@example.com", Name: "FieldInDel", Role: models.Role_ROLE_USER}
		userID, err := storage.Insert(ctx, user)
		if err != nil {
			t.Fatalf("insert user: %v", err)
		}

		active := &models.Request{Title: "Active FieldIn", RequesterId: userID, State: models.RequestState_REQUEST_STATE_ACTIVE}
		deleted := &models.Request{
			Title: "Deleted FieldIn", RequesterId: userID, State: models.RequestState_REQUEST_STATE_ACTIVE,
			Deleted: &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: 1234567890},
		}
		_, err = storage.Insert(ctx, active)
		if err != nil {
			t.Fatalf("insert active: %v", err)
		}
		_, err = storage.Insert(ctx, deleted)
		if err != nil {
			t.Fatalf("insert deleted: %v", err)
		}

		results, err := storage.QueryByFieldIn(ctx, "requester_id", []string{userID}, &models.Request{})
		if err != nil {
			t.Fatalf("QueryByFieldIn: %v", err)
		}

		for _, r := range results {
			req := r.(*models.Request)
			if req.Deleted != nil {
				t.Error("soft-deleted request should be excluded by default")
			}
		}
	})

	t.Run("unregistered type returns error", func(t *testing.T) {
		_, err := storage.QueryByFieldIn(ctx, "id", []string{"a"}, &emptypb.Empty{})
		if err == nil {
			t.Fatal("expected error for unregistered type")
		}
	})
}

func TestInsertBatch(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("inserts multiple items and returns IDs", func(t *testing.T) {
		msgs := []proto.Message{
			&models.Gear{Name: "Batch1", Description: "First"},
			&models.Gear{Name: "Batch2", Description: "Second"},
			&models.Gear{Name: "Batch3", Description: "Third"},
		}

		ids, err := storage.InsertBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("InsertBatch: %v", err)
		}
		if len(ids) != 3 {
			t.Fatalf("expected 3 IDs, got %d", len(ids))
		}

		// Verify each was stored correctly.
		for i, id := range ids {
			got := &models.Gear{}
			if err := storage.GetByID(ctx, id, got); err != nil {
				t.Fatalf("GetByID(%s): %v", id, err)
			}
			expected := msgs[i].(*models.Gear).Name
			if got.Name != expected {
				t.Errorf("item %d: expected name %q, got %q", i, expected, got.Name)
			}
		}
	})

	t.Run("generates UUIDs for empty IDs", func(t *testing.T) {
		msgs := []proto.Message{
			&models.Gear{Name: "AutoID1"},
			&models.Gear{Name: "AutoID2"},
		}

		ids, err := storage.InsertBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("InsertBatch: %v", err)
		}
		for i, id := range ids {
			if id == "" {
				t.Errorf("item %d: expected non-empty ID", i)
			}
		}
		// IDs should be unique.
		if ids[0] == ids[1] {
			t.Error("expected unique IDs")
		}
	})

	t.Run("preserves existing IDs", func(t *testing.T) {
		msgs := []proto.Message{
			&models.Gear{Id: "keep-this-id", Name: "PresetID"},
		}

		ids, err := storage.InsertBatch(ctx, msgs)
		if err != nil {
			t.Fatalf("InsertBatch: %v", err)
		}
		if ids[0] != "keep-this-id" {
			t.Errorf("expected keep-this-id, got %s", ids[0])
		}
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		ids, err := storage.InsertBatch(ctx, nil)
		if err != nil {
			t.Fatalf("InsertBatch empty: %v", err)
		}
		if ids != nil {
			t.Errorf("expected nil, got %v", ids)
		}
	})

	t.Run("rejects mixed types", func(t *testing.T) {
		msgs := []proto.Message{
			&models.Gear{Name: "Gear"},
			&models.User{Name: "User"},
		}

		_, err := storage.InsertBatch(ctx, msgs)
		if err == nil {
			t.Fatal("expected error for mixed types")
		}
	})

	t.Run("unregistered type returns error", func(t *testing.T) {
		msgs := []proto.Message{
			&emptypb.Empty{},
		}
		_, err := storage.InsertBatch(ctx, msgs)
		if err == nil {
			t.Fatal("expected error for unregistered type")
		}
	})
}
