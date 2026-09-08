package auth

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/logging"
)

// RequireAuth extracts authentication info from context and returns an error if not authenticated.
// This is a convenience function for RPC handlers that require authentication.
func RequireAuth(ctx context.Context) (*Info, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RequireAuth",
	)

	authInfo, ok := GetAuthInfo(ctx)
	if !ok {
		logger.WarnContext(ctx, "authentication required but not present")
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	logger.DebugContext(ctx, "authentication verified",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)
	return authInfo, nil
}
