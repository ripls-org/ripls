package admin

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// ListSimulations implements the ListSimulations RPC.
// It returns all distinct simulation IDs found in the database with counts.
// Only available when dev mode is enabled.
func (s *Service) ListSimulations(
	ctx context.Context,
	req *connect.Request[api.ListSimulationsRequest],
) (*connect.Response[api.ListSimulationsResponse], error) {
	if !s.devMode {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("ListSimulations is only available in dev mode"))
	}

	// Query distinct simulation_ids from both user and community tables.
	userSimIDs, err := s.db.QueryDistinctField(ctx, "user", "simulation_id")
	if err != nil {
		return nil, fmt.Errorf("failed to query user simulation IDs: %w", err)
	}
	communitySimIDs, err := s.db.QueryDistinctField(ctx, "community", "simulation_id")
	if err != nil {
		return nil, fmt.Errorf("failed to query community simulation IDs: %w", err)
	}

	// Deduplicate.
	seen := make(map[string]bool)
	var allIDs []string
	for _, id := range userSimIDs {
		if !seen[id] {
			seen[id] = true
			allIDs = append(allIDs, id)
		}
	}
	for _, id := range communitySimIDs {
		if !seen[id] {
			seen[id] = true
			allIDs = append(allIDs, id)
		}
	}

	// Build info for each simulation ID.
	var simulations []*api.SimulationInfo
	for _, simID := range allIDs {
		userCount, err := s.db.CountByField(ctx, "user", "simulation_id", simID)
		if err != nil {
			return nil, fmt.Errorf("failed to count users for %s: %w", simID, err)
		}
		communityCount, err := s.db.CountByField(ctx, "community", "simulation_id", simID)
		if err != nil {
			return nil, fmt.Errorf("failed to count communities for %s: %w", simID, err)
		}
		simulations = append(simulations, &api.SimulationInfo{
			SimulationId:   simID,
			UserCount:      userCount,
			CommunityCount: communityCount,
		})
	}

	return connect.NewResponse(&api.ListSimulationsResponse{
		Simulations: simulations,
	}), nil
}

