// Package eval is a golden-driven prompt-eval harness for the ai package.
// It runs hand-crafted cases against a live AI provider and reports
// per-field pass rates, so prompt edits, schema trims, and model swaps
// can be validated with a quantitative gate rather than anecdotal
// spot-checks.
//
// Per-type evaluators live in experience.go, gear.go, and request.go;
// this file holds the type-agnostic pieces (Check, Tally, Report) shared
// across them. Each evaluator consumes a golden case plus the AI output
// and emits a slice of field-level Checks; Tally aggregates Checks into
// a human-readable Report with an overall pass rate.
//
// The harness runs as a regular Go test gated behind the `benchmark`
// build tag (see prompt_test.go). It is not invoked by CI or by default
// `go test`; see server/ai/README.md for the invocation recipes.
package eval

import (
	"fmt"
	"sort"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// Check is a single field-level assertion result. One golden case produces
// 0..N Checks depending on how many fields its expected block asserts.
type Check struct {
	CaseID string
	Field  string
	Pass   bool
	Reason string
}

// Tally aggregates Check results across cases for a summary report.
type Tally struct {
	perField    map[string]*fieldCounter
	invoFails   []string
	latenciesMs []int64
}

type fieldCounter struct {
	passed int
	total  int
}

func NewTally() *Tally {
	return &Tally{perField: make(map[string]*fieldCounter)}
}

func (t *Tally) Record(chk Check) {
	c, ok := t.perField[chk.Field]
	if !ok {
		c = &fieldCounter{}
		t.perField[chk.Field] = c
	}
	c.total++
	if chk.Pass {
		c.passed++
	}
}

func (t *Tally) RecordInvocationFailure(caseID string) {
	t.invoFails = append(t.invoFails, caseID)
}

// RecordLatency notes one provider-call duration in milliseconds. Called
// once per case from inside each Test*Prompt function. Surfaces in the
// baseline JSON's median_latency_ms / p95_latency_ms fields and is the
// dimension on which we judge "private vs cloud" performance in #2057.
func (t *Tally) RecordLatency(ms int64) {
	t.latenciesMs = append(t.latenciesMs, ms)
}

// Report is the human-readable output of an eval run.
type Report struct {
	Lines       []string
	TotalPassed int
	TotalChecks int
	OverallRate float64
}

func (t *Tally) Report() Report {
	fields := make([]string, 0, len(t.perField))
	for f := range t.perField {
		fields = append(fields, f)
	}
	sort.Strings(fields)

	r := Report{}
	r.Lines = append(r.Lines, "--- eval report ---")
	for _, f := range fields {
		c := t.perField[f]
		rate := 0.0
		if c.total > 0 {
			rate = float64(c.passed) / float64(c.total)
		}
		r.Lines = append(r.Lines, fmt.Sprintf("  %-26s %3d/%3d  %5.1f%%", f, c.passed, c.total, 100*rate))
		r.TotalPassed += c.passed
		r.TotalChecks += c.total
	}
	if r.TotalChecks > 0 {
		r.OverallRate = float64(r.TotalPassed) / float64(r.TotalChecks)
	}
	if len(t.invoFails) > 0 {
		r.Lines = append(r.Lines, fmt.Sprintf("  invocation failures (%d): %v", len(t.invoFails), t.invoFails))
	}
	return r
}

// Containment modes for the *_contains_mode suite fields.
const (
	containsModeAll = "all"
	containsModeAny = "any"
)

// containsCheck verifies `haystack` contains the required substrings per
// `mode`. "all" requires every needle; "any" requires at least one. Case-insensitive.
func containsCheck(haystack string, needles []string, mode string) (bool, string) {
	lc := strings.ToLower(haystack)
	var missing, present []string
	for _, n := range needles {
		if strings.Contains(lc, strings.ToLower(n)) {
			present = append(present, n)
		} else {
			missing = append(missing, n)
		}
	}
	switch mode {
	case "any":
		if len(present) > 0 {
			return true, ""
		}
		return false, fmt.Sprintf("none of %v in %q", needles, haystack)
	default:
		if len(missing) == 0 {
			return true, ""
		}
		return false, fmt.Sprintf("missing %v in %q", missing, haystack)
	}
}

// keywordsMustNotContainCheck asserts that none of the banned terms appear
// as a case-insensitive substring of any emitted keyword. It is the negative
// counterpart of the must-contain grounding check: goldens whose prompts
// mention venues, cities, businesses, or people use it to catch proper-noun
// leakage into search_keywords, which pollutes stock-media queries (#1209).
// An empty keyword list trivially passes — the positive assertions are what
// guard against missing keywords.
func keywordsMustNotContainCheck(got, banned []string) (bool, string) {
	joined := strings.ToLower(strings.Join(got, " | "))
	var leaked []string
	for _, term := range banned {
		if strings.Contains(joined, strings.ToLower(term)) {
			leaked = append(leaked, term)
		}
	}
	if len(leaked) == 0 {
		return true, ""
	}
	return false, fmt.Sprintf("banned proper-noun term(s) leaked into %v: %v", got, leaked)
}

// jaccardSimilarity computes |A∩B| / |A∪B| with case-insensitive element
// comparison. Empty-and-empty returns 0 (not 1) so "expected keywords but got
// none" fails rather than silently passing.
func jaccardSimilarity(a, b []string) float64 {
	setA := lowerSet(a)
	setB := lowerSet(b)
	if len(setA) == 0 && len(setB) == 0 {
		return 0
	}
	intersect := 0
	for k := range setA {
		if _, ok := setB[k]; ok {
			intersect++
		}
	}
	union := len(setA) + len(setB) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

func lowerSet(xs []string) map[string]struct{} {
	s := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		t := strings.TrimSpace(strings.ToLower(x))
		if t != "" {
			s[t] = struct{}{}
		}
	}
	return s
}

func setEqualCaseInsensitive(want, got []string) (bool, string) {
	w := lowerSet(want)
	g := lowerSet(got)
	if len(w) != len(g) {
		return false, fmt.Sprintf("expected set %v, got %v", sortedKeys(w), sortedKeys(g))
	}
	for k := range w {
		if _, ok := g[k]; !ok {
			return false, fmt.Sprintf("expected set %v, got %v (missing %q)", sortedKeys(w), sortedKeys(g), k)
		}
	}
	return true, ""
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// valueEstimateCheck validates a ValueEstimate's USD amount within a percent
// tolerance. A nil ValueEstimate when one was expected is a failure; expected
// = 0 requires actual = 0 (no tolerance on zero).
func valueEstimateCheck(ve *ai.ValueEstimate, expectedUSD, tolerancePct float64) (bool, string) {
	if ve == nil {
		return false, fmt.Sprintf("expected value_estimate $%.2f ±%.0f%%, got nil", expectedUSD, tolerancePct)
	}
	actual := float64(ve.EstimatedValueUSD)
	if expectedUSD == 0 {
		if actual == 0 {
			return true, ""
		}
		return false, fmt.Sprintf("expected 0, got %.2f", actual)
	}
	diff := actual - expectedUSD
	if diff < 0 {
		diff = -diff
	}
	pctDiff := 100 * diff / expectedUSD
	if pctDiff <= tolerancePct {
		return true, ""
	}
	return false, fmt.Sprintf("expected $%.2f ±%.0f%%, got $%.2f (%.1f%% off)", expectedUSD, tolerancePct, actual, pctDiff)
}

// weightWithinTolerance is the numeric analogue for non-value fields.
// Expected = 0 requires actual = 0. Re-generalize by threading a label back
// through if a second numeric field ever needs it.
func weightWithinTolerance(actual, expected, tolerancePct float64) (bool, string) {
	const label = "weight_grams"
	if expected == 0 {
		if actual == 0 {
			return true, ""
		}
		return false, fmt.Sprintf("expected %s=0, got %.2f", label, actual)
	}
	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	pctDiff := 100 * diff / expected
	if pctDiff <= tolerancePct {
		return true, ""
	}
	return false, fmt.Sprintf("expected %s=%.2f ±%.0f%%, got %.2f (%.1f%% off)", label, expected, tolerancePct, actual, pctDiff)
}

// materialCategoryAnyOf reports whether got matches any of the allowed
// material-category strings, case-insensitive. It also validates that every
// entry in allowed parses to a known enum value; an unknown golden token causes
// a panic so the test fails with a clear message rather than a silent miss.
func materialCategoryAnyOf(got string, allowed []string) bool {
	for _, a := range allowed {
		if ai.MaterialCategoryFromJSON(a) == 0 {
			panic(fmt.Sprintf("eval golden contains unknown material_category token %q — fix the golden file", a))
		}
		if strings.EqualFold(got, a) {
			return true
		}
	}
	return false
}

// hasDescriptionCheck asserts that the Description field is non-empty (when
// expected=true) or empty (when expected=false). Whitespace-only is treated
// as empty.
func hasDescriptionCheck(description string, expected bool) (bool, string) {
	got := strings.TrimSpace(description) != ""
	if got == expected {
		return true, ""
	}
	return false, fmt.Sprintf("expected has_description=%v, got description=%q", expected, description)
}

// minConfidenceCheck asserts that actual confidence meets a floor.
func minConfidenceCheck(actual, minRequired float64) (bool, string) {
	if actual >= minRequired {
		return true, ""
	}
	return false, fmt.Sprintf("confidence %.2f < min %.2f", actual, minRequired)
}
