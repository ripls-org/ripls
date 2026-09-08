// Package firebase provides shared Firebase app initialization for use across
// the server (FCM, phone auth, etc.).
package firebase

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"

	"go.ripls.org/ripls/server/logging"
)

// NewApp initializes a Firebase app instance using Application Default
// Credentials. The projectID must be set when ADC cannot infer it from the
// environment — required for local dev under
// `gcloud auth application-default login`. In Cloud Run / GKE it can be left
// empty; the SDK reads it from the metadata server. Call once at startup
// and share the app across services.
func NewApp(ctx context.Context, projectID string) (*firebase.App, error) {
	logger := logging.LoggerWithContext(ctx)

	cfg := &firebase.Config{ProjectID: projectID}
	app, err := firebase.NewApp(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase app with ADC: %w", err)
	}
	if projectID == "" {
		logger.InfoContext(ctx, "initialized Firebase app with ADC; project ID will be inferred from metadata server")
	} else {
		logger.InfoContext(ctx, "initialized Firebase app with ADC", "project_id", projectID)
	}
	return app, nil
}
