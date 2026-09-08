// Per-user realtime event streaming (#2867).
//
// This used to be one connection per community, which was sized for the "users
// belong to 1-5 communities" world the design was written in. The phone-first
// pivot (#2492) made every item spawn a per-item community, so membership — and
// therefore connection count — grows with item count. A browser caps concurrent
// connections per origin, so past a handful of communities the streams consumed
// every slot and ordinary unary RPCs were never sent at all. The per-community
// RPC was deprecated by #2867 and deleted by #2869.
//
// One stream per user replaces it. The subscription carries no community:
// membership is resolved server-side, freshly, on every event (see
// stream_subscriber.go), which is what makes joining or leaving a community
// take effect with no client action and no reconnect.
//
// Two behaviors deliberately differ from the per-community stream that came
// before, and both follow from the subscription naming no community:
//
//   - COMMUNITY_DELETED does not terminate the stream. On a per-community
//     stream it was terminal because nothing else would ever arrive on that
//     subscription. Here it is one community among many, so ending the stream
//     would take every other community's realtime with it; the event is
//     forwarded like any other and the client reacts to it (event_router.dart).
//   - There is no membership gate at open time. A per-community stream had to
//     prove membership of the community it named; this one names none, and
//     every event is filtered through live membership before delivery.

package community

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// userCatchUpLimit bounds a single catch-up replay. A dormant user returning
// after a week across a large portfolio could otherwise replay tens of
// thousands of rows into a stream the client will process one at a time.
// Events are invalidation signals rather than state, so a truncated replay
// costs a redundant refresh, not correctness.
const userCatchUpLimit = 500

type userStreamInfo struct {
	userID string
	ch     chan *api.StreamUserEventsResponse
}

// StreamUserEvents streams every event across the caller's communities on one
// connection. See the file comment for why this supersedes the per-community
// stream.
func (s *Service) StreamUserEvents(
	ctx context.Context,
	req *connect.Request[api.StreamUserEventsRequest],
	stream *connect.ServerStream[api.StreamUserEventsResponse],
) error {
	streamStartTime := time.Now()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"operation", "StreamUserEvents",
	)
	logger.InfoContext(ctx, "user event stream started")

	// Register before the catch-up query, closing the window where an event
	// could fire between the query and registration and be lost by both paths.
	eventChan := make(chan *api.StreamUserEventsResponse, 10)
	s.registerUserStream(ctx, authInfo.UserID, eventChan)
	// Deliberately NOT closed on unregister: broadcastToUserStreams snapshots
	// the subscriber slice under RLock and sends outside the lock, so an
	// in-flight broadcaster can still hold this channel after unregister
	// returns. Closing would risk a send-on-closed-channel panic. The
	// broadcaster's send timeout bounds the hang, and the channel becomes
	// unreachable once every broadcaster drops its snapshot.
	defer s.unregisterUserStream(ctx, authInfo.UserID, eventChan)

	sentIDs := make(map[string]bool)
	if req.Msg.SinceUnixSec != nil {
		if err := s.sendUserCatchUp(ctx, stream, logger, authInfo.UserID, *req.Msg.SinceUnixSec, sentIDs, streamStartTime); err != nil {
			return err
		}
	}

	return s.runUserStreamLoop(ctx, stream, eventChan, sentIDs, logger, streamStartTime)
}

