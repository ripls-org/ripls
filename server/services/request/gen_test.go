package request

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestService_GenRequest tests the AI request generation functionality.
func TestService_GenRequest(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	userID := setupTestUser(t, testStorage, "Test User", "testuser@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, userID)

	ctx := createAuthenticatedContext(userID, "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenRequestRequest{
		Prompt:      "Need a power drill for weekend project",
		CommunityId: communityID,
	})

	resp, err := service.GenRequest(ctx, req)
	if err != nil {
		t.Fatalf("GenRequest failed: %v", err)
	}

	// Verify AI-generated title is present
	if resp.Msg.Title == "" {
		t.Error("Expected non-empty title from AI generation")
	}

	// Verify title is reasonable length (should be concise, max 50 chars per spec)
	if len(resp.Msg.Title) > 50 {
		t.Errorf("Title too long (%d chars): %s", len(resp.Msg.Title), resp.Msg.Title)
	}

	// Verify AI-generated description is present
	if resp.Msg.Description == "" {
		t.Error("Expected non-empty description from AI generation")
	}

	// In text mode, description is set to the user's original prompt verbatim.
	if resp.Msg.Description != req.Msg.Prompt {
		t.Errorf("Expected description to equal prompt %q, got %q", req.Msg.Prompt, resp.Msg.Description)
	}

	// Note: media_ids may be empty if Unsplash search fails (which is OK)
	// The mock Unsplash client returns no results, so media_ids will be empty in tests
	if len(resp.Msg.MediaIds) > 0 {
		t.Logf("Generated media_ids: %v", resp.Msg.MediaIds)
	}

	// Verify tags/keywords are present
	if len(resp.Msg.Tags) == 0 {
		t.Error("Expected AI-generated tags/keywords")
	}

	// Note: GenRequest now returns GeocodedLocation instead of location_id
	// Client will save the geocoded location when user confirms the request
	if resp.Msg.GeocodedLocation != nil {
		t.Logf("Note: geocoded_location=%s (lat=%f, lon=%f)",
			resp.Msg.GeocodedLocation.Name,
			resp.Msg.GeocodedLocation.LatitudeDeg,
			resp.Msg.GeocodedLocation.LongitudeDeg)
	}

	t.Logf("Generated request: title=%q, description=%q, media_ids=%v, tags=%v",
		resp.Msg.Title, resp.Msg.Description, resp.Msg.MediaIds, resp.Msg.Tags)
}

// TestService_GenRequest_MinimalPrompt tests generation with a minimal prompt.
func TestService_GenRequest_MinimalPrompt(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	userID := setupTestUser(t, testStorage, "Test User", "testuser@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, userID)

	ctx := createAuthenticatedContext(userID, "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenRequestRequest{
		Prompt:      "ladder",
		CommunityId: communityID,
	})

	resp, err := service.GenRequest(ctx, req)
	if err != nil {
		t.Fatalf("GenRequest failed: %v", err)
	}

	// Even with minimal prompt, should get reasonable results
	if resp.Msg.Title == "" {
		t.Error("Expected title even with minimal prompt")
	}

	if resp.Msg.Description == "" {
		t.Error("Expected description even with minimal prompt")
	}

	// In text mode, description is set to the user's original prompt verbatim.
	if resp.Msg.Description != req.Msg.Prompt {
		t.Errorf("Expected description to equal prompt %q, got %q", req.Msg.Prompt, resp.Msg.Description)
	}

	t.Logf("From minimal prompt %q, generated: title=%q, description=%q",
		req.Msg.Prompt, resp.Msg.Title, resp.Msg.Description)
}

