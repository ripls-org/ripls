//go:build benchmark

// Paired-call image benchmark. Measures the latency of the
// classifier + per-type generator pair that runs on every image-mode
// unified-create request. Designed for #1939 phase 5c / #1952: does
// the same-URL + cache_control wiring actually reduce per-type-call
// TTFT on Anthropic when the image is reused?
//
// Each case runs ClassifyUnifiedCreate followed by the matching
// Detect*Streaming / Generate*FromImageStreaming, using the SAME
// *DetectionImage on both calls (mimicking what
// server/services/unified_create/stream_gen.go does in production).
//
// EVAL_PAIRED_LATENCY summary lines per (provider, model) carry
// classifier_p50_ms, per_type_p50_ms, total_p50_ms so before/after
// runs can be diffed straight from CI logs.

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

// TestUnifiedCreatePairedImageLatency runs each image-mode case in
// the goldens corpus through the classifier + per-type sequence and
// reports paired latency per provider.
func TestUnifiedCreatePairedImageLatency(t *testing.T) {
	requireMode(t, "image")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadUnifiedClassifierGoldens("testdata/unified_create_classifier_goldens.json")
	if err != nil {
		t.Fatalf("LoadUnifiedClassifierGoldens: %v", err)
	}

	// Filter to image-mode cases only.
	var imageCases []UnifiedClassifierCase
	for _, c := range goldens.Cases {
		if c.Mode() == "image" {
			imageCases = append(imageCases, c)
		}
	}
	if len(imageCases) == 0 {
		t.Skip("no image-mode goldens to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			var (
				classifierDurs []time.Duration
				perTypeDurs    []time.Duration
				totalDurs      []time.Duration
			)
			for _, c := range imageCases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					// Read fixture bytes once; both calls share the same
					// *DetectionImage so the second call (Anthropic) can hit
					// the prompt cache primed by the first.
					data, err := os.ReadFile(filepath.Join(testDataDir, c.ImageFile))
					if err != nil {
						t.Fatalf("read fixture: %v", err)
					}
					img := &ai.DetectionImage{
						ImageData: data,
						MimeType:  c.MimeType,
					}

					// Classifier call.
					classifierStart := time.Now()
					classification, err := retryOnRateLimit(t, c.ID+":classify", func() (*ai.UnifiedCreateClassification, error) {
						return pm.Provider.ClassifyUnifiedCreate(ctx, ai.UnifiedCreateClassifierInput{Image: img})
					})
					classifierDur := time.Since(classifierStart)
					if err != nil || classification == nil {
						t.Errorf("classifier: %v", err)
						return
					}

					// Per-type call. Dispatch on the classification result so
					// the routing matches production. Each per-type method has
					// its own signature — keep this switch close to what
					// streamPerTypeImage does.
					perTypeStart := time.Now()
					var perTypeErr error
					switch classification.Type {
					case ai.UnifiedCreateContentTypeGear:
						_, perTypeErr = retryOnRateLimit(t, c.ID+":per-type", func() (*ai.GearDetection, error) {
							return pm.Provider.DetectGearInImage(ctx, img)
						})
					case ai.UnifiedCreateContentTypeEvent:
						_, perTypeErr = retryOnRateLimit(t, c.ID+":per-type", func() (*ai.ExperienceGeneration, error) {
							return pm.Provider.GenerateExperienceFromImage(ctx, img, "Boulder, CO", "", "2026-05-17T10:00:00-06:00")
						})
					case ai.UnifiedCreateContentTypeRequest:
						_, perTypeErr = retryOnRateLimit(t, c.ID+":per-type", func() (*ai.RequestGeneration, error) {
							return pm.Provider.GenerateRequestFromImage(ctx, img, "Boulder, CO")
						})
					default:
						t.Errorf("unexpected classification %q", classification.Type)
						return
					}
					perTypeDur := time.Since(perTypeStart)
					if perTypeErr != nil {
						t.Errorf("per-type: %v", perTypeErr)
						return
					}

					total := classifierDur + perTypeDur
					classifierDurs = append(classifierDurs, classifierDur)
					perTypeDurs = append(perTypeDurs, perTypeDur)
					totalDurs = append(totalDurs, total)
					t.Logf("[%s] type=%s classifier_ms=%d per_type_ms=%d total_ms=%d",
						c.ID, classification.Type,
						classifierDur.Milliseconds(), perTypeDur.Milliseconds(), total.Milliseconds())
				})
			}
			reportPairedLatency(t, pm.Key(), classifierDurs, perTypeDurs, totalDurs)
		})
	}
}

// reportPairedLatency emits a stable grep-able EVAL_PAIRED_LATENCY
// summary so before/after runs can be diffed. Skips cleanly when no
// successful calls were made.
func reportPairedLatency(t *testing.T, key string, classifier, perType, total []time.Duration) {
	t.Helper()
	if len(total) == 0 {
		t.Logf("EVAL_PAIRED_LATENCY provider=%s n=0 (no successful calls)", key)
		return
	}
	cl50 := percentile(classifier, 50)
	pt50 := percentile(perType, 50)
	tot50 := percentile(total, 50)
	cl95 := percentile(classifier, 95)
	pt95 := percentile(perType, 95)
	tot95 := percentile(total, 95)
	t.Logf("EVAL_PAIRED_LATENCY provider=%s n=%d %s",
		key, len(total),
		fmt.Sprintf("classifier_p50_ms=%d per_type_p50_ms=%d total_p50_ms=%d classifier_p95_ms=%d per_type_p95_ms=%d total_p95_ms=%d",
			cl50.Milliseconds(), pt50.Milliseconds(), tot50.Milliseconds(),
			cl95.Milliseconds(), pt95.Milliseconds(), tot95.Milliseconds()))
}

func percentile(durs []time.Duration, p int) time.Duration {
	if len(durs) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(durs))
	copy(sorted, durs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := len(sorted) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
