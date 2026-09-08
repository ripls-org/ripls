package experience

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// qtDurationFromStats returns the QT estimated_duration_minutes from a
// GetExperienceStats response, or 0 if unavailable.
func qtDurationFromStats(t *testing.T, svc *Service, ctx context.Context, expID, communityID string) float32 {
	t.Helper()
	resp, err := svc.GetExperienceStats(ctx, connect.NewRequest(&api.GetExperienceStatsRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
	}))
	if err != nil {
		t.Fatalf("GetExperienceStats failed: %v", err)
	}
	pi := resp.Msg.PotentialImpact
	if pi == nil || pi.QualityTime == nil || pi.QualityTime.Attributes == nil {
		return 0
	}
	return pi.QualityTime.Attributes.EstimatedDurationMinutes
}

// TestSaveExperience_UsesUserDurationAtCreation verifies that SaveExperience persists
// an ImpactEstimate that reflects the user-set duration when a specific time is provided.
func TestSaveExperience_UsesUserDurationAtCreation(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "user1", "user1@example.com", "User One")
	ctx := createAuthenticatedContext("user1", "user1@example.com", models.Role_ROLE_USER)
	communityID := createTestCommunity(t, testStorage, "Test Community", "user1")
	createTestCommunityMembership(t, testStorage, communityID, "user1")

	// Create experience with a specific time that has duration_minutes = 90.
	resp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "90-Minute Pottery Workshop",
		Description: "A hands-on ceramics session",
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1800000000,
					DurationMinutes:  90,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := resp.Msg.Experience.Id

	// Share to the community so GetExperienceStats can find it.
	shareExperienceForTest(t, service, ctx, expID, communityID)

	duration := qtDurationFromStats(t, service, ctx, expID, communityID)
	// The estimated duration should reflect the 90-minute user-set value.
	if duration < 80 || duration > 100 {
		t.Errorf("expected estimated_duration_minutes ≈ 90, got %.1f", duration)
	}
}

// TestSaveExperience_RecalculatesQTOnUpdate verifies that updating an experience's
// duration produces a larger QT duration estimate than the config default.
func TestSaveExperience_RecalculatesQTOnUpdate(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "user2", "user2@example.com", "User Two")
	ctx := createAuthenticatedContext("user2", "user2@example.com", models.Role_ROLE_USER)
	communityID := createTestCommunity(t, testStorage, "Test Community 2", "user2")
	createTestCommunityMembership(t, testStorage, communityID, "user2")

	// Create experience with no time set (uses config default duration).
	createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Community Dinner",
		Description: "A shared meal together",
	}))
	if err != nil {
		t.Fatalf("SaveExperience (create) failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ctx, expID, communityID)

	defaultDuration := qtDurationFromStats(t, service, ctx, expID, communityID)

	// Update the experience to set a specific 180-minute duration.
	_, err = service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Id:   &expID,
		Name: "Community Dinner",
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1800000000,
					DurationMinutes:  180,
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveExperience (update) failed: %v", err)
	}

	updatedDuration := qtDurationFromStats(t, service, ctx, expID, communityID)

	if updatedDuration <= defaultDuration {
		t.Errorf("expected updated QT duration (%.1f) > default (%.1f) after setting 180-min duration",
			updatedDuration, defaultDuration)
	}
}

// TestSaveExperience_LLMFallbackOnFailure verifies that LLM inference failure
// during creation does not fail the overall SaveExperience call — the stored
// estimate falls back to config defaults gracefully.
func TestSaveExperience_LLMFallbackOnFailure(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "user3", "user3@example.com", "User Three")
	ctx := createAuthenticatedContext("user3", "user3@example.com", models.Role_ROLE_USER)

	// Configure a mock AI provider that always errors on InferSocialAttributes.
	mockAI := ai.NewMockProvider()
	mockAI.InferSocialAttributesFunc = func(_ context.Context, _, _, _ string, _ float32) (*ai.SocialAttributeInference, error) {
		return nil, errors.New("simulated LLM failure")
	}
	service.SetAIProvider(mockAI)

	// Must succeed even though LLM fails.
	resp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Fallback Test",
		Description: "LLM will fail",
	}))
	if err != nil {
		t.Fatalf("SaveExperience should not fail when LLM errors: %v", err)
	}
	if resp.Msg.Experience.Id == "" {
		t.Fatal("expected a valid experience ID in the response")
	}
}
