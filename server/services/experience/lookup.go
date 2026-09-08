package experience

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

// fetchExperienceForRead loads an experience by ID and distinguishes
// "soft-deleted" from "truly missing" for logging purposes.
//
// Clients can hold stale references to an experience long after its owner
// soft-deletes it: Stories and CommunityEvent rows that point at the
// experience are deliberately preserved as activity-log history (see #1698
// and server/services/experience/delete.go). Tapping such a story triggers
// a GetExperience or GetExperienceStats for a soft-deleted row, which is
// expected — not a server bug. Logging those at ERROR makes the alerting
// policy "Server Error Logged" page on normal user behavior.
//
// Behavior:
//   - Live experience: returned to the caller, no log.
//   - Soft-deleted: logs WARN with reason="soft_deleted" and returns
//     connect.CodeNotFound. Callers should treat this identically to a hard
//     miss; we just don't want to wake oncall.
//   - Truly missing or storage error: logs ERROR and returns
//     connect.CodeNotFound (missing) or an Internal error (other storage
//     failure). A hard miss for an ID a client referenced is a real bug
//     (broken story payload, bad cache) and stays loud.
func (s *Service) fetchExperienceForRead(
	ctx context.Context,
	id string,
	logger *slog.Logger,
	operation string,
) (*models.Experience, error) {
	exp := &models.Experience{}
	err := s.storage.GetByID(ctx, id, exp, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.ErrorContext(ctx, "failed to get experience", "error", err)
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("experience not found"))
		}
		logger.ErrorContext(ctx, "failed to get experience", "error", err)
		return nil, connecterr.Internal(ctx, operation, err)
	}
	if exp.Deleted != nil {
		logger.WarnContext(ctx, "experience is soft-deleted",
			"reason", "soft_deleted",
			"deleted_by_user_id", exp.Deleted.DeletedByUserId,
			"deleted_at_unix_sec", exp.Deleted.DeletedAtUnixSec,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("experience not found"))
	}
	return exp, nil
}
