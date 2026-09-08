package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// BaselineReportSchemaVersion is the current shape of the per-suite
// JSON written under -eval-report-out. Bump when the schema changes in
// a way that breaks consumers; v1 captures aggregate pass rates and
// latency stats only (no per-case detail). See
// docs/ai/eval/baselines/README.md for the field semantics.
const BaselineReportSchemaVersion = 1

// BaselineReport is the machine-readable record emitted per Test*Prompt
// invocation under -eval-report-out. One file per suite per run; the
// surrounding directory name (date + git SHA + run label) identifies
// the run. Consumers diff across runs to detect drift; the schema
// version lets us evolve fields without silently breaking tooling.
type BaselineReport struct {
	SchemaVersion  int          `json:"schema_version"`
	Suite          string       `json:"suite"`
	Run            RunMetadata  `json:"run"`
	GoldensFile    string       `json:"goldens_file"`
	GoldensVersion int          `json:"goldens_version"`
	Pairs          []PairResult `json:"pairs"`
}

// RunMetadata identifies *when* and *what* the run reflects. Together
// with the goldens version, these are the bits that let a future
// reader judge whether a saved baseline is still valid for comparison.
type RunMetadata struct {
	TimestampUTC string `json:"timestamp_utc"`
	GitCommit    string `json:"git_commit"`
	GitBranch    string `json:"git_branch"`
	Label        string `json:"label,omitempty"`
}

// PairResult captures one (provider, model) pair's aggregate results
// for a single suite. Per-case detail is intentionally omitted at
// schema v1 to keep files small and diff-friendly; field-level pass
// rates already capture per-case granularity for the assertions that
// matter, and the overall PassRate gives the headline number. An
// eventual v2 can add `cases []CaseResult` when deep-dive comparison
// requires it.
type PairResult struct {
	Provider           string             `json:"provider"`
	Model              string             `json:"model"`
	PassRate           float64            `json:"pass_rate"`
	Threshold          float64            `json:"threshold"`
	Passed             bool               `json:"passed"`
	MedianLatencyMs    int64              `json:"median_latency_ms"`
	P95LatencyMs       int64              `json:"p95_latency_ms"`
	FieldPassRates     map[string]float64 `json:"field_pass_rates"`
	InvocationFailures []string           `json:"invocation_failures,omitempty"`
}

// WriteBaselineJSON writes a BaselineReport to <dir>/<filename>.json,
// creating the directory if it doesn't exist. The filename argument
// should be the suite slug (e.g. "experience" not "TestExperiencePrompt")
// so the produced layout reads like a per-axis index inside the run
// directory.
//
// Relative `dir` paths are resolved against the repo root (discovered
// by walking up from the test's cwd looking for a `.git` entry), not
// against the test process's working directory. Go tests run in their
// package directory, so a flag value like
// `docs/ai/eval/baselines/2026-05-19_cloud` would otherwise land at
// `server/ai/eval/docs/ai/eval/baselines/...` — surprising and easy
// to miss in CI. Absolute paths are used as-is. If the repo root
// can't be found (no `.git` anywhere up the tree), the relative path
// is used as-is and the caller gets whatever Go's filepath.Join does
// against the test process's cwd.
func WriteBaselineJSON(dir, filename string, report BaselineReport) error {
	resolvedDir := resolveBaselineDir(dir)
	if err := os.MkdirAll(resolvedDir, 0o755); err != nil {
		return fmt.Errorf("mkdir baseline dir: %w", err)
	}
	path := filepath.Join(resolvedDir, filename+".json")
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}
	// Trailing newline matches gofumpt-style file conventions and keeps
	// diff tooling happy.
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

// resolveBaselineDir maps a -eval-report-out flag value to the
// directory where the JSON should actually land. Absolute paths are
// kept verbatim. Relative paths are resolved against the repo root if
// findable (via .git lookup); otherwise the path is returned as-is
// (Go's default cwd resolution applies, surprising but recoverable).
func resolveBaselineDir(dir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	root, ok := findRepoRoot()
	if !ok {
		return dir
	}
	return filepath.Join(root, dir)
}

// findRepoRoot walks up from the current working directory looking for
// a `.git` entry (file for worktrees, directory for normal checkouts).
// Returns the path and true on success, "" and false when no .git is
// found before hitting the filesystem root.
func findRepoRoot() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// medianMs returns the median of latenciesMs in milliseconds, or 0 if
// the slice is empty. Linear-time partial sort is fine — we never call
// this against more than a few hundred samples per suite.
func medianMs(latenciesMs []int64) int64 {
	if len(latenciesMs) == 0 {
		return 0
	}
	sorted := append([]int64(nil), latenciesMs...)
	slices.Sort(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// p95Ms returns the 95th-percentile latency in milliseconds. The naive
// "index at ceil(0.95 * (n-1))" definition works for the small samples
// (~10-50) the eval produces per pair; precision beyond the next
// integer index doesn't matter at this sample size.
func p95Ms(latenciesMs []int64) int64 {
	if len(latenciesMs) == 0 {
		return 0
	}
	sorted := append([]int64(nil), latenciesMs...)
	slices.Sort(sorted)
	// Index 95% of the way through (0-based). Clamp to last index.
	idx := int(float64(len(sorted)-1)*0.95 + 0.5)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// fieldPassRates converts a Tally's per-field counters into the simple
// map[name]rate shape stored in BaselineReport. Sort happens when the
// JSON marshaler sorts keys, so we don't need to sort here.
func fieldPassRates(t *Tally) map[string]float64 {
	out := make(map[string]float64, len(t.perField))
	for f, c := range t.perField {
		if c.total == 0 {
			out[f] = 0
			continue
		}
		out[f] = float64(c.passed) / float64(c.total)
	}
	return out
}
