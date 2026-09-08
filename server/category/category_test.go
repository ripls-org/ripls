package category

import "testing"

func TestCategorize(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		description string
		want        string
	}{
		{"matches cooking from title", "Sunday potluck", "", "Cooking"},
		{"matches camping from description", "Weekend hangout", "We'll set up tents at the campsite", "Camping"},
		{"prefers more specific bucket", "Trail hike at sunrise", "Hiking the foothills together", "Hiking"},
		{"falls back to Outdoors when only generic keyword matches", "Afternoon at the park", "Just outdoor time", "Outdoors"},
		{"returns empty when nothing matches", "Important meeting", "Discuss quarterly KPIs", ""},
		{"trims whitespace-only inputs", "   ", "  ", ""},
		{"is case-insensitive", "BIKE TUNE-UP", "", "Biking"},
		{"matches across title + description join", "Saturday plans", "Going on a kayak trip", "Paddling"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Categorize(tt.title, tt.description)
			if got != tt.want {
				t.Errorf("Categorize(%q, %q) = %q, want %q",
					tt.title, tt.description, got, tt.want)
			}
		})
	}
}

func TestCategorize_OrderingTieBreak(t *testing.T) {
	// "Cooking" is listed before "Gardening"; a text containing both
	// keyword sets should return the earlier-defined category.
	got := Categorize("Garden potluck", "Bring a recipe to share")
	if got != "Cooking" {
		t.Errorf("got %q, want Cooking (Cooking is listed before Gardening)", got)
	}
}
