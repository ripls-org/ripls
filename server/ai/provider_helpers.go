package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	genkitai "github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"google.golang.org/genai"

	"go.ripls.org/ripls/server/logging"
)

// blockTypeToolUse is the Anthropic content-block type carrying a tool call.
// Structured output is extracted by scanning a response's blocks for it.
const blockTypeToolUse = "tool_use"

// vertexLocationGlobal is the Vertex AI location string for the non-regional
// endpoint. Preview models are only served there, so the factory selects it
// per-model rather than using the configured region.
const vertexLocationGlobal = "global"

// anthropicTempParam returns the Temperature value to set on an Anthropic
// request. A NaN input means "let the model use its default" — used by the
// eval harness to call newer reasoning-class models (e.g. claude-opus-4-7)
// that reject any explicit temperature value. Returns an unset Opt in
// that case, so the field is omitted on the wire.
func anthropicTempParam(temp float64) anthropicparam.Opt[float64] {
	if math.IsNaN(temp) {
		return anthropicparam.Opt[float64]{}
	}
	return anthropic.Float(temp)
}

// geminiTempPtr returns a *float32 suitable for the Temperature field of
// a Gemini GenerateContentConfig, or nil when temp is NaN (causing the
// field to be omitted and the model to use its default sampling
// temperature).
func geminiTempPtr(temp float64) *float32 {
	if math.IsNaN(temp) {
		return nil
	}
	v := float32(temp)
	return &v
}

// Anthropic MaxTokens constants for different output size budgets.
// Truncation at these limits produces invalid JSON; warnIfTruncated fires when it happens.
const (
	anthropicDefaultMaxTokens    = int64(4096)
	anthropicExperienceMaxTokens = int64(1500)
	anthropicSmallMaxTokens      = int64(1024)
	anthropicTinyMaxTokens       = int64(512)
)

// openaiCallStructured calls the OpenAI Chat Completions API with a strict JSON-schema
// response format and returns the unmarshaled result of type T. It logs the error with
// duration on failure; the caller is responsible for success logging.
func openaiCallStructured[T any](
	ctx context.Context,
	logger *logging.Logger,
	startTime time.Time,
	client *openai.Client,
	model string,
	tempParam param.Opt[float64],
	msgs []openai.ChatCompletionMessageParamUnion,
	schemaName string,
) (T, error) {
	var zero T

	schema := generateSchema[T]()
	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       model,
		Temperature: tempParam,
		Messages:    msgs,
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   schemaName,
					Schema: schema,
					Strict: param.NewOpt(true),
				},
			},
		},
	})
	if err != nil {
		logger.WarnContext(ctx, "openai API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return zero, fmt.Errorf("openai API call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return zero, fmt.Errorf("openai returned no choices")
	}
	var result T
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return zero, fmt.Errorf("failed to parse openai response: %w", err)
	}
	return result, nil
}

// anthropicCallStructured calls the Anthropic Messages API with a tool-use schema and
// returns the unmarshaled result of type T along with the raw response (for token logging
// via logAPICallCompleted). It calls warnIfTruncated on the response and logs errors with
// duration; the caller is responsible for success logging.
func anthropicCallStructured[T any](
	ctx context.Context,
	logger *logging.Logger,
	startTime time.Time,
	client *anthropic.Client,
	model string,
	temp float64,
	maxTokens int64,
	msgs []anthropic.MessageParam,
	toolName string,
) (T, *anthropic.Message, error) {
	var zero T

	toolSchema := generateToolSchema[T]()
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       model,
		MaxTokens:   maxTokens,
		Temperature: anthropicTempParam(temp),
		Messages:    msgs,
		Tools:       []anthropic.ToolUnionParam{anthropic.ToolUnionParamOfTool(toolSchema, toolName)},
		ToolChoice:  anthropic.ToolChoiceParamOfTool(toolName),
	})
	if err != nil {
		logger.WarnContext(ctx, "anthropic API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return zero, nil, fmt.Errorf("anthropic API call failed: %w", err)
	}
	warnIfTruncated(ctx, logger, resp)

	var result T
	for _, block := range resp.Content {
		if block.Type == blockTypeToolUse {
			inputBytes, err := json.Marshal(block.Input)
			if err != nil {
				return zero, resp, fmt.Errorf("failed to marshal tool input: %w", err)
			}
			if err := json.Unmarshal(inputBytes, &result); err != nil {
				return zero, resp, fmt.Errorf("failed to parse anthropic response: %w", err)
			}
			break
		}
	}
	return result, resp, nil
}

// geminiCallStructured calls genkit.GenerateData[T] with model and temperature pre-set,
// and returns a pointer to the typed result. msgOpts should contain the
// genkitai.WithMessages(...) option. It logs errors with duration; the caller is
// responsible for success logging.
func geminiCallStructured[T any](
	ctx context.Context,
	logger *logging.Logger,
	startTime time.Time,
	g *genkit.Genkit,
	model string,
	temp float64,
	msgOpts ...genkitai.GenerateOption,
) (*T, error) {
	opts := append([]genkitai.GenerateOption{
		genkitai.WithModelName(model),
		genkitai.WithConfig(&genai.GenerateContentConfig{Temperature: geminiTempPtr(temp)}),
	}, msgOpts...)
	result, _, err := genkit.GenerateData[T](ctx, g, opts...)
	if err != nil {
		logger.WarnContext(ctx, "gemini API call failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
		return nil, fmt.Errorf("gemini API call failed: %w", err)
	}
	return result, nil
}
