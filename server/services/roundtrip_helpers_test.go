package services

import (
	"errors"
	"testing"
)

// TestAssertFieldRoundTrip_Success verifies the happy path: a value written
// by save and returned by fetch passes the assertion without failing the
// test.
func TestAssertFieldRoundTrip_Success(t *testing.T) {
	stored := ""
	AssertFieldRoundTrip(t, "example",
		"hello",
		func() error { stored = "hello"; return nil },
		func() (string, error) { return stored, nil },
	)
}

// TestAssertFieldRoundTrip_Mismatch verifies that a value which does not
// survive the round-trip (save stores one value, fetch returns another)
// causes the test to fail.
func TestAssertFieldRoundTrip_Mismatch(t *testing.T) {
	fake := &mockT{}
	stored := ""
	AssertFieldRoundTrip(fake, "example",
		"hello",
		func() error { stored = "hello"; return nil },
		func() (string, error) { return stored + "-corrupted", nil },
	)
	if !fake.failed {
		t.Fatal("expected AssertFieldRoundTrip to fail on value mismatch")
	}
	if fake.fatal {
		t.Fatal("expected Errorf (not Fatalf) on mismatch so further fields can be checked")
	}
}

// TestAssertFieldRoundTrip_SaveError verifies that a save error is reported
// as a fatal test failure (the round trip cannot continue without a save).
func TestAssertFieldRoundTrip_SaveError(t *testing.T) {
	fake := &mockT{}
	AssertFieldRoundTrip(fake, "example",
		"hello",
		func() error { return errors.New("boom") },
		func() (string, error) { return "hello", nil },
	)
	if !fake.failed || !fake.fatal {
		t.Fatal("expected save error to produce a fatal test failure")
	}
}

// TestAssertFieldRoundTrip_FetchError verifies that a fetch error is
// reported as a fatal test failure.
func TestAssertFieldRoundTrip_FetchError(t *testing.T) {
	fake := &mockT{}
	AssertFieldRoundTrip(fake, "example",
		"hello",
		func() error { return nil },
		func() (string, error) { return "", errors.New("boom") },
	)
	if !fake.failed || !fake.fatal {
		t.Fatal("expected fetch error to produce a fatal test failure")
	}
}

// TestAssertFieldRoundTrip_Pointer verifies that pointer-valued fields
// (e.g. optional proto scalars) are compared by dereferenced value.
func TestAssertFieldRoundTrip_Pointer(t *testing.T) {
	want := "mapbox:poi.123"
	var stored *string
	AssertFieldRoundTrip(t, "external_place_id",
		&want,
		func() error { v := "mapbox:poi.123"; stored = &v; return nil },
		func() (*string, error) { return stored, nil },
	)
}

// TestAssertFieldRoundTrip_Slice verifies that repeated fields survive
// round-trip comparison.
func TestAssertFieldRoundTrip_Slice(t *testing.T) {
	var stored []string
	AssertFieldRoundTrip(t, "address_lines",
		[]string{"506 Monroe St", "Apt 4"},
		func() error { stored = []string{"506 Monroe St", "Apt 4"}; return nil },
		func() ([]string, error) { return stored, nil },
	)
}

// mockT implements just enough of testing.TB for AssertFieldRoundTrip's
// t.Helper / t.Errorf / t.Fatalf calls. AssertFieldRoundTrip is typed
// against *testing.T, so we use a small adapter in these negative tests.
type mockT struct {
	testing.TB
	failed bool
	fatal  bool
}

func (m *mockT) Helper()                           {}
func (m *mockT) Errorf(format string, args ...any) { m.failed = true }
func (m *mockT) Fatalf(format string, args ...any) { m.failed = true; m.fatal = true }
