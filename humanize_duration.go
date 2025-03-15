package main

import (
	"fmt"
)

func HumanizeDuration(seconds int) string {
	// Define time constants
	const (
		secondsPerMinute = 60
		secondsPerHour   = 3600
		secondsPerDay    = 86400
		secondsPerMonth  = 2629800  // Approximate: 30.44 days per month
		secondsPerYear   = 31557600 // Approximate: 365.25 days per year
	)

	// Calculate years
	years := seconds / secondsPerYear
	seconds %= secondsPerYear

	// Calculate months
	months := seconds / secondsPerMonth
	seconds %= secondsPerMonth

	// Calculate days
	days := seconds / secondsPerDay
	seconds %= secondsPerDay

	// Calculate hours
	hours := seconds / secondsPerHour
	seconds %= secondsPerHour

	// Calculate minutes
	minutes := seconds / secondsPerMinute
	seconds %= secondsPerMinute

	// Build the human-readable string
	result := ""
	if years > 0 {
		result += fmt.Sprintf("%dy ", years)
	}
	if months > 0 {
		result += fmt.Sprintf("%dmo ", months)
	}
	if days > 0 {
		result += fmt.Sprintf("%dd ", days)
	}
	if hours > 0 {
		result += fmt.Sprintf("%dh ", hours)
	}
	if minutes > 0 {
		result += fmt.Sprintf("%dm ", minutes)
	}

	// Remove trailing space if any
	if len(result) > 0 {
		result = result[:len(result)-1]
	} else {
		result = "0s"
	}

	return result
}
