package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/provisional"
)

func TestService_CreateProvisionalUser(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	outsiderID := setupTestUser(t, testStorage, "outsider@example.com", "Outsider")

	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)
	ctxOutsider := createAuthenticatedContext(outsiderID, "outsider@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Provisional Test Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	t.Run("member can create provisional user", func(t *testing.T) {
		resp, err := service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Mike Placeholder",
		}))
		if err != nil {
			t.Fatalf("CreateProvisionalUser failed: %v", err)
		}
		if resp.Msg.ProvisionalUser.Name != "Mike Placeholder" {
			t.Errorf("Expected name 'Mike Placeholder', got %q", resp.Msg.ProvisionalUser.Name)
		}
		if resp.Msg.ProvisionalUser.Id == "" {
			t.Error("Expected non-empty ID")
		}
		if resp.Msg.ProvisionalUser.IsClaimed {
			t.Error("Expected is_claimed to be false for new provisional user")
		}
	})

	t.Run("non-member cannot create provisional user", func(t *testing.T) {
		_, err := service.CreateProvisionalUser(ctxOutsider, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Sneaky Provisional",
		}))
		if err == nil {
			t.Fatal("Expected permission denied error for non-member")
		}
	})

	t.Run("empty name is rejected", func(t *testing.T) {
		_, err := service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "",
		}))
		if err == nil {
			t.Fatal("Expected invalid argument error for empty name")
		}
	})
}

func TestService_ListProvisionalUsers(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice2@example.com", "Alice List")
	ctxAlice := createAuthenticatedContext(aliceID, "alice2@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "List Provisional Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	// Create two provisional users.
	_, err = service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
		CommunityId: communityID,
		Name:        "Provisional One",
	}))
	if err != nil {
		t.Fatalf("CreateProvisionalUser 1 failed: %v", err)
	}
	_, err = service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
		CommunityId: communityID,
		Name:        "Provisional Two",
	}))
	if err != nil {
		t.Fatalf("CreateProvisionalUser 2 failed: %v", err)
	}

	resp, err := service.ListProvisionalUsers(ctxAlice, connect.NewRequest(&api.ListProvisionalUsersRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("ListProvisionalUsers failed: %v", err)
	}
	if len(resp.Msg.ProvisionalUsers) != 2 {
		t.Errorf("Expected 2 provisional users, got %d", len(resp.Msg.ProvisionalUsers))
	}
}

