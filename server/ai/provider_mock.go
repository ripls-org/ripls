package ai

import (
	"context"
	"fmt"
	"sync"

	"go.ripls.org/ripls/server/health"
)

// MockProvider is a mock AI provider for testing.
//
// Usage:
//
//	mock := ai.NewMockProvider()                       // sensible defaults for all methods
//	mock.GenerateGearFromTextFunc = func(...) { ... }  // override specific behavior
//	len(mock.Calls.GenerateGearFromText)               // check call count
//	mock.Calls.GenerateGearFromText[0].Prompt          // inspect captured arguments
//	mock.TotalCalls()                                  // total calls across all methods
//	mock.Reset()                                       // zero out all call records
//
// To create a mock with a custom Name() return value (e.g., for load-balancer tests):
//
//	mock := ai.NewMockProvider()
//	mock.NameValue = "provider-1"
//
// Concurrency: provider methods are safe to invoke from multiple goroutines
// concurrently — call recording is serialized by an internal mutex. Reading
// [MockProvider.Calls] fields directly (as the examples above do) is only
// safe once all concurrent invocations have returned; for a lock-protected
// snapshot mid-flight, use [MockProvider.Snapshot].
type MockProvider struct {
	// mu guards Calls. It serializes both the internal appends performed by
	// each recording method and the external snapshot accessor. All fields
	// besides Calls (NameValue, the *Func overrides) are expected to be set
	// before concurrent use begins and are not guarded.
	mu sync.Mutex

	// NameValue controls what Name() returns. Defaults to "mock".
	NameValue string

	// Calls records every call made to each method, capturing arguments.
	Calls MockProviderCalls

	DetectGearFunc                             func(ctx context.Context, req *DetectionImage) (*GearDetection, error)
	DetectGearInImageStreamingFunc             func(ctx context.Context, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal, error)
	GenerateGearFromTextFunc                   func(ctx context.Context, prompt, region string) (*GearGeneration, error)
	GenerateGearFromWebpageFunc                func(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (*GearGeneration, error)
	GenerateGearFromWebpageStreamingFunc       func(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error)
	GenerateCommunityFunc                      func(ctx context.Context, prompt, region string) (*CommunityGeneration, error)
	GenerateCommunityContentStreamingFunc      func(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal, error)
	GenerateRequestFunc                        func(ctx context.Context, prompt, region string) (*RequestGeneration, error)
	GenerateExperienceFromTextFunc             func(ctx context.Context, prompt, region, currentTime string) (*ExperienceGeneration, error)
	GenerateExperienceFromTextStreamingFunc    func(ctx context.Context, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error)
	GenerateRequestContentStreamingFunc        func(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error)
	GenerateGearFromTextStreamingFunc          func(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error)
	GenerateExperienceFromImageFunc            func(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (*ExperienceGeneration, error)
	GenerateExperienceFromImageStreamingFunc   func(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error)
	GenerateExperienceFromWebpageFunc          func(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (*ExperienceGeneration, error)
	GenerateExperienceFromWebpageStreamingFunc func(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error)
	GenerateRequestFromImageFunc               func(ctx context.Context, image *DetectionImage, region string) (*RequestGeneration, error)
	GenerateRequestFromImageStreamingFunc      func(ctx context.Context, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error)
	GenerateConversationSummaryFunc            func(ctx context.Context, messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) (*ConversationSummary, error)
	GenerateExperienceCompletionSummaryFunc    func(ctx context.Context, messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) (*ConversationSummary, error)
	ParseInformalTimeFunc                      func(ctx context.Context, informalDescription, currentTime, timezone string) (string, error)
	GenerateExperienceSuggestionsFunc          func(ctx context.Context, name, description, category string) (*ExperienceSuggestionResult, error)
	GenerateRequestSuggestionsFunc             func(ctx context.Context, title, description, location string) (*RequestSuggestionResult, error)
	InferSocialAttributesFunc                  func(ctx context.Context, title, description, txType string, itemValueUSD float32) (*SocialAttributeInference, error)
	ClassifyUnifiedCreateFunc                  func(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error)
	CheckHealthFunc                            func(ctx context.Context) ([]*health.Status, error)
}

// MockProviderCalls records all calls made to a MockProvider, with captured arguments.
type MockProviderCalls struct {
	DetectGearInImage                   []DetectGearInImageCall
	GenerateGearFromText                []GenerateGearFromTextCall
	GenerateGearFromWebpage             []GenerateGearFromWebpageCall
	GenerateCommunityContent            []GenerateCommunityContentCall
	GenerateRequestContent              []GenerateRequestContentCall
	GenerateExperienceFromText          []GenerateExperienceFromTextCall
	GenerateExperienceFromImage         []GenerateExperienceFromImageCall
	GenerateExperienceFromWebpage       []GenerateExperienceFromWebpageCall
	GenerateRequestFromImage            []GenerateRequestFromImageCall
	GenerateConversationSummary         []GenerateConversationSummaryCall
	GenerateExperienceCompletionSummary []GenerateExperienceCompletionSummaryCall
	ParseInformalTime                   []ParseInformalTimeCall
	GenerateExperienceSuggestions       []GenerateExperienceSuggestionsCall
	GenerateRequestSuggestions          []GenerateRequestSuggestionsCall
	InferSocialAttributes               []InferSocialAttributesCall
	ClassifyUnifiedCreate               []ClassifyUnifiedCreateCall
	CheckHealth                         []CheckHealthCall
}

// Argument capture types for each Provider method.

// DetectGearInImageCall captures arguments passed to DetectGearInImage.
type DetectGearInImageCall struct {
	Req *DetectionImage
}

// GenerateGearFromTextCall captures arguments passed to GenerateGearFromText.
type GenerateGearFromTextCall struct {
	Prompt string
	Region string
}

// GenerateGearFromWebpageCall captures arguments passed to GenerateGearFromWebpage.
type GenerateGearFromWebpageCall struct {
	PageTitle       string
	PageDescription string
	PageBody        string
	Region          string
}

// GenerateCommunityContentCall captures arguments passed to GenerateCommunityContent.
type GenerateCommunityContentCall struct {
	Prompt string
	Region string
}

// GenerateRequestContentCall captures arguments passed to GenerateRequestContent.
type GenerateRequestContentCall struct {
	Prompt string
	Region string
}

// GenerateExperienceFromTextCall captures arguments passed to GenerateExperienceFromText.
type GenerateExperienceFromTextCall struct {
	Prompt      string
	Region      string
	CurrentTime string
}

// GenerateExperienceFromImageCall captures arguments passed to GenerateExperienceFromImage.
type GenerateExperienceFromImageCall struct {
	Image       *DetectionImage
	Region      string
	Notes       string
	CurrentTime string
}

// GenerateExperienceFromWebpageCall captures arguments passed to GenerateExperienceFromWebpage.
type GenerateExperienceFromWebpageCall struct {
	PageTitle       string
	PageDescription string
	PageBody        string
	Region          string
	CurrentTime     string
}

// GenerateRequestFromImageCall captures arguments passed to GenerateRequestFromImage.
type GenerateRequestFromImageCall struct {
	Image  *DetectionImage
	Region string
}

// GenerateConversationSummaryCall captures arguments passed to GenerateConversationSummary.
type GenerateConversationSummaryCall struct {
	Messages           []ConversationMessage
	RequestTitle       string
	RequestDescription string
	Style              SummaryStyle
}

// GenerateExperienceCompletionSummaryCall captures arguments passed to GenerateExperienceCompletionSummary.
type GenerateExperienceCompletionSummaryCall struct {
	Messages              []ConversationMessage
	ExperienceTitle       string
	ExperienceDescription string
	AttendeeNames         []string
}

// ParseInformalTimeCall captures arguments passed to ParseInformalTime.
type ParseInformalTimeCall struct {
	InformalDescription string
	CurrentTime         string
	Timezone            string
}

// GenerateExperienceSuggestionsCall captures arguments passed to GenerateExperienceSuggestions.
type GenerateExperienceSuggestionsCall struct {
	Name        string
	Description string
	Category    string
}

// GenerateRequestSuggestionsCall captures arguments passed to GenerateRequestSuggestions.
type GenerateRequestSuggestionsCall struct {
	Title       string
	Description string
	Location    string
}

// InferSocialAttributesCall captures arguments passed to InferSocialAttributes.
type InferSocialAttributesCall struct {
	Title        string
	Description  string
	TxType       string
	ItemValueUSD float32
}

// ClassifyUnifiedCreateCall captures arguments passed to ClassifyUnifiedCreate.
type ClassifyUnifiedCreateCall struct {
	Input UnifiedCreateClassifierInput
}

// CheckHealthCall captures arguments passed to CheckHealth (none beyond context).
type CheckHealthCall struct{}

// TotalCalls returns the total number of calls across all methods.
func (c *MockProviderCalls) TotalCalls() int {
	return len(c.DetectGearInImage) +
		len(c.GenerateGearFromText) +
		len(c.GenerateGearFromWebpage) +
		len(c.GenerateCommunityContent) +
		len(c.GenerateRequestContent) +
		len(c.GenerateExperienceFromText) +
		len(c.GenerateExperienceFromImage) +
		len(c.GenerateExperienceFromWebpage) +
		len(c.GenerateRequestFromImage) +
		len(c.GenerateConversationSummary) +
		len(c.GenerateExperienceCompletionSummary) +
		len(c.ParseInformalTime) +
		len(c.GenerateExperienceSuggestions) +
		len(c.InferSocialAttributes) +
		len(c.CheckHealth)
}

// Reset zeroes out all call records.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = MockProviderCalls{}
}

// Snapshot returns a copy of the recorded calls taken under the mock's
// internal lock. Use this to inspect calls while other goroutines may
// still be invoking the mock — direct reads of [MockProvider.Calls] race
// with the recording appends in that case. The returned struct's slice
// fields are freshly allocated so callers may retain them freely.
func (m *MockProvider) Snapshot() MockProviderCalls {
	m.mu.Lock()
	defer m.mu.Unlock()
	return MockProviderCalls{
		DetectGearInImage:                   append([]DetectGearInImageCall(nil), m.Calls.DetectGearInImage...),
		GenerateGearFromText:                append([]GenerateGearFromTextCall(nil), m.Calls.GenerateGearFromText...),
		GenerateGearFromWebpage:             append([]GenerateGearFromWebpageCall(nil), m.Calls.GenerateGearFromWebpage...),
		GenerateCommunityContent:            append([]GenerateCommunityContentCall(nil), m.Calls.GenerateCommunityContent...),
		GenerateRequestContent:              append([]GenerateRequestContentCall(nil), m.Calls.GenerateRequestContent...),
		GenerateExperienceFromText:          append([]GenerateExperienceFromTextCall(nil), m.Calls.GenerateExperienceFromText...),
		GenerateExperienceFromImage:         append([]GenerateExperienceFromImageCall(nil), m.Calls.GenerateExperienceFromImage...),
		GenerateExperienceFromWebpage:       append([]GenerateExperienceFromWebpageCall(nil), m.Calls.GenerateExperienceFromWebpage...),
		GenerateRequestFromImage:            append([]GenerateRequestFromImageCall(nil), m.Calls.GenerateRequestFromImage...),
		GenerateConversationSummary:         append([]GenerateConversationSummaryCall(nil), m.Calls.GenerateConversationSummary...),
		GenerateExperienceCompletionSummary: append([]GenerateExperienceCompletionSummaryCall(nil), m.Calls.GenerateExperienceCompletionSummary...),
		ParseInformalTime:                   append([]ParseInformalTimeCall(nil), m.Calls.ParseInformalTime...),
		GenerateExperienceSuggestions:       append([]GenerateExperienceSuggestionsCall(nil), m.Calls.GenerateExperienceSuggestions...),
		GenerateRequestSuggestions:          append([]GenerateRequestSuggestionsCall(nil), m.Calls.GenerateRequestSuggestions...),
		InferSocialAttributes:               append([]InferSocialAttributesCall(nil), m.Calls.InferSocialAttributes...),
		ClassifyUnifiedCreate:               append([]ClassifyUnifiedCreateCall(nil), m.Calls.ClassifyUnifiedCreate...),
		CheckHealth:                         append([]CheckHealthCall(nil), m.Calls.CheckHealth...),
	}
}

// NewMockProvider creates a new mock provider with default behavior.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		NameValue:             "mock",
		DetectGearFunc:        defaultDetectGear,
		GenerateCommunityFunc: defaultGenerateCommunity,
		GenerateRequestFunc:   defaultGenerateRequest,
	}
}

