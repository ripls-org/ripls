package storage

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"go.ripls.org/ripls/server/logging"
)

// SafeDSNFields parses a PostgreSQL DSN and returns structured fields safe for
// logging (host, database name, user, sslmode). The password is never included.
func SafeDSNFields(connectionString string) []any {
	u, err := url.Parse(connectionString)
	if err != nil {
		return []any{"db_host", "unknown"}
	}
	fields := []any{
		"db_host", u.Host,
		"db_name", strings.TrimPrefix(u.Path, "/"),
		"db_user", u.User.Username(),
	}
	if sslmode := u.Query().Get("sslmode"); sslmode != "" {
		fields = append(fields, "db_sslmode", sslmode)
	}
	return fields
}

// InitializeDatabase validates the DSN scheme, logs its password-free
// connection fields, and opens PostgreSQL storage with the default storage
// types. The DSN itself never appears in errors or logs — it can embed the
// database password.
func InitializeDatabase(ctx context.Context, connectionString string, logger *logging.Logger) (*ProtoSQLStorage, error) {
	if !strings.HasPrefix(connectionString, "postgres://") && !strings.HasPrefix(connectionString, "postgresql://") {
		return nil, fmt.Errorf("invalid database URL: must be a PostgreSQL connection string (postgres:// or postgresql://)")
	}
	logger.Info("using PostgreSQL database", SafeDSNFields(connectionString)...)
	return InitializePostgreSQLDatabase(ctx, connectionString, DefaultStorageTypes())
}

// InitializeBucketStorage opens bucket storage from configuration. Exactly one
// of localStoragePath (development: local filesystem) or gcsBucketName
// (production: GCS) must be set.
func InitializeBucketStorage(ctx context.Context, localStoragePath, gcsBucketName, serverURL, signerEmail, signerPrivateKey string, logger *logging.Logger) (BucketStorage, error) {
	localSet := localStoragePath != ""
	gcsSet := gcsBucketName != ""

	if !localSet && !gcsSet {
		return nil, fmt.Errorf("exactly one of --local-media-storage or --gcs-bucket must be specified")
	}
	if localSet && gcsSet {
		return nil, fmt.Errorf("--local-media-storage and --gcs-bucket are mutually exclusive")
	}

	if localSet {
		logger.Info("initializing local bucket storage", "path", localStoragePath)
		bucket, err := NewLocalBucketStorage(localStoragePath, serverURL)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize local bucket storage: %w", err)
		}
		logger.Info("local bucket storage initialized")
		return bucket, nil
	}
	logger.Info("initializing GCS bucket storage", "bucket", gcsBucketName)
	var opts []GCSOption
	if signerEmail != "" && signerPrivateKey != "" {
		// Durable feedback-screenshot URLs require an explicit user-managed signer
		// (see GetDurableSignedURL / #2549). Absent ⇒ those URLs fall back to
		// short-lived signBlob signing.
		opts = append(opts, WithURLSigner(signerEmail, []byte(signerPrivateKey)))
		logger.Info("GCS URL signer configured for durable feedback URLs")
	}
	bucket, err := NewGCSBucketStorage(ctx, gcsBucketName, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCS bucket storage: %w", err)
	}
	logger.Info("GCS bucket storage initialized")
	return bucket, nil
}
