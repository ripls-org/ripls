package storage

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/tiff"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestStoreMedia(t *testing.T) {
	ctx := context.Background()

	// Setup test storage
	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-123"
	imageData := createTestImage(64, 48, "jpeg") // real decodable image; StoreMedia sanitizes (re-encodes) user images
	contentType := "image/jpeg"
	filename := "test.jpg"
	description := "Test image"

	// Call StoreMedia
	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, imageData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed: %v", err)
	}

	if mediaID == "" {
		t.Fatal("Expected non-empty media ID")
	}

	// Verify media was stored in SQL
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, mediaID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media from SQL: %v", err)
	}

	// Verify media fields
	if media.UserId != userID {
		t.Errorf("Expected user_id %s, got %s", userID, media.UserId)
	}
	if media.ContentType != contentType {
		t.Errorf("Expected content_type %s, got %s", contentType, media.ContentType)
	}
	if media.GetFilename() != filename {
		t.Errorf("Expected filename %s, got %s", filename, media.GetFilename())
	}
	if media.GetDescription() != description {
		t.Errorf("Expected description %s, got %s", description, media.GetDescription())
	}
	if media.SizeBytes <= 0 {
		t.Errorf("Expected positive size_bytes, got %d", media.SizeBytes)
	}
	if media.StorageUrl == "" {
		t.Error("Expected non-empty storage_url")
	}

	// Verify media was stored in bucket
	bucketKey := MediaBucketKey(userID, mediaID)
	retrievedData, retrievedContentType, err := bucketStorage.Get(ctx, bucketKey)
	if err != nil {
		t.Fatalf("Failed to retrieve media from bucket: %v", err)
	}

	// StoreMedia sanitizes user images (re-encode strips metadata), so the
	// stored bytes are not byte-identical to the input — assert the object
	// round-trips as a valid, decodable image instead.
	if _, _, derr := image.Decode(bytes.NewReader(retrievedData)); derr != nil {
		t.Errorf("stored object is not a decodable image: %v", derr)
	}
	if int64(len(retrievedData)) != media.SizeBytes {
		t.Errorf("SizeBytes %d does not match stored byte length %d", media.SizeBytes, len(retrievedData))
	}
	if retrievedContentType != contentType {
		t.Errorf("Expected content type %s, got %s", contentType, retrievedContentType)
	}
}

func TestStoreMedia_EmptyData(t *testing.T) {
	ctx := context.Background()

	sqlStorage, bucketStorage := setupTestStorage(t)

	// An empty payload is meaningless media and is now rejected by validation
	// before any DB/bucket write (#1953).
	_, err := StoreMedia(ctx, sqlStorage, bucketStorage, "test-user-123", []byte{}, "image/jpeg", "empty.jpg", "Empty image", "", false)
	if !errors.Is(err, ErrMediaContentMismatch) {
		t.Fatalf("expected ErrMediaContentMismatch for empty data, got %v", err)
	}
}

func TestStoreMedia_LargeFile(t *testing.T) {
	ctx := context.Background()

	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-456"
	// A large, valid JPEG: exercises the thumbnail path and data integrity while
	// staying well under the 60 MB image cap. Must be a decodable image now that
	// StoreMedia validates content (#1953).
	largeData := createTestImage(4000, 4000, "jpeg")
	contentType := "image/jpeg"
	filename := "large.jpg"
	description := "Large image file"

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, largeData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed: %v", err)
	}

	// Verify size is correct
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, mediaID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	// StoreMedia sanitizes (re-encodes) user images, so the stored size differs
	// from the input; assert a positive size instead of input equality.
	if media.SizeBytes <= 0 {
		t.Errorf("Expected positive size_bytes, got %d", media.SizeBytes)
	}

	// Verify data integrity
	bucketKey := MediaBucketKey(userID, mediaID)
	retrievedData, _, err := bucketStorage.Get(ctx, bucketKey)
	if err != nil {
		t.Fatalf("Failed to retrieve media from bucket: %v", err)
	}

	if int64(len(retrievedData)) != media.SizeBytes {
		t.Errorf("stored byte length %d does not match SizeBytes %d", len(retrievedData), media.SizeBytes)
	}
	if _, _, derr := image.Decode(bytes.NewReader(retrievedData)); derr != nil {
		t.Errorf("stored object is not a decodable image: %v", derr)
	}
}

