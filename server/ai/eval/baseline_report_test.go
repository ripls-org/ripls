package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMedianMs(t *testing.T) {
	tests := []struct {
		name string
		in   []int64
		want int64
	}{
		{"empty returns zero", nil, 0},
		{"single element", []int64{500}, 500},
		{"odd count: 3", []int64{300, 100, 500}, 300},
		{"even count: 4 -> avg of middle two", []int64{100, 200, 400, 800}, 300},
		{"unsorted input", []int64{1000, 50, 500, 100, 200}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := medianMs(tt.in); got != tt.want {
				t.Errorf("medianMs(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestP95Ms(t *testing.T) {
	tests := []struct {
		name string
		in   []int64
		want int64
	}{
		{"empty returns zero", nil, 0},
		{"single element", []int64{500}, 500},
		// With n=20, 0.95 * 19 = 18.05 → rounds to index 18 (the 19th
		// element), which is the 95th percentile by the simple
		// nearest-rank definition.
		{"twenty elements", []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, 19},
		// With n=5, 0.95 * 4 = 3.8 → rounds to index 4 (the 5th
		// element = max).
		{"five elements", []int64{10, 20, 30, 40, 50}, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p95Ms(tt.in); got != tt.want {
				t.Errorf("p95Ms(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestWriteBaselineJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "baseline")
	report := BaselineReport{
		SchemaVersion: BaselineReportSchemaVersion,
		Suite:         "TestRequestPrompt",
		Run: RunMetadata{
			TimestampUTC: "2026-05-19T22:00:00Z",
			GitCommit:    "abc1234567",
			GitBranch:    "feature",
			Label:        "smoke",
		},
		GoldensFile:    "request_goldens.json",
		GoldensVersion: 1,
		Pairs: []PairResult{
			{
				Provider:        "anthropic",
				Model:           "claude-haiku-4-5",
				PassRate:        0.857,
				Threshold:       0.78,
				Passed:          true,
				MedianLatencyMs: 3200,
				P95LatencyMs:    4900,
				FieldPassRates: map[string]float64{
					"title_contains": 1.0,
					"value_estimate": 0.83,
				},
			},
		},
	}

	if err := WriteBaselineJSON(dir, "request", report); err != nil {
		t.Fatalf("WriteBaselineJSON: %v", err)
	}

	// File exists and is valid JSON matching what we wrote.
	path := filepath.Join(dir, "request.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if data[len(data)-1] != '\n' {
		t.Errorf("written file missing trailing newline")
	}

	var roundTrip BaselineReport
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal written JSON: %v", err)
	}
	if roundTrip.SchemaVersion != BaselineReportSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", roundTrip.SchemaVersion, BaselineReportSchemaVersion)
	}
	if roundTrip.Suite != "TestRequestPrompt" {
		t.Errorf("Suite = %q, want %q", roundTrip.Suite, "TestRequestPrompt")
	}
	if len(roundTrip.Pairs) != 1 {
		t.Fatalf("len(Pairs) = %d, want 1", len(roundTrip.Pairs))
	}
	if roundTrip.Pairs[0].MedianLatencyMs != 3200 {
		t.Errorf("MedianLatencyMs round-trip = %d, want 3200", roundTrip.Pairs[0].MedianLatencyMs)
	}

	// Re-writing the same file is a no-error overwrite — the baseline
	// directory is meant to accept fresh runs at the same path.
	if err := WriteBaselineJSON(dir, "request", report); err != nil {
		t.Fatalf("WriteBaselineJSON (overwrite): %v", err)
	}
}

func TestResolveBaselineDir(t *testing.T) {
	t.Run("absolute path used verbatim", func(t *testing.T) {
		got := resolveBaselineDir("/tmp/baseline")
		if got != "/tmp/baseline" {
			t.Errorf("got %q, want %q", got, "/tmp/baseline")
		}
	})

	t.Run("relative path resolves against repo root", func(t *testing.T) {
		// findRepoRoot walks up from cwd looking for .git. In this
		// test, cwd is server/ai/eval/, so the repo root is three
		// levels up. Verify the resolved path lands at the repo root,
		// not at server/ai/eval/docs/...
		got := resolveBaselineDir("docs/ai/eval/baselines/test")
		if got == "docs/ai/eval/baselines/test" {
			t.Errorf("got unmodified relative path %q; expected resolution against repo root", got)
		}
		// The resolved path's basename chain should end correctly.
		if filepath.Base(got) != "test" ||
			filepath.Base(filepath.Dir(got)) != "baselines" {
			t.Errorf("resolved path %q doesn't end in baselines/test", got)
		}
	})
}

func TestFieldPassRates(t *testing.T) {
	tally := NewTally()
	tally.Record(Check{CaseID: "a", Field: "title", Pass: true})
	tally.Record(Check{CaseID: "a", Field: "title", Pass: true})
	tally.Record(Check{CaseID: "b", Field: "title", Pass: false})
	tally.Record(Check{CaseID: "a", Field: "weight", Pass: true})

	got := fieldPassRates(tally)
	wantTitle := 2.0 / 3.0
	if got["title"] != wantTitle {
		t.Errorf("title = %v, want %v", got["title"], wantTitle)
	}
	if got["weight"] != 1.0 {
		t.Errorf("weight = %v, want 1.0", got["weight"])
	}
}
