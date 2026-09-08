package experience

import (
	"context"
	"math"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impact_metrics "go.ripls.org/ripls/server/impact_metrics"
)

// TestPreviewExperienceImpact_ReturnsRealEstimate verifies that PreviewExperienceImpact
// returns a non-empty impact estimate for a valid experience and attendee count.
func TestPreviewExperienceImpact_ReturnsRealEstimate(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "owner1", "owner1@example.com", "Owner One")
	createTestUser(t, testStorage, "att1", "att1@example.com", "Attendee One")
	ctx := createAuthenticatedContext("owner1", "owner1@example.com", models.Role_ROLE_USER)

	// Create experience.
	createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Preview Test Experience",
		Description: "Testing the preview RPC",
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1800000000,
					DurationMinutes:  60,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Preview with 3 confirmed attendees (including provisional users via count).
	previewResp, err := service.PreviewExperienceImpact(ctx, connect.NewRequest(&api.PreviewExperienceImpactRequest{
		ExperienceId:           expID,
		ConfirmedAttendeeIds:   []string{"owner1", "att1"},
		ConfirmedAttendeeCount: 3,
	}))
	if err != nil {
		t.Fatalf("PreviewExperienceImpact failed: %v", err)
	}

	if previewResp.Msg.Impact == nil {
		t.Fatal("expected non-nil impact in preview response")
	}
	if previewResp.Msg.Impact.QualityTime == nil {
		t.Fatal("expected quality_time to be populated in preview response")
	}
	if previewResp.Msg.Impact.QualityTime.QualityTimeMinutes == nil {
		t.Fatal("expected quality_time_minutes to be set")
	}
	if previewResp.Msg.Impact.QualityTime.QualityTimeMinutes.Mean <= 0 {
		t.Errorf("expected positive quality_time_minutes, got %.2f",
			previewResp.Msg.Impact.QualityTime.QualityTimeMinutes.Mean)
	}
}

// TestPreviewExperienceImpact_GroupSizeScalesQT verifies that a larger confirmed
// attendee count produces a higher QT estimate than a smaller one.
func TestPreviewExperienceImpact_GroupSizeScalesQT(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "host2", "host2@example.com", "Host Two")
	ctx := createAuthenticatedContext("host2", "host2@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Scaling Group Test",
		Description: "Group size scaling",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	previewQT := func(count int32) float32 {
		resp, err := service.PreviewExperienceImpact(ctx, connect.NewRequest(&api.PreviewExperienceImpactRequest{
			ExperienceId:           expID,
			ConfirmedAttendeeCount: count,
		}))
		if err != nil {
			t.Fatalf("PreviewExperienceImpact(%d) failed: %v", count, err)
		}
		if resp.Msg.Impact == nil || resp.Msg.Impact.QualityTime == nil {
			return 0
		}
		qt := resp.Msg.Impact.QualityTime
		if qt.QualityTimeMinutes == nil {
			return 0
		}
		return qt.QualityTimeMinutes.Mean
	}

	qt2 := previewQT(2)
	qt5 := previewQT(5)

	if qt5 <= qt2 {
		t.Errorf("expected QT(5 people)=%.2f > QT(2 people)=%.2f", qt5, qt2)
	}
}

// TestPreviewExperienceImpact_UserDurationReflected verifies that the preview
// uses the user-set duration from the experience time field.
func TestPreviewExperienceImpact_UserDurationReflected(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "host3", "host3@example.com", "Host Three")
	ctx := createAuthenticatedContext("host3", "host3@example.com", models.Role_ROLE_USER)

	// Create experience with 120-minute duration.
	createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Duration Reflected Test",
		Description: "Testing that preview uses user-set duration",
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1800000000,
					DurationMinutes:  120,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	previewResp, err := service.PreviewExperienceImpact(ctx, connect.NewRequest(&api.PreviewExperienceImpactRequest{
		ExperienceId:           expID,
		ConfirmedAttendeeCount: 2,
	}))
	if err != nil {
		t.Fatalf("PreviewExperienceImpact failed: %v", err)
	}

	if previewResp.Msg.Impact == nil || previewResp.Msg.Impact.QualityTime == nil ||
		previewResp.Msg.Impact.QualityTime.Attributes == nil {
		t.Fatal("expected impact with quality_time attributes in preview response")
	}

	duration := previewResp.Msg.Impact.QualityTime.Attributes.EstimatedDurationMinutes
	if duration < 110 || duration > 130 {
		t.Errorf("expected estimated_duration_minutes ≈ 120, got %.1f", duration)
	}
}

