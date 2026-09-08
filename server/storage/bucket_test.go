package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/fsouza/fake-gcs-server/fakestorage"
)

func TestGCSBucketStorage(t *testing.T) {
	// Create fake GCS server
	server := fakestorage.NewServer([]fakestorage.Object{})
	defer server.Stop()

	ctx := context.Background()
	bucketName := "test-bucket"

	// Create bucket in fake server
	server.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: bucketName})

	// Create storage using the fake server's client
	gcs := &GCSBucketStorage{
		client:     server.Client(),
		bucketName: bucketName,
	}

	t.Run("Put and Get", func(t *testing.T) {
		key := "test/file.txt"
		data := []byte("test content")
		contentType := "text/plain"
		metadata := map[string]string{"user": "test"}

		// Put object
		url, err := gcs.Put(ctx, key, data, contentType, metadata)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		expectedURL := "gs://test-bucket/test/file.txt"
		if url != expectedURL {
			t.Errorf("Expected URL %s, got %s", expectedURL, url)
		}

		// Get object
		retrievedData, retrievedContentType, err := gcs.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if !bytes.Equal(retrievedData, data) {
			t.Errorf("Retrieved data doesn't match. Expected %s, got %s", string(data), string(retrievedData))
		}

		if retrievedContentType != contentType {
			t.Errorf("Expected content type %s, got %s", contentType, retrievedContentType)
		}
	})

	t.Run("GetSignedURL", func(t *testing.T) {
		t.Skip("Signed URL generation requires real GCP credentials - test in integration tests")
		// NOTE: Signed URLs require real GCP service account credentials.
		// The fake-gcs-server doesn't support signed URL generation.
		// This functionality should be tested in integration tests with real GCS.
	})

	t.Run("Delete", func(t *testing.T) {
		key := "test/delete.txt"
		data := []byte("delete me")

		// Put object
		_, err := gcs.Put(ctx, key, data, "text/plain", nil)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		// Verify it exists
		_, _, err = gcs.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed before delete: %v", err)
		}

		// Delete object
		err = gcs.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify it's gone
		_, _, err = gcs.Get(ctx, key)
		if err == nil {
			t.Error("Expected error when getting deleted object")
		}
	})

	t.Run("Get non-existent object", func(t *testing.T) {
		_, _, err := gcs.Get(ctx, "non-existent/file.txt")
		if err == nil {
			t.Error("Expected error when getting non-existent object")
		}
	})

	t.Run("Copy", func(t *testing.T) {
		srcKey := "copy-test/source.txt"
		dstKey := "copy-test/destination.txt"
		data := []byte("data to copy")
		contentType := "text/plain"

		_, err := gcs.Put(ctx, srcKey, data, contentType, nil)
		if err != nil {
			t.Fatalf("Put source failed: %v", err)
		}

		storageURL, size, err := gcs.Copy(ctx, srcKey, dstKey)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		expectedURL := "gs://test-bucket/" + dstKey
		if storageURL != expectedURL {
			t.Errorf("expected URL %s, got %s", expectedURL, storageURL)
		}
		if size != int64(len(data)) {
			t.Errorf("expected size %d, got %d", len(data), size)
		}

		// Verify the destination object contains the same bytes.
		retrievedData, _, err := gcs.Get(ctx, dstKey)
		if err != nil {
			t.Fatalf("Get copied object failed: %v", err)
		}
		if string(retrievedData) != string(data) {
			t.Errorf("copied data mismatch: expected %q, got %q", string(data), string(retrievedData))
		}

		// Source must be unchanged.
		srcData, _, err := gcs.Get(ctx, srcKey)
		if err != nil {
			t.Fatalf("Get source after copy failed: %v", err)
		}
		if string(srcData) != string(data) {
			t.Errorf("source data changed after copy: expected %q, got %q", string(data), string(srcData))
		}
	})

	t.Run("Copy non-existent source", func(t *testing.T) {
		_, _, err := gcs.Copy(ctx, "non-existent/src.txt", "non-existent/dst.txt")
		if err == nil {
			t.Error("expected error when copying non-existent source")
		}
	})

	t.Run("GetToFile", func(t *testing.T) {
		key := "gettofile/source.txt"
		data := []byte("streaming content")
		contentType := "text/plain"

		_, err := gcs.Put(ctx, key, data, contentType, nil)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		destDir := t.TempDir()
		destPath := filepath.Join(destDir, "output.txt")

		gotContentType, err := gcs.GetToFile(ctx, key, destPath)
		if err != nil {
			t.Fatalf("GetToFile failed: %v", err)
		}

		if gotContentType != contentType {
			t.Errorf("expected content type %q, got %q", contentType, gotContentType)
		}

		retrievedData, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("failed to read destination file: %v", err)
		}

		if !bytes.Equal(retrievedData, data) {
			t.Errorf("data mismatch: expected %q, got %q", string(data), string(retrievedData))
		}
	})

	t.Run("GetToFile non-existent object", func(t *testing.T) {
		destPath := filepath.Join(t.TempDir(), "output.txt")
		_, err := gcs.GetToFile(ctx, "non-existent/file.txt", destPath)
		if err == nil {
			t.Error("expected error when getting non-existent object")
		}
	})

	t.Run("PutFromFile", func(t *testing.T) {
		key := "putfromfile/dest.txt"
		data := []byte("file streaming content")
		contentType := "application/octet-stream"

		srcDir := t.TempDir()
		srcPath := filepath.Join(srcDir, "source.bin")
		if err := os.WriteFile(srcPath, data, 0o600); err != nil {
			t.Fatalf("failed to write source file: %v", err)
		}

		storageURL, err := gcs.PutFromFile(ctx, key, srcPath, contentType, nil)
		if err != nil {
			t.Fatalf("PutFromFile failed: %v", err)
		}

		expectedURL := "gs://test-bucket/" + key
		if storageURL != expectedURL {
			t.Errorf("expected URL %q, got %q", expectedURL, storageURL)
		}

		retrievedData, retrievedContentType, err := gcs.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after PutFromFile failed: %v", err)
		}

		if !bytes.Equal(retrievedData, data) {
			t.Errorf("data mismatch: expected %q, got %q", string(data), string(retrievedData))
		}
		if retrievedContentType != contentType {
			t.Errorf("content type mismatch: expected %q, got %q", contentType, retrievedContentType)
		}
	})

	t.Run("PutFromFile non-existent source", func(t *testing.T) {
		_, err := gcs.PutFromFile(ctx, "any/key", "/non-existent/path.bin", "application/octet-stream", nil)
		if err == nil {
			t.Error("expected error when source file does not exist")
		}
	})
}

