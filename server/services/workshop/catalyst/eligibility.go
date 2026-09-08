package catalyst

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// HostModeEligibleUserIDs returns the subset of a circle's members who
// have taken at least one *originating* action. Per the brief, the
// originating actions are:
//
//   - Hosted any experience (Experience.owner_id == userID, any state).
//   - Listed an offer (Gear.owner_id == userID, with that gear shared
//     into the community).
//
// "Proposed an event" is implicit in hosting — the experience's owner_id
// is set when the experience is first created, before it transitions to
// COMPLETED. "Accepted a sub-host slot" is not yet modeled in storage; it
// will be added when the sub-host mechanic ships.
//
// Slot-claiming alone (RSVPing yes, attending) is intentionally NOT an
// originating action. Per the brief: "the lonely guy who has only said
// yes to four cookouts is not eligible to be catalyst-pulled."
//
// excludeUserID is omitted from the result — typically the host themself
// (a host is not a candidate to be catalyst-pulled by themself).
func HostModeEligibleUserIDs(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	communityID, excludeUserID string,
) ([]string, error) {
	eligible := map[string]bool{}

	// Hosted experiences: any experience whose owner is a community member.
	// We approximate "in this community" by joining through CommunityExperience.
	communityExps, err := storage.QueryByFields[*models.CommunityExperience](
		store, ctx, map[string]any{"community_id": communityID},
	)
	if err != nil {
		return nil, fmt.Errorf("eligibility: query community_experience: %w", err)
	}
	for _, ce := range communityExps {
		exp := &models.Experience{}
		if getErr := store.GetByID(ctx, ce.ExperienceId, exp); getErr != nil {
			continue
		}
		if exp.OwnerId == "" || exp.OwnerId == excludeUserID {
			continue
		}
		eligible[exp.OwnerId] = true
	}

	// Listed offers: gear shared into the community.
	communityGear, err := storage.QueryByFields[*models.CommunityGear](
		store, ctx, map[string]any{"community_id": communityID},
	)
	if err != nil {
		return nil, fmt.Errorf("eligibility: query community_gear: %w", err)
	}
	for _, cg := range communityGear {
		gear := &models.Gear{}
		if getErr := store.GetByID(ctx, cg.GearId, gear); getErr != nil {
			continue
		}
		if gear.OwnerId == "" || gear.OwnerId == excludeUserID {
			continue
		}
		eligible[gear.OwnerId] = true
	}

	out := make([]string, 0, len(eligible))
	for u := range eligible {
		out = append(out, u)
	}
	return out, nil
}
