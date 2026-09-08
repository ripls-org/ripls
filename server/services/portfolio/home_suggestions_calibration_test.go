package portfolio

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/ai/embedding"
	"go.ripls.org/ripls/server/storage"
)

// calibrationKinds is the curated fixture backing the open-day-suggestion
// clustering threshold (#2674). Each key is an intended activity "kind"; its
// values are representative experience names. The calibration test embeds these
// (name-only, matching production) with the real model and asserts the
// *precision* invariant for a picky threshold: no two names of different kinds
// score at/above activityClusterMinSimilarity, so the engine never merges
// unrelated activities. Recall is checked separately via paraphrasePairs.
//
// Keep the kinds semantically distinct. A known limitation (see
// clustering_strategy_test.go): generic, venue-flavored names embed toward a
// diffuse "community event" direction and can score high against unrelated
// activities — do not add such names here.
var calibrationKinds = map[string][]string{
	"hiking":      {"Flatirons hike", "Sunday morning hike", "Hike up Green Mountain"},
	"dinner":      {"Taco Tuesday dinner", "Homemade pasta dinner", "Neighborhood potluck dinner"},
	"board games": {"Board game night", "Board games with friends", "Weekly game night"},
	"swimming":    {"Lap swim at the pool", "Morning swim", "Evening swim workout"},
}

// paraphrasePairs are name pairs that clearly describe the same activity and so
// MUST cluster — the recall floor that catches a threshold set too high.
var paraphrasePairs = [][2]string{
	{"Trail run", "Morning trail run"},
	{"Board game night", "Board game night with friends"},
	{"Flatirons hike", "Hiking the Flatirons"},
	{"Taco night", "Taco night dinner"},
}

// TestActivityClusterThresholdCalibration validates activityClusterMinSimilarity
// against real embeddings. Requires the local ONNX model (skips cleanly when
// unavailable, per SetupTestEmbedder) and should be run for real during
// development — a self-skip is not verification.
func TestActivityClusterThresholdCalibration(t *testing.T) {
	embedder := storage.SetupTestEmbedder(t)
	if embedder == nil {
		return
	}
	ctx := context.Background()

	embed := func(name string) []float32 {
		v, err := embedder.Generate(ctx, name)
		if err != nil {
			t.Fatalf("Generate(%q): %v", name, err)
		}
		return v
	}

	// --- Precision: no cross-kind pair reaches the threshold. ---
	type named struct {
		kind, name string
		vec        []float32
	}
	var all []named
	for kind, names := range calibrationKinds {
		for _, name := range names {
			all = append(all, named{kind, name, embed(name)})
		}
	}

	maxCross := float32(-1)
	var maxCrossPair [2]string
	sameAbove := 0
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			sim := embedding.CosineSimilarity(all[i].vec, all[j].vec)
			if all[i].kind == all[j].kind {
				if float64(sim) >= activityClusterMinSimilarity {
					sameAbove++
				}
				continue
			}
			if sim > maxCross {
				maxCross, maxCrossPair = sim, [2]string{all[i].name, all[j].name}
			}
			if float64(sim) >= activityClusterMinSimilarity {
				t.Errorf("cross-kind pair merges at threshold %.2f (false merge): %q ~ %q = %.3f",
					activityClusterMinSimilarity, all[i].name, all[j].name, sim)
			}
		}
	}
	t.Logf("precision: max cross-kind similarity %.3f %v (threshold %.2f); %d same-kind pairs also cluster",
		maxCross, maxCrossPair, activityClusterMinSimilarity, sameAbove)

	// --- Recall floor: obvious paraphrases must cluster. ---
	for _, p := range paraphrasePairs {
		sim := embedding.CosineSimilarity(embed(p[0]), embed(p[1]))
		if float64(sim) < activityClusterMinSimilarity {
			t.Errorf("paraphrase pair fails to merge at threshold %.2f (recall too strict): %q ~ %q = %.3f",
				activityClusterMinSimilarity, p[0], p[1], sim)
		}
	}
}
