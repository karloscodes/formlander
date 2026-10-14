package forms_test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"formlander/internal/forms"
	"formlander/internal/pkg/testsupport"
)

func countRows(t *testing.T, db *gorm.DB, model any) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(model).Count(&count).Error)
	return count
}

func TestDeleteSubmissions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("removes the submission, its delivery attempts, and its files", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		dataDir := t.TempDir()
		form := &forms.Form{Name: "Contact", Slug: "contact"}
		require.NoError(t, db.Create(form).Error)
		require.NoError(t, db.Create(&forms.WebhookDelivery{FormID: form.ID, Enabled: true, WebhookProfileID: webhookProfile(t, db)}).Error)
		require.NoError(t, db.Preload("WebhookDelivery").First(form, form.ID).Error)
		upload := &forms.UploadedFile{FieldName: "cv", Filename: "cv.txt", Data: strings.NewReader("hello")}
		submission, err := forms.CreateSubmissionWithFiles(logger, db, form, map[string]any{"name": "Ada"}, "test", dataDir, []*forms.UploadedFile{upload})
		require.NoError(t, err)
		kept, err := forms.CreateSubmission(logger, db, form, map[string]any{"name": "Grace"}, "test")
		require.NoError(t, err)
		stored := forms.GetFilePath(dataDir, submission.Files[0])
		require.FileExists(t, stored)

		deleted, err := forms.DeleteSubmissions(logger, db, dataDir, []uint{submission.ID})

		require.NoError(t, err)
		assert.Equal(t, 1, deleted)
		assert.NoFileExists(t, stored)
		_, statErr := os.Stat(filepath.Dir(stored))
		assert.True(t, os.IsNotExist(statErr), "the upload directory of the submission is gone")
		assert.Equal(t, int64(0), countRows(t, db, &forms.SubmissionFile{}))
		assert.Equal(t, int64(1), countRows(t, db, &forms.WebhookEvent{}), "the event of the other submission stays")
		var left []forms.Submission
		require.NoError(t, db.Find(&left).Error)
		require.Len(t, left, 1)
		assert.Equal(t, kept.ID, left[0].ID)
	})

	t.Run("skips an ID that does not exist", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)

		deleted, err := forms.DeleteSubmissions(logger, db, t.TempDir(), []uint{42})

		require.NoError(t, err)
		assert.Equal(t, 0, deleted)
	})
}

func TestDeleteSpam(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	seed := func(t *testing.T, db *gorm.DB, slug string) *forms.Form {
		t.Helper()
		form := &forms.Form{Name: slug, Slug: slug}
		require.NoError(t, db.Create(form).Error)
		_, err := forms.CreateSubmission(logger, db, form, map[string]any{"name": "Ada"}, "test")
		require.NoError(t, err)
		_, err = forms.CreateSubmission(logger, db, form, map[string]any{"name": "Bot", forms.HoneypotField: "x"}, "test")
		require.NoError(t, err)
		return form
	}

	t.Run("removes the spam of every form and keeps the rest", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		seed(t, db, "one")
		seed(t, db, "two")

		deleted, err := forms.DeleteSpam(logger, db, t.TempDir(), 0)

		require.NoError(t, err)
		assert.Equal(t, 2, deleted)
		var spam int64
		db.Model(&forms.Submission{}).Where("is_spam = ?", true).Count(&spam)
		assert.Equal(t, int64(0), spam)
		assert.Equal(t, int64(2), countRows(t, db, &forms.Submission{}))
	})

	t.Run("with a form, removes only the spam of that form", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		one := seed(t, db, "one")
		seed(t, db, "two")

		deleted, err := forms.DeleteSpam(logger, db, t.TempDir(), one.ID)

		require.NoError(t, err)
		assert.Equal(t, 1, deleted)
		assert.Equal(t, int64(3), countRows(t, db, &forms.Submission{}))
	})
}
