package eval

import (
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestEvaluateExperienceWebpage_OnlyAssertedFieldsProduceChecks(t *testing.T) {
	c := ExperienceWebpageCase{
		ID:        "minimal",
		PageTitle: "Concert at Red Rocks",
		Expected: ExperienceWebpageExpected{
			TitleContains: []string{"concert"},
		},
	}
	gen := &ai.ExperienceGeneration{Title: "Red Rocks Concert"}
	checks := EvaluateExperienceWebpage(c, ExperienceWebpageDefaults{}, gen)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if !checks[0].Pass {
		t.Errorf("expected pass, got %+v", checks[0])
	}
}

func TestEvaluateExperienceWebpage_DateTimeLocation(t *testing.T) {
	c := ExperienceWebpageCase{
		ID:        "event",
		PageTitle: "x",
		Expected: ExperienceWebpageExpected{
			Date:                  "2026-06-14",
			Time:                  "19:00",
			TimeConfidenceAnyOf:   []string{"EXPLICIT"},
			LocationQueryContains: []string{"red rocks", "morrison"},
		},
	}
	gen := &ai.ExperienceGeneration{
		Date:           "2026-06-14",
		Time:           "19:00",
		TimeConfidence: "EXPLICIT",
		LocationQuery:  "Red Rocks Amphitheatre, Morrison CO",
	}
	checks := EvaluateExperienceWebpage(c, ExperienceWebpageDefaults{}, gen)
	for _, chk := range checks {
		if !chk.Pass {
			t.Errorf("expected all checks pass, %s failed: %s", chk.Field, chk.Reason)
		}
	}
}

func TestEvaluateExperienceWebpage_EmptyAndCeilingAssertions(t *testing.T) {
	truth := true
	maxConf := 0.5
	c := ExperienceWebpageCase{
		ID: "non-event-page",
		Expected: ExperienceWebpageExpected{
			DateEmpty:          &truth,
			LocationQueryEmpty: &truth,
			MaxConfidence:      &maxConf,
		},
	}

	good := &ai.ExperienceGeneration{Date: "", LocationQuery: "", Confidence: 0.4}
	checks := EvaluateExperienceWebpage(c, ExperienceWebpageDefaults{}, good)
	if len(checks) != 3 {
		t.Fatalf("expected 3 checks, got %d: %+v", len(checks), checks)
	}
	for _, chk := range checks {
		if !chk.Pass {
			t.Errorf("expected %s to pass: %s", chk.Field, chk.Reason)
		}
	}

	bad := &ai.ExperienceGeneration{Date: "2026-06-01", LocationQuery: "Somewhere", Confidence: 0.9}
	for _, chk := range EvaluateExperienceWebpage(c, ExperienceWebpageDefaults{}, bad) {
		if chk.Pass {
			t.Errorf("expected %s to fail for non-empty/high-confidence generation", chk.Field)
		}
	}
}

func TestLoadExperienceWebpageGoldens(t *testing.T) {
	g, err := LoadExperienceWebpageGoldens("testdata/experience_webpage_goldens.json")
	if err != nil {
		t.Fatalf("LoadExperienceWebpageGoldens: %v", err)
	}
	if len(g.Cases) == 0 {
		t.Fatal("expected at least one case")
	}
	ids := map[string]bool{}
	for _, c := range g.Cases {
		if c.ID == "" {
			t.Error("case with empty ID")
		}
		if c.PageTitle == "" && c.PageBody == "" {
			t.Errorf("case %s has no page_title and no page_body", c.ID)
		}
		if ids[c.ID] {
			t.Errorf("duplicate case ID %q", c.ID)
		}
		ids[c.ID] = true
	}
}
