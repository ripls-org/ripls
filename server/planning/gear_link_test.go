package planning

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestValidateGearLink_Empty returns nil when gearID is empty.
func TestValidateGearLink_Empty(t *testing.T) {
	st := setupTestStorage(t)
	if err := ValidateGearLink(context.Background(), st, "user-1", ""); err != nil {
		t.Fatalf("expected nil for empty gearID, got %v", err)
	}
}

// TestValidateGearLink_Owned returns nil when gear exists and is owned by caller.
func TestValidateGearLink_Owned(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	gear := &models.Gear{OwnerId: "user-1", Name: "Tarp"}
	id, err := st.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if err := ValidateGearLink(ctx, st, "user-1", id); err != nil {
		t.Fatalf("expected nil for owned gear, got %v", err)
	}
}

// TestValidateGearLink_NotOwned rejects when gear is owned by another user.
func TestValidateGearLink_NotOwned(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	gear := &models.Gear{OwnerId: "user-2", Name: "Tarp"}
	id, err := st.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	err = ValidateGearLink(ctx, st, "user-1", id)
	if err == nil {
		t.Fatal("expected error for non-owned gear")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("expected CodeInvalidArgument, got %v", got)
	}
}

// TestValidateGearLink_Missing rejects when gear does not exist.
func TestValidateGearLink_Missing(t *testing.T) {
	st := setupTestStorage(t)
	err := ValidateGearLink(context.Background(), st, "user-1", "missing-id")
	if err == nil {
		t.Fatal("expected error for missing gear")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("expected CodeInvalidArgument, got %v", got)
	}
}

// TestValidateGearLink_Deleted rejects when gear has been soft-deleted.
func TestValidateGearLink_Deleted(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	gear := &models.Gear{
		OwnerId: "user-1",
		Name:    "Tarp",
		Deleted: &models.DeletedMetadata{DeletedAtUnixSec: 1},
	}
	id, err := st.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	err = ValidateGearLink(ctx, st, "user-1", id)
	if err == nil {
		t.Fatal("expected error for deleted gear")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Fatalf("expected CodeInvalidArgument, got %v", got)
	}
}
