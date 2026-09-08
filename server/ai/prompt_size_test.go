package ai

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// promptSizeFixtures renders every Gen* prompt builder with short fixed
// inputs so the measured size reflects the static template, not
// caller-supplied content. Bytes ≈ characters for these ASCII templates;
// a rough token estimate is bytes/4 for English prose. The fixed inputs
// must stay stable so sizes are comparable across commits (#1265).
func promptSizeFixtures() map[string]string {
	const (
		userPrompt  = "X"
		region      = "Boulder, CO"
		currentTime = "2026-03-05T15:00:00-07:00"
	)
	return map[string]string{
		"community_from_text":     buildCommunityGenerationPrompt(userPrompt, region),
		"experience_from_image":   buildExperienceFromImagePrompt(region, "", currentTime),
		"experience_from_text":    buildExperienceFromTextPrompt(userPrompt, region, currentTime),
		"experience_from_webpage": buildExperienceFromWebpagePrompt("X", "X", "X", region, currentTime),
		"gear_detection_image":    buildGearDetectionPrompt(),
		"gear_from_text":          buildGearGenerationPrompt(userPrompt, region),
		"gear_from_webpage":       buildGearFromWebpagePrompt("X", "X", "X", region),
		"request_from_image":      buildRequestImageAnalysisPrompt(region),
		"request_from_text":       buildRequestGenerationPrompt(userPrompt, region),
	}
}

// promptSizeTable returns a stable, human-readable table of rendered
// template sizes, one "name bytes ~tokens" line per builder, sorted by
// name. The -v output of TestPromptSizes is the recording surface —
// paste it into the eval baseline directory when capturing a run.
func promptSizeTable() string {
	sizes := promptSizeFixtures()
	names := make([]string, 0, len(sizes))
	for name := range sizes {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		n := len(sizes[name])
		fmt.Fprintf(&b, "%-24s %6d bytes  ~%4d tokens\n", name, n, n/4)
	}
	return b.String()
}

// promptSizeBudgets caps each rendered template at ~10% above its size
// after the #1265 rewrites. The point is a cheap, no-API-cost ratchet:
// prompt growth costs TTFT and money on every Gen* call, so exceeding a
// budget should be a deliberate act. To raise a budget legitimately:
// run the corresponding benchmark-tagged eval suite (see
// server/ai/README.md → "Prompt Eval Harness") against the production
// provider trio, confirm no regression, and bump the budget in the
// same commit as the prompt change.
var promptSizeBudgets = map[string]int{
	"community_from_text":     800,
	"experience_from_image":   4400,
	"experience_from_text":    5000,
	"experience_from_webpage": 5100,
	"gear_detection_image":    4850,
	"gear_from_text":          3600,
	"gear_from_webpage":       4400,
	"request_from_image":      3400,
	"request_from_text":       3250,
}

// TestPromptSizes fails when a rendered Gen* template exceeds its size
// budget, and logs the full size table for visibility. Run with
// `go test ./server/ai/ -run TestPromptSizes -v` to print the table.
func TestPromptSizes(t *testing.T) {
	sizes := promptSizeFixtures()
	for name, rendered := range sizes {
		budget, ok := promptSizeBudgets[name]
		if !ok {
			t.Errorf("%s has no entry in promptSizeBudgets — add one so growth stays visible", name)
			continue
		}
		if len(rendered) > budget {
			t.Errorf("%s is %d bytes, over its %d-byte budget — trim the prompt, or re-run its eval suite and raise the budget in the same commit (see promptSizeBudgets doc)", name, len(rendered), budget)
		}
	}
	t.Logf("rendered Gen* prompt template sizes:\n%s", promptSizeTable())
}
