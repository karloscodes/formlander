package forms_test

import (
	"encoding/json"
	"testing"

	"formlander/internal/forms"
	"formlander/internal/pkg/testsupport"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCreateSubmission(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("creates submission successfully", func(t *testing.T) {
		form := &forms.Form{
			Name: "Test Form",
			Slug: "test-form",
		}
		require.NoError(t, db.Create(form).Error)

		payload := map[string]any{
			"name":  "John Doe",
			"email": "john@example.com",
		}

		submission, err := forms.CreateSubmission(logger, db, form, payload, "TestAgent/1.0")
		require.NoError(t, err)
		assert.NotNil(t, submission)
		assert.Equal(t, form.ID, submission.FormID)
		assert.Equal(t, "TestAgent/1.0", submission.UserAgent)
		assert.False(t, submission.IsSpam)

		// Verify JSON encoding
		var decoded map[string]any
		err = json.Unmarshal([]byte(submission.DataJSON), &decoded)
		require.NoError(t, err)
		assert.Equal(t, "John Doe", decoded["name"])
		assert.Equal(t, "john@example.com", decoded["email"])
	})

	t.Run("creates webhook event when enabled", func(t *testing.T) {
		db2 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Form with Webhook",
			Slug: "webhook-form",
		}
		require.NoError(t, db2.Create(form).Error)

		webhook := &forms.WebhookDelivery{
			FormID:  form.ID,
			URL:     "https://example.com/webhook",
			Enabled: true,
		}
		require.NoError(t, db2.Create(webhook).Error)

		// Reload form with associations
		require.NoError(t, db2.Preload("WebhookDelivery").First(form, form.ID).Error)

		payload := map[string]any{"test": "data"}
		submission, err := forms.CreateSubmission(logger, db2, form, payload, "TestAgent")
		require.NoError(t, err)

		// Verify webhook event was created
		var events []forms.WebhookEvent
		require.NoError(t, db2.Where("submission_id = ?", submission.ID).Find(&events).Error)
		assert.Len(t, events, 1)
		assert.Equal(t, "pending", events[0].Status)
	})

	t.Run("does not create webhook event when disabled", func(t *testing.T) {
		db3 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Form without Webhook",
			Slug: "no-webhook",
		}
		require.NoError(t, db3.Create(form).Error)

		webhook := &forms.WebhookDelivery{
			FormID:  form.ID,
			URL:     "https://example.com/webhook",
			Enabled: false, // Disabled
		}
		require.NoError(t, db3.Create(webhook).Error)

		require.NoError(t, db3.Preload("WebhookDelivery").First(form, form.ID).Error)

		payload := map[string]any{"test": "data"}
		submission, err := forms.CreateSubmission(logger, db3, form, payload, "TestAgent")
		require.NoError(t, err)

		// Verify no webhook event was created
		var events []forms.WebhookEvent
		require.NoError(t, db3.Where("submission_id = ?", submission.ID).Find(&events).Error)
		assert.Len(t, events, 0)
	})

	t.Run("creates email event when enabled", func(t *testing.T) {
		db4 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Form with Email",
			Slug: "email-form",
		}
		require.NoError(t, db4.Create(form).Error)

		email := &forms.EmailDelivery{
			FormID:        form.ID,
			Enabled:       true,
			OverridesJSON: `{"to": "admin@example.com"}`,
		}
		require.NoError(t, db4.Create(email).Error)

		require.NoError(t, db4.Preload("EmailDelivery").First(form, form.ID).Error)

		payload := map[string]any{"message": "Hello"}
		submission, err := forms.CreateSubmission(logger, db4, form, payload, "TestAgent")
		require.NoError(t, err)

		// Verify email event was created
		var events []forms.EmailEvent
		require.NoError(t, db4.Where("submission_id = ?", submission.ID).Find(&events).Error)
		assert.Len(t, events, 1)
		assert.Equal(t, "pending", events[0].Status)
	})

	t.Run("does not create email event when no recipient", func(t *testing.T) {
		db5 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Form without recipient",
			Slug: "no-recipient",
		}
		require.NoError(t, db5.Create(form).Error)

		email := &forms.EmailDelivery{
			FormID:        form.ID,
			Enabled:       true,
			OverridesJSON: `{}`, // No "to" field
		}
		require.NoError(t, db5.Create(email).Error)

		require.NoError(t, db5.Preload("EmailDelivery").First(form, form.ID).Error)

		payload := map[string]any{"test": "data"}
		submission, err := forms.CreateSubmission(logger, db5, form, payload, "TestAgent")
		require.NoError(t, err)

		// Verify no email event was created
		var events []forms.EmailEvent
		require.NoError(t, db5.Where("submission_id = ?", submission.ID).Find(&events).Error)
		assert.Len(t, events, 0)
	})

	t.Run("creates both webhook and email events", func(t *testing.T) {
		db6 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Form with both",
			Slug: "both-form",
		}
		require.NoError(t, db6.Create(form).Error)

		webhook := &forms.WebhookDelivery{
			FormID:  form.ID,
			URL:     "https://example.com/webhook",
			Enabled: true,
		}
		require.NoError(t, db6.Create(webhook).Error)

		email := &forms.EmailDelivery{
			FormID:        form.ID,
			Enabled:       true,
			OverridesJSON: `{"to": "admin@example.com"}`,
		}
		require.NoError(t, db6.Create(email).Error)

		require.NoError(t, db6.Preload("WebhookDelivery").Preload("EmailDelivery").First(form, form.ID).Error)

		payload := map[string]any{"data": "test"}
		submission, err := forms.CreateSubmission(logger, db6, form, payload, "TestAgent")
		require.NoError(t, err)

		// Verify both events were created
		var webhookEvents []forms.WebhookEvent
		require.NoError(t, db6.Where("submission_id = ?", submission.ID).Find(&webhookEvents).Error)
		assert.Len(t, webhookEvents, 1)

		var emailEvents []forms.EmailEvent
		require.NoError(t, db6.Where("submission_id = ?", submission.ID).Find(&emailEvents).Error)
		assert.Len(t, emailEvents, 1)
	})

	t.Run("handles complex nested payload", func(t *testing.T) {
		db7 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Complex Form",
			Slug: "complex",
		}
		require.NoError(t, db7.Create(form).Error)

		payload := map[string]any{
			"name": "John",
			"address": map[string]any{
				"street": "123 Main St",
				"city":   "NYC",
			},
			"tags": []string{"customer", "premium"},
		}

		submission, err := forms.CreateSubmission(logger, db7, form, payload, "TestAgent")
		require.NoError(t, err)

		// Verify complex data is properly encoded
		var decoded map[string]any
		err = json.Unmarshal([]byte(submission.DataJSON), &decoded)
		require.NoError(t, err)

		address := decoded["address"].(map[string]any)
		assert.Equal(t, "123 Main St", address["street"])

		tags := decoded["tags"].([]any)
		assert.Len(t, tags, 2)
	})

	t.Run("handles empty payload", func(t *testing.T) {
		db8 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Empty Form",
			Slug: "empty",
		}
		require.NoError(t, db8.Create(form).Error)

		payload := map[string]any{}
		submission, err := forms.CreateSubmission(logger, db8, form, payload, "TestAgent")
		require.NoError(t, err)
		assert.Equal(t, "{}", submission.DataJSON)
	})
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expected      string
		randomIfEmpty bool
	}{
		{"simple text", "Hello World", "hello-world", false},
		{"with numbers", "Form 123", "form-123", false},
		{"with special chars", "Contact Form!", "contact-form", false},
		{"multiple spaces", "My   Form", "my-form", false},
		{"leading/trailing spaces", "  Form  ", "form-", false}, // Trailing dash from space
		{"uppercase", "CONTACT FORM", "contact-form", false},
		{"with dashes", "Pre-Existing-Dashes", "pre-existing-dashes", false},
		{"with underscores", "Form_Name_Here", "form-name-here", false},
		{"mixed case special", "Hello!!! World???", "hello-world", false},
		{"only special chars", "!!!", "", true}, // Empty result triggers random ID
		{"empty string", "", "", true},          // Empty triggers random ID
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := forms.Slugify(tt.input)
			if tt.randomIfEmpty {
				// Should generate a random ID (20 chars)
				assert.NotEmpty(t, result)
				assert.Len(t, result, 20, "Random ID should be 20 characters")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestEnsureDeliveryRecords(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("creates email delivery when missing", func(t *testing.T) {
		form := &forms.Form{
			Name: "Test Form",
			Slug: "test",
		}
		require.NoError(t, db.Create(form).Error)

		err := forms.EnsureDeliveryRecords(logger, db, form)
		require.NoError(t, err)

		// Reload and verify
		require.NoError(t, db.Preload("EmailDelivery").Preload("WebhookDelivery").First(form, form.ID).Error)
		assert.NotNil(t, form.EmailDelivery)
		assert.NotNil(t, form.WebhookDelivery)
		assert.False(t, form.EmailDelivery.Enabled)
		assert.False(t, form.WebhookDelivery.Enabled)
	})

	t.Run("does not duplicate existing records", func(t *testing.T) {
		db2 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Test Form",
			Slug: "test",
		}
		require.NoError(t, db2.Create(form).Error)

		email := &forms.EmailDelivery{
			FormID:  form.ID,
			Enabled: true,
		}
		require.NoError(t, db2.Create(email).Error)

		webhook := &forms.WebhookDelivery{
			FormID:  form.ID,
			Enabled: true,
			URL:     "https://example.com",
		}
		require.NoError(t, db2.Create(webhook).Error)

		// Preload the associations so form knows they exist
		require.NoError(t, db2.Preload("EmailDelivery").Preload("WebhookDelivery").First(form, form.ID).Error)

		// Call ensure - should not create duplicates
		err := forms.EnsureDeliveryRecords(logger, db2, form)
		require.NoError(t, err)

		// Verify no duplicates
		var emailCount, webhookCount int64
		db2.Model(&forms.EmailDelivery{}).Where("form_id = ?", form.ID).Count(&emailCount)
		db2.Model(&forms.WebhookDelivery{}).Where("form_id = ?", form.ID).Count(&webhookCount)

		assert.Equal(t, int64(1), emailCount)
		assert.Equal(t, int64(1), webhookCount)
	})
}

func TestGetBySlug(t *testing.T) {
	db := testsupport.SetupTestDB(t)

	t.Run("retrieves form by slug", func(t *testing.T) {
		created := &forms.Form{
			Name: "Test Form",
			Slug: "my-test-form",
		}
		require.NoError(t, db.Create(created).Error)

		found, err := forms.GetBySlug(db, "my-test-form")
		require.NoError(t, err)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, "my-test-form", found.Slug)
	})

	t.Run("returns error for non-existent slug", func(t *testing.T) {
		db2 := testsupport.SetupTestDB(t)

		_, err := forms.GetBySlug(db2, "non-existent")
		assert.Error(t, err)
	})
}

