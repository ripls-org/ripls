package request

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestBuildHintFromStoredRequest covers the four meaningful input combinations for
// buildHintFromStoredRequest.
func TestBuildHintFromStoredRequest(t *testing.T) {
	t.Run("nil social context and nil impact estimate returns nil", func(t *testing.T) {
		req := &models.Request{}
		got := buildHintFromStoredRequest(req)
		if got != nil {
			t.Errorf("expected nil hint, got %+v", got)
		}
	})

	t.Run("social context only sets SocialContext field", func(t *testing.T) {
		req := &models.Request{
			SocialContext: &models.SocialContext{
				TieStrength:   int32(api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACQUAINTANCE),
				Vulnerability: int32(api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH),
			},
		}
		got := buildHintFromStoredRequest(req)
		if got == nil {
			t.Fatal("expected non-nil hint, got nil")
		}
		if got.SocialContext == nil {
			t.Fatal("expected SocialContext to be set in hint")
		}
		if got.SocialContext.Vulnerability != api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH {
			t.Errorf("expected HIGH vulnerability, got %v", got.SocialContext.Vulnerability)
		}
		if got.VulnerabilityLevel != "" {
			t.Errorf("expected VulnerabilityLevel to be empty when SocialContext is set, got %q", got.VulnerabilityLevel)
		}
	})

	t.Run("impact estimate only sets VulnerabilityLevel from stored attributes", func(t *testing.T) {
		req := &models.Request{
			ImpactEstimate: &models.ImpactEstimate{
				QualityTime: &models.QualityTimeEstimate{
					Attributes: &models.QualityTimeAttributes{
						Vulnerability: int32(api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM),
					},
				},
			},
		}
		got := buildHintFromStoredRequest(req)
		if got == nil {
			t.Fatal("expected non-nil hint, got nil")
		}
		if got.VulnerabilityLevel != "medium" {
			t.Errorf("expected VulnerabilityLevel %q, got %q", "medium", got.VulnerabilityLevel)
		}
		if got.SocialContext != nil {
			t.Errorf("expected SocialContext to be nil, got %+v", got.SocialContext)
		}
	})

	t.Run("both social context and impact estimate: social context wins", func(t *testing.T) {
		req := &models.Request{
			SocialContext: &models.SocialContext{
				Vulnerability: int32(api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_LOW),
			},
			ImpactEstimate: &models.ImpactEstimate{
				QualityTime: &models.QualityTimeEstimate{
					Attributes: &models.QualityTimeAttributes{
						Vulnerability: int32(api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH),
					},
				},
			},
		}
		got := buildHintFromStoredRequest(req)
		if got == nil {
			t.Fatal("expected non-nil hint, got nil")
		}
		// SocialContext is set, so VulnerabilityLevel from ImpactEstimate should not override.
		if got.SocialContext == nil {
			t.Fatal("expected SocialContext to be set")
		}
		if got.VulnerabilityLevel != "" {
			t.Errorf("expected VulnerabilityLevel to be empty when SocialContext is set, got %q", got.VulnerabilityLevel)
		}
	})

	t.Run("impact estimate with unspecified vulnerability returns nil", func(t *testing.T) {
		req := &models.Request{
			ImpactEstimate: &models.ImpactEstimate{
				QualityTime: &models.QualityTimeEstimate{
					Attributes: &models.QualityTimeAttributes{
						Vulnerability: int32(api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED),
					},
				},
			},
		}
		got := buildHintFromStoredRequest(req)
		if got != nil {
			t.Errorf("expected nil hint for unspecified vulnerability, got %+v", got)
		}
	})
}