// Name returns the provider's name for logging.
func (m *MockProvider) Name() string {
	if m.NameValue != "" {
		return m.NameValue
	}
	return "mock"
}

// DetectGearInImage implements the Provider interface.
func (m *MockProvider) DetectGearInImage(ctx context.Context, req *DetectionImage) (*GearDetection, error) {
	m.mu.Lock()
	m.Calls.DetectGearInImage = append(m.Calls.DetectGearInImage, DetectGearInImageCall{Req: req})
	m.mu.Unlock()
	if m.DetectGearFunc == nil {
		return defaultDetectGear(ctx, req)
	}
	return m.DetectGearFunc(ctx, req)
}

// DetectGearInImageStreaming implements the Provider interface. Defaults
// to a unary wrapper around DetectGearInImage. Tests that need scripted
// event timing override DetectGearInImageStreamingFunc.
func (m *MockProvider) DetectGearInImageStreaming(ctx context.Context, req *DetectionImage) (<-chan FieldEvent, <-chan GearDetectionStreamFinal, error) {
	if m.DetectGearInImageStreamingFunc != nil {
		return m.DetectGearInImageStreamingFunc(ctx, req)
	}
	fields, final := runUnaryDetectGearInImageStream(ctx, m, req)
	return fields, final, nil
}

