package storage

import (
	"context"
	"fmt"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestProtoSQLStorage_UpdateIfNotDeleted verifies the guarded update only
// writes live rows and reports a concurrently soft-deleted row as a skipped
// no-op rather than resurrecting it. Uses Request because it carries the
// DeletedMetadata field and a media_ids array column.
func TestProtoSQLStorage_UpdateIfNotDeleted(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	userID, err := store.Insert(ctx, &models.User{Email: "u@example.com", Name: "U", Role: models.Role_ROLE_USER})
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	newRequest := func(title string) string {
		id, err := store.Insert(ctx, &models.Request{
			Title:       title,
			Description: "desc",
			RequesterId: userID,
			State:       models.RequestState_REQUEST_STATE_ACTIVE,
		})
		if err != nil {
			t.Fatalf("insert request: %v", err)
		}
		return id
	}

	t.Run("writes a live row and reports applied", func(t *testing.T) {
		id := newRequest("Live")

		live := &models.Request{}
		if err := store.GetByID(ctx, id, live); err != nil {
			t.Fatalf("get: %v", err)
		}
		live.MediaIds = []string{"media-1"}

		applied, err := store.UpdateIfNotDeleted(ctx, live)
		if err != nil {
			t.Fatalf("UpdateIfNotDeleted: %v", err)
		}
		if !applied {
			t.Fatal("expected applied=true for a live row")
		}

		got := &models.Request{}
		if err := store.GetByID(ctx, id, got); err != nil {
			t.Fatalf("get after update: %v", err)
		}
		if len(got.MediaIds) != 1 || got.MediaIds[0] != "media-1" {
			t.Errorf("expected media_ids [media-1], got %v", got.MediaIds)
		}
	})

	t.Run("skips a concurrently soft-deleted row without resurrecting it", func(t *testing.T) {
		id := newRequest("Will be deleted")

		// Load a live snapshot, mirroring an async enricher that read the row
		// before a concurrent delete.
		stale := &models.Request{}
		if err := store.GetByID(ctx, id, stale); err != nil {
			t.Fatalf("get: %v", err)
		}

		// Soft-delete the row out from under the stale snapshot.
		toDelete := &models.Request{}
		if err := store.GetByID(ctx, id, toDelete); err != nil {
			t.Fatalf("get for delete: %v", err)
		}
		toDelete.Deleted = &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: 1234567890}
		if err := store.Update(ctx, toDelete); err != nil {
			t.Fatalf("soft-delete update: %v", err)
		}

		// The stale snapshot still has Deleted == nil; a plain Update would
		// resurrect the row. The guard must make it a no-op.
		stale.MediaIds = []string{"media-stale"}
		applied, err := store.UpdateIfNotDeleted(ctx, stale)
		if err != nil {
			t.Fatalf("UpdateIfNotDeleted: %v", err)
		}
		if applied {
			t.Fatal("expected applied=false for a soft-deleted row")
		}

		// The row must still be deleted, and the stale media must not have landed.
		got := &models.Request{}
		if err := store.GetByID(ctx, id, got, QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get with IncludeDeleted: %v", err)
		}
		if got.Deleted == nil {
			t.Error("expected row to remain soft-deleted, but Deleted was cleared")
		}
		if len(got.MediaIds) != 0 {
			t.Errorf("expected stale media write to be skipped, got %v", got.MediaIds)
		}
	})

	t.Run("reports skip for a missing row", func(t *testing.T) {
		applied, err := store.UpdateIfNotDeleted(ctx, &models.Request{
			Id:          "00000000-0000-0000-0000-000000000000",
			Title:       "ghost",
			RequesterId: userID,
		})
		if err != nil {
			t.Fatalf("UpdateIfNotDeleted: %v", err)
		}
		if applied {
			t.Fatal("expected applied=false for a missing row")
		}
	})
}

