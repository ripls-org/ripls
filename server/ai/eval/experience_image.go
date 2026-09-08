package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// ExperienceImageGoldens is the on-disk schema for
// testdata/experience_image_goldens.json. Targets
// GenerateExperienceFromImage — analyzes a photo of a flyer, scene, or
// event and produces a suggested experience. The model returns an
// ExperienceGeneration (same type as text-mode), but typically with
// weaker date/time/location signal than text since images rarely carry
// that structure — expectations here reflect that.
type ExperienceImageGoldens struct {
	Description string                  `json:"description"`
	Version     int                     `json:"version"`
	Defaults    ExperienceImageDefaults `json:"defaults"`
	Cases       []ExperienceImageCase   `json:"cases"`
}

type ExperienceImageDefaults struct {
	Region      string `json:"region"`
	CurrentTime string `json:"current_time"`
}

type ExperienceImageCase struct {
	ID          string                  `json:"id"`
	Tags        []string                `json:"tags,omitempty"`
	ImageFile   string                  `json:"image_file"`
	MimeType    string                  `json:"mime_type"`
	Notes       string                  `json:"notes,omitempty"`
	Region      string                  `json:"region,omitempty"`
	CurrentTime string                  `json:"current_time,omitempty"`
	Expected    ExperienceImageExpected `json:"expected"`
}

// ExperienceImageExpected mirrors ExperienceExpected but with any-mode
// defaults for TitleContains and LocationQueryContains (image goldens
// enumerate a set of reasonable phrasings and any match is acceptable,
// per the same logic as gear image-mode).
type ExperienceImageExpected struct {
	TitleContains     []string `json:"title_contains,omitempty"`
	TitleContainsMode string   `json:"title_contains_mode,omitempty"`
	HasDescription    *bool    `json:"has_description,omitempty"`
	Date              string   `json:"date,omitempty"`
	// DateEmpty asserts the model returned no date — the contract for flyers
	// and scenes carrying no calendar information, where inventing one would
	// surface a bogus calendar entry to the user.
	DateEmpty                 *bool    `json:"date_empty,omitempty"`
	Time                      string   `json:"time,omitempty"`
	TimeConfidenceAnyOf       []string `json:"time_confidence_any_of,omitempty"`
	LocationQueryContains     []string `json:"location_query_contains,omitempty"`
	LocationQueryContainsMode string   `json:"location_query_contains_mode,omitempty"`
	MinConfidence             *float64 `json:"min_confidence,omitempty"`
}

// LoadExperienceImageGoldens reads and parses the image goldens JSON file.
func LoadExperienceImageGoldens(path string) (*ExperienceImageGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g ExperienceImageGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateExperienceImage runs every asserted field for an image-based
// experience generation case.
func EvaluateExperienceImage(c ExperienceImageCase, defaults ExperienceImageDefaults, gen *ai.ExperienceGeneration) []Check {
	_ = defaults // defaults reserved for future shared tolerances
	var checks []Check
	exp := c.Expected

	if len(exp.TitleContains) > 0 {
		mode := modeOrAny(exp.TitleContainsMode)
		pass, reason := containsCheck(gen.Title, exp.TitleContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "title_contains", Pass: pass, Reason: reason})
	}
	if exp.HasDescription != nil {
		pass, reason := hasDescriptionCheck(gen.Description, *exp.HasDescription)
		checks = append(checks, Check{CaseID: c.ID, Field: "has_description", Pass: pass, Reason: reason})
	}
	if exp.Date != "" {
		pass := gen.Date == exp.Date
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected date %q, got %q", exp.Date, gen.Date)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "date", Pass: pass, Reason: reason})
	}
	if exp.DateEmpty != nil && *exp.DateEmpty {
		pass := strings.TrimSpace(gen.Date) == ""
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected empty date, got %q", gen.Date)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "date_empty", Pass: pass, Reason: reason})
	}
	if exp.Time != "" {
		pass := gen.Time == exp.Time
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected time %q, got %q", exp.Time, gen.Time)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "time", Pass: pass, Reason: reason})
	}
	if len(exp.TimeConfidenceAnyOf) > 0 {
		pass := false
		for _, allowed := range exp.TimeConfidenceAnyOf {
			if gen.TimeConfidence == allowed {
				pass = true
				break
			}
		}
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected time_confidence in %v, got %q", exp.TimeConfidenceAnyOf, gen.TimeConfidence)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "time_confidence_any_of", Pass: pass, Reason: reason})
	}
	if len(exp.LocationQueryContains) > 0 {
		mode := modeOrAny(exp.LocationQueryContainsMode)
		pass, reason := containsCheck(gen.LocationQuery, exp.LocationQueryContains, mode)
		checks = append(checks, Check{CaseID: c.ID, Field: "location_query_contains", Pass: pass, Reason: reason})
	}
	if exp.MinConfidence != nil {
		pass, reason := minConfidenceCheck(float64(gen.Confidence), *exp.MinConfidence)
		checks = append(checks, Check{CaseID: c.ID, Field: "min_confidence", Pass: pass, Reason: reason})
	}

	return checks
}
