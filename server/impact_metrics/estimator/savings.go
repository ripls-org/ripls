package estimator

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// TransactionType discriminates how savings are calculated.
type TransactionType int

const (
	TransactionLoan TransactionType = iota
	TransactionGiveaway
	TransactionRequestFulfilled
	TransactionExperienceConcluded
)

// EstimateMoneySaved estimates the money saved by a transaction.
// Loans: value × loan_prevented_purchase_rate.
// Giveaways: value × giveaway_prevented_purchase_rate.
// Requests/experiences: value passed through (already estimated at creation).
func EstimateMoneySaved(valueUSD float32, txType TransactionType, cfg *Config) *api.MoneySavings {
	if valueUSD <= 0 {
		return nil
	}

	var rate float32
	var reasoning string
	var name string
	var sources []string

	switch txType {
	case TransactionLoan:
		rate = cfg.Global.GetLoanPreventedPurchaseRate()
		reasoning = fmt.Sprintf("Loan: $%.2f × %.0f%% prevented purchase rate.", valueUSD, rate*100)
		name = ProvenancePreventedPurchase
		sources = []string{"Library of Things UK — prevented purchase rate survey data"}
	case TransactionGiveaway:
		rate = cfg.Global.GetGiveawayPreventedPurchaseRate()
		reasoning = fmt.Sprintf("Giveaway: $%.2f × %.0f%% prevented purchase rate.", valueUSD, rate*100)
		name = ProvenancePreventedPurchase
		sources = []string{"Library of Things UK — prevented purchase rate survey data"}
	case TransactionRequestFulfilled:
		rate = 1.0
		reasoning = fmt.Sprintf("Request fulfilled: $%.2f equivalent service value.", valueUSD)
		name = "service_value"
		sources = []string{"Community labor valuation"}
	case TransactionExperienceConcluded:
		rate = 1.0
		reasoning = fmt.Sprintf("Experience: $%.2f equivalent commercial value.", valueUSD)
		name = "commercial_value"
		sources = []string{"Commercial equivalent estimation"}
	default:
		return nil
	}

	savedUSD := valueUSD * rate
	relStddev := cfg.Global.GetMoneySavedRelativeStddev()
	return &api.MoneySavings{
		ValueUsd: RelativeUncertainty(savedUSD, relStddev),
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:      name,
			Version:   cfg.ProvenanceVersion(name),
			Reasoning: proto.String(reasoning),
			Sources:   sources,
		},
	}
}

// EmissionsInput provides the data needed to calculate prevented emissions.
// For gear (loans/giveaways): EmbodiedCarbon and WeightGrams are required.
// For requests/experiences: flat defaults from config are used.
type EmissionsInput struct {
	// EmbodiedCarbon is the item's estimated manufacturing carbon (for gear).
	EmbodiedCarbon *api.CarbonEstimate

	// WeightGrams is the item's estimated weight (for waste carbon calculation).
	WeightGrams float32
}

// EstimateEmissionsPrevented estimates carbon emissions prevented by a transaction.
//
// For gear (loans/giveaways):
//   - ManufactureAvoidedCarbon = embodied_carbon × prevented_purchase_rate
//   - WasteReducedCarbon = weight_kg × waste_co2e_per_kg × 1000 (to grams)
//
// For requests: flat default from config.
// For experiences: flat default from config.
func EstimateEmissionsPrevented(input *EmissionsInput, txType TransactionType, cfg *Config) *api.PreventedEmissions {
	switch txType {
	case TransactionLoan, TransactionGiveaway:
		return calculateGearEmissionsPrevented(input, txType, cfg)
	case TransactionRequestFulfilled:
		return calculateRequestEmissionsPrevented(cfg)
	case TransactionExperienceConcluded:
		return calculateExperienceEmissionsPrevented(cfg)
	default:
		return nil
	}
}

