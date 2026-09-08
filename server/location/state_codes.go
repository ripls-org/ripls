package location

import "strings"

// stateCodeMap maps lowercase, trimmed US state and territory names to their
// USPS two-letter codes. Keys are normalized with strings.ToLower +
// strings.TrimSpace before lookup.
//
// The map covers:
//   - 50 US states
//   - District of Columbia (multiple name variants Mapbox and Google emit)
//   - 5 inhabited US territories: PR, GU, VI, AS, MP
//
// Non-US or unrecognized inputs return "" intentionally — the caller still
// creates the state region with the full region_name; only the region_code
// is left empty, which matches today's behavior for non-US callers.
//
// TODO(#2807): Backfill existing state regions that were written with an
// empty region_code before this fix landed. State rows written before this
// change have region_code="" because extractStateCode was a stub. After this
// fix ships, new writes get the correct two-letter code but old rows remain
// stale. The backfill also needs to rescope city region_name values from
// "Boulder" to "Boulder, CO" for rows written when the state-code suffix
// was silently dropped. See the issue for the target SQL and rate-limit notes.
var stateCodeMap = map[string]string{
	// 50 states
	"alabama":        "AL",
	"alaska":         "AK",
	"arizona":        "AZ",
	"arkansas":       "AR",
	"california":     "CA",
	"colorado":       "CO",
	"connecticut":    "CT",
	"delaware":       "DE",
	"florida":        "FL",
	"georgia":        "GA",
	"hawaii":         "HI",
	"idaho":          "ID",
	"illinois":       "IL",
	"indiana":        "IN",
	"iowa":           "IA",
	"kansas":         "KS",
	"kentucky":       "KY",
	"louisiana":      "LA",
	"maine":          "ME",
	"maryland":       "MD",
	"massachusetts":  "MA",
	"michigan":       "MI",
	"minnesota":      "MN",
	"mississippi":    "MS",
	"missouri":       "MO",
	"montana":        "MT",
	"nebraska":       "NE",
	"nevada":         "NV",
	"new hampshire":  "NH",
	"new jersey":     "NJ",
	"new mexico":     "NM",
	"new york":       "NY",
	"north carolina": "NC",
	"north dakota":   "ND",
	"ohio":           "OH",
	"oklahoma":       "OK",
	"oregon":         "OR",
	"pennsylvania":   "PA",
	"rhode island":   "RI",
	"south carolina": "SC",
	"south dakota":   "SD",
	"tennessee":      "TN",
	"texas":          "TX",
	"utah":           "UT",
	"vermont":        "VT",
	"virginia":       "VA",
	"washington":     "WA",
	"west virginia":  "WV",
	"wisconsin":      "WI",
	"wyoming":        "WY",

	// District of Columbia — multiple name variants from geocoding providers.
	// Mapbox emits "Washington, D.C."; Google emits "District of Columbia".
	"district of columbia": "DC",
	"washington, d.c.":     "DC",
	"washington d.c.":      "DC",
	"washington dc":        "DC",
	"d.c.":                 "DC",
	"dc":                   "DC",

	// Inhabited US territories
	"puerto rico":                  "PR",
	"guam":                         "GU",
	"u.s. virgin islands":          "VI",
	"us virgin islands":            "VI",
	"united states virgin islands": "VI",
	"virgin islands":               "VI",
	"american samoa":               "AS",
	"northern mariana islands":     "MP",
	"commonwealth of the northern mariana islands": "MP",
}

// ExtractStateCode returns the USPS two-letter code for a US state or territory
// name (e.g., "Colorado" → "CO"). Lookup is case-insensitive on trimmed input.
//
// Returns "" for unrecognized or non-US inputs — this is intentional: callers
// still create the state region with the full region_name; only the code is
// left empty, preserving today's behavior for non-US callers and gracefully
// handling any new Mapbox/Google spelling variant we haven't seen yet. A Warn
// log at each call site surfaces unexpected empty results as a drift signal.
func ExtractStateCode(stateName string) string {
	return stateCodeMap[strings.ToLower(strings.TrimSpace(stateName))]
}
