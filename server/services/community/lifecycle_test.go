package community

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/clock"
	communitylib "go.ripls.org/ripls/server/community"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// =============================================================================
// COMMUNITY CRUD TESTS
// =============================================================================.

func TestService_CreateCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	t.Run("successful creation with authentication", func(t *testing.T) {
		userID := setupTestUser(t, testStorage, "creator@example.com", "Creator User")
		ctx := createAuthenticatedContext(userID, "creator@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.CreateCommunityRequest{
			Name:        "Test Community",
			Description: "A community for testing",
			MediaIds:    []string{"media-123"},
		})

		resp, err := service.CreateCommunity(ctx, req)
		if err != nil {
			t.Fatalf("CreateCommunity failed: %v", err)
		}

		if resp.Msg.Id == "" {
			t.Error("Expected non-empty community ID")
		}

		// Verify community was stored
		community := &models.Community{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, community)
		if err != nil {
			t.Fatalf("Failed to retrieve stored community: %v", err)
		}

		if community.Name != "Test Community" {
			t.Errorf("Expected name 'Test Community', got %s", community.Name)
		}

		if community.Description != "A community for testing" {
			t.Errorf("Expected description 'A community for testing', got %s", community.Description)
		}

		if community.CreatorId != userID {
			t.Errorf("Expected creator ID %s, got %s", userID, community.CreatorId)
		}

		if len(community.MediaIds) != 1 || community.MediaIds[0] != "media-123" {
			t.Errorf("Expected media IDs ['media-123'], got %v", community.MediaIds)
		}

		// Verify timestamps are set
		if community.CreatedAtUnixSec == 0 {
			t.Error("Expected created_at_unix_sec to be set")
		}

		if community.UpdatedAtUnixSec == 0 {
			t.Error("Expected updated_at_unix_sec to be set")
		}

		if community.CreatedAtUnixSec != community.UpdatedAtUnixSec {
			t.Errorf("Expected created_at and updated_at to be equal on creation, got created=%d, updated=%d",
				community.CreatedAtUnixSec, community.UpdatedAtUnixSec)
		}

		// Verify creator was automatically added as a member
		memberships, err := testStorage.QueryByField(ctx, "community_id", resp.Msg.Id, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("Failed to query memberships: %v", err)
		}

		if len(memberships) != 1 {
			t.Fatalf("Expected 1 membership, got %d", len(memberships))
		}

		membership := memberships[0].(*models.CommunityUser)
		if membership.UserId != userID {
			t.Errorf("Expected creator to be member, got user ID %s", membership.UserId)
		}

		if membership.CreatedAtUnixSec == 0 {
			t.Error("Expected membership created_at_unix_sec to be set")
		}

		if membership.CreatedAtUnixSec != community.CreatedAtUnixSec {
			t.Errorf("Expected membership created_at to match community created_at, got membership=%d, community=%d",
				membership.CreatedAtUnixSec, community.CreatedAtUnixSec)
		}
	})

	t.Run("creates community with initial region from creator location", func(t *testing.T) {
		userID := setupTestUser(t, testStorage, "creator-with-location@example.com", "Creator With Location")
		ctx := createAuthenticatedContext(userID, "creator-with-location@example.com", models.Role_ROLE_USER)

		// Create a location for the creator
		now := time.Now().Unix()
		location := &models.Location{
			Geolocation: &models.Geolocation{
				LatitudeDeg:  40.0150,
				LongitudeDeg: -105.2705,
			},
			Address: &models.Address{
				Locality:           "Boulder",
				AdministrativeArea: proto.String("Colorado"),
				County:             proto.String("Boulder County"),
				Neighborhood:       proto.String("Downtown"),
			},
			CreatedAtUnixSec: now,
			UpdatedAtUnixSec: now,
		}
		locationID, err := testStorage.Insert(ctx, location)
		if err != nil {
			t.Fatalf("Failed to create location: %v", err)
		}

		// Update user with primary residence location
		user := &models.User{}
		err = testStorage.GetByID(ctx, userID, user)
		if err != nil {
			t.Fatalf("Failed to get user: %v", err)
		}
		user.PrimaryResidenceLocationId = locationID
		err = testStorage.Update(ctx, user)
		if err != nil {
			t.Fatalf("Failed to update user location: %v", err)
		}

		// Create the community
		req := connect.NewRequest(&api.CreateCommunityRequest{
			Name:        "Test Community with Location",
			Description: "A community that should have regions",
		})

		resp, err := service.CreateCommunity(ctx, req)
		if err != nil {
			t.Fatalf("CreateCommunity failed: %v", err)
		}

		// Verify regions were computed
		regions, err := testStorage.QueryByField(ctx, "community_id", resp.Msg.Id, &models.CommunityRegion{})
		if err != nil {
			t.Fatalf("Failed to query regions: %v", err)
		}

		// With only one member, they should create regions for all levels
		// (neighborhood, city, county, state) since they are 100% of the community
		if len(regions) == 0 {
			t.Error("Expected regions to be computed for community with located creator")
		}

		// Check that we have regions for different levels
		regionTypes := make(map[string]bool)
		for _, msg := range regions {
			communityRegion := msg.(*models.CommunityRegion)

			// Fetch the actual region details
			region := &models.Region{}
			if err := testStorage.GetByID(ctx, communityRegion.RegionId, region); err != nil {
				t.Fatalf("Failed to get region %s: %v", communityRegion.RegionId, err)
			}
			regionTypes[region.RegionType] = true

			// All regions should have 100% since there's only one member
			if communityRegion.MemberPercentage != 1.0 {
				t.Errorf("Expected member_percentage to be 1.0, got %f for region type %s",
					communityRegion.MemberPercentage, region.RegionType)
			}

			// Verify region is not marked as override
			if communityRegion.IsOverride {
				t.Error("Expected auto-computed regions to have is_override=false")
			}
		}

		// Verify we have expected region types
		expectedTypes := []string{"neighborhood", "city", "county", "state"}
		for _, expectedType := range expectedTypes {
			if !regionTypes[expectedType] {
				t.Errorf("Expected region type %s to be present", expectedType)
			}
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Test Community",
		})

		_, err := service.CreateCommunity(ctx, req)
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
}

