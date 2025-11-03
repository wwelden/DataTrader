package handlers

import (
	"errors"
	"time"
)

func FormatDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}

	return t.Format("01/02/06")
}

func ConvertFilterDate(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}

	return t.Format("1/2/2006")
}

func ParseDateToTime(dateStr string) (time.Time, error) {
	if t, err := time.Parse("1/2/2006", dateStr); err == nil {
		return t, nil
	}
	if t, err := time.Parse("01/02/2006", dateStr); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		return t, nil
	}
	if t, err := time.Parse("01/02/06", dateStr); err == nil {
		return t, nil
	}
	if t, err := time.Parse("1/2/06", dateStr); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("unable to parse date")
}

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

func NormalizeDateToMMDDYY(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	t, err := ParseDateToTime(dateStr)
	if err != nil {
		return dateStr // Return original if we can't parse
	}

	return t.Format("01/02/06")
}
