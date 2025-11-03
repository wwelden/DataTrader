package handlers

import (
	"errors"
	"time"
)

// FormatDate converts a date string from YYYY-MM-DD format to MM/DD/YY format
func FormatDate(dateStr string) string {
	// Try to parse the date in YYYY-MM-DD format
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		// If parsing fails, return the original string
		return dateStr
	}

	// Format as MM/DD/YY
	return t.Format("01/02/06")
}

// ConvertFilterDate converts a date from YYYY-MM-DD format (HTML input) to M/D/YYYY format (database)
func ConvertFilterDate(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}

	// Format as M/D/YYYY (no leading zeros)
	return t.Format("1/2/2006")
}

// ParseDateToTime parses various date formats and returns a time.Time
func ParseDateToTime(dateStr string) (time.Time, error) {
	// Try M/D/YYYY format
	if t, err := time.Parse("1/2/2006", dateStr); err == nil {
		return t, nil
	}
	// Try MM/DD/YYYY format
	if t, err := time.Parse("01/02/2006", dateStr); err == nil {
		return t, nil
	}
	// Try YYYY-MM-DD format
	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		return t, nil
	}
	// Try MM/DD/YY format
	if t, err := time.Parse("01/02/06", dateStr); err == nil {
		return t, nil
	}
	// Try M/D/YY format
	if t, err := time.Parse("1/2/06", dateStr); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("unable to parse date")
}

// IsDateInRange checks if a date string is within the given range
func IsDateInRange(dateStr, fromStr, toStr string) bool {
	date, err := ParseDateToTime(dateStr)
	if err != nil {
		return true // If we can't parse, include it
	}

	if fromStr != "" {
		fromDate, err := ParseDateToTime(fromStr)
		if err == nil && date.Before(fromDate) {
			return false
		}
	}

	if toStr != "" {
		toDate, err := ParseDateToTime(toStr)
		if err == nil && date.After(toDate) {
			return false
		}
	}

	return true
}

// NormalizeDateToMMDDYY converts any date format to MM/DD/YY format for storage
func NormalizeDateToMMDDYY(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	t, err := ParseDateToTime(dateStr)
	if err != nil {
		return dateStr // Return original if we can't parse
	}

	// Format as MM/DD/YY
	return t.Format("01/02/06")
}