// TestService_GenRequest_DetailedPrompt tests generation with a detailed prompt.
func TestService_GenRequest_DetailedPrompt(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	userID := setupTestUser(t, testStorage, "Test User", "testuser@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, userID)

	ctx := createAuthenticatedContext(userID, "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenRequestRequest{
		Prompt:      "I need to borrow a power drill, preferably cordless, to install shelves in my garage this weekend. I'll need it from Friday evening to Sunday afternoon. Happy to help with a project in return!",
		CommunityId: communityID,
	})

	resp, err := service.GenRequest(ctx, req)
	if err != nil {
		t.Fatalf("GenRequest failed: %v", err)
	}

	// With detailed prompt, should still produce concise title
	if len(resp.Msg.Title) > 50 {
		t.Errorf("Title should be concise even with detailed prompt, got %d chars: %s",
			len(resp.Msg.Title), resp.Msg.Title)
	}

	// Description should preserve key details from prompt
	if resp.Msg.Description == "" {
		t.Error("Expected description to preserve details from prompt")
	}

	// Note: media_id may be empty if Unsplash search fails (which is OK for tests)

	t.Logf("From detailed prompt, generated: title=%q, description=%q",
		resp.Msg.Title, resp.Msg.Description)
}

// TestService_GenRequest_EmptyPrompt tests validation of empty prompt.
func TestService_GenRequest_EmptyPrompt(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	userID := setupTestUser(t, testStorage, "Test User", "testuser@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, userID)

	ctx := createAuthenticatedContext(userID, "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenRequestRequest{
		Prompt:      "",
		CommunityId: communityID,
	})

	_, err := service.GenRequest(ctx, req)
	if err == nil {
		t.Fatal("Expected error for empty prompt, got nil")
	}

	// Verify it's an InvalidArgument error
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("Expected connect.Error, got %T", err)
	}

	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected CodeInvalidArgument, got %v", connectErr.Code())
	}
}

// TestService_GenRequest_Unauthenticated tests that unauthenticated calls are rejected.
func TestService_GenRequest_Unauthenticated(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	userID := setupTestUser(t, testStorage, "Test User", "testuser@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, userID)

	// Use unauthenticated context
	ctx := context.Background()

	req := connect.NewRequest(&api.GenRequestRequest{
		Prompt:      "Need a power drill",
		CommunityId: communityID,
	})

	_, err := service.GenRequest(ctx, req)
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

// TestService_GenRequest_NoDatabase tests that GenRequest does NOT save to database.
func TestService_GenRequest_NoDatabase(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	userID := setupTestUser(t, testStorage, "Test User", "testuser@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, userID)

	ctx := createAuthenticatedContext(userID, "testuser@example.com", models.Role_ROLE_USER)

	// Count requests before GenRequest (via CommunityRequest join table)
	communityRequestsBefore, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
	}, &models.CommunityRequest{})
	if err != nil {
		t.Fatalf("Failed to query requests: %v", err)
	}

	req := connect.NewRequest(&api.GenRequestRequest{
		Prompt:      "Need a power drill",
		CommunityId: communityID,
	})

	resp, err := service.GenRequest(ctx, req)
	if err != nil {
		t.Fatalf("GenRequest failed: %v", err)
	}

	// Verify response is valid
	if resp.Msg.Title == "" || resp.Msg.Description == "" {
		t.Error("Expected valid generation response")
	}

	// Count requests after GenRequest (via CommunityRequest join table)
	communityRequestsAfter, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
	}, &models.CommunityRequest{})
	if err != nil {
		t.Fatalf("Failed to query requests: %v", err)
	}

	// Verify no new requests were created
	if len(communityRequestsAfter) != len(communityRequestsBefore) {
		t.Errorf("GenRequest should not save to database. Before: %d requests, After: %d requests",
			len(communityRequestsBefore), len(communityRequestsAfter))
	}
}

