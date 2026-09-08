// Shared types and helpers for the streaming variants of the Provider
// interface methods declared in provider.go. Contains the three terminal-
// value types (ExperienceStreamFinal, RequestStreamFinal, GearStreamFinal)
// returned on each streaming method's `final` channel, the watched-key
// lists that drive post-AI fan-out, and a synthetic-stream adapter used by
// provider implementations that don't stream natively (Mock) to satisfy
// the same streaming contract.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"

	"go.ripls.org/ripls/server/logging"
)

// ExperienceStreamFinal terminates an experience streaming call. Exactly one
// of Result or Err is populated. Sent as the single value on the final-result
// channel returned from GenerateExperienceFromTextStreaming.
type ExperienceStreamFinal struct {
	Result *ExperienceGeneration
	Err    error
}

// RequestStreamFinal terminates a request streaming call.
type RequestStreamFinal struct {
	Result *RequestGeneration
	Err    error
}

// GearStreamFinal terminates a gear streaming call.
type GearStreamFinal struct {
	Result *GearGeneration
	Err    error
}

// GearDetectionStreamFinal terminates a DetectGearInImage streaming call.
// Distinct from GearStreamFinal because DetectGearInImage returns
// *GearDetection (image-derived metadata) rather than *GearGeneration
// (text/webpage-derived content).
type GearDetectionStreamFinal struct {
	Result *GearDetection
	Err    error
}

// CommunityStreamFinal terminates a community streaming call.
type CommunityStreamFinal struct {
	Result *CommunityGeneration
	Err    error
}

// StreamFieldKey names a top-level JSON key that Provider streaming methods
// surface via FieldEvent. The string form is the wire name — matches the
// provider's JSON schema exactly — so it round-trips cleanly through
// StreamingJSONFields and json.Unmarshal.
type StreamFieldKey string

const (
	StreamFieldTitle          StreamFieldKey = "title"
	StreamFieldDescription    StreamFieldKey = "description"
	StreamFieldLocationQuery  StreamFieldKey = "location_query"
	StreamFieldSearchKeywords StreamFieldKey = "search_keywords"
	StreamFieldDate           StreamFieldKey = "date"
	StreamFieldTime           StreamFieldKey = "time"
	StreamFieldTimeConfidence StreamFieldKey = "time_confidence"
)

// streamFieldKeysToStrings converts a typed key list to its wire form for
// the StreamingJSONFields parser, which stays string-keyed so it remains a
// general-purpose streaming-JSON utility.
func streamFieldKeysToStrings(keys []StreamFieldKey) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = string(k)
	}
	return out
}

// knownStreamFieldKey returns true if the wire-name string matches one of
// the StreamFieldKey constants. Used by streamingKeysFromSchema to filter
// schema fields down to those that drive post-AI fan-out.
func knownStreamFieldKey(name string) (StreamFieldKey, bool) {
	switch StreamFieldKey(name) {
	case StreamFieldTitle, StreamFieldDescription, StreamFieldLocationQuery,
		StreamFieldSearchKeywords, StreamFieldDate, StreamFieldTime,
		StreamFieldTimeConfidence:
		return StreamFieldKey(name), true
	}
	return "", false
}

// streamingKeysFromSchema reflects on the canonical schema struct T and
// returns the StreamFieldKey constants matching its top-level json tags,
// preserving struct field order. Used at package init time to derive the
// per-method watch lists (experienceStreamingKeys, requestStreamingKeys,
// gearStreamingKeys) so that adding a field to a schema struct in
// schemas.go automatically extends the corresponding watch list with no
// hand-editing — the StreamFieldKey constants and the wire names stay in
// lockstep by construction.
//
// Panics at init time if T has no fields, or if a json tag references a
// known StreamFieldKey constant under a different name (i.e., the tag
// uses one of the reserved key names but the struct field is not
// recognizable). Intentional: the package will fail to load rather than
// silently produce a watch list that doesn't match the wire schema.
func streamingKeysFromSchema[T any]() []StreamFieldKey {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("streamingKeysFromSchema: T must be a struct, got %s", t.Kind()))
	}
	var keys []StreamFieldKey
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if k, ok := knownStreamFieldKey(name); ok {
			keys = append(keys, k)
		}
	}
	return keys
}

