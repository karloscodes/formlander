package integrations

import (
	"time"

	"gorm.io/gorm"
)

// MailerProfile stores reusable email provider configuration.
type MailerProfile struct {
	ID               uint   `gorm:"primaryKey"`
	Name             string `gorm:"size:255;not null;uniqueIndex"`
	Provider         string `gorm:"size:50;not null;default:'mailgun'"` // mailgun, smtp, etc.
	APIKey           string `gorm:"type:text"`                          // Secret credential
	Domain           string `gorm:"size:255"`
	DefaultFromName  string `gorm:"size:255"`
	DefaultFromEmail string `gorm:"size:255"`
	DefaultsJSON     string `gorm:"type:text"` // JSON: tags, template, headers
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CaptchaProfile stores reusable captcha provider configuration.
type CaptchaProfile struct {
	ID           uint   `gorm:"primaryKey"`
	Name         string `gorm:"size:255;not null;uniqueIndex"`
	Provider     string `gorm:"size:50;not null;default:'turnstile'"` // turnstile, recaptcha, etc.
	SecretKey    string `gorm:"type:text"`                            // Server-side secret
	SiteKeysJSON string `gorm:"type:text"`                            // JSON: [{host_pattern, site_key}]
	PolicyJSON   string `gorm:"type:text"`                            // JSON: {required, action, widget}
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ListMailerProfiles retrieves all mailer profiles ordered by name
func ListMailerProfiles(db *gorm.DB) ([]MailerProfile, error) {
	var profiles []MailerProfile
	if err := db.Order("name ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// ListCaptchaProfiles retrieves all captcha profiles ordered by name
func ListCaptchaProfiles(db *gorm.DB) ([]CaptchaProfile, error) {
	var profiles []CaptchaProfile
	if err := db.Order("name ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// GetMailerProfileByID retrieves a mailer profile by ID
func GetMailerProfileByID(db *gorm.DB, id uint) (*MailerProfile, error) {
	var profile MailerProfile
	if err := db.First(&profile, id).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// GetCaptchaProfileByID retrieves a captcha profile by ID
func GetCaptchaProfileByID(db *gorm.DB, id uint) (*CaptchaProfile, error) {
	var profile CaptchaProfile
	if err := db.First(&profile, id).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}
