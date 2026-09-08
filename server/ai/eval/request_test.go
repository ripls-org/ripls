package eval

import (
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestEvaluateRequest_OnlyAssertedFieldsProduceChecks(t *testing.T) {
	c := RequestCase{
		ID:     "minimal",
		Prompt: "need help moving",
		Expected: RequestExpected{
			TitleContains: []string{"mov"},
		},
	}
	gen := &ai.RequestGeneration{Title: "Help Moving Furniture"}
	checks := EvaluateRequest(c, RequestDefaults{}, gen)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Field != "title_contains" || !checks[0].Pass {
		t.Errorf("unexpected check: %+v", checks[0])
	}
}

func TestEvaluateRequest_SearchKeywordsMustNotContain(t *testing.T) {
	c := RequestCase{
		ID:     "venue-leak",
		Prompt: "borrow a ladder from the Ace Hardware on Pearl",
		Expected: RequestExpected{
			SearchKeywordsMustNotContain: []string{"ace hardware", "pearl"},
		},
	}

	clean := &ai.RequestGeneration{SearchKeywords: []string{"ladder", "painting", "tools"}}
	checks := EvaluateRequest(c, RequestDefaults{}, clean)
	if len(checks) != 1 || checks[0].Field != "search_keywords_must_not_contain" || !checks[0].Pass {
		t.Fatalf("expected single passing must_not_contain check, got %+v", checks)
	}

	leaky := &ai.RequestGeneration{SearchKeywords: []string{"ladder", "Pearl Street"}}
	checks = EvaluateRequest(c, RequestDefaults{}, leaky)
	if len(checks) != 1 || checks[0].Pass {
		t.Fatalf("expected single failing must_not_contain check, got %+v", checks)
	}
}

func TestEvaluateRequest_FullExpectedBlock(t *testing.T) {
	minConf := 0.4
	valUSD := 75.0
	c := RequestCase{
		ID:     "full",
		Prompt: "need tutor for math",
		Expected: RequestExpected{
			TitleContains:         []string{"tutor"},
			LocationQueryContains: []string{"boulder"},
			SearchKeywords:        []string{"tutoring", "math", "education"},
			ValueEstimateUSD:      &valUSD,
			MinConfidence:         &minConf,
		},
	}
	gen := &ai.RequestGeneration{
		Title:          "Math Tutor for Homework",
		LocationQuery:  "Boulder, CO",
		SearchKeywords: []string{"tutoring", "math", "homework"},
		Confidence:     0.6,
		ValueEstimate:  &ai.ValueEstimate{EstimatedValueUSD: 60},
	}
	defaults := RequestDefaults{
		MinSearchKeywordsJaccard:  0.3,
		ValueEstimateTolerancePct: 40,
	}
	checks := EvaluateRequest(c, defaults, gen)

	// title, location_query_contains, search_keywords_jaccard, value_estimate_usd, min_confidence = 5
	if len(checks) != 5 {
		t.Fatalf("expected 5 checks, got %d: %+v", len(checks), checks)
	}
	for _, chk := range checks {
		if !chk.Pass {
			t.Errorf("expected all checks to pass, %s failed: %s", chk.Field, chk.Reason)
		}
	}
}

func TestLoadRequestGoldens(t *testing.T) {
	g, err := LoadRequestGoldens("testdata/request_goldens.json")
	if err != nil {
		t.Fatalf("LoadRequestGoldens: %v", err)
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
