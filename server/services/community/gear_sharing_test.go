package community

import (
	"testing"

	"connectrpc.com/connect"

	chatlib "go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// =============================================================================
// COMMUNITY GEAR SHARING TESTS
// =============================================================================.

// TestService_ShareGearToCommunity covers the gear sharing core that
// CommunityService.ShareItem drives: the CommunityGear junction, the
// GEAR_SHARED event, idempotence, availability updates on re-share, and the
// owner check the ItemSharer hook applies before the core runs.
//
// Membership of the target community is enforced by ShareItem itself, not by
// the core — see TestShareItem_NonMemberTargetCommunityRejected.
func TestService_ShareGearToCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	memberID := setupTestUser(t, testStorage, "member@example.com", "Member")

	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	ctxMember := createAuthenticatedContext(memberID, "member@example.com", models.Role_ROLE_USER)

	// Create community with owner
	createCommunityReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createCommunityResp, err := service.CreateCommunity(ctxOwner, createCommunityReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createCommunityResp.Msg.Id

	// Add member to community
	addUserToCommunity(t, service, communityID, ownerID, memberID, "owner@example.com", "member@example.com")

	// Create gear as owner
	gear := &models.Gear{
		Name:    "Test Gear",
		OwnerId: ownerID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	}
	gearID, err := testStorage.Insert(ctxOwner, gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	t.Run("owner can share their gear", func(t *testing.T) {
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
		)

		// Verify gear is shared
		shares, err := testStorage.QueryByField(ctxOwner, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}

		if len(shares) != 1 {
			t.Fatalf("Expected 1 share, got %d", len(shares))
		}

		share := shares[0].(*models.CommunityGear)
		if share.CommunityId != communityID {
			t.Errorf("Expected community ID %s, got %s", communityID, share.CommunityId)
		}

		if share.CreatedAtUnixSec == 0 {
			t.Error("Expected community gear created_at_unix_sec to be set")
		}

		// Verify availability was stored correctly
		if share.Availability != models.Availability_AVAILABILITY_FOR_LOAN {
			t.Errorf("Expected availability FOR_LOAN, got %v", share.Availability)
		}

		// Verify event was logged
		events, err := testStorage.QueryByField(ctxOwner, "community_id", communityID, &models.CommunityEvent{})
		if err != nil {
			t.Fatalf("Failed to query events: %v", err)
		}

		var foundEvent *models.CommunityEvent
		for _, msg := range events {
			event := msg.(*models.CommunityEvent)
			if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED &&
				event.ActorId == ownerID &&
				event.GearId == gearID {
				foundEvent = event
				break
			}
		}

		if foundEvent == nil {
			t.Error("Expected GEAR_SHARED event to be logged")
		}
	})

	// Audience mutation is owner-only, enforced by ShareItem through the
	// VerifyGearOwner hook before the sharing core runs.
	t.Run("non-owner cannot share gear", func(t *testing.T) {
		// Create gear for another user
		otherGear := &models.Gear{
			Name:    "Other Gear",
			OwnerId: memberID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		otherGearID, err := testStorage.Insert(ctxMember, otherGear)
		if err != nil {
			t.Fatalf("Failed to create other gear: %v", err)
		}

		err = service.VerifyGearOwner(ctxOwner, otherGearID, ownerID)
		if err == nil {
			t.Fatal("Expected error when non-owner tries to share gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok || connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", err)
		}
	})

	t.Run("sharing is idempotent with same availability", func(t *testing.T) {
		// Share again
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
		)

		// Verify still only one share
		shares, err := testStorage.QueryByField(ctxOwner, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}

		if len(shares) != 1 {
			t.Errorf("Expected 1 share after idempotent call, got %d", len(shares))
		}

		// Verify availability is still FOR_LOAN
		share := shares[0].(*models.CommunityGear)
		if share.Availability != models.Availability_AVAILABILITY_FOR_LOAN {
			t.Errorf("Expected availability FOR_LOAN after idempotent call, got %v", share.Availability)
		}
	})

	t.Run("re-sharing with different availability updates the availability", func(t *testing.T) {
		// Share with AVAILABILITY_FOR_GIVEAWAY (was previously FOR_LOAN)
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_GIVEAWAY,
		)

		// Verify still only one share
		shares, err := testStorage.QueryByField(ctxOwner, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}

		if len(shares) != 1 {
			t.Errorf("Expected 1 share after re-sharing, got %d", len(shares))
		}

		// Verify availability was updated to FOR_GIVEAWAY
		share := shares[0].(*models.CommunityGear)
		if share.Availability != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("Expected availability FOR_GIVEAWAY after re-sharing, got %v", share.Availability)
		}

		// Change back to FOR_LOAN
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
		)

		// Verify availability was updated back to FOR_LOAN
		shares, err = testStorage.QueryByField(ctxOwner, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}

		share = shares[0].(*models.CommunityGear)
		if share.Availability != models.Availability_AVAILABILITY_FOR_LOAN {
			t.Errorf("Expected availability FOR_LOAN after changing back, got %v", share.Availability)
		}
	})

	// ShareGearToCommunity is the ItemSharer hook ShareItem actually calls, and
	// it takes no availability: it inherits whatever the gear already carries so
	// that inviting someone to a giveaway doesn't silently demote it to a loan.
	t.Run("sharing without an availability inherits the gear's existing mode", func(t *testing.T) {
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_GIVEAWAY,
		)

		second, err := service.CreateCommunity(ctxOwner, connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Inherit Community",
		}))
		if err != nil {
			t.Fatalf("Failed to create second community: %v", err)
		}
		shareGearForTest(t, service, ctxOwner, gearID, second.Msg.Id)

		cg, err := GetCommunityGear(ctxOwner, testStorage, second.Msg.Id, gearID)
		if err != nil {
			t.Fatalf("Failed to load the new CommunityGear: %v", err)
		}
		if cg.Availability != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("Expected the new share to inherit FOR_GIVEAWAY, got %v", cg.Availability)
		}
	})
}

