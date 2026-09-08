package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// ExperienceGoldens is the on-disk schema for testdata/experience_goldens.json.
type ExperienceGoldens struct {
	Description string             `json:"description"`
	Version     int                `json:"version"`
	Defaults    ExperienceDefaults `json:"defaults"`
	Cases       []ExperienceCase   `json:"cases"`
}

// ExperienceDefaults supplies per-field fallbacks. A case may override any
// of these inline; whatever is set on the case wins.
type ExperienceDefaults struct {
	Region                    string  `json:"region"`
	CurrentTime               string  `json:"current_time"`
	MinSearchKeywordsJaccard  float64 `json:"min_search_keywords_jaccard"`
	ValueEstimateTolerancePct float64 `json:"value_estimate_tolerance_pct"`
}

// ExperienceCase is a single golden — prompt + context + asserted fields.
type ExperienceCase struct {
	ID          string             `json:"id"`
	Tags        []string           `json:"tags,omitempty"`
	Prompt      string             `json:"prompt"`
	Region      string             `json:"region,omitempty"`
	CurrentTime string             `json:"current_time,omitempty"`
	Expected    ExperienceExpected `json:"expected"`
}

// ExperienceExpected holds opt-in field assertions. A field left zero/nil is
// not asserted. Pointer types for numeric thresholds distinguish "not asserted"
// from "asserted to be zero".
type ExperienceExpected struct {
	TitleContains             []string `json:"title_contains,omitempty"`
	TitleContainsMode         string   `json:"title_contains_mode,omitempty"` // "all" (default) or "any"
	Date                      string   `json:"date,omitempty"`
	Time                      string   `json:"time,omitempty"`
	TimeConfidenceAnyOf       []string `json:"time_confidence_any_of,omitempty"`
	LocationQuery             string   `json:"location_query,omitempty"`
	LocationQueryContains     []string `json:"location_query_contains,omitempty"`
	LocationQueryContainsMode string   `json:"location_query_contains_mode,omitempty"`
	SearchKeywords            []string `json:"search_keywords,omitempty"`
	MinSearchKeywordsJaccard  *float64 `json:"min_search_keywords_jaccard,omitempty"`
	// SearchKeywordsMustContain asserts that each term appears as a substring of at least one emitted keyword
	// (case-insensitive). Anchors the test on the primary activity the user's prompt describes, so a model that
	// drifts to only surrounding context ("outdoor, nature, weekend") without naming the activity ("hiking")
	// fails the grounding check.
	SearchKeywordsMustContain []string `json:"search_keywords_must_contain,omitempty"`
	// SearchKeywordsMustNotContain asserts that none of the terms appear as a substring of any emitted
	// keyword (case-insensitive). Set on cases whose prompts mention venues, cities, businesses, or people
	// to catch proper-noun leakage into stock-media queries (#1209) — photo libraries index subject matter,
	// not place names, so a leaked "Longmont" or "3rd Shot" degrades the search this field feeds.
	SearchKeywordsMustNotContain []string `json:"search_keywords_must_not_contain,omitempty"`
	// SearchKeywordsGeneral is a broad pool of acceptable general-category terms (mood, setting, activity
	// domain). Paired with MinGeneralMatches, this asserts the model produced at least N keywords that
	// substring-match a term in the pool. Tests keyword breadth without being brittle to the model's
	// specific word choice.
	SearchKeywordsGeneral     []string `json:"search_keywords_general,omitempty"`
	MinGeneralMatches         *int     `json:"min_general_matches,omitempty"` // default 2 when SearchKeywordsGeneral set
	ValueEstimateUSD          *float64 `json:"value_estimate_usd,omitempty"`
	ValueEstimateTolerancePct *float64 `json:"value_estimate_tolerance_pct,omitempty"`
	MentionedNamesSet         []string `json:"mentioned_names_set,omitempty"`
	MinConfidence             *float64 `json:"min_confidence,omitempty"`
}