// TestGetRequestStats_NoOffers verifies that a fresh request has zero offers,
// zero fulfillments, and zero days open (shared_at is set by the insert).
func TestGetRequestStats_NoOffers(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	resp, err := service.GetRequestStats(ctx, connect.NewRequest(&api.GetRequestStatsRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequestStats failed: %v", err)
	}

	if resp.Msg.OffersReceived != 0 {
		t.Errorf("expected 0 offers, got %d", resp.Msg.OffersReceived)
	}
	if resp.Msg.TimesFulfilled != 0 {
		t.Errorf("expected 0 fulfillments, got %d", resp.Msg.TimesFulfilled)
	}
}

// TestGetRequestStats_WithOffers verifies that offers_received counts non-withdrawn offers.
func TestGetRequestStats_WithOffers(t *testing.T) {
	service, testStorage, _, notifDone, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offerer1ID := setupTestUser(t, testStorage, "Offerer1", "offerer1@example.com")
	offerer2ID := setupTestUser(t, testStorage, "Offerer2", "offerer2@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offerer1ID, offerer2ID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	// Both offerers make offers.
	offerer1Ctx := createAuthenticatedContext(offerer1ID, "offerer1@example.com", models.Role_ROLE_USER)
	_, err = service.OfferToFulfill(offerer1Ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill (offerer1) failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	offerer2Ctx := createAuthenticatedContext(offerer2ID, "offerer2@example.com", models.Role_ROLE_USER)
	_, err = service.OfferToFulfill(offerer2Ctx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill (offerer2) failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	resp, err := service.GetRequestStats(requesterCtx, connect.NewRequest(&api.GetRequestStatsRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequestStats failed: %v", err)
	}

	if resp.Msg.OffersReceived != 2 {
		t.Errorf("expected 2 offers, got %d", resp.Msg.OffersReceived)
	}
	if resp.Msg.TimesFulfilled != 0 {
		t.Errorf("expected 0 fulfillments, got %d", resp.Msg.TimesFulfilled)
	}
}

// TestGetRequestStats_Fulfilled verifies that a fulfilled request has times_fulfilled=1
// and a non-nil impact estimate.
func TestGetRequestStats_Fulfilled(t *testing.T) {
	service, testStorage, _, notifDone, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	offererCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	_, err = service.OfferToFulfill(offererCtx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	_, err = service.MarkRequestFulfilled(requesterCtx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	resp, err := service.GetRequestStats(requesterCtx, connect.NewRequest(&api.GetRequestStatsRequest{
		RequestId: requestID,
	}))
	if err != nil {
		t.Fatalf("GetRequestStats failed: %v", err)
	}

	if resp.Msg.TimesFulfilled != 1 {
		t.Errorf("expected TimesFulfilled=1, got %d", resp.Msg.TimesFulfilled)
	}
	if resp.Msg.Impact == nil {
		t.Error("expected non-nil Impact for fulfilled request")
	}
	if resp.Msg.PotentialImpact == nil {
		t.Error("expected non-nil PotentialImpact for fulfilled request")
	}
}

// TestGetRequestStats_WithCommunityFilter verifies that passing community_id filters
// offers to those from that community.
func TestGetRequestStats_WithCommunityFilter(t *testing.T) {
	service, testStorage, _, notifDone, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	offererCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)

	createResp, err := service.SubmitRequest(requesterCtx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, requesterCtx, service, createResp.Msg.RequestId, communityID)
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId

	_, err = service.OfferToFulfill(offererCtx, connect.NewRequest(&api.OfferToFulfillRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("OfferToFulfill failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	// Query with community filter — should still see the offer.
	resp, err := service.GetRequestStats(requesterCtx, connect.NewRequest(&api.GetRequestStatsRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("GetRequestStats with community filter failed: %v", err)
	}

	if resp.Msg.OffersReceived != 1 {
		t.Errorf("expected 1 offer with community filter, got %d", resp.Msg.OffersReceived)
	}

	// Query with a non-existent community ID — Phase 4 of #1621 surfaces
	// this as NotFound at the entry-point gate rather than silently returning
	// 0 offers, so stale or fabricated community IDs surface as client bugs.
	_, err = service.GetRequestStats(requesterCtx, connect.NewRequest(&api.GetRequestStatsRequest{
		RequestId:   requestID,
		CommunityId: "non-existent-community",
	}))
	if err == nil {
		t.Fatalf("expected NotFound for non-existent community, got nil")
	}
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("expected NotFound, got %s", got)
	}
}

// TestGetRequestStats_NotFound verifies that a non-existent request returns NotFound.
func TestGetRequestStats_NotFound(t *testing.T) {
	service, _, _, _, _ := setupTestServiceWithNotifications(t)

	ctx := createAuthenticatedContext("user-id", "user@example.com", models.Role_ROLE_USER)

	_, err := service.GetRequestStats(ctx, connect.NewRequest(&api.GetRequestStatsRequest{
		RequestId: "non-existent-request",
	}))
	if err == nil {
		t.Fatal("expected NotFound error for non-existent request, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

func TestGetRequestStats_HelpValueUSD(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	userID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	ctx := createAuthenticatedContext(userID, "owner@example.com", models.Role_ROLE_USER)

	t.Run("help_value_usd is zero when request is not fulfilled", func(t *testing.T) {
		request := &models.Request{
			RequesterId: userID,
			Title:       "Need a ladder",
			Description: "short term",
			State:       models.RequestState_REQUEST_STATE_ACTIVE,
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 50.0,
			},
		}
		reqID, err := testStorage.Insert(ctx, request)
		if err != nil {
			t.Fatalf("failed to insert request: %v", err)
		}

		resp, err := service.GetRequestStats(ctx, connect.NewRequest(&api.GetRequestStatsRequest{RequestId: reqID}))
		if err != nil {
			t.Fatalf("GetRequestStats failed: %v", err)
		}

		if resp.Msg.HelpValueUsd != 0 {
			t.Errorf("expected HelpValueUsd=0 for unfulfilled request, got %f", resp.Msg.HelpValueUsd)
		}
		if resp.Msg.TimesFulfilled != 0 {
			t.Errorf("expected TimesFulfilled=0, got %d", resp.Msg.TimesFulfilled)
		}
	})

	t.Run("help_value_usd comes from persisted ImpactEstimate when request is fulfilled", func(t *testing.T) {
		const storedMoneyMean = float32(42.50)

		request := &models.Request{
			RequesterId: userID,
			Title:       "Need a drill",
			Description: "weekend project",
			State:       models.RequestState_REQUEST_STATE_FULFILLED,
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 100.0,
			},
			ImpactEstimate: &models.ImpactEstimate{
				MoneySaved: &models.MoneySavings{
					ValueUsd: &models.Estimate{Mean: storedMoneyMean},
				},
			},
		}
		reqID, err := testStorage.Insert(ctx, request)
		if err != nil {
			t.Fatalf("failed to insert request: %v", err)
		}

		resp, err := service.GetRequestStats(ctx, connect.NewRequest(&api.GetRequestStatsRequest{RequestId: reqID}))
		if err != nil {
			t.Fatalf("GetRequestStats failed: %v", err)
		}

		if resp.Msg.TimesFulfilled != 1 {
			t.Errorf("expected TimesFulfilled=1, got %d", resp.Msg.TimesFulfilled)
		}
		if resp.Msg.HelpValueUsd != storedMoneyMean {
			t.Errorf("expected HelpValueUsd=%f from ImpactEstimate, got %f", storedMoneyMean, resp.Msg.HelpValueUsd)
		}
	})

	t.Run("help_value_usd falls back to value×fulfilled for old records without ImpactEstimate", func(t *testing.T) {
		const estimatedValue = float32(80.0)

		request := &models.Request{
			RequesterId:    userID,
			Title:          "Need a saw",
			Description:    "cutting wood",
			State:          models.RequestState_REQUEST_STATE_FULFILLED,
			ImpactEstimate: nil, // no stored estimate (pre-Phase-4 record)
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: estimatedValue,
			},
		}
		reqID, err := testStorage.Insert(ctx, request)
		if err != nil {
			t.Fatalf("failed to insert request: %v", err)
		}

		// Remove estimator so BuildRequestImpactMetrics is not called, keeping totalImpact nil.
		service.SetEstimatorConfig(nil)

		resp, err := service.GetRequestStats(ctx, connect.NewRequest(&api.GetRequestStatsRequest{RequestId: reqID}))
		if err != nil {
			t.Fatalf("GetRequestStats failed: %v", err)
		}

		if resp.Msg.TimesFulfilled != 1 {
			t.Errorf("expected TimesFulfilled=1, got %d", resp.Msg.TimesFulfilled)
		}
		// Fallback: estimatedValue × timesFulfilled = 80.0
		expected := estimatedValue * float32(resp.Msg.TimesFulfilled)
		if resp.Msg.HelpValueUsd != expected {
			t.Errorf("expected HelpValueUsd=%f (fallback), got %f", expected, resp.Msg.HelpValueUsd)
		}
	})
}
