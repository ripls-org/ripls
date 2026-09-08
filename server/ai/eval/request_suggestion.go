package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.ripls.org/ripls/server/ai"
)

// RequestSuggestionGoldens is the on-disk schema for
// testdata/request_suggestion_goldens.json. It exercises the seed_needs
// extraction on GenerateRequestSuggestions: given a request's own title +
// description, how well does the model recover the claimable things the text
// plainly names, without inventing extras (#2731)?
type RequestSuggestionGoldens struct {
	Description string                    `json:"description"`
	Version     int                       `json:"version"`
	Defaults    RequestSuggestionDefaults `json:"defaults"`
	Cases       []RequestSuggestionCase   `json:"cases"`
}

// RequestSuggestionDefaults holds the pass bars applied when a case does not
// override them. Precision guards against invented needs; recall guards against
// dropped ones. Precision is weighted at least as strictly as recall because a
// wrongly-added need makes a request read as more demanding than it is.
type RequestSuggestionDefaults struct {
	MinPrecision float64 `json:"min_precision"`
	MinRecall    float64 `json:"min_recall"`
}

type RequestSuggestionCase struct {
	ID          string                    `json:"id"`
	Tags        []string                  `json:"tags,omitempty"`
	Title       string                    `json:"title"`
	Description string                    `json:"description,omitempty"`
	Expected    RequestSuggestionExpected `json:"expected"`
}

// RequestSuggestionExpected is the asserted seed_needs contract for one case.
// SeedNeeds is the set of things the text plainly names — empty for a prompt
// that names nothing concrete, which asserts the model returns no needs.
type RequestSuggestionExpected struct {
	SeedNeeds []string `json:"seed_needs"`
	// MinPrecision / MinRecall override the file defaults for this case.
	MinPrecision *float64 `json:"min_precision,omitempty"`
	MinRecall    *float64 `json:"min_recall,omitempty"`
	// ExpectCount, when set, asserts the model returned exactly this many
	// needs (used to pin the single-item and empty cases hard).
	ExpectCount *int `json:"expect_count,omitempty"`
}

// LoadRequestSuggestionGoldens reads and parses the request-suggestion goldens.
func LoadRequestSuggestionGoldens(path string) (*RequestSuggestionGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g RequestSuggestionGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// labelFillerWords are dropped before comparing seed-need labels — articles and
// connectors that carry no identity ("a whiteboard" ≡ "whiteboard").
var labelFillerWords = map[string]bool{
	"a": true, "an": true, "the": true, "some": true, "of": true, "for": true, "to": true,
}

// normalizeLabel lowercases, strips punctuation, drops filler words, and crudely
// singularizes each remaining word (trailing-"s" strip) so "Picture books" and
// "picture book" — the kind of singular/plural drift a model produces — compare
// equal.
func normalizeLabel(s string) string {
	var out []string
	for _, w := range strings.Fields(strings.ToLower(strings.TrimSpace(s))) {
		w = strings.Trim(w, ".,;:!?\"'()")
		if w == "" || labelFillerWords[w] {
			continue
		}
		if len(w) > 3 && strings.HasSuffix(w, "s") {
			w = strings.TrimSuffix(w, "s")
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}

// seedNeedMatch reports whether an emitted label refers to the same thing as an
// expected label. Both are normalized (case, punctuation, filler words,
// singular/plural) and then matched when either is a substring of the other
// ("picture book set" vs "picture book") or their word sets overlap by at least
// half ("storage bin" vs "plastic storage bin"). The fuzziness keeps the eval
// from penalizing a model for reasonable wording it can't be expected to
// reproduce verbatim, while still requiring it to name the right thing.
func seedNeedMatch(emitted, expected string) bool {
	e := normalizeLabel(emitted)
	x := normalizeLabel(expected)
	if e == "" || x == "" {
		return false
	}
	if strings.Contains(e, x) || strings.Contains(x, e) {
		return true
	}
	return jaccardSimilarity(strings.Fields(e), strings.Fields(x)) >= 0.5
}

// seedNeedMetrics computes precision and recall of the emitted seed-need list
// against the expected list, using seedNeedMatch for fuzzy per-item comparison.
//
//   - recall = fraction of expected needs covered by at least one emitted need.
//     Empty-expected recalls trivially to 1.0.
//   - precision = fraction of emitted needs that match at least one expected
//     need. With an empty expected list, any emission is spurious, so precision
//     is 1.0 only when nothing was emitted (the "extract nothing from a vague
//     prompt" contract) and 0.0 otherwise. With a non-empty expected list, an
//     empty emission scores precision 0.0.
func seedNeedMetrics(expected, got []string) (precision, recall float64) {
	matchedExpected := 0
	for _, x := range expected {
		for _, e := range got {
			if seedNeedMatch(e, x) {
				matchedExpected++
				break
			}
		}
	}
	matchedEmitted := 0
	for _, e := range got {
		for _, x := range expected {
			if seedNeedMatch(e, x) {
				matchedEmitted++
				break
			}
		}
	}

	if len(expected) == 0 {
		recall = 1
	} else {
		recall = float64(matchedExpected) / float64(len(expected))
	}
	switch {
	case len(got) == 0 && len(expected) == 0:
		precision = 1
	case len(got) == 0:
		precision = 0
	default:
		precision = float64(matchedEmitted) / float64(len(got))
	}
	return precision, recall
}

// EvaluateRequestSuggestions runs the seed_needs precision/recall checks for one
// case. It emits a recall check and a precision check (each gated against the
// case's or file's minimum), plus an optional exact-count check.
func EvaluateRequestSuggestions(c RequestSuggestionCase, defaults RequestSuggestionDefaults, res *ai.RequestSuggestionResult) []Check {
	got := res.SeedNeeds
	precision, recall := seedNeedMetrics(c.Expected.SeedNeeds, got)

	minRecall := defaults.MinRecall
	if c.Expected.MinRecall != nil {
		minRecall = *c.Expected.MinRecall
	}
	minPrecision := defaults.MinPrecision
	if c.Expected.MinPrecision != nil {
		minPrecision = *c.Expected.MinPrecision
	}

	checks := []Check{
		{
			CaseID: c.ID,
			Field:  "seed_needs_recall",
			Pass:   recall >= minRecall,
			Reason: fmt.Sprintf("recall %.2f (want ≥%.2f) — expected %v, got %v", recall, minRecall, c.Expected.SeedNeeds, got),
		},
		{
			CaseID: c.ID,
			Field:  "seed_needs_precision",
			Pass:   precision >= minPrecision,
			Reason: fmt.Sprintf("precision %.2f (want ≥%.2f) — expected %v, got %v", precision, minPrecision, c.Expected.SeedNeeds, got),
		},
	}
	if c.Expected.ExpectCount != nil {
		checks = append(checks, Check{
			CaseID: c.ID,
			Field:  "seed_needs_count",
			Pass:   len(got) == *c.Expected.ExpectCount,
			Reason: fmt.Sprintf("count %d, want %d — got %v", len(got), *c.Expected.ExpectCount, got),
		})
	}
	return checks
}
