package itemkind

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
)

// FromDailyItemType converts a legacy DailyItemType to the unified
// ItemKind. Returns ITEM_KIND_UNSPECIFIED when the input is the
// zero value or otherwise unrecognized.
func FromDailyItemType(t api.DailyItemType) api.ItemKind {
	switch t {
	case api.DailyItemType_DAILY_ITEM_TYPE_TRANSFER:
		return api.ItemKind_ITEM_KIND_TRANSFER
	case api.DailyItemType_DAILY_ITEM_TYPE_EXPERIENCE:
		return api.ItemKind_ITEM_KIND_EXPERIENCE
	case api.DailyItemType_DAILY_ITEM_TYPE_REQUEST:
		return api.ItemKind_ITEM_KIND_REQUEST
	case api.DailyItemType_DAILY_ITEM_TYPE_GIVEAWAY:
		return api.ItemKind_ITEM_KIND_GIVEAWAY
	case api.DailyItemType_DAILY_ITEM_TYPE_COMMUNITY:
		return api.ItemKind_ITEM_KIND_COMMUNITY
	default:
		return api.ItemKind_ITEM_KIND_UNSPECIFIED
	}
}

// FromCTAActionString interprets the legacy cta_action string used by
// BriefCTARow and BriefLever. When the value matches a known
// item-kind string ("gear", "experience", "request"), returns the
// matching ItemKind and true. Otherwise returns
// ITEM_KIND_UNSPECIFIED and false — the caller treats the action as
// a non-entity verb (e.g. "schedule_repeat", "revive_experience")
// that belongs in action_name rather than the item's kind.
func FromCTAActionString(s string) (api.ItemKind, bool) {
	switch s {
	case "gear":
		return api.ItemKind_ITEM_KIND_GEAR, true
	case "experience":
		return api.ItemKind_ITEM_KIND_EXPERIENCE, true
	case "request":
		return api.ItemKind_ITEM_KIND_REQUEST, true
	default:
		return api.ItemKind_ITEM_KIND_UNSPECIFIED, false
	}
}

// ToScreenRouteName returns the legacy item-type string the Dart
// NavigationHelpers.pushToItemScreen helper accepts ("gear",
// "experience", "request"). TRANSFER and GIVEAWAY collapse onto
// "gear" because they both route to GearScreen on tap. Returns the
// empty string for kinds that have no tap routing (UNSPECIFIED,
// COMMUNITY).
func ToScreenRouteName(k api.ItemKind) string {
	switch k {
	case api.ItemKind_ITEM_KIND_GEAR,
		api.ItemKind_ITEM_KIND_GIVEAWAY,
		api.ItemKind_ITEM_KIND_TRANSFER:
		return "gear"
	case api.ItemKind_ITEM_KIND_EXPERIENCE:
		return "experience"
	case api.ItemKind_ITEM_KIND_REQUEST:
		return "request"
	default:
		return ""
	}
}