// GenerateGearFromText implements the Provider interface.
func (m *MockProvider) GenerateGearFromText(ctx context.Context, prompt, region string) (*GearGeneration, error) {
	m.mu.Lock()
	m.Calls.GenerateGearFromText = append(m.Calls.GenerateGearFromText, GenerateGearFromTextCall{Prompt: prompt, Region: region})
	m.mu.Unlock()
	if m.GenerateGearFromTextFunc == nil {
		return defaultGenerateGearFromText(ctx, prompt, region)
	}
	return m.GenerateGearFromTextFunc(ctx, prompt, region)
}

// GenerateGearFromWebpage implements the Provider interface.
func (m *MockProvider) GenerateGearFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (*GearGeneration, error) {
	m.mu.Lock()
	m.Calls.GenerateGearFromWebpage = append(m.Calls.GenerateGearFromWebpage, GenerateGearFromWebpageCall{
		PageTitle: pageTitle, PageDescription: pageDescription, PageBody: pageBody, Region: region,
	})
	m.mu.Unlock()
	if m.GenerateGearFromWebpageFunc == nil {
		return defaultGenerateGearFromWebpage(ctx, pageTitle, pageDescription, pageBody, region)
	}
	return m.GenerateGearFromWebpageFunc(ctx, pageTitle, pageDescription, pageBody, region)
}

