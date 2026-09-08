package ai

import (
	"slices"
	"strings"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

// placeholderPatterns lists known placeholder values that AI models return
// instead of leaving a field empty. All comparisons are case-insensitive.
var placeholderPatterns = []string{
	"unknown",
	"<unknown>",
	"n/a",
	"na",
	"none",
	"not visible",
	"not available",
	"not specified",
	"not found",
	"generic",
	"unspecified",
	"unbranded",
}

// SanitizePlaceholder returns empty string if the value matches a known
// placeholder pattern (case-insensitive, trimmed). Otherwise returns the
// original value unchanged.
func SanitizePlaceholder(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if slices.Contains(placeholderPatterns, lower) {
		return ""
	}
	return value
}

// SanitizeGearDetection clears placeholder values from all string fields
// on a GearDetection returned by an AI provider.
func SanitizeGearDetection(d *GearDetection) {
	if d == nil {
		return
	}
	d.Brand = SanitizePlaceholder(d.Brand)
	d.Model = SanitizePlaceholder(d.Model)
	d.Category = SanitizePlaceholder(d.Category)
	d.MaterialCategory = sanitizeMaterialCategory(d.MaterialCategory)
}

// SanitizeGearGeneration clears placeholder values from all string fields
// on a GearGeneration returned by an AI provider.
func SanitizeGearGeneration(g *GearGeneration) {
	if g == nil {
		return
	}
	g.Brand = SanitizePlaceholder(g.Brand)
	g.Category = SanitizePlaceholder(g.Category)
	g.MaterialCategory = sanitizeMaterialCategory(g.MaterialCategory)
}

// sanitizeMaterialCategory clears the value if it is a placeholder or does not
// parse to a known MaterialCategory enum value.
func sanitizeMaterialCategory(s string) string {
	s = SanitizePlaceholder(s)
	if s == "" {
		return ""
	}
	if MaterialCategoryFromJSON(s) == api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED {
		return ""
	}
	return s
}
