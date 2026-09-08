package github

import (
	"context"
	"testing"
)

func TestNewAppClient_Validation(t *testing.T) {
	// Use a placeholder for private key in validation tests
	// Real GitHub App initialization is tested in integration tests
	placeholderKey := []byte("placeholder")

	tests := []struct {
		name    string
		config  AppConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing app ID",
			config: AppConfig{
				AppID:          0,
				InstallationID: 67890,
				PrivateKey:     placeholderKey,
				Owner:          "test-owner",
				Repo:           "test-repo",
			},
			wantErr: true,
			errMsg:  "app ID is required",
		},
		{
			name: "missing installation ID",
			config: AppConfig{
				AppID:          12345,
				InstallationID: 0,
				PrivateKey:     placeholderKey,
				Owner:          "test-owner",
				Repo:           "test-repo",
			},
			wantErr: true,
			errMsg:  "installation ID is required",
		},
		{
			name: "missing private key",
			config: AppConfig{
				AppID:          12345,
				InstallationID: 67890,
				PrivateKey:     nil,
				Owner:          "test-owner",
				Repo:           "test-repo",
			},
			wantErr: true,
			errMsg:  "private key is required",
		},
		{
			name: "empty private key",
			config: AppConfig{
				AppID:          12345,
				InstallationID: 67890,
				PrivateKey:     []byte{},
				Owner:          "test-owner",
				Repo:           "test-repo",
			},
			wantErr: true,
			errMsg:  "private key is required",
		},
		{
			name: "missing owner",
			config: AppConfig{
				AppID:          12345,
				InstallationID: 67890,
				PrivateKey:     placeholderKey,
				Owner:          "",
				Repo:           "test-repo",
			},
			wantErr: true,
			errMsg:  "owner is required",
		},
		{
			name: "missing repo",
			config: AppConfig{
				AppID:          12345,
				InstallationID: 67890,
				PrivateKey:     placeholderKey,
				Owner:          "test-owner",
				Repo:           "",
			},
			wantErr: true,
			errMsg:  "repo is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, err := NewAppClient(ctx, tt.config)

			if !tt.wantErr {
				if err != nil {
					t.Errorf("NewAppClient() unexpected error = %v", err)
				}
				return
			}

			if err == nil {
				t.Errorf("NewAppClient() error = nil, want error containing %q", tt.errMsg)
				return
			}

			if tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("NewAppClient() error = %q, want %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestMockClient(t *testing.T) {
	ctx := context.Background()

	t.Run("default behavior returns mock issue", func(t *testing.T) {
		mock := &MockClient{}
		issue, err := mock.CreateIssue(ctx, "owner", "repo", "title", "body", "", nil)
		if err != nil {
			t.Errorf("MockClient.CreateIssue() unexpected error = %v", err)
		}
		if issue == nil {
			t.Fatal("MockClient.CreateIssue() returned nil issue")
		}
		if issue.Number != 123 {
			t.Errorf("MockClient.CreateIssue() issue.Number = %d, want 123", issue.Number)
		}
		if issue.URL == "" {
			t.Error("MockClient.CreateIssue() issue.URL is empty")
		}
	})

	t.Run("uses custom mock function when set", func(t *testing.T) {
		expectedIssue := &Issue{Number: 456, URL: "https://custom.url"}
		mock := &MockClient{
			CreateIssueFunc: func(ctx context.Context, owner, repo, title, body, issueType string, labels []string) (*Issue, error) {
				return expectedIssue, nil
			},
		}

		issue, err := mock.CreateIssue(ctx, "owner", "repo", "title", "body", "", nil)
		if err != nil {
			t.Errorf("MockClient.CreateIssue() unexpected error = %v", err)
		}
		if issue != expectedIssue {
			t.Errorf("MockClient.CreateIssue() = %v, want %v", issue, expectedIssue)
		}
	})
}
