package needs

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// LookbackWindowDays is how far back the detector considers a Request
// "fresh" (creation must fall within this window). Older requests are
// stale enough that surfacing them as a "this week" need would feel
// off — they belong in the Bring-Back row, not here.
const LookbackWindowDays = 14

// MaxOffersForUnderclaimed is the maximum number of non-withdrawn
// offers a Request can have and still be considered "under-claimed."
// 1 request + 0 claims is the canonical case the JSX mock illustrates.
const MaxOffersForUnderclaimed = 0

// Detection is the structured result of `DetectThisWeekNeed`. The
// caller (the Workshop service) translates this into a
// `WeeklyNeedPayload` for the wire.
type Detection struct {
	Request     *models.Request
	CommunityID string
	OfferCount  int
}

// DetectThisWeekNeed walks the open Requests across the host's
// circles and returns the highest-priority one to surface — or nil
// when nothing qualifies. Selection rules:
//
//   - Request.state == REQUEST_STATE_ACTIVE
//   - Not soft-deleted
//   - Created within `LookbackWindowDays`
//   - At most `MaxOffersForUnderclaimed` non-withdrawn offers
//   - Has a CommunityRequest row joining the Request to one of the
//     host's `communityIDs`
//
// When multiple Requests qualify, the most recently created one wins.
// The detector is read-only and side-effect-free.
func DetectThisWeekNeed(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	communityIDs []string,
	now time.Time,
) (*Detection, error) {
	if len(communityIDs) == 0 {
		return nil, nil
	}

	cutoff := now.Add(-time.Duration(LookbackWindowDays) * 24 * time.Hour).Unix()

	type candidate struct {
		req          *models.Request
		communityID  string
		offerCount   int
		createdAtSec int64
	}
	var candidates []candidate

	for _, communityID := range communityIDs {
		joins, err := storage.QueryByFields[*models.CommunityRequest](
			store, ctx,
			map[string]any{"community_id": communityID},
		)
		if err != nil {
			return nil, fmt.Errorf("needs.DetectThisWeekNeed: load CommunityRequest: %w", err)
		}
		for _, join := range joins {
			req := &models.Request{}
			if err := store.GetByID(ctx, join.RequestId, req); err != nil {
				continue
			}
			if !isOpenRecent(req, cutoff) {
				continue
			}
			offers, err := storage.QueryByFields[*models.RequestOffer](
				store, ctx,
				map[string]any{"request_id": req.Id},
			)
			if err != nil {
				continue
			}
			activeOffers := 0
			for _, o := range offers {
				if o.Withdrawn {
					continue
				}
				activeOffers++
			}
			if activeOffers > MaxOffersForUnderclaimed {
				continue
			}
			createdAt := requestCreatedAt(req)
			candidates = append(candidates, candidate{
				req:          req,
				communityID:  communityID,
				offerCount:   activeOffers,
				createdAtSec: createdAt,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].createdAtSec > candidates[j].createdAtSec
	})
	pick := candidates[0]
	return &Detection{
		Request:     pick.req,
		CommunityID: pick.communityID,
		OfferCount:  pick.offerCount,
	}, nil
}

// isOpenRecent returns true when req is in ACTIVE state, not deleted,
// and created within the lookback window.
func isOpenRecent(req *models.Request, cutoffUnixSec int64) bool {
	if req.State != models.RequestState_REQUEST_STATE_ACTIVE {
		return false
	}
	if req.Deleted != nil {
		return false
	}
	if requestCreatedAt(req) < cutoffUnixSec {
		return false
	}
	return true
}

// requestCreatedAt extracts the request's creation timestamp. Rows
// written before created_at_unix_sec existed carry 0, which fails the
// lookback check — the safe default for rows old enough to predate
// the column.
func requestCreatedAt(req *models.Request) int64 {
	return req.CreatedAtUnixSec
}
