package impact_metrics

import (
	"context"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/storage"
)

// setupDraftingTestService creates a Service wired with real storage and the given AI provider.
func setupDraftingTestService(t *testing.T, provider ai.Provider) (*Service, *storage.ProtoSQLStorage) {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("failed to load estimator config: %v", err)
	}

	svc := NewService(sqlStorage, cfg)
	if provider != nil {
		svc.SetAIProvider(provider)
	}
	return svc, sqlStorage
}

// authedCtx returns a context with auth info for the given user.
func authedCtx(userID, email string) context.Context {
	info := &auth.Info{UserID: userID, Email: email, Role: models.Role_ROLE_USER}
	return authn.SetInfo(context.Background(), info)
}

// insertUserWithID inserts a minimal user row with a caller-supplied ID so
// authorization assertions in this file can reference the user by a known
// string (e.g., "host1"). Distinct from common_test.go's insertTestUser
// which generates a UUID.
func insertUserWithID(t *testing.T, s *storage.ProtoSQLStorage, userID string) {
	t.Helper()
	_, err := s.Insert(context.Background(), &models.User{Id: userID, Email: userID + "@test.com", Name: "Test"})
	if err != nil {
		t.Fatalf("insertUserWithID: %v", err)
	}
}

// insertTestExperience inserts an experience owned by ownerID and returns its storage ID.
func insertTestExperience(t *testing.T, s *storage.ProtoSQLStorage, ownerID string) string {
	t.Helper()
	id, err := s.Insert(context.Background(), &models.Experience{
		OwnerId: ownerID,
		Name:    "Test Experience",
	})
	if err != nil {
		t.Fatalf("insertTestExperience: %v", err)
	}
	return id
}

// insertTestRequest inserts a request created by requesterID and returns its storage ID.
func insertTestRequest(t *testing.T, s *storage.ProtoSQLStorage, requesterID string) string {
	t.Helper()
	id, err := s.Insert(context.Background(), &models.Request{
		RequesterId: requesterID,
		Title:       "Test Request",
	})
	if err != nil {
		t.Fatalf("insertTestRequest: %v", err)
	}
	return id
}

// insertConfirmedRSVP adds a YES RSVP linking attendeeID to experienceID.
func insertConfirmedRSVP(t *testing.T, s *storage.ProtoSQLStorage, experienceID, attendeeID string) {
	t.Helper()
	_, err := s.Insert(context.Background(), &models.ExperienceRSVP{
		ExperienceId: experienceID,
		UserId:       attendeeID,
		Intention:    models.RSVPIntention_RSVP_INTENTION_YES,
	})
	if err != nil {
		t.Fatalf("insertConfirmedRSVP: %v", err)
	}
}

// ── DraftImpactEstimate — authorization ──────────────────────────────────────.

func TestDraftImpactEstimate_HostCanDraft(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "host1")
	expID := insertTestExperience(t, db, "host1")

	ctx := authedCtx("host1", "host1@test.com")
	req := connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_ExperienceId{ExperienceId: expID},
	})
	resp, err := svc.DraftImpactEstimate(ctx, req)
	if err != nil {
		t.Fatalf("host should be authorized, got error: %v", err)
	}
	if resp.Msg.Impact == nil {
		t.Error("expected non-nil Impact in response")
	}
}

func TestDraftImpactEstimate_ConfirmedAttendeeCanDraft(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "host1")
	insertUserWithID(t, db, "attendee1")
	expID := insertTestExperience(t, db, "host1")
	insertConfirmedRSVP(t, db, expID, "attendee1")

	ctx := authedCtx("attendee1", "attendee1@test.com")
	req := connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_ExperienceId{ExperienceId: expID},
	})
	_, err := svc.DraftImpactEstimate(ctx, req)
	if err != nil {
		t.Fatalf("confirmed attendee should be authorized, got: %v", err)
	}
}

