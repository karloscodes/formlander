package integrations

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"formlander/internal/pkg/dbtxn"
)

// MailerProfileParams holds parameters for creating/updating a mailer profile
type MailerProfileParams struct {
	Name             string
	Provider         string
	APIKey           string
	Domain           string
	DefaultFromName  string
	DefaultFromEmail string
	DefaultsJSON     string
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// CreateMailerProfile creates a new mailer profile
func CreateMailerProfile(logger *zap.Logger, db *gorm.DB, params MailerProfileParams) (*MailerProfile, error) {
	// Validate required fields
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "Name is required"}
	}

	// Check for duplicate name
	var count int64
	if err := db.Model(&MailerProfile{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, &ValidationError{Field: "name", Message: "A profile with this name already exists"}
	}

	// Validate JSON if provided
	defaultsJSON := strings.TrimSpace(params.DefaultsJSON)
	if defaultsJSON != "" {
		var temp interface{}
		if err := json.Unmarshal([]byte(defaultsJSON), &temp); err != nil {
			return nil, &ValidationError{Field: "defaults_json", Message: "Invalid JSON in defaults field"}
		}
	}

	profile := &MailerProfile{
		Name:             name,
		Provider:         strings.TrimSpace(params.Provider),
		APIKey:           strings.TrimSpace(params.APIKey),
		Domain:           strings.TrimSpace(params.Domain),
		DefaultFromName:  strings.TrimSpace(params.DefaultFromName),
		DefaultFromEmail: strings.TrimSpace(params.DefaultFromEmail),
		DefaultsJSON:     defaultsJSON,
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Create(profile).Error
	}); err != nil {
		logger.Error("failed to create mailer profile", zap.Error(err))
		return nil, fmt.Errorf("failed to create profile: %w", err)
	}

	return profile, nil
}

// UpdateMailerProfile updates an existing mailer profile
func UpdateMailerProfile(logger *zap.Logger, db *gorm.DB, id uint, params MailerProfileParams) (*MailerProfile, error) {
	// Validate required fields
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "Name is required"}
	}

	// Get existing profile
	profile, err := GetMailerProfileByID(db, id)
	if err != nil {
		return nil, err
	}

	// Check for duplicate name (excluding current profile)
	var count int64
	if err := db.Model(&MailerProfile{}).Where("name = ? AND id != ?", name, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, &ValidationError{Field: "name", Message: "A profile with this name already exists"}
	}

	// Validate JSON if provided
	defaultsJSON := strings.TrimSpace(params.DefaultsJSON)
	if defaultsJSON != "" {
		var temp interface{}
		if err := json.Unmarshal([]byte(defaultsJSON), &temp); err != nil {
			return nil, &ValidationError{Field: "defaults_json", Message: "Invalid JSON in defaults field"}
		}
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Model(profile).Updates(map[string]any{
			"name":               name,
			"provider":           strings.TrimSpace(params.Provider),
			"api_key":            strings.TrimSpace(params.APIKey),
			"domain":             strings.TrimSpace(params.Domain),
			"default_from_name":  strings.TrimSpace(params.DefaultFromName),
			"default_from_email": strings.TrimSpace(params.DefaultFromEmail),
			"defaults_json":      defaultsJSON,
		}).Error
	}); err != nil {
		logger.Error("failed to update mailer profile", zap.Error(err), zap.Uint("id", id))
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Reload profile
	return GetMailerProfileByID(db, id)
}

// DeleteMailerProfile deletes a mailer profile
func DeleteMailerProfile(logger *zap.Logger, db *gorm.DB, id uint) error {
	return dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Delete(&MailerProfile{}, id).Error
	})
}
