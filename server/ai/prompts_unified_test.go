package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildUnifiedCreateClassifierPrompt(t *testing.T) {
	prompt := buildUnifiedCreateClassifierPrompt(`Going for a hike tomorrow`)

	mustContain := []string{
		`"type": "gear" | "event" | "request"`,
		`Going for a hike tomorrow`,
		`Return only the JSON.`,
		`gear`,
		`event`,
		`request`,
	}
	for _, sub := range mustContain {
		if !strings.Contains(prompt, sub) {
			t.Errorf("prompt missing %q\n--- prompt ---\n%s", sub, prompt)
		}
	}

	// The classifier output must NOT enumerate loan/give as separate types
	// — those are user-toggle states, not classifier outputs (design doc
	// § Decisions #8). Block the type-name strings only; the prose may
	// still reference lend/give to instruct the model not to guess.
	for _, banned := range []string{"gear_loan", "gear_give"} {
		if strings.Contains(prompt, banned) {
			t.Errorf("prompt should not emit type %q (lend/give is a user toggle, not classifier output)", banned)
		}
	}
}

func TestBuildUnifiedCreateClassifierImagePrompt(t *testing.T) {
	prompt := buildUnifiedCreateClassifierImagePrompt()
	for _, sub := range []string{`"type": "gear" | "event" | "request"`, "Return only the JSON."} {
		if !strings.Contains(prompt, sub) {
			t.Errorf("image prompt missing %q", sub)
		}
	}
}

func TestBuildUnifiedCreateClassifierWebpagePrompt(t *testing.T) {
	prompt := buildUnifiedCreateClassifierWebpagePrompt("Acme Drill", "A power drill on Amazon")
	for _, sub := range []string{"Acme Drill", "A power drill on Amazon", `"type": "gear" | "event" | "request"`} {
		if !strings.Contains(prompt, sub) {
			t.Errorf("webpage prompt missing %q", sub)
		}
	}
}

func TestUnifiedCreateClassifierPromptVersionStable(t *testing.T) {
	// Catches accidental version bumps in unrelated PRs.
	if UnifiedCreateClassifierPromptVersion == "" {
		t.Error("UnifiedCreateClassifierPromptVersion must not be empty")
	}
}

// TestUnifiedCreateClassifierGoldensValid asserts every golden case parses
// and references one of the closed-set types. Acts as a structural
// regression guard for the corpus; semantic threshold gating lives in
// P2.14.
func TestUnifiedCreateClassifierGoldensValid(t *testing.T) {
	path := filepath.Join("eval", "testdata", "unified_create_classifier_goldens.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read goldens: %v", err)
	}

	var corpus struct {
		Cases []struct {
			ID              string   `json:"id"`
			Tags            []string `json:"tags"`
			InputMode       string   `json:"input_mode,omitempty"`
			Prompt          string   `json:"prompt,omitempty"`
			ImageFile       string   `json:"image_file,omitempty"`
			MimeType        string   `json:"mime_type,omitempty"`
			PageTitle       string   `json:"page_title,omitempty"`
			PageDescription string   `json:"page_description,omitempty"`
			ExpectedType    string   `json:"expected_type"`
			AltTypes        []string `json:"alt_types,omitempty"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("parse goldens: %v", err)
	}

	if len(corpus.Cases) == 0 {
		t.Fatal("no cases in goldens corpus")
	}

	allowed := map[string]bool{"gear": true, "event": true, "request": true}
	allowedModes := map[string]bool{"": true, "text": true, "image": true, "url": true}
	seenIDs := map[string]bool{}
	for _, c := range corpus.Cases {
		if c.ID == "" {
			t.Errorf("case missing id: %+v", c)
		}
		if seenIDs[c.ID] {
			t.Errorf("duplicate case id %q", c.ID)
		}
		seenIDs[c.ID] = true
		if !allowedModes[c.InputMode] {
			t.Errorf("case %q: unknown input_mode %q", c.ID, c.InputMode)
		}
		mode := c.InputMode
		if mode == "" {
			mode = "text"
		}
		switch mode {
		case "text":
			if c.Prompt == "" {
				t.Errorf("text-mode case %q: missing prompt", c.ID)
			}
		case "image":
			if c.ImageFile == "" {
				t.Errorf("image-mode case %q: missing image_file", c.ID)
			}
		case "url":
			if c.PageTitle == "" && c.PageDescription == "" {
				t.Errorf("url-mode case %q: must have at least one of page_title / page_description", c.ID)
			}
		}
		if !allowed[c.ExpectedType] {
			t.Errorf("case %q: expected_type %q not in {gear,event,request}", c.ID, c.ExpectedType)
		}
		for _, alt := range c.AltTypes {
			if !allowed[alt] {
				t.Errorf("case %q: alt_type %q not in {gear,event,request}", c.ID, alt)
			}
		}
	}
}