// TestService_GenRequest_VariousPromptTypes tests different types of help requests.
func TestService_GenRequest_VariousPromptTypes(t *testing.T) {
	testCases := []struct {
		name        string
		prompt      string
		expectError bool
	}{
		{
			name:        "Tool request",
			prompt:      "Need a power drill",
			expectError: false,
		},
		{
			name:        "Moving help",
			prompt:      "Need help moving furniture on Saturday",
			expectError: false,
		},
		{
			name:        "Advice/expertise",
			prompt:      "Looking for advice on deck repair",
			expectError: false,
		},
		{
			name:        "Skill sharing",
			prompt:      "Need someone to teach me guitar basics",
			expectError: false,
		},
		{
			name:        "Transportation",
			prompt:      "Need a ride to the airport next Tuesday",
			expectError: false,
		},
		{
			name:        "Empty prompt",
			prompt:      "",
			expectError: true,
		},
	}

	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)
	userID := setupTestUser(t, testStorage, "Test User", "testuser@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, userID)
	ctx := createAuthenticatedContext(userID, "testuser@example.com", models.Role_ROLE_USER)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := connect.NewRequest(&api.GenRequestRequest{
				Prompt:      tc.prompt,
				CommunityId: communityID,
			})

			resp, err := service.GenRequest(ctx, req)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("GenRequest failed: %v", err)
			}

			if resp.Msg.Title == "" {
				t.Error("Expected non-empty title")
			}

			if resp.Msg.Description == "" {
				t.Error("Expected non-empty description")
			}

			t.Logf("Prompt: %q → Title: %q, Description: %q",
				tc.prompt, resp.Msg.Title, resp.Msg.Description)
		})
	}
}

// TestService_GenRequest_WithStockImagery tests that GenRequest creates media copies with attribution tracking.
func TestService_GenRequest_WithStockImagery(t *testing.T) {
	service, testStorage, _, stockImageryDone := setupTestServiceForAsyncTesting(t)

	userID := setupTestUser(t, testStorage, "Test User", "testuser@example.com")

	ctx := createAuthenticatedContext(userID, "testuser@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GenRequestRequest{
		Prompt: "Need a power drill for weekend project",
	})

	resp, err := service.GenRequest(ctx, req)
	if err != nil {
		t.Fatalf("GenRequest failed: %v", err)
	}

	// Verify we got media IDs back (stock imagery is available via FakeProvider)
	if len(resp.Msg.MediaIds) == 0 {
		t.Fatal("Expected media_ids from stock imagery, got empty list")
	}

	mediaID := resp.Msg.MediaIds[0]
	t.Logf("Generated request with media_id: %s", mediaID)

	// Verify the media record exists and has source_stock_image_id set
	mediaRecord := &models.Media{}
	err = testStorage.GetByID(context.Background(), mediaID, mediaRecord)
	if err != nil {
		t.Fatalf("Failed to get media: %v", err)
	}

	// CRITICAL: Verify source_stock_image_id is set (enables attribution tracking)
	if mediaRecord.GetSourceStockImageId() == "" {
		t.Error("Expected media to have source_stock_image_id set for attribution tracking")
	} else {
		t.Logf("Media has source_stock_image_id: %s", mediaRecord.GetSourceStockImageId())
	}

	// Verify source_stock_image_id references a valid StockImage record
	stockImage := &models.StockImage{}
	err = testStorage.GetByID(context.Background(), mediaRecord.GetSourceStockImageId(), stockImage)
	if err != nil {
		t.Fatalf("Failed to get referenced stock image: %v", err)
	}

	// Verify the StockImage has provider metadata for attribution
	if stockImage.Provider == models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPECIFIED {
		t.Error("Expected stock image to have provider set")
	} else {
		t.Logf("Stock image provider: %s", stockImage.Provider)
	}

	if stockImage.ProviderImage == nil {
		t.Error("Expected stock image to have provider_image metadata")
	}

	// Ensure we didn't accidentally trigger async stock image fetch (channel should be empty)
	select {
	case <-stockImageryDone:
		t.Error("Async stock image fetch should not have been triggered in GenRequest")
	default:
		t.Log("Correctly did not trigger async stock image fetch")
	}
}
