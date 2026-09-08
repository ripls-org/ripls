package community

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	communitylib "go.ripls.org/ripls/server/community"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestGetNumCommunityMembers(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	bobID := setupTestUser(t, testStorage, "bob@example.com", "Bob Jones")

	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	// Create a community (Alice is creator and first member)
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createResp, err := service.CreateCommunity(ctxAlice, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id

	t.Run("new community has one member (creator)", func(t *testing.T) {
		count, err := communitylib.GetNumCommunityMembers(context.Background(), testStorage, communityID)
		if err != nil {
			t.Fatalf("GetNumCommunityMembers failed: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 member (creator), got %d", count)
		}
	})

	t.Run("count increases after member joins", func(t *testing.T) {
		addUserToCommunity(t, service, communityID, aliceID, bobID, "alice@example.com", "bob@example.com")

		count, err := communitylib.GetNumCommunityMembers(context.Background(), testStorage, communityID)
		if err != nil {
			t.Fatalf("GetNumCommunityMembers failed: %v", err)
		}
		if count != 2 {
			t.Errorf("Expected 2 members, got %d", count)
		}
	})

	t.Run("non-existent community returns error", func(t *testing.T) {
		_, err := communitylib.GetNumCommunityMembers(context.Background(), testStorage, "non-existent-id")
		if err == nil {
			t.Fatal("Expected error for non-existent community")
		}
		if !strings.Contains(err.Error(), "community not found") {
			t.Errorf("Expected 'community not found' error, got: %v", err)
		}
	})
}

func TestService_LeaveCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	user1ID := setupTestUser(t, testStorage, "user1@example.com", "User One")
	user2ID := setupTestUser(t, testStorage, "user2@example.com", "User Two")

	ctx1 := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)
	ctx2 := createAuthenticatedContext(user2ID, "user2@example.com", models.Role_ROLE_USER)

	// Create community and add user2 as member
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createResp, err := service.CreateCommunity(ctx1, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	communityID := createResp.Msg.Id

	// Add user2 as member using invitation link
	addUserToCommunity(t, service, communityID, user1ID, user2ID, "user1@example.com", "user2@example.com")

	t.Run("member can leave community", func(t *testing.T) {
		leaveReq := connect.NewRequest(&api.LeaveCommunityRequest{
			CommunityId: communityID,
		})

		_, err := service.LeaveCommunity(ctx2, leaveReq)
		if err != nil {
			t.Fatalf("LeaveCommunity failed: %v", err)
		}

		// Verify membership was deleted
		memberships, err := testStorage.QueryByField(ctx2, "community_id", communityID, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("Failed to query memberships: %v", err)
		}

		for _, msg := range memberships {
			membership := msg.(*models.CommunityUser)
			if membership.UserId == user2ID {
				t.Error("User should not be a member after leaving")
			}
		}

		// Verify event was logged
		events, err := testStorage.QueryByField(ctx2, "community_id", communityID, &models.CommunityEvent{})
		if err != nil {
			t.Fatalf("Failed to query events: %v", err)
		}

		var foundEvent *models.CommunityEvent
		for _, msg := range events {
			event := msg.(*models.CommunityEvent)
			if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT &&
				event.ActorId == user2ID {
				foundEvent = event
				break
			}
		}

		if foundEvent == nil {
			t.Error("Expected MEMBER_LEFT event to be logged")
		}
	})

	t.Run("sole-member-leave converts to community soft-delete", func(t *testing.T) {
		// After user2 leaves above, user1 is the only active member.
		// Per design doc §2.4 the leave morphs into a soft-delete
		// with user1 as the actor and as the sole entry in the
		// deleted_snapshot, leaving the §2.5 restore path open.
		leaveReq := connect.NewRequest(&api.LeaveCommunityRequest{
			CommunityId: communityID,
		})
		if _, err := service.LeaveCommunity(ctx1, leaveReq); err != nil {
			t.Fatalf("sole-member-leave failed: %v", err)
		}

		// Re-fetch with IncludeDeleted: the community must be
		// soft-deleted with user1 in the snapshot.
		got := &models.Community{}
		if err := testStorage.GetByID(context.Background(), communityID, got, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("re-fetch community: %v", err)
		}
		if got.Deleted == nil || got.Deleted.DeletedAtUnixSec == 0 {
			t.Error("community should be soft-deleted")
		}
		if got.Deleted != nil && got.Deleted.DeletedByUserId != user1ID {
			t.Errorf("DeletedByUserId = %q, want %q", got.Deleted.DeletedByUserId, user1ID)
		}
		if got.DeletedSnapshot == nil || len(got.DeletedSnapshot.MemberUserIds) == 0 {
			t.Fatal("expected non-empty deleted_snapshot")
		}
		var sawUser1 bool
		for _, id := range got.DeletedSnapshot.MemberUserIds {
			if id == user1ID {
				sawUser1 = true
			}
		}
		if !sawUser1 {
			t.Errorf("snapshot members %v missing user1 %q", got.DeletedSnapshot.MemberUserIds, user1ID)
		}

		// COMMUNITY_DELETED, not MEMBER_LEFT, is the audit event for
		// sole-member-leave (§6.4).
		events, _ := testStorage.QueryByField(context.Background(), "community_id", communityID, &models.CommunityEvent{})
		var sawDeleted bool
		for _, msg := range events {
			ev := msg.(*models.CommunityEvent)
			if ev.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED && ev.ActorId == user1ID {
				sawDeleted = true
			}
		}
		if !sawDeleted {
			t.Error("expected COMMUNITY_DELETED event with user1 as actor")
		}
	})
}

