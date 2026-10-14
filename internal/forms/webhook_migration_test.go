package forms_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"formlander/internal/forms"
	"formlander/internal/integrations"
	"formlander/internal/pkg/testsupport"
)

// inlineWebhook makes a form that holds its webhook the old way: in the
// delivery row.
func inlineWebhook(t *testing.T, db *gorm.DB, slug, url, secret string) *forms.Form {
	t.Helper()
	form := &forms.Form{Name: slug, Slug: slug}
	require.NoError(t, db.Create(form).Error)
	require.NoError(t, db.Create(&forms.WebhookDelivery{
		FormID:  form.ID,
		Enabled: true,
		URL:     url,
		Secret:  secret,
	}).Error)
	return form
}

func TestMigrateInlineWebhooks(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("a form with an inline webhook keeps delivering through a profile", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		form := inlineWebhook(t, db, "contact", "https://hooks.example.com/in", "s3cret")

		require.NoError(t, forms.MigrateInlineWebhooks(logger, db))

		loaded, err := forms.GetByID(db, form.ID)
		require.NoError(t, err)
		require.NotNil(t, loaded.WebhookDelivery.WebhookProfile)
		assert.True(t, loaded.WebhookDelivery.Delivers())
		assert.Equal(t, "hooks.example.com", loaded.WebhookDelivery.WebhookProfile.Name)
		assert.Equal(t, "https://hooks.example.com/in", loaded.WebhookDelivery.WebhookProfile.URL)
		assert.Equal(t, "s3cret", loaded.WebhookDelivery.WebhookProfile.Secret)

		_, err = forms.CreateSubmission(logger, db, loaded, map[string]any{"name": "Ada"}, "test")
		require.NoError(t, err)
		var events int64
		db.Model(&forms.WebhookEvent{}).Count(&events)
		assert.Equal(t, int64(1), events)
	})

	t.Run("forms with the same webhook share one profile", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		inlineWebhook(t, db, "one", "https://hooks.example.com/in", "")
		inlineWebhook(t, db, "two", "https://hooks.example.com/in", "")
		inlineWebhook(t, db, "three", "https://hooks.example.com/other", "")

		require.NoError(t, forms.MigrateInlineWebhooks(logger, db))

		profiles, err := integrations.ListWebhookProfiles(db)
		require.NoError(t, err)
		require.Len(t, profiles, 2)
		assert.Equal(t, "hooks.example.com", profiles[0].Name)
		assert.Equal(t, "hooks.example.com 2", profiles[1].Name)
	})

	t.Run("a second run changes nothing", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		inlineWebhook(t, db, "contact", "https://hooks.example.com/in", "")

		require.NoError(t, forms.MigrateInlineWebhooks(logger, db))
		require.NoError(t, forms.MigrateInlineWebhooks(logger, db))

		profiles, err := integrations.ListWebhookProfiles(db)
		require.NoError(t, err)
		assert.Len(t, profiles, 1)
	})

	t.Run("a profile that the owner removed from a form does not come back", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		form := inlineWebhook(t, db, "contact", "https://hooks.example.com/in", "")
		require.NoError(t, forms.MigrateInlineWebhooks(logger, db))

		_, err := forms.Update(logger, db, forms.UpdateParams{ID: form.ID, Name: "contact"})
		require.NoError(t, err)
		require.NoError(t, forms.MigrateInlineWebhooks(logger, db))

		loaded, err := forms.GetByID(db, form.ID)
		require.NoError(t, err)
		assert.False(t, loaded.WebhookDelivery.Delivers())
		assert.Nil(t, loaded.WebhookDelivery.WebhookProfileID)
	})

	t.Run("a form without a webhook gets no profile", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		form := &forms.Form{Name: "plain", Slug: "plain"}
		require.NoError(t, db.Create(form).Error)
		require.NoError(t, db.Create(&forms.WebhookDelivery{FormID: form.ID}).Error)

		require.NoError(t, forms.MigrateInlineWebhooks(logger, db))

		profiles, err := integrations.ListWebhookProfiles(db)
		require.NoError(t, err)
		assert.Empty(t, profiles)
	})
}
