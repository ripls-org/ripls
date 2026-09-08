// Canonical Gen* JSON output schemas — the single source of truth for the
// wire shape each AI provider is asked to produce. Each leaf provider
// (Anthropic, OpenAI, Gemini) feeds these structs through its JSON-Schema
// reflector (`generateToolSchema[T]`, `generateSchema[T]`, or Genkit's
// implicit reflection) so the JSON-Schema sent to the model and the Go
// struct used to unmarshal the response can never drift apart per
// provider.
//
// Ordering matters: per-token streaming on Anthropic and Gemini-lite
// surfaces each top-level key the moment the parser observes that key's
// closing comma/brace. The watched-keys list (`experienceStreamingKeys`,
// `requestStreamingKeys`, `gearStreamingKeys` in
// `streaming_provider.go`) is derived from these schemas at init time
// via `streamingKeysFromSchema[T]`, so any field rename on a struct
// here automatically renames the wire key the parser watches for.
//
// The fields are positioned to maximize the early-fire wall-clock win
// documented in `docs/evals/1157-genexperience-latency.md`:
//
//   - `title` first — gives the client a renderable preview ~1 s before
//     terminal on real per-token streamers.
//   - `search_keywords` and `location_query` next — drive Pexels and
//     Mapbox fan-out the moment they close. Closing them earlier in the
//     JSON saves wall-clock on every call.
//   - Date / time / time_confidence — drive the mid-stream `time` event.
//   - Value estimate, mentioned names, confidence — late, no fan-out.
//
// These structs are wire-only. The receiver-side types (`ExperienceGeneration`,
// `RequestGeneration`, `GearGeneration`) live in `provider.go` and may
// carry additional fields that are not LLM-emitted.
package ai

import (
	"encoding/json"
	"fmt"
)

// robustFloat32 is a float32 that never returns an error during JSON unmarshal
// when the model emits an out-of-range numeric value (e.g. a confidence score
// with a gigantic exponent). Values beyond float64 range are saturated to 0 or
// 1 by sign; values in float64 range but outside float32 range produce ±Inf,
// which NormalizeConfidence clamps to [0.0, 1.0].
type robustFloat32 float32

func (r *robustFloat32) UnmarshalJSON(data []byte) error {
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		*r = 0
		return nil
	}
	f, err := n.Float64()
	if err != nil {
		// strconv.ErrRange: value exceeds float64 range; clamp by sign.
		if len(data) > 0 && data[0] == '-' {
			*r = 0
		} else {
			*r = 1
		}
		return nil
	}
	// float64 → float32 may produce ±Inf; NormalizeConfidence handles those.
	*r = robustFloat32(float32(f))
	return nil
}

// valueEstimateOutput is the slim variant of ValueEstimate that the LLM is
// asked to produce on every Gen* method that includes a value estimate.
// The receiver-side ValueEstimate adds a `Sources []string` field that
// the LLMs we currently use don't populate reliably; including it nudges
// the model to fabricate URLs. The receiver carries an empty Sources
// slice — no information loss.
type valueEstimateOutput struct {
	EstimatedValueUSD float32 `json:"estimated_value_usd"`
	Confidence        float64 `json:"confidence"`
	Reasoning         string  `json:"reasoning"`
}

// UnmarshalJSON tolerates the LLM occasionally returning a bare string
// (e.g. "n/a") instead of the expected object. The string lands in
// Reasoning with zeroed numerics — same behavior as the legacy
// per-provider valueEstimateSchema types this struct replaces.
func (v *valueEstimateOutput) UnmarshalJSON(data []byte) error {
	type alias valueEstimateOutput
	var obj alias
	if err := json.Unmarshal(data, &obj); err == nil {
		*v = valueEstimateOutput(obj)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*v = valueEstimateOutput{Reasoning: s}
		return nil
	}
	return fmt.Errorf("valueEstimateOutput: cannot unmarshal %s", string(data))
}

// toValueEstimate converts a wire-side valueEstimateOutput into the
// receiver-side ValueEstimate, returning nil for a nil input. Sources is
// always nil — the LLM doesn't populate it (see valueEstimateOutput).
func (v *valueEstimateOutput) toValueEstimate() *ValueEstimate {
	if v == nil {
		return nil
	}
	return &ValueEstimate{
		EstimatedValueUSD: v.EstimatedValueUSD,
		Confidence:        v.Confidence,
		Reasoning:         v.Reasoning,
	}
}

// experienceFromTextOutput is the canonical schema for
// GenerateExperienceFromText / GenerateExperienceFromTextStreaming on
// every provider. Description is intentionally absent: text mode emits
// the user's prompt verbatim as the description, not an AI-rewritten
// version (see streamGenExperienceFromText in
// server/services/experience/gen_ai_streaming.go).
type experienceFromTextOutput struct {
	Title          string               `json:"title"`
	SearchKeywords []string             `json:"search_keywords"`
	LocationQuery  string               `json:"location_query"`
	Date           string               `json:"date"`
	Time           string               `json:"time"`
	TimeConfidence string               `json:"time_confidence"`
	ValueEstimate  *valueEstimateOutput `json:"value_estimate"`
	MentionedNames []string             `json:"mentioned_names"`
	Confidence     robustFloat32        `json:"confidence"`
}

// experienceFromImageOutput is the canonical schema for
// GenerateExperienceFromImage / GenerateExperienceFromImageStreaming on
// every provider. Adds Description (image mode generates one — text
// mode reuses the user's prompt).
type experienceFromImageOutput struct {
	Title          string               `json:"title"`
	Description    string               `json:"description"`
	SearchKeywords []string             `json:"search_keywords"`
	LocationQuery  string               `json:"location_query"`
	Date           string               `json:"date"`
	Time           string               `json:"time"`
	TimeConfidence string               `json:"time_confidence"`
	ValueEstimate  *valueEstimateOutput `json:"value_estimate"`
	Confidence     robustFloat32        `json:"confidence"`
}

