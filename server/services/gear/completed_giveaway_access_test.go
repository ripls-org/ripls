package gear

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// Post-completion access (#2695): completing a giveaway marks the gear
// GIVEN_AWAY and archives every CommunityGear row, but the item must stay
// READABLE to the communities it happened in (the recipient included) —
// GetGear succeeds, community-scoped fields still resolve so the client
// renders the completed-giveaway card instead of regressing to the loan
// look, and the conversation id stays reachable. New bookings must stay
// blocked.
func TestGetGear_CompletedGiveawayStaysReadable(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := New(testStorage, &services.MockBucketStorage{})

	ownerID := "owner-" + uuid.New().String()
	recipientID := "recipient-" + uuid.New().String()
	ctx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	for id, name := range map[string]string{ownerID: "Maya", recipientID: "Dana"} {
		if _, err := testStorage.Insert(ctx, &models.User{
			Id: id, Name: name, Email: name + "@example.com",
		}); err != nil {
			t.Fatalf("insert user %s: %v", name, err)
		}
	}

	community := &models.Community{
		Name:        "Fourth Grade Moms",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}
	communityID, err := testStorage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	for _, uid := range []string{ownerID, recipientID} {
		if _, err := testStorage.Insert(ctx, &models.CommunityUser{
			CommunityId: communityID, UserId: uid, InviterId: ownerID,
		}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
	}

	// The world a completed giveaway leaves behind: GIVEN_AWAY gear with a
	// conversation, and an ARCHIVED share (transfer/state_machine.go
	// completeGiveaway).
	gear := &models.Gear{
		OwnerId:        ownerID,
		Name:           "Spaghetti (2 boxes)",
		State:          models.GearState_GEAR_STATE_GIVEN_AWAY,
		ConversationId: "conv-" + uuid.New().String(),
	}
	gearID, err := testStorage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := testStorage.Insert(ctx, &models.CommunityGear{
		GearId:       gearID,
		CommunityId:  communityID,
		Availability: models.Availability_AVAILABILITY_FOR_GIVEAWAY,
		Archived:     true,
	}); err != nil {
		t.Fatalf("insert archived community gear: %v", err)
	}

	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

	t.Run("recipient can still read the item", func(t *testing.T) {
		resp, err := service.GetGear(recipientCtx, connect.NewRequest(&api.GetGearRequest{Id: gearID}))
		if err != nil {
			t.Fatalf("GetGear as recipient after completion: %v", err)
		}
		if got := resp.Msg.State; got != api.GearItemState_GEAR_ITEM_STATE_GIVEN_AWAY {
			t.Errorf("state = %v, want GIVEN_AWAY", got)
		}
		// The archived community stays in SharedCommunities so the client's
		// community resolver can re-fetch community-scoped fields.
		if len(resp.Msg.SharedCommunities) != 1 {
			t.Fatalf("SharedCommunities = %v, want the archived community", resp.Msg.SharedCommunities)
		}
	})

	t.Run("community-scoped fields resolve from the archived share", func(t *testing.T) {
		resp, err := service.GetGear(recipientCtx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID, CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetGear (scoped) as recipient: %v", err)
		}
		if got := resp.Msg.Availability; got != api.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("availability = %v, want FOR_GIVEAWAY (drives the completed-giveaway card)", got)
		}
		if resp.Msg.ConversationId == "" {
			t.Error("conversation id missing — the perpetual conversation must stay reachable")
		}
	})

	t.Run("owner sees the giveaway context too, not a loan regression", func(t *testing.T) {
		resp, err := service.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
			Id: gearID, CommunityId: communityID,
		}))
		if err != nil {
			t.Fatalf("GetGear (scoped) as owner: %v", err)
		}
		if got := resp.Msg.Availability; got != api.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("owner availability = %v, want FOR_GIVEAWAY", got)
		}
		if len(resp.Msg.SharedCommunities) != 1 {
			t.Errorf("owner SharedCommunities = %v, want the archived community", resp.Msg.SharedCommunities)
		}
	})

	t.Run("non-member stays denied", func(t *testing.T) {
		strangerCtx := createAuthenticatedContext("stranger-"+uuid.New().String(), "s@example.com", models.Role_ROLE_USER)
		_, err := service.GetGear(strangerCtx, connect.NewRequest(&api.GetGearRequest{Id: gearID}))
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("stranger GetGear = %v, want PermissionDenied", err)
		}
	})

	t.Run("new bookings are blocked on given-away gear", func(t *testing.T) {
		// Even with an ACTIVE share row (e.g. a community the completion
		// missed, or pre-fix data), GIVEN_AWAY blocks new bookings.
		if _, err := testStorage.Insert(ctx, &models.CommunityGear{
			GearId:       gearID,
			CommunityId:  communityID,
			Availability: models.Availability_AVAILABILITY_FOR_LOAN,
		}); err != nil {
			t.Fatalf("insert active community gear: %v", err)
		}
		start := time.Now().Add(24 * time.Hour).Unix()
		_, err := service.ClaimGearDays(recipientCtx, connect.NewRequest(&api.ClaimGearDaysRequest{
			GearId:           gearID,
			CommunityId:      communityID,
			StartDateUnixSec: start,
			EndDateUnixSec:   start + 24*3600,
		}))
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("ClaimGearDays on GIVEN_AWAY gear = %v, want FailedPrecondition", err)
		}
	})
}
