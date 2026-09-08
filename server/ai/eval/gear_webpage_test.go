package eval

import (
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestEvaluateGearWebpage_OnlyAssertedFieldsProduceChecks(t *testing.T) {
	c := GearWebpageCase{
		ID:        "minimal",
		PageTitle: "DEWALT Drill",
		Expected: GearWebpageExpected{
			TitleContains: []string{"drill"},
		},
	}
	gen := &ai.GearGeneration{Title: "DEWALT Cordless Drill"}
	checks := EvaluateGearWebpage(c, GearWebpageDefaults{}, gen)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Field != "title_contains" || !checks[0].Pass {
		t.Errorf("unexpected check: %+v", checks[0])
	}
}

func TestEvaluateGearWebpage_BrandContainsAnyMode(t *testing.T) {
	c := GearWebpageCase{
		ID: "brand-any",
		Expected: GearWebpageExpected{
			BrandContains: []string{"kitchenaid", "kitchen aid"},
		},
	}
	// Passes: "KitchenAid" matches "kitchenaid" (case-insensitive).
	checks := EvaluateGearWebpage(c, GearWebpageDefaults{}, &ai.GearGeneration{Brand: "KitchenAid"})
	if !checks[0].Pass {
		t.Errorf("expected pass, got %+v", checks[0])
	}
	// Passes: spaced variant.
	checks = EvaluateGearWebpage(c, GearWebpageDefaults{}, &ai.GearGeneration{Brand: "Kitchen Aid"})
	if !checks[0].Pass {
		t.Errorf("expected pass for spaced variant, got %+v", checks[0])
	}
	// Fails: unrelated brand.
	checks = EvaluateGearWebpage(c, GearWebpageDefaults{}, &ai.GearGeneration{Brand: "Cuisinart"})
	if checks[0].Pass {
		t.Errorf("expected fail, got %+v", checks[0])
	}
}

func TestEvaluateGearWebpage_HasDescription(t *testing.T) {
	yes := true
	c := GearWebpageCase{
		ID: "desc",
		Expected: GearWebpageExpected{
			HasDescription: &yes,
		},
	}
	checks := EvaluateGearWebpage(c, GearWebpageDefaults{}, &ai.GearGeneration{Description: "a chainsaw"})
	if !checks[0].Pass {
		t.Errorf("expected pass, got %+v", checks[0])
	}
	checks = EvaluateGearWebpage(c, GearWebpageDefaults{}, &ai.GearGeneration{Description: ""})
	if checks[0].Pass {
		t.Errorf("expected fail, got %+v", checks[0])
	}
}

func TestEvaluateGearWebpage_MaxConfidence(t *testing.T) {
	maxConf := 0.5
	c := GearWebpageCase{
		ID:       "non-product-page",
		Expected: GearWebpageExpected{MaxConfidence: &maxConf},
	}

	checks := EvaluateGearWebpage(c, GearWebpageDefaults{}, &ai.GearGeneration{Confidence: 0.3})
	if len(checks) != 1 || checks[0].Field != "max_confidence" || !checks[0].Pass {
		t.Fatalf("expected single passing max_confidence check, got %+v", checks)
	}

	checks = EvaluateGearWebpage(c, GearWebpageDefaults{}, &ai.GearGeneration{Confidence: 0.9})
	if len(checks) != 1 || checks[0].Pass {
		t.Fatalf("expected single failing max_confidence check, got %+v", checks)
	}
}

func TestLoadGearWebpageGoldens(t *testing.T) {
	g, err := LoadGearWebpageGoldens("testdata/gear_webpage_goldens.json")
	if err != nil {
		t.Fatalf("LoadGearWebpageGoldens: %v", err)
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
