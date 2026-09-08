package eval

import (
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestEvaluateExperienceImage_OnlyAssertedFieldsProduceChecks(t *testing.T) {
	c := ExperienceImageCase{
		ID:        "minimal",
		ImageFile: "scene.jpg",
		Expected: ExperienceImageExpected{
			TitleContains: []string{"hike"},
		},
	}
	gen := &ai.ExperienceGeneration{Title: "Morning Hike"}
	checks := EvaluateExperienceImage(c, ExperienceImageDefaults{}, gen)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Field != "title_contains" || !checks[0].Pass {
		t.Errorf("unexpected check: %+v", checks[0])
	}
}

func TestEvaluateExperienceImage_TitleContainsAnyMode(t *testing.T) {
	c := ExperienceImageCase{
		ID:        "any",
		ImageFile: "x.jpg",
		Expected: ExperienceImageExpected{
			TitleContains: []string{"beer", "brew", "happy hour", "social"},
		},
	}
	checks := EvaluateExperienceImage(c, ExperienceImageDefaults{}, &ai.ExperienceGeneration{Title: "Beer Tasting"})
	if !checks[0].Pass {
		t.Errorf("expected pass on any-match, got %+v", checks[0])
	}
	checks = EvaluateExperienceImage(c, ExperienceImageDefaults{}, &ai.ExperienceGeneration{Title: "Knitting Circle"})
	if checks[0].Pass {
		t.Errorf("expected fail, got %+v", checks[0])
	}
}

func TestEvaluateExperienceImage_DateAndTime(t *testing.T) {
	c := ExperienceImageCase{
		ID:        "date-time",
		ImageFile: "flyer.jpg",
		Expected: ExperienceImageExpected{
			Date: "2026-06-14",
			Time: "19:00",
		},
	}
	gen := &ai.ExperienceGeneration{Date: "2026-06-14", Time: "19:00"}
	checks := EvaluateExperienceImage(c, ExperienceImageDefaults{}, gen)
	if len(checks) != 2 || !checks[0].Pass || !checks[1].Pass {
		t.Errorf("expected both checks pass, got %+v", checks)
	}
	gen = &ai.ExperienceGeneration{Date: "2026-06-15", Time: "19:00"}
	checks = EvaluateExperienceImage(c, ExperienceImageDefaults{}, gen)
	dateOK := true
	for _, chk := range checks {
		if chk.Field == "date" {
			dateOK = chk.Pass
		}
	}
	if dateOK {
		t.Errorf("expected date to fail on wrong day, got pass")
	}
}

func TestEvaluateExperienceImage_TimeConfidenceAnyOf(t *testing.T) {
	c := ExperienceImageCase{
		ID:        "tc",
		ImageFile: "x.jpg",
		Expected: ExperienceImageExpected{
			TimeConfidenceAnyOf: []string{"EXPLICIT", "INFERRED"},
		},
	}
	checks := EvaluateExperienceImage(c, ExperienceImageDefaults{}, &ai.ExperienceGeneration{TimeConfidence: "EXPLICIT"})
	if !checks[0].Pass {
		t.Errorf("expected pass, got %+v", checks[0])
	}
	checks = EvaluateExperienceImage(c, ExperienceImageDefaults{}, &ai.ExperienceGeneration{TimeConfidence: "UNKNOWN"})
	if checks[0].Pass {
		t.Errorf("expected fail, got %+v", checks[0])
	}
}

func TestLoadExperienceImageGoldens(t *testing.T) {
	g, err := LoadExperienceImageGoldens("testdata/experience_image_goldens.json")
	if err != nil {
		t.Fatalf("LoadExperienceImageGoldens: %v", err)
	}
	if len(g.Cases) == 0 {
		t.Fatal("expected at least one case")
	}
	ids := map[string]bool{}
	for _, c := range g.Cases {
		if c.ID == "" {
			t.Error("case with empty ID")
		}
		if c.ImageFile == "" {
			t.Errorf("case %s has empty image_file", c.ID)
		}
		if ids[c.ID] {
			t.Errorf("duplicate case ID %q", c.ID)
		}
		ids[c.ID] = true
	}
}
