package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impact_metrics "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// distinctInferenceProvider returns a provider whose social-attribute
// inference is far from the config defaults, so an estimate computed with it
// is distinguishable from one computed without.
func distinctInferenceProvider() ai.Provider {
	m := ai.NewMockProvider()
	m.InferSocialAttributesFunc = func(_ context.Context, _, _, _ string, _ float32) (*ai.SocialAttributeInference, error) {
		return &ai.SocialAttributeInference{
			DurationMinutes:    240,
			DurationConfidence: 0.9,
			VulnerabilityLevel: "high",
		}, nil
	}
	return m
}

// insertGearOfferWithImpact inserts a gear row plus an origin-linked
// RECIPIENT_SELECTED transfer carrying a persisted item-based impact estimate,
// returning the transfer id. Mirrors the rows a gear-backed offer (#2702)
// leaves behind once its item-based impact has been stamped.
func insertGearOfferWithImpact(t *testing.T, st *storage.ProtoSQLStorage, requestID, communityID, helperID, requesterID, gearName string, impact *models.ImpactEstimate) string {
	t.Helper()
	ctx := context.Background()

	gearID, err := st.Insert(ctx, &models.Gear{
		Name:    gearName,
		OwnerId: helperID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	transferID, err := st.Insert(ctx, &models.Transfer{
		GearId:         gearID,
		OwnerId:        helperID,
		RecipientId:    requesterID,
		TransferType:   models.TransferType_TRANSFER_TYPE_LOAN,
		State:          models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		CommunityId:    communityID,
		Origin:         &models.Transfer_OriginRequestId{OriginRequestId: requestID},
		ImpactEstimate: impact,
	})
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}
	return transferID
}

// impactMeans extracts the four displayed dimension means from an estimate,
// returning 0 for absent dimensions: money USD, emissions grams CO2e
// (manufacture + waste), time-saved minutes, and quality-time minutes.
func impactMeans(ie *api.ImpactEstimate) (money, emissions, timeMin, qt float32) {
	if ie == nil {
		return 0, 0, 0, 0
	}
	money = ie.GetMoneySaved().GetValueUsd().GetMean()
	emissions = ie.GetEmissionsPrevented().GetManufactureAvoidedCarbon().GetCo2EGrams().GetMean() +
		ie.GetEmissionsPrevented().GetWasteReducedCarbon().GetCo2EGrams().GetMean()
	timeMin = ie.GetTimeSaved().GetMinutes().GetMean()
	qt = ie.GetQualityTime().GetQualityTimeMinutes().GetMean()
	return money, emissions, timeMin, qt
}

// assertImpactMeansEqual fails the test when the four dimension means of two
// estimates are not exactly equal. Preview and commit run the same computation
// on the same inputs (#2724), so the values must match bit-for-bit — any
// tolerance here would hide a real divergence.
func assertImpactMeansEqual(t *testing.T, label string, want, got *api.ImpactEstimate) {
	t.Helper()
	wantMoney, wantEmissions, wantTime, wantQT := impactMeans(want)
	gotMoney, gotEmissions, gotTime, gotQT := impactMeans(got)
	if wantMoney != gotMoney {
		t.Errorf("%s: MoneySaved mean = %v, want %v", label, gotMoney, wantMoney)
	}
	if wantEmissions != gotEmissions {
		t.Errorf("%s: EmissionsPrevented mean = %v, want %v", label, gotEmissions, wantEmissions)
	}
	if wantTime != gotTime {
		t.Errorf("%s: TimeSaved mean = %v, want %v", label, gotTime, wantTime)
	}
	if wantQT != gotQT {
		t.Errorf("%s: QualityTime mean = %v, want %v", label, gotQT, wantQT)
	}
}

// TestPreviewRequestImpact_GearBackedParityWithFulfillment is the #2724
// parity guard for the gear-backed path: on a $0-value request with two
// deliverable gear offers, the numbers PreviewRequestImpact shows for the
// confirmed helper set must be exactly the numbers MarkRequestFulfilled
// returns AND persists — money/emissions/time rolled up from the transfers'
// item-based estimates, Quality Time rebuilt for the confirmed group size.
func TestPreviewRequestImpact_GearBackedParityWithFulfillment(t *testing.T) {
	service, testStorage, _, notifDone, _ := setupTestServiceWithNotifications(t)
	// A live provider whose inference differs from the config defaults: the
	// preview cannot afford an LLM call per toggle, so any inference on the
	// commit side alone would break the parity this test asserts. Leaving the
	// provider nil would mask that.
	service.aiProvider = distinctInferenceProvider()

	requesterID := setupTestUser(t, testStorage, "Requester", "requester-gear-parity@example.com")
	helperAID := setupTestUser(t, testStorage, "Helper A", "helper-a-gear-parity@example.com")
	helperBID := setupTestUser(t, testStorage, "Helper B", "helper-b-gear-parity@example.com")
	// setupRequestWithCommunity leaves ValueEstimate nil — the $0-value case
	// where only the gear roll-up can produce non-zero material dimensions.
	requestID, communityID := setupRequestWithCommunity(t, testStorage, requesterID, helperAID, helperBID)

	insertGearOfferWithImpact(t, testStorage, requestID, communityID, helperAID, requesterID, "Ladder", &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{ValueUsd: &models.Estimate{Mean: 25}},
		EmissionsPrevented: &models.PreventedEmissions{
			ManufactureAvoidedCarbon: &models.CarbonEstimate{Co2EGrams: &models.Estimate{Mean: 8000}},
			WasteReducedCarbon:       &models.CarbonEstimate{Co2EGrams: &models.Estimate{Mean: 2000}},
		},
		TimeSaved: &models.TimeSavings{Minutes: &models.Estimate{Mean: 30}},
	})
	insertGearOfferWithImpact(t, testStorage, requestID, communityID, helperBID, requesterID, "Drill", &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{ValueUsd: &models.Estimate{Mean: 15}},
		EmissionsPrevented: &models.PreventedEmissions{
			ManufactureAvoidedCarbon: &models.CarbonEstimate{Co2EGrams: &models.Estimate{Mean: 4000}},
			WasteReducedCarbon:       &models.CarbonEstimate{Co2EGrams: &models.Estimate{Mean: 1000}},
		},
		TimeSaved: &models.TimeSavings{Minutes: &models.Estimate{Mean: 15}},
	})

	requesterCtx := createAuthenticatedContext(requesterID, "requester-gear-parity@example.com", models.Role_ROLE_USER)

	previewResp, err := service.PreviewRequestImpact(requesterCtx, connect.NewRequest(&api.PreviewRequestImpactRequest{
		RequestId:          requestID,
		ConfirmedHelperIds: []string{helperAID, helperBID},
	}))
	if err != nil {
		t.Fatalf("PreviewRequestImpact failed: %v", err)
	}
	preview := previewResp.Msg.Impact
	if preview == nil {
		t.Fatal("preview impact is nil")
	}

	// The previewed material dimensions must be the sum of the two children's
	// item-based estimates — a value-based default on this $0 request would be 0.
	previewMoney, previewEmissions, previewTime, previewQT := impactMeans(preview)
	if previewMoney != 40 {
		t.Errorf("preview MoneySaved mean = %v, want 40 (25+15 gear roll-up)", previewMoney)
	}
	if previewEmissions != 15000 {
		t.Errorf("preview EmissionsPrevented mean = %v, want 15000 (gear roll-up)", previewEmissions)
	}
	if previewTime != 45 {
		t.Errorf("preview TimeSaved mean = %v, want 45 (30+15 gear roll-up)", previewTime)
	}
	if previewQT <= 0 {
		t.Errorf("preview QualityTime mean = %v, want > 0", previewQT)
	}

	fulfillResp, err := service.MarkRequestFulfilled(requesterCtx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId:          requestID,
		ConfirmedHelperIds: []string{helperAID, helperBID},
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	assertImpactMeansEqual(t, "MarkRequestFulfilled response vs preview", preview, fulfillResp.Msg.Impact)

	stored := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, stored); err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	assertImpactMeansEqual(t, "persisted estimate vs preview", preview, impact_metrics.ModelsImpactToAPI(stored.ImpactEstimate))
	if stored.GetImpactAdoptedFromTransferId() == "" {
		t.Error("expected ImpactAdoptedFromTransferId to be set on a gear-backed fulfillment")
	}
}

