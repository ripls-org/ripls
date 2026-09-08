package ai

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

func TestEveryMaterialCategoryHasJSONMapping(t *testing.T) {
	for num, name := range api.MaterialCategory_name {
		cat := api.MaterialCategory(num)
		if cat == api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED {
			continue
		}
		token := MaterialCategoryJSON(cat)
		if token == "" {
			t.Errorf("MaterialCategoryJSON(%s) returned empty string", name)
			continue
		}
		roundTrip := MaterialCategoryFromJSON(token)
		if roundTrip != cat {
			t.Errorf("round-trip %s: MaterialCategoryFromJSON(%q) = %v, want %v", name, token, roundTrip, cat)
		}
	}
}

func TestMaterialCategoryFromJSONUnknown(t *testing.T) {
	cases := []string{"", "unknown", "BogusValue", "notreal_category", "MATERIAL_CATEGORY_SOLID_METAL"}
	for _, s := range cases {
		got := MaterialCategoryFromJSON(s)
		if got != api.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED {
			t.Errorf("MaterialCategoryFromJSON(%q) = %v, want UNSPECIFIED", s, got)
		}
	}
}

func TestMaterialCategoryFromJSONLowercases(t *testing.T) {
	cases := []struct {
		input string
		want  api.MaterialCategory
	}{
		{"Solid_Metal", api.MaterialCategory_MATERIAL_CATEGORY_SOLID_METAL},
		{"SOLID_METAL", api.MaterialCategory_MATERIAL_CATEGORY_SOLID_METAL},
		{" solid_metal ", api.MaterialCategory_MATERIAL_CATEGORY_SOLID_METAL},
		{"mixed_plastic_metal", api.MaterialCategory_MATERIAL_CATEGORY_MIXED_PLASTIC_METAL},
		{"electronics_small", api.MaterialCategory_MATERIAL_CATEGORY_ELECTRONICS_SMALL},
	}
	for _, tc := range cases {
		got := MaterialCategoryFromJSON(tc.input)
		if got != tc.want {
			t.Errorf("MaterialCategoryFromJSON(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