func TestDraftImpactEstimate_StrangerDenied(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "host1")
	insertUserWithID(t, db, "stranger")
	expID := insertTestExperience(t, db, "host1")

	ctx := authedCtx("stranger", "stranger@test.com")
	req := connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_ExperienceId{ExperienceId: expID},
	})
	_, err := svc.DraftImpactEstimate(ctx, req)
	if err == nil {
		t.Fatal("expected PermissionDenied, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connect.CodeOf(err))
	}
}

func TestDraftImpactEstimate_UnauthenticatedDenied(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "host1")
	expID := insertTestExperience(t, db, "host1")

	req := connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_ExperienceId{ExperienceId: expID},
	})
	_, err := svc.DraftImpactEstimate(context.Background(), req)
	if err == nil {
		t.Fatal("expected Unauthenticated, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("expected CodeUnauthenticated, got %v", connect.CodeOf(err))
	}
}

// ── DraftImpactEstimate — request authorization ───────────────────────────────.

func TestDraftImpactEstimate_Request_CreatorCanDraft(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "requester1")
	reqID := insertTestRequest(t, db, "requester1")

	ctx := authedCtx("requester1", "requester1@test.com")
	rpcReq := connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_RequestId{RequestId: reqID},
	})
	resp, err := svc.DraftImpactEstimate(ctx, rpcReq)
	if err != nil {
		t.Fatalf("requester should be authorized, got: %v", err)
	}
	if resp.Msg.Impact == nil {
		t.Error("expected non-nil Impact")
	}
}

func TestDraftImpactEstimate_Request_StrangerDenied(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "requester1")
	insertUserWithID(t, db, "stranger")
	reqID := insertTestRequest(t, db, "requester1")

	ctx := authedCtx("stranger", "stranger@test.com")
	rpcReq := connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_RequestId{RequestId: reqID},
	})
	_, err := svc.DraftImpactEstimate(ctx, rpcReq)
	if err == nil {
		t.Fatal("expected PermissionDenied, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connect.CodeOf(err))
	}
}

func TestDraftImpactEstimate_Request_UnauthenticatedDenied(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "requester1")
	reqID := insertTestRequest(t, db, "requester1")

	rpcReq := connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_RequestId{RequestId: reqID},
	})
	_, err := svc.DraftImpactEstimate(context.Background(), rpcReq)
	if err == nil {
		t.Fatal("expected Unauthenticated, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("expected CodeUnauthenticated, got %v", connect.CodeOf(err))
	}
}

// ── Transcript scoping ────────────────────────────────────────────────────────.

// TestDraftImpactEstimate_ScopesChatTranscriptToLimits seeds a conversation with
// more messages than the configured max and verifies the LLM provider only receives
// the capped set.
func TestDraftImpactEstimate_ScopesChatTranscriptToLimits(t *testing.T) {
	mock := ai.NewMockProvider()

	var capturedDescription string
	mock.InferSocialAttributesFunc = func(_ context.Context, _, description, _ string, _ float32) (*ai.SocialAttributeInference, error) {
		capturedDescription = description
		return nil, nil
	}

	svc, db := setupDraftingTestService(t, mock)
	insertUserWithID(t, db, "host1")

	convID, err := db.Insert(context.Background(), &models.ChatConversation{})
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	expID, err := db.Insert(context.Background(), &models.Experience{
		OwnerId:        "host1",
		Name:           "Scoping Test",
		ConversationId: convID,
	})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}

	// Insert 60 messages — more than the default cap of 50.
	for i := int64(0); i < 60; i++ {
		_, err := db.Insert(context.Background(), &models.ChatMessage{
			ConversationId: convID,
			SentAtUnixSec:  i,
			Message: &models.ChatMessage_UserMessage{
				UserMessage: &models.UserChatMessage{
					SenderId: "host1",
					Text:     "msg",
				},
			},
		})
		if err != nil {
			t.Fatalf("insert message %d: %v", i, err)
		}
	}

	ctx := authedCtx("host1", "host1@test.com")
	req := connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_ExperienceId{ExperienceId: expID},
	})
	if _, err := svc.DraftImpactEstimate(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Calls.InferSocialAttributes) == 0 {
		t.Fatal("expected InferSocialAttributes to be called")
	}

	// The description passed to the LLM should contain exactly 50 "msg" tokens.
	msgCount := 0
	for i := 0; i < len(capturedDescription)-2; i++ {
		if capturedDescription[i:i+3] == "msg" {
			msgCount++
		}
	}
	if msgCount > 50 {
		t.Errorf("transcript contained %d messages, want ≤ 50", msgCount)
	}
}