func TestService_SearchProvisionalUsers(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice3@example.com", "Alice Search")
	ctxAlice := createAuthenticatedContext(aliceID, "alice3@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Search Provisional Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	_, err = service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
		CommunityId: communityID,
		Name:        "Mike Reynolds",
	}))
	if err != nil {
		t.Fatalf("CreateProvisionalUser failed: %v", err)
	}
	_, err = service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
		CommunityId: communityID,
		Name:        "Bhavna Patel",
	}))
	if err != nil {
		t.Fatalf("CreateProvisionalUser failed: %v", err)
	}

	t.Run("partial match returns correct provisional user", func(t *testing.T) {
		resp, err := service.SearchProvisionalUsers(ctxAlice, connect.NewRequest(&api.SearchProvisionalUsersRequest{
			CommunityId: communityID,
			Query:       "mike",
		}))
		if err != nil {
			t.Fatalf("SearchProvisionalUsers failed: %v", err)
		}
		if len(resp.Msg.ProvisionalUsers) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(resp.Msg.ProvisionalUsers))
		}
		if resp.Msg.ProvisionalUsers[0].Name != "Mike Reynolds" {
			t.Errorf("Expected Mike Reynolds, got %q", resp.Msg.ProvisionalUsers[0].Name)
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		resp, err := service.SearchProvisionalUsers(ctxAlice, connect.NewRequest(&api.SearchProvisionalUsersRequest{
			CommunityId: communityID,
			Query:       "zzz_no_match",
		}))
		if err != nil {
			t.Fatalf("SearchProvisionalUsers failed: %v", err)
		}
		if len(resp.Msg.ProvisionalUsers) != 0 {
			t.Errorf("Expected 0 results, got %d", len(resp.Msg.ProvisionalUsers))
		}
	})

	t.Run("claimed provisional users are excluded", func(t *testing.T) {
		// Promote-on-verify keeps the claimed row for history, but search
		// feeds the completion quick-add — a claimed placeholder there would
		// double-attribute a person who is a real member now (#2699).
		realUserID := setupTestUser(t, testStorage, "mike.real@example.com", "Mike Reynolds")
		searchBefore, err := service.SearchProvisionalUsers(ctxAlice, connect.NewRequest(&api.SearchProvisionalUsersRequest{
			CommunityId: communityID,
			Query:       "mike",
		}))
		if err != nil {
			t.Fatalf("SearchProvisionalUsers failed: %v", err)
		}
		if len(searchBefore.Msg.ProvisionalUsers) != 1 {
			t.Fatalf("Expected 1 result before claim, got %d", len(searchBefore.Msg.ProvisionalUsers))
		}
		provID := searchBefore.Msg.ProvisionalUsers[0].Id

		if err := provisional.MergeIntoUser(context.Background(), testStorage, provID, realUserID); err != nil {
			t.Fatalf("MergeIntoUser failed: %v", err)
		}

		searchAfter, err := service.SearchProvisionalUsers(ctxAlice, connect.NewRequest(&api.SearchProvisionalUsersRequest{
			CommunityId: communityID,
			Query:       "mike",
		}))
		if err != nil {
			t.Fatalf("SearchProvisionalUsers after claim failed: %v", err)
		}
		if len(searchAfter.Msg.ProvisionalUsers) != 0 {
			t.Errorf("Expected claimed provisional to be excluded from search, got %d results", len(searchAfter.Msg.ProvisionalUsers))
		}

		// The manage-members surface still lists the claimed row (with its
		// claimed flag) — only search excludes it.
		listResp, err := service.ListProvisionalUsers(ctxAlice, connect.NewRequest(&api.ListProvisionalUsersRequest{
			CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("ListProvisionalUsers failed: %v", err)
		}
		foundClaimed := false
		for _, p := range listResp.Msg.ProvisionalUsers {
			if p.Id == provID && p.IsClaimed {
				foundClaimed = true
			}
		}
		if !foundClaimed {
			t.Error("Expected the claimed provisional to remain listed (claimed) via ListProvisionalUsers")
		}
	})
}

func TestService_GetProvisionalUserInviteLink(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice4@example.com", "Alice Invite")
	ctxAlice := createAuthenticatedContext(aliceID, "alice4@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Invite Link Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	provisionalResp, err := service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
		CommunityId: communityID,
		Name:        "Pending Person",
	}))
	if err != nil {
		t.Fatalf("CreateProvisionalUser failed: %v", err)
	}
	provID := provisionalResp.Msg.ProvisionalUser.Id

	t.Run("returns invite link with short code and url", func(t *testing.T) {
		resp, err := service.GetProvisionalUserInviteLink(ctxAlice, connect.NewRequest(&api.GetProvisionalUserInviteLinkRequest{
			CommunityId:       communityID,
			ProvisionalUserId: provID,
		}))
		if err != nil {
			t.Fatalf("GetProvisionalUserInviteLink failed: %v", err)
		}
		if resp.Msg.ShortCode == "" {
			t.Error("Expected non-empty short_code")
		}
		if resp.Msg.InviteUrl == "" {
			t.Error("Expected non-empty invite_url")
		}
	})

	t.Run("calling again returns same link", func(t *testing.T) {
		resp1, err := service.GetProvisionalUserInviteLink(ctxAlice, connect.NewRequest(&api.GetProvisionalUserInviteLinkRequest{
			CommunityId:       communityID,
			ProvisionalUserId: provID,
		}))
		if err != nil {
			t.Fatalf("First GetProvisionalUserInviteLink failed: %v", err)
		}
		resp2, err := service.GetProvisionalUserInviteLink(ctxAlice, connect.NewRequest(&api.GetProvisionalUserInviteLinkRequest{
			CommunityId:       communityID,
			ProvisionalUserId: provID,
		}))
		if err != nil {
			t.Fatalf("Second GetProvisionalUserInviteLink failed: %v", err)
		}
		if resp1.Msg.ShortCode != resp2.Msg.ShortCode {
			t.Errorf("Expected same short_code on second call, got %q vs %q",
				resp1.Msg.ShortCode, resp2.Msg.ShortCode)
		}
	})

	t.Run("wrong community returns not found", func(t *testing.T) {
		_, err := service.GetProvisionalUserInviteLink(ctxAlice, connect.NewRequest(&api.GetProvisionalUserInviteLinkRequest{
			CommunityId:       communityID,
			ProvisionalUserId: "nonexistent-prov-id",
		}))
		if err == nil {
			t.Fatal("Expected not found error for invalid provisional user ID")
		}
	})
}

