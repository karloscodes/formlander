package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"formlander/internal/config"
)

const SessionCookieName = "formlander_session"

// SessionData stores session information
type SessionData struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

var sessionSecret []byte
var sessionTTL time.Duration
var isProduction bool

// Initialize configures the session manager.
func Initialize(cfg *config.Config) {
	sessionSecret = []byte(cfg.SessionSecret)
	sessionTTL = time.Duration(cfg.SessionTimeoutSeconds) * time.Second
	isProduction = cfg.IsProduction()
}

// GeneratePasswordHash creates a bcrypt hash of the password
func GeneratePasswordHash(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// VerifyPassword checks if a password matches its hash
func VerifyPassword(hashedPassword string, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// SetAuthCookie sets a signed session cookie for a user
func SetAuthCookie(c *fiber.Ctx, userID uint) error {
	sessionData := SessionData{
		UserID:    strconv.FormatUint(uint64(userID), 10),
		ExpiresAt: time.Now().Add(sessionTTL),
	}

	jsonData, err := json.Marshal(sessionData)
	if err != nil {
		return err
	}

	token, err := sign(jsonData)
	if err != nil {
		return err
	}

	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		Expires:  sessionData.ExpiresAt,
		Secure:   isProduction,
		HTTPOnly: true,
		SameSite: "Lax",
	})

	zap.L().Debug("Setting auth session",
		zap.Uint("userID", userID),
		zap.Time("expiresAt", sessionData.ExpiresAt))
	return nil
}

// ClearAuthCookie removes the authentication cookie
func ClearAuthCookie(c *fiber.Ctx) {
	c.ClearCookie(SessionCookieName)
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Now().Add(-24 * time.Hour),
		Secure:   isProduction,
		HTTPOnly: true,
		SameSite: "Lax",
	})
	zap.L().Debug("Cleared auth session")
}

// IsAuthenticated checks if the user is authenticated
func IsAuthenticated(c *fiber.Ctx) bool {
	token := c.Cookies(SessionCookieName)
	if token == "" {
		zap.L().Debug("No session cookie found")
		return false
	}

	sessionData, err := verify(token)
	if err != nil {
		zap.L().Debug("Failed to verify session", zap.Error(err))
		return false
	}

	if time.Now().After(sessionData.ExpiresAt) {
		zap.L().Debug("Session expired",
			zap.Time("expiresAt", sessionData.ExpiresAt),
			zap.Time("currentTime", time.Now()))
		return false
	}

	if _, err := strconv.ParseUint(sessionData.UserID, 10, 64); err != nil {
		zap.L().Debug("User ID in session is not valid",
			zap.String("userID", sessionData.UserID),
			zap.Error(err))
		return false
	}

	zap.L().Debug("Session validated",
		zap.String("userID", sessionData.UserID),
		zap.Time("expiresAt", sessionData.ExpiresAt))
	return true
}

// GetUserID retrieves the user ID from the session cookie
func GetUserID(c *fiber.Ctx) (uint, bool) {
	token := c.Cookies(SessionCookieName)
	if token == "" {
		return 0, false
	}

	sessionData, err := verify(token)
	if err != nil {
		zap.L().Debug("Failed to verify session", zap.Error(err))
		return 0, false
	}

	if time.Now().After(sessionData.ExpiresAt) {
		return 0, false
	}

	userID, err := strconv.ParseUint(sessionData.UserID, 10, 32)
	if err != nil {
		return 0, false
	}

	return uint(userID), true
}

// Middleware ensures authenticated admin access.
func Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !IsAuthenticated(c) {
			// For HTMX requests respond with 401 to trigger login.
			if c.Get("HX-Request") == "true" {
				return c.Status(fiber.StatusUnauthorized).SendString("authentication required")
			}
			return c.Redirect("/admin/login")
		}
		return c.Next()
	}
}

func sign(payload []byte) (string, error) {
	sig := computeHMAC(payload, sessionSecret)
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	sigEnc := base64.RawURLEncoding.EncodeToString(sig)
	return payloadEnc + "." + sigEnc, nil
}

func verify(token string) (*SessionData, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid session token")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid session payload")
	}

	expectedSig := computeHMAC(payload, sessionSecret)
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid session signature")
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("session signature mismatch")
	}

	var sessionData SessionData
	if err := json.Unmarshal(payload, &sessionData); err != nil {
		return nil, errors.New("invalid session data")
	}

	return &sessionData, nil
}

func computeHMAC(payload, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return mac.Sum(nil)
}
