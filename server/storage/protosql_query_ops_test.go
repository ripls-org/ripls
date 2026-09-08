package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestSoftDeletionFiltering tests that soft-deleted items are properly filtered from queries.
// Uses Request model since it has the DeletedMetadata field.
func TestSoftDeletionFiltering(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test user first (for requester_id)
	testUser := &models.User{
		Email: "requester@example.com",
		Name:  "Test Requester",
		Role:  models.Role_ROLE_USER,
	}
	userID, err := storage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Helper to create a request
	createRequest := func(title string, deleted bool) string {
		req := &models.Request{
			Title:       title,
			Description: "Test request description",
			RequesterId: userID,
			State:       models.RequestState_REQUEST_STATE_ACTIVE,
		}
		if deleted {
			req.Deleted = &models.DeletedMetadata{
				DeletedByUserId:  userID,
				DeletedAtUnixSec: 1234567890,
			}
		}
		id, err := storage.Insert(ctx, req)
		if err != nil {
			t.Fatalf("Failed to insert request: %v", err)
		}
		return id
	}

	t.Run("GetByID excludes deleted items by default", func(t *testing.T) {
		// Create a deleted request
		deletedID := createRequest("Deleted Request for GetByID", true)

		// Try to get it without IncludeDeleted - should fail
		req := &models.Request{}
		err := storage.GetByID(ctx, deletedID, req)
		if err == nil {
			t.Error("Expected error when getting deleted request, but got none")
		}
		if err != nil && !strings.Contains(err.Error(), "record not found") {
			t.Errorf("Expected 'record not found' error, got: %v", err)
		}
	})

	t.Run("GetByID with IncludeDeleted returns deleted items", func(t *testing.T) {
		// Create a deleted request
		deletedID := createRequest("Deleted Request for GetByID Include", true)

		// Get with IncludeDeleted - should succeed
		req := &models.Request{}
		err := storage.GetByID(ctx, deletedID, req, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Errorf("Expected to retrieve deleted request with IncludeDeleted, got error: %v", err)
		}
		if req.Title != "Deleted Request for GetByID Include" {
			t.Errorf("Expected title 'Deleted Request for GetByID Include', got '%s'", req.Title)
		}
		if req.Deleted == nil {
			t.Error("Expected deleted metadata to be present")
		}
	})

	t.Run("GetByID returns active items normally", func(t *testing.T) {
		// Create an active (non-deleted) request
		activeID := createRequest("Active Request for GetByID", false)

		// Should be retrievable without IncludeDeleted
		req := &models.Request{}
		err := storage.GetByID(ctx, activeID, req)
		if err != nil {
			t.Errorf("Expected to retrieve active request, got error: %v", err)
		}
		if req.Title != "Active Request for GetByID" {
			t.Errorf("Expected title 'Active Request for GetByID', got '%s'", req.Title)
		}
	})

	t.Run("QueryByField excludes deleted items by default", func(t *testing.T) {
		// Create one active and one deleted request with same requester
		activeID := createRequest("Active for QueryByField", false)
		_ = createRequest("Deleted for QueryByField", true)

		// Query by requester_id - should only return active request
		results, err := storage.QueryByField(ctx, "requester_id", userID, &models.Request{})
		if err != nil {
			t.Fatalf("QueryByField failed: %v", err)
		}

		// Count how many of our test requests are in results
		foundActive := false
		foundDeleted := false
		for _, msg := range results {
			req := msg.(*models.Request)
			if req.Id == activeID {
				foundActive = true
			}
			if req.Title == "Deleted for QueryByField" {
				foundDeleted = true
			}
		}

		if !foundActive {
			t.Error("Expected to find active request in QueryByField results")
		}
		if foundDeleted {
			t.Error("Did not expect to find deleted request in QueryByField results")
		}
	})

	t.Run("QueryByField with IncludeDeleted returns all items", func(t *testing.T) {
		// Query with IncludeDeleted - should return both
		results, err := storage.QueryByField(ctx, "requester_id", userID, &models.Request{}, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("QueryByField with IncludeDeleted failed: %v", err)
		}

		// Should find at least one deleted request
		foundDeleted := false
		for _, msg := range results {
			req := msg.(*models.Request)
			if req.Deleted != nil {
				foundDeleted = true
				break
			}
		}

		if !foundDeleted {
			t.Error("Expected to find at least one deleted request with IncludeDeleted")
		}
	})

	t.Run("QueryByFields excludes deleted items by default", func(t *testing.T) {
		// Create specific requests for this test
		activeID := createRequest("Active for QueryByFields", false)
		deletedID := createRequest("Deleted for QueryByFields", true)

		// Query by requester_id field
		results, err := storage.QueryByFields(ctx, map[string]any{
			"requester_id": userID,
		}, &models.Request{})
		if err != nil {
			t.Fatalf("QueryByFields failed: %v", err)
		}

		// Check results
		foundActive := false
		foundDeleted := false
		for _, msg := range results {
			req := msg.(*models.Request)
			if req.Id == activeID {
				foundActive = true
			}
			if req.Id == deletedID {
				foundDeleted = true
			}
		}

		if !foundActive {
			t.Error("Expected to find active request in QueryByFields results")
		}
		if foundDeleted {
			t.Error("Did not expect to find deleted request in QueryByFields results")
		}
	})

	t.Run("QueryByFields with IncludeDeleted returns all items", func(t *testing.T) {
		// Query with IncludeDeleted
		results, err := storage.QueryByFields(ctx, map[string]any{
			"requester_id": userID,
		}, &models.Request{}, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("QueryByFields with IncludeDeleted failed: %v", err)
		}

		// Should find deleted requests
		foundDeleted := false
		for _, msg := range results {
			req := msg.(*models.Request)
			if req.Deleted != nil {
				foundDeleted = true
				break
			}
		}

		if !foundDeleted {
			t.Error("Expected to find at least one deleted request with IncludeDeleted")
		}
	})

	t.Run("models without deleted field are unaffected", func(t *testing.T) {
		// Gear doesn't have deleted field yet, so all queries should work normally
		testGear := &models.Gear{
			Name:        "Test Gear for Deletion Filter",
			Description: "Testing that non-deleted models work",
			OwnerId:     userID,
		}

		gearID, err := storage.Insert(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// GetByID should work
		gear := &models.Gear{}
		err = storage.GetByID(ctx, gearID, gear)
		if err != nil {
			t.Errorf("GetByID for gear (no deleted field) failed: %v", err)
		}

		// QueryByField should work
		results, err := storage.QueryByField(ctx, "owner_id", userID, &models.Gear{})
		if err != nil {
			t.Errorf("QueryByField for gear (no deleted field) failed: %v", err)
		}
		if len(results) == 0 {
			t.Error("Expected to find gear in QueryByField results")
		}
	})
}

func testDeleteByField(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Insert multiple community events for different communities.
	community1 := "comm-1"
	community2 := "comm-2"
	for i := 0; i < 3; i++ {
		_, err := storage.Insert(ctx, &models.CommunityEvent{
			CommunityId: community1,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			ActorId:     "user-1",
		})
		if err != nil {
			t.Fatalf("Insert event for community1: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		_, err := storage.Insert(ctx, &models.CommunityEvent{
			CommunityId: community2,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			ActorId:     "user-2",
		})
		if err != nil {
			t.Fatalf("Insert event for community2: %v", err)
		}
	}

	// Delete events for community1 only.
	deleted, err := storage.DeleteByField(ctx, "community_event", "community_id", community1)
	if err != nil {
		t.Fatalf("DeleteByField failed: %v", err)
	}
	if deleted != 3 {
		t.Errorf("Expected 3 deleted, got %d", deleted)
	}

	// Verify community2 events are untouched.
	remaining, err := storage.QueryByField(ctx, "community_id", community2, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("QueryByField failed: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining events for community2, got %d", len(remaining))
	}

	// Verify community1 events are gone.
	gone, err := storage.QueryByField(ctx, "community_id", community1, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("QueryByField failed: %v", err)
	}
	if len(gone) != 0 {
		t.Errorf("Expected 0 events for community1, got %d", len(gone))
	}

	// Delete with no matching records returns 0.
	deleted, err = storage.DeleteByField(ctx, "community_event", "community_id", "nonexistent")
	if err != nil {
		t.Fatalf("DeleteByField for nonexistent: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Expected 0 deleted for nonexistent, got %d", deleted)
	}
}

func testDeleteByFieldIn(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Insert users with different owners.
	users := []struct {
		name  string
		email string
	}{
		{"User A", "a@test.com"},
		{"User B", "b@test.com"},
		{"User C", "c@test.com"},
		{"User D", "d@test.com"},
	}

	ids := make([]string, len(users))
	for i, u := range users {
		id, err := storage.Insert(ctx, &models.User{
			Name:  u.name,
			Email: u.email,
		})
		if err != nil {
			t.Fatalf("Insert user %s: %v", u.name, err)
		}
		ids[i] = id
	}

	// Delete users A and C by ID.
	deleted, err := storage.DeleteByFieldIn(ctx, "user", "id", []string{ids[0], ids[2]})
	if err != nil {
		t.Fatalf("DeleteByFieldIn failed: %v", err)
	}
	if deleted != 2 {
		t.Errorf("Expected 2 deleted, got %d", deleted)
	}

	// Verify A and C are gone.
	err = storage.GetByID(ctx, ids[0], &models.User{})
	if err == nil {
		t.Error("Expected User A to be deleted")
	}
	err = storage.GetByID(ctx, ids[2], &models.User{})
	if err == nil {
		t.Error("Expected User C to be deleted")
	}

	// Verify B and D still exist.
	var userB models.User
	if err := storage.GetByID(ctx, ids[1], &userB); err != nil {
		t.Errorf("User B should still exist: %v", err)
	}
	var userD models.User
	if err := storage.GetByID(ctx, ids[3], &userD); err != nil {
		t.Errorf("User D should still exist: %v", err)
	}

	// Empty values list returns 0 without error.
	deleted, err = storage.DeleteByFieldIn(ctx, "user", "id", []string{})
	if err != nil {
		t.Fatalf("DeleteByFieldIn with empty list: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Expected 0 deleted for empty list, got %d", deleted)
	}

	// Non-matching values return 0.
	deleted, err = storage.DeleteByFieldIn(ctx, "user", "id", []string{"nonexistent-1", "nonexistent-2"})
	if err != nil {
		t.Fatalf("DeleteByFieldIn with non-matching: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Expected 0 deleted for non-matching, got %d", deleted)
	}
}

func testQueryDistinctField(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Empty table returns no values.
	values, err := storage.QueryDistinctField(ctx, "user", "simulation_id")
	if err != nil {
		t.Fatalf("QueryDistinctField on empty table: %v", err)
	}
	if len(values) != 0 {
		t.Errorf("Expected 0 distinct values, got %d", len(values))
	}

	// Insert users with simulation IDs.
	for _, u := range []struct {
		name  string
		simID *string
	}{
		{"User A", proto.String("sim-alpha")},
		{"User B", proto.String("sim-alpha")},
		{"User C", proto.String("sim-beta")},
		{"User D", nil}, // No simulation_id.
	} {
		user := &models.User{Name: u.name, Email: fmt.Sprintf("%s@test.com", u.name)}
		if u.simID != nil {
			user.SimulationId = u.simID
		}
		if _, err := storage.Insert(ctx, user); err != nil {
			t.Fatalf("Insert %s: %v", u.name, err)
		}
	}

	values, err = storage.QueryDistinctField(ctx, "user", "simulation_id")
	if err != nil {
		t.Fatalf("QueryDistinctField: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("Expected 2 distinct simulation IDs, got %d: %v", len(values), values)
	}

	// Verify both IDs are present.
	found := make(map[string]bool)
	for _, v := range values {
		found[v] = true
	}
	if !found["sim-alpha"] {
		t.Error("Missing sim-alpha")
	}
	if !found["sim-beta"] {
		t.Error("Missing sim-beta")
	}
}

func testCountByField(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Empty table returns 0.
	count, err := storage.CountByField(ctx, "user", "simulation_id", "sim-count-test")
	if err != nil {
		t.Fatalf("CountByField on empty table: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0, got %d", count)
	}

	// Insert users with same simulation ID.
	simID := "sim-count-test"
	for i := range 3 {
		user := &models.User{
			Name:         fmt.Sprintf("Count User %d", i),
			Email:        fmt.Sprintf("count%d@test.com", i),
			SimulationId: proto.String(simID),
		}
		if _, err := storage.Insert(ctx, user); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	// Insert one with a different simulation ID.
	if _, err := storage.Insert(ctx, &models.User{
		Name:         "Other",
		Email:        "other@test.com",
		SimulationId: proto.String("sim-other"),
	}); err != nil {
		t.Fatalf("Insert other: %v", err)
	}

	count, err = storage.CountByField(ctx, "user", "simulation_id", simID)
	if err != nil {
		t.Fatalf("CountByField: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3, got %d", count)
	}

	// Different ID returns its own count.
	count, err = storage.CountByField(ctx, "user", "simulation_id", "sim-other")
	if err != nil {
		t.Fatalf("CountByField other: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1, got %d", count)
	}
}

// testIdentifierInjectionRejected verifies that SQL identifier injection attempts
// are rejected before reaching the database, not silently passed through.
func testIdentifierInjectionRejected(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()
	badField := "1=1; --"
	badTable := "user; DROP TABLE user; --"

	template := &models.Gear{}

	// QueryByField rejects bad field name.
	_, err := storage.QueryByField(ctx, badField, "x", template)
	if err == nil {
		t.Error("QueryByField: expected error for injection field name, got nil")
	}

	// QueryByFields rejects bad field name in map.
	_, err = storage.QueryByFields(ctx, map[string]any{badField: "x"}, template)
	if err == nil {
		t.Error("QueryByFields: expected error for injection field name, got nil")
	}

	// DeleteByField rejects bad field name.
	_, err = storage.DeleteByField(ctx, "gear", badField, "x")
	if err == nil {
		t.Error("DeleteByField: expected error for injection field name, got nil")
	}

	// DeleteByField rejects bad table name.
	_, err = storage.DeleteByField(ctx, badTable, "name", "x")
	if err == nil {
		t.Error("DeleteByField: expected error for injection table name, got nil")
	}

	// DeleteByFieldIn rejects bad field name.
	_, err = storage.DeleteByFieldIn(ctx, "gear", badField, []string{"x"})
	if err == nil {
		t.Error("DeleteByFieldIn: expected error for injection field name, got nil")
	}

	// QueryDistinctField rejects bad field name.
	_, err = storage.QueryDistinctField(ctx, "gear", badField)
	if err == nil {
		t.Error("QueryDistinctField: expected error for injection field name, got nil")
	}

	// QueryDistinctField rejects bad table name.
	_, err = storage.QueryDistinctField(ctx, badTable, "name")
	if err == nil {
		t.Error("QueryDistinctField: expected error for injection table name, got nil")
	}

	// CountByField rejects bad field name.
	_, err = storage.CountByField(ctx, "gear", badField, "x")
	if err == nil {
		t.Error("CountByField: expected error for injection field name, got nil")
	}

	// DecrementFieldIfPositive rejects bad field name.
	_, err = storage.DecrementFieldIfPositive(ctx, "gear", "some-id", badField)
	if err == nil {
		t.Error("DecrementFieldIfPositive: expected error for injection field name, got nil")
	}

	// IncrementField rejects bad field name.
	err = storage.IncrementField(ctx, "gear", "some-id", badField)
	if err == nil {
		t.Error("IncrementField: expected error for injection field name, got nil")
	}
}

// TestForUpdateOutsideTransaction verifies that GetByID with ForUpdate:true
// returns ErrForUpdateOutsideTransaction when called outside a WithTx callback.
func TestForUpdateOutsideTransaction(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	req := &models.Request{
		Title:       "ForUpdate outside tx test",
		Description: "should not matter",
		RequesterId: "user-1",
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	id, err := store.Insert(ctx, req)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	out := &models.Request{}
	err = store.GetByID(ctx, id, out, QueryOptions{ForUpdate: true})
	if err == nil {
		t.Fatal("expected ErrForUpdateOutsideTransaction, got nil")
	}
	if !errors.Is(err, ErrForUpdateOutsideTransaction) {
		t.Errorf("expected ErrForUpdateOutsideTransaction, got: %v", err)
	}
}

// TestForUpdateSerializesConcurrentWriters verifies that two concurrent WithTx
// callers on the same row serialize: the second one waits for the first to
// commit and reads the fresh committed value.
func TestForUpdateSerializesConcurrentWriters(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	user := &models.User{Name: "ForUpdate User", Email: "forUpdate@example.com"}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Insert user: %v", err)
	}
	req := &models.Request{
		Title:       "Original title",
		Description: "ForUpdate serialization test",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	id, err := store.Insert(ctx, req)
	if err != nil {
		t.Fatalf("Insert request: %v", err)
	}

	// firstEntered signals that the first tx has acquired the row lock.
	firstEntered := make(chan struct{})
	// firstRelease signals the first tx to commit.
	firstRelease := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	// First writer: acquires lock, signals, waits for release, updates title.
	go func() {
		defer wg.Done()
		txErr := store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
			fresh := &models.Request{}
			if err := tx.GetByID(ctx, id, fresh, QueryOptions{ForUpdate: true}); err != nil {
				return err
			}
			close(firstEntered) // signal: lock acquired
			<-firstRelease      // wait for test to release
			fresh.Title = "Written by first"
			return tx.Update(ctx, fresh)
		})
		if txErr != nil {
			t.Errorf("first WithTx: %v", txErr)
		}
	}()

	// Second writer: waits until first has the lock, then races to update.
	// It will block on SELECT FOR UPDATE until the first tx commits.
	go func() {
		defer wg.Done()
		<-firstEntered // wait until first holds the lock
		txErr := store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
			fresh := &models.Request{}
			// This SELECT FOR UPDATE blocks until the first tx commits.
			if err := tx.GetByID(ctx, id, fresh, QueryOptions{ForUpdate: true}); err != nil {
				return err
			}
			// After the first tx committed, we must see "Written by first".
			if fresh.Title != "Written by first" {
				t.Errorf("second tx read stale title %q, want %q", fresh.Title, "Written by first")
			}
			fresh.Title = "Written by second"
			return tx.Update(ctx, fresh)
		})
		if txErr != nil {
			t.Errorf("second WithTx: %v", txErr)
		}
	}()

	// Release the first writer.
	<-firstEntered
	close(firstRelease)
	wg.Wait()

	// Final state: second writer committed last.
	final := &models.Request{}
	if err := store.GetByID(ctx, id, final); err != nil {
		t.Fatalf("final GetByID: %v", err)
	}
	if final.Title != "Written by second" {
		t.Errorf("final title = %q, want %q", final.Title, "Written by second")
	}
}

