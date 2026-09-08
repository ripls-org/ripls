package impact_metrics

import (
	"fmt"
	"math"
)

// FormatMoney formats a USD amount with appropriate scaling.
// Returns "$1,241" for small values, "$12.3K" for thousands, "$1.2M" for millions.
func FormatMoney(usd float64) string {
	if usd < 0 {
		return "-" + FormatMoney(-usd)
	}

	if usd == 0 {
		return "$0"
	}

	// Millions
	if usd >= 1_000_000 {
		millions := usd / 1_000_000
		if millions >= 10 {
			return fmt.Sprintf("$%.0fM", millions)
		}
		return fmt.Sprintf("$%.1fM", millions)
	}

	// Thousands
	if usd >= 1_000 {
		thousands := usd / 1_000
		// Below 10K: always show 1 decimal ($1.0K, $5.6K, $9.9K)
		// 10K and above: show decimal only if non-zero ($10K, $12.3K, $450K)
		if thousands < 10 {
			return fmt.Sprintf("$%.1fK", thousands)
		}
		if thousands == float64(int(thousands)) {
			// Whole number, no decimal needed
			return fmt.Sprintf("$%.0fK", thousands)
		}
		return fmt.Sprintf("$%.1fK", thousands)
	}

	// Small amounts (dollars)
	if usd >= 10 {
		return fmt.Sprintf("$%.0f", usd)
	}

	return fmt.Sprintf("$%.2f", usd)
}

// FormatTime formats minutes into human-readable time with appropriate units.
// Returns "18 mins", "2.5 hrs", "3 days" based on magnitude.
func FormatTime(minutes float64) string {
	if minutes < 0 {
		return "-" + FormatTime(-minutes)
	}

	if minutes == 0 {
		return "0 mins"
	}

	// Hours (always use hours for >= 60 minutes, never days)
	if minutes >= 60 {
		hours := minutes / 60
		if hours >= 10 {
			return fmt.Sprintf("%.0f hrs", hours)
		}
		return fmt.Sprintf("%.1f hrs", hours)
	}

	// Minutes
	if minutes >= 10 {
		return fmt.Sprintf("%.0f mins", minutes)
	}

	return fmt.Sprintf("%.1f mins", minutes)
}

// FormatCO2 formats grams of CO2 into human-readable weight with appropriate units.
// Returns "0.51 kg", "50 kg", "1.2 tonnes" based on magnitude.
func FormatCO2(grams float64) string {
	if grams < 0 {
		return "-" + FormatCO2(-grams)
	}

	if grams == 0 {
		return "0 kg"
	}

	// Tonnes (metric tons)
	if grams >= 1_000_000 {
		tonnes := grams / 1_000_000
		if tonnes >= 10 {
			return fmt.Sprintf("%.0f tonnes", tonnes)
		}
		return fmt.Sprintf("%.1f tonnes", tonnes)
	}

	// Kilograms
	kg := grams / 1_000
	if kg >= 10 {
		return fmt.Sprintf("%.0f kg", kg)
	}

	return fmt.Sprintf("%.2f kg", kg)
}

// FormatCount formats an integer count with thousand separators.
// Returns "59", "1,234", "1,234,567".
func FormatCount(count int) string {
	if count < 0 {
		return "-" + FormatCount(-count)
	}

	if count < 1_000 {
		return fmt.Sprintf("%d", count)
	}

	// Add thousand separators
	s := fmt.Sprintf("%d", count)
	n := len(s)

	// Calculate number of commas needed
	numCommas := (n - 1) / 3
	result := make([]byte, n+numCommas)

	// Fill result from right to left
	srcIdx := n - 1
	dstIdx := len(result) - 1
	digitsSinceComma := 0

	for srcIdx >= 0 {
		if digitsSinceComma == 3 {
			result[dstIdx] = ','
			dstIdx--
			digitsSinceComma = 0
		}
		result[dstIdx] = s[srcIdx]
		dstIdx--
		srcIdx--
		digitsSinceComma++
	}

	return string(result)
}

// FormatPercentage formats a float (0.0-1.0 or 0-100) as a percentage.
// Assumes input is already 0-100 range. Returns "27%", "54%", "100%".
func FormatPercentage(percent float64) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	rounded := math.Round(percent)
	return fmt.Sprintf("%.0f%%", rounded)
}
