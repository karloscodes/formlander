package internal

import (
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/karloscodes/cartridge"

	"formlander/internal/accounts"
	"formlander/internal/auth"
	"formlander/internal/config"
	"formlander/internal/database"
	"formlander/internal/jobs"
	"formlander/internal/pkg/dbtxn"
	"formlander/internal/server"
	"formlander/web"
)

// App wraps the cartridge application with formlander-specific components.
type App struct {
	*cartridge.Application
	Config    *config.Config
	Logger    *slog.Logger
	DBManager *database.Manager
}

// AppOptions configures application initialization.
type AppOptions struct {
	TemplatesDirectory string
}

// NewApp creates the application using cartridge defaults.
func NewApp() (*App, error) {
	return NewAppWithOptions(nil)
}

// NewAppWithOptions creates the application with custom options.
func NewAppWithOptions(opts *AppOptions) (*App, error) {
	cfg := config.Get()

	// Initialize auth
	auth.Initialize(cfg)

	// Initialize logger using cartridge
	logger := cartridge.NewLogger(cfg, &cartridge.LogConfig{
		Level:      string(cfg.LogLevel),
		Directory:  cfg.LogsDirectory,
		MaxSizeMB:  cfg.LogsMaxSizeInMB,
		MaxBackups: cfg.LogsMaxBackups,
		MaxAgeDays: cfg.LogsMaxAgeInDays,
		AppName:    cfg.AppName,
	})
	slog.SetDefault(logger)

	// Initialize database manager
	dbManager := database.NewManager(cfg, logger)

	// Build server configuration
	serverCfg := server.ServerConfig{
		Config:    cfg,
		Logger:    logger,
		DBManager: dbManager,
	}

	// Configure assets based on environment
	if !cfg.IsDevelopment() {
		serverCfg.TemplatesFS = web.Templates
		serverCfg.StaticFS = web.Static
	} else if opts != nil && opts.TemplatesDirectory != "" {
		serverCfg.TemplatesDirectory = opts.TemplatesDirectory
	}

	// Create formlander server
	srv, err := server.NewServer(serverCfg)
	if err != nil {
		return nil, fmt.Errorf("create server: %w", err)
	}

	// Mount routes
	MountRoutes(srv)

	// Create jobs dispatcher as a background worker
	dispatcher := jobs.NewUnifiedDispatcher(cfg, logger, dbManager)

	// Create cartridge application with background worker
	application, err := cartridge.NewApplication(cartridge.ApplicationOptions{
		Config:            cfg,
		Logger:            logger,
		DBManager:         dbManager,
		BackgroundWorkers: []cartridge.BackgroundWorker{dispatcher},
		ServerConfig: &cartridge.ServerConfig{
			Config:    cfg,
			Logger:    logger,
			DBManager: dbManager,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create application: %w", err)
	}

	// Replace the cartridge server with our formlander server
	application.Server = srv

	return &App{
		Application: application,
		Config:      cfg,
		Logger:      logger,
		DBManager:   dbManager,
	}, nil
}

// RunMigrations runs database migrations and ensures admin user exists.
func RunMigrations(app *App) error {
	db, err := app.DBManager.Connect()
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	// Run migrations
	if err := database.Migrate(db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	if err := ensureAdminUser(db, app.Config, app.Logger); err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}

	// Checkpoint WAL to ensure migrations are persisted
	if err := app.DBManager.CheckpointWAL("FULL"); err != nil {
		app.Logger.Warn("failed to checkpoint WAL after migration", slog.Any("error", err))
	}

	return nil
}

func ensureAdminUser(db *gorm.DB, cfg *config.Config, logger *slog.Logger) error {
	var count int64
	if err := db.Model(&accounts.User{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	// Create default admin user with temporary password
	defaultEmail := "admin@formlander.local"
	defaultPassword := "formlander"

	hash, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	admin := &accounts.User{
		Email:        defaultEmail,
		PasswordHash: string(hash),
		LastLoginAt:  nil, // nil = first login, will force password change
	}

	if cfg.IsTest() {
		now := time.Now()
		admin.LastLoginAt = &now
	}

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Create(admin).Error
	}); err != nil {
		logger.Error("failed to create default admin user", slog.Any("error", err))
		return err
	}

	fmt.Printf("\n🔐 Default admin user created:\n")
	fmt.Printf("   Email: %s\n", defaultEmail)
	// Intentionally logging default password during initial setup - must be changed on first login
	// codeql[go/clear-text-logging]
	fmt.Printf("   Temporary credentials: %s\n", defaultPassword)
	fmt.Printf("   ⚠️  You will be required to change this on first login\n\n")

	return nil
}

// GetDB returns the database instance.
func (a *App) GetDB() *gorm.DB {
	db, _ := a.DBManager.Connect()
	return db
}