// GenerateCommunityContent implements the Provider interface.
func (m *MockProvider) GenerateCommunityContent(ctx context.Context, prompt, region string) (*CommunityGeneration, error) {
	m.mu.Lock()
	m.Calls.GenerateCommunityContent = append(m.Calls.GenerateCommunityContent, GenerateCommunityContentCall{Prompt: prompt, Region: region})
	m.mu.Unlock()
	if m.GenerateCommunityFunc == nil {
		return defaultGenerateCommunity(ctx, prompt, region)
	}
	return m.GenerateCommunityFunc(ctx, prompt, region)
}

// GenerateRequestContent implements the Provider interface.
func (m *MockProvider) GenerateRequestContent(ctx context.Context, prompt, region string) (*RequestGeneration, error) {
	m.mu.Lock()
	m.Calls.GenerateRequestContent = append(m.Calls.GenerateRequestContent, GenerateRequestContentCall{Prompt: prompt, Region: region})
	m.mu.Unlock()
	if m.GenerateRequestFunc == nil {
		return defaultGenerateRequest(ctx, prompt, region)
	}
	return m.GenerateRequestFunc(ctx, prompt, region)
}

// GenerateExperienceFromText implements the Provider interface.
func (m *MockProvider) GenerateExperienceFromText(ctx context.Context, prompt, region, currentTime string) (*ExperienceGeneration, error) {
	m.mu.Lock()
	m.Calls.GenerateExperienceFromText = append(m.Calls.GenerateExperienceFromText, GenerateExperienceFromTextCall{
		Prompt: prompt, Region: region, CurrentTime: currentTime,
	})
	m.mu.Unlock()
	if m.GenerateExperienceFromTextFunc == nil {
		return defaultGenerateExperienceFromText(ctx, prompt, region, currentTime)
	}
	return m.GenerateExperienceFromTextFunc(ctx, prompt, region, currentTime)
}

// GenerateExperienceFromTextStreaming implements the Provider interface.
// Defaults to a unary wrapper around GenerateExperienceFromText. Tests that
// need scripted event timing override GenerateExperienceFromTextStreamingFunc.
func (m *MockProvider) GenerateExperienceFromTextStreaming(ctx context.Context, prompt, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if m.GenerateExperienceFromTextStreamingFunc != nil {
		return m.GenerateExperienceFromTextStreamingFunc(ctx, prompt, region, currentTime)
	}
	fields, final := runUnaryExperienceStream(ctx, m, prompt, region, currentTime)
	return fields, final, nil
}

// GenerateRequestContentStreaming implements the Provider interface.
func (m *MockProvider) GenerateRequestContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	if m.GenerateRequestContentStreamingFunc != nil {
		return m.GenerateRequestContentStreamingFunc(ctx, prompt, region)
	}
	fields, final := runUnaryRequestStream(ctx, m, prompt, region)
	return fields, final, nil
}

// GenerateCommunityContentStreaming implements the Provider interface.
func (m *MockProvider) GenerateCommunityContentStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan CommunityStreamFinal, error) {
	if m.GenerateCommunityContentStreamingFunc != nil {
		return m.GenerateCommunityContentStreamingFunc(ctx, prompt, region)
	}
	fields, final := runUnaryCommunityStream(ctx, m, prompt, region)
	return fields, final, nil
}

// GenerateGearFromTextStreaming implements the Provider interface.
func (m *MockProvider) GenerateGearFromTextStreaming(ctx context.Context, prompt, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	if m.GenerateGearFromTextStreamingFunc != nil {
		return m.GenerateGearFromTextStreamingFunc(ctx, prompt, region)
	}
	fields, final := runUnaryGearStream(ctx, m, prompt, region)
	return fields, final, nil
}