// experienceFromWebpageOutput is the canonical schema for
// GenerateExperienceFromWebpage / GenerateExperienceFromWebpageStreaming
// on every provider. Same shape as image-mode.
type experienceFromWebpageOutput struct {
	Title          string               `json:"title"`
	Description    string               `json:"description"`
	SearchKeywords []string             `json:"search_keywords"`
	LocationQuery  string               `json:"location_query"`
	Date           string               `json:"date"`
	Time           string               `json:"time"`
	TimeConfidence string               `json:"time_confidence"`
	ValueEstimate  *valueEstimateOutput `json:"value_estimate"`
	Confidence     robustFloat32        `json:"confidence"`
}

// gearDetectionOutput is the canonical schema for DetectGearInImage /
// DetectGearInImageStreaming on every provider. Flat (unwrapped) so
// title and description are top-level keys — the parser fires
// FieldEvents on them mid-stream, matching the gearStreamingKeys watch
// list. Wire-only; converts to the receiver-side GearDetection in each
// provider's stream finalization. Sources is intentionally absent (see
// valueEstimateOutput).
//
// Description is intentionally a single crisp sentence (~80 chars, max
// 160) — see the description block in buildGearDetectionPrompt for
// rationale (issue #2225). The product-spec lookup in
// server/services/gear/gen_ai.go can override it with longer
// manufacturer-authored text when the structured spec database hits.
type gearDetectionOutput struct {
	Title            string               `json:"title"`
	Description      string               `json:"description"`
	Category         string               `json:"category"`
	Brand            string               `json:"brand"`
	Model            string               `json:"model"`
	MaterialCategory string               `json:"material_category"`
	WeightGrams      float32              `json:"weight_grams"`
	Confidence       robustFloat32        `json:"confidence"`
	ValueEstimate    *valueEstimateOutput `json:"value_estimate"`
}

// gearFromTextOutput is the canonical schema for GenerateGearFromText /
// GenerateGearFromTextStreaming on every provider. Description is
// intentionally absent: text mode synthesizes the description from the
// user's prompt server-side and discards any AI-generated description
// (see streamGenGearFromText in
// server/services/gear/gen_ai_streaming.go), so emitting it would be
// wasted output tokens. Model is likewise absent: the receiver-side
// GearGeneration carries no model field and no service consumes one on
// the text path (the title already includes brand/model when the user
// mentions them), so emitting it was pure output-token waste (#1265).
// Title + location_query lead so the title-visible and Mapbox fan-out
// branches fire as early as possible on real per-token streamers.
type gearFromTextOutput struct {
	Title            string               `json:"title"`
	LocationQuery    string               `json:"location_query"`
	Category         string               `json:"category"`
	Brand            string               `json:"brand"`
	MaterialCategory string               `json:"material_category"`
	WeightGrams      float32              `json:"weight_grams"`
	ValueEstimate    *valueEstimateOutput `json:"value_estimate"`
	Confidence       robustFloat32        `json:"confidence"`
}

// gearFromWebpageOutput is the canonical schema for
// GenerateGearFromWebpage / GenerateGearFromWebpageStreaming on every
// provider. Webpage mode emits its own description (the AI rewrites the
// page body into a gear blurb that the client renders mid-stream) so
// description is present here, unlike text mode. SearchKeywords leads
// after Title to fire the Pexels branch early.
type gearFromWebpageOutput struct {
	Title            string               `json:"title"`
	SearchKeywords   []string             `json:"search_keywords"`
	Description      string               `json:"description"`
	Category         string               `json:"category"`
	Brand            string               `json:"brand"`
	MaterialCategory string               `json:"material_category"`
	WeightGrams      float32              `json:"weight_grams"`
	ValueEstimate    *valueEstimateOutput `json:"value_estimate"`
	Confidence       robustFloat32        `json:"confidence"`
}

// requestFromTextOutput is the canonical schema for
// GenerateRequestContent / GenerateRequestContentStreaming on every
// provider. Description is intentionally absent: text mode reuses the
// user's prompt as the description (mirrors the experience and gear
// text paths) so emitting an AI-rewritten description would be wasted
// output tokens. Title leads, then search_keywords (Pexels) and
// location_query (Mapbox) for early-fire fan-out on per-token streamers.
type requestFromTextOutput struct {
	Title          string               `json:"title"`
	SearchKeywords []string             `json:"search_keywords"`
	LocationQuery  string               `json:"location_query"`
	ValueEstimate  *valueEstimateOutput `json:"value_estimate"`
	Confidence     robustFloat32        `json:"confidence"`
}

// requestFromImageOutput is the canonical schema for
// GenerateRequestFromImage / GenerateRequestFromImageStreaming on every
// provider. Adds Description (image mode generates one — text mode
// reuses the user's prompt) which fires the mid-stream description
// event for the client.
type requestFromImageOutput struct {
	Title          string               `json:"title"`
	Description    string               `json:"description"`
	SearchKeywords []string             `json:"search_keywords"`
	LocationQuery  string               `json:"location_query"`
	ValueEstimate  *valueEstimateOutput `json:"value_estimate"`
	Confidence     robustFloat32        `json:"confidence"`
}

// communityGenerationOutput is the canonical schema for
// GenerateCommunityContent / GenerateCommunityContentStreaming on every
// provider. Community generation produces only search_keywords — the
// terms that drive Pexels stock-image fan-out for the community's
// background image. It's the simplest of the genAI surfaces: no title,
// description, confidence, or location/value dimensions.
type communityGenerationOutput struct {
	SearchKeywords []string `json:"search_keywords"`
}
