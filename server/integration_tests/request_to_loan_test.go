package integration_tests

// Integration tests for gear-backed request offers (#2702): a helper offers
// their actual item toward a request, the handoff auto-fulfills the request,
// the fulfilled request stands down its sibling offers, cancellation unwinds the
// claim/offer, and undoing a handoff-driven fulfillment cancels the loan.
// Covers docs/issues/2702-request-to-loan.md Phases 1–2.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/storage"
)

// pollUntil retries cond every 200ms until it returns true or the timeout
// elapses. Bus-driven couplings land asynchronously after the triggering RPC
// returns, so assertions on them must poll.
func pollUntil(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		//nolint:forbidigo // Backoff inside a bounded condition poll, which is the
		// pattern #1364 prescribes as the alternative to a fixed-duration sleep.
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

func TestRequestToLoan_HandoffAutoFulfillsAndUndoUnwinds(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL := startTestServer(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	// Requester June with a community; helpers Theo and Bea join it.
	juneToken, _ := registerFirstUser(t, serverURL, "june@example.com", "June")
	juneCommunityClient := createAuthCommunityClient(juneToken, serverURL)
	juneRequestClient := createAuthRequestClient(juneToken, serverURL)
	communityID := setupTestCommunity(t, ctx, juneCommunityClient, "Cedar Court", "Request-to-loan tests")

	theoToken, theoID := registerUserByInvite(t, serverURL, juneToken, communityID, "theo@example.com", "Theo")
	theoRequestClient := createAuthRequestClient(theoToken, serverURL)
	theoTransferClient := createAuthTransferClient(theoToken, serverURL)
	theoGearClient := createAuthGearClient(theoToken, serverURL)
	theoLocationClient := createAuthLocationClient(theoToken, serverURL)

	beaToken, _ := registerUserByInvite(t, serverURL, juneToken, communityID, "bea@example.com", "Bea")
	beaRequestClient := createAuthRequestClient(beaToken, serverURL)
	beaTransferClient := createAuthTransferClient(beaToken, serverURL)
	beaGearClient := createAuthGearClient(beaToken, serverURL)
	beaLocationClient := createAuthLocationClient(beaToken, serverURL)

	// June posts the request with a single one-slot need and shares it.
	seedName := "Lawn mower"
	submitResp, err := juneRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:         "Looking for a lawn mower",
		Description:   "Ours gave up mid-mow and the grass won",
		SeedNeedNames: []string{seedName},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId
	shareRequestIntoCommunity(t, ctx, juneCommunityClient, requestID, communityID)
	// Every request is born with one claimable need (#2702) — resolve it.
	needID := seededNeedID(t, ctx, juneRequestClient, requestID, "Lawn mower")

	// Theo claims the need with his mower and escalates into a loan offer.
	theoLocation := setupTestLocation(t, ctx, theoLocationClient, "Portland")
	theoGearID := setupTestGear(t, ctx, theoGearClient, "Honda Mower", "Self-propelled, starts first pull", theoLocation)
	claimResp, err := theoRequestClient.ClaimRequestNeed(ctx, connect.NewRequest(&api.ClaimRequestNeedRequest{
		NeedId:      needID,
		RequestId:   requestID,
		CommunityId: communityID,
		GearId:      &theoGearID,
	}))
	if err != nil {
		t.Fatalf("ClaimRequestNeed: %v", err)
	}
	theoContributionID := claimResp.Msg.Contribution.Id

	theoOfferResp, err := theoTransferClient.OfferTransfer(ctx, connect.NewRequest(&api.OfferTransferRequest{
		GearId:          theoGearID,
		TransferType:    api.TransferType_TRANSFER_TYPE_LOAN,
		RecipientUserId: submitRequesterID(t, ctx, juneRequestClient, requestID),
		CommunityId:     communityID,
		OriginRequestId: requestID,
		ContributionId:  theoContributionID,
	}))
	if err != nil {
		t.Fatalf("Theo OfferTransfer: %v", err)
	}
	theoTransferID := theoOfferResp.Msg.Transfer.Id
	if theoOfferResp.Msg.Transfer.State != api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("expected RECIPIENT_SELECTED, got %s", theoOfferResp.Msg.Transfer.State)
	}

	// Bea offers her mower too — a freestanding contribution, second offer.
	beaLocation := setupTestLocation(t, ctx, beaLocationClient, "Portland")
	beaGearID := setupTestGear(t, ctx, beaGearClient, "Reel Mower", "Quiet and sharp", beaLocation)
	beaContribResp, err := beaRequestClient.AddRequestContribution(ctx, connect.NewRequest(&api.AddRequestContributionRequest{
		RequestId: requestID,
		Title:     "Reel mower",
		GearId:    &beaGearID,
	}))
	if err != nil {
		t.Fatalf("Bea AddRequestContribution: %v", err)
	}
	beaOfferResp, err := beaTransferClient.OfferTransfer(ctx, connect.NewRequest(&api.OfferTransferRequest{
		GearId:          beaGearID,
		TransferType:    api.TransferType_TRANSFER_TYPE_LOAN,
		RecipientUserId: submitRequesterID(t, ctx, juneRequestClient, requestID),
		CommunityId:     communityID,
		OriginRequestId: requestID,
		ContributionId:  beaContribResp.Msg.Contribution.Id,
	}))
	if err != nil {
		t.Fatalf("Bea OfferTransfer: %v", err)
	}
	beaTransferID := beaOfferResp.Msg.Transfer.Id

	// June sees both gear offers and accepts Theo's.
	getResp, err := juneRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if len(getResp.Msg.Request.GearOffers) != 2 {
		t.Fatalf("expected 2 gear offers, got %d", len(getResp.Msg.Request.GearOffers))
	}
	acceptResp, err := juneRequestClient.AcceptRequestOffer(ctx, connect.NewRequest(&api.AcceptRequestOfferRequest{
		RequestId:      requestID,
		ContributionId: theoContributionID,
	}))
	if err != nil {
		t.Fatalf("AcceptRequestOffer: %v", err)
	}
	acceptedSeen := false
	for _, offer := range acceptResp.Msg.Request.GearOffers {
		if offer.TransferId == theoTransferID && offer.GetAcceptedAtUnixSec() > 0 {
			acceptedSeen = true
		}
	}
	if !acceptedSeen {
		t.Fatal("expected Theo's offer to carry accepted_at after AcceptRequestOffer")
	}

	// Theo hands the mower over — the request auto-fulfills and Bea's offer
	// politely stands down.
	if _, err := theoTransferClient.StartLoan(ctx, connect.NewRequest(&api.StartLoanRequest{TransferId: theoTransferID})); err != nil {
		t.Fatalf("StartLoan: %v", err)
	}
	pollUntil(t, 15*time.Second, "request auto-fulfilled", func() bool {
		resp, err := juneRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
		return err == nil && resp.Msg.Request.State == api.RequestState_REQUEST_STATE_FULFILLED
	})
	fulfilled, err := juneRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest after fulfill: %v", err)
	}
	if len(fulfilled.Msg.Request.ConfirmedHelperIds) != 1 || fulfilled.Msg.Request.ConfirmedHelperIds[0] != theoID {
		t.Errorf("expected confirmed helper [%s], got %v", theoID, fulfilled.Msg.Request.ConfirmedHelperIds)
	}
	pollUntil(t, 15*time.Second, "sibling offer cancelled", func() bool {
		resp, err := beaTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{TransferId: beaTransferID}))
		return err == nil && resp.Msg.Transfer.State == api.TransferState_TRANSFER_STATE_CANCELLED
	})

	// June undoes the fulfillment — the loan cancels too (decision 10) and
	// Theo's claim unwinds (decision 11), reopening the need.
	//
	// Poll for the event rather than reading once. fulfillRequestCore writes
	// the request's FULFILLED state and only then records the community event,
	// so the state poll above can return between the two writes and find no
	// event yet. Reaching FULFILLED is not a promise that its event has landed.
	var eventsResp *connect.Response[api.ListCommunityEventsResponse]
	pollUntil(t, 15*time.Second, "REQUEST_FULFILLED event recorded", func() bool {
		resp, err := juneCommunityClient.ListCommunityEvents(ctx, connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId: communityID,
			EventTypes:  []api.CommunityEventType{api.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED},
		}))
		if err != nil || len(resp.Msg.Events) == 0 {
			return false
		}
		eventsResp = resp
		return true
	})
	if _, err := juneRequestClient.UndoMarkRequestFulfilled(ctx, connect.NewRequest(&api.UndoMarkRequestFulfilledRequest{
		CommunityEventId: eventsResp.Msg.Events[0].Id,
	})); err != nil {
		t.Fatalf("UndoMarkRequestFulfilled: %v", err)
	}

	pollUntil(t, 15*time.Second, "loan cancelled by fulfillment undo", func() bool {
		resp, err := theoTransferClient.GetTransfer(ctx, connect.NewRequest(&api.GetTransferRequest{TransferId: theoTransferID}))
		return err == nil && resp.Msg.Transfer.State == api.TransferState_TRANSFER_STATE_CANCELLED
	})
	pollUntil(t, 15*time.Second, "claim unwound and need reopened", func() bool {
		listResp, err := theoRequestClient.ListRequestNeedsAndContributions(ctx, connect.NewRequest(&api.ListRequestNeedsAndContributionsRequest{RequestId: requestID}))
		if err != nil {
			return false
		}
		for _, c := range listResp.Msg.Contributions {
			if c.Id == theoContributionID {
				return false // Theo's claim should be gone.
			}
		}
		for _, n := range listResp.Msg.Needs {
			if n.Id == needID {
				return n.SlotsRemaining == 1
			}
		}
		return false
	})

	// The request is open again — Bea's offer was already terminal so hers
	// stays cancelled, and her chat offer keeps the request in OFFERS_RECEIVED.
	// The undo restores state synchronously, but the delivered-loan cancellation
	// it cascades feeds unwindCancelledOffer, which can re-write state as a bus
	// step; poll so the read lands after the pipeline settles.
	pollUntil(t, 15*time.Second, "request reopened after undo (not FULFILLED)", func() bool {
		resp, err := juneRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
		return err == nil && resp.Msg.Request.State != api.RequestState_REQUEST_STATE_FULFILLED
	})
}

