package impact_metrics

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/ai"
)

// updateGolden enables `go test -run TestImpactDraftPrompt -update-golden` to
// rewrite the checked-in golden file. Default is read-only, which is what CI runs.
var updateGolden = flag.Bool("update-golden", false, "rewrite golden files in place")

// TestImpactDraftPrompt_GoldenRoundTrip is the canonical guard against PII
// leakage in the prompt that drives DraftImpactEstimate's LLM call. We
// assemble a fixture (description + 5 messages with sender names + emails),
// run the same path the production code takes (buildTranscriptContext),
// and diff the output against a golden file.
//
// The prompt-context invariant is: only message *text* is included; sender
// names, emails, and any other identifying metadata never appear in the
// context string passed to the AI. If a future refactor accidentally
// concatenates names or emails, the golden diff will fail.
func TestImpactDraftPrompt_GoldenRoundTrip(t *testing.T) {
	const description = "Camping trip to Lost Maples — bringing tents, stove, and headlamps."

	fixture := []ai.ConversationMessage{
		{SenderName: "Alice Smith", Text: "I have a 4-person tent we can use", SentAtUnixSec: 1700000000},
		{SenderName: "Bob Lee", Text: "I'll bring my camp stove", SentAtUnixSec: 1700000060},
		{SenderName: "Carol Lee", Text: "Two headlamps in my pack", SentAtUnixSec: 1700000120},
		{SenderName: "Dave Romero", Text: "Meeting at the trailhead 6am", SentAtUnixSec: 1700000180},
		{SenderName: "Eve Tran", Text: "Bringing extra water filters", SentAtUnixSec: 1700000240},
	}

	got := buildTranscriptContext(description, fixture)

	goldenPath := filepath.Join("testdata", "impact_draft_prompt.golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update-golden to create)", goldenPath, err)
	}
	want := string(wantBytes)

	if got != want {
		t.Errorf("prompt context drift — re-run with -update-golden after human review.\nwant:\n%q\n\ngot:\n%q", want, got)
	}

	// Independent invariant check: sender names and any email strings must
	// NOT appear in the prompt context, regardless of golden contents.
	leakProbes := []string{
		"Alice", "Smith", "Bob Lee", "Carol", "Dave", "Eve", "Tran",
		"@", "alice@", "bob@",
	}
	for _, probe := range leakProbes {
		if strings.Contains(got, probe) {
			t.Errorf("PII leak: prompt context contains %q (expected only message text)", probe)
		}
	}
}
