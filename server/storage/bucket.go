package storage

// This package provides cloud bucket storage abstraction for binary media files.
// Implementations support Google Cloud Storage and local file-based storage for development.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/storage"

	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

// MediaBucketKey generates the bucket key for media storage.
// This is the single source of truth for media bucket key format.
// Key format: "userId/mediaId".
func MediaBucketKey(userID, mediaID string) string {
	return fmt.Sprintf("%s/%s", userID, mediaID)
}

// ThumbnailBucketKey generates the bucket key for a media thumbnail.
// Key format: "userId/mediaId-thumb".
func ThumbnailBucketKey(userID, mediaID string) string {
	return MediaBucketKey(userID, mediaID) + "-thumb"
}

// BucketStorage provides cloud storage for binary media files.
type BucketStorage interface {
	// Put stores media with a key and returns the permanent storage URL.
	Put(ctx context.Context, key string, data []byte, contentType string, metadata map[string]string) (string, error)

	// Get retrieves media by key. Returns data and content-type.
	Get(ctx context.Context, key string) ([]byte, string, error)

	// GetToFile streams an object directly to a local path, returning the object's
	// content type. No []byte materialises in the caller's heap.
	GetToFile(ctx context.Context, key, destPath string) (contentType string, err error)

	// PutFromFile streams a local file directly to bucket storage, returning the
	// permanent storage URL. No []byte materialises in the caller's heap.
	PutFromFile(ctx context.Context, key, srcPath, contentType string, metadata map[string]string) (storageURL string, err error)

	// Copy duplicates the object at srcKey to dstKey within the same storage backend
	// without routing bytes through the caller. Returns the storage URL for dstKey
	// and the byte size of the copied object.
	Copy(ctx context.Context, srcKey, dstKey string) (storageURL string, size int64, err error)

	// GetSignedURL generates a time-limited URL for external access (e.g., AI providers).
	// On GCS this signs via the runtime SA's IAM signBlob; the signature is only valid
	// until the SA's Google-managed key rotates (~1-2 weeks) regardless of duration, so
	// it suits short-lived use only. For long-lived URLs use GetDurableSignedURL.
	GetSignedURL(ctx context.Context, key string, duration time.Duration) (string, error)

	// GetDurableSignedURL generates a signed URL whose signature stays valid for the
	// full duration. On GCS this requires a configured user-managed signing key and
	// signs locally (no managed-key rotation dependency). See the GCS implementation
	// for the rationale (#2549).
	GetDurableSignedURL(ctx context.Context, key string, duration time.Duration) (string, error)

	// Delete removes media from storage.
	Delete(ctx context.Context, key string) error

	// CheckHealth verifies storage is accessible and returns status information.
	CheckHealth(ctx context.Context) ([]*health.Status, error)
}

// GCSBucketStorage implements BucketStorage using Google Cloud Storage native API.
// Uses Application Default Credentials (works automatically in GCP environments).
type GCSBucketStorage struct {
	client     *storage.Client
	bucketName string

	// signerEmail and signerPrivateKey, when set, enable GetDurableSignedURL to
	// sign locally with a user-managed key instead of the runtime SA's IAM
	// signBlob path. See GetDurableSignedURL for why this matters.
	signerEmail      string
	signerPrivateKey []byte
}

// GCSOption configures a GCSBucketStorage at construction time.
type GCSOption func(*GCSBucketStorage)

// WithURLSigner configures a user-managed signing identity (a service-account
// email and its PEM private key) used by GetDurableSignedURL. Without it,
// GetDurableSignedURL returns an error. email is the key's client_email and
// privateKeyPEM is its private_key.
func WithURLSigner(email string, privateKeyPEM []byte) GCSOption {
	return func(g *GCSBucketStorage) {
		g.signerEmail = email
		g.signerPrivateKey = privateKeyPEM
	}
}

