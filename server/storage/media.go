package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/adrium/goheif" // Register HEIC/HEIF decoder
	"github.com/adrium/goheif/heif"
	"github.com/disintegration/imaging"
	"github.com/rwcarlsen/goexif/exif"
	_ "golang.org/x/image/tiff" // Register TIFF decoder
	_ "golang.org/x/image/webp" // Register WebP decoder

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

const (
	// Thumbnail generation thresholds.
	thumbnailSizeThresholdBytes   = 500 * 1024 // 500 KB
	thumbnailDimensionThresholdPx = 1000       // 1000px on longest side
	thumbnailTargetSizePx         = 400        // 400px on longest side
	thumbnailJPEGQuality          = 85         // JPEG quality for thumbnails (0-100)
)

// isImageContentType checks if the content type represents an image.
func isImageContentType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "image/")
}

// isVideoContentType checks if the content type represents a video.
func isVideoContentType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "video/")
}

// shouldGenerateThumbnail determines if a thumbnail should be generated.
// For videos, always attempts generation (ffmpeg extracts a still frame).
// For images, only generates when the file exceeds size or dimension thresholds.
func shouldGenerateThumbnail(data []byte, contentType string) bool {
	// Video thumbnails are always attempted (ffmpeg frame extraction)
	if isVideoContentType(contentType) {
		return true
	}

	// Only generate thumbnails for images (not pdf, audio, etc.)
	if !isImageContentType(contentType) {
		return false
	}

	// Check file size threshold
	if len(data) < thumbnailSizeThresholdBytes {
		// Read only the header to check dimensions — DecodeConfig allocates no
		// pixel buffer, so a malformed/huge-dimension header can't OOM here.
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			// If we can't read the config, don't generate a thumbnail
			return false
		}

		// Check if dimensions are large enough to warrant a thumbnail
		maxDimension := max(cfg.Width, cfg.Height)

		return maxDimension > thumbnailDimensionThresholdPx
	}

	// File size is large enough, generate thumbnail
	return true
}

// generateVideoThumbnail extracts a still frame from the video at inputPath using ffmpeg.
// It first tries at 1 second; if that fails (e.g., video shorter than 1s), it retries at 0s.
// On success the resized JPEG thumbnail is written to a caller-owned temp directory and its
// path is returned. The caller must clean up via os.RemoveAll(filepath.Dir(thumbnailPath)).
// On failure the function cleans up its own temp directory and returns ("", err).
func generateVideoThumbnail(ctx context.Context, inputPath string) (thumbnailPath string, err error) {
	logger := logging.LoggerWithContext(ctx)

	// Create a temporary directory for ffmpeg output only; the caller owns the input file.
	tmpDir, err := os.MkdirTemp("", "ripls-video-thumb-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Clean up the temp dir on failure; on success the caller takes ownership.
	succeeded := false
	defer func() {
		if !succeeded {
			if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
				logger.WarnContext(ctx, "failed to remove thumbnail temp dir on error",
					"path", tmpDir, "error", removeErr)
			}
		}
	}()

	outputPath := filepath.Join(tmpDir, "thumbnail.jpg")

	// Try at 1s first; retry at 0s for videos shorter than 1 second.
	var lastErr error
	for _, timestamp := range []string{"00:00:01", "00:00:00"} {
		var stderr bytes.Buffer
		// inputPath is caller-supplied (trusted temp dir); outputPath is internal.
		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-i", inputPath,
			"-ss", timestamp,
			"-vframes", "1",
			"-f", "image2",
			"-y",
			outputPath,
		)
		cmd.Stderr = &stderr

		if runErr := cmd.Run(); runErr != nil {
			lastErr = fmt.Errorf("ffmpeg failed at %s: %w", timestamp, runErr)
			logger.WarnContext(ctx, "ffmpeg thumbnail attempt failed",
				"timestamp", timestamp,
				"exit_error", runErr.Error(),
				"stderr", stderr.String(),
			)
			continue
		}
		lastErr = nil
		break
	}

	if lastErr != nil {
		return "", lastErr
	}

	// Read the raw extracted frame.
	frameData, err := os.ReadFile(outputPath) // path is from trusted temp dir
	if err != nil {
		return "", fmt.Errorf("failed to read ffmpeg output: %w", err)
	}

	if len(frameData) == 0 {
		return "", fmt.Errorf("ffmpeg produced empty output")
	}

	// Decode and resize to the standard thumbnail size.
	img, _, err := image.Decode(bytes.NewReader(frameData))
	if err != nil {
		return "", fmt.Errorf("failed to decode ffmpeg output frame: %w", err)
	}

	thumbnail := imaging.Fit(img, thumbnailTargetSizePx, thumbnailTargetSizePx, imaging.Lanczos)

	// Write the resized thumbnail back to disk (overwriting the raw ffmpeg frame).
	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("failed to create thumbnail file: %w", err)
	}
	if encErr := imaging.Encode(f, thumbnail, imaging.JPEG, imaging.JPEGQuality(thumbnailJPEGQuality)); encErr != nil {
		f.Close()
		return "", fmt.Errorf("failed to encode thumbnail: %w", encErr)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("failed to close thumbnail file: %w", err)
	}

	succeeded = true
	return outputPath, nil
}

