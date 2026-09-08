package portfolio

import (
	"context"

	"google.golang.org/protobuf/proto"

	convstate "go.ripls.org/ripls/server/conversation"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
)

// fetchedData holds all raw data retrieved by fetchAll for a given user.
// Multiple portfolio RPCs (inbox feed, inbox view, etc.) use this shared dataset.
type fetchedData struct {
	// Community membership.
	communityIDs      []string
	communityNameMap  map[string]string
	communityMediaMap map[string]string // community_id -> first media_id

	// Transfer lists: active (non-terminal) and all (including completed/cancelled).
	activeTransfers []*models.Transfer
	allTransfers    []*models.Transfer

	// Experience data.
	commExpRaw      []proto.Message
	commExpMap      map[string]*models.CommunityExperience // non-archived only, keyed by experience_id
	expMap          map[string]proto.Message
	activeExpIDs    []string
	expAttendeesMap map[string][]string
	userRSVPTimes   map[string]int64
	// peopleRSVPTimes maps user_id → their latest RSVP time across visible
	// experiences (#2568 directory recency signal).
	peopleRSVPTimes map[string]int64
	// userRSVPIntentions maps experience_id → the viewer's RSVP intention, so
	// the home subtitle can distinguish Going vs Maybe vs Invited. Absent key
	// means no RSVP (and reads as UNSPECIFIED).
	userRSVPIntentions map[string]models.RSVPIntention
	// invitedExpIDs are experiences the viewer was directly invited to — they
	// are a member of the experience's per-item (origin) community. Drives
	// home/calendar inclusion for invitees who haven't responded yet (#2492).
	invitedExpIDs map[string]bool
	// expActivityGroups partitions the fetched experiences (expMap keys) into
	// semantically-similar activity groups by embedding similarity, so the
	// open-day suggestion engine can treat near-but-not-identical events as the
	// same recurring activity (#2674). Nil/empty when no embedder is configured
	// or grouping failed — viewerActivities then falls back to exact-name keying.
	expActivityGroups [][]string

	// Request data.
	commReqRaw    []proto.Message
	commReqMap    map[string]*models.CommunityRequest // non-archived only, keyed by request_id
	reqMap        map[string]proto.Message
	activeReqIDs  []string
	reqOffererIDs map[string][]string // request_id → non-withdrawn offerer user IDs

	// Conversations and messages.
	transferConvMap  map[string]*models.ChatConversation // keyed by transfer_id
	convByID         map[string]*models.ChatConversation // keyed by conv_id
	messagesByConvID map[string][]proto.Message          // keyed by conv_id
	convUnreadCount  map[string]int32                    // keyed by conv_id

	// Gear and locations.
	gearProtoMap    map[string]proto.Message
	locationNameMap map[string]string

	// Community gear.
	commGearRaw         []proto.Message
	allCommunityGearMap map[string]proto.Message
	cgByGearID          map[string]*models.CommunityGear

	// Community membership milestone events (created, joined) for the user.
	communityMilestones []*models.CommunityEvent

	// Users.
	userMap map[string]*api.User

	// Ring data — used to render the access ring on inbox cards identically to
	// the content view. communityMemberCountMap maps community_id → member count.
	// communityMemberUserIDs maps community_id → []user_id for distinct-count dedup.
	// The three per-item maps key item_id → []community_id for all communities
	// (in the user's set) that have the item shared.
	communityMemberCountMap map[string]int32
	communityMemberUserIDs  map[string][]string
	gearCommunityIDs        map[string][]string // gear_id → community_ids
	expCommunityIDs         map[string][]string // experience_id → community_ids
	reqCommunityIDs         map[string][]string // request_id → community_ids

	// watchedExpIDs, watchedReqIDs, and watchedGearIDs are the sets of item IDs
	// for which the user has an active (non-dismissed) watch. assembleFeedItems
	// uses these to bypass the normal message-count inclusion gate so that
	// explicitly watched items always appear in the user's inbox.
	watchedExpIDs  map[string]bool
	watchedReqIDs  map[string]bool
	watchedGearIDs map[string]bool

	// ownedGear, ownedExperiences, and ownedRequests hold every item the
	// viewer is the owner of — used to fill the "Your Stuff" library
	// view with items that have zero engagement (no transfer, no
	// non-owner RSVP, no offers, no messages, no community listing).
	// `assembleLibraryItems` emits rows for any owned item that the
	// engagement-driven loops in `assembleFeedItems` skipped.
	ownedGear        []*models.Gear
	ownedExperiences []*models.Experience
	ownedRequests    []*models.Request
}