// CleanupSimulation implements the CleanupSimulation RPC.
// It removes all data created by a specific simulation run, cascading through
// all associated records. Only available when dev mode is enabled.
func (s *Service) CleanupSimulation(
	ctx context.Context,
	req *connect.Request[api.CleanupSimulationRequest],
) (*connect.Response[api.CleanupSimulationResponse], error) {
	logger := logging.LoggerWithContext(ctx)
	startTime := time.Now()

	if !s.devMode {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("CleanupSimulation is only available in dev mode"))
	}

	simID := req.Msg.SimulationId
	if simID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("simulation_id is required"))
	}

	logger.InfoContext(ctx, "starting simulation cleanup",
		"simulation_id", simID)

	// Find simulation users and communities.
	userIDs, err := s.queryIDsBySimulationID("user", simID)
	if err != nil {
		return nil, fmt.Errorf("failed to query simulation users: %w", err)
	}
	communityIDs, err := s.queryIDsBySimulationID("community", simID)
	if err != nil {
		return nil, fmt.Errorf("failed to query simulation communities: %w", err)
	}

	logger.InfoContext(ctx, "found simulation entities",
		"simulation_id", simID,
		"users", len(userIDs),
		"communities", len(communityIDs))

	if len(userIDs) == 0 && len(communityIDs) == 0 {
		return connect.NewResponse(&api.CleanupSimulationResponse{
			DurationMilliseconds: time.Since(startTime).Milliseconds(),
		}), nil
	}

	var totalDeleted int64

	// Delete in reverse dependency order — leaf entities first, then parents.

	// 1. Community events (by community_id).
	n, err := s.deleteByFieldIn(ctx, "community_event", "community_id", communityIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean community_event: %w", err)
	}
	totalDeleted += n

	// 2. Feed item views (by community_id).
	n, err = s.deleteByFieldIn(ctx, "FeedItemView", "community_id", communityIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean FeedItemView: %w", err)
	}
	totalDeleted += n

	// 3. Stories (by community_id).
	n, err = s.deleteByFieldIn(ctx, "Story", "community_id", communityIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean Story: %w", err)
	}
	totalDeleted += n

	// 4. Chat messages (by conversation, which links to community).
	// Find conversation IDs from community gear/request/experience, then delete messages.
	conversationIDs, err := s.collectConversationIDs(ctx, communityIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to collect conversation IDs: %w", err)
	}
	n, err = s.deleteByFieldIn(ctx, "chat_message", "conversation_id", conversationIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean chat_message: %w", err)
	}
	totalDeleted += n
	n, err = s.deleteByFieldIn(ctx, "chat_conversation", "community_id", communityIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean chat_conversation: %w", err)
	}
	totalDeleted += n

	// 5. Experience sub-entities (RSVPs, time proposals, votes) — by user_id.
	// Also find experience IDs owned by simulation users for targeted cleanup.
	experienceIDs, err := s.collectExperienceIDs(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to collect experience IDs: %w", err)
	}
	for _, table := range []string{"experience_rsvp", "experience_time_proposal"} {
		n, err = s.deleteByFieldIn(ctx, table, "experience_id", experienceIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to clean %s: %w", table, err)
		}
		totalDeleted += n
	}
	n, err = s.deleteTimeVotes(ctx, experienceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean time_vote: %w", err)
	}
	totalDeleted += n

	// 6. Request sub-entities (offers) — find request IDs from simulation users.
	requestIDs, err := s.collectRequestIDs(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to collect request IDs: %w", err)
	}
	n, err = s.deleteByFieldIn(ctx, "request_offer", "request_id", requestIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean request_offer: %w", err)
	}
	totalDeleted += n

	// 7. Transfers (by community_id).
	n, err = s.deleteByFieldIn(ctx, "transfer", "community_id", communityIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean transfer: %w", err)
	}
	totalDeleted += n

	// 8. Community junction tables.
	for _, table := range []string{
		"community_gear", "community_request", "community_experience",
		"community_invitation_link", "community_user", "community_region",
	} {
		n, err = s.deleteByFieldIn(ctx, table, "community_id", communityIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to clean %s: %w", table, err)
		}
		totalDeleted += n
	}

	// 9. Owned entities (by owner_id/requester_id).
	n, err = s.deleteByFieldIn(ctx, "gear", "owner_id", userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean gear: %w", err)
	}
	totalDeleted += n

	n, err = s.deleteByFieldIn(ctx, "experience", "owner_id", userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean experience: %w", err)
	}
	totalDeleted += n

	n, err = s.deleteByFieldIn(ctx, "request", "requester_id", userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean request: %w", err)
	}
	totalDeleted += n

	// 10. User sub-entities.
	for _, table := range []string{"user_device", "pending_password_reset"} {
		n, err = s.deleteByFieldIn(ctx, table, "user_id", userIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to clean %s: %w", table, err)
		}
		totalDeleted += n
	}

	// 11. Media uploaded by simulation users.
	n, err = s.deleteByFieldIn(ctx, "media", "user_id", userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to clean media: %w", err)
	}
	totalDeleted += n

	// 12. Communities (by simulation_id).
	for _, id := range communityIDs {
		if err := s.db.Delete(ctx, &models.Community{Id: id}); err != nil {
			return nil, fmt.Errorf("failed to delete community %s: %w", id, err)
		}
		totalDeleted++
	}

	// 13. Users (by simulation_id).
	for _, id := range userIDs {
		if err := s.db.Delete(ctx, &models.User{Id: id}); err != nil {
			return nil, fmt.Errorf("failed to delete user %s: %w", id, err)
		}
		totalDeleted++
	}

	duration := time.Since(startTime)
	logger.InfoContext(ctx, "simulation cleanup complete",
		"simulation_id", simID,
		"users_deleted", len(userIDs),
		"communities_deleted", len(communityIDs),
		"total_records_deleted", totalDeleted,
		"duration_ms", duration.Milliseconds())

	return connect.NewResponse(&api.CleanupSimulationResponse{
		UsersDeleted:         int64(len(userIDs)),
		CommunitiesDeleted:   int64(len(communityIDs)),
		RecordsDeleted:       totalDeleted,
		DurationMilliseconds: duration.Milliseconds(),
	}), nil
}

// queryIDsBySimulationID queries a table for records matching the simulation_id and returns their IDs.
func (s *Service) queryIDsBySimulationID(tableName, simID string) ([]string, error) {
	ctx := context.Background()
	var msgType proto.Message
	switch tableName {
	case "user":
		msgType = &models.User{}
	case "community":
		msgType = &models.Community{}
	default:
		return nil, fmt.Errorf("unsupported table for simulation_id query: %s", tableName)
	}

	messages, err := s.db.QueryByField(ctx, "simulation_id", simID, msgType)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		id := msg.ProtoReflect().Get(msg.ProtoReflect().Descriptor().Fields().ByName("id")).String()
		ids = append(ids, id)
	}
	return ids, nil
}

