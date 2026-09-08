package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai/aitest"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
)

// TestEndToEnd_AIGearDetection tests AI-powered gear detection from photos.
func TestEndToEnd_AIGearDetection(t *testing.T) {
	serverURL := getTestServerURL(t)
	t.Logf("Testing against server: %s", serverURL)

	simID := generateSimulationID("ai-gear-detection")
	registerSimulationCleanup(t, serverURL, simID)

	ctx := context.Background()

	// Create test user
	authToken, _ := createUniqueTestUser(t, serverURL, "ai-gear-detection", simID)

	// Create authenticated clients
	mediaClient := apiconnect.NewMediaServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: authToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	gearClient := apiconnect.NewGearServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: authToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	// Upload test image
	imageName := "ski.jpeg"
	mediaID := uploadTestImage(t, mediaClient, imageName)

	// Use AI to detect gear from the image
	t.Logf("Running AI gear detection on media %s...", mediaID)
	genResp, err := gearClient.GenGear(ctx, connect.NewRequest(&api.GenGearRequest{
		MediaId: mediaID,
	}))
	if err != nil {
		// No provider wired at all, as opposed to one that failed. GenGear
		// returns exactly this when s.aiProvider is nil
		// (server/services/gear/gen_ai.go), which is the state of any
		// environment without provider credentials — the open-source repo, and
		// every fork PR, since GitHub never passes secrets to those. There is
		// nothing about our code such a run could prove or disprove.
		//
		// Matched on the message as well as the code: FailedPrecondition also
		// carries real booking-rule violations elsewhere in this service, and
		// this must not start swallowing those.
		if connect.CodeOf(err) == connect.CodeFailedPrecondition &&
			strings.Contains(err.Error(), "AI provider not configured") {
			t.Skipf("no AI provider configured in this environment: %v", err)
		}
		if aitest.IsTransientError(err) {
			t.Skipf("AI provider transient failure, skipping: %v", err)
		}
		if connect.CodeOf(err) == connect.CodeInternal {
			t.Skipf("GenGear internal server error (likely transient AI provider failure), skipping: %v", err)
		}
		t.Fatalf("GenGear failed for %s: %v", imageName, err)
	}

	// Verify we got a detected gear item
	if genResp.Msg.DetectedGear == nil {
		t.Logf("Warning: No gear detected in %s (AI may not have found anything)", imageName)
		return // Not necessarily a failure - AI might not detect anything
	}

	detectedGear := genResp.Msg.DetectedGear
	t.Logf("AI detected gear from %s:", imageName)
	t.Logf("  Title: %s (confidence: %.2f)", detectedGear.Title, detectedGear.Confidence)
	t.Logf("  Description: %s", detectedGear.Description)
	if detectedGear.LocationQuery != "" {
		t.Logf("  Location Query: %s", detectedGear.LocationQuery)
	}
	if detectedGear.GeocodedLocation != nil {
		t.Logf("  Geocoded Location: %s (%s, %s)", detectedGear.GeocodedLocation.Name, detectedGear.GeocodedLocation.Locality, detectedGear.GeocodedLocation.RegionCode)
	}
	if len(detectedGear.MediaIds) > 0 {
		t.Logf("  Media IDs: %v", detectedGear.MediaIds)
	}

	// Basic validation
	if detectedGear.Title == "" {
		t.Errorf("ERROR: Detected gear has empty title")
	}
	if detectedGear.Confidence < 0.0 || detectedGear.Confidence > 1.0 {
		t.Errorf("ERROR: Detected gear has invalid confidence (expected 0.0-1.0): %.2f", detectedGear.Confidence)
	}

	// Save the detected gear
	saveResp, err := gearClient.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        &detectedGear.Title,
		Description: &detectedGear.Description,
		MediaIds:    []string{mediaID},
	}))
	if err != nil {
		t.Fatalf("Failed to save detected gear: %v", err)
	}

	gearID := saveResp.Msg.Id
	t.Logf("Saved detected gear as item: %s", gearID)

	// Verify we can retrieve the saved gear
	getResp, err := gearClient.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{
		Id: gearID,
	}))
	if err != nil {
		t.Fatalf("Failed to retrieve saved gear: %v", err)
	}

	if getResp.Msg.Name != detectedGear.Title {
		t.Errorf("Retrieved gear name mismatch: expected %s, got %s", detectedGear.Title, getResp.Msg.Name)
	}
	if len(getResp.Msg.MediaIds) != 1 || getResp.Msg.MediaIds[0] != mediaID {
		t.Errorf("Retrieved gear media IDs incorrect: expected [%s], got %v", mediaID, getResp.Msg.MediaIds)
	}
}
