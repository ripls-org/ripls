package experience

import (
	"context"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/conversation"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// buildAPIExperiences builds API Experiences from a slice of stored Experience models.
// It performs all dependency fetches as batches before assembling responses,
// eliminating N+1 queries from the list endpoints. Output order matches input order.
//
// Community-scoped RSVP filtering and created_at selection are not applied here;
// the singular buildAPIExperience wrapper handles those for callers that pass a
// communityID.
func (s *Service) buildAPIExperiences(ctx context.Context, experiences []*models.Experience) ([]*api.Experience, error) {
	if len(experiences) == 0 {
		return nil, nil
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "buildAPIExperiences",
		"count", len(experiences),
	)

	// Collect IDs needed for batch queries.
	expIDs := make([]string, 0, len(experiences))
	ownerIDs := make([]string, 0, len(experiences))
	locationIDs := make([]string, 0, len(experiences))
	convIDs := make([]string, 0, len(experiences))
	for _, exp := range experiences {
		expIDs = append(expIDs, exp.Id)
		ownerIDs = append(ownerIDs, exp.OwnerId)
		if exp.LocationId != "" {
			locationIDs = append(locationIDs, exp.LocationId)
		}
		if exp.ConversationId != "" {
			convIDs = append(convIDs, exp.ConversationId)
		}
	}

	// Step 2: Batch-fetch CommunityExperience rows and group by experience ID.
	allCEs, err := storage.QueryByFieldIn[*models.CommunityExperience](s.storage, ctx, "experience_id", expIDs)
	if err != nil {
		logger.Warn("failed to batch fetch community experiences", "error", err)
		allCEs = nil
	}
	ceByExp := make(map[string][]*models.CommunityExperience, len(experiences))
	for _, ce := range allCEs {
		ceByExp[ce.ExperienceId] = append(ceByExp[ce.ExperienceId], ce)
	}

	// Step 3: Batch-fetch locations.
	locMap, err := storage.GetByIDs[*models.Location](s.storage, ctx, locationIDs)
	if err != nil {
		logger.Warn("failed to batch fetch locations", "error", err)
		locMap = make(map[string]*models.Location)
	}

	// Step 4: Batch-fetch RSVPs and group by experience ID.
	allRSVPs, err := storage.QueryByFieldIn[*models.ExperienceRSVP](s.storage, ctx, "experience_id", expIDs)
	if err != nil {
		logger.Warn("failed to batch fetch RSVPs", "error", err)
		allRSVPs = nil
	}
	rsvpsByExp := make(map[string][]*models.ExperienceRSVP, len(experiences))
	for _, rsvp := range allRSVPs {
		rsvpsByExp[rsvp.ExperienceId] = append(rsvpsByExp[rsvp.ExperienceId], rsvp)
	}

	// Step 5: Batch-fetch time proposals and group by experience ID; collect proposal IDs.
	allProposals, err := storage.QueryByFieldIn[*models.ExperienceTimeProposal](s.storage, ctx, "experience_id", expIDs)
	if err != nil {
		logger.Warn("failed to batch fetch time proposals", "error", err)
		allProposals = nil
	}
	proposalsByExp := make(map[string][]*models.ExperienceTimeProposal, len(experiences))
	proposalIDs := make([]string, 0, len(allProposals))
	for _, p := range allProposals {
		proposalsByExp[p.ExperienceId] = append(proposalsByExp[p.ExperienceId], p)
		proposalIDs = append(proposalIDs, p.Id)
		ownerIDs = append(ownerIDs, p.ProposedByUserId)
	}

	// Step 6: Batch-fetch votes and group by proposal ID; collect voter IDs.
	var allVotes []*models.TimeVote
	if len(proposalIDs) > 0 {
		allVotes, err = storage.QueryByFieldIn[*models.TimeVote](s.storage, ctx, "proposal_id", proposalIDs)
		if err != nil {
			logger.Warn("failed to batch fetch time votes", "error", err)
			allVotes = nil
		}
	}
	votesByProposal := make(map[string][]*models.TimeVote, len(proposalIDs))
	for _, v := range allVotes {
		votesByProposal[v.ProposalId] = append(votesByProposal[v.ProposalId], v)
		ownerIDs = append(ownerIDs, v.UserId)
	}

	// Batch-fetch location proposals and group by experience ID; collect proposal IDs.
	allLocationProposals, err := storage.QueryByFieldIn[*models.ExperienceLocationProposal](s.storage, ctx, "experience_id", expIDs)
	if err != nil {
		logger.Warn("failed to batch fetch location proposals", "error", err)
		allLocationProposals = nil
	}
	locationProposalsByExp := make(map[string][]*models.ExperienceLocationProposal, len(experiences))
	locationProposalIDs := make([]string, 0, len(allLocationProposals))
	for _, p := range allLocationProposals {
		locationProposalsByExp[p.ExperienceId] = append(locationProposalsByExp[p.ExperienceId], p)
		locationProposalIDs = append(locationProposalIDs, p.Id)
		ownerIDs = append(ownerIDs, p.ProposedByUserId)
	}

	// Batch-fetch location votes and group by proposal ID; collect voter IDs.
	var allLocationVotes []*models.LocationVote
	if len(locationProposalIDs) > 0 {
		allLocationVotes, err = storage.QueryByFieldIn[*models.LocationVote](s.storage, ctx, "proposal_id", locationProposalIDs)
		if err != nil {
			logger.Warn("failed to batch fetch location votes", "error", err)
			allLocationVotes = nil
		}
	}
	locationVotesByProposal := make(map[string][]*models.LocationVote, len(locationProposalIDs))
	for _, v := range allLocationVotes {
		locationVotesByProposal[v.ProposalId] = append(locationVotesByProposal[v.ProposalId], v)
		ownerIDs = append(ownerIDs, v.UserId)
	}

	// Step 7: Batch-fetch all users (owners + proposers + voters).
	// Comment-preview senders are handled inside BatchEnrichWithCommentPreviewAndCounts.
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, dedupStrings(ownerIDs))
	if err != nil {
		return nil, connecterr.Internal(ctx, "buildAPIExperiences", err, "detail", "failed to batch fetch users")
	}

	// Step 8: Batch-fetch comment previews and message counts.
	authUserID := ""
	if authInfo, ok := auth.GetAuthInfo(ctx); ok {
		authUserID = authInfo.UserID
	}
	previewMap, countMap, _, err := conversation.BatchEnrichWithCommentPreviewAndCounts(ctx, s.storage, convIDs, authUserID)
	if err != nil {
		logger.Warn("failed to batch enrich comment previews", "error", err)
		previewMap = make(map[string]*conversation.CommentPreview)
		countMap = make(map[string]int32)
	}

	// Step 9: Assemble one *api.Experience per input experience.
	results := make([]*api.Experience, 0, len(experiences))
	for _, exp := range experiences {
		owner := userMap[exp.OwnerId]
		if owner == nil {
			logger.Warn("owner not found for experience, skipping",
				"experience_id", exp.Id, "owner_id", exp.OwnerId)
			continue
		}

		// Determine sharedCommunityIDs and createdAtUnixSec from CommunityExperience rows.
		ces := ceByExp[exp.Id]
		sharedCommunityIDs := make([]string, 0, len(ces))
		var createdAtUnixSec int64
		for _, ce := range ces {
			sharedCommunityIDs = append(sharedCommunityIDs, ce.CommunityId)
			if createdAtUnixSec == 0 {
				createdAtUnixSec = ce.SharedAtUnixSec
			}
		}

		// Resolve lat/lng from the pre-fetched location.
		var latitudeDeg, longitudeDeg float64
		if exp.LocationId != "" {
			if loc, ok := locMap[exp.LocationId]; ok && loc.Geolocation != nil {
				latitudeDeg = loc.Geolocation.LatitudeDeg
				longitudeDeg = loc.Geolocation.LongitudeDeg
			}
		}

		// Compute cross-community RSVP counts (dedup by user, most-recently-updated wins).
		var rsvpYesCount, rsvpMaybeCount int32
		{
			byUser := make(map[string]*models.ExperienceRSVP, len(rsvpsByExp[exp.Id]))
			for _, rsvp := range rsvpsByExp[exp.Id] {
				if existing, ok := byUser[rsvp.UserId]; !ok || rsvp.LastUpdatedUnixSec > existing.LastUpdatedUnixSec {
					byUser[rsvp.UserId] = rsvp
				}
			}
			for _, rsvp := range byUser {
				switch rsvp.GetIntention() {
				case models.RSVPIntention_RSVP_INTENTION_YES:
					rsvpYesCount++
				case models.RSVPIntention_RSVP_INTENTION_MAYBE:
					rsvpMaybeCount++
				}
			}
		}

		// Build time proposals from in-memory maps.
		timeProposals := buildTimeProposalsFromMaps(ctx, exp.Id, proposalsByExp, votesByProposal, userMap, logger)

		// Build location proposals from in-memory maps.
		locationProposals := buildLocationProposalsFromMaps(ctx, exp.Id, locationProposalsByExp, locationVotesByProposal, userMap, logger)

		// Attach comment preview and message count.
		conversationID := exp.ConversationId
		var messageCount int32
		if conversationID != "" {
			messageCount = countMap[conversationID]
		}

		result := &api.Experience{
			Id:                          exp.Id,
			Name:                        exp.Name,
			Description:                 exp.Description,
			SourceUrl:                   exp.SourceUrl,
			Owner:                       owner,
			MediaIds:                    exp.MediaIds,
			LocationId:                  exp.LocationId,
			ConversationId:              &conversationID,
			LatitudeDeg:                 latitudeDeg,
			LongitudeDeg:                longitudeDeg,
			State:                       convertExperienceState(exp.State),
			Time:                        services.ConvertTimeModelsToAPI(exp.Time),
			MaxParticipants:             exp.MaxParticipants,
			RsvpYesCount:                rsvpYesCount,
			RsvpMaybeCount:              rsvpMaybeCount,
			MessageCount:                messageCount,
			StartedAtUnixSec:            exp.StartedAtUnixSec,
			CompletedAtUnixSec:          exp.CompletedAtUnixSec,
			CompletionSummary:           exp.CompletionSummary,
			SharedCommunityIds:          sharedCommunityIDs,
			CreatedAtUnixSec:            createdAtUnixSec,
			TimeProposals:               timeProposals,
			TimePollActive:              exp.TimePollActive,
			TimePollCompleted:           exp.TimePollCompleted,
			CurrentPollId:               exp.CurrentPollId,
			LocationProposals:           locationProposals,
			LocationPollActive:          exp.LocationPollActive,
			LocationPollCompleted:       exp.LocationPollCompleted,
			CurrentLocationPollId:       exp.CurrentLocationPollId,
			LocationPollDeadlineUnixSec: exp.LocationPollDeadlineUnixSec,
			LocationProposalsLocked:     exp.LocationProposalsLocked,
			TimePollDeadlineUnixSec:     exp.TimePollDeadlineUnixSec,
			TimeProposalsLocked:         exp.TimeProposalsLocked,
			Suggestions:                 exp.Suggestions,
			CategoryHint:                exp.CategoryHint,
		}

		// Attach comment preview fields if conversation exists and user is authenticated.
		if conversationID != "" {
			if preview := previewMap[conversationID]; preview != nil {
				result.UnreadCount = preview.UnreadCount
				result.LastMessageText = preview.LastMessageText
				result.LastMessageSender = preview.LastMessageSender
				result.LastMessageTimeAgo = preview.LastMessageTimeAgo
				result.RecentCommenters = preview.RecentCommenters
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// dedupStrings returns a deduplicated slice preserving order.
func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
