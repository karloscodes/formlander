package internal

import (
	"context"
	"fmt"
	"time"

	"log/slog"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"formlander/internal/accounts"
	"formlander/internal/auth"
	"formlander/internal/config"
	"formlander/internal/database"
	"formlander/internal/jobs"
	"formlander/internal/server"
	"formlander/internal/pkg/dbtxn"
	"formlander/web"
)

// App wraps the cartridge application with background workers.
type App struct {
	*server.Application
	dispatcher *jobs.UnifiedDispatcher
}

// AppOptions configures application initialization
type AppOptions struct {
	TemplatesDirectory string
}

// NewApp creates the application using cartridge defaults.
func NewApp() (*App, error) {
	return NewAppWithOptions(nil)
}

// NewAppWithOptions creates the application with custom options
func NewAppWithOptions(opts *AppOptions) (*App, error) {
	cfg := config.Get()

	auth.Initialize(cfg)

	cartridgeOpts := server.ApplicationOptions{
		Config:         cfg,
		RouteMountFunc: MountRoutes,
	}

	// Only use embedded assets in production
	if !cfg.IsDevelopment() {
		cartridgeOpts.TemplatesFS = web.Templates
		cartridgeOpts.StaticFS = web.Static
	} else if opts != nil && opts.TemplatesDirectory != "" {
		// Use custom template directory in development
		cartridgeOpts.TemplatesDirectory = opts.TemplatesDirectory
	}

	application, err := server.NewApplication(cartridgeOpts)
	if err != nil {
		return nil, err
	}

	return &App{
		Application: application,
		dispatcher:  jobs.NewUnifiedDispatcher(cfg, application.Logger, application.DBManager),
	}, nil
}

// RunMigrations runs database migrations and ensures admin user exists.
func RunMigrations(app *App) error {
	return runMigrations(app.Application, app.Config)
}

func runMigrations(app *server.Application, cfg *config.Config) error {
	db, err := app.DBManager.Connect()
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	// Run migrations
	if err := database.Migrate(db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	if err := ensureAdminUser(db, cfg, app.Logger); err != nil {
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

// Start begins the HTTP server and unified dispatcher.
func (a *App) Start() error {
	if err := a.dispatcher.Start(); err != nil {
		return err
	}
	if err := a.Application.Start(); err != nil {
		a.dispatcher.Stop()
		return err
	}
	return nil
}

// StartAsync starts the components asynchronously.
func (a *App) StartAsync() error {
	if err := a.dispatcher.Start(); err != nil {
		return err
	}
	if err := a.Application.StartAsync(); err != nil {
		a.dispatcher.Stop()
		return err
	}
	return nil
}

// Shutdown gracefully stops background workers and the HTTP server.
func (a *App) Shutdown(ctx context.Context) error {
	a.dispatcher.Stop()
	return a.Application.Shutdown(ctx)
}

// GetConfig returns the application configuration.
func (a *App) GetConfig() *config.Config {
	return a.Application.Config
}

// GetDB returns the database instance.
func (a *App) GetDB() *gorm.DB {
	db, _ := a.Application.DBManager.Connect()
	return db
}
