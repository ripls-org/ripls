package experience

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestGetExperienceStats_ReturnsPersistedImpactWithOverrides ensures that
// once an experience is completed with user overrides, GetExperienceStats
// returns the persisted impact rather than recomputing from raw inputs.
//
// Regression guard: an earlier implementation always called
// BuildExperienceImpactMetrics here, which silently dropped any user
// overrides committed via CompleteExperience and made the results-tab
// numbers diverge from the completion-modal numbers.
func TestGetExperienceStats_ReturnsPersistedImpactWithOverrides(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "host_stats", "host_stats@example.com", "Stats Host")
	ctx := createAuthenticatedContext("host_stats", "host_stats@example.com", models.Role_ROLE_USER)
	communityID := createTestCommunity(t, testStorage, "Stats Community", "host_stats")
	createTestCommunityMembership(t, testStorage, communityID, "host_stats")

	// Create + share + advance to IN_PROCESS.
	createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Stats Persistence Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	shareExperienceForTest(t, service, ctx, expID, communityID)
	if _, err := service.MarkExperienceInProcess(ctx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	})); err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}

	// Complete with a money-savings override that the estimator would never
	// produce on its own — picks an unusual value so any rebuild is detectable.
	const overrideUSD = float32(137)
	if _, err := service.CompleteExperience(ctx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: expID,
		MoneySavingsOverrides: &api.MoneySavings{
			Inputs: &api.MoneySavingsInput{
				HireEquivalentValue: &api.Estimate{Mean: overrideUSD},
			},
		},
	})); err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}

	// Read the results-tab numbers via GetExperienceStats. The override must survive.
	statsResp, err := service.GetExperienceStats(ctx, connect.NewRequest(&api.GetExperienceStatsRequest{
		ExperienceId: expID,
		CommunityId:  communityID,
	}))
	if err != nil {
		t.Fatalf("GetExperienceStats failed: %v", err)
	}

	got := statsResp.Msg.Impact
	if got == nil || got.MoneySaved == nil {
		t.Fatal("expected stats Impact.MoneySaved to be populated for completed experience")
	}
	if got.MoneySaved.GetValueUsd().GetMean() != overrideUSD {
		t.Errorf("results-tab MoneySaved = %v, want %v (override dropped — stats is recomputing instead of reading persisted impact)",
			got.MoneySaved.GetValueUsd().GetMean(), overrideUSD)
	}
	if got.MoneySaved.GetProvenance().GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("results-tab MoneySaved provenance = %v, want USER (provenance lost on read path)",
			got.MoneySaved.GetProvenance().GetSource())
	}
}

// TestGetExperienceStats_FallsBackToBuildWhenUnpersisted exercises the fallback
// path for legacy completed experiences that never had an ImpactEstimate
// persisted. Stats should still produce non-nil impact via the estimator.
func TestGetExperienceStats_FallsBackToBuildWhenUnpersisted(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "host_legacy", "host_legacy@example.com", "Legacy Host")
	ctx := createAuthenticatedContext("host_legacy", "host_legacy@example.com", models.Role_ROLE_USER)
	communityID := createTestCommunity(t, testStorage, "Legacy Community", "host_legacy")
	createTestCommunityMembership(t, testStorage, communityID, "host_legacy")

	// Stitch together a completed experience with no ImpactEstimate persisted —
	// simulates an older record from before completion-time impact existed.
	exp := &models.Experience{
		Id:      "legacy-exp-1",
		Name:    "Legacy Completed",
		OwnerId: "host_legacy",
		State:   models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
	}
	if _, err := testStorage.Insert(context.Background(), exp); err != nil {
		t.Fatalf("Insert experience: %v", err)
	}
	cr := &models.CommunityExperience{
		Id:           "legacy-ce-1",
		ExperienceId: exp.Id,
		CommunityId:  communityID,
	}
	if _, err := testStorage.Insert(context.Background(), cr); err != nil {
		t.Fatalf("Insert CommunityExperience: %v", err)
	}

	resp, err := service.GetExperienceStats(ctx, connect.NewRequest(&api.GetExperienceStatsRequest{
		ExperienceId: exp.Id,
		CommunityId:  communityID,
	}))
	if err != nil {
		t.Fatalf("GetExperienceStats failed: %v", err)
	}
	if resp.Msg.Impact == nil {
		t.Fatal("expected fallback impact to be non-nil for legacy completed experience")
	}
}
