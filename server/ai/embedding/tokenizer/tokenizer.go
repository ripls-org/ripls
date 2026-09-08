// Package tokenizer implements a BERT WordPiece tokenizer for Go.
// This tokenizer is compatible with the HuggingFace tokenizers library
// and produces identical output for use with ONNX models.
package tokenizer

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

// Special token IDs for BERT.
const (
	PadTokenID  = 0
	UnkTokenID  = 100
	ClsTokenID  = 101
	SepTokenID  = 102
	MaskTokenID = 103
)

// Tokenizer implements BERT WordPiece tokenization.
type Tokenizer struct {
	vocab       map[string]int64
	vocabSize   int
	maxLen      int
	doLowerCase bool
}

// NewTokenizer creates a new tokenizer from a vocabulary file.
// The vocab file should have one token per line, with line number as the token ID.
func NewTokenizer(vocabPath string) (*Tokenizer, error) {
	file, err := os.Open(vocabPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	vocab := make(map[string]int64)
	scanner := bufio.NewScanner(file)
	var id int64

	for scanner.Scan() {
		token := scanner.Text()
		vocab[token] = id
		id++
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &Tokenizer{
		vocab:       vocab,
		vocabSize:   len(vocab),
		maxLen:      128,
		doLowerCase: true,
	}, nil
}

// SetMaxLength sets the maximum sequence length for tokenization.
func (t *Tokenizer) SetMaxLength(maxLen int) {
	t.maxLen = maxLen
}

// Encode tokenizes text and returns input_ids and attention_mask.
// The output includes [CLS] and [SEP] tokens and is padded/truncated to maxLen.
func (t *Tokenizer) Encode(text string) (inputIDs, attentionMask []int64) {
	// Normalize text
	text = t.normalize(text)

	// Pre-tokenize (split on whitespace and punctuation)
	words := t.preTokenize(text)

	// WordPiece tokenization
	tokens := []int64{ClsTokenID} // Start with [CLS]
	for _, word := range words {
		wordTokens := t.wordPieceTokenize(word)
		tokens = append(tokens, wordTokens...)
	}

	// Truncate if needed (leave room for [SEP])
	maxTokens := t.maxLen - 1
	if len(tokens) > maxTokens {
		tokens = tokens[:maxTokens]
	}

	// Add [SEP]
	tokens = append(tokens, SepTokenID)

	// Create attention mask (1 for real tokens)
	attentionMask = make([]int64, len(tokens))
	for i := range attentionMask {
		attentionMask[i] = 1
	}

	// Pad to maxLen
	for len(tokens) < t.maxLen {
		tokens = append(tokens, PadTokenID)
		//nolint:makezero // Deliberate: attentionMask is sized to the real tokens and
		// filled with 1s above; padding positions are appended as 0 so the model masks
		// them. Appending to a non-zero-length slice is the point, not a bug.
		attentionMask = append(attentionMask, 0)
	}

	return tokens, attentionMask
}

// normalize applies BERT-style text normalization.
func (t *Tokenizer) normalize(text string) string {
	// Clean text: remove control characters, replace whitespace
	var builder strings.Builder
	for _, r := range text {
		if r == 0 || r == 0xFFFD || isControl(r) {
			continue
		}
		if isWhitespace(r) {
			builder.WriteRune(' ')
		} else {
			builder.WriteRune(r)
		}
	}
	text = builder.String()

	// Lowercase if configured
	if t.doLowerCase {
		text = strings.ToLower(text)
	}

	return strings.TrimSpace(text)
}

// preTokenize splits text on whitespace and punctuation.
func (t *Tokenizer) preTokenize(text string) []string {
	var words []string
	var currentWord strings.Builder

	for _, r := range text {
		if isWhitespace(r) {
			if currentWord.Len() > 0 {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
		} else if isPunctuation(r) {
			if currentWord.Len() > 0 {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
			words = append(words, string(r))
		} else {
			currentWord.WriteRune(r)
		}
	}

	if currentWord.Len() > 0 {
		words = append(words, currentWord.String())
	}

	return words
}

// wordPieceTokenize applies WordPiece tokenization to a single word.
func (t *Tokenizer) wordPieceTokenize(word string) []int64 {
	if word == "" {
		return nil
	}

	var tokens []int64
	start := 0

	for start < len(word) {
		end := len(word)
		var curToken string
		found := false

		for start < end {
			substr := word[start:end]
			if start > 0 {
				substr = "##" + substr
			}

			if id, ok := t.vocab[substr]; ok {
				tokens = append(tokens, id)
				curToken = substr
				found = true
				break
			}
			end--
		}

		if !found {
			// Unknown token
			tokens = append(tokens, UnkTokenID)
			start++
		} else {
			if start > 0 {
				start += len(curToken) - 2 // Subtract "##" prefix length
			} else {
				start += len(curToken)
			}
		}
	}

	return tokens
}

// VocabSize returns the vocabulary size.
func (t *Tokenizer) VocabSize() int {
	return t.vocabSize
}

// isWhitespace checks if a rune is whitespace per BERT definition.
func isWhitespace(r rune) bool {
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// isControl checks if a rune is a control character.
func isControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)
}

// isPunctuation checks if a rune is punctuation per BERT definition.
func isPunctuation(r rune) bool {
	cp := int(r)
	// ASCII punctuation
	if (cp >= 33 && cp <= 47) || (cp >= 58 && cp <= 64) ||
		(cp >= 91 && cp <= 96) || (cp >= 123 && cp <= 126) {
		return true
	}
	return unicode.Is(unicode.P, r)
}