// LoadExperienceGoldens reads and parses the experience goldens JSON file.
func LoadExperienceGoldens(path string) (*ExperienceGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g ExperienceGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateExperience runs every asserted field for a case against the
// generated ExperienceGeneration and returns one Check per asserted field.
// Fields omitted from Expected produce no check.
func EvaluateExperience(c ExperienceCase, defaults ExperienceDefaults, gen *ai.ExperienceGeneration) []Check {
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
	if exp.Date != "" {
		pass := gen.Date == exp.Date
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected %q, got %q", exp.Date, gen.Date)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "date", Pass: pass, Reason: reason})
	}
	if exp.Time != "" {
		pass := gen.Time == exp.Time
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected %q, got %q", exp.Time, gen.Time)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "time", Pass: pass, Reason: reason})
	}
	if len(exp.TimeConfidenceAnyOf) > 0 {
		pass := false
		for _, allowed := range exp.TimeConfidenceAnyOf {
			if strings.EqualFold(gen.TimeConfidence, allowed) {
				pass = true
				break
			}
		}
		reason := ""
		if !pass {
			reason = fmt.Sprintf("expected one of %v, got %q", exp.TimeConfidenceAnyOf, gen.TimeConfidence)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "time_confidence", Pass: pass, Reason: reason})
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
	if len(exp.SearchKeywords) > 0 {
		threshold := defaults.MinSearchKeywordsJaccard
		if exp.MinSearchKeywordsJaccard != nil {
			threshold = *exp.MinSearchKeywordsJaccard
		}
		sim := jaccardSimilarity(exp.SearchKeywords, gen.SearchKeywords)
		pass := sim >= threshold
		reason := ""
		if !pass {
			reason = fmt.Sprintf("Jaccard %.2f < threshold %.2f (expected %v, got %v)",
				sim, threshold, exp.SearchKeywords, gen.SearchKeywords)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "search_keywords_jaccard", Pass: pass, Reason: reason})
	}
	if len(exp.SearchKeywordsMustContain) > 0 {
		joined := strings.ToLower(strings.Join(gen.SearchKeywords, " | "))
		var missing []string
		for _, term := range exp.SearchKeywordsMustContain {
			if !strings.Contains(joined, strings.ToLower(term)) {
				missing = append(missing, term)
			}
		}
		pass := len(missing) == 0
		reason := ""
		if !pass {
			reason = fmt.Sprintf("primary-activity keyword(s) missing from %v: %v", gen.SearchKeywords, missing)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "search_keywords_must_contain", Pass: pass, Reason: reason})
	}
	if len(exp.SearchKeywordsMustNotContain) > 0 {
		pass, reason := keywordsMustNotContainCheck(gen.SearchKeywords, exp.SearchKeywordsMustNotContain)
		checks = append(checks, Check{CaseID: c.ID, Field: "search_keywords_must_not_contain", Pass: pass, Reason: reason})
	}
	if len(exp.SearchKeywordsGeneral) > 0 {
		lo := 2
		if exp.MinGeneralMatches != nil {
			lo = *exp.MinGeneralMatches
		}
		joined := strings.ToLower(strings.Join(gen.SearchKeywords, " | "))
		var matched []string
		for _, term := range exp.SearchKeywordsGeneral {
			if strings.Contains(joined, strings.ToLower(term)) {
				matched = append(matched, term)
			}
		}
		pass := len(matched) >= lo
		reason := ""
		if !pass {
			reason = fmt.Sprintf("general-category breadth: matched %d of %v in %v, need ≥%d", len(matched), exp.SearchKeywordsGeneral, gen.SearchKeywords, lo)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "search_keywords_general_matches", Pass: pass, Reason: reason})
	}
	if exp.ValueEstimateUSD != nil {
		tolerancePct := defaults.ValueEstimateTolerancePct
		if exp.ValueEstimateTolerancePct != nil {
			tolerancePct = *exp.ValueEstimateTolerancePct
		}
		pass, reason := valueEstimateCheck(gen.ValueEstimate, *exp.ValueEstimateUSD, tolerancePct)
		checks = append(checks, Check{CaseID: c.ID, Field: "value_estimate_usd", Pass: pass, Reason: reason})
	}
	if len(exp.MentionedNamesSet) > 0 {
		pass, reason := setEqualCaseInsensitive(exp.MentionedNamesSet, gen.MentionedNames)
		checks = append(checks, Check{CaseID: c.ID, Field: "mentioned_names_set", Pass: pass, Reason: reason})
	}
	if exp.MinConfidence != nil {
		pass := float64(gen.Confidence) >= *exp.MinConfidence
		reason := ""
		if !pass {
			reason = fmt.Sprintf("confidence %.2f < lo %.2f", gen.Confidence, *exp.MinConfidence)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "min_confidence", Pass: pass, Reason: reason})
	}

	return checks
}