// NewGCSBucketStorage creates a new GCS bucket storage.
// Uses Application Default Credentials - works automatically in Cloud Run.
func NewGCSBucketStorage(ctx context.Context, bucketName string, opts ...GCSOption) (*GCSBucketStorage, error) {
	// Use Application Default Credentials (works in Cloud Run with service account IAM)
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	g := &GCSBucketStorage{
		client:     client,
		bucketName: bucketName,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g, nil
}

// Put stores media in GCS bucket.
func (g *GCSBucketStorage) Put(ctx context.Context, key string, data []byte, contentType string, metadata map[string]string) (string, error) {
	obj := g.client.Bucket(g.bucketName).Object(key)
	writer := obj.NewWriter(ctx)
	writer.ContentType = contentType
	writer.Metadata = metadata

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return "", fmt.Errorf("failed to write to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close GCS writer: %w", err)
	}

	// Return GCS URL
	url := fmt.Sprintf("gs://%s/%s", g.bucketName, key)
	return url, nil
}

// Get retrieves media from GCS bucket.
func (g *GCSBucketStorage) Get(ctx context.Context, key string) ([]byte, string, error) {
	obj := g.client.Bucket(g.bucketName).Object(key)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to open GCS object: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read from GCS: %w", err)
	}

	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get GCS object attributes: %w", err)
	}

	return data, attrs.ContentType, nil
}

// GetToFile streams the GCS object identified by key directly to destPath.
// Returns the object's content type. The destination file is created or truncated.
func (g *GCSBucketStorage) GetToFile(ctx context.Context, key, destPath string) (string, error) {
	obj := g.client.Bucket(g.bucketName).Object(key)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to open GCS object: %w", err)
	}
	defer reader.Close()

	f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}

	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		return "", fmt.Errorf("failed to stream GCS object to file: %w", err)
	}

	if err := f.Close(); err != nil {
		return "", fmt.Errorf("failed to close destination file: %w", err)
	}

	// ContentType is populated from the response headers when the reader opens.
	return reader.Attrs.ContentType, nil
}

// PutFromFile streams the file at srcPath to the GCS object identified by key.
// Returns the permanent GCS storage URL.
func (g *GCSBucketStorage) PutFromFile(ctx context.Context, key, srcPath, contentType string, metadata map[string]string) (string, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %w", err)
	}
	defer f.Close()

	obj := g.client.Bucket(g.bucketName).Object(key)
	writer := obj.NewWriter(ctx)
	writer.ContentType = contentType
	writer.Metadata = metadata

	if _, err := io.Copy(writer, f); err != nil {
		writer.Close()
		return "", fmt.Errorf("failed to stream file to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close GCS writer: %w", err)
	}

	return fmt.Sprintf("gs://%s/%s", g.bucketName, key), nil
}

// Copy duplicates a GCS object within the same bucket using a server-side copy RPC.
// No object bytes transit through Cloud Run.
func (g *GCSBucketStorage) Copy(ctx context.Context, srcKey, dstKey string) (string, int64, error) {
	src := g.client.Bucket(g.bucketName).Object(srcKey)
	dst := g.client.Bucket(g.bucketName).Object(dstKey)
	attrs, err := dst.CopierFrom(src).Run(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("failed to copy GCS object %q to %q: %w", srcKey, dstKey, err)
	}
	return fmt.Sprintf("gs://%s/%s", g.bucketName, dstKey), attrs.Size, nil
}

// GetSignedURL generates a signed URL for GCS object.
func (g *GCSBucketStorage) GetSignedURL(ctx context.Context, key string, duration time.Duration) (string, error) {
	opts := &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(duration),
	}

	url, err := g.client.Bucket(g.bucketName).SignedURL(key, opts)
	if err != nil {
		return "", fmt.Errorf("failed to generate GCS signed URL: %w", err)
	}

	return url, nil
}

