package request

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestModelRequestStateToApi(t *testing.T) {
	tests := []struct {
		name  string
		input models.RequestState
		want  api.RequestState
	}{
		{"active", models.RequestState_REQUEST_STATE_ACTIVE, api.RequestState_REQUEST_STATE_ACTIVE},
		{"offers_received", models.RequestState_REQUEST_STATE_OFFERS_RECEIVED, api.RequestState_REQUEST_STATE_OFFERS_RECEIVED},
		{"fulfilled", models.RequestState_REQUEST_STATE_FULFILLED, api.RequestState_REQUEST_STATE_FULFILLED},
		{"cancelled", models.RequestState_REQUEST_STATE_CANCELLED, api.RequestState_REQUEST_STATE_CANCELLED},
		{"unspecified", models.RequestState_REQUEST_STATE_UNSPECIFIED, api.RequestState_REQUEST_STATE_UNSPECIFIED},
		{"unknown value", models.RequestState(99), api.RequestState_REQUEST_STATE_UNSPECIFIED},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := modelRequestStateToAPI(tc.input)
			if got != tc.want {
				t.Errorf("modelRequestStateToAPI(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestApiRequestStateToModel(t *testing.T) {
	tests := []struct {
		name  string
		input api.RequestState
		want  models.RequestState
	}{
		{"active", api.RequestState_REQUEST_STATE_ACTIVE, models.RequestState_REQUEST_STATE_ACTIVE},
		{"offers_received", api.RequestState_REQUEST_STATE_OFFERS_RECEIVED, models.RequestState_REQUEST_STATE_OFFERS_RECEIVED},
		{"fulfilled", api.RequestState_REQUEST_STATE_FULFILLED, models.RequestState_REQUEST_STATE_FULFILLED},
		{"cancelled", api.RequestState_REQUEST_STATE_CANCELLED, models.RequestState_REQUEST_STATE_CANCELLED},
		{"unspecified", api.RequestState_REQUEST_STATE_UNSPECIFIED, models.RequestState_REQUEST_STATE_UNSPECIFIED},
		{"unknown value", api.RequestState(99), models.RequestState_REQUEST_STATE_UNSPECIFIED},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := apiRequestStateToModel(tc.input)
			if got != tc.want {
				t.Errorf("apiRequestStateToModel(%v) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestVulnerabilityLevelToString(t *testing.T) {
	tests := []struct {
		name  string
		want  string
		input api.SocialVulnerabilityLevel
	}{
		{"high", "high", api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH},
		{"medium", "medium", api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM},
		{"low", "low", api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_LOW},
		{"unspecified", "", api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED},
		{"unknown value", "", api.SocialVulnerabilityLevel(99)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := vulnerabilityLevelToString(tc.input)
			if got != tc.want {
				t.Errorf("vulnerabilityLevelToString(%v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
