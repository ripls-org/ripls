package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/branding"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/safehttp"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

const (
	// addMediaFromURLTimeout caps the server-side fetch for
	// AddMediaFromURL. Matches the existing webpage-image download
	// timeout so the two paths behave consistently.
	addMediaFromURLTimeout = 10 * time.Second

	// addMediaFromURLMaxBytes caps the maximum body size accepted for
	// AddMediaFromURL. Shares the codebase-wide image cap (#1953) so every
	// media path bounds on the same limit; 60 MB covers high-resolution
	// stock images without opening a download-bomb surface.
	addMediaFromURLMaxBytes = storage.MaxImageUploadBytes
)

// Service implements the MediaService RPC interface.
type Service struct {
	storage              *storage.ProtoSQLStorage
	bucket               storage.BucketStorage
	stockImageryProvider media.StockImageryProvider // Optional: nil-safe
	stockVideoProvider   media.StockVideoProvider   // Optional: nil-safe

	// outboundClient builds the *http.Client used for the generic-URL
	// fetch in AddMediaFromURL. Defaults to safehttp.NewClient
	// (SSRF-guarded). Tests inject a client that trusts the httptest
	// TLS cert and skips the Dialer.Control denylist so they can hit a
	// loopback server.
	outboundClient func(timeout time.Duration) *http.Client

	// branding names this instance in the User-Agent of outbound fetches, so
	// the operator a site owner sees is the one actually making the request.
	// Zero value is fine — BotUserAgent falls back to a generic agent.
	branding branding.Config
}

// New creates a new media service.
func New(sqlStorage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage) *Service {
	return &Service{
		storage: sqlStorage,
		bucket:  bucketStorage,
		outboundClient: func(t time.Duration) *http.Client {
			return safehttp.NewClient(
				safehttp.WithTimeout(t),
				safehttp.WithRedirectSchemes("https"),
			)
		},
	}
}

// SetStockImageryProvider injects the stock-imagery provider used by
// AddMediaFromURL when the request carries a known provider id —
// candidates routed through this path bypass the generic URL body cap
// and preserve photographer attribution.
// SetBranding supplies the instance identity used in the outbound User-Agent.
func (s *Service) SetBranding(b branding.Config) {
	s.branding = b
}

func (s *Service) SetStockImageryProvider(p media.StockImageryProvider) {
	s.stockImageryProvider = p
}

// SetStockVideoProvider injects the stock-video provider used by
// AddMediaFromURL when the request carries a known provider id.
func (s *Service) SetStockVideoProvider(p media.StockVideoProvider) {
	s.stockVideoProvider = p
}

