package storage

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/tiff"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

const gpsFixturePath = "../test_data/books_exif.jpg"

func TestSanitizeImageForStorage_StripsEXIF(t *testing.T) {
	original, err := os.ReadFile(gpsFixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Sanity: the fixture really does carry EXIF (with GPS) before sanitization.
	if _, err := exif.Decode(bytes.NewReader(original)); err != nil {
		t.Fatalf("fixture should contain EXIF before sanitization, got: %v", err)
	}

	out, outCT, err := SanitizeImageForStorage(original, "image/jpeg")
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	if outCT != "image/jpeg" {
		t.Errorf("content type: got %q, want image/jpeg", outCT)
	}
	// After re-encode there is no EXIF segment at all — GPS/device/timestamps gone.
	if _, err := exif.Decode(bytes.NewReader(out)); err == nil {
		t.Error("sanitized image still has decodable EXIF; expected none")
	}
	// And it is still a valid, decodable JPEG.
	if _, _, err := image.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("sanitized image no longer decodes: %v", err)
	}
}

func TestSanitizeImageForStorage_GIFPassthrough(t *testing.T) {
	in := []byte{'G', 'I', 'F', '8', '9', 'a', 0x10, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00}
	out, outCT, err := SanitizeImageForStorage(in, "image/gif")
	if err != nil {
		t.Fatalf("sanitize gif: %v", err)
	}
	if outCT != "image/gif" {
		t.Errorf("content type: got %q, want image/gif", outCT)
	}
	if !bytes.Equal(out, in) {
		t.Error("gif should pass through unchanged")
	}
}

func TestSanitizeImageForStorage_TranscodesNonWebFormats(t *testing.T) {
	// TIFF doesn't render in browsers; sanitization transcodes it to JPEG.
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var tiffBuf bytes.Buffer
	if err := tiff.Encode(&tiffBuf, img, nil); err != nil {
		t.Fatalf("encode tiff fixture: %v", err)
	}

	out, outCT, err := SanitizeImageForStorage(tiffBuf.Bytes(), "image/tiff")
	if err != nil {
		t.Fatalf("sanitize tiff: %v", err)
	}
	if outCT != "image/jpeg" {
		t.Errorf("tiff should transcode to image/jpeg, got %q", outCT)
	}
	if _, _, err := image.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("transcoded output does not decode: %v", err)
	}
}

func TestSanitizeImageForStorage_FailsClosed(t *testing.T) {
	// JPEG magic prefix but truncated/garbage body: must error and return no
	// bytes so the caller rejects rather than storing the original.
	bad := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
	out, _, err := SanitizeImageForStorage(bad, "image/jpeg")
	if err == nil {
		t.Fatal("expected error on undecodable image")
	}
	if out != nil {
		t.Error("expected nil bytes on failure (fail closed)")
	}
}

func TestStoreMedia_StripsImageMetadata(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	original, err := os.ReadFile(gpsFixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// stripMetadata=true opts into sanitization.
	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, "user-strip", original, "image/jpeg", "books.jpg", "", "", true)
	if err != nil {
		t.Fatalf("StoreMedia: %v", err)
	}

	stored, _, err := bucketStorage.Get(ctx, MediaBucketKey("user-strip", mediaID))
	if err != nil {
		t.Fatalf("bucket get: %v", err)
	}
	if _, err := exif.Decode(bytes.NewReader(stored)); err == nil {
		t.Error("stored object still has EXIF; StoreMedia should have stripped it")
	}
}

// TestStoreMedia_RetainsMetadataByDefault verifies the default path
// (stripMetadata=false) stores the original bytes verbatim — EXIF retained.
func TestStoreMedia_RetainsMetadataByDefault(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	original, err := os.ReadFile(gpsFixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, "user-keep", original, "image/jpeg", "books.jpg", "", "", false)
	if err != nil {
		t.Fatalf("StoreMedia: %v", err)
	}

	stored, _, err := bucketStorage.Get(ctx, MediaBucketKey("user-keep", mediaID))
	if err != nil {
		t.Fatalf("bucket get: %v", err)
	}
	if !bytes.Equal(stored, original) {
		t.Error("default path should store the original bytes unchanged")
	}
	// EXIF (incl. GPS) is retained when stripping is off.
	if _, err := exif.Decode(bytes.NewReader(stored)); err != nil {
		t.Errorf("EXIF should be retained by default, but decode failed: %v", err)
	}
}

