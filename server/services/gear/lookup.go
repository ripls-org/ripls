package gear

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// fetchGearForModify loads a gear by ID for a write path (delete, update,
// state change) and distinguishes "soft-deleted" from "truly missing" for
// logging purposes.
//
// Clients can attempt to modify a gear the owner has already soft-deleted:
// a stale detail screen, a multi-device deletion race, or a batch selection
// that includes an item deleted on another device (see #2711). Any of those
// hits DeleteGear/etc. with a gear_id whose row is soft-deleted, which
// logged at ERROR pages oncall for expected client behavior.
//
// Behavior:
//   - Live gear: returned to the caller, no log.
//   - Soft-deleted: logs WARN with reason="soft_deleted" and returns
//     connect.CodeNotFound. Callers should treat this identically to a hard
//     miss; we just don't wake oncall on stale-client deletes.
//   - Truly missing or storage error: logs ERROR and returns
//     connect.CodeNotFound (missing) or an Internal error (other storage
//     failure). A hard miss for an id a client referenced is a real bug
//     (bad cache, referential integrity gap) and stays loud — same doctrine
//     as fetchExperienceForRead (#1701, #1184).
func (s *Service) fetchGearForModify(
	ctx context.Context,
	id string,
	logger *slog.Logger,
	operation string,
) (*models.Gear, error) {
	gear := &models.Gear{}
	err := s.storage.GetByID(ctx, id, gear, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.ErrorContext(ctx, "failed to get gear", "error", err)
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
		}
		logger.ErrorContext(ctx, "failed to get gear", "error", err)
		return nil, connecterr.Internal(ctx, operation, err)
	}
	if gear.Deleted != nil {
		logger.WarnContext(ctx, "gear is soft-deleted",
			"reason", "soft_deleted",
			"deleted_by_user_id", gear.Deleted.DeletedByUserId,
			"deleted_at_unix_sec", gear.Deleted.DeletedAtUnixSec,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}
	return gear, nil
}
