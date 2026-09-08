package portfolio

import (
	"context"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/rsvpstate"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// Sections of fetchAll.
//
// fetchAll was one 671-line function (298 statements, cyclomatic complexity
// 127 — the worst in the codebase) built from eleven numbered sections whose
// locals flowed into one another. Every value that crossed a section boundary
// already had a field on fetchedData, so each section is now a method that
// populates *fetchedData in place and fetchAll is the ordered list of them
// (#2816).
//
// Two rules hold across this file. Sections that the view cannot be assembled
// without return an error; everything else logs and degrades, matching what the
// original did per-query. And the maps on fetchedData are allocated once by
// newFetchedData, so a section fills them rather than assigning a fresh one —
// the exceptions are the maps that come straight back from a batch fetch.

// newFetchedData returns a fetchedData with every map allocated.
//
// fetchAll's no-community early return used to hand-build a zeroed struct that
// listed twenty of the forty-two fields and left the rest nil. Reads of a nil
// map work, so nothing was broken, but it was a standing trap for whoever next
// wrote to one, and it had to be kept in sync with the real return by hand.
func newFetchedData() *fetchedData {
	return &fetchedData{
		communityNameMap:        make(map[string]string),
		communityMediaMap:       make(map[string]string),
		commExpMap:              make(map[string]*models.CommunityExperience),
		expMap:                  make(map[string]proto.Message),
		expAttendeesMap:         make(map[string][]string),
		userRSVPTimes:           make(map[string]int64),
		peopleRSVPTimes:         make(map[string]int64),
		userRSVPIntentions:      make(map[string]models.RSVPIntention),
		invitedExpIDs:           make(map[string]bool),
		commReqMap:              make(map[string]*models.CommunityRequest),
		reqMap:                  make(map[string]proto.Message),
		reqOffererIDs:           make(map[string][]string),
		transferConvMap:         make(map[string]*models.ChatConversation),
		convByID:                make(map[string]*models.ChatConversation),
		messagesByConvID:        make(map[string][]proto.Message),
		convUnreadCount:         make(map[string]int32),
		gearProtoMap:            make(map[string]proto.Message),
		locationNameMap:         make(map[string]string),
		allCommunityGearMap:     make(map[string]proto.Message),
		cgByGearID:              make(map[string]*models.CommunityGear),
		userMap:                 make(map[string]*api.User),
		communityMemberCountMap: make(map[string]int32),
		communityMemberUserIDs:  make(map[string][]string),
		gearCommunityIDs:        make(map[string][]string),
		expCommunityIDs:         make(map[string][]string),
		reqCommunityIDs:         make(map[string][]string),
		watchedExpIDs:           make(map[string]bool),
		watchedReqIDs:           make(map[string]bool),
		watchedGearIDs:          make(map[string]bool),
	}
}

// fetchCommunities loads the viewer's community memberships plus the display
// data hanging off them and their milestone events. Membership scopes every
// later section, so this runs first and a failure is fatal to the view.
func (s *Service) fetchCommunities(
	ctx context.Context, logger *logging.Logger, d *fetchedData, userID string,
) error {
	membershipRaw, err := s.storage.QueryByField(ctx, "user_id", userID, &models.CommunityUser{})
	if err != nil {
		return connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to query user communities")
	}

	communityIDs := make([]string, 0, len(membershipRaw))
	for _, m := range membershipRaw {
		if cu := m.(*models.CommunityUser); cu.CommunityId != "" {
			communityIDs = append(communityIDs, cu.CommunityId)
		}
	}
	d.communityIDs = dedup(communityIDs)

	if len(d.communityIDs) > 0 {
		commProtoMap, commErr := s.storage.GetByIDs(ctx, d.communityIDs, &models.Community{})
		if commErr != nil {
			logger.WarnContext(ctx, "failed to fetch community names", "error", commErr)
		} else {
			for id, m := range commProtoMap {
				c := m.(*models.Community)
				d.communityNameMap[id] = c.Name
				d.communityMediaMap[id] = firstMediaID(c.MediaIds)
				// A community with an origin experience is that experience's
				// per-item community, so belonging to it means the viewer was
				// directly invited to the event. Used below to surface invited
				// events on home/calendar before the invitee responds (#2492).
				if oxid := c.GetOriginExperienceId(); oxid != "" {
					d.invitedExpIDs[oxid] = true
				}
			}
		}
	}

	d.communityMilestones = fetchCommunityMilestones(ctx, s, logger, userID, d.communityIDs)
	return nil
}

// fetchTransfers loads every transfer the viewer owns or received, keeps the
// ones in a community they belong to, and splits them into the active
// (non-terminal) and complete lists.
func (s *Service) fetchTransfers(ctx context.Context, d *fetchedData, userID string) error {
	transfersAsOwnerRaw, err := s.storage.QueryByField(ctx, "owner_id", userID, &models.Transfer{})
	if err != nil {
		return connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to query transfers as owner")
	}
	transfersAsRecipientRaw, err := s.storage.QueryByField(ctx, "recipient_id", userID, &models.Transfer{})
	if err != nil {
		return connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to query transfers as recipient")
	}

	communitySet := make(map[string]bool, len(d.communityIDs))
	for _, id := range d.communityIDs {
		communitySet[id] = true
	}
	inCommunity := func(raw []proto.Message) []proto.Message {
		var out []proto.Message
		for _, m := range raw {
			if communitySet[m.(*models.Transfer).CommunityId] {
				out = append(out, m)
			}
		}
		return out
	}

	filteredOwner := inCommunity(transfersAsOwnerRaw)
	filteredRecipient := inCommunity(transfersAsRecipientRaw)

	d.activeTransfers = deduplicateAndFilterTransfers(filteredOwner, filteredRecipient)
	d.allTransfers = deduplicateAllTransfers(filteredOwner, filteredRecipient)
	return nil
}

// viewerRSVPs is the viewer's own RSVP state, split into the two sets the
// active-experience filter consults. Only the parts that outlive the filter —
// the raw intention and the RSVP time — live on fetchedData.
type viewerRSVPs struct {
	attending map[string]bool // intention YES or MAYBE
	declined  map[string]bool // intention NO
}

// fetchExperiences loads the experiences shared into the viewer's communities,
// narrows them to the ones the viewer is connected to, and loads the attendee
// lists for those. The community-experience pivot and the experiences
// themselves are required; watches and attendees degrade.
func (s *Service) fetchExperiences(
	ctx context.Context, logger *logging.Logger, d *fetchedData, userID string,
) error {
	commExpRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", d.communityIDs, &models.CommunityExperience{})
	if err != nil {
		return connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to query community experiences")
	}
	d.commExpRaw = commExpRaw

	expIDs := make([]string, 0, len(commExpRaw))
	for _, m := range commExpRaw {
		if ce := m.(*models.CommunityExperience); !ce.Archived {
			d.commExpMap[ce.ExperienceId] = ce
			expIDs = append(expIDs, ce.ExperienceId)
		}
	}

	expMap, err := s.storage.GetByIDs(ctx, dedup(expIDs), &models.Experience{})
	if err != nil {
		return connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to fetch experiences")
	}
	d.expMap = expMap

	rsvps, err := s.loadViewerRSVPs(ctx, d, userID)
	if err != nil {
		return err
	}
	s.loadWatches(ctx, logger, d, userID)
	d.activeExpIDs = activeExperienceIDs(d, rsvps, userID)
	s.loadExperienceAttendees(ctx, logger, d)
	return nil
}

// loadViewerRSVPs records the viewer's intention and RSVP time per experience
// and returns the attending/declined split that decides which experiences count
// as theirs.
func (s *Service) loadViewerRSVPs(ctx context.Context, d *fetchedData, userID string) (viewerRSVPs, error) {
	raw, err := s.storage.QueryByField(ctx, "user_id", userID, &models.ExperienceRSVP{})
	if err != nil {
		return viewerRSVPs{}, connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to query user RSVPs")
	}

	rsvps := viewerRSVPs{
		attending: make(map[string]bool),
		declined:  make(map[string]bool),
	}
	for _, m := range raw {
		rsvp := m.(*models.ExperienceRSVP)
		d.userRSVPIntentions[rsvp.ExperienceId] = rsvp.GetIntention()
		switch rsvp.GetIntention() {
		case models.RSVPIntention_RSVP_INTENTION_YES, models.RSVPIntention_RSVP_INTENTION_MAYBE:
			rsvps.attending[rsvp.ExperienceId] = true
			d.userRSVPTimes[rsvp.ExperienceId] = rsvp.RsvpedAtUnixSec
		case models.RSVPIntention_RSVP_INTENTION_NO:
			rsvps.declined[rsvp.ExperienceId] = true
		}
	}
	return rsvps, nil
}

// loadWatches fills the three watched-item sets. An explicit watch means the
// item should always appear in the user's inbox regardless of RSVP or
// message-count rules, so assembleFeedItems reads these to bypass the normal
// inclusion gate. Best-effort: on failure nothing is watch-expanded.
func (s *Service) loadWatches(ctx context.Context, logger *logging.Logger, d *fetchedData, userID string) {
	activeWatches, err := s.watchStorage.GetActiveWatches(ctx, userID)
	if err != nil {
		logger.WarnContext(ctx, "failed to fetch active watches for inbox expansion", "error", err)
		return
	}
	for _, w := range activeWatches {
		switch w.ItemType {
		case models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE:
			d.watchedExpIDs[w.ItemId] = true
		case models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST:
			d.watchedReqIDs[w.ItemId] = true
		case models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR:
			d.watchedGearIDs[w.ItemId] = true
		}
	}
}

// activeExperienceIDs picks the experiences the viewer should see: neither
// terminal nor deleted, and connected to them as owner, attendee, watcher, or
// direct invitee who has not declined. The invited case is what puts an event
// on an invitee's home and calendar before they respond (#2492).
func activeExperienceIDs(d *fetchedData, rsvps viewerRSVPs, userID string) []string {
	var activeExpIDs []string
	for expID, msg := range d.expMap {
		e := msg.(*models.Experience)
		if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED ||
			e.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			continue
		}
		if e.Deleted != nil && e.Deleted.DeletedAtUnixSec > 0 {
			continue
		}
		if e.OwnerId == userID || rsvps.attending[expID] || d.watchedExpIDs[expID] ||
			(d.invitedExpIDs[expID] && !rsvps.declined[expID]) {
			activeExpIDs = append(activeExpIDs, expID)
		}
	}
	return activeExpIDs
}