func TestService_ListCommunityUsers(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	bobID := setupTestUser(t, testStorage, "bob@example.com", "Bob Jones")
	charlieID := setupTestUser(t, testStorage, "charlie@example.com", "Charlie Brown")
	nonMemberID := setupTestUser(t, testStorage, "nonmember@example.com", "Non Member")

	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)
	ctxNonMember := createAuthenticatedContext(nonMemberID, "nonmember@example.com", models.Role_ROLE_USER)

	// Alice creates a community
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Test Community",
		Description: "A test community",
	})
	createResp, err := service.CreateCommunity(ctxAlice, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id

	// Add Bob to community using invitation link
	addUserToCommunity(t, service, communityID, aliceID, bobID, "alice@example.com", "bob@example.com")

	t.Run("member can list community users", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityUsersRequest{
			CommunityId: communityID,
		})

		listResp, err := service.ListCommunityUsers(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityUsers failed: %v", err)
		}

		// Should have Alice and Bob (Charlie hasn't accepted yet)
		if len(listResp.Msg.Members) != 2 {
			t.Fatalf("Expected 2 users, got %d", len(listResp.Msg.Members))
		}

		// Check that Alice and Bob are in the list
		userNames := make(map[string]bool)
		for _, member := range listResp.Msg.Members {
			userNames[member.User.Name] = true
			// Verify user fields are populated
			if member.User.Id == "" {
				t.Error("Expected user ID to be set")
			}
			if member.User.Name == "" {
				t.Error("Expected user name to be set")
			}
			if member.JoinedAtUnixSec == 0 {
				t.Error("Expected joined_at_unix_sec to be set")
			}
		}

		if !userNames["Alice Smith"] {
			t.Error("Expected Alice to be in the user list")
		}
		if !userNames["Bob Jones"] {
			t.Error("Expected Bob to be in the user list")
		}
		if userNames["Charlie Brown"] {
			t.Error("Charlie should not be in the list (hasn't accepted invitation)")
		}
	})

	t.Run("non-member cannot list community users", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityUsersRequest{
			CommunityId: communityID,
		})

		_, err := service.ListCommunityUsers(ctxNonMember, listReq)
		if err == nil {
			t.Fatal("Expected error when non-member tries to list users")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok || connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", err)
		}
	})

	t.Run("list includes all members after Charlie joins", func(t *testing.T) {
		// Add Charlie to community
		addUserToCommunity(t, service, communityID, aliceID, charlieID, "alice@example.com", "charlie@example.com")

		listReq := connect.NewRequest(&api.ListCommunityUsersRequest{
			CommunityId: communityID,
		})

		listResp, err := service.ListCommunityUsers(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityUsers failed: %v", err)
		}

		// Should now have all three users
		if len(listResp.Msg.Members) != 3 {
			t.Fatalf("Expected 3 users, got %d", len(listResp.Msg.Members))
		}

		userNames := make(map[string]bool)
		for _, member := range listResp.Msg.Members {
			userNames[member.User.Name] = true
		}

		if !userNames["Alice Smith"] || !userNames["Bob Jones"] || !userNames["Charlie Brown"] {
			t.Error("Expected all three users to be in the list")
		}
	})

	t.Run("empty community has creator as only user", func(t *testing.T) {
		// Create new community
		createReq := connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Empty Community",
		})
		createResp, err := service.CreateCommunity(ctxAlice, createReq)
		if err != nil {
			t.Fatalf("Failed to create empty community: %v", err)
		}

		listReq := connect.NewRequest(&api.ListCommunityUsersRequest{
			CommunityId: createResp.Msg.Id,
		})

		listResp, err := service.ListCommunityUsers(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityUsers failed: %v", err)
		}

		if len(listResp.Msg.Members) != 1 {
			t.Fatalf("Expected 1 user (creator), got %d", len(listResp.Msg.Members))
		}

		if listResp.Msg.Members[0].User.Name != "Alice Smith" {
			t.Errorf("Expected creator Alice, got %s", listResp.Msg.Members[0].User.Name)
		}
	})
}

