package experience

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/pubsub"
)

func TestService_SaveExperience(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	// Create test users once for all sub-tests
	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	createTestUser(t, testStorage, "user456", "user456@example.com", "User 456")

	t.Run("insert experience with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:            "Weekend Hike",
			Description:     "A fun hiking trip to the mountains",
			MediaIds:        []string{"media-001", "media-002"},
			LocationId:      "location-mountains",
			MaxParticipants: 10,
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		if resp.Msg.Experience.Id == "" {
			t.Error("Expected non-empty ID")
		}

		// Verify experience was stored
		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, resp.Msg.Experience.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored experience: %v", err)
		}

		if stored.OwnerId != "user123" {
			t.Errorf("Expected OwnerId to be user123, got %s", stored.OwnerId)
		}

		if stored.Name != "Weekend Hike" {
			t.Errorf("Expected name 'Weekend Hike', got %s", stored.Name)
		}

		if stored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE {
			t.Errorf("Expected state ACTIVE, got %v", stored.State)
		}

		if stored.MaxParticipants != 10 {
			t.Errorf("Expected MaxParticipants 10, got %d", stored.MaxParticipants)
		}

		// Creation stamps a creation timestamp (drives most-recent-first
		// ordering in the Home "Yours" section).
		if stored.CreatedAtUnixSec <= 0 {
			t.Errorf("Expected CreatedAtUnixSec to be set, got %d", stored.CreatedAtUnixSec)
		}

		// Verify ImpactEstimate was persisted at creation
		if stored.ImpactEstimate == nil {
			t.Error("Expected ImpactEstimate to be set after SaveExperience")
		} else if stored.ImpactEstimate.TimeSaved == nil || stored.ImpactEstimate.TimeSaved.Minutes == nil {
			t.Error("Expected TimeSaved.Minutes to be set after SaveExperience")
		}
	})

	t.Run("insert experience with default TBD time", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "TBD Time Experience",
			Description: "Time not yet decided",
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, resp.Msg.Experience.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored experience: %v", err)
		}

		if stored.Time == nil {
			t.Fatal("Expected Time to be set")
		}

		if stored.Time.GetTbd() == nil {
			t.Error("Expected TBD time type")
		}
	})

	t.Run("update experience with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create experience
		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Original Name",
			Description: "Original Description",
			MediaIds:    []string{"media-001"},
		})

		createResp, err := service.SaveExperience(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience: %v", err)
		}

		expID := createResp.Msg.Experience.Id

		// Update it
		updateReq := connect.NewRequest(&api.SaveExperienceRequest{
			Id:          &expID,
			Name:        "Updated Name",
			Description: "Updated Description",
			MediaIds:    []string{"media-002", "media-003"},
		})

		updateResp, err := service.SaveExperience(ctx, updateReq)
		if err != nil {
			t.Fatalf("Failed to update experience: %v", err)
		}

		if updateResp.Msg.Experience.Id != expID {
			t.Errorf("Expected ID %s, got %s", expID, updateResp.Msg.Experience.Id)
		}

		// Verify updates
		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, expID, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve updated experience: %v", err)
		}

		if stored.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got %s", stored.Name)
		}

		if len(stored.MediaIds) != 2 {
			t.Fatalf("Expected 2 media IDs, got %d", len(stored.MediaIds))
		}
	})

	t.Run("update non-existent experience fails", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		nonExistentID := "nonexistent-id"
		req := connect.NewRequest(&api.SaveExperienceRequest{
			Id:   &nonExistentID,
			Name: "Should Fail",
		})

		_, err := service.SaveExperience(ctx, req)
		if err == nil {
			t.Fatal("Expected error when updating non-existent experience")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("update experience by non-owner fails", func(t *testing.T) {
		ctx123 := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create experience as user123
		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name: "User123's Experience",
		})

		createResp, err := service.SaveExperience(ctx123, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience: %v", err)
		}

		// Try to update as user456
		ctx456 := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)
		expID := createResp.Msg.Experience.Id
		updateReq := connect.NewRequest(&api.SaveExperienceRequest{
			Id:   &expID,
			Name: "Should Fail",
		})

		_, err = service.SaveExperience(ctx456, updateReq)
		if err == nil {
			t.Fatal("Expected error when non-owner tries to update experience")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name: "Test Experience",
		})

		_, err := service.SaveExperience(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unauthenticated request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("insert experience with source URL", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		sourceURL := "https://www.eventbrite.com/e/community-game-night-123456"
		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Community Game Night",
			Description: "A fun evening of board games",
			SourceUrl:   &sourceURL,
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		// Verify source URL is returned in the API response
		if resp.Msg.Experience.SourceUrl == nil || *resp.Msg.Experience.SourceUrl != sourceURL {
			t.Errorf("Expected SourceUrl %s in response, got %v", sourceURL, resp.Msg.Experience.SourceUrl)
		}

		// Verify experience was stored with source URL
		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, resp.Msg.Experience.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored experience: %v", err)
		}

		if stored.SourceUrl == nil {
			t.Fatal("Expected SourceUrl to be set in stored experience")
		}

		if *stored.SourceUrl != sourceURL {
			t.Errorf("Expected stored SourceUrl %s, got %s", sourceURL, *stored.SourceUrl)
		}
	})

	t.Run("insert experience without source URL", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Manual Experience",
			Description: "Created without URL",
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		// Verify source URL is nil in the API response
		if resp.Msg.Experience.SourceUrl != nil {
			t.Errorf("Expected nil SourceUrl in response, got %v", resp.Msg.Experience.SourceUrl)
		}

		// Verify experience was stored without source URL
		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, resp.Msg.Experience.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored experience: %v", err)
		}

		if stored.SourceUrl != nil {
			t.Errorf("Expected nil SourceUrl in stored experience, got %s", *stored.SourceUrl)
		}
	})

	t.Run("does not auto-create a time proposal for specific time", func(t *testing.T) {
		// SaveExperience used to mint an initial TimeProposal mirroring
		// experience.time. We dropped that — proposals exist only as poll
		// options now (created via ProposeTime, which stamps a poll_id).
		// Otherwise the client's per-poll history list would surface a
		// "poll" the organizer never created.
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		specificTime := &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1700000000,
				},
			},
		}

		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Timed Experience",
			Description: "Event with specific time",
			Time:        specificTime,
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		proposals, err := testStorage.QueryByField(ctx, "experience_id", resp.Msg.Experience.Id, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query time proposals: %v", err)
		}

		if len(proposals) != 0 {
			t.Errorf("Expected 0 auto-created time proposals, got %d", len(proposals))
		}
	})

	t.Run("no proposal for TBD time", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "TBD Experience",
			Description: "Event with TBD time",
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		// Verify no time proposal was created
		proposals, err := testStorage.QueryByField(ctx, "experience_id", resp.Msg.Experience.Id, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query time proposals: %v", err)
		}

		if len(proposals) != 0 {
			t.Errorf("Expected 0 time proposals for TBD time, got %d", len(proposals))
		}
	})

	t.Run("no new proposal on update", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create experience with specific time
		specificTime := &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1700000000,
				},
			},
		}

		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Original Experience",
			Description: "Original description",
			Time:        specificTime,
		})

		createResp, err := service.SaveExperience(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience: %v", err)
		}

		expID := createResp.Msg.Experience.Id

		// Update with a different time
		newTime := &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: 1800000000,
				},
			},
		}

		updateReq := connect.NewRequest(&api.SaveExperienceRequest{
			Id:   &expID,
			Name: "Updated Experience",
			Time: newTime,
		})

		_, err = service.SaveExperience(ctx, updateReq)
		if err != nil {
			t.Fatalf("Failed to update experience: %v", err)
		}

		// SaveExperience never creates a TimeProposal (neither on initial
		// save nor on update). Proposals exist only as poll options.
		proposals, err := testStorage.QueryByField(ctx, "experience_id", expID, &models.ExperienceTimeProposal{})
		if err != nil {
			t.Fatalf("Failed to query time proposals: %v", err)
		}

		if len(proposals) != 0 {
			t.Errorf("Expected 0 time proposals, got %d", len(proposals))
		}
	})
}

