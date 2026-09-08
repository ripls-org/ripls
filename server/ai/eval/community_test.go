package eval

import (
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestEvaluateCommunity_OnlyAssertedFieldsProduceChecks(t *testing.T) {
	c := CommunityCase{
		ID:     "minimal",
		Prompt: "rock climbers in Boulder",
		Expected: CommunityExpected{
			SearchKeywordsMustContain: []string{"climb"},
		},
	}
	gen := &ai.CommunityGeneration{SearchKeywords: []string{"rock climbing", "bouldering"}}
	checks := EvaluateCommunity(c, CommunityDefaults{}, gen)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Field != "search_keywords_must_contain" || !checks[0].Pass {
		t.Errorf("unexpected check: %+v", checks[0])
	}
}

func TestEvaluateCommunity_FullExpectedBlock(t *testing.T) {
	lo := 2
	c := CommunityCase{
		ID:     "full",
		Prompt: "neighborhood gardening club",
		Expected: CommunityExpected{
			SearchKeywords:            []string{"gardening", "plants", "neighborhood"},
			SearchKeywordsMustContain: []string{"garden"},
			SearchKeywordsGeneral:     []string{"plants", "vegetables", "outdoor", "growing"},
			MinGeneralMatches:         &lo,
		},
	}
	gen := &ai.CommunityGeneration{
		SearchKeywords: []string{"gardening", "plants", "vegetables", "outdoor"},
	}
	defaults := CommunityDefaults{
		MinSearchKeywordsJaccard: 0.3,
	}
	checks := EvaluateCommunity(c, defaults, gen)

	// search_keywords_jaccard, must_contain, general_matches = 3
	if len(checks) != 3 {
		t.Fatalf("expected 3 checks, got %d: %+v", len(checks), checks)
	}
	for _, chk := range checks {
		if !chk.Pass {
			t.Errorf("expected all checks to pass, %s failed: %s", chk.Field, chk.Reason)
		}
	}
}

func TestLoadCommunityGoldens(t *testing.T) {
	g, err := LoadCommunityGoldens("testdata/community_goldens.json")
	if err != nil {
		t.Fatalf("LoadCommunityGoldens: %v", err)
	}
	if len(g.Cases) == 0 {
		t.Fatal("expected at least one case")
	}
	if g.Defaults.Region == "" {
		t.Error("expected default region")
	}
	ids := map[string]bool{}
	for _, c := range g.Cases {
		if c.ID == "" {
			t.Error("case with empty ID")
		}
		if c.Prompt == "" {
			t.Errorf("case %s has empty prompt", c.ID)
		}
		if ids[c.ID] {
			t.Errorf("duplicate case ID %q", c.ID)
		}
		ids[c.ID] = true
	}
}
