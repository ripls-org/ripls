package feedback

import (
	"context"
	"testing"

	githubclient "go.ripls.org/ripls/server/github"
)

// NewFromGitHubApp's contract is that it always returns a usable service and
// never a client it couldn't build — feedback RPCs then skip issue creation
// instead of failing startup.
//
// Note what this test can and cannot see. Every case below ends with a nil
// client whether the gate rejected the config or NewAppClient did, so this
// does NOT catch the #2953 regression where Owner/Repo were missing from the
// gate and an unconfigured deployment logged at ERROR on every boot. The
// observable difference there is the log level, and the decision behind it is
// AppConfig.IsComplete — tested directly in
// server/github/appconfig_test.go, where removing the owner check does fail
// the test.
func TestNewFromGitHubAppDegradesWhenUnconfigured(t *testing.T) {
	// A syntactically valid key is not needed: every case here must be
	// rejected by the gate before NewAppClient is reached.
	somePrivateKey := []byte("-----BEGIN RSA PRIVATE KEY-----\nnot-a-real-key\n-----END RSA PRIVATE KEY-----")

	tests := []struct {
		name string
		cfg  githubclient.AppConfig
	}{
		{"nothing configured", githubclient.AppConfig{}},
		{"no app id", githubclient.AppConfig{
			InstallationID: 2, PrivateKey: somePrivateKey, Owner: "o", Repo: "r",
		}},
		{"no installation id", githubclient.AppConfig{
			AppID: 1, PrivateKey: somePrivateKey, Owner: "o", Repo: "r",
		}},
		{"no private key", githubclient.AppConfig{
			AppID: 1, InstallationID: 2, Owner: "o", Repo: "r",
		}},
		{
			// The regression: credentials present, repo coordinates missing.
			"credentials but no owner",
			githubclient.AppConfig{
				AppID: 1, InstallationID: 2, PrivateKey: somePrivateKey, Repo: "r",
			},
		},
		{"credentials but no repo", githubclient.AppConfig{
			AppID: 1, InstallationID: 2, PrivateKey: somePrivateKey, Owner: "o",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewFromGitHubApp(context.Background(), tt.cfg, nil)
			if svc == nil {
				t.Fatal("NewFromGitHubApp returned nil; it must always return a service")
			}
			if svc.githubClient != nil {
				t.Error("expected a nil GitHub client for an unconfigured integration")
			}
		})
	}
}
