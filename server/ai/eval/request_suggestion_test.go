package eval

import (
	"math"
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSeedNeedMatch(t *testing.T) {
	cases := []struct {
		emitted, expected string
		want              bool
	}{
		{"Lawn mower", "lawn mower", true},          // case-insensitive
		{"Picture book set", "picture books", true}, // substring-ish via word overlap
		{"Storage bins", "Plastic storage bins", true},
		{"Whiteboard", "whiteboard", true},
		{"Ride to the airport", "lawn mower", false},
		{"Cooler", "camp stove", false},
	}
	for _, tc := range cases {
		if got := seedNeedMatch(tc.emitted, tc.expected); got != tc.want {
			t.Errorf("seedNeedMatch(%q,%q)=%v, want %v", tc.emitted, tc.expected, got, tc.want)
		}
	}
}

func TestSeedNeedMetrics(t *testing.T) {
	cases := []struct {
		name              string
		expected, got     []string
		wantPrec, wantRec float64
	}{
		{
			name:     "perfect multi",
			expected: []string{"picture books", "whiteboard", "storage bins"},
			got:      []string{"Picture books", "Whiteboard", "Storage bins"},
			wantPrec: 1, wantRec: 1,
		},
		{
			name:     "one dropped",
			expected: []string{"tent", "stove", "cooler", "lantern", "bags"},
			got:      []string{"Tent", "Stove", "Cooler", "Lantern"},
			wantPrec: 1, wantRec: 0.8,
		},
		{
			name:     "one invented",
			expected: []string{"lawn mower"},
			got:      []string{"Lawn mower", "Gas can"},
			wantPrec: 0.5, wantRec: 1,
		},
		{
			name:     "vague: nothing expected, nothing emitted",
			expected: []string{},
			got:      []string{},
			wantPrec: 1, wantRec: 1,
		},
		{
			name:     "vague: nothing expected but model invented needs",
			expected: []string{},
			got:      []string{"Casserole", "Gift card"},
			wantPrec: 0, wantRec: 1,
		},
		{
			name:     "expected some but emitted none",
			expected: []string{"lawn mower"},
			got:      []string{},
			wantPrec: 0, wantRec: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prec, rec := seedNeedMetrics(tc.expected, tc.got)
			if !almostEqual(prec, tc.wantPrec) || !almostEqual(rec, tc.wantRec) {
				t.Errorf("seedNeedMetrics = (p=%.3f, r=%.3f), want (p=%.3f, r=%.3f)",
					prec, rec, tc.wantPrec, tc.wantRec)
			}
		})
	}
}

func TestEvaluateRequestSuggestions_GatesOnThresholds(t *testing.T) {
	defaults := RequestSuggestionDefaults{MinPrecision: 0.8, MinRecall: 0.8}

	// A clean multi-item extraction passes both bars.
	pass := EvaluateRequestSuggestions(
		RequestSuggestionCase{ID: "ok", Expected: RequestSuggestionExpected{
			SeedNeeds: []string{"picture books", "whiteboard", "storage bins", "art supplies", "construction paper"},
		}},
		defaults,
		&ai.RequestSuggestionResult{SeedNeeds: []string{"Picture books", "Whiteboard", "Storage bins", "Art supplies", "Construction paper"}},
	)
	for _, c := range pass {
		if !c.Pass {
			t.Errorf("expected pass for clean extraction, %s failed: %s", c.Field, c.Reason)
		}
	}

	// Over-extraction on a vague prompt fails precision.
	fail := EvaluateRequestSuggestions(
		RequestSuggestionCase{ID: "over", Expected: RequestSuggestionExpected{SeedNeeds: []string{}, ExpectCount: intPtr(0)}},
		defaults,
		&ai.RequestSuggestionResult{SeedNeeds: []string{"Casserole", "Gift card"}},
	)
	var precisionFailed, countFailed bool
	for _, c := range fail {
		if c.Field == "seed_needs_precision" && !c.Pass {
			precisionFailed = true
		}
		if c.Field == "seed_needs_count" && !c.Pass {
			countFailed = true
		}
	}
	if !precisionFailed {
		t.Error("expected precision check to fail on invented needs")
	}
	if !countFailed {
		t.Error("expected count check to fail (2 != 0)")
	}
}

func TestLoadRequestSuggestionGoldens(t *testing.T) {
	g, err := LoadRequestSuggestionGoldens("testdata/request_suggestion_goldens.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(g.Cases) == 0 {
		t.Fatal("expected at least one case")
	}
	if g.Defaults.MinPrecision <= 0 || g.Defaults.MinRecall <= 0 {
		t.Errorf("expected positive default thresholds, got %+v", g.Defaults)
	}
	// Every case must name a title and a non-nil expected list.
	for _, c := range g.Cases {
		if c.Title == "" {
			t.Errorf("case %q has no title", c.ID)
		}
		if c.Expected.SeedNeeds == nil {
			t.Errorf("case %q has nil seed_needs (use [] for the empty contract)", c.ID)
		}
	}
}

func intPtr(i int) *int { return &i }
