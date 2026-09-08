package eval

import (
	"strings"
	"testing"

	"go.ripls.org/ripls/server/ai"
)

func TestJaccardSimilarity(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want float64
	}{
		{"identical", []string{"hike", "trail"}, []string{"hike", "trail"}, 1.0},
		{"disjoint", []string{"hike"}, []string{"dinner"}, 0.0},
		{"half-overlap", []string{"hike", "trail"}, []string{"hike", "mountain"}, 1.0 / 3.0},
		{"case-insensitive", []string{"Hike"}, []string{"hike"}, 1.0},
		{"empty-both", nil, nil, 0.0},
		{"empty-one-side", []string{"hike"}, nil, 0.0},
		{"duplicates-dedupe", []string{"hike", "hike"}, []string{"hike"}, 1.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := jaccardSimilarity(c.a, c.b)
			if abs(got-c.want) > 1e-9 {
				t.Errorf("jaccardSimilarity(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestContainsCheck(t *testing.T) {
	cases := []struct {
		name     string
		haystack string
		needles  []string
		mode     string
		wantPass bool
	}{
		{"all-mode-pass", "Morning Hike at Chautauqua", []string{"hike", "chautauqua"}, "all", true},
		{"all-mode-missing-one", "Morning Hike at Chautauqua", []string{"hike", "brainard"}, "all", false},
		{"any-mode-one-present", "Morning Hike", []string{"hike", "dinner"}, "any", true},
		{"any-mode-none", "Morning Hike", []string{"dinner", "coffee"}, "any", false},
		{"empty-mode-defaults-to-all", "Hello World", []string{"hello"}, "", true},
		{"case-insensitive", "HELLO WORLD", []string{"hello"}, "all", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pass, _ := containsCheck(c.haystack, c.needles, c.mode)
			if pass != c.wantPass {
				t.Errorf("containsCheck(%q, %v, %q) = %v, want %v", c.haystack, c.needles, c.mode, pass, c.wantPass)
			}
		})
	}
}

func TestKeywordsMustNotContainCheck(t *testing.T) {
	cases := []struct {
		name     string
		got      []string
		banned   []string
		wantPass bool
	}{
		{"no-leak", []string{"pickleball", "paddle", "court"}, []string{"3rd shot", "longmont"}, true},
		{"city-leaked", []string{"pickleball", "Longmont", "sport"}, []string{"3rd shot", "longmont"}, false},
		{"venue-leaked-case-insensitive", []string{"yoga", "REC CENTER"}, []string{"rec center"}, false},
		{"substring-within-keyword", []string{"camping at Brainard Lake"}, []string{"brainard"}, false},
		{"empty-keywords-pass", nil, []string{"longmont"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pass, reason := keywordsMustNotContainCheck(c.got, c.banned)
			if pass != c.wantPass {
				t.Errorf("keywordsMustNotContainCheck(%v, %v) = %v (%s), want %v", c.got, c.banned, pass, reason, c.wantPass)
			}
		})
	}
}

func TestSetEqualCaseInsensitive(t *testing.T) {
	cases := []struct {
		name     string
		want     []string
		got      []string
		wantPass bool
	}{
		{"equal", []string{"Mike", "Bhavna"}, []string{"mike", "bhavna"}, true},
		{"reordered", []string{"Mike", "Bhavna"}, []string{"Bhavna", "Mike"}, true},
		{"missing-one", []string{"Mike", "Bhavna"}, []string{"Mike"}, false},
		{"extra-one", []string{"Mike"}, []string{"Mike", "Extra"}, false},
		{"both-empty", nil, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pass, _ := setEqualCaseInsensitive(c.want, c.got)
			if pass != c.wantPass {
				t.Errorf("setEqualCaseInsensitive(%v, %v) = %v, want %v", c.want, c.got, pass, c.wantPass)
			}
		})
	}
}

func TestValueEstimateCheck(t *testing.T) {
	cases := []struct {
		name        string
		ve          *ai.ValueEstimate
		expectedUSD float64
		tolerance   float64
		wantPass    bool
	}{
		{"nil-when-expected", nil, 25, 30, false},
		{"exact", &ai.ValueEstimate{EstimatedValueUSD: 25}, 25, 30, true},
		{"within-tolerance-low", &ai.ValueEstimate{EstimatedValueUSD: 20}, 25, 30, true},  // 20% off, tol 30%
		{"within-tolerance-high", &ai.ValueEstimate{EstimatedValueUSD: 30}, 25, 30, true}, // 20% off
		{"outside-tolerance", &ai.ValueEstimate{EstimatedValueUSD: 50}, 25, 30, false},    // 100% off
		{"expected-zero-actual-zero", &ai.ValueEstimate{EstimatedValueUSD: 0}, 0, 30, true},
		{"expected-zero-actual-nonzero", &ai.ValueEstimate{EstimatedValueUSD: 5}, 0, 30, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pass, _ := valueEstimateCheck(c.ve, c.expectedUSD, c.tolerance)
			if pass != c.wantPass {
				t.Errorf("valueEstimateCheck(%v, %v, %v) = %v, want %v", c.ve, c.expectedUSD, c.tolerance, pass, c.wantPass)
			}
		})
	}
}

func TestNumericWithinTolerance(t *testing.T) {
	cases := []struct {
		name     string
		actual   float64
		expected float64
		tol      float64
		wantPass bool
	}{
		{"exact", 100, 100, 10, true},
		{"within-tolerance-low", 90, 100, 15, true},
		{"within-tolerance-high", 110, 100, 15, true},
		{"outside-tolerance", 80, 100, 15, false},
		{"expected-zero-actual-zero", 0, 0, 10, true},
		{"expected-zero-actual-nonzero", 5, 0, 10, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pass, _ := weightWithinTolerance(c.actual, c.expected, c.tol)
			if pass != c.wantPass {
				t.Errorf("weightWithinTolerance(%v, %v, %v) = %v, want %v", c.actual, c.expected, c.tol, pass, c.wantPass)
			}
		})
	}
}

func TestTally_Report(t *testing.T) {
	tally := NewTally()
	tally.Record(Check{CaseID: "a", Field: "title_contains", Pass: true})
	tally.Record(Check{CaseID: "b", Field: "title_contains", Pass: false})
	tally.Record(Check{CaseID: "a", Field: "date", Pass: true})
	tally.RecordInvocationFailure("c")

	r := tally.Report()
	if r.TotalChecks != 3 {
		t.Errorf("TotalChecks = %d, want 3", r.TotalChecks)
	}
	if r.TotalPassed != 2 {
		t.Errorf("TotalPassed = %d, want 2", r.TotalPassed)
	}
	if abs(r.OverallRate-2.0/3.0) > 1e-9 {
		t.Errorf("OverallRate = %v, want ~0.667", r.OverallRate)
	}
	foundInvoFailLine := false
	for _, line := range r.Lines {
		if strings.Contains(line, "invocation failures") && strings.Contains(line, "c") {
			foundInvoFailLine = true
		}
	}
	if !foundInvoFailLine {
		t.Errorf("expected invocation-failure line in report, got lines: %v", r.Lines)
	}
}