// fetchAll performs all batch-fetch operations needed to assemble the portfolio
// inbox for the given user. It returns a fetchedData struct containing the raw
// data, or an error if any critical fetch fails.
//
// The order below is load-bearing and is the original eleven numbered sections
// of this function (#2816): transfers, experiences and requests all scope
// themselves to the viewer's communities; chat resolves conversations from the
// active items those produce; community-gear chat augments the message maps
// chat built; and the owned-item library merges into the gear, experience and
// request maps before locations and users are resolved from them.
func (s *Service) fetchAll(ctx context.Context, userID string) (*fetchedData, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "fetchAll",
		"user_id", userID,
	)

	d := newFetchedData()

	if err := s.fetchCommunities(ctx, logger, d, userID); err != nil {
		return nil, err
	}
	if len(d.communityIDs) == 0 {
		// Every section below is scoped to the viewer's communities, so with no
		// membership they all no-op. Return the initialised struct.
		return d, nil
	}

	if err := s.fetchTransfers(ctx, d, userID); err != nil {
		return nil, err
	}
	if err := s.fetchExperiences(ctx, logger, d, userID); err != nil {
		return nil, err
	}
	if err := s.fetchRequests(ctx, logger, d); err != nil {
		return nil, err
	}

	s.fetchChat(ctx, logger, d, userID)
	s.fetchTransferGear(ctx, logger, d)
	s.fetchCommunityGear(ctx, logger, d)
	s.loadCommunityGearChat(ctx, logger, d, userID)
	s.fetchRingData(ctx, logger, d)
	s.fetchOwnedLibrary(ctx, logger, d, userID)

	if err := s.fetchUsers(ctx, d, userID); err != nil {
		return nil, err
	}
	s.groupExperienceActivities(ctx, logger, d, userID)

	return d, nil
}

// fetchCommunityMilestones returns CommunityEvents where the user created or
// joined one of their communities. Events outside the user's current community
// set are excluded.
func fetchCommunityMilestones(
	ctx context.Context,
	s *Service,
	logger *logging.Logger,
	userID string,
	communityIDs []string,
) []*models.CommunityEvent {
	communitySet := make(map[string]bool, len(communityIDs))
	for _, id := range communityIDs {
		communitySet[id] = true
	}

	raw, err := s.storage.QueryByField(ctx, "actor_id", userID, &models.CommunityEvent{})
	if err != nil {
		logger.WarnContext(ctx, "failed to fetch community milestone events", "error", err)
		return nil
	}

	out := make([]*models.CommunityEvent, 0)
	for _, m := range raw {
		ev := m.(*models.CommunityEvent)
		if !communitySet[ev.CommunityId] {
			continue
		}
		if ev.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED &&
			ev.EventType != models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// buildConvUnreadCounts computes unread message counts for all conversations
// across transfers, experiences, and requests in a single pass.
// transferMap and reqMap are pre-fetched maps (keyed by ID) used to determine
// terminal state without issuing per-conversation database queries.
func buildConvUnreadCounts(
	convByID map[string]*models.ChatConversation,
	messagesByConvID map[string][]proto.Message,
	transferMap map[string]proto.Message,
	reqMap map[string]proto.Message,
	expMap map[string]proto.Message,
	activeExpIDs []string,
	activeReqIDs []string,
	userID string,
) map[string]int32 {
	convUnreadCount := make(map[string]int32)

	for convID, msgs := range messagesByConvID {
		conv := convByID[convID]
		if conv != nil && convstate.IsItemDoneFromMaps(conv, transferMap, reqMap) {
			continue
		}
		convUnreadCount[convID] = computeUnreadCount(msgs, userID)
	}

	for _, expID := range activeExpIDs {
		// Use the global experience conversation (same one the Chat tab uses).
		exp, ok := expMap[expID]
		if !ok {
			continue
		}
		convID := exp.(*models.Experience).ConversationId
		if convID == "" {
			continue
		}
		if _, done := convUnreadCount[convID]; !done {
			convUnreadCount[convID] = computeUnreadCount(messagesByConvID[convID], userID)
		}
	}
	for _, reqID := range activeReqIDs {
		req, ok := reqMap[reqID]
		if !ok {
			continue
		}
		convID := req.(*models.Request).ConversationId
		if convID == "" {
			continue
		}
		if _, done := convUnreadCount[convID]; !done {
			convUnreadCount[convID] = computeUnreadCount(messagesByConvID[convID], userID)
		}
	}
	return convUnreadCount
}

// batchFetchLocationNames fetches all location display names for gear, experiences,
// and requests in a single batch query.
func batchFetchLocationNames(
	ctx context.Context,
	s *Service,
	logger *logging.Logger,
	gearProtoMap map[string]proto.Message,
	expMap map[string]proto.Message,
	reqMap map[string]proto.Message,
) map[string]string {
	var locationIDs []string
	for _, m := range gearProtoMap {
		if locID := m.(*models.Gear).LocationId; locID != "" {
			locationIDs = append(locationIDs, locID)
		}
	}
	for _, m := range expMap {
		if locID := m.(*models.Experience).LocationId; locID != "" {
			locationIDs = append(locationIDs, locID)
		}
	}
	for _, m := range reqMap {
		if locID := m.(*models.Request).LocationId; locID != "" {
			locationIDs = append(locationIDs, locID)
		}
	}
	locationProtoMap, err := s.storage.GetByIDs(ctx, dedup(locationIDs), &models.Location{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch locations", "error", err)
		locationProtoMap = nil
	}
	locationNameMap := make(map[string]string, len(locationProtoMap))
	for locID, m := range locationProtoMap {
		locationNameMap[locID] = location.GenerateLocationDisplayName(m.(*models.Location))
	}
	return locationNameMap
}
