package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// UpdatePresence updates the user's app presence status (foreground/background).
// This is used to determine when to send push notifications vs. relying on real-time streams.
func (s *Service) UpdatePresence(
	ctx context.Context,
	req *connect.Request[api.UpdatePresenceRequest],
) (*connect.Response[api.UpdatePresenceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Validate status is specified
	if req.Msg.Status == api.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("presence status must be specified"))
	}

	// Update presence state
	isInForeground := req.Msg.Status == api.PresenceStatus_PRESENCE_STATUS_FOREGROUND

	s.presenceMu.Lock()
	s.userInForeground[authInfo.UserID] = isInForeground
	s.presenceMu.Unlock()

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
	)

	statusStr := "BACKGROUND"
	if isInForeground {
		statusStr = "FOREGROUND"
	}
	logger.Info("updated presence", "status", statusStr)

	return connect.NewResponse(&api.UpdatePresenceResponse{}), nil
}

// isUserInForeground checks if a user's app is currently in the foreground.
func (s *Service) isUserInForeground(userID string) bool {
	s.presenceMu.RLock()
	defer s.presenceMu.RUnlock()

	isInForeground, exists := s.userInForeground[userID]
	// If presence not explicitly set, assume background (conservative approach for notifications)
	if !exists {
		return false
	}

	return isInForeground
}
