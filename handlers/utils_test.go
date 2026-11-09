package handlers

import (
	"testing"
	"time"
)

func TestFormatDate(t *testing.T) {
	tests := map[string]string{
		"2025-01-15": "01/15/25",
		"1999-12-31": "12/31/99",
		"1/15/2025":  "1/15/2025", // unparseable input is returned unchanged
		"":           "",
		"garbage":    "garbage",
	}
	for in, want := range tests {
		if got := FormatDate(in); got != want {
			t.Errorf("FormatDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConvertFilterDate(t *testing.T) {
	tests := map[string]string{
		"2025-01-05": "1/5/2025",
		"2025-12-25": "12/25/2025",
		"":           "",
		"01/05/2025": "01/05/2025",
	}
	for in, want := range tests {
		if got := ConvertFilterDate(in); got != want {
			t.Errorf("ConvertFilterDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDateToTime(t *testing.T) {
	want := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
	for _, in := range []string{"1/5/2025", "01/05/2025", "2025-01-05", "01/05/25", "1/5/25"} {
		got, err := ParseDateToTime(in)
		if err != nil {
			t.Errorf("ParseDateToTime(%q) error: %v", in, err)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("ParseDateToTime(%q) = %v, want %v", in, got, want)
		}
	}

	for _, in := range []string{"", "yesterday", "2025/01/05", "13/45/2025", "2025-1-5"} {
		if _, err := ParseDateToTime(in); err == nil {
			t.Errorf("ParseDateToTime(%q) should fail", in)
		}
	}
}

func TestIsDateInRange(t *testing.T) {
	tests := []struct {
		name           string
		date, from, to string
		want           bool
	}{
		{"no bounds", "2025-01-15", "", "", true},
		{"inside both bounds", "2025-01-15", "2025-01-01", "2025-01-31", true},
		{"on from bound", "2025-01-01", "2025-01-01", "2025-01-31", true},
		{"on to bound", "2025-01-31", "2025-01-01", "2025-01-31", true},
		{"before from", "2024-12-31", "2025-01-01", "", false},
		{"after to", "2025-02-01", "", "2025-01-31", false},
		{"mixed formats", "1/15/2025", "2025-01-01", "01/31/25", true},
		{"mixed formats out of range", "2/15/2025", "2025-01-01", "01/31/25", false},
		{"unparseable date is included", "not-a-date", "2025-01-01", "2025-01-31", true},
		{"unparseable bound is ignored", "2025-01-15", "bad", "also-bad", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDateInRange(tt.date, tt.from, tt.to); got != tt.want {
				t.Errorf("IsDateInRange(%q, %q, %q) = %v, want %v", tt.date, tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestNormalizeDateToMMDDYY(t *testing.T) {
	tests := map[string]string{
		"1/5/2025":   "01/05/25",
		"2025-01-05": "01/05/25",
		"01/05/25":   "01/05/25",
		"":           "",
		"whenever":   "whenever",
	}
	for in, want := range tests {
		if got := NormalizeDateToMMDDYY(in); got != want {
			t.Errorf("NormalizeDateToMMDDYY(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelected(t *testing.T) {
	if selected("call", "call") != "selected" {
		t.Error("matching values should be selected")
	}
	if selected("call", "put") != "" {
		t.Error("different values should not be selected")
	}
}
