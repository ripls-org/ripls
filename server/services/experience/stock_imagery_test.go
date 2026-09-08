package experience

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

// setupTestServiceWithStockImagery creates an experience service with stock imagery provider for testing.
func setupTestServiceWithStockImagery(t *testing.T) (*Service, *storage.ProtoSQLStorage, storage.BucketStorage, *media.FakeProvider, *ai.MockProvider) {
	testStorage := setupTestStorage(t)
	testBucket := setupTestBucket(t)

	mockNotif := notifications.NewMockService()
	mockAIProvider := ai.NewMockProvider()

	// Create FakeProvider for stock imagery
	fakeProvider := media.NewFakeProvider(testStorage, testBucket)

	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(testStorage, topic)
	service := New(testStorage, testBucket, mockNotif, bus)
	service.SetAIProvider(mockAIProvider)
	service.SetStockImageryProvider(fakeProvider)

	return service, testStorage, testBucket, fakeProvider, mockAIProvider
}

func TestGenExperience_TextMode_WithStockImage(t *testing.T) {
	service, testStorage, _, _, mockAI := setupTestServiceWithStockImagery(t)

	// Setup test user
	userID := "user123"
	createTestUser(t, testStorage, userID, "user@example.com", "Test User")

	// Configure mock AI to return search keywords
	mockAI.GenerateExperienceFromTextFunc = func(ctx context.Context, prompt, region, currentTime string) (*ai.ExperienceGeneration, error) {
		return &ai.ExperienceGeneration{
			Title:          "Weekly Basketball Pickup Games",
			Description:    "Join us for friendly basketball games! All skill levels welcome.",
			Confidence:     0.95,
			SearchKeywords: []string{"basketball court", "outdoor sports", "pickup game"},
		}, nil
	}

	// Generate experience from text prompt (should trigger stock image fetch)
	ctx := createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)
	genReq := connect.NewRequest(&api.GenExperienceRequest{
		Prompt: &api.GenExperienceRequest_Text{Text: "weekly basketball at the park"},
	})

	genResp, err := service.GenExperience(ctx, genReq)
	if err != nil {
		t.Fatalf("GenExperience failed: %v", err)
	}

	// Verify response contains the AI-generated content
	if genResp.Msg.Name != "Weekly Basketball Pickup Games" {
		t.Errorf("Expected name 'Weekly Basketball Pickup Games', got %q", genResp.Msg.Name)
	}

	// Verify a media_id was attached from stock imagery
	if len(genResp.Msg.MediaIds) == 0 {
		t.Error("Expected media_ids to contain stock image, but it was empty")
	} else {
		t.Logf("Generated experience has media_id: %s", genResp.Msg.MediaIds[0])

		// Verify the media was actually stored in the database
		storedMedia := &models.Media{}
		err = testStorage.GetByID(context.Background(), genResp.Msg.MediaIds[0], storedMedia)
		if err != nil {
			t.Fatalf("Failed to get media: %v", err)
		}

		if storedMedia.ContentType != "image/jpeg" {
			t.Errorf("Expected content type 'image/jpeg', got %q", storedMedia.ContentType)
		}

		if storedMedia.UserId != userID {
			t.Errorf("Expected user_id %q for stock images, got %q", userID, storedMedia.UserId)
		}
	}
}

func TestGenExperience_CameraMode_SkipsStockImage(t *testing.T) {
	service, testStorage, bucket, _, mockAI := setupTestServiceWithStockImagery(t)

	// Setup test user
	userID := "user123"
	createTestUser(t, testStorage, userID, "user@example.com", "Test User")

	// Create a media record for the uploaded flyer image
	mediaRecord := &models.Media{
		UserId:      userID,
		ContentType: "image/jpeg",
		Filename:    proto.String("flyer.jpg"),
		SizeBytes:   1024,
		StorageUrl:  "http://localhost:8080/bucket/media/test-media-id",
	}
	mediaID, err := testStorage.Insert(context.Background(), mediaRecord)
	if err != nil {
		t.Fatalf("Failed to insert media: %v", err)
	}

	// Store fake image data in bucket so GetSignedURL can find it
	ctx := context.Background()
	bucketKey := storage.MediaBucketKey(userID, mediaID)
	_, err = bucket.Put(ctx, bucketKey, []byte("fake image data"), "image/jpeg", map[string]string{"userId": userID})
	if err != nil {
		t.Fatalf("Failed to store media in bucket: %v", err)
	}

	// Configure mock AI to return experience from image
	mockAI.GenerateExperienceFromImageFunc = func(ctx context.Context, image *ai.DetectionImage, region, notes, currentTime string) (*ai.ExperienceGeneration, error) {
		return &ai.ExperienceGeneration{
			Title:          "Community Potluck Dinner",
			Description:    "Monthly gathering with homemade dishes",
			Confidence:     0.90,
			SearchKeywords: []string{}, // Keywords not needed for camera mode
		}, nil
	}

	// Generate experience from media (camera mode - should NOT trigger stock image fetch)
	ctx = createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)
	genReq := connect.NewRequest(&api.GenExperienceRequest{
		Prompt: &api.GenExperienceRequest_MediaId{MediaId: mediaID},
	})

	genResp, err := service.GenExperience(ctx, genReq)
	if err != nil {
		t.Fatalf("GenExperience failed: %v", err)
	}

	// Verify response contains the uploaded media_id (not stock image)
	if len(genResp.Msg.MediaIds) != 1 {
		t.Errorf("Expected exactly 1 media_id, got %d", len(genResp.Msg.MediaIds))
	}

	if genResp.Msg.MediaIds[0] != mediaID {
		t.Errorf("Expected media_id %q, got %q", mediaID, genResp.Msg.MediaIds[0])
	}
}

