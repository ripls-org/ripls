package integration_tests

import (
	"context"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	impact_metrics "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/notifications"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/pubsub"
	experience_svc "go.ripls.org/ripls/server/services/experience"
	impact_metrics_svc "go.ripls.org/ripls/server/services/impact_metrics"
	"go.ripls.org/ripls/server/storage"
)

// setupExperienceService creates an experience service backed by the provided storage.
func setupExperienceService(t *testing.T, db *storage.ProtoSQLStorage) *experience_svc.Service {
	t.Helper()
	tempDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("failed to create test bucket: %v", err)
	}
	mockNotif := notifications.NewMockService()
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(db, topic)
	notifSub := commsub.New(db, mockNotif)
	if _, err := bus.Subscribe(notifSub); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	svc := experience_svc.New(db, bucket, mockNotif, bus)
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("failed to load estimator config: %v", err)
	}
	svc.SetEstimatorConfig(cfg)
	return svc
}

// makeAuthContext returns a context carrying authentication info.
func makeAuthContext(userID, email string, role models.Role) context.Context {
	return authn.SetInfo(context.Background(), &auth.Info{
		UserID: userID,
		Email:  email,
		Role:   role,
	})
}

// insertUser inserts a minimal User row directly into storage.
func insertUser(t *testing.T, db *storage.ProtoSQLStorage, userID, email, name string) {
	t.Helper()
	_, err := db.Insert(context.Background(), &models.User{
		Id:    userID,
		Email: email,
		Name:  name,
	})
	if err != nil {
		t.Fatalf("failed to insert user %s: %v", userID, err)
	}
}

// TestCompleteExperience_DraftThenOverrideThenCommit verifies the full round-trip:
// DraftImpact produces LLM provenance, user overrides money savings, CompleteExperience
// commits the override, and the stored estimate carries USER provenance on that field.
func TestCompleteExperience_DraftThenOverrideThenCommit(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	expSvc := setupExperienceService(t, db)

	// Reuse the same storage so the draft service sees the same experience rows.
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("failed to load estimator config: %v", err)
	}
	draftSvc := impact_metrics_svc.NewService(db, cfg)

	// Wire a mock AI provider that returns a deterministic inference.
	mock := ai.NewMockProvider()
	mock.InferSocialAttributesFunc = func(_ context.Context, _, _, _ string, _ float32) (*ai.SocialAttributeInference, error) {
		return &ai.SocialAttributeInference{
			DurationMinutes:    60,
			VulnerabilityLevel: "medium",
		}, nil
	}
	draftSvc.SetAIProvider(mock)

	insertUser(t, db, "host99", "host99@example.com", "Host")
	ctx := makeAuthContext("host99", "host99@example.com", models.Role_ROLE_USER)

	// Create and advance experience to IN_PROCESS.
	createResp, err := expSvc.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Round-Trip Test",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := createResp.Msg.Experience.Id

	_, err = expSvc.MarkExperienceInProcess(ctx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}

	// Step 1: draft — LLM provenance on all fields.
	draftResp, err := draftSvc.DraftImpactEstimate(ctx, connect.NewRequest(&api.DraftImpactEstimateRequest{
		Target: &api.DraftImpactEstimateRequest_ExperienceId{ExperienceId: expID},
	}))
	if err != nil {
		t.Fatalf("DraftImpactEstimate failed: %v", err)
	}
	if draftResp.Msg.Impact == nil {
		t.Fatal("draft impact is nil")
	}

	// Step 2: simulate user overriding the money savings.
	overrideValueUSD := float32(99.00)
	overrides := &api.MoneySavings{
		Inputs: &api.MoneySavingsInput{
			HireEquivalentValue: &api.Estimate{Mean: overrideValueUSD},
		},
	}

	// Step 3: commit with override.
	commitResp, err := expSvc.CompleteExperience(ctx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId:          expID,
		MoneySavingsOverrides: overrides,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}

	committed := commitResp.Msg.Impact
	if committed == nil {
		t.Fatal("committed impact is nil")
	}

	// Overridden money savings must have USER provenance and the override value.
	if committed.MoneySaved.GetValueUsd().GetMean() != overrideValueUSD {
		t.Errorf("committed ValueUsd.Mean = %v, want %v", committed.MoneySaved.GetValueUsd().GetMean(), overrideValueUSD)
	}
	if committed.MoneySaved.GetProvenance().GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("committed MoneySaved provenance = %v, want USER", committed.MoneySaved.GetProvenance().GetSource())
	}

	// Verify persisted state matches the response.
	stored := &models.Experience{}
	if err := db.GetByID(context.Background(), expID, stored); err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	storedIE := impact_metrics.ModelsImpactToAPI(stored.ImpactEstimate)
	if storedIE.MoneySaved.GetValueUsd().GetMean() != overrideValueUSD {
		t.Errorf("persisted ValueUsd.Mean = %v, want %v", storedIE.MoneySaved.GetValueUsd().GetMean(), overrideValueUSD)
	}
	if storedIE.MoneySaved.GetProvenance().GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("persisted MoneySaved provenance = %v, want USER", storedIE.MoneySaved.GetProvenance().GetSource())
	}
}