// AddMedia uploads media to bucket storage and inserts metadata into the database.
func (s *Service) AddMedia(
	ctx context.Context,
	req *connect.Request[api.AddMediaRequest],
) (*connect.Response[api.AddMediaResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(ctx, "adding media",
		"filename", req.Msg.Filename,
		"content_type", req.Msg.ContentType,
	)

	// Store media using shared helper
	mediaID, err := storage.StoreMedia(
		ctx,
		s.storage,
		s.bucket,
		authInfo.UserID,
		req.Msg.EncodedBytes,
		req.Msg.ContentType,
		req.Msg.Filename,
		req.Msg.Description,
		"",    // Not from stock imagery
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		if errors.Is(err, storage.ErrUnsupportedMediaType) ||
			errors.Is(err, storage.ErrMediaTooLarge) ||
			errors.Is(err, storage.ErrMediaContentMismatch) ||
			errors.Is(err, storage.ErrImageDimensionsExceeded) {
			logger.WarnContext(ctx, "rejected media upload",
				"error", err,
				"content_type", req.Msg.ContentType,
				"size_bytes", len(req.Msg.EncodedBytes),
			)
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		logger.ErrorContext(ctx, "failed to store media", "error", err)
		return nil, connecterr.Internal(ctx, "AddMedia", err, "detail", "failed to store media")
	}

	logger.InfoContext(ctx, "media added successfully", "media_id", mediaID)

	res := connect.NewResponse(&api.AddMediaResponse{
		Id: mediaID,
	})
	return res, nil
}

// AddMediaFromURL fetches a remote URL server-side and stores the
// resulting bytes as a new media row owned by the caller. Used by the
// Replace Media flow so the client can swap to a streamed candidate
// (Pexels alternate, webpage og:image alternate, etc.) without
// round-tripping bytes through the device.
//
// Two paths:
//
//   - Trusted-provider import (provider + provider_photo_id set): routes
//     through importViaTrustedProvider, which downloads via the
//     stock-imagery provider and creates a per-user copy with
//     Attribution populated from the canonical StockImage row.
//   - Generic-URL import (fallback): server-side fetch through the SSRF-
//     guarded outbound client (see safe_outbound.go), with size cap and
//     content-type allow-list ("image/*" or "video/*"). No Attribution
//     is recorded on this path — used for webpage og:image candidates
//     and any other untrusted URL the client surfaces.
//
// Validates: URL is https on every redirect hop, resolved IP is publicly
// routable (rejects loopback/link-local/RFC1918/CGNAT/metadata),
// response Content-Type matches the allow-list, body is within the cap.
// Per-user rate limit (10/min, burst 3) is enforced by the shared
// ratelimit interceptor via ratelimit.DefaultMediaRules.
func (s *Service) AddMediaFromURL(
	ctx context.Context,
	req *connect.Request[api.AddMediaFromURLRequest],
) (*connect.Response[api.AddMediaFromURLResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "AddMediaFromURL",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	rawURL := strings.TrimSpace(req.Msg.Url)
	if rawURL == "" {
		logger.WarnContext(ctx, "addmediafromurl rejected: empty url")
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("url is required"))
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		logger.WarnContext(ctx, "addmediafromurl rejected: parse failed", "url", rawURL, "error", err)
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid url: %w", err))
	}
	if parsed.Scheme != "https" {
		logger.WarnContext(ctx, "addmediafromurl rejected: non-https scheme",
			"url", rawURL, "scheme", parsed.Scheme)
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("url scheme must be https"))
	}

	// Trusted-provider path: when the request carries an explicit
	// stock-imagery provider id, route through the corresponding
	// provider helper. The provider performs the fetch with no body
	// cap (Pexels/Unsplash URLs are trusted) and populates Attribution
	// via the canonical StockImage row, so a Pexels swap shows the
	// photographer credit on the resulting media.
	if req.Msg.Provider != nil && req.Msg.ProviderPhotoId != nil &&
		*req.Msg.ProviderPhotoId != "" {
		if mediaID, handled, err := s.importViaTrustedProvider(
			ctx, logger, authInfo.UserID,
			*req.Msg.Provider, *req.Msg.ProviderPhotoId,
			req.Msg.Description,
		); handled {
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(&api.AddMediaFromURLResponse{Id: mediaID}), nil
		}
		// fall through to generic-URL path when the provider doesn't
		// support direct lookup (returns "unsupported" — generic path
		// still works as long as the URL is reachable + under the cap).
	}

	start := time.Now()
	logger.DebugContext(ctx, "fetching candidate url", "url", rawURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, connecterr.Internal(ctx, "AddMediaFromURL", err, "detail", "failed to build request")
	}
	httpReq.Header.Set("User-Agent", s.branding.BotUserAgent())
	httpReq.Header.Set("Accept", "image/*, video/*")

	clientFactory := s.outboundClient
	if clientFactory == nil {
		clientFactory = func(t time.Duration) *http.Client {
			return safehttp.NewClient(
				safehttp.WithTimeout(t),
				safehttp.WithRedirectSchemes("https"),
			)
		}
	}
	client := clientFactory(addMediaFromURLTimeout)
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.WarnContext(ctx, "candidate fetch failed",
			"url", rawURL, "error", err,
			"duration_ms", time.Since(start).Milliseconds())
		if safehttp.IsSSRFBlockError(err) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("url is not publicly routable"))
		}
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("failed to fetch url: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.WarnContext(ctx, "candidate fetch returned non-OK",
			"url", rawURL, "status", resp.StatusCode,
			"duration_ms", time.Since(start).Milliseconds())
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("upstream status %d", resp.StatusCode))
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if i := strings.Index(contentType, ";"); i >= 0 {
		contentType = strings.TrimSpace(contentType[:i])
	}
	if !strings.HasPrefix(contentType, "image/") && !strings.HasPrefix(contentType, "video/") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported content type %q", contentType))
	}

	limited := io.LimitReader(resp.Body, addMediaFromURLMaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, connecterr.Internal(ctx, "AddMediaFromURL", err, "detail", "failed to read body")
	}
	if int64(len(body)) > addMediaFromURLMaxBytes {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("body exceeds %d bytes", addMediaFromURLMaxBytes))
	}

	description := ""
	if req.Msg.Description != nil {
		description = *req.Msg.Description
	}
	mediaID, err := storage.StoreMedia(
		ctx, s.storage, s.bucket, authInfo.UserID,
		body, contentType,
		"candidate-import",
		description,
		// Attribution is only populated on the trusted-provider path
		// above; generic URL imports (no provider + id) have no source
		// stock image to attribute.
		"",
		false, // stripMetadata: retain EXIF (#1953)
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to store imported media", "error", err)
		return nil, connecterr.Internal(ctx, "AddMediaFromURL", err, "detail", "failed to store media")
	}

	logger.InfoContext(ctx, "imported media from url",
		"media_id", mediaID,
		"content_type", contentType,
		"size_bytes", len(body),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return connect.NewResponse(&api.AddMediaFromURLResponse{Id: mediaID}), nil
}

