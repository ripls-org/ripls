package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// ExperienceWebpageGoldens is the on-disk schema for
// testdata/experience_webpage_goldens.json. Targets
// GenerateExperienceFromWebpage — parses an event page (Eventbrite-style
// or similar) into an ExperienceGeneration. Event pages typically carry
// explicit date, time, and location, so these cases assert those fields
// more strictly than the image-mode ones.
type ExperienceWebpageGoldens struct {
	Description string                    `json:"description"`
	Version     int                       `json:"version"`
	Defaults    ExperienceWebpageDefaults `json:"defaults"`
	Cases       []ExperienceWebpageCase   `json:"cases"`
}

type ExperienceWebpageDefaults struct {
	Region      string `json:"region"`
	CurrentTime string `json:"current_time"`
}

type ExperienceWebpageCase struct {
	ID              string                    `json:"id"`
	Tags            []string                  `json:"tags,omitempty"`
	PageTitle       string                    `json:"page_title"`
	PageDescription string                    `json:"page_description"`
	PageBody        string                    `json:"page_body"`
	Region          string                    `json:"region,omitempty"`
	CurrentTime     string                    `json:"current_time,omitempty"`
	Expected        ExperienceWebpageExpected `json:"expected"`
}

// ExperienceWebpageExpected is the same shape as ExperienceImageExpected —
// same fields, same semantics. Extracted as its own type for symmetry
// with the other per-mode goldens.
type ExperienceWebpageExpected struct {
	TitleContains     []string `json:"title_contains,omitempty"`
	TitleContainsMode string   `json:"title_contains_mode,omitempty"`
	HasDescription    *bool    `json:"has_description,omitempty"`
	Date              string   `json:"date,omitempty"`
	// DateEmpty asserts the model returned no date at all — the contract for
	// venue landing pages and non-event pages, where inventing a date would
	// surface a bogus calendar entry to the user.
	DateEmpty           *bool    `json:"date_empty,omitempty"`
	Time                string   `json:"time,omitempty"`
	TimeConfidenceAnyOf []string `json:"time_confidence_any_of,omitempty"`
	// LocationQueryEmpty asserts location_query is empty — the contract for
	// online/virtual events, where the prompt directs the model to mention
	// the format in the description but leave the geocodable query blank.
	LocationQueryEmpty        *bool    `json:"location_query_empty,omitempty"`
	LocationQueryContains     []string `json:"location_query_contains,omitempty"`
	LocationQueryContainsMode string   `json:"location_query_contains_mode,omitempty"`
	MinConfidence             *float64 `json:"min_confidence,omitempty"`
	// MaxConfidence asserts confidence stays at or below a ceiling — used on
	// pages that are NOT a specific event, where high confidence would mean
	// the model failed to notice there is nothing to extract.
	MaxConfidence *float64 `json:"max_confidence,omitempty"`
}

// LoadExperienceWebpageGoldens reads and parses the webpage goldens JSON file.
func LoadExperienceWebpageGoldens(path string) (*ExperienceWebpageGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g ExperienceWebpageGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateExperienceWebpage runs every asserted field for a webpage-based
// experience generation case.
func EvaluateExperienceWebpage(c ExperienceWebpageCase, defaults ExperienceWebpageDefaults, gen *ai.ExperienceGeneration) []Check {
	_ = defaults
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
	if exp.LocationQueryEmpty != nil && *exp.LocationQueryEmpty {
		pass := strings.TrimSpace(gen.LocationQuery) == ""
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected empty location_query, got %q", gen.LocationQuery)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "location_query_empty", Pass: pass, Reason: reason})
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
