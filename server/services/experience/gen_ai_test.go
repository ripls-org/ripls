package experience

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/webfetch"
)

func TestService_GenExperience(t *testing.T) {
	service, testStorage, bucket := setupTestService(t)

	// Setup mock AI provider
	mockProvider := newMockAIProvider(
		&ai.ExperienceGeneration{
			Title:       "Community Potluck",
			Description: "A neighborhood gathering to share food and stories",
			Confidence:  0.92,
		},
		&ai.ExperienceGeneration{
			Title:       "Summer Festival",
			Description: "Annual summer music and arts festival",
			Confidence:  0.88,
		},
		nil,
	)

	service.aiProvider = mockProvider

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	t.Run("generate from text prompt", func(t *testing.T) {
		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "neighborhood potluck dinner"},
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		if resp.Msg.Name != "Community Potluck" {
			t.Errorf("Expected name 'Community Potluck', got %s", resp.Msg.Name)
		}

		// In text mode, description is set to the user's original prompt text.
		if resp.Msg.Description != "neighborhood potluck dinner" {
			t.Errorf("Expected description to equal user prompt, got %s", resp.Msg.Description)
		}

		// Verify time defaults to TBD
		if resp.Msg.SuggestedTime == nil {
			t.Fatal("Expected SuggestedTime to be set")
		}
		if _, ok := resp.Msg.SuggestedTime.TimeType.(*api.ExperienceTime_Tbd); !ok {
			t.Error("Expected time type to be TBD")
		}

		// Verify location is empty
		if resp.Msg.LocationId != "" {
			t.Errorf("Expected empty location, got %s", resp.Msg.LocationId)
		}

		// Verify no media IDs for text mode
		if len(resp.Msg.MediaIds) != 0 {
			t.Errorf("Expected no media IDs for text mode, got %d", len(resp.Msg.MediaIds))
		}
	})

	t.Run("generate from media", func(t *testing.T) {
		// Create a test media item with actual file in bucket
		media := &models.Media{
			UserId:      "user123",
			ContentType: "image/jpeg",
		}
		mediaID, err := testStorage.Insert(ctx, media)
		if err != nil {
			t.Fatalf("Failed to create media: %v", err)
		}

		// Store fake image data in bucket so GetSignedURL can find it
		bucketKey := storage.MediaBucketKey("user123", mediaID)
		_, err = bucket.Put(ctx, bucketKey, []byte("fake image data"), "image/jpeg", map[string]string{"userId": "user123"})
		if err != nil {
			t.Fatalf("Failed to store media in bucket: %v", err)
		}

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_MediaId{MediaId: mediaID},
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		if resp.Msg.Name != "Summer Festival" {
			t.Errorf("Expected name 'Summer Festival', got %s", resp.Msg.Name)
		}

		// Verify media ID is included
		if len(resp.Msg.MediaIds) != 1 || resp.Msg.MediaIds[0] != mediaID {
			t.Errorf("Expected media ID %s in response, got %v", mediaID, resp.Msg.MediaIds)
		}
	})

	t.Run("require prompt type", func(t *testing.T) {
		req := connect.NewRequest(&api.GenExperienceRequest{})

		_, err := service.GenExperience(ctx, req)
		if err == nil {
			t.Fatal("Expected error when no prompt type provided")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("require authentication", func(t *testing.T) {
		unauthCtx := context.Background()
		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "test"},
		})

		_, err := service.GenExperience(unauthCtx, req)
		if err == nil {
			t.Fatal("Expected error when not authenticated")
		}
	})

	t.Run("verify media ownership", func(t *testing.T) {
		// Create media owned by different user
		otherMedia := &models.Media{
			UserId:      "other-user",
			ContentType: "image/jpeg",
		}
		otherMediaID, err := testStorage.Insert(ctx, otherMedia)
		if err != nil {
			t.Fatalf("Failed to create other user's media: %v", err)
		}

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_MediaId{MediaId: otherMediaID},
		})

		_, err = service.GenExperience(ctx, req)
		if err == nil {
			t.Fatal("Expected error when accessing other user's media")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("error when AI provider not configured", func(t *testing.T) {
		serviceNoAI, _, _ := setupTestService(t)
		// Don't set aiProvider

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "test"},
		})

		_, err := serviceNoAI.GenExperience(ctx, req)
		if err == nil {
			t.Fatal("Expected error when AI provider not configured")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeFailedPrecondition {
			t.Errorf("Expected FailedPrecondition error, got %v", connectErr.Code())
		}
	})

	t.Run("scene-based inference from food image", func(t *testing.T) {
		// Setup mock to return food scene type
		mockProvider := newMockAIProvider(
			nil,
			&ai.ExperienceGeneration{
				Title:          "Dinner party",
				Description:    "Planning a dinner party - would be great to get everyone together for a meal!",
				Confidence:     0.85,
				SearchKeywords: []string{"dinner", "gathering", "food"},
				// No specific time or location for scene inference
				TimeConfidence: "UNKNOWN",
				LocationQuery:  "",
			},
			nil,
		)
		service.aiProvider = mockProvider

		// Create test media
		media := &models.Media{
			UserId:      "user123",
			ContentType: "image/jpeg",
		}
		mediaID, err := testStorage.Insert(ctx, media)
		if err != nil {
			t.Fatalf("Failed to create media: %v", err)
		}

		bucketKey := storage.MediaBucketKey("user123", mediaID)
		_, err = bucket.Put(ctx, bucketKey, []byte("fake food image"), "image/jpeg", map[string]string{"userId": "user123"})
		if err != nil {
			t.Fatalf("Failed to store media: %v", err)
		}

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_MediaId{MediaId: mediaID},
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		if resp.Msg.Name != "Dinner party" {
			t.Errorf("Expected name 'Dinner party', got %s", resp.Msg.Name)
		}

		// Verify time is TBD (not hallucinated)
		if _, ok := resp.Msg.SuggestedTime.TimeType.(*api.ExperienceTime_Tbd); !ok {
			t.Error("Expected time type to be TBD for scene without explicit time")
		}

		// Verify location is empty (not hallucinated)
		if resp.Msg.LocationQuery != "" {
			t.Errorf("Expected empty location query for scene without location, got %s", resp.Msg.LocationQuery)
		}
	})

	t.Run("flyer extraction with explicit time and location", func(t *testing.T) {
		// Setup mock to return flyer with explicit details
		mockProvider := newMockAIProvider(
			nil,
			&ai.ExperienceGeneration{
				Title:          "Saturday morning yoga in the park",
				Description:    "Free community yoga class every Saturday at 9am at Central Park. Bring a mat and water!",
				Confidence:     0.95,
				SearchKeywords: []string{"yoga", "park", "outdoor fitness"},
				Date:           "2025-12-06",
				Time:           "09:00",
				TimeConfidence: "EXPLICIT",
				LocationQuery:  "Central Park",
			},
			nil,
		)
		service.aiProvider = mockProvider

		// Create test media
		media := &models.Media{
			UserId:      "user123",
			ContentType: "image/jpeg",
		}
		mediaID, err := testStorage.Insert(ctx, media)
		if err != nil {
			t.Fatalf("Failed to create media: %v", err)
		}

		bucketKey := storage.MediaBucketKey("user123", mediaID)
		_, err = bucket.Put(ctx, bucketKey, []byte("fake flyer image"), "image/jpeg", map[string]string{"userId": "user123"})
		if err != nil {
			t.Fatalf("Failed to store media: %v", err)
		}

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt:             &api.GenExperienceRequest_MediaId{MediaId: mediaID},
			CurrentTimeUnixSec: 1732996800, // 2025-11-30
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify extracted time (not TBD)
		if resp.Msg.ExtractedTimeUnixSec == 0 {
			t.Error("Expected extracted time from flyer to be set")
		}

		// Verify location query extracted
		if resp.Msg.LocationQuery != "Central Park" {
			t.Errorf("Expected location query 'Central Park', got %s", resp.Msg.LocationQuery)
		}

		// Verify time confidence is EXPLICIT
		if resp.Msg.TimeConfidence != api.TimeConfidence_TIME_CONFIDENCE_EXPLICIT {
			t.Errorf("Expected EXPLICIT time confidence for flyer, got %v", resp.Msg.TimeConfidence)
		}
	})

	t.Run("text prompt without location or time should not hallucinate", func(t *testing.T) {
		// Setup mock to return conservative response without inventing details
		mockProvider := newMockAIProvider(
			&ai.ExperienceGeneration{
				Title:          "Picnic hangout",
				Description:    "Thinking about having a picnic - we can figure out when and where works best!",
				Confidence:     0.70,
				SearchKeywords: []string{"picnic", "outdoor", "gathering"},
				// No date, time, or location
				TimeConfidence: "UNKNOWN",
				LocationQuery:  "",
			},
			nil,
			nil,
		)
		service.aiProvider = mockProvider

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "picnic with friends"},
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify title doesn't include invented location or time
		if resp.Msg.Name != "Picnic hangout" {
			t.Errorf("Expected generic title without location/time, got %s", resp.Msg.Name)
		}

		// Verify description is welcoming but not specific
		if resp.Msg.Description == "" {
			t.Error("Expected description to be set")
		}

		// Verify time is TBD
		if _, ok := resp.Msg.SuggestedTime.TimeType.(*api.ExperienceTime_Tbd); !ok {
			t.Error("Expected time type to be TBD when not mentioned in prompt")
		}

		// Verify location is empty
		if resp.Msg.LocationQuery != "" {
			t.Errorf("Expected empty location query when not mentioned, got %s", resp.Msg.LocationQuery)
		}

		// Verify time confidence is UNKNOWN
		if resp.Msg.TimeConfidence != api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN {
			t.Errorf("Expected UNKNOWN time confidence, got %v", resp.Msg.TimeConfidence)
		}
	})

	t.Run("generate from website URL", func(t *testing.T) {
		// Setup mock web fetcher
		mockFetcher := &webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:         url,
					Title:       "Summer Jazz Festival 2025 - Downtown Arts District",
					Description: "Join us for three nights of amazing jazz music featuring local and international artists.",
					BodyText: `Summer Jazz Festival 2025
Downtown Arts District presents the 15th Annual Summer Jazz Festival!

Date: July 18-20, 2025
Time: Gates open at 5pm, music starts at 6pm
Location: Main Street Plaza, Downtown Arts District

Featuring:
- Friday: Local Jazz Ensemble & Friends
- Saturday: Grammy-nominated artist special performance
- Sunday: Community Jazz Jam

Tickets:
- Single day: $35
- Weekend pass: $85
- VIP package: $150 (includes meet & greet)

Food trucks and craft beverages will be available.
Rain or shine event - bring blankets and lawn chairs!

Contact: info@summerjazzfest.example.com`,
				}, nil
			},
		}
		service.SetWebFetcher(mockFetcher)

		// Setup mock AI provider for webpage response
		mockProvider := newMockAIProvider(
			nil,
			nil,
			&ai.ExperienceGeneration{
				Title:          "Summer Jazz Festival 2025",
				Description:    "Three nights of amazing jazz music at Main Street Plaza featuring local and international artists. Gates open at 5pm, music starts at 6pm. Food trucks and craft beverages available.",
				Confidence:     0.95,
				SearchKeywords: []string{"jazz", "festival", "music", "outdoor"},
				Date:           "2025-07-18",
				Time:           "17:00",
				TimeConfidence: "EXPLICIT",
				LocationQuery:  "Main Street Plaza, Downtown Arts District",
			},
		)
		service.aiProvider = mockProvider

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt:             &api.GenExperienceRequest_WebsiteUrl{WebsiteUrl: "https://summerjazzfest.example.com/2025"},
			CurrentTimeUnixSec: 1735689600, // 2025-01-01
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify extracted details
		if resp.Msg.Name != "Summer Jazz Festival 2025" {
			t.Errorf("Expected name 'Summer Jazz Festival 2025', got %s", resp.Msg.Name)
		}

		if resp.Msg.Description == "" {
			t.Error("Expected description to be set")
		}

		// Verify time confidence is EXPLICIT
		if resp.Msg.TimeConfidence != api.TimeConfidence_TIME_CONFIDENCE_EXPLICIT {
			t.Errorf("Expected EXPLICIT time confidence for webpage event, got %v", resp.Msg.TimeConfidence)
		}

		// Verify location query extracted
		if resp.Msg.LocationQuery != "Main Street Plaza, Downtown Arts District" {
			t.Errorf("Expected location query 'Main Street Plaza, Downtown Arts District', got %s", resp.Msg.LocationQuery)
		}

		// Verify source URL is returned for storage
		if resp.Msg.SourceUrl != "https://summerjazzfest.example.com/2025" {
			t.Errorf("Expected SourceUrl 'https://summerjazzfest.example.com/2025', got %s", resp.Msg.SourceUrl)
		}

		// Verify mock fetcher was called
		if len(mockFetcher.FetchPageContentCalls) != 1 {
			t.Errorf("Expected 1 fetch call, got %d", len(mockFetcher.FetchPageContentCalls))
		}
		if mockFetcher.FetchPageContentCalls[0] != "https://summerjazzfest.example.com/2025" {
			t.Errorf("Unexpected URL fetched: %s", mockFetcher.FetchPageContentCalls[0])
		}
	})

	t.Run("generate from text prompt returns empty source URL", func(t *testing.T) {
		// Reset to text prompt mock
		mockProvider := newMockAIProvider(
			&ai.ExperienceGeneration{
				Title:       "Community Potluck",
				Description: "A neighborhood gathering to share food and stories",
				Confidence:  0.92,
			},
			nil,
			nil,
		)
		service.aiProvider = mockProvider

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_Text{Text: "neighborhood potluck dinner"},
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify source URL is empty for text prompts
		if resp.Msg.SourceUrl != "" {
			t.Errorf("Expected empty SourceUrl for text prompt, got %s", resp.Msg.SourceUrl)
		}
	})

	t.Run("error when web fetcher not configured", func(t *testing.T) {
		serviceNoFetcher, _, _ := setupTestService(t)
		serviceNoFetcher.aiProvider = mockProvider
		// Don't set webFetcher

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_WebsiteUrl{WebsiteUrl: "https://example.com/event"},
		})

		_, err := serviceNoFetcher.GenExperience(ctx, req)
		if err == nil {
			t.Fatal("Expected error when web fetcher not configured")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeFailedPrecondition {
			t.Errorf("Expected FailedPrecondition error, got %v", connectErr.Code())
		}
	})

	t.Run("reject empty website URL", func(t *testing.T) {
		service.SetWebFetcher(&webfetch.MockFetcher{})

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt: &api.GenExperienceRequest_WebsiteUrl{WebsiteUrl: ""},
		})

		_, err := service.GenExperience(ctx, req)
		if err == nil {
			t.Fatal("Expected error when website URL is empty")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})
}

