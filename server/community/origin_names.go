package community

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// ResolveOriginItemNames returns, per nameless ad-hoc community id, the
// display name of the origin item the community was spun up for (#2492)
// — the experience name, gear name, request title, or the gear behind an
// origin transfer. Named communities are skipped (they show their own
// name). Best-effort: a resolution failure logs a warning and leaves
// that community's name absent rather than failing the caller; the
// caller decides on a fallback label. Shared here (not in the community
// RPC service) so non-service consumers like the ops activity digest
// can label ad-hoc communities too.
func ResolveOriginItemNames(
	ctx context.Context,
	s *storage.ProtoSQLStorage,
	communityMap map[string]*models.Community,
) map[string]string {
	logger := logging.LoggerWithContext(ctx)

	// Bucket origin ids by kind for nameless communities only. Each
	// *ByComm map records communityID -> origin item id.
	var expIDs, gearIDs, reqIDs, transferIDs []string
	expByComm := map[string]string{}
	gearByComm := map[string]string{}
	reqByComm := map[string]string{}
	transferByComm := map[string]string{}
	for cid, c := range communityMap {
		if c == nil || c.Name != "" {
			continue
		}
		switch {
		case c.GetOriginExperienceId() != "":
			expByComm[cid] = c.GetOriginExperienceId()
			expIDs = append(expIDs, c.GetOriginExperienceId())
		case c.GetOriginGearId() != "":
			gearByComm[cid] = c.GetOriginGearId()
			gearIDs = append(gearIDs, c.GetOriginGearId())
		case c.GetOriginRequestId() != "":
			reqByComm[cid] = c.GetOriginRequestId()
			reqIDs = append(reqIDs, c.GetOriginRequestId())
		case c.GetOriginTransferId() != "":
			transferByComm[cid] = c.GetOriginTransferId()
			transferIDs = append(transferIDs, c.GetOriginTransferId())
		}
	}

	names := make(map[string]string)

	if len(expIDs) > 0 {
		m, err := storage.GetByIDs[*models.Experience](s, ctx, expIDs)
		if err != nil {
			logger.WarnContext(ctx, "failed to resolve origin experiences", "error", err)
		}
		for cid, id := range expByComm {
			if e, ok := m[id]; ok {
				names[cid] = e.GetName()
			}
		}
	}
	if len(gearIDs) > 0 {
		m, err := storage.GetByIDs[*models.Gear](s, ctx, gearIDs)
		if err != nil {
			logger.WarnContext(ctx, "failed to resolve origin gear", "error", err)
		}
		for cid, id := range gearByComm {
			if g, ok := m[id]; ok {
				names[cid] = g.GetName()
			}
		}
	}
	if len(reqIDs) > 0 {
		m, err := storage.GetByIDs[*models.Request](s, ctx, reqIDs)
		if err != nil {
			logger.WarnContext(ctx, "failed to resolve origin requests", "error", err)
		}
		for cid, id := range reqByComm {
			if r, ok := m[id]; ok {
				names[cid] = r.GetTitle()
			}
		}
	}
	// A transfer (loan/giveaway) carries no name; resolve the gear behind it.
	if len(transferIDs) > 0 {
		tm, err := storage.GetByIDs[*models.Transfer](s, ctx, transferIDs)
		if err != nil {
			logger.WarnContext(ctx, "failed to resolve origin transfers", "error", err)
		}
		var tGearIDs []string
		tGearByComm := map[string]string{}
		for cid, tid := range transferByComm {
			if t, ok := tm[tid]; ok && t.GetGearId() != "" {
				tGearByComm[cid] = t.GetGearId()
				tGearIDs = append(tGearIDs, t.GetGearId())
			}
		}
		if len(tGearIDs) > 0 {
			gm, err := storage.GetByIDs[*models.Gear](s, ctx, tGearIDs)
			if err != nil {
				logger.WarnContext(ctx, "failed to resolve transfer gear", "error", err)
			}
			for cid, gid := range tGearByComm {
				if g, ok := gm[gid]; ok {
					names[cid] = g.GetName()
				}
			}
		}
	}

	return names
}
