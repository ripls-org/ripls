package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// RequestGoldens is the on-disk schema for testdata/request_goldens.json.
type RequestGoldens struct {
	Description string          `json:"description"`
	Version     int             `json:"version"`
	Defaults    RequestDefaults `json:"defaults"`
	Cases       []RequestCase   `json:"cases"`
}

type RequestDefaults struct {
	Region                    string  `json:"region"`
	MinSearchKeywordsJaccard  float64 `json:"min_search_keywords_jaccard"`
	ValueEstimateTolerancePct float64 `json:"value_estimate_tolerance_pct"`
}

type RequestCase struct {
	ID       string          `json:"id"`
	Tags     []string        `json:"tags,omitempty"`
	Prompt   string          `json:"prompt"`
	Region   string          `json:"region,omitempty"`
	Expected RequestExpected `json:"expected"`
}

// RequestExpected holds opt-in field assertions for request generation.
type RequestExpected struct {
	TitleContains             []string `json:"title_contains,omitempty"`
	TitleContainsMode         string   `json:"title_contains_mode,omitempty"`
	LocationQuery             string   `json:"location_query,omitempty"`
	LocationQueryContains     []string `json:"location_query_contains,omitempty"`
	LocationQueryContainsMode string   `json:"location_query_contains_mode,omitempty"`
	SearchKeywords            []string `json:"search_keywords,omitempty"`
	MinSearchKeywordsJaccard  *float64 `json:"min_search_keywords_jaccard,omitempty"`
	// SearchKeywordsMustContain asserts that each term appears as a substring of at least one emitted keyword (case-insensitive).
	// Used to anchor the test on the primary item/service the user's prompt is about, so a model that drifts to only context
	// terms ("DIY, workshop, project") without naming the actual thing ("drill") fails the grounding check.
	SearchKeywordsMustContain []string `json:"search_keywords_must_contain,omitempty"`
	// SearchKeywordsMustNotContain asserts that none of the terms appear as a substring of any emitted
	// keyword (case-insensitive). Set on cases whose prompts mention venues, cities, businesses, or people
	// to catch proper-noun leakage into stock-media queries (#1209).
	SearchKeywordsMustNotContain []string `json:"search_keywords_must_not_contain,omitempty"`
	// SearchKeywordsGeneral is a broad pool of acceptable general-category terms (activity, setting, mood, category).
	// Paired with MinGeneralMatches, this asserts the model produced at least N keywords that substring-match a term
	// in the pool. Tests keyword breadth (progression from the specific item to broader category terms for stock-image
	// fallback) without being brittle to the model's specific word choice.
	SearchKeywordsGeneral     []string `json:"search_keywords_general,omitempty"`
	MinGeneralMatches         *int     `json:"min_general_matches,omitempty"` // default 2 when SearchKeywordsGeneral set
	ValueEstimateUSD          *float64 `json:"value_estimate_usd,omitempty"`
	ValueEstimateTolerancePct *float64 `json:"value_estimate_tolerance_pct,omitempty"`
	MinConfidence             *float64 `json:"min_confidence,omitempty"`
}

// LoadRequestGoldens reads and parses the request goldens JSON file.
func LoadRequestGoldens(path string) (*RequestGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g RequestGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateRequest runs every asserted field for a request case and returns
// one Check per asserted field. Fields omitted from Expected produce no check.
func EvaluateRequest(c RequestCase, defaults RequestDefaults, gen *ai.RequestGeneration) []Check {
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
			reason = fmt.Sprintf("primary-item keyword(s) missing from %v: %v", gen.SearchKeywords, missing)
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
