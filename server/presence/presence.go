package presence

import (
	"context"
	"fmt"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// MaxFaces caps the named social-pull stack on ask/event cards; the
// remaining people collapse into the "+N" overflow clients derive
// from the card's count field.
const MaxFaces = 3

// LoadFaces resolves user ids into named face-stack entries, capped at
// [MaxFaces]. Order follows the input slice. Callers must pass only
// ids the viewer is authorized to see (co-members of the scope).
func LoadFaces(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	userIDs []string,
) ([]*api.ProfilePresenceFace, error) {
	if len(userIDs) > MaxFaces {
		userIDs = userIDs[:MaxFaces]
	}
	if len(userIDs) == 0 {
		return nil, nil
	}
	users, err := storage.GetByIDs[*models.User](s, ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("get face users: %w", err)
	}
	faces := make([]*api.ProfilePresenceFace, 0, len(userIDs))
	for _, id := range userIDs {
		u, ok := users[id]
		if !ok || u == nil {
			continue
		}
		face := &api.ProfilePresenceFace{
			UserId:      u.Id,
			DisplayName: u.Name,
		}
		if len(u.MediaIds) > 0 && u.MediaIds[0] != "" {
			m := u.MediaIds[0]
			face.MediaId = &m
		}
		faces = append(faces, face)
	}
	return faces, nil
}

// AskQualifies reports whether a request can hold or queue for the
// sheet: open (active / offers-received), not deleted, and either
// carrying a future deadline or created within the freshness cutoff.
func AskQualifies(r *models.Request, nowSec, cutoffSec int64) bool {
	if r == nil || r.Deleted != nil {
		return false
	}
	if r.State != models.RequestState_REQUEST_STATE_ACTIVE &&
		r.State != models.RequestState_REQUEST_STATE_OFFERS_RECEIVED {
		return false
	}
	if r.NeededByUnixSec != nil {
		return *r.NeededByUnixSec > nowSec
	}
	return r.CreatedAtUnixSec >= cutoffSec
}

// OfferTally summarizes a request's active offers for the ask card.
type OfferTally struct {
	// Committed is the count of non-withdrawn offers.
	Committed int32
	// OffererIDs lists the non-withdrawn offerers, excluding the
	// viewer (they know they're in).
	OffererIDs []string
	// ViewerCommitted is true when the viewer has an active offer.
	ViewerCommitted bool
}

// TallyOffers loads and summarizes the active offers on a request.
func TallyOffers(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	requestID, viewerID string,
) (OfferTally, error) {
	offers, err := storage.QueryByField[*models.RequestOffer](
		s, ctx, "request_id", requestID,
	)
	if err != nil {
		return OfferTally{}, fmt.Errorf("query request offers: %w", err)
	}
	var tally OfferTally
	for _, o := range offers {
		if o.Withdrawn {
			continue
		}
		tally.Committed++
		if o.UserId == viewerID {
			tally.ViewerCommitted = true
			continue
		}
		tally.OffererIDs = append(tally.OffererIDs, o.UserId)
	}
	return tally, nil
}
