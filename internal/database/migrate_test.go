package database_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"formlander/internal/database"
)

func TestMigrate(t *testing.T) {
	t.Run("the newest submissions of a form come from an index", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "formlander.db")), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, database.Migrate(db))

		var plan []struct {
			Detail string `gorm:"column:detail"`
		}
		err = db.Raw("EXPLAIN QUERY PLAN SELECT * FROM submissions WHERE form_id = ? ORDER BY created_at DESC LIMIT 20", 1).Scan(&plan).Error

		require.NoError(t, err)
		for _, step := range plan {
			assert.NotContains(t, step.Detail, "TEMP B-TREE", "the dashboard list must not sort every submission of a form")
		}
	})
}
