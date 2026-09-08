package momentum

import (
	"fmt"
	"strings"
)

// MaxLeverWords is the maximum word count for a workshop lever string.
const MaxLeverWords = 9

// ValidateLeverCopy enforces the lever-copy writing rule:
// lever text must be non-empty and ≤9 words.
func ValidateLeverCopy(text string) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return fmt.Errorf("lever text is empty")
	}
	words := strings.Fields(trimmed)
	if len(words) > MaxLeverWords {
		return fmt.Errorf("lever has %d words (max %d): %q", len(words), MaxLeverWords, trimmed)
	}
	return nil
}