func TestStoreMedia_StockImageNotSanitized(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	original, err := os.ReadFile(gpsFixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Even with stripMetadata=true, stock-provider images (sourceStockImageID
	// set) are exempt and stored byte-for-byte.
	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, "user-stock", original, "image/jpeg", "stock.jpg", "", "stock-123", true)
	if err != nil {
		t.Fatalf("StoreMedia: %v", err)
	}

	stored, _, err := bucketStorage.Get(ctx, MediaBucketKey("user-stock", mediaID))
	if err != nil {
		t.Fatalf("bucket get: %v", err)
	}
	if !bytes.Equal(stored, original) {
		t.Error("stock image should be stored unchanged, but bytes differ")
	}

	// Confirm provenance was recorded.
	m := &models.Media{}
	if err := sqlStorage.GetByID(ctx, mediaID, m); err != nil {
		t.Fatalf("get media: %v", err)
	}
	if m.GetSourceStockImageId() != "stock-123" {
		t.Errorf("source_stock_image_id: got %q, want stock-123", m.GetSourceStockImageId())
	}
}

func TestTranscodeForWebIfNeeded_PassesThroughWebFormats(t *testing.T) {
	cases := []struct {
		ct   string
		data []byte
	}{
		{"image/jpeg", jpegBytes},
		{"image/png", pngBytes},
		{"image/gif", gifBytes},
	}
	for _, tc := range cases {
		out, outCT, err := TranscodeForWebIfNeeded(tc.data, tc.ct)
		if err != nil {
			t.Fatalf("%s: %v", tc.ct, err)
		}
		if outCT != tc.ct {
			t.Errorf("%s: content type changed to %q (web formats should pass through)", tc.ct, outCT)
		}
		if !bytes.Equal(out, tc.data) {
			t.Errorf("%s: bytes changed; web-native formats must pass through unmodified", tc.ct)
		}
	}
}

func TestTranscodeForWebIfNeeded_TranscodesHEIC(t *testing.T) {
	heic, err := os.ReadFile("../test_data/example.heic")
	if err != nil {
		t.Fatalf("read heic fixture: %v", err)
	}
	out, outCT, err := TranscodeForWebIfNeeded(heic, "image/heic")
	if err != nil {
		t.Fatalf("transcode heic: %v", err)
	}
	if outCT != "image/jpeg" {
		t.Errorf("heic should transcode to image/jpeg, got %q", outCT)
	}
	if _, _, derr := image.Decode(bytes.NewReader(out)); derr != nil {
		t.Errorf("transcoded heic is not a decodable image: %v", derr)
	}
}

func TestTranscodeForWebIfNeeded_FailsClosed(t *testing.T) {
	// Bytes that pass the validation sniff (octet-stream) but aren't a decodable
	// HEIC must be rejected, not stored unrenderable.
	bad := []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0}
	out, _, err := TranscodeForWebIfNeeded(bad, "image/heic")
	if err == nil {
		t.Fatal("expected error for undecodable heic")
	}
	if out != nil {
		t.Error("expected nil bytes on failure (fail closed)")
	}
}

// TestStoreMedia_TranscodesHEICForWeb verifies the default path (stripMetadata
// off) still transcodes HEIC to JPEG so the web client can render it (#2210).
func TestStoreMedia_TranscodesHEICForWeb(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	heic, err := os.ReadFile("../test_data/example.heic")
	if err != nil {
		t.Fatalf("read heic fixture: %v", err)
	}

	mediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, "user-heic", heic, "image/heic", "x.heic", "", "", false)
	if err != nil {
		t.Fatalf("StoreMedia: %v", err)
	}

	m := &models.Media{}
	if err := sqlStorage.GetByID(ctx, mediaID, m); err != nil {
		t.Fatalf("get media: %v", err)
	}
	if m.ContentType != "image/jpeg" {
		t.Errorf("HEIC should be stored as image/jpeg for web compatibility, got %q", m.ContentType)
	}

	stored, _, err := bucketStorage.Get(ctx, MediaBucketKey("user-heic", mediaID))
	if err != nil {
		t.Fatalf("bucket get: %v", err)
	}
	if _, _, derr := image.Decode(bytes.NewReader(stored)); derr != nil {
		t.Errorf("stored object is not a decodable image: %v", derr)
	}
}
