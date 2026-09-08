package request

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// fetchAndAttachStockImage fetches a stock image and creates a copy for the request.
// This runs asynchronously and updates the request's media_ids when complete.
// Each request gets its own copy of the image to support cascade deletion.
func (s *Service) fetchAndAttachStockImage(ctx context.Context, requestID, communityID, description, userID string) {
	logger := logging.LoggerWithContext(ctx).With(
		"service", "request",
		"operation", "fetchAndAttachStockImage",
		"target_request_id", requestID,
		"community_id", communityID,
	)
	startTime := time.Now()

	// Signal completion when done (for testing)
	defer func() {
		if s.stockImageryDone != nil {
			s.stockImageryDone <- struct{}{}
		}
	}()

	logger.InfoContext(ctx, "fetching stock image for request")

	// Get stock image from provider. Read-only external call — no DB side
	// effect, so we let it run unconditionally and gate just before the
	// first side effect (the GCS copy + Media row insert below).
	stockImage, err := s.stockImageryProvider.GetStockImage(ctx, description, nil)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get stock image",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return
	}

	// Soft-delete gate: positioned between the read-only Pexels/Unsplash
	// fetch and the first side-effecting step (CopyStockImageForUser
	// creates a Media row + GCS object). Skipping here avoids the orphan
	// Media row that would result if the gate fired after the copy. The
	// remaining race window — community deleted DURING the GCS copy — is
	// small; any orphan from that window is user-scoped (not
	// community-scoped) and swept by the day-30 purge job (#1620). See
	// #1623.
	if communityID != "" && !community.IsActive(ctx, s.storage, communityID) {
		logger.InfoContext(ctx, "skipping stock-image copy + attach — community is soft-deleted",
			"reason", "community_deleted_during_job",
			"duration_ms", time.Since(startTime).Milliseconds())
		return
	}

	// Create a copy for this request (enables cascade deletion and attribution tracking)
	mediaID, err := storage.CopyStockImageForUser(
		ctx,
		s.storage,
		s.bucket,
		stockImage,
		userID,
		fmt.Sprintf("request-%s.jpg", requestID),
		fmt.Sprintf("Stock image for request: %s", description),
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to copy stock image",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds())
		return
	}

	// Block here for deterministic race testing: test receives to release the goroutine
	// into WithTx, allowing it to call CancelRequest/UpdateRequest first. Nil in production.
	if s.stockImageryBeforeWrite != nil {
		s.stockImageryBeforeWrite <- struct{}{}
	}

	// Reload the request under a row lock and attach the media ID atomically.
	// The SELECT FOR UPDATE ensures a concurrent CancelRequest/UpdateRequest
	// cannot commit between our reload and our write: it blocks until this tx
	// commits, and it sees the merged state rather than a stale snapshot.
	// See WithTx godoc → "Locked reload" and #2900.
	if err := s.storage.WithTx(ctx, nil, func(tx *storage.ProtoSQLStorage) error {
		fresh := &models.Request{}
		if err := tx.GetByID(ctx, requestID, fresh, storage.QueryOptions{IncludeDeleted: true, ForUpdate: true}); err != nil {
			return err
		}
		if fresh.Deleted != nil {
			logger.InfoContext(ctx, "skipping stock-image attach — request was soft-deleted",
				"reason", "request_deleted_during_job",
				"duration_ms", time.Since(startTime).Milliseconds())
			return nil
		}
		fresh.MediaIds = []string{mediaID}
		return tx.Update(ctx, fresh)
	}); err != nil {
		logger.WarnContext(ctx, "failed to attach stock image to request",
			"operation", "fetchAndAttachStockImage",
			"target_request_id", requestID,
			"duration_ms", time.Since(startTime).Milliseconds(),
			"error", err)
		return
	}

	logger.InfoContext(ctx, "successfully attached stock image to request",
		"media_id", mediaID,
		"stock_image_id", stockImage.Id,
		"duration_ms", time.Since(startTime).Milliseconds())
}
