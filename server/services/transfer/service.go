package transfer

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the TransferService RPC interface.
type Service struct {
	storage             *storage.ProtoSQLStorage
	notificationService notifications.Service
	bus                 cebus.Publisher // CommunityEvent publisher; required (#510 PR 3).
	systemMessageWriter *chat.SystemMessageWriter
	estimatorCfg        *estimator.Config                         // Optional: can be nil (enables full impact estimation)
	socialResolver      *impact_metrics.ConnectionContextResolver // Optional: can be nil (enables SF estimation)
	aiProvider          ai.Provider                               // Optional: can be nil (enables LLM social attribute inference)
	gearSharer          GearSharer                                // Required for OfferTransfer; injected from wiring (community service)
	notificationDone    chan struct{}                             // For testing: signals when async notifications complete (nil in production)
}

// SetEstimatorConfig sets the impact estimator config for this service (optional).
// When set, enables full three-dimensional impact estimation on transfer completion.
func (s *Service) SetEstimatorConfig(cfg *estimator.Config) {
	s.estimatorCfg = cfg
}

// SetSocialResolver sets the social connection context resolver (optional).
// When set, enables social footprint estimation on transfer lifecycle events.
func (s *Service) SetSocialResolver(r *impact_metrics.ConnectionContextResolver) {
	s.socialResolver = r
}

// SetAIProvider sets the AI provider for LLM-assisted social attribute inference (optional).
// When set, enables LLM-inferred duration and vulnerability on transfer completion.
func (s *Service) SetAIProvider(provider ai.Provider) {
	s.aiProvider = provider
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

// New creates a new transfer service. bus is required — emit sites publish
// via it (#510 PR 3). Story generation is owned by the story_subscriber
// on the bus (#510 PR 4); the constructor no longer takes a story.Creator.
func New(storage *storage.ProtoSQLStorage, notificationService notifications.Service, bus cebus.Publisher, systemMessageWriter *chat.SystemMessageWriter) *Service {
	return &Service{
		storage:             storage,
		notificationService: notificationService,
		bus:                 bus,
		systemMessageWriter: systemMessageWriter,
		notificationDone:    nil, // nil in production (no signaling)
	}
}

// NewWithNotificationSignal creates a new transfer service with a notification completion signal.
// This is useful for testing to wait for async notifications to complete.
// Tests can read from the returned channel to know when each notification batch completes.
func NewWithNotificationSignal(storage *storage.ProtoSQLStorage, notificationService notifications.Service, bus cebus.Publisher, systemMessageWriter *chat.SystemMessageWriter) (*Service, chan struct{}) {
	done := make(chan struct{}, 100) // Buffered channel so sends don't block
	return &Service{
		storage:             storage,
		notificationService: notificationService,
		bus:                 bus,
		systemMessageWriter: systemMessageWriter,
		notificationDone:    done,
	}, done
}

// Helper functions to convert between API and model enum types.

func apiTransferTypeToModel(apiType api.TransferType) models.TransferType {
	switch apiType {
	case api.TransferType_TRANSFER_TYPE_LOAN:
		return models.TransferType_TRANSFER_TYPE_LOAN
	case api.TransferType_TRANSFER_TYPE_GIVEAWAY:
		return models.TransferType_TRANSFER_TYPE_GIVEAWAY
	default:
		return models.TransferType_TRANSFER_TYPE_UNSPECIFIED
	}
}

func modelTransferTypeToAPI(modelType models.TransferType) api.TransferType {
	switch modelType {
	case models.TransferType_TRANSFER_TYPE_LOAN:
		return api.TransferType_TRANSFER_TYPE_LOAN
	case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
		return api.TransferType_TRANSFER_TYPE_GIVEAWAY
	default:
		return api.TransferType_TRANSFER_TYPE_UNSPECIFIED
	}
}

func apiTransferStateToModel(apiState api.TransferState) models.TransferState {
	switch apiState {
	case api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
		return models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED
	case api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		return models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
	case api.TransferState_TRANSFER_STATE_ACTIVE:
		return models.TransferState_TRANSFER_STATE_ACTIVE
	case api.TransferState_TRANSFER_STATE_COMPLETED:
		return models.TransferState_TRANSFER_STATE_COMPLETED
	case api.TransferState_TRANSFER_STATE_CANCELLED:
		return models.TransferState_TRANSFER_STATE_CANCELLED
	default:
		return models.TransferState_TRANSFER_STATE_UNSPECIFIED
	}
}

func modelTransferStateToAPI(modelState models.TransferState) api.TransferState {
	switch modelState {
	case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED:
		return api.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED
	case models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED:
		return api.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED
	case models.TransferState_TRANSFER_STATE_ACTIVE:
		return api.TransferState_TRANSFER_STATE_ACTIVE
	case models.TransferState_TRANSFER_STATE_COMPLETED:
		return api.TransferState_TRANSFER_STATE_COMPLETED
	case models.TransferState_TRANSFER_STATE_CANCELLED:
		return api.TransferState_TRANSFER_STATE_CANCELLED
	default:
		return api.TransferState_TRANSFER_STATE_UNSPECIFIED
	}
}

// buildTransfer builds an API Transfer from a stored Transfer model.
func (s *Service) buildTransfer(ctx context.Context, transfer *models.Transfer) (*api.Transfer, error) {
	results, err := s.buildTransfers(ctx, []*models.Transfer{transfer})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, connecterr.Internal(ctx, "buildTransfer", fmt.Errorf("failed to build transfer %s", transfer.Id))
	}
	return results[0], nil
}

