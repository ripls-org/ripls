package experience

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
)

func TestService_MarkExperienceInProcess(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Lifecycle Test Experience",
		Description: "Test lifecycle transitions",
	})
	createResp, err := service.SaveExperience(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	t.Run("mark experience in process", func(t *testing.T) {
		markReq := connect.NewRequest(&api.MarkExperienceInProcessRequest{
			ExperienceId: expID,
		})

		_, err := service.MarkExperienceInProcess(ctx, markReq)
		if err != nil {
			t.Fatalf("MarkExperienceInProcess failed: %v", err)
		}

		// Verify state changed
		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, expID, stored)
		if err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}

		if stored.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
			t.Errorf("Expected state IN_PROCESS, got %v", stored.State)
		}
	})
}

func TestService_CompleteExperience(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create experience and mark in process
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Complete Test Experience",
	})
	createResp, err := service.SaveExperience(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	markReq := connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	})
	_, err = service.MarkExperienceInProcess(ctx, markReq)
	if err != nil {
		t.Fatalf("Failed to mark in process: %v", err)
	}

	t.Run("complete experience", func(t *testing.T) {
		completeReq := connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: expID,
		})

		_, err := service.CompleteExperience(ctx, completeReq)
		if err != nil {
			t.Fatalf("CompleteExperience failed: %v", err)
		}

		// Verify state changed
		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, expID, stored)
		if err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}

		if stored.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			t.Errorf("Expected state COMPLETED, got %v", stored.State)
		}

		// Verify ImpactEstimate was persisted at completion
		if stored.ImpactEstimate == nil {
			t.Error("Expected ImpactEstimate to be set after CompleteExperience")
		} else if stored.ImpactEstimate.TimeSaved == nil || stored.ImpactEstimate.TimeSaved.Minutes == nil {
			t.Error("Expected TimeSaved.Minutes to be set after CompleteExperience")
		}

		// Verify provenance on TimeSaved
		if ts := stored.ImpactEstimate.GetTimeSaved(); ts != nil {
			if ts.Provenance == nil {
				t.Error("Expected TimeSaved.Provenance to be set after CompleteExperience")
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
	})
}

func TestService_CompleteExperience_DirectFromActive(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create experience (starts in ACTIVE state) — do NOT call MarkInProcess.
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Direct Complete Test",
	})
	createResp, err := service.SaveExperience(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	t.Run("complete directly from ACTIVE", func(t *testing.T) {
		completeReq := connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: expID,
		})

		_, err := service.CompleteExperience(ctx, completeReq)
		if err != nil {
			t.Fatalf("CompleteExperience from ACTIVE failed: %v", err)
		}

		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, expID, stored)
		if err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}

		if stored.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			t.Errorf("Expected state COMPLETED, got %v", stored.State)
		}
		if stored.StartedAtUnixSec == nil {
			t.Error("Expected StartedAtUnixSec to be set on direct completion")
		}
		if stored.CompletedAtUnixSec == nil {
			t.Error("Expected CompletedAtUnixSec to be set")
		}
	})
}

func TestService_CancelExperience(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Cancel Test Experience",
	})
	createResp, err := service.SaveExperience(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	t.Run("cancel experience", func(t *testing.T) {
		cancelReq := connect.NewRequest(&api.CancelExperienceRequest{
			ExperienceId: expID,
		})

		_, err := service.CancelExperience(ctx, cancelReq)
		if err != nil {
			t.Fatalf("CancelExperience failed: %v", err)
		}

		// Verify state changed
		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, expID, stored)
		if err != nil {
			t.Fatalf("Failed to get experience: %v", err)
		}

		if stored.State != models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			t.Errorf("Expected state CANCELLED, got %v", stored.State)
		}
	})
}

