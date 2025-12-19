package auth

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"
	"golang.org/x/crypto/bcrypt"

	"formlander/internal/config"
)

// SessionCookieName is the name of the session cookie.
const SessionCookieName = "formlander_session"

var sessionManager *cartridge.SessionManager

// Initialize configures the session manager.
func Initialize(cfg *config.Config) {
	sessionManager = cartridge.NewSessionManager(cartridge.SessionConfig{
		CookieName: SessionCookieName,
		Secret:     cfg.SessionSecret,
		TTL:        time.Duration(cfg.SessionTimeout) * time.Second,
		Secure:     cfg.IsProduction(),
		LoginPath:  "/admin/login",
	})
}

// GeneratePasswordHash creates a bcrypt hash of the password.
func GeneratePasswordHash(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// VerifyPassword checks if a password matches its hash.
func VerifyPassword(hashedPassword string, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// SetAuthCookie sets a signed session cookie for a user.
func SetAuthCookie(c *fiber.Ctx, userID uint) error {
	return sessionManager.SetSession(c, userID)
}

// ClearAuthCookie removes the authentication cookie.
func ClearAuthCookie(c *fiber.Ctx) {
	sessionManager.ClearSession(c)
}

// IsAuthenticated checks if the user is authenticated.
func IsAuthenticated(c *fiber.Ctx) bool {
	return sessionManager.IsAuthenticated(c)
}

// GetUserID retrieves the user ID from the session cookie.
func GetUserID(c *fiber.Ctx) (uint, bool) {
	return sessionManager.GetUserID(c)
}

// Middleware ensures authenticated admin access.
func Middleware() fiber.Handler {
	return sessionManager.Middleware()
}
