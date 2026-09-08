package request

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/clock"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impact_metrics "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/services"
)

func TestService_MarkRequestFulfilled(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	// Create request and have someone offer
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	ctx = createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	offerReq := connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	_, _ = service.OfferToFulfill(ctx, offerReq)
	services.WaitForNotification(t, done) // REQUEST_OFFER_MADE triggers notification

	// Requester marks request as fulfilled
	ctx = createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	fulfillReq := connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId: requestID,
	})

	_, err := service.MarkRequestFulfilled(ctx, fulfillReq)
	if err != nil {
		t.Fatalf("MarkRequestFulfilled failed: %v", err)
	}
	services.WaitForNotification(t, done) // REQUEST_FULFILLED triggers notification

	// Verify request state changed to FULFILLED
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to retrieve request: %v", err)
	}

	if request.State != models.RequestState_REQUEST_STATE_FULFILLED {
		t.Errorf("Expected state FULFILLED, got %v", request.State)
	}

	// Verify ImpactEstimate was persisted at fulfillment
	if request.ImpactEstimate == nil {
		t.Error("Expected ImpactEstimate to be set after MarkRequestFulfilled")
	} else if request.ImpactEstimate.TimeSaved == nil || request.ImpactEstimate.TimeSaved.Minutes == nil {
		t.Error("Expected TimeSaved.Minutes to be set after MarkRequestFulfilled")
	}

	// Verify provenance on TimeSaved
	if ts := request.ImpactEstimate.GetTimeSaved(); ts != nil {
		if ts.Provenance == nil {
			t.Error("Expected TimeSaved.Provenance to be set after MarkRequestFulfilled")
		} else {
			if ts.Provenance.Source != models.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
				t.Errorf("Expected TimeSaved source CONFIG_DEFAULT, got %v", ts.Provenance.Source)
			}
			if ts.Provenance.Name == "" {
				t.Error("Expected TimeSaved provenance name to be non-empty")
			}
			if ts.Provenance.Version <= 0 {
				t.Errorf("Expected TimeSaved provenance version > 0, got %d", ts.Provenance.Version)
			}
		}
	}

	// Verify event was created
	events, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
	}, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 fulfilled event, got %d", len(events))
	}
}

func TestService_MarkRequestFulfilledWithConfirmedHelpers(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	helper1ID := setupTestUser(t, testStorage, "Helper One", "helper1@example.com")
	helper2ID := setupTestUser(t, testStorage, "Helper Two", "helper2@example.com")
	helper3ID := setupTestUser(t, testStorage, "Helper Three", "helper3@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, helper1ID, helper2ID, helper3ID)

	// Create request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone)

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// Have three people offer to help
	ctx = createAuthenticatedContext(helper1ID, "helper1@example.com", models.Role_ROLE_USER)
	_, _ = service.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	services.WaitForNotification(t, done)

	ctx = createAuthenticatedContext(helper2ID, "helper2@example.com", models.Role_ROLE_USER)
	_, _ = service.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	services.WaitForNotification(t, done)

	ctx = createAuthenticatedContext(helper3ID, "helper3@example.com", models.Role_ROLE_USER)
	_, _ = service.OfferToFulfill(ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	services.WaitForNotification(t, done)

	// Requester marks request as fulfilled with summary and confirmed helpers
	// Only helper1 and helper2 are confirmed as main helpers
	ctx = createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	resolutionSummary := "Got the drill from Helper One and Helper Two. They were very helpful!"
	fulfillReq := connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId:          requestID,
		ResolutionSummary:  &resolutionSummary,
		ConfirmedHelperIds: []string{helper1ID, helper2ID},
	})

	resp, err := service.MarkRequestFulfilled(ctx, fulfillReq)
	if err != nil {
		t.Fatalf("MarkRequestFulfilled failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Verify request was updated with confirmed helpers
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to retrieve request: %v", err)
	}

	if request.State != models.RequestState_REQUEST_STATE_FULFILLED {
		t.Errorf("Expected state FULFILLED, got %v", request.State)
	}

	if request.GetResolutionSummary() != resolutionSummary {
		t.Errorf("Expected resolution summary %q, got %q", resolutionSummary, request.GetResolutionSummary())
	}

	if len(request.ConfirmedHelperIds) != 2 {
		t.Errorf("Expected 2 confirmed helpers, got %d", len(request.ConfirmedHelperIds))
	}

	// Verify confirmed helper IDs match
	confirmedMap := make(map[string]bool)
	for _, id := range request.ConfirmedHelperIds {
		confirmedMap[id] = true
	}

	if !confirmedMap[helper1ID] {
		t.Errorf("Expected helper1 (%s) to be in confirmed helpers", helper1ID)
	}

	if !confirmedMap[helper2ID] {
		t.Errorf("Expected helper2 (%s) to be in confirmed helpers", helper2ID)
	}

	if confirmedMap[helper3ID] {
		t.Errorf("Did not expect helper3 (%s) to be in confirmed helpers", helper3ID)
	}

	// Verify response includes the confirmed helper IDs
	if len(resp.Msg.Request.Offerers) != 3 {
		t.Errorf("Expected 3 offerers in response, got %d", len(resp.Msg.Request.Offerers))
	}
}

// TestService_MarkRequestFulfilled_SkipsAIDuringSimulation verifies that InferSocialAttributes
// is not called when fulfilling a request in a simulated context.
func TestService_MarkRequestFulfilled_SkipsAIDuringSimulation(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	// Override AI provider with one that tracks InferSocialAttributes calls.
	mockAI := ai.NewMockProvider()
	inferCalled := false
	mockAI.InferSocialAttributesFunc = func(_ context.Context, _, _, _ string, _ float32) (*ai.SocialAttributeInference, error) {
		inferCalled = true
		t.Error("InferSocialAttributes should not be called during simulation")
		return nil, nil
	}
	service.aiProvider = mockAI

	// Submit request.
	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a power drill",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)

	// Offerer offers to fulfill.
	offererCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	_, err = service.OfferToFulfill(offererCtx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Mark fulfilled with simulated context — AI should be skipped.
	simCtx := clock.WithSimulationTime(requesterCtx, time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC))
	_, err = service.MarkRequestFulfilled(simCtx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled failed: %v", err)
	}
	services.WaitForNotification(t, done)

	if inferCalled {
		t.Fatal("InferSocialAttributes was called during simulation")
	}

	// Verify request still fulfilled successfully with impact estimate.
	request := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, request); err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}
	if request.State != models.RequestState_REQUEST_STATE_FULFILLED {
		t.Errorf("Expected FULFILLED state, got %v", request.State)
	}
	if request.ImpactEstimate == nil {
		t.Error("Expected ImpactEstimate to be set even without AI inference")
	}
}

