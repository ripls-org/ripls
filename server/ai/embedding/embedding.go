// Package embedding provides local embedding generation using a fine-tuned ONNX model.
// This package is separate from the AI provider system and provides a simple interface
// for generating text embeddings for semantic search.
package embedding

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"

	"go.ripls.org/ripls/server/ai/embedding/tokenizer"
	"go.ripls.org/ripls/server/logging"
)

// Dimensions is the embedding dimension for the fine-tuned model.
const Dimensions = 384

// Info contains metadata about the embedding model.
type Info struct {
	// Vendor/source of the embedding model (e.g., "local", "openai").
	Vendor string

	// Model identifier (e.g., "ripls-minilm-v1").
	Model string

	// Number of dimensions in the embedding vector.
	Dimensions int
}

// DefaultInfo returns the embedding info for the local fine-tuned model.
func DefaultInfo() *Info {
	return &Info{
		Vendor:     "local",
		Model:      "ripls-minilm-v1",
		Dimensions: Dimensions,
	}
}

// Info returns metadata about this embedder's model.
func (e *Embedder) Info() *Info {
	return DefaultInfo()
}

// onnxEnv manages the ONNX Runtime environment lifecycle with reference counting.
// The environment is initialized on first use and destroyed when the last Embedder closes.
var onnxEnv struct {
	mu       sync.Mutex
	refCount int
	initErr  error
}

// Embedder generates text embeddings using a local ONNX model.
type Embedder struct {
	session   *ort.DynamicAdvancedSession
	tokenizer *tokenizer.Tokenizer
	modelPath string
	hiddenDim int
	maxSeqLen int
}

// New creates a new Embedder from the given model and vocabulary files.
// modelPath is the path to the ONNX model file.
// vocabPath is the path to the vocabulary file (vocab.txt).
func New(modelPath, vocabPath string) (*Embedder, error) {
	// Verify files exist
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model file not found: %w", err)
	}
	if _, err := os.Stat(vocabPath); err != nil {
		return nil, fmt.Errorf("vocab file not found: %w", err)
	}

	// Initialize ONNX Runtime environment (ref-counted so it survives across
	// multiple Embedders in tests, but gets destroyed on final Close).
	onnxEnv.mu.Lock()
	if onnxEnv.refCount == 0 {
		libPath := findONNXRuntimeLib()
		if libPath == "" {
			onnxEnv.mu.Unlock()
			return nil, fmt.Errorf("ONNX Runtime library not found")
		}
		ort.SetSharedLibraryPath(libPath)
		if err := ort.InitializeEnvironment(); err != nil {
			onnxEnv.mu.Unlock()
			return nil, fmt.Errorf("failed to initialize ONNX Runtime: %w", err)
		}
	}
	onnxEnv.refCount++
	onnxEnv.mu.Unlock()

	// Load tokenizer
	tok, err := tokenizer.NewTokenizer(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load tokenizer: %w", err)
	}

	// Create ONNX session
	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask"},
		[]string{"last_hidden_state"},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX session: %w", err)
	}

	return &Embedder{
		session:   session,
		tokenizer: tok,
		modelPath: modelPath,
		hiddenDim: Dimensions,
		maxSeqLen: 128,
	}, nil
}

// findONNXRuntimeLib searches for the ONNX Runtime shared library.
func findONNXRuntimeLib() string {
	paths := []string{
		// Explicit override first: the binding vendors one ORT_API_VERSION and
		// refuses any native library that predates it ("Error setting ORT API
		// base"), so the version this loads is a correctness constraint, not a
		// preference. When it came last it could never win against an installed
		// system library, which made it impossible to check a binding bump
		// against the ORT the runner image actually ships.
		os.Getenv("ONNXRUNTIME_LIB_PATH"),
		// Homebrew on Apple Silicon
		"/opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib",
		// Homebrew on Intel Mac
		"/usr/local/opt/onnxruntime/lib/libonnxruntime.dylib",
		// Linux system paths
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		// Docker/container paths
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
	}

	for _, path := range paths {
		if path == "" {
			continue
		}
		// G703 flags the ONNXRUNTIME_LIB_PATH element of `paths` as tainted.
		// That variable exists precisely so an operator can point the embedder at
		// a specific runtime build, and it is read with the process's own
		// privileges — someone who can set it can already run code here.
		if _, err := os.Stat(path); err == nil { //nolint:gosec // G703: candidate paths are a fixed list plus the operator-set ONNXRUNTIME_LIB_PATH, which is the point of that variable.
			return path
		}
	}
	return ""
}

// Generate generates a vector embedding for the given text.
// Returns a normalized float32 slice with length equal to Dimensions (384).
func (e *Embedder) Generate(ctx context.Context, text string) ([]float32, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"component", "embedding",
		"operation", "Generate",
	)
	startTime := time.Now()

	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	// Tokenize
	inputIDs, attentionMask := e.tokenizer.Encode(text)

	// Create input tensors
	inputShape := ort.NewShape(1, int64(len(inputIDs)))

	inputIDsTensor, err := ort.NewTensor(inputShape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	defer func() { _ = inputIDsTensor.Destroy() }()

	attentionMaskTensor, err := ort.NewTensor(inputShape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}
	defer func() { _ = attentionMaskTensor.Destroy() }()

	// Create output tensor
	outputShape := ort.NewShape(1, int64(len(inputIDs)), int64(e.hiddenDim))
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("failed to create output tensor: %w", err)
	}
	defer func() { _ = outputTensor.Destroy() }()

	// Run inference
	err = e.session.Run(
		[]ort.Value{inputIDsTensor, attentionMaskTensor},
		[]ort.Value{outputTensor},
	)
	if err != nil {
		return nil, fmt.Errorf("ONNX inference failed: %w", err)
	}

	// Get output data
	hiddenStates := outputTensor.GetData()

	// Apply mean pooling
	embedding := e.meanPool(hiddenStates, attentionMask, len(inputIDs), e.hiddenDim)

	// L2 normalize
	normalized := e.normalize(embedding)

	logger.Debug("embedding generated",
		"text_len", len(text),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return normalized, nil
}

// meanPool applies mean pooling over the sequence dimension.
func (e *Embedder) meanPool(hiddenStates []float32, attentionMask []int64, seqLen, hiddenSize int) []float32 {
	embedding := make([]float32, hiddenSize)
	var validTokens float32

	for i := 0; i < seqLen; i++ {
		if attentionMask[i] == 1 {
			for j := 0; j < hiddenSize; j++ {
				embedding[j] += hiddenStates[i*hiddenSize+j]
			}
			validTokens++
		}
	}

	if validTokens > 0 {
		for j := 0; j < hiddenSize; j++ {
			embedding[j] /= validTokens
		}
	}

	return embedding
}

// normalize applies L2 normalization to an embedding.
func (e *Embedder) normalize(v []float32) []float32 {
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))

	result := make([]float32, len(v))
	if norm > 0 {
		for i, x := range v {
			result[i] = x / norm
		}
	}
	return result
}

// Close cleans up resources used by the Embedder. When the last Embedder is closed,
// the ONNX Runtime environment is also destroyed. Must be called before process exit
// to avoid C++ destructor crashes.
func (e *Embedder) Close() error {
	if e.session != nil {
		if err := e.session.Destroy(); err != nil {
			return err
		}
		e.session = nil
	}

	onnxEnv.mu.Lock()
	defer onnxEnv.mu.Unlock()
	onnxEnv.refCount--
	if onnxEnv.refCount == 0 {
		return ort.DestroyEnvironment()
	}
	return nil
}