// TestPreviewRequestImpact_ValueBasedParityWithFulfillment is the #2724
// parity guard for the value-based path: with no gear offers, the previewed
// estimate for a $50 request with one confirmed helper must exactly equal the
// MarkRequestFulfilled response and the persisted estimate on all four
// dimensions.
func TestPreviewRequestImpact_ValueBasedParityWithFulfillment(t *testing.T) {
	service, testStorage, _, notifDone, _ := setupTestServiceWithNotifications(t)
	service.aiProvider = nil

	requesterID := setupTestUser(t, testStorage, "Requester", "requester-value-parity@example.com")
	helperID := setupTestUser(t, testStorage, "Helper", "helper-value-parity@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, helperID)

	ctx := context.Background()
	requestID, err := testStorage.Insert(ctx, &models.Request{
		RequesterId:   requesterID,
		Title:         "Need help assembling a shed",
		State:         models.RequestState_REQUEST_STATE_OFFERS_RECEIVED,
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 50},
	})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := testStorage.Insert(ctx, &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: communityID,
		Archived:    false,
	}); err != nil {
		t.Fatalf("insert community request: %v", err)
	}

	requesterCtx := createAuthenticatedContext(requesterID, "requester-value-parity@example.com", models.Role_ROLE_USER)

	previewResp, err := service.PreviewRequestImpact(requesterCtx, connect.NewRequest(&api.PreviewRequestImpactRequest{
		RequestId:          requestID,
		ConfirmedHelperIds: []string{helperID},
	}))
	if err != nil {
		t.Fatalf("PreviewRequestImpact failed: %v", err)
	}
	preview := previewResp.Msg.Impact
	previewMoney, _, _, previewQT := impactMeans(preview)
	if previewMoney <= 0 {
		t.Errorf("preview MoneySaved mean = %v, want > 0 from the $50 value estimate", previewMoney)
	}
	if previewQT <= 0 {
		t.Errorf("preview QualityTime mean = %v, want > 0", previewQT)
	}

	fulfillResp, err := service.MarkRequestFulfilled(requesterCtx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId:          requestID,
		ConfirmedHelperIds: []string{helperID},
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled failed: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	assertImpactMeansEqual(t, "MarkRequestFulfilled response vs preview", preview, fulfillResp.Msg.Impact)

	stored := &models.Request{}
	if err := testStorage.GetByID(ctx, requestID, stored); err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	assertImpactMeansEqual(t, "persisted estimate vs preview", preview, impact_metrics.ModelsImpactToAPI(stored.ImpactEstimate))
	if stored.GetImpactAdoptedFromTransferId() != "" {
		t.Errorf("expected no adopted-transfer marker on the value-based path, got %q", stored.GetImpactAdoptedFromTransferId())
	}
}

