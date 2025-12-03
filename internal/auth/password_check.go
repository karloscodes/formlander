package auth

import (
	"github.com/gofiber/fiber/v2"
)

// RequirePasswordChanged is a middleware that ensures the user has changed their password.
// It redirects to the password change page if this is their first login (LastLoginAt was nil before login).
// Note: This middleware is currently not used, as the password change redirect happens at login time.
func RequirePasswordChanged() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// This middleware is kept for compatibility but the actual redirect happens
		// in AdminLoginSubmit when user.LastLoginAt is nil (first login)
		return c.Next()
	}
}
