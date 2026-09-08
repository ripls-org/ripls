package known_for

import (
	"context"
	"fmt"
	"strings"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// addHostCounts accumulates experience-participation contributions
// into [counts]. The "participation" set covers both hosts (per the
// owner_id) and, on per-user mode, attendees (per RSVP.attended ==
// "YES"). Per-community mode counts each experience once regardless
// of attendee count — the chip describes the community's activity,
// not its turnout.
//
// Weighting per category:
//   - +1 for every distinct participated experience
//   - +1 bonus when the experience's state is COMPLETED
//
// Query budget:
//   - per-user: 4 reads (2 for hosted + 2 for attended)
//   - per-community: 2 reads (community_experience by community-in +
//     experience batch)
func addHostCounts(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
	counts categoryCounts,
) error {
	experiences, err := loadScopedExperiences(ctx, s, opts, sharedSet)
	if err != nil {
		return fmt.Errorf("load experiences: %w", err)
	}
	seen := make(map[string]struct{}, len(experiences))
	for _, e := range experiences {
		seen[e.Id] = struct{}{}
		// Cancelled experiences are loud noise — they never ran, so
		// they shouldn't claim that the scope is "known for" them.
		if e.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			continue
		}
		cat := strings.TrimSpace(e.Category)
		counts.add(cat)
		if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
			counts.add(cat)
		}
	}

	// Per-user: also count experiences the target attended (and
	// didn't host). Hosting and attending the same event still
	// counts once — the dedup happens via [seen].
	if opts.Mode == ModePerUser {
		attended, err := loadAttendedScopedExperiencesByUser(ctx, s, opts.OwnerID, sharedSet)
		if err != nil {
			return fmt.Errorf("load attended experiences: %w", err)
		}
		for id, e := range attended {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			if e.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
				continue
			}
			cat := strings.TrimSpace(e.Category)
			counts.add(cat)
			if e.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
				counts.add(cat)
			}
		}
	}
	return nil
}

// loadAttendedScopedExperiencesByUser returns experiences inside the
// requested community scope that [target] attended (RSVP.attended ==
// "YES"), regardless of who hosted them. Used by per-user paths
// only — per-community mode aggregates attendees through
// [loadAttendeesByExperience].
//
// Query budget: 2 reads (RSVP by user_id + experience batch).
func loadAttendedScopedExperiencesByUser(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	target string,
	sharedSet map[string]struct{},
) (map[string]*models.Experience, error) {
	if target == "" {
		return map[string]*models.Experience{}, nil
	}
	rsvps, err := storage.QueryByField[*models.ExperienceRSVP](
		s, ctx, "user_id", target,
	)
	if err != nil {
		return nil, fmt.Errorf("query rsvps by user: %w", err)
	}
	expIDSet := make(map[string]struct{}, len(rsvps))
	for _, r := range rsvps {
		if r.Deleted != nil {
			continue
		}
		if r.GetAttended() != models.AttendedStatus_ATTENDED_STATUS_YES {
			continue
		}
		if _, ok := sharedSet[r.CommunityId]; !ok {
			continue
		}
		if r.ExperienceId == "" {
			continue
		}
		expIDSet[r.ExperienceId] = struct{}{}
	}
	if len(expIDSet) == 0 {
		return map[string]*models.Experience{}, nil
	}
	ids := make([]string, 0, len(expIDSet))
	for id := range expIDSet {
		ids = append(ids, id)
	}
	byID, err := storage.GetByIDs[*models.Experience](s, ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get attended experiences: %w", err)
	}
	out := make(map[string]*models.Experience, len(byID))
	for id, e := range byID {
		if e == nil || e.Deleted != nil {
			continue
		}
		out[id] = e
	}
	return out, nil
}

// loadAttendeesByExperience returns a map of experience-id → list of
// user IDs who attended (RSVP.attended == "YES") inside the
// requested community scope. Used by [Detail] to fan attendees into
// the members list for each in-scope experience.
//
// Query budget: 1 read.
func loadAttendeesByExperience(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	expIDs []string,
	sharedSet map[string]struct{},
) (map[string][]string, error) {
	out := make(map[string][]string)
	if len(expIDs) == 0 {
		return out, nil
	}
	rsvps, err := storage.QueryByFieldIn[*models.ExperienceRSVP](
		s, ctx, "experience_id", expIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query rsvps by experience: %w", err)
	}
	seen := make(map[string]map[string]struct{}, len(expIDs))
	for _, r := range rsvps {
		if r.Deleted != nil {
			continue
		}
		if r.GetAttended() != models.AttendedStatus_ATTENDED_STATUS_YES {
			continue
		}
		if _, ok := sharedSet[r.CommunityId]; !ok {
			continue
		}
		if r.UserId == "" {
			continue
		}
		set := seen[r.ExperienceId]
		if set == nil {
			set = make(map[string]struct{})
			seen[r.ExperienceId] = set
		}
		if _, dup := set[r.UserId]; dup {
			continue
		}
		set[r.UserId] = struct{}{}
		out[r.ExperienceId] = append(out[r.ExperienceId], r.UserId)
	}
	return out, nil
}

// loadScopedExperiences returns the non-deleted experiences shared
// into the requested community scope, regardless of state. The
// COMPLETED-state bonus is applied at the counting stage.
//
//   - Per-user: query Experience by owner_id, pivot through
//     community_experience to keep only ones shared into the scope.
//   - Per-community: pivot through community_experience by community,
//     batch-fetch experiences.
func loadScopedExperiences(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	opts Options,
	sharedSet map[string]struct{},
) (map[string]*models.Experience, error) {
	return loadScoped(ctx, s, opts, sharedSet,
		scopedSpec[*models.Experience, *models.CommunityExperience]{
			entity:     &models.Experience{},
			pivot:      &models.CommunityExperience{},
			ownerField: "owner_id",
			pivotField: "experience_id",
			pivotTable: "community_experience",
			noun:       "experience",
			plural:     "experiences",
			entityID:   (*models.CommunityExperience).GetExperienceId,
		})
}
