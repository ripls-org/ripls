package feed

import (
	"go.ripls.org/ripls/server/available_now"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// effectiveLastSeenAt returns the best available "last seen" timestamp for a
// view record.  Records created before the last_seen_at_unix_sec field was
// added have zero there; fall back to last_viewed_at_unix_sec so they are not
// incorrectly treated as never-seen.
func effectiveLastSeenAt(view *models.FeedItemView) int64 {
	if view.LastSeenAtUnixSec > 0 {
		return view.LastSeenAtUnixSec
	}
	return view.LastViewedAtUnixSec
}

// unreadWindowSeconds is the maximum age of activity that can mark an item as
// unread.  Activity older than 7 days is considered expired and does not cause
// an item to appear as unread, even if the user has never seen it.
const unreadWindowSeconds = 7 * 24 * 3600

// isItemUnread reports whether a feed item has unseen activity within the
// unread window.
//
// An item is unread when (a) its most recent activity is within the last 7 days
// AND (b) the user has never seen it or has not seen the latest activity.
// Activity older than 7 days is expired and never triggers unread.
func isItemUnread(view *models.FeedItemView, lastActivityAt, now int64) bool {
	if now-lastActivityAt > unreadWindowSeconds {
		return false // Activity too old — not unread regardless of view state.
	}
	if view == nil {
		return true // Never seen and recent activity — unread.
	}
	return effectiveLastSeenAt(view) < lastActivityAt
}

// isActionRequired reports whether a feed item requires the viewer to take
// action. This is true when the viewer owns the gear and there are pending
// transfer interest records, or the viewer owns a request with pending offers.
func isActionRequired(event *models.CommunityEvent, viewerUserID string, fc *feedLookups) bool {
	if event.GearId != "" {
		gear := fc.gearMap[event.GearId]
		if gear != nil && gear.OwnerId == viewerUserID && fc.pendingInterestMap[event.GearId] > 0 {
			return true
		}
	}
	if reqID := event.GetRequestId(); reqID != "" {
		req := fc.requestMap[reqID]
		if req != nil && req.RequesterId == viewerUserID && fc.offerCountMap[reqID] > 0 {
			return true
		}
	}
	return false
}

// feedHorizonSeconds bounds how far back the feed reaches for items that are
// no longer live opportunities.  Beyond it an item is dropped regardless of
// view state, because the seen-based expiry in isItemFresh only ever fires for
// items the viewer actually scrolled to — and the Home pulse, now the feed's
// only surface, marks nothing viewed (#2799).  Without this bound a community
// accumulates every story and gear share it has ever produced.
//
// Two weeks is double unreadWindowSeconds, so nothing can be unread and beyond
// the horizon at the same time.
const feedHorizonSeconds = 14 * 24 * 3600

// experienceLiveGraceSeconds keeps an event live through the day it happens
// and the day after.  Without a grace window an event scheduled long in
// advance would stop being live the instant it starts, and — its creation
// event being older than the horizon — vanish from the feed partway through
// the very day it occurs.
const experienceLiveGraceSeconds = 24 * 3600

// isWithinHorizon reports whether an item's most recent activity is recent
// enough for the item to still belong in the feed.
func isWithinHorizon(lastActivityAt, now int64) bool {
	return now-lastActivityAt <= feedHorizonSeconds
}

// isLiveOpportunity reports whether an event represents an opportunity that is
// still open: an event that has not happened yet, a request still wanted by
// its due date, a giveaway still on offer, or a transfer mid-handoff.  Live
// items bypass both the staleness expiry and the horizon, because a thing that
// has not happened yet cannot be stale no matter how long ago it was posted.
//
// This is deliberately narrower than an "is it an experience/request/giveaway"
// test.  Exempting those kinds unconditionally is what left past-dated events
// advertising themselves — and offering a Join button — indefinitely (#2799).
// For gear-related event types, fc.communityGearMap is consulted to determine
// availability:
//   - Giveaway gear: all event types (including GEAR_SHARED) stay live while
//     the item is still on offer.
//   - Loan gear: only transfer lifecycle events are live (interest expressed,
//     recipient selected, active). A loan gear that is merely available
//     (GEAR_SHARED with no open transfer) expires normally.
func isLiveOpportunity(event *models.CommunityEvent, fc *feedLookups, now int64) bool {
	switch event.EventType {
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE:
		return isRequestStillWanted(fc.requestMap[event.GetRequestId()], now)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO:
		return isExperienceUpcoming(fc.experienceMap[event.GetExperienceId()], now)
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED:
		cg := fc.communityGearMap[event.GearId]
		return cg != nil && cg.Availability == models.Availability_AVAILABILITY_FOR_GIVEAWAY
	case models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE:
		// Open transfers stay live for both loans and giveaways — the card must
		// remain visible to both the owner and the receiver until resolved.
		cg := fc.communityGearMap[event.GearId]
		return cg != nil
	default:
		return false
	}
}

// isExperienceUpcoming reports whether an experience is still ahead of the
// viewer: not called off or wrapped up, and starting within the grace window
// or later.  An experience with no scheduled time (TBD) has nothing to be
// ahead of, so it falls through to the horizon.
func isExperienceUpcoming(e *models.Experience, now int64) bool {
	if e == nil {
		return false
	}
	if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED ||
		e.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
		return false
	}
	start := available_now.ExperienceStartUnixSec(e)
	return start != 0 && start >= now-experienceLiveGraceSeconds
}

