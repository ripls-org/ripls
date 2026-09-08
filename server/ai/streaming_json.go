// Incremental JSON-object parser that reports a FieldEvent the moment a
// named top-level key's value closes on the wire, without waiting for the
// full object to finish. Providers append bytes as the model emits them;
// downstream consumers fire work the instant a watched field's value is
// complete. Supports scalars (strings, numbers, bools, null), arrays, and
// nested objects; value bytes are returned verbatim as json.RawMessage so
// callers unmarshal into their own typed target.
package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// FieldEvent reports that a watched top-level key in the streaming JSON
// object has its value fully closed. Value carries the raw JSON bytes of the
// value as it appeared on the wire (e.g., `"hello"`, `["a","b"]`, `42`).
// Callers unmarshal Value with json.Unmarshal into a typed target.
type FieldEvent struct {
	Key   StreamFieldKey
	Value json.RawMessage
}

// StreamingJSONFields is an incremental parser for the top-level keys of a
// single JSON object. AI tool/structured-output streaming (Anthropic
// input_json_delta, OpenAI Choices[*].Delta.Content, Gemini chunk.Text)
// emits the JSON object character-by-character over many events. This
// parser feeds those fragments in, and emits a FieldEvent the moment a
// watched key's value closes — which lets downstream work (Mapbox geocode,
// Pexels image fetch) start ~1 second earlier than waiting for the full
// response.
//
// Implementation: each Write attempts to parse as many `"key":value` pairs
// as the accumulated buffer allows, by spinning up a fresh json.Decoder
// over the unconsumed slice and advancing a cursor on success. When the
// Decoder returns io.ErrUnexpectedEOF (value mid-stream) the cursor stays
// put and the next Write retries from the same position. This delegates
// all the structural parsing — strings, escapes, surrogate pairs, scalars,
// nested objects/arrays — to the standard library; the only state we own
// is the cursor and a few flags.
//
// Not safe for concurrent use.
type StreamingJSONFields struct {
	watch map[string]struct{}

	buf     []byte
	cursor  int  // next byte to scan
	started bool // saw opening '{'
	closed  bool // saw closing '}'
	err     error
}

// NewStreamingJSONFields constructs a parser that emits events for the
// given top-level keys. Unwatched keys are still parsed (so structural
// correctness is enforced) but produce no events.
func NewStreamingJSONFields(watch []string) *StreamingJSONFields {
	m := make(map[string]struct{}, len(watch))
	for _, k := range watch {
		m[k] = struct{}{}
	}
	return &StreamingJSONFields{watch: m}
}

// Write appends bytes to the parse buffer and runs the state machine
// forward to the new end. Any watched-key value-close events that occur
// are returned in order. A non-nil error means the input is structurally
// invalid; the parser stops scanning further bytes after an error
// (subsequent Write calls return the same error).
func (p *StreamingJSONFields) Write(b []byte) ([]FieldEvent, error) {
	if p.err != nil {
		return nil, p.err
	}
	p.buf = append(p.buf, b...)

	var events []FieldEvent
	for !p.closed {
		p.cursor = skipJSONInterstitial(p.buf, p.cursor)
		if p.cursor >= len(p.buf) {
			return events, nil
		}

		if !p.started {
			if p.buf[p.cursor] != '{' {
				p.err = fmt.Errorf("streaming_json: expected '{', got %q", p.buf[p.cursor])
				return events, p.err
			}
			p.started = true
			p.cursor++
			continue
		}

		if p.buf[p.cursor] == '}' {
			p.cursor++
			p.closed = true
			return events, nil
		}

		// Expect a `"key":value` pair starting at cursor. Prepend a synthetic
		// '{' so the Decoder enters object-member mode; without it, Decode
		// after the key Token would error on the colon (the Decoder would
		// be in top-of-stream mode where '"key":value' isn't a single value
		// — `:` after a string is invalid). dec.InputOffset() then reports
		// position in the multi-reader, which is one ahead of the position
		// in p.buf (the leading synthetic byte).
		dec := json.NewDecoder(io.MultiReader(
			bytes.NewReader([]byte{'{'}),
			bytes.NewReader(p.buf[p.cursor:]),
		))
		if _, err := dec.Token(); err != nil { // consume synthetic '{'
			p.err = fmt.Errorf("streaming_json: internal: %w", err)
			return events, p.err
		}

		keyTok, err := dec.Token()
		if isIncompleteJSON(err) {
			return events, nil
		}
		if err != nil {
			p.err = fmt.Errorf("streaming_json: parse key: %w", err)
			return events, p.err
		}
		key, ok := keyTok.(string)
		if !ok {
			p.err = fmt.Errorf("streaming_json: expected string key, got %v", keyTok)
			return events, p.err
		}

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if isIncompleteJSON(err) {
				return events, nil
			}
			p.err = fmt.Errorf("streaming_json: parse value for key %q: %w", key, err)
			return events, p.err
		}

		valueEnd := p.cursor + int(dec.InputOffset()) - 1 // subtract the synthetic '{'

		// For scalar values (number, true, false, null) we need to see at
		// least one terminator byte (`,`, `}`, whitespace) past the parsed
		// value before committing. Without it, the Decoder may have
		// declared a partial scalar complete because bytes.Reader hit EOF
		// — e.g., `0.8` parses successfully even though `0.85` was on the
		// wire. Strings, arrays, and objects have explicit closing
		// characters so they are unambiguous once Decode succeeds.
		if isScalarRawValue(raw) {
			if valueEnd >= len(p.buf) || !isJSONValueTerminator(p.buf[valueEnd]) {
				return events, nil
			}
		}

		p.cursor = valueEnd

		if _, watched := p.watch[key]; watched {
			// Copy raw — Decoder buffers may be reused across calls; the
			// caller may hold the slice past the next Write.
			value := make(json.RawMessage, len(raw))
			copy(value, raw)
			events = append(events, FieldEvent{Key: StreamFieldKey(key), Value: value})
		}
	}

	return events, nil
}

// Bytes returns all bytes accumulated so far. Useful for the unary code
// path that still needs to unmarshal the complete document.
func (p *StreamingJSONFields) Bytes() []byte {
	return p.buf
}

// Closed reports whether the top-level object has fully closed.
func (p *StreamingJSONFields) Closed() bool {
	return p.closed
}

// skipJSONInterstitial advances past whitespace and a single field-
// separator comma, in either order. Lenient to repeated whitespace; a
// stray second comma would land on a non-key character and surface as a
// parse error on the next iteration.
func skipJSONInterstitial(b []byte, i int) int {
	sawComma := false
	for i < len(b) {
		c := b[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == ',' && !sawComma:
			sawComma = true
			i++
		default:
			return i
		}
	}
	return i
}

func isIncompleteJSON(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// isJSONValueTerminator reports whether c is a byte that legally follows a
// JSON value within an object: structural separator (`,`, `}`, `]`) or
// whitespace.
func isJSONValueTerminator(c byte) bool {
	switch c {
	case ',', '}', ']', ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// isScalarRawValue reports whether the raw JSON value bytes look like a
// scalar (number, true, false, null) rather than a string, array, or
// object. Scalars are the only value shape whose extent isn't disambiguated
// by an explicit closing character, so they are the only shape that needs
// the lookahead-terminator check above.
func isScalarRawValue(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	switch raw[0] {
	case '"', '[', '{':
		return false
	}
	return true
}
