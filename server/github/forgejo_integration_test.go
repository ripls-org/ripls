//go:build integration

package github

import (
	"context"
	"os"
	"testing"
)

// TestForgejoIntegration files one issue in a real Forgejo repository. It
// needs these environment variables and is skipped without them:
//   - FORGEJO_TEST_URL (the instance root, e.g. https://code.example.org)
//   - FORGEJO_TEST_TOKEN (can write issues in the repository)
//   - FORGEJO_TEST_REPO_OWNER
//   - FORGEJO_TEST_REPO_NAME
//
// Run with: go test -tags=integration -run TestForgejoIntegration ./github/
func TestForgejoIntegration(t *testing.T) {
	cfg := ForgejoConfig{
		BaseURL: os.Getenv("FORGEJO_TEST_URL"),
		Token:   os.Getenv("FORGEJO_TEST_TOKEN"),
		Owner:   os.Getenv("FORGEJO_TEST_REPO_OWNER"),
		Repo:    os.Getenv("FORGEJO_TEST_REPO_NAME"),
	}
	if !cfg.IsComplete() {
		t.Skip("Skipping integration test: Forgejo test repository not configured")
	}

	client, err := NewForgejoClient(cfg)
	if err != nil {
		t.Fatalf("NewForgejoClient: %v", err)
	}

	// "test" exercises the label lookup, and "Bug" the type-as-label path.
	issue, err := client.CreateIssue(context.Background(), "", "", "[Test] Integration test issue",
		"This is a test issue created by integration tests. It can be safely closed.", "Bug", []string{"test"})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if issue.Number == 0 || issue.URL == "" {
		t.Errorf("issue = %+v, want a number and a URL", issue)
	}
	t.Logf("created test issue #%d: %s", issue.Number, issue.URL)
}