func TestRequestToLoan_CancelUnwindsClaimAndOffer(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL := startTestServer(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	juneToken, _ := registerFirstUser(t, serverURL, "june2@example.com", "June")
	juneCommunityClient := createAuthCommunityClient(juneToken, serverURL)
	juneRequestClient := createAuthRequestClient(juneToken, serverURL)
	communityID := setupTestCommunity(t, ctx, juneCommunityClient, "Cedar Court", "Cancel unwind test")

	theoToken, _ := registerUserByInvite(t, serverURL, juneToken, communityID, "theo2@example.com", "Theo")
	theoRequestClient := createAuthRequestClient(theoToken, serverURL)
	theoTransferClient := createAuthTransferClient(theoToken, serverURL)
	theoGearClient := createAuthGearClient(theoToken, serverURL)
	theoLocationClient := createAuthLocationClient(theoToken, serverURL)

	seedName := "Pressure washer"
	submitResp, err := juneRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:         "Need a pressure washer",
		Description:   "Deck season",
		SeedNeedNames: []string{seedName},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId
	shareRequestIntoCommunity(t, ctx, juneCommunityClient, requestID, communityID)
	seededID := seededNeedID(t, ctx, juneRequestClient, requestID, "Pressure washer")

	theoLocation := setupTestLocation(t, ctx, theoLocationClient, "Portland")
	theoGearID := setupTestGear(t, ctx, theoGearClient, "Pressure Washer", "2000 PSI", theoLocation)
	claimResp, err := theoRequestClient.ClaimRequestNeed(ctx, connect.NewRequest(&api.ClaimRequestNeedRequest{
		NeedId:      seededID,
		RequestId:   requestID,
		CommunityId: communityID,
		GearId:      &theoGearID,
	}))
	if err != nil {
		t.Fatalf("ClaimRequestNeed: %v", err)
	}
	offerResp, err := theoTransferClient.OfferTransfer(ctx, connect.NewRequest(&api.OfferTransferRequest{
		GearId:          theoGearID,
		TransferType:    api.TransferType_TRANSFER_TYPE_LOAN,
		RecipientUserId: submitRequesterID(t, ctx, juneRequestClient, requestID),
		CommunityId:     communityID,
		OriginRequestId: requestID,
		ContributionId:  claimResp.Msg.Contribution.Id,
	}))
	if err != nil {
		t.Fatalf("OfferTransfer: %v", err)
	}

	// The claim's auto-offer moves the request to OFFERS_RECEIVED. The
	// transition itself is synchronous inside ClaimRequestNeed, but the
	// escalation's brief window where the auto-offer is withdrawn before the
	// escalated offer lands lets the bus-driven last-offer reversion
	// (transfer_subscriber.unwindCancelledOffer) race the read, so poll.
	pollUntil(t, 15*time.Second, "request settled at OFFERS_RECEIVED before cancel", func() bool {
		resp, err := juneRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
		return err == nil && resp.Msg.Request.State == api.RequestState_REQUEST_STATE_OFFERS_RECEIVED
	})

	// Theo cancels his offer pre-handoff: claim unwinds, slot reopens, offer
	// withdraws, and the request reverts to ACTIVE (he was the only offerer).
	if _, err := theoTransferClient.CancelTransfer(ctx, connect.NewRequest(&api.CancelTransferRequest{
		TransferId: offerResp.Msg.Transfer.Id,
	})); err != nil {
		t.Fatalf("CancelTransfer: %v", err)
	}

	pollUntil(t, 15*time.Second, "claim unwound, offer withdrawn, request reverted", func() bool {
		resp, err := juneRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
		if err != nil {
			return false
		}
		if resp.Msg.Request.State != api.RequestState_REQUEST_STATE_ACTIVE {
			return false
		}
		if len(resp.Msg.Request.Offerers) != 0 {
			return false
		}
		listResp, err := juneRequestClient.ListRequestNeedsAndContributions(ctx, connect.NewRequest(&api.ListRequestNeedsAndContributionsRequest{RequestId: requestID}))
		if err != nil {
			return false
		}
		if len(listResp.Msg.Contributions) != 0 {
			return false
		}
		for _, n := range listResp.Msg.Needs {
			if n.Id == seededID {
				return n.SlotsRemaining == 1
			}
		}
		return false
	})
}

func TestRequestToLoan_MultiNeedHandoffLeavesRequestOpen(t *testing.T) {
	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	serverCmd, serverURL := startTestServer(t, dbURL)
	defer func() { _ = serverCmd.Process.Kill() }()

	ctx := context.Background()

	juneToken, _ := registerFirstUser(t, serverURL, "june3@example.com", "June")
	juneCommunityClient := createAuthCommunityClient(juneToken, serverURL)
	juneRequestClient := createAuthRequestClient(juneToken, serverURL)
	communityID := setupTestCommunity(t, ctx, juneCommunityClient, "Cedar Court", "Multi-need test")

	theoToken, _ := registerUserByInvite(t, serverURL, juneToken, communityID, "theo3@example.com", "Theo")
	theoRequestClient := createAuthRequestClient(theoToken, serverURL)
	theoTransferClient := createAuthTransferClient(theoToken, serverURL)
	theoGearClient := createAuthGearClient(theoToken, serverURL)
	theoLocationClient := createAuthLocationClient(theoToken, serverURL)

	seedName := "Folding table"
	submitResp, err := juneRequestClient.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Title:         "Moving day help",
		Description:   "Table and chairs to move",
		SeedNeedNames: []string{seedName},
	}))
	if err != nil {
		t.Fatalf("SubmitRequest: %v", err)
	}
	requestID := submitResp.Msg.RequestId
	shareRequestIntoCommunity(t, ctx, juneCommunityClient, requestID, communityID)
	tableNeedID := seededNeedID(t, ctx, juneRequestClient, requestID, "Folding table")
	if _, err := juneRequestClient.AddRequestNeed(ctx, connect.NewRequest(&api.AddRequestNeedRequest{
		RequestId: requestID, Name: "Chairs", Slots: 1,
	})); err != nil {
		t.Fatalf("AddRequestNeed chairs: %v", err)
	}

	theoLocation := setupTestLocation(t, ctx, theoLocationClient, "Portland")
	tableGearID := setupTestGear(t, ctx, theoGearClient, "Folding Table", "Seats eight", theoLocation)
	claimResp, err := theoRequestClient.ClaimRequestNeed(ctx, connect.NewRequest(&api.ClaimRequestNeedRequest{
		NeedId:      tableNeedID,
		RequestId:   requestID,
		CommunityId: communityID,
		GearId:      &tableGearID,
	}))
	if err != nil {
		t.Fatalf("ClaimRequestNeed: %v", err)
	}
	offerResp, err := theoTransferClient.OfferTransfer(ctx, connect.NewRequest(&api.OfferTransferRequest{
		GearId:          tableGearID,
		TransferType:    api.TransferType_TRANSFER_TYPE_LOAN,
		RecipientUserId: submitRequesterID(t, ctx, juneRequestClient, requestID),
		CommunityId:     communityID,
		OriginRequestId: requestID,
		ContributionId:  claimResp.Msg.Contribution.Id,
	}))
	if err != nil {
		t.Fatalf("OfferTransfer: %v", err)
	}

	// The table hands off. The request has two needs, so it must stay open —
	// the requester closes it via Mark Fulfilled once the chairs land too.
	if _, err := theoTransferClient.StartLoan(ctx, connect.NewRequest(&api.StartLoanRequest{TransferId: offerResp.Msg.Transfer.Id})); err != nil {
		t.Fatalf("StartLoan: %v", err)
	}

	// Give the bus time to (wrongly) fulfill, then assert it didn't.
	//nolint:forbidigo // Asserts an absence (a multi-need request must NOT
	// auto-fulfill on a single handoff). The event bus is asynchronous, so the
	// only way to observe that it never fired is to outwait it.
	time.Sleep(3 * time.Second)
	resp, err := juneRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if resp.Msg.Request.State == api.RequestState_REQUEST_STATE_FULFILLED {
		t.Fatal("multi-need request must not auto-fulfill on a single handoff")
	}

	// Manual fulfillment still works and closes the request.
	if _, err := juneRequestClient.MarkRequestFulfilled(ctx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId: requestID,
	})); err != nil {
		t.Fatalf("MarkRequestFulfilled: %v", err)
	}
	final, err := juneRequestClient.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest final: %v", err)
	}
	if final.Msg.Request.State != api.RequestState_REQUEST_STATE_FULFILLED {
		t.Fatalf("expected FULFILLED after manual fulfill, got %s", final.Msg.Request.State)
	}
}

