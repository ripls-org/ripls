package feed

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// insertNudge persists a new StoredNudge record.
func insertNudge(ctx context.Context, store *storage.ProtoSQLStorage, nudge *models.StoredNudge) error {
	if _, err := store.Insert(ctx, nudge); err != nil {
		return fmt.Errorf("insertNudge: %w", err)
	}
	return nil
}

// nudgeAudience names the pool a query is filling. The two surfaces want
// different things, and both differences are deliberate.
type nudgeAudience int

const (
	// feedAudience is the browse feed: imagery required (a nudge without a
	// background is invisible there), and feed-surface rows only.
	feedAudience nudgeAudience = iota

	// inboxAudience is the Home inbox: text-only rows are fine (the inbox card
	// renders on a solid background, which avoids both the stock-imagery
	// dependency and the two-load latency the feed accepts), and it also takes
	// **host prompts written for the retired Workshop tab** — a
	// WORKSHOP_* nudge that names the entity it acts on (`context_id`), such as
	// the momentum engine's "schedule {event} again". Those had no reader at
	// all once the Workshop tab left the nav (#2568); the inbox is where a host
	// meets their next move now. Workshop rows *without* a context_id stay out:
	// they would dispatch to an empty create modal, which the feed's own
	// `plan_experience` nudges already cover.
	inboxAudience
)

// isHostPrompt reports whether a nudge is one of the momentum engine's
// entity-referencing host prompts ("schedule {event} again") rather than a
// generic feed nudge — i.e. it was written for a WORKSHOP_* surface and names
// the thing its CTA acts on.
func isHostPrompt(n *models.StoredNudge) bool {
	isFeedSurface := n.Surface == models.NudgeSurface_NUDGE_SURFACE_FEED ||
		n.Surface == models.NudgeSurface_NUDGE_SURFACE_UNSPECIFIED
	return !isFeedSurface && n.ContextId != nil && *n.ContextId != ""
}

// accepts reports whether a stored nudge belongs in this audience's pool.
func (a nudgeAudience) accepts(n *models.StoredNudge) bool {
	if isHostPrompt(n) {
		return a == inboxAudience
	}
	// Everything else is admissible only on the surface it was written for.
	return n.Surface == models.NudgeSurface_NUDGE_SURFACE_FEED ||
		n.Surface == models.NudgeSurface_NUDGE_SURFACE_UNSPECIFIED
}

// requiresMedia reports whether this audience hides nudges whose imagery has
// not arrived yet.
func (a nudgeAudience) requiresMedia() bool { return a == feedAudience }

// getActiveNudgesForUser returns nudges that are ready to display in the
// feed: media_id is set, not consumed, not soft-deleted, not a terminator,
// and on the feed surface.
func getActiveNudgesForUser(ctx context.Context, store *storage.ProtoSQLStorage, userID, communityID string) ([]*models.StoredNudge, error) {
	return queryActiveNudges(ctx, store, userID, communityID, feedAudience)
}

