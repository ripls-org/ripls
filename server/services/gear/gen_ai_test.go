package gear

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/webfetch"
)

// TestService_GenGear tests the unified AI gear generation functionality for both text and image modes.
func TestService_GenGear(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	t.Run("generate from text prompt", func(t *testing.T) {
		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "camping tent",
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear failed: %v", err)
		}

		if resp.Msg.DetectedGear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		gear := resp.Msg.DetectedGear

		// Verify AI-generated title is present
		if gear.Title == "" {
			t.Error("Expected non-empty title from AI generation")
		}

		// Verify AI-generated description is present
		if gear.Description == "" {
			t.Error("Expected non-empty description from AI generation")
		}

		// Verify confidence score is reasonable
		if gear.Confidence < 0.0 || gear.Confidence > 1.0 {
			t.Errorf("Expected confidence between 0.0 and 1.0, got %f", gear.Confidence)
		}

		t.Logf("Generated gear: title=%q, description=%q, confidence=%f",
			gear.Title, gear.Description, gear.Confidence)
	})

	t.Run("generate from media", func(t *testing.T) {
		// Create test media
		testMedia := &models.Media{
			UserId:      "user123",
			StorageUrl:  "mock://bucket/media/user123/test123",
			ContentType: "image/jpeg",
			Filename:    proto.String("test.jpg"),
			SizeBytes:   1024,
		}

		mediaID, err := testStorage.Insert(ctx, testMedia)
		if err != nil {
			t.Fatalf("Failed to insert test media: %v", err)
		}

		req := connect.NewRequest(&api.GenGearRequest{
			MediaId: mediaID,
		})

		resp, err := service.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear from media failed: %v", err)
		}

		// Verify detected_gear field is populated
		if resp.Msg.DetectedGear == nil {
			t.Fatal("Expected DetectedGear to be populated")
		}

		gear := resp.Msg.DetectedGear
		if gear.Title == "" {
			t.Error("Expected non-empty title from image detection")
		}
		if gear.Confidence < 0.0 || gear.Confidence > 1.0 {
			t.Errorf("Expected confidence between 0.0 and 1.0, got %f", gear.Confidence)
		}

		// Verify media ID is included in response
		if len(gear.MediaIds) != 1 || gear.MediaIds[0] != mediaID {
			t.Errorf("Expected media ID %s in response, got %v", mediaID, gear.MediaIds)
		}
	})

	t.Run("require prompt or media_id", func(t *testing.T) {
		req := connect.NewRequest(&api.GenGearRequest{})

		_, err := service.GenGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error when neither prompt nor media provided")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("generate from URL in prompt without webFetcher returns error", func(t *testing.T) {
		// Without webFetcher configured, URL mode should return FailedPrecondition
		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "https://example.com/product",
		})

		_, err := service.GenGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error when webFetcher not configured")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeFailedPrecondition {
			t.Errorf("Expected FailedPrecondition error, got %v", connectErr.Code())
		}
	})

	t.Run("generate from URL in prompt with webFetcher", func(t *testing.T) {
		// Create service with webFetcher configured
		serviceWithFetcher := New(testStorage, mockBucket)
		serviceWithFetcher.SetAIProvider(ai.NewMockProvider())
		serviceWithFetcher.SetWebFetcher(&webfetch.MockFetcher{
			FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
				return &webfetch.PageContent{
					URL:         url,
					Title:       "DeWalt 20V MAX Cordless Drill",
					Description: "Professional-grade cordless drill with lithium-ion battery",
					BodyText:    "Features: 20V MAX battery, 1/2 inch chuck, 2-speed transmission",
					ImageURL:    "", // No image to simplify test
				}, nil
			},
		})

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "https://example.com/product/dewalt-drill",
		})

		resp, err := serviceWithFetcher.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("GenGear from URL failed: %v", err)
		}

		if resp.Msg.DetectedGear == nil {
			t.Fatal("Expected DetectedGear to be non-nil")
		}

		gear := resp.Msg.DetectedGear

		// Verify source_url is set
		if gear.SourceUrl != "https://example.com/product/dewalt-drill" {
			t.Errorf("Expected source_url to be set, got %q", gear.SourceUrl)
		}

		// Verify AI-generated content is present
		if gear.Title == "" {
			t.Error("Expected non-empty title from URL generation")
		}

		t.Logf("Generated gear from URL: title=%q, source_url=%q", gear.Title, gear.SourceUrl)
	})

	t.Run("no detection returns nil detected_gear", func(t *testing.T) {
		// Create test media
		testMedia := &models.Media{
			UserId:      "user123",
			StorageUrl:  "mock://bucket/media/user123/no-detection",
			ContentType: "image/jpeg",
			Filename:    proto.String("no-gear.jpg"),
			SizeBytes:   1024,
		}

		mediaID, err := testStorage.Insert(ctx, testMedia)
		if err != nil {
			t.Fatalf("Failed to insert test media: %v", err)
		}

		// Create service with AI provider that returns nil detection
		mockProvider := ai.NewMockProvider()
		mockProvider.DetectGearFunc = func(_ context.Context, _ *ai.DetectionImage) (*ai.GearDetection, error) {
			return nil, nil
		}

		serviceNoDetection := New(testStorage, mockBucket)
		serviceNoDetection.SetAIProvider(mockProvider)

		req := connect.NewRequest(&api.GenGearRequest{
			MediaId: mediaID,
		})

		resp, err := serviceNoDetection.GenGear(ctx, req)
		if err != nil {
			t.Fatalf("Expected success even with no detection, got error: %v", err)
		}

		// Verify detected_gear is nil when nothing detected
		if resp.Msg.DetectedGear != nil {
			t.Errorf("Expected nil DetectedGear when nothing detected, got: %+v", resp.Msg.DetectedGear)
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		unauthCtx := context.Background()
		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "test",
		})

		_, err := service.GenGear(unauthCtx, req)
		if err == nil {
			t.Fatal("Expected error when not authenticated")
		}
	})

	t.Run("verify media ownership", func(t *testing.T) {
		// Create media owned by different user
		otherMedia := &models.Media{
			UserId:      "other-user",
			StorageUrl:  "mock://bucket/media/other-user/test",
			ContentType: "image/jpeg",
			Filename:    proto.String("other.jpg"),
			SizeBytes:   1024,
		}
		otherMediaID, err := testStorage.Insert(ctx, otherMedia)
		if err != nil {
			t.Fatalf("Failed to create other user's media: %v", err)
		}

		req := connect.NewRequest(&api.GenGearRequest{
			MediaId: otherMediaID,
		})

		_, err = service.GenGear(ctx, req)
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

	t.Run("no AI provider configured", func(t *testing.T) {
		serviceNoAI := New(testStorage, mockBucket)
		// Don't set aiProvider

		req := connect.NewRequest(&api.GenGearRequest{
			Prompt: "test",
		})

		_, err := serviceNoAI.GenGear(ctx, req)
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
}

// TestService_GenGear_MinimalPrompt tests generation with a minimal prompt.
func TestService_GenGear_MinimalPrompt(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenGearRequest{
		Prompt: "saw",
	})

	resp, err := service.GenGear(ctx, req)
	if err != nil {
		t.Fatalf("GenGear failed: %v", err)
	}

	if resp.Msg.DetectedGear.Title == "" {
		t.Error("Expected title even with minimal prompt")
	}

	if resp.Msg.DetectedGear.Description == "" {
		t.Error("Expected description even with minimal prompt")
	}

	t.Logf("From minimal prompt %q, generated: title=%q, description=%q",
		req.Msg.Prompt, resp.Msg.DetectedGear.Title, resp.Msg.DetectedGear.Description)
}

// TestService_GenGear_DetailedPrompt tests generation with a detailed prompt.
func TestService_GenGear_DetailedPrompt(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenGearRequest{
		Prompt: "DeWalt cordless power drill with 20V battery and charger, includes drill bits",
	})

	resp, err := service.GenGear(ctx, req)
	if err != nil {
		t.Fatalf("GenGear failed: %v", err)
	}

	if resp.Msg.DetectedGear.Title == "" {
		t.Error("Expected title to capture main item")
	}

	if resp.Msg.DetectedGear.Description == "" {
		t.Error("Expected description to preserve details from prompt")
	}

	t.Logf("From detailed prompt, generated: title=%q, description=%q",
		resp.Msg.DetectedGear.Title, resp.Msg.DetectedGear.Description)
}

// TestService_GenGear_NoInput tests validation when no input is provided.
func TestService_GenGear_NoInput(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenGearRequest{
		// No input set
	})

	_, err := service.GenGear(ctx, req)
	if err == nil {
		t.Fatal("Expected error for no input, got nil")
	}

	// Verify it's an InvalidArgument error
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected CodeInvalidArgument, got %v", connectErr.Code())
	}

	// No input triggers "either prompt or media_id must be provided" error
	if !strings.Contains(connectErr.Message(), "either prompt or media_id must be provided") {
		t.Errorf("Expected message about missing input, got: %s", connectErr.Message())
	}
}

