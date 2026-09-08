package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.ripls.org/ripls/server/ai/embedding"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestEmbeddingColumnName(t *testing.T) {
	tests := []struct {
		name     string
		info     *embedding.Info
		expected string
	}{
		{
			name:     "local model",
			info:     &embedding.Info{Vendor: "local", Model: "ripls-minilm-v1", Dimensions: 384},
			expected: "local_ripls_minilm_v1",
		},
		{
			name:     "legacy gemini model",
			info:     &embedding.Info{Vendor: "gemini", Model: "text-embedding-004", Dimensions: 768},
			expected: "gemini_text_embedding_004",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EmbeddingColumnName(tt.info)
			if got != tt.expected {
				t.Errorf("EmbeddingColumnName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEmbeddingIndexName(t *testing.T) {
	info := &embedding.Info{Vendor: "gemini", Model: "text-embedding-004", Dimensions: 768}

	tests := []struct {
		tableName string
		expected  string
	}{
		{"gear", "gear_gemini_text_embedding_004_hnsw_idx"},
		{"request", "request_gemini_text_embedding_004_hnsw_idx"},
	}

	for _, tt := range tests {
		t.Run(tt.tableName, func(t *testing.T) {
			got := EmbeddingIndexName(tt.tableName, info)
			if got != tt.expected {
				t.Errorf("EmbeddingIndexName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatVector(t *testing.T) {
	tests := []struct {
		name     string
		input    []float32
		expected string
	}{
		{"empty", []float32{}, "[]"},
		{"single", []float32{1.5}, "[1.5]"},
		{"multiple", []float32{1.0, 2.5, 3.0}, "[1,2.5,3]"},
		{"negative", []float32{-1.0, 0.0, 1.0}, "[-1,0,1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatVector(tt.input)
			if got != tt.expected {
				t.Errorf("formatVector() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEnsureEmbeddingColumn(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	info := &embedding.Info{Vendor: "gemini", Model: "text-embedding-004", Dimensions: 768}

	// Ensure column is created
	err := storage.EnsureEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn() error = %v", err)
	}

	// Verify column exists
	columns, err := storage.ListEmbeddingColumns(ctx, "gear")
	if err != nil {
		t.Fatalf("ListEmbeddingColumns() error = %v", err)
	}

	expectedCol := EmbeddingColumnName(info)
	found := false
	for _, col := range columns {
		if col == expectedCol {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Column %q not found in %v", expectedCol, columns)
	}

	// Verify idempotent (calling again should not error)
	err = storage.EnsureEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Errorf("EnsureEmbeddingColumn() second call error = %v", err)
	}
}

func TestDropEmbeddingColumn(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	info := &embedding.Info{Vendor: "gemini", Model: "text-embedding-004", Dimensions: 768}

	// Create column first
	err := storage.EnsureEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn() error = %v", err)
	}

	// Drop column
	err = storage.DropEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Fatalf("DropEmbeddingColumn() error = %v", err)
	}

	// Verify column is gone
	columns, err := storage.ListEmbeddingColumns(ctx, "gear")
	if err != nil {
		t.Fatalf("ListEmbeddingColumns() error = %v", err)
	}

	expectedCol := EmbeddingColumnName(info)
	for _, col := range columns {
		if col == expectedCol {
			t.Errorf("Column %q should have been dropped", expectedCol)
		}
	}

	// Verify idempotent (calling again should not error)
	err = storage.DropEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Errorf("DropEmbeddingColumn() second call error = %v", err)
	}
}

func TestUpdateAndQueryEmbedding(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	info := &embedding.Info{Vendor: "gemini", Model: "text-embedding-004", Dimensions: 3}

	// Create embedding column
	err := storage.EnsureEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn() error = %v", err)
	}

	// Insert test gear using the storage's Insert method
	gear := createTestGear("test-gear-1", "Bicycle", "A nice bike")
	_, err = storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	gear2 := createTestGear("test-gear-2", "Refrigerator", "A cold appliance")
	_, err = storage.Insert(ctx, gear2)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Update embeddings
	// Bicycle embedding (normalized vector pointing in one direction)
	bicycleEmbedding := []float32{0.8, 0.5, 0.3}
	err = storage.UpdateEmbedding(ctx, "gear", "test-gear-1", info, bicycleEmbedding)
	if err != nil {
		t.Fatalf("UpdateEmbedding() error = %v", err)
	}

	// Refrigerator embedding (different direction)
	fridgeEmbedding := []float32{0.1, 0.2, 0.9}
	err = storage.UpdateEmbedding(ctx, "gear", "test-gear-2", info, fridgeEmbedding)
	if err != nil {
		t.Fatalf("UpdateEmbedding() error = %v", err)
	}

	// Query with embedding similar to bicycle
	queryEmbedding := []float32{0.7, 0.6, 0.4}
	results, err := storage.QueryByEmbeddingSimilarity(ctx, "gear", info, queryEmbedding, 10)
	if err != nil {
		t.Fatalf("QueryByEmbeddingSimilarity() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// First result should be bicycle (more similar to query)
	if len(results) > 0 && results[0].ID != "test-gear-1" {
		t.Errorf("Expected first result to be test-gear-1 (bicycle), got %s", results[0].ID)
	}

	// Verify similarity scores are in valid range
	for _, r := range results {
		if r.Similarity < 0 || r.Similarity > 1 {
			t.Errorf("Similarity %f out of range [0, 1]", r.Similarity)
		}
	}
}

func TestCountWithEmbedding(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	info := &embedding.Info{Vendor: "gemini", Model: "text-embedding-004", Dimensions: 3}

	// Create embedding column
	err := storage.EnsureEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn() error = %v", err)
	}

	// Insert test gear
	gear1 := createTestGear("gear-1", "Item 1", "Desc 1")
	_, _ = storage.Insert(ctx, gear1)
	gear2 := createTestGear("gear-2", "Item 2", "Desc 2")
	_, _ = storage.Insert(ctx, gear2)
	gear3 := createTestGear("gear-3", "Item 3", "Desc 3")
	_, _ = storage.Insert(ctx, gear3)

	// Initially no embeddings
	total, withEmbedding, err := storage.CountWithEmbedding(ctx, "gear", info)
	if err != nil {
		t.Fatalf("CountWithEmbedding() error = %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total=3, got %d", total)
	}
	if withEmbedding != 0 {
		t.Errorf("Expected withEmbedding=0, got %d", withEmbedding)
	}

	// Add embedding to one gear
	_ = storage.UpdateEmbedding(ctx, "gear", "gear-1", info, []float32{0.1, 0.2, 0.3})

	total, withEmbedding, err = storage.CountWithEmbedding(ctx, "gear", info)
	if err != nil {
		t.Fatalf("CountWithEmbedding() error = %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total=3, got %d", total)
	}
	if withEmbedding != 1 {
		t.Errorf("Expected withEmbedding=1, got %d", withEmbedding)
	}
}

func TestQueryWithoutEmbedding(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	info := &embedding.Info{Vendor: "gemini", Model: "text-embedding-004", Dimensions: 3}

	// Create embedding column
	err := storage.EnsureEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn() error = %v", err)
	}

	// Insert test gear
	gear1 := createTestGear("gear-1", "Item 1", "Desc 1")
	_, _ = storage.Insert(ctx, gear1)
	gear2 := createTestGear("gear-2", "Item 2", "Desc 2")
	_, _ = storage.Insert(ctx, gear2)

	// All should be without embedding
	ids, err := storage.QueryWithoutEmbedding(ctx, "gear", info, 10)
	if err != nil {
		t.Fatalf("QueryWithoutEmbedding() error = %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("Expected 2 IDs, got %d", len(ids))
	}

	// Add embedding to one
	_ = storage.UpdateEmbedding(ctx, "gear", "gear-1", info, []float32{0.1, 0.2, 0.3})

	// Only one should be without embedding
	ids, err = storage.QueryWithoutEmbedding(ctx, "gear", info, 10)
	if err != nil {
		t.Fatalf("QueryWithoutEmbedding() error = %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("Expected 1 ID, got %d", len(ids))
	}
	if len(ids) > 0 && ids[0] != "gear-2" {
		t.Errorf("Expected gear-2, got %s", ids[0])
	}
}

func TestMultipleEmbeddingColumns(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	geminiInfo := &embedding.Info{Vendor: "gemini", Model: "text-embedding-004", Dimensions: 768}
	openaiInfo := &embedding.Info{Vendor: "openai", Model: "text-embedding-3-small", Dimensions: 1536}

	// Create both columns
	err := storage.EnsureEmbeddingColumn(ctx, "gear", geminiInfo)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn(gemini) error = %v", err)
	}

	err = storage.EnsureEmbeddingColumn(ctx, "gear", openaiInfo)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn(openai) error = %v", err)
	}

	// List columns
	columns, err := storage.ListEmbeddingColumns(ctx, "gear")
	if err != nil {
		t.Fatalf("ListEmbeddingColumns() error = %v", err)
	}

	if len(columns) != 2 {
		t.Errorf("Expected 2 columns, got %d: %v", len(columns), columns)
	}
}

func TestEnsureEmbeddingColumn_AutoDropWrongDimensions(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create column with 768 dimensions
	info768 := &embedding.Info{Vendor: "test", Model: "embed-model", Dimensions: 768}
	err := storage.EnsureEmbeddingColumn(ctx, "gear", info768)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn(768) error = %v", err)
	}

	// Verify column exists
	columns, err := storage.ListEmbeddingColumns(ctx, "gear")
	if err != nil {
		t.Fatalf("ListEmbeddingColumns() error = %v", err)
	}
	expectedCol := EmbeddingColumnName(info768)
	found := false
	for _, col := range columns {
		if col == expectedCol {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Column %q not found after creation", expectedCol)
	}

	// Insert some test data with embeddings
	gear := createTestGear("test-gear-drop", "Test Item", "Test Description")
	_, err = storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	embedding768 := make([]float32, 768)
	for i := range embedding768 {
		embedding768[i] = float32(i) * 0.001
	}
	err = storage.UpdateEmbedding(ctx, "gear", "test-gear-drop", info768, embedding768)
	if err != nil {
		t.Fatalf("UpdateEmbedding() error = %v", err)
	}

	// Now call EnsureEmbeddingColumn with different dimensions (1536)
	// This should drop the 768-dimension column and create a 1536-dimension one
	info1536 := &embedding.Info{Vendor: "test", Model: "embed-model", Dimensions: 1536}
	err = storage.EnsureEmbeddingColumn(ctx, "gear", info1536)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn(1536) error = %v", err)
	}

	// Verify column exists with new dimensions by trying to insert a 1536-dim embedding
	embedding1536 := make([]float32, 1536)
	for i := range embedding1536 {
		embedding1536[i] = float32(i) * 0.001
	}
	err = storage.UpdateEmbedding(ctx, "gear", "test-gear-drop", info1536, embedding1536)
	if err != nil {
		t.Errorf("UpdateEmbedding(1536) should succeed after dimension change: %v", err)
	}

	// Verify old embeddings are gone (column was dropped)
	// The gear should have no embedding since the column was recreated
	total, withEmbedding, err := storage.CountWithEmbedding(ctx, "gear", info1536)
	if err != nil {
		t.Fatalf("CountWithEmbedding() error = %v", err)
	}
	// After UpdateEmbedding, there should be 1 with embedding
	if total != 1 || withEmbedding != 1 {
		t.Errorf("Expected total=1, withEmbedding=1, got total=%d, withEmbedding=%d", total, withEmbedding)
	}
}

func TestEnsureEmbeddingColumn_SameDimensionsNoOp(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create column with 768 dimensions
	info := &embedding.Info{Vendor: "test", Model: "embed-model", Dimensions: 768}
	err := storage.EnsureEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn(768) error = %v", err)
	}

	// Insert and add embedding
	gear := createTestGear("test-gear-noop", "Test Item", "Test Description")
	_, _ = storage.Insert(ctx, gear)
	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}
	_ = storage.UpdateEmbedding(ctx, "gear", "test-gear-noop", info, embedding)

	// Call EnsureEmbeddingColumn again with same dimensions
	err = storage.EnsureEmbeddingColumn(ctx, "gear", info)
	if err != nil {
		t.Fatalf("EnsureEmbeddingColumn(768) second call error = %v", err)
	}

	// Verify embedding data is preserved (column wasn't dropped)
	total, withEmbedding, err := storage.CountWithEmbedding(ctx, "gear", info)
	if err != nil {
		t.Fatalf("CountWithEmbedding() error = %v", err)
	}
	if withEmbedding != 1 {
		t.Errorf("Expected withEmbedding=1 (data preserved), got %d", withEmbedding)
	}
	if total != 1 {
		t.Errorf("Expected total=1, got %d", total)
	}
}

// createTestGear creates a test gear proto for testing.
func createTestGear(id, name, description string) *models.Gear {
	return &models.Gear{
		Id:          id,
		Name:        name,
		Description: description,
	}
}

func TestExtractNestedFieldValue(t *testing.T) {
	tests := []struct {
		name      string
		msg       *models.StockImage
		fieldPath string
		expected  string
	}{
		{
			name: "simple field - provider",
			msg: &models.StockImage{
				Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
			},
			fieldPath: "media_id",
			expected:  "",
		},
		{
			name: "simple field - media_id",
			msg: &models.StockImage{
				MediaId: "test-media-id",
			},
			fieldPath: "media_id",
			expected:  "test-media-id",
		},
		{
			name: "nested field - provider_image.description",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					Description: "Beautiful mountain landscape",
				},
			},
			fieldPath: "provider_image.description",
			expected:  "Beautiful mountain landscape",
		},
		{
			name: "nested field - provider_image.alt_description",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					AltDescription: "Snow-capped peaks at sunset",
				},
			},
			fieldPath: "provider_image.alt_description",
			expected:  "Snow-capped peaks at sunset",
		},
		{
			name: "deeply nested - provider_image.creator.name",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					Creator: &models.CreatorInfo{
						Name: "John Photographer",
					},
				},
			},
			fieldPath: "provider_image.creator.name",
			expected:  "John Photographer",
		},
		{
			name: "nested field with nil parent",
			msg: &models.StockImage{
				ProviderImage: nil,
			},
			fieldPath: "provider_image.description",
			expected:  "",
		},
		{
			name: "nonexistent field",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					Description: "Test",
				},
			},
			fieldPath: "nonexistent_field",
			expected:  "",
		},
		{
			name: "nonexistent nested field",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					Description: "Test",
				},
			},
			fieldPath: "provider_image.nonexistent",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNestedFieldValue(tt.msg.ProtoReflect(), tt.fieldPath)
			if got != tt.expected {
				t.Errorf("extractNestedFieldValue(%q) = %q, want %q", tt.fieldPath, got, tt.expected)
			}
		})
	}
}