func TestLocalBucketStorage(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()
	serverURL := "http://localhost:8080"

	local, err := NewLocalBucketStorage(tempDir, serverURL)
	if err != nil {
		t.Fatalf("Failed to create LocalBucketStorage: %v", err)
	}

	ctx := context.Background()

	t.Run("Put and Get", func(t *testing.T) {
		key := "test/file.txt"
		data := []byte("test content")
		contentType := "text/plain"
		metadata := map[string]string{"user": "test"}

		// Put object
		url, err := local.Put(ctx, key, data, contentType, metadata)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		expectedURL := "http://localhost:8080/media/test/file.txt"
		if url != expectedURL {
			t.Errorf("Expected URL %s, got %s", expectedURL, url)
		}

		// Verify file exists on disk
		filePath := filepath.Join(tempDir, key)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("File not created at %s", filePath)
		}

		// Get object
		retrievedData, retrievedContentType, err := local.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if !bytes.Equal(retrievedData, data) {
			t.Errorf("Retrieved data doesn't match. Expected %s, got %s", string(data), string(retrievedData))
		}

		if retrievedContentType != contentType {
			t.Errorf("Expected content type %s, got %s", contentType, retrievedContentType)
		}
	})

	t.Run("GetSignedURL returns HTTP URL with default host", func(t *testing.T) {
		key := "test/signed.txt"
		data := []byte("signed content")

		// Put object first
		_, err := local.Put(ctx, key, data, "text/plain", nil)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		// Get signed URL without request host in context (uses default serverURL)
		signedURL, err := local.GetSignedURL(ctx, key, 15*time.Minute)
		if err != nil {
			t.Fatalf("GetSignedURL failed: %v", err)
		}

		expectedURL := "http://localhost:8080/media/test/signed.txt"
		if signedURL != expectedURL {
			t.Errorf("Expected URL %s, got %s", expectedURL, signedURL)
		}
	})

	t.Run("GetSignedURL uses request host from context", func(t *testing.T) {
		key := "test/context-host.txt"
		data := []byte("content with custom host")

		// Put object first
		_, err := local.Put(ctx, key, data, "text/plain", nil)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		// Add custom host to context (simulating Android emulator)
		ctxWithHost := WithRequestHost(ctx, "10.0.2.2:8080")

		// Get signed URL - should use the request host from context
		signedURL, err := local.GetSignedURL(ctxWithHost, key, 15*time.Minute)
		if err != nil {
			t.Fatalf("GetSignedURL failed: %v", err)
		}

		expectedURL := "http://10.0.2.2:8080/media/test/context-host.txt"
		if signedURL != expectedURL {
			t.Errorf("Expected URL %s, got %s", expectedURL, signedURL)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		key := "test/delete.txt"
		data := []byte("delete me")

		// Put object
		_, err := local.Put(ctx, key, data, "text/plain", nil)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		filePath := filepath.Join(tempDir, key)

		// Verify file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("File should exist before delete")
		}

		// Delete object
		err = local.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify file is gone
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Error("File should not exist after delete")
		}
	})

	t.Run("Get non-existent object", func(t *testing.T) {
		_, _, err := local.Get(ctx, "non-existent/file.txt")
		if err == nil {
			t.Error("Expected error when getting non-existent object")
		}
	})

	t.Run("Creates nested directories", func(t *testing.T) {
		key := "deeply/nested/path/file.txt"
		data := []byte("nested content")

		_, err := local.Put(ctx, key, data, "text/plain", nil)
		if err != nil {
			t.Fatalf("Put failed for nested path: %v", err)
		}

		filePath := filepath.Join(tempDir, key)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Nested file not created at %s", filePath)
		}
	})

	t.Run("Copy", func(t *testing.T) {
		srcKey := "copy-src/file.txt"
		dstKey := "copy-dst/subdir/file.txt"
		data := []byte("file to copy")
		contentType := "text/plain"

		_, err := local.Put(ctx, srcKey, data, contentType, nil)
		if err != nil {
			t.Fatalf("Put source failed: %v", err)
		}

		storageURL, size, err := local.Copy(ctx, srcKey, dstKey)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		expectedURL := "http://localhost:8080/media/" + dstKey
		if storageURL != expectedURL {
			t.Errorf("expected URL %s, got %s", expectedURL, storageURL)
		}
		if size != int64(len(data)) {
			t.Errorf("expected size %d, got %d", len(data), size)
		}

		// Destination file must exist on disk.
		dstPath := filepath.Join(tempDir, dstKey)
		if _, err := os.Stat(dstPath); os.IsNotExist(err) {
			t.Errorf("destination file not created at %s", dstPath)
		}

		// Destination must contain the same bytes.
		retrievedData, _, err := local.Get(ctx, dstKey)
		if err != nil {
			t.Fatalf("Get copied file failed: %v", err)
		}
		if string(retrievedData) != string(data) {
			t.Errorf("copied data mismatch: expected %q, got %q", string(data), string(retrievedData))
		}
	})

	t.Run("Copy preserves metadata sidecar", func(t *testing.T) {
		srcKey := "meta-src/file.bin"
		dstKey := "meta-dst/file.bin"
		data := []byte("binary content")
		contentType := "application/octet-stream"

		_, err := local.Put(ctx, srcKey, data, contentType, map[string]string{"owner": "test"})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		_, _, err = local.Copy(ctx, srcKey, dstKey)
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// Content type must be readable from the copied metadata sidecar.
		_, retrievedContentType, err := local.Get(ctx, dstKey)
		if err != nil {
			t.Fatalf("Get copied file failed: %v", err)
		}
		if retrievedContentType != contentType {
			t.Errorf("expected content type %s, got %s", contentType, retrievedContentType)
		}
	})

	t.Run("Copy non-existent source", func(t *testing.T) {
		_, _, err := local.Copy(ctx, "no-such/file.txt", "dst/file.txt")
		if err == nil {
			t.Error("expected error when copying non-existent source")
		}
	})

	t.Run("GetToFile", func(t *testing.T) {
		key := "gettofile/source.bin"
		data := []byte("local streaming content")
		contentType := "application/octet-stream"

		_, err := local.Put(ctx, key, data, contentType, nil)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		destDir := t.TempDir()
		destPath := filepath.Join(destDir, "output.bin")

		gotContentType, err := local.GetToFile(ctx, key, destPath)
		if err != nil {
			t.Fatalf("GetToFile failed: %v", err)
		}

		if gotContentType != contentType {
			t.Errorf("expected content type %q, got %q", contentType, gotContentType)
		}

		retrievedData, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("failed to read destination file: %v", err)
		}
		if !bytes.Equal(retrievedData, data) {
			t.Errorf("data mismatch: expected %q, got %q", string(data), string(retrievedData))
		}
	})

	t.Run("GetToFile preserves metadata sidecar content type", func(t *testing.T) {
		key := "gettofile/meta.bin"
		data := []byte("metadata content")
		contentType := "video/mp4"

		_, err := local.Put(ctx, key, data, contentType, map[string]string{"owner": "test"})
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		destPath := filepath.Join(t.TempDir(), "video.mp4")
		gotContentType, err := local.GetToFile(ctx, key, destPath)
		if err != nil {
			t.Fatalf("GetToFile failed: %v", err)
		}
		if gotContentType != contentType {
			t.Errorf("expected content type %q, got %q", contentType, gotContentType)
		}
	})

	t.Run("GetToFile non-existent object", func(t *testing.T) {
		destPath := filepath.Join(t.TempDir(), "output.bin")
		_, err := local.GetToFile(ctx, "no-such/file.bin", destPath)
		if err == nil {
			t.Error("expected error when getting non-existent object")
		}
	})

	t.Run("PutFromFile", func(t *testing.T) {
		key := "putfromfile/dest.bin"
		data := []byte("put from file content")
		contentType := "image/jpeg"

		srcDir := t.TempDir()
		srcPath := filepath.Join(srcDir, "source.jpg")
		if err := os.WriteFile(srcPath, data, 0o600); err != nil {
			t.Fatalf("failed to write source file: %v", err)
		}

		storageURL, err := local.PutFromFile(ctx, key, srcPath, contentType, nil)
		if err != nil {
			t.Fatalf("PutFromFile failed: %v", err)
		}

		expectedURL := "http://localhost:8080/media/" + key
		if storageURL != expectedURL {
			t.Errorf("expected URL %q, got %q", expectedURL, storageURL)
		}

		retrievedData, retrievedContentType, err := local.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after PutFromFile failed: %v", err)
		}
		if !bytes.Equal(retrievedData, data) {
			t.Errorf("data mismatch: expected %q, got %q", string(data), string(retrievedData))
		}
		if retrievedContentType != contentType {
			t.Errorf("content type mismatch: expected %q, got %q", contentType, retrievedContentType)
		}
	})

	t.Run("PutFromFile preserves metadata sidecar", func(t *testing.T) {
		key := "putfromfile/meta.bin"
		data := []byte("content with metadata")
		contentType := "image/png"

		srcPath := filepath.Join(t.TempDir(), "source.png")
		if err := os.WriteFile(srcPath, data, 0o600); err != nil {
			t.Fatalf("failed to write source file: %v", err)
		}

		_, err := local.PutFromFile(ctx, key, srcPath, contentType, map[string]string{"original_media_id": "abc123"})
		if err != nil {
			t.Fatalf("PutFromFile failed: %v", err)
		}

		_, gotContentType, err := local.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after PutFromFile failed: %v", err)
		}
		if gotContentType != contentType {
			t.Errorf("expected content type %q, got %q", contentType, gotContentType)
		}
	})

	t.Run("PutFromFile non-existent source", func(t *testing.T) {
		_, err := local.PutFromFile(ctx, "any/key", "/non-existent/path.bin", "application/octet-stream", nil)
		if err == nil {
			t.Error("expected error when source file does not exist")
		}
	})
}

