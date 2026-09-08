package simulation

import "testing"

// validVenueCategories is the set of categories present in venues.json.
var validVenueCategories = map[string]bool{
	"park":             true,
	"library":          true,
	"cafe":             true,
	"restaurant":       true,
	"community_center": true,
	"brewery":          true,
	"gym":              true,
	"church":           true,
	"school":           true,
}

func TestLoadExperienceTemplates(t *testing.T) {
	templates, err := LoadExperienceTemplates()
	if err != nil {
		t.Fatalf("LoadExperienceTemplates() error: %v", err)
	}

	if len(templates) < 50 {
		t.Fatalf("Expected at least 50 experience templates, got %d", len(templates))
	}

	for i, tmpl := range templates {
		if tmpl.Name == "" {
			t.Errorf("Experience[%d] has empty name", i)
		}
		if tmpl.Description == "" {
			t.Errorf("Experience[%d] %q has empty description", i, tmpl.Name)
		}
		if tmpl.DurationMinutes <= 0 {
			t.Errorf("Experience[%d] %q has non-positive duration: %d", i, tmpl.Name, tmpl.DurationMinutes)
		}
		if tmpl.Category == "" {
			t.Errorf("Experience[%d] %q has empty category", i, tmpl.Name)
		}
		if tmpl.VenueCategory == "" {
			t.Errorf("Experience[%d] %q has empty venue_category", i, tmpl.Name)
		}
		if !validVenueCategories[tmpl.VenueCategory] {
			t.Errorf("Experience[%d] %q has invalid venue_category %q", i, tmpl.Name, tmpl.VenueCategory)
		}
	}
}

func TestLoadExperienceTemplatesVariety(t *testing.T) {
	templates, err := LoadExperienceTemplates()
	if err != nil {
		t.Fatalf("LoadExperienceTemplates() error: %v", err)
	}

	categories := make(map[string]int)
	venueCategories := make(map[string]int)
	for _, tmpl := range templates {
		categories[tmpl.Category]++
		venueCategories[tmpl.VenueCategory]++
	}

	if len(categories) < 4 {
		t.Errorf("Expected at least 4 experience categories, got %d: %v", len(categories), categories)
	}
	if len(venueCategories) < 4 {
		t.Errorf("Expected at least 4 venue categories, got %d: %v", len(venueCategories), venueCategories)
	}
}

func TestLoadRequestTemplates(t *testing.T) {
	templates, err := LoadRequestTemplates()
	if err != nil {
		t.Fatalf("LoadRequestTemplates() error: %v", err)
	}

	if len(templates) < 50 {
		t.Fatalf("Expected at least 50 request templates, got %d", len(templates))
	}

	for i, tmpl := range templates {
		if tmpl.Title == "" {
			t.Errorf("Request[%d] has empty title", i)
		}
		if tmpl.Description == "" {
			t.Errorf("Request[%d] %q has empty description", i, tmpl.Title)
		}
		if tmpl.Category == "" {
			t.Errorf("Request[%d] %q has empty category", i, tmpl.Title)
		}
	}
}

func TestLoadChatTemplates(t *testing.T) {
	templates, err := LoadChatTemplates()
	if err != nil {
		t.Fatalf("LoadChatTemplates() error: %v", err)
	}

	// Verify all transaction types have the expected number of phases.
	types := []struct {
		name       string
		phases     []ChatPhase
		wantPhases int
	}{
		{"loan", templates.Loan, 5},
		{"giveaway", templates.Giveaway, 4},
		{"request", templates.Request, 5},
		{"experience", templates.Experience, 4},
	}

	for _, tt := range types {
		if len(tt.phases) != tt.wantPhases {
			t.Errorf("Chat type %q has %d phases, want %d", tt.name, len(tt.phases), tt.wantPhases)
		}
		if len(tt.phases) == 0 {
			continue
		}
		for _, phase := range tt.phases {
			if phase.Phase == "" {
				t.Errorf("Chat type %q has phase with empty name", tt.name)
			}
			if len(phase.Messages) < 3 {
				t.Errorf("Chat type %q phase %q has %d messages (want >= 3)",
					tt.name, phase.Phase, len(phase.Messages))
			}
			for j, msg := range phase.Messages {
				if msg.Text == "" {
					t.Errorf("Chat type %q phase %q message[%d] has empty text",
						tt.name, phase.Phase, j)
				}
			}
		}
	}
}

func TestLoadRequestTemplatesVariety(t *testing.T) {
	templates, err := LoadRequestTemplates()
	if err != nil {
		t.Fatalf("LoadRequestTemplates() error: %v", err)
	}

	categories := make(map[string]int)
	for _, tmpl := range templates {
		categories[tmpl.Category]++
	}

	if len(categories) < 3 {
		t.Errorf("Expected at least 3 request categories, got %d: %v", len(categories), categories)
	}
}