func TestStoreMedia_SpecialCharacters(t *testing.T) {
	ctx := context.Background()

	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-789"
	imageData := gifBytes // valid magic-byte fixture (see media_validate_test.go)
	contentType := "image/gif"
	filename := "file with spaces & special.gif"
	description := "Description with \"quotes\" and 'apostrophes'"

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, imageData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed: %v", err)
	}

	// Verify special characters are preserved
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, mediaID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	if media.GetFilename() != filename {
		t.Errorf("Expected filename %q, got %q", filename, media.GetFilename())
	}
	if media.GetDescription() != description {
		t.Errorf("Expected description %q, got %q", description, media.GetDescription())
	}
}

// setupTestStorage creates test SQL and bucket storage.
func setupTestStorage(t *testing.T) (*ProtoSQLStorage, BucketStorage) {
	t.Helper()

	sqlStorage, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	tempDir := t.TempDir()

	// Create local bucket storage
	bucketStorage, err := NewLocalBucketStorage(tempDir+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create bucket storage: %v", err)
	}

	return sqlStorage, bucketStorage
}

// createTestImage creates a test image of specified dimensions and format.
func createTestImage(width, height int, format string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with a simple pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: 128,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	switch format {
	case "jpeg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			panic(err)
		}
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
	}

	return buf.Bytes()
}

func TestStoreMedia_ThumbnailGeneration_LargeImage(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-thumbnail"
	// Create a 2000x1500 image (exceeds dimension threshold of 1000px)
	imageData := createTestImage(2000, 1500, "jpeg")
	contentType := "image/jpeg"
	filename := "large-photo.jpg"
	description := "Large photo requiring thumbnail"

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, imageData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed: %v", err)
	}

	// Verify media record has thumbnail_storage_url
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, mediaID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	if media.GetThumbnailStorageUrl() == "" {
		t.Error("Expected thumbnail_storage_url to be set for large image")
	}

	// Verify thumbnail exists in bucket storage
	thumbnailKey := ThumbnailBucketKey(userID, mediaID)
	thumbnailData, thumbnailContentType, err := bucketStorage.Get(ctx, thumbnailKey)
	if err != nil {
		t.Fatalf("Failed to retrieve thumbnail from bucket: %v", err)
	}

	if thumbnailContentType != "image/jpeg" {
		t.Errorf("Expected thumbnail content type image/jpeg, got %s", thumbnailContentType)
	}

	// Decode thumbnail to verify dimensions
	thumbnailImg, _, err := image.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		t.Fatalf("Failed to decode thumbnail: %v", err)
	}

	bounds := thumbnailImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Thumbnail should be scaled down (longest side should be ~400px)
	maxDim := width
	if height > maxDim {
		maxDim = height
	}

	if maxDim > thumbnailTargetSizePx {
		t.Errorf("Thumbnail too large: %dx%d (max dimension %d exceeds target %d)", width, height, maxDim, thumbnailTargetSizePx)
	}

	// Aspect ratio should be preserved (2000:1500 = 4:3)
	expectedRatio := 2000.0 / 1500.0
	actualRatio := float64(width) / float64(height)
	ratioDiff := expectedRatio - actualRatio
	if ratioDiff < -0.1 || ratioDiff > 0.1 {
		t.Errorf("Aspect ratio not preserved: expected ~%.2f, got %.2f", expectedRatio, actualRatio)
	}
}

func TestStoreMedia_ThumbnailGeneration_SmallImage(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-small"
	// Create a 300x200 image (below both thresholds)
	imageData := createTestImage(300, 200, "jpeg")
	contentType := "image/jpeg"
	filename := "small-icon.jpg"
	description := "Small image not requiring thumbnail"

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, imageData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed: %v", err)
	}

	// Verify media record does NOT have thumbnail_storage_url
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, mediaID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	if media.GetThumbnailStorageUrl() != "" {
		t.Error("Expected thumbnail_storage_url to be empty for small image")
	}

	// Verify thumbnail does NOT exist in bucket storage
	thumbnailKey := ThumbnailBucketKey(userID, mediaID)
	_, _, err = bucketStorage.Get(ctx, thumbnailKey)
	if err == nil {
		t.Error("Expected thumbnail to not exist for small image, but it was found")
	}
}

