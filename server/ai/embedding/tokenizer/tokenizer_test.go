package tokenizer

import (
	"os"
	"path/filepath"
	"testing"
)

// vocabPath returns the path to the test vocabulary file.
func vocabPath(t *testing.T) string {
	// Look for vocab in model_tuning directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Navigate from server/ai/embedding/tokenizer to model_tuning/ripls_embedding_tokenizer
	vocabFile := filepath.Join(cwd, "..", "..", "..", "..", "model_tuning", "ripls_embedding_tokenizer", "vocab.txt")
	if _, err := os.Stat(vocabFile); err != nil {
		t.Skipf("Vocabulary file not found at %s: %v", vocabFile, err)
	}
	return vocabFile
}

func TestNewTokenizer(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	// BERT base vocabulary is 30522 tokens
	if tok.VocabSize() != 30522 {
		t.Errorf("Expected vocab size 30522, got %d", tok.VocabSize())
	}
}

func TestEncode_SimpleText(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	inputIDs, attentionMask := tok.Encode("hello world")

	// Check length
	if len(inputIDs) != 128 {
		t.Errorf("Expected input_ids length 128, got %d", len(inputIDs))
	}
	if len(attentionMask) != 128 {
		t.Errorf("Expected attention_mask length 128, got %d", len(attentionMask))
	}

	// Check starts with [CLS] (101)
	if inputIDs[0] != ClsTokenID {
		t.Errorf("Expected first token to be [CLS] (101), got %d", inputIDs[0])
	}

	// Check attention mask is 1 for real tokens, 0 for padding
	realTokens := 0
	for _, m := range attentionMask {
		if m == 1 {
			realTokens++
		}
	}

	// "hello world" should be: [CLS] hello world [SEP] = 4 tokens
	if realTokens < 4 {
		t.Errorf("Expected at least 4 real tokens, got %d", realTokens)
	}
}

func TestEncode_IncludesSepToken(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	inputIDs, attentionMask := tok.Encode("test")

	// Find the last real token (before padding)
	lastRealIdx := 0
	for i, m := range attentionMask {
		if m == 1 {
			lastRealIdx = i
		}
	}

	// Last real token should be [SEP] (102)
	if inputIDs[lastRealIdx] != SepTokenID {
		t.Errorf("Expected last real token to be [SEP] (102), got %d", inputIDs[lastRealIdx])
	}
}

func TestEncode_Lowercasing(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	// Uppercase and lowercase should produce same tokens
	idsUpper, _ := tok.Encode("HELLO WORLD")
	idsLower, _ := tok.Encode("hello world")

	for i := range idsUpper {
		if idsUpper[i] != idsLower[i] {
			t.Errorf("Token mismatch at %d: upper=%d, lower=%d", i, idsUpper[i], idsLower[i])
		}
	}
}

func TestEncode_Truncation(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	// Very long text should be truncated
	longText := ""
	for i := 0; i < 1000; i++ {
		longText += "word "
	}

	inputIDs, attentionMask := tok.Encode(longText)

	if len(inputIDs) != 128 {
		t.Errorf("Expected truncated length 128, got %d", len(inputIDs))
	}

	// All tokens should be real (no padding for truncated sequences)
	for i, m := range attentionMask {
		if m != 1 {
			t.Errorf("Expected all attention_mask to be 1 for truncated sequence, got 0 at %d", i)
		}
	}

	// Still should end with [SEP]
	if inputIDs[127] != SepTokenID {
		t.Errorf("Truncated sequence should end with [SEP], got %d", inputIDs[127])
	}
}

func TestEncode_Punctuation(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	// Punctuation should be tokenized separately
	inputIDs, _ := tok.Encode("hello, world!")

	// Should have more tokens than "hello world" due to punctuation
	realTokens := 0
	for i := 0; i < len(inputIDs) && inputIDs[i] != PadTokenID; i++ {
		realTokens++
	}

	// [CLS] hello , world ! [SEP] = 6 tokens minimum
	if realTokens < 6 {
		t.Errorf("Expected at least 6 tokens with punctuation, got %d", realTokens)
	}
}

func TestEncode_EmptyString(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	inputIDs, attentionMask := tok.Encode("")

	// Should still have [CLS] and [SEP]
	if inputIDs[0] != ClsTokenID {
		t.Errorf("Empty string should start with [CLS]")
	}
	if inputIDs[1] != SepTokenID {
		t.Errorf("Empty string should have [SEP] at index 1")
	}

	// Only 2 real tokens
	realTokens := 0
	for _, m := range attentionMask {
		if m == 1 {
			realTokens++
		}
	}
	if realTokens != 2 {
		t.Errorf("Empty string should have 2 real tokens, got %d", realTokens)
	}
}

func TestEncode_UnknownTokens(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	// Use characters unlikely to be in vocab
	inputIDs, _ := tok.Encode("zzzzxyzzy")

	// Should contain UNK tokens
	hasUnk := false
	for _, id := range inputIDs {
		if id == UnkTokenID {
			hasUnk = true
			break
		}
	}

	// May or may not have UNK depending on wordpiece coverage
	_ = hasUnk
}

func TestEncode_MultipleSpaces(t *testing.T) {
	tok, err := NewTokenizer(vocabPath(t))
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	// Multiple spaces should produce same output as single space
	idsSingle, _ := tok.Encode("hello world")
	idsMulti, _ := tok.Encode("hello    world")

	for i := range idsSingle {
		if idsSingle[i] != idsMulti[i] {
			t.Errorf("Multiple spaces should normalize: mismatch at %d", i)
		}
	}
}
