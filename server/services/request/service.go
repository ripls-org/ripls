package request

import (
	"context"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the RequestService RPC interface.
type Service struct {
	storage                    *storage.ProtoSQLStorage
	bucket                     storage.BucketStorage
	notificationService        notifications.Service
	bus                        cebus.Publisher // CommunityEvent publisher; required (#510 PR 3).
	systemMessageWriter        *chat.SystemMessageWriter
	stockImageryProvider       media.StockImageryProvider
	aiProvider                 ai.Provider
	locationProvider           location.Provider
	estimatorCfg               *estimator.Config                         // Optional: can be nil (enables full impact estimation)
	socialResolver             *impact_metrics.ConnectionContextResolver // Optional: can be nil (enables SF estimation)
	notificationDone           chan struct{}                             // For testing: signals when async notifications complete (nil in production)
	stockImageryDone           chan struct{}                             // For testing: signals when async stock imagery fetch completes (nil in production)
	stockImageryBeforeWrite    chan struct{}                             // For testing: goroutine blocks here before entering WithTx; test receives to release (nil in production)
	suggestionChipsBeforeWrite chan struct{}                             // For testing: goroutine blocks here before entering WithTx; test receives to release (nil in production)
	suggestionChipsDone        chan struct{}                             // For testing: signals when the suggestion-chips goroutine finishes (nil in production)
}

// SetEstimatorConfig sets the impact estimator config for this service (optional).
// When set, enables full three-dimensional impact estimation on request fulfillment.
func (s *Service) SetEstimatorConfig(cfg *estimator.Config) {
	s.estimatorCfg = cfg
}

// SetSocialResolver sets the social connection context resolver (optional).
// When set, enables social footprint estimation on request fulfillment.
func (s *Service) SetSocialResolver(r *impact_metrics.ConnectionContextResolver) {
	s.socialResolver = r
}

// resolveConnectionContext resolves the social connection context between two users.
// Returns nil if no resolver is set or if resolution fails (logs warning on failure).
func (s *Service) resolveConnectionContext(ctx context.Context, userA, userB, communityID string, logger *logging.Logger) *api.ConnectionContext {
	if s.socialResolver == nil || userA == "" || userB == "" {
		return nil
	}
	connCtx, err := s.socialResolver.Resolve(ctx, userA, userB, communityID)
	if err != nil {
		logger.WarnContext(ctx, "failed to resolve connection context for SF estimation",
			"user_a", userA, "user_b", userB, "error", err)
		return nil
	}
	return connCtx
}

// New creates a new request service. bus is required — emit sites publish
// via it (#510 PR 3). Story generation is owned by the story_subscriber
// on the bus (#510 PR 4); the constructor no longer takes a story.Creator.
func New(storage *storage.ProtoSQLStorage, bucket storage.BucketStorage, notificationService notifications.Service, bus cebus.Publisher, stockImageryProvider media.StockImageryProvider, aiProvider ai.Provider, systemMessageWriter *chat.SystemMessageWriter) *Service {
	return &Service{
		storage:              storage,
		bucket:               bucket,
		notificationService:  notificationService,
		bus:                  bus,
		systemMessageWriter:  systemMessageWriter,
		stockImageryProvider: stockImageryProvider,
		aiProvider:           aiProvider,
		notificationDone:     nil, // nil in production (no signaling)
		stockImageryDone:     nil, // nil in production (no signaling)
	}
}

// NewWithNotificationSignal creates a new request service with a notification completion signal.
// This is useful for testing to wait for async notifications to complete.
func NewWithNotificationSignal(storage *storage.ProtoSQLStorage, bucket storage.BucketStorage, notificationService notifications.Service, bus cebus.Publisher, stockImageryProvider media.StockImageryProvider, aiProvider ai.Provider, systemMessageWriter *chat.SystemMessageWriter) (*Service, chan struct{}) {
	done := make(chan struct{}, 100) // Buffered channel so sends don't block
	return &Service{
		storage:              storage,
		bucket:               bucket,
		notificationService:  notificationService,
		bus:                  bus,
		systemMessageWriter:  systemMessageWriter,
		stockImageryProvider: stockImageryProvider,
		aiProvider:           aiProvider,
		notificationDone:     done,
		stockImageryDone:     nil, // No stock imagery signal in this constructor
	}, done
}

// NewForTesting creates a new request service for integration tests. Returns:
//   - *Service
//   - notifDone: receives after each async notification is dispatched
//   - stockImageryDone: receives after the stock-imagery goroutine finishes
//
// The *BeforeWrite synchronization channels needed for deterministic race tests
// are nil by default so that goroutines run freely in non-race tests. Call
// activateRaceChannels (defined in lifecycle_race_test.go) after this to wire
// the blocking channels in for the four race regression tests (#2900).
func NewForTesting(storage *storage.ProtoSQLStorage, bucket storage.BucketStorage, notificationService notifications.Service, bus cebus.Publisher, stockImageryProvider media.StockImageryProvider, aiProvider ai.Provider, systemMessageWriter *chat.SystemMessageWriter) (*Service, chan struct{}, chan struct{}) {
	notifDone := make(chan struct{}, 100)       // Buffered channel so sends don't block
	stockImageryDone := make(chan struct{}, 10) // Buffered channel for stock imagery completions
	return &Service{
		storage:              storage,
		bucket:               bucket,
		notificationService:  notificationService,
		bus:                  bus,
		systemMessageWriter:  systemMessageWriter,
		stockImageryProvider: stockImageryProvider,
		aiProvider:           aiProvider,
		notificationDone:     notifDone,
		stockImageryDone:     stockImageryDone,
		// *BeforeWrite channels are nil: goroutines run without blocking.
		// Race tests activate them via activateRaceChannels().
	}, notifDone, stockImageryDone
}

// SetLocationProvider sets the geocoding/place-search provider.
// This is optional - if not set, location features will be disabled.
func (s *Service) SetLocationProvider(provider location.Provider) {
	s.locationProvider = provider
}

// getUserDisplayName fetches the display name for a user, returning "Someone" if not found.
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

// convertAPIValueEstimateToStorage converts an API ValueEstimate to a storage model ValueEstimate.
func convertAPIValueEstimateToStorage(apiEstimate *api.ValueEstimate) *models.ValueEstimate {
	if apiEstimate == nil {
		return nil
	}
	result := &models.ValueEstimate{
		EstimatedValueUsd: apiEstimate.EstimatedValueUsd,
	}
	if apiEstimate.Provenance != nil {
		result.Provenance = &models.Provenance{
			Source:     models.ProvenanceSource(apiEstimate.Provenance.Source),
			Name:       apiEstimate.Provenance.Name,
			Version:    apiEstimate.Provenance.Version,
			Confidence: apiEstimate.Provenance.Confidence,
			Reasoning:  apiEstimate.Provenance.Reasoning,
			Sources:    apiEstimate.Provenance.Sources,
		}
	}
	return result
}

// convertAPIRequestMetadataToStorage unpacks API RequestMetadata into individual storage fields.
// Returns value_estimate and category.
func convertAPIRequestMetadataToStorage(metadata *api.RequestMetadata) (*models.ValueEstimate, string) {
	if metadata == nil {
		return nil, ""
	}
	return convertAPIValueEstimateToStorage(metadata.ValueEstimate),
		metadata.Category
}

// convertAPISocialContextToModels converts an api.SocialContext to its
// storage representation, encoding enums as int32 to avoid cross-package imports.
func convertAPISocialContextToModels(sc *api.SocialContext) *models.SocialContext {
	if sc == nil {
		return nil
	}
	return &models.SocialContext{
		TieStrength:   int32(sc.TieStrength),
		Reciprocity:   int32(sc.Reciprocity),
		Novelty:       int32(sc.Novelty),
		Vulnerability: int32(sc.Vulnerability),
		Modality:      int32(sc.Modality),
	}
}

// Verify that Service implements the RequestServiceHandler interface.
var _ apiconnect.RequestServiceHandler = (*Service)(nil)