func TestService_SearchCommunityUsers(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	bobID := setupTestUser(t, testStorage, "bob@example.com", "Bob Jones")
	charlieID := setupTestUser(t, testStorage, "charlie@example.com", "Charlie Brown")
	nonMemberID := setupTestUser(t, testStorage, "nonmember@example.com", "Non Member")

	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)
	ctxNonMember := createAuthenticatedContext(nonMemberID, "nonmember@example.com", models.Role_ROLE_USER)

	// Alice creates a community and adds Bob and Charlie.
	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Search Test Community",
	}))
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id
	addUserToCommunity(t, service, communityID, aliceID, bobID, "alice@example.com", "bob@example.com")
	addUserToCommunity(t, service, communityID, aliceID, charlieID, "alice@example.com", "charlie@example.com")

	t.Run("exact name match returns user", func(t *testing.T) {
		req := connect.NewRequest(&api.SearchCommunityUsersRequest{
			CommunityId: communityID,
			Query:       "Alice Smith",
		})
		resp, err := service.SearchCommunityUsers(ctxAlice, req)
		if err != nil {
			t.Fatalf("SearchCommunityUsers failed: %v", err)
		}
		if len(resp.Msg.Users) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(resp.Msg.Users))
		}
		if resp.Msg.Users[0].Name != "Alice Smith" {
			t.Errorf("Expected Alice Smith, got %s", resp.Msg.Users[0].Name)
		}
	})

	t.Run("partial name match returns matching users", func(t *testing.T) {
		req := connect.NewRequest(&api.SearchCommunityUsersRequest{
			CommunityId: communityID,
			Query:       "lic", // matches "Alice" but not "Charlie"
		})
		resp, err := service.SearchCommunityUsers(ctxAlice, req)
		if err != nil {
			t.Fatalf("SearchCommunityUsers failed: %v", err)
		}
		if len(resp.Msg.Users) != 1 {
			t.Fatalf("Expected 1 result for 'lic', got %d", len(resp.Msg.Users))
		}
		if resp.Msg.Users[0].Name != "Alice Smith" {
			t.Errorf("Expected Alice Smith, got %s", resp.Msg.Users[0].Name)
		}
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		req := connect.NewRequest(&api.SearchCommunityUsersRequest{
			CommunityId: communityID,
			Query:       "bob",
		})
		resp, err := service.SearchCommunityUsers(ctxAlice, req)
		if err != nil {
			t.Fatalf("SearchCommunityUsers failed: %v", err)
		}
		if len(resp.Msg.Users) != 1 {
			t.Fatalf("Expected 1 result for 'bob', got %d", len(resp.Msg.Users))
		}
		if resp.Msg.Users[0].Name != "Bob Jones" {
			t.Errorf("Expected Bob Jones, got %s", resp.Msg.Users[0].Name)
		}
	})

	t.Run("no match returns empty results", func(t *testing.T) {
		req := connect.NewRequest(&api.SearchCommunityUsersRequest{
			CommunityId: communityID,
			Query:       "xyz_no_match",
		})
		resp, err := service.SearchCommunityUsers(ctxAlice, req)
		if err != nil {
			t.Fatalf("SearchCommunityUsers failed: %v", err)
		}
		if len(resp.Msg.Users) != 0 {
			t.Errorf("Expected 0 results, got %d", len(resp.Msg.Users))
		}
	})

	t.Run("non-member outside community not returned", func(t *testing.T) {
		req := connect.NewRequest(&api.SearchCommunityUsersRequest{
			CommunityId: communityID,
			Query:       "Non Member",
		})
		resp, err := service.SearchCommunityUsers(ctxAlice, req)
		if err != nil {
			t.Fatalf("SearchCommunityUsers failed: %v", err)
		}
		if len(resp.Msg.Users) != 0 {
			t.Errorf("Expected 0 results for non-member, got %d", len(resp.Msg.Users))
		}
	})

	t.Run("non-member cannot search community users", func(t *testing.T) {
		req := connect.NewRequest(&api.SearchCommunityUsersRequest{
			CommunityId: communityID,
			Query:       "Alice",
		})
		_, err := service.SearchCommunityUsers(ctxNonMember, req)
		if err == nil {
			t.Fatal("Expected error when non-member searches community users")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok || connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", err)
		}
	})
}
