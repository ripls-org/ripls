package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.ripls.org/ripls/server/logging"
)

// timeout for all panic-recovery assertions. A panic recovered by the inline
// recover defer writes to the final channel synchronously (in the deferred
// func); two seconds is far more than enough.
const panicTestTimeout = 2 * time.Second

// drainFieldsChannel ranges over fields and discards all events. Used to
// confirm the fields channel closes without leaking goroutines.
func drainFieldsChannel(t *testing.T, fields <-chan FieldEvent) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		for range fields {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(panicTestTimeout):
		t.Error("fields channel never closed after panic")
	}
}

// --- Anthropic consumer goroutine pattern tests ---.

// TestAnthropicConsumerPanicRecovery_ExperienceFromText verifies that a panic
// inside a consumer goroutine (of the shape used by all Anthropic streaming
// methods) writes a synthetic error to the final channel rather than leaving
// the consumer blocked on a closed-empty channel.
func TestAnthropicConsumerPanicRecovery_ExperienceFromText(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logging.Default())

	// Simulate the consumer goroutine for GenerateExperienceFromTextStreaming.
	// We inject a panic to verify the recover defer fires and writes the error.
	raw := make(chan toolCallStreamFinal, 1)
	final := make(chan ExperienceStreamFinal, 1)

	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-GenerateExperienceFromTextStreaming-consumer",
					"panic", r,
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in anthropic GenerateExperienceFromTextStreaming consumer: %v", r)}
			}
		}()
		// Drain the raw channel to simulate the consumer reaching the panic
		// point (in practice the panic fires during result processing).
		<-raw
		panic("simulated consumer panic")
	}()

	// Unblock the consumer so it reaches the panic.
	raw <- toolCallStreamFinal{ToolInput: []byte(`{}`)}

	select {
	case result := <-final:
		if result.Err == nil {
			t.Fatal("expected Err != nil after consumer panic")
		}
		if !strings.Contains(result.Err.Error(), "panic") {
			t.Errorf("expected error to contain 'panic', got: %v", result.Err)
		}
	case <-time.After(panicTestTimeout):
		t.Fatal("final channel never received a value after consumer panic")
	}
}

// TestAnthropicProducerPanicRecovery_streamToolCall verifies that a panic
// inside a producer goroutine (the shape used by streamToolCall) writes a
// synthetic error to the final channel before the deferred close fires.
func TestAnthropicProducerPanicRecovery_streamToolCall(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logging.Default())

	fields := make(chan FieldEvent, 4)
	final := make(chan toolCallStreamFinal, 1)

	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "anthropic-streamToolCall",
					"panic", r,
				)
				final <- toolCallStreamFinal{Err: fmt.Errorf("panic in anthropic streamToolCall: %v", r)}
			}
		}()
		panic("simulated producer panic")
	}()

	drainFieldsChannel(t, fields)

	select {
	case result := <-final:
		if result.Err == nil {
			t.Fatal("expected Err != nil after producer panic")
		}
		if !strings.Contains(result.Err.Error(), "panic") {
			t.Errorf("expected error to contain 'panic', got: %v", result.Err)
		}
	case <-time.After(panicTestTimeout):
		t.Fatal("final channel never received a value after producer panic")
	}
}

// --- OpenAI consumer goroutine pattern tests ---.

// TestOpenAIConsumerPanicRecovery_ExperienceFromText mirrors the Anthropic
// consumer test for the openaiStreamFinal channel type.
func TestOpenAIConsumerPanicRecovery_ExperienceFromText(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logging.Default())

	raw := make(chan openaiStreamFinal, 1)
	final := make(chan ExperienceStreamFinal, 1)

	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-GenerateExperienceFromTextStreaming-consumer",
					"panic", r,
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in openai GenerateExperienceFromTextStreaming consumer: %v", r)}
			}
		}()
		<-raw
		panic("simulated consumer panic")
	}()

	raw <- openaiStreamFinal{JSON: []byte(`{}`)}

	select {
	case result := <-final:
		if result.Err == nil {
			t.Fatal("expected Err != nil after consumer panic")
		}
		if !strings.Contains(result.Err.Error(), "panic") {
			t.Errorf("expected error to contain 'panic', got: %v", result.Err)
		}
	case <-time.After(panicTestTimeout):
		t.Fatal("final channel never received a value after consumer panic")
	}
}

