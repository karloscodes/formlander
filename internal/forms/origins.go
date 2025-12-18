package forms

import (
	"errors"
	"net/url"
	"strings"
)

// ErrOriginNotConfigured is returned when AllowedOrigins is empty
var ErrOriginNotConfigured = errors.New("allowed origins not configured")

// ErrOriginNotAllowed is returned when the origin doesn't match any allowed origin
var ErrOriginNotAllowed = errors.New("origin not allowed")

// ErrRedirectNotAllowed is returned when the redirect URL is not in allowed origins
var ErrRedirectNotAllowed = errors.New("redirect URL not in allowed origins")

// IsOriginAllowed checks if the given domain is allowed for this form.
// The domain parameter should be an already-extracted domain (e.g., "example.com").
// Returns true if:
// - AllowedOrigins is "*" (wildcard)
// - Domain matches an allowed domain exactly
// - Domain is a subdomain of an allowed domain
// - Domain matches a wildcard pattern (*.example.com)
//
// Returns false if AllowedOrigins is empty or domain doesn't match.
func (f *Form) IsOriginAllowed(domain string) bool {
	allowedOrigins := strings.TrimSpace(f.AllowedOrigins)

	// Reject if no origins configured
	if allowedOrigins == "" {
		return false
	}

	// Wildcard allows all origins
	if allowedOrigins == "*" {
		return true
	}

	// Empty domain with configured restrictions = reject
	if domain == "" {
		return false
	}

	// Normalize domain to lowercase
	domain = strings.ToLower(domain)

	// Check against allowed origins (comma-separated list)
	return matchesDomainList(domain, allowedOrigins)
}

// ValidateRedirectURL checks if the redirect URL is allowed for this form.
// - Empty URL is always allowed (no redirect)
// - Relative URLs are always allowed
// - Absolute URLs must match AllowedOrigins
func (f *Form) ValidateRedirectURL(redirectURL string) error {
	if redirectURL == "" {
		return nil // No redirect is fine
	}

	parsed, err := url.Parse(redirectURL)
	if err != nil {
		return errors.New("invalid redirect URL")
	}

	// Allow relative URLs (no host)
	if parsed.Host == "" {
		return nil
	}

	// Check against allowed origins for this form
	if strings.TrimSpace(f.AllowedOrigins) == "" {
		return errors.New("absolute redirects not allowed without configured origins")
	}

	// Extract domain from redirect URL (without port)
	redirectDomain := strings.ToLower(parsed.Host)
	if idx := strings.LastIndex(redirectDomain, ":"); idx >= 0 {
		redirectDomain = redirectDomain[:idx]
	}

	if matchesDomainList(redirectDomain, f.AllowedOrigins) {
		return nil
	}

	return ErrRedirectNotAllowed
}

// matchesDomainList checks if a domain matches any entry in a comma-separated list.
// Entries in the list can be plain domains or full URLs - they will be normalized.
func matchesDomainList(domain, allowedList string) bool {
	for _, allowed := range strings.Split(allowedList, ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}

		// Wildcard allows all
		if allowed == "*" {
			return true
		}

		// Support wildcard patterns like *.example.com
		if strings.HasPrefix(allowed, "*.") {
			baseDomain := strings.ToLower(strings.TrimPrefix(allowed, "*."))
			if domain == baseDomain || strings.HasSuffix(domain, "."+baseDomain) {
				return true
			}
			continue
		}

		// Normalize allowed origin to plain domain
		allowedDomain := normalizeToDomain(allowed)

		// Exact match or subdomain match
		if domain == allowedDomain || strings.HasSuffix(domain, "."+allowedDomain) {
			return true
		}
	}

	return false
}

// normalizeToDomain extracts just the domain from an allowed origin entry.
// Handles plain domains (example.com) and full URLs (https://example.com/path).
func normalizeToDomain(s string) string {
	// Remove protocol
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")

	// Remove path, query and fragment
	if idx := strings.IndexAny(s, "/?#"); idx >= 0 {
		s = s[:idx]
	}

	// Remove port
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		s = s[:idx]
	}

	return strings.ToLower(s)
}
