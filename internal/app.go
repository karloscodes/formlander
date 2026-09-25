package internal

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/karloscodes/cartridge"

	"formlander/internal/accounts"
	"formlander/internal/config"
	"formlander/internal/database"
	httphandlers "formlander/internal/http"
	"formlander/internal/jobs"
	"formlander/internal/pkg/dbtxn"
	"formlander/internal/server"
	"formlander/web"
)

// App wraps the cartridge app with formlander-specific config.
type App struct {
	*cartridge.App
	Config *config.Config
}

// NewApp creates the formlander application.
func NewApp() (*App, error) {
	cfg := config.Get()

	app, err := cartridge.NewSSRApp("formlander",
		cartridge.WithConfig(cfg.Config),
		cartridge.WithAssets(web.Templates, web.Static),
		cartridge.WithTemplateFuncs(server.TemplateFuncs()),
		cartridge.WithErrorHandler(server.ErrorHandler(slog.Default(), cfg)),
		cartridge.WithSession("/admin/login"),
		cartridge.WithJobs(2*time.Minute,
			jobs.NewWebhookDispatcher(cfg),
			jobs.NewEmailDispatcher(cfg),
		),
		cartridge.WithRoutes(func(s *cartridge.Server) {
			MountRoutes(s, cfg)
		}),
	)
	if err != nil {
		return nil, err
	}

	return &App{App: app, Config: cfg}, nil
}

// RunMigrations runs database migrations and ensures admin user exists.
func RunMigrations(app *App) error {
	db, err := app.DBManager.Connect()
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	if err := database.Migrate(db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	if err := ensureAdminUser(db, app.Config, app.Logger); err != nil {
		return fmt.Errorf("ensure admin user: %w", err)
	}

	if err := httphandlers.UpgradeOldStarterHTML(app.Logger, db); err != nil {
		app.Logger.Warn("failed to upgrade old starter templates", slog.Any("error", err))
	}

	if err := app.DBManager.CheckpointWAL("FULL"); err != nil {
		app.Logger.Warn("failed to checkpoint WAL after migration", slog.Any("error", err))
	}

	return nil
}

// ensureAdminUser creates the admin on an empty install. Outside the test
// environment, the admin gets a random password, and an install that still
// uses the old public default password gets a new random one.
func ensureAdminUser(db *gorm.DB, cfg *config.Config, logger *slog.Logger) error {
	var count int64
	if err := db.Model(&accounts.User{}).Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		return createAdminUser(db, cfg, logger)
	}
	if !cfg.IsTest() && accounts.IsDefaultAdminActive(db) {
		return replaceDefaultPassword(db, cfg, logger)
	}
	return nil
}

func createAdminUser(db *gorm.DB, cfg *config.Config, logger *slog.Logger) error {
	password := accounts.DefaultAdminPassword
	if !cfg.IsTest() {
		password = accounts.GenerateInitialPassword()
		// Write the file before the database, so the operator never has a
		// password that is stored nowhere.
		if err := accounts.WriteInitialPassword(cfg.DataDirectory, password); err != nil {
			return fmt.Errorf("write initial admin password: %w", err)
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	admin := &accounts.User{
		Email:        accounts.DefaultAdminEmail,
		PasswordHash: string(hash),
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

	if !cfg.IsTest() {
		printInitialPassword(cfg, "Admin user created", password)
	}
	return nil
}

func replaceDefaultPassword(db *gorm.DB, cfg *config.Config, logger *slog.Logger) error {
	admin, err := accounts.FindByEmail(db, accounts.DefaultAdminEmail)
	if err != nil {
		return err
	}

	password := accounts.GenerateInitialPassword()
	if err := accounts.WriteInitialPassword(cfg.DataDirectory, password); err != nil {
		return fmt.Errorf("write initial admin password: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	admin.PasswordHash = string(hash)

	if err := dbtxn.WithRetry(logger, db, func(tx *gorm.DB) error {
		return tx.Save(admin).Error
	}); err != nil {
		logger.Error("failed to replace default admin password", slog.Any("error", err))
		return err
	}

	logger.Warn("replaced the public default admin password with a random one")
	printInitialPassword(cfg, "Default admin password replaced (the old one is public)", password)
	return nil
}

// printInitialPassword writes to stdout only, so the password reaches the
// container logs but not the log file.
func printInitialPassword(cfg *config.Config, title, password string) {
	fmt.Printf("\n🔐 %s:\n", title)
	fmt.Printf("   Email:    %s\n", accounts.DefaultAdminEmail)
	fmt.Printf("   Password: %s\n", password)
	fmt.Printf("   Also in:  %s\n", filepath.Join(cfg.DataDirectory, accounts.InitialPasswordFile))
	fmt.Printf("   Change it in Settings after you sign in.\n\n")
}

// GetDB returns the database instance.
func (a *App) GetDB() *gorm.DB {
	db, _ := a.DBManager.Connect()
	return db
}