// queryActiveNudges returns the user's non-terminal, non-consumed nudges for a
// community, filtered to what `audience` can render.
func queryActiveNudges(ctx context.Context, store *storage.ProtoSQLStorage, userID, communityID string, audience nudgeAudience) ([]*models.StoredNudge, error) {
	all, err := storage.QueryByFields[*models.StoredNudge](store, ctx,
		map[string]any{
			"user_id":       userID,
			"community_id":  communityID,
			"is_terminator": false,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("queryActiveNudges: %w", err)
	}

	// Filter in Go: keep only nudges that are not consumed/deleted (and, for the
	// feed, that have imagery). QueryByFields filters out soft-deleted rows.
	active := make([]*models.StoredNudge, 0, len(all))
	for _, n := range all {
		if audience.requiresMedia() && (n.MediaId == nil || *n.MediaId == "") {
			continue // imagery not yet ready
		}
		if n.ConsumedAtUnixSec != nil {
			continue // already consumed
		}
		if !audience.accepts(n) {
			continue
		}
		active = append(active, n)
	}
	return active, nil
}

// updateNudgeMediaID sets the media_id on a stored nudge once imagery has been fetched.
func updateNudgeMediaID(ctx context.Context, store *storage.ProtoSQLStorage, nudgeID, mediaID string) error {
	nudge := &models.StoredNudge{}
	if err := store.GetByID(ctx, nudgeID, nudge); err != nil {
		return fmt.Errorf("updateNudgeMediaID get: %w", err)
	}
	nudge.MediaId = &mediaID
	if err := store.Update(ctx, nudge); err != nil {
		return fmt.Errorf("updateNudgeMediaID update: %w", err)
	}
	return nil
}

// markNudgeConsumed records when and how a nudge was consumed by the user.
func markNudgeConsumed(ctx context.Context, store *storage.ProtoSQLStorage, nudgeID, action string) error {
	nudge := &models.StoredNudge{}
	if err := store.GetByID(ctx, nudgeID, nudge); err != nil {
		return fmt.Errorf("markNudgeConsumed get: %w", err)
	}

	now := timeNowUnix()
	nudge.ConsumedAtUnixSec = &now
	nudge.ConsumedAction = &action

	if err := store.Update(ctx, nudge); err != nil {
		return fmt.Errorf("markNudgeConsumed update: %w", err)
	}
	return nil
}

// softDeleteNudge marks a nudge as deleted (expired or replaced).
// The record is retained for analytics.
func softDeleteNudge(ctx context.Context, store *storage.ProtoSQLStorage, nudgeID string) error {
	nudge := &models.StoredNudge{}
	if err := store.GetByID(ctx, nudgeID, nudge); err != nil {
		return fmt.Errorf("softDeleteNudge get: %w", err)
	}

	nudge.Deleted = &models.DeletedMetadata{
		DeletedAtUnixSec: timeNowUnix(),
	}
	if err := store.Update(ctx, nudge); err != nil {
		return fmt.Errorf("softDeleteNudge update: %w", err)
	}
	return nil
}

// GlobalTerminatorUserID is the sentinel stored in user_id for the shared daily
// terminator pool. These records are not owned by any real user — they are
// generated once per day and served to everyone.
const GlobalTerminatorUserID = "_terminator_pool_"

// getGlobalTerminatorsForToday returns all non-deleted global terminator nudges
// created today. The pool is shared across all users and communities.
func getGlobalTerminatorsForToday(ctx context.Context, store *storage.ProtoSQLStorage, dayOfYear int) ([]*models.StoredNudge, error) {
	all, err := storage.QueryByFields[*models.StoredNudge](store, ctx,
		map[string]any{
			"user_id":       GlobalTerminatorUserID,
			"is_terminator": true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("getGlobalTerminatorsForToday: %w", err)
	}

	var today []*models.StoredNudge
	for _, n := range all {
		if n.Deleted != nil {
			continue
		}
		if unixSecToDayOfYear(n.CreatedAtUnixSec) == dayOfYear {
			today = append(today, n)
		}
	}
	return today, nil
}

// getPreviousGlobalTerminators returns all non-deleted global terminator nudges
// NOT from today, for soft-deletion when a new day begins.
func getPreviousGlobalTerminators(ctx context.Context, store *storage.ProtoSQLStorage, todayDayOfYear int) ([]*models.StoredNudge, error) {
	all, err := storage.QueryByFields[*models.StoredNudge](store, ctx,
		map[string]any{
			"user_id":       GlobalTerminatorUserID,
			"is_terminator": true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("getPreviousGlobalTerminators: %w", err)
	}

	var stale []*models.StoredNudge
	for _, n := range all {
		if n.Deleted != nil {
			continue
		}
		if unixSecToDayOfYear(n.CreatedAtUnixSec) != todayDayOfYear {
			stale = append(stale, n)
		}
	}
	return stale, nil
}