func TestProtoSQLStorage_Update(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("successful update", func(t *testing.T) {
		// Insert a gear
		testGear := &models.Gear{
			Name:        "Original Name",
			Description: "Original Description",
			OwnerId:     "owner123",
			State:       models.GearState_GEAR_STATE_UNAVAILABLE,
		}

		gearID, err := storage.Insert(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Update the gear
		testGear.Name = "Updated Name"
		testGear.Description = "Updated Description"
		testGear.State = models.GearState_GEAR_STATE_AVAILABLE

		err = storage.Update(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to update gear: %v", err)
		}

		// Retrieve and verify
		retrievedGear := &models.Gear{}
		err = storage.GetByID(ctx, gearID, retrievedGear)
		if err != nil {
			t.Fatalf("Failed to retrieve updated gear: %v", err)
		}

		if retrievedGear.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got '%s'", retrievedGear.Name)
		}

		if retrievedGear.Description != "Updated Description" {
			t.Errorf("Expected description 'Updated Description', got '%s'", retrievedGear.Description)
		}

		if retrievedGear.State != models.GearState_GEAR_STATE_AVAILABLE {
			t.Errorf("Expected state AVAILABLE, got %v", retrievedGear.State)
		}

		if retrievedGear.OwnerId != "owner123" {
			t.Errorf("Expected owner ID unchanged, got %s", retrievedGear.OwnerId)
		}
	})

	t.Run("update with empty ID fails", func(t *testing.T) {
		testGear := &models.Gear{
			Name: "Gear without ID",
		}

		err := storage.Update(ctx, testGear)
		if err == nil {
			t.Fatal("Expected error when updating gear without ID")
		}
	})

	t.Run("update non-existent record fails", func(t *testing.T) {
		testGear := &models.Gear{
			Id:   "non-existent-id",
			Name: "Ghost Gear",
		}

		err := storage.Update(ctx, testGear)
		if err == nil {
			t.Fatal("Expected error when updating non-existent gear")
		}
	})

	t.Run("update transfer state", func(t *testing.T) {
		// Insert a transfer
		testTransfer := &models.Transfer{
			GearId:       "gear123",
			OwnerId:      "sharer456",
			RecipientId:  "recipient789",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
			CommunityId:  "community123",
		}

		transferID, err := storage.Insert(ctx, testTransfer)
		if err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		// Update transfer state
		testTransfer.State = models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED

		err = storage.Update(ctx, testTransfer)
		if err != nil {
			t.Fatalf("Failed to update transfer: %v", err)
		}

		// Retrieve and verify
		retrievedTransfer := &models.Transfer{}
		err = storage.GetByID(ctx, transferID, retrievedTransfer)
		if err != nil {
			t.Fatalf("Failed to retrieve updated transfer: %v", err)
		}

		if retrievedTransfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			t.Errorf("Expected state RECIPIENT_SELECTED, got %v", retrievedTransfer.State)
		}
	})
}