// GetDurableSignedURL generates a signed URL whose signature stays valid for the
// full duration, signed locally with a configured user-managed key. It requires
// WithURLSigner to have been supplied at construction; otherwise it errors.
//
// GetSignedURL (above) signs via the runtime service account's IAM signBlob, using
// the SA's Google-managed key. GCS validates that signature against the SA's
// currently-valid public keys, and Google rotates that key roughly every 1-2 weeks
// — so a signBlob-signed URL stops verifying (HTTP 403 SignatureDoesNotMatch) long
// before its stated Expires. That is fine for short-lived media URLs but breaks the
// multi-month feedback-screenshot URLs embedded in GitHub issues (#2549). Signing
// with an explicit user-managed key removes the rotation dependency: the signature
// holds for the full Expires.
func (g *GCSBucketStorage) GetDurableSignedURL(_ context.Context, key string, duration time.Duration) (string, error) {
	if g.signerEmail == "" || len(g.signerPrivateKey) == 0 {
		return "", fmt.Errorf("durable signed URL requested but no URL signer is configured")
	}

	opts := &storage.SignedURLOptions{
		// V2 (explicit) imposes no expiry cap; V4 would cap Expires at 7 days,
		// too short for a feedback URL that must outlive its GitHub issue.
		Scheme:         storage.SigningSchemeV2,
		Method:         "GET",
		Expires:        time.Now().Add(duration),
		GoogleAccessID: g.signerEmail,
		PrivateKey:     g.signerPrivateKey,
	}

	// Package-level SignedURL signs locally from the explicit key — no GCS client
	// or network call — so this works (and unit-tests) without live credentials.
	url, err := storage.SignedURL(g.bucketName, key, opts)
	if err != nil {
		return "", fmt.Errorf("failed to generate durable GCS signed URL: %w", err)
	}

	return url, nil
}

// Delete removes media from GCS bucket.
func (g *GCSBucketStorage) Delete(ctx context.Context, key string) error {
	obj := g.client.Bucket(g.bucketName).Object(key)
	if err := obj.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete from GCS: %w", err)
	}
	return nil
}

// CheckHealth verifies GCS bucket is accessible by fetching bucket attributes.
func (g *GCSBucketStorage) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	start := time.Now()

	status := &health.Status{
		Name:    "storage",
		Backend: "gcs",
		Metadata: map[string]string{
			"bucket": g.bucketName,
		},
	}

	_, err := g.client.Bucket(g.bucketName).Attrs(ctx)
	status.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		status.Error = err.Error()
	}

	return []*health.Status{status}, nil
}

// LocalBucketStorage implements BucketStorage using local filesystem.
// Intended for development and testing. Returns HTTP URLs that are served by the server.
type LocalBucketStorage struct {
	basePath  string
	serverURL string // e.g., "http://localhost:8080"
}

// NewLocalBucketStorage creates a new file-based bucket storage for local development.
// serverURL should be the base URL of the server (e.g., "http://localhost:8080").
func NewLocalBucketStorage(basePath, serverURL string) (*LocalBucketStorage, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalBucketStorage{
		basePath:  basePath,
		serverURL: serverURL,
	}, nil
}

// GetBasePath returns the filesystem base path for serving files.
func (l *LocalBucketStorage) GetBasePath() string {
	return l.basePath
}

// resolveKey joins key onto basePath, rejecting any key that would escape it.
//
// Every caller today builds keys with MediaBucketKey/ThumbnailBucketKey, whose
// components are a user ID and a generated media ID — so no traversal is
// reachable in the current tree. This is here so that stays true without
// depending on caller discipline: filepath.Join CLEANS its result, so a key of
// "../../etc/passwd" resolves outside basePath silently and every method on
// this type would then read or write there.
//
// filepath.IsLocal is the precise test — it rejects absolute paths, empty
// paths, and anything that climbs out with "..", while allowing the
// "userID/mediaID" shape the real keys use. Surfaced by gosec's G703 taint
// analysis, new in the golangci-lint v2 toolchain (#3008).
func (l *LocalBucketStorage) resolveKey(key string) (string, error) {
	if !filepath.IsLocal(key) {
		return "", fmt.Errorf("invalid storage key %q: must be a relative path within the bucket", key)
	}
	return filepath.Join(l.basePath, key), nil
}

