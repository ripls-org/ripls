package eval

import (
	"reflect"
	"sort"
	"testing"
)

// providerSpec captures one provider's configured models and whether its
// credentials are present, independent of how the values are sourced. Pure
// data so selectProviderSpecs can be unit-tested without flag plumbing or
// real provider construction.
type providerSpec struct {
	Name     string
	Models   []string
	HasCreds bool
}

// selectProviderSpecs filters a set of provider specs by credential
// availability and an optional name filter. The filter accepts "" or "all"
// (no-op — keep every credentialed spec) or a specific provider name (keep
// only that one). Specs without credentials are always dropped.
func selectProviderSpecs(specs []providerSpec, filter string) []providerSpec {
	var out []providerSpec
	for _, s := range specs {
		if !s.HasCreds {
			continue
		}
		if filter != "" && filter != "all" && filter != s.Name {
			continue
		}
		out = append(out, s)
	}
	return out
}

func TestSelectProviderSpecs(t *testing.T) {
	all := []providerSpec{
		{Name: "anthropic", Models: []string{"a1"}, HasCreds: true},
		{Name: "openai", Models: []string{"o1", "o2"}, HasCreds: false},
		{Name: "gemini", Models: []string{"g1"}, HasCreds: true},
	}

	tests := []struct {
		name   string
		filter string
		want   []string // provider names, in order
	}{
		{"empty filter keeps all credentialed", "", []string{"anthropic", "gemini"}},
		{"all filter keeps all credentialed", "all", []string{"anthropic", "gemini"}},
		{"specific provider with creds", "anthropic", []string{"anthropic"}},
		{"specific provider without creds", "openai", nil},
		{"unknown provider name yields none", "claude", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectProviderSpecs(all, tt.filter)
			var gotNames []string
			for _, s := range got {
				gotNames = append(gotNames, s.Name)
			}
			sort.Strings(gotNames)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(gotNames, want) {
				t.Errorf("selectProviderSpecs(filter=%q) names = %v, want %v", tt.filter, gotNames, want)
			}
		})
	}
}

func TestSelectProviderSpecs_PreservesModels(t *testing.T) {
	in := []providerSpec{
		{Name: "anthropic", Models: []string{"a1", "a2", "a3"}, HasCreds: true},
	}
	got := selectProviderSpecs(in, "anthropic")
	if len(got) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].Models, []string{"a1", "a2", "a3"}) {
		t.Errorf("models not preserved: got %v", got[0].Models)
	}
}
