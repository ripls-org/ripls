package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// PexelsProvider implements StockImageryProvider using the Pexels API.
type PexelsProvider struct {
	client  pexelsClientInterface
	storage *storage.ProtoSQLStorage
	bucket  storage.BucketStorage
}

// NewPexelsProvider creates a new Pexels-based stock imagery provider.
func NewPexelsProvider(
	client *PexelsClient,
	sqlStorage *storage.ProtoSQLStorage,
	bucket storage.BucketStorage,
) *PexelsProvider {
	return &PexelsProvider{
		client:  client,
		storage: sqlStorage,
		bucket:  bucket,
	}
}

// GetStockImage fetches a stock image from Pexels matching the query.
func (p *PexelsProvider) GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "pexels",
		"operation", "GetStockImage",
		"query", query,
	)
	startTime := time.Now()

	if opts == nil {
		opts = &StockImageOptions{}
	}

	// Search for photos
	searchResp, err := p.client.SearchPhotos(ctx, query, defaultSearchResults)
	if err != nil {
		logger.ErrorContext(ctx, "stock imagery fetch failed",
			"failure_type", "search_error",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to search Pexels: %w", err)
	}

	if len(searchResp.Photos) == 0 {
		logger.WarnContext(ctx, "stock imagery fetch failed",
			"failure_type", "no_results",
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("no images found for query: %s", query)
	}

	// Filter results if uniqueness is required
	var photo *PexelsPhoto
	if opts.RequireUnique {
		photo, err = p.selectUniquePhoto(ctx, searchResp.Photos)
		if err != nil {
			logger.ErrorContext(ctx, "stock imagery fetch failed",
				"failure_type", "all_results_used",
				"error", err,
				"duration_ms", time.Since(startTime).Milliseconds())
			return nil, err
		}
	} else {
		photo = &searchResp.Photos[0]

		// Check if we already have this image stored
		existing, err := p.getExistingStockImage(ctx, fmt.Sprintf("%d", photo.ID))
		if err == nil && existing != nil {
			logger.InfoContext(ctx, "stock imagery fetch succeeded",
				"result", "cached",
				"stock_image_id", existing.Id,
				"media_id", existing.MediaId,
				"photo_id", photo.ID,
				"duration_ms", time.Since(startTime).Milliseconds(),
			)
			return existing, nil
		}
	}

	// Download the image
	imageData, err := p.client.DownloadPhoto(ctx, photo)
	if err != nil {
		logger.ErrorContext(ctx, "stock imagery fetch failed",
			"failure_type", "download_error",
			"error", err,
			"photo_id", photo.ID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	// Store as Media
	description := photo.Alt
	if description == "" {
		description = fmt.Sprintf("Photo by %s", photo.Photographer)
	}
	mediaID, err := storage.StoreMedia(
		ctx,
		p.storage,
		p.bucket,
		SystemUserID,
		imageData,
		"image/jpeg",
		fmt.Sprintf("pexels_%d.jpg", photo.ID),
		description,
		"",    // Canonical stock images don't reference themselves
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		logger.ErrorContext(ctx, "stock imagery fetch failed",
			"failure_type", "storage_error",
			"error", err,
			"photo_id", photo.ID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to store media: %w", err)
	}

	// Build provider image metadata
	providerImage := &models.ProviderImage{
		Id:             fmt.Sprintf("%d", photo.ID),
		Url:            photo.URL, // Photo page URL for attribution
		Description:    photo.Alt,
		AltDescription: "",
		Creator: &models.CreatorInfo{
			Name:            photo.Photographer,
			Username:        "", // Pexels API doesn't provide username in search results
			PhotographerUrl: photo.PhotographerURL,
		},
	}

	// Create StockImage record
	stockImage := &models.StockImage{
		Provider:         models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage:    providerImage,
		MediaId:          mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}

	stockImageID, err := p.storage.Insert(ctx, stockImage)
	if err != nil {
		logger.ErrorContext(ctx, "stock imagery fetch failed",
			"failure_type", "database_error",
			"error", err,
			"media_id", mediaID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to insert stock image record: %w", err)
	}
	stockImage.Id = stockImageID

	logger.InfoContext(ctx, "stock imagery fetch succeeded",
		"result", "new",
		"stock_image_id", stockImageID,
		"media_id", mediaID,
		"photo_id", photo.ID,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return stockImage, nil
}

// SearchStockImageCandidates returns up to `limit` candidate photos for
// `query` as lightweight URL+metadata entries (no download). Used by the
// streaming Gen* flows to surface alternates in the Replace Media modal.
// Returns nil, nil when the search yields no usable results.
func (p *PexelsProvider) SearchStockImageCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "pexels",
		"operation", "SearchStockImageCandidates",
		"query", query,
		"limit", limit,
	)
	startTime := time.Now()

	// Pexels caps perPage to a reasonable number; ask for a few extra
	// so we have headroom after filtering, then trim.
	perPage := limit + 2
	if perPage > defaultSearchResults {
		perPage = defaultSearchResults
	}
	resp, err := p.client.SearchPhotos(ctx, query, perPage)
	if err != nil {
		logger.WarnContext(ctx, "candidate search failed",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("failed to search Pexels candidates: %w", err)
	}
	if len(resp.Photos) == 0 {
		return nil, nil
	}

	out := make([]StockImageCandidate, 0, limit)
	for i := range resp.Photos {
		if len(out) >= limit {
			break
		}
		photo := &resp.Photos[i]
		// Prefer the Medium variant — fast to load for a 72-100px
		// thumbnail and within Pexels's CDN-cached set. Fall back to
		// Small when Medium is empty.
		url := photo.Src.Medium
		if url == "" {
			url = photo.Src.Small
		}
		if url == "" {
			continue
		}
		out = append(out, StockImageCandidate{
			URL:             url,
			ContentType:     "image/jpeg",
			WidthPx:         int32(photo.Width),
			HeightPx:        int32(photo.Height),
			Provider:        models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
			ProviderPhotoID: fmt.Sprintf("%d", photo.ID),
		})
	}
	logger.DebugContext(ctx, "candidate search succeeded",
		"result_count", len(out),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return out, nil
}

// getExistingStockImage returns an existing StockImage for the given provider image ID, if one exists.
func (p *PexelsProvider) getExistingStockImage(ctx context.Context, providerImageID string) (*models.StockImage, error) {
	results, err := p.storage.QueryByField(ctx, "provider_image_id", providerImageID, &models.StockImage{})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0].(*models.StockImage), nil
}

// selectUniquePhoto finds a photo that hasn't been used before in Ripls.
func (p *PexelsProvider) selectUniquePhoto(ctx context.Context, photos []PexelsPhoto) (*PexelsPhoto, error) {
	// Get all provider image IDs from the search results
	providerIDs := make([]string, len(photos))
	for i, photo := range photos {
		providerIDs[i] = fmt.Sprintf("%d", photo.ID)
	}

	// Query which ones already exist
	existingIDs, err := p.getExistingProviderImageIDs(ctx, providerIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing images: %w", err)
	}

	// Find first photo that doesn't exist
	existingSet := make(map[string]bool, len(existingIDs))
	for _, id := range existingIDs {
		existingSet[id] = true
	}

	for i := range photos {
		photoID := fmt.Sprintf("%d", photos[i].ID)
		if !existingSet[photoID] {
			return &photos[i], nil
		}
	}

	return nil, fmt.Errorf("all %d search results already exist in Ripls", len(photos))
}

// getExistingProviderImageIDs returns which of the given provider image IDs already exist.
func (p *PexelsProvider) getExistingProviderImageIDs(ctx context.Context, providerIDs []string) ([]string, error) {
	var existing []string
	for _, id := range providerIDs {
		// Query by the flattened field name
		results, err := p.storage.QueryByField(ctx, "provider_image_id", id, &models.StockImage{})
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			existing = append(existing, id)
		}
	}
	return existing, nil
}

// SearchStockVideoCandidates returns up to `limit` candidate videos
// for `query` as lightweight entries (poster image + metadata only, no
// download). Mirrors SearchStockImageCandidates so the client can
// render video alternates with a play overlay in the Replace Media
// modal. Returns nil, nil when the search yields no usable results.
func (p *PexelsProvider) SearchStockVideoCandidates(ctx context.Context, query string, limit int) ([]StockImageCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_video",
		"provider", "pexels",
		"operation", "SearchStockVideoCandidates",
		"query", query,
		"limit", limit,
	)
	startTime := time.Now()

	perPage := limit + 2
	if perPage > defaultSearchResults {
		perPage = defaultSearchResults
	}
	resp, err := p.client.SearchVideos(ctx, query, perPage, "portrait")
	if err != nil {
		logger.WarnContext(ctx, "candidate search failed",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("failed to search Pexels video candidates: %w", err)
	}
	if len(resp.Videos) == 0 {
		return nil, nil
	}

	out := make([]StockImageCandidate, 0, limit)
	for i := range resp.Videos {
		if len(out) >= limit {
			break
		}
		v := &resp.Videos[i]
		mp4 := pickPexelsVideoFile(v.VideoFiles)
		if v.Image == "" || mp4 == "" {
			continue
		}
		out = append(out, StockImageCandidate{
			URL:             mp4,
			ContentType:     "video/mp4",
			Provider:        models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
			ProviderPhotoID: fmt.Sprintf("%d", v.ID),
			ThumbnailURL:    v.Image,
		})
	}
	logger.DebugContext(ctx, "candidate search succeeded",
		"result_count", len(out),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return out, nil
}

// pickPexelsVideoFile selects an MP4 variant suitable for the Replace
// Media import — targets ~720p so the resulting file renders sharply
// on a portrait hero without pulling a multi-hundred-megabyte 4k
// master. Returns "" when no variant is usable (no link / dimensions
// out of range), so the caller can drop the candidate.
//
// No size filter: the candidate-import path routes through
// AddMediaFromURL → importViaTrustedProvider → GetStockVideoByID,
// which downloads via the internal client (no body cap). The generic
// AddMediaFromURL fallback path with its 60 MB cap is only reachable
// when the trusted provider is unavailable — which today is never the
// case in prod — so capping the picker is purely defensive against a
// dead code path and costs us legitimate candidates.
func pickPexelsVideoFile(files []PexelsVideoFile) string {
	const minAcceptableWidth = 360
	const targetWidth = 720

	var (
		bestNear PexelsVideoFile
		bestAny  PexelsVideoFile
	)
	for _, f := range files {
		if f.Link == "" || f.Width < minAcceptableWidth {
			continue
		}
		// Track the smallest acceptable variant as a fallback.
		if bestAny.Link == "" || f.Width < bestAny.Width {
			bestAny = f
		}
		// Prefer the smallest variant that meets the target width — a
		// 720p clip is sharp enough for the hero without paying the
		// quadratic cost of 1080p/4k bytes.
		if f.Width >= targetWidth && (bestNear.Link == "" || f.Width < bestNear.Width) {
			bestNear = f
		}
	}
	if bestNear.Link != "" {
		return bestNear.Link
	}
	return bestAny.Link
}

// GetStockVideo fetches a stock video from Pexels matching the query.
// Returns a StockImage record whose media has content_type video/mp4.
func (p *PexelsProvider) GetStockVideo(ctx context.Context, query string) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_video",
		"provider", "pexels",
		"operation", "GetStockVideo",
		"query", query,
	)
	startTime := time.Now()

	// Search for portrait-oriented videos (preferred for mobile backgrounds)
	searchResp, err := p.client.SearchVideos(ctx, query, defaultSearchResults, "portrait")
	if err != nil {
		logger.ErrorContext(ctx, "stock video fetch failed",
			"failure_type", "search_error",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to search Pexels videos: %w", err)
	}

	if len(searchResp.Videos) == 0 {
		logger.WarnContext(ctx, "stock video fetch failed",
			"failure_type", "no_results",
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("no videos found for query: %s", query)
	}

	video := &searchResp.Videos[0]

	// Select the best quality video file
	videoFile := selectBestVideoFile(video.VideoFiles)
	if videoFile == nil {
		return nil, fmt.Errorf("no usable video files for video %d", video.ID)
	}

	// Download video bytes
	videoData, err := p.client.DownloadVideo(ctx, videoFile.Link)
	if err != nil {
		logger.ErrorContext(ctx, "stock video fetch failed",
			"failure_type", "download_error",
			"error", err,
			"video_id", video.ID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to download video: %w", err)
	}

	// Store as Media with video/mp4 content type
	description := fmt.Sprintf("Video by %s", video.User.Name)
	mediaID, err := storage.StoreMedia(
		ctx,
		p.storage,
		p.bucket,
		SystemUserID,
		videoData,
		"video/mp4",
		fmt.Sprintf("pexels_video_%d.mp4", video.ID),
		description,
		"",
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		logger.ErrorContext(ctx, "stock video fetch failed",
			"failure_type", "storage_error",
			"error", err,
			"video_id", video.ID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to store video: %w", err)
	}

	// Use the Pexels poster image as the video thumbnail, avoiding ffmpeg.
	// This is a best-effort operation; failure does not block video storage.
	if video.Image != "" {
		if err := p.setPosterAsThumbnail(ctx, mediaID, video.Image); err != nil {
			logger.WarnContext(ctx, "failed to set poster as thumbnail",
				"media_id", mediaID,
				"poster_url", video.Image,
				"error", err)
		}
	}

	// Build provider image metadata with attribution
	providerImage := &models.ProviderImage{
		Id:          fmt.Sprintf("video_%d", video.ID),
		Url:         video.URL, // Video page URL for attribution
		Description: description,
		Creator: &models.CreatorInfo{
			Name:            video.User.Name,
			PhotographerUrl: video.User.URL,
		},
	}

	// Create StockImage record (reused for video — media content_type distinguishes them)
	stockImage := &models.StockImage{
		Provider:         models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage:    providerImage,
		MediaId:          mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}

	stockImageID, err := p.storage.Insert(ctx, stockImage)
	if err != nil {
		logger.ErrorContext(ctx, "stock video fetch failed",
			"failure_type", "database_error",
			"error", err,
			"media_id", mediaID,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to insert stock video record: %w", err)
	}
	stockImage.Id = stockImageID

	logger.InfoContext(ctx, "stock video fetch succeeded",
		"stock_image_id", stockImageID,
		"media_id", mediaID,
		"video_id", video.ID,
		"quality", videoFile.Quality,
		"width", videoFile.Width,
		"height", videoFile.Height,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return stockImage, nil
}

// GetStockImageByID downloads the Pexels photo identified by photoID
// and stores it as a canonical StockImage (deduped against existing).
// Used by the Replace Media import path so the swap preserves
// photographer attribution. Skips the search step that GetStockImage
// runs; otherwise behaves identically.
func (p *PexelsProvider) GetStockImageByID(ctx context.Context, photoID string) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "pexels",
		"operation", "GetStockImageByID",
		"photo_id", photoID,
	)
	startTime := time.Now()

	// Dedupe: if we already have this photo stored, return the existing
	// canonical StockImage. The candidate-import path will then create a
	// per-user copy via CopyStockImageForUser.
	if existing, err := p.getExistingStockImage(ctx, photoID); err == nil && existing != nil {
		logger.InfoContext(ctx, "stock imagery by-id returning cached",
			"stock_image_id", existing.Id,
			"media_id", existing.MediaId,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return existing, nil
	}

	photo, err := p.client.GetPhotoByID(ctx, photoID)
	if err != nil {
		logger.WarnContext(ctx, "pexels photo lookup failed",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("pexels photo by id: %w", err)
	}
	return p.storePexelsPhotoAsStockImage(ctx, photo, startTime)
}

// GetStockVideoByID downloads the Pexels video identified by videoID
// and stores it as a canonical StockImage (deduped against existing).
// Mirrors GetStockImageByID for the video endpoint.
func (p *PexelsProvider) GetStockVideoByID(ctx context.Context, videoID string) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_video",
		"provider", "pexels",
		"operation", "GetStockVideoByID",
		"video_id", videoID,
	)
	startTime := time.Now()

	// Stock videos are keyed under `video_<id>` to keep the provider's
	// image and video namespaces distinct in the StockImage table.
	if existing, err := p.getExistingStockImage(ctx, "video_"+videoID); err == nil && existing != nil {
		logger.InfoContext(ctx, "stock video by-id returning cached",
			"stock_image_id", existing.Id,
			"media_id", existing.MediaId,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return existing, nil
	}

	video, err := p.client.GetVideoByID(ctx, videoID)
	if err != nil {
		logger.WarnContext(ctx, "pexels video lookup failed",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return nil, fmt.Errorf("pexels video by id: %w", err)
	}
	return p.storePexelsVideoAsStockImage(ctx, video, startTime)
}

// storePexelsPhotoAsStockImage handles the shared "download + store +
// insert StockImage" path used by both GetStockImage (search-then-pick)
// and GetStockImageByID (direct lookup).
func (p *PexelsProvider) storePexelsPhotoAsStockImage(ctx context.Context, photo *PexelsPhoto, startTime time.Time) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_imagery",
		"provider", "pexels",
		"operation", "storePexelsPhotoAsStockImage",
		"photo_id", photo.ID,
	)
	imageData, err := p.client.DownloadPhoto(ctx, photo)
	if err != nil {
		logger.ErrorContext(ctx, "photo download failed", "error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	description := photo.Alt
	if description == "" {
		description = fmt.Sprintf("Photo by %s", photo.Photographer)
	}
	mediaID, err := storage.StoreMedia(
		ctx, p.storage, p.bucket, SystemUserID,
		imageData, "image/jpeg",
		fmt.Sprintf("pexels_%d.jpg", photo.ID),
		description, "",
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store media: %w", err)
	}
	stockImage := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:             fmt.Sprintf("%d", photo.ID),
			Url:            photo.URL,
			Description:    photo.Alt,
			AltDescription: "",
			Creator: &models.CreatorInfo{
				Name:            photo.Photographer,
				PhotographerUrl: photo.PhotographerURL,
			},
		},
		MediaId:          mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	stockImageID, err := p.storage.Insert(ctx, stockImage)
	if err != nil {
		return nil, fmt.Errorf("failed to insert stock image record: %w", err)
	}
	stockImage.Id = stockImageID
	logger.InfoContext(ctx, "stock image stored",
		"stock_image_id", stockImageID, "media_id", mediaID,
		"duration_ms", time.Since(startTime).Milliseconds())
	return stockImage, nil
}

// storePexelsVideoAsStockImage handles the shared download + store path
// for Pexels videos. Picks the best variant from VideoFiles, downloads,
// stores as media with content_type video/mp4, sets the poster as the
// thumbnail (best-effort), and creates the StockImage row.
func (p *PexelsProvider) storePexelsVideoAsStockImage(ctx context.Context, video *PexelsVideo, startTime time.Time) (*models.StockImage, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "stock_video",
		"provider", "pexels",
		"operation", "storePexelsVideoAsStockImage",
		"video_id", video.ID,
	)
	videoFile := selectBestVideoFile(video.VideoFiles)
	if videoFile == nil {
		return nil, fmt.Errorf("no usable video files for video %d", video.ID)
	}
	videoData, err := p.client.DownloadVideo(ctx, videoFile.Link)
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	description := fmt.Sprintf("Video by %s", video.User.Name)
	mediaID, err := storage.StoreMedia(
		ctx, p.storage, p.bucket, SystemUserID,
		videoData, "video/mp4",
		fmt.Sprintf("pexels_video_%d.mp4", video.ID),
		description, "",
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store video: %w", err)
	}
	if video.Image != "" {
		if err := p.setPosterAsThumbnail(ctx, mediaID, video.Image); err != nil {
			logger.WarnContext(ctx, "failed to set poster as thumbnail",
				"media_id", mediaID, "poster_url", video.Image, "error", err)
		}
	}
	stockImage := &models.StockImage{
		Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
		ProviderImage: &models.ProviderImage{
			Id:          fmt.Sprintf("video_%d", video.ID),
			Url:         video.URL,
			Description: description,
			Creator: &models.CreatorInfo{
				Name:            video.User.Name,
				PhotographerUrl: video.User.URL,
			},
		},
		MediaId:          mediaID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	stockImageID, err := p.storage.Insert(ctx, stockImage)
	if err != nil {
		return nil, fmt.Errorf("failed to insert stock video record: %w", err)
	}
	stockImage.Id = stockImageID
	logger.InfoContext(ctx, "stock video stored",
		"stock_image_id", stockImageID, "media_id", mediaID,
		"width", videoFile.Width, "height", videoFile.Height,
		"duration_ms", time.Since(startTime).Milliseconds())
	return stockImage, nil
}

// setPosterAsThumbnail downloads a Pexels poster image and stores it as the thumbnail for a video media record.
// This avoids ffmpeg frame extraction for Pexels stock videos, which already provide a high-quality poster.
// Returns an error if download or storage fails; callers should treat this as non-fatal.
func (p *PexelsProvider) setPosterAsThumbnail(ctx context.Context, mediaID, posterURL string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PexelsProvider.setPosterAsThumbnail",
		"media_id", mediaID,
	)

	// Download the poster image
	posterData, err := p.client.DownloadVideo(ctx, posterURL)
	if err != nil {
		return fmt.Errorf("failed to download poster: %w", err)
	}

	// Get the media record to find the owner (SystemUserID)
	media := &models.Media{}
	if err := p.storage.GetByID(ctx, mediaID, media); err != nil {
		return fmt.Errorf("failed to get media record: %w", err)
	}

	// Upload poster as thumbnail
	thumbnailKey := storage.ThumbnailBucketKey(media.UserId, mediaID)
	thumbnailURL, err := p.bucket.Put(ctx, thumbnailKey, posterData, "image/jpeg", map[string]string{
		"original_media_id": mediaID,
	})
	if err != nil {
		return fmt.Errorf("failed to upload poster thumbnail: %w", err)
	}

	// Update media record with thumbnail URL
	media.ThumbnailStorageUrl = &thumbnailURL
	if err := p.storage.Update(ctx, media); err != nil {
		return fmt.Errorf("failed to update media thumbnail URL: %w", err)
	}

	logger.InfoContext(ctx, "poster set as thumbnail", "thumbnail_url", thumbnailURL)
	return nil
}

// selectBestVideoFile chooses the best video file from the available formats.
// Preference order: portrait HD → any portrait → landscape HD → first available.
func selectBestVideoFile(files []PexelsVideoFile) *PexelsVideoFile {
	// Priority 1: Portrait orientation (height > width) with HD quality
	for i := range files {
		f := &files[i]
		if f.Quality == "hd" && f.Height > f.Width {
			return f
		}
	}
	// Priority 2: Any portrait orientation
	for i := range files {
		f := &files[i]
		if f.Height > f.Width {
			return f
		}
	}
	// Priority 3: HD quality in any orientation
	for i := range files {
		f := &files[i]
		if f.Quality == "hd" {
			return f
		}
	}
	// Priority 4: First available
	if len(files) > 0 {
		return &files[0]
	}
	return nil
}

// CheckHealth validates Pexels API connectivity with a minimal search request.
func (p *PexelsProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()

	status := &health.Status{
		Name:    "stock_imagery",
		Backend: "pexels",
	}

	// Use a unique search query with timestamp to validate API key and connectivity.
	// Pexels CDN caches responses for common queries (even multi-word ones) and serves
	// them without validating the API key. Using a unique timestamp-based query ensures
	// the request reaches the API and the key is actually validated.
	query := fmt.Sprintf("healthcheck%d", time.Now().UnixNano())
	requestURL := fmt.Sprintf("%s/search?query=%s&per_page=1", pexelsAPIURL, query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}

	// Need to get the API key from the client - access it via the underlying client
	client, ok := p.client.(*PexelsClient)
	if !ok {
		// For mock clients in tests, just return healthy
		return []*health.Status{status}, nil
	}
	req.Header.Set("Authorization", client.apiKey)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		status.LatencyMs = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return []*health.Status{status}, nil
	}
	defer resp.Body.Close()

	status.LatencyMs = time.Since(start).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		status.Error = fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))
		return []*health.Status{status}, nil
	}

	return []*health.Status{status}, nil
}
