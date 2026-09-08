package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
)

// TestEndToEnd_MediaUploadRetrieval tests media upload and retrieval flow.
func TestEndToEnd_MediaUploadRetrieval(t *testing.T) {
	serverURL := getTestServerURL(t)
	t.Logf("Testing against server: %s", serverURL)

	simID := generateSimulationID("media")
	registerSimulationCleanup(t, serverURL, simID)

	ctx := context.Background()

	// Create test user
	authToken, _ := createUniqueTestUser(t, serverURL, "media-test", simID)

	// Create authenticated media client
	mediaClient := apiconnect.NewMediaServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: authToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	// Step 1: Upload test image
	t.Log("Step 1: Uploading test image...")
	mediaID := uploadTestImage(t, mediaClient, "ski.jpeg")

	// Step 2: Retrieve media metadata and URL
	t.Log("Step 2: Retrieving media metadata...")
	getResp, err := mediaClient.GetMedia(ctx, connect.NewRequest(&api.GetMediaRequest{
		Id: mediaID,
	}))
	if err != nil {
		t.Fatalf("GetMedia failed: %v", err)
	}

	if getResp.Msg.Url == "" {
		t.Fatal("Expected non-empty presigned URL in GetMedia response")
	}
	if getResp.Msg.ContentType != "image/jpeg" {
		t.Errorf("Expected content_type image/jpeg, got %s", getResp.Msg.ContentType)
	}
	if getResp.Msg.Filename != "ski.jpeg" {
		t.Errorf("Expected filename ski.jpeg, got %s", getResp.Msg.Filename)
	}

	presignedURL := getResp.Msg.Url
	t.Logf("Got presigned URL: %s", presignedURL)

	// Step 3: Download content from presigned URL
	t.Log("Step 3: Downloading content from presigned URL...")
	httpClient := &http.Client{Timeout: 10 * time.Second}
	downloadResp, err := httpClient.Get(presignedURL)
	if err != nil {
		t.Fatalf("Failed to download from presigned URL: %v", err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", downloadResp.StatusCode)
	}

	downloadedData, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		t.Fatalf("Failed to read downloaded data: %v", err)
	}

	// Step 4: Verify downloaded content matches original
	t.Log("Step 4: Verifying downloaded content matches original...")
	originalPath := filepath.Join("test_data", "ski.jpeg")
	originalData, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("Failed to read original file: %v", err)
	}

	if len(downloadedData) != len(originalData) {
		t.Errorf("Downloaded data length %d does not match original length %d",
			len(downloadedData), len(originalData))
	}

	if !bytes.Equal(downloadedData, originalData) {
		t.Error("Downloaded content does not match original image data")
	}

	t.Logf("✓ Successfully verified %d bytes match original", len(downloadedData))
	t.Log("✓ Media upload and retrieval test passed!")
}
