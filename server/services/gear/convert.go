package gear

import (
	"go.ripls.org/ripls/server/gen/ripls/models"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

// convertAPIValueEstimateToStorage converts an API ValueEstimate to a storage model ValueEstimate.
func convertAPIValueEstimateToStorage(apiEstimate *api.ValueEstimate) *models.ValueEstimate {
	if apiEstimate == nil {
		return nil
	}
	result := &models.ValueEstimate{
		EstimatedValueUsd: apiEstimate.EstimatedValueUsd,
	}
	if apiEstimate.Provenance != nil {
		result.Provenance = &models.Provenance{
			Source:     models.ProvenanceSource(apiEstimate.Provenance.Source),
			Name:       apiEstimate.Provenance.Name,
			Version:    apiEstimate.Provenance.Version,
			Confidence: apiEstimate.Provenance.Confidence,
			Reasoning:  apiEstimate.Provenance.Reasoning,
			Sources:    apiEstimate.Provenance.Sources,
		}
	}
	return result
}

// convertStorageValueEstimateToAPI converts a storage model ValueEstimate to an API ValueEstimate.
func convertStorageValueEstimateToAPI(storageEstimate *models.ValueEstimate) *api.ValueEstimate {
	if storageEstimate == nil {
		return nil
	}
	result := &api.ValueEstimate{
		EstimatedValueUsd: storageEstimate.EstimatedValueUsd,
	}
	if storageEstimate.Provenance != nil {
		result.Provenance = &api.Provenance{
			Source:     api.ProvenanceSource(storageEstimate.Provenance.Source),
			Name:       storageEstimate.Provenance.Name,
			Version:    storageEstimate.Provenance.Version,
			Confidence: storageEstimate.Provenance.Confidence,
			Reasoning:  storageEstimate.Provenance.Reasoning,
			Sources:    storageEstimate.Provenance.Sources,
		}
	}
	return result
}

// convertStorageEstimateToAPI converts a storage model Estimate to an API Estimate.
func convertStorageEstimateToAPI(storageEstimate *models.Estimate) *api.Estimate {
	if storageEstimate == nil {
		return nil
	}
	return &api.Estimate{
		Mean:   storageEstimate.Mean,
		Stddev: storageEstimate.Stddev,
	}
}

// convertStorageCarbonEstimateToAPI converts a storage model CarbonEstimate to an API CarbonEstimate.
func convertStorageCarbonEstimateToAPI(storageEstimate *models.CarbonEstimate) *api.CarbonEstimate {
	if storageEstimate == nil {
		return nil
	}
	result := &api.CarbonEstimate{
		Co2EGrams: convertStorageEstimateToAPI(storageEstimate.Co2EGrams),
	}
	if storageEstimate.Provenance != nil {
		result.Provenance = &api.Provenance{
			Source:    api.ProvenanceSource(storageEstimate.Provenance.Source),
			Name:      storageEstimate.Provenance.Name,
			Version:   storageEstimate.Provenance.Version,
			Reasoning: storageEstimate.Provenance.Reasoning,
			Sources:   storageEstimate.Provenance.Sources,
		}
	}
	return result
}

// convertStorageGearMetadataToAPI converts storage gear metadata fields to an API GearMetadata.
func convertStorageGearMetadataToAPI(gearStored *models.Gear) *api.GearMetadata {
	// Return nil if all metadata fields are empty/unset
	if gearStored.Category == nil &&
		gearStored.Brand == nil &&
		gearStored.Model == nil &&
		gearStored.MaterialCategory == nil &&
		gearStored.WeightGrams == nil &&
		gearStored.EmbodiedCarbon == nil {
		return nil
	}

	return &api.GearMetadata{
		Category:         convertStorageTrackedStringToAPI(gearStored.Category),
		Brand:            convertStorageTrackedStringToAPI(gearStored.Brand),
		Model:            convertStorageTrackedStringToAPI(gearStored.Model),
		MaterialCategory: convertStorageTrackedMaterialCategoryToAPI(gearStored.MaterialCategory),
		WeightGrams:      convertStorageTrackedEstimateToAPI(gearStored.WeightGrams),
		EmbodiedCarbon:   convertStorageCarbonEstimateToAPI(gearStored.EmbodiedCarbon),
	}
}

// convertAPITrackedStringToStorage converts an API TrackedString to a storage model TrackedString.
func convertAPITrackedStringToStorage(ts *api.TrackedString) *models.TrackedString {
	if ts == nil {
		return nil
	}
	result := &models.TrackedString{Value: ts.Value}
	if ts.Provenance != nil {
		result.Provenance = convertAPIProvenanceToStorage(ts.Provenance)
	}
	return result
}

// convertAPITrackedMaterialCategoryToStorage converts an API TrackedMaterialCategory to storage.
func convertAPITrackedMaterialCategoryToStorage(tmc *api.TrackedMaterialCategory) *models.TrackedMaterialCategory {
	if tmc == nil {
		return nil
	}
	result := &models.TrackedMaterialCategory{Value: models.MaterialCategory(tmc.Value)}
	if tmc.Provenance != nil {
		result.Provenance = convertAPIProvenanceToStorage(tmc.Provenance)
	}
	return result
}

// convertAPITrackedEstimateToStorage converts an API TrackedEstimate to storage.
func convertAPITrackedEstimateToStorage(te *api.TrackedEstimate) *models.TrackedEstimate {
	if te == nil {
		return nil
	}
	result := &models.TrackedEstimate{}
	if te.Value != nil {
		result.Value = &models.Estimate{Mean: te.Value.Mean, Stddev: te.Value.Stddev}
	}
	if te.Provenance != nil {
		result.Provenance = convertAPIProvenanceToStorage(te.Provenance)
	}
	return result
}

// convertAPIProvenanceToStorage converts an API Provenance to a storage model Provenance.
func convertAPIProvenanceToStorage(p *api.Provenance) *models.Provenance {
	if p == nil {
		return nil
	}
	return &models.Provenance{
		Source:     models.ProvenanceSource(p.Source),
		Name:       p.Name,
		Version:    p.Version,
		Confidence: p.Confidence,
		Reasoning:  p.Reasoning,
		Sources:    p.Sources,
	}
}

// convertStorageTrackedStringToAPI converts a storage TrackedString to an API TrackedString.
func convertStorageTrackedStringToAPI(ts *models.TrackedString) *api.TrackedString {
	if ts == nil {
		return nil
	}
	result := &api.TrackedString{Value: ts.Value}
	if ts.Provenance != nil {
		result.Provenance = convertStorageProvenanceToAPI(ts.Provenance)
	}
	return result
}

// convertStorageTrackedMaterialCategoryToAPI converts a storage TrackedMaterialCategory to API.
func convertStorageTrackedMaterialCategoryToAPI(tmc *models.TrackedMaterialCategory) *api.TrackedMaterialCategory {
	if tmc == nil {
		return nil
	}
	result := &api.TrackedMaterialCategory{Value: api.MaterialCategory(tmc.Value)}
	if tmc.Provenance != nil {
		result.Provenance = convertStorageProvenanceToAPI(tmc.Provenance)
	}
	return result
}

// convertStorageTrackedEstimateToAPI converts a storage TrackedEstimate to an API TrackedEstimate.
func convertStorageTrackedEstimateToAPI(te *models.TrackedEstimate) *api.TrackedEstimate {
	if te == nil {
		return nil
	}
	result := &api.TrackedEstimate{}
	if te.Value != nil {
		result.Value = &api.Estimate{Mean: te.Value.Mean, Stddev: te.Value.Stddev}
	}
	if te.Provenance != nil {
		result.Provenance = convertStorageProvenanceToAPI(te.Provenance)
	}
	return result
}

// convertStorageProvenanceToAPI converts a storage Provenance to an API Provenance.
func convertStorageProvenanceToAPI(p *models.Provenance) *api.Provenance {
	if p == nil {
		return nil
	}
	return &api.Provenance{
		Source:     api.ProvenanceSource(p.Source),
		Name:       p.Name,
		Version:    p.Version,
		Confidence: p.Confidence,
		Reasoning:  p.Reasoning,
		Sources:    p.Sources,
	}
}

// trackedStringChanged reports whether the incoming TrackedString value differs from the stored one.
func trackedStringChanged(stored, incoming *models.TrackedString) bool {
	if stored == nil && incoming == nil {
		return false
	}
	if stored == nil || incoming == nil {
		return true
	}
	return stored.Value != incoming.Value
}

// trackedMaterialCategoryChanged reports whether the incoming material category differs from stored.
func trackedMaterialCategoryChanged(stored, incoming *models.TrackedMaterialCategory) bool {
	if stored == nil && incoming == nil {
		return false
	}
	if stored == nil || incoming == nil {
		return true
	}
	return stored.Value != incoming.Value
}

// trackedEstimateChanged reports whether the incoming estimate mean differs from stored.
func trackedEstimateChanged(stored, incoming *models.TrackedEstimate) bool {
	if stored == nil && incoming == nil {
		return false
	}
	if stored == nil || incoming == nil {
		return true
	}
	storedMean := float32(0)
	if stored.Value != nil {
		storedMean = stored.Value.Mean
	}
	incomingMean := float32(0)
	if incoming.Value != nil {
		incomingMean = incoming.Value.Mean
	}
	return storedMean != incomingMean
}

// trackedStringValue extracts the string value from a TrackedString, returning "" if nil.
func trackedStringValue(ts *models.TrackedString) string {
	if ts == nil {
		return ""
	}
	return ts.Value
}

// trackedMaterialCategoryValue extracts the MaterialCategory from a TrackedMaterialCategory.
func trackedMaterialCategoryValue(tmc *models.TrackedMaterialCategory) models.MaterialCategory {
	if tmc == nil {
		return models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED
	}
	return tmc.Value
}

// trackedEstimateMean extracts the mean from a TrackedEstimate, returning 0 if nil.
func trackedEstimateMean(te *models.TrackedEstimate) float32 {
	if te == nil || te.Value == nil {
		return 0
	}
	return te.Value.Mean
}

// generationModeToProvenanceName maps a generation mode string to a provenance name.
// Returns empty string for unrecognized or empty modes.
func generationModeToProvenanceName(mode string) string {
	switch mode {
	case "text":
		return "genai_from_text"
	case "image":
		return "genai_from_image"
	case "web":
		return "genai_from_web"
	default:
		return ""
	}
}

// buildMetadataProvenance builds an LLM provenance for AI-generated metadata fields.
// Returns nil if the mode is empty or unrecognized.
func (s *Service) buildMetadataProvenance(mode string) *models.Provenance {
	name := generationModeToProvenanceName(mode)
	if name == "" {
		return nil
	}
	prov := &models.Provenance{
		Source: models.ProvenanceSource_PROVENANCE_SOURCE_LLM,
		Name:   name,
	}
	if s.estimatorCfg != nil {
		prov.Version = s.estimatorCfg.ProvenanceVersion(name)
	}
	return prov
}

// applyDefaultProvenanceTrackedString sets provenance on a TrackedString if it has a value but no provenance.
func applyDefaultProvenanceTrackedString(ts *models.TrackedString, prov *models.Provenance) {
	if ts == nil || ts.Value == "" || prov == nil {
		return
	}
	if ts.Provenance == nil {
		ts.Provenance = prov
	}
}

// applyDefaultProvenanceTrackedMaterialCategory sets provenance on a TrackedMaterialCategory if it has a value but no provenance.
func applyDefaultProvenanceTrackedMaterialCategory(tmc *models.TrackedMaterialCategory, prov *models.Provenance) {
	if tmc == nil || tmc.Value == models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED || prov == nil {
		return
	}
	if tmc.Provenance == nil {
		tmc.Provenance = prov
	}
}

// applyDefaultProvenanceTrackedEstimate sets provenance on a TrackedEstimate if it has a value but no provenance.
func applyDefaultProvenanceTrackedEstimate(te *models.TrackedEstimate, prov *models.Provenance) {
	if te == nil || te.Value == nil || te.Value.Mean == 0 || prov == nil {
		return
	}
	if te.Provenance == nil {
		te.Provenance = prov
	}
}
