package genai

import "testing"

func TestNormalizeReading(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Perfect formatting with question mark",
			input:    "03956.49?",
			expected: "03956.49?",
		},
		{
			name:     "Perfect formatting fully resolved",
			input:    "03956.490",
			expected: "03956.490",
		},
		{
			name:     "Misplaced decimal point (6 integers, 1 decimal)",
			input:    "039564.9",
			expected: "03956.49?",
		},
		{
			name:     "Misplaced decimal point and multiple question marks",
			input:    "0395649.???",
			expected: "03956.49?",
		},
		{
			name:     "No decimal point at all",
			input:    "0395649?",
			expected: "03956.49?",
		},
		{
			name:     "Missing the final digit entirely",
			input:    "03956.49",
			expected: "03956.49?",
		},
		{
			name:     "Whitespace and newlines",
			input:    "  03956.49?  \n",
			expected: "03956.49?",
		},
		{
			name:     "Short string (should be left as-is)",
			input:    "3956.49",
			expected: "3956.49",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := NormalizeReading(tt.input)
			if actual != tt.expected {
				t.Errorf("NormalizeReading(%q) = %q; expected %q", tt.input, actual, tt.expected)
			}
		})
	}
}