// Put stores media in local filesystem.
func (l *LocalBucketStorage) Put(ctx context.Context, key string, data []byte, contentType string, metadata map[string]string) (string, error) {
	filePath, err := l.resolveKey(key)
	if err != nil {
		return "", err
	}

	// Create directory structure if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G703: path comes from resolveKey, which rejects any key escaping basePath; gosec cannot see that guard.
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file with restricted permissions (owner-only for security)
	if err := os.WriteFile(filePath, data, 0o600); err != nil { //nolint:gosec // G703: path comes from resolveKey, which rejects any key escaping basePath; gosec cannot see that guard.
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Store metadata in a separate file
	if len(metadata) > 0 || contentType != "" {
		metadataPath := filePath + ".metadata"
		metadataContent := fmt.Sprintf("content_type: %s\n", contentType)
		for k, v := range metadata {
			metadataContent += fmt.Sprintf("%s: %s\n", k, v)
		}
		if err := os.WriteFile(metadataPath, []byte(metadataContent), 0o600); err != nil { //nolint:gosec // G703: path comes from resolveKey, which rejects any key escaping basePath; gosec cannot see that guard.
			// Non-fatal: metadata write failure doesn't prevent file storage
			logger := logging.LoggerWithContext(ctx)
			logger.WarnContext(ctx, "failed to write metadata file", "key", key, "error", err)
		}
	}

	// Return HTTP URL that will be served by the server
	url := fmt.Sprintf("%s/media/%s", l.serverURL, key)
	return url, nil
}

// readLocalContentType reads the content type from a .metadata sidecar for filePath.
// Returns empty string if the sidecar is absent or unparseable.
func readLocalContentType(filePath string) string {
	metadataBytes, err := os.ReadFile(filePath + ".metadata") //nolint:gosec // G703: path comes from resolveKey, which rejects any key escaping basePath; gosec cannot see that guard.
	if err != nil {
		return ""
	}
	for _, line := range bytes.Split(metadataBytes, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("content_type: ")) {
			return string(bytes.TrimPrefix(line, []byte("content_type: ")))
		}
	}
	return ""
}

// Get retrieves media from local filesystem.
func (l *LocalBucketStorage) Get(ctx context.Context, key string) ([]byte, string, error) {
	filePath, err := l.resolveKey(key)
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(filePath) //nolint:gosec // G703: path comes from resolveKey, which rejects any key escaping basePath; gosec cannot see that guard.
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}

	return data, readLocalContentType(filePath), nil
}

// GetToFile hard-links (or byte-copies) the stored object at key to destPath.
// Returns the object's content type from the .metadata sidecar.
func (l *LocalBucketStorage) GetToFile(_ context.Context, key, destPath string) (string, error) {
	srcPath, err := l.resolveKey(key)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %w", err)
	}

	if err := os.Link(srcPath, destPath); err != nil {
		if err := copyLocalFile(srcPath, destPath); err != nil {
			return "", fmt.Errorf("failed to copy file to destination: %w", err)
		}
	}

	return readLocalContentType(srcPath), nil
}

// PutFromFile hard-links (or byte-copies) srcPath into the storage location for key.
// Writes a .metadata sidecar and returns the HTTP URL for the stored object.
func (l *LocalBucketStorage) PutFromFile(ctx context.Context, key, srcPath, contentType string, metadata map[string]string) (string, error) {
	dstPath, err := l.resolveKey(key)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %w", err)
	}

	if err := os.Link(srcPath, dstPath); err != nil {
		if err := copyLocalFile(srcPath, dstPath); err != nil {
			return "", fmt.Errorf("failed to store file: %w", err)
		}
	}

	if len(metadata) > 0 || contentType != "" {
		metadataContent := fmt.Sprintf("content_type: %s\n", contentType)
		for k, v := range metadata {
			metadataContent += fmt.Sprintf("%s: %s\n", k, v)
		}
		if err := os.WriteFile(dstPath+".metadata", []byte(metadataContent), 0o600); err != nil {
			logger := logging.LoggerWithContext(ctx)
			logger.WarnContext(ctx, "failed to write metadata file", "key", key, "error", err)
		}
	}

	return fmt.Sprintf("%s/media/%s", l.serverURL, key), nil
}

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const requestHostKey = contextKey("requestHost")