// seededNeedID resolves the id of the need a request was born with (#2702),
// asserting its name matches the seed that was passed to SubmitRequest.
func seededNeedID(t *testing.T, ctx context.Context, client interface {
	ListRequestNeedsAndContributions(ctx context.Context, req *connect.Request[api.ListRequestNeedsAndContributionsRequest]) (*connect.Response[api.ListRequestNeedsAndContributionsResponse], error)
}, requestID, wantName string,
) string {
	t.Helper()
	resp, err := client.ListRequestNeedsAndContributions(ctx, connect.NewRequest(&api.ListRequestNeedsAndContributionsRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("ListRequestNeedsAndContributions: %v", err)
	}
	for _, n := range resp.Msg.Needs {
		if n.Name == wantName {
			return n.Id
		}
	}
	t.Fatalf("seeded need %q not found; needs: %+v", wantName, resp.Msg.Needs)
	return ""
}

// submitRequesterID resolves the requester's user id from the request — the
// OfferTransfer contract requires recipient_user_id to match it exactly.
func submitRequesterID(t *testing.T, ctx context.Context, client interface {
	GetRequest(ctx context.Context, req *connect.Request[api.GetRequestRequest]) (*connect.Response[api.GetRequestResponse], error)
}, requestID string,
) string {
	t.Helper()
	resp, err := client.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{RequestId: requestID}))
	if err != nil {
		t.Fatalf("GetRequest for requester id: %v", err)
	}
	return resp.Msg.Request.Requester.Id
}