func TestService_GetProvisionalUserActivities(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice6@example.com", "Alice Activities")
	outsiderID := setupTestUser(t, testStorage, "outsider2@example.com", "Outsider Two")
	ctxAlice := createAuthenticatedContext(aliceID, "alice6@example.com", models.Role_ROLE_USER)
	ctxOutsider := createAuthenticatedContext(outsiderID, "outsider2@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Activities Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	provisionalResp, err := service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
		CommunityId: communityID,
		Name:        "Active Provisional",
	}))
	if err != nil {
		t.Fatalf("CreateProvisionalUser failed: %v", err)
	}
	provID := provisionalResp.Msg.ProvisionalUser.Id

	// Insert two experiences and mark the provisional user as attended.
	insertProvisionalAttendance := func(expName string, completedAt int64) {
		expID, err := testStorage.Insert(context.Background(), &models.Experience{
			OwnerId:            aliceID,
			Name:               expName,
			Description:        "Test experience",
			State:              models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
			CompletedAtUnixSec: &completedAt,
		})
		if err != nil {
			t.Fatalf("Insert experience failed: %v", err)
		}
		_, err = testStorage.Insert(context.Background(), &models.ExperienceRSVP{
			ExperienceId:      expID,
			CommunityId:       communityID,
			ProvisionalUserId: &provID,
			Intention:         models.RSVPIntention_RSVP_INTENTION_YES,
			Attended:          models.AttendedStatus_ATTENDED_STATUS_YES,
		})
		if err != nil {
			t.Fatalf("Insert RSVP failed: %v", err)
		}
	}

	completedAt1 := int64(1000)
	completedAt2 := int64(2000)
	insertProvisionalAttendance("First Experience", completedAt1)
	insertProvisionalAttendance("Second Experience", completedAt2)

	// Insert a non-attended RSVP — should not appear in results.
	nonAttendedExp, err := testStorage.Insert(context.Background(), &models.Experience{
		OwnerId: aliceID,
		Name:    "Not Attended",
		State:   models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	})
	if err != nil {
		t.Fatalf("Insert non-attended experience failed: %v", err)
	}
	_, err = testStorage.Insert(context.Background(), &models.ExperienceRSVP{
		ExperienceId:      nonAttendedExp,
		CommunityId:       communityID,
		ProvisionalUserId: &provID,
		Intention:         models.RSVPIntention_RSVP_INTENTION_YES,
		Attended:          models.AttendedStatus_ATTENDED_STATUS_NO,
	})
	if err != nil {
		t.Fatalf("Insert non-attended RSVP failed: %v", err)
	}

	t.Run("returns attended activities sorted most-recent-first", func(t *testing.T) {
		resp, err := service.GetProvisionalUserActivities(ctxAlice, connect.NewRequest(&api.GetProvisionalUserActivitiesRequest{
			CommunityId:       communityID,
			ProvisionalUserId: provID,
		}))
		if err != nil {
			t.Fatalf("GetProvisionalUserActivities failed: %v", err)
		}
		if len(resp.Msg.Activities) != 2 {
			t.Fatalf("Expected 2 activities, got %d", len(resp.Msg.Activities))
		}
		if resp.Msg.TotalActivityCount != 2 {
			t.Errorf("Expected total_activity_count=2, got %d", resp.Msg.TotalActivityCount)
		}
		// Most recent first.
		if resp.Msg.Activities[0].CompletedAtUnixSec != completedAt2 {
			t.Errorf("Expected first activity completed_at=%d, got %d", completedAt2, resp.Msg.Activities[0].CompletedAtUnixSec)
		}
		if resp.Msg.Activities[1].CompletedAtUnixSec != completedAt1 {
			t.Errorf("Expected second activity completed_at=%d, got %d", completedAt1, resp.Msg.Activities[1].CompletedAtUnixSec)
		}
	})

	t.Run("non-member cannot view activities", func(t *testing.T) {
		_, err := service.GetProvisionalUserActivities(ctxOutsider, connect.NewRequest(&api.GetProvisionalUserActivitiesRequest{
			CommunityId:       communityID,
			ProvisionalUserId: provID,
		}))
		if err == nil {
			t.Fatal("Expected permission denied for non-member")
		}
	})

	t.Run("missing community_id returns invalid argument", func(t *testing.T) {
		_, err := service.GetProvisionalUserActivities(ctxAlice, connect.NewRequest(&api.GetProvisionalUserActivitiesRequest{
			ProvisionalUserId: provID,
		}))
		if err == nil {
			t.Fatal("Expected invalid argument error for missing community_id")
		}
	})

	t.Run("provisional user with no attended rsvps returns empty", func(t *testing.T) {
		emptyResp, err := service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Inactive Provisional",
		}))
		if err != nil {
			t.Fatalf("CreateProvisionalUser failed: %v", err)
		}
		resp, err := service.GetProvisionalUserActivities(ctxAlice, connect.NewRequest(&api.GetProvisionalUserActivitiesRequest{
			CommunityId:       communityID,
			ProvisionalUserId: emptyResp.Msg.ProvisionalUser.Id,
		}))
		if err != nil {
			t.Fatalf("GetProvisionalUserActivities failed: %v", err)
		}
		if len(resp.Msg.Activities) != 0 {
			t.Errorf("Expected 0 activities for new provisional user, got %d", len(resp.Msg.Activities))
		}
		if resp.Msg.TotalActivityCount != 0 {
			t.Errorf("Expected total_activity_count=0, got %d", resp.Msg.TotalActivityCount)
		}
	})

	t.Run("limit caps results", func(t *testing.T) {
		resp, err := service.GetProvisionalUserActivities(ctxAlice, connect.NewRequest(&api.GetProvisionalUserActivitiesRequest{
			CommunityId:       communityID,
			ProvisionalUserId: provID,
			Limit:             1,
		}))
		if err != nil {
			t.Fatalf("GetProvisionalUserActivities failed: %v", err)
		}
		if len(resp.Msg.Activities) != 1 {
			t.Errorf("Expected 1 activity with limit=1, got %d", len(resp.Msg.Activities))
		}
		// Total count reflects all activities, not just the page.
		if resp.Msg.TotalActivityCount != 2 {
			t.Errorf("Expected total_activity_count=2, got %d", resp.Msg.TotalActivityCount)
		}
	})
}

