package feedback

import (
	githubclient "go.ripls.org/ripls/server/github"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// NewFromForgejo constructs the feedback service for a tracker hosted on a
// Forgejo instance. Like NewFromGitHubApp, an incomplete config or a
// client-construction failure degrades to a service with a nil client:
// feedback RPCs then report the service unavailable instead of failing server
// startup.
func NewFromForgejo(cfg githubclient.ForgejoConfig, bucket storage.BucketStorage) *Service {
	logger := logging.Default()
	svc := func(client githubclient.Client) *Service {
		s := New(client, cfg.Owner, cfg.Repo, bucket)
		s.backend = backendForgejo
		return s
	}

	if !cfg.IsComplete() {
		logger.Warn("Forgejo tracker not fully configured - feedback service will not create issues",
			"has_base_url", cfg.BaseURL != "",
			"has_token", cfg.Token != "",
			"repo_owner", cfg.Owner,
			"repo_name", cfg.Repo)
		return svc(nil)
	}

	client, err := githubclient.NewForgejoClient(cfg)
	if err != nil {
		logger.Error("failed to create Forgejo tracker client", "error", err)
		// Degrade to a nil client rather than failing startup.
		return svc(nil)
	}

	logger.Info("feedback service initialized with a Forgejo tracker",
		"base_url", cfg.BaseURL,
		"repo_owner", cfg.Owner,
		"repo_name", cfg.Repo)
	return svc(client)
}
