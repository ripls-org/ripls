package eval

import (
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestEvaluateGearDetection_OnlyAssertedFieldsProduceChecks(t *testing.T) {
	c := GearDetectionCase{
		ID:        "minimal",
		ImageFile: "ski.jpeg",
		Expected: GearDetectionExpected{
			TitleContains: []string{"ski"},
		},
	}
	det := &ai.GearDetection{Title: "Atomic Maverick Ski"}
	checks := EvaluateGearDetection(c, GearDetectionDefaults{}, det)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d: %+v", len(checks), checks)
	}
	if checks[0].Field != "title_contains" || !checks[0].Pass {
		t.Errorf("unexpected check: %+v", checks[0])
	}
}

// TestEvaluateGearDetection_TitleContainsAnyMode confirms image-mode
// TitleContains defaults to "any" rather than "all" — goldens list
// alternative valid phrasings and only one needs to match.
func TestEvaluateGearDetection_TitleContainsAnyMode(t *testing.T) {
	c := GearDetectionCase{
		ID:        "any-mode",
		ImageFile: "radio.jpeg",
		Expected: GearDetectionExpected{
			TitleContains: []string{"rocky talkie", "walkie", "radio", "two-way"},
		},
	}
	// Passes because "Radio" matches "radio" (case-insensitive any).
	checks := EvaluateGearDetection(c, GearDetectionDefaults{}, &ai.GearDetection{Title: "Handheld Radio"})
	if !checks[0].Pass {
		t.Errorf("expected any-match pass, got %+v", checks[0])
	}
	// Fails because no listed substring appears.
	checks = EvaluateGearDetection(c, GearDetectionDefaults{}, &ai.GearDetection{Title: "Antique Lamp"})
	if checks[0].Pass {
		t.Errorf("expected fail, got %+v", checks[0])
	}
}

func TestEvaluateGearDetection_ModelContains(t *testing.T) {
	c := GearDetectionCase{
		ID:        "model",
		ImageFile: "tent.jpg",
		Expected: GearDetectionExpected{
			ModelContains: []string{"half dome"},
		},
	}
	checks := EvaluateGearDetection(c, GearDetectionDefaults{}, &ai.GearDetection{Model: "Half Dome SL 2"})
	if !checks[0].Pass {
		t.Errorf("expected pass, got %+v", checks[0])
	}
	checks = EvaluateGearDetection(c, GearDetectionDefaults{}, &ai.GearDetection{Model: "Kingdom 6"})
	if checks[0].Pass {
		t.Errorf("expected fail, got %+v", checks[0])
	}
}

func TestEvaluateGearDetection_HasDescription(t *testing.T) {
	yes := true
	c := GearDetectionCase{
		ID:        "has-desc",
		ImageFile: "x.jpg",
		Expected: GearDetectionExpected{
			HasDescription: &yes,
		},
	}
	checks := EvaluateGearDetection(c, GearDetectionDefaults{}, &ai.GearDetection{Description: "A handheld radio"})
	if !checks[0].Pass {
		t.Errorf("expected pass for non-empty description, got %+v", checks[0])
	}
	checks = EvaluateGearDetection(c, GearDetectionDefaults{}, &ai.GearDetection{Description: ""})
	if checks[0].Pass {
		t.Errorf("expected fail for empty description, got %+v", checks[0])
	}
	checks = EvaluateGearDetection(c, GearDetectionDefaults{}, &ai.GearDetection{Description: "   "})
	if checks[0].Pass {
		t.Errorf("expected fail for whitespace-only description, got %+v", checks[0])
	}
}

func TestEvaluateGearDetection_WeightAndValueWithinTolerance(t *testing.T) {
	weight := 2000.0
	value := 300.0
	c := GearDetectionCase{
		ID:        "numeric",
		ImageFile: "x.jpg",
		Expected: GearDetectionExpected{
			WeightGramsApprox: &weight,
			ValueEstimateUSD:  &value,
		},
	}
	defaults := GearDetectionDefaults{WeightTolerancePct: 30, ValueEstimateTolerancePct: 50}

	// Pass both
	det := &ai.GearDetection{
		WeightGrams:   2200,
		ValueEstimate: &ai.ValueEstimate{EstimatedValueUSD: 350},
	}
	checks := EvaluateGearDetection(c, defaults, det)
	if len(checks) != 2 || !checks[0].Pass || !checks[1].Pass {
		t.Errorf("expected both checks to pass, got %+v", checks)
	}

	// Weight out of tolerance, value in tolerance
	det = &ai.GearDetection{
		WeightGrams:   5000,
		ValueEstimate: &ai.ValueEstimate{EstimatedValueUSD: 350},
	}
	checks = EvaluateGearDetection(c, defaults, det)
	weightPass := false
	valuePass := false
	for _, chk := range checks {
		if chk.Field == "weight_grams_approx" {
			weightPass = chk.Pass
		}
		if chk.Field == "value_estimate_usd" {
			valuePass = chk.Pass
		}
	}
	if weightPass {
		t.Errorf("expected weight fail at 150%% off, got pass")
	}
	if !valuePass {
		t.Errorf("expected value pass within 50%% tolerance, got fail")
	}
}

func TestLoadGearDetectionGoldens(t *testing.T) {
	g, err := LoadGearDetectionGoldens("testdata/gear_image_goldens.json")
	if err != nil {
		t.Fatalf("LoadGearDetectionGoldens: %v", err)
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
		if c.MimeType == "" {
			t.Errorf("case %s has empty mime_type", c.ID)
		}
		if ids[c.ID] {
			t.Errorf("duplicate case ID %q", c.ID)
		}
		ids[c.ID] = true
	}
}