func TestGenExperience_TextMode_NoKeywords_SkipsStockImage(t *testing.T) {
	service, testStorage, _, _, mockAI := setupTestServiceWithStockImagery(t)

	// Setup test user
	userID := "user123"
	createTestUser(t, testStorage, userID, "user@example.com", "Test User")

	// Configure mock AI to return NO search keywords
	mockAI.GenerateExperienceFromTextFunc = func(ctx context.Context, prompt, region, currentTime string) (*ai.ExperienceGeneration, error) {
		return &ai.ExperienceGeneration{
			Title:          "Casual Community Hangout",
			Description:    "Let's connect in a relaxed setting",
			Confidence:     0.60,
			SearchKeywords: []string{}, // Empty keywords
		}, nil
	}

	// Generate experience from text prompt
	ctx := createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)
	genReq := connect.NewRequest(&api.GenExperienceRequest{
		Prompt: &api.GenExperienceRequest_Text{Text: "hang out"},
	})

	genResp, err := service.GenExperience(ctx, genReq)
	if err != nil {
		t.Fatalf("GenExperience failed: %v", err)
	}

	// Verify no media_id was attached (no keywords = no stock image fetch)
	if len(genResp.Msg.MediaIds) != 0 {
		t.Errorf("Expected no media_ids without keywords, but got %d", len(genResp.Msg.MediaIds))
	}
}

func TestGenExperience_TextMode_StockImageFailure_ContinuesWithoutImage(t *testing.T) {
	testStorage := setupTestStorage(t)
	testBucket := setupTestBucket(t)

	mockNotif := notifications.NewMockService()
	mockAIProvider := ai.NewMockProvider()

	// Create FakeProvider configured to simulate errors
	fakeProvider := media.NewFakeProviderWithConfig(testStorage, testBucket, media.FakeProviderConfig{
		SimulateError: fmt.Errorf("simulated error"),
	})

	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(testStorage, topic)
	service := New(testStorage, testBucket, mockNotif, bus)
	service.SetAIProvider(mockAIProvider)
	service.SetStockImageryProvider(fakeProvider)

	// Setup test user
	userID := "user123"
	createTestUser(t, testStorage, userID, "user@example.com", "Test User")

	// Configure mock AI to return search keywords
	mockAIProvider.GenerateExperienceFromTextFunc = func(ctx context.Context, prompt, region, currentTime string) (*ai.ExperienceGeneration, error) {
		return &ai.ExperienceGeneration{
			Title:          "Pottery Workshop",
			Description:    "Learn basic clay techniques",
			Confidence:     0.85,
			SearchKeywords: []string{"pottery wheel", "ceramic art"},
		}, nil
	}

	// Generate experience from text prompt
	ctx := createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)
	genReq := connect.NewRequest(&api.GenExperienceRequest{
		Prompt: &api.GenExperienceRequest_Text{Text: "learn pottery"},
	})

	genResp, err := service.GenExperience(ctx, genReq)
	if err != nil {
		t.Fatalf("GenExperience should succeed even when stock imagery fails, but got error: %v", err)
	}

	// Verify response contains the AI-generated content (image is optional)
	if genResp.Msg.Name != "Pottery Workshop" {
		t.Errorf("Expected name 'Pottery Workshop', got %q", genResp.Msg.Name)
	}

	// Verify no media_id (stock imagery failed gracefully)
	if len(genResp.Msg.MediaIds) != 0 {
		t.Errorf("Expected no media_ids when stock imagery fails, but got %d", len(genResp.Msg.MediaIds))
	}
}
