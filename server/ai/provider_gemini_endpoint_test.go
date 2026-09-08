package ai

import (
	"strings"
	"testing"
)

// TestCountTokensEndpoint pins the URL construction used by CheckHealth.
// The host transformation differs between regional locations (which prefix
// the host with the location) and the "global" location (which uses the
// bare aiplatform.googleapis.com host), so a regression in either branch
// produces a 404 against a working API project — which surfaces as
// "ai: UNHEALTHY" in TestHealthEndpoint_Integration. Catching it here
// keeps that signal clear and avoids burning a CI cycle to find it.
func TestCountTokensEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		location string
		project  string
		model    string
		want     string
	}{
		{
			name:     "regional location prefixes host",
			location: "us-central1",
			project:  "example-dev",
			model:    "gemini-2.5-flash",
			want:     "https://us-central1-aiplatform.googleapis.com/v1/projects/example-dev/locations/us-central1/publishers/google/models/gemini-2.5-flash:countTokens",
		},
		{
			name:     "europe regional location",
			location: "europe-west4",
			project:  "example-prod",
			model:    "gemini-2.5-pro",
			want:     "https://europe-west4-aiplatform.googleapis.com/v1/projects/example-prod/locations/europe-west4/publishers/google/models/gemini-2.5-pro:countTokens",
		},
		{
			name:     "global location drops the host prefix",
			location: "global",
			project:  "example-dev",
			model:    "gemini-3.1-flash-lite",
			want:     "https://aiplatform.googleapis.com/v1/projects/example-dev/locations/global/publishers/google/models/gemini-3.1-flash-lite:countTokens",
		},
		{
			name:     "global with preview model",
			location: "global",
			project:  "example-dev",
			model:    "gemini-3-flash-preview",
			want:     "https://aiplatform.googleapis.com/v1/projects/example-dev/locations/global/publishers/google/models/gemini-3-flash-preview:countTokens",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countTokensEndpoint(tt.location, tt.project, tt.model)
			if got != tt.want {
				t.Errorf("countTokensEndpoint(%q, %q, %q)\n  got:  %q\n  want: %q",
					tt.location, tt.project, tt.model, got, tt.want)
			}
		})
	}
}

// TestCountTokensEndpoint_GlobalIsNotGlobalHyphenated guards specifically
// against a regression where someone reverts the global host branch and
// the URL becomes "global-aiplatform.googleapis.com" (a nonexistent
// subdomain that returns generic HTML 404). The negative assertion is
// noisy but high-signal — this exact regression has happened once.
func TestCountTokensEndpoint_GlobalIsNotGlobalHyphenated(t *testing.T) {
	got := countTokensEndpoint("global", "p", "m")
	bad := "global-aiplatform.googleapis.com"
	if want := "aiplatform.googleapis.com"; !strings.Contains(got, want) {
		t.Errorf("expected URL to include %q, got %q", want, got)
	}
	if strings.Contains(got, bad) {
		t.Errorf("expected URL to NOT include %q (the nonexistent subdomain), got %q", bad, got)
	}
}
