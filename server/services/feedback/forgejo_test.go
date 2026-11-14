package feedback

import (
	"context"
	"testing"

	githubclient "go.ripls.org/ripls/server/github"
)

// NewFromForgejo always returns a usable service. Without a full config it has
// no client, and the health check says so under the Forgejo backend's name.
func TestNewFromForgejoDegradesWhenUnconfigured(t *testing.T) {
	full := githubclient.ForgejoConfig{BaseURL: "https://code.example.org", Token: "t", Owner: "o", Repo: "r"}
	tests := map[string]githubclient.ForgejoConfig{
		"nothing configured": {},
		"no token":           {BaseURL: full.BaseURL, Owner: "o", Repo: "r"},
		"no owner":           {BaseURL: full.BaseURL, Token: "t", Repo: "r"},
		"bad base URL":       {BaseURL: "not a url", Token: "t", Owner: "o", Repo: "r"},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			svc := NewFromForgejo(cfg, nil)
			if svc == nil {
				t.Fatal("NewFromForgejo must always return a service")
			}
			if svc.githubClient != nil {
				t.Error("an unusable config must leave the client nil")
			}
			statuses, err := svc.CheckHealth(context.Background())
			if err != nil {
				t.Fatalf("CheckHealth: %v", err)
			}
			if len(statuses) != 1 || statuses[0].Name != "forgejo" || statuses[0].Error == "" {
				t.Errorf("health = %+v, want one failing status named forgejo", statuses)
			}
		})
	}

	svc := NewFromForgejo(full, nil)
	if svc.githubClient == nil {
		t.Fatal("a full config must build a client")
	}
	statuses, err := svc.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if statuses[0].Name != "forgejo" || statuses[0].Backend != "forgejo" || statuses[0].Error != "" {
		t.Errorf("health = %+v, want a healthy status named forgejo", statuses[0])
	}
}

// The GitHub path keeps reporting under its own name.
func TestHealthNamesTheGitHubBackend(t *testing.T) {
	svc := New(&githubclient.MockClient{}, "o", "r", nil)
	statuses, err := svc.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if statuses[0].Name != "github" || statuses[0].Backend != "github" || statuses[0].Error != "" {
		t.Errorf("health = %+v, want a healthy status named github", statuses[0])
	}
}
