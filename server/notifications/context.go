package notifications

import "context"

type contextKey string

const communityEventIDKey contextKey = "community_event_id"

// WithCommunityEventID attaches a community event ID to the context so it propagates
// through the notification dispatch pipeline (service.go, fcm/provider.go) for log
// correlation. Called by community_subscriber before invoking NotifyUser.
func WithCommunityEventID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, communityEventIDKey, id)
}

// CommunityEventIDFromContext retrieves the community event ID attached by
// WithCommunityEventID, or empty string if not set.
func CommunityEventIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(communityEventIDKey).(string); ok {
		return id
	}
	return ""
}