func TestProtoSQLStorage_Delete(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("delete existing gear", func(t *testing.T) {
		// Insert gear
		testGear := &models.Gear{
			Name:        "Test Drill",
			Description: "A test drill",
			OwnerId:     "user123",
		}

		gearID, err := storage.Insert(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Verify gear exists
		retrievedGear := &models.Gear{}
		err = storage.GetByID(ctx, gearID, retrievedGear)
		if err != nil {
			t.Fatalf("Failed to retrieve gear before deletion: %v", err)
		}

		// Delete gear
		err = storage.Delete(ctx, retrievedGear)
		if err != nil {
			t.Fatalf("Failed to delete gear: %v", err)
		}

		// Verify gear is deleted
		err = storage.GetByID(ctx, gearID, &models.Gear{})
		if err == nil {
			t.Fatal("Expected error when retrieving deleted gear")
		}
	})

	t.Run("delete non-existent gear fails", func(t *testing.T) {
		// Try to delete gear with non-existent ID
		testGear := &models.Gear{
			Id: "nonexistent-id",
		}

		err := storage.Delete(ctx, testGear)
		if err == nil {
			t.Fatal("Expected error when deleting non-existent gear")
		}
	})

	t.Run("delete with empty ID fails", func(t *testing.T) {
		testGear := &models.Gear{}

		err := storage.Delete(ctx, testGear)
		if err == nil {
			t.Fatal("Expected error when deleting gear with empty ID")
		}
	})

	t.Run("delete removes from list", func(t *testing.T) {
		// Insert multiple gear items
		var gearIDs []string
		for i := 0; i < 3; i++ {
			testGear := &models.Gear{
				Name:    fmt.Sprintf("Gear %d", i),
				OwnerId: "user123",
			}
			gearID, err := storage.Insert(ctx, testGear)
			if err != nil {
				t.Fatalf("Failed to insert gear %d: %v", i, err)
			}
			gearIDs = append(gearIDs, gearID)
		}

		// List all gear
		messages, err := storage.ListAll(ctx, &models.Gear{})
		if err != nil {
			t.Fatalf("Failed to list gear: %v", err)
		}

		initialCount := len(messages)

		// Delete one gear
		gearToDelete := &models.Gear{Id: gearIDs[1]}
		err = storage.GetByID(ctx, gearIDs[1], gearToDelete)
		if err != nil {
			t.Fatalf("Failed to get gear for deletion: %v", err)
		}

		err = storage.Delete(ctx, gearToDelete)
		if err != nil {
			t.Fatalf("Failed to delete gear: %v", err)
		}

		// List again and verify count decreased
		messages, err = storage.ListAll(ctx, &models.Gear{})
		if err != nil {
			t.Fatalf("Failed to list gear after deletion: %v", err)
		}

		if len(messages) != initialCount-1 {
			t.Errorf("Expected %d items after deletion, got %d", initialCount-1, len(messages))
		}

		// Verify deleted gear is not in list
		for _, msg := range messages {
			gear := msg.(*models.Gear)
			if gear.Id == gearIDs[1] {
				t.Error("Deleted gear should not appear in list")
			}
		}
	})

	t.Run("delete user", func(t *testing.T) {
		// Insert user
		testUser := &models.User{
			Name:  "Test User",
			Email: "test@example.com",
			Role:  models.Role_ROLE_USER,
		}

		userID, err := storage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert user: %v", err)
		}

		// Delete user
		retrievedUser := &models.User{}
		err = storage.GetByID(ctx, userID, retrievedUser)
		if err != nil {
			t.Fatalf("Failed to retrieve user: %v", err)
		}

		err = storage.Delete(ctx, retrievedUser)
		if err != nil {
			t.Fatalf("Failed to delete user: %v", err)
		}

		// Verify user is deleted
		err = storage.GetByID(ctx, userID, &models.User{})
		if err == nil {
			t.Fatal("Expected error when retrieving deleted user")
		}
	})

	t.Run("delete transfer", func(t *testing.T) {
		// Insert transfer
		testTransfer := &models.Transfer{
			GearId:       "gear123",
			OwnerId:      "sharer456",
			RecipientId:  "recipient789",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
			CommunityId:  "community123",
		}

		transferID, err := storage.Insert(ctx, testTransfer)
		if err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		// Delete transfer
		retrievedTransfer := &models.Transfer{}
		err = storage.GetByID(ctx, transferID, retrievedTransfer)
		if err != nil {
			t.Fatalf("Failed to retrieve transfer: %v", err)
		}

		err = storage.Delete(ctx, retrievedTransfer)
		if err != nil {
			t.Fatalf("Failed to delete transfer: %v", err)
		}

		// Verify transfer is deleted
		err = storage.GetByID(ctx, transferID, &models.Transfer{})
		if err == nil {
			t.Fatal("Expected error when retrieving deleted transfer")
		}
	})
}
