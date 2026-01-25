package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractCaptchaToken(t *testing.T) {
	t.Run("extracts cf-turnstile-response", func(t *testing.T) {
		payload := map[string]any{
			"name":                  "John",
			"cf-turnstile-response": "test-token-123",
		}

		token, ok := extractCaptchaToken(payload)

		assert.True(t, ok)
		assert.Equal(t, "test-token-123", token)
		assert.NotContains(t, payload, "cf-turnstile-response", "should remove token from payload")
	})

	t.Run("extracts cf_turnstile_response (underscore variant)", func(t *testing.T) {
		payload := map[string]any{
			"email":                  "test@example.com",
			"cf_turnstile_response": "underscore-token",
		}

		token, ok := extractCaptchaToken(payload)

		assert.True(t, ok)
		assert.Equal(t, "underscore-token", token)
		assert.NotContains(t, payload, "cf_turnstile_response")
	})

	t.Run("returns false when no token present", func(t *testing.T) {
		payload := map[string]any{
			"name":  "John",
			"email": "john@example.com",
		}

		token, ok := extractCaptchaToken(payload)

		assert.False(t, ok)
		assert.Empty(t, token)
	})

	t.Run("returns false for empty token", func(t *testing.T) {
		payload := map[string]any{
			"cf-turnstile-response": "",
		}

		token, ok := extractCaptchaToken(payload)

		assert.False(t, ok)
		assert.Empty(t, token)
	})

	t.Run("returns false for nil payload", func(t *testing.T) {
		token, ok := extractCaptchaToken(nil)

		assert.False(t, ok)
		assert.Empty(t, token)
	})

	t.Run("handles string array (form multipart)", func(t *testing.T) {
		payload := map[string]any{
			"cf-turnstile-response": []string{"array-token"},
		}

		token, ok := extractCaptchaToken(payload)

		assert.True(t, ok)
		assert.Equal(t, "array-token", token)
	})
}

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
