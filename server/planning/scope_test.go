package planning

import "testing"

func TestScope_ScopeID(t *testing.T) {
	tests := []struct {
		name string
		s    Scope
		want string
	}{
		{"experience set", Scope{ExperienceID: "exp-1"}, "exp-1"},
		{"request set", Scope{RequestID: "req-1"}, "req-1"},
		{"both empty", Scope{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.ScopeID(); got != tt.want {
				t.Errorf("ScopeID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScope_IsExperience(t *testing.T) {
	tests := []struct {
		name string
		s    Scope
		want bool
	}{
		{"experience set", Scope{ExperienceID: "exp-1"}, true},
		{"request set", Scope{RequestID: "req-1"}, false},
		{"both empty", Scope{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.IsExperience(); got != tt.want {
				t.Errorf("IsExperience() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScope_IsRequest(t *testing.T) {
	tests := []struct {
		name string
		s    Scope
		want bool
	}{
		{"experience set", Scope{ExperienceID: "exp-1"}, false},
		{"request set", Scope{RequestID: "req-1"}, true},
		{"both empty", Scope{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.IsRequest(); got != tt.want {
				t.Errorf("IsRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScope_scopeFieldName(t *testing.T) {
	tests := []struct {
		name string
		s    Scope
		want string
	}{
		{"experience scope", Scope{ExperienceID: "exp-1"}, "experience_id"},
		{"request scope", Scope{RequestID: "req-1"}, "request_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.scopeFieldName(); got != tt.want {
				t.Errorf("scopeFieldName() = %q, want %q", got, tt.want)
			}
		})
	}
}
