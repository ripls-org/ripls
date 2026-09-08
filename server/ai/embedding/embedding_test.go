package embedding

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// getModelPaths returns paths to the local ONNX model and vocab files.
// Returns empty strings if files are not found.
func getModelPaths() (modelPath, vocabPath string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}

	// Navigate from server/ai/embedding to model_tuning
	modelPath = filepath.Join(cwd, "..", "..", "..", "model_tuning", "ripls_embedding.onnx")
	vocabPath = filepath.Join(cwd, "..", "..", "..", "model_tuning", "ripls_embedding_tokenizer", "vocab.txt")

	if _, err := os.Stat(modelPath); err != nil {
		return "", ""
	}
	if _, err := os.Stat(vocabPath); err != nil {
		return "", ""
	}

	return modelPath, vocabPath
}

func TestEmbedderNew(t *testing.T) {
	modelPath, vocabPath := getModelPaths()
	if modelPath == "" || vocabPath == "" {
		t.Skip("Model files not found")
	}

	embedder, err := New(modelPath, vocabPath)
	if err != nil {
		t.Fatalf("Failed to create Embedder: %v", err)
	}
	defer embedder.Close()
}

func TestEmbedderNew_InvalidPaths(t *testing.T) {
	t.Run("MissingModel", func(t *testing.T) {
		_, err := New("/nonexistent/model.onnx", "/nonexistent/vocab.txt")
		if err == nil {
			t.Error("Expected error for missing model file")
		}
	})
}

func TestEmbedderGenerate(t *testing.T) {
	modelPath, vocabPath := getModelPaths()
	if modelPath == "" || vocabPath == "" {
		t.Skip("Model files not found")
	}

	embedder, err := New(modelPath, vocabPath)
	if err != nil {
		t.Fatalf("Failed to create Embedder: %v", err)
	}
	defer embedder.Close()

	ctx := context.Background()

	t.Run("GeneratesCorrectDimensions", func(t *testing.T) {
		embedding, err := embedder.Generate(ctx, "camping tent")
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}

		if len(embedding) != Dimensions {
			t.Errorf("Expected %d dimensions, got %d", Dimensions, len(embedding))
		}
	})

	t.Run("EmbeddingsAreNormalized", func(t *testing.T) {
		embedding, err := embedder.Generate(ctx, "test query")
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}

		// Calculate L2 norm
		var norm float64
		for _, v := range embedding {
			norm += float64(v) * float64(v)
		}
		norm = math.Sqrt(norm)

		// Should be very close to 1.0
		if math.Abs(norm-1.0) > 1e-4 {
			t.Errorf("Expected norm of 1.0, got %f", norm)
		}
	})

	t.Run("SimilarTextHasHigherSimilarity", func(t *testing.T) {
		query, _ := embedder.Generate(ctx, "camping tent")
		similar, _ := embedder.Generate(ctx, "dome tent for camping")
		dissimilar, _ := embedder.Generate(ctx, "coffee maker")

		// Calculate cosine similarities (embeddings are normalized, so dot product = cosine)
		simSimilar := dotProduct(query, similar)
		simDissimilar := dotProduct(query, dissimilar)

		if simSimilar <= simDissimilar {
			t.Errorf("Expected similar text to have higher similarity: similar=%.4f, dissimilar=%.4f",
				simSimilar, simDissimilar)
		}
	})

	t.Run("Deterministic", func(t *testing.T) {
		text := "test query for determinism"
		emb1, _ := embedder.Generate(ctx, text)
		emb2, _ := embedder.Generate(ctx, text)

		for i := range emb1 {
			if emb1[i] != emb2[i] {
				t.Errorf("Embeddings not deterministic at index %d: %f != %f", i, emb1[i], emb2[i])
			}
		}
	})

	t.Run("EmptyTextReturnsError", func(t *testing.T) {
		_, err := embedder.Generate(ctx, "")
		if err == nil {
			t.Error("Expected error for empty text")
		}
	})

	t.Run("HandlesLongText", func(t *testing.T) {
		longText := ""
		for i := 0; i < 1000; i++ {
			longText += "word "
		}

		embedding, err := embedder.Generate(ctx, longText)
		if err != nil {
			t.Fatalf("Generate failed for long text: %v", err)
		}

		if len(embedding) != Dimensions {
			t.Errorf("Expected %d dimensions, got %d", Dimensions, len(embedding))
		}
	})

	t.Run("HandlesUnicode", func(t *testing.T) {
		embedding, err := embedder.Generate(ctx, "café résumé naïve")
		if err != nil {
			t.Fatalf("Generate failed for unicode text: %v", err)
		}

		if len(embedding) != Dimensions {
			t.Errorf("Expected %d dimensions, got %d", Dimensions, len(embedding))
		}
	})

	t.Run("HandlesMixedCase", func(t *testing.T) {
		// Should produce similar embeddings for case variants
		lower, _ := embedder.Generate(ctx, "camping tent")
		upper, _ := embedder.Generate(ctx, "CAMPING TENT")
		mixed, _ := embedder.Generate(ctx, "Camping Tent")

		// All should be very similar (tokenizer lowercases)
		simLowerUpper := dotProduct(lower, upper)
		simLowerMixed := dotProduct(lower, mixed)

		if simLowerUpper < 0.99 {
			t.Errorf("Expected high similarity for case variants: lower-upper=%.4f", simLowerUpper)
		}
		if simLowerMixed < 0.99 {
			t.Errorf("Expected high similarity for case variants: lower-mixed=%.4f", simLowerMixed)
		}
	})
}

// dotProduct calculates the dot product of two vectors.
func dotProduct(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// BenchmarkEmbedderGenerate benchmarks embedding generation.
func BenchmarkEmbedderGenerate(b *testing.B) {
	modelPath, vocabPath := getModelPaths()
	if modelPath == "" || vocabPath == "" {
		b.Skip("Model files not found")
	}

	embedder, err := New(modelPath, vocabPath)
	if err != nil {
		b.Fatalf("Failed to create Embedder: %v", err)
	}
	defer embedder.Close()

	ctx := context.Background()
	texts := []string{
		"camping tent",
		"baby stroller for jogging in the park with my toddler",
		"cordless drill for home improvement projects",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		text := texts[i%len(texts)]
		_, err := embedder.Generate(ctx, text)
		if err != nil {
			b.Fatalf("Generate failed: %v", err)
		}
	}
}

func BenchmarkEmbedderGenerate_ShortText(b *testing.B) {
	modelPath, vocabPath := getModelPaths()
	if modelPath == "" || vocabPath == "" {
		b.Skip("Model files not found")
	}

	embedder, err := New(modelPath, vocabPath)
	if err != nil {
		b.Fatalf("Failed to create Embedder: %v", err)
	}
	defer embedder.Close()

	ctx := context.Background()
	text := "tent"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := embedder.Generate(ctx, text)
		if err != nil {
			b.Fatalf("Generate failed: %v", err)
		}
	}
}

func BenchmarkEmbedderGenerate_LongText(b *testing.B) {
	modelPath, vocabPath := getModelPaths()
	if modelPath == "" || vocabPath == "" {
		b.Skip("Model files not found")
	}

	embedder, err := New(modelPath, vocabPath)
	if err != nil {
		b.Fatalf("Failed to create Embedder: %v", err)
	}
	defer embedder.Close()

	ctx := context.Background()
	text := "I'm looking for a high-quality camping tent that can accommodate four people comfortably and is suitable for all-season use including winter camping in cold weather conditions with good ventilation"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := embedder.Generate(ctx, text)
		if err != nil {
			b.Fatalf("Generate failed: %v", err)
		}
	}
}
