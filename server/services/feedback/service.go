package feedback

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	githubclient "go.ripls.org/ripls/server/github"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// feedbackUserID is used as the "owner" for feedback screenshots in bucket storage.
// This keeps feedback screenshots separate from user-uploaded media.
const feedbackUserID = "feedback-system"

// feedbackURLExpiry is how long a feedback screenshot URL embedded in a GitHub
// issue stays valid. The URL is signed durably (see BucketStorage.GetDurableSignedURL),
// so the signature lasts this full window rather than dying at the next managed
// signing-key rotation (#2549). One year keeps the screenshot available for the
// realistic lifetime of an open issue; a backfill can re-sign if a URL is needed
// past it (#2549).
const feedbackURLExpiry = 365 * 24 * time.Hour

// Service implements the FeedbackService RPC interface.
type Service struct {
	githubClient githubclient.Client
	bucket       storage.BucketStorage
	repoOwner    string
	repoName     string
}

// New creates a new feedback service.
// client: GitHub client for creating issues (use github.NewAppClient for GitHub App auth)
// repoOwner: GitHub repository owner, user or org (e.g., "acme")
// repoName: GitHub repository name (e.g., "ripls")
// bucket: Storage for screenshot attachments (optional, screenshots disabled if nil)
func New(client githubclient.Client, repoOwner, repoName string, bucket storage.BucketStorage) *Service {
	if client == nil {
		logging.Default().Warn("GitHub client not configured, feedback service will fail")
	}

	return &Service{
		githubClient: client,
		bucket:       bucket,
		repoOwner:    repoOwner,
		repoName:     repoName,
	}
}

// SubmitFeedback creates a GitHub issue from user feedback.
func (s *Service) SubmitFeedback(
	ctx context.Context,
	req *connect.Request[api.SubmitFeedbackRequest],
) (*connect.Response[api.SubmitFeedbackResponse], error) {
	// Validate request
	if err := validateFeedbackRequest(req.Msg); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Extract submitter email from auth context or request
	submitterEmail := getSubmitterEmail(ctx, req.Msg)

	logger := logging.LoggerWithContext(ctx).With(
		"feedback_type", req.Msg.Type.String(),
		"title", req.Msg.Title,
		"screenshot_count", len(req.Msg.Screenshots),
		"has_submitter_email", submitterEmail != "",
	)

	// Store screenshots and generate presigned URLs
	var screenshotURLs []string
	if s.bucket != nil && len(req.Msg.Screenshots) > 0 {
		logger.Info("storing feedback screenshots")
		for i, screenshot := range req.Msg.Screenshots {
			if i >= 3 {
				logger.Warn("ignoring extra screenshots beyond limit of 3")
				break
			}

			mediaID := uuid.NewString()
			key := storage.MediaBucketKey(feedbackUserID, mediaID)

			// Store screenshot in bucket
			_, err := s.bucket.Put(ctx, key, screenshot.Data, screenshot.ContentType, map[string]string{
				"filename": screenshot.Filename,
				"feedback": "true",
			})
			if err != nil {
				logger.Warn("failed to store screenshot", "index", i, "error", err)
				continue
			}

			// Sign a durable URL that stays valid for its full lifetime. In
			// environments with no configured signer (local/dev), fall back to the
			// short-lived signBlob URL so feedback still works there.
			url, err := s.bucket.GetDurableSignedURL(ctx, key, feedbackURLExpiry)
			if err != nil {
				logger.Warn("durable screenshot URL unavailable, falling back to short-lived signing",
					"index", i, "error", err)
				url, err = s.bucket.GetSignedURL(ctx, key, feedbackURLExpiry)
				if err != nil {
					logger.Warn("failed to generate screenshot URL", "index", i, "error", err)
					continue
				}
			}

			screenshotURLs = append(screenshotURLs, url)
			logger.Debug("stored screenshot", "index", i, "media_id", mediaID)
		}
	}

	// Format issue title and body
	title := strings.TrimSpace(req.Msg.Title)
	body := formatIssueBody(req.Msg, screenshotURLs, submitterEmail)
	labels := getIssueLabels(req.Msg)
	issueType := getIssueType(req.Msg)

	// Check if GitHub client is configured
	if s.githubClient == nil {
		logger.Error("GitHub client not configured - cannot create issue")
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("feedback service not configured: GitHub client unavailable"))
	}

	logger.Info("creating GitHub issue", "issue_type", issueType)

	// Create GitHub issue using GitHub App client
	issue, err := s.githubClient.CreateIssue(ctx, s.repoOwner, s.repoName, title, body, issueType, labels)
	if err != nil {
		logger.Error("failed to create GitHub issue", "error", err)
		return nil, connecterr.Internal(ctx, "SubmitFeedback", err, "detail", "failed to create GitHub issue")
	}

	logger.Info("created GitHub issue",
		"issue_number", issue.Number,
		"issue_url", issue.URL)

	return connect.NewResponse(&api.SubmitFeedbackResponse{
		IssueUrl:    issue.URL,
		IssueNumber: int32(issue.Number),
		Message:     "Thank you for your feedback! We've created an issue to track this.",
	}), nil
}

