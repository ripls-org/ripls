package weather

// conditionFromWMO maps a WMO weather-interpretation code (the `weather_code`
// Open-Meteo returns) onto one of the five coarse buckets. Precipitation of any
// kind — drizzle, rain, freezing rain, snow, showers, thunderstorm — collapses
// to ConditionRain, since the calendar's glyph scheme has a single "wet" glyph.
//
// Reference: WMO code table 4677, as surfaced by Open-Meteo's `weather_code`.
func conditionFromWMO(code int) Condition {
	switch code {
	case 0:
		return ConditionClear
	case 1:
		return ConditionMostlySunny
	case 2:
		return ConditionPartlyCloudy
	case 3:
		return ConditionOvercast
	case 45, 48: // fog / depositing rime fog
		return ConditionOvercast
	case 51, 53, 55, // drizzle
		56, 57, // freezing drizzle
		61, 63, 65, // rain
		66, 67, // freezing rain
		71, 73, 75, 77, // snow
		80, 81, 82, // rain showers
		85, 86, // snow showers
		95, 96, 99: // thunderstorm
		return ConditionRain
	default:
		return ConditionUnknown
	}
}
