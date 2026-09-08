package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// CommunityGoldens is the on-disk schema for testdata/community_goldens.json.
type CommunityGoldens struct {
	Description string            `json:"description"`
	Version     int               `json:"version"`
	Defaults    CommunityDefaults `json:"defaults"`
	Cases       []CommunityCase   `json:"cases"`
}

type CommunityDefaults struct {
	Region                   string  `json:"region"`
	MinSearchKeywordsJaccard float64 `json:"min_search_keywords_jaccard"`
}

type CommunityCase struct {
	ID       string            `json:"id"`
	Tags     []string          `json:"tags,omitempty"`
	Prompt   string            `json:"prompt"`
	Region   string            `json:"region,omitempty"`
	Expected CommunityExpected `json:"expected"`
}

// CommunityExpected holds opt-in field assertions for community generation.
// Community generation produces only image-search keywords — no title,
// description, confidence, location_query, or value_estimate dimensions —
// so every assertion here targets the emitted search_keywords.
type CommunityExpected struct {
	SearchKeywords           []string `json:"search_keywords,omitempty"`
	MinSearchKeywordsJaccard *float64 `json:"min_search_keywords_jaccard,omitempty"`
	// SearchKeywordsMustContain asserts each term appears as a substring of
	// at least one emitted keyword (case-insensitive). Anchors the test on
	// the primary topic the user's prompt is about.
	SearchKeywordsMustContain []string `json:"search_keywords_must_contain,omitempty"`
	// SearchKeywordsGeneral is a broad pool of acceptable category terms.
	// MinGeneralMatches asserts at least N keywords substring-match a term
	// in the pool. Tests breadth without being brittle to specific word
	// choice.
	SearchKeywordsGeneral []string `json:"search_keywords_general,omitempty"`
	MinGeneralMatches     *int     `json:"min_general_matches,omitempty"` // default 2 when SearchKeywordsGeneral set
}

// LoadCommunityGoldens reads and parses the community goldens JSON file.
func LoadCommunityGoldens(path string) (*CommunityGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g CommunityGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateCommunity runs every asserted field for a community case and
// returns one Check per asserted field. Fields omitted from Expected
// produce no check.
func EvaluateCommunity(c CommunityCase, defaults CommunityDefaults, gen *ai.CommunityGeneration) []Check {
	var checks []Check
	exp := c.Expected

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
			reason = fmt.Sprintf("primary-topic keyword(s) missing from %v: %v", gen.SearchKeywords, missing)
		}
		checks = append(checks, Check{CaseID: c.ID, Field: "search_keywords_must_contain", Pass: pass, Reason: reason})
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

	return checks
}