func TestService_CompleteExperienceAutoMarkAttendance(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create users
	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner")
	createTestUser(t, testStorage, "user1", "user1@example.com", "User One")
	createTestUser(t, testStorage, "user2", "user2@example.com", "User Two")
	createTestUser(t, testStorage, "user3", "user3@example.com", "User Three")

	ownerCtx := createAuthenticatedContext("owner123", "owner@example.com", models.Role_ROLE_USER)

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Auto-Mark Test Experience",
		Description: "Test auto-marking attendance",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share with a community
	communityID := createTestCommunity(t, testStorage, "Test Community", "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "owner123")
	createTestCommunityMembership(t, testStorage, communityID, "user1")
	createTestCommunityMembership(t, testStorage, communityID, "user2")
	createTestCommunityMembership(t, testStorage, communityID, "user3")
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Create RSVPs with different intentions
	user1Ctx := createAuthenticatedContext("user1", "user1@example.com", models.Role_ROLE_USER)
	user2Ctx := createAuthenticatedContext("user2", "user2@example.com", models.Role_ROLE_USER)
	user3Ctx := createAuthenticatedContext("user3", "user3@example.com", models.Role_ROLE_USER)

	// User 1: YES
	rsvpReq1 := connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})
	_, err = service.RSVPToExperience(user1Ctx, rsvpReq1)
	if err != nil {
		t.Fatalf("Failed to RSVP user1: %v", err)
	}

	// User 2: YES
	rsvpReq2 := connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})
	_, err = service.RSVPToExperience(user2Ctx, rsvpReq2)
	if err != nil {
		t.Fatalf("Failed to RSVP user2: %v", err)
	}

	// User 3: MAYBE
	rsvpReq3 := connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_MAYBE,
	})
	_, err = service.RSVPToExperience(user3Ctx, rsvpReq3)
	if err != nil {
		t.Fatalf("Failed to RSVP user3: %v", err)
	}

	t.Run("auto-marks YES RSVPs when no attendance recorded", func(t *testing.T) {
		// Mark experience in process
		markReq := connect.NewRequest(&api.MarkExperienceInProcessRequest{
			ExperienceId: expID,
		})
		_, err := service.MarkExperienceInProcess(ownerCtx, markReq)
		if err != nil {
			t.Fatalf("Failed to mark in process: %v", err)
		}

		// Complete experience (should auto-mark YES RSVPs)
		completeReq := connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: expID,
		})
		_, err = service.CompleteExperience(ownerCtx, completeReq)
		if err != nil {
			t.Fatalf("CompleteExperience failed: %v", err)
		}

		// Verify YES RSVPs were auto-marked as attended. Query by experience_id
		// only (not community_id): the owner's auto-RSVP lives in the experience's
		// per-item community (#2492), while user1/user2/user3 RSVP'd in communityID.
		rsvps, err := testStorage.QueryByFields(ownerCtx, map[string]interface{}{
			"experience_id": expID,
		}, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("Failed to query RSVPs: %v", err)
		}

		yesCount := 0
		maybeCount := 0
		for _, r := range rsvps {
			rsvp := r.(*models.ExperienceRSVP)
			switch rsvp.GetIntention() {
			case models.RSVPIntention_RSVP_INTENTION_YES:
				yesCount++
				if rsvp.GetAttended() != models.AttendedStatus_ATTENDED_STATUS_YES {
					t.Errorf("Expected YES RSVP to be auto-marked as attended, got %v",
						rsvp.GetAttended())
				}
			case models.RSVPIntention_RSVP_INTENTION_MAYBE:
				maybeCount++
				if rsvp.GetAttended() == models.AttendedStatus_ATTENDED_STATUS_YES {
					t.Errorf("MAYBE RSVP should not be auto-marked as attended, got %v",
						rsvp.GetAttended())
				}
			}
		}

		// owner (auto-RSVP on share) + user1 + user2 = 3 YES
		if yesCount != 3 {
			t.Errorf("Expected 3 YES RSVPs (owner + user1 + user2), got %d", yesCount)
		}
		if maybeCount != 1 {
			t.Errorf("Expected 1 MAYBE RSVP, got %d", maybeCount)
		}
	})
}

