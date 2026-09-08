// Package workshop implements the Workshop tab's curation layer.
//
// GetWorkshopBrief returns the structured Workshop home payload — a
// stat-hero, three-column ticker, and a list of action postcards
// ranked by the momentum cascade. The signal is aggregated from the
// host's Workshop-surface StoredNudge rows and the impact_metrics
// totals across the requested circle scope.
package workshop

import (
	"context"
	"fmt"
	"math"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/available_now"
	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/itemkind"
	"go.ripls.org/ripls/server/known_for"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services/workshop/momentum"
	"go.ripls.org/ripls/server/storage"
)

// GetWorkshopBrief returns the structured Workshop home payload for the
// given circle scope: a stat-hero (since-X anchor, hours-together
// number), a three-column ticker (events / replaced / CO₂), and a list
// of action postcards ranked by the momentum cascade.
func (s *Service) GetWorkshopBrief(
	ctx context.Context,
	req *connect.Request[api.GetWorkshopBriefRequest],
) (*connect.Response[api.GetWorkshopBriefResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Drop soft-deleted or non-member communities before any data access.
	communityIDs, _, err := auth.FilterActiveMemberCommunities(ctx, s.storage, req.Msg.CommunityIds, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetWorkshopBrief",
		"user_id", authInfo.UserID,
		"community_count", len(communityIDs),
	)

	if len(communityIDs) == 0 {
		return connect.NewResponse(&api.GetWorkshopBriefResponse{}), nil
	}

	// Kick off per-event Hero card and Bring-Back materialization in the
	// background. Each EnsureHeroCard call can run dozens of AI copy
	// generations serially (~1.5 s each) — way too slow for the request
	// path. We fire-and-forget with a detached context so a client
	// timeout doesn't cancel the work, and rely on the next pull-to-
	// refresh to pick up the freshly-materialized nudges. Both helpers
	// are idempotent so duplicate kickoffs are safe.
	s.kickoffNudgeMaterialization(ctx, authInfo.UserID, communityIDs, time.Now())

	// Batch-load Workshop nudges across all requested circles in a single
	// query per community. Avoids N+1 per-experience round-trips. Nudges
	// drive the postcard list; the hero/ticker render even when no
	// nudges exist, so we don't short-circuit on empty.
	nudges, err := s.loadBriefNudges(ctx, authInfo.UserID, communityIDs)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetWorkshopBrief.loadBriefNudges", err)
	}

	// Rank priorities and build the brief signal. Empty nudges → empty
	// ranked result → zero postcards but a still-rendering hero/ticker.
	ranker := &BriefRanker{}
	ranked := ranker.Rank(nudges)

	// Resolve thumbnails for every contextId referenced by the ranked rows.
	thumbs := s.resolveContextThumbnails(ctx, contextIDsFromRanked(ranked), logger)

	// Collect season metrics from the calculator (best-effort; zero values
	// are acceptable — the home renders the hero/ticker on whatever
	// totals are available).
	totals := s.collectSeasonTotals(ctx, communityIDs, logger)

	// People count powers the PEOPLE ticker cell on the new Workshop
	// (#1898). Best-effort: a storage failure yields zero and the cell
	// renders dashed-out, matching the other totals' fallback behavior.
	uniquePeople, err := s.countUniquePeople(ctx, communityIDs)
	if err != nil {
		logger.WarnContext(ctx, "brief: count unique people failed",
			"error", err,
		)
		uniquePeople = 0
	}

	signal := &momentum.BriefSignal{
		Priorities:               ranked.TopPriorities,
		TotalEventCount:          int(totals.pastEvents),
		TotalReplacedCostUSD:     totals.costSavings,
		TotalHoursTogether:       math.Round(totals.timeMinutes / 60),
		TotalCO2KeptOffRoadGrams: totals.carbonGrams,
		TotalActsCount:           totals.actsCount,
		TotalProblemsSolved:      totals.problemsSolved,
		TotalProblemsPotential:   totals.problemsPotential,
		UniquePeopleCount:        uniquePeople,
	}

	brief := buildTemplatedBrief(signal, ranked)

	sinceUnixSec := s.earliestCommunityCreatedAt(ctx, communityIDs, logger)
	suppressedKeys, err := loadSuppressedKeysForScope(ctx, s, known_for.ModePerCommunity, "", communityIDs)
	if err != nil {
		// Suppression is a soft layer — log and keep going. A failure
		// here means previously-hidden chips might briefly reappear,
		// which is strictly less harmful than blocking the whole brief.
		logger.WarnContext(ctx, "workshop brief: suppression load failed",
			"error", err,
		)
		suppressedKeys = nil
	}
	specialties, err := known_for.Derive(ctx, s.storage, known_for.Options{
		Mode:           known_for.ModePerCommunity,
		CommunityIDs:   communityIDs,
		SuppressedKeys: suppressedKeys,
	})
	if err != nil {
		// Specialties are decorative — never block the brief on them.
		// Log the failure and continue with an empty list.
		logger.WarnContext(ctx, "workshop brief: specialty derivation failed",
			"error", err,
		)
		specialties = nil
	}
	availableNow, err := available_now.Gather(ctx, s.storage, available_now.Options{
		Mode:         available_now.ModePerCommunity,
		CommunityIDs: communityIDs,
	})
	if err != nil {
		// Available-now items are decorative — never block the brief on
		// them. Log the failure and continue with an empty list.
		logger.WarnContext(ctx, "workshop brief: available-now gather failed",
			"error", err,
		)
		availableNow = nil
	}
	payload := briefToPayload(brief, thumbs, signal, sinceUnixSec, specialties, availableNow)

	logger.InfoContext(ctx, "workshop brief assembled",
		"cta_row_count", len(payload.CtaRows),
	)

	return connect.NewResponse(&api.GetWorkshopBriefResponse{
		Brief: payload,
	}), nil
}