// validateFeedbackRequest validates the feedback request fields.
func validateFeedbackRequest(req *api.SubmitFeedbackRequest) error {
	// Validate title
	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) < 5 {
		return fmt.Errorf("title must be at least 5 characters")
	}
	if len(title) > 100 {
		return fmt.Errorf("title must be at most 100 characters")
	}

	// Validate description
	description := strings.TrimSpace(req.Description)
	if description == "" || len(description) < 10 {
		return fmt.Errorf("description must be at least 10 characters")
	}
	if len(description) > 5000 {
		return fmt.Errorf("description must be at most 5000 characters")
	}

	// Validate email if provided
	if req.ContactEmail != "" {
		email := strings.TrimSpace(req.ContactEmail)
		if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
			return fmt.Errorf("invalid email format")
		}
	}

	return nil
}

// getSubmitterEmail extracts the submitter's email from auth context or request.
// If the request has a contact email, use that. Otherwise, if the user is authenticated,
// use the email from the auth context.
func getSubmitterEmail(ctx context.Context, req *api.SubmitFeedbackRequest) string {
	// If request already has an email, use that
	if req.ContactEmail != "" {
		return strings.TrimSpace(req.ContactEmail)
	}

	// Try to get email from auth context
	authInfo, ok := auth.GetAuthInfo(ctx)
	if ok && authInfo.Email != "" {
		logger := logging.LoggerWithContext(ctx)
		logger.InfoContext(ctx, "using authenticated user email for feedback",
			"user_email", logging.MaskEmail(authInfo.Email),
		)
		return authInfo.Email
	}

	return ""
}

// formatIssueBody formats the GitHub issue body with all feedback details.
func formatIssueBody(req *api.SubmitFeedbackRequest, screenshotURLs []string, submitterEmail string) string {
	var body strings.Builder

	// Description section
	body.WriteString("## Description\n\n")
	body.WriteString(strings.TrimSpace(req.Description))
	body.WriteString("\n\n")

	// Screenshots section
	if len(screenshotURLs) > 0 {
		body.WriteString("## Screenshots\n\n")
		for i, url := range screenshotURLs {
			fmt.Fprintf(&body, "![Screenshot %d](%s)\n\n", i+1, url)
		}
	}

	// Steps to reproduce (for bug reports)
	if req.Type == api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT && req.StepsToReproduce != "" {
		body.WriteString("## Steps to Reproduce\n\n")
		body.WriteString(strings.TrimSpace(req.StepsToReproduce))
		body.WriteString("\n\n")
	}

	// Submitted by section
	if submitterEmail != "" {
		body.WriteString("## Submitted By\n\n")
		body.WriteString(submitterEmail)
		body.WriteString("\n\n")
	}

	// Contact information (deprecated in favor of Submitted By, but kept for backward compatibility)
	if req.ContactEmail != "" && req.ContactEmail != submitterEmail {
		body.WriteString("## Contact\n\n")
		body.WriteString(strings.TrimSpace(req.ContactEmail))
		body.WriteString("\n\n")
	}

	// Device context
	if req.DeviceContext != nil {
		body.WriteString("## Device Context\n\n")
		fmt.Fprintf(&body, "- **App Version**: %s\n", req.DeviceContext.AppVersion)
		fmt.Fprintf(&body, "- **Platform**: %s\n", req.DeviceContext.Platform)
		fmt.Fprintf(&body, "- **OS Version**: %s\n", req.DeviceContext.OsVersion)
		fmt.Fprintf(&body, "- **Device Model**: %s\n", req.DeviceContext.DeviceModel)
		fmt.Fprintf(&body, "- **Build Number**: %s\n", req.DeviceContext.BuildNumber)
		fmt.Fprintf(&body, "- **Environment**: %s\n", req.DeviceContext.Environment)
		body.WriteString("\n")
	}

	// Footer
	body.WriteString("---\n")
	body.WriteString("*Submitted via Ripls app*")

	return body.String()
}

// getIssueLabels returns the GitHub labels to apply to the issue.
// Classification (bug/feature) lives on the native Issue Type (see getIssueType);
// labels here are reserved for cross-cutting tags.
func getIssueLabels(_ *api.SubmitFeedbackRequest) []string {
	return []string{"user-feedback"}
}

// getIssueType maps the feedback type to a GitHub Issue Type name.
// Returns an empty string for unspecified / "other" feedback; triage will
// assign a type later.
func getIssueType(req *api.SubmitFeedbackRequest) string {
	switch req.Type {
	case api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT:
		return "Bug"
	case api.FeedbackType_FEEDBACK_TYPE_FEATURE_REQUEST:
		return "Feature"
	default:
		return ""
	}
}

// CheckHealth validates GitHub API connectivity by fetching the authenticated user.
func (s *Service) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	status := &health.Status{
		Name:    "github",
		Backend: "github",
		Metadata: map[string]string{
			"repo": fmt.Sprintf("%s/%s", s.repoOwner, s.repoName),
		},
	}

	// Verify GitHub client is configured
	if s.githubClient == nil {
		status.Error = "GitHub client not configured"
	}

	return []*health.Status{status}, nil
}