// TestService_CompleteExperience_UserSetDuration verifies that when an experience has a
// host-set duration in its time field, CompleteExperience uses USER provenance on the
// social footprint instead of the config default.
func TestService_CompleteExperience_UserSetDuration(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner-dur", "owner@dur.com", "Owner")
	ctx := createAuthenticatedContext("owner-dur", "owner@dur.com", models.Role_ROLE_USER)

	// Create experience with an explicit 180-minute duration using a future time
	// so the experience starts in ACTIVE state (not auto-promoted to IN_PROCESS).
	futureTime := time.Now().Add(24 * time.Hour).Unix()
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name: "4-Hour Potluck",
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: futureTime,
					DurationMinutes:  180,
				},
			},
		},
	})
	createResp, err := service.SaveExperience(ctx, createReq)
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Advance to IN_PROCESS then COMPLETED.
	_, err = service.MarkExperienceInProcess(ctx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}
	resp, err := service.CompleteExperience(ctx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}

	// The returned ImpactEstimate's QT should have USER provenance.
	ie := resp.Msg.Impact
	if ie == nil || ie.QualityTime == nil {
		t.Fatal("impact or QualityTime is nil")
	}
	if ie.QualityTime.Provenance == nil {
		t.Fatal("QualityTime.Provenance is nil")
	}
	if ie.QualityTime.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("Provenance.Source = %v, want USER (host set 180-min duration)",
			ie.QualityTime.Provenance.Source)
	}

	// Duration attribute must match the host-set value.
	attrs := ie.QualityTime.Attributes
	if attrs == nil {
		t.Fatal("SocialAttributes is nil")
	}
	if attrs.EstimatedDurationMinutes != 180 {
		t.Errorf("EstimatedDurationMinutes = %v, want 180", attrs.EstimatedDurationMinutes)
	}
}

func TestService_CompleteExperienceSkipAutoMarkWhenAttendanceRecorded(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create users
	createTestUser(t, testStorage, "owner456", "owner2@example.com", "Owner 2")
	createTestUser(t, testStorage, "user4", "user4@example.com", "User Four")
	createTestUser(t, testStorage, "user5", "user5@example.com", "User Five")

	ownerCtx := createAuthenticatedContext("owner456", "owner2@example.com", models.Role_ROLE_USER)

	// Create experience
	createReq := connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Skip Auto-Mark Test",
	})
	createResp, err := service.SaveExperience(ownerCtx, createReq)
	if err != nil {
		t.Fatalf("Failed to create experience: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Share with community
	communityID := createTestCommunity(t, testStorage, "Test Community 2", "owner456")
	createTestCommunityMembership(t, testStorage, communityID, "owner456")
	createTestCommunityMembership(t, testStorage, communityID, "user4")
	createTestCommunityMembership(t, testStorage, communityID, "user5")
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	// Create RSVPs
	user4Ctx := createAuthenticatedContext("user4", "user4@example.com", models.Role_ROLE_USER)
	user5Ctx := createAuthenticatedContext("user5", "user5@example.com", models.Role_ROLE_USER)

	rsvpReq4 := connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})
	_, err = service.RSVPToExperience(user4Ctx, rsvpReq4)
	if err != nil {
		t.Fatalf("Failed to RSVP user4: %v", err)
	}

	rsvpReq5 := connect.NewRequest(&api.RSVPToExperienceRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
		Intention:    api.RSVPIntention_RSVP_INTENTION_YES,
	})
	_, err = service.RSVPToExperience(user5Ctx, rsvpReq5)
	if err != nil {
		t.Fatalf("Failed to RSVP user5: %v", err)
	}

	t.Run("skips auto-mark when attendance already recorded", func(t *testing.T) {
		// Mark experience in process
		markReq := connect.NewRequest(&api.MarkExperienceInProcessRequest{
			ExperienceId: expID,
		})
		_, err := service.MarkExperienceInProcess(ownerCtx, markReq)
		if err != nil {
			t.Fatalf("Failed to mark in process: %v", err)
		}

		// Manually record attendance (user4 attended, user5 did not)
		recordReq := connect.NewRequest(&api.RecordAttendanceRequest{
			ExperienceId: expID,
			CommunityId:  communityID,
			Attendance: []*api.AttendanceRecord{
				{UserId: "user4", Attended: api.AttendedStatus_ATTENDED_STATUS_YES},
				{UserId: "user5", Attended: api.AttendedStatus_ATTENDED_STATUS_NO},
			},
		})
		_, err = service.RecordAttendance(ownerCtx, recordReq)
		if err != nil {
			t.Fatalf("RecordAttendance failed: %v", err)
		}

		// Complete experience (should NOT auto-mark since attendance already recorded)
		completeReq := connect.NewRequest(&api.CompleteExperienceRequest{
			ExperienceId: expID,
		})
		_, err = service.CompleteExperience(ownerCtx, completeReq)
		if err != nil {
			t.Fatalf("CompleteExperience failed: %v", err)
		}

		// Verify attendance was NOT changed by auto-mark
		rsvps, err := testStorage.QueryByFields(ownerCtx, map[string]interface{}{
			"experience_id": expID,
			"community_id":  communityID,
		}, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("Failed to query RSVPs: %v", err)
		}

		for _, r := range rsvps {
			rsvp := r.(*models.ExperienceRSVP)
			switch rsvp.UserId {
			case "user4":
				if rsvp.GetAttended() != models.AttendedStatus_ATTENDED_STATUS_YES {
					t.Errorf("user4 should still be marked as attended=YES, got %v",
						rsvp.GetAttended())
				}
			case "user5":
				if rsvp.GetAttended() != models.AttendedStatus_ATTENDED_STATUS_NO {
					t.Errorf("user5 should still be marked as attended=NO, got %v",
						rsvp.GetAttended())
				}
			}
		}
	})
}