// getEXIFOrientation extracts EXIF orientation from image data.
// Returns the EXIF orientation value (1-8) or 1 (no rotation) if not found.
func getEXIFOrientation(data []byte) int {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return 1
	}
	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return 1
	}
	orient, err := tag.Int(0)
	if err != nil {
		return 1
	}
	return orient
}

// getHEICRotations extracts ISOBMFF rotation from HEIC image data.
// Returns the number of 90-degree counter-clockwise rotations (0-3).
func getHEICRotations(data []byte) int {
	hf := heif.Open(bytes.NewReader(data))
	item, err := hf.PrimaryItem()
	if err != nil {
		return 0
	}
	return item.Rotations()
}

// applyEXIFOrientation applies EXIF orientation rotation to an image.
// Only handles rotation cases (3, 6, 8), ignoring flip cases.
func applyEXIFOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 3:
		// Rotate 180
		return imaging.Rotate180(img)
	case 6:
		// Rotate 270 CW (90 CCW)
		return imaging.Rotate270(img)
	case 8:
		// Rotate 90 CW (270 CCW)
		return imaging.Rotate90(img)
	default:
		// No rotation needed
		return img
	}
}

// applyHEICRotations applies ISOBMFF rotation to an image.
// rotations is the number of 90-degree counter-clockwise rotations (0-3).
func applyHEICRotations(img image.Image, rotations int) image.Image {
	for range rotations {
		img = imaging.Rotate90(img)
	}
	return img
}

// generateThumbnail creates a thumbnail from image data.
// Returns the thumbnail data as JPEG bytes, or an error.
// The thumbnail respects EXIF and ISOBMFF rotation metadata, returning a pre-rotated thumbnail.
func generateThumbnail(data []byte) ([]byte, error) {
	// Decode the image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Apply orientation based on format
	if format == "heic" {
		// HEIC uses ISOBMFF rotation metadata
		rotations := getHEICRotations(data)
		img = applyHEICRotations(img, rotations)
	} else {
		// JPEG and other formats use EXIF orientation
		orientation := getEXIFOrientation(data)
		img = applyEXIFOrientation(img, orientation)
	}

	// Resize to target size (maintains aspect ratio)
	thumbnail := imaging.Fit(img, thumbnailTargetSizePx, thumbnailTargetSizePx, imaging.Lanczos)

	// Encode as JPEG with reasonable quality
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, thumbnail, imaging.JPEG, imaging.JPEGQuality(thumbnailJPEGQuality)); err != nil {
		return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	return buf.Bytes(), nil
}

