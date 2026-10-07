package database

import (
	"gorm.io/gorm"

	"formlander/internal/accounts"
	"formlander/internal/forms"
	"formlander/internal/integrations"
)

// Migrate performs the schema migration for all application models.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&accounts.User{},
		&accounts.Settings{},
		&accounts.EndedSession{},
		&integrations.MailerProfile{},
		&integrations.CaptchaProfile{},
		&integrations.WebhookProfile{},
		&forms.Form{},
		&forms.WebhookDelivery{},
		&forms.EmailDelivery{},
		&forms.Submission{},
		&forms.WebhookEvent{},
		&forms.EmailEvent{},
		&forms.SubmissionFile{},
	)
}