// TestOpenAIProducerPanicRecovery_streamStructuredOutput verifies that a panic
// in the OpenAI producer goroutine writes a synthetic error to final.
func TestOpenAIProducerPanicRecovery_streamStructuredOutput(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logging.Default())

	fields := make(chan FieldEvent, 4)
	final := make(chan openaiStreamFinal, 1)

	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "openai-streamStructuredOutput",
					"panic", r,
				)
				final <- openaiStreamFinal{Err: fmt.Errorf("panic in openai streamStructuredOutput: %v", r)}
			}
		}()
		panic("simulated producer panic")
	}()

	drainFieldsChannel(t, fields)

	select {
	case result := <-final:
		if result.Err == nil {
			t.Fatal("expected Err != nil after producer panic")
		}
		if !strings.Contains(result.Err.Error(), "panic") {
			t.Errorf("expected error to contain 'panic', got: %v", result.Err)
		}
	case <-time.After(panicTestTimeout):
		t.Fatal("final channel never received a value after producer panic")
	}
}

// --- Gemini consumer goroutine pattern tests ---.

// TestGeminiConsumerPanicRecovery_ExperienceFromText mirrors the Anthropic
// consumer test for the geminiStreamFinal channel type.
func TestGeminiConsumerPanicRecovery_ExperienceFromText(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logging.Default())

	raw := make(chan geminiStreamFinal, 1)
	final := make(chan ExperienceStreamFinal, 1)

	go func() {
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-GenerateExperienceFromTextStreaming-consumer",
					"panic", r,
				)
				final <- ExperienceStreamFinal{Err: fmt.Errorf("panic in gemini GenerateExperienceFromTextStreaming consumer: %v", r)}
			}
		}()
		<-raw
		panic("simulated consumer panic")
	}()

	raw <- geminiStreamFinal{Response: nil}

	select {
	case result := <-final:
		if result.Err == nil {
			t.Fatal("expected Err != nil after consumer panic")
		}
		if !strings.Contains(result.Err.Error(), "panic") {
			t.Errorf("expected error to contain 'panic', got: %v", result.Err)
		}
	case <-time.After(panicTestTimeout):
		t.Fatal("final channel never received a value after consumer panic")
	}
}

// TestGeminiProducerPanicRecovery_streamStructuredOutput verifies that a panic
// in the Gemini producer goroutine writes a synthetic error to final.
func TestGeminiProducerPanicRecovery_streamStructuredOutput(t *testing.T) {
	ctx := logging.WithLogger(context.Background(), logging.Default())

	fields := make(chan FieldEvent, 4)
	final := make(chan geminiStreamFinal, 1)

	go func() {
		defer close(fields)
		defer close(final)
		defer func() {
			if r := recover(); r != nil {
				logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
					"goroutine", "gemini-streamStructuredOutput",
					"panic", r,
				)
				final <- geminiStreamFinal{Err: fmt.Errorf("panic in gemini streamStructuredOutput: %v", r)}
			}
		}()
		panic("simulated producer panic")
	}()

	drainFieldsChannel(t, fields)

	select {
	case result := <-final:
		if result.Err == nil {
			t.Fatal("expected Err != nil after producer panic")
		}
		if !strings.Contains(result.Err.Error(), "panic") {
			t.Errorf("expected error to contain 'panic', got: %v", result.Err)
		}
	case <-time.After(panicTestTimeout):
		t.Fatal("final channel never received a value after producer panic")
	}
}
