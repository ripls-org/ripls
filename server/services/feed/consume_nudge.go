package feed

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// ConsumeNudge marks a nudge as consumed when the user taps its CTA.
//
// This is designed for fire-and-forget use: the client navigates immediately
// after tapping and calls this RPC asynchronously. If the call fails, the
// nudge may reappear on the next feed load — an acceptable trade-off for
// smooth UX.
func (s *Service) ConsumeNudge(
	ctx context.Context,
	req *connect.Request[api.ConsumeNudgeRequest],
) (*connect.Response[api.ConsumeNudgeResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ConsumeNudge",
		"user_id", authInfo.UserID,
		"nudge_id", req.Msg.NudgeId,
		"cta_action", req.Msg.Action,
	)

	startTime := time.Now()

	if err := markNudgeConsumed(ctx, s.sqlStorage, req.Msg.NudgeId, req.Msg.Action); err != nil {
		logger.ErrorContext(ctx, "failed to mark nudge consumed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, connecterr.Internal(ctx, "ConsumeNudge", err)
	}

	logger.InfoContext(ctx, "nudge consumed",
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return connect.NewResponse(&api.ConsumeNudgeResponse{}), nil
}
