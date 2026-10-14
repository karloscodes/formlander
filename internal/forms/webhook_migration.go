package forms

import (
	"fmt"
	"log/slog"
	"net/url"

	"gorm.io/gorm"

	"formlander/internal/integrations"
)

// MigrateInlineWebhooks moves the webhooks that forms held inline (URL,
// secret, headers) to webhook profiles, and links each form to its profile.
// Forms with the same URL, secret, and headers share one profile.
//
// It runs at every start. A form that has a profile is skipped, so a second
// run changes nothing.
func MigrateInlineWebhooks(logger *slog.Logger, db *gorm.DB) error {
	var deliveries []WebhookDelivery
	if err := db.Where("webhook_profile_id IS NULL AND url <> ''").Find(&deliveries).Error; err != nil {
		return err
	}

	for _, delivery := range deliveries {
		delivery := delivery
		if err := db.Transaction(func(tx *gorm.DB) error {
			var profile integrations.WebhookProfile
			err := tx.Where("url = ? AND secret = ? AND headers_json = ?", delivery.URL, delivery.Secret, delivery.HeadersJSON).
				First(&profile).Error
			if err == gorm.ErrRecordNotFound {
				profile = integrations.WebhookProfile{
					Name:        freeWebhookProfileName(tx, delivery.URL),
					URL:         delivery.URL,
					Secret:      delivery.Secret,
					HeadersJSON: delivery.HeadersJSON,
				}
				err = tx.Create(&profile).Error
			}
			if err != nil {
				return err
			}
			return tx.Model(&WebhookDelivery{}).Where("id = ?", delivery.ID).
				Update("webhook_profile_id", profile.ID).Error
		}); err != nil {
			return fmt.Errorf("move the webhook of form %d to a profile: %w", delivery.FormID, err)
		}
		logger.Info("moved an inline webhook to a webhook profile", slog.Uint64("form_id", uint64(delivery.FormID)))
	}
	return nil
}

// freeWebhookProfileName names a profile after the host of its URL, and adds
// a number when that name is taken.
func freeWebhookProfileName(tx *gorm.DB, address string) string {
	base := address
	if parsed, err := url.Parse(address); err == nil && parsed.Host != "" {
		base = parsed.Host
	}
	name := base
	for n := 2; ; n++ {
		var count int64
		tx.Model(&integrations.WebhookProfile{}).Where("name = ?", name).Count(&count)
		if count == 0 {
			return name
		}
		name = fmt.Sprintf("%s %d", base, n)
	}
}