// importViaTrustedProvider routes a candidate import through the
// stock-imagery provider that produced it. The provider downloads the
// underlying media (image or video) without a body cap and returns a
// canonical StockImage with attribution populated; we then create a
// per-user copy so cascade-delete + attribution work correctly.
//
// Returns (mediaID, handled=true, nil) on success. Returns (_, true,
// err) when we attempted the trusted path but it errored — the caller
// must propagate that error. Returns (_, false, nil) when the provider
// is not configured or doesn't support direct lookup; the caller should
// fall back to the generic-URL path.
func (s *Service) importViaTrustedProvider(
	ctx context.Context,
	logger *logging.Logger,
	userID string,
	provider api.StockImageProvider,
	providerPhotoID string,
	description *string,
) (mediaID string, handled bool, err error) {
	// Translate the api enum to the models enum so we can compare.
	var modelProvider models.StockImageryProvider
	switch provider {
	case api.StockImageProvider_STOCK_IMAGE_PROVIDER_PEXELS:
		modelProvider = models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS
	case api.StockImageProvider_STOCK_IMAGE_PROVIDER_UNSPLASH:
		modelProvider = models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH
	default:
		return "", false, nil
	}

	// Strategy: try video first, then image. Video candidates carry
	// ids prefixed `video_*` in the StockImage table but the request
	// here passes the bare provider id — both StockImageryProvider and
	// StockVideoProvider know how to handle their own. We try video
	// first because it's the smaller surface (most candidates are
	// photos; videos are limited to Pexels today).
	desc := ""
	if description != nil {
		desc = *description
	}

	if s.stockVideoProvider != nil && modelProvider == models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS {
		stockImage, vErr := s.stockVideoProvider.GetStockVideoByID(ctx, providerPhotoID)
		if vErr == nil && stockImage != nil {
			id, copyErr := storage.CopyStockImageForUser(
				ctx, s.storage, s.bucket, stockImage, userID,
				"candidate-import.mp4",
				fallbackDescription(desc, stockImage),
			)
			if copyErr != nil {
				logger.ErrorContext(ctx, "failed to copy stock video for user",
					"error", copyErr, "stock_image_id", stockImage.Id)
				return "", true, connecterr.Internal(ctx, "AddMediaFromURL", copyErr,
					"detail", "failed to copy stock video")
			}
			logger.InfoContext(ctx, "imported via trusted provider (video)",
				"media_id", id, "provider", provider.String(),
				"provider_photo_id", providerPhotoID, "stock_image_id", stockImage.Id)
			return id, true, nil
		}
		// video provider didn't recognise this id (or doesn't support
		// by-id lookup); fall through to image attempt.
		logger.DebugContext(ctx, "stock video by-id miss, trying image",
			"provider_photo_id", providerPhotoID, "error", vErr)
	}

	if s.stockImageryProvider != nil {
		stockImage, iErr := s.stockImageryProvider.GetStockImageByID(ctx, providerPhotoID)
		if iErr == nil && stockImage != nil {
			id, copyErr := storage.CopyStockImageForUser(
				ctx, s.storage, s.bucket, stockImage, userID,
				"candidate-import.jpg",
				fallbackDescription(desc, stockImage),
			)
			if copyErr != nil {
				logger.ErrorContext(ctx, "failed to copy stock image for user",
					"error", copyErr, "stock_image_id", stockImage.Id)
				return "", true, connecterr.Internal(ctx, "AddMediaFromURL", copyErr,
					"detail", "failed to copy stock image")
			}
			logger.InfoContext(ctx, "imported via trusted provider (image)",
				"media_id", id, "provider", provider.String(),
				"provider_photo_id", providerPhotoID, "stock_image_id", stockImage.Id)
			return id, true, nil
		}
		logger.DebugContext(ctx, "stock image by-id miss, falling through to generic url",
			"provider_photo_id", providerPhotoID, "error", iErr)
	}

	// Neither provider supports by-id lookup for this candidate; caller
	// falls back to generic URL fetch (still subject to body cap).
	return "", false, nil
}

