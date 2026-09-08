package simulation

import (
	"fmt"
	"testing"
	"time"
)

func TestPersonaType_String(t *testing.T) {
	tests := []struct {
		persona PersonaType
		want    string
	}{
		{PersonaAlfred, "Alfred"},
		{PersonaDerek, "Derek"},
		{PersonaGary, "Gary"},
		{PersonaBetty, "Betty"},
		{PersonaEmma, "Emma"},
		{PersonaHenry, "Henry"},
		{PersonaType(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.persona.String(); got != tt.want {
			t.Errorf("PersonaType(%d).String() = %q, want %q", int(tt.persona), got, tt.want)
		}
	}
}

func TestScenario_SimulationID(t *testing.T) {
	s := Scenario{
		Name:      "suburban-neighborhood",
		StartTime: time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC),
	}
	want := "sim-suburban-neighborhood"
	if got := s.SimulationID(); got != want {
		t.Errorf("SimulationID() = %q, want %q", got, want)
	}
}

func TestScenario_Validate(t *testing.T) {
	validScenario := func() Scenario {
		return Scenario{
			Name: "test",
			Communities: []CommunityDef{
				{
					Name: "Test Community",
					Members: []MemberDef{
						{Name: "Alice", Email: "alice@example.com", Persona: PersonaAlfred},
					},
				},
			},
			StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		}
	}

	t.Run("valid scenario", func(t *testing.T) {
		if err := validScenario().Validate(); err != nil {
			t.Errorf("expected valid, got: %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		s := validScenario()
		s.Name = ""
		if err := s.Validate(); err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("no communities", func(t *testing.T) {
		s := validScenario()
		s.Communities = nil
		if err := s.Validate(); err == nil {
			t.Error("expected error for no communities")
		}
	})

	t.Run("missing start time", func(t *testing.T) {
		s := validScenario()
		s.StartTime = time.Time{}
		if err := s.Validate(); err == nil {
			t.Error("expected error for missing start time")
		}
	})

	t.Run("end before start", func(t *testing.T) {
		s := validScenario()
		s.EndTime = s.StartTime.Add(-time.Hour)
		if err := s.Validate(); err == nil {
			t.Error("expected error for end before start")
		}
	})
}

func TestCommunityDef_Validate(t *testing.T) {
	t.Run("missing name", func(t *testing.T) {
		c := CommunityDef{
			Members: []MemberDef{{Name: "A", Email: "a@example.com", Persona: PersonaAlfred}},
		}
		if err := c.Validate(); err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("no members", func(t *testing.T) {
		c := CommunityDef{Name: "Test"}
		if err := c.Validate(); err == nil {
			t.Error("expected error for no members")
		}
	})

	t.Run("duplicate emails", func(t *testing.T) {
		c := CommunityDef{
			Name: "Test",
			Members: []MemberDef{
				{Name: "A", Email: "a@example.com", Persona: PersonaAlfred},
				{Name: "B", Email: "a@example.com", Persona: PersonaDerek},
			},
		}
		if err := c.Validate(); err == nil {
			t.Error("expected error for duplicate emails")
		}
	})

	t.Run("too many members", func(t *testing.T) {
		var members []MemberDef
		for i := range MaxCommunityMembers + 1 {
			members = append(members, MemberDef{
				Name:    fmt.Sprintf("User%d", i),
				Email:   fmt.Sprintf("user%d@example.com", i),
				Persona: PersonaAlfred,
			})
		}
		c := CommunityDef{Name: "Test", Members: members}
		if err := c.Validate(); err == nil {
			t.Errorf("expected error for %d members (max %d)", len(members), MaxCommunityMembers)
		}
	})
}

func TestMemberDef_Validate(t *testing.T) {
	t.Run("missing name", func(t *testing.T) {
		m := MemberDef{Email: "a@example.com", Persona: PersonaAlfred}
		if err := m.Validate(); err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("missing email", func(t *testing.T) {
		m := MemberDef{Name: "A", Persona: PersonaAlfred}
		if err := m.Validate(); err == nil {
			t.Error("expected error for missing email")
		}
	})

	t.Run("invalid persona", func(t *testing.T) {
		m := MemberDef{Name: "A", Email: "a@example.com", Persona: PersonaType(99)}
		if err := m.Validate(); err == nil {
			t.Error("expected error for invalid persona")
		}
	})
}

func TestRunConfig_Validate(t *testing.T) {
	t.Run("missing server URL", func(t *testing.T) {
		rc := RunConfig{
			Scenario: Scenario{
				Name: "test",
				Communities: []CommunityDef{
					{
						Name: "Test",
						Members: []MemberDef{
							{Name: "A", Email: "a@example.com", Persona: PersonaAlfred},
						},
					},
				},
				StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			},
		}
		if err := rc.Validate(); err == nil {
			t.Error("expected error for missing server URL")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		rc := RunConfig{
			ServerURL: "http://localhost:8080",
			Scenario: Scenario{
				Name: "test",
				Communities: []CommunityDef{
					{
						Name: "Test",
						Members: []MemberDef{
							{Name: "A", Email: "a@example.com", Persona: PersonaAlfred},
						},
					},
				},
				StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			},
		}
		if err := rc.Validate(); err != nil {
			t.Errorf("expected valid, got: %v", err)
		}
	})
}
