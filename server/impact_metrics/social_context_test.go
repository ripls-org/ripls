package impact_metrics

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestStorage creates a PostgreSQL database for testing.
func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

func TestNewConnectionContextResolver(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	r := NewConnectionContextResolver(sqlStorage)
	if r == nil {
		t.Fatal("NewConnectionContextResolver returned nil")
	}
}

func TestResolve_NoInteractions(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	r := NewConnectionContextResolver(sqlStorage)

	ctx := context.Background()
	connCtx, err := r.Resolve(ctx, "userA", "userB", "community1")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if connCtx == nil {
		t.Fatal("Resolve returned nil ConnectionContext")
	}
	if connCtx.PriorInteractionCount != 0 {
		t.Errorf("PriorInteractionCount = %d, want 0", connCtx.PriorInteractionCount)
	}
	if connCtx.MutualConnectionCount != 0 {
		t.Errorf("MutualConnectionCount = %d, want 0", connCtx.MutualConnectionCount)
	}
	if connCtx.DistinctContactsThisWeek != 0 {
		t.Errorf("DistinctContactsThisWeek = %d, want 0", connCtx.DistinctContactsThisWeek)
	}
}

func TestResolve_WithCompletedTransfer(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	r := NewConnectionContextResolver(sqlStorage)
	ctx := context.Background()

	// Insert a completed transfer: userA lent to userB.
	transfer := &models.Transfer{
		OwnerId:      "userA",
		RecipientId:  "userB",
		State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		GearId:       "gear1",
		CommunityId:  "community1",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
	}
	if _, err := sqlStorage.Insert(ctx, transfer); err != nil {
		t.Fatalf("failed to insert transfer: %v", err)
	}

	connCtx, err := r.Resolve(ctx, "userA", "userB", "community1")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if connCtx.PriorInteractionCount != 1 {
		t.Errorf("PriorInteractionCount = %d, want 1", connCtx.PriorInteractionCount)
	}
}

func TestResolve_TransferBothDirections(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	r := NewConnectionContextResolver(sqlStorage)
	ctx := context.Background()

	// A→B
	t1 := &models.Transfer{
		OwnerId:      "userA",
		RecipientId:  "userB",
		State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		GearId:       "gear1",
		CommunityId:  "community1",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
	}
	// B→A
	t2 := &models.Transfer{
		OwnerId:      "userB",
		RecipientId:  "userA",
		State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		GearId:       "gear2",
		CommunityId:  "community1",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
	}
	if _, err := sqlStorage.Insert(ctx, t1); err != nil {
		t.Fatalf("failed to insert t1: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, t2); err != nil {
		t.Fatalf("failed to insert t2: %v", err)
	}

	connCtx, err := r.Resolve(ctx, "userA", "userB", "community1")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if connCtx.PriorInteractionCount != 2 {
		t.Errorf("PriorInteractionCount = %d, want 2", connCtx.PriorInteractionCount)
	}
}

func TestResolve_WithFulfilledRequest(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	r := NewConnectionContextResolver(sqlStorage)
	ctx := context.Background()

	// userA requested, userB was a confirmed helper.
	req := &models.Request{
		RequesterId:        "userA",
		State:              models.RequestState_REQUEST_STATE_FULFILLED,
		ConfirmedHelperIds: []string{"userB"},
		Title:              "need a drill",
	}
	if _, err := sqlStorage.Insert(ctx, req); err != nil {
		t.Fatalf("failed to insert request: %v", err)
	}

	connCtx, err := r.Resolve(ctx, "userA", "userB", "community1")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if connCtx.PriorInteractionCount != 1 {
		t.Errorf("PriorInteractionCount = %d, want 1", connCtx.PriorInteractionCount)
	}
}

func TestResolve_MutualConnections(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	r := NewConnectionContextResolver(sqlStorage)
	ctx := context.Background()

	// userC transacted with both userA and userB.
	tAC := &models.Transfer{
		OwnerId:      "userA",
		RecipientId:  "userC",
		State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		GearId:       "gear1",
		CommunityId:  "community1",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
	}
	tBC := &models.Transfer{
		OwnerId:      "userB",
		RecipientId:  "userC",
		State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		GearId:       "gear2",
		CommunityId:  "community1",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
	}
	if _, err := sqlStorage.Insert(ctx, tAC); err != nil {
		t.Fatalf("failed to insert tAC: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, tBC); err != nil {
		t.Fatalf("failed to insert tBC: %v", err)
	}

	connCtx, err := r.Resolve(ctx, "userA", "userB", "community1")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if connCtx.MutualConnectionCount != 1 {
		t.Errorf("MutualConnectionCount = %d, want 1", connCtx.MutualConnectionCount)
	}
}

func TestResolve_DeletedTransferNotCounted(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	r := NewConnectionContextResolver(sqlStorage)
	ctx := context.Background()

	transfer := &models.Transfer{
		OwnerId:      "userA",
		RecipientId:  "userB",
		State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		GearId:       "gear1",
		CommunityId:  "community1",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  "userA",
			DeletedAtUnixSec: 1000,
		},
	}
	if _, err := sqlStorage.Insert(ctx, transfer); err != nil {
		t.Fatalf("failed to insert transfer: %v", err)
	}

	connCtx, err := r.Resolve(ctx, "userA", "userB", "community1")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if connCtx.PriorInteractionCount != 0 {
		t.Errorf("PriorInteractionCount = %d, want 0 (deleted transfer should not count)", connCtx.PriorInteractionCount)
	}
}