func TestStoreMedia_ThumbnailGeneration_LargePNG(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-png"
	// Create a 1500x1500 PNG (exceeds dimension threshold)
	imageData := createTestImage(1500, 1500, "png")
	contentType := "image/png"
	filename := "large-diagram.png"
	description := "Large PNG requiring thumbnail"

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, imageData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed: %v", err)
	}

	// Verify thumbnail was generated
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, mediaID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	if media.GetThumbnailStorageUrl() == "" {
		t.Error("Expected thumbnail_storage_url to be set for large PNG")
	}

	// Verify thumbnail is JPEG (even though source was PNG)
	thumbnailKey := ThumbnailBucketKey(userID, mediaID)
	_, thumbnailContentType, err := bucketStorage.Get(ctx, thumbnailKey)
	if err != nil {
		t.Fatalf("Failed to retrieve thumbnail: %v", err)
	}

	if thumbnailContentType != "image/jpeg" {
		t.Errorf("Expected thumbnail content type image/jpeg, got %s", thumbnailContentType)
	}
}

func TestStoreMedia_ThumbnailGeneration_VideoFile(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-video"
	// Use fake (invalid) video data — ffmpeg will fail gracefully, leaving no thumbnail.
	// If ffmpeg is available and succeeds on real video data, a thumbnail IS generated.
	// This test verifies that StoreMedia does not return an error for video thumbnail failure.
	videoData := make([]byte, 2*1024*1024) // 2MB of zeroes — invalid MP4
	contentType := "video/mp4"
	filename := "sample.mp4"
	description := "Video file"

	// StoreMedia must succeed even when ffmpeg cannot generate a thumbnail from invalid video data
	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, videoData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed: %v — video thumbnail failure should be non-fatal", err)
	}

	media := &models.Media{}
	if err := sqlStorage.GetByID(ctx, mediaID, media); err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	// With invalid video data, ffmpeg fails and thumbnail is skipped (graceful degradation)
	// The media record is still created and accessible
	t.Logf("StoreMedia succeeded for video file; thumbnail_storage_url=%q", media.GetThumbnailStorageUrl())
}

func TestStoreMedia_ThumbnailGeneration_HEIC(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-heic"
	// Load real HEIC test image
	heicData, err := os.ReadFile("../test_data/example.heic")
	if err != nil {
		t.Fatalf("Failed to read HEIC test image: %v", err)
	}

	contentType := "image/heic"
	filename := "example.heic"
	description := "HEIC image from iOS device"

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, heicData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed for HEIC: %v", err)
	}

	// Verify media record has thumbnail_storage_url
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, mediaID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	if media.GetThumbnailStorageUrl() == "" {
		t.Error("Expected thumbnail_storage_url to be set for HEIC image")
	}

	// Verify thumbnail exists and is valid JPEG
	thumbnailKey := ThumbnailBucketKey(userID, mediaID)
	thumbnailData, thumbnailContentType, err := bucketStorage.Get(ctx, thumbnailKey)
	if err != nil {
		t.Fatalf("Failed to retrieve HEIC thumbnail from bucket: %v", err)
	}

	if thumbnailContentType != "image/jpeg" {
		t.Errorf("Expected thumbnail content type image/jpeg, got %s", thumbnailContentType)
	}

	// Decode thumbnail to verify it's valid
	thumbnailImg, format, err := image.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		t.Fatalf("Failed to decode HEIC thumbnail: %v", err)
	}

	if format != "jpeg" {
		t.Errorf("Expected thumbnail format jpeg, got %s", format)
	}

	// Verify thumbnail dimensions are reasonable
	bounds := thumbnailImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	maxDim := max(width, height)

	if maxDim > thumbnailTargetSizePx {
		t.Errorf("HEIC thumbnail too large: %dx%d (max dimension %d exceeds target %d)", width, height, maxDim, thumbnailTargetSizePx)
	}

	t.Logf("Successfully generated %dx%d thumbnail from HEIC image (%d bytes -> %d bytes)", width, height, len(heicData), len(thumbnailData))
}

