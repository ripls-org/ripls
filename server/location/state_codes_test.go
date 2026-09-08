package location

import "testing"

func TestExtractStateCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// All 50 states — exact canonical names
		{"Alabama", "AL"},
		{"Alaska", "AK"},
		{"Arizona", "AZ"},
		{"Arkansas", "AR"},
		{"California", "CA"},
		{"Colorado", "CO"},
		{"Connecticut", "CT"},
		{"Delaware", "DE"},
		{"Florida", "FL"},
		{"Georgia", "GA"},
		{"Hawaii", "HI"},
		{"Idaho", "ID"},
		{"Illinois", "IL"},
		{"Indiana", "IN"},
		{"Iowa", "IA"},
		{"Kansas", "KS"},
		{"Kentucky", "KY"},
		{"Louisiana", "LA"},
		{"Maine", "ME"},
		{"Maryland", "MD"},
		{"Massachusetts", "MA"},
		{"Michigan", "MI"},
		{"Minnesota", "MN"},
		{"Mississippi", "MS"},
		{"Missouri", "MO"},
		{"Montana", "MT"},
		{"Nebraska", "NE"},
		{"Nevada", "NV"},
		{"New Hampshire", "NH"},
		{"New Jersey", "NJ"},
		{"New Mexico", "NM"},
		{"New York", "NY"},
		{"North Carolina", "NC"},
		{"North Dakota", "ND"},
		{"Ohio", "OH"},
		{"Oklahoma", "OK"},
		{"Oregon", "OR"},
		{"Pennsylvania", "PA"},
		{"Rhode Island", "RI"},
		{"South Carolina", "SC"},
		{"South Dakota", "SD"},
		{"Tennessee", "TN"},
		{"Texas", "TX"},
		{"Utah", "UT"},
		{"Vermont", "VT"},
		{"Virginia", "VA"},
		{"Washington", "WA"},
		{"West Virginia", "WV"},
		{"Wisconsin", "WI"},
		{"Wyoming", "WY"},

		// District of Columbia — all name variants geocoding providers emit
		{"District of Columbia", "DC"},
		{"Washington, D.C.", "DC"},
		{"Washington D.C.", "DC"},
		{"Washington DC", "DC"},
		{"D.C.", "DC"},
		{"DC", "DC"},

		// Inhabited territories
		{"Puerto Rico", "PR"},
		{"Guam", "GU"},
		{"U.S. Virgin Islands", "VI"},
		{"US Virgin Islands", "VI"},
		{"United States Virgin Islands", "VI"},
		{"Virgin Islands", "VI"},
		{"American Samoa", "AS"},
		{"Northern Mariana Islands", "MP"},
		{"Commonwealth of the Northern Mariana Islands", "MP"},

		// Case insensitivity
		{"colorado", "CO"},
		{"COLORADO", "CO"},
		{"CoLoRaDo", "CO"},
		{"new york", "NY"},
		{"NEW YORK", "NY"},

		// Whitespace trimming
		{"  Colorado  ", "CO"},
		{"\tNew York\n", "NY"},

		// Empty input → empty output
		{"", ""},

		// Non-US inputs → empty output (intentional: callers handle gracefully)
		{"Ontario", ""},
		{"São Paulo", ""},
		{"Bavaria", ""},
		{"Queensland", ""},
	}

	for _, tc := range tests {
		got := ExtractStateCode(tc.input)
		if got != tc.want {
			t.Errorf("ExtractStateCode(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