// loadExperienceAttendees fills the per-experience attendee lists and, from the
// same rows, each person's latest RSVP time — the max of first RSVP and last
// intention change — which feeds the directory's recency signal (#2568).
func (s *Service) loadExperienceAttendees(ctx context.Context, logger *logging.Logger, d *fetchedData) {
	raw, err := s.storage.QueryByFieldIn(ctx, "experience_id", d.activeExpIDs, &models.ExperienceRSVP{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch RSVPs for experiences", "error", err)
		return
	}
	for _, m := range raw {
		rsvp := m.(*models.ExperienceRSVP)
		if rsvpstate.IsGoing(rsvp.GetIntention()) {
			d.expAttendeesMap[rsvp.ExperienceId] = append(d.expAttendeesMap[rsvp.ExperienceId], rsvp.UserId)
		}
		ts := rsvp.RsvpedAtUnixSec
		if rsvp.LastUpdatedUnixSec > ts {
			ts = rsvp.LastUpdatedUnixSec
		}
		if ts > d.peopleRSVPTimes[rsvp.UserId] {
			d.peopleRSVPTimes[rsvp.UserId] = ts
		}
	}
}

// fetchRequests loads the requests shared into the viewer's communities, the
// non-terminal subset, and the offers on those so the inbox can show offerer
// counts and avatars. The pivot and the requests are required; offers degrade.
func (s *Service) fetchRequests(ctx context.Context, logger *logging.Logger, d *fetchedData) error {
	commReqRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", d.communityIDs, &models.CommunityRequest{})
	if err != nil {
		return connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to query community requests")
	}
	d.commReqRaw = commReqRaw

	reqIDs := make([]string, 0, len(commReqRaw))
	for _, m := range commReqRaw {
		if cr := m.(*models.CommunityRequest); !cr.Archived {
			d.commReqMap[cr.RequestId] = cr
			reqIDs = append(reqIDs, cr.RequestId)
		}
	}

	reqMap, err := s.storage.GetByIDs(ctx, dedup(reqIDs), &models.Request{})
	if err != nil {
		return connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to fetch requests")
	}
	d.reqMap = reqMap

	for reqID, msg := range d.reqMap {
		r := msg.(*models.Request)
		if r.State == models.RequestState_REQUEST_STATE_FULFILLED ||
			r.State == models.RequestState_REQUEST_STATE_CANCELLED {
			continue
		}
		if r.Deleted != nil && r.Deleted.DeletedAtUnixSec > 0 {
			continue
		}
		d.activeReqIDs = append(d.activeReqIDs, reqID)
	}

	if len(d.activeReqIDs) == 0 {
		return nil
	}
	offersRaw, offErr := s.storage.QueryByFieldIn(ctx, "request_id", d.activeReqIDs, &models.RequestOffer{})
	if offErr != nil {
		logger.WarnContext(ctx, "failed to fetch request offers", "error", offErr)
		return nil
	}
	for _, m := range offersRaw {
		if offer := m.(*models.RequestOffer); !offer.Withdrawn {
			d.reqOffererIDs[offer.RequestId] = append(d.reqOffererIDs[offer.RequestId], offer.UserId)
		}
	}
	return nil
}

// loadTransferConversations resolves the conversation attached to each active
// transfer. Best-effort: without it those transfers simply show no chat.
func (s *Service) loadTransferConversations(ctx context.Context, logger *logging.Logger, d *fetchedData) {
	transferIDs := make([]string, 0, len(d.activeTransfers))
	for _, t := range d.activeTransfers {
		transferIDs = append(transferIDs, t.Id)
	}
	if len(transferIDs) == 0 {
		return
	}

	convRaw, err := s.storage.QueryByFieldIn(ctx, "topic_transfer_id", transferIDs, &models.ChatConversation{})
	if err != nil {
		logger.WarnContext(ctx, "failed to fetch transfer conversations", "error", err)
		return
	}
	for _, m := range convRaw {
		conv := m.(*models.ChatConversation)
		if tid := conv.GetTopic().GetTransferId(); tid != "" {
			d.transferConvMap[tid] = conv
		}
	}
}

// fetchChat collects the conversations behind the viewer's active transfers,
// experiences and requests, then loads every message across them in one batch
// and derives the per-conversation unread counts.
func (s *Service) fetchChat(ctx context.Context, logger *logging.Logger, d *fetchedData, userID string) {
	s.loadTransferConversations(ctx, logger, d)

	allConvIDs := make([]string, 0, len(d.activeTransfers)+len(d.activeExpIDs)+len(d.activeReqIDs))
	for _, t := range d.activeTransfers {
		if conv := d.transferConvMap[t.Id]; conv != nil {
			allConvIDs = append(allConvIDs, conv.Id)
			d.convByID[conv.Id] = conv
		}
	}
	for _, expID := range d.activeExpIDs {
		// Use the global experience conversation (same one the Chat tab uses) so
		// that inbox message previews and unread counts reflect the actual chat.
		if exp, ok := d.expMap[expID]; ok {
			if convID := exp.(*models.Experience).ConversationId; convID != "" {
				allConvIDs = append(allConvIDs, convID)
			}
		}
	}
	for _, reqID := range d.activeReqIDs {
		if msg, ok := d.reqMap[reqID]; ok {
			if convID := msg.(*models.Request).ConversationId; convID != "" {
				allConvIDs = append(allConvIDs, convID)
			}
		}
	}

	allMessagesRaw, err := s.storage.QueryByFieldIn(ctx, "conversation_id", dedup(allConvIDs), &models.ChatMessage{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch messages", "error", err)
		allMessagesRaw = nil
	}
	for _, m := range allMessagesRaw {
		msg := m.(*models.ChatMessage)
		d.messagesByConvID[msg.ConversationId] = append(d.messagesByConvID[msg.ConversationId], m)
	}

	transferProtoMap := make(map[string]proto.Message, len(d.allTransfers))
	for _, t := range d.allTransfers {
		transferProtoMap[t.Id] = t
	}
	d.convUnreadCount = buildConvUnreadCounts(
		d.convByID, d.messagesByConvID,
		transferProtoMap, d.reqMap,
		d.expMap,
		d.activeExpIDs, d.activeReqIDs,
		userID,
	)
}

// fetchTransferGear resolves the gear behind every transfer. It walks
// allTransfers rather than the active ones so that historical and completed
// transfers, shown in past-week views, still resolve their gear name and
// background image.
func (s *Service) fetchTransferGear(ctx context.Context, logger *logging.Logger, d *fetchedData) {
	gearIDs := make([]string, 0, len(d.allTransfers))
	for _, t := range d.allTransfers {
		gearIDs = append(gearIDs, t.GearId)
	}
	gearProtoMap, err := s.storage.GetByIDs(ctx, dedup(gearIDs), &models.Gear{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch gear for transfers", "error", err)
		return
	}
	d.gearProtoMap = gearProtoMap
}

// fetchCommunityGear loads the gear shared into the viewer's communities. The
// pivot rows are kept whole because the ring data reads archived and
// non-archived alike; only the non-archived gear is resolved.
func (s *Service) fetchCommunityGear(ctx context.Context, logger *logging.Logger, d *fetchedData) {
	commGearRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", d.communityIDs, &models.CommunityGear{})
	if err != nil {
		logger.WarnContext(ctx, "failed to query community gear", "error", err)
		return
	}
	d.commGearRaw = commGearRaw

	allCommunityGearIDs := make([]string, 0, len(commGearRaw))
	for _, m := range commGearRaw {
		cg := m.(*models.CommunityGear)
		d.cgByGearID[cg.GearId] = cg
		if !cg.Archived {
			allCommunityGearIDs = append(allCommunityGearIDs, cg.GearId)
		}
	}

	allCommunityGearMap, err := s.storage.GetByIDs(ctx, dedup(allCommunityGearIDs), &models.Gear{})
	if err != nil {
		logger.WarnContext(ctx, "failed to batch-fetch all community gear", "error", err)
		return
	}
	d.allCommunityGearMap = allCommunityGearMap
}

// loadCommunityGearChat augments the message and unread maps with the
// conversations on the viewer's own shared gear. fetchChat cannot pick these up
// because CommunityGear is loaded after it, and shared-gear inbox cards need
// the unread indicator.
func (s *Service) loadCommunityGearChat(
	ctx context.Context, logger *logging.Logger, d *fetchedData, userID string,
) {
	var convIDs []string
	for _, m := range d.commGearRaw {
		cg := m.(*models.CommunityGear)
		if cg.Archived {
			continue
		}
		g, ok := d.allCommunityGearMap[cg.GearId]
		if !ok {
			continue
		}
		gear := g.(*models.Gear)
		if gear.OwnerId != userID || gear.ConversationId == "" {
			continue
		}
		convIDs = append(convIDs, gear.ConversationId)
	}
	if len(convIDs) == 0 {
		return
	}

	cgMsgsRaw, err := s.storage.QueryByFieldIn(ctx, "conversation_id", dedup(convIDs), &models.ChatMessage{})
	if err != nil {
		logger.WarnContext(ctx, "failed to fetch community gear messages", "error", err)
		return
	}
	for _, m := range cgMsgsRaw {
		msg := m.(*models.ChatMessage)
		d.messagesByConvID[msg.ConversationId] = append(d.messagesByConvID[msg.ConversationId], m)
	}
	for _, convID := range convIDs {
		d.convUnreadCount[convID] = computeUnreadCount(d.messagesByConvID[convID], userID)
	}
}

// fetchRingData builds what the access ring on an inbox card needs: the member
// count and roster of each community, and, per item, the communities it is
// shared to.
func (s *Service) fetchRingData(ctx context.Context, logger *logging.Logger, d *fetchedData) {
	allMembershipsRaw, err := s.storage.QueryByFieldIn(ctx, "community_id", d.communityIDs, &models.CommunityUser{})
	if err != nil {
		logger.WarnContext(ctx, "failed to fetch community memberships for ring data", "error", err)
	} else {
		for _, m := range allMembershipsRaw {
			cu := m.(*models.CommunityUser)
			d.communityMemberCountMap[cu.CommunityId]++
			d.communityMemberUserIDs[cu.CommunityId] = append(d.communityMemberUserIDs[cu.CommunityId], cu.UserId)
		}
	}

	for _, m := range d.commGearRaw {
		if cg := m.(*models.CommunityGear); !cg.Archived {
			d.gearCommunityIDs[cg.GearId] = append(d.gearCommunityIDs[cg.GearId], cg.CommunityId)
		}
	}
	for _, m := range d.commExpRaw {
		if ce := m.(*models.CommunityExperience); !ce.Archived {
			d.expCommunityIDs[ce.ExperienceId] = append(d.expCommunityIDs[ce.ExperienceId], ce.CommunityId)
		}
	}
	for _, m := range d.commReqRaw {
		if cr := m.(*models.CommunityRequest); !cr.Archived {
			d.reqCommunityIDs[cr.RequestId] = append(d.reqCommunityIDs[cr.RequestId], cr.CommunityId)
		}
	}

	// The pivot queries above return rows in an unordered fashion, and every
	// item also belongs to its own nameless ad-hoc community (#2492). Order each
	// slice named-first so the display-community pick is deterministic and
	// prefers the real community over the item's ad-hoc backing community, and
	// so the community_ids carried on calendar entries lead with the real
	// community (#2675).
	for _, ids := range d.gearCommunityIDs {
		stableNamedFirst(ids, d.communityNameMap)
	}
	for _, ids := range d.expCommunityIDs {
		stableNamedFirst(ids, d.communityNameMap)
	}
	for _, ids := range d.reqCommunityIDs {
		stableNamedFirst(ids, d.communityNameMap)
	}
}

// ownedItem is the shape the owned-item library needs from a stored item: an
// identity and a soft-delete marker.
type ownedItem interface {
	proto.Message
	GetId() string
	GetDeleted() *models.DeletedMetadata
}

// liveOwned narrows a raw owned-item query result to the live, distinct items
// in query order, and merges anything new into lookup.
//
// De-duping by ID matters because these queries return one row per community an
// item is shared to, and an item must list only once across Your library, Up
// next and Needs you. Merging into lookup is what lets the engagement-driven
// maps (and so location resolution and ring data) see library entries too.
func liveOwned[T ownedItem](raw []proto.Message, lookup map[string]proto.Message) []T {
	out := make([]T, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, m := range raw {
		item, ok := m.(T)
		if !ok {
			continue
		}
		if del := item.GetDeleted(); del != nil && del.GetDeletedAtUnixSec() > 0 {
			continue
		}
		id := item.GetId()
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, item)
		if _, exists := lookup[id]; !exists {
			lookup[id] = item
		}
	}
	return out
}

// queryOwned runs one owned-item query, degrading to no rows on failure — a
// library section that cannot load is not worth failing the whole view for.
func (s *Service) queryOwned(
	ctx context.Context, logger *logging.Logger,
	field, userID string, prototype proto.Message, label string,
) []proto.Message {
	raw, err := s.storage.QueryByField(ctx, field, userID, prototype)
	if err != nil {
		logger.WarnContext(ctx, "failed to query owned "+label, "error", err)
		return nil
	}
	return raw
}

// fetchOwnedLibrary loads every item the viewer owns regardless of engagement.
// These power the "Your Stuff" filter's library view — gear with no transfer,
// experiences with no RSVPs, requests with no offers — none of which the
// engagement-driven loops in assembleFeedItems emit.
//
// Location names are resolved last, from the now-merged maps, so library
// entries inherit them. batchFetchLocationNames is cheap and returns a fresh
// map, so re-running it here rather than in a section of its own is what makes
// one batch cover both the engaged and the owned-but-unengaged items.
func (s *Service) fetchOwnedLibrary(
	ctx context.Context, logger *logging.Logger, d *fetchedData, userID string,
) {
	d.ownedGear = liveOwned[*models.Gear](
		s.queryOwned(ctx, logger, "owner_id", userID, &models.Gear{}, "gear"), d.gearProtoMap,
	)
	d.ownedExperiences = liveOwned[*models.Experience](
		s.queryOwned(ctx, logger, "owner_id", userID, &models.Experience{}, "experiences"), d.expMap,
	)
	d.ownedRequests = liveOwned[*models.Request](
		s.queryOwned(ctx, logger, "requester_id", userID, &models.Request{}, "requests"), d.reqMap,
	)

	d.locationNameMap = batchFetchLocationNames(ctx, s, logger, d.gearProtoMap, d.expMap, d.reqMap)
}

// fetchUsers resolves every user referenced anywhere in the assembled data,
// including the viewer, whose avatar community milestone cards show.
func (s *Service) fetchUsers(ctx context.Context, d *fetchedData, userID string) error {
	allUserIDs := collectUserIDs(d.activeTransfers, d.expMap, d.activeExpIDs, d.expAttendeesMap, d.reqMap, d.activeReqIDs)
	allUserIDs = dedup(append(allUserIDs,
		collectCommunityItemOwnerIDs(d.allCommunityGearMap, d.commExpRaw, d.expMap, d.commReqRaw, d.reqMap)...))
	allUserIDs = dedup(append(allUserIDs, userID))

	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, allUserIDs)
	if err != nil {
		return connecterr.Internal(ctx, "fetchAll", err, "detail", "failed to fetch users")
	}
	d.userMap = userMap
	return nil
}

// groupExperienceActivities clusters the viewer's past experiences into
// semantically-similar activity groups for open-day suggestions (#2674), so
// near-but-not-identical titles ("Flatirons hike" / "Sunday hike") count toward
// the ≥2 threshold together.
//
// Only experiences the viewer took part in matter. Clustering reads the
// precomputed name-only embedding — name+description clusters activities poorly
// (#2694) — with the read and the cosine math staying in storage. Best-effort
// throughout: without an embedder, or on any failure, the groups stay empty and
// viewerActivities falls back to exact-name keying. It never fails the view.
func (s *Service) groupExperienceActivities(
	ctx context.Context, logger *logging.Logger, d *fetchedData, userID string,
) {
	embedder := s.storage.GetEmbedder()
	if embedder == nil {
		return
	}

	var clusterIDs []string
	for id, msg := range d.expMap {
		exp, ok := msg.(*models.Experience)
		if !ok || !isViewerInvolved(exp, d.expAttendeesMap[id], userID) {
			continue
		}
		if strings.TrimSpace(exp.GetName()) == "" {
			continue
		}
		clusterIDs = append(clusterIDs, id)
	}
	if len(clusterIDs) < minActivityOccurrences {
		return
	}

	groups, err := s.storage.GroupIDsByStoredEmbedding(
		ctx, &models.Experience{}, embedder.Info(),
		storage.ExperienceNameEmbeddingVariant, clusterIDs, activityClusterMinSimilarity,
	)
	if err != nil {
		logger.WarnContext(ctx, "failed to group experiences for suggestions", "error", err)
		return
	}
	d.expActivityGroups = groups
}
