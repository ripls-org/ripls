package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"go.ripls.org/ripls/server/health"
)

// Provider defines the interface for AI/LLM providers that can analyze images
// and generate content with structured output.
// Note: Embeddings are handled by the separate embedding package (server/ai/embedding).
// task (gear/experience/request/community x text/image/webpage, streaming and
// unary). Every implementation is a whole AI vendor; splitting the interface
// would fragment the provider registry and the fallback chain described in
// docs/server/llm.md without removing a single method.
//
//nolint:interfacebloat // ai.Provider is deliberately one method per generation
type Provider interface {
	// Name returns the provider's name for logging and identification.
	Name() string

	// DetectGearInImage analyzes an image and returns the primary detected gear item.
	// Returns nil if no gear is detected in the image.
	DetectGearInImage(ctx context.Context, req *DetectionImage) (*GearDetection, error)

	// DetectGearInImageStreaming is the streaming variant of
	// DetectGearInImage. Same channel semantics as
	// GenerateExperienceFromTextStreaming, but the terminal value type is
	// GearDetectionStreamFinal (carries *GearDetection rather than
	// *GearGeneration).
	DetectGearInImageStreaming(ctx context.Context, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal, error)

	// GenerateGearFromText creates gear content from a text prompt.
	GenerateGearFromText(ctx context.Context, prompt, region string) (*GearGeneration, error)

	// GenerateGearFromWebpage extracts gear content and value estimate from webpage content.
	// Designed for product pages (e.g., Amazon, REI) to extract item details and pricing.
	GenerateGearFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (*GearGeneration, error)

	// GenerateGearFromWebpageStreaming is the streaming variant of
	// GenerateGearFromWebpage. Same channel semantics as
	// GenerateGearFromTextStreaming.
	GenerateGearFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error)

	// GenerateCommunityContent creates community content from a user prompt.
	GenerateCommunityContent(ctx context.Context, prompt, region string) (*CommunityGeneration, error)

	// GenerateCommunityContentStreaming is the streaming variant of
	// GenerateCommunityContent. Same channel semantics as
	// GenerateExperienceFromTextStreaming.
	GenerateCommunityContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal, error)

	// GenerateRequestContent creates request content from a user prompt.
	GenerateRequestContent(ctx context.Context, prompt, region string) (*RequestGeneration, error)

	// GenerateRequestFromImage analyzes an image and creates request content.
	GenerateRequestFromImage(ctx context.Context, image *DetectionImage, region string) (*RequestGeneration, error)

	// GenerateRequestFromImageStreaming is the streaming variant of
	// GenerateRequestFromImage. Same channel semantics as
	// GenerateExperienceFromTextStreaming.
	GenerateRequestFromImageStreaming(ctx context.Context, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error)

	// GenerateExperienceFromText creates experience content from a text prompt.
	GenerateExperienceFromText(ctx context.Context, prompt, region, currentTime string) (*ExperienceGeneration, error)

	// GenerateExperienceFromTextStreaming is the streaming variant of
	// GenerateExperienceFromText. Returns two read-side channels: fields emits
	// a FieldEvent the moment a watched top-level JSON key in the AI tool
	// output closes, and final receives one terminal value (and is then
	// closed); fields is closed when the parser is done. The synchronous
	// error is reserved for setup-time failures; mid-stream failures arrive
	// as Err on the final value.
	GenerateExperienceFromTextStreaming(ctx context.Context, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error)

	// GenerateRequestContentStreaming is the streaming variant of
	// GenerateRequestContent. Same semantics as
	// GenerateExperienceFromTextStreaming.
	GenerateRequestContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error)

	// GenerateGearFromTextStreaming is the streaming variant of
	// GenerateGearFromText. Same semantics as
	// GenerateExperienceFromTextStreaming.
	GenerateGearFromTextStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error)

	// GenerateExperienceFromImage analyzes an image and creates experience content.
	GenerateExperienceFromImage(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (*ExperienceGeneration, error)

	// GenerateExperienceFromImageStreaming is the streaming variant of
	// GenerateExperienceFromImage. Same channel semantics as
	// GenerateExperienceFromTextStreaming.
	GenerateExperienceFromImageStreaming(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error)

	// GenerateExperienceFromWebpage extracts experience content from webpage text.
	GenerateExperienceFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (*ExperienceGeneration, error)

	// GenerateExperienceFromWebpageStreaming is the streaming variant of
	// GenerateExperienceFromWebpage. Same channel semantics as
	// GenerateExperienceFromTextStreaming.
	GenerateExperienceFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error)

	// GenerateConversationSummary creates a summary of a conversation from message history.
	// requestTitle and requestDescription provide optional context about what the conversation
	// is about. When provided, they help the AI generate more accurate summaries.
	// Pass empty strings if no context is available.
	// style determines whether to generate a progress (third-person) or completion (first-person) summary.
	GenerateConversationSummary(ctx context.Context, messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) (*ConversationSummary, error)

	// GenerateExperienceCompletionSummary creates a summary of how an experience went.
	// experienceTitle and experienceDescription provide context about the experience.
	// attendeeNames lists the people who attended (first names).
	// messages may be empty if no conversation existed; the summary will be based on
	// the experience details and attendee names instead.
	GenerateExperienceCompletionSummary(ctx context.Context, messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) (*ConversationSummary, error)

	// ParseInformalTime converts an informal time description to structured JSON.
	ParseInformalTime(ctx context.Context, informalDescription, currentTime, timezone string) (string, error)

	// GenerateExperienceSuggestions produces category-specific need/contribution chip labels
	// for the experience Plan tab. name and description describe the experience; category is the
	// AI-detected category (e.g., "Fitness", "Cooking"). Returns up to 8 short noun-phrase labels,
	// ordered by relevance. Best-effort: callers should log a warning and continue if this returns an error.
	GenerateExperienceSuggestions(ctx context.Context, name, description, category string) (*ExperienceSuggestionResult, error)

	// GenerateRequestSuggestions produces suggestion chips for a help request's Plan tab.
	// Returns three lists: additional_asks (things that round out the request), breakdown_pieces
	// (sub-tasks that make it more claimable), and offer_ideas (ways helpers can contribute).
	// Best-effort: callers should log a warning and continue if this returns an error.
	GenerateRequestSuggestions(ctx context.Context, title, description, location string) (*RequestSuggestionResult, error)

	// InferSocialAttributes estimates Social Footprint input attributes from transaction context.
	// Used by LLM-assisted enrichment to improve duration and vulnerability estimates
	// beyond config defaults. title and description describe the item, request, or experience.
	// txType is one of "gear_loan", "giveaway", "request", "experience".
	// itemValueUSD is the estimated item value (0 if unknown).
	// Returns nil if inference is unavailable or the description provides insufficient signal.
	InferSocialAttributes(ctx context.Context, title, description, txType string, itemValueUSD float32) (*SocialAttributeInference, error)

	// ClassifyUnifiedCreate classifies a unified-create input into one of
	// three content types (gear, event, request) using the classifier prompt
	// defined in prompts_unified.go. Used by the unified-create service to
	// route incoming streams to the matching per-type generator. Implementations
	// must reject any classification outside the three supported types.
	ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error)

	// CheckHealth verifies the provider is accessible and returns status information.
	// Leaf providers return a single-element slice. Composite providers (fallback,
	// load-balanced) return statuses for all their underlying leaf providers.
	CheckHealth(ctx context.Context) ([]*health.Status, error)
}

