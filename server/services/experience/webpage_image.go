package experience

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/safehttp"
	"go.ripls.org/ripls/server/storage"
)

const (
	// imageDownloadTimeout is the timeout for downloading webpage images.
	imageDownloadTimeout = 10 * time.Second

	// maxImageSize is the maximum image size to download. Shares the
	// codebase-wide image cap (#1953) so every media path bounds the same.
	maxImageSize = storage.MaxImageUploadBytes
)

// downloadAndStoreWebpageImage downloads an image from a URL and stores it as media.
// Returns the media ID on success, or empty string if download fails (non-fatal).
// Failures are logged but don't block experience creation.
func (s *Service) downloadAndStoreWebpageImage(
	ctx context.Context,
	imageURL string,
	userID string,
) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "downloadAndStoreWebpageImage",
		"user_id", userID,
	)
	startTime := time.Now()

	// Validate scheme before making any network request.
	parsedURL, err := url.Parse(imageURL)
	if err != nil {
		logger.Warn("invalid image URL", "error", err)
		return "", fmt.Errorf("invalid image URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		logger.Warn("image URL has unsupported scheme", "scheme", parsedURL.Scheme)
		return "", fmt.Errorf("unsupported image URL scheme: %q (must be http or https)", parsedURL.Scheme)
	}

	// Use the SSRF-guarded client for user-supplied og:image URLs.
	// Tests inject imageDownloadClient to allow loopback httptest servers;
	// when it is set the IP-literal pre-check below is also skipped.
	clientFactory := s.imageDownloadClient
	usingDefaultClient := clientFactory == nil
	if usingDefaultClient {
		clientFactory = func(t time.Duration) *http.Client {
			return safehttp.NewClient(
				safehttp.WithTimeout(t),
				safehttp.WithRedirectSchemes("http", "https"),
			)
		}
	}
	client := clientFactory(imageDownloadTimeout)

	// Fail fast for IP-literal hosts in non-public ranges when using the
	// production client. The dialer Control hook re-enforces this at
	// connect time to catch DNS rebinding.
	if usingDefaultClient {
		if host := parsedURL.Hostname(); !safehttp.IsHostPubliclyRoutable(host) {
			logger.Warn("image URL host is not publicly routable",
				"reason", "ssrf_blocked")
			return "", fmt.Errorf("image URL host is not publicly routable")
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		logger.Warn("failed to create image request", "error", err)
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set reasonable headers
	req.Header.Set("User-Agent", s.branding.BotUserAgent())
	req.Header.Set("Accept", "image/*")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		if safehttp.IsSSRFBlockError(err) {
			logger.Warn("image download blocked by SSRF guard",
				"reason", "ssrf_blocked",
				"error", err,
				"duration_ms", time.Since(startTime).Milliseconds())
			return "", fmt.Errorf("image URL is not publicly routable")
		}
		logger.Warn("failed to download image", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		logger.Warn("image download returned non-OK status", "status", resp.StatusCode)
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Validate content type is an image
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		logger.Warn("image URL returned non-image content type", "content_type", contentType)
		return "", fmt.Errorf("not an image: content-type %s", contentType)
	}

	// Read image data with size limit
	limitedReader := io.LimitReader(resp.Body, maxImageSize+1)
	imageData, err := io.ReadAll(limitedReader)
	if err != nil {
		logger.Warn("failed to read image data", "error", err)
		return "", fmt.Errorf("failed to read image: %w", err)
	}

	// Check if image exceeded size limit
	if int64(len(imageData)) > maxImageSize {
		logger.Warn("image exceeds size limit", "size_bytes", len(imageData), "limit_bytes", maxImageSize)
		return "", fmt.Errorf("image too large: %d bytes (max %d)", len(imageData), maxImageSize)
	}

	// Store as media (empty sourceStockImageID since this is not from stock imagery)
	mediaID, err := storage.StoreMedia(
		ctx,
		s.storage,
		s.bucket,
		userID,
		imageData,
		contentType,
		"webpage-image.jpg",
		"Image extracted from webpage",
		"",    // sourceStockImageID - not from stock
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		logger.Warn("failed to store webpage image", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return "", fmt.Errorf("failed to store image: %w", err)
	}

	logger.Info(
		"successfully downloaded and stored webpage image",
		"media_id", mediaID,
		"size_bytes", len(imageData),
		"content_type", contentType,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return mediaID, nil
}
