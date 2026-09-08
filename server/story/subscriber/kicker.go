package story_subscriber

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// formatStoryKicker formats the editorial kicker line rendered above
// the title on the experience-recap story screen — e.g.
// "TUE · APR 28 · CLEAR CREEK, CO". Returns an empty string when the
// experience carries no usable date or location, in which case the
// client renders the title flush with the top of the editorial column.
func formatStoryKicker(ctx context.Context, store *storage.ProtoSQLStorage, experience *models.Experience) string {
	parts := make([]string, 0, 2)

	if datePart := formatKickerDate(experience); datePart != "" {
		parts = append(parts, datePart)
	}
	if locPart := formatKickerLocation(ctx, store, experience); locPart != "" {
		parts = append(parts, locPart)
	}

	return strings.Join(parts, " · ")
}

// formatKickerDate renders the day-of-week and month/day pair using
// the moment the experience actually happened. Prefers the in-process
// timestamp (when participants converged on the event), falling back
// to the completion timestamp. Returns "" when neither is set.
func formatKickerDate(experience *models.Experience) string {
	var ts int64
	switch {
	case experience.StartedAtUnixSec != nil && *experience.StartedAtUnixSec > 0:
		ts = *experience.StartedAtUnixSec
	case experience.CompletedAtUnixSec != nil && *experience.CompletedAtUnixSec > 0:
		ts = *experience.CompletedAtUnixSec
	default:
		return ""
	}

	t := time.Unix(ts, 0).UTC()
	return fmt.Sprintf("%s · %s %d",
		strings.ToUpper(t.Format("Mon")),
		strings.ToUpper(t.Format("Jan")),
		t.Day(),
	)
}

// formatKickerLocation renders the trailing location segment of the
// kicker. Prefers the location's human-readable name; falls back to
// the locality, optionally suffixed with the administrative-area code
// (e.g. "CO"). All-uppercase to match the small-caps editorial style.
func formatKickerLocation(ctx context.Context, store *storage.ProtoSQLStorage, experience *models.Experience) string {
	if experience.LocationId == "" {
		return ""
	}
	loc := &models.Location{}
	if err := store.GetByID(ctx, experience.LocationId, loc); err != nil {
		logging.LoggerWithContext(ctx).DebugContext(ctx,
			"experience location lookup for kicker failed",
			"experience_id", experience.Id,
			"location_id", experience.LocationId,
			"error", err,
		)
		return ""
	}

	if loc.Name != nil && strings.TrimSpace(*loc.Name) != "" {
		return strings.ToUpper(strings.TrimSpace(*loc.Name))
	}
	if loc.Address == nil {
		return ""
	}
	locality := strings.TrimSpace(loc.Address.Locality)
	if locality == "" {
		return ""
	}
	region := ""
	if loc.Address.AdministrativeArea != nil {
		region = strings.TrimSpace(*loc.Address.AdministrativeArea)
	}
	if region == "" {
		return strings.ToUpper(locality)
	}
	return strings.ToUpper(locality + ", " + region)
}
