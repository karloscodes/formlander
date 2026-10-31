package forms_test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"formlander/internal/forms"
	"formlander/internal/integrations"
	"formlander/internal/pkg/testsupport"
)

func TestDelete(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("removes the form, its records, and its files when foreign keys are off", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		// Production leaves the foreign_keys pragma off, so a cascade on the
		// form row would not run. This is that database.
		require.Equal(t, 0, foreignKeys(t, db))
		assertDeleteRemovesTheForm(t, logger, db)
	})

	t.Run("removes them when foreign keys are on", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
		require.Equal(t, 1, foreignKeys(t, db))
		assertDeleteRemovesTheForm(t, logger, db)
	})

	t.Run("returns not found when the form does not exist", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)

		err := forms.Delete(logger, db, t.TempDir(), 0)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

		err = forms.Delete(logger, db, t.TempDir(), 42)
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("still removes the rows when no data directory is given", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		dataDir := t.TempDir()
		form := &forms.Form{Name: "Contact", Slug: "contact"}
		require.NoError(t, db.Create(form).Error)
		kept := filepath.Join(dataDir, "uploads", strconv.FormatUint(uint64(form.ID), 10), "note.txt")
		require.NoError(t, os.MkdirAll(filepath.Dir(kept), 0o755))
		require.NoError(t, os.WriteFile(kept, []byte("stay"), 0o644))

		err := forms.Delete(logger, db, "", form.ID)

		require.NoError(t, err)
		assert.FileExists(t, kept)
		assert.Equal(t, int64(0), countRows(t, db, &forms.Form{}))
	})
}

func foreignKeys(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var on int
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Scan(&on).Error)
	return on
}

func assertDeleteRemovesTheForm(t *testing.T, logger *slog.Logger, db *gorm.DB) {
	t.Helper()
	dataDir := t.TempDir()
	captcha := &integrations.CaptchaProfile{Name: "Turnstile", Provider: "turnstile", SecretKey: "secret"}
	hook := &integrations.WebhookProfile{Name: "Hook", URL: "https://example.com/hook"}
	mailer := &integrations.MailerProfile{Name: "Mail", Provider: "smtp", SMTPHost: "smtp.example.com"}
	require.NoError(t, db.Create(captcha).Error)
	require.NoError(t, db.Create(hook).Error)
	require.NoError(t, db.Create(mailer).Error)

	form := &forms.Form{Name: "Contact", Slug: "contact", CaptchaProfileID: &captcha.ID}
	require.NoError(t, db.Create(form).Error)
	require.NoError(t, db.Create(&forms.WebhookDelivery{FormID: form.ID, Enabled: true, WebhookProfileID: &hook.ID}).Error)
	require.NoError(t, db.Create(&forms.EmailDelivery{
		FormID: form.ID, Enabled: true, MailerProfileID: &mailer.ID, OverridesJSON: `{"to":"owner@example.com"}`,
	}).Error)
	require.NoError(t, db.Preload("WebhookDelivery").Preload("EmailDelivery").First(form, form.ID).Error)

	upload := &forms.UploadedFile{FieldName: "cv", Filename: "cv.txt", ContentType: "text/plain", Data: strings.NewReader("hello")}
	submission, err := forms.CreateSubmissionWithFiles(logger, db, form, map[string]any{"name": "Ada"}, "test", dataDir, []*forms.UploadedFile{upload})
	require.NoError(t, err)
	require.NotEmpty(t, submission.Files)
	stored := forms.GetFilePath(dataDir, submission.Files[0])
	require.FileExists(t, stored)
	_, err = forms.CreateSubmission(logger, db, form, map[string]any{"name": "Bot", forms.HoneypotField: "x"}, "test")
	require.NoError(t, err)

	// A file with no row, sitting next to the submission directory. Deleting
	// the known files alone would leave it, and the whole form directory has
	// to go.
	formDir := filepath.Join(dataDir, "uploads", strconv.FormatUint(uint64(form.ID), 10))
	orphan := filepath.Join(formDir, "orphan.txt")
	require.NoError(t, os.WriteFile(orphan, []byte("left behind"), 0o644))

	other := &forms.Form{Name: "Other", Slug: "other"}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Create(&forms.WebhookDelivery{FormID: other.ID, Enabled: true, WebhookProfileID: &hook.ID}).Error)
	require.NoError(t, db.Preload("WebhookDelivery").First(other, other.ID).Error)
	keptUpload := &forms.UploadedFile{FieldName: "note", Filename: "note.txt", ContentType: "text/plain", Data: strings.NewReader("keep")}
	keptSubmission, err := forms.CreateSubmissionWithFiles(logger, db, other, map[string]any{"name": "Grace"}, "test", dataDir, []*forms.UploadedFile{keptUpload})
	require.NoError(t, err)
	kept := forms.GetFilePath(dataDir, keptSubmission.Files[0])
	require.FileExists(t, kept)

	err = forms.Delete(logger, db, dataDir, form.ID)

	require.NoError(t, err)
	_, err = forms.GetByID(db, form.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.NoFileExists(t, stored)
	assert.NoFileExists(t, orphan)
	_, statErr := os.Stat(formDir)
	assert.True(t, os.IsNotExist(statErr))
	assert.FileExists(t, kept)

	assert.Equal(t, int64(1), countRows(t, db, &forms.Form{}))
	assert.Equal(t, int64(1), countRows(t, db, &forms.Submission{}))
	assert.Equal(t, int64(1), countRows(t, db, &forms.WebhookDelivery{}))
	assert.Equal(t, int64(0), countRows(t, db, &forms.EmailDelivery{}))
	assert.Equal(t, int64(1), countRows(t, db, &forms.WebhookEvent{}))
	assert.Equal(t, int64(0), countRows(t, db, &forms.EmailEvent{}))
	assert.Equal(t, int64(1), countRows(t, db, &forms.SubmissionFile{}))
	assert.Equal(t, int64(1), countRows(t, db, &integrations.CaptchaProfile{}))
	assert.Equal(t, int64(1), countRows(t, db, &integrations.WebhookProfile{}))
	assert.Equal(t, int64(1), countRows(t, db, &integrations.MailerProfile{}))

	var left forms.Submission
	require.NoError(t, db.First(&left).Error)
	assert.Equal(t, other.ID, left.FormID)
	assert.Equal(t, keptSubmission.ID, left.ID)
}