// experienceStreamingKeys lists the top-level JSON keys that drive post-AI
// fan-out for experience generation. Derived from the canonical
// experienceFromImageOutput schema (the broadest of the three experience
// schemas — image and webpage have description, all three share title /
// search_keywords / location_query / date / time / time_confidence).
// Mapbox fires on location_query, stock-media fires on search_keywords,
// title is published to the client for the title-visible perceived-
// latency win, the date / time / time_confidence trio drives the mid-
// stream `time` event published to the client once all three have closed,
// and description is watched so the image and webpage handlers can
// publish the AI-generated description mid-stream (text mode emits the
// user prompt as the description and ignores any AI description that
// arrives later).
var experienceStreamingKeys = streamingKeysFromSchema[experienceFromImageOutput]()

// requestStreamingKeys lists the top-level JSON keys that drive post-AI
// fan-out for request generation. Derived from the union of the
// text-mode and image-mode request schemas: image mode contributes
// description (drives the mid-stream description event); text mode and
// image mode share title, search_keywords, and location_query.
var requestStreamingKeys = mergeStreamKeys(
	streamingKeysFromSchema[requestFromTextOutput](),
	streamingKeysFromSchema[requestFromImageOutput](),
)

// gearStreamingKeys lists the top-level JSON keys that drive post-AI fan-out
// for gear generation. Derived from the union of the text-mode and
// webpage-mode gear schemas: text mode contributes location_query (drives
// Mapbox), webpage mode contributes description (drives the mid-stream
// description event) and search_keywords (drives Pexels). Title is shared
// across both. Detection mode wraps in gear_item so its top-level fields
// don't fire mid-stream — no contribution to this list.
var gearStreamingKeys = mergeStreamKeys(
	streamingKeysFromSchema[gearFromTextOutput](),
	streamingKeysFromSchema[gearFromWebpageOutput](),
)

// communityStreamingKeys lists the top-level JSON keys that drive post-AI
// fan-out for community generation. Community generation emits only
// search_keywords, which drives Pexels stock-image fan-out the moment the
// array closes.
var communityStreamingKeys = streamingKeysFromSchema[communityGenerationOutput]()

