package product

import (
	"strings"
	"testing"
)

func TestBuildSearchURL(t *testing.T) {
	tests := []struct {
		name     string
		brand    string
		model    string
		provider SearchProvider
		wantURL  string
	}{
		{
			name:     "google shopping with brand and model",
			brand:    "DeWalt",
			model:    "DCD771C2",
			provider: ProviderGoogle,
			wantURL:  "https://www.google.com/search?q=DeWalt+DCD771C2&tbm=shop",
		},
		{
			name:     "amazon with brand and model",
			brand:    "Milwaukee",
			model:    "2804-20",
			provider: ProviderAmazon,
			wantURL:  "https://www.amazon.com/s?k=Milwaukee+2804-20",
		},
		{
			name:     "home depot with brand and model",
			brand:    "Ryobi",
			model:    "P208",
			provider: ProviderHomeDepot,
			wantURL:  "https://www.homedepot.com/s/Ryobi+P208",
		},
		{
			name:     "rei with brand and model",
			brand:    "Osprey",
			model:    "Atmos AG 65",
			provider: ProviderREI,
			wantURL:  "https://www.rei.com/search?q=Osprey+Atmos+AG+65",
		},
		{
			name:     "brand only",
			brand:    "Makita",
			model:    "",
			provider: ProviderGoogle,
			wantURL:  "https://www.google.com/search?q=Makita&tbm=shop",
		},
		{
			name:     "model only",
			brand:    "",
			model:    "DCD771C2",
			provider: ProviderGoogle,
			wantURL:  "https://www.google.com/search?q=DCD771C2&tbm=shop",
		},
		{
			name:     "empty brand and model",
			brand:    "",
			model:    "",
			provider: ProviderGoogle,
			wantURL:  "",
		},
		{
			name:     "whitespace only brand and model",
			brand:    "  ",
			model:    "  ",
			provider: ProviderGoogle,
			wantURL:  "",
		},
		{
			name:     "special characters in query",
			brand:    "Black+Decker",
			model:    "LDX120C",
			provider: ProviderGoogle,
			wantURL:  "https://www.google.com/search?q=Black%2BDecker+LDX120C&tbm=shop",
		},
		{
			name:     "unknown provider defaults to google",
			brand:    "DeWalt",
			model:    "DCD771C2",
			provider: "unknown",
			wantURL:  "https://www.google.com/search?q=DeWalt+DCD771C2&tbm=shop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSearchURL(tt.brand, tt.model, tt.provider)
			if got != tt.wantURL {
				t.Errorf("BuildSearchURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestBuildManufacturerSearchURL(t *testing.T) {
	tests := []struct {
		name      string
		brand     string
		model     string
		wantEmpty bool
		wantSite  string
	}{
		{
			name:      "dewalt brand",
			brand:     "DeWalt",
			model:     "DCD771C2",
			wantEmpty: false,
			wantSite:  "dewalt.com",
		},
		{
			name:      "milwaukee brand lowercase",
			brand:     "milwaukee",
			model:     "2804-20",
			wantEmpty: false,
			wantSite:  "milwaukeetool.com",
		},
		{
			name:      "rei brand",
			brand:     "REI",
			model:     "Flash 55",
			wantEmpty: false,
			wantSite:  "rei.com",
		},
		{
			name:      "unknown brand",
			brand:     "UnknownBrand",
			model:     "Model123",
			wantEmpty: true,
			wantSite:  "",
		},
		{
			name:      "empty brand",
			brand:     "",
			model:     "Model123",
			wantEmpty: true,
			wantSite:  "",
		},
		{
			name:      "patagonia brand",
			brand:     "Patagonia",
			model:     "Nano Puff",
			wantEmpty: false,
			wantSite:  "patagonia.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildManufacturerSearchURL(tt.brand, tt.model)
			if tt.wantEmpty && got != "" {
				t.Errorf("BuildManufacturerSearchURL() = %q, want empty", got)
			}
			if !tt.wantEmpty {
				if got == "" {
					t.Errorf("BuildManufacturerSearchURL() = empty, want URL with site %s", tt.wantSite)
				} else if !strings.Contains(got, tt.wantSite) {
					t.Errorf("BuildManufacturerSearchURL() = %q, should contain site %s", got, tt.wantSite)
				}
			}
		})
	}
}

func TestBuildSearchQuery(t *testing.T) {
	tests := []struct {
		name  string
		brand string
		model string
		want  string
	}{
		{
			name:  "both brand and model",
			brand: "DeWalt",
			model: "DCD771C2",
			want:  "DeWalt DCD771C2",
		},
		{
			name:  "brand only",
			brand: "DeWalt",
			model: "",
			want:  "DeWalt",
		},
		{
			name:  "model only",
			brand: "",
			model: "DCD771C2",
			want:  "DCD771C2",
		},
		{
			name:  "both empty",
			brand: "",
			model: "",
			want:  "",
		},
		{
			name:  "with whitespace",
			brand: "  DeWalt  ",
			model: "  DCD771C2  ",
			want:  "DeWalt DCD771C2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSearchQuery(tt.brand, tt.model)
			if got != tt.want {
				t.Errorf("buildSearchQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetBrandDomain(t *testing.T) {
	tests := []struct {
		brand string
		want  string
	}{
		{"DeWalt", "dewalt.com"},
		{"dewalt", "dewalt.com"},
		{"DEWALT", "dewalt.com"},
		{"Milwaukee", "milwaukeetool.com"},
		{"REI", "rei.com"},
		{"Osprey", "osprey.com"},
		{"UnknownBrand", ""},
		{"", ""},
		{"  dewalt  ", "dewalt.com"},
	}

	for _, tt := range tests {
		t.Run(tt.brand, func(t *testing.T) {
			got := GetBrandDomain(tt.brand)
			if got != tt.want {
				t.Errorf("GetBrandDomain(%q) = %q, want %q", tt.brand, got, tt.want)
			}
		})
	}
}
