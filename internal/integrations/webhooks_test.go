package integrations_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"formlander/internal/integrations"
	"formlander/internal/pkg/testsupport"
)

func TestWebhookProfiles(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	valid := integrations.WebhookProfileParams{
		Name:   "Zapier",
		URL:    "https://hooks.example.com/in",
		Secret: "s3cret",
		Headers: []integrations.WebhookHeader{
			{Name: "X-Team", Value: "sales"},
			{Name: "Authorization", Value: "Bearer token"},
			{Name: "", Value: ""},
		},
	}

	t.Run("creates a profile and keeps its headers", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)

		profile, err := integrations.CreateWebhookProfile(logger, db, valid)

		require.NoError(t, err)
		loaded, err := integrations.GetWebhookProfileByID(db, profile.ID)
		require.NoError(t, err)
		assert.Equal(t, "Zapier", loaded.Name)
		assert.Equal(t, "https://hooks.example.com/in", loaded.URL)
		assert.Equal(t, "s3cret", loaded.Secret)
		assert.Equal(t, []integrations.WebhookHeader{
			{Name: "Authorization", Value: "Bearer token"},
			{Name: "X-Team", Value: "sales"},
		}, loaded.Headers())
	})

	t.Run("rejects what cannot work", func(t *testing.T) {
		tests := []struct {
			name   string
			change func(*integrations.WebhookProfileParams)
			field  string
		}{
			{"no name", func(p *integrations.WebhookProfileParams) { p.Name = " " }, "name"},
			{"no URL", func(p *integrations.WebhookProfileParams) { p.URL = "" }, "url"},
			{"a URL without http", func(p *integrations.WebhookProfileParams) { p.URL = "hooks.example.com/in" }, "url"},
			{"a header value without a name", func(p *integrations.WebhookProfileParams) {
				p.Headers = []integrations.WebhookHeader{{Value: "Bearer token"}}
			}, "headers"},
			{"a header name with a colon", func(p *integrations.WebhookProfileParams) {
				p.Headers = []integrations.WebhookHeader{{Name: "Authorization:", Value: "x"}}
			}, "headers"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				db := testsupport.SetupTestDB(t)
				params := valid
				tt.change(&params)

				_, err := integrations.CreateWebhookProfile(logger, db, params)

				valErr, ok := err.(*integrations.ValidationError)
				require.True(t, ok, "want a validation error, got %v", err)
				assert.Equal(t, tt.field, valErr.Field)
			})
		}
	})

	t.Run("rejects a second profile with the same name", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		_, err := integrations.CreateWebhookProfile(logger, db, valid)
		require.NoError(t, err)

		_, err = integrations.CreateWebhookProfile(logger, db, valid)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("updates a profile and removes headers that are gone", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		profile, err := integrations.CreateWebhookProfile(logger, db, valid)
		require.NoError(t, err)

		updated, err := integrations.UpdateWebhookProfile(logger, db, profile.ID, integrations.WebhookProfileParams{
			Name: "Zapier",
			URL:  "https://hooks.example.com/new",
		})

		require.NoError(t, err)
		assert.Equal(t, "https://hooks.example.com/new", updated.URL)
		assert.Empty(t, updated.Secret)
		assert.Empty(t, updated.Headers())
	})

	t.Run("lists profiles by name and deletes one", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		second := valid
		second.Name = "Airtable"
		_, err := integrations.CreateWebhookProfile(logger, db, valid)
		require.NoError(t, err)
		airtable, err := integrations.CreateWebhookProfile(logger, db, second)
		require.NoError(t, err)

		require.NoError(t, integrations.DeleteWebhookProfile(logger, db, airtable.ID))

		profiles, err := integrations.ListWebhookProfiles(db)
		require.NoError(t, err)
		require.Len(t, profiles, 1)
		assert.Equal(t, "Zapier", profiles[0].Name)
	})
}
