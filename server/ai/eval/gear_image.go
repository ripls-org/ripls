package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// GearDetectionGoldens is the on-disk schema for testdata/gear_image_goldens.json.
// It targets the DetectGearInImage provider method, which analyzes a real
// product photo and returns a GearDetection (with Model and Description
// fields that text-mode GearGeneration does not populate).
type GearDetectionGoldens struct {
	Description string                `json:"description"`
	Version     int                   `json:"version"`
	Defaults    GearDetectionDefaults `json:"defaults"`
	Cases       []GearDetectionCase   `json:"cases"`
}

type GearDetectionDefaults struct {
	ValueEstimateTolerancePct float64 `json:"value_estimate_tolerance_pct"`
	WeightTolerancePct        float64 `json:"weight_tolerance_pct"`
}

type GearDetectionCase struct {
	ID        string                `json:"id"`
	Tags      []string              `json:"tags,omitempty"`
	ImageFile string                `json:"image_file"`
	MimeType  string                `json:"mime_type"`
	Expected  GearDetectionExpected `json:"expected"`
}

// GearDetectionExpected holds opt-in field assertions for image-based gear
// detection. Fields left empty / nil are not checked.
type GearDetectionExpected struct {
	// TitleContains / BrandContains / ModelContains each default to "any"
	// mode — passing one of the listed substrings is enough. Image-mode
	// prompts allow multiple valid phrasings of the same item (e.g. a
	// two-way radio can reasonably be titled "walkie talkie", "radio",
	// or the brand name), so an any-match is the right shape.
	TitleContains             []string `json:"title_contains,omitempty"`
	TitleContainsMode         string   `json:"title_contains_mode,omitempty"` // "any" (default) or "all"
	BrandContains             []string `json:"brand_contains,omitempty"`
	BrandContainsMode         string   `json:"brand_contains_mode,omitempty"`
	ModelContains             []string `json:"model_contains,omitempty"`
	ModelContainsMode         string   `json:"model_contains_mode,omitempty"`
	CategoryContainsAnyOf     []string `json:"category_contains_any_of,omitempty"`
	MaterialCategoryAnyOf     []string `json:"material_category_any_of,omitempty"`
	WeightGramsApprox         *float64 `json:"weight_grams_approx,omitempty"`
	WeightTolerancePct        *float64 `json:"weight_tolerance_pct,omitempty"`
	ValueEstimateUSD          *float64 `json:"value_estimate_usd,omitempty"`
	ValueEstimateTolerancePct *float64 `json:"value_estimate_tolerance_pct,omitempty"`
	HasDescription            *bool    `json:"has_description,omitempty"`
	MinConfidence             *float64 `json:"min_confidence,omitempty"`
}

// LoadGearDetectionGoldens reads and parses the gear-detection goldens JSON file.
func LoadGearDetectionGoldens(path string) (*GearDetectionGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g GearDetectionGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateGearDetection runs every asserted field for an image-based gear
// detection case and returns one Check per asserted field. Fields omitted
// from Expected produce no check.
func EvaluateGearDetection(c GearDetectionCase, defaults GearDetectionDefaults, det *ai.GearDetection) []Check {
	var checks []Check
	exp := c.Expected

	if len(exp.TitleContains) > 0 {
		mode := modeOrAny(exp.TitleContainsMode)
		pass, reason := containsCheck(det.Title, exp.TitleContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "title_contains", Pass: pass, Reason: reason})
	}
	if len(exp.BrandContains) > 0 {
		mode := modeOrAny(exp.BrandContainsMode)
		pass, reason := containsCheck(det.Brand, exp.BrandContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "brand_contains", Pass: pass, Reason: reason})
	}
	if len(exp.ModelContains) > 0 {
		mode := modeOrAny(exp.ModelContainsMode)
		pass, reason := containsCheck(det.Model, exp.ModelContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "model_contains", Pass: pass, Reason: reason})
	}
	if len(exp.CategoryContainsAnyOf) > 0 {
		pass, reason := containsCheck(det.Category, exp.CategoryContainsAnyOf, "any")
		checks = append(checks, Check{CaseID: c.ID, Field: "category_contains_any_of", Pass: pass, Reason: reason})
	}
	if len(exp.MaterialCategoryAnyOf) > 0 {
		pass := materialCategoryAnyOf(det.MaterialCategory, exp.MaterialCategoryAnyOf)
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected material_category in %v, got %q", exp.MaterialCategoryAnyOf, det.MaterialCategory)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "material_category_any_of", Pass: pass, Reason: reason})
	}
	if exp.WeightGramsApprox != nil {
		tolerancePct := defaults.WeightTolerancePct
		if exp.WeightTolerancePct != nil {
			tolerancePct = *exp.WeightTolerancePct
		}
		pass, reason := weightWithinTolerance(float64(det.WeightGrams), *exp.WeightGramsApprox, tolerancePct)
		checks = append(checks, Check{CaseID: c.ID, Field: "weight_grams_approx", Pass: pass, Reason: reason})
	}
	if exp.ValueEstimateUSD != nil {
		tolerancePct := defaults.ValueEstimateTolerancePct
		if exp.ValueEstimateTolerancePct != nil {
			tolerancePct = *exp.ValueEstimateTolerancePct
		}
		pass, reason := valueEstimateCheck(det.ValueEstimate, *exp.ValueEstimateUSD, tolerancePct)
		checks = append(checks, Check{CaseID: c.ID, Field: "value_estimate_usd", Pass: pass, Reason: reason})
	}
	if exp.HasDescription != nil {
		pass := *exp.HasDescription == (strings.TrimSpace(det.Description) != "")
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected has_description=%v, got description=%q", *exp.HasDescription, det.Description)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "has_description", Pass: pass, Reason: reason})
	}
	if exp.MinConfidence != nil {
		pass := float64(det.Confidence) >= *exp.MinConfidence
		reason := ""
		if !pass {
			reason = fmt.Sprintf("confidence %.2f < min %.2f", det.Confidence, *exp.MinConfidence)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "min_confidence", Pass: pass, Reason: reason})
	}

	return checks
}

// modeOrAny returns s if non-empty, else "any". Defaults *ContainsMode fields
// to the image/webpage semantics, where text-mode defaults to "all" instead.
func modeOrAny(s string) string {
	if s != "" {
		return s
	}
	return "any"
}
