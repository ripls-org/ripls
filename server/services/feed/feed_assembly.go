package feed

import (
	"context"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// Batch loading for generateFeedItems.
//
// generateFeedItems was 438 lines and cyclomatic complexity 97: one pass to
// collect IDs off the events, then fourteen batch fetches hanging off those
// IDs, then the two loops that turn events and stories into feed items. The
// fetches all share the same shape — skip when there is nothing to ask for, log
// and degrade on failure — so each is now its own loader and the RPC reads as
// collect, load, assemble (#2816).
//
// Every loader here is best-effort by design: a feed that renders without offer
// counts is better than no feed. The two fetches the feed genuinely cannot do
// without — the events themselves and the view records — stay in
// generateFeedItems and return errors.

// feedEntityIDs is what one pass over the events and stories yields: the IDs
// every downstream batch fetch keys off.
type feedEntityIDs struct {
	eventIDs      []string
	storyIDs      []string
	userIDs       []string
	gearIDs       []string
	requestIDs    []string
	experienceIDs []string
}

// collectFeedEntityIDs walks the events once, routing each to the entity its
// card needs resolved. Creation and lifecycle events for the same entity type
// land in the same bucket — a transfer event needs the gear card just as a
// gear-shared event does.
func collectFeedEntityIDs(events []*models.CommunityEvent, stories []*models.Story) *feedEntityIDs {
	ids := &feedEntityIDs{
		eventIDs: make([]string, 0, len(events)),
		storyIDs: make([]string, 0, len(stories)),
		userIDs:  make([]string, 0, len(events)+len(stories)*3),
	}

	for _, event := range events {
		ids.eventIDs = append(ids.eventIDs, event.Id)
		ids.userIDs = append(ids.userIDs, event.ActorId)

		switch event.EventType {
		// Creation and transfer-lifecycle events both render a gear card.
		case models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE:
			if event.GearId != "" {
				ids.gearIDs = append(ids.gearIDs, event.GearId)
			}
		case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE:
			if id := event.GetRequestId(); id != "" {
				ids.requestIDs = append(ids.requestIDs, id)
			}
		case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE,
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO:
			if id := event.GetExperienceId(); id != "" {
				ids.experienceIDs = append(ids.experienceIDs, id)
			}
		}
	}

	for _, story := range stories {
		ids.storyIDs = append(ids.storyIDs, story.Id)
		ids.userIDs = append(ids.userIDs, story.ParticipantIds...)
	}
	return ids
}

// loadFeedLookups performs every batch fetch the card builders read from, and
// returns them as the lookup context those builders take.
//
// Only the user fetch is fatal: without it no card can name an actor. The rest
// degrade to an empty map, which the builders already handle.
func (s *Service) loadFeedLookups(
	ctx context.Context, logger *logging.Logger,
	userID, communityID string, ids *feedEntityIDs,
) (*feedLookups, error) {
	gearMap := s.batchGear(ctx, logger, ids.gearIDs)
	requestMap := s.batchRequests(ctx, logger, ids.requestIDs)
	experienceMap := s.batchExperiences(ctx, logger, ids.experienceIDs)

	// The owner and requester of a fetched entity also need naming, and they are
	// only known once the entity is in hand — hence the second collection pass.
	userIDs := ids.userIDs
	for _, g := range gearMap {
		userIDs = append(userIDs, g.OwnerId)
	}
	for _, r := range requestMap {
		userIDs = append(userIDs, r.RequesterId)
	}
	for _, e := range experienceMap {
		userIDs = append(userIDs, e.OwnerId)
	}
	userMap, err := services.FetchAPIUsersBatch(ctx, s.sqlStorage, userIDs)
	if err != nil {
		return nil, err
	}

	offerCounts, viewerOffered := s.batchOfferState(ctx, logger, userID, ids.requestIDs)
	rsvpYes, rsvpMaybe, viewerRSVP := s.batchRSVPState(ctx, logger, userID, ids.experienceIDs)

	return &feedLookups{
		userMap:                userMap,
		gearMap:                gearMap,
		requestMap:             requestMap,
		experienceMap:          experienceMap,
		communityGearMap:       communityGearsFor(s, ctx, logger, communityID, ids.gearIDs),
		communityRequestMap:    communityRequestsFor(s, ctx, logger, communityID, ids.requestIDs),
		communityExperienceMap: communityExperiencesFor(s, ctx, logger, communityID, ids.experienceIDs),
		locationMap:            s.batchFeedLocations(ctx, logger, requestMap, experienceMap),
		offerCountMap:          offerCounts,
		pendingInterestMap:     s.batchPendingInterest(ctx, logger, ids.gearIDs),
		rsvpYesCountMap:        rsvpYes,
		rsvpMaybeCountMap:      rsvpMaybe,
		viewerRSVPMap:          viewerRSVP,
		viewerOfferedSet:       viewerOffered,
	}, nil
}

// batchGear resolves the gear behind gear and transfer events.
func (s *Service) batchGear(ctx context.Context, logger *logging.Logger, gearIDs []string) map[string]*models.Gear {
	gearMap, err := storage.GetByIDs[*models.Gear](s.sqlStorage, ctx, gearIDs)
	if err != nil {
		logger.Error("failed to batch fetch gear for feed", "error", err)
		return nil
	}
	return gearMap
}

// batchRequests resolves the requests behind request events.
func (s *Service) batchRequests(ctx context.Context, logger *logging.Logger, requestIDs []string) map[string]*models.Request {
	requestMap, err := storage.GetByIDs[*models.Request](s.sqlStorage, ctx, requestIDs)
	if err != nil {
		logger.Error("failed to batch fetch requests for feed", "error", err)
		return nil
	}
	return requestMap
}

// batchExperiences resolves the experiences behind experience events.
func (s *Service) batchExperiences(ctx context.Context, logger *logging.Logger, experienceIDs []string) map[string]*models.Experience {
	experienceMap, err := storage.GetByIDs[*models.Experience](s.sqlStorage, ctx, experienceIDs)
	if err != nil {
		logger.Error("failed to batch fetch experiences for feed", "error", err)
		return nil
	}
	return experienceMap
}

// communityScopedPivot is what the pivot loader needs from a community join
// row: the community it belongs to.
type communityScopedPivot[E any] interface {
	storage.ProtoMessage[E]
	GetCommunityId() string
}

// communityPivot batch-fetches the join rows for one entity type and keeps the
// ones belonging to the community being rendered, keyed by entity ID.
//
// The three pivots differ only in the stored type, the field to query, and
// which ID keys the result; the community filter and the degrade-on-failure
// behaviour are the same for all three.
func communityPivot[T communityScopedPivot[E], E any](
	s *Service, ctx context.Context, logger *logging.Logger,
	communityID, field, label string, ids []string,
	key func(T) string,
) map[string]T {
	out := make(map[string]T)
	if len(ids) == 0 {
		return out
	}
	records, err := storage.QueryByFieldIn[T, E](s.sqlStorage, ctx, field, ids)
	if err != nil {
		logger.Error("failed to batch fetch "+label+" for feed", "error", err)
		return out
	}
	for _, rec := range records {
		if rec.GetCommunityId() == communityID {
			out[key(rec)] = rec
		}
	}
	return out
}

func communityGearsFor(
	s *Service, ctx context.Context, logger *logging.Logger, communityID string, gearIDs []string,
) map[string]*models.CommunityGear {
	return communityPivot(s, ctx, logger, communityID, "gear_id", "community gear", gearIDs,
		(*models.CommunityGear).GetGearId)
}

func communityRequestsFor(
	s *Service, ctx context.Context, logger *logging.Logger, communityID string, requestIDs []string,
) map[string]*models.CommunityRequest {
	return communityPivot(s, ctx, logger, communityID, "request_id", "community requests", requestIDs,
		(*models.CommunityRequest).GetRequestId)
}

func communityExperiencesFor(
	s *Service, ctx context.Context, logger *logging.Logger, communityID string, experienceIDs []string,
) map[string]*models.CommunityExperience {
	return communityPivot(s, ctx, logger, communityID, "experience_id", "community experiences", experienceIDs,
		(*models.CommunityExperience).GetExperienceId)
}

// batchFeedLocations resolves the place names shown on request and experience
// cards.
func (s *Service) batchFeedLocations(
	ctx context.Context, logger *logging.Logger,
	requestMap map[string]*models.Request, experienceMap map[string]*models.Experience,
) map[string]*models.Location {
	locationIDs := make([]string, 0, len(requestMap)+len(experienceMap))
	for _, r := range requestMap {
		if r.LocationId != "" {
			locationIDs = append(locationIDs, r.LocationId)
		}
	}
	for _, e := range experienceMap {
		if e.LocationId != "" {
			locationIDs = append(locationIDs, e.LocationId)
		}
	}
	if len(locationIDs) == 0 {
		return make(map[string]*models.Location)
	}
	locationMap, err := storage.GetByIDs[*models.Location](s.sqlStorage, ctx, locationIDs)
	if err != nil {
		logger.Error("failed to batch fetch locations for feed", "error", err)
		return make(map[string]*models.Location)
	}
	return locationMap
}

// batchOfferState returns the offer count per request and the set of requests
// the viewer has offered on.
//
// The returned set stays nil on failure so the payload reports "unknown" rather
// than "has not offered" (#2800).
func (s *Service) batchOfferState(
	ctx context.Context, logger *logging.Logger, userID string, requestIDs []string,
) (map[string]int32, map[string]bool) {
	offerCounts := make(map[string]int32)
	if len(requestIDs) == 0 {
		return offerCounts, nil
	}
	offers, err := storage.QueryByFieldIn[*models.RequestOffer](s.sqlStorage, ctx, "request_id", requestIDs)
	if err != nil {
		logger.Error("failed to batch fetch offers for feed", "error", err)
		return offerCounts, nil
	}

	viewerOffered := make(map[string]bool, len(requestIDs))
	for _, offer := range offers {
		if offer.Withdrawn {
			continue
		}
		offerCounts[offer.RequestId]++
		if offer.UserId == userID {
			viewerOffered[offer.RequestId] = true
		}
	}
	return offerCounts, viewerOffered
}

// batchPendingInterest counts, per gear, the transfers sitting in
// INTEREST_EXPRESSED — someone wants to borrow or receive it and the owner has
// not yet acted.
func (s *Service) batchPendingInterest(
	ctx context.Context, logger *logging.Logger, gearIDs []string,
) map[string]int32 {
	pending := make(map[string]int32)
	if len(gearIDs) == 0 {
		return pending
	}
	transfers, err := storage.QueryByFieldIn[*models.Transfer](s.sqlStorage, ctx, "gear_id", gearIDs)
	if err != nil {
		logger.Error("failed to batch fetch transfers for feed", "error", err)
		return pending
	}
	for _, tr := range transfers {
		if tr.State == models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED {
			pending[tr.GearId]++
		}
	}
	return pending
}

// batchRSVPState returns the yes and maybe counts per experience and the
// viewer's own intention per experience.
//
// The viewer map stays nil on failure so the payload reports "unknown" rather
// than "has not responded" (#2800).
func (s *Service) batchRSVPState(
	ctx context.Context, logger *logging.Logger, userID string, experienceIDs []string,
) (yes, maybe map[string]int32, viewer map[string]api.FeedRSVPIntention) {
	yes = make(map[string]int32)
	maybe = make(map[string]int32)
	if len(experienceIDs) == 0 {
		return yes, maybe, nil
	}
	rsvps, err := storage.QueryByFieldIn[*models.ExperienceRSVP](s.sqlStorage, ctx, "experience_id", experienceIDs)
	if err != nil {
		logger.Error("failed to batch fetch RSVPs for feed", "error", err)
		return yes, maybe, nil
	}

	viewer = make(map[string]api.FeedRSVPIntention, len(experienceIDs))
	for _, rsvp := range rsvps {
		switch rsvp.GetIntention() {
		case models.RSVPIntention_RSVP_INTENTION_YES:
			yes[rsvp.ExperienceId]++
		case models.RSVPIntention_RSVP_INTENTION_MAYBE:
			maybe[rsvp.ExperienceId]++
		}
		if rsvp.UserId == userID {
			viewer[rsvp.ExperienceId] = rsvpIntentionToAPI(rsvp.GetIntention())
		}
	}
	return yes, maybe, viewer
}

// feedConversations resolves, per event, the conversation whose latest
// non-viewer message counts as activity on that event's item.
type feedConversations struct {
	gearConvID  map[string]string                   // gear_id -> conversation_id
	requestConv map[string]*models.ChatConversation // request_id -> conversation
	expConv     map[string]*models.ChatConversation // experience_id -> conversation
	latestMsgAt map[string]int64                    // conversation_id -> unix sec
}

// activityAt returns the timestamp of the latest message another member sent on
// this event's item, or zero when there is no conversation or no such message.
func (fc *feedConversations) activityAt(event *models.CommunityEvent) int64 {
	var convID string
	switch {
	case event.GearId != "":
		convID = fc.gearConvID[event.GearId]
	default:
		if reqID := event.GetRequestId(); reqID != "" {
			if conv, ok := fc.requestConv[reqID]; ok {
				convID = conv.Id
			}
		} else if expID := event.GetExperienceId(); expID != "" {
			if conv, ok := fc.expConv[expID]; ok {
				convID = conv.Id
			}
		}
	}
	return fc.latestMsgAt[convID]
}

// loadFeedConversations resolves the conversation behind each item and then
// batch-fetches the latest non-viewer message timestamp across all of them in
// one query. Best-effort throughout: without it, items fall back to their event
// time as their last activity.
func (s *Service) loadFeedConversations(
	ctx context.Context, logger *logging.Logger,
	userID string, ids *feedEntityIDs, gearMap map[string]*models.Gear,
	communityGearMap map[string]*models.CommunityGear,
) *feedConversations {
	conv := &feedConversations{
		gearConvID:  make(map[string]string),
		requestConv: make(map[string]*models.ChatConversation),
		expConv:     make(map[string]*models.ChatConversation),
		latestMsgAt: make(map[string]int64),
	}
	allConvIDs := make([]string, 0)

	// Gear.conversation_id is canonical; fall back to a ChatConversation query
	// only for the gear it does not cover.
	for gearID := range communityGearMap {
		if g, ok := gearMap[gearID]; ok && g.ConversationId != "" {
			conv.gearConvID[gearID] = g.ConversationId
			allConvIDs = append(allConvIDs, g.ConversationId)
		}
	}
	unresolvedGearIDs := make([]string, 0)
	for _, gearID := range ids.gearIDs {
		if _, ok := conv.gearConvID[gearID]; !ok {
			unresolvedGearIDs = append(unresolvedGearIDs, gearID)
		}
	}
	if len(unresolvedGearIDs) > 0 {
		gearConvs, err := storage.QueryByFieldIn[*models.ChatConversation](s.sqlStorage, ctx, "topic_gear_id", unresolvedGearIDs)
		if err != nil {
			logger.Warn("failed to batch fetch gear conversations for feed", "error", err)
		} else {
			for _, c := range gearConvs {
				if gearID := c.GetTopic().GetGearId(); gearID != "" {
					conv.gearConvID[gearID] = c.Id
					allConvIDs = append(allConvIDs, c.Id)
				}
			}
		}
	}

	if len(ids.requestIDs) > 0 {
		reqConvs, err := storage.QueryByFieldIn[*models.ChatConversation](s.sqlStorage, ctx, "topic_request_id", ids.requestIDs)
		if err != nil {
			logger.Warn("failed to batch fetch request conversations for feed", "error", err)
		} else {
			for _, c := range reqConvs {
				conv.requestConv[c.GetTopic().GetRequestId()] = c
				allConvIDs = append(allConvIDs, c.Id)
			}
		}
	}

	if len(ids.experienceIDs) > 0 {
		expConvs, err := storage.QueryByFieldIn[*models.ChatConversation](s.sqlStorage, ctx, "topic_experience_id", ids.experienceIDs)
		if err != nil {
			logger.Warn("failed to batch fetch experience conversations for feed", "error", err)
		} else {
			for _, c := range expConvs {
				conv.expConv[c.GetTopic().GetExperienceId()] = c
				allConvIDs = append(allConvIDs, c.Id)
			}
		}
	}

	if len(allConvIDs) > 0 {
		latest, err := s.feedStorage.GetLatestNonViewerMessageAtBatch(ctx, allConvIDs, userID)
		if err != nil {
			logger.Warn("failed to batch fetch message timestamps for feed", "error", err)
		} else {
			conv.latestMsgAt = latest
		}
	}
	return conv
}

// feedExpiry reads the community's staleness thresholds, falling back to the
// defaults when unset.
func feedExpiry(community *models.Community) (views, hours int32) {
	const defaultExpiryViews = 2
	const defaultExpiryHours = 24

	views, hours = defaultExpiryViews, defaultExpiryHours
	if community.FeedExpiryViews > 0 {
		views = community.FeedExpiryViews
	}
	if community.FeedExpiryHours > 0 {
		hours = community.FeedExpiryHours
	}
	return views, hours
}