// sendUserCatchUp replays events across the caller's communities that occurred
// after sinceUnixSec, capped at 7 days and userCatchUpLimit rows, recording
// what it sent in sentIDs so the live loop does not repeat them.
func (s *Service) sendUserCatchUp(
	ctx context.Context,
	sender userStreamSender,
	logger *logging.Logger,
	userID string,
	sinceUnixSec int64,
	sentIDs map[string]bool,
	streamStartTime time.Time,
) error {
	// Cap the backlog so a long-dormant client cannot ask for an unbounded
	// replay. Same horizon as the two listings.
	sinceUnixSec = max(sinceUnixSec, sevenDaysAgoUnixSec())

	events, err := s.storage.FindEventsForUserSince(ctx, userID, sinceUnixSec, userCatchUpLimit)
	if err != nil {
		return connecterr.Internal(ctx, "StreamUserEvents.FindEventsForUserSince", err)
	}
	if len(events) == userCatchUpLimit {
		// Worth a log line: the client's local state is now provably behind
		// what it was replayed, and its poll backstop is what recovers the gap.
		logger.InfoContext(ctx, "catch-up replay truncated at limit",
			"limit", userCatchUpLimit,
			"since_unix_sec", sinceUnixSec,
		)
	}

	for _, event := range events {
		item, err := s.communityEventToItem(ctx, event)
		if err != nil {
			logger.WarnContext(ctx, "skipping event during catch-up", "community_event_id", event.Id, "error", err)
			continue
		}
		resp := &api.StreamUserEventsResponse{
			Payload: &api.StreamUserEventsResponse_Event{Event: item},
		}
		if err := sender.Send(resp); err != nil {
			logger.WarnContext(ctx, "user event stream ended during catch-up send",
				"error", err,
				"community_event_id", event.Id,
				"duration_ms", time.Since(streamStartTime).Milliseconds(),
			)
			return err
		}
		sentIDs[event.Id] = true
	}
	return nil
}

// userStreamSender is satisfied by *connect.ServerStream[StreamUserEventsResponse]
// and by test fakes, so the loop can be exercised without standing up a Connect
// HTTP handler. Mirrors streamSender in streaming.go.
type userStreamSender interface {
	Send(resp *api.StreamUserEventsResponse) error
}

// runUserStreamLoop is the lifetime-cap + heartbeat + delivery select. It
// mirrors runStreamLoop with one deliberate difference: no event type is
// terminal here, because the stream is not scoped to the community an event
// belongs to.
func (s *Service) runUserStreamLoop(
	ctx context.Context,
	sender userStreamSender,
	eventChan <-chan *api.StreamUserEventsResponse,
	sentIDs map[string]bool,
	logger *logging.Logger,
	streamStartTime time.Time,
) error {
	// Self-lifetime below Cloud Run's 900s frontend timeout so the stream ends
	// with a retryable EOF rather than a 504.
	streamCtx, cancel := context.WithTimeout(ctx, s.streamLifetime())
	defer cancel()

	ticker := time.NewTicker(s.streamHeartbeat())
	defer ticker.Stop()

	heartbeatResp := &api.StreamUserEventsResponse{
		Payload: &api.StreamUserEventsResponse_Heartbeat{Heartbeat: &api.Heartbeat{}},
	}

	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "user event stream ended normally",
				"reason", "context_done",
				"context_error", ctx.Err(),
				"duration_ms", time.Since(streamStartTime).Milliseconds(),
			)
			return nil
		case <-streamCtx.Done():
			logger.InfoContext(ctx, "user event stream ended at lifetime cap",
				"reason", "lifetime_cap",
				"duration_ms", time.Since(streamStartTime).Milliseconds(),
			)
			return nil
		case <-ticker.C:
			if err := sender.Send(heartbeatResp); err != nil {
				logger.InfoContext(ctx, "user event stream ended on heartbeat send",
					"error", err,
					"duration_ms", time.Since(streamStartTime).Milliseconds(),
				)
				return err
			}
		case resp := <-eventChan:
			item := resp.GetEvent()
			if item == nil {
				continue
			}
			if sentIDs[item.Id] {
				continue // Already sent during catch-up.
			}
			logger.DebugContext(ctx, "streaming event to client",
				"community_event_id", item.Id,
				"community_id", item.CommunityId,
				"event_type", item.EventType.String(),
			)
			if err := sender.Send(resp); err != nil {
				logger.WarnContext(ctx, "user event stream ended with send error",
					"error", err,
					"community_event_id", item.Id,
					"duration_ms", time.Since(streamStartTime).Milliseconds(),
				)
				return err
			}
		}
	}
}

