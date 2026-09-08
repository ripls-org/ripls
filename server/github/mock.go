package github

import "context"

// MockClient is a mock implementation of Client for testing.
type MockClient struct {
	CreateIssueFunc func(ctx context.Context, owner, repo, title, body, issueType string, labels []string) (*Issue, error)
}

// CreateIssue calls the mock function if set, otherwise returns a default response.
func (m *MockClient) CreateIssue(ctx context.Context, owner, repo, title, body, issueType string, labels []string) (*Issue, error) {
	if m.CreateIssueFunc != nil {
		return m.CreateIssueFunc(ctx, owner, repo, title, body, issueType, labels)
	}
	// Default mock response
	return &Issue{
		Number: 123,
		URL:    "https://github.com/owner/repo/issues/123",
	}, nil
}
