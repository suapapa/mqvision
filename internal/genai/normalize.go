package genai

import "strings"

// NormalizeReading cleans and formats an analog meter reading string into the "NNNNN.NNN" format.
// It extracts all numeric digits and '?' characters, then reconstructs the string
// with exactly 5 integer digits and 3 decimal digits. If the input contains only 7
// digits/characters, it appends a '?' as the 8th digit.
func NormalizeReading(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	var sb strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '?' {
			sb.WriteRune(r)
		}
	}
	digits := sb.String()

	// If we have at least 8 digits/?, reconstruct as NNNNN.NNN
	if len(digits) >= 8 {
		return digits[:5] + "." + digits[5:8]
	}

	// If we have exactly 7 digits/?, append '?' to make it 8, and format
	if len(digits) == 7 {
		return digits[:5] + "." + digits[5:] + "?"
	}

	// For any other length, return original string to avoid over-guessing
	return s
}
