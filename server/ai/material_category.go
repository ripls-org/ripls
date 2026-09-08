package ai

import (
	"strings"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

var (
	materialCategoryByJSON map[string]api.MaterialCategory
	materialCategoryToJSON map[api.MaterialCategory]string
)

// enum names. Pure and dependency-free, and the maps must exist before first use.
//
//nolint:gochecknoinits // derives the MaterialCategory JSON lookup maps from the generated
func init() {
	prefix := "MATERIAL_CATEGORY_"
	materialCategoryByJSON = make(map[string]api.MaterialCategory, len(api.MaterialCategory_name))
	materialCategoryToJSON = make(map[api.MaterialCategory]string, len(api.MaterialCategory_name))
	for num, name := range api.MaterialCategory_name {
		cat := api.MaterialCategory(num)
		if cat == api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		token := strings.ToLower(strings.TrimPrefix(name, prefix))
		materialCategoryByJSON[token] = cat
		materialCategoryToJSON[cat] = token
	}
}

// MaterialCategoryFromJSON converts a JSON token (e.g. "solid_metal") to the
// corresponding api.MaterialCategory enum value. Returns UNSPECIFIED for
// unrecognized or empty input. Input is trimmed and lowercased before lookup.
func MaterialCategoryFromJSON(s string) api.MaterialCategory {
	token := strings.ToLower(strings.TrimSpace(s))
	if token == "" {
		return api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED
	}
	if cat, ok := materialCategoryByJSON[token]; ok {
		return cat
	}
	return api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED
}

// MaterialCategoryJSON returns the JSON token for c (e.g.
// MATERIAL_CATEGORY_SOLID_METAL → "solid_metal"). Returns "" for
// UNSPECIFIED or any value not in the enum.
func MaterialCategoryJSON(c api.MaterialCategory) string {
	return materialCategoryToJSON[c]
}
