package login

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// RefreshToken validates a refresh token and issues a new access token and rotated refresh token.
func (s *Service) RefreshToken(
	ctx context.Context,
	req *connect.Request[api.RefreshTokenRequest],
) (*connect.Response[api.RefreshTokenResponse], error) {
	if req.Msg.RefreshToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("refresh_token is required"))
	}

	tokenHash := auth.HashRefreshToken(req.Msg.RefreshToken)

	tokens, err := s.storage.QueryByField(ctx, "token_hash", tokenHash, &models.RefreshToken{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "RefreshToken", fmt.Errorf("failed to validate refresh token"))
	}

	if len(tokens) == 0 {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "authentication_failed",
			"reason", "refresh_token_unknown",
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("invalid refresh token"))
	}

	refreshToken := tokens[0].(*models.RefreshToken)

	// Reuse of a revoked token is a theft signal — revoke all tokens for the user.
	if refreshToken.IsRevoked {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "revoked refresh token reuse detected, revoking all user tokens",
			"user_id", refreshToken.UserId,
		)
		s.revokeAllUserRefreshTokens(ctx, refreshToken.UserId)
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("invalid refresh token"))
	}

	if time.Now().Unix() > refreshToken.ExpiresAtUnixSec {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "authentication_failed",
			"user_id", refreshToken.UserId,
			"reason", "refresh_token_expired",
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("refresh token expired"))
	}

	user := &models.User{}
	if err := s.storage.GetByID(ctx, refreshToken.UserId, user); err != nil {
		return nil, connecterr.Internal(ctx, "RefreshToken", fmt.Errorf("failed to look up user"))
	}

	// Generate new access token. The phone number must be passed so
	// phone-only users (Email == "") get a token with a non-empty
	// identity claim — token validation requires at least one of email
	// or phone_number alongside the user_id.
	accessToken, err := s.authTokenConfig.GenerateToken(
		user.Id, user.Email, user.Role, user.GetPhoneNumber(),
	)
	if err != nil {
		return nil, connecterr.Internal(ctx, "RefreshToken", fmt.Errorf("failed to generate access token"))
	}

	// Revoke the old refresh token (rotation).
	refreshToken.IsRevoked = true
	if err := s.storage.Update(ctx, refreshToken); err != nil {
		return nil, connecterr.Internal(ctx, "RefreshToken", fmt.Errorf("failed to revoke old refresh token"))
	}

	// Issue a new refresh token.
	newRawToken, err := s.createRefreshToken(ctx, user.Id, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_ROTATION)
	if err != nil {
		return nil, connecterr.Internal(ctx, "RefreshToken", fmt.Errorf("failed to create new refresh token"))
	}

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "token refreshed",
		"user_id", user.Id,
		"user_email", logging.MaskEmail(user.Email),
	)

	return connect.NewResponse(&api.RefreshTokenResponse{
		Tokens: &api.Tokens{AccessToken: accessToken, RefreshToken: newRawToken},
	}), nil
}

// createRefreshToken generates a new refresh token, stores its hash, and returns
// the raw token string to be sent to the client. origin records whether the
// token was minted by an interactive sign-in or by rotation, so metrics can tell
// people from credential refresh.
func (s *Service) createRefreshToken(ctx context.Context, userID string, origin models.RefreshTokenOrigin) (rawToken string, err error) {
	rawToken, err = auth.GenerateRefreshToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	now := time.Now().Unix()
	stored := &models.RefreshToken{
		Id:               uuid.New().String(),
		UserId:           userID,
		TokenHash:        auth.HashRefreshToken(rawToken),
		ExpiresAtUnixSec: now + int64(s.refreshTokenExpiration.Seconds()),
		CreatedAtUnixSec: now,
		Origin:           origin,
		IsRevoked:        false,
	}

	if _, err := s.storage.Insert(ctx, stored); err != nil {
		return "", fmt.Errorf("failed to store refresh token: %w", err)
	}

	return rawToken, nil
}

// revokeAllUserRefreshTokens revokes all active refresh tokens for a user.
// Called when token theft is detected (reuse of a revoked token).
func (s *Service) revokeAllUserRefreshTokens(ctx context.Context, userID string) {
	logger := logging.LoggerWithContext(ctx)

	tokens, err := s.storage.QueryByField(ctx, "user_id", userID, &models.RefreshToken{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query user refresh tokens for revocation",
			"user_id", userID,
			"error", err,
		)
		return
	}

	for _, t := range tokens {
		rt := t.(*models.RefreshToken)
		if rt.IsRevoked {
			continue
		}
		rt.IsRevoked = true
		if err := s.storage.Update(ctx, rt); err != nil {
			logger.ErrorContext(ctx, "failed to revoke refresh token",
				"user_id", userID,
				"token_id", rt.Id,
				"error", err,
			)
		}
	}
}
