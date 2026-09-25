package accounts

import (
	"crypto/rand"
	"errors"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"log/slog"

	"formlander/internal/pkg/dbtxn"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrPasswordMismatch   = errors.New("current password is incorrect")
	ErrMissingFields      = errors.New("required fields are missing")
	ErrDuplicateEmail     = errors.New("email is already in use")
	ErrInvalidEmail       = errors.New("email is not valid")
)

// DefaultAdminEmail is the login of the admin created on first boot.
// DefaultAdminPassword is used only in the test environment. Every other
// environment gets a random first password, because a known password lets
// the first visitor of a new instance take it over.
const (
	DefaultAdminEmail    = "admin@formlander.local"
	DefaultAdminPassword = "formlander"
)

// InitialPasswordFile holds the random first password in the data directory,
// so the operator can read it on the server. It is removed when the admin
// sets a new password.
const InitialPasswordFile = "initial-admin-password"

// GenerateInitialPassword returns a random first password for the admin.
func GenerateInitialPassword() string {
	return rand.Text()
}

// WriteInitialPassword stores the first password where only the server
// operator can read it.
func WriteInitialPassword(dataDir, password string) error {
	return os.WriteFile(filepath.Join(dataDir, InitialPasswordFile), []byte(password+"\n"), 0o600)
}

// HasInitialPassword reports whether the first password is still unchanged.
func HasInitialPassword(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, InitialPasswordFile))
	return err == nil
}

// RemoveInitialPassword deletes the first password once it no longer works.
func RemoveInitialPassword(dataDir string) error {
	err := os.Remove(filepath.Join(dataDir, InitialPasswordFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// timingEqualizerHash is a valid bcrypt digest compared against when the
// supplied email doesn't exist, so Authenticate takes constant time regardless
// of whether the account exists (email-enumeration defense). It is the digest
// of a random string; no real password matches it.
const timingEqualizerHash = "$2a$10$Q1pg.L2uyfJ2QportzoH9.UPdkdy2skSFqtGaRfOXpO0SBGCQ1qIW"

// IsDefaultAdminActive reports whether the default admin still has the old
// public default password. Boot replaces that password on existing installs.
func IsDefaultAdminActive(db *gorm.DB) bool {
	user, err := FindByEmail(db, DefaultAdminEmail)
	if err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(DefaultAdminPassword)) == nil
}

// User represents the single admin user for the MVP.
type User struct {
	ID           uint       `gorm:"primaryKey"`
	Email        string     `gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string     `gorm:"size:255;not null"`
	LastLoginAt  *time.Time `gorm:"index"` // nil = first login required, force password change
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Settings stores global application configuration as key-value pairs.
type Settings struct {
	ID        uint      `gorm:"primaryKey"`
	Key       string    `gorm:"uniqueIndex;not null"`
	Value     string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;autoCreateTime:milli"`
	UpdatedAt time.Time `gorm:"not null;autoUpdateTime:milli"`
}

// AuthenticationResult contains the result of a successful authentication
type AuthenticationResult struct {
	User         *User
	IsFirstLogin bool
}

// FindByEmail retrieves a user by email address
func FindByEmail(db *gorm.DB, email string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// FindByID retrieves a user by ID
func FindByID(db *gorm.DB, id uint) (*User, error) {
	var user User
	if err := db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// Authenticate verifies credentials and updates last login timestamp
func Authenticate(logger *slog.Logger, db *gorm.DB, email, password string) (*AuthenticationResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	if email == "" || password == "" {
		return nil, ErrMissingFields
	}

	var user User
	err := db.Where("email = ?", email).First(&user).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		logger.Error("database query failed during authentication", slog.Any("error", err), slog.String("email", email))
		return nil, err
	}

	// Always run bcrypt — against the real hash, or a dummy when the email
	// doesn't exist — so response time can't reveal whether an account exists.
	hash := user.PasswordHash
	if err == gorm.ErrRecordNotFound {
		hash = timingEqualizerHash
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || err == gorm.ErrRecordNotFound {
		return nil, ErrInvalidCredentials
	}

	// Check if this is the first login
	isFirstLogin := user.LastLoginAt == nil

	// Update last login timestamp
	now := time.Now()
	user.LastLoginAt = &now

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Save(&user).Error
	}); err != nil {
		logger.Error("failed to update last login timestamp", slog.Any("error", err), slog.String("email", email))
		return nil, err
	}

	return &AuthenticationResult{
		User:         &user,
		IsFirstLogin: isFirstLogin,
	}, nil
}

// ChangeEmail updates a user's email after verifying the current password.
// The new email is lowercased and trimmed. A request to "change" to the same
// email (after normalization) is a no-op and returns nil.
func ChangeEmail(logger *slog.Logger, db *gorm.DB, currentEmail, newEmail, currentPassword string) error {
	currentEmail = strings.ToLower(strings.TrimSpace(currentEmail))
	newEmail = strings.ToLower(strings.TrimSpace(newEmail))

	if newEmail == "" {
		return ErrInvalidEmail
	}
	if _, err := mail.ParseAddress(newEmail); err != nil {
		return ErrInvalidEmail
	}

	var user User
	if err := db.Where("email = ?", currentEmail).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrUserNotFound
		}
		logger.Error("database query failed during email change", slog.Any("error", err), slog.String("email", currentEmail))
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrPasswordMismatch
	}

	if newEmail == currentEmail {
		return nil
	}

	var conflict User
	err := db.Where("email = ?", newEmail).First(&conflict).Error
	if err == nil {
		return ErrDuplicateEmail
	}
	if err != gorm.ErrRecordNotFound {
		logger.Error("database query failed during email uniqueness check", slog.Any("error", err), slog.String("email", newEmail))
		return err
	}

	user.Email = newEmail

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Save(&user).Error
	}); err != nil {
		logger.Error("failed to update email", slog.Any("error", err), slog.String("email", currentEmail))
		return err
	}

	return nil
}

// ChangePassword validates and updates user password
func ChangePassword(logger *slog.Logger, db *gorm.DB, email, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrWeakPassword
	}

	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrUserNotFound
		}
		logger.Error("database query failed during password change", slog.Any("error", err), slog.String("email", email))
		return err
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrPasswordMismatch
	}

	// Generate new password hash
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		logger.Error("failed to generate password hash", slog.Any("error", err))
		return err
	}

	// Update user password
	user.PasswordHash = string(hash)

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Save(&user).Error
	}); err != nil {
		logger.Error("failed to update password", slog.Any("error", err), slog.String("email", email))
		return err
	}

	return nil
}