func TestSaveExperience_PastTime(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "owner1", "owner@example.com", "Owner")

	pastTimestamp := int64(1000000000) // Unix 2001 — clearly in the past

	t.Run("past specific time sets IN_PROCESS state", func(t *testing.T) {
		ctx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Past Hike",
			Description: "A hike that already happened",
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: pastTimestamp,
					},
				},
			},
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		stored := &models.Experience{}
		if err := testStorage.GetByID(ctx, resp.Msg.Experience.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored experience: %v", err)
		}

		if stored.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
			t.Errorf("Expected state IN_PROCESS for past event, got %v", stored.State)
		}

		if stored.StartedAtUnixSec == nil {
			t.Error("Expected StartedAtUnixSec to be set for past event")
		}
	})

	t.Run("future specific time stays ACTIVE", func(t *testing.T) {
		ctx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

		futureTimestamp := int64(9999999999) // Far future
		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Future Hike",
			Description: "A hike that hasn't happened yet",
			Time: &api.ExperienceTime{
				TimeType: &api.ExperienceTime_Specific{
					Specific: &api.SpecificTime{
						UnixTimestampSec: futureTimestamp,
					},
				},
			},
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		stored := &models.Experience{}
		if err := testStorage.GetByID(ctx, resp.Msg.Experience.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored experience: %v", err)
		}

		if stored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE {
			t.Errorf("Expected state ACTIVE for future event, got %v", stored.State)
		}
	})

	t.Run("TBD time stays ACTIVE", func(t *testing.T) {
		ctx := createAuthenticatedContext("owner1", "owner@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "TBD Hike",
			Description: "Time not yet decided",
		})

		resp, err := service.SaveExperience(ctx, req)
		if err != nil {
			t.Fatalf("SaveExperience failed: %v", err)
		}

		stored := &models.Experience{}
		if err := testStorage.GetByID(ctx, resp.Msg.Experience.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored experience: %v", err)
		}

		if stored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE {
			t.Errorf("Expected state ACTIVE for TBD event, got %v", stored.State)
		}
	})
}