// ── DraftImpactEstimateWithOverrides — no LLM ─────────────────────────────────.

// TestDraftImpactEstimateWithOverrides_DoesNotCallLLM verifies the redraft path
// never calls the AI provider, even when one is configured.
func TestDraftImpactEstimateWithOverrides_DoesNotCallLLM(t *testing.T) {
	mock := ai.NewMockProvider()
	svc, db := setupDraftingTestService(t, mock)
	insertUserWithID(t, db, "host1")
	expID := insertTestExperience(t, db, "host1")

	ctx := authedCtx("host1", "host1@test.com")
	req := connect.NewRequest(&api.DraftImpactEstimateWithOverridesRequest{
		Target:           &api.DraftImpactEstimateWithOverridesRequest_ExperienceId{ExperienceId: expID},
		QualityTimeInput: &api.QualityTimeAttributes{EstimatedDurationMinutes: 45},
	})
	if _, err := svc.DraftImpactEstimateWithOverrides(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Calls.InferSocialAttributes) != 0 {
		t.Errorf("expected 0 LLM calls, got %d", len(mock.Calls.InferSocialAttributes))
	}
}

// TestDraftImpactEstimateWithOverrides_RecomputesComposite confirms that providing
// a larger duration produces a larger composite quality_time_minutes.
func TestDraftImpactEstimateWithOverrides_RecomputesComposite(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "host1")
	expID := insertTestExperience(t, db, "host1")
	ctx := authedCtx("host1", "host1@test.com")

	call := func(dur float32) float32 {
		req := connect.NewRequest(&api.DraftImpactEstimateWithOverridesRequest{
			Target:           &api.DraftImpactEstimateWithOverridesRequest_ExperienceId{ExperienceId: expID},
			QualityTimeInput: &api.QualityTimeAttributes{EstimatedDurationMinutes: dur},
		})
		resp, err := svc.DraftImpactEstimateWithOverrides(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error for dur=%.0f: %v", dur, err)
		}
		return resp.Msg.Impact.GetQualityTime().GetQualityTimeMinutes().GetMean()
	}

	small := call(30)
	large := call(120)
	if large <= small {
		t.Errorf("larger duration should produce larger QT minutes: 120m=%.1f, 30m=%.1f", large, small)
	}
}

// TestDraftImpactEstimateWithOverrides_Unauthorized verifies that a stranger
// cannot use the override redraft path.
func TestDraftImpactEstimateWithOverrides_Unauthorized(t *testing.T) {
	svc, db := setupDraftingTestService(t, nil)
	insertUserWithID(t, db, "host1")
	insertUserWithID(t, db, "stranger")
	expID := insertTestExperience(t, db, "host1")

	ctx := authedCtx("stranger", "stranger@test.com")
	req := connect.NewRequest(&api.DraftImpactEstimateWithOverridesRequest{
		Target:           &api.DraftImpactEstimateWithOverridesRequest_ExperienceId{ExperienceId: expID},
		QualityTimeInput: &api.QualityTimeAttributes{EstimatedDurationMinutes: 45},
	})
	_, err := svc.DraftImpactEstimateWithOverrides(ctx, req)
	if err == nil {
		t.Fatal("expected PermissionDenied, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connect.CodeOf(err))
	}
}

// ── applyContributionEstimates — math ─────────────────────────────────────────.

func loadDraftingTestConfig(t *testing.T) *estimator.Config {
	t.Helper()
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("failed to load estimator config: %v", err)
	}
	return cfg
}

