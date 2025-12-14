package accounts

import (
	"errors"
	"strings"
	"time"

	"log/slog"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"formlander/internal/pkg/dbtxn"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotFound       = errors.New("user not found")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrPasswordMismatch   = errors.New("current password is incorrect")
	ErrMissingFields      = errors.New("required fields are missing")
)

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
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrInvalidCredentials
		}
		logger.Error("database query failed during authentication", slog.Any("error", err), slog.String("email", email))
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
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

// GetByEmail retrieves user by email
func GetByEmail(db *gorm.DB, email string) (*User, error) {
	var user User
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}