// kickoffNudgeMaterialization fires the per-event Hero card and
// Bring-Back materialization passes for every requested community in
// a detached goroutine. Detaching the context ensures client
// cancellation doesn't kill the work mid-way; idempotency on the
// helpers means a duplicate kickoff (next pull-to-refresh while
// materialization is still running) is safe.
func (s *Service) kickoffNudgeMaterialization(
	parent context.Context,
	userID string,
	communityIDs []string,
	now time.Time,
) {
	// Snapshot the slice — the caller's slice is not safe to read
	// from the goroutine if the request returns and any pooling
	// reuses the backing array.
	communities := make([]string, len(communityIDs))
	copy(communities, communityIDs)

	logging.GoSafe(parent, "workshop_brief_materialize", func() {
		ctx := context.Background()
		logger := logging.LoggerWithContext(parent).With(
			"operation", "GetWorkshopBrief.materialize",
			"user_id", userID,
		)
		for _, communityID := range communities {
			// Re-check active state inside the goroutine — community
			// may have been soft-deleted between the request entering
			// and this work running. Persisting StoredNudge rows into
			// a deleted community would resurface on restore (#1623).
			if !community.IsActive(ctx, s.storage, communityID) {
				continue
			}
			if _, err := s.EnsureHeroCard(ctx, userID, communityID, now); err != nil {
				logger.WarnContext(ctx, "EnsureHeroCard failed",
					"community_id", communityID,
					"error", err,
				)
			}
			if _, err := s.EnsureBringBackItems(ctx, userID, communityID, "", now); err != nil {
				logger.WarnContext(ctx, "EnsureBringBackItems failed",
					"community_id", communityID,
					"error", err,
				)
			}
		}
	})
}

// countUniquePeople returns the count of distinct user ids across every
// CommunityUser row whose community_id is in the requested scope. A
// person who belongs to two included communities is counted once. Uses a
// single batched IN query (chunked internally by `QueryByFieldIn`) so
// the cost is one round-trip regardless of how many communities the
// host has. Returns an error from the underlying storage call; callers
// are expected to wrap it via connecterr.Internal at the RPC handler.
func (s *Service) countUniquePeople(ctx context.Context, communityIDs []string) (int, error) {
	if len(communityIDs) == 0 {
		return 0, nil
	}
	rows, err := s.storage.QueryByFieldIn(ctx, "community_id", communityIDs, &models.CommunityUser{})
	if err != nil {
		return 0, fmt.Errorf("query community_user by community_id: %w", err)
	}
	seen := make(map[string]struct{}, len(rows))
	for _, msg := range rows {
		cu, ok := msg.(*models.CommunityUser)
		if !ok || cu.UserId == "" {
			continue
		}
		seen[cu.UserId] = struct{}{}
	}
	return len(seen), nil
}

// earliestCommunityCreatedAt returns the earliest created_at timestamp
// across the given community ids — the anchor for the home hero's
// "Since X" label. Returns 0 when no community is loadable; the client
// hides the anchor in that case. Failures are logged but never block
// the brief.
func (s *Service) earliestCommunityCreatedAt(
	ctx context.Context,
	communityIDs []string,
	logger interface {
		WarnContext(ctx context.Context, msg string, args ...any)
	},
) int64 {
	earliest := int64(0)
	for _, id := range communityIDs {
		c := &models.Community{}
		if err := s.storage.GetByID(ctx, id, c); err != nil {
			logger.WarnContext(ctx, "brief: load community for anchor failed",
				"community_id", id,
				"error", err,
			)
			continue
		}
		if c.CreatedAtUnixSec > 0 && (earliest == 0 || c.CreatedAtUnixSec < earliest) {
			earliest = c.CreatedAtUnixSec
		}
	}
	return earliest
}

