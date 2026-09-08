package eval

import (
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestEvaluateGear_OnlyAssertedFieldsProduceChecks(t *testing.T) {
	c := GearCase{
		ID:     "minimal",
		Prompt: "drill",
		Expected: GearExpected{
			TitleContains: []string{"drill"},
		},
	}
	gen := &ai.GearGeneration{Title: "Cordless Drill"}
	checks := EvaluateGear(c, GearDefaults{}, gen)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Field != "title_contains" || !checks[0].Pass {
		t.Errorf("unexpected check: %+v", checks[0])
	}
}

func TestEvaluateGear_BrandEmpty(t *testing.T) {
	brandEmpty := true
	c := GearCase{
		ID:     "no-brand",
		Prompt: "generic camping tent",
		Expected: GearExpected{
			BrandEmpty: &brandEmpty,
		},
	}

	// Pass: model correctly returned empty brand.
	checks := EvaluateGear(c, GearDefaults{}, &ai.GearGeneration{Brand: ""})
	if len(checks) != 1 || !checks[0].Pass {
		t.Errorf("expected empty-brand check to pass, got %+v", checks)
	}

	// Fail: model hallucinated a brand.
	checks = EvaluateGear(c, GearDefaults{}, &ai.GearGeneration{Brand: "Generic"})
	if len(checks) != 1 || checks[0].Pass {
		t.Errorf("expected empty-brand check to fail, got %+v", checks)
	}
}

func TestEvaluateGear_MaterialCategoryEnum(t *testing.T) {
	c := GearCase{
		ID:     "mat",
		Prompt: "drill",
		Expected: GearExpected{
			MaterialCategoryAnyOf: []string{"cordless_power_tool", "corded_power_tool"},
		},
	}

	checks := EvaluateGear(c, GearDefaults{}, &ai.GearGeneration{MaterialCategory: "cordless_power_tool"})
	if !checks[0].Pass {
		t.Errorf("expected pass, got %+v", checks[0])
	}

	checks = EvaluateGear(c, GearDefaults{}, &ai.GearGeneration{MaterialCategory: "fabric"})
	if checks[0].Pass {
		t.Errorf("expected fail, got %+v", checks[0])
	}
}

func TestEvaluateGear_WeightWithinTolerance(t *testing.T) {
	approx := 2000.0
	c := GearCase{
		ID:     "weight",
		Prompt: "drill",
		Expected: GearExpected{
			WeightGramsApprox: &approx,
		},
	}
	defaults := GearDefaults{WeightTolerancePct: 30}

	checks := EvaluateGear(c, defaults, &ai.GearGeneration{WeightGrams: 2200})
	if !checks[0].Pass {
		t.Errorf("expected pass for 10%% off with 30%% tolerance, got %+v", checks[0])
	}

	checks = EvaluateGear(c, defaults, &ai.GearGeneration{WeightGrams: 5000})
	if checks[0].Pass {
		t.Errorf("expected fail for 150%% off with 30%% tolerance, got %+v", checks[0])
	}
}

func TestLoadGearGoldens(t *testing.T) {
	g, err := LoadGearGoldens("testdata/gear_goldens.json")
	if err != nil {
		t.Fatalf("LoadGearGoldens: %v", err)
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
