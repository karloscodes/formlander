package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "https URL",
			input:    "https://example.com",
			expected: "example.com",
		},
		{
			name:     "http URL",
			input:    "http://example.com",
			expected: "example.com",
		},
		{
			name:     "URL with path",
			input:    "https://example.com/path/to/page",
			expected: "example.com",
		},
		{
			name:     "URL with query",
			input:    "https://example.com?foo=bar",
			expected: "example.com",
		},
		{
			name:     "URL with port",
			input:    "https://example.com:8080",
			expected: "example.com",
		},
		{
			name:     "URL with port and path",
			input:    "https://example.com:8080/path",
			expected: "example.com",
		},
		{
			name:     "subdomain",
			input:    "https://sub.example.com",
			expected: "sub.example.com",
		},
		{
			name:     "mixed case normalized to lowercase",
			input:    "https://EXAMPLE.COM",
			expected: "example.com",
		},
		{
			name:     "URL with fragment",
			input:    "https://example.com#section",
			expected: "example.com",
		},
		{
			name:     "bare domain",
			input:    "example.com",
			expected: "example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDomain(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
