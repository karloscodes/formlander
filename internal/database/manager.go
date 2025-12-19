package database

import (
	"fmt"
	"sync"
	"time"

	"log/slog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"formlander/internal/config"
	"formlander/internal/pkg/logger"
)

// Manager owns the SQLite connection lifecycle.
type Manager struct {
	cfg     *config.Config
	logger  *slog.Logger
	db      *gorm.DB
	dbOnce  sync.Once
	dbMutex sync.Mutex
	id      string // Unique ID for debugging
}

// NewManager constructs a database manager for the provided configuration and logger.
func NewManager(cfg *config.Config, log *slog.Logger) *Manager {
	return &Manager{
		cfg:    cfg,
		logger: log,
		id:     fmt.Sprintf("%p", &Manager{}), // Use memory address as unique ID
	}
}

// Connect returns a gorm.DB instance, opening the underlying connection on first use.
func (m *Manager) Connect() (*gorm.DB, error) {
	var err error
	m.dbOnce.Do(func() {
		err = m.open()
	})
	if err != nil {
		return nil, err
	}
	return m.db.Session(&gorm.Session{}), nil
}

// GetConnection returns a database connection (implements cartridge.DBManager interface).
// Returns nil if the connection is unavailable.
func (m *Manager) GetConnection() *gorm.DB {
	db, err := m.Connect()
	if err != nil {
		m.logger.Error("failed to get database connection", slog.Any("error", err))
		return nil
	}
	return db
}

// Close closes the underlying sql.DB connection.
func (m *Manager) Close() error {
	m.dbMutex.Lock()
	defer m.dbMutex.Unlock()

	if m.db == nil {
		return nil
	}

	sqlDB, err := m.db.DB()
	if err != nil {
		return fmt.Errorf("database: access sql.DB: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("database: close: %w", err)
	}

	m.db = nil
	m.dbOnce = sync.Once{}
	return nil
}

// CheckpointWAL forces a WAL checkpoint with the given mode.
func (m *Manager) CheckpointWAL(mode string) error {
	conn, err := m.Connect()
	if err != nil {
		return err
	}
	return conn.Exec("PRAGMA wal_checkpoint(" + mode + ");").Error
}

func (m *Manager) open() error {
	m.dbMutex.Lock()
	defer m.dbMutex.Unlock()

	if m.db != nil {
		return nil
	}

	dsn := m.cfg.DatabaseDSN() + "?_txlock=immediate"
	gormLogger := logger.NewGormLogger(m.logger.With(slog.String("component", "gorm")))

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:                 gormLogger,
		SkipDefaultTransaction: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return fmt.Errorf("database: open sqlite: %w", err)
	}

	if err := applySQLitePragmas(db, m.logger); err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("database: access sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(m.cfg.GetMaxOpenConns())
	sqlDB.SetMaxIdleConns(m.cfg.GetMaxIdleConns())
	sqlDB.SetConnMaxLifetime(10 * time.Minute)

	m.logger.Info("database connection established",
		slog.String("path", m.cfg.DatabaseDSN()),
		slog.Int("max_open", m.cfg.GetMaxOpenConns()),
		slog.Int("max_idle", m.cfg.GetMaxIdleConns()),
		slog.String("manager_id", m.id),
	)

	m.db = db
	return nil
}

func applySQLitePragmas(db *gorm.DB, log *slog.Logger) error {
	pragmas := []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
	}

	for _, pragma := range pragmas {
		if err := db.Exec(pragma).Error; err != nil {
			log.Error("failed to apply pragma", slog.String("pragma", pragma), slog.Any("error", err))
			return fmt.Errorf("database: apply pragma %s: %w", pragma, err)
		}
	}

	return nil
}