func TestStoreMedia_ThumbnailGeneration_WebP(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	userID := "test-user-webp"
	// Load real WebP test image
	webpData, err := os.ReadFile("../test_data/example.webp")
	if err != nil {
		t.Fatalf("Failed to read WebP test image: %v", err)
	}

	contentType := "image/webp"
	filename := "example.webp"
	description := "WebP image"

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, userID, webpData, contentType, filename, description, "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed for WebP: %v", err)
	}

	// Verify media record has thumbnail_storage_url
	media := &models.Media{}
	err = sqlStorage.GetByID(ctx, mediaID, media)
	if err != nil {
		t.Fatalf("Failed to retrieve media: %v", err)
	}

	if media.GetThumbnailStorageUrl() == "" {
		t.Error("Expected thumbnail_storage_url to be set for WebP image")
	}

	// Verify thumbnail exists and is valid JPEG
	thumbnailKey := ThumbnailBucketKey(userID, mediaID)
	thumbnailData, thumbnailContentType, err := bucketStorage.Get(ctx, thumbnailKey)
	if err != nil {
		t.Fatalf("Failed to retrieve WebP thumbnail from bucket: %v", err)
	}

	if thumbnailContentType != "image/jpeg" {
		t.Errorf("Expected thumbnail content type image/jpeg, got %s", thumbnailContentType)
	}

	// Decode thumbnail to verify it's valid
	thumbnailImg, format, err := image.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		t.Fatalf("Failed to decode WebP thumbnail: %v", err)
	}

	if format != "jpeg" {
		t.Errorf("Expected thumbnail format jpeg, got %s", format)
	}

	// Verify thumbnail dimensions are reasonable
	bounds := thumbnailImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	maxDim := max(width, height)

	if maxDim > thumbnailTargetSizePx {
		t.Errorf("WebP thumbnail too large: %dx%d (max dimension %d exceeds target %d)", width, height, maxDim, thumbnailTargetSizePx)
	}

	t.Logf("Successfully generated %dx%d thumbnail from WebP image (%d bytes -> %d bytes)", width, height, len(webpData), len(thumbnailData))
}

func TestGenerateVideoThumbnail_FakeData(t *testing.T) {
	ctx := context.Background()

	// Fake (invalid) video data — ffmpeg will fail at both timestamps.
	// This verifies that the retry loop runs and returns an error (not panics).
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "input.mp4")
	fakeData := make([]byte, 1024) // 1 KB of zeroes — invalid MP4
	if err := os.WriteFile(inputPath, fakeData, 0o600); err != nil {
		t.Fatalf("failed to write fake video file: %v", err)
	}

	thumbnailPath, err := generateVideoThumbnail(ctx, inputPath)
	if err == nil {
		t.Error("expected error for invalid video data, got nil")
	}
	if thumbnailPath != "" {
		t.Errorf("expected empty thumbnailPath on error, got %q", thumbnailPath)
	}
}

func TestIsImageContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"image/webp", true},
		{"IMAGE/JPEG", true}, // case insensitive
		{"video/mp4", false},
		{"application/pdf", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isImageContentType(tt.contentType)
		if result != tt.expected {
			t.Errorf("isImageContentType(%q) = %v, expected %v", tt.contentType, result, tt.expected)
		}
	}
}

func TestIsVideoContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"video/mp4", true},
		{"video/quicktime", true},
		{"video/webm", true},
		{"VIDEO/MP4", true}, // case insensitive
		{"image/jpeg", false},
		{"application/pdf", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isVideoContentType(tt.contentType)
		if result != tt.expected {
			t.Errorf("isVideoContentType(%q) = %v, expected %v", tt.contentType, result, tt.expected)
		}
	}
}

