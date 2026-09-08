package timezone

import "testing"

func TestValidateIANA(t *testing.T) {
	tests := []struct {
		name    string
		tz      string
		wantErr bool
	}{
		{"valid PST", "America/Los_Angeles", false},
		{"valid EST", "America/New_York", false},
		{"valid UTC", "UTC", false},
		{"valid GMT", "GMT", false},
		{"valid JST", "Asia/Tokyo", false},
		{"valid CET", "Europe/Paris", false},
		{"invalid format", "PST", true},
		{"invalid name", "America/Fake", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIANA(tt.tz)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIANA() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		tz   string
		want bool
	}{
		{"valid PST", "America/Los_Angeles", true},
		{"valid UTC", "UTC", true},
		{"empty string", "", true},
		{"invalid format", "PST", false},
		{"invalid name", "America/Fake", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidOrEmpty(tt.tz); got != tt.want {
				t.Errorf("IsValidOrEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