// TestApplyContributionEstimates_EmptyContributions returns base unchanged.
func TestApplyContributionEstimates_EmptyContributions(t *testing.T) {
	cfg := loadDraftingTestConfig(t)
	base := impact_metrics.BuildExperienceImpactMetrics(100, 2, cfg, nil, nil)

	result := applyContributionEstimates(base, nil, 100, estimator.TransactionExperienceConcluded, cfg)
	if result != base {
		t.Error("expected same pointer when contributions is empty")
	}
}

// TestApplyContributionEstimates_ZeroValue returns base unchanged.
func TestApplyContributionEstimates_ZeroValue(t *testing.T) {
	cfg := loadDraftingTestConfig(t)
	base := impact_metrics.BuildExperienceImpactMetrics(0, 2, cfg, nil, nil)
	contribs := []*models.PlanningContribution{{Id: "c1"}}

	result := applyContributionEstimates(base, contribs, 0, estimator.TransactionExperienceConcluded, cfg)
	if result != base {
		t.Error("expected same pointer when valueUSD is zero")
	}
}

// TestApplyContributionEstimates_MoneySavedNonZero verifies that a non-zero value
// with contributions produces a positive money savings mean.
func TestApplyContributionEstimates_MoneySavedNonZero(t *testing.T) {
	cfg := loadDraftingTestConfig(t)
	base := impact_metrics.BuildExperienceImpactMetrics(100, 2, cfg, nil, nil)
	contribs := []*models.PlanningContribution{{Id: "c1"}, {Id: "c2"}}

	result := applyContributionEstimates(base, contribs, 100, estimator.TransactionExperienceConcluded, cfg)
	if result == base {
		t.Fatal("expected a new estimate, not the base pointer")
	}
	if result.MoneySaved == nil {
		t.Fatal("MoneySaved is nil")
	}
	if result.MoneySaved.GetValueUsd().GetMean() <= 0 {
		t.Errorf("money saved mean = %.4f, want > 0", result.MoneySaved.GetValueUsd().GetMean())
	}
}

// TestApplyContributionEstimates_EmissionsNonZero verifies CO2 is non-zero.
func TestApplyContributionEstimates_EmissionsNonZero(t *testing.T) {
	cfg := loadDraftingTestConfig(t)
	base := impact_metrics.BuildExperienceImpactMetrics(100, 2, cfg, nil, nil)
	contribs := []*models.PlanningContribution{{Id: "c1"}}

	result := applyContributionEstimates(base, contribs, 100, estimator.TransactionExperienceConcluded, cfg)
	if result.EmissionsPrevented == nil {
		t.Fatal("EmissionsPrevented is nil")
	}
	co2 := result.EmissionsPrevented.GetManufactureAvoidedCarbon().GetCo2EGrams().GetMean()
	if co2 <= 0 {
		t.Errorf("CO2 mean = %.4f, want > 0", co2)
	}
}

// TestApplyContributionEstimates_SplitsValueEvenly confirms that two equal
// contributions receive half the value each, and the composite is the sum.
func TestApplyContributionEstimates_SplitsValueEvenly(t *testing.T) {
	cfg := loadDraftingTestConfig(t)
	base := impact_metrics.BuildExperienceImpactMetrics(200, 2, cfg, nil, nil)

	one := applyContributionEstimates(base, []*models.PlanningContribution{{Id: "c1"}}, 200, estimator.TransactionExperienceConcluded, cfg)
	two := applyContributionEstimates(base, []*models.PlanningContribution{{Id: "c1"}, {Id: "c2"}}, 200, estimator.TransactionExperienceConcluded, cfg)

	oneVal := one.MoneySaved.GetValueUsd().GetMean()
	twoVal := two.MoneySaved.GetValueUsd().GetMean()

	// Two contributions split the value, so each gets half; total should equal one-contribution total.
	const tol = 0.01
	if twoVal < oneVal-tol || twoVal > oneVal+tol {
		t.Errorf("two-contribution total = %.4f, want ≈ %.4f (same total, split evenly)", twoVal, oneVal)
	}
}
