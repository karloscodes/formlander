package testsupport

import (
	"testing"

	"formlander/internal/accounts"
	"formlander/internal/forms"
	"formlander/internal/integrations"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SetupTestDB creates an in-memory SQLite database for testing
// with all models migrated. This is useful for integration tests
// that need a real database without external dependencies.
func SetupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test database")

	// Migrate all models
	err = db.AutoMigrate(
		// Accounts
		&accounts.User{},
		// Forms
		&forms.Form{},
		&forms.Submission{},
		&forms.EmailDelivery{},
		&forms.WebhookDelivery{},
		&forms.WebhookEvent{},
		&forms.EmailEvent{},
		// Integrations
		&integrations.MailerProfile{},
		&integrations.CaptchaProfile{},
	)
	require.NoError(t, err, "failed to migrate test database")

	return db
}