// StoreMedia stores media data in bucket storage and creates a database record.
// It returns the media ID on success.
// The sourceStockImageID parameter links this media to a StockImage record if it was created
// from stock imagery. Pass empty string if not applicable.
//
// stripMetadata opts into image sanitization: when true (and the upload is a
// non-stock image) the bytes are run through SanitizeImageForStorage, which
// strips EXIF/metadata and transcodes non-web-renderable formats. The default
// is false — we retain EXIF metadata, since the trust level in every sharing
// flow makes location/device leakage an accepted risk (#1953). See #2210 for
// the web-compatibility transcode pipeline that will supersede the opt-in path.
func StoreMedia(
	ctx context.Context,
	sqlStorage *ProtoSQLStorage,
	bucketStorage BucketStorage,
	userID string,
	data []byte,
	contentType string,
	filename string,
	description string,
	sourceStockImageID string,
	stripMetadata bool,
) (string, error) {
	// Reject spoofed, disallowed, or oversized payloads before touching the DB
	// or bucket. Returns sentinel errors (ErrUnsupportedMediaType / ErrMediaTooLarge
	// / ErrMediaContentMismatch) that RPC handlers map to CodeInvalidArgument.
	if err := ValidateMediaBytes(data, contentType); err != nil {
		return "", err
	}

	// Process images before storage. Two paths, mutually exclusive:
	//   - Opt-in sanitization (stripMetadata, non-stock): strip EXIF/metadata AND
	//     transcode (re-encodes every format). Default off — see the stripMetadata
	//     doc above; we otherwise retain metadata.
	//   - Web-compatibility transcode (always, including stock — a no-op for the
	//     JPEG providers return): web-native formats pass through unchanged
	//     (metadata retained); HEIC/TIFF are transcoded to JPEG so the web client
	//     can render every upload (#2210).
	// Both fail closed — on any processing failure we reject rather than store an
	// unrenderable or unsanitized original.
	if isImageContentType(contentType) {
		var (
			processed   []byte
			processedCT string
			processErr  error
		)
		if stripMetadata && sourceStockImageID == "" {
			processed, processedCT, processErr = SanitizeImageForStorage(data, contentType)
		} else {
			processed, processedCT, processErr = TranscodeForWebIfNeeded(data, contentType)
		}
		if processErr != nil {
			return "", fmt.Errorf("failed to process image for storage: %w", processErr)
		}
		data = processed
		contentType = processedCT
	}

	// Create media proto with metadata
	media := &models.Media{
		UserId:             userID,
		ContentType:        contentType,
		Filename:           &filename,
		Description:        &description,
		SizeBytes:          int64(len(data)),
		SourceStockImageId: &sourceStockImageID,
		CreatedAtUnixSec:   time.Now().Unix(),
	}

	// Insert into SQL to generate ID
	mediaID, err := sqlStorage.Insert(ctx, media)
	if err != nil {
		return "", fmt.Errorf("failed to insert media metadata: %w", err)
	}

	// Upload to bucket storage using generated ID
	bucketKey := MediaBucketKey(userID, mediaID)
	metadata := map[string]string{
		"filename":    filename,
		"description": description,
	}

	logger := logging.LoggerWithContext(ctx).With("media_id", mediaID, "bucket_key", bucketKey)

	storageURL, err := bucketStorage.Put(ctx, bucketKey, data, contentType, metadata)
	if err != nil {
		// Try to clean up SQL record (best-effort)
		if deleteErr := sqlStorage.Delete(ctx, media); deleteErr != nil {
			logger.WarnContext(ctx, "failed to clean up media record after bucket upload failure", "error", deleteErr)
		}
		return "", fmt.Errorf("failed to upload media to bucket: %w", err)
	}

	// Update media proto with storage URL
	media.StorageUrl = storageURL

	// Generate and store thumbnail if appropriate
	if shouldGenerateThumbnail(data, contentType) {
		if isVideoContentType(contentType) {
			// Video thumbnail is best-effort: ffmpeg may not be available or video may be invalid.
			// Write the upload bytes to a temp file so ffmpeg reads from disk, dropping the heap
			// reference before subprocess invocation. The outer caller still holds the original
			// slice, but we stop extending its lifetime here.
			thumbErr := func() error {
				videoTmpDir, err := os.MkdirTemp("", "ripls-video-store-*")
				if err != nil {
					return fmt.Errorf("create temp dir: %w", err)
				}
				defer func() {
					if removeErr := os.RemoveAll(videoTmpDir); removeErr != nil {
						logger.WarnContext(ctx, "failed to remove video temp dir",
							"path", videoTmpDir, "error", removeErr)
					}
				}()

				// G703 traces taint from the uploaded bytes, but neither path
				// component comes from them: videoTmpDir is from os.MkdirTemp and
				// the filename is a literal.
				inputPath := filepath.Join(videoTmpDir, "input.mp4")
				if err := os.WriteFile(inputPath, data, 0o600); err != nil { //nolint:gosec // G703: both path components are internal (os.MkdirTemp + a literal filename).
					return fmt.Errorf("write video to temp file: %w", err)
				}
				data = nil // Stop extending the upload buffer lifetime past this point.

				thumbnailPath, err := generateVideoThumbnail(ctx, inputPath)
				if err != nil {
					return err
				}
				defer func() {
					thumbTmpDir := filepath.Dir(thumbnailPath)
					if removeErr := os.RemoveAll(thumbTmpDir); removeErr != nil {
						logger.WarnContext(ctx, "failed to remove thumbnail temp dir",
							"path", thumbTmpDir, "error", removeErr)
					}
				}()

				thumbnailKey := ThumbnailBucketKey(userID, mediaID)
				thumbnailMetadata := map[string]string{"original_media_id": mediaID}
				thumbnailStorageURL, err := bucketStorage.PutFromFile(ctx, thumbnailKey, thumbnailPath, "image/jpeg", thumbnailMetadata)
				if err != nil {
					return fmt.Errorf("upload thumbnail: %w", err)
				}
				media.ThumbnailStorageUrl = &thumbnailStorageURL
				return nil
			}()

			if thumbErr != nil {
				logger.WarnContext(ctx, "skipping video thumbnail generation", "error", thumbErr)
			}
		} else {
			thumbnailData, thumbErr := generateThumbnail(data)
			if thumbErr != nil {
				logger.ErrorContext(ctx, "failed to generate thumbnail", "error", thumbErr)
				return "", fmt.Errorf("failed to generate thumbnail: %w", thumbErr)
			}

			if len(thumbnailData) > 0 {
				thumbnailKey := ThumbnailBucketKey(userID, mediaID)
				thumbnailMetadata := map[string]string{"original_media_id": mediaID}
				thumbnailStorageURL, err := bucketStorage.Put(ctx, thumbnailKey, thumbnailData, "image/jpeg", thumbnailMetadata)
				if err != nil {
					logger.ErrorContext(ctx, "failed to upload thumbnail to bucket", "thumbnail_key", thumbnailKey, "error", err)
					return "", fmt.Errorf("failed to upload thumbnail: %w", err)
				}
				media.ThumbnailStorageUrl = &thumbnailStorageURL
			}
		}
	}

	// Update SQL record with storage URL (and thumbnail URL if generated)
	if err := sqlStorage.Update(ctx, media); err != nil {
		logger.ErrorContext(ctx, "failed to update media with storage URL", "error", err)
		return "", fmt.Errorf("failed to update media record: %w", err)
	}

	return mediaID, nil
}