// ExperienceSuggestionResult holds LLM-generated suggestion chip labels for an experience.
type ExperienceSuggestionResult struct {
	// Suggestions is the list of up to 8 short noun-phrase labels, ordered by relevance
	// (e.g., "Water", "First aid kit"). Capped by truncateSuggestions at parse time.
	Suggestions []string `json:"suggestions"`

	// CategoryHint is a short, human-readable category label for display
	// (e.g., "outdoor hike", "cooking class"). May be empty.
	CategoryHint string `json:"category_hint"`
}

// RequestSuggestionResult holds LLM-generated suggestion chip labels for a help request Plan tab.
type RequestSuggestionResult struct {
	// AdditionalAsks are things that round out the owner's request (owner chip strip).
	AdditionalAsks []string `json:"additional_asks"`

	// BreakdownPieces are sub-tasks that make the request more claimable (owner chip strip).
	BreakdownPieces []string `json:"breakdown_pieces"`

	// OfferIdeas are ways helpers can contribute (helper chip strip).
	OfferIdeas []string `json:"offer_ideas"`

	// SeedNeeds are the claimable things the request's text plainly names, as
	// short labels ("Lawn mower", "Moving help"), most important first. One
	// entry for a request that names a single thing, several when the text
	// enumerates a list, empty when it names nothing concrete. Each entry seeds
	// one need on the request at creation (#2702, #2731). Capped at 8.
	SeedNeeds []string `json:"seed_needs"`
}

