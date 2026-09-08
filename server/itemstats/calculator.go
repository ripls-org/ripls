// Package itemstats provides shared statistics calculation utilities for item types (gear, requests, experiences).
package itemstats

import (
	"time"
)

// CalculateValueShared calculates the total value shared based on an item's estimated value and usage count.
// Returns the value in USD.
func CalculateValueShared(estimatedValueUSD float32, timesUsed int32) float32 {
	return estimatedValueUSD * float32(timesUsed)
}

// CalculateDaysOpen calculates the number of days since a given timestamp.
// Returns the number of full days elapsed.
func CalculateDaysOpen(createdAtUnixSec int64) int32 {
	if createdAtUnixSec <= 0 {
		return 0
	}
	created := time.Unix(createdAtUnixSec, 0)
	duration := time.Since(created)
	return int32(duration.Hours() / 24)
}

// CountUniqueUsers counts unique user IDs from a slice.
// Returns the count of unique users.
func CountUniqueUsers(userIDs []string) int32 {
	uniqueMap := make(map[string]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id != "" {
			uniqueMap[id] = struct{}{}
		}
	}
	return int32(len(uniqueMap))
}

// CalculateValuePerHour calculates hourly value based on total value and duration in hours.
// Returns the value per hour in USD.
func CalculateValuePerHour(totalValueUSD float32, durationHours float64) float32 {
	if durationHours <= 0 {
		return 0
	}
	return totalValueUSD / float32(durationHours)
}

// CalculateTimeSavedHours estimates time saved by sharing/reusing instead of shopping for new items.
// Uses a standard estimate of 2 hours per transaction (time to research, shop, purchase).
// Returns the total time saved in hours.
func CalculateTimeSavedHours(transactionCount int32) int32 {
	const hoursPerTransaction = 2
	return transactionCount * hoursPerTransaction
}

// CalculateCO2SavedKg estimates CO2 emissions avoided by reusing items instead of manufacturing new.
// Uses category-based estimates for manufacturing + shipping emissions.
// Returns 0.0 if unable to estimate for the item category.
// Note: This is a simplified estimation. Future enhancement: use detailed category mappings.
func CalculateCO2SavedKg(estimatedValueUSD float32, timesUsed int32) float32 {
	// Simplified model: ~0.5kg CO2 per $100 of product value (manufacturing + shipping).
	// This is a rough average across consumer goods. More accurate estimates would require
	// detailed category-specific data (electronics vs textiles vs tools, etc.).
	if estimatedValueUSD <= 0 || timesUsed <= 0 {
		return 0.0
	}

	const co2PerDollar = 0.005 // kg CO2 per dollar (~0.5kg per $100)
	co2PerItem := estimatedValueUSD * co2PerDollar

	return co2PerItem * float32(timesUsed)
}
