//go:build time_parsing_golden

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// timeParsingScenarios mirrors the JSON structure of time_parsing_golden.json.
type timeParsingScenarios struct {
	TextScenarios     []textScenario     `json:"generate_experience_from_text"`
	WebpageScenarios  []webpageScenario  `json:"generate_experience_from_webpage"`
	InformalScenarios []informalScenario `json:"convert_informal_time"`
}

type textScenario struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Prompt      string          `json:"prompt"`
	Region      string          `json:"region"`
	CurrentTime string          `json:"current_time"`
	Timezone    string          `json:"timezone"`
	Expected    *expExpectation `json:"expected,omitempty"`
	ExpectError bool            `json:"expected_error,omitempty"`
}

type webpageScenario struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	PageTitle   string          `json:"page_title"`
	PageDesc    string          `json:"page_description"`
	PageBody    string          `json:"page_body"`
	Region      string          `json:"region"`
	CurrentTime string          `json:"current_time"`
	Timezone    string          `json:"timezone"`
	Expected    *expExpectation `json:"expected,omitempty"`
}

type informalScenario struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	InformalDescription string               `json:"informal_description"`
	CurrentTime         string               `json:"current_time"`
	Timezone            string               `json:"timezone"`
	Expected            *informalExpectation `json:"expected,omitempty"`
}

type expExpectation struct {
	ExpectedDate     string   `json:"expected_date"`
	ExpectedTime     string   `json:"expected_time"`
	ExpectedISO      string   `json:"expected_datetime_iso"`
	TimeConfidence   []string `json:"time_confidence"`
	TitleContains    []string `json:"title_contains"`
	HasDescription   bool     `json:"has_description"`
	LocationContains []string `json:"location_contains"`
	MinConfidence    float32  `json:"min_confidence"`
}

type informalExpectation struct {
	TimeType     string   `json:"time_type"`
	ExpectedDate string   `json:"expected_date"`
	ExpectedTime string   `json:"expected_time"`
	ExpectedISO  string   `json:"expected_datetime_iso"`
	Duration     int32    `json:"duration_minutes"`
	Confidence   []string `json:"confidence"`
}

// recordedResponse stores one LLM response for golden data.
type recordedResponse struct {
	ScenarioID string                `json:"scenario_id"`
	Provider   string                `json:"provider"`
	Response   *ExperienceGeneration `json:"response,omitempty"`
	RawJSON    string                `json:"raw_json,omitempty"`
	Error      string                `json:"error,omitempty"`
}

// recordedResponses is the top-level structure written to the golden response file.
type recordedResponses struct {
	GeneratedAt       string             `json:"generated_at"`
	Provider          string             `json:"provider"`
	Model             string             `json:"model"`
	TextResponses     []recordedResponse `json:"text_responses"`
	WebpageResponses  []recordedResponse `json:"webpage_responses"`
	InformalResponses []recordedResponse `json:"informal_responses"`
}