// buildTransfers builds API Transfers from stored Transfer models using batch lookups.
// Fetches all gear, users, community-gear, and conversations in bulk.
func (s *Service) buildTransfers(ctx context.Context, transfers []*models.Transfer) ([]*api.Transfer, error) {
	if len(transfers) == 0 {
		return nil, nil
	}

	logger := logging.LoggerWithContext(ctx)

	// Collect all IDs needed
	gearIDs := make([]string, 0, len(transfers))
	userIDs := make([]string, 0, len(transfers)*2)
	type cgKey struct{ gearID, communityID string }
	cgKeys := make([]cgKey, 0, len(transfers))
	cgKeySeen := make(map[cgKey]bool)

	for _, t := range transfers {
		gearIDs = append(gearIDs, t.GearId)
		userIDs = append(userIDs, t.OwnerId)
		if t.RecipientId != "" {
			userIDs = append(userIDs, t.RecipientId)
		}
		k := cgKey{t.GearId, t.CommunityId}
		if !cgKeySeen[k] {
			cgKeySeen[k] = true
			cgKeys = append(cgKeys, k)
		}
	}

	// Batch fetch gear, including soft-deleted rows so transfer history
	// remains viewable after the underlying gear is deleted (#1641).
	gearMap, err := storage.GetByIDs[*models.Gear](s.storage, ctx, gearIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		return nil, connecterr.Internal(ctx, "buildTransfers", err, "detail", "batch fetch gear")
	}

	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, userIDs)
	if err != nil {
		return nil, connecterr.Internal(ctx, "buildTransfers", err, "detail", "batch fetch users")
	}

	// Fetch community-gear records to get conversation IDs.
	// We need to query per (gear_id, community_id) pair; collect conversation IDs for batch fetch.

	type cgInfo struct {
		conversationID string
	}
	cgMap := make(map[cgKey]cgInfo)
	var conversationIDs []string
	for _, k := range cgKeys {
		// Read conversation_id from Gear (canonical location).
		gearStored, ok := gearMap[k.gearID]
		if !ok {
			continue
		}
		convID := gearStored.ConversationId
		if convID != "" {
			cgMap[k] = cgInfo{conversationID: convID}
			conversationIDs = append(conversationIDs, convID)
		}
	}

	// Batch fetch conversations for participant counts
	convMap, err := storage.GetByIDs[*models.ChatConversation](s.storage, ctx, conversationIDs)
	if err != nil {
		logger.WarnContext(ctx, "failed to batch fetch conversations", "error", err)
		convMap = nil
	}

	// Build results
	results := make([]*api.Transfer, 0, len(transfers))
	for _, t := range transfers {
		// Gear
		gear, ok := gearMap[t.GearId]
		if !ok {
			logger.ErrorContext(ctx, "gear not found for transfer",
				"gear_id", t.GearId, "transfer_id", t.Id)
			return nil, connecterr.Internal(ctx, "buildTransfers", fmt.Errorf("gear %s not found", t.GearId))
		}

		// Owner — placeholder if the user record has been deleted.
		ownerUser := services.ResolveUserOrFormer(userMap, t.OwnerId)

		// Recipient — placeholder if the user record has been deleted.
		recipientUser := services.ResolveUserOrFormer(userMap, t.RecipientId)

		// Conversation from community-gear
		k := cgKey{t.GearId, t.CommunityId}
		conversationID := cgMap[k].conversationID
		participantCount := int32(0)
		if conversationID != "" && convMap != nil {
			if conv, found := convMap[conversationID]; found {
				participantCount = int32(len(conv.ParticipantIds))
			}
		}

		gearMediaID := ""
		if len(gear.MediaIds) > 0 {
			gearMediaID = gear.MediaIds[0]
		}

		apiTransfer := &api.Transfer{
			Id:                     t.Id,
			GearId:                 t.GearId,
			GearName:               gear.Name,
			GearMediaId:            gearMediaID,
			Owner:                  ownerUser,
			Recipient:              recipientUser,
			TransferType:           modelTransferTypeToAPI(t.TransferType),
			State:                  modelTransferStateToAPI(t.State),
			ConversationId:         &conversationID,
			LatestRequestUnixSec:   t.LatestRequestUnixSec,
			ParticipantCount:       participantCount,
			EstimatedPickupUnixSec: t.EstimatedPickupUnixSec,
			ActualPickupUnixSec:    t.ActualPickupUnixSec,
			LoanDurationDays:       t.LoanDurationDays,
			CommunityId:            t.CommunityId,
		}
		if originRequestID := t.GetOriginRequestId(); originRequestID != "" {
			apiTransfer.Origin = &api.Transfer_OriginRequestId{OriginRequestId: originRequestID}
		} else if originExperienceID := t.GetOriginExperienceId(); originExperienceID != "" {
			apiTransfer.Origin = &api.Transfer_OriginExperienceId{OriginExperienceId: originExperienceID}
		}
		results = append(results, apiTransfer)
	}

	return results, nil
}

// getUserDisplayName fetches a user's display name for system messages.
// Returns the user's name if found, or "Someone" as a fallback.
func (s *Service) getUserDisplayName(ctx context.Context, userID string) string {
	logger := logging.LoggerWithContext(ctx)

	user := &models.User{}
	if err := s.storage.GetByID(ctx, userID, user); err != nil {
		logger.ErrorContext(
			ctx, "failed to get user for display name",
			"user_id", userID,
			"error", err,
		)
		return "Someone"
	}
	if user.Name == "" {
		return "Someone"
	}
	return user.Name
}

// Verify that Service implements the TransferServiceHandler interface.
var _ apiconnect.TransferServiceHandler = (*Service)(nil)
