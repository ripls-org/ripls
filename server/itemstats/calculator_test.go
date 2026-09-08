package itemstats

import (
	"testing"
	"time"
)

func TestCalculateValueShared(t *testing.T) {
	tests := []struct {
		name              string
		estimatedValueUSD float32
		timesUsed         int32
		expectedValueUSD  float32
	}{
		{
			name:              "zero usage",
			estimatedValueUSD: 100.0,
			timesUsed:         0,
			expectedValueUSD:  0,
		},
		{
			name:              "single use",
			estimatedValueUSD: 149.0,
			timesUsed:         1,
			expectedValueUSD:  149.0,
		},
		{
			name:              "multiple uses",
			estimatedValueUSD: 149.0,
			timesUsed:         7,
			expectedValueUSD:  1043.0,
		},
		{
			name:              "zero value",
			estimatedValueUSD: 0,
			timesUsed:         5,
			expectedValueUSD:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateValueShared(tt.estimatedValueUSD, tt.timesUsed)
			if result != tt.expectedValueUSD {
				t.Errorf("CalculateValueShared(%f, %d) = %f; want %f",
					tt.estimatedValueUSD, tt.timesUsed, result, tt.expectedValueUSD)
			}
		})
	}
}

func TestCalculateDaysOpen(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name             string
		createdAtUnixSec int64
		expectedDays     int32
	}{
		{
			name:             "zero timestamp",
			createdAtUnixSec: 0,
			expectedDays:     0,
		},
		{
			name:             "negative timestamp",
			createdAtUnixSec: -1,
			expectedDays:     0,
		},
		{
			name:             "one day ago",
			createdAtUnixSec: now.Add(-24 * time.Hour).Unix(),
			expectedDays:     1,
		},
		{
			name:             "one week ago",
			createdAtUnixSec: now.Add(-7 * 24 * time.Hour).Unix(),
			expectedDays:     7,
		},
		{
			name:             "just created",
			createdAtUnixSec: now.Unix(),
			expectedDays:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateDaysOpen(tt.createdAtUnixSec)
			if result != tt.expectedDays {
				t.Errorf("CalculateDaysOpen(%d) = %d; want %d",
					tt.createdAtUnixSec, result, tt.expectedDays)
			}
		})
	}
}

func TestCountUniqueUsers(t *testing.T) {
	tests := []struct {
		name          string
		userIDs       []string
		expectedCount int32
	}{
		{
			name:          "empty list",
			userIDs:       []string{},
			expectedCount: 0,
		},
		{
			name:          "single user",
			userIDs:       []string{"user1"},
			expectedCount: 1,
		},
		{
			name:          "multiple unique users",
			userIDs:       []string{"user1", "user2", "user3"},
			expectedCount: 3,
		},
		{
			name:          "duplicate users",
			userIDs:       []string{"user1", "user2", "user1", "user3", "user2"},
			expectedCount: 3,
		},
		{
			name:          "empty string IDs",
			userIDs:       []string{"user1", "", "user2", ""},
			expectedCount: 2,
		},
		{
			name:          "all empty strings",
			userIDs:       []string{"", "", ""},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountUniqueUsers(tt.userIDs)
			if result != tt.expectedCount {
				t.Errorf("CountUniqueUsers(%v) = %d; want %d",
					tt.userIDs, result, tt.expectedCount)
			}
		})
	}
}

func TestCalculateValuePerHour(t *testing.T) {
	tests := []struct {
		name                 string
		totalValueUSD        float32
		durationHours        float64
		expectedValuePerHour float32
	}{
		{
			name:                 "zero duration",
			totalValueUSD:        100.0,
			durationHours:        0,
			expectedValuePerHour: 0,
		},
		{
			name:                 "negative duration",
			totalValueUSD:        100.0,
			durationHours:        -1,
			expectedValuePerHour: 0,
		},
		{
			name:                 "one hour",
			totalValueUSD:        50.0,
			durationHours:        1,
			expectedValuePerHour: 50.0,
		},
		{
			name:                 "multiple hours",
			totalValueUSD:        150.0,
			durationHours:        3,
			expectedValuePerHour: 50.0,
		},
		{
			name:                 "fractional hours",
			totalValueUSD:        100.0,
			durationHours:        2.5,
			expectedValuePerHour: 40.0,
		},
		{
			name:                 "zero value",
			totalValueUSD:        0,
			durationHours:        5,
			expectedValuePerHour: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateValuePerHour(tt.totalValueUSD, tt.durationHours)
			if result != tt.expectedValuePerHour {
				t.Errorf("CalculateValuePerHour(%f, %f) = %f; want %f",
					tt.totalValueUSD, tt.durationHours, result, tt.expectedValuePerHour)
			}
		})
	}
}
