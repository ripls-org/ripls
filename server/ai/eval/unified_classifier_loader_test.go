package eval

import "testing"

// TestLoadUnifiedClassifierGoldens parses the goldens file and verifies
// per-case invariants. Runs in the default test suite (no `benchmark`
// tag) so a malformed corpus or a missing required field fails fast in
// CI instead of only at live-API run time.
func TestLoadUnifiedClassifierGoldens(t *testing.T) {
	g, err := LoadUnifiedClassifierGoldens("testdata/unified_create_classifier_goldens.json")
	if err != nil {
		t.Fatalf("LoadUnifiedClassifierGoldens: %v", err)
	}
	if len(g.Cases) == 0 {
		t.Fatal("expected at least one case")
	}

	seenIDs := map[string]bool{}
	allowedTypes := map[string]bool{"gear": true, "event": true, "request": true}
	allowedModes := map[string]bool{"text": true, "image": true, "url": true}

	for _, c := range g.Cases {
		if c.ID == "" {
			t.Error("case with empty ID")
		}
		if seenIDs[c.ID] {
			t.Errorf("duplicate case ID %q", c.ID)
		}
		seenIDs[c.ID] = true

		if !allowedModes[c.Mode()] {
			t.Errorf("case %q: unknown input_mode %q (want text|image|url)", c.ID, c.InputMode)
		}
		if !allowedTypes[c.ExpectedType] {
			t.Errorf("case %q: expected_type %q not in {gear,event,request}", c.ID, c.ExpectedType)
		}
		for _, alt := range c.AltTypes {
			if !allowedTypes[alt] {
				t.Errorf("case %q: alt_types contains %q, not in {gear,event,request}", c.ID, alt)
			}
		}

		// Per-mode required fields.
		switch c.Mode() {
		case "text":
			if c.Prompt == "" {
				t.Errorf("text-mode case %q: missing prompt", c.ID)
			}
		case "image":
			if c.ImageFile == "" {
				t.Errorf("image-mode case %q: missing image_file", c.ID)
			}
			if c.MimeType == "" {
				t.Errorf("image-mode case %q: missing mime_type", c.ID)
			}
		case "url":
			if c.PageTitle == "" && c.PageDescription == "" {
				t.Errorf("url-mode case %q: at least one of page_title / page_description required", c.ID)
			}
		}
	}
}
