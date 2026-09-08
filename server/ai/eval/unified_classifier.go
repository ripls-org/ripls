package eval

import (
	"encoding/json"
	"fmt"
	"os"

	"go.ripls.org/ripls/server/ai"
)

// UnifiedClassifierGoldens is the on-disk schema for
// testdata/unified_create_classifier_goldens.json.
type UnifiedClassifierGoldens struct {
	Description string                  `json:"description,omitempty"`
	Version     int                     `json:"version,omitempty"`
	Cases       []UnifiedClassifierCase `json:"cases"`
}

// UnifiedClassifierCase is a single unified-create classifier golden.
// AltTypes lists additional types the model may legitimately produce
// for an ambiguous prompt without being marked wrong.
//
// InputMode selects which classifier prompt builder + provider input
// shape the test exercises:
//   - "text" (default): Prompt is sent verbatim.
//   - "image": ImageFile is read from server/test_data/ and its bytes
//     are attached to the classifier API call as a vision content
//     block (see #1939). Image-mode accuracy measures actual vision
//     capability — the classifier reads the photo, not a placeholder
//     prompt. Production goes through bucket resolution to a signed
//     URL; the eval skips that and passes bytes directly.
//   - "url": PageTitle + PageDescription stand in for what the live
//     web-fetcher would produce; the classifier sees only those two
//     fields (the per-type generator gets the page body).
type UnifiedClassifierCase struct {
	ID              string   `json:"id"`
	Tags            []string `json:"tags,omitempty"`
	InputMode       string   `json:"input_mode,omitempty"` // "text" (default) | "image" | "url"
	Prompt          string   `json:"prompt,omitempty"`
	ImageFile       string   `json:"image_file,omitempty"`
	MimeType        string   `json:"mime_type,omitempty"`
	PageTitle       string   `json:"page_title,omitempty"`
	PageDescription string   `json:"page_description,omitempty"`
	ExpectedType    string   `json:"expected_type"`
	AltTypes        []string `json:"alt_types,omitempty"`
}

// Mode returns the case's input mode, defaulting to "text" when unset
// so the original 17-case text-only corpus stays valid without
// per-case edits.
func (c UnifiedClassifierCase) Mode() string {
	if c.InputMode == "" {
		return "text"
	}
	return c.InputMode
}

// LoadUnifiedClassifierGoldens reads and parses the unified-create
// classifier goldens JSON file.
func LoadUnifiedClassifierGoldens(path string) (*UnifiedClassifierGoldens, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var g UnifiedClassifierGoldens
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &g, nil
}

// EvaluateUnifiedClassifier emits one Check per case: classified_type
// passes when the result matches ExpectedType or any AltTypes entry.
func EvaluateUnifiedClassifier(c UnifiedClassifierCase, result *ai.UnifiedCreateClassification) []Check {
	got := string(result.Type)
	pass := got == c.ExpectedType
	if !pass {
		for _, alt := range c.AltTypes {
			if got == alt {
				pass = true
				break
			}
		}
	}
	reason := ""
	if !pass {
		reason = fmt.Sprintf("expected %q (alt %v), got %q",
			c.ExpectedType, c.AltTypes, got)
	}
	return []Check{{CaseID: c.ID, Field: "classified_type", Pass: pass, Reason: reason}}
}