// truncateSuggestions returns the first n elements of s, or s unchanged if len(s) <= n.
// Used to enforce a hard cap on LLM suggestion counts regardless of what the model returns.
func truncateSuggestions(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// SocialAttributeInference holds LLM-inferred Social Footprint attributes for a transaction.
// Non-zero fields replace the corresponding config defaults in SF estimation.
// Zero values indicate the LLM could not confidently infer the attribute.
type SocialAttributeInference struct {
	// DurationMinutes is the estimated face-to-face interaction duration in minutes.
	// Zero means the LLM did not produce a confident estimate; caller uses config default.
	DurationMinutes float32 `json:"duration_minutes"`

	// DurationConfidence is the LLM's self-reported confidence for DurationMinutes (0.0–1.0).
	DurationConfidence float32 `json:"duration_confidence"`

	// DurationReasoning explains how the duration was inferred.
	DurationReasoning string `json:"duration_reasoning"`

	// VulnerabilityLevel is one of "high", "medium", or "low".
	// Empty means the LLM did not produce a confident classification; caller uses config default.
	VulnerabilityLevel string `json:"vulnerability_level"`

	// VulnerabilityReasoning explains how the vulnerability level was classified.
	VulnerabilityReasoning string `json:"vulnerability_reasoning"`
}

// DetectionImage is an image to detect gear in.
// Exactly one of ImageData or ImageURL must be set.
//
// Do not log this struct: ImageURL carries a time-limited presigned URL
// and Filename is user-supplied.
type DetectionImage struct {
	// Raw image bytes (optional - use this OR ImageURL)
	ImageData []byte

	// Presigned URL to the image (optional - use this OR ImageData)
	ImageURL string

	// MIME type of the image (e.g., "image/jpeg", "image/png", "image/heic")
	MimeType string

	// Original upload filename from the media record, when known.
	// Real providers ignore it; the e2e deterministic provider keys its
	// canned per-fixture content on it (#2687).
	Filename string
}

// GearDetection represents a single piece of detected gear in the image.
type GearDetection struct {
	// The name/title of the detected gear
	Title string `json:"title"`

	// Detailed description of the gear
	Description string `json:"description"`

	// Category of the gear (e.g., "Power Tools", "Sports Equipment")
	Category string `json:"category"`

	// Brand/manufacturer name
	Brand string `json:"brand"`

	// Model number/name (if visible)
	Model string `json:"model,omitempty"`

	// Material composition category for carbon estimation (e.g., "mixed_plastic_metal").
	MaterialCategory string `json:"material_category,omitempty"`

	// Estimated weight in grams.
	WeightGrams float32 `json:"weight_grams,omitempty"`

	// Confidence score for this detection (0.0 to 1.0)
	Confidence float32 `json:"confidence"`

	// Value estimate for the detected item
	ValueEstimate *ValueEstimate `json:"value_estimate,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for GearDetection.
// It handles both object and string inputs from the LLM.
func (g *GearDetection) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as an object first.
	type Alias GearDetection
	var alias Alias
	if err := json.Unmarshal(data, &alias); err == nil {
		*g = GearDetection(alias)
		return nil
	}

	// Fallback: if it's a string (e.g., the LLM returned a description instead of structured data),
	// treat it as the title.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*g = GearDetection{Title: s}
		return nil
	}

	return fmt.Errorf("GearDetection: cannot unmarshal %s", string(data))
}

// GearGeneration represents AI-generated gear content from text prompt.
type GearGeneration struct {
	// Generated gear title (e.g., "4-Person Camping Tent")
	Title string `json:"title"`

	// Enhanced/refined description
	Description string `json:"description"`

	// Category of the gear (e.g., "Camping & Outdoors", "Power Tools")
	Category string `json:"category"`

	// Brand/manufacturer name (empty if not mentioned in prompt)
	Brand string `json:"brand"`

	// Material composition category for carbon estimation (e.g., "mixed_plastic_metal").
	MaterialCategory string `json:"material_category,omitempty"`

	// Estimated weight in grams.
	WeightGrams float32 `json:"weight_grams,omitempty"`

	// Location query string extracted from prompt (for server-side geocoding)
	// Can be "USER_PRIMARY_LOCATION" for home references
	LocationQuery string `json:"location_query,omitempty"`

	// Confidence score (0.0 to 1.0)
	Confidence float32 `json:"confidence"`

	// Value estimate for the item (optional, populated when extracting from webpage)
	ValueEstimate *ValueEstimate `json:"value_estimate,omitempty"`

	// Keywords for searching stock images (optional, populated when AI generates content)
	SearchKeywords []string `json:"search_keywords,omitempty"`
}

// ValueEstimate represents an AI-generated estimate of item value.
type ValueEstimate struct {
	// Estimated value in USD (e.g., 50.0 = $50.00).
	EstimatedValueUSD float32 `json:"estimated_value_usd"`

	// Confidence score for the estimate (0.0 to 1.0)
	Confidence float64 `json:"confidence"`

	// Explanation of how the value was determined
	Reasoning string `json:"reasoning"`

	// URLs or references used to determine value
	Sources []string `json:"sources,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ValueEstimate.
// It handles both object and string inputs from the LLM.
func (v *ValueEstimate) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as an object first.
	type Alias ValueEstimate
	var alias Alias
	if err := json.Unmarshal(data, &alias); err == nil {
		*v = ValueEstimate(alias)
		return nil
	}

	// Fallback: if it's a string (e.g., "n/a"), return defaults with the string as reasoning.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*v = ValueEstimate{
			EstimatedValueUSD: 0,
			Confidence:        0.0,
			Reasoning:         s,
			Sources:           []string{},
		}
		return nil
	}

	return fmt.Errorf("ValueEstimate: cannot unmarshal %s", string(data))
}

