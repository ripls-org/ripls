package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// forgejoRequestTimeout bounds one Forgejo API call. Filing feedback makes at
// most three (list labels, create a label, create the issue).
const forgejoRequestTimeout = 15 * time.Second

// forgejoNewLabelColor is the color given to a label this client has to
// create. Neutral grey: a person triaging the tracker picks the real one.
const forgejoNewLabelColor = "#ededed"

// forgejoLabelPageSize is the page size used when listing a repository's
// labels; Forgejo caps a page at its configured maximum, 50 by default.
const forgejoLabelPageSize = 50

// ForgejoConfig holds what ForgejoClient needs to file issues in one
// repository of a Forgejo instance.
type ForgejoConfig struct {
	// BaseURL is the instance's root, e.g. "https://code.example.org".
	BaseURL string
	// Token is an access token for an account that can write issues in the
	// repository.
	Token string
	Owner string
	Repo  string
}

// IsComplete reports whether every field NewForgejoClient requires is
// present. It plays the role AppConfig.IsComplete plays for GitHub: it lets a
// caller tell "this deployment has no Forgejo tracker" from "the tracker is
// configured but the client could not be built".
func (c ForgejoConfig) IsComplete() bool {
	return c.BaseURL != "" && c.Token != "" && c.Owner != "" && c.Repo != ""
}

// ForgejoClient implements Client against a Forgejo instance, authenticating
// with an access token.
//
// Forgejo differs from GitHub in two ways that matter here. An issue takes
// label ids, not names, so names are looked up and missing labels are created.
// And there are no issue types, so the type is applied as a label of the same
// name in lower case ("Bug" becomes the label "bug").
type ForgejoClient struct {
	http    *http.Client
	baseURL string
	token   string
	owner   string
	repo    string
}

// NewForgejoClient creates a client for the repository named in config.
func NewForgejoClient(config ForgejoConfig) (*ForgejoClient, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if config.Token == "" {
		return nil, fmt.Errorf("token is required")
	}
	if config.Owner == "" {
		return nil, fmt.Errorf("owner is required")
	}
	if config.Repo == "" {
		return nil, fmt.Errorf("repo is required")
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("base URL is not a URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("base URL must be http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("base URL has no host")
	}

	return &ForgejoClient{
		// A plain client: the host is the operator's own configuration, not
		// user input (docs/server/conventions.md, "Security: Outbound HTTP").
		http:    &http.Client{Timeout: forgejoRequestTimeout},
		baseURL: strings.TrimRight(config.BaseURL, "/"),
		token:   config.Token,
		owner:   config.Owner,
		repo:    config.Repo,
	}, nil
}

// forgejoLabel is the part of a Forgejo label this client reads.
type forgejoLabel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// CreateIssue creates a new issue in the repository. issueType, when set, is
// applied as a lower-case label because Forgejo has no issue types.
func (c *ForgejoClient) CreateIssue(ctx context.Context, owner, repo, title, body, issueType string, labels []string) (*Issue, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if owner == "" {
		owner = c.owner
	}
	if repo == "" {
		repo = c.repo
	}

	names := append([]string{}, labels...)
	if issueType != "" {
		names = append(names, strings.ToLower(issueType))
	}
	ids, err := c.labelIDs(ctx, owner, repo, names)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{"title": title, "body": body}
	if len(ids) > 0 {
		payload["labels"] = ids
	}
	var created struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := c.do(ctx, http.MethodPost, c.repoPath(owner, repo, "issues"), payload, &created); err != nil {
		return nil, fmt.Errorf("failed to create Forgejo issue: %w", err)
	}
	return &Issue{Number: created.Number, URL: created.HTMLURL}, nil
}

// labelIDs returns the ids of the named labels in the repository, creating
// any that do not exist. A feedback report must not be lost because nobody
// created its label first.
func (c *ForgejoClient) labelIDs(ctx context.Context, owner, repo string, names []string) ([]int64, error) {
	if len(names) == 0 {
		return nil, nil
	}
	existing := map[string]int64{}
	for page := 1; ; page++ {
		var batch []forgejoLabel
		path := fmt.Sprintf("%s?page=%d&limit=%d", c.repoPath(owner, repo, "labels"), page, forgejoLabelPageSize)
		if err := c.do(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return nil, fmt.Errorf("failed to list Forgejo labels: %w", err)
		}
		for _, l := range batch {
			existing[l.Name] = l.ID
		}
		if len(batch) < forgejoLabelPageSize {
			break
		}
	}

	ids := make([]int64, 0, len(names))
	for _, name := range names {
		id, ok := existing[name]
		if !ok {
			var created forgejoLabel
			payload := map[string]any{"name": name, "color": forgejoNewLabelColor}
			if err := c.do(ctx, http.MethodPost, c.repoPath(owner, repo, "labels"), payload, &created); err != nil {
				return nil, fmt.Errorf("failed to create Forgejo label %q: %w", name, err)
			}
			id = created.ID
			existing[name] = id
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// repoPath builds an API path under the repository.
func (c *ForgejoClient) repoPath(owner, repo, rest string) string {
	return fmt.Sprintf("/api/v1/repos/%s/%s/%s", url.PathEscape(owner), url.PathEscape(repo), rest)
}

// do sends one JSON request and decodes a JSON response into out. A status
// outside 2xx is an error carrying the server's message, which Forgejo sends
// as {"message": "..."}; the token never appears in it.
func (c *ForgejoClient) do(ctx context.Context, method, path string, payload, out any) error {
	var reqBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		// The body has been read or is being discarded; a close error leaves
		// nothing to act on.
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var apiErr struct {
			Message string `json:"message"`
		}
		// Best effort: an error body that is not JSON still yields the status.
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = json.Unmarshal(raw, &apiErr)
		if apiErr.Message != "" {
			return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, apiErr.Message)
		}
		return fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