func TestService_UpdateCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	user1ID := setupTestUser(t, testStorage, "user1@example.com", "User One")
	user2ID := setupTestUser(t, testStorage, "user2@example.com", "User Two")

	ctx1 := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)
	ctx2 := createAuthenticatedContext(user2ID, "user2@example.com", models.Role_ROLE_USER)

	// Create community as user1
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Original Name",
		Description: "Original Description",
	})
	createResp, err := service.CreateCommunity(ctx1, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	communityID := createResp.Msg.Id

	t.Run("member can update community", func(t *testing.T) {
		// Get original community to check timestamps
		originalCommunity := &models.Community{}
		err := testStorage.GetByID(ctx1, communityID, originalCommunity)
		if err != nil {
			t.Fatalf("Failed to retrieve original community: %v", err)
		}

		// Use a simulated future time so UpdatedAtUnixSec is strictly greater
		// than the value set during CreateCommunity.
		laterCtx := clock.WithSimulationTime(ctx1, time.Now().Add(2*time.Second))
		updateReq := connect.NewRequest(&api.UpdateCommunityRequest{
			Id:          communityID,
			Name:        "Updated Name",
			Description: "Updated Description",
			MediaIds:    []string{"new-media-123"},
		})

		_, err = service.UpdateCommunity(laterCtx, updateReq)
		if err != nil {
			t.Fatalf("UpdateCommunity failed: %v", err)
		}

		// Verify updates
		community := &models.Community{}
		err = testStorage.GetByID(ctx1, communityID, community)
		if err != nil {
			t.Fatalf("Failed to retrieve community: %v", err)
		}

		if community.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got %s", community.Name)
		}

		if community.Description != "Updated Description" {
			t.Errorf("Expected description 'Updated Description', got %s", community.Description)
		}

		if len(community.MediaIds) != 1 || community.MediaIds[0] != "new-media-123" {
			t.Errorf("Expected media IDs ['new-media-123'], got %v", community.MediaIds)
		}

		// Verify updated_at timestamp changed
		if community.UpdatedAtUnixSec <= originalCommunity.UpdatedAtUnixSec {
			t.Errorf("Expected updated_at to be newer, got original=%d, updated=%d",
				originalCommunity.UpdatedAtUnixSec, community.UpdatedAtUnixSec)
		}

		// Verify created_at timestamp did not change
		if community.CreatedAtUnixSec != originalCommunity.CreatedAtUnixSec {
			t.Errorf("Expected created_at to remain the same, got original=%d, new=%d",
				originalCommunity.CreatedAtUnixSec, community.CreatedAtUnixSec)
		}
	})

	t.Run("non-member cannot update community", func(t *testing.T) {
		updateReq := connect.NewRequest(&api.UpdateCommunityRequest{
			Id:   communityID,
			Name: "Should Fail",
		})

		_, err := service.UpdateCommunity(ctx2, updateReq)
		if err == nil {
			t.Fatal("Expected error when non-member tries to update community")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("invited member can update after joining", func(t *testing.T) {
		// Add user2 to community using invitation link
		addUserToCommunity(t, service, communityID, user1ID, user2ID, "user1@example.com", "user2@example.com")

		// Now user2 can update
		updateReq := connect.NewRequest(&api.UpdateCommunityRequest{
			Id:   communityID,
			Name: "Updated by User2",
		})
		_, err = service.UpdateCommunity(ctx2, updateReq)
		if err != nil {
			t.Fatalf("Member should be able to update: %v", err)
		}
	})
}

func TestService_DeleteCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	user1ID := setupTestUser(t, testStorage, "user1@example.com", "User One")
	user2ID := setupTestUser(t, testStorage, "user2@example.com", "User Two")

	ctx1 := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)
	ctx2 := createAuthenticatedContext(user2ID, "user2@example.com", models.Role_ROLE_USER)

	// Create community as user1
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createResp, err := service.CreateCommunity(ctx1, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	communityID := createResp.Msg.Id

	t.Run("creator can delete community", func(t *testing.T) {
		deleteReq := connect.NewRequest(&api.DeleteCommunityRequest{
			Id: communityID,
		})

		_, err := service.DeleteCommunity(ctx1, deleteReq)
		if err != nil {
			t.Fatalf("DeleteCommunity failed: %v", err)
		}

		// Verify community is deleted
		community := &models.Community{}
		err = testStorage.GetByID(ctx1, communityID, community)
		if err == nil {
			t.Error("Expected error when retrieving deleted community")
		}
	})

	t.Run("non-creator cannot delete community", func(t *testing.T) {
		// Create new community
		createReq := connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Another Community",
		})
		createResp, err := service.CreateCommunity(ctx1, createReq)
		if err != nil {
			t.Fatalf("Failed to create community: %v", err)
		}

		// Try to delete as user2
		deleteReq := connect.NewRequest(&api.DeleteCommunityRequest{
			Id: createResp.Msg.Id,
		})

		_, err = service.DeleteCommunity(ctx2, deleteReq)
		if err == nil {
			t.Fatal("Expected error when non-creator tries to delete community")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("soft deletion sets deleted metadata", func(t *testing.T) {
		// Create new community
		createReq := connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Soft Delete Test Community",
		})
		createResp, err := service.CreateCommunity(ctx1, createReq)
		if err != nil {
			t.Fatalf("Failed to create community: %v", err)
		}
		communityID := createResp.Msg.Id

		// Delete the community
		deleteReq := connect.NewRequest(&api.DeleteCommunityRequest{Id: communityID})
		_, err = service.DeleteCommunity(ctx1, deleteReq)
		if err != nil {
			t.Fatalf("DeleteCommunity failed: %v", err)
		}

		// Verify community is not accessible via normal GetByID
		community := &models.Community{}
		err = testStorage.GetByID(ctx1, communityID, community)
		if err == nil {
			t.Error("Expected error when getting deleted community via normal GetByID")
		}

		// Verify community still exists with IncludeDeleted option and has deletion metadata
		err = testStorage.GetByID(ctx1, communityID, community, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get deleted community with IncludeDeleted: %v", err)
		}

		if community.Deleted == nil {
			t.Fatal("Expected community to have deleted metadata")
		}
		if community.Deleted.DeletedByUserId != user1ID {
			t.Errorf("Expected deleted_by_user_id '%s', got '%s'", user1ID, community.Deleted.DeletedByUserId)
		}
		if community.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected deleted_at_unix_sec to be set")
		}
	})
}

