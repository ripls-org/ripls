package impact_metrics

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// ActsCategory identifies a user-visible bucket in the Acts breakdown.
type ActsCategory string

// Stable category keys surfaced to the client. The values are part of
// the wire contract — the client maps them to localized labels.
const (
	ActsCategoryItemsShared    ActsCategory = "items_shared"
	ActsCategoryEventsCreated  ActsCategory = "events_created"
	ActsCategoryRSVPs          ActsCategory = "rsvps"
	ActsCategoryHelpRequested  ActsCategory = "help_requested"
	ActsCategoryHelpOffered    ActsCategory = "help_offered"
	ActsCategoryPitchedIn      ActsCategory = "pitched_in"
	ActsCategoryLoansGiveaways ActsCategory = "loans_giveaways"
)

// ActsCategoryOrder is the canonical client-facing row order for the
// Acts detail breakdown table.
var ActsCategoryOrder = []ActsCategory{
	ActsCategoryItemsShared,
	ActsCategoryEventsCreated,
	ActsCategoryRSVPs,
	ActsCategoryHelpRequested,
	ActsCategoryHelpOffered,
	ActsCategoryPitchedIn,
	ActsCategoryLoansGiveaways,
}

// ActsCategoryLabel maps a category to its server-rendered fallback
// label. Clients should localize via [ActsCategory] keys; the label is
// only used when a client does not recognise the key.
var ActsCategoryLabel = map[ActsCategory]string{
	ActsCategoryItemsShared:    "Items shared",
	ActsCategoryEventsCreated:  "Events created",
	ActsCategoryRSVPs:          "RSVPs",
	ActsCategoryHelpRequested:  "Help requested",
	ActsCategoryHelpOffered:    "Help offered",
	ActsCategoryPitchedIn:      "Pitched in",
	ActsCategoryLoansGiveaways: "Loans & giveaways",
}

// actCategoryByEventType maps each in-scope CommunityEventType to its
// breakdown category. Event types absent from this map are not acts.
var actCategoryByEventType = map[models.CommunityEventType]ActsCategory{
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED:                 ActsCategoryItemsShared,
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED:          ActsCategoryEventsCreated,
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES:         ActsCategoryRSVPs,
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED:             ActsCategoryHelpRequested,
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE:          ActsCategoryHelpOffered,
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED:       ActsCategoryPitchedIn,
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED: ActsCategoryPitchedIn,
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED:          ActsCategoryLoansGiveaways,
}

// ActsBreakdown is the aggregate result of [Calculator.CountActs].
type ActsBreakdown struct {
	// Total is the sum of acts across all categories.
	Total int32

	// ByCategory maps each [ActsCategory] to its count. Categories with
	// zero acts are still present with value 0 so callers can render a
	// stable row order.
	ByCategory map[ActsCategory]int32
}

// MonthlyActs is one bucket in the per-month acts series returned by
// [Calculator.CountActsByMonth].
type MonthlyActs struct {
	// Year and month are the calendar coordinates of the bucket. Callers put
	// the bucket start on the wire and the client renders the month name in
	// the viewer's locale; this used to carry a rendered English label too
	// ("Sep", "Nov 25") until #2835.
	Year  int
	Month time.Month

	// Count is the number of acts that occurred in this month.
	Count int32
}

// CountActs counts community acts for a single community, returning a
// total + per-category breakdown.
//
// An act is one CommunityEvent row whose event_type appears in
// [actCategoryByEventType]. Soft-deleted events (if any) are excluded.
func (c *Calculator) CountActs(ctx context.Context, communityID string) (*ActsBreakdown, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CountActs",
		"community_id", communityID,
	)

	events, err := storage.QueryByField[*models.CommunityEvent](
		c.storage, ctx, "community_id", communityID,
	)
	if err != nil {
		return nil, fmt.Errorf("query community_event for %s: %w", communityID, err)
	}

	out := &ActsBreakdown{
		ByCategory: make(map[ActsCategory]int32, len(ActsCategoryOrder)),
	}
	for _, cat := range ActsCategoryOrder {
		out.ByCategory[cat] = 0
	}

	for _, e := range events {
		cat, ok := actCategoryByEventType[e.EventType]
		if !ok {
			continue
		}
		out.ByCategory[cat]++
		out.Total++
	}

	logger.DebugContext(ctx, "counted acts",
		"total", out.Total,
		"event_rows_seen", len(events),
	)
	return out, nil
}

// yearMonth identifies a single calendar month bucket.
type yearMonth struct {
	year  int
	month time.Month
}

func (a yearMonth) before(b yearMonth) bool {
	if a.year != b.year {
		return a.year < b.year
	}
	return a.month < b.month
}

func (a yearMonth) next() yearMonth {
	if a.month == time.December {
		return yearMonth{year: a.year + 1, month: time.January}
	}
	return yearMonth{year: a.year, month: a.month + 1}
}

// CountActsByMonth groups community acts into monthly buckets, oldest
// first. The returned slice has no gaps — months between the earliest
// and latest acts are emitted with count 0 so the chart renders a
// continuous time axis.
//
// Returns an empty slice when the community has no acts.
func (c *Calculator) CountActsByMonth(ctx context.Context, communityID string) ([]MonthlyActs, error) {
	events, err := storage.QueryByField[*models.CommunityEvent](
		c.storage, ctx, "community_id", communityID,
	)
	if err != nil {
		return nil, fmt.Errorf("query community_event for %s: %w", communityID, err)
	}

	buckets := map[yearMonth]int32{}
	var earliest, latest yearMonth
	hasAny := false

	for _, e := range events {
		if _, ok := actCategoryByEventType[e.EventType]; !ok {
			continue
		}
		if e.OccurredAtUnixSec <= 0 {
			continue
		}
		t := time.Unix(e.OccurredAtUnixSec, 0).UTC()
		key := yearMonth{year: t.Year(), month: t.Month()}
		buckets[key]++
		if !hasAny {
			earliest = key
			latest = key
			hasAny = true
			continue
		}
		if key.before(earliest) {
			earliest = key
		}
		if latest.before(key) {
			latest = key
		}
	}

	if !hasAny {
		return []MonthlyActs{}, nil
	}

	out := make([]MonthlyActs, 0)
	cursor := earliest
	for {
		out = append(out, MonthlyActs{
			Year:  cursor.year,
			Month: cursor.month,
			Count: buckets[cursor],
		})
		if cursor == latest {
			break
		}
		cursor = cursor.next()
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Year != out[j].Year {
			return out[i].Year < out[j].Year
		}
		return out[i].Month < out[j].Month
	})
	return out, nil
}
