package eval

import "testing"

// defaultPromptEvalPassRate is the per-(provider, model) pass-rate gate
// applied when no entry exists in modelThresholds. 0.85 leaves headroom for
// one or two cases in a small suite to drift without failing the run, but
// catches a broad regression. Pairs that diverge meaningfully from this
// floor should add an explicit modelThresholds entry rather than lowering
// the default.
const defaultPromptEvalPassRate = 0.85

// modelThresholds overrides the default pass-rate gate for specific
// (provider, model) pairs. Keyed by "<provider>/<model>" so swapping the
// model via the corresponding -<provider>-models flag does not silently
// inherit a stale override.
//
// Initial values from the 9-pair gear-image baseline run on 2026-05-09
// (see commit history for #1777). Each threshold sits ~3 percentage
// points below the observed pass rate — enough buffer to absorb
// run-to-run variance without masking real regressions. Re-tune
// whenever a model is upgraded or the goldens are expanded.
var modelThresholds = map[string]float64{
	"anthropic/claude-haiku-4-5":             0.78,
	"anthropic/claude-sonnet-4-6":            0.90,
	"anthropic/claude-opus-4-7":              0.93,
	"openai/gpt-5.4-nano":                    0.65,
	"openai/gpt-5.4-mini":                    0.78,
	"openai/gpt-5.5":                         0.85,
	"gemini/vertexai/gemini-3.1-flash-lite":  0.85,
	"gemini/vertexai/gemini-3-flash-preview": 0.89,
	// "gemini/vertexai/gemini-3.1-pro-preview": pending — the 9-pair
	// baseline run hit the 60-min wall-clock budget at case 32/50 of the
	// Pro variant. Re-run that pair alone with a longer timeout to
	// observe a full-suite pass rate, then add the threshold.
}

// thresholdFor returns the configured pass-rate gate for the given
// "<provider>/<model>" key, or defaultPromptEvalPassRate if none is set.
func thresholdFor(key string) float64 {
	if t, ok := modelThresholds[key]; ok {
		return t
	}
	return defaultPromptEvalPassRate
}

func TestThresholdFor_DefaultWhenUnset(t *testing.T) {
	if got := thresholdFor("nonexistent/model"); got != defaultPromptEvalPassRate {
		t.Errorf("thresholdFor unknown key = %v, want default %v", got, defaultPromptEvalPassRate)
	}
}

func TestThresholdFor_OverrideWins(t *testing.T) {
	key := "test-provider/test-model"
	modelThresholds[key] = 0.42
	defer delete(modelThresholds, key)
	if got := thresholdFor(key); got != 0.42 {
		t.Errorf("thresholdFor(%q) = %v, want 0.42", key, got)
	}
}

func TestThresholdFor_DefaultIs85(t *testing.T) {
	// Guard against accidentally lowering the default. If you intend to
	// raise or lower the floor, update both this assertion and the docs in
	// server/ai/README.md.
	if defaultPromptEvalPassRate != 0.85 {
		t.Errorf("defaultPromptEvalPassRate = %v, want 0.85", defaultPromptEvalPassRate)
	}
}