// mergeStreamKeys returns the union of two StreamFieldKey slices,
// preserving first-seen order.
func mergeStreamKeys(a, b []StreamFieldKey) []StreamFieldKey {
	seen := make(map[StreamFieldKey]struct{}, len(a)+len(b))
	out := make([]StreamFieldKey, 0, len(a)+len(b))
	for _, k := range a {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	for _, k := range b {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

// emitExperienceFromUnary turns an *ExperienceGeneration produced by a unary
// provider call into a synthetic stream of FieldEvents followed by a single
// terminal Final. Used by non-streaming providers (Gemini, OpenAI today,
// MockProvider) so the service layer's streaming logic is uniform across
// providers — every provider hands back the same shape, even if the win
// from early-fire is only realized on real streamers (Anthropic).
func emitExperienceFromUnary(result *ExperienceGeneration, err error, fields chan<- FieldEvent, final chan<- ExperienceStreamFinal) {
	if err != nil {
		final <- ExperienceStreamFinal{Err: err}
		return
	}
	if result == nil {
		final <- ExperienceStreamFinal{}
		return
	}

	emitFieldIfSet(fields, StreamFieldTitle, result.Title)
	// Description must ride the field stream like title: the client
	// reducer fills the preview's text fields only from mid-stream
	// events (the terminal payload's description is ignored), and the
	// Save gate requires a non-empty description — without this event,
	// image mode (which has no prompt to echo) is never saveable (#2687).
	emitFieldIfSet(fields, StreamFieldDescription, result.Description)
	emitFieldIfSet(fields, StreamFieldLocationQuery, result.LocationQuery)
	emitFieldIfSetSlice(fields, StreamFieldSearchKeywords, result.SearchKeywords)
	// Date/time extraction rides the field stream too: the service folds
	// these into the mid-stream `time` event (maybeEmitTime), which is the
	// ONLY path that pre-fills the create preview's WHEN card — the client
	// takes no time from the terminal payload. Without these, every unary
	// provider silently loses extracted times in unified create.
	emitFieldIfSet(fields, StreamFieldDate, result.Date)
	emitFieldIfSet(fields, StreamFieldTime, result.Time)
	emitFieldIfSet(fields, StreamFieldTimeConfidence, result.TimeConfidence)

	final <- ExperienceStreamFinal{Result: result}
}

// emitRequestFromUnary mirrors emitExperienceFromUnary for request generation.
func emitRequestFromUnary(result *RequestGeneration, err error, fields chan<- FieldEvent, final chan<- RequestStreamFinal) {
	if err != nil {
		final <- RequestStreamFinal{Err: err}
		return
	}
	if result == nil {
		final <- RequestStreamFinal{}
		return
	}

	emitFieldIfSet(fields, StreamFieldTitle, result.Title)
	emitFieldIfSet(fields, StreamFieldDescription, result.Description)
	emitFieldIfSetSlice(fields, StreamFieldSearchKeywords, result.SearchKeywords)
	emitFieldIfSet(fields, StreamFieldLocationQuery, result.LocationQuery)

	final <- RequestStreamFinal{Result: result}
}

// emitGearFromUnary mirrors emitExperienceFromUnary for gear generation. Gear
// has no search_keywords field — the title doubles as the stock-image query.
func emitGearFromUnary(result *GearGeneration, err error, fields chan<- FieldEvent, final chan<- GearStreamFinal) {
	if err != nil {
		final <- GearStreamFinal{Err: err}
		return
	}
	if result == nil {
		final <- GearStreamFinal{}
		return
	}

	emitFieldIfSet(fields, StreamFieldTitle, result.Title)
	emitFieldIfSet(fields, StreamFieldDescription, result.Description)
	emitFieldIfSet(fields, StreamFieldLocationQuery, result.LocationQuery)

	final <- GearStreamFinal{Result: result}
}

// emitCommunityFromUnary mirrors emitExperienceFromUnary for community
// generation. Community emits only search_keywords, which drives the
// stock-image fan-out.
func emitCommunityFromUnary(result *CommunityGeneration, err error, fields chan<- FieldEvent, final chan<- CommunityStreamFinal) {
	if err != nil {
		final <- CommunityStreamFinal{Err: err}
		return
	}
	if result == nil {
		final <- CommunityStreamFinal{}
		return
	}

	emitFieldIfSetSlice(fields, StreamFieldSearchKeywords, result.SearchKeywords)

	final <- CommunityStreamFinal{Result: result}
}

// runUnaryExperienceStream adapts a unary result into the streaming
// contract for provider implementations that don't stream natively: call
// the unary method on a goroutine, then emit a synthetic stream with all
// FieldEvents queued up before the terminal value. Same shape as a real
// streaming provider, just without the early-fire win — callers stay
// uniform across providers.
func runUnaryExperienceStream(ctx context.Context, p Provider, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal) {
	fields := make(chan FieldEvent, len(experienceStreamingKeys))
	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryExperienceStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in runUnaryExperienceStream: %v", r)}
			}
		}()
		result, err := p.GenerateExperienceFromText(ctx, prompt, region, currentTime)
		emitExperienceFromUnary(result, err, fields, final)
	}()
	return fields, final
}

// runUnaryRequestStream is the request-generation analogue.
func runUnaryRequestStream(ctx context.Context, p Provider, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal) {
	fields := make(chan FieldEvent, len(requestStreamingKeys))
	final := make(chan RequestStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryRequestStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- RequestStreamFinal{Err: fmt.Errorf("panic in runUnaryRequestStream: %v", r)}
			}
		}()
		result, err := p.GenerateRequestContent(ctx, prompt, region)
		emitRequestFromUnary(result, err, fields, final)
	}()
	return fields, final
}

// runUnaryCommunityStream is the community-generation analogue.
func runUnaryCommunityStream(ctx context.Context, p Provider, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal) {
	fields := make(chan FieldEvent, len(communityStreamingKeys))
	final := make(chan CommunityStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryCommunityStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- CommunityStreamFinal{Err: fmt.Errorf("panic in runUnaryCommunityStream: %v", r)}
			}
		}()
		result, err := p.GenerateCommunityContent(ctx, prompt, region)
		emitCommunityFromUnary(result, err, fields, final)
	}()
	return fields, final
}

// runUnaryRequestFromImageStream is the image-mode analogue of
// runUnaryRequestStream. Used by providers that don't stream natively to
// satisfy the streaming contract uniformly across providers.
func runUnaryRequestFromImageStream(ctx context.Context, p Provider, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal) {
	fields := make(chan FieldEvent, len(requestStreamingKeys))
	final := make(chan RequestStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryRequestFromImageStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- RequestStreamFinal{Err: fmt.Errorf("panic in runUnaryRequestFromImageStream: %v", r)}
			}
		}()
		result, err := p.GenerateRequestFromImage(ctx, image, region)
		emitRequestFromUnary(result, err, fields, final)
	}()
	return fields, final
}