// TestTimeParsingIntegration runs all time-parsing scenarios against a live LLM
// and records the responses to server/test_data/time_parsing_responses.json.
//
// This test is gated behind the "time_parsing_golden" build tag (not "integration")
// so it does NOT run in CI. It must be invoked explicitly to regenerate golden data:
//
//	ANTHROPIC_API_KEY=sk-... go test -v -tags=time_parsing_golden -run TestTimeParsingIntegration -timeout 10m ./server/ai/
func TestTimeParsingIntegration(t *testing.T) {
	ctx := context.Background()

	// Load scenarios.
	scenarioData, err := os.ReadFile("../../server/test_data/time_parsing_golden.json")
	if err != nil {
		// Try relative to the package directory.
		scenarioData, err = os.ReadFile("../test_data/time_parsing_golden.json")
		if err != nil {
			t.Fatalf("Failed to load time_parsing_golden.json: %v", err)
		}
	}

	var scenarios timeParsingScenarios
	if err := json.Unmarshal(scenarioData, &scenarios); err != nil {
		t.Fatalf("Failed to parse scenarios: %v", err)
	}

	// Find an available provider.
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	openaiKey := os.Getenv("OPENAI_API_KEY")

	var provider Provider
	var providerName, modelName string

	if anthropicKey != "" {
		modelName = "claude-haiku-4-5"
		provider, err = NewAnthropicProvider(anthropicKey, modelName)
		if err != nil {
			t.Fatalf("Failed to create Anthropic provider: %v", err)
		}
		providerName = "anthropic"
	} else if openaiKey != "" {
		modelName = "gpt-5-mini"
		provider, err = NewOpenAIProvider(openaiKey, modelName)
		if err != nil {
			t.Fatalf("Failed to create OpenAI provider: %v", err)
		}
		providerName = "openai"
	} else {
		t.Skip("Skipping: no ANTHROPIC_API_KEY or OPENAI_API_KEY set")
	}

	recorded := recordedResponses{
		GeneratedAt: "auto-generated by TestTimeParsingIntegration",
		Provider:    providerName,
		Model:       modelName,
	}

	// Run text scenarios.
	t.Run("text_scenarios", func(t *testing.T) {
		for _, sc := range scenarios.TextScenarios {
			sc := sc
			t.Run(sc.ID, func(t *testing.T) {
				if sc.ExpectError {
					t.Logf("Skipping error scenario %s (tested in unit tests)", sc.ID)
					recorded.TextResponses = append(recorded.TextResponses, recordedResponse{
						ScenarioID: sc.ID,
						Provider:   providerName,
						Error:      "skipped: error scenario",
					})
					return
				}

				result, err := retryOnRateLimit(t, sc.ID, func() (*ExperienceGeneration, error) {
					return provider.GenerateExperienceFromText(ctx, sc.Prompt, sc.Region, sc.CurrentTime)
				})

				rec := recordedResponse{
					ScenarioID: sc.ID,
					Provider:   providerName,
				}

				if err != nil {
					rec.Error = err.Error()
					t.Logf("ERROR: %v", err)
				} else {
					rec.Response = result
					logExperienceResult(t, sc.ID, result, sc.Expected)
				}

				recorded.TextResponses = append(recorded.TextResponses, rec)
			})
		}
	})

	// Run webpage scenarios.
	t.Run("webpage_scenarios", func(t *testing.T) {
		for _, sc := range scenarios.WebpageScenarios {
			sc := sc
			t.Run(sc.ID, func(t *testing.T) {
				result, err := retryOnRateLimit(t, sc.ID, func() (*ExperienceGeneration, error) {
					return provider.GenerateExperienceFromWebpage(ctx, sc.PageTitle, sc.PageDesc, sc.PageBody, sc.Region, sc.CurrentTime)
				})

				rec := recordedResponse{
					ScenarioID: sc.ID,
					Provider:   providerName,
				}

				if err != nil {
					rec.Error = err.Error()
					t.Logf("ERROR: %v", err)
				} else {
					rec.Response = result
					logExperienceResult(t, sc.ID, result, sc.Expected)
				}

				recorded.WebpageResponses = append(recorded.WebpageResponses, rec)
			})
		}
	})

	// Run informal time scenarios.
	t.Run("informal_scenarios", func(t *testing.T) {
		for _, sc := range scenarios.InformalScenarios {
			sc := sc
			t.Run(sc.ID, func(t *testing.T) {
				rawJSON, err := retryOnRateLimit(t, sc.ID, func() (string, error) {
					return provider.ParseInformalTime(ctx, sc.InformalDescription, sc.CurrentTime, sc.Timezone)
				})

				rec := recordedResponse{
					ScenarioID: sc.ID,
					Provider:   providerName,
				}

				if err != nil {
					rec.Error = err.Error()
					t.Logf("ERROR: %v", err)
				} else {
					rec.RawJSON = rawJSON
					t.Logf("--- %s ---", sc.ID)
					t.Logf("Input: %q", sc.InformalDescription)
					t.Logf("Timezone: %s", sc.Timezone)
					t.Logf("Response: %s", rawJSON)
				}

				recorded.InformalResponses = append(recorded.InformalResponses, rec)
			})
		}
	})

	// Only write the golden response file when the full suite ran (all three
	// scenario groups populated). This prevents partial runs (e.g.,
	// -run "text_scenarios/text_dst_spring_forward") from clobbering the file.
	totalRecorded := len(recorded.TextResponses) + len(recorded.WebpageResponses) + len(recorded.InformalResponses)
	totalExpected := len(scenarios.TextScenarios) + len(scenarios.WebpageScenarios) + len(scenarios.InformalScenarios)

	if totalRecorded == totalExpected {
		outputPath := "../test_data/time_parsing_responses.json"
		outputData, err := json.MarshalIndent(recorded, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal responses: %v", err)
		}

		if err := os.WriteFile(outputPath, outputData, 0o644); err != nil {
			t.Fatalf("Failed to write responses to %s: %v", outputPath, err)
		}

		t.Logf("\nRecorded %d responses to %s", totalRecorded, outputPath)
	} else {
		t.Logf("\nPartial run: %d/%d scenarios executed. Golden file NOT updated.",
			totalRecorded, totalExpected)
	}
}

