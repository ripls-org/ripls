package workshop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/known_for"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// HideKnownForCategory writes a non-permanent suppression row so the
// requested category stops surfacing on the requested scope. Returns
// the new row's id so the client can pass it back to UndoHide via
// the snackbar undo path.
func (s *Service) HideKnownForCategory(
	ctx context.Context,
	req *connect.Request[api.HideKnownForCategoryRequest],
) (*connect.Response[api.HideKnownForCategoryResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	scopeKind := storageScopeKind(req.Msg.ScopeKind)
	if err := s.authorizeKnownForSuppression(ctx, authInfo.UserID, scopeKind, req.Msg.ScopeId); err != nil {
		return nil, err
	}
	row, err := insertSuppressionRow(ctx, s.storage, authInfo.UserID, scopeKind, req.Msg.ScopeId, req.Msg.Category, false)
	if err != nil {
		return nil, connecterr.Internal(ctx, "HideKnownForCategory.Insert", err)
	}
	logging.LoggerWithContext(ctx).InfoContext(ctx, "known_for: category hidden",
		"operation", "HideKnownForCategory",
		"scope_kind", req.Msg.ScopeKind.String(),
		"scope_id", req.Msg.ScopeId,
		"category", req.Msg.Category,
		"suppressed_by_user_id", authInfo.UserID,
		"suppression_id", row.Id,
	)
	return connect.NewResponse(&api.HideKnownForCategoryResponse{
		SuppressionId: row.Id,
	}), nil
}

// UndoHideKnownForCategory deletes a suppression row by id. Caller
// must match the row's suppressed_by_user_id. Permanent rows refuse
// to be undone — that's the irreversible-by-design contract.
func (s *Service) UndoHideKnownForCategory(
	ctx context.Context,
	req *connect.Request[api.UndoHideKnownForCategoryRequest],
) (*connect.Response[api.UndoHideKnownForCategoryResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	row := &models.SuppressedKnownFor{}
	if err := s.storage.GetByID(ctx, req.Msg.SuppressionId, row); err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("suppression not found"))
		}
		return nil, connecterr.Internal(ctx, "UndoHideKnownForCategory.Get", err)
	}
	if row.SuppressedByUserId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not your suppression"))
	}
	if row.Permanent {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("permanent suppressions cannot be undone"))
	}
	if err := s.storage.Delete(ctx, row); err != nil {
		return nil, connecterr.Internal(ctx, "UndoHideKnownForCategory.Delete", err)
	}
	logging.LoggerWithContext(ctx).InfoContext(ctx, "known_for: hide undone",
		"operation", "UndoHideKnownForCategory",
		"suppression_id", row.Id,
		"scope_kind", row.ScopeKind.String(),
		"scope_id", row.ScopeId,
		"category", row.CategoryDisplay,
	)
	return connect.NewResponse(&api.UndoHideKnownForCategoryResponse{}), nil
}

