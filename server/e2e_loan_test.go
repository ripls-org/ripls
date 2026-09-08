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

// TestEndToEnd_CompleteLoanLifecycle tests complete loan lifecycle.
func TestEndToEnd_CompleteLoanLifecycle(t *testing.T) {
	serverURL := getTestServerURL(t)
	t.Logf("Testing against server: %s", serverURL)

	simID := generateSimulationID("loan")
	registerSimulationCleanup(t, serverURL, simID)

	ctx := context.Background()

	// Create two test users: owner and borrower
	ownerToken, _ := createUniqueTestUser(t, serverURL, "loan-owner", simID)

	borrowerToken, _ := createUniqueTestUser(t, serverURL, "loan-borrower", simID)

	// Create authenticated clients for both users
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

	ownerLoanClient := apiconnect.NewTransferServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: ownerToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	borrowerCommunityClient := apiconnect.NewCommunityServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: borrowerToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	borrowerLoanClient := apiconnect.NewTransferServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: borrowerToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	// Step 1: Owner creates a community
	t.Log("Step 1: Owner creating community...")
	communityResp, err := ownerCommunityClient.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:         "E2E Test Loan Community",
		Description:  "Community for testing loan lifecycle",
		SimulationId: proto.String(simID),
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	communityID := communityResp.Msg.Id
	t.Logf("Created community: %s", communityID)

	// Step 2: Owner creates invitation link
	t.Log("Step 2: Owner creating invitation link...")
	inviteLinkResp, err := ownerCommunityClient.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	}))
	if err != nil {
		t.Fatalf("GetOrCreateShareLink failed: %v", err)
	}

	// Step 3: Borrower accepts invitation link
	t.Log("Step 3: Borrower accepting invitation link...")
	_, err = borrowerCommunityClient.AcceptInvitationLink(ctx, connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: inviteLinkResp.Msg.ShortCode,
	}))
	if err != nil {
		t.Fatalf("AcceptInvitationLink failed: %v", err)
	}

	// Step 4: Owner adds gear
	t.Log("Step 4: Owner adding gear...")
	addResp, err := ownerGearClient.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        proto.String("E2E Test Power Drill"),
		Description: proto.String("20V cordless drill for end-to-end testing"),
	}))
	if err != nil {
		t.Fatalf("SaveGear failed: %v", err)
	}
	gearID := addResp.Msg.Id
	t.Logf("Created gear: %s", gearID)

	// Step 5: Owner shares gear with community (makes it available for borrowing)
	t.Log("Step 5: Owner sharing gear with community...")
	// Sharing goes through the unified CommunityService.ShareItem; the gear's
	// Lend/Give mode is item-wide (#2492/#2687) and FOR_LOAN by default, so no
	// separate availability call is needed here.
	_, err = ownerCommunityClient.ShareItem(ctx, connect.NewRequest(&api.ShareItemRequest{
		Item:                &api.ShareItemRequest_GearId{GearId: gearID},
		ShareToCommunityIds: []string{communityID},
	}))
	if err != nil {
		t.Fatalf("ShareItem failed: %v", err)
	}

	// Verify gear is AVAILABLE (shared with at least one community)
	getResp, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
		Id: gearID,
	}))
	if err != nil {
		t.Fatalf("GetGear failed: %v", err)
	}
	if getResp.Msg.State != api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
		t.Errorf("Expected gear state AVAILABLE, got %v", getResp.Msg.State)
	}

	// Step 6: Borrower expresses interest in transfer
	t.Log("Step 6: Borrower expressing interest in transfer...")
	_, err = borrowerLoanClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}

	// Get the transfer ID by listing received transfers
	listTransfersResp, err := borrowerLoanClient.ListReceivedTransfers(ctx, connect.NewRequest(&api.ListReceivedTransfersRequest{}))
	if err != nil {
		t.Fatalf("ListReceivedTransfers failed: %v", err)
	}
	if len(listTransfersResp.Msg.Transfers) == 0 {
		t.Fatal("Expected at least one transfer after expressing interest")
	}
	transferID := listTransfersResp.Msg.Transfers[0].Id
	t.Logf("Created transfer request: %s", transferID)

	// Step 7: Handle transfer state (auto-approval or manual selection)
	t.Log("Step 7: Checking transfer state...")
	transferState := listTransfersResp.Msg.Transfers[0].State

	switch transferState {
	case api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		// New behavior: loans are auto-approved
		t.Log("Transfer was auto-approved to RECIPIENT_SELECTED state (new behavior)")
	case api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
		// Old behavior: owner must manually select recipient
		t.Log("Transfer is in INTEREST_EXPRESSED state, owner must select recipient (old behavior)")

		// Owner selects recipient
		_, err = ownerLoanClient.SelectRecipient(ctx, connect.NewRequest(&api.SelectRecipientRequest{
			TransferId:  transferID,
			RecipientId: listTransfersResp.Msg.Transfers[0].Recipient.Id,
		}))
		if err != nil {
			t.Fatalf("SelectRecipient failed: %v", err)
		}
		t.Log("Owner selected recipient")
	default:
		t.Fatalf("Unexpected transfer state: %v", transferState)
	}

	// Step 8: Owner starts transfer (ACTIVE state)
	t.Log("Step 8: Owner starting transfer...")
	_, err = ownerLoanClient.StartLoan(ctx, connect.NewRequest(&api.StartLoanRequest{
		TransferId: transferID,
	}))
	if err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Step 9: Verify gear is now UNAVAILABLE (cannot express interest in new transfer)
	t.Log("Step 9: Verifying gear is unavailable during active transfer...")
	_, err = borrowerLoanClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err == nil {
		t.Fatal("Expected error when expressing interest for unavailable gear, but got none")
	}
	t.Logf("Correctly rejected transfer request for unavailable gear: %v", err)

	// Step 10: Borrower completes transfer
	t.Log("Step 10: Borrower completing transfer...")
	_, err = borrowerLoanClient.CompleteTransfer(ctx, connect.NewRequest(&api.CompleteTransferRequest{
		TransferId: transferID,
	}))
	if err != nil {
		t.Fatalf("CompleteTransfer failed: %v", err)
	}

	// Step 11: Verify gear is AVAILABLE again
	t.Log("Step 11: Verifying gear is available again after completion...")
	getResp2, err := ownerGearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
		Id: gearID,
	}))
	if err != nil {
		t.Fatalf("GetGear failed after completion: %v", err)
	}
	if getResp2.Msg.State != api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
		t.Errorf("Expected gear state AVAILABLE after completion, got %v", getResp2.Msg.State)
	}

	// Step 12: Verify can express interest in new transfer now
	t.Log("Step 12: Verifying can express interest in new transfer after completion...")
	_, err = borrowerLoanClient.ExpressInterest(ctx, connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	}))
	if err != nil {
		t.Fatalf("Second ExpressInterest should succeed after completion: %v", err)
	}

	// Get the second transfer ID
	listTransfers2Resp, err := borrowerLoanClient.ListReceivedTransfers(ctx, connect.NewRequest(&api.ListReceivedTransfersRequest{}))
	if err != nil {
		t.Fatalf("ListReceivedTransfers failed: %v", err)
	}
	if len(listTransfers2Resp.Msg.Transfers) == 0 {
		t.Fatal("Expected at least one transfer")
	}
	transfer2ID := listTransfers2Resp.Msg.Transfers[0].Id // Most recent transfer

	// Check if we got a new transfer or if the system reused the archived one
	if transfer2ID == transferID {
		t.Logf("System reused archived transfer (this may be expected behavior)")
	} else {
		t.Logf("Successfully created second transfer request: %s", transfer2ID)

		// Cancel the second transfer to clean up (only if it's a new transfer)
		_, err = borrowerLoanClient.CancelTransfer(ctx, connect.NewRequest(&api.CancelTransferRequest{
			TransferId: transfer2ID,
		}))
		if err != nil {
			t.Logf("Warning: Failed to cancel second transfer: %v", err)
		}
	}

	t.Log("✓ Complete transfer lifecycle test passed!")
}
