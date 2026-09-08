package impact_metrics

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

func TestShouldRefresh(t *testing.T) {
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	t.Run("nil provenance returns true", func(t *testing.T) {
		if !ShouldRefresh(nil, cfg) {
			t.Error("expected true for nil provenance")
		}
	})

	t.Run("USER source returns false", func(t *testing.T) {
		prov := &models.Provenance{
			Source:  models.ProvenanceSource_PROVENANCE_SOURCE_USER,
			Name:    "user_edit",
			Version: 0,
		}
		if ShouldRefresh(prov, cfg) {
			t.Error("expected false for USER source")
		}
	})

	t.Run("up-to-date version returns false", func(t *testing.T) {
		prov := &models.Provenance{
			Source:  models.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Name:    "genai_from_text",
			Version: cfg.ProvenanceVersion("genai_from_text"),
		}
		if ShouldRefresh(prov, cfg) {
			t.Error("expected false when version is current")
		}
	})

	t.Run("outdated version returns true", func(t *testing.T) {
		prov := &models.Provenance{
			Source:  models.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Name:    "genai_from_text",
			Version: 0,
		}
		if !ShouldRefresh(prov, cfg) {
			t.Error("expected true when version is outdated")
		}
	})

	t.Run("unknown config name returns false", func(t *testing.T) {
		prov := &models.Provenance{
			Source:  models.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Name:    "unknown_algorithm",
			Version: 0,
		}
		if ShouldRefresh(prov, cfg) {
			t.Error("expected false for unknown provenance name")
		}
	})
}
