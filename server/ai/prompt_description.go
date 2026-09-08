package ai

import (
	"strings"
	"unicode/utf8"
)

// DescriptionWithoutTitleClause removes a leading title clause from a
// prompt-derived description.
//
// The text-mode create streams emit the user's prompt verbatim as the
// description the instant it arrives, before the AI has returned a title —
// that immediacy is the point (the client renders something on the first
// frame). But the title the AI then extracts is usually the prompt's opening
// clause, which leaves the content reading with its own heading repeated
// underneath it (#2724).
//
// Once the title lands, callers pass it here and re-emit: the description
// drops the duplicated clause and keeps the rest of what the user typed.
// Matching is deliberately conservative — the clause must appear at the very
// start (case-insensitively), be followed by sentence punctuation or a dash,
// and leave a usable remainder. Anything else returns the description
// unchanged, so a title the AI invented rather than quoted never truncates
// what the user wrote.
func DescriptionWithoutTitleClause(description, title string) string {
	desc := strings.TrimSpace(description)
	t := strings.TrimSpace(title)
	if desc == "" || t == "" {
		return description
	}
	if len(desc) <= len(t) || !strings.EqualFold(desc[:len(t)], t) {
		return description
	}

	rest := strings.TrimLeft(desc[len(t):], " ")
	// The clause must actually END at the title, so a title that is only a
	// prefix of the opening word never strips it. Decode the rune rather than
	// indexing a byte: the separator is often a multi-byte em dash.
	sep, _ := utf8.DecodeRuneInString(rest)
	if rest == "" || !strings.ContainsRune(".!?—–-:", sep) {
		return description
	}
	rest = strings.TrimSpace(strings.TrimLeft(rest, ".!?—–-: "))
	if len(rest) < 10 {
		// Nothing meaningful left — keep the user's text as they wrote it.
		return description
	}
	return rest
}
