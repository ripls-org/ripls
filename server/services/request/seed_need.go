package request

import (
	"strings"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// maxSeedNeeds bounds how many initial needs a request can be born with. The
// AI provider already caps its seed_needs list, but SubmitRequest re-applies
// the bound so a hand-crafted client can't create an unbounded batch (#2731).
const maxSeedNeeds = 8

// buildSeedNeeds turns the client-supplied seed-need names into PlanningNeed
// rows for a new request. It trims whitespace, drops blanks, de-dupes
// case-insensitively (keeping first-seen order — the provider emits the names
// most-important-first), and caps the count at maxSeedNeeds. Each need gets a
// single slot and is scoped to the request. Returns an empty slice when nothing
// concrete was named, which leaves the request born with no needs for helpers
// or the requester's compose flow to fill (rather than restating the request's
// own title as a need nobody would claim, #2731).
func buildSeedNeeds(names []string, proposerID, requestID string, nowUnixSec int64) []proto.Message {
	seen := make(map[string]struct{}, len(names))
	needs := make([]proto.Message, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		needs = append(needs, &models.PlanningNeed{
			ProposerId:       proposerID,
			Name:             name,
			Slots:            1,
			SlotsRemaining:   1,
			CreatedAtUnixSec: nowUnixSec,
			Scope:            &models.PlanningNeed_RequestId{RequestId: requestID},
		})
		if len(needs) >= maxSeedNeeds {
			break
		}
	}
	return needs
}