// TestService_GenGear_TooShortPrompt tests validation of too-short prompt.
func TestService_GenGear_TooShortPrompt(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenGearRequest{
		Prompt: "ab", // Only 2 characters
	})

	_, err := service.GenGear(ctx, req)
	if err == nil {
		t.Fatal("Expected error for too-short prompt, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected CodeInvalidArgument, got %v", connectErr.Code())
	}

	if !strings.Contains(connectErr.Message(), "at least 3 characters") {
		t.Errorf("Expected message about minimum length, got: %s", connectErr.Message())
	}
}

// TestService_GenGear_TooLongPrompt tests validation of too-long prompt.
func TestService_GenGear_TooLongPrompt(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	// Create a prompt that exceeds 500 characters
	longPrompt := strings.Repeat("a", 501)

	req := connect.NewRequest(&api.GenGearRequest{
		Prompt: longPrompt,
	})

	_, err := service.GenGear(ctx, req)
	if err == nil {
		t.Fatal("Expected error for too-long prompt, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected CodeInvalidArgument, got %v", connectErr.Code())
	}

	if !strings.Contains(connectErr.Message(), "not exceed 500 characters") {
		t.Errorf("Expected message about maximum length, got: %s", connectErr.Message())
	}
}

// TestService_GenGear_Unauthenticated tests authentication requirement.
func TestService_GenGear_Unauthenticated(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	// Create unauthenticated context
	ctx := context.Background()

	req := connect.NewRequest(&api.GenGearRequest{
		Prompt: "camping tent",
	})

	_, err := service.GenGear(ctx, req)
	if err == nil {
		t.Fatal("Expected error for unauthenticated request, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("Expected CodeUnauthenticated, got %v", connectErr.Code())
	}
}

// TestService_GenGear_NoAIProvider tests error when AI provider not configured.
func TestService_GenGear_NoAIProvider(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	// Don't set AI provider

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenGearRequest{
		Prompt: "camping tent",
	})

	_, err := service.GenGear(ctx, req)
	if err == nil {
		t.Fatal("Expected error when AI provider not configured, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeFailedPrecondition {
		t.Errorf("Expected CodeFailedPrecondition, got %v", connectErr.Code())
	}
}

// TestService_GenGear_NoDatabase tests that generation works without database access for text mode.
func TestService_GenGear_NoDatabase(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	// Use a user ID that doesn't exist in the database
	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenGearRequest{
		Prompt: "camping tent",
	})

	// Should succeed even though user doesn't exist (for text mode)
	resp, err := service.GenGear(ctx, req)
	if err != nil {
		t.Fatalf("GenGear should work without database user, got error: %v", err)
	}

	if resp.Msg.DetectedGear == nil {
		t.Fatal("Expected DetectedGear to be populated")
	}
}

