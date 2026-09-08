package ai

import "testing"

func TestSanitizePlaceholder(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Known placeholders → empty.
		{"Unknown", ""},
		{"unknown", ""},
		{"UNKNOWN", ""},
		{"<UNKNOWN>", ""},
		{"<unknown>", ""},
		{"N/A", ""},
		{"n/a", ""},
		{"NA", ""},
		{"None", ""},
		{"Not visible", ""},
		{"not available", ""},
		{"Not specified", ""},
		{"Generic", ""},
		{"Unspecified", ""},
		{"Unbranded", ""},
		{"not found", ""},

		// Whitespace-wrapped placeholders.
		{"  unknown  ", ""},
		{" N/A ", ""},

		// Empty / blank.
		{"", ""},
		{"   ", ""},

		// Real values preserved.
		{"DeWalt", "DeWalt"},
		{"DCD771C2", "DCD771C2"},
		{"Coleman", "Coleman"},
		{"mixed_plastic_metal", "mixed_plastic_metal"},
		{"Power Tools", "Power Tools"},
	}

	for _, tt := range tests {
		got := SanitizePlaceholder(tt.input)
		if got != tt.want {
			t.Errorf("SanitizePlaceholder(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeGearDetection(t *testing.T) {
	t.Run("nil detection is safe", func(t *testing.T) {
		SanitizeGearDetection(nil) // should not panic
	})

	t.Run("clears placeholder fields", func(t *testing.T) {
		d := &GearDetection{
			Title:            "Cordless Drill",
			Brand:            "<UNKNOWN>",
			Model:            "Unknown",
			Category:         "Power Tools",
			MaterialCategory: "N/A",
			Confidence:       0.8,
		}
		SanitizeGearDetection(d)

		if d.Title != "Cordless Drill" {
			t.Errorf("Title changed: %q", d.Title)
		}
		if d.Brand != "" {
			t.Errorf("Brand not cleared: %q", d.Brand)
		}
		if d.Model != "" {
			t.Errorf("Model not cleared: %q", d.Model)
		}
		if d.Category != "Power Tools" {
			t.Errorf("Category changed: %q", d.Category)
		}
		if d.MaterialCategory != "" {
			t.Errorf("MaterialCategory not cleared: %q", d.MaterialCategory)
		}
	})

	t.Run("preserves real values", func(t *testing.T) {
		d := &GearDetection{
			Brand:            "DeWalt",
			Model:            "DCD771C2",
			Category:         "Power Tools",
			MaterialCategory: "cordless_power_tool",
		}
		SanitizeGearDetection(d)

		if d.Brand != "DeWalt" {
			t.Errorf("Brand changed: %q", d.Brand)
		}
		if d.Model != "DCD771C2" {
			t.Errorf("Model changed: %q", d.Model)
		}
	})
}

func TestSanitizeGearGeneration(t *testing.T) {
	t.Run("nil generation is safe", func(t *testing.T) {
		SanitizeGearGeneration(nil) // should not panic
	})

	t.Run("clears placeholder fields", func(t *testing.T) {
		g := &GearGeneration{
			Title:            "Camping Tent",
			Brand:            "Generic",
			Category:         "Camping & Outdoors",
			MaterialCategory: "not specified",
		}
		SanitizeGearGeneration(g)

		if g.Title != "Camping Tent" {
			t.Errorf("Title changed: %q", g.Title)
		}
		if g.Brand != "" {
			t.Errorf("Brand not cleared: %q", g.Brand)
		}
		if g.Category != "Camping & Outdoors" {
			t.Errorf("Category changed: %q", g.Category)
		}
		if g.MaterialCategory != "" {
			t.Errorf("MaterialCategory not cleared: %q", g.MaterialCategory)
		}
	})
}

func TestSanitizeMaterialCategory(t *testing.T) {
	t.Run("clears unknown enum tokens", func(t *testing.T) {
		cases := []struct {
			input string
			want  string
		}{
			// Unknown tokens are cleared.
			{"bogus_material", ""},
			{"not_a_category", ""},
			// Placeholder strings are cleared (existing behavior).
			{"unknown", ""},
			{"N/A", ""},
			{"", ""},
			// Valid enum tokens are preserved.
			{"solid_metal", "solid_metal"},
			{"mixed_plastic_metal", "mixed_plastic_metal"},
			{"electronics_small", "electronics_small"},
		}
		for _, tc := range cases {
			d := &GearDetection{MaterialCategory: tc.input}
			SanitizeGearDetection(d)
			if d.MaterialCategory != tc.want {
				t.Errorf("SanitizeGearDetection MaterialCategory(%q) = %q, want %q", tc.input, d.MaterialCategory, tc.want)
			}
			g := &GearGeneration{MaterialCategory: tc.input}
			SanitizeGearGeneration(g)
			if g.MaterialCategory != tc.want {
				t.Errorf("SanitizeGearGeneration MaterialCategory(%q) = %q, want %q", tc.input, g.MaterialCategory, tc.want)
			}
		}
	})
}
