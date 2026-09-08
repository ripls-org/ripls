// GENERATED from design/tokens.json by scripts/gen_design_tokens.js — DO NOT EDIT.
// Edit design/tokens.json and run `npm run generate:design-tokens`. CI
// (`npm run lint:design-tokens`) fails if this file drifts from the source.

package email

// BrandColors is the design-token color palette (issue #2441) for one theme,
// as #RRGGBB strings inlined into email templates at render time (email
// clients can't use CSS custom properties). Edit design/tokens.json and run
// `npm run generate:design-tokens` to update every email we send.
type BrandColors struct {
	Background    string
	Surface       string
	Border        string
	TextPrimary   string
	TextSecondary string
	TextFaint     string
	Primary       string
	OnPrimary     string
	PrimaryHover  string
	Accent        string
	OnAccent      string
	Success       string
	Warning       string
	Error         string
	Info          string
}

// LightColors is the light-theme palette.
var LightColors = BrandColors{
	Background:    "#FFFFFF",
	Surface:       "#F2F2EE",
	Border:        "#DDDDD4",
	TextPrimary:   "#191C19",
	TextSecondary: "#4A524C",
	TextFaint:     "#7C837C",
	Primary:       "#3E5A47",
	OnPrimary:     "#F4F7F1",
	PrimaryHover:  "#2F4636",
	Accent:        "#1F2421",
	OnAccent:      "#F4F4EF",
	Success:       "#2E7041",
	Warning:       "#8A6200",
	Error:         "#B5492B",
	Info:          "#3E6471",
}

// DarkColors is the dark-theme palette.
var DarkColors = BrandColors{
	Background:    "#141714",
	Surface:       "#1F2421",
	Border:        "#323832",
	TextPrimary:   "#F4F4EF",
	TextSecondary: "#ABB3AB",
	TextFaint:     "#7B837B",
	Primary:       "#9DBFA8",
	OnPrimary:     "#14241A",
	PrimaryHover:  "#B1CFBA",
	Accent:        "#E6E8E1",
	OnAccent:      "#191C19",
	Success:       "#7A9B76",
	Warning:       "#E8A661",
	Error:         "#DB7F63",
	Info:          "#7FA0A9",
}
