package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// UploadMediaAssets uploads all images from the assets directory and returns
// a map of relative path to media ID. Results are cached in state so repeated
// calls are free.
func UploadMediaAssets(ctx context.Context, client *Client, state *State, assetsDir string) error {
	if assetsDir == "" {
		return nil
	}

	entries, err := os.ReadDir(assetsDir)
	if err != nil {
		return fmt.Errorf("read assets dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Recurse into subdirectories.
			subDir := filepath.Join(assetsDir, entry.Name())
			if err := UploadMediaAssets(ctx, client, state, subDir); err != nil {
				return err
			}
			continue
		}

		if !isImageFile(entry.Name()) {
			continue
		}

		relPath := entry.Name()
		if _, ok := state.MediaIDs[relPath]; ok {
			continue // Already uploaded.
		}

		mediaID, err := uploadSingleAsset(ctx, client, filepath.Join(assetsDir, entry.Name()), entry.Name())
		if err != nil {
			slog.Warn("failed to upload asset, skipping",
				"file", entry.Name(),
				"error", err,
			)
			continue
		}

		state.MediaIDs[relPath] = mediaID
	}

	return nil
}

// UploadMediaDir uploads all images from a specific subdirectory of assets
// and returns a map of filename to media ID.
func UploadMediaDir(ctx context.Context, client *Client, state *State, dir string) (map[string]string, error) {
	result := make(map[string]string)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !isImageFile(entry.Name()) {
			continue
		}

		// Use category-prefixed key for deduplication.
		key := filepath.Base(dir) + "/" + entry.Name()
		if id, ok := state.MediaIDs[key]; ok {
			result[entry.Name()] = id
			continue
		}

		mediaID, err := uploadSingleAsset(ctx, client, filepath.Join(dir, entry.Name()), entry.Name())
		if err != nil {
			slog.Warn("failed to upload asset, skipping",
				"file", entry.Name(),
				"error", err,
			)
			continue
		}

		state.MediaIDs[key] = mediaID
		result[entry.Name()] = mediaID
	}

	return result, nil
}

// uploadSingleAsset reads a file and uploads it via MediaService.
func uploadSingleAsset(ctx context.Context, client *Client, path, filename string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	contentType := contentTypeForFile(filename)

	resp, err := client.Media().AddMedia(ctx, connect.NewRequest(&api.AddMediaRequest{
		EncodedBytes: data,
		ContentType:  contentType,
		Filename:     filename,
	}))
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}

	return resp.Msg.Id, nil
}

// isImageFile returns true if the filename has an image extension.
func isImageFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic":
		return true
	}
	return false
}

// contentTypeForFile returns the MIME type for a filename based on extension.
func contentTypeForFile(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".heic":
		return "image/heic"
	default:
		return "application/octet-stream"
	}
}