// calculateGearEmissionsPrevented computes emissions prevented for gear loans/giveaways.
func calculateGearEmissionsPrevented(input *EmissionsInput, txType TransactionType, cfg *Config) *api.PreventedEmissions {
	if input == nil {
		return nil
	}

	var rate float32
	var reasoning string
	switch txType {
	case TransactionLoan:
		rate = cfg.Global.GetLoanPreventedPurchaseRate()
		reasoning = fmt.Sprintf("Loan prevents %.0f%% of manufacturing emissions", rate*100)
	case TransactionGiveaway:
		rate = cfg.Global.GetGiveawayPreventedPurchaseRate()
		reasoning = fmt.Sprintf("Giveaway prevents %.0f%% of manufacturing emissions", rate*100)
	default:
		return nil
	}

	result := &api.PreventedEmissions{
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Reasoning: proto.String(reasoning),
			Sources:   []string{"Edinburgh Tool Library / ICE Database — material emission factors"},
		},
	}

	// Manufacture avoided carbon: embodied_carbon × prevented_purchase_rate.
	if input.EmbodiedCarbon != nil && input.EmbodiedCarbon.Co2EGrams != nil && input.EmbodiedCarbon.Co2EGrams.Mean > 0 {
		scaled := ScaleEstimate(input.EmbodiedCarbon.Co2EGrams, rate)
		result.ManufactureAvoidedCarbon = &api.CarbonEstimate{
			Co2EGrams:  scaled,
			Provenance: input.EmbodiedCarbon.Provenance,
		}
		// Derive top-level provenance name + version from primary carbon component.
		if input.EmbodiedCarbon.Provenance != nil {
			result.Provenance.Name = input.EmbodiedCarbon.Provenance.Name
			result.Provenance.Version = input.EmbodiedCarbon.Provenance.Version
		}
	}

	// Waste reduced carbon: weight_kg × waste_co2e_per_kg (converted to grams CO2e).
	if input.WeightGrams > 0 {
		weightKg := input.WeightGrams / 1000
		wasteFactor := cfg.Global.GetWasteCo2EPerKg()
		wasteGrams := weightKg * wasteFactor * 1000
		wasteScaled := ScaleEstimate(
			RelativeUncertainty(wasteGrams, cfg.MethodStddev("weight_material_v1")),
			rate,
		)
		wasteProvName := ProvenanceWeightMaterialCarbon
		result.WasteReducedCarbon = &api.CarbonEstimate{
			Co2EGrams: wasteScaled,
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    wasteProvName,
				Version: cfg.ProvenanceVersion(wasteProvName),
			},
		}
		// If no manufacture carbon set the top-level name, use waste method name.
		if result.Provenance.Name == "" {
			result.Provenance.Name = wasteProvName
			result.Provenance.Version = cfg.ProvenanceVersion(wasteProvName)
		}
	}

	// Return nil if neither component could be computed.
	if result.ManufactureAvoidedCarbon == nil && result.WasteReducedCarbon == nil {
		return nil
	}

	return result
}

// calculateRequestEmissionsPrevented returns a flat default for requests.
func calculateRequestEmissionsPrevented(cfg *Config) *api.PreventedEmissions {
	grams := cfg.Global.GetRequestDefaultCarbonGrams()
	if grams <= 0 {
		return nil
	}
	provName := ProvenanceSpendBasedCarbon
	provVersion := cfg.ProvenanceVersion(provName)
	return &api.PreventedEmissions{
		ManufactureAvoidedCarbon: &api.CarbonEstimate{
			Co2EGrams: RelativeUncertainty(grams, cfg.MethodStddev("spend_based_v1")),
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    provName,
				Version: provVersion,
			},
		},
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:      provName,
			Version:   provVersion,
			Reasoning: proto.String("Flat default estimate for request."),
			Sources:   []string{"Spend-based EEIO emission factors"},
		},
	}
}

// calculateExperienceEmissionsPrevented returns a flat default for experiences.
func calculateExperienceEmissionsPrevented(cfg *Config) *api.PreventedEmissions {
	grams := cfg.Global.GetExperienceDefaultCarbonGrams()
	if grams <= 0 {
		return nil
	}
	provName := ProvenanceSpendBasedCarbon
	provVersion := cfg.ProvenanceVersion(provName)
	return &api.PreventedEmissions{
		ManufactureAvoidedCarbon: &api.CarbonEstimate{
			Co2EGrams: RelativeUncertainty(grams, cfg.MethodStddev("spend_based_v1")),
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    provName,
				Version: provVersion,
			},
		},
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:      provName,
			Version:   provVersion,
			Reasoning: proto.String("Flat default estimate for experience."),
			Sources:   []string{"Spend-based EEIO emission factors"},
		},
	}
}

// ScaleTimeSaved scales a base time estimate by a multiplier (e.g., attendee count).
// The multiplier scales the base estimate (e.g., attendee count for experiences).
func ScaleTimeSaved(timeSavings *api.TimeSavings, multiplier int32) *api.TimeSavings {
	if timeSavings == nil || timeSavings.Minutes == nil || timeSavings.Minutes.Mean <= 0 {
		return nil
	}
	if multiplier <= 0 {
		multiplier = 1
	}
	return &api.TimeSavings{
		Minutes:    ScaleEstimate(timeSavings.Minutes, float32(multiplier)),
		Provenance: timeSavings.Provenance,
	}
}
