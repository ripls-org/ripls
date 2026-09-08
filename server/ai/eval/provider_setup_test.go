//go:build benchmark

// Provider and flag plumbing for the live-API eval suites. Split out of
// prompt_test.go so the test functions there stay focused on the goldens
// loops; the multi-model + per-(provider, model) refactor would otherwise
// push prompt_test.go past the 1,000-line ceiling set by
// docs/server/architecture.md.

package eval

import (
	"context"
	"flag"
	"math"
	"strings"
	"sync"
	"testing"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/secretsflag"
)

// Live-provider flags for the eval suites. Passed via binary flags rather
// than env vars to match the convention used by the main server binary
// (`--anthropic-api-key`, etc.); see server/ai/README.md for invocation
// recipes. Tests skip a provider when its credential flag is empty.
//
// `-provider` is a filter: empty (default) or "all" runs every credentialed
// provider; an explicit value ("anthropic" | "openai" | "gemini") restricts
// to that provider only. Each provider takes a comma-separated `-models`
// list so a single invocation can rank multiple models from the same
// provider against the goldens.
// Live-provider flags. Temperature flags are intentionally absent — the
// eval harness constructs every provider with NaN as the temperature,
// which the providers translate to "omit the temperature field on the
// wire." That way newer reasoning-class models (claude-opus-4-7, gpt-5.5)
// that reject any explicit temperature work uniformly, and per-pair
// comparisons are fair: every model uses its own default sampling
// behavior rather than one we impose unevenly.
var (
	evalProvider = flag.String("provider", "all", "Provider filter: all (default) | anthropic | openai | gemini")
	evalMode     = flag.String("mode", "all", "Input mode(s) to eval: all | text | image | webpage")

	anthropicAPIKey     = flag.String("anthropic-api-key", "", "Anthropic API key for live eval tests")
	anthropicAPIKeyFile = flag.String("anthropic-api-key-file", "", "Path to a file containing the Anthropic API key for live eval tests (mutually exclusive with -anthropic-api-key; keeps the key out of `ps`/cmdline)")
	anthropicModels     = newStringSliceFlag("anthropic-models", []string{"claude-haiku-4-5"}, "Comma-separated Anthropic model names for live eval tests")

	openaiAPIKey     = flag.String("openai-api-key", "", "OpenAI API key for live eval tests")
	openaiAPIKeyFile = flag.String("openai-api-key-file", "", "Path to a file containing the OpenAI API key for live eval tests (mutually exclusive with -openai-api-key; keeps the key out of `ps`/cmdline)")
	openaiModels     = newStringSliceFlag("openai-models", []string{"gpt-5-mini"}, "Comma-separated OpenAI model names for live eval tests")

	geminiProject  = flag.String("gemini-project", "", "Vertex AI project ID for live Gemini eval tests")
	geminiLocation = flag.String("gemini-location", "global", "Vertex AI location for live Gemini eval tests; default is global to spread regional capacity")
	// Mirrors provider_factory.go defaultVertexModel so the eval baseline
	// reflects what prod actually serves. Pass -gemini-models on the
	// command line to override (e.g. to compare against older models).
	geminiModels = newStringSliceFlag("gemini-models", []string{"vertexai/gemini-3.1-flash-lite"}, "Comma-separated Gemini model names for live eval tests")

	// Baseline-record output. When set, each Test*Prompt writes a JSON
	// summary to <dir>/<suite>.json with per-(provider, model) aggregate
	// stats — pass rates, latency median/p95, per-field rates. Used to
	// commit periodic baselines under docs/ai/eval/baselines/ so we
	// don't have to re-run the expensive cloud sweep every time we want
	// to compare a candidate (e.g. a new model or prompt revision)
	// against the prod providers. See docs/ai/eval/baselines/README.md.
	evalReportOut   = flag.String("eval-report-out", "", "Directory to write machine-readable baseline JSON files (one per Test*Prompt suite)")
	evalReportLabel = flag.String("eval-report-label", "", "Optional human label embedded in the baseline JSON's run.label (e.g. \"cloud\")")
)

// newStringSliceFlag registers a stringSliceFlag with the given name and
// default values, returning the flag for the caller to dereference. Keeping
// this helper here (rather than next to the type) means the type stays
// usable in no-tag unit tests while flag registration remains
// benchmark-only.
func newStringSliceFlag(name string, defaults []string, usage string) *stringSliceFlag {
	s := &stringSliceFlag{values: append([]string(nil), defaults...)}
	flag.Var(s, name, usage)
	return s
}

