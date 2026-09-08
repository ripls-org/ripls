package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	chatlib "go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_SetGearAvailability(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)
	// System messages are only inserted when a writer is wired (setupTestService
	// leaves it nil), so the announcement assertions below need this.
	service.SetSystemMessageWriter(chatlib.NewSystemMessageWriter(testStorage, nil))

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	nonOwnerID := setupTestUser(t, testStorage, "nonowner@example.com", "Non-Owner")
	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	ctxNonOwner := createAuthenticatedContext(nonOwnerID, "nonowner@example.com", models.Role_ROLE_USER)

	createCommunity := func(name string) string {
		resp, err := service.CreateCommunity(ctxOwner, connect.NewRequest(&api.CreateCommunityRequest{Name: name}))
		if err != nil {
			t.Fatalf("CreateCommunity(%q): %v", name, err)
		}
		return resp.Msg.Id
	}

	shareGear := func(communityID, name string, availability api.Availability) string {
		gear := &models.Gear{Name: name, OwnerId: ownerID, State: models.GearState_GEAR_STATE_AVAILABLE}
		gearID, err := testStorage.Insert(context.Background(), gear)
		if err != nil {
			t.Fatalf("insert gear %q: %v", name, err)
		}
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, apiAvailabilityToModel(availability),
		)
		return gearID
	}

	availabilityOf := func(communityID, gearID string) models.Availability {
		cg, err := GetCommunityGear(context.Background(), testStorage, communityID, gearID)
		if err != nil {
			t.Fatalf("GetCommunityGear: %v", err)
		}
		if cg == nil {
			t.Fatalf("gear %s not shared with community %s", gearID, communityID)
		}
		return cg.Availability
	}

	hasSystemMessage := func(gearID string, action models.ChatSystemAction) bool {
		gear := &models.Gear{}
		if err := testStorage.GetByID(context.Background(), gearID, gear); err != nil {
			t.Fatalf("reload gear: %v", err)
		}
		if gear.ConversationId == "" {
			return false
		}
		messages, err := testStorage.QueryByField(context.Background(), "conversation_id", gear.ConversationId, &models.ChatMessage{})
		if err != nil {
			t.Fatalf("query messages: %v", err)
		}
		for _, msg := range messages {
			sysMsg := msg.(*models.ChatMessage).GetSystemMessage()
			if sysMsg != nil && sysMsg.Action == action {
				return true
			}
		}
		return false
	}

	t.Run("owner flips a single-community loan to giveaway and it is announced", func(t *testing.T) {
		communityID := createCommunity("Solo Community")
		gearID := shareGear(communityID, "Puzzle", api.Availability_AVAILABILITY_FOR_LOAN)

		if _, err := service.SetGearAvailability(ctxOwner, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId: gearID, Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		})); err != nil {
			t.Fatalf("SetGearAvailability: %v", err)
		}

		if got := availabilityOf(communityID, gearID); got != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("availability = %v, want FOR_GIVEAWAY", got)
		}
		if !hasSystemMessage(gearID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED) {
			t.Error("expected a GIVEAWAY_SHARED system message after the flip")
		}
	})

	t.Run("flip applies across every community the gear is shared with", func(t *testing.T) {
		commA := createCommunity("Community A")
		commB := createCommunity("Community B")

		// One gear shared into two communities, both FOR_LOAN.
		gear := &models.Gear{Name: "Ladder", OwnerId: ownerID, State: models.GearState_GEAR_STATE_AVAILABLE}
		gearID, err := testStorage.Insert(context.Background(), gear)
		if err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		for _, c := range []string{commA, commB} {
			shareGearForTestWithAvailability(
				t, service, ctxOwner, gearID, c, models.Availability_AVAILABILITY_FOR_LOAN,
			)
		}

		if _, err := service.SetGearAvailability(ctxOwner, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId: gearID, Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		})); err != nil {
			t.Fatalf("SetGearAvailability: %v", err)
		}

		if got := availabilityOf(commA, gearID); got != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("community A availability = %v, want FOR_GIVEAWAY", got)
		}
		if got := availabilityOf(commB, gearID); got != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("community B availability = %v, want FOR_GIVEAWAY", got)
		}
	})

	t.Run("rejects the flip while an in-progress transfer exists", func(t *testing.T) {
		communityID := createCommunity("Busy Community")
		gearID := shareGear(communityID, "On-Loan Tent", api.Availability_AVAILABILITY_FOR_LOAN)

		// An active loan makes the item physically out — the giveaway machine has
		// no ACTIVE state, so the flip must be refused.
		if _, err := testStorage.Insert(context.Background(), &models.Transfer{
			GearId:       gearID,
			CommunityId:  communityID,
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_ACTIVE,
			RecipientId:  nonOwnerID,
		}); err != nil {
			t.Fatalf("insert transfer: %v", err)
		}

		_, err := service.SetGearAvailability(ctxOwner, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId: gearID, Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		}))
		if err == nil {
			t.Fatal("expected SetGearAvailability to fail while a transfer is in progress")
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
		}
		// The junction must be untouched.
		if got := availabilityOf(communityID, gearID); got != models.Availability_AVAILABILITY_FOR_LOAN {
			t.Errorf("availability = %v after rejected flip, want unchanged FOR_LOAN", got)
		}
	})

	t.Run("a terminal (cancelled) transfer does not block the flip", func(t *testing.T) {
		communityID := createCommunity("Past Transfer Community")
		gearID := shareGear(communityID, "Formerly Requested Grill", api.Availability_AVAILABILITY_FOR_LOAN)
		if _, err := testStorage.Insert(context.Background(), &models.Transfer{
			GearId:       gearID,
			CommunityId:  communityID,
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_CANCELLED,
			RecipientId:  nonOwnerID,
		}); err != nil {
			t.Fatalf("insert transfer: %v", err)
		}

		if _, err := service.SetGearAvailability(ctxOwner, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId: gearID, Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		})); err != nil {
			t.Fatalf("SetGearAvailability should succeed past a cancelled transfer: %v", err)
		}
		if got := availabilityOf(communityID, gearID); got != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("availability = %v, want FOR_GIVEAWAY", got)
		}
	})

	t.Run("non-owner cannot set availability", func(t *testing.T) {
		communityID := createCommunity("Owned Community")
		addUserToCommunity(t, service, communityID, ownerID, nonOwnerID, "owner@example.com", "nonowner@example.com")
		gearID := shareGear(communityID, "Owner-Only Saw", api.Availability_AVAILABILITY_FOR_LOAN)

		_, err := service.SetGearAvailability(ctxNonOwner, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId: gearID, Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		}))
		if err == nil {
			t.Fatal("expected non-owner to be denied")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("code = %v, want PermissionDenied", connect.CodeOf(err))
		}
	})

	t.Run("unknown gear returns NotFound", func(t *testing.T) {
		_, err := service.SetGearAvailability(ctxOwner, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId: "nonexistent-gear-id", Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		}))
		if err == nil {
			t.Fatal("expected NotFound for unknown gear")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("code = %v, want NotFound", connect.CodeOf(err))
		}
	})

	t.Run("unspecified availability is rejected", func(t *testing.T) {
		communityID := createCommunity("Validation Community")
		gearID := shareGear(communityID, "Well-Shared Wrench", api.Availability_AVAILABILITY_FOR_LOAN)
		_, err := service.SetGearAvailability(ctxOwner, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId: gearID, Availability: api.Availability_AVAILABILITY_UNSPECIFIED,
		}))
		if err == nil {
			t.Fatal("expected InvalidArgument for unspecified availability")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("setting the mode it already has is a no-op success", func(t *testing.T) {
		communityID := createCommunity("Idempotent Community")
		gearID := shareGear(communityID, "Already Giveaway", api.Availability_AVAILABILITY_FOR_GIVEAWAY)
		if _, err := service.SetGearAvailability(ctxOwner, connect.NewRequest(&api.SetGearAvailabilityRequest{
			GearId: gearID, Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		})); err != nil {
			t.Fatalf("idempotent SetGearAvailability: %v", err)
		}
		if got := availabilityOf(communityID, gearID); got != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("availability = %v, want FOR_GIVEAWAY", got)
		}
	})
}