// logExperienceResult logs and validates an experience generation result.
func logExperienceResult(t *testing.T, id string, result *ExperienceGeneration, expected *expExpectation) {
	t.Helper()

	t.Logf("--- %s ---", id)
	t.Logf("Title: %s", result.Title)
	t.Logf("Description: %s", truncate(result.Description, 100))
	t.Logf("Date: %q  Time: %q  TimeConfidence: %s", result.Date, result.Time, result.TimeConfidence)
	t.Logf("LocationQuery: %q", result.LocationQuery)
	t.Logf("Confidence: %.2f", result.Confidence)

	if expected == nil {
		return
	}

	// Validate date.
	if expected.ExpectedDate != "" && result.Date != expected.ExpectedDate {
		t.Errorf("Date mismatch: got %q, want %q", result.Date, expected.ExpectedDate)
	}

	// Validate time.
	if expected.ExpectedTime != "" && result.Time != expected.ExpectedTime {
		t.Errorf("Time mismatch: got %q, want %q", result.Time, expected.ExpectedTime)
	}

	// Validate time confidence.
	if len(expected.TimeConfidence) > 0 {
		matched := false
		for _, tc := range expected.TimeConfidence {
			if result.TimeConfidence == tc {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("TimeConfidence mismatch: got %q, want one of %v", result.TimeConfidence, expected.TimeConfidence)
		}
	}

	// Validate title contains.
	if len(expected.TitleContains) > 0 {
		titleLower := strings.ToLower(result.Title)
		anyMatch := false
		for _, sub := range expected.TitleContains {
			if strings.Contains(titleLower, strings.ToLower(sub)) {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			t.Errorf("Title %q doesn't contain any of %v", result.Title, expected.TitleContains)
		}
	}

	// Validate description.
	if expected.HasDescription && result.Description == "" {
		t.Error("Expected non-empty description")
	}

	// Validate confidence.
	if result.Confidence < expected.MinConfidence {
		t.Errorf("Confidence %.2f below minimum %.2f", result.Confidence, expected.MinConfidence)
	}

	// Validate location.
	if len(expected.LocationContains) > 0 && result.LocationQuery != "" {
		locLower := strings.ToLower(result.LocationQuery)
		anyMatch := false
		for _, sub := range expected.LocationContains {
			if strings.Contains(locLower, strings.ToLower(sub)) {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			t.Logf("Warning: LocationQuery %q doesn't match any of %v", result.LocationQuery, expected.LocationContains)
		}
	}
}

// truncate truncates a string to maxLen and appends "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TestTimeParsingIntegration_LoadExisting verifies that the golden response file
// can be loaded and the scenario mock can be constructed from it. This is a
// quick sanity check that doesn't call any LLM.
func TestTimeParsingIntegration_LoadExisting(t *testing.T) {
	data, err := os.ReadFile("../test_data/time_parsing_responses.json")
	if err != nil {
		t.Skip("No time_parsing_responses.json found — run TestTimeParsingIntegration first")
	}

	var responses recordedResponses
	if err := json.Unmarshal(data, &responses); err != nil {
		t.Fatalf("Failed to parse responses: %v", err)
	}

	t.Logf("Loaded responses: provider=%s, model=%s", responses.Provider, responses.Model)
	t.Logf("Text: %d, Webpage: %d, Informal: %d",
		len(responses.TextResponses), len(responses.WebpageResponses), len(responses.InformalResponses))

	// Verify we can build a scenario mock.
	mock := NewScenarioMockFromResponses(&responses)
	if mock == nil {
		t.Fatal("Failed to create scenario mock")
	}

	t.Logf("Scenario mock created with %d text, %d webpage, %d informal mappings",
		len(mock.textResponses), len(mock.webpageResponses), len(mock.informalResponses))

	// Verify each scenario ID has a mapping.
	for _, r := range responses.TextResponses {
		if r.Error != "" {
			continue
		}
		if _, ok := mock.textResponses[r.ScenarioID]; !ok {
			t.Errorf("Missing text mapping for scenario %s", r.ScenarioID)
		}
	}

	for _, r := range responses.WebpageResponses {
		if r.Error != "" {
			continue
		}
		if _, ok := mock.webpageResponses[r.ScenarioID]; !ok {
			t.Errorf("Missing webpage mapping for scenario %s", r.ScenarioID)
		}
	}

	for _, r := range responses.InformalResponses {
		if r.Error != "" {
			continue
		}
		if _, ok := mock.informalResponses[r.ScenarioID]; !ok {
			t.Errorf("Missing informal mapping for scenario %s", r.ScenarioID)
		}
	}
}

// NewScenarioMockFromResponses creates a ScenarioMock from recorded LLM responses.
func NewScenarioMockFromResponses(responses *recordedResponses) *ScenarioMock {
	mock := &ScenarioMock{
		textResponses:     make(map[string]*ExperienceGeneration),
		webpageResponses:  make(map[string]*ExperienceGeneration),
		informalResponses: make(map[string]string),
	}

	for _, r := range responses.TextResponses {
		if r.Response != nil {
			mock.textResponses[r.ScenarioID] = r.Response
		}
	}

	for _, r := range responses.WebpageResponses {
		if r.Response != nil {
			mock.webpageResponses[r.ScenarioID] = r.Response
		}
	}

	for _, r := range responses.InformalResponses {
		if r.RawJSON != "" {
			mock.informalResponses[r.ScenarioID] = r.RawJSON
		}
	}

	return mock
}

// ScenarioMock holds pre-recorded responses keyed by scenario ID.
// Used by unit tests to replay LLM responses without calling a live provider.
type ScenarioMock struct {
	textResponses     map[string]*ExperienceGeneration
	webpageResponses  map[string]*ExperienceGeneration
	informalResponses map[string]string
}

// ToMockProvider converts the scenario mock into a standard MockProvider
// that can be injected into the experience Service. The caller must set
// the active scenario ID before each call.
func (sm *ScenarioMock) ToMockProvider(activeScenarioID *string) *MockProvider {
	mock := NewMockProvider()

	mock.GenerateExperienceFromTextFunc = func(_ context.Context, _, _, _ string) (*ExperienceGeneration, error) {
		if activeScenarioID == nil || *activeScenarioID == "" {
			return nil, fmt.Errorf("no active scenario ID set")
		}
		resp, ok := sm.textResponses[*activeScenarioID]
		if !ok {
			return nil, fmt.Errorf("no recorded response for text scenario %q", *activeScenarioID)
		}
		return resp, nil
	}

	mock.GenerateExperienceFromWebpageFunc = func(_ context.Context, _, _, _, _, _ string) (*ExperienceGeneration, error) {
		if activeScenarioID == nil || *activeScenarioID == "" {
			return nil, fmt.Errorf("no active scenario ID set")
		}
		resp, ok := sm.webpageResponses[*activeScenarioID]
		if !ok {
			return nil, fmt.Errorf("no recorded response for webpage scenario %q", *activeScenarioID)
		}
		return resp, nil
	}

	mock.ParseInformalTimeFunc = func(_ context.Context, _, _, _ string) (string, error) {
		if activeScenarioID == nil || *activeScenarioID == "" {
			return "", fmt.Errorf("no active scenario ID set")
		}
		resp, ok := sm.informalResponses[*activeScenarioID]
		if !ok {
			return "", fmt.Errorf("no recorded response for informal scenario %q", *activeScenarioID)
		}
		return resp, nil
	}

	return mock
}
