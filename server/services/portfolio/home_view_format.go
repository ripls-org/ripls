package portfolio

import (
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// optionalHomeString returns s as an optional-field pointer, or nil when
// empty — absence tells clients there is no value to compose with.
func optionalHomeString(s string) *string {
	if s == "" {
		return nil
	}
	return proto.String(s)
}

// dailyPersonFor builds a DailyPerson from an API user.
func dailyPersonFor(u *api.User) *api.DailyPerson {
	return &api.DailyPerson{
		UserId:      u.Id,
		DisplayName: u.Name,
		MediaId:     u.MediaId,
	}
}

// homeFirstName resolves a user ID to a first name via the prefetched user
// map, or "" when unknown.
func homeFirstName(d *fetchedData, userID string) string {
	if u := d.userMap[userID]; u != nil {
		return firstName(u.Name)
	}
	return ""
}

// eventIsAllDay reports whether an event shows an all-day label instead of a
// clock: it is flagged all-day, or its scheduled value lands exactly on local
// midnight — a defensive fallback for legacy rows created before the all-day
// flag was persisted.
func eventIsAllDay(e *models.Experience, ts int64, tz *time.Location) bool {
	return experienceIsAllDay(e) || time.Unix(ts, 0).In(tz).Format("15:04") == "00:00"
}

// dayStartIn returns the Unix timestamp of local midnight for now in tz.
func dayStartIn(now time.Time, tz *time.Location) int64 {
	local := now.In(tz)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, tz).Unix()
}

// firstID returns the first element of ids, or "" when empty. Used to pick
// the display community for items shared into multiple communities. The slice
// is expected to be ordered named-first (see stableNamedFirst), so firstID
// returns a real, user-facing community rather than an item's own nameless
// ad-hoc backing community when both are present.
func firstID(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// stableNamedFirst reorders community ids in place so ids that name a real,
// user-facing community (a non-empty name in names) precede nameless ad-hoc
// per-item communities (community.IsAdHoc keys on an empty name). Order is
// stable within each group.
//
// Every item is born into its own nameless ad-hoc community (#2492), so an
// event/gear/request shared into a real community belongs to both. The
// CommunityExperience/Gear/Request rows come back from an unordered query, so
// without this the display-community pick (firstID) and the calendar entry's
// community attribution were nondeterministic and frequently landed on the
// ad-hoc backing community — which dropped the item from the real community's
// filtered calendar (#2675).
func stableNamedFirst(ids []string, names map[string]string) {
	sort.SliceStable(ids, func(i, j int) bool {
		return names[ids[i]] != "" && names[ids[j]] == ""
	})
}

// communityIDList returns id as a one-element slice, or nil when empty. Used
// for HomeUpNextEntry.community_ids on obligation entries, which carry a single
// transfer community rather than a shared-into set.
func communityIDList(id string) []string {
	if id == "" {
		return nil
	}
	return []string{id}
}
