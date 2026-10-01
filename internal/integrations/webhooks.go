package integrations

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"formlander/internal/pkg/dbtxn"
)

// WebhookProfile stores one place that receives submissions: a URL, the
// secret that signs each request, and the headers to send with it. Many
// forms can use one profile.
type WebhookProfile struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"size:255;not null;uniqueIndex"`
	URL         string `gorm:"type:text;not null"`
	Secret      string `gorm:"size:255"`  // Signs the body with HMAC-SHA256
	HeadersJSON string `gorm:"type:text"` // JSON object: {"Header-Name": "value"}
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WebhookHeader is one custom header of a webhook profile.
type WebhookHeader struct {
	Name  string
	Value string
}

// Headers returns the custom headers of the profile, sorted by name.
func (p *WebhookProfile) Headers() []WebhookHeader {
	var stored map[string]string
	if p == nil || json.Unmarshal([]byte(p.HeadersJSON), &stored) != nil {
		return nil
	}
	headers := make([]WebhookHeader, 0, len(stored))
	for name, value := range stored {
		headers = append(headers, WebhookHeader{Name: name, Value: value})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
	return headers
}

// WebhookProfileParams holds parameters for creating/updating a webhook profile
type WebhookProfileParams struct {
	Name    string
	URL     string
	Secret  string
	Headers []WebhookHeader
}

// ListWebhookProfiles retrieves all webhook profiles ordered by name
func ListWebhookProfiles(db *gorm.DB) ([]WebhookProfile, error) {
	var profiles []WebhookProfile
	if err := db.Order("name ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

// GetWebhookProfileByID retrieves a webhook profile by ID
func GetWebhookProfileByID(db *gorm.DB, id uint) (*WebhookProfile, error) {
	var profile WebhookProfile
	if err := db.First(&profile, id).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// CreateWebhookProfile creates a new webhook profile
func CreateWebhookProfile(logger *slog.Logger, db *gorm.DB, params WebhookProfileParams) (*WebhookProfile, error) {
	profile, err := validWebhookProfile(db, 0, params)
	if err != nil {
		return nil, err
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Create(profile).Error
	}); err != nil {
		logger.Error("failed to create webhook profile", slog.Any("error", err))
		return nil, fmt.Errorf("failed to create profile: %w", err)
	}

	return profile, nil
}

// UpdateWebhookProfile updates an existing webhook profile
func UpdateWebhookProfile(logger *slog.Logger, db *gorm.DB, id uint, params WebhookProfileParams) (*WebhookProfile, error) {
	if _, err := GetWebhookProfileByID(db, id); err != nil {
		return nil, err
	}
	profile, err := validWebhookProfile(db, id, params)
	if err != nil {
		return nil, err
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Model(&WebhookProfile{}).Where("id = ?", id).Updates(map[string]any{
			"name":         profile.Name,
			"url":          profile.URL,
			"secret":       profile.Secret,
			"headers_json": profile.HeadersJSON,
		}).Error
	}); err != nil {
		logger.Error("failed to update webhook profile", slog.Any("error", err), slog.Uint64("id", uint64(id)))
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	return GetWebhookProfileByID(db, id)
}

// DeleteWebhookProfile deletes a webhook profile
func DeleteWebhookProfile(logger *slog.Logger, db *gorm.DB, id uint) error {
	return dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Delete(&WebhookProfile{}, id).Error
	})
}

// validWebhookProfile checks the parameters and returns the profile they
// describe. id is the profile that is being updated, or 0 for a new one.
func validWebhookProfile(db *gorm.DB, id uint, params WebhookProfileParams) (*WebhookProfile, error) {
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "Name is required"}
	}

	var count int64
	if err := db.Model(&WebhookProfile{}).Where("name = ? AND id != ?", name, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, &ValidationError{Field: "name", Message: "A profile with this name already exists"}
	}

	address := strings.TrimSpace(params.URL)
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, &ValidationError{Field: "url", Message: "The URL must start with https:// or http://"}
	}

	headers := map[string]string{}
	for _, header := range params.Headers {
		headerName, value := strings.TrimSpace(header.Name), strings.TrimSpace(header.Value)
		if headerName == "" && value == "" {
			continue
		}
		if headerName == "" || strings.ContainsAny(headerName, " :\r\n") || strings.ContainsAny(value, "\r\n") {
			return nil, &ValidationError{Field: "headers", Message: "A header needs a name without spaces or colons, for example Authorization"}
		}
		headers[headerName] = value
	}
	headersJSON := ""
	if len(headers) > 0 {
		encoded, _ := json.Marshal(headers)
		headersJSON = string(encoded)
	}

	return &WebhookProfile{
		Name:        name,
		URL:         address,
		Secret:      strings.TrimSpace(params.Secret),
		HeadersJSON: headersJSON,
	}, nil
}