// TestPreviewRequestImpact_GroupSizeScalesQualityTime sanity-checks that the
// preview's Quality Time is rebuilt live for the confirmed group size: five
// confirmed helpers must yield strictly more QT than one.
func TestPreviewRequestImpact_GroupSizeScalesQualityTime(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester-groupsize@example.com")
	requestID, _ := setupRequestWithCommunity(t, testStorage, requesterID)

	requesterCtx := createAuthenticatedContext(requesterID, "requester-groupsize@example.com", models.Role_ROLE_USER)
	previewQT := func(confirmedHelperCount int32) float32 {
		resp, err := service.PreviewRequestImpact(requesterCtx, connect.NewRequest(&api.PreviewRequestImpactRequest{
			RequestId:            requestID,
			ConfirmedHelperCount: confirmedHelperCount,
		}))
		if err != nil {
			t.Fatalf("PreviewRequestImpact(%d helpers) failed: %v", confirmedHelperCount, err)
		}
		return resp.Msg.Impact.GetQualityTime().GetQualityTimeMinutes().GetMean()
	}

	qt1 := previewQT(1)
	qt5 := previewQT(5)
	if qt1 <= 0 {
		t.Fatalf("QT with 1 confirmed helper = %v, want > 0", qt1)
	}
	if qt5 <= qt1 {
		t.Errorf("QT with 5 confirmed helpers = %v, want > QT with 1 helper = %v", qt5, qt1)
	}
}

