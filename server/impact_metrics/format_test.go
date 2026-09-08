package impact_metrics

import (
	"testing"
)

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  string
	}{
		// Zero and small values
		{name: "zero", input: 0, want: "$0"},
		{name: "small amount", input: 5.50, want: "$5.50"},
		{name: "single digit", input: 8, want: "$8.00"},

		// Dollars (no scaling)
		{name: "tens", input: 42, want: "$42"},
		{name: "hundreds", input: 341, want: "$341"},
		{name: "999 boundary", input: 999, want: "$999"},

		// Thousands (K)
		{name: "1K boundary", input: 1000, want: "$1.0K"},
		{name: "small K", input: 1241, want: "$1.2K"},
		{name: "mid K", input: 5600, want: "$5.6K"},
		{name: "large K", input: 12300, want: "$12.3K"},
		{name: "10K boundary", input: 10000, want: "$10K"},
		{name: "hundreds of K", input: 450000, want: "$450K"},
		{name: "999K boundary", input: 999000, want: "$999K"},

		// Millions (M)
		{name: "1M boundary", input: 1000000, want: "$1.0M"},
		{name: "small M", input: 1200000, want: "$1.2M"},
		{name: "mid M", input: 5600000, want: "$5.6M"},
		{name: "10M boundary", input: 10000000, want: "$10M"},
		{name: "large M", input: 123000000, want: "$123M"},

		// Negative values
		{name: "negative small", input: -42, want: "-$42"},
		{name: "negative K", input: -1200, want: "-$1.2K"},
		{name: "negative M", input: -2500000, want: "-$2.5M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMoney(tt.input)
			if got != tt.want {
				t.Errorf("FormatMoney(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name  string
		input float64 // minutes
		want  string
	}{
		// Zero and small values
		{name: "zero", input: 0, want: "0 mins"},
		{name: "1 minute", input: 1, want: "1.0 mins"},
		{name: "5 minutes", input: 5, want: "5.0 mins"},

		// Minutes
		{name: "10 mins boundary", input: 10, want: "10 mins"},
		{name: "18 mins", input: 18, want: "18 mins"},
		{name: "45 mins", input: 45, want: "45 mins"},
		{name: "59 mins boundary", input: 59, want: "59 mins"},

		// Hours
		{name: "1 hour", input: 60, want: "1.0 hrs"},
		{name: "2.5 hours", input: 150, want: "2.5 hrs"},
		{name: "5.2 hours", input: 312, want: "5.2 hrs"},
		{name: "10 hours boundary", input: 600, want: "10 hrs"},
		{name: "18 hours", input: 1080, want: "18 hrs"},
		{name: "23 hours", input: 1380, want: "23 hrs"},

		// Large hours (always hours, never days)
		{name: "24 hours", input: 1440, want: "24 hrs"},
		{name: "72 hours", input: 4320, want: "72 hrs"},
		{name: "180 hours", input: 10800, want: "180 hrs"},
		{name: "240 hours", input: 14400, want: "240 hrs"},
		{name: "720 hours", input: 43200, want: "720 hrs"},

		// Negative values
		{name: "negative minutes", input: -18, want: "-18 mins"},
		{name: "negative hours", input: -150, want: "-2.5 hrs"},
		{name: "negative large hours", input: -4320, want: "-72 hrs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTime(tt.input)
			if got != tt.want {
				t.Errorf("FormatTime(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatCO2(t *testing.T) {
	tests := []struct {
		name  string
		input float64 // grams
		want  string
	}{
		// Zero and small values
		{name: "zero", input: 0, want: "0 kg"},
		{name: "100 grams", input: 100, want: "0.10 kg"},
		{name: "500 grams", input: 500, want: "0.50 kg"},

		// Kilograms
		{name: "1 kg", input: 1000, want: "1.00 kg"},
		{name: "5.2 kg", input: 5200, want: "5.20 kg"},
		{name: "8.2 kg", input: 8200, want: "8.20 kg"},
		{name: "10 kg boundary", input: 10000, want: "10 kg"},
		{name: "50 kg", input: 50000, want: "50 kg"},
		{name: "500 kg", input: 500000, want: "500 kg"},
		{name: "999 kg boundary", input: 999000, want: "999 kg"},

		// Tonnes
		{name: "1 tonne", input: 1000000, want: "1.0 tonnes"},
		{name: "1.2 tonnes", input: 1200000, want: "1.2 tonnes"},
		{name: "5.6 tonnes", input: 5600000, want: "5.6 tonnes"},
		{name: "10 tonnes boundary", input: 10000000, want: "10 tonnes"},
		{name: "50 tonnes", input: 50000000, want: "50 tonnes"},

		// Negative values
		{name: "negative grams", input: -500, want: "-0.50 kg"},
		{name: "negative kg", input: -8200, want: "-8.20 kg"},
		{name: "negative tonnes", input: -1200000, want: "-1.2 tonnes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCO2(tt.input)
			if got != tt.want {
				t.Errorf("FormatCO2(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  string
	}{
		// Zero and small values
		{name: "zero", input: 0, want: "0"},
		{name: "single digit", input: 5, want: "5"},
		{name: "tens", input: 42, want: "42"},
		{name: "hundreds", input: 341, want: "341"},
		{name: "999 boundary", input: 999, want: "999"},

		// Thousands
		{name: "1K", input: 1000, want: "1,000"},
		{name: "1234", input: 1234, want: "1,234"},
		{name: "59", input: 59, want: "59"},
		{name: "12345", input: 12345, want: "12,345"},
		{name: "999999", input: 999999, want: "999,999"},

		// Millions
		{name: "1M", input: 1000000, want: "1,000,000"},
		{name: "1234567", input: 1234567, want: "1,234,567"},
		{name: "large", input: 123456789, want: "123,456,789"},

		// Negative values
		{name: "negative small", input: -42, want: "-42"},
		{name: "negative thousands", input: -1234, want: "-1,234"},
		{name: "negative millions", input: -1234567, want: "-1,234,567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCount(tt.input)
			if got != tt.want {
				t.Errorf("FormatCount(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatPercentage(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  string
	}{
		// Normal range
		{name: "zero", input: 0, want: "0%"},
		{name: "small", input: 5.2, want: "5%"},
		{name: "27%", input: 27, want: "27%"},
		{name: "54%", input: 54, want: "54%"},
		{name: "99%", input: 99, want: "99%"},
		{name: "100%", input: 100, want: "100%"},

		// Rounding
		{name: "round down", input: 27.4, want: "27%"},
		{name: "round up", input: 27.6, want: "28%"},
		{name: "round half up", input: 27.5, want: "28%"},

		// Edge cases
		{name: "negative clamped to 0", input: -10, want: "0%"},
		{name: "over 100 clamped", input: 150, want: "100%"},

		// Decimal inputs
		{name: "0.5 as fraction", input: 0.5, want: "1%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPercentage(tt.input)
			if got != tt.want {
				t.Errorf("FormatPercentage(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
