package itemkind

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

func TestFromDailyItemType(t *testing.T) {
	tests := []struct {
		name  string
		input api.DailyItemType
		want  api.ItemKind
	}{
		{"transfer", api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER, api.ItemKind_ITEM_KIND_TRANSFER},
		{"experience", api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE, api.ItemKind_ITEM_KIND_EXPERIENCE},
		{"request", api.DailyItemType_DAILY_ITEM_TYPE_REQUEST, api.ItemKind_ITEM_KIND_REQUEST},
		{"giveaway", api.DailyItemType_DAILY_ITEM_TYPE_GIVEAWAY, api.ItemKind_ITEM_KIND_GIVEAWAY},
		{"community", api.DailyItemType_DAILY_ITEM_TYPE_COMMUNITY, api.ItemKind_ITEM_KIND_COMMUNITY},
		{"unspecified", api.DailyItemType_DAILY_ITEM_TYPE_UNSPECIFIED, api.ItemKind_ITEM_KIND_UNSPECIFIED},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromDailyItemType(tt.input); got != tt.want {
				t.Errorf("FromDailyItemType(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestEveryDailyItemTypeHasMapping ensures the discriminator
// conversion has an arm for every DailyItemType value, so adding a
// new enum value without a conversion fails the build.
func TestEveryDailyItemTypeHasMapping(t *testing.T) {
	for value, name := range api.DailyItemType_name {
		dt := api.DailyItemType(value)
		// UNSPECIFIED is the zero value; DECISION is a watch-only discriminator
		// (the item_type on DismissInboxItem for a Needs-you card) that never
		// reaches the content-kind conversion — neither maps to an ItemKind.
		if dt == api.DailyItemType_DAILY_ITEM_TYPE_UNSPECIFIED ||
			dt == api.DailyItemType_DAILY_ITEM_TYPE_DECISION {
			continue
		}
		if got := FromDailyItemType(dt); got == api.ItemKind_ITEM_KIND_UNSPECIFIED {
			t.Errorf("DailyItemType value %s (%d) has no itemkind mapping", name, value)
		}
	}
}

func TestFromCTAActionString(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantKind     api.ItemKind
		wantIsEntity bool
	}{
		{"gear", "gear", api.ItemKind_ITEM_KIND_GEAR, true},
		{"experience", "experience", api.ItemKind_ITEM_KIND_EXPERIENCE, true},
		{"request", "request", api.ItemKind_ITEM_KIND_REQUEST, true},
		{"non-entity verb", "schedule_repeat", api.ItemKind_ITEM_KIND_UNSPECIFIED, false},
		{"non-entity verb 2", "revive_experience", api.ItemKind_ITEM_KIND_UNSPECIFIED, false},
		{"empty", "", api.ItemKind_ITEM_KIND_UNSPECIFIED, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotIsEntity := FromCTAActionString(tt.input)
			if gotKind != tt.wantKind || gotIsEntity != tt.wantIsEntity {
				t.Errorf("FromCTAActionString(%q) = (%v, %v), want (%v, %v)",
					tt.input, gotKind, gotIsEntity, tt.wantKind, tt.wantIsEntity)
			}
		})
	}
}

func TestToScreenRouteName(t *testing.T) {
	tests := []struct {
		name  string
		input api.ItemKind
		want  string
	}{
		{"gear", api.ItemKind_ITEM_KIND_GEAR, "gear"},
		{"giveaway maps to gear", api.ItemKind_ITEM_KIND_GIVEAWAY, "gear"},
		{"transfer maps to gear", api.ItemKind_ITEM_KIND_TRANSFER, "gear"},
		{"experience", api.ItemKind_ITEM_KIND_EXPERIENCE, "experience"},
		{"request", api.ItemKind_ITEM_KIND_REQUEST, "request"},
		{"community", api.ItemKind_ITEM_KIND_COMMUNITY, ""},
		{"unspecified", api.ItemKind_ITEM_KIND_UNSPECIFIED, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToScreenRouteName(tt.input); got != tt.want {
				t.Errorf("ToScreenRouteName(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