// CopyStockImageForUser creates a copy of a stock image for a specific user and purpose.
// This follows the copy-on-use pattern: the copy has source_stock_image_id set for
// provenance tracking and enables cascade deletion when the parent object is deleted.
// The object is duplicated via a server-side bucket copy; no image bytes transit through
// the caller's heap.
//
// Parameters:
//   - stockImage: The StockImage record from the stock imagery provider
//   - userID: The user who will own the copied media
//   - filename: Filename for the copied media (e.g., "request-123.jpg")
//   - description: Description for the copied media (e.g., "Stock image for gear: tent")
//
// Returns the media ID of the newly created copy.
func CopyStockImageForUser(
	ctx context.Context,
	sqlStorage *ProtoSQLStorage,
	bucketStorage BucketStorage,
	stockImage *models.StockImage,
	userID string,
	filename string,
	description string,
) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"stock_image_id", stockImage.Id,
		"canonical_media_id", stockImage.MediaId,
		"user_id", userID,
	)

	// Fetch the canonical stock media record for content type and bucket key.
	stockMedia := &models.Media{}
	if err := sqlStorage.GetByID(ctx, stockImage.MediaId, stockMedia); err != nil {
		logger.ErrorContext(ctx, "failed to get stock media record", "error", err)
		return "", fmt.Errorf("failed to get stock media: %w", err)
	}

	// Insert a new media record to obtain a stable ID before the bucket copy.
	sourceStockImageID := stockImage.Id
	media := &models.Media{
		UserId:             userID,
		ContentType:        stockMedia.ContentType,
		Filename:           &filename,
		Description:        &description,
		SizeBytes:          0, // updated below once the copy reports its size
		SourceStockImageId: &sourceStockImageID,
		CreatedAtUnixSec:   time.Now().Unix(),
	}
	mediaID, err := sqlStorage.Insert(ctx, media)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert media record", "error", err)
		return "", fmt.Errorf("failed to insert media record: %w", err)
	}

	// Server-side copy: the storage backend duplicates the object internally.
	// For GCS this is a metadata-only RPC; no bytes transit through Cloud Run.
	srcKey := MediaBucketKey(stockMedia.UserId, stockImage.MediaId)
	dstKey := MediaBucketKey(userID, mediaID)
	storageURL, size, err := bucketStorage.Copy(ctx, srcKey, dstKey)
	if err != nil {
		logger.ErrorContext(ctx, "failed to copy stock image in bucket", "src_key", srcKey, "dst_key", dstKey, "error", err)
		if deleteErr := sqlStorage.Delete(ctx, media); deleteErr != nil {
			logger.WarnContext(ctx, "failed to clean up media record after copy failure", "error", deleteErr)
		}
		return "", fmt.Errorf("failed to copy stock image: %w", err)
	}

	// Update the record with the storage URL and actual byte size from the copy.
	media.StorageUrl = storageURL
	media.SizeBytes = size
	if err := sqlStorage.Update(ctx, media); err != nil {
		if errors.Is(err, ErrRecordNotFound) {
			// Deleted during the copy, e.g. with its owner or community.
			// Remove the object the copy just made, which nothing references.
			logger.InfoContext(ctx, "media record deleted during stock image copy; removing the copied object",
				"copied_media_id", mediaID)
			if deleteErr := bucketStorage.Delete(ctx, dstKey); deleteErr != nil {
				logger.WarnContext(ctx, "failed to remove copied object after its media record was deleted",
					"dst_key", dstKey, "error", deleteErr)
			}
			return "", fmt.Errorf("%w: %w", ErrMediaDeletedDuringCopy, err)
		}
		logger.ErrorContext(ctx, "failed to update media record after copy", "error", err)
		return "", fmt.Errorf("failed to update media record: %w", err)
	}

	// Copy thumbnail if the source stock media has one (best-effort).
	if stockMedia.ThumbnailStorageUrl != nil && *stockMedia.ThumbnailStorageUrl != "" {
		if copyErr := copyStockThumbnail(ctx, sqlStorage, bucketStorage, stockMedia, userID, mediaID); copyErr != nil {
			logger.WarnContext(ctx, "failed to copy stock thumbnail to user copy",
				"error", copyErr, "copied_media_id", mediaID)
		}
	}

	logger.InfoContext(ctx, "successfully created media copy from stock image",
		"copied_media_id", mediaID,
	)

	return mediaID, nil
}