// loadBriefNudges reads Workshop-surface StoredNudge rows for the given user
// and communities. Uses batched storage reads to avoid N+1 queries.
func (s *Service) loadBriefNudges(
	ctx context.Context,
	userID string,
	communityIDs []string,
) ([]*models.StoredNudge, error) {
	var all []*models.StoredNudge
	for _, communityID := range communityIDs {
		rows, err := storage.QueryByFields[*models.StoredNudge](
			s.storage, ctx,
			map[string]any{
				"user_id":      userID,
				"community_id": communityID,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("query nudges for community %s: %w", communityID, err)
		}
		all = append(all, rows...)
	}
	return all, nil
}

// collectSeasonTotals aggregates activity counts and impact savings across
// communities. Best-effort: failures log a warning and contribute zero to
// the totals rather than blocking the brief.
func (s *Service) collectSeasonTotals(
	ctx context.Context,
	communityIDs []string,
	logger interface {
		WarnContext(ctx context.Context, msg string, args ...any)
	},
) aggregateTotals {
	if s.calculator == nil {
		return aggregateTotals{}
	}
	var totals aggregateTotals
	for _, communityID := range communityIDs {
		acts, err := s.calculator.CalculateActivityCounts(ctx, communityID)
		if err != nil {
			logger.WarnContext(ctx, "brief: activity counts failed",
				"community_id", communityID,
				"error", err,
			)
			acts = nil
		}
		savings, err := s.calculator.CalculateImpactSavings(ctx, communityID)
		if err != nil {
			logger.WarnContext(ctx, "brief: impact savings failed",
				"community_id", communityID,
				"error", err,
			)
			savings = nil
		}
		totals.add(acts, savings)

		actsBreakdown, err := s.calculator.CountActs(ctx, communityID)
		if err != nil {
			logger.WarnContext(ctx, "brief: count acts failed",
				"community_id", communityID,
				"error", err,
			)
			actsBreakdown = nil
		}
		totals.addActs(actsBreakdown)

		// "Problems handled" — the X-of-Y fraction (completed loans +
		// fulfilled requests + claimed needs, over every non-cancelled loan /
		// request / posted need). One calculator call owns the whole metric so
		// the headline and the deep-dive can never drift apart.
		problems, err := s.calculator.CalculateProblemsCounts(ctx, communityID)
		if err != nil {
			logger.WarnContext(ctx, "brief: problems counts failed",
				"community_id", communityID,
				"error", err,
			)
			problems = nil
		}
		if problems != nil {
			totals.problemsSolved += problems.Handled()
			totals.problemsPotential += problems.Potential()
		}
	}
	return totals
}

// buildTemplatedBrief assembles the Workshop home brief from ranked
// priorities. Each priority becomes one postcard CTA row; the ranker's
// overflow rows follow. The hero/ticker fields are populated separately
// in briefToPayload from the BriefSignal totals.
func buildTemplatedBrief(sig *momentum.BriefSignal, ranked RankResult) *momentum.Brief {
	brief := &momentum.Brief{}
	for _, p := range sig.Priorities {
		brief.CTARows = append(brief.CTARows, momentum.BriefCTARow{
			Headline:       p.Headline,
			AtmosphereLine: p.AtmosphereLine,
			CtaAction:      p.CtaAction,
			ContextID:      p.ContextID,
		})
	}
	brief.CTARows = append(brief.CTARows, ranked.CTARows...)
	return brief
}

// briefToPayload converts a momentum.Brief into the wire BriefPayload shape.
// thumbs maps contextId → first media_id of the referenced entity; rows whose
// ContextID is missing from the map render with no thumbnail (the client
// falls back to a category icon). signal carries the cumulative season totals
// the home hero/ticker render. sinceUnixSec anchors the hero "Since X" label
// (zero when no anchor is available). specialties are the per-scope known-for
// tags rendered as chips above the impact rows; nil/empty collapses the chip
// row on the client. availableNowItems are the per-scope Available-Now rail
// tiles rendered below the impact rows.
func briefToPayload(
	b *momentum.Brief,
	thumbs map[string]string,
	signal *momentum.BriefSignal,
	sinceUnixSec int64,
	specialties []string,
	availableNowItems []*api.Item,
) *api.BriefPayload {
	payload := &api.BriefPayload{
		Specialties:       specialties,
		AvailableNowItems: availableNowItems,
	}

	for i, row := range b.CTARows {
		apiRow := &api.BriefCTARow{
			Headline:  row.Headline,
			IsPrimary: i == 0,
		}
		if row.AtmosphereLine != "" {
			a := row.AtmosphereLine
			apiRow.AtmosphereLine = &a
		}
		if row.CommunityID != "" {
			cid := row.CommunityID
			apiRow.CommunityId = &cid
		}
		thumb := ""
		if mid, ok := thumbs[row.ContextID]; ok && mid != "" {
			thumb = mid
		}
		// Split the internal cta_action string into the unified
		// underlying item ([item]) vs. non-entity action verb
		// (action_name). When cta_action carries an entity
		// discriminator, populate item; otherwise stash the verb in
		// action_name for non-entity dispatch (e.g. "schedule_repeat").
		if kind, isEntity := itemkind.FromCTAActionString(row.CtaAction); isEntity {
			item := &api.Item{
				ContextId: row.ContextID,
				Kind:      kind,
				Title:     row.Headline,
			}
			if row.AtmosphereLine != "" {
				a := row.AtmosphereLine
				item.Subtitle = &a
			}
			if thumb != "" {
				item.MediaId = &thumb
			}
			apiRow.Item = item
		} else if row.CtaAction != "" {
			action := row.CtaAction
			apiRow.ActionName = &action
		}
		payload.CtaRows = append(payload.CtaRows, apiRow)
	}

	if signal != nil {
		if signal.TotalHoursTogether > 0 {
			h := int32(math.Round(signal.TotalHoursTogether))
			payload.HoursTogether = &h
		}
		if signal.TotalEventCount > 0 {
			c := int32(signal.TotalEventCount)
			payload.EventsCount = &c
		}
		if signal.TotalReplacedCostUSD > 0 {
			r := int32(math.Round(signal.TotalReplacedCostUSD))
			payload.ReplacedCostUsd = &r
		}
		if signal.TotalCO2KeptOffRoadGrams > 0 {
			lbs := int32(math.Round(signal.TotalCO2KeptOffRoadGrams / 453.592))
			if lbs > 0 {
				payload.Co2AvoidedPounds = &lbs
			}
		}
		if signal.TotalActsCount > 0 {
			a := int32(signal.TotalActsCount)
			payload.ActsCount = &a
		}
		if signal.TotalProblemsSolved > 0 {
			p := int32(signal.TotalProblemsSolved)
			payload.ProblemsSolvedCount = &p
		}
		if signal.TotalProblemsPotential > 0 {
			p := int32(signal.TotalProblemsPotential)
			payload.ProblemsPotentialCount = &p
		}
		if signal.UniquePeopleCount > 0 {
			p := int32(signal.UniquePeopleCount)
			payload.UniquePeopleCount = &p
		}
	}
	if sinceUnixSec > 0 {
		s := sinceUnixSec
		payload.SinceUnixSec = &s
	}

	return payload
}

// contextIDsFromRanked collects unique non-empty contextIds from the ranker
// output (top priorities + CTA rows). Order is irrelevant — the result feeds
// a batched lookup.
func contextIDsFromRanked(r RankResult) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, p := range r.TopPriorities {
		add(p.ContextID)
	}
	for _, row := range r.CTARows {
		add(row.ContextID)
	}
	return out
}