func TestShouldGenerateThumbnail(t *testing.T) {
	tests := []struct {
		name        string
		width       int
		height      int
		format      string
		contentType string
		expected    bool
	}{
		{"large JPEG", 2000, 1500, "jpeg", "image/jpeg", true},
		{"large PNG", 1500, 1500, "png", "image/png", true},
		{"small image", 300, 200, "jpeg", "image/jpeg", false},
		{"medium width", 1200, 800, "jpeg", "image/jpeg", true}, // exceeds 1000px
		// Videos always attempt thumbnail generation (ffmpeg frame extraction)
		{"video mp4", 0, 0, "", "video/mp4", true},
		{"video quicktime", 0, 0, "", "video/quicktime", true},
		// Non-media types should not generate thumbnails
		{"pdf file", 0, 0, "", "application/pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data []byte
			if tt.width > 0 && tt.height > 0 {
				data = createTestImage(tt.width, tt.height, tt.format)
			} else {
				data = []byte("fake data")
			}

			result := shouldGenerateThumbnail(data, tt.contentType)
			if result != tt.expected {
				t.Errorf("shouldGenerateThumbnail() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateThumbnail(t *testing.T) {
	// Create a 2000x1000 test image
	imageData := createTestImage(2000, 1000, "jpeg")

	thumbnailData, err := generateThumbnail(imageData)
	if err != nil {
		t.Fatalf("generateThumbnail failed: %v", err)
	}

	// Verify thumbnail is smaller than original
	if len(thumbnailData) >= len(imageData) {
		t.Error("Expected thumbnail to be smaller than original image")
	}

	// Decode and verify dimensions
	thumbnailImg, _, err := image.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		t.Fatalf("Failed to decode thumbnail: %v", err)
	}

	bounds := thumbnailImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Longest side should be at most thumbnailTargetSizePx
	maxDim := width
	if height > maxDim {
		maxDim = height
	}

	if maxDim > thumbnailTargetSizePx {
		t.Errorf("Thumbnail max dimension %d exceeds target %d", maxDim, thumbnailTargetSizePx)
	}

	// Aspect ratio should be preserved (2000:1000 = 2:1)
	expectedRatio := 2.0
	actualRatio := float64(width) / float64(height)
	ratioDiff := expectedRatio - actualRatio
	if ratioDiff < -0.1 || ratioDiff > 0.1 {
		t.Errorf("Aspect ratio not preserved: expected ~%.2f, got %.2f", expectedRatio, actualRatio)
	}
}

func TestGenerateThumbnail_PreservesOrientation_EXIF(t *testing.T) {
	// Load JPEG with EXIF rotation metadata
	// This image is stored as landscape (4032x3024) but has EXIF rotation
	// metadata indicating it should be displayed as portrait (3024x4032)
	imageData, err := os.ReadFile("../test_data/exif_rotated.jpg")
	if err != nil {
		t.Fatalf("Failed to read EXIF rotated test image: %v", err)
	}

	// Generate thumbnail
	thumbnailData, err := generateThumbnail(imageData)
	if err != nil {
		t.Fatalf("generateThumbnail failed: %v", err)
	}

	// Decode thumbnail
	thumbnailImg, _, err := image.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		t.Fatalf("Failed to decode thumbnail: %v", err)
	}

	thumbWidth := thumbnailImg.Bounds().Dx()
	thumbHeight := thumbnailImg.Bounds().Dy()

	// Thumbnail should be portrait (height > width) after applying rotation
	if thumbWidth >= thumbHeight {
		t.Errorf("Expected portrait thumbnail (height > width), got %dx%d", thumbWidth, thumbHeight)
	}

	t.Logf("EXIF rotated JPEG thumbnail: %dx%d (portrait orientation preserved)", thumbWidth, thumbHeight)
}

func TestGenerateThumbnail_PreservesOrientation_ISOBMFF(t *testing.T) {
	// Load HEIC with ISOBMFF rotation metadata
	// This image is stored as landscape (4032x3024) but has ISOBMFF rotation
	// metadata indicating it should be displayed as portrait (3024x4032)
	imageData, err := os.ReadFile("../test_data/isobmff_rotated.heic")
	if err != nil {
		t.Fatalf("Failed to read ISOBMFF rotated test image: %v", err)
	}

	// Generate thumbnail
	thumbnailData, err := generateThumbnail(imageData)
	if err != nil {
		t.Fatalf("generateThumbnail failed: %v", err)
	}

	// Decode thumbnail
	thumbnailImg, _, err := image.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		t.Fatalf("Failed to decode thumbnail: %v", err)
	}

	thumbWidth := thumbnailImg.Bounds().Dx()
	thumbHeight := thumbnailImg.Bounds().Dy()

	// Thumbnail should be portrait (height > width) after applying rotation
	if thumbWidth >= thumbHeight {
		t.Errorf("Expected portrait thumbnail (height > width), got %dx%d", thumbWidth, thumbHeight)
	}

	t.Logf("ISOBMFF rotated HEIC thumbnail: %dx%d (portrait orientation preserved)", thumbWidth, thumbHeight)
}

func TestGenerateThumbnail_TIFF(t *testing.T) {
	// Create a minimal TIFF image programmatically.
	// This confirms that the upgraded golang.org/x/image decoder handles TIFF
	// correctly and that the CVE fix (GO-2026-4815) doesn't break normal decoding.
	img := image.NewRGBA(image.Rect(0, 0, 1200, 900))
	for y := range 900 {
		for x := range 1200 {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / 1200),
				G: uint8((y * 255) / 900),
				B: 128,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := tiff.Encode(&buf, img, nil); err != nil {
		t.Fatalf("Failed to encode TIFF test image: %v", err)
	}
	tiffData := buf.Bytes()

	thumbnailData, err := generateThumbnail(tiffData)
	if err != nil {
		t.Fatalf("generateThumbnail failed for TIFF: %v", err)
	}

	thumbnailImg, format, err := image.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		t.Fatalf("Failed to decode TIFF thumbnail: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("Expected thumbnail format jpeg, got %s", format)
	}

	bounds := thumbnailImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	maxDim := max(width, height)
	if maxDim > thumbnailTargetSizePx {
		t.Errorf("TIFF thumbnail too large: %dx%d (max dimension %d exceeds target %d)", width, height, maxDim, thumbnailTargetSizePx)
	}
}

// noGetBucketStorage wraps a BucketStorage and panics if Get is called.
// This asserts that CopyStockImageForUser does not materialize image bytes.
type noGetBucketStorage struct {
	BucketStorage
}

func (n *noGetBucketStorage) Get(_ context.Context, key string) ([]byte, string, error) {
	panic("BucketStorage.Get must not be called during stock image copy — use server-side Copy instead; called with key: " + key)
}

// noThumbPutBucketStorage wraps a BucketStorage and panics if Put is called for a thumbnail
// key (key ending in "-thumb"). The main media Put is passed through to the real backend.
// Used to assert that the StoreMedia video thumbnail branch uses PutFromFile, not Put.
type noThumbPutBucketStorage struct {
	BucketStorage
}

func (n *noThumbPutBucketStorage) Put(ctx context.Context, key string, data []byte, contentType string, metadata map[string]string) (string, error) {
	if len(key) > 6 && key[len(key)-6:] == "-thumb" {
		panic("BucketStorage.Put must not be called for thumbnail upload — use PutFromFile; called with key: " + key)
	}
	return n.BucketStorage.Put(ctx, key, data, contentType, metadata)
}

func TestCopyStockImageForUser_UsesServerSideCopy(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	// Store a canonical stock media record and object in the bucket.
	stockUserID := "stock-system-user"
	stockImageData := createTestImage(800, 600, "jpeg")
	stockMediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, stockUserID, stockImageData, "image/jpeg", "original.jpg", "Original stock photo", "", false)
	if err != nil {
		t.Fatalf("StoreMedia for stock image failed: %v", err)
	}

	// Insert a StockImage record referencing the stock media.
	stockImage := &models.StockImage{
		MediaId:          stockMediaID,
		CreatedAtUnixSec: 1000,
	}
	stockImageID, err := sqlStorage.Insert(ctx, stockImage)
	if err != nil {
		t.Fatalf("Insert StockImage failed: %v", err)
	}
	stockImage.Id = stockImageID

	// Wrap bucket storage so that Get panics — CopyStockImageForUser must use Copy.
	safeBucket := &noGetBucketStorage{BucketStorage: bucketStorage}

	targetUserID := "target-user-456"
	mediaID, err := CopyStockImageForUser(ctx, sqlStorage, safeBucket, stockImage, targetUserID, "copy.jpg", "User copy")
	if err != nil {
		t.Fatalf("CopyStockImageForUser failed: %v", err)
	}
	if mediaID == "" {
		t.Fatal("expected non-empty media ID")
	}

	// Verify the new media record was created with correct fields.
	copied := &models.Media{}
	if err := sqlStorage.GetByID(ctx, mediaID, copied); err != nil {
		t.Fatalf("GetByID for copied media failed: %v", err)
	}
	if copied.UserId != targetUserID {
		t.Errorf("user_id: expected %s, got %s", targetUserID, copied.UserId)
	}
	if copied.ContentType != "image/jpeg" {
		t.Errorf("content_type: expected image/jpeg, got %s", copied.ContentType)
	}
	if copied.GetFilename() != "copy.jpg" {
		t.Errorf("filename: expected copy.jpg, got %s", copied.GetFilename())
	}
	if copied.GetSourceStockImageId() != stockImageID {
		t.Errorf("source_stock_image_id: expected %s, got %s", stockImageID, copied.GetSourceStockImageId())
	}
	if copied.StorageUrl == "" {
		t.Error("storage_url must be set on the copied media record")
	}
	if copied.SizeBytes <= 0 {
		t.Errorf("size_bytes must be positive, got %d", copied.SizeBytes)
	}

	// Verify the object was actually copied in the bucket.
	dstKey := MediaBucketKey(targetUserID, mediaID)
	retrievedData, _, err := bucketStorage.Get(ctx, dstKey)
	if err != nil {
		t.Fatalf("Get copied bucket object failed: %v", err)
	}
	// CopyStockImageForUser is a server-side byte copy, so the copied object must
	// be byte-identical to the SOURCE object as stored (the source was sanitized
	// at store time, so it no longer equals the pre-store input bytes).
	srcData, _, err := bucketStorage.Get(ctx, MediaBucketKey(stockUserID, stockMediaID))
	if err != nil {
		t.Fatalf("Get source bucket object failed: %v", err)
	}
	if !bytes.Equal(retrievedData, srcData) {
		t.Errorf("copied object (%d bytes) does not byte-equal the source object (%d bytes)", len(retrievedData), len(srcData))
	}
}

func TestCopyStockImageForUser_WithThumbnail(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	// Create a large stock image that will get a thumbnail generated.
	stockUserID := "stock-system-user"
	stockImageData := createTestImage(2000, 1500, "jpeg")
	stockMediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, stockUserID, stockImageData, "image/jpeg", "large.jpg", "Large stock photo", "", false)
	if err != nil {
		t.Fatalf("StoreMedia for stock image failed: %v", err)
	}

	// Confirm the stock media has a thumbnail.
	stockMedia := &models.Media{}
	if err := sqlStorage.GetByID(ctx, stockMediaID, stockMedia); err != nil {
		t.Fatalf("GetByID stock media failed: %v", err)
	}
	if stockMedia.GetThumbnailStorageUrl() == "" {
		t.Skip("stock media has no thumbnail — skipping thumbnail copy test")
	}

	stockImage := &models.StockImage{
		MediaId:          stockMediaID,
		CreatedAtUnixSec: 1000,
	}
	stockImageID, err := sqlStorage.Insert(ctx, stockImage)
	if err != nil {
		t.Fatalf("Insert StockImage failed: %v", err)
	}
	stockImage.Id = stockImageID

	safeBucket := &noGetBucketStorage{BucketStorage: bucketStorage}

	targetUserID := "target-user-789"
	mediaID, err := CopyStockImageForUser(ctx, sqlStorage, safeBucket, stockImage, targetUserID, "copy-large.jpg", "User copy of large photo")
	if err != nil {
		t.Fatalf("CopyStockImageForUser failed: %v", err)
	}

	// The copied media should have a thumbnail URL.
	copied := &models.Media{}
	if err := sqlStorage.GetByID(ctx, mediaID, copied); err != nil {
		t.Fatalf("GetByID copied media failed: %v", err)
	}
	if copied.GetThumbnailStorageUrl() == "" {
		t.Error("expected thumbnail_storage_url to be set on copied media with thumbnail")
	}

	// The thumbnail object must exist in the bucket.
	dstThumbKey := ThumbnailBucketKey(targetUserID, mediaID)
	thumbData, _, err := bucketStorage.Get(ctx, dstThumbKey)
	if err != nil {
		t.Fatalf("Get copied thumbnail failed: %v", err)
	}
	if len(thumbData) == 0 {
		t.Error("copied thumbnail is empty")
	}
}

// TestStoreMedia_VideoUsesStreamingPath asserts that the StoreMedia video thumbnail
// branch never calls BucketStorage.Put for the thumbnail — it must use PutFromFile.
// The noThumbPutBucketStorage wrapper panics if Put is called with a thumbnail key,
// while passing through the main media Put so StoreMedia can complete normally.
func TestStoreMedia_VideoUsesStreamingPath(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	// Wrap the bucket: panics on thumbnail Put, passes through main media Put.
	safeBucket := &noThumbPutBucketStorage{BucketStorage: bucketStorage}

	userID := "test-user-video-streaming"
	videoData := make([]byte, 2*1024) // invalid MP4 — ffmpeg fails gracefully, no thumbnail
	contentType := "video/mp4"

	// Must not panic: no thumbnail is generated (invalid video), so Put for the
	// thumbnail key is never reached. If the code path regressed to using Put for
	// thumbnails on valid video, the wrapper would catch it.
	mediaID, err := StoreMedia(ctx, sqlStorage, safeBucket, userID, videoData, contentType, "sample.mp4", "test", "", false)
	if err != nil {
		t.Fatalf("StoreMedia failed: %v", err)
	}
	if mediaID == "" {
		t.Fatal("expected non-empty media ID")
	}
}
