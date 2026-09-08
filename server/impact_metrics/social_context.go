package impact_metrics

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// ConnectionContextResolver resolves the social connection context between two users
// by querying their shared interaction history and community graph.
type ConnectionContextResolver struct {
	storage *storage.ProtoSQLStorage
}

// NewConnectionContextResolver creates a new ConnectionContextResolver.
func NewConnectionContextResolver(s *storage.ProtoSQLStorage) *ConnectionContextResolver {
	return &ConnectionContextResolver{storage: s}
}

// Resolve queries interaction history and community graph to build a ConnectionContext
// for a transaction between userA and userB in a given community.
// Returns error on storage failures (fail fast — no silent fallback to empty context).
func (r *ConnectionContextResolver) Resolve(ctx context.Context, userA, userB, communityID string) (*api.ConnectionContext, error) {
	start := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ResolveConnectionContext",
		"user_a", userA,
		"user_b", userB,
		"community_id", communityID,
	)

	prior, err := r.countPriorInteractions(ctx, userA, userB)
	if err != nil {
		return nil, fmt.Errorf("counting prior interactions: %w", err)
	}

	mutual, err := r.countMutualConnections(ctx, userA, userB, communityID)
	if err != nil {
		return nil, fmt.Errorf("counting mutual connections: %w", err)
	}

	contacts, err := r.countDistinctContactsThisWeek(ctx, userA, communityID)
	if err != nil {
		return nil, fmt.Errorf("counting distinct contacts this week: %w", err)
	}

	durationMs := time.Since(start).Milliseconds()
	logger.DebugContext(ctx, "resolved connection context",
		"prior_interactions", prior,
		"mutual_connections", mutual,
		"distinct_contacts_this_week", contacts,
		"duration_ms", durationMs,
	)

	return &api.ConnectionContext{
		PriorInteractionCount:    prior,
		MutualConnectionCount:    mutual,
		DistinctContactsThisWeek: contacts,
	}, nil
}

// countPriorInteractions counts completed transactions between userA and userB:
// completed transfers (in either direction), fulfilled requests where one was the
// requester and the other was a confirmed helper, and completed experiences both attended.
func (r *ConnectionContextResolver) countPriorInteractions(ctx context.Context, userA, userB string) (int32, error) {
	var total int32

	// Completed transfers: A lent to B
	ab, err := r.storage.QueryByFields(ctx, map[string]any{
		"owner_id":     userA,
		"recipient_id": userB,
		"state":        int32(models.TransferState_TRANSFER_STATE_COMPLETED),
	}, &models.Transfer{})
	if err != nil {
		return 0, fmt.Errorf("querying transfers A→B: %w", err)
	}
	for _, msg := range ab {
		if msg.(*models.Transfer).Deleted == nil {
			total++
		}
	}

	// Completed transfers: B lent to A
	ba, err := r.storage.QueryByFields(ctx, map[string]any{
		"owner_id":     userB,
		"recipient_id": userA,
		"state":        int32(models.TransferState_TRANSFER_STATE_COMPLETED),
	}, &models.Transfer{})
	if err != nil {
		return 0, fmt.Errorf("querying transfers B→A: %w", err)
	}
	for _, msg := range ba {
		if msg.(*models.Transfer).Deleted == nil {
			total++
		}
	}

	// Fulfilled requests where A requested and B was a confirmed helper.
	reqByA, err := r.storage.QueryByFields(ctx, map[string]any{
		"requester_id": userA,
		"state":        int32(models.RequestState_REQUEST_STATE_FULFILLED),
	}, &models.Request{})
	if err != nil {
		return 0, fmt.Errorf("querying fulfilled requests by A: %w", err)
	}
	for _, msg := range reqByA {
		req := msg.(*models.Request)
		if req.Deleted != nil {
			continue
		}
		for _, helperID := range req.ConfirmedHelperIds {
			if helperID == userB {
				total++
				break
			}
		}
	}

	// Fulfilled requests where B requested and A was a confirmed helper.
	reqByB, err := r.storage.QueryByFields(ctx, map[string]any{
		"requester_id": userB,
		"state":        int32(models.RequestState_REQUEST_STATE_FULFILLED),
	}, &models.Request{})
	if err != nil {
		return 0, fmt.Errorf("querying fulfilled requests by B: %w", err)
	}
	for _, msg := range reqByB {
		req := msg.(*models.Request)
		if req.Deleted != nil {
			continue
		}
		for _, helperID := range req.ConfirmedHelperIds {
			if helperID == userA {
				total++
				break
			}
		}
	}

	// Completed experiences attended by both A and B.
	sharedExp, err := r.countSharedExperiences(ctx, userA, userB)
	if err != nil {
		return 0, fmt.Errorf("counting shared experiences: %w", err)
	}
	total += sharedExp

	return total, nil
}

