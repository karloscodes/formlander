package database

import (
	"log/slog"

	"github.com/karloscodes/cartridge/sqlite"

	"formlander/internal/config"
)

// Manager wraps sqlite.Manager with formlander config.
type Manager struct {
	*sqlite.Manager
}

// NewManager creates a database manager using cartridge's sqlite.Manager.
func NewManager(cfg *config.Config, log *slog.Logger) *Manager {
	return &Manager{
		Manager: sqlite.NewManager(sqlite.Config{
			Path:         cfg.DatabaseDSN(),
			MaxOpenConns: cfg.GetMaxOpenConns(),
			MaxIdleConns: cfg.GetMaxIdleConns(),
			Logger:       log,
		}),
	}
}