func TestProvisionalUserMerge(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice5@example.com", "Alice Merge")
	realUserID := setupTestUser(t, testStorage, "real@example.com", "Real User")
	ctxAlice := createAuthenticatedContext(aliceID, "alice5@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Merge Test Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	provisionalResp, err := service.CreateProvisionalUser(ctxAlice, connect.NewRequest(&api.CreateProvisionalUserRequest{
		CommunityId: communityID,
		Name:        "Soon To Be Real",
	}))
	if err != nil {
		t.Fatalf("CreateProvisionalUser failed: %v", err)
	}
	provID := provisionalResp.Msg.ProvisionalUser.Id

	// Verify not claimed initially.
	if provisionalResp.Msg.ProvisionalUser.IsClaimed {
		t.Fatal("Provisional user should not be claimed initially")
	}

	// Merge into a real user.
	if err := provisional.MergeIntoUser(context.Background(), testStorage, provID, realUserID); err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	// Verify the provisional user is now claimed.
	listResp, err := service.ListProvisionalUsers(ctxAlice, connect.NewRequest(&api.ListProvisionalUsersRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("ListProvisionalUsers after merge failed: %v", err)
	}

	if len(listResp.Msg.ProvisionalUsers) != 1 {
		t.Fatalf("Expected 1 provisional user, got %d", len(listResp.Msg.ProvisionalUsers))
	}
	if !listResp.Msg.ProvisionalUsers[0].IsClaimed {
		t.Error("Expected provisional user to be claimed after merge")
	}
}

