//go:build integration

package github

import (
	"context"
	"os"
	"strconv"
	"testing"
)

// TestGitHubAppIntegration tests actual GitHub App authentication and issue creation.
// This test requires real GitHub App credentials set in environment variables:
// - GITHUB_APP_ID
// - GITHUB_INSTALLATION_ID
// - GITHUB_APP_PRIVATE_KEY
// - GITHUB_TEST_REPO_OWNER
// - GITHUB_TEST_REPO_NAME
//
// Run with: go test -tags=integration ./server/github/...
func TestGitHubAppIntegration(t *testing.T) {
	// Read credentials from environment
	appIDStr := os.Getenv("GITHUB_APP_ID")
	installationIDStr := os.Getenv("GITHUB_INSTALLATION_ID")
	privateKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	owner := os.Getenv("GITHUB_TEST_REPO_OWNER")
	repo := os.Getenv("GITHUB_TEST_REPO_NAME")

	if appIDStr == "" || installationIDStr == "" || privateKey == "" || owner == "" || repo == "" {
		t.Skip("Skipping integration test: GitHub App credentials not configured")
	}

	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		t.Fatalf("Invalid GITHUB_APP_ID: %v", err)
	}

	installationID, err := strconv.ParseInt(installationIDStr, 10, 64)
	if err != nil {
		t.Fatalf("Invalid GITHUB_INSTALLATION_ID: %v", err)
	}

	ctx := context.Background()

	// Create GitHub App client
	client, err := NewAppClient(ctx, AppConfig{
		AppID:          appID,
		InstallationID: installationID,
		PrivateKey:     []byte(privateKey),
		Owner:          owner,
		Repo:           repo,
	})
	if err != nil {
		t.Fatalf("Failed to create GitHub App client: %v", err)
	}

	// Create a test issue
	issue, err := client.CreateIssue(ctx, "", "", "[Test] Integration test issue", "This is a test issue created by integration tests. It can be safely closed.", "", []string{"test"})
	if err != nil {
		t.Fatalf("Failed to create GitHub issue: %v", err)
	}

	if issue.Number == 0 {
		t.Error("Issue number should not be 0")
	}
	if issue.URL == "" {
		t.Error("Issue URL should not be empty")
	}

	t.Logf("Successfully created test issue #%d: %s", issue.Number, issue.URL)
}
