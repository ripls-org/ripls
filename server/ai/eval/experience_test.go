package eval

import (
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestEvaluateExperience_OnlyAssertedFieldsProduceChecks(t *testing.T) {
	c := ExperienceCase{
		ID:     "minimal",
		Prompt: "hike",
		Expected: ExperienceExpected{
			TitleContains: []string{"hike"},
		},
	}
	gen := &ai.ExperienceGeneration{Title: "Morning Hike"}
	checks := EvaluateExperience(c, ExperienceDefaults{}, gen)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Field != "title_contains" || !checks[0].Pass {
		t.Errorf("unexpected check: %+v", checks[0])
	}
}

func TestEvaluateExperience_FullExpectedBlock(t *testing.T) {
	minConf := 0.3
	valUSD := 25.0
	tol := 30.0
	c := ExperienceCase{
		ID:     "full",
		Prompt: "cooking class $25",
		Expected: ExperienceExpected{
			TitleContains:             []string{"cook"},
			Date:                      "2026-04-25",
			Time:                      "14:00",
			TimeConfidenceAnyOf:       []string{"EXPLICIT"},
			LocationQueryContains:     []string{"community"},
			SearchKeywords:            []string{"cooking", "class"},
			ValueEstimateUSD:          &valUSD,
			ValueEstimateTolerancePct: &tol,
			MentionedNamesSet:         []string{"Sarah"},
			MinConfidence:             &minConf,
		},
	}
	gen := &ai.ExperienceGeneration{
		Title:          "Cooking Class",
		Date:           "2026-04-25",
		Time:           "14:00",
		TimeConfidence: "EXPLICIT",
		LocationQuery:  "community center",
		SearchKeywords: []string{"cooking", "class", "food"},
		ValueEstimate:  &ai.ValueEstimate{EstimatedValueUSD: 25},
		MentionedNames: []string{"Sarah"},
		Confidence:     0.8,
	}
	defaults := ExperienceDefaults{MinSearchKeywordsJaccard: 0.5}
	checks := EvaluateExperience(c, defaults, gen)

	// One check per asserted field: title, date, time, time_confidence,
	// location_query_contains, search_keywords_jaccard, value_estimate_usd,
	// mentioned_names_set, min_confidence = 9 checks.
	if len(checks) != 9 {
		t.Fatalf("expected 9 checks, got %d: %+v", len(checks), checks)
	}
	for _, chk := range checks {
		if !chk.Pass {
			t.Errorf("expected all checks to pass, %s failed: %s", chk.Field, chk.Reason)
		}
	}
}

func TestEvaluateExperience_SearchKeywordsMustNotContain(t *testing.T) {
	c := ExperienceCase{
		ID:     "venue-leak",
		Prompt: "go play pickleball at 3rd shot in longmont",
		Expected: ExperienceExpected{
			SearchKeywordsMustNotContain: []string{"3rd shot", "longmont"},
		},
	}

	clean := &ai.ExperienceGeneration{SearchKeywords: []string{"pickleball", "paddle", "court"}}
	checks := EvaluateExperience(c, ExperienceDefaults{}, clean)
	if len(checks) != 1 || checks[0].Field != "search_keywords_must_not_contain" || !checks[0].Pass {
		t.Fatalf("expected single passing must_not_contain check, got %+v", checks)
	}

	leaky := &ai.ExperienceGeneration{SearchKeywords: []string{"pickleball", "Longmont"}}
	checks = EvaluateExperience(c, ExperienceDefaults{}, leaky)
	if len(checks) != 1 || checks[0].Pass {
		t.Fatalf("expected single failing must_not_contain check, got %+v", checks)
	}
}

func TestLoadExperienceGoldens(t *testing.T) {
	g, err := LoadExperienceGoldens("testdata/experience_goldens.json")
	if err != nil {
		t.Fatalf("LoadExperienceGoldens: %v", err)
	}
	if len(g.Cases) == 0 {
		t.Fatal("expected at least one case")
	}
	if g.Defaults.Region == "" {
		t.Error("expected default region")
	}
	if g.Defaults.CurrentTime == "" {
		t.Error("expected default current_time")
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