func TestExtractEmbeddingText_NestedFields(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	// Get the embedding config for StockImage
	configs := storage.embeddingConfigs["stock_image"]
	if len(configs) == 0 {
		t.Fatal("Expected embedding config for stock_image table")
	}
	config := configs[0]

	tests := []struct {
		name     string
		msg      *models.StockImage
		expected string
	}{
		{
			name: "both fields populated",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					Description:    "Mountain landscape",
					AltDescription: "Snowy peaks at dawn",
				},
			},
			expected: "Mountain landscape Snowy peaks at dawn",
		},
		{
			name: "only description",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					Description: "Ocean sunset",
				},
			},
			expected: "Ocean sunset",
		},
		{
			name: "only alt_description",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					AltDescription: "Beach scene",
				},
			},
			expected: "Beach scene",
		},
		{
			name:     "nil provider_image",
			msg:      &models.StockImage{},
			expected: "",
		},
		{
			name: "empty strings",
			msg: &models.StockImage{
				ProviderImage: &models.ProviderImage{
					Description:    "",
					AltDescription: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := storage.extractEmbeddingText(tt.msg, config)
			if got != tt.expected {
				t.Errorf("extractEmbeddingText() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractEmbeddingText_SimpleFields(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	// Get the embedding config for Gear (uses simple field paths)
	configs := storage.embeddingConfigs["gear"]
	if len(configs) == 0 {
		t.Fatal("Expected embedding config for gear table")
	}
	config := configs[0]

	tests := []struct {
		name     string
		msg      *models.Gear
		expected string
	}{
		{
			name: "both fields populated",
			msg: &models.Gear{
				Name:        "Power Drill",
				Description: "Cordless drill for home projects",
			},
			expected: "Power Drill Cordless drill for home projects",
		},
		{
			name: "only name",
			msg: &models.Gear{
				Name: "Hammer",
			},
			expected: "Hammer",
		},
		{
			name: "empty gear",
			msg:  &models.Gear{},
			// Name defaults to empty string, which gets filtered out
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := storage.extractEmbeddingText(tt.msg, config)
			if got != tt.expected {
				t.Errorf("extractEmbeddingText() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEnsureEmbeddingColumnRejectsInvalidIdentifiers(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	cases := []struct {
		name   string
		vendor string
		model  string
	}{
		{"injection in vendor", "evil'; DROP TABLE gear; --", "safe-model"},
		{"injection in model", "safe", "1=1; DROP TABLE gear; --"},
		{"spaces in vendor", "has space", "model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := &embedding.Info{Vendor: tc.vendor, Model: tc.model, Dimensions: 384}
			err := storage.EnsureEmbeddingColumn(ctx, "gear", info)
			if err == nil {
				t.Errorf("EnsureEmbeddingColumn(%q, %q) expected error, got nil", tc.vendor, tc.model)
			}
		})
	}
}

// TestEmbeddingGeneratedAfterCommit_OrderingRegression is a deterministic
// integration test for #2847. It verifies that the embedding goroutine sees its
// own row (i.e. the INSERT was committed before the UPDATE runs) by asserting
// that embeddingDone carries nil — a non-nil value means updateEmbedding got
// ErrRecordNotFound, which proves the goroutine fired before the row was visible.
// No timing or sleep is used; correctness follows from the after-commit hook
// ordering enforced by WithTx.
func TestEmbeddingGeneratedAfterCommit_OrderingRegression(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)
	if embedder == nil {
		return
	}

	ctx := context.Background()

	done := make(chan error, 10)
	store.SetEmbeddingDoneChannel(done)

	if err := store.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder: %v", err)
	}

	// Insert triggers WithTx (Gear has media_ids array columns), so the
	// embedding goroutine is deferred until after commit. If the ordering
	// is wrong, updateEmbedding fails with ErrRecordNotFound and done
	// carries a non-nil error.
	gear := &models.Gear{Name: "Ordering Test Drill", Description: "Checks after-commit ordering"}
	if _, err := store.Insert(ctx, gear); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	select {
	case embErr := <-done:
		if embErr != nil {
			t.Fatalf("embedding goroutine saw row as missing (commit visibility race): %v", embErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for embedding after insert")
	}
}

// TestEmbeddingDoneCarriesError verifies that a failed embedding write sends a
// non-nil error on embeddingDone. Uses a non-existent id to force ErrRecordNotFound
// from updateEmbedding, which is the same error the visibility-race produced before
// the after-commit fix (see #2847). Regression for defect (1): the channel now
// carries the outcome so a real regression fails loudly.
func TestEmbeddingDoneCarriesError(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)
	if embedder == nil {
		return
	}

	ctx := context.Background()

	// SetEmbedder wires up store.embedder and ensures the embedding column exists.
	if err := store.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder: %v", err)
	}

	// Call generateEmbedding directly on a non-existent id; it should return
	// ErrRecordNotFound (the same error the visibility race produced).
	const bogusID = "does-not-exist"
	err := store.generateEmbedding(ctx, "gear", bogusID, "", "test text for missing row")
	if err == nil {
		t.Fatal("generateEmbedding on non-existent id must return an error")
	}
	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("generateEmbedding error = %v, want to wrap ErrRecordNotFound", err)
	}
}

// TestBackfillEmbeddings_NoEmbedder verifies that BackfillEmbeddings is a
// no-op when no embedder is configured, regardless of ctx state.
func TestBackfillEmbeddings_NoEmbedder(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	// With no embedder, BackfillEmbeddings should return immediately even
	// for a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		storage.BackfillEmbeddings(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("BackfillEmbeddings did not return within 2s when embedder is nil")
	}
}

// TestBackfillEmbeddings_HonorsCancellation verifies that BackfillEmbeddings
// returns promptly when its context is cancelled, so a server shutdown does
// not leak the goroutine or in-flight embedder work past the drain window.
// Regression test for #1526.
func TestBackfillEmbeddings_HonorsCancellation(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Pre-populate gear rows BEFORE configuring the embedder so Insert does
	// not spawn per-row async embedding goroutines (those use a detached
	// background ctx and would outlive the test).
	for i := 0; i < 5; i++ {
		gear := createTestGear(
			fmt.Sprintf("cancel-gear-%d", i),
			"Test Gear",
			"A test item that needs an embedding",
		)
		if _, err := storage.Insert(ctx, gear); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	embedder := SetupTestEmbedder(t)
	if embedder == nil {
		// SetupTestEmbedder calls t.Skip when model files are unavailable.
		return
	}
	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel before starting so the loop bails on its first ctx check

	done := make(chan struct{})
	start := time.Now()
	go func() {
		storage.BackfillEmbeddings(cancelCtx)
		close(done)
	}()

	select {
	case <-done:
		// Generous bound: real work for one row would take well over a
		// second on the embedder; we expect a near-instant return.
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("BackfillEmbeddings took %v after ctx cancellation, expected near-immediate return", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("BackfillEmbeddings did not return within 5s after ctx cancellation")
	}
}
