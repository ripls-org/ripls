package github

import "testing"

// IsComplete decides whether a caller treats a missing GitHub integration as
// "not configured" (quiet, expected) or lets it reach NewAppClient and fail
// (an ERROR log, and an alert). Both paths end with a nil client, so this
// predicate is the only observable difference — which is why it is tested
// directly rather than through NewFromGitHubApp, where a deliberately invalid
// key would make an ungated config fail for the wrong reason and pass anyway.
func TestAppConfigIsComplete(t *testing.T) {
	complete := AppConfig{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     []byte("key"),
		Owner:          "example-org",
		Repo:           "example-repo",
	}

	if !complete.IsComplete() {
		t.Fatal("a fully populated config must be complete")
	}

	tests := []struct {
		name   string
		mutate func(*AppConfig)
	}{
		{"no app id", func(c *AppConfig) { c.AppID = 0 }},
		{"no installation id", func(c *AppConfig) { c.InstallationID = 0 }},
		{"no private key", func(c *AppConfig) { c.PrivateKey = nil }},
		// The regression: the owner and repo used to be omitted from the
		// caller's gate, so losing them produced an ERROR instead of the
		// documented disabled mode.
		{"no owner", func(c *AppConfig) { c.Owner = "" }},
		{"no repo", func(c *AppConfig) { c.Repo = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := complete
			tt.mutate(&cfg)
			if cfg.IsComplete() {
				t.Errorf("config missing %s must not be complete", tt.name)
			}
		})
	}
}

// Every field IsComplete checks must also be rejected by NewAppClient, so a
// caller that skips the predicate still cannot build a half-configured client.
func TestNewAppClientRejectsEveryIncompleteField(t *testing.T) {
	complete := AppConfig{
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     []byte("key"),
		Owner:          "example-org",
		Repo:           "example-repo",
	}

	for _, mutate := range []func(*AppConfig){
		func(c *AppConfig) { c.AppID = 0 },
		func(c *AppConfig) { c.InstallationID = 0 },
		func(c *AppConfig) { c.PrivateKey = nil },
		func(c *AppConfig) { c.Owner = "" },
		func(c *AppConfig) { c.Repo = "" },
	} {
		cfg := complete
		mutate(&cfg)
		if _, err := NewAppClient(t.Context(), cfg); err == nil {
			t.Errorf("NewAppClient accepted an incomplete config: %+v", cfg)
		}
	}
}
