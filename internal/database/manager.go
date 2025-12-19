package database

import (
	"log/slog"

	"github.com/karloscodes/cartridge"

	"formlander/internal/config"
)

// Manager wraps cartridge.SQLiteManager with formlander config.
type Manager struct {
	*cartridge.SQLiteManager
}

// NewManager creates a database manager using cartridge's SQLiteManager.
func NewManager(cfg *config.Config, log *slog.Logger) *Manager {
	return &Manager{
		SQLiteManager: cartridge.NewSQLiteManager(cartridge.SQLiteConfig{
			Path:         cfg.DatabaseDSN(),
			MaxOpenConns: cfg.GetMaxOpenConns(),
			MaxIdleConns: cfg.GetMaxIdleConns(),
			Logger:       log,
		}),
	}
}
