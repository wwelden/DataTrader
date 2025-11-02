package utils

import (
	"fmt"
	"os"
	"strings"
)

func ReadFile(path string) string {
	inputFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(inputFile)
}

func Parse(input string) [][]string {
	if input == "" {
		return [][]string{}
	}

	result := [][]string{}
	var currentRow []string
	var currentField strings.Builder
	inQuotes := false
	fieldWasQuoted := false

	for i := 0; i < len(input); i++ {
		char := input[i]

		switch char {
		case '"':
			// Check for escaped quote (two consecutive quotes)
			if inQuotes && i+1 < len(input) && input[i+1] == '"' {
				currentField.WriteByte('"')
				i++ // Skip the next quote
			} else {
				// Toggle quote state
				inQuotes = !inQuotes
				if inQuotes {
					fieldWasQuoted = true
				}
			}

		case ',':
			if inQuotes {
				// Comma is part of the field value
				currentField.WriteByte(char)
			} else {
				// Comma is a field separator
				fieldValue := currentField.String()
				if !fieldWasQuoted {
					fieldValue = strings.TrimSpace(fieldValue)
				}
				currentRow = append(currentRow, fieldValue)
				currentField.Reset()
				fieldWasQuoted = false
			}

		case '\n':
			if inQuotes {
				// Newline is part of the field value
				currentField.WriteByte(char)
			} else {
				// Newline marks end of row
				// Add the last field of the row
				fieldValue := currentField.String()
				if !fieldWasQuoted {
					fieldValue = strings.TrimSpace(fieldValue)
				}
				currentRow = append(currentRow, fieldValue)
				currentField.Reset()
				fieldWasQuoted = false

				// Only add non-empty rows
				if len(currentRow) > 0 && !(len(currentRow) == 1 && currentRow[0] == "") {
					result = append(result, currentRow)
				}
				currentRow = []string{}
			}

		case '\r':
			// Handle Windows-style line endings (\r\n)
			if !inQuotes && i+1 < len(input) && input[i+1] == '\n' {
				// Skip \r, let \n handle the row ending
				continue
			} else if inQuotes {
				currentField.WriteByte(char)
			} else {
				// Treat standalone \r as newline
				fieldValue := currentField.String()
				if !fieldWasQuoted {
					fieldValue = strings.TrimSpace(fieldValue)
				}
				currentRow = append(currentRow, fieldValue)
				currentField.Reset()
				fieldWasQuoted = false

				if len(currentRow) > 0 && !(len(currentRow) == 1 && currentRow[0] == "") {
					result = append(result, currentRow)
				}
				currentRow = []string{}
			}

		default:
			currentField.WriteByte(char)
		}
	}

	// Handle last field and row if input doesn't end with newline
	if currentField.Len() > 0 || len(currentRow) > 0 {
		fieldValue := currentField.String()
		if !fieldWasQuoted {
			fieldValue = strings.TrimSpace(fieldValue)
		}
		currentRow = append(currentRow, fieldValue)
		if len(currentRow) > 0 && !(len(currentRow) == 1 && currentRow[0] == "") {
			result = append(result, currentRow)
		}
	}

	return result
}

func parseStrikePrice(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")

	// Try to parse as float
	if value, err := parseFloat64(s); err == nil {
		return value, nil
	} else {
		return 0, err
	}
}

// parseFloat64 is a simple float parser
func parseFloat64(s string) (float64, error) {
	s = strings.TrimSpace(s)
	var result float64
	_, err := fmt.Sscanf(s, "%f", &result)
	return result, err
}
