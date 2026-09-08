// Package estimator provides impact estimation for items in the sharing economy.
// It estimates cost, carbon, and time savings using a combination of emission
// factor tables and LLM inference.
package estimator

import (
	"embed"
	"fmt"
	"os"
	"sort"

	"google.golang.org/protobuf/encoding/prototext"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

//go:embed config.textproto
var configFS embed.FS

// Config wraps the proto EstimatorConfig with indexed lookups for efficient access.
type Config struct {
	*models.EstimatorConfig

	// Indexed lookups built from the repeated fields.
	materialFactors map[models.MaterialCategory]*models.MaterialEmissionFactorEntry
	methodStddevs   map[string]float32

	// Sorted breakpoints for LLM confidence interpolation.
	sortedBreakpoints []confidenceBreakpoint
}

type confidenceBreakpoint struct {
	confidence float32
	stddev     float32
}

// LoadConfig loads the impact estimation config from the given file path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	return parseAndValidate(data)
}

// LoadConfigFromEmbed loads the impact estimation config from the embedded config.textproto.
func LoadConfigFromEmbed() (*Config, error) {
	data, err := configFS.ReadFile("config.textproto")
	if err != nil {
		return nil, fmt.Errorf("reading embedded config: %w", err)
	}
	return parseAndValidate(data)
}

func parseAndValidate(data []byte) (*Config, error) {
	pb := &models.EstimatorConfig{}
	if err := prototext.Unmarshal(data, pb); err != nil {
		return nil, fmt.Errorf("parsing config textproto: %w", err)
	}

	cfg := &Config{EstimatorConfig: pb}
	cfg.buildIndexes()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return cfg, nil
}

func (c *Config) buildIndexes() {
	// Index material factors by category enum.
	c.materialFactors = make(map[models.MaterialCategory]*models.MaterialEmissionFactorEntry, len(c.MaterialEmissionFactors))
	for _, entry := range c.MaterialEmissionFactors {
		c.materialFactors[entry.Category] = entry
	}

	// Index method stddevs by version string.
	c.methodStddevs = make(map[string]float32, len(c.CarbonMethodStddevs))
	for _, entry := range c.CarbonMethodStddevs {
		c.methodStddevs[entry.Method] = entry.RelativeStddev
	}

	// Sort LLM confidence breakpoints ascending by confidence.
	c.sortedBreakpoints = make([]confidenceBreakpoint, len(c.LlmConfidenceBreakpoints))
	for i, bp := range c.LlmConfidenceBreakpoints {
		c.sortedBreakpoints[i] = confidenceBreakpoint{bp.Confidence, bp.RelativeStddev}
	}
	sort.Slice(c.sortedBreakpoints, func(i, j int) bool {
		return c.sortedBreakpoints[i].confidence < c.sortedBreakpoints[j].confidence
	})
}

// MaterialFactor returns the emission factor for a material category, or nil if not found.
func (c *Config) MaterialFactor(cat models.MaterialCategory) *models.MaterialEmissionFactorEntry {
	return c.materialFactors[cat]
}

// MethodStddev returns the relative standard deviation for a carbon estimation method version.
// Returns 0.5 (LLM inference default) if not found.
func (c *Config) MethodStddev(version string) float32 {
	if v, ok := c.methodStddevs[version]; ok {
		return v
	}
	return 0.5
}

// Provenance formula names. Each is both the value stamped on
// [api.Provenance.Name] and the key looked up in
// [models.EstimatorConfig.provenance_versions], so the two must agree — a
// typo silently yields version 0 rather than an error. Callers outside this
// package (server/services/impact_metrics) stamp the same names.
const (
	ProvenanceWeightMaterialCarbon = "weight_material_carbon"
	ProvenanceSpendBasedCarbon     = "spend_based_carbon"
	ProvenancePreventedPurchase    = "prevented_purchase"
)

// ProvenanceVersion returns the current version for a named provenance source.
// Returns 0 if the name is not found in the config.
func (c *Config) ProvenanceVersion(name string) int32 {
	if v, ok := c.ProvenanceVersions[name]; ok {
		return v
	}
	return 0
}