func TestIsPastTime(t *testing.T) {
	now := int64(1700000000)

	t.Run("specific time in the past returns true", func(t *testing.T) {
		et := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{
					UnixTimestampSec: now - 3600, // 1 hour ago
				},
			},
		}
		if !isPastTime(et, now) {
			t.Errorf("expected true for past specific time")
		}
	})

	t.Run("specific time in the future returns false", func(t *testing.T) {
		et := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{
					UnixTimestampSec: now + 3600, // 1 hour from now
				},
			},
		}
		if isPastTime(et, now) {
			t.Errorf("expected false for future specific time")
		}
	})

	t.Run("TBD time returns false", func(t *testing.T) {
		et := &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}},
		}
		if isPastTime(et, now) {
			t.Errorf("expected false for TBD time")
		}
	})

	t.Run("nil time returns false", func(t *testing.T) {
		if isPastTime(nil, now) {
			t.Errorf("expected false for nil time")
		}
	})
}

func TestNew(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	bucket := setupTestBucket(t)
	mockNotification := notifications.NewMockService()
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(sqlStorage, topic)
	service := New(sqlStorage, bucket, mockNotification, bus)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.storage != sqlStorage {
		t.Error("Expected storage to be set correctly")
	}

	if service.bucket != bucket {
		t.Error("Expected bucket to be set correctly")
	}

	if service.notificationService != mockNotification {
		t.Error("Expected notification service to be set correctly")
	}
}