func TestNewLocalBucketStorage_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "new-storage-dir")

	// Directory shouldn't exist yet
	if _, err := os.Stat(storagePath); !os.IsNotExist(err) {
		t.Error("Directory should not exist before creation")
	}

	// Create storage (should create directory)
	_, err := NewLocalBucketStorage(storagePath, "http://localhost:8080")
	if err != nil {
		t.Fatalf("NewLocalBucketStorage failed: %v", err)
	}

	// Directory should now exist
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		t.Error("Directory should exist after creation")
	}
}

func TestLocalBucketStorage_GetBasePath(t *testing.T) {
	tempDir := t.TempDir()
	local, err := NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create LocalBucketStorage: %v", err)
	}

	basePath := local.GetBasePath()
	if basePath != tempDir {
		t.Errorf("Expected base path %s, got %s", tempDir, basePath)
	}
}

// TestLocalBucketStorage_RejectsEscapingKeys locks the guard added for gosec's
// G703 (#3008): filepath.Join CLEANS its result, so without the check a key
// that climbs out of basePath would read or write anywhere the process can
// reach. Every method that takes a key is exercised, because the guard is only
// worth anything if none of them skipped it.
func TestLocalBucketStorage_RejectsEscapingKeys(t *testing.T) {
	tempDir := t.TempDir()
	local, err := NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create LocalBucketStorage: %v", err)
	}
	ctx := context.Background()

	// A file outside the bucket that a traversal would reach.
	outside := filepath.Join(filepath.Dir(tempDir), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("failed to seed the outside file: %v", err)
	}
	defer os.Remove(outside)

	escaping := []string{
		"../outside.txt",
		"a/../../outside.txt",
		"/etc/passwd",
		"",
	}

	for _, key := range escaping {
		t.Run("key="+strconv.Quote(key), func(t *testing.T) {
			if _, err := local.Put(ctx, key, []byte("x"), "text/plain", nil); err == nil {
				t.Error("Put accepted an escaping key")
			}
			if _, _, err := local.Get(ctx, key); err == nil {
				t.Error("Get accepted an escaping key")
			}
			if _, err := local.GetToFile(ctx, key, filepath.Join(tempDir, "dest")); err == nil {
				t.Error("GetToFile accepted an escaping key")
			}
			if _, err := local.PutFromFile(ctx, key, outside, "text/plain", nil); err == nil {
				t.Error("PutFromFile accepted an escaping key")
			}
			if _, err := local.GetSignedURL(ctx, key, time.Hour); err == nil {
				t.Error("GetSignedURL accepted an escaping key")
			}
			if _, _, err := local.Copy(ctx, key, "safe/dst"); err == nil {
				t.Error("Copy accepted an escaping source key")
			}
			if _, _, err := local.Copy(ctx, "safe/src", key); err == nil {
				t.Error("Copy accepted an escaping destination key")
			}
			if err := local.Delete(ctx, key); err == nil {
				t.Error("Delete accepted an escaping key")
			}
		})
	}

	// The file a traversal would have clobbered is untouched.
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("the outside file went missing: %v", err)
	}
	if string(data) != "secret" {
		t.Errorf("the outside file was modified: got %q", data)
	}

	// The real key shape still works, so the guard is not simply rejecting
	// everything — which is the way this kind of test passes for free.
	if _, err := local.Put(ctx, MediaBucketKey("user-1", "media-1"), []byte("ok"), "text/plain", nil); err != nil {
		t.Errorf("Put rejected a normal userID/mediaID key: %v", err)
	}
}