// LLMConfidenceToRelativeStddev maps an LLM self-assessed confidence score (0.0-1.0) to
// a relative standard deviation, using linear interpolation between the configured breakpoints.
func (c *Config) LLMConfidenceToRelativeStddev(confidence float32) float32 {
	bps := c.sortedBreakpoints
	if len(bps) == 0 {
		return c.MethodStddev("llm_inference_v1")
	}

	// Clamp confidence to [0, 1].
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	// Below or at first breakpoint.
	if confidence <= bps[0].confidence {
		return bps[0].stddev
	}
	// Above or at last breakpoint.
	if confidence >= bps[len(bps)-1].confidence {
		return bps[len(bps)-1].stddev
	}

	// Linear interpolation between surrounding breakpoints.
	for i := 0; i < len(bps)-1; i++ {
		if confidence >= bps[i].confidence && confidence <= bps[i+1].confidence {
			t := (confidence - bps[i].confidence) / (bps[i+1].confidence - bps[i].confidence)
			return bps[i].stddev + t*(bps[i+1].stddev-bps[i].stddev)
		}
	}

	return c.MethodStddev("llm_inference_v1")
}

func (c *Config) validate() error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}

	g := c.Global
	if g == nil {
		return fmt.Errorf("global config is required")
	}
	if g.LoanPreventedPurchaseRate <= 0 || g.LoanPreventedPurchaseRate > 1 {
		return fmt.Errorf("loan_prevented_purchase_rate must be in (0, 1], got %f", g.LoanPreventedPurchaseRate)
	}
	if g.GiveawayPreventedPurchaseRate <= 0 || g.GiveawayPreventedPurchaseRate > 1 {
		return fmt.Errorf("giveaway_prevented_purchase_rate must be in (0, 1], got %f", g.GiveawayPreventedPurchaseRate)
	}
	if g.RepairCarbonFraction <= 0 || g.RepairCarbonFraction > 1 {
		return fmt.Errorf("repair_carbon_fraction must be in (0, 1], got %f", g.RepairCarbonFraction)
	}
	if g.TransportationCo2EPerMileKg <= 0 {
		return fmt.Errorf("transportation_co2e_per_mile_kg must be > 0, got %f", g.TransportationCo2EPerMileKg)
	}
	if g.MoneySavedRelativeStddev <= 0 || g.MoneySavedRelativeStddev > 1 {
		return fmt.Errorf("money_saved_relative_stddev must be in (0, 1], got %f", g.MoneySavedRelativeStddev)
	}
	if g.WasteCo2EPerKg <= 0 {
		return fmt.Errorf("waste_co2e_per_kg must be > 0, got %f", g.WasteCo2EPerKg)
	}
	// request_default_carbon_grams and experience_default_carbon_grams may be 0 (no carbon).

	// Method stddevs.
	if len(c.CarbonMethodStddevs) == 0 {
		return fmt.Errorf("carbon_method_stddevs must have at least one entry")
	}
	for _, entry := range c.CarbonMethodStddevs {
		if entry.Method == "" {
			return fmt.Errorf("carbon_method_stddevs entry has empty method")
		}
		if entry.RelativeStddev <= 0 || entry.RelativeStddev > 1 {
			return fmt.Errorf("carbon_method_stddevs.%s relative_stddev must be in (0, 1], got %f",
				entry.Method, entry.RelativeStddev)
		}
	}

	// LLM confidence breakpoints.
	if len(c.LlmConfidenceBreakpoints) == 0 {
		return fmt.Errorf("llm_confidence_breakpoints must have at least one entry")
	}
	for _, bp := range c.LlmConfidenceBreakpoints {
		if bp.Confidence < 0 || bp.Confidence > 1 {
			return fmt.Errorf("llm_confidence_breakpoints confidence must be in [0, 1], got %f", bp.Confidence)
		}
		if bp.RelativeStddev <= 0 {
			return fmt.Errorf("llm_confidence_breakpoints relative_stddev must be > 0, got %f for confidence %f",
				bp.RelativeStddev, bp.Confidence)
		}
	}

	// Material emission factors.
	if len(c.MaterialEmissionFactors) == 0 {
		return fmt.Errorf("material_emission_factors must have at least one entry")
	}
	for _, entry := range c.MaterialEmissionFactors {
		if entry.Category == models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED {
			return fmt.Errorf("material_emission_factors entry has unspecified category")
		}
		if entry.KgCo2EPerKg <= 0 {
			return fmt.Errorf("material_emission_factors %s kg_co2e_per_kg must be > 0, got %f",
				entry.Category, entry.KgCo2EPerKg)
		}
	}

	// Spend-based emission factors.
	if len(c.SpendBasedEmissionFactors) == 0 {
		return fmt.Errorf("spend_based_emission_factors must have at least one entry")
	}
	for name, factor := range c.SpendBasedEmissionFactors {
		if factor <= 0 {
			return fmt.Errorf("spend_based_emission_factors.%s must be > 0, got %f", name, factor)
		}
	}

	// Time defaults.
	td := c.TimeDefaults
	if td == nil {
		return fmt.Errorf("time_defaults is required")
	}
	if td.GearShoppingTimeMinutes <= 0 {
		return fmt.Errorf("time_defaults.gear_shopping_time_minutes must be > 0")
	}
	if td.RequestLaborTimeMinutes <= 0 {
		return fmt.Errorf("time_defaults.request_labor_time_minutes must be > 0")
	}
	if td.ExperienceDurationMinutes <= 0 {
		return fmt.Errorf("time_defaults.experience_duration_minutes must be > 0")
	}
	if td.DefaultRelativeStddev <= 0 || td.DefaultRelativeStddev > 1 {
		return fmt.Errorf("time_defaults.default_relative_stddev must be in (0, 1]")
	}
	if td.HintRelativeStddev <= 0 || td.HintRelativeStddev > 1 {
		return fmt.Errorf("time_defaults.hint_relative_stddev must be in (0, 1]")
	}

	// Quality Time config.
	qt := c.QualityTime
	if qt == nil {
		return fmt.Errorf("quality_time config is required")
	}
	if qt.GearHandoffDurationMinutes <= 0 {
		return fmt.Errorf("quality_time.gear_handoff_duration_minutes must be > 0")
	}
	if qt.GiveawayHandoffDurationMinutes <= 0 {
		return fmt.Errorf("quality_time.giveaway_handoff_duration_minutes must be > 0")
	}
	if qt.RequestDurationMinutes <= 0 {
		return fmt.Errorf("quality_time.request_duration_minutes must be > 0")
	}
	if qt.ExperienceDurationMinutes <= 0 {
		return fmt.Errorf("quality_time.experience_duration_minutes must be > 0")
	}
	if qt.ModalityInPersonShared <= 0 {
		return fmt.Errorf("quality_time.modality_in_person_shared must be > 0")
	}
	if qt.ModalityInPersonTransactional <= 0 {
		return fmt.Errorf("quality_time.modality_in_person_transactional must be > 0")
	}
	if qt.DurationRelativeStddev <= 0 || qt.DurationRelativeStddev > 1 {
		return fmt.Errorf("quality_time.duration_relative_stddev must be in (0, 1]")
	}
	if qt.ModalityRelativeStddev <= 0 || qt.ModalityRelativeStddev > 1 {
		return fmt.Errorf("quality_time.modality_relative_stddev must be in (0, 1]")
	}
	if qt.VulnerabilityRelativeStddev <= 0 || qt.VulnerabilityRelativeStddev > 1 {
		return fmt.Errorf("quality_time.vulnerability_relative_stddev must be in (0, 1]")
	}
	if qt.DurationLlmRelativeStddev <= 0 || qt.DurationLlmRelativeStddev > 1 {
		return fmt.Errorf("quality_time.duration_llm_relative_stddev must be in (0, 1]")
	}
	if qt.VulnerabilityLlmRelativeStddev <= 0 || qt.VulnerabilityLlmRelativeStddev > 1 {
		return fmt.Errorf("quality_time.vulnerability_llm_relative_stddev must be in (0, 1]")
	}
	if qt.SufficiencyWeeklyMinutes <= 0 {
		return fmt.Errorf("quality_time.sufficiency_weekly_minutes must be > 0")
	}

	// Equivalencies.
	eq := c.Equivalencies
	if eq == nil {
		return fmt.Errorf("equivalencies is required")
	}
	if eq.MilesDrivenPerKgCo2E <= 0 {
		return fmt.Errorf("equivalencies.miles_driven_per_kg_co2e must be > 0")
	}
	if eq.SmartphoneChargesPerKgCo2E <= 0 {
		return fmt.Errorf("equivalencies.smartphone_charges_per_kg_co2e must be > 0")
	}
	if eq.TreeSeedlings_10YrPerTonneCo2E <= 0 {
		return fmt.Errorf("equivalencies.tree_seedlings_10yr_per_tonne_co2e must be > 0")
	}

	return nil
}
