package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// authorizeAndPrepareGearConversation validates that the caller may start (or
// retrieve) the perpetual chat for a gear item. It enforces three invariants:
//
//  1. The gear exists.
//  2. The gear is shared with at least one community.
//  3. The request's community_id is among the gear's sharing communities AND
//     the caller is either the gear owner or a member of that community.
//
// Authorization follows #1675: any active member of a community the gear is
// shared with may access gear conversations, not just the owner.
func (s *Service) authorizeAndPrepareGearConversation(
	ctx context.Context,
	logger *logging.Logger,
	userID, gearID, communityID string,
) error {
	// Fetch the gear.
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.Error("failed to get gear", "error", err)
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}

	// Fetch all communities this gear is shared with.
	communityGears, err := s.storage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
	if err != nil {
		logger.Error("failed to query community gear", "error", err)
		return connecterr.Internal(ctx, "StartConversation", err, "gear_id", gearID)
	}

	// Gear must be shared with at least one community.
	if len(communityGears) == 0 {
		logger.Info("gear is not shared with any community", "gear_id", gearID)
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("this gear must be shared with a community before a discussion can start"))
	}

	// Verify the request's community_id is in the gear's share set.
	sharedInRequestedCommunity := false
	for _, msg := range communityGears {
		if msg.(*models.CommunityGear).CommunityId == communityID {
			sharedInRequestedCommunity = true
			break
		}
	}
	if !sharedInRequestedCommunity {
		logger.Info("gear is not shared with the requested community",
			"gear_id", gearID, "community_id", communityID)
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("gear is not shared with this community"))
	}

	// Gear owner is always authorized.
	if gear.OwnerId == userID {
		return nil
	}

	// Non-owner: verify active membership in the requested community.
	if err := requireMemberOfGearCommunity(ctx, s.storage, communityID, userID); err != nil {
		return err
	}
	return nil
}

// requireMemberOfGearCommunity checks active membership in a single community
// and returns PermissionDenied if the user is not a member.
func requireMemberOfGearCommunity(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	communityID, userID string,
) error {
	members, err := store.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"user_id":      userID,
	}, &models.CommunityUser{})
	if err != nil {
		return connecterr.Internal(ctx, "StartConversation", err,
			"community_id", communityID, "user_id", userID)
	}
	if len(members) == 0 {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user is not a member of any community this gear is shared with"))
	}
	return nil
}

// finalizeGearConversation persists the conversation_id back onto the Gear row
// when it was absent (recovery path for gear that missed the ShareGear-time
// creation) and adds the gear owner as a participant. It is idempotent: if the
// gear already has a conversation_id no write is issued.
func (s *Service) finalizeGearConversation(
	ctx context.Context,
	logger *logging.Logger,
	communityID, gearID, conversationID string,
) error {
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.Error("failed to re-fetch gear for finalization", "error", err)
		return connecterr.Internal(ctx, "StartConversation", err, "gear_id", gearID)
	}

	if gear.ConversationId == "" {
		// Conversation was just created (or the ShareGear path missed it).
		// Persist the id on the gear row so future GetGear calls return it directly.
		communityGears, err := s.storage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			logger.Error("failed to query community gear for backfill check", "error", err)
			return connecterr.Internal(ctx, "StartConversation", err, "gear_id", gearID)
		}
		if len(communityGears) > 0 {
			// Gear is shared but conversation_id was missing — this is the backfill
			// path (#2003). Warn so we can quantify how often ShareGear-time creation
			// misfires, and decide whether a migration is warranted.
			logger.Warn("backfilling gear conversation at chat open",
				"operation", "StartConversation",
				"gear_id", gearID,
				"community_id", communityID,
				"topic_type", "gear",
			)
		}

		gear.ConversationId = conversationID
		if err := s.storage.Update(ctx, gear); err != nil {
			logger.Error("failed to persist conversation_id on gear", "error", err)
			return connecterr.Internal(ctx, "StartConversation", err, "gear_id", gearID)
		}
		logger.Info("persisted conversation_id on gear", "conversation_id", conversationID)

		// The conversation was born here rather than at share time, so it has
		// neither the sharing anchor nor the owner's description. Seed both now
		// — the same opening pair ShareGear/SubmitRequest/ShareExperience write —
		// so a gear conversation opens identically regardless of which path
		// created it (#2509). Best-effort and dedup-guarded inside the helper.
		var availability models.Availability
		for _, cg := range communityGears {
			if g, ok := cg.(*models.CommunityGear); ok && g.CommunityId == communityID {
				availability = g.Availability
				break
			}
		}
		s.seedBackfilledGearConversation(ctx, logger, conversationID, gear, availability)
	}

	// Add gear owner as participant if not already present. This mirrors the
	// policy in createGearConversation (gear_sharing.go) and the recovery path.
	if err := chat.AddParticipantToConversation(ctx, s.chatConvStorage, conversationID, gear.OwnerId); err != nil {
		logger.Warn("failed to add gear owner as participant", "error", err)
		// Non-fatal: the conversation exists; the owner can still access it.
	}

	return nil
}

// seedBackfilledGearConversation writes the sharing anchor + the owner's
// description as the first comment into a gear conversation that was created
// lazily at chat-open (rather than by ShareGear), so it opens with the same
// pair every other gear conversation does (#2509). Best-effort: failures are
// logged, never fatal. Dedup-guarded — it only seeds a conversation that has no
// messages yet, so it can never double up with ShareGear's seed.
func (s *Service) seedBackfilledGearConversation(
	ctx context.Context,
	logger *logging.Logger,
	conversationID string,
	gear *models.Gear,
	availability models.Availability,
) {
	existing, err := storage.ListByConversation(ctx, s.storage, conversationID, true, 1)
	if err != nil {
		logger.Warn("failed to check messages before seeding gear conversation",
			"conversation_id", conversationID, "error", err)
		return
	}
	if len(existing) > 0 {
		return // Already seeded (e.g. by ShareGear) — nothing to do.
	}

	ownerName := "Someone"
	owner := &models.User{}
	if err := s.storage.GetByID(ctx, gear.OwnerId, owner); err == nil && owner.Name != "" {
		ownerName = owner.Name
	}

	writer := chat.NewSystemMessageWriter(s.storage, s.bus)
	if err := chat.PostGearCreationMessages(
		ctx, writer, conversationID, gear.OwnerId, ownerName, gear.Name, gear.Description, availability,
	); err != nil {
		logger.Warn("failed to seed gear creation messages at backfill",
			"conversation_id", conversationID, "error", err)
	}
}
