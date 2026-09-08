// Client-side timeouts for AI streaming calls. A wedged upstream HTTP/2
// stream blocks indefinitely without one, hanging the caller's Gen* RPC
// until a higher-level handler timeout fires minutes later.
//
// Timeouts are per-(provider, model-family, call-type) because the observed
// wall-clock spread is ~20× — a single global timeout would either kill the
// slow models or let the fast ones hang. Image calls are consistently slower
// than text calls for the same model, so they get a separate, larger budget.
// Values target ~3× p95 per family so a legitimately slow call still
// completes but a stalled stream fails fast.
package ai

import (
	"strings"
	"time"
)

const (
	// defaultStreamTimeout is the fallback for any (provider, model) combination
	// not explicitly matched below. Generous enough to not fire on a legitimately
	// slow call; short enough that a wedged stream fails in under a minute.
	defaultStreamTimeout = 60 * time.Second
)

// streamTimeout returns the maximum wall-clock duration a streaming AI call
// should be allowed to run before the client cancels the context. Matches
// model families by substring because exact model IDs change often (e.g.,
// "gpt-5-mini" vs "gpt-5-mini-2026-03-01") but family naming is stable.
// isImageCall must be true for *FromImage and DetectGear* streaming methods;
// those calls consistently take 2–3× longer than text-mode calls on the same
// model and require a separate, larger budget.
func streamTimeout(provider ProviderType, model string, isImageCall bool) time.Duration {
	m := strings.ToLower(model)

	switch provider {
	case ProviderTypeAnthropic:
		if isImageCall {
			// Haiku image calls: estimated p95 ~20s. 60s gives 3× headroom.
			return 60 * time.Second
		}
		// Haiku 4.5 text median ~2s, p95 ~5s. 15s gives 3× headroom.
		if strings.Contains(m, "haiku") {
			return 15 * time.Second
		}
		// Opus/Sonnet are slower; give them more room.
		return 45 * time.Second

	case ProviderTypeOpenAI:
		if strings.Contains(m, "gpt-5-mini") {
			if isImageCall {
				// Image analysis takes consistently longer than text. 150s gives
				// 2× headroom over the observed text ceiling of 75s.
				return 150 * time.Second
			}
			// Text median ~20s, p95 ~30s. 75s absorbs the tail.
			return 75 * time.Second
		}
		// gpt-5.4-nano and similar nano-class: text median ~1.2s, p95 ~3s.
		if strings.Contains(m, "nano") {
			if isImageCall {
				return 45 * time.Second
			}
			return 15 * time.Second
		}
		// Unknown OpenAI model: assume mini-class latency.
		if isImageCall {
			return 120 * time.Second
		}
		return 60 * time.Second

	case ProviderTypeGemini:
		// flash-lite: text median ~0.8s, p95 ~2s.
		if strings.Contains(m, "flash-lite") {
			if isImageCall {
				return 45 * time.Second
			}
			return 15 * time.Second
		}
		// flash: text median ~3s, but HTTP/2 streams occasionally stall for
		// minutes with no upstream cancellation. 30s fast-fails the stalls
		// while giving ordinary calls 10× headroom.
		if strings.Contains(m, "flash") {
			if isImageCall {
				return 90 * time.Second
			}
			return 30 * time.Second
		}
		// Unknown Gemini model.
		if isImageCall {
			return 120 * time.Second
		}
		return 60 * time.Second
	}

	return defaultStreamTimeout
}
