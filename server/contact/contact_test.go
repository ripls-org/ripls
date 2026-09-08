package contact

import "testing"

func TestNormalizePhoneE164(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "bare US 10-digit", in: "5551234567", want: "+15551234567"},
		{name: "formatted US", in: "(555) 123-4567", want: "+15551234567"},
		{name: "dotted US", in: "555.123.4567", want: "+15551234567"},
		{name: "US with leading 1", in: "1 555 123 4567", want: "+15551234567"},
		{name: "already E.164", in: "+15551234567", want: "+15551234567"},
		{name: "international with plus", in: "+44 7911 123456", want: "+447911123456"},
		{name: "plus with formatting", in: "+1 (555) 123-4567", want: "+15551234567"},
		{name: "surrounding whitespace", in: "  5551234567  ", want: "+15551234567"},
		{name: "empty", in: "", wantErr: true},
		{name: "whitespace only", in: "   ", wantErr: true},
		{name: "no digits", in: "abc-def", wantErr: true},
		{name: "too short bare", in: "12345", wantErr: true},
		{name: "ambiguous 9-digit", in: "123456789", wantErr: true},
		{name: "leading zero country code", in: "+0123456789", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePhoneE164(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizePhoneE164(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizePhoneE164(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("NormalizePhoneE164(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsDialableE164(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		// Shape-valid but not dialable — these pass NormalizePhoneE164 yet a
		// carrier rejects them (Twilio 21211). The +1 555 area code is
		// unassigned in the North American Numbering Plan.
		{name: "firebase OTP test number", in: "+15555550001", want: false},
		{name: "555 area code", in: "+15551234567", want: false},
		{name: "shape-invalid junk", in: "+1", want: false},
		{name: "not E.164 (no plus)", in: "5551234567", want: false},
		{name: "empty", in: "", want: false},
		// Real, dialable numbers — a real area code with a 555 exchange is
		// valid (only the 555 *area code* is unassigned).
		{name: "US 415 with 555 exchange", in: "+14155551234", want: true},
		{name: "US 415 fictional 555-0100 exchange", in: "+14155550100", want: true},
		{name: "UK mobile", in: "+447911123456", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDialableE164(tc.in); got != tc.want {
				t.Errorf("IsDialableE164(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
