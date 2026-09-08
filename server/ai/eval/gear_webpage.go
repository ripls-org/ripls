package eval

import (
	"encoding/json"
	"fmt"
	"os"

	"go.ripls.org/ripls/server/ai"
)

// GearWebpageGoldens is the on-disk schema for testdata/gear_webpage_goldens.json.
// It targets the GenerateGearFromWebpage provider method, which extracts a
// product description + value estimate from pre-fetched page title /
// description / body text (typically an Amazon or REI product page, or a
// review blog).
type GearWebpageGoldens struct {
	Description string              `json:"description"`
	Version     int                 `json:"version"`
	Defaults    GearWebpageDefaults `json:"defaults"`
	Cases       []GearWebpageCase   `json:"cases"`
}

type GearWebpageDefaults struct {
	Region                    string  `json:"region"`
	ValueEstimateTolerancePct float64 `json:"value_estimate_tolerance_pct"`
	WeightTolerancePct        float64 `json:"weight_tolerance_pct"`
}

type GearWebpageCase struct {
	ID              string              `json:"id"`
	Tags            []string            `json:"tags,omitempty"`
	PageTitle       string              `json:"page_title"`
	PageDescription string              `json:"page_description"`
	PageBody        string              `json:"page_body"`
	Region          string              `json:"region,omitempty"`
	Expected        GearWebpageExpected `json:"expected"`
}

// GearWebpageExpected mirrors GearDetectionExpected but without the Model
// field — GearGeneration (what the webpage method returns) has no Model
// attribute. Brand is asserted via BrandContains (any-match) rather than
// the exact Brand check used in text-mode Gear goldens, because product
// pages often phrase brand names inconsistently ("Kitchenaid" / "KitchenAid").
type GearWebpageExpected struct {
	TitleContains             []string `json:"title_contains,omitempty"`
	TitleContainsMode         string   `json:"title_contains_mode,omitempty"`
	BrandContains             []string `json:"brand_contains,omitempty"`
	BrandContainsMode         string   `json:"brand_contains_mode,omitempty"`
	CategoryContainsAnyOf     []string `json:"category_contains_any_of,omitempty"`
	MaterialCategoryAnyOf     []string `json:"material_category_any_of,omitempty"`
	WeightGramsApprox         *float64 `json:"weight_grams_approx,omitempty"`
	WeightTolerancePct        *float64 `json:"weight_tolerance_pct,omitempty"`
	ValueEstimateUSD          *float64 `json:"value_estimate_usd,omitempty"`
	ValueEstimateTolerancePct *float64 `json:"value_estimate_tolerance_pct,omitempty"`
	HasDescription            *bool    `json:"has_description,omitempty"`
	MinConfidence             *float64 `json:"min_confidence,omitempty"`
	// MaxConfidence asserts confidence stays at or below a ceiling — used on
	// pages that are NOT a product listing, where high confidence would mean
	// the model failed to notice there is no item to extract.
	MaxConfidence *float64 `json:"max_confidence,omitempty"`
}

// LoadGearWebpageGoldens reads and parses the gear-webpage goldens JSON file.
func LoadGearWebpageGoldens(path string) (*GearWebpageGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g GearWebpageGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateGearWebpage runs every asserted field for a webpage-based gear
// generation case and returns one Check per asserted field. Fields omitted
// from Expected produce no check.
func EvaluateGearWebpage(c GearWebpageCase, defaults GearWebpageDefaults, gen *ai.GearGeneration) []Check {
	var checks []Check
	exp := c.Expected

	if len(exp.TitleContains) > 0 {
		mode := modeOrAny(exp.TitleContainsMode)
		pass, reason := containsCheck(gen.Title, exp.TitleContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "title_contains", Pass: pass, Reason: reason})
	}
	if len(exp.BrandContains) > 0 {
		mode := modeOrAny(exp.BrandContainsMode)
		pass, reason := containsCheck(gen.Brand, exp.BrandContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "brand_contains", Pass: pass, Reason: reason})
	}
	if len(exp.CategoryContainsAnyOf) > 0 {
		pass, reason := containsCheck(gen.Category, exp.CategoryContainsAnyOf, "any")
		checks = append(checks, Check{CaseID: c.ID, Field: "category_contains_any_of", Pass: pass, Reason: reason})
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
	if exp.ValueEstimateUSD != nil {
		tolerancePct := defaults.ValueEstimateTolerancePct
		if exp.ValueEstimateTolerancePct != nil {
			tolerancePct = *exp.ValueEstimateTolerancePct
		}
		pass, reason := valueEstimateCheck(gen.ValueEstimate, *exp.ValueEstimateUSD, tolerancePct)
		checks = append(checks, Check{CaseID: c.ID, Field: "value_estimate_usd", Pass: pass, Reason: reason})
	}
	if exp.HasDescription != nil {
		pass, reason := hasDescriptionCheck(gen.Description, *exp.HasDescription)
		checks = append(checks, Check{CaseID: c.ID, Field: "has_description", Pass: pass, Reason: reason})
	}
	if exp.MinConfidence != nil {
		pass, reason := minConfidenceCheck(float64(gen.Confidence), *exp.MinConfidence)
		checks = append(checks, Check{CaseID: c.ID, Field: "min_confidence", Pass: pass, Reason: reason})
	}
	if exp.MaxConfidence != nil {
		pass := float64(gen.Confidence) <= *exp.MaxConfidence
		reason := ""
		if !pass {
			reason = fmt.Sprintf("confidence %.2f > max %.2f", gen.Confidence, *exp.MaxConfidence)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "max_confidence", Pass: pass, Reason: reason})
	}

	return checks
}