// TestPreviewExperienceImpact_ConsistentWithStoredOnCompletion verifies that the
// QT value from PreviewExperienceImpact (with N confirmed attendees) matches the
// QT stored on the experience after CompleteExperience with the same N attendees.
// Both paths use BuildExperienceImpactMetrics with the same inputs, so their
// quality_time_minutes means should agree within 5%.
func TestPreviewExperienceImpact_ConsistentWithStoredOnCompletion(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "owner-cs", "owner-cs@example.com", "Owner CS")
	createTestUser(t, testStorage, "att-cs1", "att-cs1@example.com", "Attendee CS1")
	createTestUser(t, testStorage, "att-cs2", "att-cs2@example.com", "Attendee CS2")

	ownerCtx := createAuthenticatedContext("owner-cs", "owner-cs@example.com", models.Role_ROLE_USER)

	// Use a future start time so the experience starts in ACTIVE state.
	futureTime := time.Now().Add(24 * time.Hour).Unix()
	communityID := createTestCommunity(t, testStorage, "Consistency Test Community", "owner-cs")
	createTestCommunityMembership(t, testStorage, communityID, "owner-cs")

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Consistency Test Hike",
		Description: "Cross-surface QT consistency check",
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: futureTime,
					DurationMinutes:  90,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	// The experience is born in its own per-item community (#2492), where the
	// owner is auto-RSVP'd. Record attendance there — the owner has no RSVP in
	// the unrelated test community above.
	itemCommunityID := createResp.Msg.GetItemCommunityId()
	if itemCommunityID == "" {
		t.Fatal("expected SaveExperience to return ItemCommunityId")
	}

	// Preview with 3 confirmed attendees before completion.
	previewResp, err := service.PreviewExperienceImpact(ownerCtx, connect.NewRequest(&api.PreviewExperienceImpactRequest{
		ExperienceId:           expID,
		ConfirmedAttendeeIds:   []string{"owner-cs", "att-cs1", "att-cs2"},
		ConfirmedAttendeeCount: 3,
	}))
	if err != nil {
		t.Fatalf("PreviewExperienceImpact failed: %v", err)
	}
	if previewResp.Msg.Impact == nil || previewResp.Msg.Impact.QualityTime == nil {
		t.Fatal("preview response missing quality_time")
	}
	previewQT := previewResp.Msg.Impact.QualityTime.QualityTimeMinutes.Mean

	// Record attendance for the same 3 users, then complete.
	_, err = service.MarkExperienceInProcess(ownerCtx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}
	_, err = service.RecordAttendance(ownerCtx, connect.NewRequest(&api.RecordAttendanceRequest{
		ExperienceId: expID,
		CommunityId:  itemCommunityID,
		Attendance: []*api.AttendanceRecord{
			{UserId: "owner-cs", Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
			{UserId: "att-cs1", Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
			{UserId: "att-cs2", Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
		},
	}))
	if err != nil {
		t.Fatalf("RecordAttendance failed: %v", err)
	}
	completeResp, err := service.CompleteExperience(ownerCtx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}
	if completeResp.Msg.Impact == nil || completeResp.Msg.Impact.QualityTime == nil {
		t.Fatal("complete response missing quality_time")
	}
	storedQT := completeResp.Msg.Impact.QualityTime.QualityTimeMinutes.Mean

	// Both values must be positive and agree within 5%.
	if previewQT <= 0 {
		t.Fatalf("preview QT must be positive, got %.2f", previewQT)
	}
	if storedQT <= 0 {
		t.Fatalf("stored QT must be positive, got %.2f", storedQT)
	}
	diff := math.Abs(float64(previewQT-storedQT)) / float64(storedQT)
	if diff > 0.05 {
		t.Errorf("preview QT (%.2f) and stored QT (%.2f) differ by %.1f%% — expected ≤5%%",
			previewQT, storedQT, diff*100)
	}
}

// experienceImpactMeans extracts the four displayed dimension means from an
// estimate, returning 0 for absent dimensions: money USD, emissions grams
// CO2e (manufacture + waste), time-saved minutes, and quality-time minutes.
func experienceImpactMeans(ie *api.ImpactEstimate) (money, emissions, timeMin, qt float32) {
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

// TestPreviewExperienceImpact_HostWithoutRSVP_ParityWithCompletion is the
// #2724 parity guard for the host-not-in-RSVP-rows scenario: the host has
// no RSVP row, five guests RSVP YES, and the completion modal confirms the
// five guests plus the host via confirmed_attendee_count=6. Completion must
// derive the group size from the confirmed set — exactly as the preview does —
// not from the attended-YES RSVP rows (which count only the 5 guests), so the
// previewed Quality Time equals the persisted one bit-for-bit.
func TestPreviewExperienceImpact_HostWithoutRSVP_ParityWithCompletion(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ctx := context.Background()

	createTestUser(t, testStorage, "host-parity", "host-parity@example.com", "Host Parity")
	guests := []string{"guest-p1", "guest-p2", "guest-p3", "guest-p4", "guest-p5"}
	for i, g := range guests {
		createTestUser(t, testStorage, g, g+"@example.com", "Guest "+string(rune('1'+i)))
	}
	hostCtx := createAuthenticatedContext("host-parity", "host-parity@example.com", models.Role_ROLE_USER)

	// Future start time keeps the experience ACTIVE; the user-set duration
	// makes buildHintFromStoredEstimate return the same hint on both paths.
	futureTime := time.Now().Add(24 * time.Hour).Unix()
	createResp, err := service.SaveExperience(hostCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Parity Potluck",
		Description: "Host completes without an RSVP row of their own",
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: futureTime,
					DurationMinutes:  90,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id
	itemCommunityID := createResp.Msg.GetItemCommunityId()
	if itemCommunityID == "" {
		t.Fatal("expected SaveExperience to return ItemCommunityId")
	}

	// Strip the host's auto-RSVP (#2492 creates one in the item community) so
	// the host is represented only through the confirmed count.
	rsvps, err := testStorage.QueryByField(ctx, "experience_id", expID, &models.ExperienceRSVP{})
	if err != nil {
		t.Fatalf("query RSVPs: %v", err)
	}
	for _, r := range rsvps {
		rsvp := r.(*models.ExperienceRSVP)
		if rsvp.UserId == "host-parity" {
			if err := testStorage.Delete(ctx, rsvp); err != nil {
				t.Fatalf("delete host RSVP: %v", err)
			}
		}
	}

	// Five guests RSVP YES; completion auto-marks them attended, so the
	// legacy attended-YES derivation would see a group of 5, not 6.
	for _, g := range guests {
		if _, err := testStorage.Insert(ctx, &models.ExperienceRSVP{
			ExperienceId: expID,
			UserId:       g,
			CommunityId:  itemCommunityID,
			Intention:    models.RSVPIntention_RSVP_INTENTION_YES,
		}); err != nil {
			t.Fatalf("insert guest RSVP: %v", err)
		}
	}

	previewResp, err := service.PreviewExperienceImpact(hostCtx, connect.NewRequest(&api.PreviewExperienceImpactRequest{
		ExperienceId:           expID,
		ConfirmedAttendeeIds:   guests,
		ConfirmedAttendeeCount: 6,
	}))
	if err != nil {
		t.Fatalf("PreviewExperienceImpact failed: %v", err)
	}
	previewQT := previewResp.Msg.Impact.GetQualityTime().GetQualityTimeMinutes().GetMean()
	if previewQT <= 0 {
		t.Fatalf("preview QT = %v, want > 0", previewQT)
	}

	completeResp, err := service.CompleteExperience(hostCtx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId:           expID,
		ConfirmedAttendeeIds:   guests,
		ConfirmedAttendeeCount: 6,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}
	completedQT := completeResp.Msg.Impact.GetQualityTime().GetQualityTimeMinutes().GetMean()

	expStored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, expStored); err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	storedIE := impact_metrics.ModelsImpactToAPI(expStored.ImpactEstimate)
	storedQT := storedIE.GetQualityTime().GetQualityTimeMinutes().GetMean()

	// Same computation, same inputs: preview, response, and persisted QT must
	// be exactly equal — a tolerance would hide the group-size divergence.
	if completedQT != previewQT {
		t.Errorf("CompleteExperience response QT = %v, want preview QT %v", completedQT, previewQT)
	}
	if storedQT != previewQT {
		t.Errorf("persisted QT = %v, want preview QT %v", storedQT, previewQT)
	}
	// The confirmed set (host included) must win over the 5 attended-YES rows.
	if gs := storedIE.GetQualityTime().GetAttributes().GetGroupSize(); gs != 6 {
		t.Errorf("persisted QT group size = %d, want 6 (confirmed attendees incl. host)", gs)
	}
}

// TestExperienceChildTransferRollup_PreviewCompletionAndMasking covers the
// #2724 event child-transfer roll-up: a gear-backed child transfer carrying a
// stored item-based impact must (a) surface its money/emissions/time in
// PreviewExperienceImpact, (b) persist identically at CompleteExperience with
// the adopted-from marker set, and (c) be masked out of the experience's
// aggregate contribution (money zeroed, Quality Time kept) so the transfer is
// counted exactly once.
func TestExperienceChildTransferRollup_PreviewCompletionAndMasking(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	ctx := context.Background()

	createTestUser(t, testStorage, "host-rollup", "host-rollup@example.com", "Host Rollup")
	createTestUser(t, testStorage, "helper-rollup", "helper-rollup@example.com", "Helper Rollup")
	hostCtx := createAuthenticatedContext("host-rollup", "host-rollup@example.com", models.Role_ROLE_USER)

	futureTime := time.Now().Add(24 * time.Hour).Unix()
	createResp, err := service.SaveExperience(hostCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Yard Day Rollup",
		Description: "Folding table changes hands at the event",
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: futureTime,
					DurationMinutes:  60,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Seed a child transfer with a stored item-based impact, the way
	// OfferExperienceTransfer + the impact stamp would (#2708).
	transferID, err := testStorage.Insert(ctx, &models.Transfer{
		GearId:       "gear-rollup",
		OwnerId:      "helper-rollup",
		RecipientId:  "host-rollup",
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:        models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		Origin:       &models.Transfer_OriginExperienceId{OriginExperienceId: expID},
		ImpactEstimate: &models.ImpactEstimate{
			MoneySaved: &models.MoneySavings{ValueUsd: &models.Estimate{Mean: 40}},
			EmissionsPrevented: &models.PreventedEmissions{
				ManufactureAvoidedCarbon: &models.CarbonEstimate{Co2EGrams: &models.Estimate{Mean: 5000}},
				WasteReducedCarbon:       &models.CarbonEstimate{Co2EGrams: &models.Estimate{Mean: 1500}},
			},
			TimeSaved: &models.TimeSavings{Minutes: &models.Estimate{Mean: 20}},
		},
	})
	if err != nil {
		t.Fatalf("insert child transfer: %v", err)
	}

	previewResp, err := service.PreviewExperienceImpact(hostCtx, connect.NewRequest(&api.PreviewExperienceImpactRequest{
		ExperienceId:           expID,
		ConfirmedAttendeeCount: 2,
	}))
	if err != nil {
		t.Fatalf("PreviewExperienceImpact failed: %v", err)
	}
	preview := previewResp.Msg.Impact
	previewMoney, previewEmissions, previewTime, previewQT := experienceImpactMeans(preview)
	if previewMoney != 40 {
		t.Errorf("preview MoneySaved mean = %v, want 40 (child transfer roll-up)", previewMoney)
	}
	if previewEmissions != 6500 {
		t.Errorf("preview EmissionsPrevented mean = %v, want 6500 (child transfer roll-up)", previewEmissions)
	}
	if previewTime != 20 {
		t.Errorf("preview TimeSaved mean = %v, want 20 (child transfer roll-up)", previewTime)
	}
	if previewQT <= 0 {
		t.Errorf("preview QualityTime mean = %v, want > 0 (experience's own computation)", previewQT)
	}

	completeResp, err := service.CompleteExperience(hostCtx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId:           expID,
		ConfirmedAttendeeCount: 2,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}
	completedMoney, completedEmissions, completedTime, completedQT := experienceImpactMeans(completeResp.Msg.Impact)
	if completedMoney != previewMoney || completedEmissions != previewEmissions ||
		completedTime != previewTime || completedQT != previewQT {
		t.Errorf("CompleteExperience impact (money=%v emissions=%v time=%v qt=%v) != preview (money=%v emissions=%v time=%v qt=%v)",
			completedMoney, completedEmissions, completedTime, completedQT,
			previewMoney, previewEmissions, previewTime, previewQT)
	}

	expStored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, expStored); err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if expStored.GetImpactAdoptedFromTransferId() != transferID {
		t.Errorf("ImpactAdoptedFromTransferId = %q, want %q", expStored.GetImpactAdoptedFromTransferId(), transferID)
	}
	storedMoney, storedEmissions, storedTime, storedQT := experienceImpactMeans(impact_metrics.ModelsImpactToAPI(expStored.ImpactEstimate))
	if storedMoney != 40 || storedEmissions != 6500 || storedTime != 20 || storedQT != previewQT {
		t.Errorf("persisted impact (money=%v emissions=%v time=%v qt=%v) != adopted roll-up (money=40 emissions=6500 time=20 qt=%v)",
			storedMoney, storedEmissions, storedTime, storedQT, previewQT)
	}

	// Aggregation masking: the adopted material dimensions are display copies —
	// the experience contributes 0 money/emissions to aggregates (the transfer
	// carries them), while Quality Time remains the experience's own.
	if v := impact_metrics.ExperienceDimensionValue(expStored, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY); v != 0 {
		t.Errorf("ExperienceDimensionValue(MONEY) = %v, want 0 on adopted experience", v)
	}
	if v := impact_metrics.ExperienceDimensionValue(expStored, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS); v != 0 {
		t.Errorf("ExperienceDimensionValue(EMISSIONS) = %v, want 0 on adopted experience", v)
	}
	if v := impact_metrics.ExperienceDimensionValue(expStored, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME); v <= 0 {
		t.Errorf("ExperienceDimensionValue(QUALITY_TIME) = %v, want > 0 on adopted experience", v)
	}
}