// TestService_ShareGear_SystemMessages tests that LOAN_SHARED and GIVEAWAY_SHARED system messages
// are emitted when gear is shared with the appropriate availability type.
func TestService_ShareGear_SystemMessages(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	// Set up a SystemMessageWriter so system messages are actually inserted.
	writer := chatlib.NewSystemMessageWriter(testStorage, nil)
	service.SetSystemMessageWriter(writer)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Create community.
	createCommunityResp, err := service.CreateCommunity(ctxOwner, connect.NewRequest(&api.CreateCommunityRequest{
		Name: "System Message Test Community",
	}))
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createCommunityResp.Msg.Id

	tests := []struct {
		name         string
		gearName     string
		availability api.Availability
		wantAction   models.ChatSystemAction
		wantContains string
	}{
		{
			name:         "LOAN_SHARED emitted for loan gear",
			gearName:     "Loanable Drill",
			availability: api.Availability_AVAILABILITY_FOR_LOAN,
			wantAction:   models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED,
			wantContains: "for loan",
		},
		{
			name:         "GIVEAWAY_SHARED emitted for giveaway gear",
			gearName:     "Free Books",
			availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
			wantAction:   models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED,
			wantContains: "for giveaway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gear.
			gear := &models.Gear{
				Name:    tt.gearName,
				OwnerId: ownerID,
				State:   models.GearState_GEAR_STATE_AVAILABLE,
			}
			gearID, err := testStorage.Insert(ctxOwner, gear)
			if err != nil {
				t.Fatalf("Failed to create gear: %v", err)
			}

			// Share gear.
			shareGearForTestWithAvailability(
				t, service, ctxOwner, gearID, communityID, apiAvailabilityToModel(tt.availability),
			)

			// Reload gear to get the conversation_id set by sharing.
			if getErr := testStorage.GetByID(ctxOwner, gearID, gear); getErr != nil {
				t.Fatalf("Failed to reload gear: %v", getErr)
			}
			if gear.ConversationId == "" {
				t.Fatal("Expected conversation_id to be set on gear")
			}

			// Query system messages in the conversation.
			messages, err := testStorage.QueryByField(ctxOwner, "conversation_id", gear.ConversationId, &models.ChatMessage{})
			if err != nil {
				t.Fatalf("Failed to query messages: %v", err)
			}

			// Find the expected system message.
			var found bool
			for _, msg := range messages {
				chatMsg := msg.(*models.ChatMessage)
				sysMsg := chatMsg.GetSystemMessage()
				if sysMsg != nil && sysMsg.Action == tt.wantAction {
					found = true
					if sysMsg.Description == "" {
						t.Error("Expected system message description to be set")
					}
					break
				}
			}
			if !found {
				t.Errorf("Expected %v system message in conversation, found none", tt.wantAction)
			}
		})
	}
}

