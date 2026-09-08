package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// GearGoldens is the on-disk schema for testdata/gear_goldens.json.
//
// Gear-from-text generation does NOT populate search_keywords (the webpage
// variant does). Signal comes from title, category, brand, material_category,
// weight_grams, and value_estimate.
type GearGoldens struct {
	Description string       `json:"description"`
	Version     int          `json:"version"`
	Defaults    GearDefaults `json:"defaults"`
	Cases       []GearCase   `json:"cases"`
}

type GearDefaults struct {
	Region                    string  `json:"region"`
	ValueEstimateTolerancePct float64 `json:"value_estimate_tolerance_pct"`
	WeightTolerancePct        float64 `json:"weight_tolerance_pct"`
}

type GearCase struct {
	ID       string       `json:"id"`
	Tags     []string     `json:"tags,omitempty"`
	Prompt   string       `json:"prompt"`
	Region   string       `json:"region,omitempty"`
	Expected GearExpected `json:"expected"`
}

// GearExpected holds opt-in field assertions for gear generation.
type GearExpected struct {
	TitleContains             []string `json:"title_contains,omitempty"`
	TitleContainsMode         string   `json:"title_contains_mode,omitempty"`
	CategoryContainsAnyOf     []string `json:"category_contains_any_of,omitempty"`
	Brand                     string   `json:"brand,omitempty"`               // exact, case-insensitive; "" means "not asserted"
	BrandContains             []string `json:"brand_contains,omitempty"`      // substring match, case-insensitive; use when brand has multiple valid forms (e.g., "North Face" vs "The North Face")
	BrandContainsMode         string   `json:"brand_contains_mode,omitempty"` // "any" (default) or "all"
	BrandEmpty                *bool    `json:"brand_empty,omitempty"`         // when true, expect empty brand (don't hallucinate)
	MaterialCategoryAnyOf     []string `json:"material_category_any_of,omitempty"`
	WeightGramsApprox         *float64 `json:"weight_grams_approx,omitempty"`
	WeightTolerancePct        *float64 `json:"weight_tolerance_pct,omitempty"`
	LocationQuery             string   `json:"location_query,omitempty"`
	LocationQueryContains     []string `json:"location_query_contains,omitempty"`
	LocationQueryContainsMode string   `json:"location_query_contains_mode,omitempty"`
	ValueEstimateUSD          *float64 `json:"value_estimate_usd,omitempty"`
	ValueEstimateTolerancePct *float64 `json:"value_estimate_tolerance_pct,omitempty"`
	MinConfidence             *float64 `json:"min_confidence,omitempty"`
}

// LoadGearGoldens reads and parses the gear goldens JSON file.
func LoadGearGoldens(path string) (*GearGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g GearGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateGear runs every asserted field for a gear case and returns one
// Check per asserted field. Fields omitted from Expected produce no check.
func EvaluateGear(c GearCase, defaults GearDefaults, gen *ai.GearGeneration) []Check {
	var checks []Check
	exp := c.Expected

	if len(exp.TitleContains) > 0 {
		mode := exp.TitleContainsMode
		if mode == "" {
			mode = containsModeAll
		}
		pass, reason := containsCheck(gen.Title, exp.TitleContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "title_contains", Pass: pass, Reason: reason})
	}
	if len(exp.CategoryContainsAnyOf) > 0 {
		pass, reason := containsCheck(gen.Category, exp.CategoryContainsAnyOf, "any")
		checks = append(checks, Check{CaseID: c.ID, Field: "category_contains_any_of", Pass: pass, Reason: reason})
	}
	if exp.Brand != "" {
		pass := strings.EqualFold(strings.TrimSpace(gen.Brand), exp.Brand)
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected brand %q, got %q", exp.Brand, gen.Brand)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "brand", Pass: pass, Reason: reason})
	}
	if len(exp.BrandContains) > 0 {
		mode := modeOrAny(exp.BrandContainsMode)
		pass, reason := containsCheck(gen.Brand, exp.BrandContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "brand_contains", Pass: pass, Reason: reason})
	}
	if exp.BrandEmpty != nil && *exp.BrandEmpty {
		// The prompt explicitly tells the model never to invent a brand when
		// none is stated. This guards against placeholders like "Unknown",
		// "Generic", "<UNKNOWN>" that sometimes slip through.
		pass := gen.Brand == ""
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected empty brand, got %q", gen.Brand)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "brand_empty", Pass: pass, Reason: reason})
	}
	if len(exp.MaterialCategoryAnyOf) > 0 {
		pass := materialCategoryAnyOf(gen.MaterialCategory, exp.MaterialCategoryAnyOf)
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected material_category in %v, got %q", exp.MaterialCategoryAnyOf, gen.MaterialCategory)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "material_category_any_of", Pass: pass, Reason: reason})
	}
	if exp.WeightGramsApprox != nil {
		tolerancePct := defaults.WeightTolerancePct
		if exp.WeightTolerancePct != nil {
			tolerancePct = *exp.WeightTolerancePct
		}
		pass, reason := weightWithinTolerance(float64(gen.WeightGrams), *exp.WeightGramsApprox, tolerancePct)
		checks = append(checks, Check{CaseID: c.ID, Field: "weight_grams_approx", Pass: pass, Reason: reason})
	}
	if exp.LocationQuery != "" {
		pass := strings.EqualFold(strings.TrimSpace(gen.LocationQuery), exp.LocationQuery)
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected exact %q, got %q", exp.LocationQuery, gen.LocationQuery)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "location_query", Pass: pass, Reason: reason})
	}
	if len(exp.LocationQueryContains) > 0 {
		mode := exp.LocationQueryContainsMode
		if mode == "" {
			mode = containsModeAll
		}
		pass, reason := containsCheck(gen.LocationQuery, exp.LocationQueryContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "location_query_contains", Pass: pass, Reason: reason})
	}
	if exp.ValueEstimateUSD != nil {
		tolerancePct := defaults.ValueEstimateTolerancePct
		if exp.ValueEstimateTolerancePct != nil {
			tolerancePct = *exp.ValueEstimateTolerancePct
		}
		pass, reason := valueEstimateCheck(gen.ValueEstimate, *exp.ValueEstimateUSD, tolerancePct)
		checks = append(checks, Check{CaseID: c.ID, Field: "value_estimate_usd", Pass: pass, Reason: reason})
	}
	if exp.MinConfidence != nil {
		pass := float64(gen.Confidence) >= *exp.MinConfidence
		reason := ""
		if !pass {
			reason = fmt.Sprintf("confidence %.2f < min %.2f", gen.Confidence, *exp.MinConfidence)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "min_confidence", Pass: pass, Reason: reason})
	}

	return checks
}
