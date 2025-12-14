package integrations

import (
	"encoding/json"
	"fmt"
	"strings"

	"log/slog"
	"gorm.io/gorm"

	"formlander/internal/pkg/dbtxn"
)

// CaptchaProfileParams holds parameters for creating/updating a captcha profile
type CaptchaProfileParams struct {
	Name         string
	Provider     string
	SecretKey    string
	SiteKeysJSON string
	PolicyJSON   string
}

// CreateCaptchaProfile creates a new captcha profile
func CreateCaptchaProfile(logger *slog.Logger, db *gorm.DB, params CaptchaProfileParams) (*CaptchaProfile, error) {
	// Validate required fields
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "Name is required"}
	}

	// Check for duplicate name
	var count int64
	if err := db.Model(&CaptchaProfile{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, &ValidationError{Field: "name", Message: "A profile with this name already exists"}
	}

	// Validate site keys JSON if provided
	siteKeysJSON := strings.TrimSpace(params.SiteKeysJSON)
	if siteKeysJSON != "" {
		var temp interface{}
		if err := json.Unmarshal([]byte(siteKeysJSON), &temp); err != nil {
			return nil, &ValidationError{Field: "site_keys_json", Message: "Invalid JSON in site keys field"}
		}
	}

	// Validate policy JSON if provided
	policyJSON := strings.TrimSpace(params.PolicyJSON)
	if policyJSON != "" {
		var temp interface{}
		if err := json.Unmarshal([]byte(policyJSON), &temp); err != nil {
			return nil, &ValidationError{Field: "policy_json", Message: "Invalid JSON in policy field"}
		}
	}

	profile := &CaptchaProfile{
		Name:         name,
		Provider:     strings.TrimSpace(params.Provider),
		SecretKey:    strings.TrimSpace(params.SecretKey),
		SiteKeysJSON: siteKeysJSON,
		PolicyJSON:   policyJSON,
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Create(profile).Error
	}); err != nil {
		logger.Error("failed to create captcha profile", slog.Any("error", err))
		return nil, fmt.Errorf("failed to create profile: %w", err)
	}

	return profile, nil
}

// UpdateCaptchaProfile updates an existing captcha profile
func UpdateCaptchaProfile(logger *slog.Logger, db *gorm.DB, id uint, params CaptchaProfileParams) (*CaptchaProfile, error) {
	// Validate required fields
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "Name is required"}
	}

	// Get existing profile
	profile, err := GetCaptchaProfileByID(db, id)
	if err != nil {
		return nil, err
	}

	// Check for duplicate name (excluding current profile)
	var count int64
	if err := db.Model(&CaptchaProfile{}).Where("name = ? AND id != ?", name, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, &ValidationError{Field: "name", Message: "A profile with this name already exists"}
	}

	// Validate site keys JSON if provided
	siteKeysJSON := strings.TrimSpace(params.SiteKeysJSON)
	if siteKeysJSON != "" {
		var temp interface{}
		if err := json.Unmarshal([]byte(siteKeysJSON), &temp); err != nil {
			return nil, &ValidationError{Field: "site_keys_json", Message: "Invalid JSON in site keys field"}
		}
	}

	// Validate policy JSON if provided
	policyJSON := strings.TrimSpace(params.PolicyJSON)
	if policyJSON != "" {
		var temp interface{}
		if err := json.Unmarshal([]byte(policyJSON), &temp); err != nil {
			return nil, &ValidationError{Field: "policy_json", Message: "Invalid JSON in policy field"}
		}
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Model(profile).Updates(map[string]any{
			"name":           name,
			"provider":       strings.TrimSpace(params.Provider),
			"secret_key":     strings.TrimSpace(params.SecretKey),
			"site_keys_json": siteKeysJSON,
			"policy_json":    policyJSON,
		}).Error
	}); err != nil {
		logger.Error("failed to update captcha profile", slog.Any("error", err), slog.Uint64("id", uint64(id)))
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Reload profile
	return GetCaptchaProfileByID(db, id)
}

// DeleteCaptchaProfile deletes a captcha profile
func DeleteCaptchaProfile(logger *slog.Logger, db *gorm.DB, id uint) error {
	return dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Delete(&CaptchaProfile{}, id).Error
	})
}