// GenerateExperienceFromImage implements the Provider interface.
func (m *MockProvider) GenerateExperienceFromImage(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (*ExperienceGeneration, error) {
	m.mu.Lock()
	m.Calls.GenerateExperienceFromImage = append(m.Calls.GenerateExperienceFromImage, GenerateExperienceFromImageCall{
		Image: image, Region: region, Notes: notes, CurrentTime: currentTime,
	})
	m.mu.Unlock()
	if m.GenerateExperienceFromImageFunc == nil {
		return defaultGenerateExperienceFromImage(ctx, image, region, notes, currentTime)
	}
	return m.GenerateExperienceFromImageFunc(ctx, image, region, notes, currentTime)
}

// GenerateExperienceFromImageStreaming implements the Provider interface.
// Defaults to a unary wrapper around GenerateExperienceFromImage. Tests
// that need scripted event timing override
// GenerateExperienceFromImageStreamingFunc.
func (m *MockProvider) GenerateExperienceFromImageStreaming(ctx context.Context, image *DetectionImage, region, notes, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if m.GenerateExperienceFromImageStreamingFunc != nil {
		return m.GenerateExperienceFromImageStreamingFunc(ctx, image, region, notes, currentTime)
	}
	fields, final := runUnaryExperienceFromImageStream(ctx, m, image, region, notes, currentTime)
	return fields, final, nil
}

// GenerateExperienceFromWebpage implements the Provider interface.
func (m *MockProvider) GenerateExperienceFromWebpage(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (*ExperienceGeneration, error) {
	m.mu.Lock()
	m.Calls.GenerateExperienceFromWebpage = append(m.Calls.GenerateExperienceFromWebpage, GenerateExperienceFromWebpageCall{
		PageTitle: pageTitle, PageDescription: pageDescription, PageBody: pageBody, Region: region, CurrentTime: currentTime,
	})
	m.mu.Unlock()
	if m.GenerateExperienceFromWebpageFunc == nil {
		return defaultGenerateExperienceFromWebpage(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
	}
	return m.GenerateExperienceFromWebpageFunc(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
}

// GenerateRequestFromImage implements the Provider interface.
func (m *MockProvider) GenerateRequestFromImage(ctx context.Context, image *DetectionImage, region string) (*RequestGeneration, error) {
	m.mu.Lock()
	m.Calls.GenerateRequestFromImage = append(m.Calls.GenerateRequestFromImage, GenerateRequestFromImageCall{Image: image, Region: region})
	m.mu.Unlock()
	if m.GenerateRequestFromImageFunc == nil {
		return defaultGenerateRequestFromImage(ctx, image, region)
	}
	return m.GenerateRequestFromImageFunc(ctx, image, region)
}

// GenerateRequestFromImageStreaming implements the Provider interface.
// Defaults to a unary wrapper around GenerateRequestFromImage. Tests that
// need scripted event timing override GenerateRequestFromImageStreamingFunc.
func (m *MockProvider) GenerateRequestFromImageStreaming(ctx context.Context, image *DetectionImage, region string) (<-chan FieldEvent, <-chan RequestStreamFinal, error) {
	if m.GenerateRequestFromImageStreamingFunc != nil {
		return m.GenerateRequestFromImageStreamingFunc(ctx, image, region)
	}
	fields, final := runUnaryRequestFromImageStream(ctx, m, image, region)
	return fields, final, nil
}

// GenerateExperienceFromWebpageStreaming implements the Provider interface.
// Defaults to a unary wrapper around GenerateExperienceFromWebpage. Tests
// that need scripted event timing override
// GenerateExperienceFromWebpageStreamingFunc.
func (m *MockProvider) GenerateExperienceFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region, currentTime string) (<-chan FieldEvent, <-chan ExperienceStreamFinal, error) {
	if m.GenerateExperienceFromWebpageStreamingFunc != nil {
		return m.GenerateExperienceFromWebpageStreamingFunc(ctx, pageTitle, pageDescription, pageBody, region, currentTime)
	}
	fields, final := runUnaryExperienceFromWebpageStream(ctx, m, pageTitle, pageDescription, pageBody, region, currentTime)
	return fields, final, nil
}

// GenerateGearFromWebpageStreaming implements the Provider interface.
// Defaults to a unary wrapper around GenerateGearFromWebpage. Tests that
// need scripted event timing override GenerateGearFromWebpageStreamingFunc.
func (m *MockProvider) GenerateGearFromWebpageStreaming(ctx context.Context, pageTitle, pageDescription, pageBody, region string) (<-chan FieldEvent, <-chan GearStreamFinal, error) {
	if m.GenerateGearFromWebpageStreamingFunc != nil {
		return m.GenerateGearFromWebpageStreamingFunc(ctx, pageTitle, pageDescription, pageBody, region)
	}
	fields, final := runUnaryGearFromWebpageStream(ctx, m, pageTitle, pageDescription, pageBody, region)
	return fields, final, nil
}

// GenerateConversationSummary implements the Provider interface.
func (m *MockProvider) GenerateConversationSummary(ctx context.Context, messages []ConversationMessage, requestTitle, requestDescription string, style SummaryStyle) (*ConversationSummary, error) {
	m.mu.Lock()
	m.Calls.GenerateConversationSummary = append(m.Calls.GenerateConversationSummary, GenerateConversationSummaryCall{
		Messages: messages, RequestTitle: requestTitle, RequestDescription: requestDescription, Style: style,
	})
	m.mu.Unlock()
	if m.GenerateConversationSummaryFunc == nil {
		return defaultGenerateConversationSummary(ctx, messages, requestTitle, requestDescription, style)
	}
	return m.GenerateConversationSummaryFunc(ctx, messages, requestTitle, requestDescription, style)
}

// GenerateExperienceCompletionSummary implements the Provider interface.
func (m *MockProvider) GenerateExperienceCompletionSummary(ctx context.Context, messages []ConversationMessage, experienceTitle, experienceDescription string, attendeeNames []string) (*ConversationSummary, error) {
	m.mu.Lock()
	m.Calls.GenerateExperienceCompletionSummary = append(m.Calls.GenerateExperienceCompletionSummary, GenerateExperienceCompletionSummaryCall{
		Messages: messages, ExperienceTitle: experienceTitle, ExperienceDescription: experienceDescription, AttendeeNames: attendeeNames,
	})
	m.mu.Unlock()
	if m.GenerateExperienceCompletionSummaryFunc == nil {
		return defaultGenerateExperienceCompletionSummary(ctx, messages, experienceTitle, experienceDescription, attendeeNames)
	}
	return m.GenerateExperienceCompletionSummaryFunc(ctx, messages, experienceTitle, experienceDescription, attendeeNames)
}

// defaultDetectGear is the default mock implementation for gear detection.
func defaultDetectGear(_ context.Context, _ *DetectionImage) (*GearDetection, error) {
	return &GearDetection{
		Title:            "Mock Gear Item",
		Description:      "A mock gear item for testing",
		Category:         "Test Category",
		Brand:            "Test Brand",
		Model:            "Mock Model",
		MaterialCategory: "mixed_plastic_metal",
		WeightGrams:      2000,
		Confidence:       0.95,
		ValueEstimate: &ValueEstimate{
			EstimatedValueUSD: 50.0,
			Confidence:        0.7,
			Reasoning:         "Mock value estimate from image detection",
			Sources:           []string{"AI estimation"},
		},
	}, nil
}

// defaultGenerateGearFromText is the default mock implementation for gear generation from text.
func defaultGenerateGearFromText(_ context.Context, prompt, region string) (*GearGeneration, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	title := "Mock Gear"
	if region != "" {
		title = fmt.Sprintf("Mock Gear - %s", region)
	}

	return &GearGeneration{
		Title:            title,
		Category:         "Test Category",
		Brand:            "",
		MaterialCategory: "mixed_plastic_metal",
		WeightGrams:      1500,
		LocationQuery:    "",
		Confidence:       0.85,
		ValueEstimate: &ValueEstimate{
			EstimatedValueUSD: 75.0,
			Confidence:        0.6,
			Reasoning:         "Mock value estimate from text generation",
			Sources:           []string{"AI estimation"},
		},
	}, nil
}

// defaultGenerateGearFromWebpage is the default mock implementation for gear generation from webpage.
func defaultGenerateGearFromWebpage(_ context.Context, pageTitle, pageDescription, _, _ string) (*GearGeneration, error) {
	title := "Mock Product"
	if pageTitle != "" {
		title = pageTitle
	}

	description := "A mock product generated from webpage content"
	if pageDescription != "" {
		description = pageDescription
	}

	return &GearGeneration{
		Title:            title,
		Description:      description,
		Category:         "Test Category",
		Brand:            "Test Brand",
		MaterialCategory: "solid_plastic",
		WeightGrams:      1000,
		LocationQuery:    "",
		Confidence:       0.90,
		SearchKeywords:   []string{"mock", "product", "test"},
		ValueEstimate: &ValueEstimate{
			EstimatedValueUSD: 99.99,
			Confidence:        0.85,
			Reasoning:         "Mock value estimate for testing",
			Sources:           []string{"Mock product page"},
		},
	}, nil
}

// defaultGenerateCommunity is the default mock implementation for community generation.
func defaultGenerateCommunity(_ context.Context, prompt, _ string) (*CommunityGeneration, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	return &CommunityGeneration{
		SearchKeywords: []string{"mock", "test", "community"},
	}, nil
}

// defaultGenerateRequest is the default mock implementation for request generation.
func defaultGenerateRequest(_ context.Context, prompt, region string) (*RequestGeneration, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	title := "Mock Request"
	if region != "" {
		title = fmt.Sprintf("Mock Request - %s", region)
	}

	return &RequestGeneration{
		Title:          title,
		SearchKeywords: []string{"mock", "test", "request"},
		Confidence:     0.85,
		ValueEstimate: &ValueEstimate{
			EstimatedValueUSD: 50.0, // $50
			Confidence:        0.75,
			Reasoning:         "Mock value estimate for testing purposes",
			Sources:           []string{"mock data"},
		},
	}, nil
}

// defaultGenerateRequestFromImage is the default mock implementation for request generation from image.
func defaultGenerateRequestFromImage(_ context.Context, _ *DetectionImage, region string) (*RequestGeneration, error) {
	title := "Mock Request from Image"
	if region != "" {
		title = fmt.Sprintf("Mock Request - %s", region)
	}

	return &RequestGeneration{
		Title:          title,
		Description:    "A mock request generated from an image for testing purposes",
		SearchKeywords: []string{"mock", "test", "image"},
		Confidence:     0.80,
		ValueEstimate: &ValueEstimate{
			EstimatedValueUSD: 75.0,
			Confidence:        0.70,
			Reasoning:         "Mock value estimate from image analysis for testing",
			Sources:           []string{"mock image data"},
		},
	}, nil
}

// defaultGenerateExperienceFromText is the default mock implementation for experience generation from text.
func defaultGenerateExperienceFromText(_ context.Context, prompt, region, _ string) (*ExperienceGeneration, error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	title := "Mock Experience"
	if region != "" {
		title = fmt.Sprintf("Mock Experience - %s", region)
	}

	return &ExperienceGeneration{
		Title:          title,
		Confidence:     0.85,
		Date:           "2025-12-01",
		Time:           "14:00",
		TimeConfidence: "INFERRED",
		LocationQuery:  "",
		ValueEstimate: &ValueEstimate{
			EstimatedValueUSD: 35.0,
			Confidence:        0.75,
			Reasoning:         "Mock per-person value estimate for testing purposes",
			Sources:           []string{"mock data"},
		},
	}, nil
}

// defaultGenerateExperienceFromImage is the default mock implementation for experience generation from image.
func defaultGenerateExperienceFromImage(_ context.Context, _ *DetectionImage, region, notes, _ string) (*ExperienceGeneration, error) {
	title := "Mock Experience from Image"
	if region != "" {
		title = fmt.Sprintf("Mock Experience - %s", region)
	}

	description := "A mock experience generated from image analysis"
	if notes != "" {
		description = fmt.Sprintf("%s with notes: %s", description, notes)
	}

	return &ExperienceGeneration{
		Title:          title,
		Description:    description,
		Confidence:     0.80,
		Date:           "2025-12-01",
		Time:           "18:00",
		TimeConfidence: "EXPLICIT",
		LocationQuery:  "",
		ValueEstimate: &ValueEstimate{
			EstimatedValueUSD: 40.0,
			Confidence:        0.70,
			Reasoning:         "Mock per-person value estimate from image for testing",
			Sources:           []string{"mock image data"},
		},
	}, nil
}

// defaultGenerateExperienceFromWebpage is the default mock implementation for webpage-based generation.
func defaultGenerateExperienceFromWebpage(_ context.Context, pageTitle, pageDescription, _, _, _ string) (*ExperienceGeneration, error) {
	title := "Mock Event from Webpage"
	if pageTitle != "" {
		title = pageTitle
	}

	description := "A mock experience generated from webpage content"
	if pageDescription != "" {
		description = pageDescription
	}

	return &ExperienceGeneration{
		Title:          title,
		Description:    description,
		Confidence:     0.85,
		Date:           "2025-12-15",
		Time:           "19:00",
		TimeConfidence: "EXPLICIT",
		LocationQuery:  "Downtown Convention Center",
		ValueEstimate: &ValueEstimate{
			EstimatedValueUSD: 60.0,
			Confidence:        0.80,
			Reasoning:         "Mock per-person value estimate from webpage for testing",
			Sources:           []string{"mock webpage data"},
		},
	}, nil
}

// defaultGenerateConversationSummary is the default mock implementation for conversation summary.
func defaultGenerateConversationSummary(_ context.Context, messages []ConversationMessage, _, _ string, style SummaryStyle) (*ConversationSummary, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages to summarize")
	}

	if style == SummaryStyleCompletion {
		return &ConversationSummary{
			Summary: "Thank you everyone for your help! This is a mock completion summary.",
		}, nil
	}

	return &ConversationSummary{
		Summary: "This is a mock progress summary for testing purposes.",
	}, nil
}

// defaultGenerateExperienceCompletionSummary is the default mock implementation for experience completion summary.
func defaultGenerateExperienceCompletionSummary(_ context.Context, _ []ConversationMessage, _, _ string, _ []string) (*ConversationSummary, error) {
	return &ConversationSummary{
		Summary: "Great experience! Everyone had a wonderful time.",
	}, nil
}

// defaultParseInformalTime is the default mock implementation for informal time parsing.
func defaultParseInformalTime(_ context.Context, informalDescription, _, timezone string) (string, error) {
	if informalDescription == "" {
		return "", fmt.Errorf("informal description cannot be empty")
	}

	// Return a mock JSON response that looks like a real LLM response
	// Default to "specific" time type with a fixed date for deterministic testing
	return fmt.Sprintf(`{
  "time_type": "specific",
  "unix_timestamp_sec": 1735747200,
  "timezone": "%s",
  "duration_minutes": 60,
  "confidence": "INFERRED"
}`, timezone), nil
}

// ParseInformalTime implements the Provider interface.
func (m *MockProvider) ParseInformalTime(ctx context.Context, informalDescription, currentTime, timezone string) (string, error) {
	m.mu.Lock()
	m.Calls.ParseInformalTime = append(m.Calls.ParseInformalTime, ParseInformalTimeCall{
		InformalDescription: informalDescription, CurrentTime: currentTime, Timezone: timezone,
	})
	m.mu.Unlock()
	if m.ParseInformalTimeFunc == nil {
		return defaultParseInformalTime(ctx, informalDescription, currentTime, timezone)
	}
	return m.ParseInformalTimeFunc(ctx, informalDescription, currentTime, timezone)
}

// GenerateExperienceSuggestions implements the Provider interface.
func (m *MockProvider) GenerateExperienceSuggestions(ctx context.Context, name, description, category string) (*ExperienceSuggestionResult, error) {
	m.mu.Lock()
	m.Calls.GenerateExperienceSuggestions = append(m.Calls.GenerateExperienceSuggestions, GenerateExperienceSuggestionsCall{
		Name: name, Description: description, Category: category,
	})
	m.mu.Unlock()
	if m.GenerateExperienceSuggestionsFunc == nil {
		return defaultGenerateExperienceSuggestions(ctx, name, description, category)
	}
	return m.GenerateExperienceSuggestionsFunc(ctx, name, description, category)
}

// defaultGenerateExperienceSuggestions returns mock suggestion chips for testing.
func defaultGenerateExperienceSuggestions(_ context.Context, _, _, category string) (*ExperienceSuggestionResult, error) {
	hint := "community event"
	if category != "" {
		hint = category
	}
	return &ExperienceSuggestionResult{
		Suggestions:  []string{"Snacks", "Water", "Chairs", "Photos", "Setup help", "Cleanup"},
		CategoryHint: hint,
	}, nil
}

// GenerateRequestSuggestions implements the Provider interface.
func (m *MockProvider) GenerateRequestSuggestions(_ context.Context, title, description, location string) (*RequestSuggestionResult, error) {
	m.mu.Lock()
	m.Calls.GenerateRequestSuggestions = append(m.Calls.GenerateRequestSuggestions, GenerateRequestSuggestionsCall{
		Title: title, Description: description, Location: location,
	})
	m.mu.Unlock()
	if m.GenerateRequestSuggestionsFunc != nil {
		return m.GenerateRequestSuggestionsFunc(context.Background(), title, description, location)
	}
	return &RequestSuggestionResult{
		AdditionalAsks:  []string{"Extra hands", "Vehicle access", "Tools"},
		BreakdownPieces: []string{"Planning", "Execution", "Follow-up"},
		OfferIdeas:      []string{"Offer time", "Offer expertise", "Offer materials"},
		SeedNeeds:       []string{"The thing"},
	}, nil
}

// InferSocialAttributes implements the Provider interface.
func (m *MockProvider) InferSocialAttributes(ctx context.Context, title, description, txType string, itemValueUSD float32) (*SocialAttributeInference, error) {
	m.mu.Lock()
	m.Calls.InferSocialAttributes = append(m.Calls.InferSocialAttributes, InferSocialAttributesCall{
		Title: title, Description: description, TxType: txType, ItemValueUSD: itemValueUSD,
	})
	m.mu.Unlock()
	if m.InferSocialAttributesFunc == nil {
		return defaultInferSocialAttributes(ctx, title, description, txType, itemValueUSD)
	}
	return m.InferSocialAttributesFunc(ctx, title, description, txType, itemValueUSD)
}

// defaultInferSocialAttributes is the default mock implementation for social attributes inference.
// Returns nil to signal that the estimator should fall back to config defaults.
func defaultInferSocialAttributes(_ context.Context, _, _, _ string, _ float32) (*SocialAttributeInference, error) {
	return nil, nil
}

// ClassifyUnifiedCreate implements the Provider interface.
func (m *MockProvider) ClassifyUnifiedCreate(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error) {
	m.mu.Lock()
	m.Calls.ClassifyUnifiedCreate = append(m.Calls.ClassifyUnifiedCreate, ClassifyUnifiedCreateCall{Input: in})
	m.mu.Unlock()
	if m.ClassifyUnifiedCreateFunc != nil {
		return m.ClassifyUnifiedCreateFunc(ctx, in)
	}
	return &UnifiedCreateClassification{
		Type: UnifiedCreateContentTypeGear,
	}, nil
}

// CheckHealth implements the Provider interface.
func (m *MockProvider) CheckHealth(ctx context.Context) ([]*health.Status, error) {
	m.mu.Lock()
	m.Calls.CheckHealth = append(m.Calls.CheckHealth, CheckHealthCall{})
	m.mu.Unlock()
	if m.CheckHealthFunc != nil {
		return m.CheckHealthFunc(ctx)
	}
	return []*health.Status{{
		Name:    "ai",
		Backend: "mock",
	}}, nil
}