// copyStockThumbnail copies a stock media poster thumbnail to the user's media copy.
// It is a no-op if the copied media already has a thumbnail URL set.
// Called only when the original stock media has a thumbnail URL.
func copyStockThumbnail(
	ctx context.Context,
	sqlStorage *ProtoSQLStorage,
	bucketStorage BucketStorage,
	stockMedia *models.Media,
	userID string,
	mediaID string,
) error {
	// Skip if the copied media already has a thumbnail (e.g., set by a prior attempt).
	copiedMedia := &models.Media{}
	if err := sqlStorage.GetByID(ctx, mediaID, copiedMedia); err != nil {
		return fmt.Errorf("get copied media record: %w", err)
	}
	if copiedMedia.ThumbnailStorageUrl != nil && *copiedMedia.ThumbnailStorageUrl != "" {
		return nil
	}

	// Server-side copy thumbnail; no bytes transit through the caller.
	srcThumbKey := ThumbnailBucketKey(stockMedia.UserId, stockMedia.Id)
	dstThumbKey := ThumbnailBucketKey(userID, mediaID)
	thumbURL, _, err := bucketStorage.Copy(ctx, srcThumbKey, dstThumbKey)
	if err != nil {
		return fmt.Errorf("copy thumbnail (src=%s, dst=%s): %w", srcThumbKey, dstThumbKey, err)
	}

	// Persist the thumbnail URL on the copied media record.
	copiedMedia.ThumbnailStorageUrl = &thumbURL
	if err := sqlStorage.Update(ctx, copiedMedia); err != nil {
		return fmt.Errorf("update copied media thumbnail URL: %w", err)
	}

	return nil
}