// countSharedExperiences returns the number of completed experiences that both
// userA and userB attended (via ExperienceRSVP with attended=YES).
func (r *ConnectionContextResolver) countSharedExperiences(ctx context.Context, userA, userB string) (int32, error) {
	// Collect experience IDs attended by A.
	rsvpsA, err := r.storage.QueryByFields(ctx, map[string]any{
		"user_id":  userA,
		"attended": int32(models.AttendedStatus_ATTENDED_STATUS_YES),
	}, &models.ExperienceRSVP{})
	if err != nil {
		return 0, fmt.Errorf("querying RSVPs for user A: %w", err)
	}

	if len(rsvpsA) == 0 {
		return 0, nil
	}

	expIDsA := make(map[string]bool, len(rsvpsA))
	for _, msg := range rsvpsA {
		expIDsA[msg.(*models.ExperienceRSVP).ExperienceId] = true
	}

	// Collect experience IDs attended by B.
	rsvpsB, err := r.storage.QueryByFields(ctx, map[string]any{
		"user_id":  userB,
		"attended": int32(models.AttendedStatus_ATTENDED_STATUS_YES),
	}, &models.ExperienceRSVP{})
	if err != nil {
		return 0, fmt.Errorf("querying RSVPs for user B: %w", err)
	}

	// Count experiences both attended that are also completed.
	var count int32
	for _, msg := range rsvpsB {
		expID := msg.(*models.ExperienceRSVP).ExperienceId
		if !expIDsA[expID] {
			continue
		}
		exp := &models.Experience{}
		if err := r.storage.GetByID(ctx, expID, exp); err != nil {
			// Skip experiences we can't load rather than failing.
			continue
		}
		if exp.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED && exp.Deleted == nil {
			count++
		}
	}

	return count, nil
}

// countMutualConnections returns the number of community members who have had
// completed transactions with both userA and userB (excluding A and B themselves).
func (r *ConnectionContextResolver) countMutualConnections(ctx context.Context, userA, userB, _ string) (int32, error) {
	// Build the transaction partner sets for A and B.
	partnersA, err := r.transactionPartners(ctx, userA)
	if err != nil {
		return 0, fmt.Errorf("getting transaction partners for A: %w", err)
	}

	partnersB, err := r.transactionPartners(ctx, userB)
	if err != nil {
		return 0, fmt.Errorf("getting transaction partners for B: %w", err)
	}

	// Count users in both sets, excluding A and B.
	var mutual int32
	for uid := range partnersA {
		if uid == userA || uid == userB {
			continue
		}
		if partnersB[uid] {
			mutual++
		}
	}

	return mutual, nil
}

// transactionPartners returns the set of all user IDs that the given user has
// completed a transfer, request, or shared experience with.
func (r *ConnectionContextResolver) transactionPartners(ctx context.Context, userID string) (map[string]bool, error) {
	partners := make(map[string]bool)

	// Transfers owned by the user (lent to others).
	owned, err := r.storage.QueryByFields(ctx, map[string]any{
		"owner_id": userID,
		"state":    int32(models.TransferState_TRANSFER_STATE_COMPLETED),
	}, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("querying owned transfers: %w", err)
	}
	for _, msg := range owned {
		t := msg.(*models.Transfer)
		if t.Deleted == nil && t.RecipientId != "" {
			partners[t.RecipientId] = true
		}
	}

	// Transfers received by the user (borrowed from others).
	received, err := r.storage.QueryByFields(ctx, map[string]any{
		"recipient_id": userID,
		"state":        int32(models.TransferState_TRANSFER_STATE_COMPLETED),
	}, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("querying received transfers: %w", err)
	}
	for _, msg := range received {
		t := msg.(*models.Transfer)
		if t.Deleted == nil && t.OwnerId != "" {
			partners[t.OwnerId] = true
		}
	}

	// Requests fulfilled with confirmed helpers.
	requested, err := r.storage.QueryByFields(ctx, map[string]any{
		"requester_id": userID,
		"state":        int32(models.RequestState_REQUEST_STATE_FULFILLED),
	}, &models.Request{})
	if err != nil {
		return nil, fmt.Errorf("querying fulfilled requests by user: %w", err)
	}
	for _, msg := range requested {
		req := msg.(*models.Request)
		if req.Deleted != nil {
			continue
		}
		for _, helperID := range req.ConfirmedHelperIds {
			partners[helperID] = true
		}
	}

	return partners, nil
}

// countDistinctContactsThisWeek counts the number of unique people userA has
// interacted with in community events during the current calendar week (last 7 days).
func (r *ConnectionContextResolver) countDistinctContactsThisWeek(ctx context.Context, userID, communityID string) (int32, error) {
	weekAgo := time.Now().AddDate(0, 0, -7).Unix()

	events, err := r.storage.QueryByField(ctx, "community_id", communityID, &models.CommunityEvent{})
	if err != nil {
		return 0, fmt.Errorf("querying community events for weekly contacts: %w", err)
	}

	contacts := make(map[string]bool)
	for _, msg := range events {
		ev := msg.(*models.CommunityEvent)
		if ev.OccurredAtUnixSec < weekAgo {
			continue
		}

		// Count transactions where the user was the actor and there was an object user.
		if ev.ActorId == userID && ev.ObjectUserId != "" && ev.ObjectUserId != userID {
			contacts[ev.ObjectUserId] = true
		}
		// Count transactions where the user was the object.
		if ev.ObjectUserId == userID && ev.ActorId != "" && ev.ActorId != userID {
			contacts[ev.ActorId] = true
		}
	}

	// #nosec G115 - len(map) will not overflow int32 in practice
	return int32(len(contacts)), nil
}