// fallbackDescription returns the explicit user-provided description
// when non-empty, otherwise a sensible default derived from the stock
// image's attribution.
func fallbackDescription(explicit string, stockImage *models.StockImage) string {
	if explicit != "" {
		return explicit
	}
	if stockImage != nil && stockImage.ProviderImage != nil && stockImage.ProviderImage.Description != "" {
		return stockImage.ProviderImage.Description
	}
	return "Imported from candidate"
}

// GetMedia retrieves media metadata and generates a presigned URL for access.
func (s *Service) GetMedia(
	ctx context.Context,
	req *connect.Request[api.GetMediaRequest],
) (*connect.Response[api.GetMediaResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetMedia",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"media_id", req.Msg.Id,
	)

	logger.DebugContext(ctx, "requesting media")

	// Fetch media metadata from SQL
	mediaRecord := &models.Media{}
	err = s.storage.GetByID(ctx, req.Msg.Id, mediaRecord)
	if err != nil {
		logger.InfoContext(ctx, "media not found", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// #1529: enforce the per-resource read rule. A caller with no
	// legitimate surface to this media (owner, system image, avatar,
	// community, story, gear, experience, request, chat, or transfer
	// participant) receives PermissionDenied — parity with DeleteMedia's
	// ownership check. We evaluate the predicate directly rather than via
	// the requireMediaReadAccess wrapper so the deny telemetry can name
	// the closest near-miss reason.
	allowed, reason, accessErr := canUserAccessMedia(ctx, s.storage, authInfo.UserID, mediaRecord)
	if accessErr != nil {
		// A probe failure is a system error, not an authorization
		// decision: fail closed via CodeInternal, never allow the read.
		return nil, connecterr.Internal(ctx, "GetMedia", accessErr,
			"detail", "media access check failed")
	}
	if !allowed {
		logger.WarnContext(ctx, "unauthorized read attempt",
			"phase", "enforce",
			"reason", reason,
			"media_owner_id", mediaRecord.UserId,
		)
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not authorized to access this media"))
	}
	logger.DebugContext(ctx, "media access allowed",
		"phase", "enforce",
		"reason", reason,
	)

	// presignedURLExpiry must exceed the client-side stash cache TTL (default CACHE_TTL_MINUTES = 30 min).
	// If the cache TTL is raised, raise this value too — otherwise cached GetMediaResponse entries will
	// contain expired URLs during the TTL window, causing 403 errors on image loads.
	const presignedURLExpiry = 35 * time.Minute

	// Generate presigned URL for bucket access.
	bucketKey := storage.MediaBucketKey(mediaRecord.UserId, mediaRecord.Id)
	presignedURL, err := s.bucket.GetSignedURL(ctx, bucketKey, presignedURLExpiry)
	if err != nil {
		logger.ErrorContext(ctx, "failed to generate presigned URL", "error", err)
		return nil, connecterr.Internal(ctx, "GetMedia", err, "detail", "failed to generate access URL")
	}

	resp := &api.GetMediaResponse{
		Id:               mediaRecord.Id,
		ContentType:      mediaRecord.ContentType,
		Url:              presignedURL,
		Filename:         mediaRecord.GetFilename(),
		Description:      mediaRecord.GetDescription(),
		CreatedAtUnixSec: mediaRecord.CreatedAtUnixSec,
	}

	// Generate presigned URL for thumbnail if it exists
	if mediaRecord.GetThumbnailStorageUrl() != "" {
		thumbnailKey := storage.ThumbnailBucketKey(mediaRecord.UserId, mediaRecord.Id)
		thumbnailURL, err := s.bucket.GetSignedURL(ctx, thumbnailKey, presignedURLExpiry)
		if err != nil {
			logger.ErrorContext(ctx, "failed to generate thumbnail presigned URL", "error", err)
			return nil, connecterr.Internal(ctx, "GetMedia", err, "detail", "failed to generate thumbnail URL")
		}
		resp.ThumbnailUrl = thumbnailURL
	}

	// Populate attribution if this media is derived from stock imagery
	if mediaRecord.GetSourceStockImageId() != "" {
		stockImage, err := media.GetStockImageByID(ctx, s.storage, mediaRecord.GetSourceStockImageId())
		if err != nil {
			logger.ErrorContext(ctx, "failed to get stock image for attribution", "source_stock_image_id", mediaRecord.SourceStockImageId, "error", err)
			return nil, connecterr.Internal(ctx, "GetMedia", err, "detail", "failed to get attribution")
		}
		resp.Attribution = buildAttribution(stockImage)
	}

	// Populate uploader for display in the chat media strip and carousel
	// attribution. Best-effort: if the user cannot be resolved (e.g. deleted
	// account) we leave the field absent rather than failing the request.
	uploaderUser := &models.User{}
	if err := s.storage.GetByID(ctx, mediaRecord.UserId, uploaderUser); err == nil {
		resp.Uploader = &api.User{
			Id:      mediaRecord.UserId,
			Name:    uploaderUser.Name,
			MediaId: services.PrimaryAvatarMediaID(uploaderUser),
		}
	} else {
		logger.DebugContext(ctx, "could not resolve uploader for media strip",
			"uploader_user_id", mediaRecord.UserId, "error", err)
	}

	logger.DebugContext(ctx, "media retrieved successfully",
		"filename", mediaRecord.Filename,
		"content_type", mediaRecord.ContentType,
	)

	res := connect.NewResponse(resp)
	return res, nil
}

// DeleteMedia deletes media from both bucket storage and the database.
func (s *Service) DeleteMedia(
	ctx context.Context,
	req *connect.Request[api.DeleteMediaRequest],
) (*connect.Response[api.DeleteMediaResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"media_id", req.Msg.Id,
	)

	logger.InfoContext(ctx, "deleting media")

	// Fetch media metadata from SQL to verify ownership
	media := &models.Media{}
	err = s.storage.GetByID(ctx, req.Msg.Id, media)
	if err != nil {
		logger.InfoContext(ctx, "media not found", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Verify that the requesting user owns this media
	if media.UserId != authInfo.UserID {
		logger.WarnContext(ctx, "unauthorized delete attempt", "media_owner_id", media.UserId)
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "media_delete_not_authorized", "you don't have permission to delete this media", nil)
	}

	// Note: Bucket storage files are kept for recovery. They can be cleaned up
	// later by a scheduled job that permanently deletes old soft-deleted items.

	// Set the deleted metadata (soft delete)
	media.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  authInfo.UserID,
		DeletedAtUnixSec: time.Now().Unix(),
	}
	if err := s.storage.Update(ctx, media); err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete media", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteMedia", err, "detail", "failed to delete media")
	}

	logger.InfoContext(ctx, "media deleted successfully")

	res := connect.NewResponse(&api.DeleteMediaResponse{})
	return res, nil
}

// Verify that Service implements the MediaServiceHandler interface.
var _ apiconnect.MediaServiceHandler = (*Service)(nil)

// buildAttribution creates an Attribution message from a StockImage.
func buildAttribution(stockImage *models.StockImage) *api.Attribution {
	if stockImage == nil || stockImage.ProviderImage == nil {
		return nil
	}

	attribution := &api.Attribution{
		OriginalUrl: stockImage.ProviderImage.Url,
	}

	// Map provider enum from models to API
	switch stockImage.Provider {
	case models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH:
		attribution.Provider = api.StockImageProvider_STOCK_IMAGE_PROVIDER_UNSPLASH
	case models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS:
		attribution.Provider = api.StockImageProvider_STOCK_IMAGE_PROVIDER_PEXELS
	default:
		attribution.Provider = api.StockImageProvider_STOCK_IMAGE_PROVIDER_UNSPECIFIED
	}

	// Populate creator info if available
	if stockImage.ProviderImage.Creator != nil {
		attribution.CreatorName = stockImage.ProviderImage.Creator.Name
		attribution.CreatorUsername = stockImage.ProviderImage.Creator.Username
		attribution.PhotographerUrl = stockImage.ProviderImage.Creator.PhotographerUrl
	}

	return attribution
}
