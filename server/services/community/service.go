package community

import (
	"context"
	"sync"
	"time"

	"go.ripls.org/ripls/server/ai"
	chatlib "go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the CommunityService RPC interface.
type Service struct {
	storage              *storage.ProtoSQLStorage
	bucket               storage.BucketStorage
	notificationService  notifications.Service
	stockImageryProvider media.StockImageryProvider
	aiProvider           ai.Provider
	systemMessageWriter  *chatlib.SystemMessageWriter
	bus                  cebus.Publisher // CommunityEvent publisher; required (#510 PR 3).
	notificationDone     chan struct{}   // For testing: signals when async notifications complete (nil in production)
	inviteLinkHostname   string          // Hostname for invitation links (e.g., "ripls.app" or "dev.ripls.app")

	// itemSharers fan an item out to communities for ShareItem, keyed by item
	// kind. Injected via SetItemSharer so the community service doesn't import
	// the item-service packages.
	itemSharers map[ItemKind]ItemSharer

	// inviteEmailSender sends off-app invite emails for email invitees
	// (EMAIL-1 channel). Optional; nil disables platform email invites.
	inviteEmailSender InviteEmailSender

	// Per-user event stream registry (#2867). One entry per connected client
	// rather than one per (client, community), so a user's connection count no
	// longer scales with how many communities they belong to.
	userStreamsMu sync.RWMutex
	userStreams   map[string][]userStreamInfo // userID -> active streams

	// Stream lifecycle overrides; zero values use production defaults.
	// Tests set these directly to exercise cap and heartbeat paths on a
	// reasonable timescale.
	streamLifetimeOverride  time.Duration
	streamHeartbeatOverride time.Duration
}

// defaultStreamLifetime caps server-streaming RPCs below Cloud Run's 900s
// frontend timeout so the stream ends with a clean EOF rather than a 504.
const defaultStreamLifetime = 14 * time.Minute

// defaultStreamHeartbeat sends a liveness signal on idle streams so dead
// peers are reaped promptly instead of after the full lifetime cap.
const defaultStreamHeartbeat = 30 * time.Second

func (s *Service) streamLifetime() time.Duration {
	if s.streamLifetimeOverride > 0 {
		return s.streamLifetimeOverride
	}
	return defaultStreamLifetime
}

func (s *Service) streamHeartbeat() time.Duration {
	if s.streamHeartbeatOverride > 0 {
		return s.streamHeartbeatOverride
	}
	return defaultStreamHeartbeat
}

// New creates a new community service. bus is required — every emit site
// publishes via it, and main.go constructs the bus + subscribers before
// calling New (#510 PR 3). Story generation is owned by the
// story_subscriber on the bus (#510 PR 4); the constructor no longer
// takes a story.Creator.
func New(sqlStorage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage, notifService notifications.Service, bus cebus.Publisher, inviteLinkHostname string) *Service {
	svc := &Service{
		storage:             sqlStorage,
		bucket:              bucketStorage,
		notificationService: notifService,
		bus:                 bus,
		notificationDone:    nil, // nil in production (no signaling)
		inviteLinkHostname:  inviteLinkHostname,
		userStreams:         make(map[string][]userStreamInfo),
	}
	return svc
}

// NewWithNotificationSignal creates a new community service with a notification completion signal.
// This is useful for testing to wait for async notifications to complete.
// Tests can read from the returned channel to know when each notification batch completes.
func NewWithNotificationSignal(sqlStorage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage, notifService notifications.Service, bus cebus.Publisher, inviteLinkHostname string) (*Service, chan struct{}) {
	done := make(chan struct{}, 100) // Buffered channel so sends don't block
	svc := &Service{
		storage:             sqlStorage,
		bucket:              bucketStorage,
		notificationService: notifService,
		bus:                 bus,
		notificationDone:    done,
		inviteLinkHostname:  inviteLinkHostname,
		userStreams:         make(map[string][]userStreamInfo),
	}
	return svc, done
}

// SetSystemMessageWriter sets the system message writer for emitting giveaway system messages.
func (s *Service) SetSystemMessageWriter(writer *chatlib.SystemMessageWriter) {
	s.systemMessageWriter = writer
}

// SetAIProvider sets the AI provider for the service (optional).
func (s *Service) SetAIProvider(provider ai.Provider) {
	s.aiProvider = provider
}

// SetStockImageryProvider sets the stock imagery provider for the service (optional).
func (s *Service) SetStockImageryProvider(provider media.StockImageryProvider) {
	s.stockImageryProvider = provider
}

// SetItemSharer registers the per-item-type sharer used by ShareItem to fan an
// item out to communities. Called once per item kind during service wiring.
func (s *Service) SetItemSharer(kind ItemKind, sharer ItemSharer) {
	if s.itemSharers == nil {
		s.itemSharers = make(map[ItemKind]ItemSharer)
	}
	s.itemSharers[kind] = sharer
}

// apiAvailabilityToModel converts API Availability enum to models Availability enum.
func apiAvailabilityToModel(apiAvail api.Availability) models.Availability {
	switch apiAvail {
	case api.Availability_AVAILABILITY_FOR_LOAN:
		return models.Availability_AVAILABILITY_FOR_LOAN
	case api.Availability_AVAILABILITY_FOR_GIVEAWAY:
		return models.Availability_AVAILABILITY_FOR_GIVEAWAY
	default:
		return models.Availability_AVAILABILITY_UNSPECIFIED
	}
}

func modelAvailabilityToAPI(modelAvail models.Availability) api.Availability {
	switch modelAvail {
	case models.Availability_AVAILABILITY_FOR_LOAN:
		return api.Availability_AVAILABILITY_FOR_LOAN
	case models.Availability_AVAILABILITY_FOR_GIVEAWAY:
		return api.Availability_AVAILABILITY_FOR_GIVEAWAY
	default:
		return api.Availability_AVAILABILITY_UNSPECIFIED
	}
}

// getUserDisplayName fetches the display name for a user by ID.
// Returns "Someone" if the user cannot be fetched or has no name.
func (s *Service) getUserDisplayName(ctx context.Context, userID string) string {
	user := &models.User{}
	if err := s.storage.GetByID(ctx, userID, user); err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.Warn("failed to get user for display name", "user_id", userID, "error", err)
		return "Someone"
	}
	if user.Name == "" {
		return "Someone"
	}
	return user.Name
}

// Verify that Service implements the CommunityServiceHandler interface.
var _ apiconnect.CommunityServiceHandler = (*Service)(nil)