// resolvedAPIKeys collapses -<provider>-api-key / -<provider>-api-key-file
// once per test binary and stashes the result. Eval invocations on shared
// dev hosts can now point the file flag at a tmpfile (e.g. via direnv or a
// developer's ~/.config/ripls/secrets) instead of dumping the literal key
// into argv where `ps` would surface it.
type resolvedAPIKeys struct {
	anthropic string
	openai    string
}

var (
	apiKeysOnce sync.Once
	apiKeys     resolvedAPIKeys
	apiKeysErr  error
)

func resolveAPIKeys() (resolvedAPIKeys, error) {
	apiKeysOnce.Do(func() {
		anth, err := secretsflag.Resolve("anthropic-api-key", *anthropicAPIKey, *anthropicAPIKeyFile)
		if err != nil {
			apiKeysErr = err
			return
		}
		oa, err := secretsflag.Resolve("openai-api-key", *openaiAPIKey, *openaiAPIKeyFile)
		if err != nil {
			apiKeysErr = err
			return
		}
		apiKeys = resolvedAPIKeys{anthropic: anth, openai: oa}
	})
	return apiKeys, apiKeysErr
}

// providerModel pairs a credentialed live provider with the model it was
// constructed for. The Key() form ("provider/model") is the threshold-map
// lookup key and the t.Run subtest name segment.
type providerModel struct {
	Name     string
	Model    string
	Provider ai.Provider
}

// Key returns the canonical "<provider>/<model>" identifier used for
// threshold lookups and test attribution.
func (pm providerModel) Key() string {
	return pm.Name + "/" + pm.Model
}

// providerSpecsFromFlags assembles a providerSpec per supported provider
// from the current flag values. HasCreds reflects whether the credential
// flag for that provider is set; the model list is read verbatim from the
// corresponding -<provider>-models flag.
func providerSpecsFromFlags() []providerSpec {
	// Cred presence is determined by either flag being set; the actual
	// values are resolved (file → bytes) lazily inside newProviderInstance.
	hasAnthropic := *anthropicAPIKey != "" || *anthropicAPIKeyFile != ""
	hasOpenAI := *openaiAPIKey != "" || *openaiAPIKeyFile != ""
	return []providerSpec{
		{Name: "anthropic", Models: anthropicModels.Values(), HasCreds: hasAnthropic},
		{Name: "openai", Models: openaiModels.Values(), HasCreds: hasOpenAI},
		{Name: "gemini", Models: geminiModels.Values(), HasCreds: *geminiProject != ""},
	}
}

// providersForEval returns one providerModel per (provider, model) pair
// that should run for the current invocation: every selected spec
// (credentialed and not filtered out) is expanded across its model list,
// and a live provider is constructed for each pair. Providers without
// credentials are silently dropped — not a test failure — so a developer
// can run with whichever subset of API keys they have on hand.
func providersForEval(t *testing.T) []providerModel {
	t.Helper()
	selected := selectProviderSpecs(providerSpecsFromFlags(), *evalProvider)
	var out []providerModel
	for _, s := range selected {
		for _, m := range s.Models {
			provider := newProviderInstance(t, s.Name, m)
			out = append(out, providerModel{Name: s.Name, Model: m, Provider: provider})
		}
	}
	return out
}

// newProviderInstance constructs a single live provider for the given
// (name, model) pair using the matching credential flag. Temperature is
// always passed as NaN so the provider omits the field on the wire — see
// the package-level comment on the flag block. Any construction error is
// fatal: the harness cannot continue without a working provider for a
// pair the caller asked us to test.
func newProviderInstance(t *testing.T, name, model string) ai.Provider {
	t.Helper()
	noTemp := math.NaN()
	keys, err := resolveAPIKeys()
	if err != nil {
		t.Fatalf("resolveAPIKeys: %v", err)
	}
	switch name {
	case "anthropic":
		p, err := ai.NewAnthropicProvider(keys.anthropic, model, noTemp)
		if err != nil {
			t.Fatalf("NewAnthropicProvider(%q): %v", model, err)
		}
		return p
	case "openai":
		p, err := ai.NewOpenAIProvider(keys.openai, model, noTemp)
		if err != nil {
			t.Fatalf("NewOpenAIProvider(%q): %v", model, err)
		}
		return p
	case "gemini":
		// Preview models require "global" location; mirror the factory's override.
		location := *geminiLocation
		if strings.Contains(model, "preview") {
			location = "global"
		}
		p, err := ai.NewGeminiProvider(context.Background(), model, *geminiProject, location, noTemp)
		if err != nil {
			t.Fatalf("NewGeminiProvider(%q): %v", model, err)
		}
		return p
	default:
		t.Fatalf("unknown provider name %q", name)
		return nil
	}
}

// firstNonEmpty returns a if non-empty, else b. Used by the eval suites to
// apply per-case region/time defaults from the goldens file.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