// PermanentlyRemoveKnownForCategory writes a permanent suppression
// row. Client must surface a confirmation dialog at action time.
func (s *Service) PermanentlyRemoveKnownForCategory(
	ctx context.Context,
	req *connect.Request[api.PermanentlyRemoveKnownForCategoryRequest],
) (*connect.Response[api.PermanentlyRemoveKnownForCategoryResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	scopeKind := storageScopeKind(req.Msg.ScopeKind)
	if err := s.authorizeKnownForSuppression(ctx, authInfo.UserID, scopeKind, req.Msg.ScopeId); err != nil {
		return nil, err
	}
	row, err := insertSuppressionRow(ctx, s.storage, authInfo.UserID, scopeKind, req.Msg.ScopeId, req.Msg.Category, true)
	if err != nil {
		return nil, connecterr.Internal(ctx, "PermanentlyRemoveKnownForCategory.Insert", err)
	}
	logging.LoggerWithContext(ctx).InfoContext(ctx, "known_for: category permanently removed",
		"operation", "PermanentlyRemoveKnownForCategory",
		"scope_kind", req.Msg.ScopeKind.String(),
		"scope_id", req.Msg.ScopeId,
		"category", req.Msg.Category,
		"suppressed_by_user_id", authInfo.UserID,
		"suppression_id", row.Id,
	)
	return connect.NewResponse(&api.PermanentlyRemoveKnownForCategoryResponse{}), nil
}

// authorizeKnownForSuppression enforces the per-user / per-community
// permission split: per-user suppressions can only be written by the
// target user themselves; per-community suppressions require the
// caller to be an active member of the community.
func (s *Service) authorizeKnownForSuppression(
	ctx context.Context,
	callerID string,
	scopeKind models.SuppressionScope,
	scopeID string,
) error {
	if scopeID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("scope_id required"))
	}
	switch scopeKind {
	case models.SuppressionScope_SUPPRESSION_SCOPE_USER:
		if scopeID != callerID {
			return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("only the target may suppress their own skills"))
		}
		return nil
	case models.SuppressionScope_SUPPRESSION_SCOPE_COMMUNITY:
		active, _, err := auth.FilterActiveMemberCommunities(ctx, s.storage, []string{scopeID}, callerID)
		if err != nil {
			return err
		}
		if len(active) == 0 {
			return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "community_not_member", "you're not a member of this community", nil)
		}
		return nil
	default:
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("scope_kind required"))
	}
}

// insertSuppressionRow writes a new suppression row with the given
// category + permanent flag. Idempotent at the storage level via the
// (scope_kind, scope_id, category_key) tuple — re-hiding an already
// hidden row inserts a fresh id, which is fine; the suppression is
// already in effect.
func insertSuppressionRow(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	actorID string,
	scopeKind models.SuppressionScope,
	scopeID, category string,
	permanent bool,
) (*models.SuppressedKnownFor, error) {
	key := normalizeKey(category)
	display := strings.TrimSpace(category)
	row := &models.SuppressedKnownFor{
		Id:                  uuid.New().String(),
		ScopeKind:           scopeKind,
		ScopeId:             scopeID,
		CategoryKey:         key,
		CategoryDisplay:     display,
		Permanent:           permanent,
		SuppressedByUserId:  actorID,
		SuppressedAtUnixSec: time.Now().Unix(),
	}
	if _, err := s.Insert(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

// storageScopeKind maps the API enum onto the storage enum. Both
// carry the same semantic values; the indirection keeps the two
// packages from importing each other.
func storageScopeKind(s api.SuppressionScope) models.SuppressionScope {
	switch s {
	case api.SuppressionScope_SUPPRESSION_SCOPE_USER:
		return models.SuppressionScope_SUPPRESSION_SCOPE_USER
	case api.SuppressionScope_SUPPRESSION_SCOPE_COMMUNITY:
		return models.SuppressionScope_SUPPRESSION_SCOPE_COMMUNITY
	default:
		return models.SuppressionScope_SUPPRESSION_SCOPE_UNSPECIFIED
	}
}

// loadSuppressedKeysForScope returns the union of suppressed
// category keys for the requested scope: for per-user requests the
// target's own rows; for per-community requests the union across
// every community.
func loadSuppressedKeysForScope(
	ctx context.Context,
	s *Service,
	mode known_for.Mode,
	ownerID string,
	communityIDs []string,
) (map[string]struct{}, error) {
	if mode == known_for.ModePerUser {
		return known_for.LoadSuppressedKeysForUser(ctx, s.storage, ownerID)
	}
	perCommunity, err := known_for.LoadSuppressedKeysForCommunities(ctx, s.storage, communityIDs)
	if err != nil {
		return nil, err
	}
	return known_for.CollapseCommunitySuppressions(perCommunity), nil
}

// normalizeKey mirrors the normalization the known_for package
// applies to category strings. Duplicated here to keep the package
// boundary clean — adding a public helper to known_for just for one
// caller would be more coupling than the function deserves.
func normalizeKey(raw string) string {
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	prevSpace := false
	for _, r := range lower {
		if unicode.IsSpace(r) {
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimRight(b.String(), " ")
}
