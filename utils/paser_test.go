package utils

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  [][]string
	}{
		{"empty input", "", [][]string{}},
		{"single field", "a", [][]string{{"a"}}},
		{"single row", "a,b,c", [][]string{{"a", "b", "c"}}},
		{"trailing newline", "a,b\n", [][]string{{"a", "b"}}},
		{"multiple rows LF", "a,b\nc,d", [][]string{{"a", "b"}, {"c", "d"}}},
		{"multiple rows CRLF", "a,b\r\nc,d\r\n", [][]string{{"a", "b"}, {"c", "d"}}},
		{"bare CR row separator", "a,b\rc,d", [][]string{{"a", "b"}, {"c", "d"}}},
		{"blank lines skipped", "a\n\n\nb\n", [][]string{{"a"}, {"b"}}},
		{"blank CRLF lines skipped", "a\r\n\r\nb", [][]string{{"a"}, {"b"}}},
		{"empty fields kept", "a,,c", [][]string{{"a", "", "c"}}},
		{"trailing comma yields empty field", "a,b,", [][]string{{"a", "b", ""}}},
		{"leading comma yields empty field", ",a", [][]string{{"", "a"}}},
		{"unquoted fields are trimmed", "  a , b  ,c ", [][]string{{"a", "b", "c"}}},
		{"quoted field keeps spaces", `"  a  ",b`, [][]string{{"  a  ", "b"}}},
		{"quoted comma", `"a,b",c`, [][]string{{"a,b", "c"}}},
		{"quoted newline", "\"line1\nline2\",x", [][]string{{"line1\nline2", "x"}}},
		{"quoted CRLF", "\"a\r\nb\",x", [][]string{{"a\r\nb", "x"}}},
		{"escaped quote", `"say ""hi""",x`, [][]string{{`say "hi"`, "x"}}},
		{"empty quoted field", `"",x`, [][]string{{"", "x"}}},
		{"quoted last field without newline", `a,"b"`, [][]string{{"a", "b"}}},
		{"money with comma", `"$1,234.56",x`, [][]string{{"$1,234.56", "x"}}},
		{"row of only whitespace is dropped", "a\n   \nb", [][]string{{"a"}, {"b"}}},
		{"unicode preserved", "café,日本", [][]string{{"café", "日本"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q)\n got  %q\n want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRaggedRows(t *testing.T) {
	got := Parse("a,b,c\nd\ne,f")
	want := [][]string{{"a", "b", "c"}, {"d"}, {"e", "f"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.csv")
	if err := os.WriteFile(path, []byte("a,b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ReadFile(path); got != "a,b\n" {
		t.Errorf("ReadFile = %q, want %q", got, "a,b\n")
	}
}

func TestReadFilePanicsOnMissingFile(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected ReadFile to panic for a missing file")
		}
	}()
	ReadFile(filepath.Join(t.TempDir(), "missing.csv"))
}

func TestParseStrikePrice(t *testing.T) {
	tests := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"150", 150, false},
		{"$150", 150, false},
		{" $1,250.50 ", 1250.5, false},
		{"0.5", 0.5, false},
		{"", 0, true},
		{"abc", 0, true},
		{"$", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseStrikePrice(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseStrikePrice(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseStrikePrice(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseFloat64(t *testing.T) {
	tests := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"1.5", 1.5, false},
		{"  -2 ", -2, false},
		{"1e3", 1000, false},
		{"", 0, true},
		{"x1", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseFloat64(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseFloat64(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseFloat64(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