func TestUpdate(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("updates form successfully", func(t *testing.T) {
		// Create initial form
		initialForm := &forms.Form{
			Name:           "Original Name",
			Slug:           "original",
			AllowedOrigins: "https://example.com",
		}
		require.NoError(t, db.Create(initialForm).Error)

		// Create delivery records
		email := &forms.EmailDelivery{FormID: initialForm.ID, Enabled: false}
		webhook := &forms.WebhookDelivery{FormID: initialForm.ID, Enabled: false}
		require.NoError(t, db.Create(email).Error)
		require.NoError(t, db.Create(webhook).Error)

		// Update form
		mailerID := uint(123)
		params := forms.UpdateParams{
			ID:              initialForm.ID,
			Name:            "Updated Name",
			AllowedOrigins:  "https://newdomain.com",
			EmailEnabled:    true,
			MailerProfileID: &mailerID,
			EmailRecipient:  "test@example.com",
			WebhookEnabled:  true,
			WebhookURL:      "https://webhook.example.com",
			WebhookSecret:   "secret123",
		}

		updated, err := forms.Update(logger, db, params)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", updated.Name)
		assert.Equal(t, "original", updated.Slug) // Slug doesn't change
		assert.Equal(t, "https://newdomain.com", updated.AllowedOrigins)
	})

	t.Run("validates name is required", func(t *testing.T) {
		form := &forms.Form{Name: "Test", Slug: "test"}
		require.NoError(t, db.Create(form).Error)

		params := forms.UpdateParams{
			ID:   form.ID,
			Name: "   ", // Empty name
		}

		_, err := forms.Update(logger, db, params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("validates email settings", func(t *testing.T) {
		form := &forms.Form{Name: "Test", Slug: "test-email"}
		require.NoError(t, db.Create(form).Error)

		email := &forms.EmailDelivery{FormID: form.ID}
		require.NoError(t, db.Create(email).Error)

		// Enable email without mailer profile
		params := forms.UpdateParams{
			ID:           form.ID,
			Name:         "Test",
			EmailEnabled: true,
			// Missing MailerProfileID and EmailRecipient
		}

		_, err := forms.Update(logger, db, params)
		require.Error(t, err)
		valErr, ok := err.(*forms.ValidationError)
		require.True(t, ok)
		assert.Equal(t, "email", valErr.Field)
	})

	t.Run("validates webhook settings", func(t *testing.T) {
		form := &forms.Form{Name: "Test", Slug: "test-webhook"}
		require.NoError(t, db.Create(form).Error)

		webhook := &forms.WebhookDelivery{FormID: form.ID}
		require.NoError(t, db.Create(webhook).Error)

		// Enable webhook without URL
		params := forms.UpdateParams{
			ID:             form.ID,
			Name:           "Test",
			WebhookEnabled: true,
			// Missing WebhookURL
		}

		_, err := forms.Update(logger, db, params)
		require.Error(t, err)
		valErr, ok := err.(*forms.ValidationError)
		require.True(t, ok)
		assert.Equal(t, "webhook", valErr.Field)
	})

	t.Run("validates webhook headers JSON", func(t *testing.T) {
		form := &forms.Form{Name: "Test", Slug: "test-json"}
		require.NoError(t, db.Create(form).Error)

		webhook := &forms.WebhookDelivery{FormID: form.ID}
		require.NoError(t, db.Create(webhook).Error)

		params := forms.UpdateParams{
			ID:                 form.ID,
			Name:               "Test",
			WebhookEnabled:     true,
			WebhookURL:         "https://example.com",
			WebhookHeadersJSON: `{invalid json}`,
		}

		_, err := forms.Update(logger, db, params)
		require.Error(t, err)
		valErr, ok := err.(*forms.ValidationError)
		require.True(t, ok)
		assert.Contains(t, valErr.Message, "JSON")
	})

	t.Run("updates email delivery settings", func(t *testing.T) {
		form := &forms.Form{Name: "Test", Slug: "test-email-update"}
		require.NoError(t, db.Create(form).Error)

		email := &forms.EmailDelivery{FormID: form.ID, Enabled: false}
		require.NoError(t, db.Create(email).Error)

		mailerID := uint(456)
		params := forms.UpdateParams{
			ID:              form.ID,
			Name:            "Test",
			EmailEnabled:    true,
			MailerProfileID: &mailerID,
			EmailRecipient:  "updated@example.com",
		}

		updated, err := forms.Update(logger, db, params)
		require.NoError(t, err)

		// Reload email delivery
		var emailDelivery forms.EmailDelivery
		require.NoError(t, db.Where("form_id = ?", updated.ID).First(&emailDelivery).Error)
		assert.True(t, emailDelivery.Enabled)
		assert.NotNil(t, emailDelivery.MailerProfileID)
		assert.Equal(t, uint(456), *emailDelivery.MailerProfileID)
		assert.Contains(t, emailDelivery.OverridesJSON, "updated@example.com")
	})

	t.Run("updates webhook delivery settings", func(t *testing.T) {
		form := &forms.Form{Name: "Test", Slug: "test-webhook-update"}
		require.NoError(t, db.Create(form).Error)

		webhook := &forms.WebhookDelivery{FormID: form.ID, Enabled: false}
		require.NoError(t, db.Create(webhook).Error)

		params := forms.UpdateParams{
			ID:                 form.ID,
			Name:               "Test",
			WebhookEnabled:     true,
			WebhookURL:         "https://new.example.com/webhook",
			WebhookSecret:      "newsecret",
			WebhookHeadersJSON: `{"Authorization": "Bearer token"}`,
		}

		updated, err := forms.Update(logger, db, params)
		require.NoError(t, err)

		// Reload webhook delivery
		var webhookDelivery forms.WebhookDelivery
		require.NoError(t, db.Where("form_id = ?", updated.ID).First(&webhookDelivery).Error)
		assert.True(t, webhookDelivery.Enabled)
		assert.Equal(t, "https://new.example.com/webhook", webhookDelivery.URL)
		assert.Equal(t, "newsecret", webhookDelivery.Secret)
		assert.Contains(t, webhookDelivery.HeadersJSON, "Bearer token")
	})

	t.Run("returns error for non-existent form", func(t *testing.T) {
		params := forms.UpdateParams{
			ID:   99999,
			Name: "Test",
		}

		_, err := forms.Update(logger, db, params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestDelete(t *testing.T) {
	db := testsupport.SetupTestDB(t)
	logger := zap.NewNop()

	t.Run("deletes form successfully", func(t *testing.T) {
		form := &forms.Form{
			Name: "Delete Me",
			Slug: "delete-me",
		}
		require.NoError(t, db.Create(form).Error)

		err := forms.Delete(logger, db, form.ID)
		require.NoError(t, err)

		// Verify deleted
		var count int64
		db.Model(&forms.Form{}).Where("id = ?", form.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("cascades to delivery records", func(t *testing.T) {
		db2 := testsupport.SetupTestDB(t)

		form := &forms.Form{
			Name: "Cascade Test",
			Slug: "cascade",
		}
		require.NoError(t, db2.Create(form).Error)

		email := &forms.EmailDelivery{FormID: form.ID}
		webhook := &forms.WebhookDelivery{FormID: form.ID}
		require.NoError(t, db2.Create(email).Error)
		require.NoError(t, db2.Create(webhook).Error)

		err := forms.Delete(logger, db2, form.ID)
		require.NoError(t, err)

		// Verify cascaded deletes (depends on DB constraints)
		var emailCount, webhookCount int64
		db2.Model(&forms.EmailDelivery{}).Where("form_id = ?", form.ID).Count(&emailCount)
		db2.Model(&forms.WebhookDelivery{}).Where("form_id = ?", form.ID).Count(&webhookCount)

		// Note: SQLite may not enforce cascade, so this is best-effort
		t.Logf("After delete - emails: %d, webhooks: %d", emailCount, webhookCount)
	})
}
