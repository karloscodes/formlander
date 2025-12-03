package integrations_test

import (
	"testing"

	"formlander/internal/integrations"
	"formlander/internal/pkg/testsupport"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCreateMailerProfile(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("creates profile successfully", func(t *testing.T) {
		params := integrations.MailerProfileParams{
			Name:             "Test Mailgun",
			Provider:         "mailgun",
			APIKey:           "test-key",
			Domain:           "test.com",
			DefaultFromName:  "Test",
			DefaultFromEmail: "test@test.com",
			DefaultsJSON:     `{"key": "value"}`,
		}

		profile, err := integrations.CreateMailerProfile(logger, db, params)
		require.NoError(t, err)
		assert.NotNil(t, profile)
		assert.Equal(t, "Test Mailgun", profile.Name)
		assert.Equal(t, "mailgun", profile.Provider)
		assert.NotZero(t, profile.ID)
	})

	t.Run("validates name is required", func(t *testing.T) {
		params := integrations.MailerProfileParams{
			Name: "",
		}

		_, err := integrations.CreateMailerProfile(logger, db, params)
		require.Error(t, err)

		valErr, ok := err.(*integrations.ValidationError)
		assert.True(t, ok, "Expected ValidationError")
		assert.Equal(t, "name", valErr.Field)
	})

	t.Run("prevents duplicate names", func(t *testing.T) {
		params := integrations.MailerProfileParams{
			Name: "Duplicate Test",
		}

		// Create first profile
		_, err := integrations.CreateMailerProfile(logger, db, params)
		require.NoError(t, err)

		// Try to create duplicate
		_, err = integrations.CreateMailerProfile(logger, db, params)
		require.Error(t, err)

		valErr, ok := err.(*integrations.ValidationError)
		assert.True(t, ok, "Expected ValidationError")
		assert.Contains(t, valErr.Message, "already exists")
	})

	t.Run("validates JSON format", func(t *testing.T) {
		params := integrations.MailerProfileParams{
			Name:         "JSON Test",
			DefaultsJSON: `{"invalid json`,
		}

		_, err := integrations.CreateMailerProfile(logger, db, params)
		require.Error(t, err)

		valErr, ok := err.(*integrations.ValidationError)
		assert.True(t, ok, "Expected ValidationError")
		assert.Equal(t, "defaults_json", valErr.Field)
	})
}

func TestUpdateMailerProfile(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("updates profile successfully", func(t *testing.T) {
		// Create initial profile
		createParams := integrations.MailerProfileParams{
			Name: "Original Name",
		}
		profile, err := integrations.CreateMailerProfile(logger, db, createParams)
		require.NoError(t, err)

		// Update profile
		updateParams := integrations.MailerProfileParams{
			Name:     "Updated Name",
			Provider: "smtp",
		}
		updated, err := integrations.UpdateMailerProfile(logger, db, profile.ID, updateParams)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", updated.Name)
		assert.Equal(t, "smtp", updated.Provider)
	})

	t.Run("validates name on update", func(t *testing.T) {
		createParams := integrations.MailerProfileParams{
			Name: "Update Test",
		}
		profile, err := integrations.CreateMailerProfile(logger, db, createParams)
		require.NoError(t, err)

		updateParams := integrations.MailerProfileParams{
			Name: "", // Empty name
		}
		_, err = integrations.UpdateMailerProfile(logger, db, profile.ID, updateParams)
		require.Error(t, err)
	})

	t.Run("prevents duplicate name on update", func(t *testing.T) {
		// Create two profiles
		params1 := integrations.MailerProfileParams{Name: "Profile One"}
		_, err := integrations.CreateMailerProfile(logger, db, params1)
		require.NoError(t, err)

		params2 := integrations.MailerProfileParams{Name: "Profile Two"}
		profile2, err := integrations.CreateMailerProfile(logger, db, params2)
		require.NoError(t, err)

		// Try to update profile2 with profile1's name
		updateParams := integrations.MailerProfileParams{Name: "Profile One"}
		_, err = integrations.UpdateMailerProfile(logger, db, profile2.ID, updateParams)
		require.Error(t, err)
		valErr, ok := err.(*integrations.ValidationError)
		require.True(t, ok)
		assert.Contains(t, valErr.Message, "already exists")
	})

	t.Run("returns error for non-existent profile", func(t *testing.T) {
		updateParams := integrations.MailerProfileParams{Name: "Test"}
		_, err := integrations.UpdateMailerProfile(logger, db, 99999, updateParams)
		require.Error(t, err)
	})
}

func TestCreateCaptchaProfile(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("creates profile successfully", func(t *testing.T) {
		params := integrations.CaptchaProfileParams{
			Name:         "Test Turnstile",
			Provider:     "turnstile",
			SecretKey:    "test-secret",
			SiteKeysJSON: `[{"host_pattern": "*", "site_key": "test-key"}]`,
			PolicyJSON:   `{"required": true}`,
		}

		profile, err := integrations.CreateCaptchaProfile(logger, db, params)
		require.NoError(t, err)
		assert.NotNil(t, profile)
		assert.Equal(t, "Test Turnstile", profile.Name)
		assert.Equal(t, "turnstile", profile.Provider)
		assert.NotZero(t, profile.ID)
	})

	t.Run("validates name is required", func(t *testing.T) {
		params := integrations.CaptchaProfileParams{
			Name: "",
		}

		_, err := integrations.CreateCaptchaProfile(logger, db, params)
		require.Error(t, err)

		valErr, ok := err.(*integrations.ValidationError)
		assert.True(t, ok, "Expected ValidationError")
		assert.Equal(t, "name", valErr.Field)
	})

	t.Run("validates site keys JSON", func(t *testing.T) {
		params := integrations.CaptchaProfileParams{
			Name:         "JSON Test",
			SiteKeysJSON: `{invalid}`,
		}

		_, err := integrations.CreateCaptchaProfile(logger, db, params)
		require.Error(t, err)

		valErr, ok := err.(*integrations.ValidationError)
		assert.True(t, ok, "Expected ValidationError")
		assert.Equal(t, "site_keys_json", valErr.Field)
	})

	t.Run("validates policy JSON", func(t *testing.T) {
		params := integrations.CaptchaProfileParams{
			Name:       "Policy Test",
			PolicyJSON: `[not valid json`,
		}

		_, err := integrations.CreateCaptchaProfile(logger, db, params)
		require.Error(t, err)

		valErr, ok := err.(*integrations.ValidationError)
		assert.True(t, ok, "Expected ValidationError")
		assert.Equal(t, "policy_json", valErr.Field)
	})
}

func TestListProfiles(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("lists mailer profiles", func(t *testing.T) {
		// Create some profiles
		for i := 1; i <= 3; i++ {
			params := integrations.MailerProfileParams{
				Name: "Mailer " + string(rune('A'+i-1)),
			}
			_, err := integrations.CreateMailerProfile(logger, db, params)
			require.NoError(t, err)
		}

		profiles, err := integrations.ListMailerProfiles(db)
		require.NoError(t, err)
		assert.Len(t, profiles, 3)
		// Should be ordered by name ASC
		assert.Equal(t, "Mailer A", profiles[0].Name)
	})

	t.Run("lists captcha profiles", func(t *testing.T) {
		// Create some profiles
		for i := 1; i <= 2; i++ {
			params := integrations.CaptchaProfileParams{
				Name: "Captcha " + string(rune('X'+i-1)),
			}
			_, err := integrations.CreateCaptchaProfile(logger, db, params)
			require.NoError(t, err)
		}

		profiles, err := integrations.ListCaptchaProfiles(db)
		require.NoError(t, err)
		assert.Len(t, profiles, 2)
	})
}

func TestDeleteProfiles(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("deletes mailer profile", func(t *testing.T) {
		params := integrations.MailerProfileParams{
			Name: "Delete Me",
		}
		profile, err := integrations.CreateMailerProfile(logger, db, params)
		require.NoError(t, err)

		err = integrations.DeleteMailerProfile(logger, db, profile.ID)
		require.NoError(t, err)

		// Verify it's deleted
		_, err = integrations.GetMailerProfileByID(db, profile.ID)
		assert.Error(t, err)
	})

	t.Run("deletes captcha profile", func(t *testing.T) {
		params := integrations.CaptchaProfileParams{
			Name: "Delete Me",
		}
		profile, err := integrations.CreateCaptchaProfile(logger, db, params)
		require.NoError(t, err)

		err = integrations.DeleteCaptchaProfile(logger, db, profile.ID)
		require.NoError(t, err)

		// Verify it's deleted
		_, err = integrations.GetCaptchaProfileByID(db, profile.ID)
		assert.Error(t, err)
	})
}