func TestConvertExperienceState(t *testing.T) {
	tests := []struct {
		input    models.ExperienceState
		expected api.ExperienceState
	}{
		{models.ExperienceState_EXPERIENCE_STATE_ACTIVE, api.ExperienceState_EXPERIENCE_STATE_ACTIVE},
		{models.ExperienceState_EXPERIENCE_STATE_JOINED, api.ExperienceState_EXPERIENCE_STATE_JOINED},
		{models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS, api.ExperienceState_EXPERIENCE_STATE_IN_PROCESS},
		{models.ExperienceState_EXPERIENCE_STATE_COMPLETED, api.ExperienceState_EXPERIENCE_STATE_COMPLETED},
		{models.ExperienceState_EXPERIENCE_STATE_CANCELLED, api.ExperienceState_EXPERIENCE_STATE_CANCELLED},
	}

	for _, test := range tests {
		result := convertExperienceState(test.input)
		if result != test.expected {
			t.Errorf("convertExperienceState(%v) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

// TestSaveExperience_SuggestionsGeneratedOnCreate verifies that suggestion chips are
// generated by the AI provider and included in the response when a new experience is created.
func TestSaveExperience_SuggestionsGeneratedOnCreate(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "user-sug", "sug@example.com", "Sug User")

	mock := ai.NewMockProvider()
	mock.GenerateExperienceSuggestionsFunc = func(_ context.Context, name, _, _ string) (*ai.ExperienceSuggestionResult, error) {
		return &ai.ExperienceSuggestionResult{
			Suggestions:  []string{"bring snacks", "bring water", "set up area"},
			CategoryHint: "outdoor hike",
		}, nil
	}
	service.aiProvider = mock

	ctx := createAuthenticatedContext("user-sug", "sug@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Saturday Hike",
		Description: "A morning hike through the trails",
	})

	resp, err := service.SaveExperience(ctx, req)
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}

	// Suggestions should be included in the API response.
	if len(resp.Msg.Experience.Suggestions) != 3 {
		t.Errorf("Expected 3 suggestions in response, got %d", len(resp.Msg.Experience.Suggestions))
	}
	if resp.Msg.Experience.Suggestions[0] != "bring snacks" {
		t.Errorf("Expected first suggestion 'bring snacks', got %q", resp.Msg.Experience.Suggestions[0])
	}

	// Suggestions should also be persisted to storage.
	stored := &models.Experience{}
	if err := testStorage.GetByID(ctx, resp.Msg.Experience.Id, stored); err != nil {
		t.Fatalf("Failed to retrieve stored experience: %v", err)
	}
	if len(stored.Suggestions) != 3 {
		t.Errorf("Expected 3 suggestions persisted, got %d", len(stored.Suggestions))
	}
	if stored.CategoryHint == nil || *stored.CategoryHint != "outdoor hike" {
		t.Errorf("Expected category_hint 'outdoor hike', got %v", stored.CategoryHint)
	}
}

// TestSaveExperience_SuggestionFailureDoesNotBlockCreate verifies that a failure
// in suggestion generation does not prevent the experience from being created.
func TestSaveExperience_SuggestionFailureDoesNotBlockCreate(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "user-sug2", "sug2@example.com", "Sug User 2")

	mock := ai.NewMockProvider()
	mock.GenerateExperienceSuggestionsFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceSuggestionResult, error) {
		return nil, fmt.Errorf("LLM unavailable")
	}
	service.aiProvider = mock

	ctx := createAuthenticatedContext("user-sug2", "sug2@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Potluck Dinner",
		Description: "Bring a dish to share",
	})

	resp, err := service.SaveExperience(ctx, req)
	if err != nil {
		t.Fatalf("SaveExperience should succeed even when suggestions fail: %v", err)
	}
	if resp.Msg.Experience.Id == "" {
		t.Error("Expected non-empty experience ID")
	}
	// Suggestions should be empty (fallback path — no error propagated).
	if len(resp.Msg.Experience.Suggestions) != 0 {
		t.Errorf("Expected empty suggestions on failure, got %d", len(resp.Msg.Experience.Suggestions))
	}
}

// TestSaveExperience_SuggestionsAndImpactPersistedOnCreate verifies that both
// InferSocialAttributes and GenerateExperienceSuggestions results are included in
// the response and persisted to storage on create.
func TestSaveExperience_SuggestionsAndImpactPersistedOnCreate(t *testing.T) {
	service, testStorage, _ := setupTestService(t)
	createTestUser(t, testStorage, "user-async", "async@example.com", "Async User")

	mock := ai.NewMockProvider()
	mock.InferSocialAttributesFunc = func(_ context.Context, _, _, _ string, _ float32) (*ai.SocialAttributeInference, error) {
		return &ai.SocialAttributeInference{
			VulnerabilityLevel: "high",
			DurationMinutes:    90,
		}, nil
	}
	mock.GenerateExperienceSuggestionsFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceSuggestionResult, error) {
		return &ai.ExperienceSuggestionResult{
			Suggestions:  []string{"bring gear", "arrive early", "sign up"},
			CategoryHint: "outdoor sports",
		}, nil
	}
	service.aiProvider = mock

	ctx := createAuthenticatedContext("user-async", "async@example.com", models.Role_ROLE_USER)

	resp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
		Name:        "Climbing Session",
		Description: "A group rock climbing experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := resp.Msg.Experience.Id
	if expID == "" {
		t.Fatal("expected a valid experience ID in the response")
	}

	// Suggestions are returned in the response.
	if len(resp.Msg.Experience.Suggestions) != 3 {
		t.Errorf("expected 3 suggestions in response, got %d", len(resp.Msg.Experience.Suggestions))
	}

	// Verify both results are also persisted to storage.
	stored := &models.Experience{}
	if err := testStorage.GetByID(ctx, expID, stored); err != nil {
		t.Fatalf("failed to retrieve stored experience: %v", err)
	}
	if len(stored.Suggestions) != 3 {
		t.Errorf("expected 3 suggestions persisted, got %d", len(stored.Suggestions))
	}
	if stored.CategoryHint == nil || *stored.CategoryHint != "outdoor sports" {
		t.Errorf("expected category_hint 'outdoor sports', got %v", stored.CategoryHint)
	}
	if stored.ImpactEstimate == nil {
		t.Fatal("expected ImpactEstimate to be persisted")
	}
}
