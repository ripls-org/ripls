package estimator

import (
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// configDefaultSource is the Provenance.Sources citation shared by every
// config-default time estimate.
//
// Provenance.Sources is a citation list, not UI copy — its siblings are
// "Edinburgh Tool Library / ICE Database", "Spend-based EEIO emission
// factors", "social_connection_metrics.md". Citations name a source and are
// not translated, so these stay English by the same convention a bibliography
// does. Adjudicated in #3044; the only thing worth fixing was that this one
// sentence was spelled out three times.
const configDefaultSource = "Research-based default from impact estimation config"

// EstimateGearTime returns a time estimate for gear using the config default.
// Gear time represents shopping/research time avoided by borrowing instead of buying.
func EstimateGearTime(cfg *Config) *api.TimeSavings {
	minutes := float32(cfg.TimeDefaults.GetGearShoppingTimeMinutes())
	if minutes <= 0 {
		return nil
	}
	provName := "gear_shopping_time"
	return &api.TimeSavings{
		Minutes: RelativeUncertainty(minutes, cfg.TimeDefaults.GetDefaultRelativeStddev()),
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
			Name:      provName,
			Version:   cfg.ProvenanceVersion(provName),
			Reasoning: proto.String("Config default: estimated shopping and research time avoided."),
			Sources:   []string{configDefaultSource},
		},
	}
}

// EstimateRequestTime returns a time estimate for a request.
// Uses the AI-provided duration hint if positive, otherwise falls back to config default.
func EstimateRequestTime(durationHintMinutes float32, cfg *Config) *api.TimeSavings {
	if durationHintMinutes > 0 {
		provName := "genai_time_estimate"
		return &api.TimeSavings{
			Minutes: RelativeUncertainty(durationHintMinutes, cfg.TimeDefaults.GetHintRelativeStddev()),
			Provenance: &api.Provenance{
				Source:    api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
				Name:      provName,
				Version:   cfg.ProvenanceVersion(provName),
				Reasoning: proto.String("AI-estimated labor time from request description."),
				Sources:   []string{"AI duration estimation from request context"},
			},
		}
	}

	minutes := float32(cfg.TimeDefaults.GetRequestLaborTimeMinutes())
	if minutes <= 0 {
		return nil
	}
	provName := "request_labor_time"
	return &api.TimeSavings{
		Minutes: RelativeUncertainty(minutes, cfg.TimeDefaults.GetDefaultRelativeStddev()),
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
			Name:      provName,
			Version:   cfg.ProvenanceVersion(provName),
			Reasoning: proto.String("Config default: estimated labor time for fulfillment."),
			Sources:   []string{configDefaultSource},
		},
	}
}

// EstimateExperienceTime returns a time estimate for an experience.
// Uses the AI-provided duration hint if positive, otherwise falls back to config default.
func EstimateExperienceTime(durationHintMinutes float32, cfg *Config) *api.TimeSavings {
	if durationHintMinutes > 0 {
		provName := "genai_time_estimate"
		return &api.TimeSavings{
			Minutes: RelativeUncertainty(durationHintMinutes, cfg.TimeDefaults.GetHintRelativeStddev()),
			Provenance: &api.Provenance{
				Source:    api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
				Name:      provName,
				Version:   cfg.ProvenanceVersion(provName),
				Reasoning: proto.String("AI-estimated activity duration from experience description."),
				Sources:   []string{"AI duration estimation from experience context"},
			},
		}
	}

	minutes := float32(cfg.TimeDefaults.GetExperienceDurationMinutes())
	if minutes <= 0 {
		return nil
	}
	provName := "experience_duration"
	return &api.TimeSavings{
		Minutes: RelativeUncertainty(minutes, cfg.TimeDefaults.GetDefaultRelativeStddev()),
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
			Name:      provName,
			Version:   cfg.ProvenanceVersion(provName),
			Reasoning: proto.String("Config default: estimated experience activity duration."),
			Sources:   []string{configDefaultSource},
		},
	}
}
