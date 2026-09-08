package middleware

import "testing"

func TestCORSOriginValidator(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		origin  string
		want    bool
	}{
		{name: "wildcard allows any", allowed: []string{"*"}, origin: "https://evil.com", want: true},
		{name: "exact match", allowed: []string{"https://example.com"}, origin: "https://example.com", want: true},
		{name: "exact no match", allowed: []string{"https://example.com"}, origin: "https://evil.com", want: false},
		{name: "multiple origins match first", allowed: []string{"https://example.com", "https://dev.example.com"}, origin: "https://example.com", want: true},
		{name: "multiple origins match second", allowed: []string{"https://example.com", "https://dev.example.com"}, origin: "https://dev.example.com", want: true},
		{name: "multiple origins no match", allowed: []string{"https://example.com", "https://dev.example.com"}, origin: "https://evil.com", want: false},
		{name: "localhost wildcard port matches", allowed: []string{"http://localhost:*"}, origin: "http://localhost:3000", want: true},
		{name: "localhost wildcard port matches other port", allowed: []string{"http://localhost:*"}, origin: "http://localhost:8080", want: true},
		{name: "localhost wildcard does not match different host", allowed: []string{"http://localhost:*"}, origin: "http://notlocalhost:3000", want: false},
		{name: "empty allowed denies all", allowed: []string{}, origin: "https://example.com", want: false},
		{name: "marketing origin exact match", allowed: []string{"https://www.example.com"}, origin: "https://www.example.com", want: true},
		{name: "subdomain wildcard matches one label", allowed: []string{"https://*.staging.example.com"}, origin: "https://website-tweaks.staging.example.com", want: true},
		{name: "subdomain wildcard matches different slug", allowed: []string{"https://*.staging.example.com"}, origin: "https://pr-42.staging.example.com", want: true},
		{name: "subdomain wildcard rejects bare base", allowed: []string{"https://*.staging.example.com"}, origin: "https://staging.example.com", want: false},
		{name: "subdomain wildcard rejects nested label", allowed: []string{"https://*.staging.example.com"}, origin: "https://a.b.staging.example.com", want: false},
		{name: "subdomain wildcard rejects scheme mismatch", allowed: []string{"https://*.staging.example.com"}, origin: "http://x.staging.example.com", want: false},
		{name: "subdomain wildcard rejects different base", allowed: []string{"https://*.staging.example.com"}, origin: "https://x.evil.org", want: false},
		{name: "subdomain wildcard rejects suffix spoof", allowed: []string{"https://*.staging.example.com"}, origin: "https://x.staging.example.com.evil.com", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fn := CORSOriginValidator(tc.allowed)
			if got := fn(tc.origin); got != tc.want {
				t.Errorf("CORSOriginValidator(%v)(%q) = %v; want %v", tc.allowed, tc.origin, got, tc.want)
			}
		})
	}
}
