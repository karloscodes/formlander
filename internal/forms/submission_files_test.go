package forms_test

import (
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"formlander/internal/forms"
	"formlander/internal/pkg/testsupport"
)

// filesUnder lists the regular files below dir.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	return found
}

func TestCreateSubmissionWithFiles(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	upload := func() []*forms.UploadedFile {
		return []*forms.UploadedFile{
			{FieldName: "cv", Filename: "cv.txt", Data: strings.NewReader("hello")},
			{FieldName: "photo", Filename: "photo.png", Data: strings.NewReader("png")},
		}
	}

	t.Run("stores the files in the submission's directory and leaves no staging behind", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		dataDir := t.TempDir()
		form := &forms.Form{Name: "Contact", Slug: "contact"}
		require.NoError(t, db.Create(form).Error)

		submission, err := forms.CreateSubmissionWithFiles(logger, db, form, map[string]any{"name": "Ada"}, "test", dataDir, upload())

		require.NoError(t, err)
		require.Len(t, submission.Files, 2)
		for _, f := range submission.Files {
			assert.Equal(t, submission.ID, f.SubmissionID)
			assert.True(t, strings.HasPrefix(f.StoragePath, filepath.Join("uploads", "1", "1")+string(filepath.Separator)), f.StoragePath)
		}
		body, err := os.ReadFile(forms.GetFilePath(dataDir, submission.Files[0]))
		require.NoError(t, err)
		assert.Equal(t, "hello", string(body))
		assert.Empty(t, filesUnder(t, filepath.Join(dataDir, "uploads", ".staging")))
		assert.Equal(t, int64(2), countRows(t, db, &forms.SubmissionFile{}))
	})

	t.Run("removes the files when the transaction fails", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		dataDir := t.TempDir()
		form := &forms.Form{Name: "Contact", Slug: "contact"}
		require.NoError(t, db.Create(form).Error)
		require.NoError(t, db.Exec(`CREATE TRIGGER fail_file BEFORE INSERT ON submission_files
			BEGIN SELECT RAISE(ABORT, 'disk full'); END`).Error)

		_, err := forms.CreateSubmissionWithFiles(logger, db, form, map[string]any{"name": "Ada"}, "test", dataDir, upload())

		require.Error(t, err)
		assert.Empty(t, filesUnder(t, filepath.Join(dataDir, "uploads")))
		assert.Equal(t, int64(0), countRows(t, db, &forms.Submission{}))
	})

	t.Run("writes into a directory that a reused submission ID left behind", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		dataDir := t.TempDir()
		form := &forms.Form{Name: "Contact", Slug: "contact"}
		require.NoError(t, db.Create(form).Error)
		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "uploads", "1", "1"), 0755))

		submission, err := forms.CreateSubmissionWithFiles(logger, db, form, map[string]any{"name": "Ada"}, "test", dataDir, upload())

		require.NoError(t, err)
		assert.FileExists(t, forms.GetFilePath(dataDir, submission.Files[1]))
	})
}
