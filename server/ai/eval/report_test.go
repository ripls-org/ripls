//go:build benchmark

// Cross-cutting helpers used by every eval suite: per-pair gating,
// rate-limit retries, mode filtering, and value formatting. Split out of
// prompt_test.go so it stays focused on the goldens loops; see
// docs/server/architecture.md for the file-size rationale.

package eval

import (
	"fmt"
	"math"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// reportAndGate logs the per-(provider, model) tally and fails the test
// if the aggregate pass rate is below this pair's threshold. The key
// must be the canonical "<provider>/<model>" form so threshold lookups
// match what's in modelThresholds.
//
// When `pairs` is non-nil, the per-pair PairResult is appended to it
// **before** the threshold gate fires — so failing pairs still make it
// into the saved baseline JSON. Pass nil if the caller doesn't need
// the structured result (e.g. unit tests).
//
// This function is the single source of truth for whether a Test*Prompt
// run passes or fails. Per-case check failures elsewhere in the suite
// are surfaced via t.Logf (diagnostic-only) rather than t.Errorf, so
// the threshold gate alone determines the outcome — an idiosyncratic
// stylistic difference on a handful of cases doesn't fail the whole
// suite if aggregate accuracy is still above the configured floor.
func reportAndGate(t *testing.T, tally *Tally, key string, pairs *[]PairResult) {
	t.Helper()
	threshold := thresholdFor(key)
	r := tally.Report()
	for _, line := range r.Lines {
		t.Log(line)
	}
	t.Logf("[%s] overall: %d/%d checks passed (%.1f%%) — threshold %.2f",
		key, r.TotalPassed, r.TotalChecks, 100*r.OverallRate, threshold)

	// Capture the PairResult *before* gating so failing pairs are
	// recorded in the baseline JSON. Splitting provider/model out of
	// the "provider/model" key matters: a key like
	// "gemini/vertexai/gemini-3.1-flash-lite" has slashes in the model
	// component, so we split on the *first* slash only.
	if pairs != nil {
		provider, model := splitProviderModel(key)
		*pairs = append(*pairs, PairResult{
			Provider:           provider,
			Model:              model,
			PassRate:           r.OverallRate,
			Threshold:          threshold,
			Passed:             r.OverallRate >= threshold,
			MedianLatencyMs:    medianMs(tally.latenciesMs),
			P95LatencyMs:       p95Ms(tally.latenciesMs),
			FieldPassRates:     fieldPassRates(tally),
			InvocationFailures: append([]string(nil), tally.invoFails...),
		})
	}

	if r.OverallRate < threshold {
		t.Fatalf("[%s] aggregate pass rate %.2f < threshold %.2f", key, r.OverallRate, threshold)
	}
}

// splitProviderModel splits a "provider/model" key into its two
// components on the FIRST slash. Gemini model names contain slashes
// themselves (e.g. "vertexai/gemini-3.1-flash-lite"), so a naive
// split-on-all-slashes would mis-attribute the model.
func splitProviderModel(key string) (string, string) {
	i := strings.IndexByte(key, '/')
	if i < 0 {
		return key, ""
	}
	return key[:i], key[i+1:]
}

// gitMetadata captures the current git commit and branch for embedding
// in the baseline JSON's run section. Fails silently to "unknown" if
// git isn't on PATH or this isn't a checkout — the rest of the report
// is still valuable. Run once per Test*Prompt invocation; the cost is
// trivial vs the suite itself.
func gitMetadata() (commit, branch string) {
	commit = "unknown"
	branch = "unknown"
	if out, err := exec.Command("git", "rev-parse", "--short=10", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	return commit, branch
}

// writeBaselineJSONIfRequested writes a per-suite baseline JSON to
// -eval-report-out if the flag is set; otherwise no-ops. Logs the
// destination path on success and the error on failure (but doesn't
// fail the suite — the eval data is still in t.Log even if the JSON
// write fails).
//
// Call as `defer writeBaselineJSONIfRequested(t, "experience", ...)`
// at the top of each Test*Prompt, after declaring the pairs slice.
func writeBaselineJSONIfRequested(t *testing.T, suite, suiteSlug, goldensFile string, goldensVersion int, pairs *[]PairResult) {
	t.Helper()
	if *evalReportOut == "" {
		return
	}
	if pairs == nil || len(*pairs) == 0 {
		// No pairs ran (e.g. all credentials missing, or all skipped
		// by -mode). Skip writing rather than emit a half-empty file
		// that future diffs would have to special-case.
		t.Logf("baseline JSON not written for %s: no pair results", suite)
		return
	}
	commit, branch := gitMetadata()
	report := BaselineReport{
		SchemaVersion: BaselineReportSchemaVersion,
		Suite:         suite,
		Run: RunMetadata{
			TimestampUTC: time.Now().UTC().Format(time.RFC3339),
			GitCommit:    commit,
			GitBranch:    branch,
			Label:        *evalReportLabel,
		},
		GoldensFile:    goldensFile,
		GoldensVersion: goldensVersion,
		Pairs:          *pairs,
	}
	if err := WriteBaselineJSON(*evalReportOut, suiteSlug, report); err != nil {
		t.Logf("failed to write baseline JSON for %s: %v", suite, err)
		return
	}
	t.Logf("wrote baseline JSON: %s/%s.json", *evalReportOut, suiteSlug)
}

// requireMode skips the current test when -mode excludes the test's
// input mode. -mode=all (the default) matches every mode; otherwise the
// value must equal the test's declared mode ("text" | "image" | "webpage").
func requireMode(t *testing.T, mode string) {
	t.Helper()
	if *evalMode == "all" || *evalMode == mode {
		return
	}
	switch *evalMode {
	case "text", "image", "webpage":
		t.Skipf("-mode=%s (this test is %s-mode)", *evalMode, mode)
	default:
		t.Fatalf("unknown -mode value %q (want all | text | image | webpage)", *evalMode)
	}
}

// timedCall wraps a provider call with both rate-limit retry and
// latency tracking. On success the wall-clock duration (including any
// retry sleeps) is recorded into the tally for median/p95 stats; on
// failure the case is marked as an invocation failure. Replaces direct
// retryOnRateLimit calls in the Test*Prompt loops so latency tracking
// is uniform across the 8 suites without N inline timing blocks.
func timedCall[T any](t *testing.T, tally *Tally, caseID string, fn func() (T, error)) (T, error) {
	t.Helper()
	start := time.Now()
	result, err := retryOnRateLimit(t, caseID, fn)
	if err != nil {
		tally.RecordInvocationFailure(caseID)
		return result, err
	}
	tally.RecordLatency(time.Since(start).Milliseconds())
	return result, nil
}

// retryOnRateLimit retries fn on 429/rate-limit errors with exponential
// backoff. Live providers all surface rate limits as errors containing
// "429", "rate limit", "resource exhausted", or "quota"; non-rate-limit
// errors return immediately.
const (
	rateLimitMaxRetries    = 5
	rateLimitInitialDelay  = 2 * time.Second
	rateLimitBackoffFactor = 2.0
)

func retryOnRateLimit[T any](t *testing.T, name string, fn func() (T, error)) (T, error) {
	t.Helper()
	var lastErr error
	for attempt := range rateLimitMaxRetries {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		errMsg := strings.ToLower(err.Error())
		isRateLimit := strings.Contains(errMsg, "429") ||
			strings.Contains(errMsg, "rate limit") ||
			strings.Contains(errMsg, "resource exhausted") ||
			strings.Contains(errMsg, "quota")
		if !isRateLimit {
			return result, err
		}
		lastErr = err
		delay := time.Duration(float64(rateLimitInitialDelay) * math.Pow(rateLimitBackoffFactor, float64(attempt)))
		t.Logf("%s: rate limited (attempt %d/%d), retrying in %v: %v", name, attempt+1, rateLimitMaxRetries, delay, err)
		time.Sleep(delay) //nolint:forbidigo // exponential backoff between live API retries
	}
	var zero T
	return zero, lastErr
}

// formatValueUSD renders a float as "$NN" for t.Logf lines. Used to keep
// per-case log output compact and uniform across suites.
func formatValueUSD(v float32) string {
	return fmt.Sprintf("$%.0f", v)
}
