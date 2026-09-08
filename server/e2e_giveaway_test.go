package main

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
)

// TestEndToEnd_CompleteGiveawayLifecycle tests complete giveaway lifecycle.
func TestEndToEnd_CompleteGiveawayLifecycle(t *testing.T) {
	serverURL := getTestServerURL(t)
	t.Logf("Testing against server: %s", serverURL)

	simID := generateSimulationID("giveaway")
	registerSimulationCleanup(t, serverURL, simID)

	ctx := context.Background()

	// Create three test users: owner and two potential recipients
	ownerToken, _ := createUniqueTestUser(t, serverURL, "giveaway-owner", simID)

	recipient1Token, recipient1UserID := createUniqueTestUser(t, serverURL, "giveaway-recipient1", simID)

	recipient2Token, _ := createUniqueTestUser(t, serverURL, "giveaway-recipient2", simID)

	// Create authenticated clients
	ownerGearClient := apiconnect.NewGearServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: ownerToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	ownerCommunityClient := apiconnect.NewCommunityServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: ownerToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	ownerTransferClient := apiconnect.NewTransferServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: ownerToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	recipient1CommunityClient := apiconnect.NewCommunityServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: recipient1Token,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	recipient1TransferClient := apiconnect.NewTransferServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: recipient1Token,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	recipient2CommunityClient := apiconnect.NewCommunityServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: recipient2Token,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	recipient2TransferClient := apiconnect.NewTransferServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: recipient2Token,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	// Step 1: Owner creates a community
	t.Log("Step 1: Owner creating community...")
	communityResp, err := ownerCommunityClient.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:         "E2E Test Giveaway Community",
		Description:  "Community for testing giveaway lifecycle",
		SimulationId: proto.String(simID),
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := communityResp.Msg.Id
	t.Logf("Created community: %s", communityID)

	// Step 2: Owner creates invitation link and both recipients join
	t.Log("Step 2: Owner creating invitation link...")
	inviteLinkResp, err := ownerCommunityClient.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	}))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink failed: %v", err)
	}

	t.Log("Step 3: Recipients accepting invitation link...")
	_, err = recipient1CommunityClient.AcceptInvitationLink(ctx, connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: inviteLinkResp.Msg.ShortCode,
	}))
	if err != nil {
		t.Fatalf("Recipient1 AcceptInvitationLink failed: %v", err)
	}

	_, err = recipient2CommunityClient.AcceptInvitationLink(ctx, connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: inviteLinkResp.Msg.ShortCode,
	}))
	if err != nil {
		t.Fatalf("Recipient2 AcceptInvitationLink failed: %v", err)
	}

	// Step 4: Owner adds gear
	t.Log("Step 4: Owner adding gear...")
	addResp, err := ownerGearClient.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        proto.String("E2E Test Camping Tent"),
		Description: proto.String("4-person tent for end-to-end giveaway testing"),
	}))
	if err != nil {
		t.Fatalf("SaveGear failed: %v", err)
	}
	gearID := addResp.Msg.Id
	t.Logf("Created gear: %s", gearID)

	// Step 5: Owner shares gear as giveaway
	t.Log("Step 5: Owner sharing gear as giveaway...")
	// The gear's Lend/Give mode is item-wide (#2492/#2687), so it is its own
	// call — and it comes before the share, because ShareItem inherits the mode
	// and the GEAR_SHARED event takes its "New giveaway" copy from it.
	_, err = ownerCommunityClient.SetGearAvailability(ctx, connect.NewRequest(&api.SetGearAvailabilityRequest{
		GearId:       gearID,
		Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
	}))
	if err != nil {
		t.Fatalf("SetGearAvailability failed: %v", err)
	}
	_, err = ownerCommunityClient.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_GearId{GearId: gearID},
		ShareToCommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("ShareItem failed: %v", err)
	}

	// Step 6: Two recipients express interest
	t.Log("Step 6: Recipients expressing interest...")
	_, err = recipient1TransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("Recipient1 ExpressInterest failed: %v", err)
	}

	_, err = recipient2TransferClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("Recipient2 ExpressInterest failed: %v", err)
	}

	// Get the transfer ID from owner's perspective
	listMyTransfersResp, err := ownerTransferClient.ListMyTransfers(ctx, connect.NewRequest(&api.ListMyTransfersRequest{
		TransferType: api.TransferType_TRANSFER_TYPE_GIVEAWAY,
	}))
	if err != nil {
		t.Fatalf("ListMyTransfers failed: %v", err)
	}
	if len(listMyTransfersResp.Msg.Transfers) == 0 {
		t.Fatal("Expected at least one giveaway transfer after interest expressed")
	}
	transferID := listMyTransfersResp.Msg.Transfers[0].Id
	t.Logf("Created giveaway transfer: %s", transferID)

	// Step 7: Owner selects recipient1 as the winner
	t.Log("Step 7: Owner selecting recipient...")
	_, err = ownerTransferClient.SelectRecipient(ctx, connect.NewRequest(&api.SelectRecipientRequest{
		TransferId:  transferID,
		RecipientId: recipient1UserID,
	}))
	if err != nil {
		t.Fatalf("SelectRecipient failed: %v", err)
	}

	// Step 8: Verify giveaway doesn't require StartLoan (goes directly to completion)
	// For giveaways, we skip the ACTIVE state and go directly to completion
	t.Log("Step 8: Owner completing giveaway (ownership transfer)...")
	_, err = ownerTransferClient.CompleteTransfer(ctx, connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: transferID,
	}))
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	// Step 9: Verify gear is no longer available for new transfers
	t.Log("Step 9: Verifying gear is archived after giveaway...")
	getResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
		Id: gearID,
	}))
	if err != nil {
		t.Fatalf("GetGear failed: %v", err)
	}
	// After giveaway, gear should be in a terminal state
	if getResp.Msg.State == api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
		t.Errorf("Expected gear to be unavailable after giveaway, got %v", getResp.Msg.State)
	}

	t.Log("✓ Complete giveaway lifecycle test passed!")
}
