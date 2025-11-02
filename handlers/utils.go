package handlers

import (
	"time"
)

// FormatDate converts a date string from YYYY-MM-DD format to MM-DD-YY format
func FormatDate(dateStr string) string {
	// Try to parse the date in YYYY-MM-DD format
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		// If parsing fails, return the original string
		return dateStr
	}

	// Format as MM-DD-YY
	return t.Format("01-02-06")
}