func TestGCSBucketStorage_GetDurableSignedURL(t *testing.T) {
	// Generate a throwaway RSA key to act as the user-managed signer. The durable
	// path signs locally via the package-level SignedURL, so no GCS server or real
	// credentials are needed — unlike GetSignedURL, which requires live signBlob.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	// Real GCP service-account keys carry a PKCS#8 "PRIVATE KEY" PEM in private_key.
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	const signer = "feedback-url-signer@example.iam.gserviceaccount.com"
	gcs := &GCSBucketStorage{
		bucketName:       "test-bucket",
		signerEmail:      signer,
		signerPrivateKey: pemBytes,
	}

	t.Run("signs with the configured key for the full duration", func(t *testing.T) {
		const dur = 180 * 24 * time.Hour
		raw, err := gcs.GetDurableSignedURL(context.Background(), "feedback-system/abc", dur)
		if err != nil {
			t.Fatalf("GetDurableSignedURL: %v", err)
		}

		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse url: %v", err)
		}
		if u.Host != "storage.googleapis.com" {
			t.Errorf("host = %q, want storage.googleapis.com", u.Host)
		}
		if u.Path != "/test-bucket/feedback-system/abc" {
			t.Errorf("path = %q, want /test-bucket/feedback-system/abc", u.Path)
		}

		q := u.Query()
		if got := q.Get("GoogleAccessId"); got != signer {
			t.Errorf("GoogleAccessId = %q, want %q", got, signer)
		}
		if q.Get("Signature") == "" {
			t.Error("missing Signature query param")
		}

		// Expiry must be ~180 days out — well beyond any managed-key rotation
		// window, which is the whole point of durable signing (#2549).
		exp, err := strconv.ParseInt(q.Get("Expires"), 10, 64)
		if err != nil {
			t.Fatalf("parse Expires %q: %v", q.Get("Expires"), err)
		}
		if remaining := time.Until(time.Unix(exp, 0)); remaining < 179*24*time.Hour {
			t.Errorf("Expires too soon: %v remaining, want ~180 days", remaining)
		}
	})

	t.Run("errors when no signer configured", func(t *testing.T) {
		unsigned := &GCSBucketStorage{bucketName: "test-bucket"}
		if _, err := unsigned.GetDurableSignedURL(context.Background(), "feedback-system/abc", time.Hour); err == nil {
			t.Fatal("expected an error when no URL signer is configured")
		}
	})
}
