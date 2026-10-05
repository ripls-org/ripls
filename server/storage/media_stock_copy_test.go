package storage

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// noGetBucketStorage wraps a BucketStorage and panics if Get is called.
// This asserts that CopyStockImageForUser does not materialize image bytes.
type noGetBucketStorage struct {
	BucketStorage
}

func (n *noGetBucketStorage) Get(_ context.Context, key string) ([]byte, string, error) {
	panic("BucketStorage.Get must not be called during stock image copy — use server-side Copy instead; called with key: " + key)
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

// deleteDuringCopyBucketStorage runs onCopy after each Copy, standing in for a
// concurrent delete that lands while the bucket copy is in flight.
type deleteDuringCopyBucketStorage struct {
	BucketStorage
	onCopy func(dstKey string)
}

func (d *deleteDuringCopyBucketStorage) Copy(ctx context.Context, srcKey, dstKey string) (string, int64, error) {
	url, size, err := d.BucketStorage.Copy(ctx, srcKey, dstKey)
	if err == nil {
		d.onCopy(dstKey)
	}
	return url, size, err
}

func TestCopyStockImageForUser_MediaDeletedDuringCopy(t *testing.T) {
	ctx := context.Background()
	sqlStorage, bucketStorage := setupTestStorage(t)

	stockMediaID, err := StoreMedia(ctx, sqlStorage, bucketStorage, "stock-system-user", createTestImage(800, 600, "jpeg"), "image/jpeg", "original.jpg", "Original stock photo", "", false)
	if err != nil {
		t.Fatalf("StoreMedia for stock image failed: %v", err)
	}
	stockImage := &models.StockImage{MediaId: stockMediaID, CreatedAtUnixSec: 1000}
	if stockImage.Id, err = sqlStorage.Insert(ctx, stockImage); err != nil {
		t.Fatalf("Insert StockImage failed: %v", err)
	}

	// Delete the new media record as the copy finishes, as deleting its owner
	// (or an e2e simulation cleanup) would.
	targetUserID := "target-user-789"
	var copiedKey string
	racing := &deleteDuringCopyBucketStorage{BucketStorage: bucketStorage, onCopy: func(dstKey string) {
		copiedKey = dstKey
		mediaID := strings.TrimPrefix(dstKey, targetUserID+"/")
		if err := sqlStorage.Delete(ctx, &models.Media{Id: mediaID}); err != nil {
			t.Fatalf("deleting media record mid-copy: %v", err)
		}
	}}

	mediaID, err := CopyStockImageForUser(ctx, sqlStorage, racing, stockImage, targetUserID, "copy.jpg", "User copy")
	if !errors.Is(err, ErrMediaDeletedDuringCopy) {
		t.Fatalf("err = %v, want ErrMediaDeletedDuringCopy", err)
	}
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("err = %v, want it to still wrap ErrRecordNotFound", err)
	}
	if mediaID != "" {
		t.Errorf("mediaID = %q, want empty", mediaID)
	}
	if copiedKey == "" {
		t.Fatal("the bucket copy never ran")
	}
	// The object the copy made is orphaned, so it must be gone.
	if _, _, err := bucketStorage.Get(ctx, copiedKey); err == nil {
		t.Errorf("copied object %s still exists after its media record was deleted", copiedKey)
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