func TestService_GetCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	userID := setupTestUser(t, testStorage, "user@example.com", "Test User")
	ctx := createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)

	// Create a test community
	community := &models.Community{
		Name:        "Test Community",
		Description: "Test description",
		CreatorId:   userID,
		OwnerUserId: userID,
		MediaIds:    []string{"media-456"},
	}
	communityID, err := testStorage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}

	// Add creator as member (mimicking CreateCommunity behavior)
	creatorMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
		InviterId:   userID,
	}
	_, err = testStorage.Insert(ctx, creatorMembership)
	if err != nil {
		t.Fatalf("Failed to add creator as member: %v", err)
	}

	t.Run("successful retrieval", func(t *testing.T) {
		req := connect.NewRequest(&api.GetCommunityRequest{
			Id: communityID,
		})

		resp, err := service.GetCommunity(ctx, req)
		if err != nil {
			t.Fatalf("GetCommunity failed: %v", err)
		}

		if resp.Msg.Id != communityID {
			t.Errorf("Expected ID %s, got %s", communityID, resp.Msg.Id)
		}

		if resp.Msg.Name != "Test Community" {
			t.Errorf("Expected name 'Test Community', got %s", resp.Msg.Name)
		}

		if resp.Msg.Description != "Test description" {
			t.Errorf("Expected description 'Test description', got %s", resp.Msg.Description)
		}

		if resp.Msg.OwnerUserId != userID {
			t.Errorf("Expected OwnerUserId %s, got %q", userID, resp.Msg.OwnerUserId)
		}

		if len(resp.Msg.MediaIds) != 1 || resp.Msg.MediaIds[0] != "media-456" {
			t.Errorf("Expected media IDs ['media-456'], got %v", resp.Msg.MediaIds)
		}

		// Verify member count (creator is the only member)
		if resp.Msg.NumMembers != 1 {
			t.Errorf("Expected num_members 1, got %d", resp.Msg.NumMembers)
		}

		// Verify max members equals constant
		if resp.Msg.MaxMembers != int32(communitylib.MaxCommunityMembers) {
			t.Errorf("Expected max_members %d, got %d", communitylib.MaxCommunityMembers, resp.Msg.MaxMembers)
		}
	})

	t.Run("community not found", func(t *testing.T) {
		req := connect.NewRequest(&api.GetCommunityRequest{
			Id: "nonexistent-id",
		})

		_, err := service.GetCommunity(ctx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent community")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("member count with multiple members", func(t *testing.T) {
		// Add two more members to the community
		member1ID := setupTestUser(t, testStorage, "member1@example.com", "Member One")
		member2ID := setupTestUser(t, testStorage, "member2@example.com", "Member Two")

		// Add members to community
		membership1 := &models.CommunityUser{
			CommunityId: communityID,
			UserId:      member1ID,
			InviterId:   userID,
		}
		membership2 := &models.CommunityUser{
			CommunityId: communityID,
			UserId:      member2ID,
			InviterId:   userID,
		}

		_, err := testStorage.Insert(ctx, membership1)
		if err != nil {
			t.Fatalf("Failed to add member1: %v", err)
		}
		_, err = testStorage.Insert(ctx, membership2)
		if err != nil {
			t.Fatalf("Failed to add member2: %v", err)
		}

		req := connect.NewRequest(&api.GetCommunityRequest{
			Id: communityID,
		})

		resp, err := service.GetCommunity(ctx, req)
		if err != nil {
			t.Fatalf("GetCommunity failed: %v", err)
		}

		// Should now have 3 members (creator + 2 added members)
		if resp.Msg.NumMembers != 3 {
			t.Errorf("Expected num_members 3, got %d", resp.Msg.NumMembers)
		}

		// Max members should still be the constant
		if resp.Msg.MaxMembers != int32(communitylib.MaxCommunityMembers) {
			t.Errorf("Expected max_members %d, got %d", communitylib.MaxCommunityMembers, resp.Msg.MaxMembers)
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.GetCommunityRequest{
			Id: communityID,
		})

		_, err := service.GetCommunity(ctx, req)
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
}

// TestRunLeaverCascade_StripsChatParticipants verifies that LeaveCommunity
// removes the leaver from the ParticipantIds of gear conversations in that
// community, while leaving other participants untouched.
func TestRunLeaverCascade_StripsChatParticipants(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	creatorID := setupTestUser(t, testStorage, "cascade-chat-creator@example.com", "Creator")
	leaverID := setupTestUser(t, testStorage, "cascade-chat-leaver@example.com", "Leaver")
	otherID := setupTestUser(t, testStorage, "cascade-chat-other@example.com", "Other")

	ctxCreator := createAuthenticatedContext(creatorID, "cascade-chat-creator@example.com", models.Role_ROLE_USER)
	ctxLeaver := createAuthenticatedContext(leaverID, "cascade-chat-leaver@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxCreator, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Cascade Chat Test Community",
	}))
	if err != nil {
		t.Fatalf("create community: %v", err)
	}
	communityID := createResp.Msg.Id
	addUserToCommunity(t, service, communityID, creatorID, leaverID, "cascade-chat-creator@example.com", "cascade-chat-leaver@example.com")

	// Gear shared into the community.
	gearID, err := testStorage.Insert(context.Background(), &models.Gear{Name: "shared gear", OwnerId: creatorID})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := testStorage.Insert(context.Background(), &models.CommunityGear{CommunityId: communityID, GearId: gearID}); err != nil {
		t.Fatalf("insert community_gear: %v", err)
	}

	// Chat conversation about that gear with both leaver and other as participants.
	convID, err := testStorage.Insert(context.Background(), &models.ChatConversation{
		Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
		ParticipantIds: []string{leaverID, otherID},
	})
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	// Leaver leaves the community.
	if _, err := service.LeaveCommunity(ctxLeaver, connect.NewRequest(&api.LeaveCommunityRequest{
		CommunityId: communityID,
	})); err != nil {
		t.Fatalf("LeaveCommunity: %v", err)
	}

	// runLeaverCascade runs synchronously inside LeaveCommunity, so the
	// ParticipantIds update is committed by the time LeaveCommunity returns.
	conv := &models.ChatConversation{}
	if err := testStorage.GetByID(context.Background(), convID, conv); err != nil {
		t.Fatalf("get conversation: %v", err)
	}

	for _, pid := range conv.ParticipantIds {
		if pid == leaverID {
			t.Errorf("leaver %q still in ParticipantIds after leave: %v", leaverID, conv.ParticipantIds)
		}
	}

	found := false
	for _, pid := range conv.ParticipantIds {
		if pid == otherID {
			found = true
		}
	}
	if !found {
		t.Errorf("non-leaver %q was incorrectly stripped from ParticipantIds: %v", otherID, conv.ParticipantIds)
	}
}
