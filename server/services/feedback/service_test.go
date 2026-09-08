package feedback

import (
	"context"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	githubclient "go.ripls.org/ripls/server/github"
	"go.ripls.org/ripls/server/services"
)

func TestValidateFeedbackRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *api.SubmitFeedbackRequest
		wantErr bool
	}{
		{
			name: "valid bug report",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Title:       "App crashes on startup",
				Description: "The app crashes immediately when I open it on iOS 17.",
			},
			wantErr: false,
		},
		{
			name: "valid feature request",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_FEATURE_REQUEST,
				Title:       "Add dark mode",
				Description: "It would be great to have a dark mode option for nighttime use.",
			},
			wantErr: false,
		},
		{
			name: "valid other feedback",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_UNSPECIFIED,
				Title:       "General feedback",
				Description: "Just wanted to say the app is great overall!",
			},
			wantErr: false,
		},
		{
			name: "title too short",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Title:       "Bug",
				Description: "This is a description that is long enough.",
			},
			wantErr: true,
		},
		{
			name: "title too long",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Title:       "This is a very long title that exceeds the maximum allowed length of 100 characters for a feedback title field",
				Description: "Valid description here.",
			},
			wantErr: true,
		},
		{
			name: "description too short",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Title:       "Valid title here",
				Description: "Too short",
			},
			wantErr: true,
		},
		{
			name: "empty title",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Title:       "",
				Description: "Valid description here that is long enough.",
			},
			wantErr: true,
		},
		{
			name: "empty description",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Title:       "Valid title",
				Description: "",
			},
			wantErr: true,
		},
		{
			name: "invalid email",
			req: &api.SubmitFeedbackRequest{
				Type:         api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Title:        "Valid title here",
				Description:  "This is a valid description that is long enough.",
				ContactEmail: "invalid-email",
			},
			wantErr: true,
		},
		{
			name: "valid email",
			req: &api.SubmitFeedbackRequest{
				Type:         api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Title:        "Valid title here",
				Description:  "This is a valid description that is long enough.",
				ContactEmail: "user@example.com",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFeedbackRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFeedbackRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetIssueLabels(t *testing.T) {
	// All feedback gets the same single cross-cutting label; type classification
	// lives on the native GitHub Issue Type (see TestGetIssueType), not on labels.
	tests := []struct {
		name string
		req  *api.SubmitFeedbackRequest
		want []string
	}{
		{
			name: "bug report",
			req: &api.SubmitFeedbackRequest{
				Type: api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
			},
			want: []string{"user-feedback"},
		},
		{
			name: "feature request",
			req: &api.SubmitFeedbackRequest{
				Type: api.FeedbackType_FEEDBACK_TYPE_FEATURE_REQUEST,
			},
			want: []string{"user-feedback"},
		},
		{
			name: "other feedback",
			req: &api.SubmitFeedbackRequest{
				Type: api.FeedbackType_FEEDBACK_TYPE_UNSPECIFIED,
			},
			want: []string{"user-feedback"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIssueLabels(tt.req)
			if len(got) != len(tt.want) {
				t.Errorf("getIssueLabels() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getIssueLabels()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGetIssueType(t *testing.T) {
	tests := []struct {
		name string
		req  *api.SubmitFeedbackRequest
		want string
	}{
		{
			name: "bug report maps to Bug",
			req:  &api.SubmitFeedbackRequest{Type: api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT},
			want: "Bug",
		},
		{
			name: "feature request maps to Feature",
			req:  &api.SubmitFeedbackRequest{Type: api.FeedbackType_FEEDBACK_TYPE_FEATURE_REQUEST},
			want: "Feature",
		},
		{
			name: "unspecified leaves type empty for triage to assign",
			req:  &api.SubmitFeedbackRequest{Type: api.FeedbackType_FEEDBACK_TYPE_UNSPECIFIED},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIssueType(tt.req)
			if got != tt.want {
				t.Errorf("getIssueType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatIssueBody(t *testing.T) {
	tests := []struct {
		name           string
		req            *api.SubmitFeedbackRequest
		screenshotURLs []string
		submitterEmail string
		want           string
	}{
		{
			name: "bug report with all fields no screenshots",
			req: &api.SubmitFeedbackRequest{
				Type:             api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Description:      "App crashes on startup",
				StepsToReproduce: "1. Open app\n2. Wait\n3. Crash",
				ContactEmail:     "user@example.com",
				DeviceContext: &api.DeviceContext{
					AppVersion:  "1.0.0",
					Platform:    "iOS",
					OsVersion:   "17.0",
					DeviceModel: "iPhone 14",
					BuildNumber: "100",
					Environment: "DEV",
				},
			},
			screenshotURLs: nil,
			submitterEmail: "user@example.com",
			want: "## Description\n\nApp crashes on startup\n\n" +
				"## Steps to Reproduce\n\n1. Open app\n2. Wait\n3. Crash\n\n" +
				"## Submitted By\n\nuser@example.com\n\n" +
				"## Device Context\n\n" +
				"- **App Version**: 1.0.0\n" +
				"- **Platform**: iOS\n" +
				"- **OS Version**: 17.0\n" +
				"- **Device Model**: iPhone 14\n" +
				"- **Build Number**: 100\n" +
				"- **Environment**: DEV\n\n" +
				"---\n*Submitted via Ripls app*",
		},
		{
			name: "feature request minimal",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_FEATURE_REQUEST,
				Description: "Please add dark mode",
			},
			screenshotURLs: nil,
			submitterEmail: "",
			want: "## Description\n\nPlease add dark mode\n\n" +
				"---\n*Submitted via Ripls app*",
		},
		{
			name: "bug report with one screenshot",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Description: "Button is misaligned",
			},
			screenshotURLs: []string{"https://storage.example.com/screenshot1.png"},
			submitterEmail: "",
			want: "## Description\n\nButton is misaligned\n\n" +
				"## Screenshots\n\n" +
				"![Screenshot 1](https://storage.example.com/screenshot1.png)\n\n" +
				"---\n*Submitted via Ripls app*",
		},
		{
			name: "bug report with multiple screenshots",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Description: "Multiple UI issues",
			},
			screenshotURLs: []string{
				"https://storage.example.com/screenshot1.png",
				"https://storage.example.com/screenshot2.png",
				"https://storage.example.com/screenshot3.png",
			},
			submitterEmail: "",
			want: "## Description\n\nMultiple UI issues\n\n" +
				"## Screenshots\n\n" +
				"![Screenshot 1](https://storage.example.com/screenshot1.png)\n\n" +
				"![Screenshot 2](https://storage.example.com/screenshot2.png)\n\n" +
				"![Screenshot 3](https://storage.example.com/screenshot3.png)\n\n" +
				"---\n*Submitted via Ripls app*",
		},
		{
			name: "feature request with screenshot",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_FEATURE_REQUEST,
				Description: "Here is a mockup of the feature",
			},
			screenshotURLs: []string{"https://storage.example.com/mockup.png"},
			submitterEmail: "",
			want: "## Description\n\nHere is a mockup of the feature\n\n" +
				"## Screenshots\n\n" +
				"![Screenshot 1](https://storage.example.com/mockup.png)\n\n" +
				"---\n*Submitted via Ripls app*",
		},
		{
			name: "with submitter email from auth context",
			req: &api.SubmitFeedbackRequest{
				Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Description: "Bug found by authenticated user",
			},
			screenshotURLs: nil,
			submitterEmail: "auth-user@example.com",
			want: "## Description\n\nBug found by authenticated user\n\n" +
				"## Submitted By\n\nauth-user@example.com\n\n" +
				"---\n*Submitted via Ripls app*",
		},
		{
			name: "with different contact email and submitter email",
			req: &api.SubmitFeedbackRequest{
				Type:         api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
				Description:  "Bug with different contact",
				ContactEmail: "different@example.com",
			},
			screenshotURLs: nil,
			submitterEmail: "auth-user@example.com",
			want: "## Description\n\nBug with different contact\n\n" +
				"## Submitted By\n\nauth-user@example.com\n\n" +
				"## Contact\n\ndifferent@example.com\n\n" +
				"---\n*Submitted via Ripls app*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatIssueBody(tt.req, tt.screenshotURLs, tt.submitterEmail)
			if got != tt.want {
				t.Errorf("formatIssueBody() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSubmitterEmail(t *testing.T) {
	tests := []struct {
		name     string
		req      *api.SubmitFeedbackRequest
		authInfo *auth.Info
		want     string
	}{
		{
			name: "uses request email when provided",
			req: &api.SubmitFeedbackRequest{
				ContactEmail: "request@example.com",
			},
			authInfo: &auth.Info{
				Email: "auth@example.com",
			},
			want: "request@example.com",
		},
		{
			name: "uses auth email when request email is empty",
			req: &api.SubmitFeedbackRequest{
				ContactEmail: "",
			},
			authInfo: &auth.Info{
				Email: "auth@example.com",
			},
			want: "auth@example.com",
		},
		{
			name: "returns empty when both are empty",
			req: &api.SubmitFeedbackRequest{
				ContactEmail: "",
			},
			authInfo: &auth.Info{
				Email: "",
			},
			want: "",
		},
		{
			name: "returns empty when not authenticated",
			req: &api.SubmitFeedbackRequest{
				ContactEmail: "",
			},
			authInfo: nil,
			want:     "",
		},
		{
			name: "trims whitespace from request email",
			req: &api.SubmitFeedbackRequest{
				ContactEmail: "  request@example.com  ",
			},
			authInfo: nil,
			want:     "request@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.authInfo != nil {
				ctx = authn.SetInfo(ctx, tt.authInfo)
			}

			got := getSubmitterEmail(ctx, tt.req)
			if got != tt.want {
				t.Errorf("getSubmitterEmail() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSubmitFeedback_IssueMetadata verifies that SubmitFeedback passes the
// correct native Issue Type, labels, and a prefix-free title to the GitHub
// client. Bug/feature classification must flow through the Issue Type
// parameter — not the legacy `bug`/`enhancement` labels or `[Bug Report]` /
// `[Feature Request]` title prefixes.
func TestSubmitFeedback_IssueMetadata(t *testing.T) {
	tests := []struct {
		name          string
		feedbackType  api.FeedbackType
		title         string
		wantIssueType string
		wantLabels    []string
	}{
		{
			name:          "bug report sets Bug issue type",
			feedbackType:  api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
			title:         "App crashes on startup",
			wantIssueType: "Bug",
			wantLabels:    []string{"user-feedback"},
		},
		{
			name:          "feature request sets Feature issue type",
			feedbackType:  api.FeedbackType_FEEDBACK_TYPE_FEATURE_REQUEST,
			title:         "Add dark mode",
			wantIssueType: "Feature",
			wantLabels:    []string{"user-feedback"},
		},
		{
			name:          "unspecified leaves issue type empty for triage",
			feedbackType:  api.FeedbackType_FEEDBACK_TYPE_UNSPECIFIED,
			title:         "General observation",
			wantIssueType: "",
			wantLabels:    []string{"user-feedback"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotIssueType string
			var gotLabels []string
			var gotTitle string

			mock := &githubclient.MockClient{
				CreateIssueFunc: func(_ context.Context, _, _, title, _, issueType string, labels []string) (*githubclient.Issue, error) {
					gotTitle = title
					gotIssueType = issueType
					gotLabels = labels
					return &githubclient.Issue{Number: 1, URL: "https://example.test/issues/1"}, nil
				},
			}

			svc := New(mock, "owner", "repo", nil)
			req := connect.NewRequest(&api.SubmitFeedbackRequest{
				Type:        tt.feedbackType,
				Title:       tt.title,
				Description: "This is a detailed description long enough to pass validation.",
			})

			if _, err := svc.SubmitFeedback(context.Background(), req); err != nil {
				t.Fatalf("SubmitFeedback() error = %v", err)
			}

			if gotIssueType != tt.wantIssueType {
				t.Errorf("issue type = %q, want %q", gotIssueType, tt.wantIssueType)
			}
			if !slices.Equal(gotLabels, tt.wantLabels) {
				t.Errorf("labels = %v, want %v", gotLabels, tt.wantLabels)
			}
			if gotTitle != tt.title {
				t.Errorf("title = %q, want %q (should carry no legacy prefix like [Bug Report])", gotTitle, tt.title)
			}
		})
	}
}

// TestSubmitFeedback_ScreenshotUsesDurableURL asserts a submitted screenshot is
// embedded using the durable signed URL (GetDurableSignedURL), not the
// short-lived signBlob URL — the fix for #2549.
func TestSubmitFeedback_ScreenshotUsesDurableURL(t *testing.T) {
	const durableURL = "https://storage.googleapis.com/example-prod/feedback-system/abc?durable=1"

	var gotBody string
	mockGitHub := &githubclient.MockClient{
		CreateIssueFunc: func(_ context.Context, _, _, _, body, _ string, _ []string) (*githubclient.Issue, error) {
			gotBody = body
			return &githubclient.Issue{Number: 7, URL: "https://example.test/issues/7"}, nil
		},
	}
	// DurableSignedURL set, SignedURL set to a different value: a correct
	// implementation must embed the durable one.
	mockBucket := &services.MockBucketStorage{
		DurableSignedURL: durableURL,
		SignedURL:        "https://storage.googleapis.com/short-lived-should-not-be-used",
	}

	svc := New(mockGitHub, "owner", "repo", mockBucket)
	req := connect.NewRequest(&api.SubmitFeedbackRequest{
		Type:        api.FeedbackType_FEEDBACK_TYPE_BUG_REPORT,
		Title:       "Screenshot attaches",
		Description: "This is a detailed description long enough to pass validation.",
		Screenshots: []*api.FeedbackScreenshot{
			{Data: []byte("fake-jpeg-bytes"), ContentType: "image/jpeg", Filename: "shot.jpg"},
		},
	})

	if _, err := svc.SubmitFeedback(context.Background(), req); err != nil {
		t.Fatalf("SubmitFeedback() error = %v", err)
	}

	if !strings.Contains(gotBody, durableURL) {
		t.Errorf("issue body does not embed the durable screenshot URL.\nbody:\n%s", gotBody)
	}
	if strings.Contains(gotBody, "short-lived-should-not-be-used") {
		t.Error("issue body embedded the short-lived URL instead of the durable one")
	}
}