func TestGenExperience_WebpageMode_WithImage(t *testing.T) {
	// A small, decodable JPEG. The webpage-image upload path now sanitizes
	// (decodes + re-encodes) the fetched image, so it must be genuinely decodable.
	validJPEG := realTestJPEG()

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	t.Run("webpage with og:image downloads and uses that image", func(t *testing.T) {
		service, _, _ := setupTestService(t)
		// Inject plain client so the loopback httptest image server is reachable.
		service.imageDownloadClient = plainHTTPClientFactory

		// Setup mock AI provider
		mockProvider := newMockAIProvider(
			nil,
			nil,
			&ai.ExperienceGeneration{
				Title:          "Summer Jazz Festival 2025",
				Description:    "Three nights of amazing jazz music at Main Street Plaza",
				Confidence:     0.95,
				SearchKeywords: []string{"jazz", "festival", "music"},
				Date:           "2025-07-18",
				Time:           "17:00",
				TimeConfidence: "EXPLICIT",
				LocationQuery:  "Main Street Plaza",
			},
		)
		service.aiProvider = mockProvider

		// Create a test image server
		imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(validJPEG)
		}))
		defer imageServer.Close()

		// Setup mock web fetcher that returns an image URL
		mockFetcher := &webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:         url,
					Title:       "Summer Jazz Festival 2025",
					Description: "Join us for three nights of amazing jazz music",
					BodyText:    "Summer Jazz Festival 2025 - July 18-20 at Main Street Plaza",
					ImageURL:    imageServer.URL + "/event-image.jpg",
				}, nil
			},
		}
		service.SetWebFetcher(mockFetcher)

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt:             &api.GenExperienceRequest_WebsiteUrl{WebsiteUrl: "https://summerjazzfest.example.com/2025"},
			CurrentTimeUnixSec: 1735689600,
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify webpage image was downloaded and used
		if len(resp.Msg.MediaIds) == 0 {
			t.Error("Expected media_ids to contain the downloaded webpage image")
		} else {
			t.Logf("Webpage image downloaded with media_id: %s", resp.Msg.MediaIds[0])
		}

		// Verify the fetcher was called
		if len(mockFetcher.FetchPageContentCalls) != 1 {
			t.Errorf("Expected 1 fetch call, got %d", len(mockFetcher.FetchPageContentCalls))
		}
	})

	t.Run("webpage without image falls back to stock imagery", func(t *testing.T) {
		service, testStorage, testBucket, fakeProvider, _ := setupTestServiceWithStockImagery(t)

		// Setup mock AI provider
		mockProvider := newMockAIProvider(
			nil,
			nil,
			&ai.ExperienceGeneration{
				Title:          "Summer Jazz Festival 2025",
				Description:    "Three nights of amazing jazz music at Main Street Plaza",
				Confidence:     0.95,
				SearchKeywords: []string{"jazz", "festival", "music"},
				Date:           "2025-07-18",
				Time:           "17:00",
				TimeConfidence: "EXPLICIT",
				LocationQuery:  "Main Street Plaza",
			},
		)
		service.aiProvider = mockProvider

		// Create a test user
		createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
		_ = testBucket // Keep reference to avoid unused warning
		_ = fakeProvider

		// Setup mock fetcher without image URL
		mockFetcher := &webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:         url,
					Title:       "Summer Jazz Festival 2025",
					Description: "Join us for three nights of amazing jazz music",
					BodyText:    "Summer Jazz Festival 2025 - July 18-20 at Main Street Plaza",
					ImageURL:    "", // No image
				}, nil
			},
		}
		service.SetWebFetcher(mockFetcher)

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt:             &api.GenExperienceRequest_WebsiteUrl{WebsiteUrl: "https://summerjazzfest.example.com/2025"},
			CurrentTimeUnixSec: 1735689600,
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify stock image was used as fallback
		if len(resp.Msg.MediaIds) == 0 {
			t.Error("Expected media_ids to contain the stock image when no webpage image available")
		} else {
			t.Logf("Stock image used with media_id: %s", resp.Msg.MediaIds[0])
		}
	})

	t.Run("failed webpage image download falls back to stock imagery", func(t *testing.T) {
		service, testStorage, testBucket, fakeProvider, _ := setupTestServiceWithStockImagery(t)

		// Setup mock AI provider
		mockProvider := newMockAIProvider(
			nil,
			nil,
			&ai.ExperienceGeneration{
				Title:          "Summer Jazz Festival 2025",
				Description:    "Three nights of amazing jazz music at Main Street Plaza",
				Confidence:     0.95,
				SearchKeywords: []string{"jazz", "festival", "music"},
				Date:           "2025-07-18",
				Time:           "17:00",
				TimeConfidence: "EXPLICIT",
				LocationQuery:  "Main Street Plaza",
			},
		)
		service.aiProvider = mockProvider

		// Create a test user
		createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
		_ = testBucket // Keep reference to avoid unused warning
		_ = fakeProvider

		// Create a test image server that returns 404
		imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer imageServer.Close()

		// Setup mock fetcher with bad image URL
		mockFetcher := &webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:         url,
					Title:       "Summer Jazz Festival 2025",
					Description: "Join us for three nights of amazing jazz music",
					BodyText:    "Summer Jazz Festival 2025 - July 18-20 at Main Street Plaza",
					ImageURL:    imageServer.URL + "/broken-image.jpg", // Will return 404
				}, nil
			},
		}
		service.SetWebFetcher(mockFetcher)

		req := connect.NewRequest(&api.GenExperienceRequest{
			Prompt:             &api.GenExperienceRequest_WebsiteUrl{WebsiteUrl: "https://summerjazzfest.example.com/2025"},
			CurrentTimeUnixSec: 1735689600,
		})

		resp, err := service.GenExperience(ctx, req)
		if err != nil {
			t.Fatalf("GenExperience failed: %v", err)
		}

		// Verify stock image was used as fallback after download failure
		if len(resp.Msg.MediaIds) == 0 {
			t.Error("Expected media_ids to contain the stock image when webpage image download failed")
		}
	})
}