// resolveContextThumbnails maps each contextId to the first media_id of the
// referenced entity. ContextIDs may reference an Experience or a Gear; both
// types are batch-loaded and merged. Missing entities and entities without
// media are simply absent from the result. Best-effort: storage failures
// log a warning and contribute nothing rather than blocking the brief.
func (s *Service) resolveContextThumbnails(
	ctx context.Context,
	contextIDs []string,
	logger interface {
		WarnContext(ctx context.Context, msg string, args ...any)
	},
) map[string]string {
	out := map[string]string{}
	if len(contextIDs) == 0 {
		return out
	}

	experiences, err := storage.GetByIDs[*models.Experience](s.storage, ctx, contextIDs)
	if err != nil {
		logger.WarnContext(ctx, "brief: load experiences for thumbnails failed", "error", err)
	}
	for id, e := range experiences {
		if e != nil && len(e.MediaIds) > 0 {
			out[id] = e.MediaIds[0]
		}
	}

	missing := make([]string, 0, len(contextIDs))
	for _, id := range contextIDs {
		if _, ok := out[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return out
	}

	gear, err := storage.GetByIDs[*models.Gear](s.storage, ctx, missing)
	if err != nil {
		logger.WarnContext(ctx, "brief: load gear for thumbnails failed", "error", err)
		return out
	}
	for id, g := range gear {
		if g != nil && len(g.MediaIds) > 0 {
			out[id] = g.MediaIds[0]
		}
	}
	return out
}