// CommunityGeneration represents AI-generated community content.
// Community generation produces only image-search keywords describing a
// good background image for the community the user's text describes.
type CommunityGeneration struct {
	// Keywords for image search
	SearchKeywords []string `json:"search_keywords"`
}

// RequestGeneration represents AI-generated request content.
type RequestGeneration struct {
	// Generated request title (max 50 chars)
	Title string `json:"title"`

	// Enhanced/refined description
	Description string `json:"description"`

	// Keywords for image search
	SearchKeywords []string `json:"search_keywords"`

	// Location query string extracted from prompt (for server-side geocoding)
	// Can be "USER_PRIMARY_LOCATION" for home references
	LocationQuery string `json:"location_query,omitempty"`

	// Confidence score (0.0 to 1.0)
	Confidence float32 `json:"confidence"`

	// Estimated cost to hire help for this request
	ValueEstimate *ValueEstimate `json:"value_estimate,omitempty"`
}

// ExperienceGeneration represents AI-generated experience content.
type ExperienceGeneration struct {
	// Generated experience title (max 60 chars)
	Title string `json:"title"`

	// Enhanced/refined description (max 400 chars)
	Description string `json:"description"`

	// Confidence score (0.0 to 1.0)
	Confidence float32 `json:"confidence"`

	// Search keywords for finding relevant images (2-4 terms)
	SearchKeywords []string `json:"search_keywords"`

	// Extracted date in YYYY-MM-DD format (empty if not extracted)
	Date string `json:"date"`

	// Extracted time in HH:MM format (empty if not extracted)
	Time string `json:"time"`

	// Confidence level for time extraction (EXPLICIT, INFERRED, UNKNOWN)
	TimeConfidence string `json:"time_confidence"`

	// Location query extracted from user input (e.g., "North Boulder Park", "my house")
	LocationQuery string `json:"location_query"`

	// Estimated per-person hosting value for this experience
	ValueEstimate *ValueEstimate `json:"value_estimate,omitempty"`

	// Names of people mentioned in the prompt (e.g., "hike with Mike and Bhavna" → ["Mike", "Bhavna"]).
	// Empty when no names are mentioned or for image/URL-based generation.
	MentionedNames []string `json:"mentioned_names,omitempty"`
}

// ConversationMessage represents a single message in a conversation.
type ConversationMessage struct {
	// The sender's name (first name)
	SenderName string

	// The message text content
	Text string

	// Unix timestamp when the message was sent
	SentAtUnixSec int64
}

// ConversationSummary represents an AI-generated summary of a conversation.
type ConversationSummary struct {
	// Concise summary of the conversation (2-3 sentences)
	Summary string `json:"summary"`
}

// SummaryStyle defines the style of summary to generate.
type SummaryStyle int

const (
	// SummaryStyleProgress generates a third-person, objective summary of the conversation.
	// Used during active phases to show what's happening.
	SummaryStyleProgress SummaryStyle = iota

	// SummaryStyleCompletion generates a first-person, solution-focused summary.
	// Used during wrap-up to thank helpers and highlight the solution.
	SummaryStyleCompletion
)

// NormalizeConfidence converts a confidence value to the 0.0-1.0 range.
// AI models sometimes return percentages (0-100) instead of decimals (0-1),
// or entirely out-of-range values. This function normalizes all cases to [0.0, 1.0].
func NormalizeConfidence(c float32) float32 {
	if c > 1.0 && c <= 100.0 {
		return c / 100.0
	}
	if c > 1.0 {
		return 1.0
	}
	if c < 0.0 {
		return 0.0
	}
	return c
}
