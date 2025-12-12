package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// SecFetchSiteConfig configures the Sec-Fetch-Site middleware.
type SecFetchSiteConfig struct {
	// AllowedValues specifies which Sec-Fetch-Site values are permitted.
	// Default: ["same-origin", "none"] (same-origin requests and direct navigation)
	AllowedValues []string

	// Methods specifies which HTTP methods require validation.
	// Default: ["POST", "PUT", "DELETE", "PATCH"]
	Methods []string

	// Next defines a function to skip this middleware when returning true.
	Next func(c *fiber.Ctx) bool
}

// DefaultSecFetchSiteConfig returns the default configuration.
func DefaultSecFetchSiteConfig() SecFetchSiteConfig {
	return SecFetchSiteConfig{
		AllowedValues: []string{"same-origin", "none"},
		Methods:       []string{"POST", "PUT", "DELETE", "PATCH"},
	}
}

// SecFetchSiteMiddleware validates the Sec-Fetch-Site header to prevent CSRF attacks.
// Modern browsers automatically set this header, and it cannot be spoofed by JavaScript.
//
// Sec-Fetch-Site values:
//   - "same-origin": Request from the same origin (scheme + host + port)
//   - "same-site": Request from the same site (different subdomain allowed)
//   - "cross-site": Request from a different site
//   - "none": Direct navigation (user typed URL, bookmark, etc.)
//
// By default, this middleware allows "same-origin" and "none" for state-changing methods.
func SecFetchSiteMiddleware(config ...SecFetchSiteConfig) fiber.Handler {
	cfg := DefaultSecFetchSiteConfig()
	if len(config) > 0 {
		cfg = config[0]
		if cfg.AllowedValues == nil {
			cfg.AllowedValues = DefaultSecFetchSiteConfig().AllowedValues
		}
		if cfg.Methods == nil {
			cfg.Methods = DefaultSecFetchSiteConfig().Methods
		}
	}

	methodSet := make(map[string]bool, len(cfg.Methods))
	for _, m := range cfg.Methods {
		methodSet[m] = true
	}

	allowedSet := make(map[string]bool, len(cfg.AllowedValues))
	for _, v := range cfg.AllowedValues {
		allowedSet[v] = true
	}

	return func(c *fiber.Ctx) error {
		if cfg.Next != nil && cfg.Next(c) {
			return c.Next()
		}

		// Only validate configured methods
		if !methodSet[c.Method()] {
			return c.Next()
		}

		secFetchSite := c.Get("Sec-Fetch-Site")

		// If header is missing, the browser doesn't support it (older browsers).
		// We allow these requests to maintain compatibility, but log for monitoring.
		// In strict mode, you could reject these requests.
		if secFetchSite == "" {
			return c.Next()
		}

		if !allowedSet[secFetchSite] {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":   "forbidden",
				"message": "cross-site request blocked",
			})
		}

		return c.Next()
	}
}