// isRequestStillWanted reports whether a request is open and its needed-by
// date has not passed.  A request with no needed-by date has no liveness to
// assert, so it falls through to the horizon and ages out — two weeks with no
// answer is no longer news.
func isRequestStillWanted(r *models.Request, now int64) bool {
	if r == nil {
		return false
	}
	if r.State == models.RequestState_REQUEST_STATE_FULFILLED ||
		r.State == models.RequestState_REQUEST_STATE_CANCELLED {
		return false
	}
	return r.NeededByUnixSec != nil && *r.NeededByUnixSec >= now
}

// isItemFresh reports whether a feed item should still appear in the feed.
//
// An item expires once the user has seen it in its current state (last_seen_at
// >= last_activity_at) and more than expiryHours have elapsed since they last
// viewed it.  Items the user has never seen are always fresh.
func isItemFresh(view *models.FeedItemView, lastActivityAt, now int64, _, expiryHours int32) bool {
	if view == nil {
		return true // Never seen — always fresh.
	}
	lastSeen := effectiveLastSeenAt(view)
	if lastSeen < lastActivityAt {
		return true // New activity since last seen — always fresh.
	}
	// User has seen the current state; expire after expiryHours.
	return now-lastSeen <= int64(expiryHours)*3600
}

// feedLookups holds pre-fetched entity maps for building feed items without N+1 queries.
type feedLookups struct {
	userMap                map[string]*api.User
	gearMap                map[string]*models.Gear
	requestMap             map[string]*models.Request
	experienceMap          map[string]*models.Experience
	communityGearMap       map[string]*models.CommunityGear
	communityRequestMap    map[string]*models.CommunityRequest
	communityExperienceMap map[string]*models.CommunityExperience
	locationMap            map[string]*models.Location
	offerCountMap          map[string]int32
	pendingInterestMap     map[string]int32 // gear_id -> count of INTEREST_EXPRESSED transfers
	rsvpYesCountMap        map[string]int32
	rsvpMaybeCountMap      map[string]int32

	// Viewer-scoped participation, keyed by experience_id / request_id.  Both
	// are nil when the batch fetch that populates them failed, which is why the
	// wire fields are optional: absent means "could not determine", which the
	// client must not read as "you have not responded" (#2800).
	//
	// Neither is scoped by community.  An RSVP or offer is the viewer's own
	// data being shown back to them, so which community they made it from does
	// not change the answer — and scoping it would put a Join button on an
	// event they are already attending, the bug this exists to fix.  The
	// sibling counts (yes/maybe/offer) already aggregate across communities the
	// same way.
	viewerRSVPMap    map[string]api.FeedRSVPIntention
	viewerOfferedSet map[string]bool
}