// TestService_ShareGear_AnchorMessageDeduped verifies that sharing a single
// piece of gear into multiple communities only emits a single creation-anchor
// system message in the gear's conversation. Regression test for #1679 — the
// gear's conversation is per-gear (not per-community), so the anchor message
// must not be re-emitted on each ShareGear call.
func TestService_ShareGear_AnchorMessageDeduped(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	writer := chatlib.NewSystemMessageWriter(testStorage, nil)
	service.SetSystemMessageWriter(writer)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	createCommunity := func(t *testing.T, name string) string {
		t.Helper()
		resp, err := service.CreateCommunity(ctxOwner, connect.NewRequest(&api.CreateCommunityRequest{
			Name: name,
		}))
		if err != nil {
			t.Fatalf("Failed to create community %q: %v", name, err)
		}
		return resp.Msg.Id
	}

	cases := []struct {
		name         string
		availability api.Availability
		wantAction   models.ChatSystemAction
	}{
		{
			name:         "FOR_LOAN shared into multiple communities",
			availability: api.Availability_AVAILABILITY_FOR_LOAN,
			wantAction:   models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED,
		},
		{
			name:         "FOR_GIVEAWAY shared into multiple communities",
			availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
			wantAction:   models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			communityA := createCommunity(t, tc.name+" — A")
			communityB := createCommunity(t, tc.name+" — B")
			communityC := createCommunity(t, tc.name+" — C")

			gear := &models.Gear{
				Name:    "Multi-shared item",
				OwnerId: ownerID,
				State:   models.GearState_GEAR_STATE_AVAILABLE,
			}
			gearID, err := testStorage.Insert(ctxOwner, gear)
			if err != nil {
				t.Fatalf("Failed to insert gear: %v", err)
			}

			for _, communityID := range []string{communityA, communityB, communityC} {
				shareGearForTestWithAvailability(
					t, service, ctxOwner, gearID, communityID, apiAvailabilityToModel(tc.availability),
				)
			}

			gearStored := &models.Gear{}
			if err := testStorage.GetByID(ctxOwner, gearID, gearStored); err != nil {
				t.Fatalf("Failed to reload gear: %v", err)
			}
			if gearStored.ConversationId == "" {
				t.Fatal("Expected conversation_id to be set on gear after sharing")
			}

			messages, err := testStorage.QueryByField(ctxOwner, "conversation_id", gearStored.ConversationId, &models.ChatMessage{})
			if err != nil {
				t.Fatalf("Failed to query conversation messages: %v", err)
			}

			anchorCount := 0
			for _, msg := range messages {
				chatMsg := msg.(*models.ChatMessage)
				if sys := chatMsg.GetSystemMessage(); sys != nil && sys.Action == tc.wantAction {
					anchorCount++
				}
			}
			if anchorCount != 1 {
				t.Errorf("Expected exactly 1 %v anchor message in gear conversation, got %d", tc.wantAction, anchorCount)
			}
		})
	}
}

// TestService_ShareGear_GiveawayConversation tests that conversations are created for giveaway-type gear.
func TestService_ShareGear_GiveawayConversation(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Create community
	createCommunityReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createCommunityResp, err := service.CreateCommunity(ctxOwner, createCommunityReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createCommunityResp.Msg.Id

	// Create gear as owner
	gear := &models.Gear{
		Name:    "Free Books",
		OwnerId: ownerID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	}
	gearID, err := testStorage.Insert(ctxOwner, gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	t.Run("conversation created for giveaway gear", func(t *testing.T) {
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_GIVEAWAY,
		)

		// Verify gear is shared
		shares, err := testStorage.QueryByField(ctxOwner, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}

		if len(shares) != 1 {
			t.Fatalf("Expected 1 share, got %d", len(shares))
		}

		share := shares[0].(*models.CommunityGear)

		// Verify availability is GIVEAWAY
		if share.Availability != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("Expected availability FOR_GIVEAWAY, got %v", share.Availability)
		}

		// Verify conversation was created on the gear (canonical location).
		gearStored := &models.Gear{}
		if getErr := testStorage.GetByID(ctxOwner, gearID, gearStored); getErr != nil {
			t.Fatalf("Failed to get gear: %v", getErr)
		}
		if gearStored.ConversationId == "" {
			t.Error("Expected conversation_id to be set on gear for giveaway")
		}

		// Verify conversation exists
		conversation := &models.ChatConversation{}
		err = testStorage.GetByID(ctxOwner, gearStored.ConversationId, conversation)
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}

		// Verify conversation topic is gear_id
		if conversation.GetTopic().GetGearId() != gearID {
			t.Errorf("Expected conversation gear_id %s, got %s", gearID, conversation.GetTopic().GetGearId())
		}

		// Verify owner is a participant
		ownerIsParticipant := false
		for _, participantID := range conversation.ParticipantIds {
			if participantID == ownerID {
				ownerIsParticipant = true
				break
			}
		}
		if !ownerIsParticipant {
			t.Error("Expected owner to be a participant in the conversation")
		}
	})

	t.Run("conversation NOT created for sale-type gear", func(t *testing.T) {
		// Create another gear
		saleGear := &models.Gear{
			Name:    "For Sale Item",
			OwnerId: ownerID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		saleGearID, err := testStorage.Insert(ctxOwner, saleGear)
		if err != nil {
			t.Fatalf("Failed to create sale gear: %v", err)
		}

		// AVAILABILITY_UNSPECIFIED simulates a SALE type (out of scope).
		shareGearForTestWithAvailability(
			t, service, ctxOwner, saleGearID, communityID, models.Availability_AVAILABILITY_UNSPECIFIED,
		)

		// Verify gear is shared
		shares, err := testStorage.QueryByField(ctxOwner, "gear_id", saleGearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}

		if len(shares) != 1 {
			t.Fatalf("Expected 1 share, got %d", len(shares))
		}

		// Verify conversation was NOT created on the gear.
		gearStored2 := &models.Gear{}
		if getErr := testStorage.GetByID(ctxOwner, saleGearID, gearStored2); getErr != nil {
			t.Fatalf("Failed to get gear: %v", getErr)
		}
		if gearStored2.ConversationId != "" {
			t.Errorf("Expected conversation_id to be empty for non-shareable gear, got %s", gearStored2.ConversationId)
		}
	})
}
