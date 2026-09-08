package storage

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/logging"
)

// DropUserMediaIDColumn idempotently drops the legacy User.media_id
// SQL column. The column was a flat denormalization of proto field 13,
// which #2083 marked as `reserved`. Without dropping the
// column the data sits as harmless dead weight; this helper finishes
// the deprecation by removing it.
//
// Dropping the column also auto-drops the `idx_user_media_id` partial
// index (Postgres drops any index that depends on a dropped column).
//
// Must run AFTER the proto field removal in #2083 is fully deployed
// — a server still running pre-removal code would try to write to this
// column and fail. See docs/issues/2083-avatar-media-id-deprecation.md
// for the rollout sequencing.
//
// Idempotent: subsequent runs see the column absent and no-op silently.
func (s *ProtoSQLStorage) DropUserMediaIDColumn(ctx context.Context) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DropUserMediaIDColumn",
		"table", "user",
	)
	exists, err := s.columnExists(ctx, "user", "media_id")
	if err != nil {
		return fmt.Errorf("check user.media_id existence: %w", err)
	}
	if !exists {
		logger.DebugContext(ctx, "media_id column already dropped")
		return nil
	}
	logger.InfoContext(ctx, "dropping legacy media_id column")
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE "user" DROP COLUMN IF EXISTS media_id`); err != nil {
		return fmt.Errorf("drop user.media_id: %w", err)
	}
	logger.InfoContext(ctx, "media_id column dropped")
	return nil
}
