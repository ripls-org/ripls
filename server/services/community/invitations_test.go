package community

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	communitylib "go.ripls.org/ripls/server/community"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// =============================================================================
// INVITATION LINK TESTS
// =============================================================================.

func TestService_AcceptInvitationLink(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	user1ID := setupTestUser(t, testStorage, "user1@example.com", "User One")
	user2ID := setupTestUser(t, testStorage, "user2@example.com", "User Two")

	ctx1 := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)
	ctx2 := createAuthenticatedContext(user2ID, "user2@example.com", models.Role_ROLE_USER)

	// Create community
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createResp, err := service.CreateCommunity(ctx1, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	communityID := createResp.Msg.Id

	// Get invitation link
	inviteLinkReq := connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	})
	inviteLinkResp, err := service.GetOrCreateShareLink(ctx1, inviteLinkReq)
	if err != nil {
		t.Fatalf("Failed to get invite link: %v", err)
	}

	t.Run("logged-in user can accept invitation link", func(t *testing.T) {
		acceptReq := connect.NewRequest(&api.AcceptInvitationLinkRequest{
			ShortCode: inviteLinkResp.Msg.ShortCode,
		})

		acceptResp, err := service.AcceptInvitationLink(ctx2, acceptReq)
		if err != nil {
			t.Fatalf("AcceptInvitationLink failed: %v", err)
		}

		if acceptResp.Msg.CommunityId != communityID {
			t.Errorf("Expected community ID %s, got %s", communityID, acceptResp.Msg.CommunityId)
		}

		if acceptResp.Msg.CommunityName != "Test Community" {
			t.Errorf("Expected community name 'Test Community', got %s", acceptResp.Msg.CommunityName)
		}

		// Verify membership was created
		memberships, err := testStorage.QueryByField(ctx2, "community_id", communityID, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("Failed to query memberships: %v", err)
		}

		found := false
		for _, msg := range memberships {
			membership := msg.(*models.CommunityUser)
			if membership.UserId == user2ID {
				found = true
				if membership.InviterId != user1ID {
					t.Errorf("Expected inviter ID %s, got %s", user1ID, membership.InviterId)
				}
				break
			}
		}

		if !found {
			t.Error("Expected user2 to be a member after accepting")
		}

		// Verify event was logged
		events, err := testStorage.QueryByField(ctx2, "community_id", communityID, &models.CommunityEvent{})
		if err != nil {
			t.Fatalf("Failed to query events: %v", err)
		}

		var foundEvent *models.CommunityEvent
		for _, msg := range events {
			event := msg.(*models.CommunityEvent)
			if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED &&
				event.ActorId == user2ID {
				foundEvent = event
				break
			}
		}

		if foundEvent == nil {
			t.Error("Expected INVITATION_LINK_USED event to be logged")
		}
	})

	t.Run("accepting invitation link when already a member succeeds", func(t *testing.T) {
		// User2 is already a member from previous test
		acceptReq := connect.NewRequest(&api.AcceptInvitationLinkRequest{
			ShortCode: inviteLinkResp.Msg.ShortCode,
		})

		acceptResp, err := service.AcceptInvitationLink(ctx2, acceptReq)
		if err != nil {
			t.Fatalf("AcceptInvitationLink failed: %v", err)
		}

		if acceptResp.Msg.CommunityId != communityID {
			t.Errorf("Expected community ID %s, got %s", communityID, acceptResp.Msg.CommunityId)
		}
	})

	t.Run("cannot accept invalid invitation token", func(t *testing.T) {
		acceptReq := connect.NewRequest(&api.AcceptInvitationLinkRequest{
			ShortCode: "invalid-token",
		})

		_, err := service.AcceptInvitationLink(ctx2, acceptReq)
		if err == nil {
			t.Fatal("Expected error when accepting invalid token")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("cannot accept revoked invitation link", func(t *testing.T) {
		// Revoke the link via the typed-shortcode RPC.
		revokeReq := connect.NewRequest(&api.RevokeShareLinkRequest{
			ShortCode: inviteLinkResp.Msg.ShortCode,
		})
		_, err := service.RevokeShareLink(ctx1, revokeReq)
		if err != nil {
			t.Fatalf("Failed to revoke link: %v", err)
		}

		// Try to accept revoked link
		acceptReq := connect.NewRequest(&api.AcceptInvitationLinkRequest{
			ShortCode: inviteLinkResp.Msg.ShortCode,
		})

		_, err = service.AcceptInvitationLink(ctx2, acceptReq)
		if err == nil {
			t.Fatal("Expected error when accepting revoked token")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})
}

// =============================================================================
// SHAREABLE INVITATION LINK TESTS
// =============================================================================.

func TestService_GetOrCreateShareLink_BackfillsStaleShortCode(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	user1ID := setupTestUser(t, testStorage, "user1@example.com", "User One")
	ctx1 := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

	// Create community
	createReq := connect.NewRequest(&api.CreateCommunityRequest{Name: "Test Community"})
	createResp, err := service.CreateCommunity(ctx1, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id

	// Insert a ShareLink directly with a UUID-shaped short_code,
	// simulating a record created during the proto field-4 reuse window.
	staleCode := "deeb47d0-f556-4259-a55a-ac06232e394f"
	staleInvitation := &models.ShareLink{
		CommunityId:      communityID,
		InviterId:        user1ID,
		IsRevoked:        false,
		CreatedAtUnixSec: 1000000,
		ShortCode:        staleCode,
		Target: &models.ShareLink_CommunityInviteId{
			CommunityInviteId: communityID,
		},
	}
	_, err = testStorage.Insert(ctx1, staleInvitation)
	if err != nil {
		t.Fatalf("Failed to insert stale invitation: %v", err)
	}

	// Call GetOrCreateShareLink — it should detect the stale code and backfill.
	req := connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	})
	resp, err := service.GetOrCreateShareLink(ctx1, req)
	if err != nil {
		t.Fatalf("GetOrCreateShareLink failed: %v", err)
	}

	if len(resp.Msg.ShortCode) != shortCodeLength {
		t.Errorf("Expected backfilled short code of length %d, got %q (len %d)",
			shortCodeLength, resp.Msg.ShortCode, len(resp.Msg.ShortCode))
	}
	if resp.Msg.ShortCode == staleCode {
		t.Error("Expected a new short code, but got the stale UUID back")
	}

	// Verify the record in storage was updated.
	updated, err := testStorage.QueryByField(ctx1, "community_id", communityID, &models.ShareLink{})
	if err != nil {
		t.Fatalf("Failed to query updated share link: %v", err)
	}
	if len(updated) == 0 {
		t.Fatal("Expected at least one share link record")
	}
	stored := updated[0].(*models.ShareLink)
	if len(stored.ShortCode) != shortCodeLength {
		t.Errorf("Stored short code should be 8 chars, got %q", stored.ShortCode)
	}
}

func TestService_AcceptInvitationLink_CommunitySizeLimit(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	// Create creator user
	creatorID := setupTestUser(t, testStorage, "creator@example.com", "Creator")
	ctxCreator := createAuthenticatedContext(creatorID, "creator@example.com", models.Role_ROLE_USER)

	// Create community
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Full Community",
	})
	createResp, err := service.CreateCommunity(ctxCreator, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id

	// Add 31 more members (creator is #1, so we need 31 more to reach 32)
	for i := 0; i < communitylib.MaxCommunityMembers-1; i++ {
		email := "member" + string(rune('a'+i/26)) + string(rune('a'+i%26)) + "@example.com"
		memberID := setupTestUser(t, testStorage, email, "Member "+string(rune('A'+i/26))+string(rune('a'+i%26)))
		addUserToCommunity(t, service, communityID, creatorID, memberID, "creator@example.com", email)
	}

	// Verify community is at capacity
	count, err := communitylib.GetNumCommunityMembers(ctxCreator, testStorage, communityID)
	if err != nil {
		t.Fatalf("Failed to get member count: %v", err)
	}
	if count != communitylib.MaxCommunityMembers {
		t.Fatalf("Expected %d members, got %d", communitylib.MaxCommunityMembers, count)
	}

	// Try to add one more member
	extraUserID := setupTestUser(t, testStorage, "extra@example.com", "Extra User")
	ctxExtra := createAuthenticatedContext(extraUserID, "extra@example.com", models.Role_ROLE_USER)

	// Get invitation link
	inviteLinkReq := connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	})
	inviteLinkResp, err := service.GetOrCreateShareLink(ctxCreator, inviteLinkReq)
	if err != nil {
		t.Fatalf("Failed to get invite link: %v", err)
	}

	t.Run("cannot join community at capacity", func(t *testing.T) {
		acceptReq := connect.NewRequest(&api.AcceptInvitationLinkRequest{
			ShortCode: inviteLinkResp.Msg.ShortCode,
		})

		_, err := service.AcceptInvitationLink(ctxExtra, acceptReq)
		if err == nil {
			t.Fatal("Expected error when joining full community")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeResourceExhausted {
			t.Errorf("Expected ResourceExhausted error, got %v", connectErr.Code())
		}

		if !strings.Contains(connectErr.Message(), "maximum capacity") {
			t.Errorf("Expected error message about capacity, got: %s", connectErr.Message())
		}
	})
}
