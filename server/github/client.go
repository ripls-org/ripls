// Package github provides GitHub API client functionality with App authentication support.
package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v90/github"
)

// Client provides GitHub API operations.
type Client interface {
	// CreateIssue creates a new issue in the specified repository.
	// issueType is the name of the org-level Issue Type to assign
	// (e.g. "Bug", "Feature", "Task"); empty leaves the issue untyped.
	CreateIssue(ctx context.Context, owner, repo, title, body, issueType string, labels []string) (*Issue, error)
}

// Issue represents a GitHub issue.
type Issue struct {
	Number int
	URL    string
}

// AppClient implements Client using GitHub App authentication.
type AppClient struct {
	client *github.Client
	owner  string
	repo   string
}

// AppConfig holds configuration for GitHub App authentication.
type AppConfig struct {
	AppID          int64
	InstallationID int64
	PrivateKey     []byte
	Owner          string
	Repo           string
}

// IsComplete reports whether every field NewAppClient requires is present.
//
// Callers use it to distinguish "this deployment has not configured the GitHub
// integration" — an ordinary, quiet state — from "configuration is present but
// the client could not be built", which is a real fault worth an ERROR. Both
// produce a nil client, so without this predicate the two are indistinguishable
// at the call site, and an unconfigured instance alerts on every boot (#2953).
//
// Keep the field list in sync with NewAppClient's validation below; that
// duplication is deliberate, since NewAppClient must reject an incomplete
// config even when a caller skips this check.
func (c AppConfig) IsComplete() bool {
	return c.AppID != 0 &&
		c.InstallationID != 0 &&
		len(c.PrivateKey) > 0 &&
		c.Owner != "" &&
		c.Repo != ""
}

// NewAppClient creates a new GitHub client using App authentication.
// The client generates short-lived installation tokens automatically.
func NewAppClient(ctx context.Context, config AppConfig) (*AppClient, error) {
	if config.AppID == 0 {
		return nil, fmt.Errorf("app ID is required")
	}
	if config.InstallationID == 0 {
		return nil, fmt.Errorf("installation ID is required")
	}
	if len(config.PrivateKey) == 0 {
		return nil, fmt.Errorf("private key is required")
	}
	if config.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	if config.Repo == "" {
		return nil, fmt.Errorf("repo is required")
	}

	// Create GitHub App installation transport
	itr, err := ghinstallation.New(
		http.DefaultTransport,
		config.AppID,
		config.InstallationID,
		config.PrivateKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub App transport: %w", err)
	}

	// Create GitHub client with the installation transport. As of go-github v88,
	// NewClient takes functional options and returns an error.
	client, err := github.NewClient(github.WithHTTPClient(&http.Client{Transport: itr}))
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %w", err)
	}

	return &AppClient{
		client: client,
		owner:  config.Owner,
		repo:   config.Repo,
	}, nil
}

// CreateIssue creates a new issue in the repository.
func (c *AppClient) CreateIssue(ctx context.Context, owner, repo, title, body, issueType string, labels []string) (*Issue, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	// Use configured owner/repo if not specified
	if owner == "" {
		owner = c.owner
	}
	if repo == "" {
		repo = c.repo
	}

	issueRequest := github.CreateIssueRequest{
		Title: title,
		Body:  github.Ptr(body),
	}

	if len(labels) > 0 {
		issueRequest.Labels = labels
	}

	if issueType != "" {
		issueRequest.Type = github.Ptr(issueType)
	}

	issue, _, err := c.client.Issues.Create(ctx, owner, repo, issueRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub issue: %w", err)
	}

	return &Issue{
		Number: issue.GetNumber(),
		URL:    issue.GetHTMLURL(),
	}, nil
}
