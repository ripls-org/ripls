package impact_metrics

import (
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

// ShouldRefresh reports whether a field with the given provenance should be
// re-estimated. Returns true when provenance is nil (never been estimated).
// Returns false when the source is USER (user-edited values are sacred).
// Otherwise looks up the current version from cfg by provenance name and
// compares against the stored version. Returns false for unknown names
// (cannot determine staleness).
func ShouldRefresh(prov *models.Provenance, cfg *estimator.Config) bool {
	if prov == nil {
		return true
	}
	if prov.Source == models.ProvenanceSource_PROVENANCE_SOURCE_USER {
		return false
	}
	currentVersion := cfg.ProvenanceVersion(prov.Name)
	if currentVersion == 0 {
		// Unknown provenance name — cannot determine if stale, so don't refresh.
		return false
	}
	return prov.Version < currentVersion
}
