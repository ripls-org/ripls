package gear

import (
	"errors"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_GetGearPeople(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("get people for gear with no transfers", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create a user
		user := &models.User{
			Id:    "user123",
			Email: "test@example.com",
			Name:  "Test User",
		}
		if _, err := testStorage.Insert(ctx, user); err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Create a gear item
		gear := &models.Gear{
			Id:          "gear123",
			OwnerId:     "user123",
			Name:        "Test Drill",
			Description: "A test drill",
		}
		if _, err := testStorage.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		req := connect.NewRequest(&api.GetGearPeopleRequest{
			GearId: "gear123",
		})

		resp, err := service.GetGearPeople(ctx, req)
		if err != nil {
			t.Fatalf("GetGearPeople failed: %v", err)
		}

		if resp.Msg.Owner == nil {
			t.Error("Expected owner to be set")
		} else if resp.Msg.Owner.Id != "user123" {
			t.Errorf("Expected owner ID to be user123, got %s", resp.Msg.Owner.Id)
		}

		if resp.Msg.CurrentBorrower != nil {
			t.Error("Expected no current borrower")
		}

		if len(resp.Msg.PastBorrowers) != 0 {
			t.Errorf("Expected 0 past borrowers, got %d", len(resp.Msg.PastBorrowers))
		}

		if resp.Msg.TotalCount != 1 {
			t.Errorf("Expected total count of 1 (owner only), got %d", resp.Msg.TotalCount)
		}
	})

	t.Run("get people for gear with active loan", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create users
		owner := &models.User{
			Id:    "owner123",
			Email: "owner@example.com",
			Name:  "Owner User",
		}
		if _, err := testStorage.Insert(ctx, owner); err != nil {
			t.Fatalf("Failed to create owner: %v", err)
		}

		borrower := &models.User{
			Id:    "borrower123",
			Email: "borrower@example.com",
			Name:  "Borrower User",
		}
		if _, err := testStorage.Insert(ctx, borrower); err != nil {
			t.Fatalf("Failed to create borrower: %v", err)
		}

		// Create a gear item
		gear := &models.Gear{
			Id:          "gear456",
			OwnerId:     "owner123",
			Name:        "Test Saw",
			Description: "A test saw",
		}
		if _, err := testStorage.Insert(ctx, gear); err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		// Create an active transfer
		transfer := &models.Transfer{
			Id:           "transfer123",
			GearId:       "gear456",
			OwnerId:      "owner123",
			RecipientId:  "borrower123",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
			CommunityId:  "community123",
		}
		if _, err := testStorage.Insert(ctx, transfer); err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}

		// Share the gear with community123 and add caller as a member so the
		// access check passes (caller "user123" is not the owner "owner123").
		comm123 := &models.Community{Id: "community123", Name: "Gear People Community", CreatorId: "owner123", OwnerUserId: "owner123"}
		if _, err := testStorage.Insert(ctx, comm123); err != nil {
			t.Fatalf("Failed to create community: %v", err)
		}
		if _, err := testStorage.Insert(ctx, &models.CommunityUser{CommunityId: "community123", UserId: "user123", InviterId: "owner123"}); err != nil {
			t.Fatalf("Failed to add caller membership: %v", err)
		}
		if _, err := testStorage.Insert(ctx, &models.CommunityGear{CommunityId: "community123", GearId: "gear456"}); err != nil {
			t.Fatalf("Failed to add community gear: %v", err)
		}

		req := connect.NewRequest(&api.GetGearPeopleRequest{
			GearId: "gear456",
		})

		resp, err := service.GetGearPeople(ctx, req)
		if err != nil {
			t.Fatalf("GetGearPeople failed: %v", err)
		}

		if resp.Msg.Owner == nil {
			t.Error("Expected owner to be set")
		}

		if resp.Msg.CurrentBorrower == nil {
			t.Error("Expected current borrower to be set")
		} else if resp.Msg.CurrentBorrower.Id != "borrower123" {
			t.Errorf("Expected current borrower ID to be borrower123, got %s", resp.Msg.CurrentBorrower.Id)
		}

		if resp.Msg.TotalCount != 2 {
			t.Errorf("Expected total count of 2 (owner + borrower), got %d", resp.Msg.TotalCount)
		}
	})

	t.Run("deleted owner surfaces as former-member placeholder", func(t *testing.T) {
		ctx := createAuthenticatedContext("caller-deleted-owner", "caller@example.com", models.Role_ROLE_USER)

		// Caller (must exist for any auth-gated path).
		caller := &models.User{Id: "caller-deleted-owner", Email: "caller@example.com", Name: "Caller"}
		if _, err := testStorage.Insert(ctx, caller); err != nil {
			t.Fatalf("Insert caller: %v", err)
		}

		// Owner who will be soft-deleted.
		owner := &models.User{Id: "owner-deleted", Email: "owner-deleted@example.com", Name: "Owner Deleted"}
		if _, err := testStorage.Insert(ctx, owner); err != nil {
			t.Fatalf("Insert owner: %v", err)
		}
		owner.Deleted = &models.DeletedMetadata{DeletedByUserId: owner.Id, DeletedAtUnixSec: 1}
		if err := testStorage.Update(ctx, owner); err != nil {
			t.Fatalf("Soft-delete owner: %v", err)
		}

		gear := &models.Gear{Id: "gear-deleted-owner", OwnerId: "owner-deleted", Name: "Orphan Gear"}
		if _, err := testStorage.Insert(ctx, gear); err != nil {
			t.Fatalf("Insert gear: %v", err)
		}

		// Share the gear with a community and add caller as a member so the
		// access check passes (caller is not the deleted owner).
		commDel := &models.Community{Id: "comm-deleted-owner", Name: "Del Owner Community", CreatorId: "caller-deleted-owner", OwnerUserId: "caller-deleted-owner"}
		if _, err := testStorage.Insert(ctx, commDel); err != nil {
			t.Fatalf("Insert community: %v", err)
		}
		if _, err := testStorage.Insert(ctx, &models.CommunityUser{CommunityId: "comm-deleted-owner", UserId: "caller-deleted-owner", InviterId: "caller-deleted-owner"}); err != nil {
			t.Fatalf("Add caller membership: %v", err)
		}
		if _, err := testStorage.Insert(ctx, &models.CommunityGear{CommunityId: "comm-deleted-owner", GearId: "gear-deleted-owner"}); err != nil {
			t.Fatalf("Add community gear: %v", err)
		}

		resp, err := service.GetGearPeople(ctx, connect.NewRequest(&api.GetGearPeopleRequest{
			GearId: "gear-deleted-owner",
		}))
		if err != nil {
			t.Fatalf("GetGearPeople must tolerate deleted owner; got error: %v", err)
		}
		if resp.Msg.Owner == nil {
			t.Fatal("Owner must be a placeholder, not nil")
		}
		if !resp.Msg.Owner.FormerMember || resp.Msg.Owner.Id != "owner-deleted" {
			t.Errorf("Owner = %+v, want FormerMember=true and Id=owner-deleted", resp.Msg.Owner)
		}
		if resp.Msg.Owner.Name != "" {
			t.Errorf("Owner.Name = %q, want empty (no PII for former member)", resp.Msg.Owner.Name)
		}
	})

	t.Run("deleted past borrower surfaces as former-member placeholder", func(t *testing.T) {
		ctx := createAuthenticatedContext("owner-past", "owner-past@example.com", models.Role_ROLE_USER)

		owner := &models.User{Id: "owner-past", Email: "owner-past@example.com", Name: "Owner Past"}
		if _, err := testStorage.Insert(ctx, owner); err != nil {
			t.Fatalf("Insert owner: %v", err)
		}
		ghost := &models.User{Id: "ghost-borrower", Email: "ghost@example.com", Name: "Ghost Borrower"}
		if _, err := testStorage.Insert(ctx, ghost); err != nil {
			t.Fatalf("Insert ghost: %v", err)
		}
		ghost.Deleted = &models.DeletedMetadata{DeletedByUserId: ghost.Id, DeletedAtUnixSec: 1}
		if err := testStorage.Update(ctx, ghost); err != nil {
			t.Fatalf("Soft-delete ghost: %v", err)
		}

		gear := &models.Gear{Id: "gear-past", OwnerId: "owner-past", Name: "Returned Gear"}
		if _, err := testStorage.Insert(ctx, gear); err != nil {
			t.Fatalf("Insert gear: %v", err)
		}

		// A completed loan from the now-deleted ghost.
		transfer := &models.Transfer{
			Id:           "transfer-past",
			GearId:       "gear-past",
			OwnerId:      "owner-past",
			RecipientId:  "ghost-borrower",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		}
		if _, err := testStorage.Insert(ctx, transfer); err != nil {
			t.Fatalf("Insert transfer: %v", err)
		}

		resp, err := service.GetGearPeople(ctx, connect.NewRequest(&api.GetGearPeopleRequest{
			GearId: "gear-past",
		}))
		if err != nil {
			t.Fatalf("GetGearPeople must tolerate deleted past borrower; got error: %v", err)
		}
		if len(resp.Msg.PastBorrowers) != 1 {
			t.Fatalf("PastBorrowers len = %d, want 1", len(resp.Msg.PastBorrowers))
		}
		pb := resp.Msg.PastBorrowers[0]
		if !pb.FormerMember || pb.Id != "ghost-borrower" {
			t.Errorf("PastBorrowers[0] = %+v, want FormerMember=true and Id=ghost-borrower", pb)
		}
	})

	t.Run("gear not found", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetGearPeopleRequest{
			GearId: "nonexistent",
		})

		_, err := service.GetGearPeople(ctx, req)
		if err == nil {
			t.Error("Expected error for nonexistent gear")
		}

		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			if connectErr.Code() != connect.CodeNotFound {
				t.Errorf("Expected NotFound error, got %v", connectErr.Code())
			}
		}
	})
}
