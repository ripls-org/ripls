package eval

import (
	"reflect"
	"strings"
	"testing"
)

// stringSliceFlag is a flag.Value that parses a comma-separated string into
// a slice. It supports a default value applied when the flag is not set:
// callers initialize the slice once at package init, and the flag's Set
// method replaces (does not append to) those defaults on user input.
type stringSliceFlag struct {
	values []string
}

// String implements flag.Value. Returns the comma-joined current values.
func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(s.values, ",")
}

// Set implements flag.Value. Replaces the current values with the parsed
// comma-separated entries from v. Whitespace around each entry is trimmed
// and empty entries are skipped, so "a, b,, c" yields ["a","b","c"].
func (s *stringSliceFlag) Set(v string) error {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	s.values = out
	return nil
}

// Values returns a copy of the parsed slice so callers can iterate without
// risk of accidental mutation.
func (s *stringSliceFlag) Values() []string {
	out := make([]string, len(s.values))
	copy(out, s.values)
	return out
}

func TestStringSliceFlag_SetReplacesDefault(t *testing.T) {
	s := &stringSliceFlag{values: []string{"default-model"}}
	if err := s.Set("model-a,model-b"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"model-a", "model-b"}
	if !reflect.DeepEqual(s.Values(), want) {
		t.Errorf("got %v, want %v", s.Values(), want)
	}
}

func TestStringSliceFlag_TrimsAndSkipsEmpties(t *testing.T) {
	s := &stringSliceFlag{}
	if err := s.Set("  a  , b ,, c "); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(s.Values(), want) {
		t.Errorf("got %v, want %v", s.Values(), want)
	}
}

func TestStringSliceFlag_StringRoundTrip(t *testing.T) {
	s := &stringSliceFlag{values: []string{"x", "y"}}
	if got := s.String(); got != "x,y" {
		t.Errorf("String() = %q, want %q", got, "x,y")
	}
}

func TestStringSliceFlag_DefaultPreserved(t *testing.T) {
	s := &stringSliceFlag{values: []string{"d1", "d2"}}
	if !reflect.DeepEqual(s.Values(), []string{"d1", "d2"}) {
		t.Fatalf("default not preserved before Set: %v", s.Values())
	}
}
