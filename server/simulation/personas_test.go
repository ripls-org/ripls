package simulation

import (
	"math/rand"
	"testing"
)

func TestPersonaConfigCoversAllPersonas(t *testing.T) {
	config := PersonaConfig()
	personas := []PersonaType{
		PersonaAlfred, PersonaDerek, PersonaGary,
		PersonaBetty, PersonaEmma, PersonaHenry,
	}
	for _, p := range personas {
		pw, ok := config[p]
		if !ok {
			t.Errorf("PersonaConfig missing persona %v", p)
			continue
		}
		if pw.BaseActivitiesPerWeek <= 0 {
			t.Errorf("Persona %v has non-positive base activities: %f", p, pw.BaseActivitiesPerWeek)
		}
		if len(pw.Weights) == 0 {
			t.Errorf("Persona %v has no activity weights", p)
		}
		if len(pw.GearCategories) == 0 && pw.GearCountMax > 0 {
			t.Errorf("Persona %v has gear count max %d but no gear categories", p, pw.GearCountMax)
		}
	}
}

func TestPersonaWeightsAreValid(t *testing.T) {
	for persona, pw := range PersonaConfig() {
		total := 0.0
		for _, w := range pw.Weights {
			if w < 0 || w > 1 {
				t.Errorf("Persona %v has out-of-range weight: %f", persona, w)
			}
			total += w
		}
		// Total should be close to 1.0 (within rounding).
		if total < 0.5 || total > 1.5 {
			t.Errorf("Persona %v weights sum to %f (expected ~1.0)", persona, total)
		}
	}
}

func TestSelectActivityDistribution(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	weights := PersonaConfig()[PersonaAlfred].Weights

	counts := make(map[ActivityType]int)
	n := 10000
	for range n {
		a := SelectActivity(rng, weights)
		counts[a]++
	}

	// Alfred's weights sum to 0.80, so ShareGear at 0.30 → 0.30/0.80 ≈ 0.375.
	// With 10k samples, we should see ~3750 ± 500.
	shareCount := counts[ActivityShareGear]
	if shareCount < 3000 || shareCount > 4500 {
		t.Errorf("ShareGear count = %d, expected ~3750", shareCount)
	}

	// Borrow at 0.05/0.80 ≈ 0.0625 → ~625 hits.
	borrowCount := counts[ActivityBorrow]
	if borrowCount > 1200 {
		t.Errorf("Borrow count = %d, expected ~625", borrowCount)
	}
}

func TestSelectActivityDeterministic(t *testing.T) {
	weights := PersonaConfig()[PersonaDerek].Weights

	a1 := SelectActivity(rand.New(rand.NewSource(99)), weights)
	a2 := SelectActivity(rand.New(rand.NewSource(99)), weights)

	if a1 != a2 {
		t.Errorf("Same seed produced different activities: %v vs %v", a1, a2)
	}
}

func TestGearForPersona(t *testing.T) {
	tests := []struct {
		persona  PersonaType
		minCount int
		maxCount int
	}{
		{PersonaAlfred, 12, 20},
		{PersonaDerek, 5, 10},
		{PersonaGary, 1, 3},
		{PersonaBetty, 2, 5},
		{PersonaEmma, 3, 6},
		{PersonaHenry, 0, 2},
	}

	for _, tt := range tests {
		rng := rand.New(rand.NewSource(42))
		gear := GearForPersona(rng, tt.persona)
		if len(gear) < tt.minCount || len(gear) > tt.maxCount {
			t.Errorf("Persona %v: got %d gear items, want [%d, %d]",
				tt.persona, len(gear), tt.minCount, tt.maxCount)
		}
	}
}

func TestGearForPersonaCategories(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	gear := GearForPersona(rng, PersonaAlfred)

	allowedCategories := map[string]bool{"Tools": true, "Outdoor": true, "Garden": true}
	for _, g := range gear {
		if !allowedCategories[g.Category] {
			t.Errorf("Alfred got gear from unexpected category %q: %q", g.Category, g.Name)
		}
	}
}

func TestGearForPersonaDeterministic(t *testing.T) {
	gear1 := GearForPersona(rand.New(rand.NewSource(42)), PersonaDerek)
	gear2 := GearForPersona(rand.New(rand.NewSource(42)), PersonaDerek)

	if len(gear1) != len(gear2) {
		t.Fatalf("Same seed produced different counts: %d vs %d", len(gear1), len(gear2))
	}
	for i := range gear1 {
		if gear1[i].Name != gear2[i].Name {
			t.Errorf("Same seed produced different gear at [%d]: %q vs %q", i, gear1[i].Name, gear2[i].Name)
		}
	}
}