// TestReadoptImpactFromHandoffs_KeepsFulfillmentQualityTime guards the number
// the requester was shown at commit. Re-adoption runs on the bus once per
// delivered gear child, with that child's community for context — so if it
// recomputes Quality Time it silently replaces the fulfillment's value moments
// after the requester read it, with no user action. It may only re-adopt the
// children's money/emissions/time.
func TestReadoptImpactFromHandoffs_KeepsFulfillmentQualityTime(t *testing.T) {
	service, testStorage, _, notifDone, _ := setupTestServiceWithNotifications(t)
	service.aiProvider = distinctInferenceProvider()

	requesterID := setupTestUser(t, testStorage, "Requester", "requester-readopt@example.com")
	helperAID := setupTestUser(t, testStorage, "Helper A", "helper-a-readopt@example.com")
	helperBID := setupTestUser(t, testStorage, "Helper B", "helper-b-readopt@example.com")
	requestID, communityID := setupRequestWithCommunity(t, testStorage, requesterID, helperAID, helperBID)

	transferID := insertGearOfferWithImpact(t, testStorage, requestID, communityID, helperAID, requesterID, "Ladder", &models.ImpactEstimate{
		MoneySaved: &models.MoneySavings{ValueUsd: &models.Estimate{Mean: 25}},
		TimeSaved:  &models.TimeSavings{Minutes: &models.Estimate{Mean: 30}},
	})

	requesterCtx := createAuthenticatedContext(requesterID, "requester-readopt@example.com", models.Role_ROLE_USER)
	fulfillResp, err := service.MarkRequestFulfilled(requesterCtx, connect.NewRequest(&api.MarkRequestFulfilledRequest{
		RequestId:          requestID,
		ConfirmedHelperIds: []string{helperAID, helperBID},
	}))
	if err != nil {
		t.Fatalf("MarkRequestFulfilled failed: %v", err)
	}
	committedQT := fulfillResp.Msg.Impact.GetQualityTime().GetQualityTimeMinutes().GetMean()
	if committedQT <= 0 {
		t.Fatalf("committed QualityTime = %v, want > 0", committedQT)
	}

	// Pin the committed Quality Time to a sentinel: re-adoption must carry
	// whatever the fulfillment persisted straight through, whatever context it
	// happens to run in.
	stored := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, stored); err != nil {
		t.Fatalf("get request: %v", err)
	}
	const sentinelQT float32 = 999
	stored.ImpactEstimate.QualityTime.QualityTimeMinutes = &models.Estimate{Mean: sentinelQT}
	if err := testStorage.Update(context.Background(), stored); err != nil {
		t.Fatalf("pin committed QT: %v", err)
	}

	// The delivered child lands on the bus afterwards and drives re-adoption.
	transfer := &models.Transfer{}
	if err := testStorage.GetByID(context.Background(), transferID, transfer); err != nil {
		t.Fatalf("get transfer: %v", err)
	}
	transfer.State = models.TransferState_TRANSFER_STATE_COMPLETED
	if err := testStorage.Update(context.Background(), transfer); err != nil {
		t.Fatalf("update transfer: %v", err)
	}
	service.autoFulfillFromTransfer(context.Background(), transfer)

	readopted := &models.Request{}
	if err := testStorage.GetByID(context.Background(), requestID, readopted); err != nil {
		t.Fatalf("get request: %v", err)
	}
	ie := impact_metrics.ModelsImpactToAPI(readopted.ImpactEstimate)
	if got := ie.GetQualityTime().GetQualityTimeMinutes().GetMean(); got != sentinelQT {
		t.Errorf("QualityTime after re-adoption = %v, want %v (the value the fulfillment committed and the requester read)", got, sentinelQT)
	}
	// It must still do its actual job: adopt the child's money.
	if got := ie.GetMoneySaved().GetValueUsd().GetMean(); got != 25 {
		t.Errorf("MoneySaved after re-adoption = %v, want 25 (the child's item-based value)", got)
	}
	<-notifDone
}
