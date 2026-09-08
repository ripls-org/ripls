package feedback

import (
	"context"

	githubclient "go.ripls.org/ripls/server/github"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// NewFromGitHubApp constructs the feedback service from GitHub App
// credentials. Missing credentials or a client-construction failure degrade
// to a service with a nil client — feedback RPCs then skip issue creation
// instead of failing server startup.
func NewFromGitHubApp(ctx context.Context, cfg githubclient.AppConfig, bucket storage.BucketStorage) *Service {
	logger := logging.Default()
	// "Not configured" must cover the repo coordinates too, not just the
	// credentials. This used to check only AppID/InstallationID/PrivateKey, so
	// a deployment with valid credentials and no target repo fell through to
	// the error branch below and logged at ERROR on every boot — alerting on a
	// configuration gap that the documented contract says should degrade
	// quietly to disabled (#2953).
	if !cfg.IsComplete() {
		logger.Warn("GitHub App not configured - feedback service will not create issues",
			"app_id", cfg.AppID,
			"installation_id", cfg.InstallationID,
			"has_private_key", len(cfg.PrivateKey) > 0,
			"repo_owner", cfg.Owner,
			"repo_name", cfg.Repo)
		return New(nil, cfg.Owner, cfg.Repo, bucket)
	}

	client, err := githubclient.NewAppClient(ctx, cfg)
	if err != nil {
		logger.Error("failed to create GitHub App client", "error", err)
		// Degrade to a nil client rather than failing startup.
		return New(nil, cfg.Owner, cfg.Repo, bucket)
	}

	logger.Info("feedback service initialized with GitHub App",
		"app_id", cfg.AppID,
		"installation_id", cfg.InstallationID,
		"repo_owner", cfg.Owner,
		"repo_name", cfg.Repo)

	return New(client, cfg.Owner, cfg.Repo, bucket)
}
