// prompts_unified.go contains AI prompts for the unified-create flow
// (see docs/design/unified-create.md). Edit guidance applies the same as
// prompts.go — keep token counts tight.

package ai

import "fmt"

// UnifiedCreateClassifierPromptVersion identifies the active classifier
// prompt; bumped whenever the prompt body in buildUnifiedCreateClassifierPrompt
// changes meaningfully. Surface this in structured logs so an eval result
// can be tied back to the prompt that produced it.
//
// v2 (#1932): slimmed response shape from {type, confidence, notes} to
// {type} only — confidence wasn't gating any production code path and
// notes weren't consumed by any per-type generator. Cuts output tokens
// from ~30-50 to ~5, materially reducing decode latency.
// v3 (#1939): image bytes now flow to the classifier for image-mode
// requests (previously the image prompt was text-only with a
// default-to-gear bias). Dropped the default-to-gear instruction
// since the model can now actually see the photo.
const UnifiedCreateClassifierPromptVersion = "v3"

// UnifiedCreateContentType is the string token the classifier emits in its
// JSON output. Stable wire format; do not rename without coordinating with
// the parser in server/services/unified_create.
type UnifiedCreateContentType string

const (
	UnifiedCreateContentTypeGear    UnifiedCreateContentType = "gear"
	UnifiedCreateContentTypeEvent   UnifiedCreateContentType = "event"
	UnifiedCreateContentTypeRequest UnifiedCreateContentType = "request"
)

// UnifiedCreateClassification is the JSON shape the classifier produces.
// Only Type is consumed downstream — see #1932 for the rationale on
// dropping confidence + notes.
type UnifiedCreateClassification struct {
	Type UnifiedCreateContentType `json:"type"`
}

// buildUnifiedCreateClassifierPrompt builds the classifier prompt for the
// unified-create flow. Routes the user's input to one of three types: gear,
// event, request. Lend vs. give is not classified — it is a user toggle.
//
// Output is a single-field JSON object so downstream parsers can keep using
// json.Unmarshal into UnifiedCreateClassification. A follow-up under #1932
// tightens this further via provider-native enum-constrained structured
// output, collapsing decode to 1-2 tokens.
func buildUnifiedCreateClassifierPrompt(userInput string) string {
	return fmt.Sprintf(`Classify the user's content into exactly one of three categories. Return JSON only.

{"type": "gear" | "event" | "request"}

- "gear" = a physical item the user has (whether to lend or give is the user's call later — do not guess)
- "event" = an activity at a time/place
- "request" = asking for help, an item, or someone's time

Examples:
- "Going for a hike tomorrow at Mt Sanitas" → event
- "Looking for a hiking buddy tomorrow" → request
- "Climbing rope, used a few times" → gear

User input:
%s

Return only the JSON.`, userInput)
}

// buildUnifiedCreateClassifierImagePrompt is the image-mode variant.
// The image bytes are attached to the API call as a separate content
// block; this prompt provides only the classification instruction.
func buildUnifiedCreateClassifierImagePrompt() string {
	return `Classify what the attached image depicts into exactly one of three categories. Return JSON only.

{"type": "gear" | "event" | "request"}

- "gear" = a physical item the user owns or possesses
- "event" = a flyer, poster, ticket, or scene depicting an activity at a time/place
- "request" = a photo of something the user needs help with, is looking for, or wants to find (e.g., a "lost dog" sign, a "looking for" notice)

Return only the JSON.`
}

// buildUnifiedCreateClassifierWebpagePrompt is the URL-mode variant. The
// caller fetches the page once and passes title + meta description here;
// the per-type generator gets the full body.
func buildUnifiedCreateClassifierWebpagePrompt(pageTitle, pageDescription string) string {
	return fmt.Sprintf(`Classify what this webpage represents into exactly one of three categories. Return JSON only.

{"type": "gear" | "event" | "request"}

- "gear" = a product page (e.g., Amazon, REI listing)
- "event" = an event page (e.g., Eventbrite, Meetup, venue listing)
- "request" = a help-wanted or similar ask

Page title: %s
Page description: %s

Return only the JSON.`, pageTitle, pageDescription)
}