// TestService_CompleteExperience_SkipsAIDuringSimulation verifies that InferSocialAttributes
// is not called when completing an experience in a simulated context.
func TestService_CompleteExperience_SkipsAIDuringSimulation(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create experience before wiring the strict mock so the LLM inference
	// at creation (Phase 2) does not trip the simulation check.
	createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Simulation Test Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	// Wire mock AI provider that fails if called — installed after creation so
	// only CompleteExperience is covered by the simulation guard assertion.
	mockAI := ai.NewMockProvider()
	inferCalled := false
	mockAI.InferSocialAttributesFunc = func(_ context.Context, _, _, _ string, _ float32) (*ai.SocialAttributeInference, error) {
		inferCalled = true
		t.Error("InferSocialAttributes should not be called during simulation")
		return nil, nil
	}
	service.SetAIProvider(mockAI)

	// Mark in process.
	_, err = service.MarkExperienceInProcess(ctx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}

	// Complete with simulated context — AI should be skipped.
	simCtx := clock.WithSimulationTime(ctx, time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC))
	_, err = service.CompleteExperience(simCtx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}

	if inferCalled {
		t.Fatal("InferSocialAttributes was called during simulation")
	}

	// Verify experience still completed successfully with impact estimate.
	stored := &models.Experience{}
	if err := testStorage.GetByID(context.Background(), expID, stored); err != nil {
		t.Fatalf("Failed to get experience: %v", err)
	}
	if stored.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
		t.Errorf("Expected COMPLETED state, got %v", stored.State)
	}
	if stored.ImpactEstimate == nil {
		t.Error("Expected ImpactEstimate to be set even without AI inference")
	}
}

// TestCompleteExperience_WithOverrides_PersistsPerInputProvenance verifies that
// user-supplied overrides on CompleteExperience are applied to the stored
// ImpactEstimate with USER provenance on overridden attributes.
func TestCompleteExperience_WithOverrides_PersistsPerInputProvenance(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "host1", "host1@example.com", "Host One")
	ctx := createAuthenticatedContext("host1", "host1@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Override Test Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	_, err = service.MarkExperienceInProcess(ctx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}

	// Complete with an explicit money-savings override via HireEquivalentValue input.
	overrideValueUSD := float32(42.50)
	resp, err := service.CompleteExperience(ctx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: expID,
		MoneySavingsOverrides: &api.MoneySavings{
			Inputs: &api.MoneySavingsInput{
				HireEquivalentValue: &api.Estimate{Mean: overrideValueUSD},
			},
		},
	}))
	if err != nil {
		t.Fatalf("CompleteExperience with overrides failed: %v", err)
	}

	ie := resp.Msg.Impact
	if ie == nil {
		t.Fatal("impact estimate is nil in response")
	}

	// Money savings value must reflect the override.
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

	// Stored estimate must also reflect the override (not just the response).
	stored := &models.Experience{}
	if err := testStorage.GetByID(context.Background(), expID, stored); err != nil {
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