// TestMarkRequestFulfilled_WithOverrides_PersistsPerInputProvenance verifies that
// user-supplied overrides on MarkRequestFulfilled are applied to the stored
// ImpactEstimate with USER provenance on overridden attributes.
func TestMarkRequestFulfilled_WithOverrides_PersistsPerInputProvenance(t *testing.T) {
	service, testStorage, _, done, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	offererCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)

	createResp, _ := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need help moving furniture",
	}))
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)

	_, _ = service.OfferToFulfill(offererCtx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	services.WaitForNotification(t, done)

	// Fulfill with an explicit money-savings override via HireEquivalentValue input.
	overrideValueUSD := float32(75.00)
	resp, err := service.MarkRequestFulfilled(requesterCtx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId: requestID,
		MoneySavingsOverrides: &api.MoneySavings{
			Inputs: &api.MoneySavingsInput{
				HireEquivalentValue: &api.Estimate{Mean: overrideValueUSD},
			},
		},
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled with overrides failed: %v", err)
	}
	services.WaitForNotification(t, done)

	ie := resp.Msg.Impact
	if ie == nil {
		t.Fatal("impact estimate is nil in response")
	}

	// Money savings must reflect the override value.
	ms := ie.MoneySaved
	if ms == nil {
		t.Fatal("MoneySaved is nil in response")
	}
	if ms.GetValueUsd().GetMean() != overrideValueUSD {
		t.Errorf("ValueUsd.Mean = %v, want %v", ms.GetValueUsd().GetMean(), overrideValueUSD)
	}

	// Top-level provenance must be USER because the field was overridden.
	if ms.GetProvenance().GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("MoneySaved.Provenance.Source = %v, want USER", ms.GetProvenance().GetSource())
	}

	// Stored estimate must also reflect the override.
	stored := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, stored); err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	storedIE := impact_metrics.ModelsImpactToAPI(stored.ImpactEstimate)
	if storedIE == nil || storedIE.MoneySaved == nil {
		t.Fatal("stored ImpactEstimate or MoneySaved is nil")
	}
	if storedIE.MoneySaved.GetValueUsd().GetMean() != overrideValueUSD {
		t.Errorf("stored ValueUsd.Mean = %v, want %v", storedIE.MoneySaved.GetValueUsd().GetMean(), overrideValueUSD)
	}
	if storedIE.MoneySaved.GetProvenance().GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("stored MoneySaved provenance is not USER, got %v", storedIE.MoneySaved.GetProvenance().GetSource())
	}
}