// registerUserStream adds a channel to the per-user stream registry. ctx is the
// handler's request context, so register/unregister lines share the request_id
// that scopes every other line the stream emits.
func (s *Service) registerUserStream(ctx context.Context, userID string, ch chan *api.StreamUserEventsResponse) {
	s.userStreamsMu.Lock()
	defer s.userStreamsMu.Unlock()

	s.userStreams[userID] = append(s.userStreams[userID], userStreamInfo{userID: userID, ch: ch})

	logging.LoggerWithContext(ctx).InfoContext(ctx, "registered user event stream",
		"user_id", userID,
		"user_stream_count", len(s.userStreams[userID]),
	)
}

// unregisterUserStream removes a channel from the per-user stream registry.
func (s *Service) unregisterUserStream(ctx context.Context, userID string, ch chan *api.StreamUserEventsResponse) {
	s.userStreamsMu.Lock()
	defer s.userStreamsMu.Unlock()

	streams := s.userStreams[userID]
	for i, stream := range streams {
		if stream.ch == ch {
			s.userStreams[userID] = append(streams[:i], streams[i+1:]...)
			logging.LoggerWithContext(ctx).InfoContext(ctx, "unregistered user event stream",
				"user_id", userID,
				"remaining_user_streams", len(s.userStreams[userID]),
			)
			break
		}
	}

	if len(s.userStreams[userID]) == 0 {
		delete(s.userStreams, userID)
	}
}

// hasAnyUserStream reports whether any per-user stream is currently open. The
// bus fan-out consults this first so that, with no user streams connected, an
// event costs nothing beyond a read-locked map length check — in particular it
// does not resolve community membership.
func (s *Service) hasAnyUserStream() bool {
	s.userStreamsMu.RLock()
	defer s.userStreamsMu.RUnlock()
	return len(s.userStreams) > 0
}

// HasActiveUserStream reports whether the user has an open per-user stream.
// Used for push suppression, alongside the per-community check.
func (s *Service) HasActiveUserStream(userID string) bool {
	s.userStreamsMu.RLock()
	defer s.userStreamsMu.RUnlock()
	return len(s.userStreams[userID]) > 0
}

// broadcastToUserStreams delivers an event to the per-user streams of the given
// recipients — including the attributed actor's own. The actor is deliberately
// not skipped (#2702): bus-derived events are attributed to a user whose screen
// did not initiate them, so skipping them left their open screen stale for up to
// a poll interval. Self-delivery is a cheap idempotent refresh the client's
// event router dedups by id.
func (s *Service) broadcastToUserStreams(ctx context.Context, recipientIDs []string, item *api.CommunityEventItem) {
	if len(recipientIDs) == 0 {
		return
	}

	s.userStreamsMu.RLock()
	var targets []userStreamInfo
	for _, userID := range recipientIDs {
		targets = append(targets, s.userStreams[userID]...)
	}
	s.userStreamsMu.RUnlock()

	if len(targets) == 0 {
		return
	}

	logger := logging.LoggerWithContext(ctx).With(
		"community_id", item.CommunityId,
		"community_event_id", item.Id,
		"event_type", item.EventType.String(),
		"user_stream_count", len(targets),
	)
	logger.DebugContext(ctx, "broadcasting event to user streams")

	resp := &api.StreamUserEventsResponse{
		Payload: &api.StreamUserEventsResponse_Event{Event: item},
	}
	for _, target := range targets {
		select {
		case target.ch <- proto.Clone(resp).(*api.StreamUserEventsResponse):
			// Sent.
		case <-time.After(1 * time.Second):
			// With heartbeats a dead subscriber is reaped promptly, so a send
			// timeout is transient and self-healing rather than alertable.
			logger.WarnContext(ctx, "timeout sending event to user stream",
				"recipient_user_id", target.userID,
			)
		}
	}
}
