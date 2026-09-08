package gear

import (
	"context"
	"fmt"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

func TestService_DeleteGear(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("successful deletion with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-del-1", "del1@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    "user-del-1",
			Email: "del1@example.com",
			Name:  "Delete Test User 1",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		// Create gear
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Test Drill"),
			Description: proto.String("A test drill"),
		})

		createResp, err := service.SaveGear(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		gearID := createResp.Msg.Id

		// Verify gear exists
		getReq := connect.NewRequest(&api.GetGearRequest{Id: gearID})
		_, err = service.GetGear(ctx, getReq)
		if err != nil {
			t.Fatalf("Failed to get gear before deletion: %v", err)
		}

		// Delete gear
		deleteReq := connect.NewRequest(&api.DeleteGearRequest{
			GearId: gearID,
		})

		_, err = service.DeleteGear(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteGear failed: %v", err)
		}

		// Verify GetGear returns a tombstone for a soft-deleted gear (#1701):
		// 200 OK with deleted=true so transfer-bound clients can render a
		// placeholder card instead of triggering an ERROR alert on 404.
		tombstoneResp, err := service.GetGear(ctx, getReq)
		if err != nil {
			t.Fatalf("Expected tombstone response for deleted gear, got error: %v", err)
		}
		if !tombstoneResp.Msg.GetDeleted() {
			t.Errorf("Expected deleted=true on tombstone response, got false")
		}
		if tombstoneResp.Msg.Id != gearID {
			t.Errorf("Expected tombstone id=%q, got %q", gearID, tombstoneResp.Msg.Id)
		}
		if tombstoneResp.Msg.Name == "" {
			t.Errorf("Expected tombstone to preserve last-known name; got empty")
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.DeleteGearRequest{
			GearId: "some-id",
		})

		_, err := service.DeleteGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unauthenticated request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("delete non-existent gear fails", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.DeleteGearRequest{
			GearId: "nonexistent-id",
		})

		_, err := service.DeleteGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error when deleting non-existent gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("delete gear by non-owner fails", func(t *testing.T) {
		ctx123 := createAuthenticatedContext("user-del-2", "del2@example.com", models.Role_ROLE_USER)

		// Create user123
		testUser123 := &models.User{
			Id:    "user-del-2",
			Email: "del2@example.com",
			Name:  "Delete Test User 2",
		}
		_, err := testStorage.Insert(ctx123, testUser123)
		if err != nil {
			t.Fatalf("Failed to insert user-del-2: %v", err)
		}

		// Create gear as user-del-2
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("User Del 2's Gear"),
		})

		createResp, err := service.SaveGear(ctx123, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		// Try to delete as user-del-3
		ctx456 := createAuthenticatedContext("user-del-3", "del3@example.com", models.Role_ROLE_USER)
		deleteReq := connect.NewRequest(&api.DeleteGearRequest{
			GearId: createResp.Msg.Id,
		})

		_, err = service.DeleteGear(ctx456, deleteReq)
		if err == nil {
			t.Fatal("Expected error when non-owner tries to delete gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}

		// Verify gear still exists
		getReq := connect.NewRequest(&api.GetGearRequest{Id: createResp.Msg.Id})
		_, err = service.GetGear(ctx123, getReq)
		if err != nil {
			t.Fatalf("Gear should still exist after failed delete: %v", err)
		}
	})

	t.Run("delete removes gear from list", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-del-4", "del4@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    "user-del-4",
			Email: "del4@example.com",
			Name:  "Delete Test User 4",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		// Create multiple gear items
		var gearIDs []string
		for i := 0; i < 3; i++ {
			createReq := connect.NewRequest(&api.SaveGearRequest{
				Name: proto.String(fmt.Sprintf("Gear %d", i)),
			})
			createResp, err := service.SaveGear(ctx, createReq)
			if err != nil {
				t.Fatalf("Failed to create gear %d: %v", i, err)
			}
			gearIDs = append(gearIDs, createResp.Msg.Id)
		}

		// List user's gear
		listReq := connect.NewRequest(&api.ListUserGearRequest{})
		listResp, err := service.ListUserGear(ctx, listReq)
		if err != nil {
			t.Fatalf("Failed to list user gear: %v", err)
		}

		initialCount := len(listResp.Msg.Items)

		// Delete one gear
		deleteReq := connect.NewRequest(&api.DeleteGearRequest{
			GearId: gearIDs[1],
		})
		_, err = service.DeleteGear(ctx, deleteReq)
		if err != nil {
			t.Fatalf("Failed to delete gear: %v", err)
		}

		// List again and verify count decreased
		listResp, err = service.ListUserGear(ctx, listReq)
		if err != nil {
			t.Fatalf("Failed to list user gear after deletion: %v", err)
		}

		if len(listResp.Msg.Items) != initialCount-1 {
			t.Errorf("Expected %d items after deletion, got %d", initialCount-1, len(listResp.Msg.Items))
		}

		// Verify deleted gear is not in list
		for _, item := range listResp.Msg.Items {
			if item.Id == gearIDs[1] {
				t.Error("Deleted gear should not appear in list")
			}
		}
	})

	t.Run("soft deletion sets deleted metadata", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-del-5", "del5@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    "user-del-5",
			Email: "del5@example.com",
			Name:  "Delete Test User 5",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		// Create gear
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Soft Delete Test Gear"),
			Description: proto.String("Testing soft deletion"),
		})
		createResp, err := service.SaveGear(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}
		gearID := createResp.Msg.Id

		// Delete gear
		deleteReq := connect.NewRequest(&api.DeleteGearRequest{GearId: gearID})
		_, err = service.DeleteGear(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteGear failed: %v", err)
		}

		// Verify gear is not accessible via normal GetByID
		gear := &models.Gear{}
		err = testStorage.GetByID(ctx, gearID, gear)
		if err == nil {
			t.Error("Expected error when getting deleted gear via normal GetByID")
		}

		// Verify gear still exists with IncludeDeleted option and has deletion metadata
		err = testStorage.GetByID(ctx, gearID, gear, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get deleted gear with IncludeDeleted: %v", err)
		}

		if gear.Deleted == nil {
			t.Fatal("Expected gear to have deleted metadata")
		}
		if gear.Deleted.DeletedByUserId != "user-del-5" {
			t.Errorf("Expected deleted_by_user_id 'user-del-5', got '%s'", gear.Deleted.DeletedByUserId)
		}
		if gear.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected deleted_at_unix_sec to be set")
		}
	})

	t.Run("deletion cancels active transfers", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-del-6", "del6@example.com", models.Role_ROLE_USER)

		// Create test users
		testOwner := &models.User{
			Id:    "user-del-6",
			Email: "del6@example.com",
			Name:  "Delete Test Owner 6",
		}
		_, err := testStorage.Insert(ctx, testOwner)
		if err != nil {
			t.Fatalf("Failed to insert test owner: %v", err)
		}

		testBorrower := &models.User{
			Id:    "user-del-borrower",
			Email: "delborrower@example.com",
			Name:  "Delete Test Borrower",
		}
		_, err = testStorage.Insert(ctx, testBorrower)
		if err != nil {
			t.Fatalf("Failed to insert test borrower: %v", err)
		}

		// Create gear
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("Gear With Active Transfer"),
		})
		createResp, err := service.SaveGear(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}
		gearID := createResp.Msg.Id

		// Create an active transfer manually
		transfer := &models.Transfer{
			Id:          "transfer-to-cancel",
			GearId:      gearID,
			OwnerId:     "user-del-6",
			RecipientId: "user-del-borrower",
			State:       models.TransferState_TRANSFER_STATE_ACTIVE,
		}
		_, err = testStorage.Insert(ctx, transfer)
		if err != nil {
			t.Fatalf("Failed to create transfer: %v", err)
		}

		// Delete gear
		deleteReq := connect.NewRequest(&api.DeleteGearRequest{GearId: gearID})
		_, err = service.DeleteGear(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteGear failed: %v", err)
		}

		// Verify transfer was cancelled
		cancelledTransfer := &models.Transfer{}
		err = testStorage.GetByID(ctx, "transfer-to-cancel", cancelledTransfer)
		if err != nil {
			t.Fatalf("Failed to get transfer: %v", err)
		}

		if cancelledTransfer.State != models.TransferState_TRANSFER_STATE_CANCELLED {
			t.Errorf("Expected transfer state CANCELLED, got %v", cancelledTransfer.State)
		}
	})

	t.Run("deletion_cascades_to_conversations", func(t *testing.T) {
		userID := "user-cascade-conv"
		ctx := createAuthenticatedContext(userID, "cascade@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    userID,
			Email: "cascade@example.com",
			Name:  "Cascade Test User",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert user: %v", err)
		}

		// Create gear
		gear := &models.Gear{
			OwnerId:     userID,
			Name:        "Gear with Conversation",
			Description: "Test gear",
		}
		gearID, err := testStorage.Insert(ctx, gear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Create a conversation linked to this gear via gear_id topic
		conversation := &models.ChatConversation{
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		}
		convID, err := testStorage.Insert(ctx, conversation)
		if err != nil {
			t.Fatalf("Failed to insert conversation: %v", err)
		}

		// Verify conversation exists and is not deleted
		storedConv := &models.ChatConversation{}
		err = testStorage.GetByID(ctx, convID, storedConv)
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}
		if storedConv.Deleted != nil {
			t.Fatalf("Expected conversation to not be deleted initially")
		}

		// Delete gear
		deleteReq := connect.NewRequest(&api.DeleteGearRequest{GearId: gearID})
		_, err = service.DeleteGear(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteGear failed: %v", err)
		}

		// Verify conversation was cascade-deleted
		deletedConv := &models.ChatConversation{}
		err = testStorage.GetByID(ctx, convID, deletedConv, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get conversation after deletion: %v", err)
		}

		if deletedConv.Deleted == nil {
			t.Errorf("Expected conversation to be soft-deleted after gear deletion")
		} else {
			if deletedConv.Deleted.DeletedByUserId != userID {
				t.Errorf("Expected DeletedByUserId=%s, got %s", userID, deletedConv.Deleted.DeletedByUserId)
			}
			if deletedConv.Deleted.DeletedAtUnixSec == 0 {
				t.Errorf("Expected DeletedAtUnixSec to be set")
			}
		}
	})

	t.Run("deletion_cascades_to_media", func(t *testing.T) {
		userID := "user-cascade-media"
		ctx := createAuthenticatedContext(userID, "cascademedia@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    userID,
			Email: "cascademedia@example.com",
			Name:  "Cascade Media Test User",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert user: %v", err)
		}

		// Create media first
		media1 := &models.Media{
			UserId:      userID,
			ContentType: "image/jpeg",
			Filename:    proto.String("gear-image-1.jpg"),
		}
		media2 := &models.Media{
			UserId:      userID,
			ContentType: "image/png",
			Filename:    proto.String("gear-image-2.png"),
		}
		mediaID1, err := testStorage.Insert(ctx, media1)
		if err != nil {
			t.Fatalf("Failed to create media1: %v", err)
		}
		mediaID2, err := testStorage.Insert(ctx, media2)
		if err != nil {
			t.Fatalf("Failed to create media2: %v", err)
		}

		// Create gear with media
		gear := &models.Gear{
			OwnerId:     userID,
			Name:        "Gear with Media",
			Description: "Test gear with media attachments",
			MediaIds:    []string{mediaID1, mediaID2},
		}
		gearID, err := testStorage.Insert(ctx, gear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Verify media exists before deletion
		media := &models.Media{}
		err = testStorage.GetByID(ctx, mediaID1, media)
		if err != nil {
			t.Fatalf("Failed to get media1 before deletion: %v", err)
		}

		// Delete gear
		deleteReq := connect.NewRequest(&api.DeleteGearRequest{GearId: gearID})
		_, err = service.DeleteGear(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteGear failed: %v", err)
		}

		// Verify media is no longer retrievable via normal GetByID
		err = testStorage.GetByID(ctx, mediaID1, media)
		if err == nil {
			t.Error("Expected error when getting deleted media1, but got none")
		}
		err = testStorage.GetByID(ctx, mediaID2, media)
		if err == nil {
			t.Error("Expected error when getting deleted media2, but got none")
		}

		// Verify media still exists with IncludeDeleted option and has deletion metadata
		err = testStorage.GetByID(ctx, mediaID1, media, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to retrieve deleted media1 with IncludeDeleted: %v", err)
		}
		if media.Deleted == nil {
			t.Error("Expected media1 to have deleted metadata")
		}
		if media.Deleted.DeletedByUserId != userID {
			t.Errorf("Expected media1 deleted_by_user_id %s, got %s", userID, media.Deleted.DeletedByUserId)
		}

		err = testStorage.GetByID(ctx, mediaID2, media, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to retrieve deleted media2 with IncludeDeleted: %v", err)
		}
		if media.Deleted == nil {
			t.Error("Expected media2 to have deleted metadata")
		}
	})

	t.Run("deletion_cascades_to_community_gear", func(t *testing.T) {
		userID := "user-cascade-cg"
		ctx := createAuthenticatedContext(userID, "cascadecg@example.com", models.Role_ROLE_USER)

		testUser := &models.User{Id: userID, Email: "cascadecg@example.com", Name: "Cascade CG Test User"}
		if _, err := testStorage.Insert(ctx, testUser); err != nil {
			t.Fatalf("insert user: %v", err)
		}

		createResp, err := service.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Cascade CG Gear"),
			Description: proto.String("gear with active + archived community shares"),
		}))
		if err != nil {
			t.Fatalf("SaveGear: %v", err)
		}
		gearID := createResp.Msg.Id

		liveCG := &models.CommunityGear{
			GearId:      gearID,
			CommunityId: "community-a",
		}
		archivedCG := &models.CommunityGear{
			GearId:      gearID,
			CommunityId: "community-b",
			Archived:    true,
		}
		liveCGID, err := testStorage.Insert(ctx, liveCG)
		if err != nil {
			t.Fatalf("insert liveCG: %v", err)
		}
		archivedCGID, err := testStorage.Insert(ctx, archivedCG)
		if err != nil {
			t.Fatalf("insert archivedCG: %v", err)
		}

		if _, err := service.DeleteGear(ctx, connect.NewRequest(&api.DeleteGearRequest{GearId: gearID})); err != nil {
			t.Fatalf("DeleteGear: %v", err)
		}

		live := &models.CommunityGear{}
		if err := testStorage.GetByID(ctx, liveCGID, live, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get liveCG: %v", err)
		}
		if live.Deleted == nil || live.Deleted.DeletedAtUnixSec == 0 {
			t.Error("expected live community_gear row to be soft-deleted")
		}
		if live.Deleted != nil && live.Deleted.DeletedByUserId != userID {
			t.Errorf("expected DeletedByUserId=%q, got %q", userID, live.Deleted.DeletedByUserId)
		}

		archived := &models.CommunityGear{}
		if err := testStorage.GetByID(ctx, archivedCGID, archived, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get archivedCG: %v", err)
		}
		if archived.Deleted == nil {
			t.Error("expected archived community_gear row to also be soft-deleted")
		}
		if !archived.Archived {
			t.Error("expected archived=true to be preserved alongside deleted")
		}

		// buildDeletedFilter should make the live row invisible on a normal query.
		remaining, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("QueryByField: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("expected 0 live community_gear rows after delete, got %d", len(remaining))
		}
	})
}