// TestService_CreateProvisionalUser_ContactHandle covers the contact-keyed
// path: a phone/email handle is normalized and stored, and a second create with
// the same handle (in any format) dedups to the existing provisional user. The
// handle is never exposed on the API, so storage is read directly to verify it.
func TestService_CreateProvisionalUser_ContactHandle(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice6@example.com", "Alice Contact")
	ctx := createAuthenticatedContext(aliceID, "alice6@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Contact Handle Community",
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := createResp.Msg.Id

	// load fetches the stored model; the handle is never returned on the API.
	load := func(id string) *models.ProvisionalUser {
		t.Helper()
		m := &models.ProvisionalUser{}
		if err := testStorage.GetByID(context.Background(), id, m); err != nil {
			t.Fatalf("GetByID(%s): %v", id, err)
		}
		return m
	}

	t.Run("phone is normalized to E.164 and stored", func(t *testing.T) {
		resp, err := service.CreateProvisionalUser(ctx, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Phone Person",
			Contact:     &api.CreateProvisionalUserRequest_PhoneNumber{PhoneNumber: "(555) 123-4567"},
		}))
		if err != nil {
			t.Fatalf("CreateProvisionalUser(phone) failed: %v", err)
		}
		if got := load(resp.Msg.ProvisionalUser.Id).GetPhoneNumber(); got != "+15551234567" {
			t.Errorf("stored phone = %q, want +15551234567", got)
		}
	})

	t.Run("same phone in a different format dedups to one user", func(t *testing.T) {
		first, err := service.CreateProvisionalUser(ctx, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Dedup Phone",
			Contact:     &api.CreateProvisionalUserRequest_PhoneNumber{PhoneNumber: "+1 555 987 6543"},
		}))
		if err != nil {
			t.Fatalf("first create failed: %v", err)
		}
		second, err := service.CreateProvisionalUser(ctx, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Dedup Phone Again",
			Contact:     &api.CreateProvisionalUserRequest_PhoneNumber{PhoneNumber: "555-987-6543"},
		}))
		if err != nil {
			t.Fatalf("second create failed: %v", err)
		}
		if first.Msg.ProvisionalUser.Id != second.Msg.ProvisionalUser.Id {
			t.Errorf("expected dedup to same id, got %q and %q",
				first.Msg.ProvisionalUser.Id, second.Msg.ProvisionalUser.Id)
		}
	})

	t.Run("email is normalized and stored, and dedups", func(t *testing.T) {
		first, err := service.CreateProvisionalUser(ctx, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Email Person",
			Contact:     &api.CreateProvisionalUserRequest_Email{Email: "  Jordan@Example.COM "},
		}))
		if err != nil {
			t.Fatalf("CreateProvisionalUser(email) failed: %v", err)
		}
		if got := load(first.Msg.ProvisionalUser.Id).GetEmail(); got != "jordan@example.com" {
			t.Errorf("stored email = %q, want jordan@example.com", got)
		}
		second, err := service.CreateProvisionalUser(ctx, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Email Person Again",
			Contact:     &api.CreateProvisionalUserRequest_Email{Email: "jordan@example.com"},
		}))
		if err != nil {
			t.Fatalf("second email create failed: %v", err)
		}
		if first.Msg.ProvisionalUser.Id != second.Msg.ProvisionalUser.Id {
			t.Error("expected email dedup to same id")
		}
	})

	t.Run("invalid phone is rejected", func(t *testing.T) {
		_, err := service.CreateProvisionalUser(ctx, connect.NewRequest(&api.CreateProvisionalUserRequest{
			CommunityId: communityID,
			Name:        "Bad Phone",
			Contact:     &api.CreateProvisionalUserRequest_PhoneNumber{PhoneNumber: "12345"},
		}))
		if err == nil {
			t.Fatal("expected invalid argument error for malformed phone")
		}
	})
}