// GetSignedURL generates an HTTP URL for local access.
// Duration is ignored for local storage since the URL is always accessible.
// Uses the request host from context if available, otherwise falls back to serverURL.
func (l *LocalBucketStorage) GetSignedURL(ctx context.Context, key string, duration time.Duration) (string, error) {
	filePath, err := l.resolveKey(key)
	if err != nil {
		return "", err
	}

	// Verify file exists
	if _, err := os.Stat(filePath); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	// Try to get request host from context (set by middleware)
	var baseURL string
	if host, ok := ctx.Value(requestHostKey).(string); ok && host != "" {
		// Use http:// (not https://) for local development
		baseURL = fmt.Sprintf("http://%s", host)
	} else {
		// Fallback to configured serverURL
		baseURL = l.serverURL
	}

	// Return absolute URL using the detected or configured host
	url := fmt.Sprintf("%s/media/%s", baseURL, key)
	return url, nil
}

// GetDurableSignedURL returns the same server-served URL as GetSignedURL. Local
// storage serves files directly (no signature), so there is no rotation concern
// to defend against; durability is inherent. Present to satisfy BucketStorage.
func (l *LocalBucketStorage) GetDurableSignedURL(ctx context.Context, key string, duration time.Duration) (string, error) {
	return l.GetSignedURL(ctx, key, duration)
}

// WithRequestHost adds the request host to the context for URL generation.
// This should be called by HTTP middleware to enable per-request URL customization.
func WithRequestHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, requestHostKey, host)
}

// Copy duplicates a local file by hard link (same filesystem) or byte copy (cross-device).
// Also copies the .metadata sidecar when present.
func (l *LocalBucketStorage) Copy(_ context.Context, srcKey, dstKey string) (string, int64, error) {
	srcPath, err := l.resolveKey(srcKey)
	if err != nil {
		return "", 0, err
	}
	dstPath, err := l.resolveKey(dstKey)
	if err != nil {
		return "", 0, err
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return "", 0, fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Prefer hard link (instant, no data duplication on same filesystem).
	if err := os.Link(srcPath, dstPath); err != nil {
		// Fall back to byte copy (e.g. cross-device mount or unsupported filesystem).
		if err := copyLocalFile(srcPath, dstPath); err != nil {
			return "", 0, fmt.Errorf("failed to copy file %q to %q: %w", srcKey, dstKey, err)
		}
	}

	// Propagate metadata sidecar so content-type is preserved on the copy.
	if _, statErr := os.Stat(srcPath + ".metadata"); statErr == nil {
		_ = copyLocalFile(srcPath+".metadata", dstPath+".metadata")
	}

	info, err := os.Stat(dstPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to stat copied file: %w", err)
	}

	return fmt.Sprintf("%s/media/%s", l.serverURL, dstKey), info.Size(), nil
}

// copyLocalFile copies a file byte-for-byte from src to dst.
func copyLocalFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy bytes: %w", err)
	}

	return out.Close()
}

// Delete removes media from local filesystem.
func (l *LocalBucketStorage) Delete(ctx context.Context, key string) error {
	filePath, err := l.resolveKey(key)
	if err != nil {
		return err
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Also try to delete metadata file (ignore errors)
	metadataPath := filePath + ".metadata"
	os.Remove(metadataPath)

	return nil
}

// CheckHealth verifies the storage directory is accessible.
func (l *LocalBucketStorage) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	start := time.Now()

	status := &health.Status{
		Name:    "storage",
		Backend: "local",
		Metadata: map[string]string{
			"path": l.basePath,
		},
	}

	info, err := os.Stat(l.basePath)
	status.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		status.Error = err.Error()
	} else if !info.IsDir() {
		status.Error = fmt.Sprintf("storage path is not a directory: %s", l.basePath)
	}

	return []*health.Status{status}, nil
}