// deleteByFieldIn deletes records from a table where fieldName is in the provided values.
func (s *Service) deleteByFieldIn(ctx context.Context, tableName, fieldName string, values []string) (int64, error) {
	return s.db.DeleteByFieldIn(ctx, tableName, fieldName, values)
}

// collectConversationIDs gathers conversation IDs for items shared in the given communities.
// Conversations are now stored on the primary models (Gear, Request, Experience).
// For backwards compatibility, also reads the deprecated community junction table fields.
func (s *Service) collectConversationIDs(ctx context.Context, communityIDs []string) ([]string, error) {
	seen := make(map[string]struct{})
	var ids []string
	add := func(id string) {
		if id != "" {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}

	for _, cid := range communityIDs {
		// Gear conversations: read from Gear.conversation_id via CommunityGear join.
		cgMsgs, err := s.db.QueryByField(ctx, "community_id", cid, &models.CommunityGear{})
		if err != nil {
			return nil, fmt.Errorf("failed to query community gear: %w", err)
		}
		for _, msg := range cgMsgs {
			cg := msg.(*models.CommunityGear)
			gear := &models.Gear{}
			if err := s.db.GetByID(ctx, cg.GearId, gear); err == nil {
				add(gear.ConversationId)
			}
		}

		// Request conversations: read from Request.conversation_id via CommunityRequest join.
		crMsgs, err := s.db.QueryByField(ctx, "community_id", cid, &models.CommunityRequest{})
		if err != nil {
			return nil, fmt.Errorf("failed to query community requests: %w", err)
		}
		for _, msg := range crMsgs {
			cr := msg.(*models.CommunityRequest)
			req := &models.Request{}
			if err := s.db.GetByID(ctx, cr.RequestId, req); err == nil {
				add(req.ConversationId)
			}
		}

		// Experience conversations: read from Experience.conversation_id via CommunityExperience join.
		ceMsgs, err := s.db.QueryByField(ctx, "community_id", cid, &models.CommunityExperience{})
		if err != nil {
			return nil, fmt.Errorf("failed to query community experiences: %w", err)
		}
		for _, msg := range ceMsgs {
			ce := msg.(*models.CommunityExperience)
			exp := &models.Experience{}
			if err := s.db.GetByID(ctx, ce.ExperienceId, exp); err == nil {
				add(exp.ConversationId)
			}
		}
	}
	return ids, nil
}

// collectExperienceIDs gathers experience IDs owned by the given users.
func (s *Service) collectExperienceIDs(ctx context.Context, userIDs []string) ([]string, error) {
	var ids []string
	for _, uid := range userIDs {
		msgs, err := s.db.QueryByField(ctx, "owner_id", uid, &models.Experience{})
		if err != nil {
			return nil, fmt.Errorf("failed to query experiences for user %s: %w", uid, err)
		}
		for _, msg := range msgs {
			ids = append(ids, msg.(*models.Experience).Id)
		}
	}
	return ids, nil
}

// collectRequestIDs gathers request IDs created by the given users.
func (s *Service) collectRequestIDs(ctx context.Context, userIDs []string) ([]string, error) {
	var ids []string
	for _, uid := range userIDs {
		msgs, err := s.db.QueryByField(ctx, "requester_id", uid, &models.Request{})
		if err != nil {
			return nil, fmt.Errorf("failed to query requests for user %s: %w", uid, err)
		}
		for _, msg := range msgs {
			ids = append(ids, msg.(*models.Request).Id)
		}
	}
	return ids, nil
}

// deleteTimeVotes deletes time votes for proposals belonging to the given experiences.
func (s *Service) deleteTimeVotes(ctx context.Context, experienceIDs []string) (int64, error) {
	var total int64
	for _, eid := range experienceIDs {
		// Find proposals for this experience.
		msgs, err := s.db.QueryByField(ctx, "experience_id", eid, &models.ExperienceTimeProposal{})
		if err != nil {
			return total, fmt.Errorf("failed to query time proposals for experience %s: %w", eid, err)
		}
		proposalIDs := make([]string, 0, len(msgs))
		for _, msg := range msgs {
			proposalIDs = append(proposalIDs, msg.(*models.ExperienceTimeProposal).Id)
		}
		n, err := s.db.DeleteByFieldIn(ctx, "time_vote", "proposal_id", proposalIDs)
		if err != nil {
			return total, fmt.Errorf("failed to delete time votes: %w", err)
		}
		total += n
	}
	return total, nil
}
