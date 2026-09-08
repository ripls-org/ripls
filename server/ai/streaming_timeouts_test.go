package ai

import (
	"testing"
	"time"
)

func TestStreamTimeout(t *testing.T) {
	cases := []struct {
		name        string
		provider    ProviderType
		model       string
		isImageCall bool
		want        time.Duration
	}{
		// Text calls
		{"anthropic haiku-4-5 text", ProviderTypeAnthropic, "claude-haiku-4-5", false, 15 * time.Second},
		{"anthropic haiku future-versioned text", ProviderTypeAnthropic, "claude-haiku-5-2027-01-01", false, 15 * time.Second},
		{"anthropic sonnet text", ProviderTypeAnthropic, "claude-sonnet-4-5", false, 45 * time.Second},
		{"anthropic opus text", ProviderTypeAnthropic, "claude-opus-4-5", false, 45 * time.Second},

		{"openai gpt-5-mini text", ProviderTypeOpenAI, "gpt-5-mini", false, 75 * time.Second},
		{"openai gpt-5-mini versioned text", ProviderTypeOpenAI, "gpt-5-mini-2026-03-01", false, 75 * time.Second},
		{"openai gpt-5.4-nano text", ProviderTypeOpenAI, "gpt-5.4-nano", false, 15 * time.Second},
		{"openai unknown text", ProviderTypeOpenAI, "gpt-6-turbo", false, 60 * time.Second},

		{"gemini flash-lite text", ProviderTypeGemini, "vertexai/gemini-2.5-flash-lite", false, 15 * time.Second},
		{"gemini flash text", ProviderTypeGemini, "vertexai/gemini-2.5-flash", false, 30 * time.Second},
		{"gemini pro text", ProviderTypeGemini, "vertexai/gemini-2.5-pro", false, 60 * time.Second},

		{"unknown provider text", ProviderType("unknown"), "some-model", false, defaultStreamTimeout},

		// Image calls — all longer than text equivalents
		{"anthropic haiku-4-5 image", ProviderTypeAnthropic, "claude-haiku-4-5", true, 60 * time.Second},
		{"openai gpt-5-mini image", ProviderTypeOpenAI, "gpt-5-mini", true, 150 * time.Second},
		{"openai gpt-5.4-nano image", ProviderTypeOpenAI, "gpt-5.4-nano", true, 45 * time.Second},
		{"openai unknown image", ProviderTypeOpenAI, "gpt-6-turbo", true, 120 * time.Second},
		{"gemini flash-lite image", ProviderTypeGemini, "vertexai/gemini-2.5-flash-lite", true, 45 * time.Second},
		{"gemini flash image", ProviderTypeGemini, "vertexai/gemini-2.5-flash", true, 90 * time.Second},
		{"gemini pro image", ProviderTypeGemini, "vertexai/gemini-2.5-pro", true, 120 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := streamTimeout(tc.provider, tc.model, tc.isImageCall)
			if got != tc.want {
				t.Errorf("streamTimeout(%q, %q, %v) = %v, want %v", tc.provider, tc.model, tc.isImageCall, got, tc.want)
			}
		})
	}
}

// TestStreamTimeout_CaseInsensitive verifies model matching doesn't break
// if the caller passes an unusually-cased model string.
func TestStreamTimeout_CaseInsensitive(t *testing.T) {
	got := streamTimeout(ProviderTypeAnthropic, "CLAUDE-HAIKU-4-5", false)
	want := 15 * time.Second
	if got != want {
		t.Errorf("uppercase model name: got %v, want %v (case-insensitive match)", got, want)
	}
}

// TestStreamTimeout_ImageAlwaysLonger verifies image timeouts are always
// longer than text timeouts for the same provider and model.
func TestStreamTimeout_ImageAlwaysLonger(t *testing.T) {
	cases := []struct {
		provider ProviderType
		model    string
	}{
		{ProviderTypeAnthropic, "claude-haiku-4-5"},
		{ProviderTypeOpenAI, "gpt-5-mini"},
		{ProviderTypeOpenAI, "gpt-5.4-nano"},
		{ProviderTypeOpenAI, "gpt-6-turbo"},
		{ProviderTypeGemini, "vertexai/gemini-2.5-flash"},
		{ProviderTypeGemini, "vertexai/gemini-2.5-flash-lite"},
		{ProviderTypeGemini, "vertexai/gemini-2.5-pro"},
	}
	for _, tc := range cases {
		text := streamTimeout(tc.provider, tc.model, false)
		image := streamTimeout(tc.provider, tc.model, true)
		if image <= text {
			t.Errorf("streamTimeout(%q, %q, image=%v) = %v should be > text timeout %v",
				tc.provider, tc.model, true, image, text)
		}
	}
}
