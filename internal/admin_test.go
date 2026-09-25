package internal

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cartridgeconfig "github.com/karloscodes/cartridge/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"formlander/internal/accounts"
	"formlander/internal/config"
	"formlander/internal/pkg/testsupport"
)

func adminTestConfig(t *testing.T, env string) *config.Config {
	t.Helper()
	return &config.Config{Config: &cartridgeconfig.Config{
		Environment:   env,
		DataDirectory: t.TempDir(),
	}}
}

func readInitialPassword(t *testing.T, cfg *config.Config) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cfg.DataDirectory, accounts.InitialPasswordFile))
	require.NoError(t, err)
	return strings.TrimSpace(string(data))
}

func passwordWorks(t *testing.T, db *gorm.DB, password string) bool {
	t.Helper()
	admin, err := accounts.FindByEmail(db, accounts.DefaultAdminEmail)
	require.NoError(t, err)
	return bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)) == nil
}

func TestEnsureAdminUser(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("gives a new install a random password stored in the data directory", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		cfg := adminTestConfig(t, cartridgeconfig.Production)

		err := ensureAdminUser(db, cfg, logger)

		require.NoError(t, err)
		password := readInitialPassword(t, cfg)
		assert.NotEqual(t, accounts.DefaultAdminPassword, password)
		assert.True(t, passwordWorks(t, db, password))
		assert.False(t, passwordWorks(t, db, accounts.DefaultAdminPassword))
	})

	t.Run("replaces the public default password on an existing install", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		hash, err := bcrypt.GenerateFromPassword([]byte(accounts.DefaultAdminPassword), bcrypt.MinCost)
		require.NoError(t, err)
		require.NoError(t, db.Create(&accounts.User{Email: accounts.DefaultAdminEmail, PasswordHash: string(hash)}).Error)
		cfg := adminTestConfig(t, cartridgeconfig.Production)

		err = ensureAdminUser(db, cfg, logger)

		require.NoError(t, err)
		assert.False(t, passwordWorks(t, db, accounts.DefaultAdminPassword))
		assert.True(t, passwordWorks(t, db, readInitialPassword(t, cfg)))
	})

	t.Run("keeps a password the operator already changed", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		hash, err := bcrypt.GenerateFromPassword([]byte("operator-chosen"), bcrypt.MinCost)
		require.NoError(t, err)
		require.NoError(t, db.Create(&accounts.User{Email: accounts.DefaultAdminEmail, PasswordHash: string(hash)}).Error)
		cfg := adminTestConfig(t, cartridgeconfig.Production)

		err = ensureAdminUser(db, cfg, logger)

		require.NoError(t, err)
		assert.True(t, passwordWorks(t, db, "operator-chosen"))
		assert.False(t, accounts.HasInitialPassword(cfg.DataDirectory))
	})

	t.Run("uses the fixed password in the test environment", func(t *testing.T) {
		db := testsupport.SetupTestDB(t)
		cfg := adminTestConfig(t, cartridgeconfig.Test)

		err := ensureAdminUser(db, cfg, logger)

		require.NoError(t, err)
		assert.True(t, passwordWorks(t, db, accounts.DefaultAdminPassword))
		assert.False(t, accounts.HasInitialPassword(cfg.DataDirectory))
	})
}
