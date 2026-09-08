//go:build benchmark

// Unified-create classifier live-API eval. Runs the goldens against
// every credentialed (provider, model) pair returned by providersForEval
// and gates each pair via the shared reportAndGate threshold flow.
// Gated behind the `benchmark` build tag — same as the other Test*Prompt
// suites — so CI and default `go test` runs never make real API calls.
//
// Goldens cover all three input modes (text / image / url) — see
// unified_classifier.go for the per-mode field meaning. Each case is
// dispatched to the matching classifier prompt builder by populating
// the correct fields on UnifiedCreateClassifierInput.
//
// Per-call latency is captured alongside accuracy and reported on the
// per-(provider, model) EVAL_SUMMARY line as p50_ms / p95_ms / mean_ms.
// Used by #1932 to measure the before/after of the slim-response +
// enum-constrained-output change without merging on faith.
//
// Threshold tuning (per-pair overrides in modelThresholds) is #1880's
// scope; this test only reports + applies the default gate.

package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"go.ripls.org/ripls/server/ai"
)

// TestUnifiedClassifierPrompt runs the unified-create classifier
// goldens against every credentialed (provider, model) pair.
func TestUnifiedClassifierPrompt(t *testing.T) {
	requireMode(t, "text")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadUnifiedClassifierGoldens("testdata/unified_create_classifier_goldens.json")
	if err != nil {
		t.Fatalf("LoadUnifiedClassifierGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestUnifiedClassifierPrompt", "unified_classifier", "unified_create_classifier_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			durations := make([]time.Duration, 0, len(goldens.Cases))
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					input, err := classifierInputForCase(c)
					if err != nil {
						t.Errorf("build classifier input for %q: %v", c.ID, err)
						tally.RecordInvocationFailure(c.ID)
						return
					}

					start := time.Now()
					result, err := retryOnRateLimit(t, c.ID, func() (*ai.UnifiedCreateClassification, error) {
						return pm.Provider.ClassifyUnifiedCreate(ctx, input)
					})
					dur := time.Since(start)
					if err != nil || result == nil {
						t.Errorf("ClassifyUnifiedCreate failed: %v", err)
						tally.RecordInvocationFailure(c.ID)
						return
					}
					durations = append(durations, dur)
					// Also push into the tally so the baseline JSON
					// gets median/p95 latency for this suite via the
					// same path as the other Test*Prompt suites.
					tally.RecordLatency(dur.Milliseconds())

					t.Logf("[%s] mode=%s → type=%s duration_ms=%d",
						c.ID, c.Mode(), result.Type, dur.Milliseconds())

					for _, chk := range EvaluateUnifiedClassifier(c, result) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportLatency(t, pm.Key(), durations)
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// classifierInputForCase builds the provider input for a single case
// based on its declared input mode. Image cases read the fixture bytes
// from server/test_data/ and populate Input.Image so the classifier
// actually sees the photo. Eval bypasses bucket / signed-URL resolution
// since fixtures are local files — production goes through the bucket
// instead (see server/services/unified_create/stream_gen.go).
func classifierInputForCase(c UnifiedClassifierCase) (ai.UnifiedCreateClassifierInput, error) {
	switch c.Mode() {
	case "text":
		if c.Prompt == "" {
			return ai.UnifiedCreateClassifierInput{}, fmt.Errorf("text-mode case %q missing prompt", c.ID)
		}
		return ai.UnifiedCreateClassifierInput{Text: c.Prompt}, nil
	case "image":
		if c.ImageFile == "" {
			return ai.UnifiedCreateClassifierInput{}, fmt.Errorf("image-mode case %q missing image_file", c.ID)
		}
		data, err := os.ReadFile(filepath.Join(testDataDir, c.ImageFile))
		if err != nil {
			return ai.UnifiedCreateClassifierInput{}, fmt.Errorf("image-mode case %q: read fixture: %w", c.ID, err)
		}
		return ai.UnifiedCreateClassifierInput{
			Image: &ai.DetectionImage{
				ImageData: data,
				MimeType:  c.MimeType,
			},
		}, nil
	case "url":
		if c.PageTitle == "" && c.PageDescription == "" {
			return ai.UnifiedCreateClassifierInput{}, fmt.Errorf("url-mode case %q missing page_title and page_description", c.ID)
		}
		return ai.UnifiedCreateClassifierInput{
			WebsiteURL:         "https://example.invalid/" + c.ID,
			WebsiteTitle:       c.PageTitle,
			WebsiteDescription: c.PageDescription,
		}, nil
	default:
		return ai.UnifiedCreateClassifierInput{}, fmt.Errorf("case %q has unknown input_mode %q", c.ID, c.InputMode)
	}
}

// reportLatency emits a stable single-line latency summary alongside
// the accuracy summary produced by reportAndGate. Format is grep-able
// so #1932 before/after runs can be diffed straight from CI logs:
//
//	EVAL_LATENCY provider=<provider>/<model> n=<n> p50_ms=<p50> p95_ms=<p95> mean_ms=<mean>
//
// Skips cleanly when no successful calls were made (durations empty),
// avoiding a divide-by-zero in the percentile math.
func reportLatency(t *testing.T, key string, durations []time.Duration) {
	t.Helper()
	if len(durations) == 0 {
		t.Logf("EVAL_LATENCY provider=%s n=0 (no successful calls)", key)
		return
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	p50 := sorted[len(sorted)*50/100].Milliseconds()
	p95Idx := len(sorted) * 95 / 100
	if p95Idx >= len(sorted) {
		p95Idx = len(sorted) - 1
	}
	p95 := sorted[p95Idx].Milliseconds()

	var total time.Duration
	for _, d := range sorted {
		total += d
	}
	mean := (total / time.Duration(len(sorted))).Milliseconds()

	t.Logf("EVAL_LATENCY provider=%s n=%d p50_ms=%d p95_ms=%d mean_ms=%d",
		key, len(durations), p50, p95, mean)
}