// runUnaryExperienceFromImageStream is the image-mode analogue of
// runUnaryExperienceStream.
func runUnaryExperienceFromImageStream(ctx context.Context, p Provider, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal) {
	fields := make(chan FieldEvent, len(experienceStreamingKeys))
	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryExperienceFromImageStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in runUnaryExperienceFromImageStream: %v", r)}
			}
		}()
		result, err := p.GenerateExperienceFromImage(ctx, image, region, notes, currentTime)
		emitExperienceFromUnary(result, err, fields, final)
	}()
	return fields, final
}

// emitGearDetectionFromUnary turns a *GearDetection produced by a unary
// provider call into a synthetic stream of FieldEvents followed by a
// terminal Final. Used by non-streaming providers (Mock today) to satisfy
// the streaming contract uniformly.
func emitGearDetectionFromUnary(result *GearDetection, err error, fields chan<- FieldEvent, final chan<- GearDetectionStreamFinal) {
	if err != nil {
		final <- GearDetectionStreamFinal{Err: err}
		return
	}
	if result == nil {
		final <- GearDetectionStreamFinal{}
		return
	}

	emitFieldIfSet(fields, StreamFieldTitle, result.Title)
	// See emitExperienceFromUnary: description must be a mid-stream field
	// event or image-mode unified create is never saveable (#2687).
	emitFieldIfSet(fields, StreamFieldDescription, result.Description)

	final <- GearDetectionStreamFinal{Result: result}
}

// runUnaryDetectGearInImageStream is the unary-as-stream adapter for
// DetectGearInImage. Used by providers that don't stream natively.
func runUnaryDetectGearInImageStream(ctx context.Context, p Provider, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal) {
	fields := make(chan FieldEvent, 1)
	final := make(chan GearDetectionStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryDetectGearInImageStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearDetectionStreamFinal{Err: fmt.Errorf("panic in runUnaryDetectGearInImageStream: %v", r)}
			}
		}()
		result, err := p.DetectGearInImage(ctx, req)
		emitGearDetectionFromUnary(result, err, fields, final)
	}()
	return fields, final
}

// runUnaryGearStream is the gear-generation analogue.
func runUnaryGearStream(ctx context.Context, p Provider, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal) {
	fields := make(chan FieldEvent, len(gearStreamingKeys))
	final := make(chan GearStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryGearStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearStreamFinal{Err: fmt.Errorf("panic in runUnaryGearStream: %v", r)}
			}
		}()
		result, err := p.GenerateGearFromText(ctx, prompt, region)
		emitGearFromUnary(result, err, fields, final)
	}()
	return fields, final
}

// runUnaryExperienceFromWebpageStream is the webpage-mode analogue of
// runUnaryExperienceStream.
func runUnaryExperienceFromWebpageStream(ctx context.Context, p Provider, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal) {
	fields := make(chan FieldEvent, len(experienceStreamingKeys))
	final := make(chan ExperienceStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryExperienceFromWebpageStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in runUnaryExperienceFromWebpageStream: %v", r)}
			}
		}()
		result, err := p.GenerateExperienceFromWebpage(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
		emitExperienceFromUnary(result, err, fields, final)
	}()
	return fields, final
}

// runUnaryGearFromWebpageStream is the webpage-mode gear analogue of
// runUnaryGearStream.
func runUnaryGearFromWebpageStream(ctx context.Context, p Provider, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal) {
	fields := make(chan FieldEvent, len(gearStreamingKeys))
	final := make(chan GearStreamFinal, 1)
	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "runUnaryGearFromWebpageStream",
					"panic", r,
					"stack", string(debug.Stack()),
				)
				final <- GearStreamFinal{Err: fmt.Errorf("panic in runUnaryGearFromWebpageStream: %v", r)}
			}
		}()
		result, err := p.GenerateGearFromWebpage(ctx, pageTitle, pageDescription, pageBody, region)
		emitGearFromUnary(result, err, fields, final)
	}()
	return fields, final
}

func emitFieldIfSet(fields chan<- FieldEvent, key StreamFieldKey, value string) {
	if value == "" {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		// json.Marshal of a string never fails — defensive only.
		return
	}
	fields <- FieldEvent{Key: key, Value: raw}
}

func emitFieldIfSetSlice(fields chan<- FieldEvent, key StreamFieldKey, value []string) {
	if len(value) == 0 {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	fields <- FieldEvent{Key: key, Value: raw}
}
