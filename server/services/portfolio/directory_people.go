package portfolio

import (
	"context"
	"sort"

	"google.golang.org/protobuf/proto"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// maxSharedCommunityNames caps how many shared-community names a person row
// carries. Three fits the Directory's subtitle line.
const maxSharedCommunityNames = 3

// GetDirectoryPeople returns the Directory address book's people rows.
//
// This was carved out of GetPortfolioInboxView, which the Directory used to
// call for its `people` field alone — pulling a whole inbox assembly (items,
// weekly metrics, actions-today, shared inventory) to read one list. That RPC
// is gone; this is the narrow replacement.
//
// It still pays for fetchAll, which loads the viewer's full community graph.
// buildCommunityPeople needs the gear, experience, request, and message maps to
// decide who counts as "connected", so there is no cheaper read short of a
// dedicated query. GetHomeView pays the same cost on the same data.
func (s *Service) GetDirectoryPeople(
	ctx context.Context,
	_ *connect.Request[api.GetDirectoryPeopleRequest],
) (*connect.Response[api.GetDirectoryPeopleResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetDirectoryPeople",
		"user_id", authInfo.UserID,
	)

	d, err := s.fetchAll(ctx, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// A viewer in no communities has no connections by definition.
	if len(d.communityIDs) == 0 {
		return connect.NewResponse(&api.GetDirectoryPeopleResponse{}), nil
	}

	people := buildCommunityPeople(
		authInfo.UserID, d.userMap, d.allCommunityGearMap, d.cgByGearID,
		d.commExpRaw, d.expMap, d.commReqRaw, d.reqMap, d.messagesByConvID,
		d.peopleRSVPTimes, d.communityMemberUserIDs, d.communityNameMap,
	)

	logger.InfoContext(ctx, "assembled directory people", "people_count", len(people))

	return connect.NewResponse(&api.GetDirectoryPeopleResponse{People: people}), nil
}

func buildCommunityPeople(
	selfID string,
	userMap map[string]*api.User,
	gearMap map[string]proto.Message,
	cgByGearID map[string]*models.CommunityGear,
	commExpRaw []proto.Message,
	expMap map[string]proto.Message,
	commReqRaw []proto.Message,
	reqMap map[string]proto.Message,
	messagesByConvID map[string][]proto.Message,
	rsvpTimes map[string]int64,
	memberUserIDsMap map[string][]string,
	communityNameMap map[string]string,
) []*api.DailyPerson {
	personSharedAt := make(map[string]int64)
	seenPerson := make(map[string]bool)
	updateMax := func(uid string, ts int64) {
		if uid == "" || uid == selfID {
			return
		}
		if !seenPerson[uid] || ts > personSharedAt[uid] {
			seenPerson[uid] = true
			personSharedAt[uid] = ts
		}
	}

	// commLastActivity tracks each community's most recent share time so the
	// per-person shared-community names can be ordered most-recently-active
	// first. Communities with no share signal simply sort after those with one.
	commLastActivity := make(map[string]int64)
	updateCommMax := func(cid string, ts int64) {
		if cid != "" && ts > commLastActivity[cid] {
			commLastActivity[cid] = ts
		}
	}

	for gearID, m := range gearMap {
		g := m.(*models.Gear)
		if g.Deleted != nil && g.Deleted.DeletedAtUnixSec > 0 {
			continue
		}
		cg := cgByGearID[gearID]
		if cg == nil || cg.Archived {
			continue
		}
		updateMax(g.OwnerId, cg.CreatedAtUnixSec)
		updateCommMax(cg.CommunityId, cg.CreatedAtUnixSec)
	}
	for _, m := range commExpRaw {
		ce := m.(*models.CommunityExperience)
		if ce.Archived {
			continue
		}
		em, ok := expMap[ce.ExperienceId]
		if !ok {
			continue
		}
		e := em.(*models.Experience)
		if e.Deleted != nil && e.Deleted.DeletedAtUnixSec > 0 {
			continue
		}
		updateMax(e.OwnerId, ce.SharedAtUnixSec)
		updateCommMax(ce.CommunityId, ce.SharedAtUnixSec)
	}
	for _, m := range commReqRaw {
		cr := m.(*models.CommunityRequest)
		if cr.Archived {
			continue
		}
		rm, ok := reqMap[cr.RequestId]
		if !ok {
			continue
		}
		r := rm.(*models.Request)
		if r.Deleted != nil && r.Deleted.DeletedAtUnixSec > 0 {
			continue
		}
		updateMax(r.RequesterId, cr.SharedAtUnixSec)
		updateCommMax(cr.CommunityId, cr.SharedAtUnixSec)
	}

	// Chat: each person's most recent message in a visible conversation.
	for _, msgs := range messagesByConvID {
		for _, m := range msgs {
			msg := m.(*models.ChatMessage)
			if chat.IsCreationAnchorMessage(msg) {
				continue
			}
			updateMax(messageSenderID(msg), msg.SentAtUnixSec)
		}
	}

	// RSVPs: each person's latest RSVP to a visible experience.
	for uid, ts := range rsvpTimes {
		updateMax(uid, ts)
	}

	type personEntry struct {
		userID   string
		sharedAt int64
	}
	entries := make([]personEntry, 0, len(personSharedAt))
	for uid, ts := range personSharedAt {
		entries = append(entries, personEntry{uid, ts})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sharedAt > entries[j].sharedAt
	})

	sharedNames := sharedCommunityNamesByUser(selfID, memberUserIDsMap, communityNameMap, commLastActivity)

	people := make([]*api.DailyPerson, 0, len(entries))
	for _, e := range entries {
		u := userMap[e.userID]
		if u == nil {
			continue
		}
		// Surface the person's latest shared-activity time so the directory
		// can order and day-group by recency (#2568).
		sharedAt := e.sharedAt
		people = append(people, &api.DailyPerson{
			UserId:               u.Id,
			DisplayName:          u.Name,
			MediaId:              u.MediaId,
			LastActivityUnixSec:  &sharedAt,
			SharedCommunityNames: sharedNames[e.userID],
		})
	}
	return people
}

// sharedCommunityNamesByUser maps each user to the display names of the
// communities they share with the viewer, ordered by the community's most
// recent share activity (descending, ties broken alphabetically) and capped
// at maxSharedCommunityNames. Only communities the viewer belongs to ever
// contribute a name — same privacy invariant as the profile's shared-groups
// list — and communities with an empty display name (nameless ad-hoc groups)
// are skipped.
func sharedCommunityNamesByUser(
	selfID string,
	memberUserIDsMap map[string][]string,
	communityNameMap map[string]string,
	commLastActivity map[string]int64,
) map[string][]string {
	type commEntry struct {
		name       string
		lastActive int64
	}
	commsByUser := make(map[string][]commEntry)
	for cid, memberIDs := range memberUserIDsMap {
		name := communityNameMap[cid]
		if name == "" {
			continue
		}
		viewerIsMember := false
		for _, uid := range memberIDs {
			if uid == selfID {
				viewerIsMember = true
				break
			}
		}
		if !viewerIsMember {
			continue
		}
		entry := commEntry{name: name, lastActive: commLastActivity[cid]}
		seen := make(map[string]bool, len(memberIDs))
		for _, uid := range memberIDs {
			if uid == "" || uid == selfID || seen[uid] {
				continue
			}
			seen[uid] = true
			commsByUser[uid] = append(commsByUser[uid], entry)
		}
	}

	names := make(map[string][]string, len(commsByUser))
	for uid, entries := range commsByUser {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].lastActive != entries[j].lastActive {
				return entries[i].lastActive > entries[j].lastActive
			}
			return entries[i].name < entries[j].name
		})
		if len(entries) > maxSharedCommunityNames {
			entries = entries[:maxSharedCommunityNames]
		}
		list := make([]string, 0, len(entries))
		for _, e := range entries {
			list = append(list, e.name)
		}
		names[uid] = list
	}
	return names
}
