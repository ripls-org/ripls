// Tests for the StreamingJSONFields parser: covers scalar and composite
// values, nested objects, byte-by-byte chunking that mirrors real provider
// streams, and error paths.
package ai

import (
	"encoding/json"
	"testing"
)

// TestStreamingJSONFields_FullObjectsOneShot confirms the parser produces
// the same key-value events whether bytes arrive in one Write or split byte
// by byte. The split case is the realistic provider behavior.
func TestStreamingJSONFields_FullObjectsOneShot(t *testing.T) {
	tests := []struct {
		name  string
		input string
		watch []string
		want  []FieldEvent
	}{
		{
			name:  "single string key",
			input: `{"title":"hello"}`,
			watch: []string{"title"},
			want:  []FieldEvent{{Key: "title", Value: json.RawMessage(`"hello"`)}},
		},
		{
			name:  "two string keys, both watched",
			input: `{"title":"hi","location_query":"Boulder, CO"}`,
			watch: []string{"title", "location_query"},
			want: []FieldEvent{
				{Key: "title", Value: json.RawMessage(`"hi"`)},
				{Key: "location_query", Value: json.RawMessage(`"Boulder, CO"`)},
			},
		},
		{
			name:  "unwatched keys ignored",
			input: `{"title":"hi","ignored":"x","location_query":"y"}`,
			watch: []string{"title", "location_query"},
			want: []FieldEvent{
				{Key: "title", Value: json.RawMessage(`"hi"`)},
				{Key: "location_query", Value: json.RawMessage(`"y"`)},
			},
		},
		{
			name:  "array value",
			input: `{"search_keywords":["pickleball","court","sport"]}`,
			watch: []string{"search_keywords"},
			want:  []FieldEvent{{Key: "search_keywords", Value: json.RawMessage(`["pickleball","court","sport"]`)}},
		},
		{
			name:  "scalar value followed by string",
			input: `{"confidence":0.85,"title":"hi"}`,
			watch: []string{"confidence", "title"},
			want: []FieldEvent{
				{Key: "confidence", Value: json.RawMessage(`0.85`)},
				{Key: "title", Value: json.RawMessage(`"hi"`)},
			},
		},
		{
			name:  "scalar boolean and null",
			input: `{"a":true,"b":false,"c":null}`,
			watch: []string{"a", "b", "c"},
			want: []FieldEvent{
				{Key: "a", Value: json.RawMessage(`true`)},
				{Key: "b", Value: json.RawMessage(`false`)},
				{Key: "c", Value: json.RawMessage(`null`)},
			},
		},
		{
			name:  "nested object value",
			input: `{"value_estimate":{"estimated_value_usd":42,"confidence":0.9}}`,
			watch: []string{"value_estimate"},
			want:  []FieldEvent{{Key: "value_estimate", Value: json.RawMessage(`{"estimated_value_usd":42,"confidence":0.9}`)}},
		},
		{
			name:  "string with escaped quote",
			input: `{"title":"she said \"hi\""}`,
			watch: []string{"title"},
			want:  []FieldEvent{{Key: "title", Value: json.RawMessage(`"she said \"hi\""`)}},
		},
		{
			name:  "string with unicode escape",
			input: `{"title":"café"}`,
			watch: []string{"title"},
			want:  []FieldEvent{{Key: "title", Value: json.RawMessage(`"café"`)}},
		},
		{
			name:  "string with literal backslash",
			input: `{"path":"a\\b"}`,
			watch: []string{"path"},
			want:  []FieldEvent{{Key: "path", Value: json.RawMessage(`"a\\b"`)}},
		},
		{
			name:  "array of objects",
			input: `{"items":[{"id":1},{"id":2}]}`,
			watch: []string{"items"},
			want:  []FieldEvent{{Key: "items", Value: json.RawMessage(`[{"id":1},{"id":2}]`)}},
		},
		{
			name:  "nested array with string containing brackets",
			input: `{"k":["a]b","c[d"]}`,
			watch: []string{"k"},
			want:  []FieldEvent{{Key: "k", Value: json.RawMessage(`["a]b","c[d"]`)}},
		},
		{
			name:  "scalar at end of object (no trailing comma)",
			input: `{"title":"hi","confidence":0.5}`,
			watch: []string{"confidence"},
			want:  []FieldEvent{{Key: "confidence", Value: json.RawMessage(`0.5`)}},
		},
		{
			name:  "leading whitespace tolerated",
			input: "  \n\t" + `{"title":"hi"}`,
			watch: []string{"title"},
			want:  []FieldEvent{{Key: "title", Value: json.RawMessage(`"hi"`)}},
		},
		{
			name:  "whitespace around colon and comma",
			input: `{"title" : "hi" , "confidence" : 0.5 }`,
			watch: []string{"title", "confidence"},
			want: []FieldEvent{
				{Key: "title", Value: json.RawMessage(`"hi"`)},
				{Key: "confidence", Value: json.RawMessage(`0.5`)},
			},
		},
		{
			name:  "empty object as value",
			input: `{"meta":{}}`,
			watch: []string{"meta"},
			want:  []FieldEvent{{Key: "meta", Value: json.RawMessage(`{}`)}},
		},
		{
			name:  "empty array as value",
			input: `{"tags":[]}`,
			watch: []string{"tags"},
			want:  []FieldEvent{{Key: "tags", Value: json.RawMessage(`[]`)}},
		},
		{
			name:  "empty string as value",
			input: `{"title":""}`,
			watch: []string{"title"},
			want:  []FieldEvent{{Key: "title", Value: json.RawMessage(`""`)}},
		},
		{
			name:  "negative number scalar",
			input: `{"delta":-3.14}`,
			watch: []string{"delta"},
			want:  []FieldEvent{{Key: "delta", Value: json.RawMessage(`-3.14`)}},
		},
		{
			name:  "scientific notation",
			input: `{"big":1.2e10}`,
			watch: []string{"big"},
			want:  []FieldEvent{{Key: "big", Value: json.RawMessage(`1.2e10`)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/oneshot", func(t *testing.T) {
			p := NewStreamingJSONFields(tt.watch)
			got, err := p.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertEvents(t, got, tt.want)
			if !p.Closed() {
				t.Errorf("parser not marked closed after full input")
			}
		})

		t.Run(tt.name+"/byte-by-byte", func(t *testing.T) {
			p := NewStreamingJSONFields(tt.watch)
			var got []FieldEvent
			for i := 0; i < len(tt.input); i++ {
				ev, err := p.Write([]byte{tt.input[i]})
				if err != nil {
					t.Fatalf("unexpected error at byte %d: %v", i, err)
				}
				got = append(got, ev...)
			}
			assertEvents(t, got, tt.want)
			if !p.Closed() {
				t.Errorf("parser not marked closed after byte-by-byte input")
			}
		})
	}
}

// TestStreamingJSONFields_ChunkedAtKeyBoundaries confirms that splits between
// the key, colon, value-start, and inside the value all parse correctly. This
// mimics the realistic Anthropic delta granularity (~5–40 chars per delta).
func TestStreamingJSONFields_ChunkedAtKeyBoundaries(t *testing.T) {
	chunks := []string{
		`{"ti`,
		`tle":`,
		`"hello`,
		` world",`,
		`"location_query":"Bo`,
		`ulder, CO","search_keywords":["a"`,
		`,"b","c"]}`,
	}
	p := NewStreamingJSONFields([]string{"title", "location_query", "search_keywords"})
	var got []FieldEvent
	for _, c := range chunks {
		ev, err := p.Write([]byte(c))
		if err != nil {
			t.Fatalf("unexpected error on chunk %q: %v", c, err)
		}
		got = append(got, ev...)
	}
	want := []FieldEvent{
		{Key: "title", Value: json.RawMessage(`"hello world"`)},
		{Key: "location_query", Value: json.RawMessage(`"Boulder, CO"`)},
		{Key: "search_keywords", Value: json.RawMessage(`["a","b","c"]`)},
	}
	assertEvents(t, got, want)
	if !p.Closed() {
		t.Errorf("parser not marked closed")
	}
}

// TestStreamingJSONFields_EmissionOrderMatchesArrival proves the parser
// emits each event the moment its value closes, not at the end. This is the
// behavior the post-AI fan-out depends on: title closes early, Pexels can
// fire before search_keywords; search_keywords closes mid-stream, Mapbox can
// fire before final.
func TestStreamingJSONFields_EmissionOrderMatchesArrival(t *testing.T) {
	p := NewStreamingJSONFields([]string{"title", "location_query", "search_keywords"})

	ev1, err := p.Write([]byte(`{"title":"hi",`))
	if err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if len(ev1) != 1 || ev1[0].Key != "title" {
		t.Fatalf("expected title event after first chunk, got %+v", ev1)
	}

	ev2, err := p.Write([]byte(`"location_query":"Boulder",`))
	if err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if len(ev2) != 1 || ev2[0].Key != "location_query" {
		t.Fatalf("expected location_query event after second chunk, got %+v", ev2)
	}

	ev3, err := p.Write([]byte(`"search_keywords":["a","b"]`))
	if err != nil {
		t.Fatalf("write 3: %v", err)
	}
	if len(ev3) != 1 || ev3[0].Key != "search_keywords" {
		t.Fatalf("expected search_keywords event after third chunk, got %+v", ev3)
	}

	ev4, err := p.Write([]byte(`}`))
	if err != nil {
		t.Fatalf("write 4: %v", err)
	}
	if len(ev4) != 0 {
		t.Errorf("expected no events on closing brace, got %+v", ev4)
	}
	if !p.Closed() {
		t.Errorf("parser not marked closed")
	}
}

// TestStreamingJSONFields_TruncatedInputNoEventNoError mirrors the
// MaxTokens-truncation case: Claude cuts off mid-string, parser sees a
// dangling open string. We don't emit an event for the incomplete value, but
// we also don't return an error — partial state is a normal terminal state
// for streaming.
func TestStreamingJSONFields_TruncatedInputNoEventNoError(t *testing.T) {
	p := NewStreamingJSONFields([]string{"title", "location_query"})
	got, err := p.Write([]byte(`{"title":"hi","location_query":"Boul`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Key != "title" {
		t.Fatalf("expected only title event, got %+v", got)
	}
	if p.Closed() {
		t.Errorf("parser should not be closed on truncated input")
	}
}

// TestStreamingJSONFields_RawValueRoundTrips confirms the emitted Value bytes
// are valid JSON and unmarshal cleanly into typed targets — the contract
// downstream code relies on.
func TestStreamingJSONFields_RawValueRoundTrips(t *testing.T) {
	p := NewStreamingJSONFields([]string{"title", "search_keywords", "confidence"})
	events, err := p.Write([]byte(`{"title":"café","search_keywords":["a","b"],"confidence":0.42}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	var title string
	if err := json.Unmarshal(events[0].Value, &title); err != nil {
		t.Fatalf("unmarshal title: %v", err)
	}
	if title != "café" {
		t.Errorf("title = %q, want café", title)
	}

	var keywords []string
	if err := json.Unmarshal(events[1].Value, &keywords); err != nil {
		t.Fatalf("unmarshal keywords: %v", err)
	}
	if len(keywords) != 2 || keywords[0] != "a" || keywords[1] != "b" {
		t.Errorf("keywords = %v, want [a b]", keywords)
	}

	var confidence float64
	if err := json.Unmarshal(events[2].Value, &confidence); err != nil {
		t.Fatalf("unmarshal confidence: %v", err)
	}
	if confidence != 0.42 {
		t.Errorf("confidence = %v, want 0.42", confidence)
	}
}

// TestStreamingJSONFields_BytesAccumulates confirms the buffer is preserved
// for the unary code path.
func TestStreamingJSONFields_BytesAccumulates(t *testing.T) {
	p := NewStreamingJSONFields(nil)
	if _, err := p.Write([]byte(`{"a":1`)); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if _, err := p.Write([]byte(`,"b":2}`)); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	got := string(p.Bytes())
	want := `{"a":1,"b":2}`
	if got != want {
		t.Errorf("Bytes() = %q, want %q", got, want)
	}
}

// TestStreamingJSONFields_StructuralErrors covers malformed inputs.
func TestStreamingJSONFields_StructuralErrors(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"missing colon", `{"a" "x"}`},
		{"junk before object", `xx{"a":1}`},
		{"unexpected char between fields", `{"a":1 x "b":2}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewStreamingJSONFields([]string{"a", "b"})
			_, err := p.Write([]byte(tc.input))
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", tc.input)
			}
		})
	}
}

func assertEvents(t *testing.T, got, want []FieldEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count: got %d, want %d\ngot:  %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i].Key != want[i].Key {
			t.Errorf("event %d key: got %q, want %q", i, got[i].Key, want[i].Key)
		}
		if string(got[i].Value) != string(want[i].Value) {
			t.Errorf("event %d value: got %s, want %s", i, string(got[i].Value), string(want[i].Value))
		}
	}
}
