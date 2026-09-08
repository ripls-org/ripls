package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLogAIError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantLevel     string // "WARNING" or "ERROR" (Cloud Logging severity names)
		wantTransient bool
		wantNoLog     bool
	}{
		{
			name:      "nil error emits no log",
			err:       nil,
			wantNoLog: true,
		},
		{
			name:          "429 error logs WARN with transient=true",
			err:           errors.New("gemini streaming call failed: Error 429, Status: RESOURCE_EXHAUSTED"),
			wantLevel:     "WARNING",
			wantTransient: true,
		},
		{
			name:          "RESOURCE_EXHAUSTED logs WARN with transient=true",
			err:           errors.New("resource exhausted: quota exceeded"),
			wantLevel:     "WARNING",
			wantTransient: true,
		},
		{
			name:          "deadline exceeded logs WARN with transient=true",
			err:           errors.New("context deadline exceeded"),
			wantLevel:     "WARNING",
			wantTransient: true,
		},
		{
			name:          "503 upstream error logs WARN with transient=true",
			err:           errors.New("upstream returned 503 service unavailable"),
			wantLevel:     "WARNING",
			wantTransient: true,
		},
		{
			name:          "non-transient bad-JSON error logs ERROR with transient=false",
			err:           errors.New("bad JSON: unexpected end of input"),
			wantLevel:     "ERROR",
			wantTransient: false,
		},
		{
			name:          "non-transient internal error logs ERROR with transient=false",
			err:           errors.New("internal contract violation: nil response"),
			wantLevel:     "ERROR",
			wantTransient: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf, logger := newBufLogger()
			ctx := context.Background()
			LogAIError(ctx, logger, "test message", tc.err, "extra_key", "extra_val")

			out := buf.String()

			if tc.wantNoLog {
				if out != "" {
					t.Errorf("expected no log output for nil err, got: %s", out)
				}
				return
			}

			if !strings.Contains(out, `"severity":"`+tc.wantLevel+`"`) {
				t.Errorf("expected severity=%q in log output, got:\n%s", tc.wantLevel, out)
			}

			wantTransientStr := `"transient":true`
			if !tc.wantTransient {
				wantTransientStr = `"transient":false`
			}
			if !strings.Contains(out, wantTransientStr) {
				t.Errorf("expected %s in log output, got:\n%s", wantTransientStr, out)
			}

			if !strings.Contains(out, `"error":`) {
				t.Errorf("expected error field in log output, got:\n%s", out)
			}

			if !strings.Contains(out, "extra_key") {
				t.Errorf("expected extra_key field in log output, got:\n%s", out)
			}
		})
	}
}
