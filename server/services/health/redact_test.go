package health

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	healthpkg "go.ripls.org/ripls/server/health"
	"go.ripls.org/ripls/server/logging"
)

func TestRedactURLQueries(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no url", "status 403", "status 403"},
		{"url without query", `Get "https://example.com/v1/x": EOF`, `Get "https://example.com/v1/x": EOF`},
		{
			"quoted url with query",
			`Get "https://example.com/geocode/json?address=a+b&key=sekrit": context canceled`,
			`Get "https://example.com/geocode/json?[redacted]": context canceled`,
		},
		{
			"bare url with query",
			"dial http://example.com/x?token=sekrit failed",
			"dial http://example.com/x?[redacted] failed",
		},
		{
			"two urls",
			"https://a.example/x?k=1 then https://b.example/y?k=2",
			"https://a.example/x?[redacted] then https://b.example/y?[redacted]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactURLQueries(tt.in); got != tt.want {
				t.Errorf("redactURLQueries(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCheckHealth_RedactsURLQueryFromDependencyError(t *testing.T) {
	svc := New(logging.Default())
	svc.RegisterFunc("location", func(ctx context.Context) ([]*healthpkg.Status, error) {
		return []*healthpkg.Status{{
			Name:    "location",
			Backend: "google_maps",
			Error:   `Get "https://example.com/geocode/json?address=x&key=sekrit": context canceled`,
		}}, nil
	})

	resp, err := svc.CheckHealth(context.Background(), connect.NewRequest(&api.CheckHealthRequest{}))
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	got := resp.Msg.Dependencies[0].Error
	if strings.Contains(got, "sekrit") {
		t.Errorf("dependency error still carries the query string: %q", got)
	}
	if !strings.Contains(got, "context canceled") {
		t.Errorf("dependency error lost its cause: %q", got)
	}
}
