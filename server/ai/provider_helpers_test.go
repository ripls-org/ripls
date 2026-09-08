package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/firebase/genkit/go/genkit"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"

	"go.ripls.org/ripls/server/logging"
)

// newMinimalGenkit creates a Genkit instance with no model plugins registered.
// Any call to GenerateData on this instance will fail because no models are available.
func newMinimalGenkit(ctx context.Context) *genkit.Genkit {
	return genkit.Init(ctx)
}

// helperOutput is a minimal struct used as the type argument in helper tests.
type helperOutput struct {
	Summary string `json:"summary"`
}

func TestOpenaiCallStructured(t *testing.T) {
	logger := logging.Default()
	ctx := context.Background()

	t.Run("returns parsed result on success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1700000000,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"{\"summary\":\"hello world\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`)
		}))
		defer srv.Close()

		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
		result, err := openaiCallStructured[helperOutput](ctx, logger, time.Now(), &client, "gpt-test", param.Opt[float64]{},
			[]openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}, "test_schema")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Summary != "hello world" {
			t.Errorf("expected Summary=%q, got %q", "hello world", result.Summary)
		}
	})

	t.Run("returns error when no choices", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"chatcmpl-empty","object":"chat.completion","created":1700000000,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":0,"total_tokens":5}}`)
		}))
		defer srv.Close()

		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
		_, err := openaiCallStructured[helperOutput](ctx, logger, time.Now(), &client, "gpt-test", param.Opt[float64]{},
			[]openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}, "test_schema")
		if err == nil {
			t.Fatal("expected error for empty choices, got nil")
		}
	})

	t.Run("returns error on bad JSON content", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"chatcmpl-bad","object":"chat.completion","created":1700000000,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"not-valid-json"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`)
		}))
		defer srv.Close()

		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
		_, err := openaiCallStructured[helperOutput](ctx, logger, time.Now(), &client, "gpt-test", param.Opt[float64]{},
			[]openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}, "test_schema")
		if err == nil {
			t.Fatal("expected error for bad JSON content, got nil")
		}
	})

	// Transport failure test uses port 1 which is closed on all systems.
	t.Run("returns error on transport failure", func(t *testing.T) {
		client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL("http://127.0.0.1:1"), option.WithMaxRetries(0))
		_, err := openaiCallStructured[helperOutput](ctx, logger, time.Now(), &client, "gpt-test", param.Opt[float64]{},
			[]openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}, "test_schema")
		if err == nil {
			t.Fatal("expected error on transport failure, got nil")
		}
	})
}

func TestAnthropicCallStructured(t *testing.T) {
	logger := logging.Default()
	ctx := context.Background()

	makeToolUseBody := func(stopReason string) []byte {
		body, _ := json.Marshal(map[string]any{
			"id":   "msg_test",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{
					"type":  "tool_use",
					"id":    "toolu_test",
					"name":  "test_schema",
					"input": map[string]string{"summary": "hello world"},
				},
			},
			"model":       "claude-test",
			"stop_reason": stopReason,
			"usage": map[string]int{
				"input_tokens":                10,
				"output_tokens":               20,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
			},
		})
		return body
	}

	t.Run("returns parsed result on success", func(t *testing.T) {
		body := makeToolUseBody("tool_use")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
		}))
		defer srv.Close()

		client := anthropic.NewClient(anthropicoption.WithAPIKey("test"), anthropicoption.WithBaseURL(srv.URL), anthropicoption.WithMaxRetries(0))
		result, resp, err := anthropicCallStructured[helperOutput](ctx, logger, time.Now(), &client, "claude-test", 0.7, anthropicDefaultMaxTokens,
			[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}, "test_schema")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if result.Summary != "hello world" {
			t.Errorf("expected Summary=%q, got %q", "hello world", result.Summary)
		}
		if resp == nil {
			t.Error("expected non-nil response for token logging")
		}
	})

	// warnIfTruncated fires on max_tokens but the helper does not return an error.
	t.Run("no error when stop_reason is max_tokens", func(t *testing.T) {
		body := makeToolUseBody("max_tokens")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
		}))
		defer srv.Close()

		client := anthropic.NewClient(anthropicoption.WithAPIKey("test"), anthropicoption.WithBaseURL(srv.URL), anthropicoption.WithMaxRetries(0))
		_, _, err := anthropicCallStructured[helperOutput](ctx, logger, time.Now(), &client, "claude-test", 0.7, anthropicDefaultMaxTokens,
			[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}, "test_schema")
		if err != nil {
			t.Fatalf("expected no error for truncated response, got %v", err)
		}
	})

	t.Run("returns error on transport failure", func(t *testing.T) {
		client := anthropic.NewClient(anthropicoption.WithAPIKey("test"), anthropicoption.WithBaseURL("http://127.0.0.1:1"), anthropicoption.WithMaxRetries(0))
		_, _, err := anthropicCallStructured[helperOutput](ctx, logger, time.Now(), &client, "claude-test", 0.7, anthropicDefaultMaxTokens,
			[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}, "test_schema")
		if err == nil {
			t.Fatal("expected error on transport failure, got nil")
		}
	})
}

// TestGeminiCallStructured_ErrorPath tests the error path of geminiCallStructured.
// Full success tests require a live Genkit/Vertex AI instance and belong in
// provider_integration_test.go (build tag: integration).
func TestGeminiCallStructured_ErrorPath(t *testing.T) {
	ctx := context.Background()
	logger := logging.Default()

	// genkit.Init with no plugins produces a registry with no registered models.
	// Any call to GenerateData with an unregistered model name will fail.
	// We verify the helper surfaces that error and logs it.
	t.Run("returns error when model is not registered", func(t *testing.T) {
		// Import genkit directly in the test to avoid pulling in real credentials.
		// genkit.Init with no options creates a minimal registry.
		g := newMinimalGenkit(ctx)
		if g == nil {
			t.Skip("could not create minimal genkit instance; skipping")
		}
		// no message options - lookup fails before any network call
		_, err := geminiCallStructured[helperOutput](ctx, logger, time.Now(), g, "nonexistent/model", 0.7)
		if err == nil {
			t.Fatal("expected error for unregistered model, got nil")
		}
	})
}

// newBufLogger returns a buffer and a JSON logger writing to it at DEBUG level.
// Use this to capture structured log output in tests.
func newBufLogger() (*bytes.Buffer, *logging.Logger) {
	buf := &bytes.Buffer{}
	return buf, logging.NewLogger(logging.Options{Level: "debug", Format: "json", Output: buf})
}

// assertWarnNotError verifies that buf contains at least one WARN-level entry
// and zero ERROR-level entries — the expected contract for leaf-provider helpers.
func assertWarnNotError(t *testing.T, buf *bytes.Buffer, wantMsgSubstr string) {
	t.Helper()
	out := buf.String()
	// Cloud Logging JSON uses "severity":"WARNING" for WARN and "severity":"ERROR" for ERROR.
	if !strings.Contains(out, `"severity":"WARNING"`) {
		t.Errorf("expected WARNING log entry, got:\n%s", out)
	}
	if strings.Contains(out, `"severity":"ERROR"`) {
		t.Errorf("unexpected ERROR log entry, got:\n%s", out)
	}
	if wantMsgSubstr != "" && !strings.Contains(out, wantMsgSubstr) {
		t.Errorf("expected log output to contain %q, got:\n%s", wantMsgSubstr, out)
	}
}

// TestOpenaiCallStructured_LogLevel asserts the helper emits WARN (not ERROR) on failure.
func TestOpenaiCallStructured_LogLevel(t *testing.T) {
	ctx := context.Background()
	buf, logger := newBufLogger()

	// Port 1 is always closed; this triggers a transport error.
	client := openai.NewClient(option.WithAPIKey("test"), option.WithBaseURL("http://127.0.0.1:1"), option.WithMaxRetries(0))
	_, err := openaiCallStructured[helperOutput](ctx, logger, time.Now(), &client, "gpt-test", param.Opt[float64]{},
		[]openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}, "test_schema")
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	assertWarnNotError(t, buf, "openai API call failed")
}

// TestAnthropicCallStructured_LogLevel asserts the helper emits WARN (not ERROR) on failure.
func TestAnthropicCallStructured_LogLevel(t *testing.T) {
	ctx := context.Background()
	buf, logger := newBufLogger()

	client := anthropic.NewClient(anthropicoption.WithAPIKey("test"), anthropicoption.WithBaseURL("http://127.0.0.1:1"), anthropicoption.WithMaxRetries(0))
	_, _, err := anthropicCallStructured[helperOutput](ctx, logger, time.Now(), &client, "claude-test", 0.7, anthropicDefaultMaxTokens,
		[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}, "test_schema")
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
	assertWarnNotError(t, buf, "anthropic API call failed")
}

// TestGeminiCallStructured_LogLevel asserts the helper emits WARN (not ERROR) on failure.
func TestGeminiCallStructured_LogLevel(t *testing.T) {
	ctx := context.Background()
	buf, logger := newBufLogger()

	g := newMinimalGenkit(ctx)
	if g == nil {
		t.Skip("could not create minimal genkit instance; skipping")
	}
	_, err := geminiCallStructured[helperOutput](ctx, logger, time.Now(), g, "nonexistent/model", 0.7)
	if err == nil {
		t.Fatal("expected error for unregistered model, got nil")
	}
	assertWarnNotError(t, buf, "gemini API call failed")
}