// TestForUpdateWithIncludeDeleted verifies that ForUpdate:true and IncludeDeleted:true
// compose correctly — a soft-deleted row is returned and the lock is acquired.
func TestForUpdateWithIncludeDeleted(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	user := &models.User{Name: "ForUpdate+Deleted User", Email: "fud@example.com"}
	userID, err := store.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Insert user: %v", err)
	}
	req := &models.Request{
		Title:       "Deleted request",
		Description: "should be findable with IncludeDeleted",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  userID,
			DeletedAtUnixSec: 1234567890,
		},
	}
	id, err := store.Insert(ctx, req)
	if err != nil {
		t.Fatalf("Insert deleted request: %v", err)
	}

	var foundDeleted bool
	if txErr := store.WithTx(ctx, nil, func(tx *ProtoSQLStorage) error {
		fresh := &models.Request{}
		if err := tx.GetByID(ctx, id, fresh, QueryOptions{IncludeDeleted: true, ForUpdate: true}); err != nil {
			return err
		}
		foundDeleted = fresh.Deleted != nil
		return nil
	}); txErr != nil {
		t.Fatalf("WithTx: %v", txErr)
	}

	if !foundDeleted {
		t.Error("expected soft-deleted row to be returned with IncludeDeleted:true + ForUpdate:true")
	}
}
