package forms_test

import (
	"testing"

	"formlander/internal/forms"

	"github.com/stretchr/testify/assert"
)

func TestForm_IsOriginAllowed(t *testing.T) {
	// Note: IsOriginAllowed takes an already-extracted domain (e.g., "example.com"),
	// not a full URL. The HTTP layer is responsible for extracting the domain.
	tests := []struct {
		name           string
		allowedOrigins string
		domain         string
		expected       bool
	}{
		{
			name:           "empty origins rejects all",
			allowedOrigins: "",
			domain:         "example.com",
			expected:       false,
		},
		{
			name:           "whitespace-only origins rejects all",
			allowedOrigins: "   ",
			domain:         "example.com",
			expected:       false,
		},
		{
			name:           "wildcard allows all",
			allowedOrigins: "*",
			domain:         "anything.com",
			expected:       true,
		},
		{
			name:           "exact match",
			allowedOrigins: "example.com",
			domain:         "example.com",
			expected:       true,
		},
		{
			name:           "subdomain of allowed domain",
			allowedOrigins: "example.com",
			domain:         "sub.example.com",
			expected:       true,
		},
		{
			name:           "deep subdomain of allowed domain",
			allowedOrigins: "example.com",
			domain:         "deep.sub.example.com",
			expected:       true,
		},
		{
			name:           "wildcard in list allows all",
			allowedOrigins: "example.com, *",
			domain:         "anything.com",
			expected:       true,
		},
		{
			name:           "wildcard subdomain pattern",
			allowedOrigins: "*.example.com",
			domain:         "app.example.com",
			expected:       true,
		},
		{
			name:           "wildcard subdomain pattern allows base domain",
			allowedOrigins: "*.example.com",
			domain:         "example.com",
			expected:       true,
		},
		{
			name:           "multiple origins comma separated",
			allowedOrigins: "foo.com, bar.com, baz.com",
			domain:         "bar.com",
			expected:       true,
		},
		{
			name:           "no match in list",
			allowedOrigins: "foo.com, bar.com",
			domain:         "evil.com",
			expected:       false,
		},
		{
			name:           "partial match rejected (prefix)",
			allowedOrigins: "example.com",
			domain:         "notexample.com",
			expected:       false,
		},
		{
			name:           "suffix attack rejected",
			allowedOrigins: "example.com",
			domain:         "evilexample.com",
			expected:       false,
		},
		{
			name:           "empty domain with restrictions",
			allowedOrigins: "example.com",
			domain:         "",
			expected:       false,
		},
		{
			name:           "allowed origin as full URL normalizes",
			allowedOrigins: "https://example.com",
			domain:         "example.com",
			expected:       true,
		},
		{
			name:           "case insensitive matching",
			allowedOrigins: "Example.COM",
			domain:         "EXAMPLE.com",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := &forms.Form{
				AllowedOrigins: tt.allowedOrigins,
			}
			result := form.IsOriginAllowed(tt.domain)
			assert.Equal(t, tt.expected, result, "domain: %s, allowed: %s", tt.domain, tt.allowedOrigins)
		})
	}
}

func TestForm_ValidateRedirectURL(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		allowedOrigins string
		expectError    bool
		errorContains  string
	}{
		{
			name:           "empty URL is allowed",
			url:            "",
			allowedOrigins: "",
			expectError:    false,
		},
		{
			name:           "relative URL is allowed",
			url:            "/thank-you",
			allowedOrigins: "",
			expectError:    false,
		},
		{
			name:           "relative URL with query is allowed",
			url:            "/success?id=123",
			allowedOrigins: "",
			expectError:    false,
		},
		{
			name:           "absolute URL rejected when no origins configured",
			url:            "https://example.com/thanks",
			allowedOrigins: "",
			expectError:    true,
			errorContains:  "absolute redirects not allowed",
		},
		{
			name:           "absolute URL allowed when domain matches",
			url:            "https://example.com/thanks",
			allowedOrigins: "example.com",
			expectError:    false,
		},
		{
			name:           "absolute URL allowed for subdomain",
			url:            "https://sub.example.com/thanks",
			allowedOrigins: "example.com",
			expectError:    false,
		},
		{
			name:           "absolute URL rejected for non-matching domain",
			url:            "https://evil.com/thanks",
			allowedOrigins: "example.com",
			expectError:    true,
			errorContains:  "redirect URL not in allowed origins",
		},
		{
			name:           "wildcard pattern allows subdomain",
			url:            "https://app.example.com/thanks",
			allowedOrigins: "*.example.com",
			expectError:    false,
		},
		{
			name:           "wildcard pattern allows base domain",
			url:            "https://example.com/thanks",
			allowedOrigins: "*.example.com",
			expectError:    false,
		},
		{
			name:           "comma-separated origins work",
			url:            "https://other.com/thanks",
			allowedOrigins: "example.com, other.com",
			expectError:    false,
		},
		{
			name:           "URL with port matches domain",
			url:            "https://example.com:8080/thanks",
			allowedOrigins: "example.com",
			expectError:    false,
		},
		{
			name:           "invalid URL returns error",
			url:            "://invalid",
			allowedOrigins: "example.com",
			expectError:    true,
			errorContains:  "invalid redirect URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := &forms.Form{
				AllowedOrigins: tt.allowedOrigins,
			}
			err := form.ValidateRedirectURL(tt.url)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
