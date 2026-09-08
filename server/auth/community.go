package auth

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// IsMemberOfCommunity checks if a user is a member of a community.
// Returns true if the user is a member, false otherwise.
func IsMemberOfCommunity(ctx context.Context, s *storage.ProtoSQLStorage, communityID, userID string) (bool, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "IsMemberOfCommunity",
		"community_id", communityID,
		"user_id", userID,
	)

	messages, err := s.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"user_id":      userID,
	}, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community membership",
			"error", err,
		)
		return false, fmt.Errorf("failed to query membership: %w", err)
	}

	isMember := len(messages) > 0
	logger.DebugContext(ctx, "membership check complete",
		"is_member", isMember,
	)
	return isMember, nil
}

// GetCommunityUser retrieves a user's membership record for a specific community.
// Returns the membership record if found, nil otherwise.
func GetCommunityUser(ctx context.Context, s *storage.ProtoSQLStorage, communityID, userID string) (*models.CommunityUser, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetCommunityUser",
		"community_id", communityID,
		"user_id", userID,
	)

	messages, err := s.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"user_id":      userID,
	}, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community membership",
			"error", err,
		)
		return nil, fmt.Errorf("failed to query membership: %w", err)
	}

	if len(messages) == 0 {
		logger.DebugContext(ctx, "community membership not found")
		return nil, nil
	}

	logger.DebugContext(ctx, "community membership retrieved")
	return messages[0].(*models.CommunityUser), nil
}

// RequireUserIsCommunityMember checks if a user is a member of a community and returns an error if not.
func RequireUserIsCommunityMember(ctx context.Context, s *storage.ProtoSQLStorage, communityID, userID, errorMsg string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RequireUserIsCommunityMember",
		"community_id", communityID,
		"user_id", userID,
	)

	isMember, err := IsMemberOfCommunity(ctx, s, communityID, userID)
	if err != nil {
		logger.ErrorContext(ctx, "membership check failed",
			"error", err,
		)
		return connecterr.Internal(ctx, "RequireUserIsCommunityMember", err)
	}

	if !isMember {
		logger.WarnContext(ctx, "authorization failed: user not a community member")
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("%s", errorMsg))
	}

	logger.DebugContext(ctx, "authorization successful: user is community member")
	return nil
}
