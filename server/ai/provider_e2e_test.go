package ai

import (
	"strings"
	"testing"
	"time"
)

func TestE2EExtractDate(t *testing.T) {
	now := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
	cases := []struct{ prompt, want string }{
		{"Mother's Day Brunch. At our place Sunday May 10 at 10:30am!", "2026-05-10"},
		{"picnic on September 3", "2026-09-03"},
		{"no date here", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := e2eExtractDate(c.prompt, now); got != c.want {
			t.Errorf("e2eExtractDate(%q) = %q, want %q", c.prompt, got, c.want)
		}
	}
}

func TestE2EExtractClock(t *testing.T) {
	cases := []struct{ prompt, want string }{
		{"Sunday May 10 at 10:30am", "10:30"},
		{"kickoff at 7pm", "19:00"},
		{"midnight snack at 12am", "00:00"},
		{"lunch at 12pm", "12:00"},
		{"no time here", ""},
	}
	for _, c := range cases {
		if got := e2eExtractClock(c.prompt); got != c.want {
			t.Errorf("e2eExtractClock(%q) = %q, want %q", c.prompt, got, c.want)
		}
	}
}

func TestE2EDeterministicProviderExperienceGeneration(t *testing.T) {
	p := NewE2EDeterministicProvider()
	gen, err := p.GenerateExperienceFromTextFunc(t.Context(),
		"Mother's Day Brunch. At our place Sunday May 10 at 10:30am — bring the whole crew!", "", "")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if gen.Title != "Mother's Day Brunch" {
		t.Errorf("title = %q, want Mother's Day Brunch", gen.Title)
	}
	if gen.Time != "10:30" || gen.TimeConfidence != "EXPLICIT" {
		t.Errorf("time = %q confidence %q, want 10:30 EXPLICIT", gen.Time, gen.TimeConfidence)
	}
	if gen.LocationQuery != "" {
		t.Errorf("location query = %q, want empty (primary-residence fallback)", gen.LocationQuery)
	}
}

func TestE2EDeterministicProviderRequestGeneration(t *testing.T) {
	p := NewE2EDeterministicProvider()
	prompt := "Looking for a lawn mower this weekend. Ours gave up mid-mow and the grass won."
	gen, err := p.GenerateRequestFunc(t.Context(), prompt, "")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if gen.Title != "Looking for a lawn mower this weekend" {
		t.Errorf("title = %q, want the prompt's first sentence", gen.Title)
	}
	// The leading clause became the title, so the description is the
	// remainder — a description re-opening with its own heading reads as a
	// glitch on the create preview (#2724).
	if gen.Description != "Ours gave up mid-mow and the grass won." {
		t.Errorf("description = %q, want the prompt minus its title clause", gen.Description)
	}
	if strings.Contains(gen.Title, "Mock") {
		t.Errorf("title %q leaks mock placeholder copy", gen.Title)
	}
	if gen.LocationQuery != "" {
		t.Errorf("location query = %q, want empty (primary-residence fallback)", gen.LocationQuery)
	}
}

func TestE2ESeedNeeds(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "single named item",
			text: "Looking for a lawn mower this weekend",
			want: []string{"Lawn mower"},
		},
		{
			name: "enumerated supply list, in text order",
			text: "Back-to-school supplies for Room 7. We need picture books, a whiteboard, " +
				"storage bins, art supplies, and construction paper.",
			want: []string{"Picture books", "Whiteboard", "Storage bins", "Art supplies", "Construction paper"},
		},
		{
			name: "order follows the sentence, not the vocabulary",
			text: "First a tent, then a drill, then a ladder.",
			want: []string{"Tent", "Drill", "Ladder"},
		},
		{
			name: "nothing concrete named",
			text: "The classroom is bare and we need help getting it ready for the kids.",
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := e2eSeedNeeds(tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("e2eSeedNeeds(%q) = %v, want %v", tc.text, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("e2eSeedNeeds(%q) = %v, want %v (index %d)", tc.text, got, tc.want, i)
				}
			}
		})
	}
}

// The deterministic provider's GenerateRequestSuggestions surfaces the whole
// named list as seed_needs so the walkthrough reels can film real multi-need
// extraction, not an off-camera paste (#2731).
func TestE2EDeterministicProviderRequestSuggestionsSeedsWholeList(t *testing.T) {
	p := NewE2EDeterministicProvider()
	res, err := p.GenerateRequestSuggestionsFunc(t.Context(),
		"Back-to-school supplies for Room 7",
		"We need picture books, a whiteboard, storage bins, art supplies, and construction paper.",
		"")
	if err != nil {
		t.Fatalf("suggestions: %v", err)
	}
	want := []string{"Picture books", "Whiteboard", "Storage bins", "Art supplies", "Construction paper"}
	if len(res.SeedNeeds) != len(want) {
		t.Fatalf("seed_needs = %v, want %v", res.SeedNeeds, want)
	}
	for i := range want {
		if res.SeedNeeds[i] != want[i] {
			t.Fatalf("seed_needs = %v, want %v", res.SeedNeeds, want)
		}
	}
}

func TestE2EDeterministicProviderClassifiesImageAsGear(t *testing.T) {
	p := NewE2EDeterministicProvider()
	got, err := p.ClassifyUnifiedCreate(t.Context(), UnifiedCreateClassifierInput{
		Image: &DetectionImage{ImageURL: "http://example/img", Filename: "food-bread.jpg"},
	})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got.Type != UnifiedCreateContentTypeGear {
		t.Errorf("image input classified as %v, want gear", got.Type)
	}
	// Text-only input keeps the keyword heuristic.
	got, err = p.ClassifyUnifiedCreate(t.Context(), UnifiedCreateClassifierInput{Text: "picnic sunday"})
	if err != nil {
		t.Fatalf("classify text: %v", err)
	}
	if got.Type != UnifiedCreateContentTypeEvent {
		t.Errorf("text input classified as %v, want event", got.Type)
	}
}

func TestE2EDeterministicProviderFoodFixtureContent(t *testing.T) {
	p := NewE2EDeterministicProvider()
	cases := []struct{ filename, wantTitle string }{
		{"food-spaghetti.jpg", "Spaghetti (2 boxes)"},
		{"food-bread.jpg", "Sourdough Loaf"},
		{"FOOD-BROCCOLI.JPG", "Fresh Broccoli"}, // case-insensitive
	}
	for _, c := range cases {
		det, err := p.DetectGearInImage(t.Context(), &DetectionImage{
			ImageURL: "http://example/img", Filename: c.filename,
		})
		if err != nil {
			t.Fatalf("detect %q: %v", c.filename, err)
		}
		if det.Title != c.wantTitle {
			t.Errorf("detect %q title = %q, want %q", c.filename, det.Title, c.wantTitle)
		}
		if det.Description == "" {
			t.Errorf("detect %q: description must be non-empty (Save gate)", c.filename)
		}
		if det.Brand != "" || det.Model != "" {
			t.Errorf("detect %q: food content must not carry brand/model noise", c.filename)
		}
	}

	// Unknown filenames keep the bare mock's default content.
	det, err := p.DetectGearInImage(t.Context(), &DetectionImage{
		ImageURL: "http://example/img", Filename: "vacation.jpg",
	})
	if err != nil {
		t.Fatalf("detect unknown: %v", err)
	}
	if det.Title != "Mock Gear Item" {
		t.Errorf("unknown filename title = %q, want default Mock Gear Item", det.Title)
	}
}