// TestService_GenGear_VariousPromptTypes tests different types of prompts.
func TestService_GenGear_VariousPromptTypes(t *testing.T) {
	testCases := []struct {
		name        string
		prompt      string
		expectError bool
	}{
		{
			name:        "Power tool",
			prompt:      "cordless drill",
			expectError: false,
		},
		{
			name:        "Camping equipment",
			prompt:      "4-person tent",
			expectError: false,
		},
		{
			name:        "Sporting goods",
			prompt:      "mountain bike",
			expectError: false,
		},
		{
			name:        "Kitchen appliance",
			prompt:      "stand mixer",
			expectError: false,
		},
		{
			name:        "Gardening tool",
			prompt:      "lawn mower",
			expectError: false,
		},
		{
			name:        "Brand and model",
			prompt:      "DeWalt 20V MAX drill",
			expectError: false,
		},
		{
			name:        "With accessories",
			prompt:      "tent with stakes and rain fly",
			expectError: false,
		},
		{
			name:        "Too short",
			prompt:      "ab",
			expectError: true,
		},
	}

	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	service.SetAIProvider(ai.NewMockProvider())

	ctx := createAuthenticatedContext("user123", "testuser@example.com", models.Role_ROLE_USER)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&api.GenGearRequest{
				Prompt: tc.prompt,
			})

			resp, err := service.GenGear(ctx, req)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for prompt %q, got nil", tc.prompt)
				}
				return
			}

			if err != nil {
				t.Fatalf("GenGear failed for prompt %q: %v", tc.prompt, err)
			}

			if resp.Msg.DetectedGear.Title == "" {
				t.Error("Expected non-empty title")
			}

			if resp.Msg.DetectedGear.Description == "" {
				t.Error("Expected non-empty description")
			}

			t.Logf("Prompt: %q → Title: %q, Description: %q",
				tc.prompt, resp.Msg.DetectedGear.Title, resp.Msg.DetectedGear.Description)
		})
	}
}
